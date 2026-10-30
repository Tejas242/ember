package adapter

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// The pill's line, the card's row, edge's cards and Ember's reactions.

func chips(ps []Problem) []plugin.Chip {
	if len(ps) == 0 {
		return nil
	}
	state := map[string]string{"solved": "done", "tried": "tried", "upsolve": "owed", "upsolved": "fixed", "taught": "fixed",
		"lapsed": "gone", "dropped": "gone"}
	out := make([]plugin.Chip, 0, len(ps))
	for i, p := range ps {
		if i == 9 {
			break
		}
		out = append(out, plugin.Chip{Label: p.Index, State: state[p.State]})
	}
	return out
}

// upsolves is the letters still owed, "C D".
func upsolves(ps []Problem) string {
	var ls []string
	for _, p := range ps {
		if p.State == "upsolve" {
			ls = append(ls, p.Index)
		}
	}
	return strings.Join(ls, " ")
}

func solvedLetters(ps []Problem) string {
	var ls []string
	for _, p := range ps {
		if p.State == "solved" {
			ls = append(ls, p.Index)
		}
	}
	return strings.Join(ls, " ")
}

// shown: edge has something for the pill: a task, the gate, a goal, a round.
func shown(e *Status) bool {
	return e != nil && (e.Task != "" || e.Closed || e.Need > 0 || e.Round != nil)
}

// roundLeads: the round takes the pill and the row. A task you're working
// on keeps them until 15 minutes before the round, and after it.
func roundLeads(e *Status, r *Round, now time.Time) bool {
	if !e.active() {
		return true
	}
	switch r.phase(now) {
	case "live":
		return true
	case "soon":
		return r.start().Sub(now) <= 15*time.Minute
	}
	return false
}

// judging is a submission being judged, in words: "judging · test 9".
func judging(j string) string {
	if j == "" || j == "queue" {
		return "judging"
	}
	return "judging · " + j
}

// taskNews is what's new about the task you're on: a submission being
// judged, the last rejection, or a round coming up.
func taskNews(e *Status, now time.Time) (string, string) {
	switch {
	case e.Paused || e.Stopped:
	case e.Judging != "":
		return judging(e.Judging), plugin.Wait
	case e.LastVerdict != "":
		return e.LastVerdict, plugin.Fail
	}
	if r := e.Round; r != nil && r.phase(now) == "soon" {
		return r.Short + " " + until(now, r.start()), ""
	}
	return "", ""
}

// Line is edge's line in the pill, nil when it has nothing to say.
func Line(e *Status, now time.Time) *plugin.Status {
	if !shown(e) {
		return nil
	}
	s := &plugin.Status{}
	if e.Closed {
		s.BadgeClock = "lock" // the clock says Claude is locked, too
	}
	sal := func(v float64) { s.Salience = plugin.Float(v) }
	if r := e.Round; r != nil && roundLeads(e, r, now) {
		s.Text = r.Short
		switch r.phase(now) {
		case "soon":
			s.Mood, s.Sub = plugin.Nudge, until(now, r.start())
			s.UntilMs, s.Timer, s.WarnMs = r.StartMs, "none", (15 * time.Minute).Milliseconds()
			switch d := r.start().Sub(now); {
			case d <= 15*time.Minute:
				sal(0.9)
			case d <= 2*time.Hour:
				sal(0.5)
			default:
				sal(0.3)
			}
		case "live":
			s.Mood, s.Chips = plugin.Work, chips(r.Problems)
			s.UntilMs, s.Timer, s.WarnMs = r.EndMs, "h:mm", (10 * time.Minute).Milliseconds()
			if r.Locked {
				s.Badge = "lock"
			}
			sal(0.5)
			if r.Locked || r.Joined != "" { // you're in it: it's the most important thing on the island
				sal(0.95)
				s.Focus = true
			}
			s.Satellite = r.Locked // its clock stays in sight
		default:
			s.Mood, s.Chips = plugin.Done, chips(r.Problems)
			if r.Joined == "" {
				s.Mood = plugin.Nudge
			}
			if up := upsolves(r.Problems); up != "" {
				s.Sub = "upsolve " + strings.Fields(up)[0]
			}
			sal(0.6)
		}
		return s
	}
	sal(0)
	if e.active() || e.Closed {
		sal(0.55)
	}
	if e.active() { // practising: the clock is work, and runs only while edge counts it
		s.Text, s.Mood = e.label(), plugin.Work
		w := e.workAt(now)
		switch {
		case e.Paused:
			s.Mood, s.Value = plugin.Quiet, "paused · "+mins(w)
			sal(0.2)
		case e.ticking():
			s.SinceMs, s.Satellite = e.AsOfMs-int64(e.WorkSec)*1000, true
		default:
			s.Value = mins(w)
		}
		if sub, mood := taskNews(e, now); sub != "" {
			s.Sub = sub
			if mood != "" {
				s.Mood = mood
			}
		}
		return s
	}
	s.Text, s.Mood = fmt.Sprintf("Today %d/%d", e.Today, e.Need), plugin.Nudge
	if e.Closed {
		s.Badge = "lock"
	}
	if e.Task != "" {
		s.Value = e.label()
	}
	return s
}

