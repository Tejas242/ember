package island

import (
	"fmt"
	"strings"
	"time"
)

// The open card is a brief, not a window. It answers, in order, the three
// things you look up for: does anything need me; what is Claude doing, and
// is it going well; what happened while I wasn't looking.
//
// A headline says it in a sentence. Under it, every Claude session is a
// row: a glyph for its state (an orbit while it works, with a satellite per
// subagent; a pulse while it waits on you; a check when it's done), its
// name, what it's doing, its last ten minutes. A row opens into its short
// transcript: what you asked, its latest steps, its subagents on a branch
// (each opens into its own steps), and a way to its terminal. The session
// that matters most opens by itself; what you open or close stays so. When
// it doesn't all fit, steps go first, then rows close, then the last rows
// fold into "N more". Music and edge come last. With no session at all, the
// day, hour by hour.

const (
	briefHeadH = 52 // Ember, the headline, its facts
	stepH      = 20 // a step in a session's transcript
	keepShown  = 4  // steps shown, at most ([card] steps)
	recapFresh = 45 * time.Minute
)

type brief struct {
	mode      string // wait | work | recap | day
	s         *Session
	head, sub string
	mood      mood
	rows      []*Session // every session, in the list's order
	keep      int        // steps at most: [card] steps
}

// brief decides what the headline says.
func (u *UI) brief() brief {
	now := u.now()
	ss := u.sessionList()
	if !u.cfg.Claude {
		ss = nil
	}
	b := brief{rows: ss, keep: u.cfg.CardSteps}
	var waiting, working []*Session
	for _, s := range ss {
		switch {
		case s.State == "wait":
			waiting = append(waiting, s)
		case s.status() == "work":
			working = append(working, s)
		}
	}
	also := ""
	if len(working) > 0 {
		also = fmt.Sprintf(" · %d working", len(working))
	}
	switch {
	case len(waiting) > 1:
		b.mode, b.s, b.mood = "wait", waiting[0], moodWait
		b.head = fmt.Sprintf("%d sessions need you", len(waiting))
		var names []string
		for _, s := range waiting {
			names = append(names, u.name(s))
		}
		b.sub = strings.Join(names, ", ") + also
	case len(waiting) == 1:
		s := waiting[0]
		b.mode, b.s, b.mood = "wait", s, moodWait
		b.head = u.name(s) + " needs you"
		b.sub = "It's asking in the terminal · " + ago(now.Sub(s.Seen))
		if t := s.task(); t != "" {
			b.sub = t + " · " + ago(now.Sub(s.Seen))
		}
		b.sub += also
	case len(working) == 1:
		s := working[0]
		label := s.Label
		if label == "" {
			label = "Thinking"
		}
		if s.State == "idle" { // its turn ended; its agents work on
			label = agentsText(s.running()) + " working"
		}
		// the headline is the task, in words; the rows below say how it's going
		head, sub := s.task(), s.Project+" · "+turnFacts(s, now)
		if head == "" {
			head, sub = u.name(s)+" · "+label, turnFacts(s, now)
			if g, ok := gerund(label); ok {
				head = u.name(s) + " is " + g
			}
		}
		b.mode, b.s, b.mood, b.head, b.sub = "work", s, moodWork, head, strings.TrimSuffix(sub, " · ")
	case len(working) > 1:
		b.mode, b.s, b.mood = "work", working[0], moodWork
		b.head = fmt.Sprintf("Clauding in %d sessions", len(working))
		agents, steps := 0, 0
		for _, s := range working {
			agents += s.running()
			steps += s.Turn.Count
		}
		parts := []string{plural(steps, "step")}
		if agents > 0 {
			parts = append(parts, agentsText(agents))
		}
		b.sub = strings.Join(parts, " · ") + " so far"
	default:
		var last *Session
		for _, s := range ss {
			if s.Recap != nil && now.Sub(s.Recap.At) < recapFresh && (last == nil || s.Recap.At.After(last.Recap.At)) {
				last = s
			}
		}
		if last == nil {
			b.mode, b.mood, b.head, b.sub = "day", moodIdle, greeting(now), u.dayFacts()
			break
		}
		r := last.Recap
		b.mode, b.s, b.mood = "recap", last, moodHappy
		if r.Failures > 0 && strings.Contains(strings.ToLower(r.Summary), "fail") {
			b.mood = moodNudge
		}
		b.sub = last.Project + " · done " + ago(now.Sub(r.At))
		if r.Took > 0 {
			b.sub += " · took " + mins(r.Took)
		}
		b.head = last.task()
		if b.head == "" {
			b.head, b.sub = u.name(last)+" is done", "D"+strings.TrimPrefix(b.sub, last.Project+" · d")
		}
	}
	return b
}

// turnFacts: "4m in · 9 steps · 3 files +58 −12 · 1 failed".
func turnFacts(s *Session, now time.Time) string {
	t := s.Turn
	var parts []string
	if !s.Since.IsZero() {
		parts = append(parts, mins(now.Sub(s.Since))+" in")
	}
	if t.Count > 0 {
		parts = append(parts, plural(t.Count, "step"))
	}
	if n := len(t.Files); n > 0 {
		f := plural(n, "file")
		if t.Plus > 0 || t.Minus > 0 {
			f += " " + diffText(t.Plus, t.Minus)
		}
		parts = append(parts, f)
	}
	if t.Failures > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", t.Failures))
	}
	return strings.Join(parts, " · ")
}

func recapFacts(r *Recap) string {
	var parts []string
	if r.Steps > 0 {
		parts = append(parts, plural(r.Steps, "step"))
	}
	if r.Files > 0 {
		f := plural(r.Files, "file") + " changed"
		if r.Plus > 0 || r.Minus > 0 {
			f += " " + diffText(r.Plus, r.Minus)
		}
		parts = append(parts, f)
	} else if r.Steps > 0 {
		parts = append(parts, "no files changed")
	}
	if r.Failures > 0 {
		parts = append(parts, plural(r.Failures, "failed step"))
	}
	return strings.Join(parts, " · ")
}

func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return fmt.Sprintf("%d %ss", n, w)
}

// dayFacts: "6 tasks done · 2h 14m with Claude · mostly ember".
func (u *UI) dayFacts() string {
	d := u.day
	if d == nil || d.Tasks == 0 {
		return "Nothing finished with Claude yet today"
	}
	parts := []string{plural(d.Tasks, "task") + " done"}
	if d.Busy >= time.Minute {
		parts = append(parts, mins(d.Busy)+" with Claude")
	}
	top, n := "", 0
	for p, c := range d.Projects {
		if c > n || (c == n && p < top) {
			top, n = p, c
		}
	}
	if len(d.Projects) > 1 {
		parts = append(parts, "mostly "+top)
	} else if top != "" {
		parts = append(parts, "in "+top)
	}
	return strings.Join(parts, " · ")
}
