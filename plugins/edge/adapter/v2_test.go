package adapter

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// The pill and row in contract v2's states: the clock is work, and runs
// only while edge counts it.
func TestV2Line(t *testing.T) {
	e := practising()
	e.Working, e.WorkSec = false, 19*60
	if l := Line(e, clock); l.SinceMs != 0 || l.Satellite || l.Value != "19m" || l.Mood != plugin.Work {
		t.Fatalf("not working: frozen, no satellite: %+v", l)
	}
	e = practising()
	e.Working, e.Paused, e.WorkSec = false, true, 18*60
	if l := Line(e, clock); l.Mood != plugin.Quiet || l.Value != "paused · 18m" || *l.Salience > 0.25 || l.SinceMs != 0 {
		t.Fatalf("paused %+v", l)
	}
	e = practising()
	e.Judging = "test 9"
	if l := Line(e, clock); l.Mood != plugin.Wait || l.Sub != "judging · test 9" || l.SinceMs == 0 {
		t.Fatalf("judging %+v", l)
	}
	e = practising()
	e.LastVerdict = "WA on test 3"
	if l := Line(e, clock); l.Mood != plugin.Fail || l.Sub != "WA on test 3" {
		t.Fatalf("WA %+v", l)
	}
	e = practising()
	e.Stopped = true
	if l := Line(e, clock); l.SinceMs != 0 || l.Satellite || l.Value != "23m" {
		t.Fatalf("edge stopped: no live timer %+v", l)
	}
	e = &Status{Need: 1, Task: "3542. Minimum Operations to Convert All Elements", Pending: true}
	if l := Line(e, clock); l.Value != "LC 3542" {
		t.Fatalf("LeetCode %+v", l)
	}
	// a round 40 minutes off doesn't hide the task you're on; from T−15 it leads
	e = practising()
	e.Round = round("soon")
	e.Round.StartMs = clock.Add(40 * time.Minute).UnixMilli()
	if l := Line(e, clock); l.Text != "2257C" || l.Sub != "CF 1128 in 40m" {
		t.Fatalf("round near %+v", l)
	}
	if r := Row(e, clock); r.Title != "2257C · Spying" || r.Sub != "CF 1128 in 40m · 23m" {
		t.Fatalf("round near, row %+v", r)
	}
	e.Round.StartMs = clock.Add(12 * time.Minute).UnixMilli()
	if l := Line(e, clock); l.Text != "CF 1128" {
		t.Fatalf("T−12 %+v", l)
	}
}

func TestV2Row(t *testing.T) {
	e := practising()
	e.HintsReady, e.NextHintSec = true, 0
	if r := Row(e, clock); len(r.Buttons) != 1 || r.Buttons[0].Key != "hint:cf:2257C" || r.Sub != "Practising · 23m" {
		t.Fatalf("hint ready %+v", r)
	}
	e = &Status{Today: 0, Need: 1, Closed: true, Task: "2257C · Spying", TaskID: "cf:2257C", Pending: true}
	if r := Row(e, clock); r.Buttons[0].Key != "start:cf:2257C" || r.Sub != "0/1 · locked" {
		t.Fatalf("unstarted %+v", r)
	}
	e = &Status{Today: 0, Need: 1, Owed: []Owed{{Round: "cf:2262", Short: "CF 1129", Indexes: []string{"C", "D1"}, DueMs: clock.Add(48 * time.Hour).UnixMilli()}}}
	if r := Row(e, clock); r.Buttons[0].Key != "upsolve:cf:2262" || r.Title != "Upsolve CF 1129 C D1" || r.Flame != "" || r.Sub != "0/1 today" {
		t.Fatalf("owed %+v", r)
	}
}

func heroKeys(d plugin.Dashboard) []string {
	var ks []string
	for _, b := range d.Widgets[0].Buttons {
		ks = append(ks, b.Key)
	}
	return ks
}

