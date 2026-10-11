package island

import (
	"image"
	"image/color"
	"math"
)

// The island hangs from the top edge: flat top, rounded bottom corners, and
// small concave "ears" where it meets the bar, like a notch growing out of it.

type notch struct {
	x, w, h float32 // top edge at y=0
	rb      float32 // bottom corner radius
	ear     float32 // concave fillet where the top meets the bar
}

// span returns the shape's horizontal extent on the row centred at cy.
func (n notch) span(cy float32) (lx, rx float32, ok bool) {
	if cy < 0 || cy > n.h {
		return 0, 0, false
	}
	lx, rx = n.x, n.x+n.w
	if d := cy - (n.h - n.rb); d > 0 {
		in := n.rb - float32(math.Sqrt(float64(max(0, n.rb*n.rb-d*d))))
		lx, rx = lx+in, rx-in
	}
	if cy < n.ear {
		dy := n.ear - cy
		out := n.ear - float32(math.Sqrt(float64(max(0, n.ear*n.ear-dy*dy))))
		lx, rx = lx-out, rx+out
	}
	return lx, rx, true
}

// fillNotch fills the shape grown by `grow` px (shadows, borders), skipping
// hole (interior that the fill will cover anyway).
func (c *canvas) fillNotch(n notch, grow float32, col color.NRGBA, hole image.Rectangle, yOff float32, minRow int) {
	if col.A == 0 {
		return
	}
	g := n
	g.x, g.w, g.h = n.x-grow, n.w+2*grow, n.h+grow
	g.rb = n.rb + grow
	b := c.img.Rect
	sa := float32(col.A) / 255
	sr, sg, sb := float32(col.R)*sa, float32(col.G)*sa, float32(col.B)*sa
	y1 := min(int(math.Ceil(float64(g.h+yOff))), b.Max.Y)
	for py := max(b.Min.Y, int(yOff), minRow); py < y1; py++ {
		cy := float32(py) + 0.5 - yOff
		lx, rx, ok := g.span(cy)
		if !ok {
			continue
		}
		vcov := min(float32(1), g.h-(float32(py)-yOff))
		if vcov <= 0 {
			continue
		}
		x0, x1 := max(int(math.Floor(float64(lx))), b.Min.X), min(int(math.Ceil(float64(rx))), b.Max.X)
		skip0, skip1 := 0, 0
		if py >= hole.Min.Y && py < hole.Max.Y {
			skip0, skip1 = hole.Min.X, hole.Max.X
		}
		row := c.img.Pix[py*c.img.Stride:]
		for px := x0; px < x1; px++ {
			if px >= skip0 && px < skip1 {
				px = skip1 - 1
				continue
			}
			cov := (min(rx, float32(px+1)) - max(lx, float32(px))) * vcov
			if cov <= 0 {
				continue
			}
			cov = min(cov, 1)
			a := sa * cov
			fi := 1 - a
			i := px * 4
			row[i] = uint8(sr*cov + float32(row[i])*fi)
			row[i+1] = uint8(sg*cov + float32(row[i+1])*fi)
			row[i+2] = uint8(sb*cov + float32(row[i+2])*fi)
			row[i+3] = uint8(a*255 + float32(row[i+3])*fi)
		}
	}
}

// interior is a rectangle safely inside the shape.
func (n notch) interior() image.Rectangle {
	return image.Rect(int(n.x)+2, 0, int(n.x+n.w)-2, int(n.h-n.rb*0.4)-1)
}

func (n notch) bounds() image.Rectangle {
	return image.Rect(int(n.x-n.ear)-1, 0, int(n.x+n.w+n.ear)+2, int(n.h)+1)
}

// blurRects approximates the shape with a few rectangles for the compositor.
func (n notch) blurRects() []image.Rectangle {
	if n.w < 2 || n.h < 2 {
		return nil
	}
	top := int(n.h - n.rb)
	rs := []image.Rectangle{image.Rect(int(n.x), 0, int(n.x+n.w), top)}
	const strips = 4
	for i := 0; i < strips; i++ {
		y0 := float32(top) + n.rb*float32(i)/strips
		y1 := float32(top) + n.rb*float32(i+1)/strips
		lx, rx, _ := n.span(y1 - 0.5)
		rs = append(rs, image.Rect(int(math.Ceil(float64(lx))), int(y0), int(rx), int(math.Ceil(float64(y1)))))
	}
	return rs
}

// drawNotch paints shadow, hairline and body. Nothing is drawn around the
// part that sits in the bar (rows above barH): there it is part of the bar.
func (c *canvas) drawNotch(p palette, n notch, alpha float64, barH int) {
	hole := n.interior()
	for i := 4; i >= 1; i-- {
		c.fillNotch(n, float32(i)*2, color.NRGBA{0, 0, 0, uint8(alpha * float64(5-i) * 7)}, hole, 2, barH-2)
	}
	c.fillNotch(n, 1, mixAlpha(p.line, 0.5*alpha), hole, 0, barH/2)
	body := p.island
	body.A = uint8(float64(body.A) * alpha)
	c.fillNotch(n, 0, body, image.Rectangle{}, 0, 0)
}

func mixAlpha(c color.NRGBA, a float64) color.NRGBA {
	c.A = uint8(math.Round(float64(c.A) * math.Max(0, math.Min(1, a))))
	return c
}

// contains reports whether r lies inside the shape (checked on its top and bottom rows).
func (n notch) contains(r image.Rectangle) bool {
	if float32(r.Max.Y) > n.h {
		return false
	}
	for _, y := range []int{r.Min.Y, r.Max.Y - 1} {
		lx, rx, ok := n.span(float32(y) + 0.5)
		if !ok || float32(r.Min.X) < lx || float32(r.Max.X) > rx {
			return false
		}
	}
	return true
}
