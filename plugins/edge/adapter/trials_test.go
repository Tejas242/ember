package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// evening is a trial weekday at 19:10, the cue at 19:30.
var evening = time.Date(2026, 11, 10, 19, 10, 0, 0, time.Local)

// tonight is a trial evening before the rep: tonight's pick, a cue set.
func tonight() *Status {
	return &Status{Today: 0, Need: 1, Closed: true, Task: "2779. Maximum Beauty of an Array", TaskID: "lc:2779", Short: "LC 2779", Kind: "new",
		Pending: true, GiveUpInSec: -1, Phase: "trial1", CueMs: evening.Add(20 * time.Minute).UnixMilli(), Chain: 4, SkipTokens: 1, SeenMs: evening.UnixMilli()}
}

// repping is tonight's rep started, 13m48s in, the clock running.
func repping() *Status {
	e := tonight()
	e.Pending, e.StartedMs, e.Working, e.AsOfMs, e.WorkSec = false, evening.Add(-14*time.Minute).UnixMilli(), true, evening.UnixMilli(), 13*60+48
	e.DeadlineMs = evening.Add(45*time.Minute - 13*time.Minute - 48*time.Second).UnixMilli()
	e.GiveUpInSec = 30 * 60
	return e
}

func TestTrialDecode(t *testing.T) {
	var m Message
	raw := `{"type":"status","status":{"phase":"trial2","cueMs":5,"repDone":true,"chain":3,"missed":1,"skipTokens":1,"deadlineMs":9,
		"pattern":"Sliding window","drill":true,"readyPct":62,"readyLo":54,"readyHi":70}}`
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if s := m.Status; s.Phase != "trial2" || s.CueMs != 5 || !s.RepDone || s.Chain != 3 || s.Missed != 1 || s.SkipTokens != 1 || s.DeadlineMs != 9 ||
		s.Pattern != "Sliding window" || !s.Drill || s.ReadyPct != 62 || s.ReadyLo != 54 || s.ReadyHi != 70 {
		t.Fatalf("%+v", s)
	}
	var st Stats
	raw = `{"version":1,"phase":"trial2","consistency":{"trial_from":"2026-11-06","trial_to":"2026-11-19","reps":5,"show_up_14":0.5,
		"median_cue_to_start_min":12,"chain":3,"skip_tokens":1,"days":[{"date":"2026-11-09","rep":true,"cue_set":true,"cue":"19:30","cue_to_start_min":4,"kind":"ac"}]},
		"readiness":{"r":0.62,"lo":0.54,"hi":0.7,"drills":[{},{}],"brier":0.18,"mistakes":{"last_30d":{"edge":4,"bug":2}},
		"patterns":[{"name":"Heap","weight":1,"theta":1500,"sigma":120,"p":0.8,"r":0.9,"lo":0.6,"hi":0.8,"solved":7,"last":"2026-11-02"}]}}`
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatal(err)
	}
	c, r := st.Consistency, st.Readiness
	if c.From != "2026-11-06" || c.Reps != 5 || *c.CueToStart != 12 || len(c.Days) != 1 || !c.Days[0].Rep || *c.Days[0].CueToStart != 4 {
		t.Fatalf("%+v", c)
	}
	if r.R != 0.62 || r.Drills != 2 || *r.Brier != 0.18 || r.Patterns[0].Solved != 7 || r.Mistakes.Last30["edge"] != 4 {
		t.Fatalf("%+v", r)
	}
	if k, n, of := topMistake(r); k != "edge" || n != 4 || of != 6 {
		t.Fatalf("top mistake %s %d/%d", k, n, of)
	}
	var n count
	if json.Unmarshal([]byte("3"), &n); n != 3 {
		t.Fatalf("count %d", n)
	}
	if frac(62) != 0.62 || frac(0.62) != 0.62 || pct(0.625) != 63 {
		t.Fatal("frac reads 0..1 and 0..100")
	}
}

