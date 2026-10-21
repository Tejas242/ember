package island

import (
	"image"
	"unsafe"
)

// Pixel arithmetic, two channels per multiply: a premultiplied RGBA pixel is
// one little-endian word, and R,B (or G,A) sit 16 bits apart, so a single
// multiply scales both.

// words views RGBA bytes as 32-bit pixels.
func words(b []byte) []uint32 {
	if len(b) < 4 {
		return nil
	}
	return unsafe.Slice((*uint32)(unsafe.Pointer(&b[0])), len(b)/4)
}

// scale multiplies every channel of v by k/256 (k ≤ 256).
func scale(v, k uint32) uint32 {
	rb := (v & 0x00FF00FF) * k >> 8 & 0x00FF00FF
	ag := (v >> 8 & 0x00FF00FF) * k & 0xFF00FF00
	return rb | ag
}

// over puts premultiplied src over dst: src + dst·(1 − αsrc).
func over(src, dst uint32) uint32 {
	a := src >> 24
	if a == 0 {
		return dst
	}
	return src + scale(dst, 256-(a+a>>7))
}

// fillSpan blends one premultiplied pixel over px[x0:x1].
func fillSpan(px []uint32, x0, x1 int, src uint32) {
	a := src >> 24
	if a == 0 || x1 <= x0 {
		return
	}
	inv := 256 - (a + a>>7)
	for i := x0; i < x1; i++ {
		px[i] = src + scale(px[i], inv)
	}
}

// blitAlpha lays a premultiplied sprite over the canvas at (x, y), at
// strength a (0..1): one multiply and one blend a pixel, the transparent
// ones skipped.
func (c *canvas) blitAlpha(src *image.RGBA, x, y int, a float64) {
	if src == nil || a <= 0.003 {
		return
	}
	k := uint32(min(a, 1)*256 + 0.5)
	if c.k > 0 && c.k < 1 {
		k = uint32(float64(k) * c.k)
	}
	r := src.Rect.Add(image.Pt(x, y)).Intersect(c.img.Rect)
	for py := r.Min.Y; py < r.Max.Y; py++ {
		so := (py-y)*src.Stride + (r.Min.X-x)*4
		do := py*c.img.Stride + r.Min.X*4
		s, d := words(src.Pix[so:so+r.Dx()*4]), words(c.img.Pix[do:do+r.Dx()*4])
		for i, v := range s {
			if v == 0 {
				continue
			}
			d[i] = over(scale(v, k), d[i])
		}
	}
}
