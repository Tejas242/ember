package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

var clock = time.Date(2026, 10, 20, 17, 24, 0, 0, ist)

func round(phase string) *Round {
	r := &Round{ID: "cf:2261", Short: "CF 1128", Plan: "in", Problems: []Problem{{Index: "A", State: "solved"}, {Index: "B", State: "tried"}, {Index: "C", State: "upsolve"}}}
	switch phase {
	case "soon":
		r.StartMs, r.EndMs = clock.Add(12*time.Minute).UnixMilli(), clock.Add(132*time.Minute).UnixMilli()
	case "live":
		r.StartMs, r.EndMs, r.Joined, r.Locked = clock.Add(-50*time.Minute).UnixMilli(), clock.Add(70*time.Minute).UnixMilli(), "live", true
	default:
		r.StartMs, r.EndMs, r.Joined = clock.Add(-3*time.Hour).UnixMilli(), clock.Add(-time.Hour).UnixMilli(), "live"
	}
	return r
}

// practising is a task started an hour ago with 23 minutes of work, being worked on now.
func practising() *Status {
	return &Status{Today: 1, Need: 1, Task: "2257C · Spying", TaskID: "cf:2257C", Short: "2257C", Kind: "new", StartedMs: clock.Add(-time.Hour).UnixMilli(),
		WorkSec: 23 * 60, Working: true, AsOfMs: clock.UnixMilli(), Policy: "ladder", NextHintSec: 240, GiveUpInSec: 37 * 60}
}

func TestLine(t *testing.T) {
	if Line(&Status{}, clock) != nil {
		t.Fatal("nothing to say: no line")
	}
	day := Line(&Status{Today: 1, Need: 3, Closed: true, Task: "2257C · Spying on the Beaver"}, clock)
	if day.Text != "Today 1/3" || day.Mood != plugin.Nudge || day.Badge != "lock" || day.BadgeClock != "lock" || day.Value != "2257C" || *day.Salience != 0.55 {
		t.Fatalf("day %+v", day)
	}
	pr := Line(practising(), clock)
	if pr.Text != "2257C" || pr.Mood != plugin.Work || pr.SinceMs != clock.Add(-23*time.Minute).UnixMilli() || !pr.Satellite || pr.Badge != "" {
		t.Fatalf("practising %+v", pr)
	}
	soon := Line(&Status{Round: round("soon")}, clock)
	if soon.Sub != "in 12m" || soon.Timer != "none" || soon.WarnMs != 900000 || *soon.Salience != 0.9 || soon.Mood != plugin.Nudge {
		t.Fatalf("soon %+v", soon)
	}
	live := Line(&Status{Round: round("live")}, clock)
	if live.Timer != "h:mm" || live.Badge != "lock" || !live.Focus || !live.Satellite || *live.Salience != 0.95 || len(live.Chips) != 3 || live.Chips[2].State != "owed" {
		t.Fatalf("live %+v", live)
	}
	over := Line(&Status{Round: round("over")}, clock)
	if over.Mood != plugin.Done || over.Sub != "upsolve C" || *over.Salience != 0.6 {
		t.Fatalf("over %+v", over)
	}
}

func TestRow(t *testing.T) {
	r := Row(&Status{Today: 1, Need: 2, Streak: 4}, clock)
	if r.Title != "Practice" || r.Sub != "1/2 today · 4-day streak" || r.Flame != "4-day streak" || *r.Ring.Value != 0.5 || r.Ring.Mood != plugin.Nudge || len(r.Buttons) != 0 {
		t.Fatalf("day %+v", r)
	}
	r = Row(practising(), clock)
	if r.Sub != "23m · hint in 4m" || len(r.Buttons) != 1 || r.Buttons[0].Key != "start:cf:2257C" || r.Ring.Mood != plugin.Done || r.Flame != "" {
		t.Fatalf("practising %+v", r)
	}
	r = Row(&Status{Round: round("soon")}, clock)
	if r.Sub != "in 12m · you're in" || len(r.Buttons) != 1 || r.Buttons[0].Key != "openround:cf:2261" || *r.Ring.Value != 1 {
		t.Fatalf("soon %+v", r)
	}
	r = Row(&Status{Round: round("live")}, clock)
	if r.Sub != "1:10 left" || r.Ring.EndMs == 0 || r.Badge != "lock" || len(r.Chips) != 3 || len(r.Buttons) != 0 {
		t.Fatalf("live %+v", r)
	}
	r = Row(&Status{Round: round("over")}, clock)
	if r.Sub != "solved A · upsolve C" || r.Buttons[0].Key != "upsolve:cf:2261" || r.Ring.Mood != plugin.Done {
		t.Fatalf("over %+v", r)
	}
}

func TestNewsAndCards(t *testing.T) {
	old, cur := &Status{Streak: 6, Round: round("live")}, &Status{Streak: 7, Round: round("live")}
	cur.Round.Problems[1].State = "solved"
	if got := strings.Join(News(old, cur), ","); got != "hop,celebrate" {
		t.Fatal(got)
	}
	c, bounce := Cardify(&Card{ID: "ac", Mood: "happy", Title: "Accepted", Bounce: true})
	if c.Eyebrow != "Practice" || c.Mood != plugin.Done || !bounce {
		t.Fatalf("%+v", c)
	}
}

