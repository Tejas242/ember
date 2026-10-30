package adapter

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// edge's dashboard, stacked by the questions you ask of it: where do I stand
// today and what's next (the hero), what's this week and what's coming, am I
// improving, the record, and a note. What isn't needed now folds to a line.

// dayOf: an edge day ends at day_ends (04:00), so a time before it belongs
// to the day before.
func dayOf(t time.Time, ends string) time.Time {
	hm, err := time.Parse("15:04", ends)
	if err != nil {
		return t
	}
	if t.Hour()*60+t.Minute() < hm.Hour()*60+hm.Minute() {
		return t.AddDate(0, 0, -1)
	}
	return t
}

// late: the last hour before the day ends, from 23:00 until day_ends.
func late(now time.Time, ends string) bool {
	return now.Hour() >= 23 || dayOf(now, ends).YearDay() != now.YearDay()
}

// when says how far off t is, the way the agenda does: "in 1h 07m", "18:35",
// "Sun night", "Mon 20:05", "Thu 29".
func when(now, t time.Time, ends string) string {
	d := t.Sub(now)
	td, nd := dayOf(t.Add(-time.Minute), ends), dayOf(now, ends) // a deadline at day_ends is the night before's
	switch {
	case d < 0:
		return "now"
	case d < 3*time.Hour:
		return "in " + mins(d.Round(time.Minute)) // rounded, as a live round's time left is
	case td.YearDay() == nd.YearDay() && td.Year() == nd.Year():
		return t.Format("15:04")
	case d < 6*24*time.Hour:
		if ends != "" && td.YearDay() != t.YearDay() {
			return td.Format("Mon") + " night"
		}
		return t.Format("Mon 15:04")
	}
	return t.Format("Mon 2")
}

func kindTag(k string) string {
	return map[string]string{"new": "New", "resolve": "Re-solve", "reentry": "Warm-up", "upsolve": "Upsolve"}[k]
}

func taskName(t *Task) string {
	if t.Source == "cf" {
		return strings.TrimPrefix(t.ID, "cf:") + " · " + t.Title
	}
	return t.Title
}

// taskShort is the pill's label for a task in the stats: "2261C", "LC 3542".
func taskShort(t *Task) string {
	if t.Source == "cf" {
		return strings.TrimPrefix(t.ID, "cf:")
	}
	return shortTask(t.Title)
}

func gateWords(st *Stats, e *Status) string {
	t := st.Today
	switch t.Gate {
	case "closed":
		switch {
		case e.active():
			return "Claude unlocks when it's solved"
		case e.Task != "":
			return "Claude is locked · start to unlock"
		}
		return "Claude is locked until you practise"
	case "off":
		return "The gate is off"
	}
	switch t.GateWhy {
	case "done":
		return "Claude is open: you did today's"
	case "pass":
		return "Claude is open: this week's pass"
	case "outside_study_time":
		return "Problems open at " + t.StudyFrom + " · Claude is open till then"
	case "edge_trouble":
		return "Claude is open: edge can't check right now"
	}
	return "Claude is open"
}

func milestone(n int) bool { return n == 7 || n == 30 || n == 100 || n == 365 }

// Dashboard is edge's page for this moment, from its stats alone.
func Dashboard(st *Stats, now time.Time) plugin.Dashboard { return DashboardFor(st, nil, now) }

