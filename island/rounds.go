package island

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"
)

// Rounds: edge's live contests (Codeforces, AtCoder, LeetCode). edge sends
// the round that matters now inside its status: one it holds you to, from
// two hours before; the one you're in while it runs, with a letter per
// problem filled in as verdicts land; and the result for half an hour
// after. The pill counts down, the card row opens the round or starts the
// upsolve, and the dashboard keeps the record.

// EdgeRound is the round in edge's status.
type EdgeRound struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Short    string        `json:"short"` // "CF 1128", "ABC 480", "Weekly 524"
	URL      string        `json:"url"`
	StartMs  int64         `json:"startMs"`
	EndMs    int64         `json:"endMs"`
	Plan     string        `json:"plan"`   // in | out | "" (open)
	Joined   string        `json:"joined"` // live | virtual | ""
	Locked   bool          `json:"locked"` // Claude is off for it
	Settled  bool          `json:"settled"`
	Problems []EdgeProblem `json:"problems"`
}

// EdgeProblem is one letter of a round.
type EdgeProblem struct {
	Index string `json:"index"`
	State string `json:"state"` // "" | tried | solved | upsolve | upsolved | taught | dropped | lapsed
	AtSec int    `json:"at_sec"`
	Fails int    `json:"fails"`
}

func (r *EdgeRound) start() time.Time { return time.UnixMilli(r.StartMs) }
func (r *EdgeRound) end() time.Time   { return time.UnixMilli(r.EndMs) }

// phase: soon | live | over.
func (r *EdgeRound) phase(now time.Time) string {
	switch {
	case now.Before(r.start()):
		return "soon"
	case now.Before(r.end()):
		return "live"
	}
	return "over"
}

// upsolves is the letters still owed, "C D".
func upsolves(ps []EdgeProblem) string {
	var ls []string
	for _, p := range ps {
		if p.State == "upsolve" {
			ls = append(ls, p.Index)
		}
	}
	return strings.Join(ls, " ")
}

func solvedLetters(ps []EdgeProblem) string {
	var ls []string
	for _, p := range ps {
		if p.State == "solved" {
			ls = append(ls, p.Index)
		}
	}
	return strings.Join(ls, " ")
}

// until says how far off t is: "in 42m", "in 3h 05m", "Sat 20:05".
func until(now, t time.Time) string {
	d := t.Sub(now)
	switch {
	case d < time.Minute:
		return "now"
	case d < 12*time.Hour:
		return "in " + mins(d+time.Minute-1)
	}
	return t.Format("Mon 15:04")
}

