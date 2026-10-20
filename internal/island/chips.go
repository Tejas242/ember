package island

import (
	"fmt"
	"image/color"
	"math"
	"time"
)

// Letters in chips (a contest round's problems, a plugin's stages), and the
// small pieces the plugins' rows and dashboards share: progress rings, a
// living flame, a dashboard's measures.

// chip is one letter: its label, and its state as the protocol says it:
// done, tried, owed, fixed, gone, or todo.
type chip struct {
	label, state string
}

const maxLetters = 9

func chipW(label string, size float32) float32 {
	return max(size, measure(face(fSemiBold, float64(size)*0.62), label)+size*0.5)
}

// lettersW is the room a row of chips takes.
func lettersW(cs []chip, size float32) float32 {
	w := float32(0)
	for i, c := range cs {
		if i == maxLetters {
			break
		}
		if i > 0 {
			w += size * 0.28
		}
		w += chipW(c.label, size)
	}
	return w
}

// letters draws chips from x, centred on cy: done filled green, tried ringed
// red, owed ringed amber, fixed filled blue, gone struck through, the rest
// quiet. k fades them in.
func (u *UI) letters(cs []chip, x, cy, size float32, k float64) float32 {
	p := u.pal
	f := face(fSemiBold, float64(size)*0.62)
	x0 := x
	for i, c := range cs {
		if i == maxLetters {
			break
		}
		if i > 0 {
			x += size * 0.28
		}
		w := chipW(c.label, size)
		y, r := cy-size/2, size*0.3
		var fill, ring, txt color.NRGBA
		switch c.state {
		case "done":
			fill, txt = p.green, p.ink
		case "fixed":
			fill, txt = mix(p.blue, p.bg, 0.15), p.ink
		case "tried":
			ring, txt = p.red, p.red
		case "owed":
			ring, txt = p.yellow, p.yellow
		case "gone":
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
		u.dc.text(f, c.label, x+(w-measure(f, c.label))/2, cy+size*0.22, alpha(txt, k))
		if c.state == "gone" { // struck through: it's gone
			u.dc.roundRect(x+2, cy-0.5, w-4, 1, 0.5, alpha(p.faint, 0.8*k))
		}
		x += w
	}
	return x - x0
}

// left is a countdown in hours and minutes: "1:12", "0:09".
func left(now, end time.Time) string {
	d := max(0, end.Sub(now)).Round(time.Minute)
	return fmt.Sprintf("%d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// ringArc strokes an arc with round ends: progress rings.
func ringArc(c *canvas, cx, cy, r, w float32, a0, a1 float64, col color.NRGBA) {
	c.arc(cx, cy, r, w, float32(a0), float32(a1), col)
	for _, a := range []float64{a0, a1} {
		c.circle(cx+r*float32(math.Cos(a)), cy+r*float32(math.Sin(a)), w/2, col)
	}
}

// flame draws a small living flame centred at (cx, cy): two tongues that
// sway out of step. A dead one is a cold grey ember.
func flame(c *canvas, cx, cy, s float32, t float64, hot bool, p palette) {
	outer, inner := mix(p.yellow, p.red, 0.45), mix(p.yellow, colorWhite, 0.35)
	if !hot {
		outer, inner = alpha(p.fg, 0.25), alpha(p.fg, 0.12)
		t = 0
	}
	tongue := func(sc, sway float32, col color.NRGBA) {
		tipX := cx + sway*s
		var ps []pt
		const n = 14
		for i := 0; i <= n; i++ { // the bulb: the lower half circle
			a := float64(i) / n * math.Pi
			ps = append(ps, pt{cx + float32(math.Cos(a))*0.55*s*sc, cy + 0.3*s*sc + float32(math.Sin(a))*0.55*s*sc})
		}
		for i := 1; i < n; i++ { // up the left side to the tip, and down the right
			k := float32(i) / n
			ps = append(ps, pt{cx - 0.55*s*sc*float32(math.Pow(float64(1-k), 0.8)) + (tipX-cx)*k*k, cy + 0.3*s*sc - (1.75*s*sc)*k})
		}
		ps = append(ps, pt{tipX, cy + 0.3*s*sc - 1.75*s*sc})
		for i := n - 1; i >= 1; i-- {
			k := float32(i) / n
			ps = append(ps, pt{cx + 0.55*s*sc*float32(math.Pow(float64(1-k), 0.8)) + (tipX-cx)*k*k, cy + 0.3*s*sc - (1.75*s*sc)*k})
		}
		c.poly(col, ps...)
	}
	sway := float32(0.22*math.Sin(t*5.3) + 0.1*math.Sin(t*8.9))
	tongue(1+float32(0.05*math.Sin(t*7.1)), sway, outer)
	tongue(0.58, -sway*0.8, inner)
}

// ---- a dashboard's measures ----

const (
	dashW     = 560
	dashP     = 20                      // inner margin
	dashDiv   = 9                       // the space and hairline between sections
	dashEnter = 1700 * time.Millisecond // the whole entrance
)

// phase is an entrance's progress: 0 before delay, eased to 1 over dur.
func phase(age, delay, dur time.Duration) float64 {
	return easeOut(float64(age-delay) / float64(dur))
}

// dashDivider: a hairline between sections, faded in with the entrance.
func dashDivider(u *UI, x, w, y float32, k float64) {
	u.dc.roundRect(x+dashP, y+dashDiv/2, w-2*dashP, 1, 0.5, alpha(u.pal.hair(), k))
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}