// Before the rep: its time and the pick, quiet until the cue, then asking.
func TestRepLineBefore(t *testing.T) {
	l := Line(tonight(), evening)
	if l.Text != "Rep · 19:30" || l.Value != "LC 2779" || l.Mood != plugin.Quiet || *l.Salience != 0.2 || l.Badge != "lock" {
		t.Fatalf("before the cue %+v", l)
	}
	l = Line(tonight(), evening.Add(25*time.Minute))
	if l.Text != "Rep · 19:30" || l.Mood != plugin.Nudge || *l.Salience != 0.55 {
		t.Fatalf("at the cue %+v", l)
	}
	e := tonight()
	e.Phase = "baseline" // before its phase, a trial shows nothing
	if l := Line(e, evening); l.Text != "Today 0/1" {
		t.Fatalf("baseline %+v", l)
	}
	e = tonight()
	e.Task, e.TaskID, e.Short = "", "", ""
	if l := Line(e, evening); l.Text != "Rep · 19:30" || l.Value != "" {
		t.Fatalf("no pick yet %+v", l)
	}
	e = tonight()
	e.CueMs = 0 // no cue yet: the usual line
	if l := Line(e, evening); l.Text != "Today 0/1" {
		t.Fatalf("no cue %+v", l)
	}
}

// The rep running: an interview countdown while edge counts work, frozen when
// it doesn't, and once 45 minutes are used, the task just goes on.
func TestRepCountdown(t *testing.T) {
	e := repping()
	l := Line(e, evening)
	if l.Text != "LC 2779" || l.UntilMs != e.DeadlineMs || l.SinceMs != 0 || !l.Satellite || l.Mood != plugin.Work || l.WarnMs != 0 {
		t.Fatalf("working %+v", l)
	}
	e.Working = false // not counted now: frozen as of AsOfMs, whatever the clock says
	for _, at := range []time.Time{evening, evening.Add(7 * time.Minute)} {
		if l := Line(e, at); l.Value != "31:12 left" || l.UntilMs != 0 || l.Satellite {
			t.Fatalf("frozen at %v %+v", at, l)
		}
	}
	e.Paused = true
	if l := Line(e, evening.Add(10*time.Minute)); l.Value != "paused · 31:12 left" || l.Mood != plugin.Quiet || *l.Salience != 0.2 {
		t.Fatalf("paused %+v", l)
	}
	e = repping()
	e.WorkSec, e.DeadlineMs = 50*60, evening.Add(-5*time.Minute).UnixMilli()
	if l := Line(e, evening); l.UntilMs != 0 || l.SinceMs == 0 || strings.Contains(l.Value, "left") {
		t.Fatalf("over 45m: counts up, nothing punishing %+v", l)
	}
	e = repping()
	e.Drill, e.Short = true, "LC 1234"
	if l := Line(e, evening); l.Text != "Drill · LC 1234" || l.UntilMs != e.DeadlineMs {
		t.Fatalf("drill %+v", l)
	}
	e = repping()
	e.Judging = "test 9"
	if l := Line(e, evening); l.Sub != "judging · test 9" || l.UntilMs == 0 {
		t.Fatalf("judging keeps the countdown %+v", l)
	}
	if d, ok := repLeft(repping(), evening.Add(31*time.Minute+12*time.Second)); ok || d != 0 {
		t.Fatalf("used up: no countdown %v", d)
	}
}

func TestRepLineDone(t *testing.T) {
	e := tonight()
	e.Task, e.TaskID, e.Short, e.Today, e.RepDone, e.Chain = "", "", "", 1, true, 5
	if l := Line(e, evening); l.Text != "Rep done" || l.Value != "chain 5" || l.Mood != plugin.Done || *l.Salience != 0 {
		t.Fatalf("done %+v", l)
	}
	e.Chain = 0
	if l := Line(e, evening); l.Value != "" {
		t.Fatalf("never a chain of 0 %+v", l)
	}
}

