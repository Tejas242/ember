package island

import (
	"bufio"
	"embed"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tejas242/ember/internal/config"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

//go:embed fonts/*.ttf
var fontFS embed.FS

type palette struct {
	bg, raised, line, fg, muted, faint color.NRGBA
	green, blue, red, yellow, ink      color.NRGBA
	island                             color.NRGBA // the body: dark, a little see-through over the blur
}

type fontFace = font.Face

var colorWhite = color.NRGBA{255, 255, 255, 255}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}

func alpha(c color.NRGBA, a float64) color.NRGBA {
	c.A = uint8(math.Round(float64(c.A) * math.Max(0, math.Min(1, a))))
	return c
}

// themePath is Noctalia's generated alacritty theme: it follows the wallpaper palette.
func themePath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "alacritty", "themes", "noctalia.toml")
}

func loadPalette() palette {
	col := map[string]color.NRGBA{
		"primary.background": {0x0f, 0x15, 0x0e, 255}, "primary.foreground": {0xde, 0xe5, 0xd8, 255},
		"normal.green": {0x72, 0xdd, 0x71, 255}, "normal.blue": {0x9c, 0xca, 0xff, 255},
		"normal.red": {0xff, 0xb4, 0xab, 255}, "normal.yellow": {0xf2, 0xd3, 0x8a, 255},
	}
	if f, err := os.Open(themePath()); err == nil {
		sc := bufio.NewScanner(f)
		section := ""
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "[colors.") {
				section = strings.TrimSuffix(strings.TrimPrefix(line, "[colors."), "]")
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			if c, ok := config.HexColor(v); ok {
				col[section+"."+strings.TrimSpace(k)] = c
			}
		}
		f.Close()
	}
	bg, fg := col["primary.background"], col["primary.foreground"]
	black := color.NRGBA{0, 0, 0, 255}
	base := mix(bg, black, 0.45)
	return palette{
		bg: base, raised: mix(base, fg, 0.07), line: mix(base, fg, 0.13),
		fg: fg, muted: mix(fg, base, 0.36), faint: mix(fg, base, 0.5),
		green: col["normal.green"], blue: col["normal.blue"], red: col["normal.red"],
		yellow: mix(col["normal.yellow"], color.NRGBA{0xff, 0xc4, 0x6b, 255}, 0.7), // warm amber, whatever the wallpaper
		ink:    base, island: color.NRGBA{base.R, base.G, base.B, 222},
	}
}

// ---- fonts ----

type faceKey struct {
	name string
	size float64
}

var (
	fonts = map[string]*opentype.Font{}
	faces = map[faceKey]font.Face{}
)

func face(name string, size float64) font.Face {
	k := faceKey{name, size}
	if f := faces[k]; f != nil {
		return f
	}
	ft := fonts[name]
	if ft == nil {
		b, err := fontFS.ReadFile("fonts/" + name + ".ttf")
		if err != nil {
			panic(err)
		}
		if ft, err = opentype.Parse(b); err != nil {
			panic(err)
		}
		fonts[name] = ft
	}
	f, err := opentype.NewFace(ft, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingVertical})
	if err != nil {
		panic(err)
	}
	faces[k] = f
	faceInfo[f] = k
	return f
}

const (
	fRegular  = "Geist-Regular"
	fMedium   = "Geist-Medium"
	fSemiBold = "Geist-SemiBold"
	fMono     = "GeistMono-Regular"
)

// ---- canvas ----

type canvas struct {
	img     *image.RGBA
	r       *vector.Rasterizer
	masks   map[maskKey]*textMask
	bodyRow []byte               // a row of the island's body colour, to copy
	clip    *notch               // when set, fills stay inside this shape
	bodyPx  [4]byte              // the pixel bodyRow is made of
	k       float64              // when set (0..1), everything drawn is faded by it: a live part arriving with its row
	clipR   image.Rectangle      // when set, text stays inside it (a rolling digit)
	lit     map[litKey]*textMask // the sung part of a line in a script that's shaped
	abuf    *image.Alpha         // coverage for shaded fills (Ember), reused
}

// fadeCol applies the canvas's fade to col.
func (c *canvas) fadeCol(col color.NRGBA) color.NRGBA {
	if c.k > 0 && c.k < 1 {
		col.A = uint8(float64(col.A) * c.k)
	}
	return col
}

type maskKey struct {
	f font.Face
	s string
}

type textMask struct {
	m      *image.Alpha
	w, asc int
}