func loadStats(t *testing.T) *Stats {
	b, err := os.ReadFile("testdata/stats.json")
	if err != nil {
		t.Fatal(err)
	}
	var st Stats
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	return &st
}

func TestDashboard(t *testing.T) {
	st := loadStats(t)
	morning := time.Date(2026, 10, 20, 10, 26, 0, 0, ist)
	d := Dashboard(st, morning)
	line, err := plugin.Line("dashboard", d)
	if err != nil || len(line) > plugin.MaxLine {
		t.Fatalf("%v, %d bytes", err, len(line))
	}
	m, err := plugin.Decode(line) // what Ember would take
	if err != nil {
		t.Fatal(err)
	}
	ws := m.Dashboard.Widgets
	h := ws[0]
	if h.Kind != "hero" || h.Title != "1 to go today" || h.Sub != "Problems open at 19:00 · Claude is open till then" ||
		h.Step.Tag != "Upsolve" || h.Step.Facts != "lapses Sun night" || h.Buttons[0].Key != "start:cf:2261B" {
		t.Fatalf("hero %+v %+v", h, h.Step)
	}
	if len(ws) != 4 || ws[1].Kind != "columns" || ws[1].A.Layout != "columns" || len(ws[1].B.Items) != 2 {
		t.Fatalf("%d widgets", len(ws)) // the owed round is the hero's step: the agenda doesn't repeat it
	}
	st.Current.Kind = "new" // another task: the owed round is on the agenda, pressable
	if it := Dashboard(st, morning).Widgets[1].B.Items; len(it) != 3 || it[1].Key != "upsolve:cf:2261" || it[1].Title != "CF 1128 upsolves lapse" {
		t.Fatalf("agenda %+v", it)
	}
	st.Current.Kind = "upsolve"
	// the hero's next step isn't repeated by the agenda: a day done, the cold re-solve is next
	st.Today.Done, st.Today.Got, st.Rounds.Upsolves.Queue = true, 1, nil
	d = Dashboard(st, morning)
	for _, it := range d.Widgets[1].B.Items {
		if strings.Contains(it.Title, "3542") {
			t.Fatal("the agenda repeats the hero's step")
		}
	}
	// a live round folds the rest
	st.Today.Done = false
	st.Rounds.Live = &StatsRound{ID: "cf:1", Short: "CF Div. 2", Start: morning.Add(-time.Hour), End: morning.Add(time.Hour), Problems: []Problem{{Index: "A", State: "solved"}, {Index: "B", State: "tried", Fails: 1}}}
	d = Dashboard(st, morning)
	if h := d.Widgets[0]; h.Title != "CF Div. 2 is live" || h.Ember.Pose != "game" || h.Step.Text != "Problem B" || h.Step.Facts != "1 wrong answer" ||
		d.Widgets[2].Fold != true || len(d.Widgets) != 4 {
		t.Fatalf("live %+v %+v", d.Widgets[0], d.Widgets[0].Step)
	}
}

// The plugin, run through the harness: edge's messages in, Ember's out;
// buttons run edge's actions.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")
	b, _ := os.ReadFile("testdata/stats.json")
	os.WriteFile(path, b, 0o644)
	var mu sync.Mutex
	var actions []string
	run := func(p *plugin.Plugin) error {
		return Run(p, Options{StatsPath: path, Now: func() time.Time { return clock }, Every: 20 * time.Millisecond,
			Action: func(k string) error { mu.Lock(); actions = append(actions, k); mu.Unlock(); return nil }})
	}
	h := plugin.NewHarness("edge", plugin.Caps, run)
	if _, err := h.Wait("dashboard", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if r, err := h.Wait("row", time.Second); err != nil || r.Row.Title != "2261B · What's the Capital of France?" { // the task, from the stats at once
		t.Fatalf("%v %+v", err, r.Row)
	}
	tool := func(v any) {
		raw, _ := json.Marshal(v)
		h.Send(plugin.Event{Type: "tool", Payload: raw})
	}
	tool(map[string]any{"type": "status", "status": Status{Today: 0, Need: 1, Closed: true, Task: "2261B · What's the Capital of France?"}})
	if s, err := h.WaitFor("status", time.Second, func(m plugin.Message) bool { return m.Status.Text == "Today 0/1" && m.Status.Badge == "lock" }); err != nil {
		t.Fatalf("%v %+v", err, s)
	}
	tool(map[string]any{"type": "card", "card": map[string]any{"id": "offer", "mood": "work", "title": "Today's problem", "bounce": true,
		"buttons": []map[string]any{{"key": "start", "label": "Start", "primary": true}}}})
	if c, err := h.Wait("card", time.Second); err != nil || c.Card.Eyebrow != "Practice" {
		t.Fatalf("%v %+v", err, c)
	}
	if _, err := h.Wait("cheer", time.Second); err != nil {
		t.Fatal("a bouncing card: a hop")
	}
	h.Button("offer", "start")
	h.Button("row", "hint")
	time.Sleep(100 * time.Millisecond)
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(actions, ",") != "start,hint" {
		t.Fatalf("actions %v", actions)
	}
	if len(h.Errors()) > 0 {
		t.Fatal(h.Errors())
	}
}
