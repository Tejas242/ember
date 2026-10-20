package adapter

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// Options are where the adapter reads and what it runs; the zero values are
// the real ones.
type Options struct {
	StatsPath string           // edge's stats.json
	Action    func(key string) // runs `edge action <key>`
	Now       func() time.Time // the clock
	Every     time.Duration    // how often time-worded things are refreshed (5 s)
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

// EdgeAction runs `edge action <key>` without waiting, if edge is installed.
func EdgeAction(key string) {
	if _, err := exec.LookPath("edge"); err == nil {
		cmd := exec.Command("edge", "action", key)
		if cmd.Start() == nil {
			go cmd.Wait()
		}
	}
}

// adapter is the plugin's state: what edge last said, and what Ember was last sent.
type adapter struct {
	p      *plugin.Plugin
	o      Options
	status *Status
	stats  *Stats
	open   bool
	sent   map[string][]byte // the last of each kind: only a change is sent again
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
	a := &adapter{p: p, o: o, sent: map[string][]byte{}}
	var seen time.Time
	poll := func() { // edge rewrites its stats atomically, only when something changed
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
	poll()
	a.show()
	statsT, refresh := time.NewTicker(3*time.Second), time.NewTicker(o.Every)
	defer statsT.Stop()
	defer refresh.Stop()
	for {
		select {
		case ev, ok := <-p.Events():
			if !ok {
				return nil
			}
			a.event(ev)
		case <-statsT.C:
			poll()
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
		if ev.Key != "" && ev.Key != "dismiss" {
			a.o.Action(ev.Key)
		}
	case "dash":
		a.open = ev.State == "open"
		a.show()
	}
}

// message is one of edge's messages.
func (a *adapter) message(m Message) {
	switch m.Type {
	case "status":
		for _, c := range News(a.status, m.Status) {
			a.p.Cheer(c)
		}
		a.status = m.Status
		a.show()
	case "stats":
		if m.Stats != nil && m.Stats.Version >= 1 {
			a.stats = m.Stats
			a.show()
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

// show sends what changed: the line, the row, and the dashboard (while it's
// open, it follows the clock).
func (a *adapter) show() {
	now := a.o.Now()
	if l := Line(a.status, now); l != nil {
		a.send("status", l, func() error { return a.p.Status(*l) })
	} else if a.sent["status"] != nil && string(a.sent["status"]) != "null" {
		a.send("status", nil, a.p.ClearStatus)
	}
	if a.status == nil && a.stats == nil {
		return // edge hasn't said anything yet
	}
	e := a.status
	if e == nil {
		e = FromStats(a.stats)
	}
	r := Row(e, now)
	a.send("row", r, func() error { return a.p.Row(r) })
	if a.stats != nil {
		st := *a.stats
		if n := len(st.Calendar); n > 26*7+7 {
			st.Calendar = st.Calendar[n-26*7-7:]
		}
		d := Dashboard(&st, now)
		a.send("dashboard", d, func() error { return a.p.Dashboard(d) })
	}
}
