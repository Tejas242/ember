package island

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"
)

// The design system: spacing, type, motion. Every interactive thing eases
// (hover, press, the tab indicator), numbers roll instead of jumping, and
// corners are continuous (a superellipse), not circular arcs.

const (
	pad     = 16  // the card's inner margin
	railX   = 26  // the rail and Ember's centre, from the card's left edge
	textX   = 48  // where text starts, from the card's left edge
	rowGap  = 6   // between rows
	radCard = 22  // the card's bottom corners
	radRow  = 10  // hovers, code, options
	cornerN = 2.7 // superellipse exponent: 2 is a circle; Apple's corners are ~2.5–3
)

// Type: four sizes, three weights (see docs/DESIGN.md).
var (
	fTitle   = func() fontFace { return face(fSemiBold, 14) }
	fBody    = func() fontFace { return face(fMedium, 12.5) }
	fSecond  = func() fontFace { return face(fRegular, 12) }
	fCaption = func() fontFace { return face(fRegular, 11) }
	fMonoS   = func() fontFace { return face(fMono, 11.5) }
	fEyebrow = func() fontFace { return face(fSemiBold, 9.5) }
)

// secondary is text one step quieter than fg: steps, artists, summaries.
func (p palette) secondary() color.NRGBA { return p.muted }

// hair is a divider.
func (p palette) hair() color.NRGBA { return alpha(p.fg, 0.07) }

// cornerInset is how far a continuous corner of radius r sits in from the
// straight edge, d px into the corner (0 at its start, r at the very edge).
func cornerInset(r, d float32, n float64) float32 {
	if d <= 0 || r <= 0 {
		return 0
	}
	k := math.Min(1, float64(d/r))
	if n == 2 {
		return r * float32(1-math.Sqrt(1-k*k))
	}
	if n == cornerN { // the table, interpolated
		f := k * cornerSteps
		i := int(f)
		if i >= cornerSteps {
			return r * cornerLUT[cornerSteps]
		}
		t := float32(f - float64(i))
		return r * (cornerLUT[i]*(1-t) + cornerLUT[i+1]*t)
	}
	return r * float32(1-math.Pow(1-math.Pow(k, n), 1/n))
}

const cornerSteps = 512

// cornerLUT is the continuous corner's profile, computed once.
var cornerLUT = func() (t [cornerSteps + 1]float32) {
	for i := range t {
		k := float64(i) / cornerSteps
		t[i] = float32(1 - math.Pow(1-math.Pow(k, cornerN), 1/cornerN))
	}
	return
}()

// ---- tweens: small springs keyed by name, stepped once per frame ----

type tween struct {
	x, v  float64
	frame int
}

// tw eases the value under key toward target and returns it. A new key
// starts at from. Keys nobody asks for fade out of the table.
func (u *UI) tw(key string, target, from float64) float64 { return u.twK(key, target, from, 520) }

// twK is tw with its own stiffness k (critically damped): 2500 is ~50 ms,
// 1600 ~95 ms, 520 ~250 ms to settle.
func (u *UI) twK(key string, target, from, k float64) float64 {
	t := u.tweens[key]
	if t == nil {
		t = &tween{x: from}
		u.tweens[key] = t
	}
	if t.frame != u.frame {
		t.frame = u.frame
		dt := u.dt
		for dt > 0 { // critically damped, stepped finely for stability
			h := math.Min(dt, 1.0/120)
			t.v += ((target-t.x)*k - t.v*2*math.Sqrt(k)) * h
			t.x += t.v * h
			dt -= h
		}
		if math.Abs(target-t.x) < 0.002 && math.Abs(t.v) < 0.02 {
			t.x, t.v = target, 0
		}
	}
	if t.x != target {
		u.tweening = true
	}
	return t.x
}

// hl is the eased highlight (0..1) of a hoverable control.
func (u *UI) hl(key string) float64 {
	on, k := 0.0, 520.0 // it leaves slowly
	if u.hoverKey == key || u.drag == key {
		on, k = 1, 1600 // and arrives quickly
	}
	return u.twK("hl:"+key, on, 0, k)
}