func TestV2Dashboard(t *testing.T) {
	night := time.Date(2026, 10, 20, 21, 58, 0, 0, time.Local)
	st := loadStats(t)
	st.Today.StudyTime, st.Today.Gate = true, "closed"
	st.Current = &Task{ID: "cf:2277C", Source: "cf", Kind: "new", Title: "Edge Addeleting", Started: night.Add(-time.Hour), WorkSec: 1200, Fails: 2}
	e := &Status{Need: 1, Task: "2277C · Edge Addeleting", TaskID: "cf:2277C", Kind: "new", StartedMs: night.Add(-time.Hour).UnixMilli(), WorkSec: 1200,
		Working: true, AsOfMs: night.UnixMilli(), Policy: "ladder", NextHintSec: 240, GiveUpInSec: 41 * 60, CanSkip: false}
	d := DashboardFor(st, e, night)
	h := d.Widgets[0]
	if h.Title != "Practising" || h.Sub != "Claude unlocks when it's solved" || h.Step.Facts != "2 wrong answers · hint in 4m · give up in 41m" ||
		len(h.Buttons) != 1 || h.Buttons[0].Key != "start:cf:2277C" || h.Step.Dots == nil {
		t.Fatalf("working %+v %+v", h, h.Step)
	}
	e.GiveUpInSec, e.HintsReady, e.NextHintSec = 0, true, 0
	if ks := heroKeys(DashboardFor(st, e, night)); len(ks) != 2 || ks[0] != "giveup:cf:2277C" || ks[1] != "hint:cf:2277C" {
		t.Fatalf("give up and hint: %v", ks)
	}
	e.GiveUpInSec = 60
	if ks := heroKeys(DashboardFor(st, e, night)); len(ks) != 2 || ks[0] != "start:cf:2277C" || ks[1] != "hint:cf:2277C" {
		t.Fatalf("hint primary, open beside it: %v", ks)
	}
	e.CanSkip = true
	if ks := heroKeys(DashboardFor(st, e, night)); ks[0] != "skip:cf:2277C" {
		t.Fatalf("skip: %v", ks)
	}
	// a cold re-solve: no hints, no dots
	e.Policy, e.HintsReady, e.Kind, st.Current.Kind = "", false, "resolve", "resolve"
	if h := DashboardFor(st, e, night).Widgets[0]; h.Step.Dots != nil || h.Step.Facts != "2 wrong answers · no hints · give up in 1m" {
		t.Fatalf("cold %+v", h.Step)
	}

	// nothing current, an upsolve owed: an Owed step, its round's key
	st.Current = nil
	e = &Status{Need: 1, GiveUpInSec: -1, Owed: []Owed{{Round: "cf:2262", Short: "CF 1129", Indexes: []string{"C"}, DueMs: night.Add(30 * time.Hour).UnixMilli()}}}
	if h := DashboardFor(st, e, night).Widgets[0]; h.Step.Tag != "Owed" || h.Buttons[0].Key != "upsolve:cf:2262" {
		t.Fatalf("owed %+v %+v", h, h.Step)
	}
	// the day done, an explain-back waiting
	st.Today.Done, st.Today.Got, st.Today.Gate = true, 1, "open"
	e = &Status{Today: 1, Need: 1, GiveUpInSec: -1, Explain: "cf:2057B"}
	if h := DashboardFor(st, e, night).Widgets[0]; h.Step.Text != "Explain-back · 2057B" || h.Buttons[0].Key != "explain:cf:2057B" {
		t.Fatalf("explain %+v %+v", h, h.Step)
	}
	e = &Status{Today: 1, Need: 1, GiveUpInSec: -1, Debrief: "cf:2160"}
	if h := DashboardFor(st, e, night).Widgets[0]; h.Step.Text != "Debrief CF Div. 2" || h.Buttons[0].Key != "debrief:cf:2160" {
		t.Fatalf("debrief %+v %+v", h, h.Step)
	}
	// no task, the day not done, before study time: start early
	st.Today.Done, st.Today.Got, st.Today.StudyTime, st.Rounds.Upsolves.Queue = false, 0, false, nil
	if h := Dashboard(st, night).Widgets[0]; h.Buttons[0].Key != "next" || h.Buttons[0].Label != "Start early" {
		t.Fatalf("no task %+v", h)
	}
	// edge's trouble is the note, in its own words; edge stopped says so
	e = &Status{Need: 1, GiveUpInSec: -1, Health: "Codeforces isn't answering · verdicts may be late"}
	if d := DashboardFor(st, e, night); d.Widgets[len(d.Widgets)-1].Text != e.Health {
		t.Fatalf("health %+v", d.Widgets[len(d.Widgets)-1])
	}
	e.Stopped = true
	if d := DashboardFor(st, e, night); d.Widgets[len(d.Widgets)-1].Text != "edge is stopped · nothing is being counted" {
		t.Fatalf("stopped %+v", d.Widgets[len(d.Widgets)-1])
	}
}