// DashboardFor is edge's page for this moment. e is what edge last said
// (the task's controls, owed upsolves, explain-back, debrief, health); nil
// reads it all from the stats.
func DashboardFor(st *Stats, e *Status, now time.Time) plugin.Dashboard {
	if e == nil {
		e = FromStats(st)
	}
	ends := st.Today.DayEnds
	late := late(now, ends)
	cur := st.Current
	if cur != nil && e.TaskID != "" && cur.ID != e.TaskID {
		cur = nil // the stats are behind edge's word
	}
	working := e.active()
	var live *StatsRound
	if st.Rounds != nil && st.Rounds.Live != nil {
		live = st.Rounds.Live
	}

	// ---- the hero: Ember in the day's ring, the state, the next step ----
	h := plugin.Widget{Kind: "hero", Ember: &plugin.HeroEmber{Mood: plugin.Quiet, Pose: "calm"},
		Ring: &plugin.HeroRing{Value: float64(st.Today.Got), Goal: float64(max(st.Today.Need, 1)), Mood: plugin.Nudge}, SubMood: plugin.Quiet}
	step := &plugin.Step{TagMood: plugin.Nudge}
	if st.Streak.Current > 0 {
		h.Stat = &plugin.HeroStat{Value: fmt.Sprint(st.Streak.Current), Caption: "day streak"}
	}
	stepKey := "" // what the agenda mustn't repeat
	// after is the step when there's no task: an explain-back, a debrief,
	// an owed upsolve; else (the day done) a re-solve or the next round, or
	// (not done) the day's problem early.
	after := func() {
		switch o := owedNow(e, now); {
		case e.Explain != "":
			step.Tag, step.TagMood, step.Text = "Explain", plugin.Done, "Explain-back · "+explainName(st, e.Explain)
			step.Facts = "say how you solved it"
			h.Buttons = []plugin.Button{{Key: key("explain", e.Explain), Label: "Explain", Primary: true}}
		case e.Debrief != "":
			step.Tag, step.TagMood, step.Text = "Debrief", plugin.Done, "Debrief "+roundShort(st, e.Debrief)
			step.Facts = "one lesson for the next round"
			h.Buttons = []plugin.Button{{Key: key("debrief", e.Debrief), Label: "Debrief", Primary: true}}
		case o != nil && !late:
			step.Tag, step.Text = "Owed", "Upsolve "+strings.Join(o.Indexes, " ")+" from "+o.Short
			if o.DueMs > 0 {
				step.Facts = "lapses " + when(now, time.UnixMilli(o.DueMs), ends)
			}
			h.Buttons = []plugin.Button{{Key: key("upsolve", o.Round), Label: "Upsolve", Primary: true}}
			stepKey = "upsolve:" + o.Round
		case !st.Today.Done && e.Task == "":
			step.Tag, step.Text = "Next", "Today's problem"
			step.Facts = "opens at " + st.Today.StudyFrom
			label := "Start early"
			if st.Today.StudyTime {
				step.Facts, label = "edge hasn't picked one", "Get one"
			}
			h.Buttons = []plugin.Button{{Key: "next", Label: label, Primary: true}}
		case st.Today.Done && len(st.Resolves.Upcoming) > 0 && !late:
			rs := st.Resolves.Upcoming[0]
			step.Tag, step.TagMood, step.Text, step.Facts = "Next", plugin.Wait, "Cold re-solve · "+rs.Title, when(now, rs.Due, ends)
			stepKey = "resolve:" + rs.Title
		case st.Today.Done && st.Rounds != nil && st.Rounds.Next != nil:
			n := st.Rounds.Next
			step.Tag, step.TagMood, step.Text, step.Facts = "Next", plugin.Quiet, roundName(n.Short, n.Kind), when(now, n.Start, ends)+" · you're in"
			stepKey = "round:" + n.ID
		}
	}
	switch {
	case live != nil: // the round is the day
		h.Ring = &plugin.HeroRing{Mode: "countdown", StartMs: live.Start.UnixMilli(), EndMs: live.End.UnixMilli(), Mood: plugin.Nudge}
		h.Ember.Mood, h.Ember.Pose = plugin.Work, "game"
		h.Title, h.Aside = live.Short+" is live", left(now, live.End)+" left"
		h.Chips = chips(live.Problems)
		solved := 0
		for _, p := range live.Problems {
			if p.State == "solved" {
				solved++
			}
		}
		h.Sub = fmt.Sprintf("%d solved · Claude is off until %s", solved, live.End.Format("15:04"))
		h.Stat = nil
		step.Tag = "Next"
		for _, p := range live.Problems {
			if p.State != "solved" {
				step.Text = "Problem " + p.Index
				if p.State == "tried" {
					step.Facts = plural(p.Fails, "wrong answer")
				}
				break
			}
		}
		h.Buttons = []plugin.Button{{Key: "openround:" + live.ID, Label: "Open round", Primary: true}}
	case st.Today.Done:
		h.Ring.Mood, h.Ember.Mood, h.Ember.Pose = plugin.Done, plugin.Done, "proud"
		h.Title = "Today's done"
		if st.Today.Got > st.Today.Need {
			h.Aside = fmt.Sprintf("%d of %d", st.Today.Got, st.Today.Need)
		}
		h.Sub = "Claude is open · " + mins(time.Duration(st.Today.WorkSec)*time.Second) + " focused today"
		if milestone(st.Streak.Current) {
			h.Title, h.Aside, h.Stat = fmt.Sprintf("%d days in a row", st.Streak.Current), "", nil
			h.Sub = "Today's done · your best streak yet"
			if st.Streak.Current < st.Streak.Best {
				h.Sub = "Today's done"
			}
			h.Ember.React = "celebrate"
		}
		if late {
			h.Ember.Mood, h.Ember.Pose = plugin.Quiet, "sleepy"
			h.Sub = "Next problem tomorrow at " + st.Today.StudyFrom
		}
		after()
	default:
		h.Title = fmt.Sprintf("%d to go today", st.Today.Need-st.Today.Got)
		h.Sub = gateWords(st, e)
		if st.Today.Gate == "closed" {
			h.SubIcon, h.SubMood = "lock", plugin.Nudge
		}
		if late {
			h.Sub, h.SubMood = "The day ends at "+ends, plugin.Nudge
		}
		switch {
		case working && e.Paused:
			h.Title, h.Aside = "Paused", mins(e.workAt(now))
		case working:
			h.Ember.Mood, h.Ember.Pose = plugin.Work, "attentive"
			h.Title, h.Aside = "Practising", mins(e.workAt(now))
		case st.Today.Gate == "closed" || late:
			h.Ember.Mood, h.Ember.Pose = plugin.Nudge, "sidelong"
		}
		if e.Task == "" {
			after()
		}
	}
	if e.Task != "" && live == nil && !st.Today.Done { // the problem: the next step
		taskStep(&h, step, st, cur, e, now)
		if e.Kind == "upsolve" {
			stepKey = "upsolve:" + upsolveRound(st, e.TaskID)
		}
	}
	if step.Text != "" {
		h.Step = step
	}

	// ---- the week, and what's coming ----
	wk := plugin.Widget{Kind: "bars", Layout: "columns", Title: "This week", Value: fmt.Sprintf("%d/%d", st.Week.Credits, st.Week.Target)}
	future := false
	for i, d := range st.Week.Days {
		today := d.Date == st.Today.Date
		it := plugin.Item{Label: []string{"M", "T", "W", "T", "F", "S", "S"}[i%7], Value: float64(d.Credits), Num: float64(d.Credits),
			Target: plugin.Float(float64(d.Need)), Today: today, Mood: plugin.Nudge}
		if d.Met {
			it.Mood = plugin.Done
		}
		switch {
		case future:
			it.State = "future"
		case d.Pass:
			it.State = "excused"
		}
		wk.Items = append(wk.Items, it)
		future = future || today
	}
	if togo := st.Week.Target - st.Week.Credits; togo > 0 {
		wk.Caption = fmt.Sprintf("%d to go", togo)
	}
	ag := plugin.Widget{Kind: "agenda", Title: "Coming up", DayEnds: ends, Empty: "Nothing owed or booked this week"}
	type ev struct {
		key string
		it  plugin.Item
	}
	var evs []ev
	if r := st.Rounds; r != nil {
		for _, rd := range r.Upcoming {
			if rd.Plan != "in" || (live != nil && rd.ID == live.ID) {
				continue
			}
			evs = append(evs, ev{"round:" + rd.ID, plugin.Item{AtMs: rd.Start.UnixMilli(), Title: roundName(rd.Short, rd.Kind), Tag: "you're in", TagMood: plugin.Nudge}})
		}
		if live != nil {
			evs = append(evs, ev{"", plugin.Item{AtMs: live.End.UnixMilli(), Title: "Round ends · debrief after"}})
		}
	}
	var owed []ev // owed upsolves: always on the agenda, pressable
	for _, o := range e.Owed {
		var ps []Problem
		for _, i := range o.Indexes {
			ps = append(ps, Problem{Index: i, State: "upsolve"})
		}
		owed = append(owed, ev{"upsolve:" + o.Round, plugin.Item{AtMs: o.DueMs, Title: o.Short + " upsolves lapse", Chips: chips(ps), Key: key("upsolve", o.Round)}})
	}
	for i, rs := range st.Resolves.Upcoming {
		if i == 2 {
			break
		}
		evs = append(evs, ev{"resolve:" + rs.Title, plugin.Item{AtMs: rs.Due.UnixMilli(), Title: "Re-solve · " + rs.Title, Tag: "cold", TagMood: plugin.Wait}})
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].it.AtMs < evs[j].it.AtMs })
	for _, x := range append(owed, evs...) {
		if stepKey != "" && x.key == stepKey { // the hero's next step already says it
			continue
		}
		if time.UnixMilli(x.it.AtMs).Before(now) || len(ag.Items) == 4 {
			continue
		}
		ag.Items = append(ag.Items, x.it)
	}
	sort.SliceStable(ag.Items, func(i, j int) bool { return ag.Items[i].AtMs < ag.Items[j].AtMs })

	// ---- am I improving ----
	cf := st.Sources["cf"]
	var vals []float64
	for _, r := range cf.RatingHistory {
		vals = append(vals, float64(r.Rating))
	}
	if len(vals) > 20 {
		vals = vals[len(vals)-20:]
	}
	ln := plugin.Widget{Kind: "line", Title: "Codeforces", Value: fmt.Sprint(cf.Rating), Values: vals}
	if cf.BandHi > cf.BandLo {
		ln.Band = &plugin.Band{Lo: float64(cf.BandLo), Hi: float64(cf.BandHi), Label: fmt.Sprintf("problems %d–%d", cf.BandLo, cf.BandHi)}
	}
	if n := len(vals); n >= 2 && now.Sub(cf.RatingHistory[len(cf.RatingHistory)-1].At) <= 7*24*time.Hour { // last week's change only
		d := int(vals[n-1] - vals[n-2])
		ln.Delta, ln.DeltaMood = fmt.Sprintf("+%d", d), plugin.Done
		if d < 0 {
			ln.Delta, ln.DeltaMood = fmt.Sprintf("−%d", -d), plugin.Fail
		}
	}
	if lc, ok := st.Sources["lc"]; ok && lc.Rating > 0 {
		ln.Extra = fmt.Sprint("LC ", lc.Rating)
	}
	solo := plugin.Item{Label: "Solo", Mood: plugin.Done, Value: "–", Caption: "none yet"}
	soloS := "–"
	if st.Totals.Solved > 0 {
		v := 100 * float64(st.Totals.Solo) / float64(st.Totals.Solved)
		solo.Value, solo.Unit, solo.Caption = v, "%", fmt.Sprintf("%d of %d", st.Totals.Solo, st.Totals.Solved)
		soloS = fmt.Sprintf("%d%%", int(math.Round(v)))
	}
	ret := plugin.Item{Label: "Retained", Mood: plugin.Wait, Value: "–"}
	retS := "–"
	if st.Resolves.FirstAttempts > 0 {
		v := 100 * float64(st.Resolves.FirstRetained) / float64(st.Resolves.FirstAttempts)
		ret.Value, ret.Unit, ret.Caption = v, "%", fmt.Sprintf("%d of %d", st.Resolves.FirstRetained, st.Resolves.FirstAttempts)
		retS = fmt.Sprintf("%d%%", int(math.Round(v)))
	} else if len(st.Resolves.Upcoming) > 0 {
		ret.Caption = "first " + st.Resolves.Upcoming[0].Due.Format("Mon 2")
	}
	wsec := 0
	for _, d := range st.Week.Days {
		wsec += d.WorkSec
	}
	focus := plugin.Item{Label: "Focus", Mood: plugin.Nudge, Value: float64(wsec), Unit: "s", Caption: "this week"}
	rounds := plugin.Item{Label: "Rounds", Mood: plugin.Quiet, Value: "–", Caption: "none held yet"}
	if r := st.Rounds; r != nil && r.Record.Held > 0 {
		rounds.Value, rounds.Caption = fmt.Sprintf("%d of %d", r.Record.Sat, r.Record.Held), "sat"
		rounds.Mood = plugin.Nudge // the dot follows the record
		if 4*r.Record.Sat >= 3*r.Record.Held {
			rounds.Mood = plugin.Done
		}
	}
	stats := plugin.Widget{Kind: "stats", Cols: 2, CountUp: true, Items: []plugin.Item{solo, ret, focus, rounds}}

	// ---- the record: the last months, and the last problems, honestly ----
	heat := plugin.Widget{Kind: "heatmap", Title: "Last 6 months", Weeks: 26}
	met := 0
	for _, d := range st.Calendar {
		day := plugin.Day{Date: d.Date}
		switch {
		case d.Met:
			day.Level = 1
		case d.Credits > 0:
			day.Level = math.Min(1, float64(d.Credits)/float64(max(d.Need, 1))) * 0.99
		case d.Pass:
			day.Level, day.Mood = 1, plugin.Wait
		case d.CFSolved > 0:
			day.Level, day.Mood = 1, plugin.Quiet
		}
		heat.Days = append(heat.Days, day)
	}
	for _, d := range st.Calendar[max(0, len(st.Calendar)-182):] {
		if d.Met {
			met++
		}
	}
	heat.Caption = plural(met, "day") + " met"
	ls := plugin.Widget{Kind: "list", Title: "Lately"}
	fin := 0
	for i := range st.Recent {
		if len(ls.Items) == 4 {
			break
		}
		t := &st.Recent[i]
		it := plugin.Item{Title: taskName(t)}
		switch t.Outcome {
		case "solo", "assisted":
			fin++
			it.Mark, it.Sub = "done", t.Outcome
			if t.WorkSec >= 60 {
				it.Sub += " · " + mins(time.Duration(t.WorkSec)*time.Second)
			}
		case "gaveup":
			it.Mark, it.Sub = "fail", "gave up"
		case "skipped":
			it.Mark, it.Sub = "skip", "skipped"
			if t.WorkSec >= 60 {
				it.Sub += " after " + mins(time.Duration(t.WorkSec)*time.Second)
			}
		default:
			it.Mark, it.Sub = "none", "never started"
		}
		ls.Items = append(ls.Items, it)
	}
	ls.Caption = fmt.Sprintf("%d of %d finished", fin, len(ls.Items))

	// ---- edge's trouble, or the experiment when it moved ----
	note := plugin.Widget{Kind: "note", Icon: "chart", Mood: plugin.Quiet}
	if x := st.Experiment; expMoved(st, now) {
		if x.Status == "decided" {
			note.Text = "Hint styles: " + x.Verdict + " wins"
		} else {
			note.Text = fmt.Sprintf("Hint experiment · ladder %d, instant %d of %d each · too early to call", x.Arms["ladder"].New, x.Arms["instant"].New, x.Target)
		}
	}
	switch {
	case e.Stopped:
		note = plugin.Widget{Kind: "note", Icon: "warning", Text: "edge is stopped · nothing is being counted", Mood: plugin.Fail}
	case e.Health != "":
		note = plugin.Widget{Kind: "note", Icon: "warning", Text: e.Health, Mood: plugin.Fail}
	}

	// ---- what's needed now; the rest folds to a line ----
	focusS := mins(time.Duration(wsec) * time.Second)
	improving := plugin.Widget{Kind: "columns", Title: "Progress", Split: 0.56, A: &ln, B: &stats,
		Summary: fmt.Sprintf("CF %d %s · solo %s · retained %s · %s this week", cf.Rating, ln.Delta, soloS, retS, focusS)}
	record := plugin.Widget{Kind: "columns", Title: "Record", Split: 0.42, A: &heat, B: &ls,
		Summary: ls.Caption + " lately · " + heat.Caption + " in 6 months"}
	switch {
	case live != nil, late:
		improving.Fold, record.Fold = true, true
	case !st.Today.Done && st.Today.Gate == "closed":
		record.Fold = true
	}
	if phase := phaseOf(st, e); trialOn(phase, st) {
		return trialDashboard(st, e, cur, h, ag, ln, note, live != nil, now)
	}
	ws := []plugin.Widget{h, {Kind: "columns", Split: 0.42, A: &wk, B: &ag}, improving, record}
	if note.Text != "" && (note.Mood == plugin.Fail || !(live != nil || late)) {
		ws = append(ws, note)
	}
	if phaseOf(st, e) == "baseline" || (st.Consistency != nil && st.Consistency.Reverted) {
		if pn, ok := phaseNote(st, now); ok && !(live != nil || late) {
			ws = append(ws, pn)
		}
	}
	return plugin.Dashboard{Title: "edge", Widgets: ws}
}