// pressed is how far key is pushed in, 0..1: in at once while the button is
// held, out gently after. A click from elsewhere (a command) flashes it.
func (u *UI) pressed(key string) float64 {
	held, k := 0.0, 520.0
	if u.downKey == key {
		held, k = 1, 2500
	}
	d := u.twK("pr:"+key, held, 0, k)
	if u.pressKey == key && u.downKey == "" {
		if t := u.now().Sub(u.pressAt).Seconds(); t < 0.22 {
			u.tweening = true
			d = math.Max(d, math.Sin(t/0.22*math.Pi))
		}
	}
	return d
}

func (u *UI) gcTweens() {
	for k, t := range u.tweens {
		if u.frame-t.frame > 240 {
			delete(u.tweens, k)
		}
	}
}

// ---- numbers that roll ----

type roll struct {
	old, cur string
	at       time.Time
}

const rollTime = 260 * time.Millisecond

// tabWidth is the width rollText gives s. (Geist's digits are close to
// even; at 12 px a strict tabular grid looks gappier than it is steady.)
func tabWidth(f fontFace, s string) float32 { return measure(f, s) }

// rollText draws s right-aligned at rx. When it changes, each changed
// character rolls: the old one up and out, the new one in from below.
func (u *UI) rollText(c *canvas, key, s string, f fontFace, rx, base float32, col color.NRGBA) float32 {
	r := u.rolls[key]
	if r == nil {
		r = &roll{cur: s}
		u.rolls[key] = r
	}
	if s != r.cur {
		r.old, r.cur, r.at = r.cur, s, u.now()
	}
	k := float64(u.now().Sub(r.at)) / float64(rollTime)
	if k < 1 {
		u.tweening = true
	}
	e := float32(easeOut(k))
	w := tabWidth(f, s)
	if k < 1 { // the digits roll inside their own line
		met := f.Metrics()
		asc, desc := met.Ascent.Ceil(), met.Descent.Ceil()
		c.clipR = image.Rect(int(rx-w)-8, int(base)-asc-1, int(rx)+2, int(base)+desc+1)
		defer func() { c.clipR = image.Rectangle{} }()
	}
	travel := float32(f.Metrics().Ascent.Ceil()) * 0.8
	if k >= 1 { // at rest: the whole string, kerned, exactly as wide as it measures
		c.text(f, s, rx-w, base, col)
		return w
	}
	// align old and new from the right, character by character
	newR, oldR := []rune(s), []rune(r.old)
	x := rx
	for i := 1; i <= max(len(newR), len(oldR)); i++ {
		var n, o rune
		if i <= len(newR) {
			n = newR[len(newR)-i]
		}
		if i <= len(oldR) {
			o = oldR[len(oldR)-i]
		}
		cw := float32(0)
		if n != 0 {
			cw = measure(f, string(n))
		} else {
			cw = measure(f, string(o))
		}
		x -= cw
		if k >= 1 || n == o {
			if n != 0 {
				c.text(f, string(n), x+(cw-measure(f, string(n)))/2, base, col)
			}
			continue
		}
		if o != 0 {
			c.text(f, string(o), x+(cw-measure(f, string(o)))/2, base-travel*e, alpha(col, 1-float64(e)))
		}
		if n != 0 {
			c.text(f, string(n), x+(cw-measure(f, string(n)))/2, base+travel*(1-e), alpha(col, float64(e)))
		}
	}
	return w
}

// ---- the shimmer on a label while Claude thinks ----

const (
	shimmerEvery = 2.4 // seconds between passes
	shimmerPass  = 1.0 // a pass takes this long
)

func easeInOut(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	if x < 0.5 {
		return 4 * x * x * x
	}
	return 1 - math.Pow(-2*x+2, 3)/2
}

// shimmering: a shimmer's band is crossing right now (frames should come quickly).
func shimmerPassing(t float64) bool { return math.Mod(t, shimmerEvery) < shimmerPass }