func newCanvas(w, h int) *canvas {
	return &canvas{img: image.NewRGBA(image.Rect(0, 0, w, h)), r: vector.NewRasterizer(1, 1), masks: map[maskKey]*textMask{}}
}

func (c *canvas) clear(r image.Rectangle) {
	r = r.Intersect(c.img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := c.img.Pix[y*c.img.Stride+r.Min.X*4 : y*c.img.Stride+r.Max.X*4]
		clear(row)
	}
}

// fillPath rasterizes path (in canvas coordinates) within its bounding box.
func (c *canvas) fillPath(x0, y0, x1, y1 float32, col color.NRGBA, path func(r *vector.Rasterizer, ox, oy float32)) {
	b := c.img.Rect
	bx, by := max(int(math.Floor(float64(x0)))-1, b.Min.X), max(int(math.Floor(float64(y0)))-1, b.Min.Y)
	bw := min(int(math.Ceil(float64(x1)))+1, b.Max.X) - bx
	bh := min(int(math.Ceil(float64(y1)))+1, b.Max.Y) - by
	col = c.fadeCol(col)
	if bw <= 0 || bh <= 0 || col.A == 0 {
		return
	}
	c.r.Reset(bw, bh)
	path(c.r, float32(bx), float32(by))
	c.r.Draw(c.img, image.Rect(bx, by, bx+bw, by+bh), image.NewUniform(col), image.Point{})
}

const kappa = 0.5522847

func roundRectPath(r *vector.Rasterizer, x, y, w, h, rad float32) {
	rad = min(rad, w/2, h/2)
	k := rad * kappa
	r.MoveTo(x+rad, y)
	r.LineTo(x+w-rad, y)
	r.CubeTo(x+w-rad+k, y, x+w, y+rad-k, x+w, y+rad)
	r.LineTo(x+w, y+h-rad)
	r.CubeTo(x+w, y+h-rad+k, x+w-rad+k, y+h, x+w-rad, y+h)
	r.LineTo(x+rad, y+h)
	r.CubeTo(x+rad-k, y+h, x, y+h-rad+k, x, y+h-rad)
	r.LineTo(x, y+rad)
	r.CubeTo(x, y+rad-k, x+rad-k, y, x+rad, y)
	r.ClosePath()
}

// roundRect fills a rounded rectangle by scanline spans with anti-aliased
// edges: several times faster than the general rasterizer for big shapes.
func (c *canvas) roundRect(x, y, w, h, rad float32, col color.NRGBA) {
	c.roundRectHole(x, y, w, h, rad, col, image.Rectangle{})
}