// trialDashboard is the page in a trial, in the order it's read: tonight's
// rep, the chain, readiness (Trial 2), the commonest mistake, then contests
// and the rating, folded small.
func trialDashboard(st *Stats, e *Status, cur *Task, h plugin.Widget, ag, ln, note plugin.Widget, live bool, now time.Time) plugin.Dashboard {
	if !live {
		trialHero(&h, st, cur, e, now)
	}
	ws := []plugin.Widget{h}
	c := st.Consistency
	if c == nil {
		c = &Consistency{}
	}
	ws = append(ws, chainWidget(c, e))
	r := st.Readiness
	if r != nil && phaseOf(st, e) == "trial2" && len(r.Patterns) > 0 {
		ws = append(ws, readinessWidget(r))
	}
	if r != nil && phaseOf(st, e) == "trial2" {
		if m, ok := mistakeNote(r); ok {
			ws = append(ws, m)
		}
	}
	contest := plugin.Widget{Kind: "columns", Title: "Contests", Split: 0.56, A: &ag, B: &ln, Fold: !live,
		Summary: "CF " + ln.Value}
	if ln.Delta != "" {
		contest.Summary += " " + ln.Delta
	}
	if len(ag.Items) > 0 {
		contest.Summary += " · next: " + ag.Items[0].Title + " " + when(now, time.UnixMilli(ag.Items[0].AtMs), st.Today.DayEnds)
	}
	if live {
		ws[1].Fold = true
	}
	ws = append(ws, contest)
	if note.Text != "" && note.Mood == plugin.Fail { // edge's trouble only: the hint experiment is over
		ws = append(ws, note)
	}
	return plugin.Dashboard{Title: "edge", Widgets: ws}
}