// left is a live round's time left: "1:12", "0:09".
func left(now, end time.Time) string {
	d := max(0, end.Sub(now)).Round(time.Minute)
	return fmt.Sprintf("%d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// ---- letter chips ----

const maxLetters = 9

func chipW(idx string, size float32) float32 {
	return max(size, measure(face(fSemiBold, float64(size)*0.62), idx)+size*0.5)
}

// lettersW is the room a row of chips takes.
func lettersW(ps []EdgeProblem, size float32) float32 {
	w := float32(0)
	for i, p := range ps {
		if i == maxLetters {
			break
		}
		if i > 0 {
			w += size * 0.28
		}
		w += chipW(p.Index, size)
	}
	return w
}

// letters draws a round's problems as chips from x, centred on cy: solved
// filled green, tried ringed red, owed ringed amber, upsolved filled blue,
// the rest quiet. k fades them in.
func (u *UI) letters(ps []EdgeProblem, x, cy, size float32, k float64) float32 {
	p := u.pal
	f := face(fSemiBold, float64(size)*0.62)
	x0 := x
	for i, pr := range ps {
		if i == maxLetters {
			break
		}
		if i > 0 {
			x += size * 0.28
		}
		w := chipW(pr.Index, size)
		y, r := cy-size/2, size*0.3
		var fill, ring, txt color.NRGBA
		switch pr.State {
		case "solved":
			fill, txt = p.green, p.ink
		case "upsolved", "taught":
			fill, txt = mix(p.blue, p.bg, 0.15), p.ink
		case "tried":
			ring, txt = p.red, p.red
		case "upsolve":
			ring, txt = p.yellow, p.yellow
		case "lapsed", "dropped":
			fill, txt = alpha(p.fg, 0.05), alpha(p.faint, 0.7)
		default:
			fill, txt = alpha(p.fg, 0.09), p.muted
		}
		if fill.A > 0 {
			u.dc.roundRect(x, y, w, size, r, alpha(fill, k*float64(fill.A)/255))
		}
		if ring.A > 0 {
			u.dc.frame(x+0.6, y+0.6, w-1.2, size-1.2, r, 1.2, alpha(ring, k))
		}
		u.dc.text(f, pr.Index, x+(w-measure(f, pr.Index))/2, cy+size*0.22, alpha(txt, k))
		if pr.State == "lapsed" || pr.State == "dropped" { // struck through: it's gone
			u.dc.roundRect(x+2, cy-0.5, w-4, 1, 0.5, alpha(p.faint, 0.8*k))
		}
		x += w
	}
	return x - x0
}

// ---- the pill ----

// roundLine is the pill for edge's round.
func (u *UI) roundLine(r *EdgeRound) line {
	p, now := u.pal, u.now()
	l := line{key: "edge:round:" + r.phase(now), text: r.Short, textCol: p.fg}
	switch r.phase(now) {
	case "soon":
		l.mood, l.sub = moodNudge, until(now, r.start())
		if r.start().Sub(now) <= 15*time.Minute {
			l.subCol = p.yellow
		}
	case "live":
		l.mood, l.lock, l.letters = moodWork, r.Locked, r.Problems
		l.timer, l.timerCol = left(now, r.end()), p.fg
		if r.end().Sub(now) <= 10*time.Minute {
			l.timerCol = p.yellow
		}
	default:
		l.mood, l.letters = moodHappy, r.Problems
		if r.Joined == "" {
			l.mood = moodNudge
		}
		if up := upsolves(r.Problems); up != "" {
			l.sub = "upsolve " + strings.Fields(up)[0]
		}
	}
	return l
}

// roundSalience: a live round you're in is the most important thing on the
// island; one about to start nearly so.
func roundSalience(r *EdgeRound, now time.Time) float64 {
	switch r.phase(now) {
	case "live":
		if r.Locked || r.Joined != "" {
			return 0.95
		}
		return 0.5
	case "soon":
		if d := r.start().Sub(now); d <= 15*time.Minute {
			return 0.9
		} else if d <= 2*time.Hour {
			return 0.5
		}
		return 0.3
	}
	return 0.6
}

// ---- the card row ----

// roundRow is edge's row while a round matters: the round, where it
// stands, and the one thing to do about it.
func (u *UI) roundRow(r *EdgeRound, x, w, y float32) {
	p, now := u.pal, u.now()
	cy := y + edgeH/2
	base := cy + 4.5
	gx := x + railX
	// on the rail: the round's clock, a ring that empties as it runs
	ph := r.phase(now)
	ring := p.yellow
	frac := 1.0
	switch ph {
	case "live":
		frac = math.Max(0, float64(r.end().Sub(now))/float64(r.end().Sub(r.start())))
	case "over":
		ring = p.green
		if r.Joined == "" {
			ring = p.muted
		}
	}
	u.dc.arc(gx, cy, 6, 1.8, 0, 2*math.Pi, alpha(p.fg, 0.12))
	if frac > 0.01 {
		ringArc(u.dc, gx, cy, 6, 1.8, -math.Pi/2, -math.Pi/2+2*math.Pi*frac, ring)
	}
	tx, right := x+textX, x+w-pad
	var btns []Button
	var sub string
	switch ph {
	case "soon":
		sub = until(now, r.start())
		if r.Plan == "in" {
			sub += " · you're in"
		}
		if r.start().Sub(now) <= 30*time.Minute {
			btns = []Button{{Key: "skipround:" + r.ID, Label: "Can't make it"}, {Key: "openround:" + r.ID, Label: "Open", Primary: true}}
		}
	case "live":
		sub = left(now, r.end()) + " left"
		if r.Joined == "" {
			btns = []Button{{Key: "skipround:" + r.ID, Label: "Can't make it"}, {Key: "openround:" + r.ID, Label: "Open", Primary: true}}
		}
	default:
		switch s := solvedLetters(r.Problems); {
		case r.Joined == "":
			sub = "missed"
		case s == "":
			sub = "nothing solved"
		default:
			sub = "solved " + s
		}
		if up := upsolves(r.Problems); up != "" {
			sub += " · upsolve " + up
			btns = []Button{{Key: "upsolve", Label: "Upsolve", Primary: true}}
		}
	}
	for i := len(btns) - 1; i >= 0; i-- {
		bw := u.pillButton(btns[i], "btn:edge:"+btns[i].Key, right, cy-btnH/2, btnH, p.yellow)
		right -= bw + 8
	}
	if len(btns) > 0 {
		right -= 2
	}
	nw := u.dc.text(fBody(), r.Short, tx, base, p.fg)
	sx := tx + nw + 8
	if ph == "live" && len(r.Problems) > 0 { // the letters, as the pill has them
		lw := lettersW(r.Problems, 14)
		if sx+lw+60 < right {
			sx += u.letters(r.Problems, sx, cy, 14, 1) + 8
		}
	}
	if r.Locked && ph == "live" {
		u.dc.lock(sx, cy-5.5, 10, p.yellow)
		sx += 15
	}
	u.dc.text(fSecond(), ellipsize(fSecond(), sub, right-sx), sx, base, p.muted)
}
