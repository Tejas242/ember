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
	if r := e.Round; r != nil {
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
	if e.StartedMs > 0 || e.Closed {
		sal(0.55)
	}
	if e.StartedMs > 0 && !e.Pending { // practising
		s.Text, s.Mood, s.SinceMs, s.Satellite = short(e.Task), plugin.Work, e.StartedMs, true
		return s
	}
	s.Text, s.Mood = fmt.Sprintf("Today %d/%d", e.Today, e.Need), plugin.Nudge
	if e.Closed {
		s.Badge = "lock"
	}
	if e.Task != "" {
		s.Value = short(e.Task)
	}
	return s
}

// FromStats is edge's day as its stats say, before its first status.
func FromStats(st *Stats) *Status {
	if st == nil {
		return &Status{}
	}
	return &Status{Today: st.Today.Got, Need: st.Today.Need, Streak: st.Streak.Current, Closed: st.Today.Gate == "closed"}
}

// Row is edge's row in the card: its goal (or the round's clock) on the
// rail, the task or the round, where it stands, and the one thing to do.
func Row(e *Status, now time.Time) plugin.Row {
	if r := e.Round; r != nil {
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
	row := plugin.Row{Title: e.Task, Ring: ring, Sub: fmt.Sprintf("%d/%d today · %d-day streak", e.Today, e.Need, e.Streak)}
	switch {
	case e.StartedMs > 0 && !e.Pending:
		row.Sub = "Practising · " + mins(now.Sub(time.UnixMilli(e.StartedMs)))
		row.Buttons = []plugin.Button{{Key: "hint", Label: "Hint", Primary: true}}
	case e.Task != "":
		row.Buttons = []plugin.Button{{Key: "start", Label: "Start", Primary: true}}
		if e.Closed {
			row.Sub = "Claude is locked until you practise"
		}
	}
	if row.Title == "" {
		row.Title = "Practice"
	}
	if streak := fmt.Sprintf("%d-day streak", e.Streak); e.Streak > 0 && strings.HasSuffix(row.Sub, " · "+streak) {
		row.Flame = streak // the streak gets its little flame
	}
	return row
}

// still: a round's ring is its state, not progress: it doesn't sweep in.
var still = new(bool)

func roundRow(r *Round, now time.Time) plugin.Row {
	row := plugin.Row{Title: r.Short}
	open := []plugin.Button{{Key: "skipround:" + r.ID, Label: "Can't make it"}, {Key: "openround:" + r.ID, Label: "Open", Primary: true}}
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
			row.Buttons = []plugin.Button{{Key: "upsolve", Label: "Upsolve", Primary: true}}
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
	out := plugin.Card{ID: c.ID, Eyebrow: "Practice", Title: c.Title, Body: c.Body, Mood: mood, Urgent: c.Urgent}
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
