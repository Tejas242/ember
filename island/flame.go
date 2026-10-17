package island

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// ---- Ember: a small flame with a face ----
//
// Ember has its own fire. It ignores the theme and the wallpaper, so it is
// the same character everywhere. Its mood is the flame's temperature: a calm
// coral ember, a working orange, the gold of a job well done, the blue of a
// flame running hot because something needs you, a deep red when it stopped.
// Each fire has three stops: the hot base and core, the body, the cooler tips.

type fire struct{ hot, mid, tip color.NRGBA }

func rgb(v uint32) color.NRGBA { return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

var fires = [...]fire{
	moodIdle:  {rgb(0xffdcb8), rgb(0xf08a5e), rgb(0xc4503e)},
	moodWork:  {rgb(0xfff0c4), rgb(0xffa23c), rgb(0xe0592a)},
	moodWait:  {rgb(0xe6f3ff), rgb(0x6cb0ff), rgb(0x4166e6)},
	moodHappy: {rgb(0xfffbe8), rgb(0xffcf48), rgb(0xf08c2c)},
	moodError: {rgb(0xffc8b6), rgb(0xee4f3e), rgb(0xa2262c)},
	moodNudge: {rgb(0xffe6bc), rgb(0xe6a24c), rgb(0xb4652e)},
}

func fireOf(m mood) fire {
	if int(m) < len(fires) {
		return fires[m]
	}
	return fires[moodIdle]
}

func (f fire) mix(g color.NRGBA, k float64) fire {
	return fire{mix(f.hot, g, k), mix(f.mid, g, k), mix(f.tip, g, k)}
}

type eyeKind uint8

const (
	eyeOpen  eyeKind = iota
	eyeHappy         // ^ ^
	eyeShut          // asleep
	eyeWince         // > <
)

const (
	mouthNone = iota
	mouthO
	mouthYawn
	mouthSmile
	mouthFlat
)

// pose is everything Ember looks like in one frame: its body (from the
// physics), its face (from the mind and the act it's playing), its light.
// Lengths are px at s = 16, the pill's size.
type pose struct {
	lift  float32 // up
	sq    float32 // squash: 1 rest, > 1 tall and thin, < 1 squat (its volume is kept)
	scale float32 // its size: 1 rest (smaller when worried or tired, puffed up when poked)
	lean  float32 // shear, px at the tip, + right: the base stays planted
	tip   float32 // the tip's own lag (follow-through)
	tipH  float32 // the tongue's height, a fraction (+ taller)
	sideH float32 // the left lick's
	coal  float32 // 0 a flame … 1 a round coal (asleep)

	eyeH         float32 // 1 a blink … 3 calm … 4 attentive … 5 startled
	lid          float32 // 0..1 of each eye's top covered
	slant        float32 // + determined (inner corners down), − worried (outer corners down)
	lookX, lookY float32 // where it looks, px
	eyes         eyeKind
	mouth        int
	cheeks       float32 // a blush

	core  float32 // the inner light's size, 1 rest
	flash float32 // the body toward white
	dim   float32 // the body toward the dark
}

func restPose() pose { return pose{sq: 1, scale: 1, eyeH: 3, core: 1} }

// flameGeo is Ember's outline for one pose: the transform from the drawing's
// rest coordinates to the canvas.
type flameGeo struct {
	cx, base, s, k float32 // k = s / 16
	sx, sy         float32
	ps             pose
}

func newGeo(cx, base, s float32, ps pose) flameGeo {
	sc := max(ps.scale, 0.2) * (1 - 0.12*ps.coal)
	s *= sc
	sq := max(ps.sq, 0.3)
	return flameGeo{cx: cx, base: base, s: s, k: s / 16 / sc, sx: 1 / float32(math.Sqrt(float64(sq))), sy: sq, ps: ps}
}

// at maps a rest point to the canvas: squash about the ground, a shear that
// grows with height (the base stays put), the tip lagging as the cube.
func (g flameGeo) at(x, y float32) pt {
	h := (g.base - y) / g.s
	return pt{g.cx + (x-g.cx)*g.sx + g.ps.lean*g.k*h + g.ps.tip*g.k*h*h*h, g.base - (g.base-y)*g.sy - g.ps.lift*g.k}
}

// outline is the flame's path: the round body, the tongue, the small lick on
// the left. scale shrinks it (the inner light), drop lowers it. As coal
// grows the tongues fold down and it becomes a round coal.
func (g flameGeo) outline(scale, drop float32, tipH, sideH float32) [19]pt {
	S := g.s / 1.1
	co := g.ps.coal
	r := S * 0.36 * scale
	x, y := g.cx, g.base-S*0.36+drop
	tipLen := S*0.74*scale*(1+tipH)*(1-co) + r*1.02*co
	tx, ty := x+r*0.3*(1-co), y-tipLen
	hy := func(f float32) float32 { return y - r*(1+(f-1)*(1-co)) }
	side := 1.45*(1+sideH)*(1-co) + 0.98*co
	k := r * kappa
	ps := [19]pt{
		{tx, ty},
		{tx + r*0.12, ty + r*0.8}, {x + r*1.08, y - r*0.95}, {x + r, y},
		{x + r, y + k}, {x + k, y + r}, {x, y + r},
		{x - k, y + r}, {x - r, y + k}, {x - r, y},
		{x - r*1.06, y - r*0.55}, {x - r*1.12, hy(1.1)}, {x - r*0.9, y - r*side},
		{x - r*0.72, hy(1.2)}, {x - r*0.48, hy(1.08)}, {x - r*0.28, hy(1.12)},
		{x - r*0.08, hy(1.5)}, {tx - r*0.32, ty + r*0.62}, {tx, ty},
	}
	for i, q := range ps {
		ps[i] = g.at(q.x, q.y)
	}
	return ps
}

func bounds(ps []pt) (x0, y0, x1, y1 float32) {
	x0, y0, x1, y1 = 1e9, 1e9, -1e9, -1e9
	for _, q := range ps {
		x0, y0, x1, y1 = min(x0, q.x), min(y0, q.y), max(x1, q.x), max(y1, q.y)
	}
	return
}

func cubicPath(ps []pt) func(*vector.Rasterizer, float32, float32) {
	return func(r *vector.Rasterizer, ox, oy float32) {
		r.MoveTo(ps[0].x-ox, ps[0].y-oy)
		for i := 1; i+2 < len(ps); i += 3 {
			r.CubeTo(ps[i].x-ox, ps[i].y-oy, ps[i+1].x-ox, ps[i+1].y-oy, ps[i+2].x-ox, ps[i+2].y-oy)
		}
		r.ClosePath()
	}
}

// fillShade rasterizes path and paints each pixel it covers with shade at
// that pixel: a gradient, where fillPath can only fill one colour.
func (c *canvas) fillShade(x0, y0, x1, y1 float32, shade func(x, y float32) color.NRGBA, path func(*vector.Rasterizer, float32, float32)) {
	b := c.img.Rect
	bx, by := max(int(math.Floor(float64(x0)))-1, b.Min.X), max(int(math.Floor(float64(y0)))-1, b.Min.Y)
	bw := min(int(math.Ceil(float64(x1)))+1, b.Max.X) - bx
	bh := min(int(math.Ceil(float64(y1)))+1, b.Max.Y) - by
	if bw <= 0 || bh <= 0 {
		return
	}
	if c.abuf == nil || c.abuf.Rect.Dx() < bw || c.abuf.Rect.Dy() < bh {
		c.abuf = image.NewAlpha(image.Rect(0, 0, max(bw, 64), max(bh, 64)))
	}
	m := c.abuf
	for y := 0; y < bh; y++ {
		clear(m.Pix[y*m.Stride : y*m.Stride+bw])
	}
	c.r.Reset(bw, bh)
	path(c.r, float32(bx), float32(by))
	c.r.Draw(m, image.Rect(0, 0, bw, bh), image.Opaque, image.Point{})
	for y := 0; y < bh; y++ {
		row := c.img.Pix[(by+y)*c.img.Stride:]
		for x := 0; x < bw; x++ {
			cov := uint32(m.Pix[y*m.Stride+x])
			if cov == 0 {
				continue
			}
			col := c.fadeCol(shade(float32(bx+x)+0.5, float32(by+y)+0.5))
			a := uint32(col.A) * cov / 255
			if a == 0 {
				continue
			}
			i := (bx + x) * 4
			inv := 255 - a
			row[i] = uint8((uint32(col.R)*a + uint32(row[i])*inv) / 255)
			row[i+1] = uint8((uint32(col.G)*a + uint32(row[i+1])*inv) / 255)
			row[i+2] = uint8((uint32(col.B)*a + uint32(row[i+2])*inv) / 255)
			row[i+3] = uint8(a + uint32(row[i+3])*inv/255)
		}
	}
}

func smoothstep(e0, e1, x float32) float32 {
	t := min(max((x-e0)/(e1-e0), 0), 1)
	return t * t * (3 - 2*t)
}

func lerpCol(a, b color.NRGBA, t float32) color.NRGBA { return mix(a, b, float64(t)) }

// drawFlame draws Ember standing on (cx, base), s tall, in fire f.
func (c *canvas) drawFlame(p palette, cx, base, s float32, ps pose, f fire, a float64) {
	g := newGeo(cx, base, s, ps)
	dark := mix(p.ink, color.NRGBA{40, 14, 8, 255}, 0.5)
	if ps.coal > 0 { // a coal: the body darkens, its light moves inside and warms
		f.mid = mix(f.mid, rgb(0xb4452c), float64(0.6*ps.coal))
		f.tip = mix(f.tip, rgb(0x6e2a20), float64(0.6*ps.coal))
		f.hot = mix(f.hot, rgb(0xffb070), float64(0.5*ps.coal))
	}
	if ps.flash > 0 {
		f = f.mix(colorWhite, float64(min(ps.flash, 1)))
	}
	if ps.dim > 0 {
		f.mid, f.tip = mix(f.mid, dark, float64(min(ps.dim, 1))), mix(f.tip, dark, float64(min(ps.dim, 1)))
		f.hot = mix(f.hot, f.mid, float64(min(ps.dim, 1)*0.5))
	}
	f.hot, f.mid, f.tip = mixAlpha(f.hot, a), mixAlpha(f.mid, a), mixAlpha(f.tip, a)

	out := g.outline(1, 0, ps.tipH, ps.sideH)
	x0, y0, x1, y1 := bounds(out[:])
	top, h := y0, max(g.base-y0, 1)
	r := g.s / 1.1 * 0.36
	c.fillShade(x0, y0, x1, y1, func(x, y float32) color.NRGBA {
		v := (g.base - y) / h // 0 at its base … 1 at the tip
		col := lerpCol(f.hot, f.mid, smoothstep(0, 0.32, v))
		col = lerpCol(col, f.tip, smoothstep(0.58, 1.05, v))
		// rounder: the sides fall off toward the tip colour
		if e := smoothstep(0.62, 1.2, abs32(x-g.cx-ps.lean*g.k*v)/r) * 0.24 * (1 - v/2); e > 0 {
			col = lerpCol(col, f.tip, e)
		}
		return col
	}, cubicPath(out[:]))
	_ = top

	// the light inside: the face sits on it, so the eyes read dark on bright
	in := g.outline(0.56*ps.core, g.s*0.035, ps.tipH*0.6, ps.sideH*0.5)
	ix0, iy0, ix1, iy1 := bounds(in[:])
	ih := max(iy1-iy0, 1)
	glowTop := mix(f.hot, f.mid, 0.45)
	c.fillShade(ix0, iy0, ix1, iy1, func(x, y float32) color.NRGBA {
		v := (iy1 - y) / ih
		return lerpCol(f.hot, glowTop, smoothstep(0.35, 1, v))
	}, cubicPath(in[:]))

	c.drawFace(p, g, f, a)
}

// drawFace draws the eyes (and a mouth where one reads). At pill sizes it
// snaps to whole pixels: an eye a pixel and a half wide on a fractional
// position is a grey smudge, a 2 px eye on the grid is a crisp mark.
func (c *canvas) drawFace(p palette, g flameGeo, f fire, a float64) {
	ps, k := g.ps, g.k
	if ps.coal > 0.6 && ps.eyes != eyeShut {
		ps.eyes = eyeShut
	}
	ink := mixAlpha(p.ink, a)
	small := g.s < 30
	snap := func(v float32) float32 {
		if small {
			return float32(math.Round(float64(v)))
		}
		return v
	}
	sc := g.s / 16 / k // the pose's own scale
	kk := k * sc
	ctr := g.at(g.cx+ps.lookX*kk, g.base-4.9*kk+ps.lookY*kk)
	fx, fy := snap(ctr.x), ctr.y
	ew := max(snap(2*kk), 1)
	gap := max(snap(2*kk), 1)
	eh := max(snap(ps.eyeH*kk*min(g.sy, 1.25)), 1)
	eyes := [2]float32{fx - gap/2 - ew, fx + gap/2}
	if small && int(gap)%2 == 1 {
		eyes = [2]float32{fx - float32(math.Floor(float64(gap)/2)) - ew, fx + float32(math.Ceil(float64(gap)/2))}
	}
	if small {
		eyes[0], eyes[1] = snap(eyes[0]), snap(eyes[1])
	}
	top := snap(fy - eh/2)

	if ps.cheeks > 0 {
		ch := mixAlpha(mix(f.mid, rgb(0xff6f6a), 0.6), float64(0.8*ps.cheeks)*a)
		c.circle(eyes[0]-1.2*kk, top+eh+0.8*kk, 1.2*kk, ch)
		c.circle(eyes[1]+ew+1.2*kk, top+eh+0.8*kk, 1.2*kk, ch)
	}

	switch ps.eyes {
	case eyeHappy, eyeShut, eyeWince:
		if small { // pixel glyphs: a 3×2 chevron reads at 1×, an arc that size blurs
			px := max(snap(kk), 1)
			var gl [2][]string
			switch ps.eyes {
			case eyeWince:
				gl = [2][]string{{"X.", ".X", "X."}, {".X", "X.", ".X"}}
			case eyeShut:
				gl = [2][]string{{"X.X", ".X."}, {"X.X", ".X."}}
			default:
				gl = [2][]string{{".X.", "X.X"}, {".X.", "X.X"}}
			}
			for i := range 2 {
				rows := gl[i]
				gw := float32(len(rows[0])) * px
				gx := fx - px/2 - gw
				if i == 1 {
					gx = fx + px/2
				}
				gx = snap(gx)
				gy := snap(fy - float32(len(rows))*px/2)
				for ry, row := range rows {
					for rx, ch := range row {
						if ch == 'X' {
							c.roundRect(gx+float32(rx)*px, gy+float32(ry)*px, px, px, 0, ink)
						}
					}
				}
			}
			break
		}
		w := max(1.1*kk, 1)
		for i, ex := range eyes {
			cx0, y0 := ex+ew/2, fy
			switch ps.eyes {
			case eyeShut:
				c.line(pt{cx0 - 1.6*kk, y0 - 0.6*kk}, pt{cx0, y0 + 0.7*kk}, w, ink)
				c.line(pt{cx0, y0 + 0.7*kk}, pt{cx0 + 1.6*kk, y0 - 0.6*kk}, w, ink)
			case eyeHappy:
				c.line(pt{cx0 - 1.6*kk, y0 + 1*kk}, pt{cx0, y0 - 0.8*kk}, w, ink)
				c.line(pt{cx0, y0 - 0.8*kk}, pt{cx0 + 1.6*kk, y0 + 1*kk}, w, ink)
			default:
				l, rr := ex, ex+ew
				if i == 1 {
					l, rr = rr, l
				}
				c.line(pt{l, fy - eh/2}, pt{rr, fy}, w, ink)
				c.line(pt{rr, fy}, pt{l, fy + eh/2}, w, ink)
			}
		}
	default:
		for i, ex := range eyes {
			// lids and slants cut the eye's top: whole pixels at pill sizes
			inner, outer := ps.lid*eh, ps.lid*eh
			if ps.slant > 0 {
				inner += ps.slant * eh * 0.7
			} else if ps.slant < 0 {
				outer += -ps.slant * eh * 0.7
			}
			minH := max(1, 0.9*kk)
			inner, outer = min(snap(inner), eh-minH), min(snap(outer), eh-minH)
			if inner <= 0.05 && outer <= 0.05 {
				c.roundRect(ex, top, ew, eh, min(ew, eh)*0.3, ink)
			} else {
				ly, ry := top+outer, top+inner
				if i == 1 {
					ly, ry = top+inner, top+outer
				}
				b := top + eh
				c.poly(ink, pt{ex, ly}, pt{ex + ew, ry}, pt{ex + ew, b}, pt{ex, b})
			}
			if !small && ps.eyeH >= 3 { // a catchlight, where there's room for one
				c.roundRect(ex+ew-max(1, 0.5*kk), top+max(inner, outer)*0.5, max(1, 0.5*kk), max(1, 0.5*kk), 0, mixAlpha(f.hot, a*0.9))
			}
		}
	}

	my := top + eh + snap(1.1*kk)
	switch ps.mouth {
	case mouthO:
		w := max(snap(2*kk), 2)
		c.roundRect(snap(fx-w/2), my, w, w, w*0.3, ink)
	case mouthYawn:
		w, h := max(snap(2*kk), 2), max(snap(3*kk), 2)
		c.roundRect(snap(fx-w/2), my-snap(0.4*kk), w, h, w*0.4, ink)
	case mouthSmile:
		if small {
			c.roundRect(snap(fx-1.5*kk), my, max(snap(3*kk), 3), 1, 0, ink)
			c.roundRect(snap(fx-1.5*kk)-1, my-1, 1, 1, 0, ink)
			c.roundRect(snap(fx-1.5*kk)+max(snap(3*kk), 3), my-1, 1, 1, 0, ink)
		} else {
			c.arc(fx, my-1.2*kk, 1.6*kk, max(kk, 1), 0.35, math.Pi-0.35, ink)
		}
	case mouthFlat:
		c.roundRect(snap(fx-1*kk), my, max(snap(2*kk), 2), max(snap(0.9*kk), 1), 0, ink)
	}
}

// tipAt is where the tongue's tip is, for sparks and smoke.
func (g flameGeo) tipAt() pt {
	o := g.outline(1, 0, g.ps.tipH, g.ps.sideH)
	return o[0]
}

// headAt is the centre of the round body and its radius, for the headphones.
func (g flameGeo) headAt() (pt, float32) {
	S := g.s / 1.1
	r := S * 0.36
	return g.at(g.cx, g.base-r), r * g.sx
}