// The chain, in the moment the rep's problem is solved or the rep counts.
func TestMoment(t *testing.T) {
	old := repping()
	cur := tonight()
	cur.Task, cur.TaskID, cur.Pending, cur.Today, cur.RepDone, cur.Chain, cur.Explain = "", "", false, 1, true, 5, "lc:2779"
	if m := Moment(old, cur); m == nil || m.Text != "first try · chain 5" || m.Mood != plugin.Done {
		t.Fatalf("AC %+v", m)
	}
	old.LastVerdict = "WA on test 3"
	if m := Moment(old, cur); m == nil || m.Text != "solved · chain 5" {
		t.Fatalf("AC after a WA %+v", m)
	}
	old = repping()
	cur = repping()
	cur.RepDone, cur.Chain = true, 1
	if m := Moment(old, cur); m == nil || m.Text != "Rep done · chain 1" {
		t.Fatalf("rep at 25 minutes %+v", m)
	}
	if m := Moment(cur, cur); m != nil {
		t.Fatalf("nothing new %+v", m)
	}
	old.Phase, cur.Phase = "baseline", "baseline"
	if m := Moment(old, cur); m != nil {
		t.Fatalf("baseline: no moment %+v", m)
	}
	if Moment(nil, cur) != nil {
		t.Fatal("first status: nothing new")
	}
}

func TestTrialRow(t *testing.T) {
	if r := Row(tonight(), evening); r.Sub != "Rep at 19:30 · chain 4" || r.Flame != "chain 4" || r.Buttons[0].Key != "start:lc:2779" {
		t.Fatalf("before %+v", r)
	}
	if r := Row(repping(), evening); r.Sub != "Practising · 32m left" {
		t.Fatalf("running %+v", r)
	}
	e := tonight()
	e.Missed, e.Chain = 1, 0
	if r := Row(e, evening); strings.Contains(r.Sub, "chain 0") || r.Flame != "" {
		t.Fatalf("never chain 0 %+v", r)
	}
	e = repping()
	e.Phase = "baseline"
	if r := Row(e, evening); !strings.HasPrefix(r.Sub, "Practising · 13m") {
		t.Fatalf("baseline %+v", r)
	}
}

func TestCardifyChips(t *testing.T) {
	var c Card
	raw := `{"id":"cue","mood":"nudge","title":"Tonight's rep: LC 2779 · Sliding window. When?","ttl_s":3600,
		"chips":[{"key":"cue:now","label":"Now"},{"key":"cue:19:30","label":"19:30"},{"key":"cue:21:00","label":"21:00"},{"key":"skiptonight","label":"Skip (1 left)"}]}`
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	pc, _ := Cardify(&c)
	if len(pc.Chips) != 4 || pc.Chips[1].Key != "cue:19:30" || pc.Chips[3].Label != "Skip (1 left)" || pc.TTLs != 3600 || len(pc.Buttons) != 0 {
		t.Fatalf("%+v", pc)
	}
	b, _ := plugin.Line("card", pc)
	if _, err := plugin.Decode(b); err != nil {
		t.Fatalf("Ember takes it: %v", err)
	}
}