// shimmerLive sweeps a soft band of light across text already drawn at
// (x, base): the label looks lit from inside while work is going on.
func (u *UI) shimmerLive(f fontFace, s string, x, base float32, col color.NRGBA) {
	if u.cfg.Calm {
		return
	}
	m := u.cv.mask(f, s)
	px, py := int(math.Round(float64(x)))-1, int(math.Round(float64(base)))-m.asc
	r := image.Rect(px, py, px+m.m.Rect.Dx(), py+m.m.Rect.Dy())
	w := float64(r.Dx())
	u.lives = append(u.lives, live{r: r.Inset(-1), draw: func(c *canvas, t float64) {
		ph := math.Mod(t, shimmerEvery) // every shimmer passes at once, then rests
		if ph >= shimmerPass {
			return
		}
		center := -40 + easeInOut(ph/shimmerPass)*(w+80)
		x0, x1 := max(0, int(center-42)), min(m.m.Rect.Dx(), int(center+42))
		fk := 1.0
		if c.k > 0 {
			fk = c.k
		}
		for yy := 0; yy < m.m.Rect.Dy(); yy++ {
			row := c.img.Pix[(py+yy)*c.img.Stride:]
			for xx := x0; xx < x1; xx++ {
				a := float64(m.m.Pix[yy*m.m.Stride+xx])
				if a == 0 {
					continue
				}
				d := math.Abs(float64(xx)-center) / 14
				if d >= 3 {
					continue
				}
				k := band[int(d*85)] * a / 255 * fk
				if k < 0.01 {
					continue
				}
				i := (px + xx) * 4
				if px+xx < 0 || i+3 >= len(row) {
					continue
				}
				for ch, v := range [3]uint8{col.R, col.G, col.B} {
					row[i+ch] = uint8(float64(row[i+ch])*(1-k) + float64(v)*k)
				}
			}
		}
	}})
}

// ---- syntax tint for commands ----

// shellTokens splits a command line into coloured runs: the program, flags,
// strings, the rest.
func shellTokens(p palette, ln string, first bool) []struct {
	s   string
	col color.NRGBA
} {
	type run = struct {
		s   string
		col color.NRGBA
	}
	var out []run
	i := 0
	for i < len(ln) {
		j := i
		switch ln[i] {
		case ' ', '\t':
			for j < len(ln) && (ln[j] == ' ' || ln[j] == '\t') {
				j++
			}
			out = append(out, run{ln[i:j], p.fg})
			i = j
			continue
		case '"', '\'':
			q := ln[i]
			j++
			for j < len(ln) && ln[j] != q {
				j++
			}
			j = min(j+1, len(ln))
			out = append(out, run{ln[i:j], mix(p.green, p.fg, 0.25)})
		case '|', '&', ';', '>', '<':
			for j < len(ln) && (ln[j] == '|' || ln[j] == '&' || ln[j] == ';' || ln[j] == '>' || ln[j] == '<') {
				j++
			}
			out = append(out, run{ln[i:j], p.yellow})
			first = true
		default:
			for j < len(ln) && ln[j] != ' ' && ln[j] != '\t' && ln[j] != '"' && ln[j] != '\'' && ln[j] != '|' && ln[j] != ';' {
				j++
			}
			tok := ln[i:j]
			col := p.fg
			switch {
			case first:
				col, first = mix(p.blue, p.fg, 0.15), false
			case tok[0] == '-':
				col = p.muted
			}
			out = append(out, run{tok, col})
		}
		i = j
	}
	return out
}

// panel draws a large translucent background (a row, a box) straight onto
// the island, clipped to its shape and faded with the content, instead of
// into the content layer: compositing then has only sparse text and icons
// to blend, which is most of what made full frames cost.
func (u *UI) panel(x, y, w, h, r float32, col color.NRGBA) {
	col.A = uint8(float64(col.A) * u.fadeK * u.rowK)
	u.cv.clip = &u.shape
	u.cv.roundRect(x, y, w, h, r, col)
	u.cv.clip = nil
}

