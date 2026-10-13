package island

import (
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// Small vector icons for system moments, drawn centred on (cx, cy) in a box
// of size s. Holes (the battery's inside, the screen's inside) are cut by
// winding the inner contour the other way.

type pt struct{ x, y float32 }

// poly fills a closed polygon.
func (c *canvas) poly(col color.NRGBA, ps ...pt) {
	if len(ps) < 3 {
		return
	}
	x0, y0, x1, y1 := ps[0].x, ps[0].y, ps[0].x, ps[0].y
	for _, p := range ps {
		x0, y0, x1, y1 = min(x0, p.x), min(y0, p.y), max(x1, p.x), max(y1, p.y)
	}
	c.fillPath(x0, y0, x1, y1, col, func(r *vector.Rasterizer, ox, oy float32) {
		r.MoveTo(ps[0].x-ox, ps[0].y-oy)
		for _, p := range ps[1:] {
			r.LineTo(p.x-ox, p.y-oy)
		}
		r.ClosePath()
	})
}

// line strokes a segment of width w with round caps.
func (c *canvas) line(a, b pt, w float32, col color.NRGBA) {
	dx, dy := b.x-a.x, b.y-a.y
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l == 0 {
		c.circle(a.x, a.y, w/2, col)
		return
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	c.poly(col, pt{a.x + nx, a.y + ny}, pt{b.x + nx, b.y + ny}, pt{b.x - nx, b.y - ny}, pt{a.x - nx, a.y - ny})
	c.circle(a.x, a.y, w/2, col)
	c.circle(b.x, b.y, w/2, col)
}

// arc strokes a circular arc (angles in radians, 0 = right, clockwise on screen).
func (c *canvas) arc(cx, cy, rad, w, a0, a1 float32, col color.NRGBA) {
	const n = 18
	var ps []pt
	for i := 0; i <= n; i++ {
		a := float64(a0 + (a1-a0)*float32(i)/n)
		ps = append(ps, pt{cx + (rad+w/2)*float32(math.Cos(a)), cy + (rad+w/2)*float32(math.Sin(a))})
	}
	for i := n; i >= 0; i-- {
		a := float64(a0 + (a1-a0)*float32(i)/n)
		ps = append(ps, pt{cx + (rad-w/2)*float32(math.Cos(a)), cy + (rad-w/2)*float32(math.Sin(a))})
	}
	c.poly(col, ps...)
}

// frame strokes a rounded rectangle's outline (a ring of width w).
func (c *canvas) frame(x, y, w, h, rad, sw float32, col color.NRGBA) {
	c.fillPath(x, y, x+w, y+h, col, func(r *vector.Rasterizer, ox, oy float32) {
		roundRectPath(r, x-ox, y-oy, w, h, rad)
		ir := max(rad-sw, 0.5)
		ix, iy, iw, ih := x+sw-ox, y+sw-oy, w-2*sw, h-2*sw
		k := ir * kappa
		// the same rounded rectangle, wound the other way: a hole
		r.MoveTo(ix+ir, iy)
		r.CubeTo(ix+ir-k, iy, ix, iy+ir-k, ix, iy+ir)
		r.LineTo(ix, iy+ih-ir)
		r.CubeTo(ix, iy+ih-ir+k, ix+ir-k, iy+ih, ix+ir, iy+ih)
		r.LineTo(ix+iw-ir, iy+ih)
		r.CubeTo(ix+iw-ir+k, iy+ih, ix+iw, iy+ih-ir+k, ix+iw, iy+ih-ir)
		r.LineTo(ix+iw, iy+ir)
		r.CubeTo(ix+iw, iy+ir-k, ix+iw-ir+k, iy, ix+iw-ir, iy)
		r.ClosePath()
	})
}

// battery draws a battery glyph filled to level (0..1); its width is 2.1×s.
func (c *canvas) battery(x, cy, s, level float32, body, fill color.NRGBA) {
	w, h := s*1.9, s
	y := cy - h/2
	c.frame(x, y, w, h, h*0.32, 1.2, body)
	c.roundRect(x+w+0.6, cy-h*0.18, 1.8, h*0.36, 0.9, body)
	in := float32(2.4)
	fw := max((w-2*in)*min(max(level, 0), 1), 1.5)
	c.roundRect(x+in, y+in, fw, h-2*in, h*0.18, fill)
}

func (c *canvas) icon(name string, cx, cy, s float32, col color.NRGBA) {
	u := s / 16 // design grid: 16 units
	P := func(x, y float32) pt { return pt{cx + (x-8)*u, cy + (y-8)*u} }
	switch name {
	case "volume", "volume-low", "mute":
		c.roundRect(cx-6.5*u, cy-2.6*u, 3.4*u, 5.2*u, 0.8*u, col)
		c.poly(col, P(3.6, 5.4), P(7.4, 2), P(7.4, 14), P(3.6, 10.6))
		switch name {
		case "mute":
			c.line(P(10, 5.5), P(14.5, 10.5), 1.5*u, col)
			c.line(P(14.5, 5.5), P(10, 10.5), 1.5*u, col)
		case "volume":
			c.arc(cx-0.6*u, cy, 6.6*u, 1.4*u, -0.9, 0.9, col)
			fallthrough
		default:
			c.arc(cx-0.6*u, cy, 3.6*u, 1.4*u, -0.85, 0.85, col)
		}
	case "sun":
		c.circle(cx, cy, 3.2*u, col)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			dx, dy := float32(math.Cos(a)), float32(math.Sin(a))
			c.line(pt{cx + dx*5.4*u, cy + dy*5.4*u}, pt{cx + dx*6.9*u, cy + dy*6.9*u}, 1.4*u, col)
		}
	case "battery":
		c.battery(cx-8*u, cy, 8.2*u, 0.22, col, col)
	case "bolt":
		c.poly(col, P(9.6, 1), P(3.2, 9.2), P(7.6, 9.2), P(6.4, 15), P(12.8, 6.8), P(8.4, 6.8))
	case "headphones":
		c.arc(cx, cy+0.5*u, 6*u, 1.5*u, math.Pi*1.02, math.Pi*1.98, col)
		c.roundRect(cx-7.4*u, cy+0.4*u, 3.6*u, 6.6*u, 1.4*u, col)
		c.roundRect(cx+3.8*u, cy+0.4*u, 3.6*u, 6.6*u, 1.4*u, col)
	case "bluetooth":
		w := 1.4 * u
		c.line(P(8, 1.5), P(8, 14.5), w, col)
		c.line(P(8, 1.5), P(11.6, 4.9), w, col)
		c.line(P(11.6, 4.9), P(4.4, 11.4), w, col)
		c.line(P(8, 14.5), P(11.6, 11.1), w, col)
		c.line(P(11.6, 11.1), P(4.4, 4.6), w, col)
	case "mic", "mic-off":
		c.roundRect(cx-2.4*u, cy-7*u, 4.8*u, 9*u, 2.4*u, col)
		c.arc(cx, cy-0.5*u, 4.6*u, 1.3*u, 0.15, math.Pi-0.15, col)
		c.line(P(8, 12.6), P(8, 15), 1.3*u, col)
		if name == "mic-off" {
			c.line(P(2.5, 2), P(13.5, 14.5), 1.5*u, col)
		}
	case "screen", "cast":
		c.frame(cx-7.2*u, cy-6*u, 14.4*u, 10*u, 1.8*u, 1.4*u, col)
		c.line(P(5.5, 14.4), P(10.5, 14.4), 1.4*u, col)
		if name == "cast" {
			c.circle(cx, cy-1*u, 2*u, col)
		}
	case "camera":
		c.frame(cx-7.2*u, cy-4.6*u, 14.4*u, 10.2*u, 2.2*u, 1.4*u, col)
		c.roundRect(cx-3*u, cy-6.6*u, 6*u, 2.6*u, 1*u, col)
		c.frame(cx-3*u, cy-2.5*u, 6*u, 6*u, 3*u, 1.4*u, col)
	case "read": // a page
		c.frame(cx-4.5*u, cy-6.5*u, 9*u, 13*u, 1.6*u, 1.3*u, col)
		c.line(P(6, 6), P(10, 6), 1.1*u, col)
		c.line(P(6, 8.5), P(10, 8.5), 1.1*u, col)
	case "edit": // a pencil
		c.line(P(3.5, 12.5), P(11.5, 4.5), 2.6*u, col)
		c.poly(col, P(2.2, 13.8), P(2.8, 11.2), P(4.8, 13.2))
	case "run": // a prompt: >_
		c.line(P(3, 4.5), P(7, 8), 1.5*u, col)
		c.line(P(7, 8), P(3, 11.5), 1.5*u, col)
		c.line(P(8.5, 12), P(13, 12), 1.5*u, col)
	case "search": // a lens
		c.frame(cx-6*u, cy-6*u, 9*u, 9*u, 4.5*u, 1.4*u, col)
		c.line(P(9.6, 9.6), P(13, 13), 1.6*u, col)
	case "web": // a globe
		c.frame(cx-6*u, cy-6*u, 12*u, 12*u, 6*u, 1.3*u, col)
		c.line(P(2.5, 8), P(13.5, 8), 1.1*u, col)
		c.frame(cx-2.6*u, cy-6*u, 5.2*u, 12*u, 2.6*u, 1.1*u, col)
	case "agent": // a spark
		c.poly(col, P(8, 2), P(9.4, 6.6), P(14, 8), P(9.4, 9.4), P(8, 14), P(6.6, 9.4), P(2, 8), P(6.6, 6.6))
	case "other":
		c.circle(cx, cy, 2.2*u, col)
	case "check":
		c.line(P(3, 8.5), P(6.6, 12), 1.8*u, col)
		c.line(P(6.6, 12), P(13, 4.5), 1.8*u, col)
	case "warning":
		c.poly(col, P(8, 1.5), P(15, 14), P(1, 14))
		c.roundRect(cx-0.8*u, cy-2.6*u, 1.6*u, 5*u, 0.8*u, color.NRGBA{0, 0, 0, 200})
		c.circle(cx, cy+4*u, 0.95*u, color.NRGBA{0, 0, 0, 200})
	}
}