// FromStats is edge's day as its stats say, before its first status: the
// task too, so a restart doesn't lose it.
func FromStats(st *Stats) *Status {
	if st == nil {
		return &Status{}
	}
	e := &Status{Today: st.Today.Got, Need: st.Today.Need, Streak: st.Streak.Current, Closed: st.Today.Gate == "closed", GiveUpInSec: -1}
	if c := st.Current; c != nil {
		e.Task, e.TaskID, e.Kind, e.URL, e.WorkSec, e.Short = taskName(c), c.ID, c.Kind, c.URL, c.WorkSec, taskShort(c)
		if c.HintsAllowed > 0 || c.NextHintSec > 0 {
			e.Policy = "ladder"
		}
		if c.started() {
			e.StartedMs, e.GiveUpInSec, e.NextHintSec = c.Started.UnixMilli(), c.GiveUpInSec, c.NextHintSec
			e.HintsReady = c.HintsAllowed > c.Hints
			e.CanSkip = c.Kind == "upsolve" || (c.Kind == "new" && c.WorkSec < 600)
		} else {
			e.Pending = true
		}
	}
	if r := st.Rounds; r != nil {
		for _, q := range r.Upsolves.Queue {
			if st.Current != nil && q.ID == st.Current.ID {
				e.DueMs = q.Due.UnixMilli()
			}
			if n := len(e.Owed); n > 0 && e.Owed[n-1].Round == q.Round {
				e.Owed[n-1].Indexes = append(e.Owed[n-1].Indexes, q.Index)
				continue
			}
			e.Owed = append(e.Owed, Owed{Round: q.Round, Short: q.Short, Indexes: []string{q.Index}, DueMs: q.Due.UnixMilli()})
		}
	}
	if len(st.Health) > 0 {
		e.Health = st.Health[0].Msg
	}
	if !st.Daemon.Seen.IsZero() {
		e.SeenMs = st.Daemon.Seen.UnixMilli()
	}
	return e
}

// owedNow is the owed round to offer: the oldest one not lapsed.
func owedNow(e *Status, now time.Time) *Owed {
	for i := range e.Owed {
		if o := &e.Owed[i]; o.DueMs == 0 || time.UnixMilli(o.DueMs).After(now) {
			return o
		}
	}
	return nil
}

// Row is edge's row in the card: its goal (or the round's clock) on the
// rail, the task or the round, where it stands, and the one thing to do
// (one button: two crush the words).
func Row(e *Status, now time.Time) plugin.Row {
	if r := e.Round; r != nil && roundLeads(e, r, now) {
		return roundRow(r, now)
	}
	goal := 0.0
	if e.Need > 0 {
		goal = math.Min(1, float64(e.Today)/float64(e.Need))
	}
	ring := &plugin.Ring{Value: plugin.Float(goal), Mood: plugin.Nudge}
	if goal >= 1 {
		ring.Mood = plugin.Done
	}
	row := plugin.Row{Title: e.Task, Ring: ring, Sub: fmt.Sprintf("%d/%d today", e.Today, e.Need)}
	if e.Streak > 0 {
		row.Sub += fmt.Sprintf(" · %d-day streak", e.Streak)
		row.Flame = fmt.Sprintf("%d-day streak", e.Streak) // the streak gets its little flame
	}
	switch {
	case e.active():
		row.Flame = ""
		w := e.workAt(now)
		row.Sub = "Practising · " + mins(w)
		if e.Paused {
			row.Sub = "Paused · " + mins(w)
		}
		row.Buttons = []plugin.Button{{Key: key("start", e.TaskID), Label: "Open", Primary: true}}
		if e.HintsReady {
			row.Buttons = []plugin.Button{{Key: key("hint", e.TaskID), Label: "Hint", Primary: true}}
		}
		if sub, _ := taskNews(e, now); sub != "" {
			row.Sub += " · " + sub
		} else if !e.HintsReady && e.NextHintSec > 0 && !e.Paused {
			row.Sub += " · hint in " + mins(time.Duration(e.NextHintSec)*time.Second)
		}
		if e.Stopped {
			row.Sub += " · edge is stopped"
		}
	case e.Task != "":
		row.Buttons = []plugin.Button{{Key: key("start", e.TaskID), Label: "Start", Primary: true}}
		if e.Closed {
			row.Sub, row.Flame = fmt.Sprintf("%d/%d · locked", e.Today, e.Need), ""
		}
	default:
		if o := owedNow(e, now); o != nil {
			row.Title = "Upsolve " + o.Short + " " + strings.Join(o.Indexes, " ")
			row.Buttons = []plugin.Button{{Key: key("upsolve", o.Round), Label: "Upsolve", Primary: true}}
		}
	}
	if row.Title == "" {
		row.Title = "Practice"
	}
	return row
}