// roundRectHole skips pixels inside hole: shadows and borders don't need to
// paint the interior that the fill covers anyway.
func (c *canvas) roundRectHole(x, y, w, h, rad float32, col color.NRGBA, hole image.Rectangle) {
	col = c.fadeCol(col)
	if col.A == 0 || w <= 0 || h <= 0 {
		return
	}
	rad = min(rad, w/2, h/2)
	b := c.img.Rect
	sa := float32(col.A) / 255
	sr, sg, sb := float32(col.R)*sa, float32(col.G)*sa, float32(col.B)*sa
	// integer fast path for fully covered pixels: dst = src + dst*(256-a)/256
	ia := uint32(col.A) + uint32(col.A)>>7
	ir, ig, ib, iA := uint32(sr+0.5), uint32(sg+0.5), uint32(sb+0.5), uint32(col.A)
	inv := 256 - ia
	word := ir | ig<<8 | ib<<16 | iA<<24
	y0, y1 := max(int(math.Floor(float64(y))), b.Min.Y), min(int(math.Ceil(float64(y+h))), b.Max.Y)
	for py := y0; py < y1; py++ {
		vcov := min(y+h, float32(py+1)) - max(y, float32(py))
		if vcov <= 0 {
			continue
		}
		cy := float32(py) + 0.5
		inset := float32(0)
		e := cornerN // continuous corners; pills and circles stay round
		if rad >= min(w, h)/2-0.5 {
			e = 2
		}
		if d := y + rad - cy; d > 0 {
			inset = cornerInset(rad, d, e)
		} else if d := cy - (y + h - rad); d > 0 {
			inset = cornerInset(rad, d, e)
		}
		lx, rx := x+inset, x+w-inset
		if c.clip != nil { // inside the island only
			clx, crx, ok := c.clip.span(cy)
			if !ok {
				continue
			}
			lx, rx = max(lx, clx), min(rx, crx)
			if rx <= lx {
				continue
			}
		}
		x0, x1 := max(int(math.Floor(float64(lx))), b.Min.X), min(int(math.Ceil(float64(rx))), b.Max.X)
		skip0, skip1 := 0, 0
		if py >= hole.Min.Y && py < hole.Max.Y {
			skip0, skip1 = hole.Min.X, hole.Max.X
		}
		row := c.img.Pix[py*c.img.Stride:]
		if vcov >= 1 { // the fully covered middle of the row, a word at a time
			f0, f1 := max(int(math.Ceil(float64(lx))), x0), min(int(math.Floor(float64(rx))), x1)
			if f1-f0 > 2 {
				px := words(row[:c.img.Stride])
				if skip1 > skip0 && skip0 < f1 && skip1 > f0 {
					fillSpan(px, f0, max(f0, skip0), word)
					fillSpan(px, min(f1, skip1), f1, word)
				} else {
					fillSpan(px, f0, f1, word)
				}
				// leave only the edge pixels to the loop below
				if skip1 <= skip0 || skip0 >= f1 || skip1 <= f0 {
					skip0, skip1 = f0, f1
				} else {
					skip0, skip1 = min(f0, skip0), max(f1, skip1)
				}
			}
		}
		for px := x0; px < x1; px++ {
			if px >= skip0 && px < skip1 {
				px = skip1 - 1
				continue
			}
			i := px * 4
			if vcov >= 1 && float32(px) >= lx && float32(px+1) <= rx {
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

func (c *canvas) circle(cx, cy, rad float32, col color.NRGBA) {
	c.roundRect(cx-rad, cy-rad, 2*rad, 2*rad, rad, col)
}

func (c *canvas) mask(f font.Face, s string) *textMask {
	s = display(s)
	k := maskKey{f, s}
	if m := c.masks[k]; m != nil {
		return m
	}
	if len(c.masks) > 400 {
		clear(c.masks)
	}
	if needsShaping(s) && faceInfo[f].name != "" { // joined, reordered or right to left: shaped
		tm := shapedMask(f, s)
		c.masks[k] = tm
		return tm
	}
	met := f.Metrics()
	asc, desc := met.Ascent.Ceil(), met.Descent.Ceil()
	w := advance(f, s).Ceil()
	m := image.NewAlpha(image.Rect(0, 0, w+2, asc+desc+1))
	d := font.Drawer{Dst: m, Src: image.Opaque, Face: f, Dot: fixed.P(1, asc)}
	runs(f, s, func(fc font.Face, part string) { d.Face = fc; d.DrawString(part) })
	tm := &textMask{m: m, w: w, asc: asc}
	c.masks[k] = tm
	return tm
}

// text draws s with its baseline at y and returns its advance width.
func (c *canvas) text(f font.Face, s string, x, y float32, col color.NRGBA) float32 {
	if s == "" {
		return 0
	}
	return c.drawMask(c.mask(f, s), x, y, col)
}

// drawMask draws a text mask with its baseline at y and returns its advance width.
func (c *canvas) drawMask(m *textMask, x, y float32, col color.NRGBA) float32 {
	col = c.fadeCol(col)
	if col.A > 0 {
		px, py := int(math.Round(float64(x)))-1, int(math.Round(float64(y)))-m.asc
		dr := image.Rect(px, py, px+m.m.Rect.Dx(), py+m.m.Rect.Dy())
		r := dr
		if !c.clipR.Empty() {
			r = dr.Intersect(c.clipR)
		}
		if !r.Empty() {
			draw.DrawMask(c.img, r, image.NewUniform(col), image.Point{}, m.m, r.Min.Sub(dr.Min), draw.Over)
		}
	}
	return float32(m.w)
}

var widths = map[maskKey]float32{}

// measure is a string's width in f, remembered: layout asks the same
// questions every frame, and kerning lookups aren't free.
func measure(f font.Face, s string) float32 {
	k := maskKey{f, s}
	if w, ok := widths[k]; ok {
		return w
	}
	if len(widths) > 4000 {
		clear(widths)
	}
	w := float32(advance(f, display(s)).Ceil())
	widths[k] = w
	return w
}

func ellipsize(f font.Face, s string, maxW float32) string {
	if measure(f, s) <= maxW {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r) // the longest prefix that fits with "…": bisection, not one rune at a time
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(f, string(r[:mid])+"…") <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimRight(string(r[:lo]), " ") + "…"
}

// wrap greedily breaks s into at most n lines of width maxW.
func wrap(f font.Face, s string, maxW float32, n int) []string {
	var lines []string
	for _, para := range strings.Split(strings.TrimSpace(s), "\n") {
		cur := ""
		for _, w := range strings.Fields(para) {
			try := w
			if cur != "" {
				try = cur + " " + w
			}
			if measure(f, try) <= maxW || cur == "" {
				cur = try
				continue
			}
			lines = append(lines, cur)
			cur = w
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	if len(lines) > n {
		lines = lines[:n]
		lines[n-1] = ellipsize(f, lines[n-1]+" …", maxW)
	}
	for i := range lines {
		lines[i] = ellipsize(f, lines[i], maxW)
	}
	return lines
}

// ---- moods ----

type mood int

const (
	moodIdle mood = iota
	moodWork
	moodWait // needs you
	moodHappy
	moodError
	moodNudge
)

func (p palette) moodColor(m mood) color.NRGBA {
	switch m {
	case moodWork, moodHappy:
		return p.green
	case moodWait:
		return p.blue
	case moodError:
		return p.red
	case moodNudge:
		return p.yellow
	}
	return p.muted
}

// glowInside lights the island's body (and only its body: transparent
// pixels around it stay untouched) with a soft radial light.
func (c *canvas) glowInside(r image.Rectangle, cx, cy, rad float32, col color.NRGBA, strength float64) {
	r = r.Intersect(c.img.Rect)
	inv2 := 1 / (rad * rad)
	edge := func(d int) float32 { // light fades out toward the rectangle's edges: no seams
		f := min(float32(d)/14, 1)
		return f * f * (3 - 2*f)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := c.img.Pix[y*c.img.Stride:]
		dy := float32(y) + 0.5 - cy
		fy := edge(min(y-r.Min.Y, r.Max.Y-1-y))
		for x := r.Min.X; x < r.Max.X; x++ {
			i := x * 4
			if row[i+3] < 180 {
				continue
			}
			dx := float32(x) + 0.5 - cx
			d := (dx*dx + dy*dy) * inv2
			if d >= 1 {
				continue
			}
			st := uint32(float32(strength*256) * fy * edge(min(x-r.Min.X, r.Max.X-1-x)))
			k := falloff[int(d*255)] * st // (1-d)², from the table, in 1/256ths
			if k == 0 {
				continue
			}
			a := uint32(row[i+3]) // stay premultiplied: light scales with the body's own alpha
			for ch, v := range [3]uint8{col.R, col.G, col.B} {
				cur := uint32(row[i+ch])
				tgt := uint32(v) * a / 255
				if tgt > cur {
					row[i+ch] = uint8(cur + (tgt-cur)*k>>16)
				} else {
					row[i+ch] = uint8(cur - (cur-tgt)*k>>16)
				}
			}
		}
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// lock draws a small padlock (gate closed) at x,y with height s.
func (c *canvas) lock(x, y, s float32, col color.NRGBA) {
	bw, bh := s*0.78, s*0.55
	c.roundRect(x, y+s-bh, bw, bh, s*0.12, col)
	sw := s * 0.13
	c.fillPath(x, y, x+bw, y+s-bh+1, col, func(r *vector.Rasterizer, ox, oy float32) {
		cx, top := x+bw/2-ox, y-oy
		rx := bw/2 - sw*0.6
		bottom := top + s - bh + 1
		// outer arc
		r.MoveTo(cx-rx-sw/2, bottom)
		r.LineTo(cx-rx-sw/2, top+rx)
		r.CubeTo(cx-rx-sw/2, top-sw/2+rx*(1-kappa), cx-rx*kappa, top-sw/2, cx, top-sw/2)
		r.CubeTo(cx+rx*kappa, top-sw/2, cx+rx+sw/2, top-sw/2+rx*(1-kappa), cx+rx+sw/2, top+rx)
		r.LineTo(cx+rx+sw/2, bottom)
		r.LineTo(cx+rx-sw/2, bottom)
		r.LineTo(cx+rx-sw/2, top+rx)
		r.CubeTo(cx+rx-sw/2, top+sw/2+rx*(1-kappa)*0.6, cx+rx*kappa*0.7, top+sw/2, cx, top+sw/2)
		r.CubeTo(cx-rx*kappa*0.7, top+sw/2, cx-rx+sw/2, top+sw/2+rx*(1-kappa)*0.6, cx-rx+sw/2, top+rx)
		r.LineTo(cx-rx+sw/2, bottom)
		r.ClosePath()
	})
}

// falloff is (1−d)² for d in [0,1), in 1/256ths: the glow's shape.
var falloff = func() (t [256]uint32) {
	for i := range t {
		d := float64(i) / 255
		t[i] = uint32((1 - d) * (1 - d) * 256)
	}
	return
}()
