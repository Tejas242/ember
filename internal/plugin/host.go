// Package plugin is Ember's plugin host: it finds the plugins in
// ~/.config/ember/plugins, starts each one, speaks protocol v1 with it
// (package pkg/plugin, docs/PLUGINS.md), checks and limits everything it
// sends, restarts it with backoff when it exits, suspends one that floods or
// keeps crashing, and hands what it may show to the island (a Sink), which
// draws it on its own loop. The host never touches the island's state.
package plugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
)

// Sink is the island's side. Both are called from the host's goroutines,
// for one plugin at a time in order; the island posts them to its loop.
type Sink interface {
	Apply(name string, m proto.Message) // something checked and within its limits, to show
	Gone(name string)                   // the plugin stopped: take back everything it showed
}

// States a plugin can be in.
const (
	Starting  = "starting"  // started, waiting for its hello
	Running   = "running"   // said hello
	Waiting   = "waiting"   // exited; starts again after a pause
	Suspended = "suspended" // flooded or kept crashing: stopped until its manifest changes or Ember restarts
	Stopped   = "stopped"   // the island stopped it
	Off       = "off"       // the config turns it off
	Broken    = "broken"    // its manifest can't be used
)

// Info is how a plugin is doing, for `ember plugins` and doctor.
type Info struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Icon        string    `json:"icon,omitempty"`
	State       string    `json:"state"`
	Caps        []string  `json:"caps,omitempty"`
	Opens       string    `json:"opens,omitempty"` // [opens] ember: the context a click on Ember opens it in
	Hint        string    `json:"hint,omitempty"`
	Grants      []string  `json:"grants,omitempty"` // what the user's config grants it
	Asks        []string  `json:"asks,omitempty"`   // what its manifest asks for
	Since       time.Time `json:"since"`
	Restarts    int       `json:"restarts"`
	Dropped     int       `json:"dropped"`    // over a rate limit, quietly
	Violations  int       `json:"violations"` // malformed, oversized, or beyond its capabilities
	LastError   string    `json:"lastError,omitempty"`
	Log         []string  `json:"log,omitempty"` // its last stderr and log lines
	Dir         string    `json:"dir,omitempty"`
}

// Host runs the plugins in Dir.
type Host struct {
	Dir      string
	Version  string // Ember's, for the hello
	Sink     Sink
	Enabled  func(name string) bool     // the config's say; nil: all on
	Reserved func(name string) bool     // a name a built-in plugin has; nil: none
	Grants   func(name string) []string // the config's grants, for Info; nil: none

	mu     sync.Mutex
	ctx    context.Context
	ps     map[string]*proc
	broken map[string]Info
	wg     sync.WaitGroup
}

// NewHost is a host for the plugins in dir.
func NewHost(dir, version string, sink Sink) *Host {
	return &Host{Dir: dir, Version: version, Sink: sink, ps: map[string]*proc{}, broken: map[string]Info{}}
}

// Start runs the plugins found now; Scan finds later ones. They stop when
// ctx ends (Stop waits for that).
func (h *Host) Start(ctx context.Context) {
	h.mu.Lock()
	h.ctx = ctx
	h.mu.Unlock()
	h.Scan()
}

// Scan matches the running plugins to the manifests and the config: it
// starts new ones, stops removed or disabled ones, and restarts one whose
// manifest changed (which also lifts a suspension).
func (h *Host) Scan() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ctx == nil || h.ctx.Err() != nil {
		return
	}
	found := map[string]*Manifest{}
	h.broken = map[string]Info{}
	entries, _ := os.ReadDir(h.Dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(h.Dir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "plugin.toml")); err != nil {
			continue
		}
		m, err := LoadManifest(dir)
		switch {
		case err != nil:
			h.broken[e.Name()] = Info{Name: e.Name(), State: Broken, LastError: err.Error(), Dir: dir}
		case h.Reserved != nil && h.Reserved(m.Name):
			h.broken[m.Name] = Info{Name: m.Name, State: Broken, LastError: "the name of one of Ember's own plugins", Dir: dir}
		case h.Enabled != nil && !h.Enabled(m.Name):
			h.broken[m.Name] = Info{Name: m.Name, Description: m.Description, Icon: m.Icon, State: Off, Dir: dir}
		default:
			found[m.Name] = m
		}
	}
	for name, p := range h.ps {
		if m, ok := found[name]; !ok || !m.Mod.Equal(p.m.Mod) || m.Dir != p.m.Dir {
			p.cancel()
			delete(h.ps, name)
		}
	}
	defer func() { // what the config grants each, for its hello (Grants reads the config: on the caller's goroutine)
		for name, p := range h.ps {
			var g []string
			if h.Grants != nil {
				g = h.Grants(name)
			}
			p.mu.Lock()
			p.grants = g
			p.mu.Unlock()
		}
	}()
	for name, m := range found {
		if _, ok := h.ps[name]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(h.ctx)
		p := newProc(h, m, cancel)
		h.ps[name] = p
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			p.supervise(ctx)
		}()
	}
}

