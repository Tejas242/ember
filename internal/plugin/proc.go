package plugin

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
)

// The supervision and the limits (docs/PLUGINS.md, Limits).
var (
	backoffMin   = time.Second
	backoffMax   = 5 * time.Minute
	healthyRun   = time.Minute // a run this long starts the backoff over
	crashWindow  = 10 * time.Minute
	crashLimit   = 5
	helloWithin  = 5 * time.Second
	stopGrace    = 2 * time.Second
	floodRate    = 100 // messages a second…
	floodSecs    = 3   // …for this many seconds in a row
	violationMax = 50  // within a minute
)

// Rates per kind: at most one per interval; status, row and dashboard are
// coalesced (the latest wins), the rest are dropped.
var interval = map[string]time.Duration{
	"status": 100 * time.Millisecond, "row": 100 * time.Millisecond, "dashboard": 250 * time.Millisecond,
	"moment": 10 * time.Second, "card": time.Second,
	"cheer:hop": 5 * time.Second, "cheer:celebrate": time.Hour, // hop and nod share theirs: reactions
}

const logRate = 5 // log lines a second

const maxCards = 3
const logLines = 20

var errSuspended = errors.New("suspended")

// proc is one plugin: its manifest, its process when it runs, its limits.
type proc struct {
	h      *Host
	m      *Manifest
	cancel context.CancelFunc

	out chan []byte // to the plugin; full means it isn't reading

	mu      sync.Mutex // the limits, the pending updates and info; held while applying, to keep order
	info    Info
	last    map[string]time.Time // when each kind was last applied
	pending map[string]proto.Message
	timer   *time.Timer
	cards   map[string]time.Time // open cards: when each expires
	viol    []time.Time
	logSec  time.Time
	logN    int
	alive   bool     // a run is going: applying is allowed
	shown   bool     // its page is open (the last dash event it was sent)
	grants  []string // the config's grants as of the last Scan (on the island's loop): its hello says them
}

func newProc(h *Host, m *Manifest, cancel context.CancelFunc) *proc {
	return &proc{h: h, m: m, cancel: cancel, out: make(chan []byte, 32),
		info: Info{Name: m.Name, Description: m.Description, Icon: m.Icon, Caps: m.Caps, Asks: m.Asks, Opens: m.Opens, Hint: m.Hint,
			State: Starting, Since: time.Now(), Dir: m.Dir}}
}

func (p *proc) setOpen(on bool) {
	p.mu.Lock()
	p.shown = on
	p.mu.Unlock()
}

func (p *proc) open() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.shown && p.alive
}

func (p *proc) snapshot() Info {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.info
	i.Log = append([]string(nil), p.info.Log...)
	return i
}

func (p *proc) setState(s, why string) {
	p.mu.Lock()
	p.info.State, p.info.Since = s, time.Now()
	if why != "" {
		p.info.LastError = why
	}
	p.mu.Unlock()
}

func (p *proc) logLine(s string) {
	p.mu.Lock()
	p.info.Log = append(p.info.Log, proto.Cut(s, 500))
	if n := len(p.info.Log); n > logLines {
		p.info.Log = p.info.Log[n-logLines:]
	}
	p.mu.Unlock()
}

// send queues an event for the plugin and reports whether a run took it:
// one that isn't running, or isn't reading, misses it.
func (p *proc) send(ev proto.Event) bool {
	p.mu.Lock()
	alive := p.alive
	p.mu.Unlock()
	if !alive { // the next run starts with an empty queue
		return false
	}
	b, err := proto.Line(ev.Type, ev)
	if err != nil {
		return false
	}
	select {
	case p.out <- b:
		return true
	default: // it isn't reading: it misses this one
		return false
	}
}

func (p *proc) cardClosed(id string) {
	p.mu.Lock()
	delete(p.cards, id)
	p.mu.Unlock()
}

// supervise runs the plugin until ctx ends: again after each exit, with
// backoff, and not at all once it's suspended.
func (p *proc) supervise(ctx context.Context) {
	wait := backoffMin
	var exits []time.Time
	for {
		start := time.Now()
		err := p.run(ctx)
		p.mu.Lock()
		p.alive, p.pending = false, nil
		if p.timer != nil {
			p.timer.Stop()
		}
		p.mu.Unlock()
		p.h.Sink.Gone(p.m.Name)
		if ctx.Err() != nil {
			p.setState(Stopped, "")
			return
		}
		if errors.Is(err, errSuspended) {
			p.setState(Suspended, err.Error())
			<-ctx.Done() // until its manifest changes or Ember stops
			return
		}
		now := time.Now()
		exits = append(exits, now)
		for len(exits) > 0 && now.Sub(exits[0]) > crashWindow {
			exits = exits[1:]
		}
		why := "exited"
		if err != nil {
			why = err.Error()
		}
		if len(exits) >= crashLimit {
			p.setState(Suspended, fmt.Sprintf("suspended: exited %d times within %v (%s)", len(exits), crashWindow, why))
			<-ctx.Done()
			return
		}
		if now.Sub(start) >= healthyRun {
			wait = backoffMin
		}
		p.setState(Waiting, why)
		select {
		case <-ctx.Done():
			p.setState(Stopped, "")
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, backoffMax)
		p.mu.Lock()
		p.info.Restarts++
		p.mu.Unlock()
	}
}

