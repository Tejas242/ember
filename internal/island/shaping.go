package island

import (
	"bytes"
	"image"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/di"
	gtfont "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
	xbidi "golang.org/x/text/unicode/bidi"
)

// Some scripts can't be drawn a letter at a time: Arabic's letters join and
// change shape with their neighbours, Devanagari's vowel signs move before
// the consonant they follow and its consonants fuse into conjuncts, and
// Arabic and Hebrew run right to left. A string with any of these is shaped
// (go-text's port of HarfBuzz) in the system font fontconfig has for its
// script, its runs put in visual order (the Unicode bidi algorithm), and its
// glyphs drawn from their outlines. Everything else (Latin, CJK, symbols)
// is drawn as before, letter by letter.

// needsShaping: s has a letter of a script that joins, reorders or runs
// right to left (or a bidi control). ASCII, the common case, is free.
func needsShaping(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			for _, r := range s[i:] {
				if shapedRune(r) {
					return true
				}
			}
			return false
		}
	}
	return false
}

func shapedRune(r rune) bool {
	switch {
	case r < 0x0590:
		return false
	case r <= 0x0DFF: // Hebrew, Arabic, Syriac, Thaana, N'Ko, Samaritan, Mandaic, the Indic scripts, Sinhala
		return true
	case r <= 0x109F: // Thai, Lao, Tibetan, Myanmar
		return r >= 0x0E00
	case r >= 0x1780 && r <= 0x17FF: // Khmer
		return true
	case r == 0x200E || r == 0x200F || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069): // bidi controls
		return true
	case r >= 0xA8E0 && r <= 0xA8FF: // Devanagari Extended
		return true
	case r >= 0xFB1D && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF: // Hebrew and Arabic presentation forms
		return true
	}
	return false
}

// rtlText: s reads right to left, its first strong letter Hebrew or Arabic
// (the Unicode bidi algorithm's paragraph direction). Latin answers at its
// first letter.
func rtlText(s string) bool {
	for _, r := range s {
		if r < 0x80 {
			if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
				return false
			}
			continue
		}
		p, _ := xbidi.LookupRune(r)
		switch p.Class() {
		case xbidi.L:
			return false
		case xbidi.R, xbidi.AL:
			return true
		}
	}
	return false
}

// ---- fonts for shaping ----

var (
	gtEmbedded  = map[string]*gtfont.Face{}    // Ember's own fonts, by name
	gtScript    = map[scriptKey]*gtfont.Face{} // a script's font at a weight; nil = none
	gtFiles     = map[string]*gtfont.Face{}    // by file and index
	shaper      shaping.HarfbuzzShaper
	segmenter   shaping.Segmenter
	bidiPara    bidi.Paragraph
	shapedLines = map[maskKey]*shapedText{}
)

type scriptKey struct {
	s      language.Script
	weight string
}

// scriptNames are the scripts whose fonts are asked for by name ("Noto Sans
// Devanagari"): fontconfig's first pick for a letter is often a font that
// has it without being made for it.
var scriptNames = map[language.Script]string{
	language.Arabic: "Arabic", language.Hebrew: "Hebrew", language.Syriac: "Syriac", language.Thaana: "Thaana",
	language.Nko: "NKo", language.Devanagari: "Devanagari", language.Bengali: "Bengali", language.Gurmukhi: "Gurmukhi",
	language.Gujarati: "Gujarati", language.Oriya: "Oriya", language.Tamil: "Tamil", language.Telugu: "Telugu",
	language.Kannada: "Kannada", language.Malayalam: "Malayalam", language.Sinhala: "Sinhala", language.Thai: "Thai",
	language.Lao: "Lao", language.Tibetan: "Tibetan", language.Myanmar: "Myanmar", language.Khmer: "Khmer",
}

func embeddedFace(name string) *gtfont.Face {
	if f, ok := gtEmbedded[name]; ok {
		return f
	}
	var f *gtfont.Face
	if b, err := fontFS.ReadFile("fonts/" + name + ".ttf"); err == nil {
		f, _ = gtfont.ParseTTF(bytes.NewReader(b))
	}
	gtEmbedded[name] = f
	return f
}

// weightOf is the fontconfig weight matching one of Ember's fonts.
func weightOf(name string) string {
	switch {
	case strings.HasSuffix(name, "SemiBold"):
		return "semibold"
	case strings.HasSuffix(name, "Medium"):
		return "medium"
	}
	return "regular"
}