// band is the shimmer's light across its width: 0.75·exp(−d²) for d in [0,3).
var band = func() (t [256]float64) {
	for i := range t {
		d := float64(i) / 85
		t[i] = 0.75 * math.Exp(-d*d)
	}
	return
}()

// ---- entrances: rows arrive one after another ----

const (
	enterTime  = 200 * time.Millisecond
	enterStep  = 24 * time.Millisecond // between one row and the next
	enterRise  = 4                     // px a row rises as it fades in
	enterMax   = 6                     // rows after this many arrive together
	glideStiff = 520
)

// enter is how far the i-th element of the current view has arrived, 0..1.
// Elements a view gains later (a new session while the card is open) arrive
// on their own when they first appear.
func (u *UI) enter(key string, i int, known bool) float64 {
	at, ok := u.enterAt[key]
	if !ok {
		at = u.now()
		base := u.trAt
		if u.staggerAt.After(base) {
			base = u.staggerAt
		}
		if d := base.Add(time.Duration(min(i, enterMax)) * enterStep); u.now().Sub(base) < fadeIn {
			at = d // part of an entrance: staggered
		}
		if at.Equal(u.now()) && strings.HasPrefix(key, "st:") { // a new step waits for the others to make room
			at = at.Add(110 * time.Millisecond)
		}
		if known { // it was on screen a moment ago in another role: it's already half there
			at = at.Add(-enterTime * 3 / 10)
		}
		u.enterAt[key] = at
	}
	k := float64(u.now().Sub(at)) / float64(enterTime)
	if k < 1 {
		u.tweening = true
	}
	return easeOut(k)
}

// rise is the offset an arriving element is drawn at.
func rise(e float64) float32 { return float32(1-e) * enterRise }

// glide eases a row's position toward y: rows reorder by sliding, not
// jumping. A row new to the view starts where it belongs.
func (u *UI) glide(key string, y float32) float32 {
	return float32(u.tw("y:"+key, float64(y), float64(y)))
}

// gliding reports whether key has a position from before.
func (u *UI) gliding(key string) bool { return u.tweens["y:"+key] != nil }

// resetMotion: a view opens fresh, so nothing glides from where it was
// last time, and everything enters again.
func (u *UI) resetMotion() {
	clear(u.enterAt)
	clear(u.rolls) // a timer doesn't roll from what it said last time
	for k := range u.tweens {
		for _, pre := range []string{"y:", "hl:", "row:", "chev:", "pr:", "cur:"} {
			if strings.HasPrefix(k, pre) {
				delete(u.tweens, k)
			}
		}
	}
}

// fade scales a rectangle of a premultiplied canvas by k: an element drawn
// at full strength, then faded as one.
func (c *canvas) fade(r image.Rectangle, k float64) {
	if k >= 1 {
		return
	}
	r = r.Intersect(c.img.Rect)
	kk := uint32(math.Max(0, k)*256 + 0.5)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		px := words(c.img.Pix[y*c.img.Stride : (y+1)*c.img.Stride])
		for x := r.Min.X; x < r.Max.X; x++ {
			if v := px[x]; v != 0 {
				px[x] = scale(v, kk)
			}
		}
	}
}

// eyebrow draws small tracked capitals (a card's kind, "KINVAULT WANTS TO
// RUN") and returns their width.
func (c *canvas) eyebrow(s string, x, base float32, col color.NRGBA) float32 {
	f := fEyebrow()
	x0 := x
	for _, r := range strings.ToUpper(s) {
		ch := string(r)
		c.text(f, ch, x, base, col)
		x += float32(advance(f, ch))/64 + 0.6
	}
	return x - x0 - 0.6
}

func eyebrowW(s string) float32 {
	f := fEyebrow()
	w := float32(0)
	for _, r := range strings.ToUpper(s) {
		w += float32(advance(f, string(r)))/64 + 0.6
	}
	return max(w-0.6, 0)
}