// taskStep is the hero's step for the task: what it is, where it stands,
// and its buttons: Hint (when one is ready) or Open, then Give up or Skip.
func taskStep(h *plugin.Widget, step *plugin.Step, st *Stats, cur *Task, e *Status, now time.Time) {
	step.Tag, step.Text = kindTag(e.Kind), e.Task
	if cur != nil {
		step.Tag, step.Text = kindTag(cur.Kind), taskName(cur)
		if cur.Rating > 0 {
			step.Meta = fmt.Sprint(cur.Rating)
		}
	}
	var facts []string
	id := e.TaskID
	if !e.active() {
		switch {
		case e.Lapsed:
			facts = append(facts, "lapsed")
		case e.Kind == "upsolve" && e.DueMs > 0:
			facts = append(facts, "lapses "+when(now, time.UnixMilli(e.DueMs), st.Today.DayEnds))
		}
		h.Buttons = []plugin.Button{{Key: key("start", id), Label: "Start", Primary: true}}
		if e.CanSkip {
			h.Buttons = append([]plugin.Button{{Key: key("skip", id), Label: "Skip"}}, h.Buttons...)
		}
		step.Facts = strings.Join(facts, " · ")
		return
	}
	switch {
	case e.Judging != "" && !e.Paused:
		facts = append(facts, judging(e.Judging))
		step.TagMood = plugin.Wait
	case e.LastVerdict != "":
		facts = append(facts, e.LastVerdict)
		step.TagMood = plugin.Fail
	case cur != nil && cur.Fails > 0:
		facts = append(facts, plural(cur.Fails, "wrong answer"))
	}
	hinted := e.Policy == "ladder" || e.Policy == "instant"
	switch {
	case e.HintsReady:
		facts = append(facts, "hint ready")
	case e.NextHintSec > 0:
		facts = append(facts, "hint in "+mins(time.Duration(e.NextHintSec)*time.Second))
	case !hinted:
		facts = append(facts, "no hints")
	}
	if e.GiveUpInSec > 0 {
		facts = append(facts, "give up in "+mins(time.Duration(e.GiveUpInSec)*time.Second))
	}
	if e.Kind == "upsolve" && e.DueMs > 0 {
		facts = append(facts, "lapses "+when(now, time.UnixMilli(e.DueMs), st.Today.DayEnds))
	}
	if e.Stopped {
		facts = append(facts, "edge is stopped")
	}
	step.Facts = strings.Join(facts, " · ")
	if hinted && cur != nil {
		gap := 900.0 // the ladder's rungs: 20, 35, 50 minutes of work
		if cur.HintsAllowed == 0 {
			gap = 1200
		}
		step.Dots = &plugin.Dots{Used: cur.Hints, Ready: cur.HintsAllowed, Total: 3, Next: math.Max(0.08, 1-float64(e.NextHintSec)/gap)}
	}
	open := plugin.Button{Key: key("start", id), Label: "Open", Primary: true}
	main, second := open, plugin.Button{}
	if e.HintsReady {
		main, second = plugin.Button{Key: key("hint", id), Label: "Hint", Primary: true}, plugin.Button{Key: open.Key, Label: "Open"}
	}
	switch {
	case e.GiveUpInSec == 0:
		second = plugin.Button{Key: key("giveup", id), Label: "Give up"}
	case e.CanSkip:
		second = plugin.Button{Key: key("skip", id), Label: "Skip"}
	}
	h.Buttons = []plugin.Button{main}
	if second.Key != "" {
		h.Buttons = []plugin.Button{second, main}
	}
}