// command is the program to run: relative to the plugin's directory, or on PATH.
func (p *proc) command() (string, error) {
	c := p.m.Command
	if strings.ContainsRune(c, '/') {
		if !filepath.IsAbs(c) {
			c = filepath.Join(p.m.Dir, c)
		}
		return c, nil
	}
	return exec.LookPath(c)
}

// run starts the plugin once and serves it until it exits, ctx ends, or it
// earns a suspension.
func (p *proc) run(ctx context.Context) error {
	path, err := p.command()
	if err != nil {
		return err
	}
	cmd := exec.Command(path, p.m.Args...)
	cmd.Dir = p.m.Dir
	cmd.Env = append(os.Environ(), "EMBER_VERSION="+p.h.Version, "EMBER_PLUGIN_DIR="+p.m.Dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM} // its children go with it
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.setState(Starting, "")
	p.mu.Lock()
	p.alive, p.last, p.pending, p.cards, p.viol, p.shown = true, map[string]time.Time{}, map[string]proto.Message{}, map[string]time.Time{}, nil, false
	p.mu.Unlock()
	for len(p.out) > 0 { // events for the last run aren't this one's
		<-p.out
	}

	kill := func(sig syscall.Signal) { syscall.Kill(-cmd.Process.Pid, sig) }
	exited := make(chan struct{})
	var whyMu sync.Mutex
	var why error
	end := func(err error) { // stop it now, keeping the first reason
		whyMu.Lock()
		defer whyMu.Unlock()
		if why == nil {
			why = err
			kill(syscall.SIGKILL)
		}
	}

	go func() { // its stderr is its log
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 4096), proto.MaxLine)
		for sc.Scan() {
			p.logLine(sc.Text())
		}
	}()
	go func() { // what Ember says: the hello, events, and stop when it's time
		p.mu.Lock()
		grants := p.grants
		p.mu.Unlock()
		hello, _ := proto.Line("hello", proto.HelloEmber{V: proto.Version, Ember: p.h.Version, Caps: p.m.Caps, Grants: grants})
		stdin.Write(hello)
		for {
			select {
			case b := <-p.out:
				if _, err := stdin.Write(b); err != nil {
					return
				}
			case <-ctx.Done():
				b, _ := proto.Line("stop", proto.Event{Type: "stop"})
				stdin.Write(b)
				stdin.Close()
				select {
				case <-exited:
				case <-time.After(stopGrace):
					kill(syscall.SIGTERM)
					select {
					case <-exited:
					case <-time.After(time.Second):
						kill(syscall.SIGKILL)
					}
				}
				return
			case <-exited:
				return
			}
		}
	}()
	helloed := make(chan struct{})
	go func() {
		select {
		case <-helloed:
		case <-exited:
		case <-time.After(helloWithin):
			end(errors.New("no hello within " + helloWithin.String()))
		}
	}()

	rerr := p.read(stdout, helloed, end)
	if rerr != nil {
		end(rerr)
	}
	werr := cmd.Wait()
	close(exited)
	whyMu.Lock()
	defer whyMu.Unlock()
	switch {
	case ctx.Err() != nil:
		return nil
	case why != nil:
		return why
	case werr != nil:
		return werr
	}
	return errors.New("exited")
}

// read serves the plugin's stdout until it closes. It returns an error that
// ends the run (wrapping errSuspended for a suspension).
func (p *proc) read(r io.Reader, helloed chan struct{}, end func(error)) error {
	br := bufio.NewReaderSize(r, 64<<10)
	said := false
	var sec time.Time
	n, hot := 0, 0
	for {
		line, long, err := readLine(br)
		if err != nil {
			return nil // it closed its stdout: it's exiting
		}
		now := time.Now() // flood: too many lines a second, several seconds running
		if now.Sub(sec) >= time.Second {
			if n > floodRate {
				hot++
			} else {
				hot = 0
			}
			sec, n = now, 0
		}
		if n++; n > floodRate && hot+1 >= floodSecs {
			return fmt.Errorf("%w: flooding (over %d messages a second for %d seconds)", errSuspended, floodRate, floodSecs)
		}
		if long {
			if err := p.violation(now, fmt.Errorf("a line over %d bytes", proto.MaxLine)); err != nil {
				return err
			}
			continue
		}
		m, err := proto.Decode(line)
		switch {
		case errors.Is(err, proto.ErrIgnored):
			continue
		case !said: // nothing counts before its hello
			if err == nil && m.Type == "hello" {
				if m.Hello.Name != p.m.Name {
					return fmt.Errorf("its hello says it's %q, its manifest %q", m.Hello.Name, p.m.Name)
				}
				said = true
				close(helloed)
				p.setState(Running, "")
			}
			continue
		case err != nil:
			if err := p.violation(now, err); err != nil {
				return err
			}
			continue
		case m.Type == "hello":
			continue
		case m.Cap() != "" && !p.m.Has(m.Cap()):
			if err := p.violation(now, fmt.Errorf("%s needs the %q capability, which its manifest doesn't declare", m.Type, m.Cap())); err != nil {
				return err
			}
			continue
		}
		if m.Type == "log" {
			if p.admitLog(now) {
				p.logLine(m.Log.Level + ": " + m.Log.Text)
			}
			continue
		}
		p.offer(m, now)
	}
}