// trialStats is the fixture in a trial: 28 days of reps, the model in shadow.
func trialStats(t *testing.T, phase string) *Stats {
	st := loadStats(t)
	st.Phase = phase
	st.Today.Date, st.Today.Need, st.Today.StudyTime, st.Today.Gate, st.Today.GateWhy = "2026-11-10", 1, true, "closed", ""
	st.Current = &Task{ID: "lc:2779", Source: "lc", Kind: "new", Title: "2779. Maximum Beauty of an Array"}
	c := &Consistency{From: "2026-11-06", To: "2026-11-19", Reps: 4, ShowUp: 9.0 / 14, Chain: 4, SkipTokens: 1, CueToStart: new(float64)}
	*c.CueToStart = 12
	for i := 27; i >= 0; i-- {
		d := evening.AddDate(0, 0, -i)
		c.Days = append(c.Days, RepDay{Date: d.Format("2006-01-02"), Rep: i%3 != 0 && i > 0, Token: i == 6})
	}
	st.Consistency = c
	r := &Readiness{R: 0.62, Lo: 0.54, Hi: 0.70, Drills: 2, Brier: new(float64)}
	*r.Brier = 0.18
	r.Mistakes.Last30 = map[string]int{"edge": 4, "bug": 2, "misread": 1}
	for i, n := range []string{"Two pointers", "Sliding window", "Binary search", "Prefix sums", "Hashing", "Stack", "Heap", "Intervals", "Greedy", "Graphs", "DP 1D"} {
		p := 0.4 + 0.05*float64(i)
		r.Patterns = append(r.Patterns, Pattern{Name: n, Weight: 1, P: p, R: 0.9, Lo: p*0.9 - 0.1, Hi: p*0.9 + 0.1})
	}
	st.Readiness = r
	return st
}

func widgetTitles(d plugin.Dashboard) []string {
	var ts []string
	for _, w := range d.Widgets {
		ts = append(ts, w.Kind+":"+w.Title)
	}
	return ts
}

func TestTrialDashboard(t *testing.T) {
	st := trialStats(t, "trial1")
	e := tonight()
	d := DashboardFor(st, e, evening)
	if got := strings.Join(widgetTitles(d), ","); got != "hero:Tonight's rep,columns:Chain,columns:Contests" {
		t.Fatalf("trial1 reads: %s", got)
	}
	h := d.Widgets[0]
	if h.Aside != "19:30" || h.Stat.Value != "4" || h.Step == nil || h.Buttons[len(h.Buttons)-1].Key != "start:lc:2779" {
		t.Fatalf("hero %+v %+v", h, h.Step)
	}
	if h.Buttons[0].Key != "skiptonight" {
		t.Fatalf("a skip is offered before the cue %+v", h.Buttons)
	}
	if d.Widgets[2].Fold != true || !strings.HasPrefix(d.Widgets[2].Summary, "CF 1208") {
		t.Fatal("contests fold small")
	}
	ch := d.Widgets[1]
	if ch.A.Weeks != 4 || len(ch.A.Days) != 28 || ch.B.Items[0].Value != 4.0 || ch.B.Items[1].Value != "1" || ch.B.Items[2].Value != "9 of 14" {
		t.Fatalf("chain %+v %+v", ch.A, ch.B.Items)
	}
	for _, day := range ch.A.Days {
		if day.Mood == plugin.Fail {
			t.Fatal("no red for a missed day")
		}
	}

	// after a miss: the gentle copy, and never a 0
	e.Missed, e.Chain = 1, 0
	st.Consistency.Chain = 0
	d = DashboardFor(st, e, evening)
	if h := d.Widgets[0]; h.Stat != nil || !strings.HasPrefix(h.Sub, "Yesterday slipped. Tonight keeps the chain") {
		t.Fatalf("after a miss %+v", h)
	}
	if v := d.Widgets[1].B.Items[0].Value; v != "–" {
		t.Fatalf("chain 0 shows as %v", v)
	}

	// the rep running: the countdown
	d = DashboardFor(st, repping(), evening)
	if h := d.Widgets[0]; h.Title != "Rep" || h.Aside != "32m left" || h.Ring.Mode != "countdown" || h.Ring.EndMs != repping().DeadlineMs {
		t.Fatalf("running %+v %+v", h, h.Ring)
	}

	// trial2: readiness and the commonest mistake
	st = trialStats(t, "trial2")
	e = tonight()
	e.Phase, e.Pattern = "trial2", "Sliding window"
	d = DashboardFor(st, e, evening)
	if got := strings.Join(widgetTitles(d), ","); got != "hero:Tonight's rep,columns:Chain,bars:Interview readiness · 62%,note:,columns:Contests" {
		t.Fatalf("trial2 reads: %s", got)
	}
	if f := d.Widgets[0].Step.Facts; !strings.Contains(f, "Sliding window · 41% ready") {
		t.Fatalf("why this: %q", f)
	}
	rd := d.Widgets[2]
	if rd.Value != "62%" || rd.Caption != "likely 54–70%" || len(rd.Items) != 8 || rd.Items[0].Label != "Two pointers" || *rd.Items[0].Target != 75 ||
		rd.Items[0].Lo == nil || *rd.Items[0].Lo > rd.Items[0].Num || *rd.Items[0].Hi < rd.Items[0].Num {
		t.Fatalf("readiness %+v %+v", rd, rd.Items[0])
	}
	if n := d.Widgets[3].Text; !strings.HasPrefix(n, "Most wrong answers lately: edge cases · 4 of 7") {
		t.Fatalf("mistakes %q", n)
	}
	b, _ := plugin.Line("dashboard", d)
	if _, err := plugin.Decode(b); err != nil {
		t.Fatalf("Ember takes it: %v", err)
	}
}

