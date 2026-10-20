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
	if t.Hour()*60+t.Minute() <= hm.Hour()*60+hm.Minute() {
		return t.AddDate(0, 0, -1)
	}
	return t
}

// when says how far off t is, the way the agenda does: "in 1h 07m", "18:35",
// "Sun night", "Mon 20:05", "Thu 29".
func when(now, t time.Time, ends string) string {
	d := t.Sub(now)
	day := func(x time.Time) time.Time { return dayOf(x, ends) }
	switch {
	case d < 0:
		return "now"
	case d < 3*time.Hour:
		return "in " + mins(d)
	case day(t).YearDay() == day(now).YearDay() && day(t).Year() == day(now).Year():
		return t.Format("15:04")
	case d < 6*24*time.Hour:
		if ends != "" && day(t).YearDay() != t.YearDay() {
			return day(t).Format("Mon") + " night"
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

func gateWords(st *Stats) string {
	t := st.Today
	switch t.Gate {
	case "closed":
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

// Dashboard is edge's page for this moment.
func Dashboard(st *Stats, now time.Time) plugin.Dashboard {
	ends := st.Today.DayEnds
	late := now.Hour() >= 23 || now.Hour() < 4
	cur := st.Current
	working := cur != nil && !cur.Started.IsZero() && cur.Outcome == ""
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
		r := st.Rounds
		switch { // after the day: what's owed, else what's next
		case r != nil && len(r.Upsolves.Queue) > 0 && !late:
			q := r.Upsolves.Queue[0]
			step.Tag, step.Text, step.Facts = "Extra", "Upsolve "+q.Index+" from "+q.Short, "lapses "+when(now, q.Due, ends)
			h.Buttons = []plugin.Button{{Key: "upsolve", Label: "Upsolve", Primary: true}}
			stepKey = "upsolve:" + q.Short
		case len(st.Resolves.Upcoming) > 0 && !late:
			rs := st.Resolves.Upcoming[0]
			step.Tag, step.TagMood, step.Text, step.Facts = "Next", plugin.Wait, "Cold re-solve · "+rs.Title, when(now, rs.Due, ends)
			stepKey = "resolve:" + rs.Title
		case r != nil && r.Next != nil:
			n := r.Next
			step.Tag, step.TagMood, step.Text, step.Facts = "Next", plugin.Quiet, roundName(n.Short, n.Kind), when(now, n.Start, ends)+" · you're in"
			stepKey = "round:" + n.ID
		}
	default:
		h.Title = fmt.Sprintf("%d to go today", st.Today.Need-st.Today.Got)
		h.Sub = gateWords(st)
		if st.Today.Gate == "closed" {
			h.SubIcon, h.SubMood = "lock", plugin.Nudge
		}
		if late {
			h.Sub, h.SubMood = "The day ends at "+ends, plugin.Nudge
		}
		switch {
		case working:
			h.Ember.Mood, h.Ember.Pose = plugin.Work, "attentive"
			h.Title, h.Aside = "Practising", mins(time.Duration(cur.WorkSec)*time.Second)
		case st.Today.Gate == "closed" || late:
			h.Ember.Mood, h.Ember.Pose = plugin.Nudge, "sidelong"
		}
	}
	if cur != nil && live == nil && !st.Today.Done { // the problem: the next step
		step.Tag, step.Text = kindTag(cur.Kind), taskName(cur)
		if cur.Rating > 0 {
			step.Meta = fmt.Sprint(cur.Rating)
		}
		var facts []string
		if working {
			switch {
			case cur.HintsAllowed > cur.Hints:
				facts = append(facts, "hint ready")
			case cur.NextHintSec > 0:
				facts = append(facts, "hint in "+mins(time.Duration(cur.NextHintSec)*time.Second))
			}
			if cur.Fails > 0 {
				facts = append(facts, plural(cur.Fails, "wrong"))
			}
			step.Dots = &plugin.Dots{Used: cur.Hints, Ready: cur.HintsAllowed, Total: 3, Next: math.Max(0.08, 1-float64(cur.NextHintSec)/900)}
			h.Buttons = []plugin.Button{{Key: "open", Label: "Open", Primary: true}}
			if cur.HintsAllowed > cur.Hints {
				h.Buttons = []plugin.Button{{Key: "hint", Label: "Hint", Primary: true}}
			}
		} else {
			if cur.Kind == "upsolve" && st.Rounds != nil && len(st.Rounds.Upsolves.Queue) > 0 {
				facts = append(facts, "lapses "+when(now, st.Rounds.Upsolves.Queue[0].Due, ends))
			}
			h.Buttons = []plugin.Button{{Key: "start", Label: "Start", Primary: true}}
		}
		step.Facts = strings.Join(facts, " · ")
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
		owed, dues, order := map[string][]Problem{}, map[string]time.Time{}, []string{}
		for _, q := range r.Upsolves.Queue {
			if owed[q.Short] == nil {
				order = append(order, q.Short)
			}
			owed[q.Short] = append(owed[q.Short], Problem{Index: q.Index, State: "upsolve"})
			dues[q.Short] = q.Due
		}
		for _, s := range order {
			evs = append(evs, ev{"upsolve:" + s, plugin.Item{AtMs: dues[s].UnixMilli(), Title: s + " upsolves lapse", Chips: chips(owed[s])}})
		}
	}
	for i, rs := range st.Resolves.Upcoming {
		if i == 2 {
			break
		}
		evs = append(evs, ev{"resolve:" + rs.Title, plugin.Item{AtMs: rs.Due.UnixMilli(), Title: "Re-solve · " + rs.Title, Tag: "cold", TagMood: plugin.Wait}})
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].it.AtMs < evs[j].it.AtMs })
	for _, e := range evs {
		if stepKey != "" && e.key == stepKey { // the hero's next step already says it
			continue
		}
		if time.UnixMilli(e.it.AtMs).Before(now) || len(ag.Items) == 4 {
			continue
		}
		ag.Items = append(ag.Items, e.it)
	}

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
	if n := len(vals); n >= 2 {
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
	rounds := plugin.Item{Label: "Rounds", Mood: plugin.Fail, Value: "–", Caption: "none held yet"}
	if r := st.Rounds; r != nil && r.Record.Held > 0 {
		rounds.Value, rounds.Caption = fmt.Sprintf("%d/%d", r.Record.Sat, r.Record.Held), "held sat"
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

	// ---- the experiment, or edge's trouble ----
	note := plugin.Widget{Kind: "note", Icon: "chart", Mood: plugin.Quiet}
	e := st.Experiment
	if e.Status == "decided" {
		note.Text = "Hint styles: " + e.Verdict + " wins"
	} else {
		note.Text = fmt.Sprintf("Hint experiment · ladder %d, instant %d of %d each · too early to call", e.Arms["ladder"].New, e.Arms["instant"].New, e.Target)
	}
	if len(st.Health) > 0 {
		note = plugin.Widget{Kind: "note", Icon: "warning", Text: st.Health[0].Msg, Mood: plugin.Fail}
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
	ws := []plugin.Widget{h, {Kind: "columns", Split: 0.42, A: &wk, B: &ag}, improving, record}
	if !(live != nil || late) {
		ws = append(ws, note)
	}
	return plugin.Dashboard{Title: "edge", Widgets: ws}
}