// readLine reads one line; long says it was over the limit (and has been
// skipped to its end).
func readLine(br *bufio.Reader) (line []byte, long bool, err error) {
	for {
		chunk, err := br.ReadSlice('\n')
		switch {
		case err == bufio.ErrBufferFull || (long && err == nil):
			if err == nil {
				return nil, true, nil
			}
			long = true
			continue
		case err != nil:
			return nil, false, err
		}
		if long {
			return nil, true, nil
		}
		return bytes.Clone(chunk), false, nil
	}
}

// violation counts one; too many within a minute suspend the plugin.
func (p *proc) violation(now time.Time, why error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.info.Violations++
	p.info.LastError = why.Error()
	p.viol = append(p.viol, now)
	for len(p.viol) > 0 && now.Sub(p.viol[0]) > time.Minute {
		p.viol = p.viol[1:]
	}
	if len(p.viol) >= violationMax {
		return fmt.Errorf("%w: %d bad messages within a minute (last: %v)", errSuspended, len(p.viol), why)
	}
	return nil
}

// admitLog: at most logRate lines in any second (a burst is fine).
func (p *proc) admitLog(now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if now.Sub(p.logSec) >= time.Second {
		p.logSec, p.logN = now, 0
	}
	if p.logN++; p.logN > logRate {
		p.info.Dropped++
		return false
	}
	return true
}

// offer takes a checked message: applied now, coalesced for later, or
// dropped, by its kind's limits.
func (p *proc) offer(m proto.Message, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.alive {
		return
	}
	switch m.Type {
	case "remove":
		switch m.Remove.What {
		case "status", "row", "dashboard":
			delete(p.pending, m.Remove.What) // an older update mustn't bring it back
		case "card":
			delete(p.cards, m.Remove.ID)
		}
		p.h.Sink.Apply(p.m.Name, m)
		return
	case "status", "row", "dashboard":
		if wait := p.every(m) - now.Sub(p.last[m.Type]); wait > 0 {
			if _, ok := p.pending[m.Type]; ok {
				p.info.Dropped++ // the one it replaces never showed
			}
			p.pending[m.Type] = m
			if p.timer == nil {
				p.timer = time.AfterFunc(wait, p.flush)
			} else {
				p.timer.Reset(min(wait, p.untilNext(now)))
			}
			return
		}
	case "card":
		for id, exp := range p.cards {
			if now.After(exp) {
				delete(p.cards, id)
			}
		}
		if _, replace := p.cards[m.Card.ID]; !replace {
			if len(p.cards) >= maxCards || now.Sub(p.last["card"]) < interval["card"] {
				p.info.Dropped++
				p.info.Violations++
				p.info.LastError = fmt.Sprintf("card %q: %d open, or more than one a second", m.Card.ID, len(p.cards))
				return
			}
		}
		p.cards[m.Card.ID] = now.Add(time.Duration(m.Card.TTLs * float64(time.Second)))
	case "cheer":
		key := "cheer:hop" // hop and nod: reactions
		if m.Cheer.Kind == "celebrate" {
			key = "cheer:celebrate"
		}
		if t, ok := p.last[key]; ok && now.Sub(t) < interval[key] {
			p.info.Dropped++
			return
		}
		p.last[key] = now
	default: // moment
		if now.Sub(p.last[m.Type]) < interval[m.Type] {
			p.info.Dropped++
			return
		}
	}
	p.last[m.Type] = now
	p.h.Sink.Apply(p.m.Name, m)
}

// every is how often a kind of update may apply: a dashboard with timed
// text once a second.
func (p *proc) every(m proto.Message) time.Duration {
	if m.Type == "dashboard" && m.Dashboard.Timed() {
		return timedEvery
	}
	return interval[m.Type]
}

// timedEvery: a page with timed text is updated at most this often.
var timedEvery = time.Second

// untilNext is how long until the earliest pending update may go.
func (p *proc) untilNext(now time.Time) time.Duration {
	next := time.Hour
	for k, m := range p.pending {
		next = min(next, max(0, p.every(m)-now.Sub(p.last[k])))
	}
	return next
}

// flush applies the coalesced updates that are due.
func (p *proc) flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.alive {
		return
	}
	now := time.Now()
	for k, m := range p.pending {
		if now.Sub(p.last[k]) >= p.every(m) {
			delete(p.pending, k)
			p.last[k] = now
			p.h.Sink.Apply(p.m.Name, m)
		}
	}
	if len(p.pending) > 0 {
		p.timer.Reset(p.untilNext(now))
	}
}
