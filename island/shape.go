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
		e := cornerN // continuous corners on the card; a pill keeps round ends
		if n.rb >= n.h/2-0.5 {
			e = 2
		}
		in := cornerInset(n.rb, d, e)
		lx, rx = lx+in, rx-in
	}
	if cy < n.ear {
		dy := n.ear - cy
		out := n.ear - float32(math.Sqrt(float64(max(0, n.ear*n.ear-dy*dy))))
		lx, rx = lx-out, rx+out
	}
	return lx, rx, true
}

// fillNotch fills the shape grown by `grow` px (shadows, borders). If inner
// is set, pixels well inside it are skipped: a shadow or a border needn't
// paint under the body that covers it anyway. Fully covered pixels take an
// integer path; only the anti-aliased edge pixels do float coverage.
func (c *canvas) fillNotch(n notch, grow float32, col color.NRGBA, inner *notch, yOff float32, minRow int) {
	if col.A == 0 {
		return
	}
	g := n
	g.x, g.w, g.h = n.x-grow, n.w+2*grow, n.h+grow
	g.rb = n.rb + grow
	b := c.img.Rect
	sa := float32(col.A) / 255
	sr, sg, sb := float32(col.R)*sa, float32(col.G)*sa, float32(col.B)*sa
	ia := uint32(col.A) + uint32(col.A)>>7 // 0..256
	ir, ig, ib, iA := uint32(sr+0.5), uint32(sg+0.5), uint32(sb+0.5), uint32(col.A)
	inv := 256 - ia
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
		if inner != nil {
			if ilx, irx, ok := inner.span(float32(py) + 0.5); ok && irx-ilx > 4 {
				skip0, skip1 = int(math.Ceil(float64(ilx)))+1, int(irx)-1
			}
		}
		full0, full1 := int(math.Ceil(float64(lx))), int(rx) // pixels the shape covers completely
		row := c.img.Pix[py*c.img.Stride:]
		for px := x0; px < x1; px++ {
			if px >= skip0 && px < skip1 {
				px = skip1 - 1
				continue
			}
			i := px * 4
			if vcov >= 1 && px >= full0 && px < full1 {
				row[i] = uint8(ir + uint32(row[i])*inv>>8)
				row[i+1] = uint8(ig + uint32(row[i+1])*inv>>8)
				row[i+2] = uint8(ib + uint32(row[i+2])*inv>>8)
				row[i+3] = uint8(iA + uint32(row[i+3])*inv>>8)
				continue
			}
			cov := (min(rx, float32(px+1)) - max(lx, float32(px))) * vcov
			if cov <= 0 {
				continue
			}
			cov = min(cov, 1)
			a := sa * cov
			fi := 1 - a
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
	for i := 7; i >= 1; i-- { // a soft, wide shadow: rings with a gaussian falloff, only outside the body
		a := 30 * math.Exp(-math.Pow(float64(i)/3.4, 2))
		c.fillNotch(n, float32(i)*1.6, color.NRGBA{0, 0, 0, uint8(alpha * a)}, &n, 2.5, barH-2)
	}
	c.fillNotch(n, 1, mixAlpha(p.line, 0.5*alpha), &n, 0, barH/2)
	body := p.island
	body.A = uint8(float64(body.A) * alpha)
	c.fillBody(n, body)
}

// fillBody fills the shape itself. Its interior is known to be transparent
// (the canvas was cleared and the shadow and hairline skip it), so whole
// stretches of a row are a copy of one premultiplied pixel; only the
// anti-aliased edges blend.
func (c *canvas) fillBody(n notch, col color.NRGBA) {
	if col.A == 0 {
		return
	}
	a := float32(col.A) / 255
	px := [4]uint8{uint8(float32(col.R)*a + 0.5), uint8(float32(col.G)*a + 0.5), uint8(float32(col.B)*a + 0.5), col.A}
	w := c.img.Rect.Dx()
	if len(c.bodyRow) != w*4 || c.bodyPx != px {
		c.bodyRow, c.bodyPx = make([]byte, w*4), px
		for i := 0; i < w; i++ {
			copy(c.bodyRow[i*4:], px[:])
		}
	}
	b := c.img.Rect
	for py := b.Min.Y; py < min(int(math.Ceil(float64(n.h))), b.Max.Y); py++ {
		lx, rx, ok := n.span(float32(py) + 0.5)
		if !ok {
			continue
		}
		f0, f1 := max(int(math.Ceil(float64(lx)))+1, b.Min.X), min(int(rx)-1, b.Max.X)
		if vcov := n.h - float32(py); vcov < 1 || f1-f0 < 4 { // the last, partial row: the general path
			continue
		}
		row := c.img.Pix[py*c.img.Stride:]
		copy(row[f0*4:f1*4], c.bodyRow[f0*4:f1*4])
	}
	// the edges and the partial bottom row, with coverage
	c.fillNotchEdges(n, col)
}

// fillNotchEdges blends the body into the pixels fillBody's copy left: the
// first and last two of each row, and the bottom row if it's partial.
func (c *canvas) fillNotchEdges(n notch, col color.NRGBA) {
	b := c.img.Rect
	sa := float32(col.A) / 255
	sr, sg, sb := float32(col.R)*sa, float32(col.G)*sa, float32(col.B)*sa
	for py := b.Min.Y; py < min(int(math.Ceil(float64(n.h))), b.Max.Y); py++ {
		lx, rx, ok := n.span(float32(py) + 0.5)
		if !ok {
			continue
		}
		vcov := min(float32(1), n.h-float32(py))
		x0, x1 := max(int(math.Floor(float64(lx))), b.Min.X), min(int(math.Ceil(float64(rx))), b.Max.X)
		f0, f1 := int(math.Ceil(float64(lx)))+1, int(rx)-1
		if vcov < 1 || f1-f0 < 4 {
			f0, f1 = x1, x1 // nothing was copied on this row
		}
		row := c.img.Pix[py*c.img.Stride:]
		for px := x0; px < x1; px++ {
			if px >= f0 && px < f1 {
				px = f1 - 1
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
