package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// Options are where the adapter reads and what it runs; the zero values are
// the real ones.
type Options struct {
	StatsPath string                 // edge's stats.json
	Action    func(key string) error // runs `edge action <key>`; its error is what edge said
	Now       func() time.Time       // the clock
	Every     time.Duration          // how often time-worded things are refreshed (5 s)
	Read      func(string) ([]byte, error)
}

// StatsPath is where edge keeps its live stats: $XDG_RUNTIME_DIR/edge/stats.json.
func StatsPath() string {
	d := os.Getenv("XDG_RUNTIME_DIR")
	if d == "" {
		d = os.TempDir()
	}
	return d + "/edge/stats.json"
}

// EdgeAction runs `edge action <key>` and waits for it: edge prints a
// plain sentence to stderr and exits 1 when it can't do it.
func EdgeAction(key string) error {
	if _, err := exec.LookPath("edge"); err != nil {
		return errors.New("edge isn't installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "edge", "action", key)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if line == "" {
			line = "edge couldn't do that"
		}
		return errors.New(line)
	}
	return nil
}

// quiet is how long edge may say nothing before it counts as stopped (it
// says its status at least once a minute), and how long without a push
// before the stats file is read instead.
const (
	quiet    = 90 * time.Second
	fallback = 60 * time.Second
)

// adapter is the plugin's state: what edge last said, and what Ember was last sent.
type adapter struct {
	p      *plugin.Plugin
	o      Options
	status *Status
	reread func() // the stats file, read now (set by run; a "stats" ping without a body asks for it)
	stats  *Stats
	heard  time.Time         // edge's last word (a push)
	open   bool              // the dashboard is open
	sent   map[string][]byte // the last of each kind: only a change is sent again
	press  chan string       // presses, run one at a time: edge's actions change its state
	acted  chan error        // what each came to
}

// Run serves edge's plugin until Ember says stop.
func Run(p *plugin.Plugin, o Options) error {
	if o.StatsPath == "" {
		o.StatsPath = StatsPath()
	}
	if o.Action == nil {
		o.Action = EdgeAction
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Every == 0 {
		o.Every = 5 * time.Second
	}
	if o.Read == nil {
		o.Read = os.ReadFile
	}
	a := &adapter{p: p, o: o, sent: map[string][]byte{}, press: make(chan string, 8), acted: make(chan error, 8), heard: o.Now()}
	go func() {
		for k := range a.press {
			select {
			case a.acted <- o.Action(k):
			case <-p.Context().Done():
				return
			}
		}
	}()
	defer close(a.press)
	var seen time.Time
	poll := func() { // the file, at start and when edge's pushes stop: edge rewrites it atomically, only on a change
		fi, err := os.Stat(o.StatsPath)
		if err != nil || fi.ModTime().Equal(seen) {
			return
		}
		if b, err := o.Read(o.StatsPath); err == nil {
			var st Stats
			if json.Unmarshal(b, &st) == nil && st.Version >= 1 {
				seen, a.stats = fi.ModTime(), &st
				a.show()
			}
		}
	}
	a.reread = poll
	poll()
	a.show()
	statsT, refresh := time.NewTicker(fallback), time.NewTicker(o.Every)
	defer statsT.Stop()
	defer refresh.Stop()
	for {
		select {
		case ev, ok := <-p.Events():
			if !ok {
				return nil
			}
			a.event(ev)
		case err := <-a.acted:
			if err != nil {
				a.oops(err.Error())
			}
		case <-statsT.C:
			if a.o.Now().Sub(a.heard) >= fallback { // no push lately: edge's channel is the push, the file only covers for it
				poll()
			}
		case <-refresh.C:
			a.show()
		}
	}
}

func (a *adapter) event(ev plugin.Event) {
	switch ev.Type {
	case "tool":
		var m Message
		if json.Unmarshal(ev.Payload, &m) == nil {
			a.message(m)
		}
	case "button":
		if k := ev.Key; k != "" && k != "dismiss" {
			select {
			case a.press <- k:
			default: // a burst of presses: the rest are dropped
			}
		}
	case "dash":
		a.open = ev.State == "open"
		a.show()
	}
}

// oops says, for 5 s, why a press did nothing.
func (a *adapter) oops(why string) {
	if a.p.Can(plugin.CapMoment) {
		a.p.Moment(plugin.Moment{Text: why, Mood: plugin.Fail, TTLs: 5})
		return
	}
	a.p.Card(plugin.Card{ID: "edge:oops", Eyebrow: "Practice", Title: why, Mood: plugin.Fail, TTLs: 5})
}

// message is one of edge's messages.
func (a *adapter) message(m Message) {
	switch m.Type {
	case "status":
		if m.Status == nil {
			return
		}
		a.heard = a.o.Now()
		for _, c := range News(a.status, m.Status) {
			a.p.Cheer(c)
		}
		a.status = m.Status
		a.show()
	case "stats":
		switch {
		case m.Stats != nil && m.Stats.Version >= 1:
			a.heard = a.o.Now()
			a.stats = m.Stats
			a.show()
		case m.Stats == nil && a.reread != nil: // a ping: the stats are too big to push, read the file
			a.heard = a.o.Now()
			a.reread()
		}
	case "card":
		if m.Card != nil {
			c, bounce := Cardify(m.Card)
			a.p.Card(c)
			if bounce {
				a.p.Cheer("hop")
			}
		}
	case "remove":
		a.p.RemoveCard(m.ID)
	}
}

// send sends v as kind unless it's what Ember already has.
func (a *adapter) send(kind string, v any, do func() error) {
	b, _ := json.Marshal(v)
	if bytes.Equal(b, a.sent[kind]) {
		return
	}
	if do() == nil {
		a.sent[kind] = b
	}
}

// view is edge's word as of now: its last status, else its stats; and
// whether it has gone quiet (then nothing runs on the clock).
func (a *adapter) view(now time.Time) *Status {
	var e Status
	switch {
	case a.status != nil:
		e = *a.status
	case a.stats != nil:
		e = *FromStats(a.stats)
	default:
		return nil
	}
	heard := a.heard
	if e.SeenMs > 0 && time.UnixMilli(e.SeenMs).After(heard) {
		heard = time.UnixMilli(e.SeenMs)
	}
	if now.Sub(heard) > quiet {
		e.Stopped = true
	}
	return &e
}

// show sends what changed: the line, the row, and the dashboard (once, so
// Ember has it, then only while it's open).
func (a *adapter) show() {
	now := a.o.Now()
	e := a.view(now)
	if l := Line(e, now); l != nil {
		a.send("status", l, func() error { return a.p.Status(*l) })
	} else if a.sent["status"] != nil && string(a.sent["status"]) != "null" {
		a.send("status", nil, a.p.ClearStatus)
	}
	if e == nil {
		return // edge hasn't said anything yet
	}
	r := Row(e, now)
	a.send("row", r, func() error { return a.p.Row(r) })
	if a.stats != nil && (a.open || a.sent["dashboard"] == nil) {
		st := *a.stats
		if n := len(st.Calendar); n > 26*7+7 {
			st.Calendar = st.Calendar[n-26*7-7:]
		}
		d := DashboardFor(&st, e, now)
		a.send("dashboard", d, func() error { return a.p.Dashboard(d) })
	}
}