// still: a round's ring is its state, not progress: it doesn't sweep in.
var still = new(bool)

func roundRow(r *Round, now time.Time) plugin.Row {
	row := plugin.Row{Title: r.Short}
	open := []plugin.Button{{Key: "openround:" + r.ID, Label: "Open", Primary: true}} // "Can't make it" stays on its card
	switch r.phase(now) {
	case "soon":
		row.Ring = &plugin.Ring{Value: plugin.Float(1), Mood: plugin.Nudge, Sweep: still}
		row.Sub = until(now, r.start())
		if r.Plan == "in" {
			row.Sub += " · you're in"
		}
		if r.start().Sub(now) <= 30*time.Minute {
			row.Buttons = open
		}
	case "live":
		row.Ring = &plugin.Ring{StartMs: r.StartMs, EndMs: r.EndMs, Mood: plugin.Nudge}
		row.Sub, row.Chips = left(now, r.end())+" left", chips(r.Problems)
		if r.Locked {
			row.Badge = "lock"
		}
		if r.Joined == "" {
			row.Buttons = open
		}
	default:
		row.Ring = &plugin.Ring{Value: plugin.Float(1), Mood: plugin.Done, Sweep: still}
		if r.Joined == "" {
			row.Ring.Mood = plugin.Quiet
		}
		switch s := solvedLetters(r.Problems); {
		case r.Joined == "":
			row.Sub = "missed"
		case s == "":
			row.Sub = "nothing solved"
		default:
			row.Sub = "solved " + s
		}
		if up := upsolves(r.Problems); up != "" {
			row.Sub += " · upsolve " + up
			row.Buttons = []plugin.Button{{Key: key("upsolve", r.ID), Label: "Upsolve", Primary: true}}
		}
	}
	return row
}

// Cardify is one of edge's cards as a plugin card, and whether Ember hops for it.
func Cardify(c *Card) (plugin.Card, bool) {
	mood := map[string]string{"work": plugin.Work, "wait": plugin.Wait, "happy": plugin.Done, "error": plugin.Fail,
		"nudge": plugin.Nudge}[c.Mood]
	if mood == "" {
		mood = plugin.Quiet
	}
	out := plugin.Card{ID: c.ID, Eyebrow: "Practice", Title: c.Title, Body: c.Body, Mood: mood, Urgent: c.Urgent, TTLs: c.TTLs}
	for _, b := range c.Buttons {
		out.Buttons = append(out.Buttons, plugin.Button{Key: b.Key, Label: b.Label, Primary: b.Primary})
	}
	return out, c.Bounce
}

// News is how Ember answers edge's news: a hop for a letter solved in a
// round, a celebration for a streak's milestone.
func News(old, cur *Status) []string {
	if old == nil || cur == nil {
		return nil
	}
	var out []string
	if old.Round != nil && cur.Round != nil && old.Round.ID == cur.Round.ID {
		was := map[string]string{}
		for _, p := range old.Round.Problems {
			was[p.Index] = p.State
		}
		for _, p := range cur.Round.Problems {
			if (p.State == "solved" || p.State == "upsolved") && was[p.Index] != p.State {
				out = append(out, "hop")
				break
			}
		}
	}
	if cur.Streak > old.Streak {
		switch cur.Streak {
		case 7, 30, 100, 365:
			out = append(out, "celebrate")
		}
	}
	return out
}