// scriptFace is the system font for script s at weight w that has r: the
// script's own font if fontconfig has one ("Noto Sans Arabic"), else the
// first of fontconfig's candidates for r that parses and has it.
func scriptFace(s language.Script, w string, r rune) *gtfont.Face {
	k := scriptKey{s, w}
	f, tried := gtScript[k]
	if !tried {
		if name := scriptNames[s]; name != "" {
			f = fcFace(r, "Noto Sans "+name+":weight="+w, false)
		}
		gtScript[k] = f
	}
	if f != nil && hasGlyph(f, r) {
		return f
	}
	return runeFace(r, w)
}

type runeKey struct {
	block  rune
	weight string
}

var gtRunes = map[runeKey]*gtfont.Face{} // by Unicode block and weight; nil = none

// runeFace is fontconfig's first candidate for r that parses and has it,
// asked once per Unicode block.
func runeFace(r rune, w string) *gtfont.Face {
	k := runeKey{r >> 7, w}
	f, tried := gtRunes[k]
	if !tried {
		f = fcFace(r, "sans-serif:weight="+w+":charset="+hex(r), true)
		gtRunes[k] = f
	}
	if f != nil && hasGlyph(f, r) {
		return f
	}
	return nil
}

func fcFace(r rune, pattern string, sorted bool) *gtfont.Face {
	args := []string{"-f", "%{file}\t%{index}\n", pattern}
	if sorted {
		args = append([]string{"-s"}, args...)
	}
	out, err := exec.Command("fc-match", args...).Output()
	if err != nil {
		return nil
	}
	for i, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i >= 8 {
			break
		}
		file, idx, _ := strings.Cut(ln, "\t")
		n, _ := strconv.Atoi(idx)
		if f := fileFace(file, n); f != nil && hasGlyph(f, r) {
			return f
		}
	}
	return nil
}

func fileFace(file string, idx int) *gtfont.Face {
	k := file + "#" + strconv.Itoa(idx)
	if f, ok := gtFiles[k]; ok {
		return f
	}
	var f *gtfont.Face
	if b, err := os.ReadFile(file); err == nil {
		if fs, err := gtfont.ParseTTC(bytes.NewReader(b)); err == nil && idx >= 0 && idx < len(fs) {
			f = fs[idx]
		}
	}
	gtFiles[k] = f
	return f
}

func hasGlyph(f *gtfont.Face, r rune) bool {
	gid, ok := f.NominalGlyph(r)
	return ok && gid != 0
}

// lineFonts picks a face for each run of a line: Ember's own font where it
// has the letter, else the font for the letter's script (a space or a mark
// keeps the run's).
type lineFonts struct {
	base   *gtfont.Face
	weight string
	script language.Script
}

func (m *lineFonts) SetScript(s language.Script) { m.script = s }

func (m *lineFonts) ResolveFace(r rune) *gtfont.Face {
	if hasGlyph(m.base, r) {
		return m.base
	}
	s := language.LookupScript(r)
	if !s.Strong() {
		s = m.script
	}
	if f := scriptFace(s, m.weight, r); f != nil {
		return f
	}
	return m.base
}

// ---- shaping a line ----

// shapedText is a line shaped: its glyphs placed left to right from the
// pen's start on the baseline, in pixels.
type shapedText struct {
	w        fixed.Int26_6
	glyphs   []placedGlyph
	asc, dsc float32 // how far its glyphs reach above and below the baseline
	runes    int
	rtl      bool      // it reads right to left
	mixed    bool      // it has runs both ways (an Arabic line with English words in it)
	clusters []cluster // left to right
}

// cluster is the glyphs one or more letters became (a conjunct, a
// lam-alef): letters r0..r1 of the line, drawn between x0 and x1.
type cluster struct {
	r0, r1 int
	x0, x1 float32
	rtl    bool
}

type placedGlyph struct {
	face *gtfont.Face
	gid  gtfont.GID
	x, y float32
	size float32
	end  int // the letter its cluster ends before
}

// shaped is s shaped in f's font and size, remembered like a text mask.
func shaped(f font.Face, s string) *shapedText {
	k := maskKey{f, s}
	if st := shapedLines[k]; st != nil {
		return st
	}
	if len(shapedLines) > 400 {
		clear(shapedLines)
	}
	info := faceInfo[f]
	st := shapeLine(embeddedFace(info.name), weightOf(info.name), info.size, s)
	shapedLines[k] = st
	return st
}