// The baseline: the dashboard as it was, with one quiet line; a reverted
// trial says it's off.
func TestBaselineDashboard(t *testing.T) {
	st := loadStats(t)
	before := DashboardFor(st, nil, clock)
	st.Phase = "baseline"
	d := DashboardFor(st, nil, clock)
	if len(d.Widgets) != len(before.Widgets)+1 {
		t.Fatalf("baseline adds one note: %v", widgetTitles(d))
	}
	if n := d.Widgets[len(d.Widgets)-1]; n.Kind != "note" || n.Text != "Baseline week · Trial 1 starts Fri 6 Nov" || n.Mood != plugin.Quiet {
		t.Fatalf("%+v", n)
	}
	st.Phase = "trial1"
	st.Consistency = &Consistency{Reverted: true}
	d = DashboardFor(st, nil, evening)
	if n := d.Widgets[len(d.Widgets)-1]; n.Text != "Trial 1 is off · back to the daily goal" || d.Widgets[1].Kind != "columns" || d.Widgets[1].A.Title != "This week" {
		t.Fatalf("reverted: the old page %v", widgetTitles(d))
	}
}

// The plugin says the chain in the moment, through the harness.
func TestRunMoment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	b, _ := os.ReadFile("testdata/stats.json")
	os.WriteFile(path, b, 0o644)
	run := func(p *plugin.Plugin) error {
		return Run(p, Options{StatsPath: path, Now: func() time.Time { return evening }, Every: 20 * time.Millisecond,
			Action: func(string) error { return nil }})
	}
	h := plugin.NewHarness("edge", plugin.Caps, run)
	tool := func(v any) {
		raw, _ := json.Marshal(v)
		h.Send(plugin.Event{Type: "tool", Payload: raw})
	}
	tool(map[string]any{"type": "status", "status": repping()})
	if _, err := h.WaitFor("status", time.Second, func(m plugin.Message) bool { return m.Status.UntilMs == repping().DeadlineMs }); err != nil {
		t.Fatal("the rep's countdown", err)
	}
	done := tonight()
	done.Task, done.TaskID, done.Pending, done.Today, done.RepDone, done.Chain, done.Explain = "", "", false, 1, true, 5, "lc:2779"
	tool(map[string]any{"type": "status", "status": done})
	if m, err := h.Wait("moment", time.Second); err != nil || m.Moment.Text != "first try · chain 5" {
		t.Fatalf("%v %+v", err, m)
	}
	if s, err := h.WaitFor("status", time.Second, func(m plugin.Message) bool { return m.Status.Text == "Rep done" }); err != nil || s.Status.Value != "chain 5" {
		t.Fatalf("%v %+v", err, s)
	}
	h.Stop()
	if len(h.Errors()) > 0 {
		t.Fatal(h.Errors())
	}
}
