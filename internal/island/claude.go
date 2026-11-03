package island

import (
	"fmt"
	"image"
	"math"
	"time"
)

// Claude as a counter. The pill is Ember, the session you aren't watching,
// a bead per session and the agents as a number. Claude has no colour of its
// own: a bead is plain light and takes the state's colour (blue asks, green
// done, red stopped). Detail lives in the card, not here.

type beadKind int

const (
	beadIdle beadKind = iota // a hollow ring
	beadWork                 // light, pulsing on the beat
	beadAsk
	beadDone // finished, and you haven't looked yet
	beadFail
)

const (
	beadD     = 6 // a bead's size
	beadGap   = 4
	maxBeads  = 4
	growTime  = 5 * time.Second // an ask is two lines tall this long
	growH     = 20              // and the second line's room
	growChars = 52              // the question, cut at this many characters
	growMaxW  = 340
	doneHold  = 20 * time.Minute // a finish waits this long for you to look, then it lives in the card
)

// beadOf is the state a session's bead shows.
func beadOf(s *Session) beadKind {
	switch {
	case s.State == "wait":
		return beadAsk
	case s.status() == "work":
		return beadWork
	case !s.Unseen.IsZero() && s.Outcome != nil && s.Outcome.Failed:
		return beadFail
	case !s.Unseen.IsZero() && s.Outcome != nil:
		return beadDone
	}
	return beadIdle
}

func (u *UI) beads(ss []*Session) []beadKind {
	out := make([]beadKind, len(ss))
	for i, s := range ss {
		out[i] = beadOf(s)
	}
	return out
}

// agentTotal: the subagents working, over every session.
func agentTotal(ss []*Session) int {
	n := 0
	for _, s := range ss {
		n += s.running()
	}
	return n
}

func beadsW(bs []beadKind) float32 {
	n := min(len(bs), maxBeads)
	w := float32(n*beadD + (n-1)*beadGap)
	if len(bs) > maxBeads {
		w += beadGap + measure(fBadge(), fmt.Sprintf("+%d", len(bs)-maxBeads))
	}
	return w
}

// drawBeads draws the row with its left edge at x. A working bead pulses on
// the island's beat, each a fifth of a second behind the last.
func (u *UI) drawBeads(bs []beadKind, x, cy float32) {
	p := u.pal
	for i, k := range bs[:min(len(bs), maxBeads)] {
		bx := x + float32(i*(beadD+beadGap)) + beadD/2
		switch k {
		case beadWork:
			delay := float64(i) * 0.2
			r := image.Rect(int(bx-4), int(cy-4), int(bx+4), int(cy+4))
			u.lives = append(u.lives, live{r: r, draw: func(c *canvas, t float64) {
				a := 0.8 + 0.2*math.Cos(2*math.Pi*(t-delay)/beat) // 60..100%
				c.circle(bx, cy, beadD/2, alpha(p.fg, a))
			}})
		case beadAsk:
			u.dc.circle(bx, cy, beadD/2, p.blue)
		case beadDone:
			u.dc.circle(bx, cy, beadD/2, p.green)
		case beadFail:
			u.dc.circle(bx, cy, beadD/2, p.red)
		default:
			u.dc.arc(bx, cy, beadD/2-0.5, 1, 0, 2*math.Pi, alpha(p.fg, 0.3))
		}
	}
	if len(bs) > maxBeads {
		u.dc.text(fBadge(), fmt.Sprintf("+%d", len(bs)-maxBeads), x+float32(maxBeads*(beadD+beadGap)), cy+3.5, p.faint)
	}
}

// growText is the question to show on a second line, while an ask is new.
func (u *UI) growText(s *Session) string {
	if s.Asking == "" || u.cfg.Calm || u.now().Sub(s.AskAt) > growTime || s.AskAt.IsZero() {
		return ""
	}
	r := []rune(s.Asking)
	if len(r) > growChars {
		return string(r[:growChars-1]) + "…"
	}
	return s.Asking
}

// ---- where to go: the click on the pill, and `ember go` ----

// goTarget picks the session to take you to: the oldest ask, else the newest
// finish you haven't seen, else the newest one working.
func (u *UI) goTarget() *Session {
	var ask, done, work *Session
	for _, s := range u.sessions {
		switch {
		case s.State == "wait":
			if ask == nil || s.AskAt.Before(ask.AskAt) || (s.AskAt.Equal(ask.AskAt) && s.ID < ask.ID) {
				ask = s
			}
		case !s.Unseen.IsZero():
			if done == nil || s.Unseen.After(done.Unseen) || (s.Unseen.Equal(done.Unseen) && s.ID < done.ID) {
				done = s
			}
		case s.status() == "work":
			if work == nil || s.Since.After(work.Since) || (s.Since.Equal(work.Since) && s.ID < work.ID) {
				work = s
			}
		}
	}
	switch {
	case ask != nil:
		return ask
	case done != nil:
		return done
	}
	return work
}

// goTo is `ember go`: raise that terminal, or shake Ember's head if there is none.
func (a *app) goTo() {
	if s := a.ui.goTarget(); s != nil {
		a.focusSession(s.Pids)
		return
	}
	a.ui.play("shake", 0)
}

// drawGrow: the question on the pill's second line. It stays drawn while the
// island folds back, fading with the height, so the words don't vanish first.
func (u *UI) drawGrow(l line, x, w, h float32) {
	if l.grow != "" {
		u.growLast = l.grow
	}
	k := float32(math.Min(1, (u.sh.x-float64(h))/growH))
	if u.growLast == "" || k <= 0.02 {
		if l.grow == "" {
			u.growLast = ""
		}
		return
	}
	tx := x + pillEnd + leadW + leadGap
	t := ellipsize(fPillSub(), u.growLast, x+w-pillEnd-tx)
	u.dc.text(fPillSub(), t, tx, h+12, alpha(u.pal.muted, float64(k)))
}

// pillTarget: what a click on the pill takes you to, if the pill is about
// something that has a terminal: a question, or a finish you haven't seen.
func (u *UI) pillTarget() *Session {
	ss := u.sessionList()
	if len(ss) > 0 && ss[0].State == "wait" {
		return u.goTarget()
	}
	if u.moment == nil && u.primary() == "clock" {
		if s := u.goTarget(); s != nil && !s.Unseen.IsZero() {
			return s
		}
	}
	return nil
}