func shapeLine(base *gtfont.Face, weight string, size float64, s string) *shapedText {
	st := &shapedText{}
	text := []rune(s)
	if base == nil || len(text) == 0 {
		return st
	}
	dir, bdir := di.DirectionLTR, bidi.LeftToRight
	if rtlText(s) {
		dir, bdir = di.DirectionRTL, bidi.RightToLeft
	}
	// the bidi levels, for putting the runs in visual order
	levels := make([]bidi.Level, len(text))
	brs := bidiPara.Segment(text, bdir)
	for i := 0; i < brs.NumRuns(); i++ {
		run := brs.Run(i)
		for j := run.Start; j < run.End && j < len(text); j++ {
			levels[j] = run.Level
		}
	}
	px := fixed.Int26_6(math.Round(size * 64))
	in := shaping.Input{Text: text, RunStart: 0, RunEnd: len(text), Direction: dir, Size: px, Language: language.NewLanguage("en")}
	inputs := segmenter.Split(in, &lineFonts{base: base, weight: weight})
	outs := make([]shaping.Output, len(inputs))
	lv := make([]bidi.Level, len(inputs))
	for i, ip := range inputs {
		outs[i] = shaper.Shape(ip)
		lv[i] = levels[ip.RunStart]
	}
	// L2: from the highest level down to the lowest odd one, reverse every run of runs at that level or higher
	order := make([]int, len(outs))
	hi, lowOdd := bidi.Level(0), bidi.Level(127)
	for i, l := range lv {
		order[i] = i
		hi = max(hi, l)
		if l%2 == 1 {
			lowOdd = min(lowOdd, l)
		}
	}
	for l := hi; l >= lowOdd && l > 0; l-- {
		for i := 0; i < len(order); {
			if lv[order[i]] < l {
				i++
				continue
			}
			j := i
			for j < len(order) && lv[order[j]] >= l {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			i = j
		}
	}
	var pen fixed.Int26_6
	sz := float32(size)
	st.runes, st.rtl = len(text), dir == di.DirectionRTL
	for _, i := range order {
		o := &outs[i]
		rtl := o.Direction.Progression() == di.TowardTopLeft
		st.mixed = st.mixed || rtl != st.rtl
		for _, g := range o.Glyphs { // a run's glyphs come in visual order, left to right
			gx, gy := pen+g.XOffset, g.YOffset
			st.glyphs = append(st.glyphs, placedGlyph{face: o.Face, gid: g.GlyphID, x: f26(gx), y: -f26(gy), size: sz,
				end: g.TextIndex() + max(g.RunesCount(), 1)})
			st.asc = max(st.asc, f26(g.YBearing+gy))
			st.dsc = max(st.dsc, -f26(g.YBearing+g.Height+gy))
			r0 := g.TextIndex()
			if n := len(st.clusters); n > 0 && st.clusters[n-1].r0 == r0 {
				st.clusters[n-1].x1 = f26(pen + g.Advance)
			} else {
				st.clusters = append(st.clusters, cluster{r0: r0, r1: r0 + max(g.RunesCount(), 1), x0: f26(pen), x1: f26(pen + g.Advance), rtl: rtl})
			}
			pen += g.Advance
		}
	}
	st.w = pen
	return st
}

// shapedSpans is the part of a shaped line drawn from lx lit once sung of
// its letters are sung. A line one way fills like any other, from its
// start (upto -1). A line with runs both ways fills letter by letter in
// reading order, so the English in an Arabic line lights left to right in
// its turn: the letters before upto are lit whole (litMask), the spans are
// the letter being sung.
func shapedSpans(f font.Face, s string, lx, sung float32) (sp []litSpan, upto int) {
	st := shaped(f, s)
	if st.runes == 0 {
		return nil, -1
	}
	if !st.mixed {
		lw := f26(st.w)
		return litSpans(lx, lw, sung/float32(st.runes)*lw, st.rtl), -1
	}
	upto = int(sung)
	for _, c := range st.clusters {
		if float32(c.r1) <= sung || float32(c.r0) >= sung {
			continue
		}
		upto = min(upto, c.r0)
		x0, x1 := lx+c.x0, lx+c.x1
		k := (sung - float32(c.r0)) / float32(c.r1-c.r0) // being sung: lit from where it starts, a soft leading edge
		if c.rtl {
			e := x1 - k*(x1-x0)
			sp = append(sp, litSpan{e, x1, 1}, litSpan{e - 2, e, 0.6}, litSpan{e - 4, e - 2, 0.3})
		} else {
			e := x0 + k*(x1-x0)
			sp = append(sp, litSpan{x0, e, 1}, litSpan{e, e + 2, 0.6}, litSpan{e + 2, e + 4, 0.3})
		}
	}
	return sp, upto
}

// litMask is a shaped line's mask with only the glyphs of its letters
// before upto: a mixed line's sung part, whole glyphs (an Arabic letter's
// tail reaching under the English beside it lights with its letter). A
// line's few are kept as it's sung.
func (c *canvas) litMask(f font.Face, s string, upto int) *textMask {
	k := litKey{maskKey{f, s}, upto}
	if m := c.lit[k]; m != nil {
		return m
	}
	if c.lit == nil || len(c.lit) > 24 {
		c.lit = map[litKey]*textMask{}
	}
	m := shapedMaskOf(f, s, upto)
	c.lit[k] = m
	return m
}

type litKey struct {
	maskKey
	upto int
}

func f26(v fixed.Int26_6) float32 { return float32(v) / 64 }

// shapedMask draws a shaped line into a text mask: the pen starts at 1, the
// baseline at its ascent (Ember's font's, or more if a mark reaches higher).
func shapedMask(f font.Face, s string) *textMask { return shapedMaskOf(f, s, math.MaxInt) }

// shapedMaskOf is shapedMask with only the glyphs of the letters before upto.
func shapedMaskOf(f font.Face, s string, upto int) *textMask {
	st := shaped(f, s)
	met := f.Metrics()
	asc := max(met.Ascent.Ceil(), int(math.Ceil(float64(st.asc)))+1)
	desc := max(met.Descent.Ceil(), int(math.Ceil(float64(st.dsc)))+1)
	w := st.w.Ceil()
	m := image.NewAlpha(image.Rect(0, 0, w+2, asc+desc+1))
	var r vector.Rasterizer
	for _, g := range st.glyphs {
		if g.end <= upto {
			drawGlyph(&r, m, g, 1+g.x, float32(asc)+g.y)
		}
	}
	return &textMask{m: m, w: w, asc: asc}
}

// drawGlyph rasterises g's outline with its origin at (ox, oy) in m.
func drawGlyph(r *vector.Rasterizer, m *image.Alpha, g placedGlyph, ox, oy float32) {
	out, ok := g.face.GlyphDataOutline(g.gid)
	if !ok || len(out.Segments) == 0 {
		return
	}
	k := g.size / float32(g.face.Upem())
	x0, y0, x1, y1 := float32(math.MaxFloat32), float32(math.MaxFloat32), float32(-math.MaxFloat32), float32(-math.MaxFloat32)
	for _, sg := range out.Segments {
		for _, p := range sg.Args[:segArgs(sg.Op)] {
			x, y := ox+p.X*k, oy-p.Y*k
			x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
		}
	}
	b := image.Rect(int(math.Floor(float64(x0))), int(math.Floor(float64(y0))), int(math.Ceil(float64(x1))), int(math.Ceil(float64(y1)))).Intersect(m.Rect)
	if b.Empty() {
		return
	}
	dx, dy := ox-float32(b.Min.X), oy-float32(b.Min.Y)
	r.Reset(b.Dx(), b.Dy())
	started := false
	for _, sg := range out.Segments {
		a := sg.Args
		switch sg.Op {
		case ot.SegmentOpMoveTo:
			if started {
				r.ClosePath()
			}
			started = true
			r.MoveTo(dx+a[0].X*k, dy-a[0].Y*k)
		case ot.SegmentOpLineTo:
			r.LineTo(dx+a[0].X*k, dy-a[0].Y*k)
		case ot.SegmentOpQuadTo:
			r.QuadTo(dx+a[0].X*k, dy-a[0].Y*k, dx+a[1].X*k, dy-a[1].Y*k)
		case ot.SegmentOpCubeTo:
			r.CubeTo(dx+a[0].X*k, dy-a[0].Y*k, dx+a[1].X*k, dy-a[1].Y*k, dx+a[2].X*k, dy-a[2].Y*k)
		}
	}
	r.ClosePath()
	r.Draw(m, b, image.Opaque, image.Point{})
}

func segArgs(op ot.SegmentOp) int {
	switch op {
	case ot.SegmentOpQuadTo:
		return 2
	case ot.SegmentOpCubeTo:
		return 3
	}
	return 1
}
