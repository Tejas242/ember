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
// One session is the hero: the one waiting on you, else one working that you
// aren't looking at, else the one that just finished. Its headline is the
// task in words, and its timeline hangs from Ember on a thin rail (its latest
// steps, its subagents on a branch), or, once it's done, Claude's most
// informative sentence. Every other session is one line; click one and it
// becomes the hero. Music and edge come last. With no session at all, the
// hero is the day, hour by hour.

const (
	heroHeadH  = 52 // Ember, the headline, its facts
	stepH      = 22 // a step on the rail
	keepShown  = 4  // steps shown, at most ([card] steps)
	recapFresh = 45 * time.Minute
)

type brief struct {
	mode      string // wait | work | recap | idle | day
	s         *Session
	head, sub string
	mood      mood
	rows      []*Session // every session, in the list's order
	keep      int        // steps at most: [card] steps
}

// brief decides who the hero is and what the headline says.
func (u *UI) brief() brief {
	now := u.now()
	ss := u.sessionList()
	if !u.cfg.Claude {
		ss = nil
	}
	b := brief{rows: ss, keep: u.cfg.CardSteps}
	var hero *Session
	for _, s := range ss { // one you picked in the card stays the hero while it's open
		if s.ID == u.hero {
			hero = s
		}
	}
	if hero == nil {
		for _, s := range ss {
			if s.State == "wait" || s.status() == "work" { // the list ranks these first
				hero = s
				break
			}
		}
	}
	if hero == nil {
		for _, s := range ss {
			if s.Recap != nil && now.Sub(s.Recap.At) < recapFresh && (hero == nil || s.Recap.At.After(hero.Recap.At)) {
				hero = s
			}
		}
	}
	if hero == nil {
		b.mode, b.mood, b.head, b.sub = "day", moodIdle, greeting(now), u.dayFacts()
		return b
	}
	b.s = hero
	s := hero
	switch {
	case s.State == "wait":
		b.mode, b.mood = "wait", moodWait
		b.head = u.name(s) + " needs you"
		b.sub = "Asking in the terminal · " + ago(now.Sub(s.Seen))
		if t := s.task(); t != "" {
			b.sub = t + " · " + ago(now.Sub(s.Seen))
		}
	case s.status() == "work":
		label := s.Label
		if label == "" {
			label = "Thinking"
		}
		if s.State == "idle" { // its turn ended; its agents work on
			label = agentsText(s.running()) + " working"
		}
		// the headline is the task, in words; the timeline says how it's going
		head, sub := s.task(), s.Project+" · "+turnFacts(s, now)
		if head == "" {
			head, sub = u.name(s)+" · "+label, turnFacts(s, now)
			if g, ok := gerund(label); ok {
				head = u.name(s) + " is " + g
			}
		}
		b.mode, b.mood, b.head, b.sub = "work", moodWork, head, strings.TrimSuffix(sub, " · ")
	case s.Recap != nil:
		r := s.Recap
		b.mode, b.mood = "recap", moodHappy
		if r.Failures > 0 && strings.Contains(strings.ToLower(r.Summary), "fail") {
			b.mood = moodNudge
		}
		b.sub = s.Project + " · done " + ago(now.Sub(r.At))
		if r.Took > 0 {
			b.sub += " · took " + mins(r.Took)
		}
		b.head = s.task()
		if b.head == "" {
			b.head, b.sub = u.name(s)+" is done", "D"+strings.TrimPrefix(b.sub, s.Project+" · d")
		}
	default:
		b.mode, b.mood = "idle", moodIdle
		b.head = s.task()
		if b.head == "" {
			b.head = u.name(s)
		}
		b.sub = s.Project + " · idle"
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