// upsolveRound is the round an owed task belongs to.
func upsolveRound(st *Stats, id string) string {
	if st.Rounds != nil {
		for _, q := range st.Rounds.Upsolves.Queue {
			if q.ID == id {
				return q.Round
			}
		}
	}
	return ""
}

// explainName is the finished task waiting for its explain-back, by name.
func explainName(st *Stats, id string) string {
	for i := range st.Recent {
		if t := &st.Recent[i]; t.ID == id {
			if t.Source == "cf" {
				return strings.TrimPrefix(t.ID, "cf:")
			}
			return shortTask(t.Title)
		}
	}
	if i := strings.Index(id, ":"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// roundShort is a round's short name, from what the stats know.
func roundShort(st *Stats, id string) string {
	if r := st.Rounds; r != nil {
		for _, rs := range [][]StatsRound{r.Recent, r.Upcoming} {
			for _, x := range rs {
				if x.ID == id && x.Short != "" {
					return x.Short
				}
			}
		}
	}
	return id
}

// expMoved: the hint experiment's numbers changed in the last day (a new
// problem finished), so its note is news.
func expMoved(st *Stats, now time.Time) bool {
	for _, t := range st.Recent {
		if t.Kind == "new" && !t.Finished.IsZero() && now.Sub(t.Finished) < 24*time.Hour {
			return true
		}
	}
	return false
}
