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
// becomes the hero. Music and the plugins come last. With no session at all, the
// hero is the day, hour by hour.

const (
	heroHeadH  = 52 // Ember, the headline, its facts
	stepH      = 22 // a step on the rail
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
		what := s.Asking
		if what == "" {
			what = s.task()
		}
		if what == "" {
			what = "Asking in the terminal"
		}
		b.sub = what + " · waiting " + mins(now.Sub(s.Seen))
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
		verb := "finished "
		switch {
		case r.Failed:
			b.mood, verb = moodError, "stopped "
		case r.troubled():
			b.mood = moodNudge
		}
		// one line: where, when, how long, what it changed
		parts := []string{s.Project, verb + ago(now.Sub(r.At))}
		if r.Took >= time.Minute {
			parts = append(parts, "took "+mins(r.Took))
		}
		if r.Files > 0 {
			f := plural(r.Files, "file")
			if r.Plus > 0 || r.Minus > 0 {
				f += " " + diffText(r.Plus, r.Minus)
			}
			parts = append(parts, f)
		}
		b.sub = strings.Join(parts, " · ")
		b.head = s.task()
		if b.head == "" {
			b.head = u.name(s) + " is done"
			if r.Failed {
				b.head = u.name(s) + " stopped"
			}
			rest := strings.TrimPrefix(b.sub, s.Project+" · ")
			b.sub = strings.ToUpper(rest[:1]) + rest[1:]
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

// troubled: the turn ended, and Claude's own words admit a failure (a
// step that failed and was then fixed is just how agents work).
func (r *Recap) troubled() bool {
	return r.Failed || (r.Failures > 0 && strings.Contains(strings.ToLower(r.Summary), "fail"))
}

// turnFacts: "4m in · 3 files +58 −12 · 1 failed".
func turnFacts(s *Session, now time.Time) string {
	t := s.Turn
	var parts []string
	switch {
	case s.Since.IsZero():
	case now.Sub(s.Since) < time.Minute:
		parts = append(parts, "just started")
	default:
		parts = append(parts, mins(now.Sub(s.Since))+" in")
	}
	if stalled(s, now) {
		parts = append(parts, "quiet "+mins(now.Sub(s.Seen)))
	}
	if n := len(t.Files); n > 0 {
		f := plural(n, "file")
		if t.Plus > 0 || t.Minus > 0 {
			f += " " + diffText(t.Plus, t.Minus)
		}
		parts = append(parts, f)
	}
	if t.Failures > 0 { // a failure that was retried is how agents work: only a live one is red
		if n := len(t.Steps); n > 0 && t.Steps[n-1].Failed {
			parts = append(parts, "last step failed")
		} else if t.Failures == 1 {
			parts = append(parts, "1 retry")
		} else {
			parts = append(parts, fmt.Sprintf("%d retries", t.Failures))
		}
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
	if (d == nil || d.Tasks == 0) && u.cfg.Claude && !u.bond.metClaude() {
		return "Start a Claude Code session and I'll follow it here"
	}
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
