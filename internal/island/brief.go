package island

import (
	"fmt"
	"strings"
	"time"
)

// The open card is a brief, not a window. It answers, in order, the things
// you look up for: does anything need me, what is Claude doing, what
// finished while I wasn't looking.
//
// One session is the hero: the one waiting on you, else one working that you
// aren't looking at, else the one that just finished. Its headline is the
// task in words and one quiet line under it. Every other session is one
// line. Music and the plugins come last.

const (
	heroHeadH    = 52 // Ember, the headline, its facts
	outcomeFresh = 45 * time.Minute
	cardOthers   = 3 // sessions listed besides the hero; the rest fold into "N more"
)

type brief struct {
	mode      string // wait | work | done | idle | day
	s         *Session
	head, sub string
	mood      mood
	rows      []*Session // every session, in the list's order
}

// brief decides who the hero is and what the headline says.
func (u *UI) brief() brief {
	now := u.now()
	ss := u.sessionList()
	if !u.cfg.Claude {
		ss = nil
	}
	b := brief{rows: ss}
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
			if o := s.Outcome; o != nil && now.Sub(o.At) < outcomeFresh && (hero == nil || o.At.After(hero.Outcome.At)) {
				hero = s
			}
		}
	}
	if hero == nil {
		b.mode, b.mood, b.head, b.sub = "day", moodIdle, greeting(now), "No Claude Code session running"
		if !u.bond.metClaude() {
			b.sub = "Start a Claude Code session and I'll follow it here"
		}
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
		b.mode, b.mood = "work", moodWork
		b.head = s.task()
		if b.head == "" {
			b.head = u.name(s) + " is working"
		}
		b.sub = strings.TrimSuffix(s.Project+" · "+workFacts(s, now), " · ")
	case s.Outcome != nil:
		o := s.Outcome
		b.mode, b.mood = "done", moodHappy
		verb := "finished "
		if o.Failed {
			b.mood, verb = moodError, "stopped "
		}
		b.sub = s.Project + " · " + verb + ago(now.Sub(o.At))
		if o.Failed && o.Error != "" {
			b.sub += " · " + o.Error
		}
		b.head = s.task()
		if b.head == "" {
			b.head = u.name(s) + " is done"
			if o.Failed {
				b.head = u.name(s) + " stopped"
			}
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

// workFacts: "4m in · 2 agents", and "quiet 5m" when nothing has been heard.
func workFacts(s *Session, now time.Time) string {
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
	if n := s.running(); n > 0 {
		parts = append(parts, agentsText(n))
	}
	return strings.Join(parts, " · ")
}

func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return fmt.Sprintf("%d %ss", n, w)
}