// Guilt and noise: an old rating change, the rounds dot, the experiment.
func TestV2Quiet(t *testing.T) {
	st := loadStats(t)
	d := Dashboard(st, clock)
	if ln := d.Widgets[2].A; ln.Delta != "" {
		t.Fatalf("a rating change from long ago: %q", ln.Delta)
	}
	if r := d.Widgets[2].B.Items[3]; r.Mood == plugin.Fail || r.Value != "0 of 1" || r.Caption != "sat" {
		t.Fatalf("rounds %+v", r)
	}
	for _, w := range d.Widgets {
		if w.Kind == "note" {
			t.Fatalf("the experiment hasn't moved: %+v", w)
		}
	}
	if dayOf(time.Date(2026, 10, 21, 4, 0, 30, 0, time.Local), "04:00").Day() != 21 || !late(time.Date(2026, 10, 21, 3, 59, 0, 0, time.Local), "04:00") {
		t.Fatal("04:00 starts the new day")
	}
}

// The plugin: the dashboard is sent once, then only while it's open; a
// failed press says why; a quiet edge counts as stopped.
func TestV2Run(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")
	b, _ := os.ReadFile("testdata/stats.json")
	os.WriteFile(path, b, 0o644)
	var mu sync.Mutex
	now := clock
	tick := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	run := func(p *plugin.Plugin) error {
		return Run(p, Options{StatsPath: path, Every: 10 * time.Millisecond,
			Now:    func() time.Time { mu.Lock(); defer mu.Unlock(); return now },
			Action: func(k string) error { return errors.New("Finish 2277C first") }})
	}
	h := plugin.NewHarness("edge", []string{plugin.CapPill, plugin.CapMoment, plugin.CapRow, plugin.CapCard, plugin.CapDashboard, plugin.CapCheer}, run)
	if _, err := h.Wait("dashboard", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	tool := func(v any) {
		raw, _ := json.Marshal(v)
		h.Send(plugin.Event{Type: "tool", Payload: raw})
	}
	st := loadStats(t)
	st.Today.Got = 1
	tool(map[string]any{"type": "stats", "stats": st}) // closed: not sent
	time.Sleep(50 * time.Millisecond)
	n := 0
	for _, m := range h.Messages() {
		if m.Type == "dashboard" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d dashboards while closed", n)
	}
	h.Send(plugin.Event{Type: "dash", State: "open"}) // open: sent at once
	if _, err := h.WaitFor("dashboard", time.Second, func(m plugin.Message) bool { return m.Dashboard.Widgets[0].Ring.Value == 1 }); err != nil {
		t.Fatal("opened: the dashboard follows")
	}
	h.Button("row", "start:cf:2261B")
	if m, err := h.Wait("moment", time.Second); err != nil || m.Moment.Text != "Finish 2277C first" || m.Moment.TTLs != 5 {
		t.Fatalf("%v %+v", err, m)
	}
	tool(map[string]any{"type": "status", "status": Status{Need: 1, Task: "2261B · x", TaskID: "cf:2261B", StartedMs: clock.UnixMilli(),
		WorkSec: 600, Working: true, AsOfMs: clock.UnixMilli()}})
	if _, err := h.WaitFor("status", time.Second, func(m plugin.Message) bool { return m.Status.Satellite }); err != nil {
		t.Fatal("working: a live clock")
	}
	tick(2 * time.Minute) // edge goes quiet
	if _, err := h.WaitFor("status", time.Second, func(m plugin.Message) bool { return !m.Status.Satellite && m.Status.SinceMs == 0 }); err != nil {
		t.Fatal("edge quiet for 2 minutes: no live clock")
	}
	h.Stop()
	if len(h.Errors()) > 0 {
		t.Fatal(h.Errors())
	}
}
