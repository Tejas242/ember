package adapter

// Tests from the edge audit (.audit/edge-bugs.md in ~/Gems/ember), one per
// bug, adapted to contract v2's scoped keys; they pass now.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// B2: the pill and the row count wall-clock since the task started, not
// work. A task started yesterday reads "24h 12m" and keeps counting
// overnight, until the task ends.
func TestAuditPractisingClockIsWallClock(t *testing.T) {
	started := clock.Add(-(24*time.Hour + 12*time.Minute)) // started yesterday, ~19 min of real work since
	e := &Status{Today: 0, Need: 1, Task: "2261C · I Know About a Million Things", StartedMs: started.UnixMilli()}
	l := Line(e, clock)
	if l.SinceMs > 0 && clock.Sub(time.UnixMilli(l.SinceMs)) > 12*time.Hour {
		t.Errorf("pill counts up from %s: %s on the clock", time.UnixMilli(l.SinceMs).Format("Mon 15:04"), mins(clock.Sub(time.UnixMilli(l.SinceMs))))
	}
	if r := Row(e, clock); r.Sub == "Practising · 24h 12m" {
		t.Errorf("row: %q", r.Sub)
	}
}

// B1: Ember can't give up. A practising task past its give-up time offers
// only Hint (row) or Open/Hint (dashboard); nothing reaches `teach`/skip.
func TestAuditNoGiveUpInEmber(t *testing.T) {
	st := loadStats(t)
	st.Today.Done = false
	st.Current = &Task{ID: "cf:2261C", Source: "cf", Kind: "upsolve", Title: "I Know About a Million Things",
		Started: clock.Add(-2 * time.Hour), WorkSec: 3700, Hints: 3, HintsAllowed: 3}
	d := Dashboard(st, clock)
	r := Row(&Status{Need: 1, Task: "2261C · I Know", StartedMs: clock.Add(-2 * time.Hour).UnixMilli()}, clock)
	var keys []string
	for _, b := range d.Widgets[0].Buttons {
		keys = append(keys, b.Key)
	}
	for _, b := range r.Buttons {
		keys = append(keys, b.Key)
	}
	for _, k := range keys {
		if strings.HasPrefix(k, "giveup:") || strings.HasPrefix(k, "skip:") { // contract v2: scoped keys
			return
		}
	}
	t.Errorf("an hour in, all hints used: no way to give up from Ember (buttons %v)", keys)
}

// B4: the dashboard's "Open" button sends `open`, which edge has no case
// for (onAction's default treats it as a round verb and fails silently).
func TestAuditDashboardSendsOpen(t *testing.T) {
	st := loadStats(t)
	st.Today.Done = false
	st.Current = &Task{ID: "cf:2261C", Source: "cf", Kind: "upsolve", Title: "x", Started: clock.Add(-10 * time.Minute), WorkSec: 300, NextHintSec: 900}
	d := Dashboard(st, clock)
	for _, b := range d.Widgets[0].Buttons {
		if b.Key == "open" {
			t.Errorf("hero button %q: edge's onAction has no `open` (see edge audit TestAuditOpenAction)", b.Key)
		}
	}
}

// B3: an owed upsolve with nothing current and the day not done: the
// dashboard names it nowhere as an action. The only Upsolve buttons are
// the once-only card (3 min TTL), the row for 30 min after a round they
// *sat* settles, and the hero on a day already done.
func TestAuditOwedUpsolveHasNoButton(t *testing.T) {
	st := loadStats(t)
	st.Today.Done, st.Current = false, nil
	if st.Rounds == nil || len(st.Rounds.Upsolves.Queue) == 0 {
		t.Skip("testdata has no upsolve queue")
	}
	d := Dashboard(st, clock)
	for _, b := range d.Widgets[0].Buttons {
		if strings.HasPrefix(b.Key, "upsolve:") { // contract v2: upsolve:<round>
			return
		}
	}
	t.Errorf("upsolve owed (%s %s), day not done: hero buttons %+v", st.Rounds.Upsolves.Queue[0].Short, st.Rounds.Upsolves.Queue[0].Index, d.Widgets[0].Buttons)
}

// B9: after Ember (or the plugin) restarts, the adapter shows the day from
// stats.json until edge's next status (up to a minute): FromStats drops the
// current task, so a practising pill becomes "Today 0/1" and the row loses
// its button.
func TestAuditRestartDropsCurrentTask(t *testing.T) {
	st := loadStats(t)
	st.Current = &Task{ID: "cf:2261C", Source: "cf", Kind: "upsolve", Title: "I Know", Started: clock.Add(-20 * time.Minute), WorkSec: 1100}
	e := FromStats(st)
	if e.Task == "" || e.StartedMs == 0 {
		t.Errorf("FromStats: %+v (stats has current %s, started)", e, st.Current.ID)
	}
}

// B11: the dashboard is rebuilt and resent while it's closed: each stats
// change (every 20 s tick while working: work_sec moves) and each time word
// that changes sends a whole dashboard, and Ember redraws for each one
// (applyExt invalidates). Run keeps a.open but never reads it.
func TestAuditDashboardSentWhileClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")
	b, _ := os.ReadFile("testdata/stats.json")
	os.WriteFile(path, b, 0o644)
	run := func(p *plugin.Plugin) error {
		return Run(p, Options{StatsPath: path, Now: func() time.Time { return clock }, Every: 10 * time.Millisecond, Action: func(string) error { return nil }})
	}
	h := plugin.NewHarness("edge", plugin.Caps, run)
	if _, err := h.Wait("dashboard", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	st := loadStats(t)
	for i := 1; i <= 3; i++ { // edge's pushes while he works, the dashboard never opened
		st.Week.Days[0].WorkSec += 20 * i
		raw, _ := json.Marshal(map[string]any{"type": "stats", "stats": st})
		h.Send(plugin.Event{Type: "tool", Payload: raw})
		time.Sleep(50 * time.Millisecond)
	}
	h.Stop()
	n := 0
	for _, m := range h.Messages() {
		if m.Type == "dashboard" {
			n++
		}
	}
	if n > 1 {
		t.Errorf("%d dashboards sent while it was closed", n)
	}
}
