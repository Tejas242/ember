package island

import "unsafe"

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

// premul packs a straight colour as a premultiplied pixel at coverage k (0..256).
func premul(r, g, b, a uint8, k uint32) uint32 {
	aa := uint32(a) * k >> 8
	return uint32(r)*aa/255 | (uint32(g)*aa/255)<<8 | (uint32(b)*aa/255)<<16 | aa<<24
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