// Stop waits (a few seconds at most) for every plugin to stop, once the
// context Start had has ended.
func (h *Host) Stop() {
	done := make(chan struct{})
	go func() { h.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func (h *Host) proc(name string) *proc {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ps[name]
}

// Has says whether a plugin by that name is installed and on.
func (h *Host) Has(name string) bool { return h.proc(name) != nil }

// Send tells a plugin about an event. It never blocks: a plugin that isn't
// reading misses it. A media event goes only to a plugin granted media whose
// page is open (as the dash events it was sent say).
func (h *Host) Send(name string, ev proto.Event) {
	p := h.proc(name)
	if p == nil {
		return
	}
	switch ev.Type {
	case "button":
		p.cardClosed(ev.Card)
	case "dash":
		p.setOpen(ev.State == "open")
	case "media":
		if !p.open() || !h.granted(name, proto.GrantMedia) {
			return
		}
	}
	p.send(ev)
}

func (h *Host) granted(name, grant string) bool {
	return h.Grants != nil && slices.Contains(h.Grants(name), grant)
}

// Opener is the plugin whose page a click on Ember opens in a context
// (music, idle), and its hint: the first by name that asks for the context
// in its manifest and is granted entry. "" if none.
func (h *Host) Opener(context string) (name, hint string) {
	h.mu.Lock()
	var names []string
	for n, p := range h.ps {
		if p.m.Opens == context {
			names = append(names, n)
		}
	}
	h.mu.Unlock()
	sort.Strings(names)
	for _, n := range names {
		if h.granted(n, proto.GrantEntry) {
			p := h.proc(n)
			if p == nil {
				continue
			}
			hint := p.m.Hint
			if hint == "" {
				hint = n
			}
			return n, hint
		}
	}
	return "", ""
}

// CardClosed: one of the plugin's cards went without a button (dismissed,
// expired), so it no longer counts toward its open cards.
func (h *Host) CardClosed(name, id string) {
	if p := h.proc(name); p != nil {
		p.cardClosed(id)
	}
}

// Click does what a click on a plugin's moment or row asks: the manifest's
// target (a URL opened, a command run), then tells the plugin.
func (h *Host) Click(name, target string) {
	p := h.proc(name)
	if p == nil {
		return
	}
	if t, ok := p.m.Targets[target]; ok {
		openTarget(t, p.m.Dir)
	}
	if target == "" {
		target = "row"
	}
	p.send(proto.Event{Type: "click", Target: target})
}

// openTarget is Open; tests replace it.
var openTarget = Open

// Open opens a URL (xdg-open) or runs a command (no shell) in dir, without
// waiting.
func Open(t []string, dir string) {
	var cmd *exec.Cmd
	switch {
	case len(t) == 0:
		return
	case IsURL(t):
		cmd = exec.Command("xdg-open", t[0])
	default:
		cmd = exec.Command(t[0], t[1:]...)
		cmd.Dir = dir
	}
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

// Info is every plugin's state, by name.
func (h *Host) Info() []Info {
	h.mu.Lock()
	var out []Info
	for _, p := range h.ps {
		i := p.snapshot()
		if h.Grants != nil {
			i.Grants = h.Grants(i.Name)
		}
		out = append(out, i)
	}
	for _, b := range h.broken {
		out = append(out, b)
	}
	h.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
