package island

import (
	"testing"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
	"github.com/go-text/typesetting/language"
)

// needScript skips a test when this machine has no font for r's script.
func needScript(t testing.TB, r rune) {
	t.Helper()
	if scriptFace(language.LookupScript(r), "semibold", r) == nil {
		t.Skipf("no font with %U", r)
	}
}

func TestNeedsShaping(t *testing.T) {
	for s, want := range map[string]bool{
		"Turn the radio up": false, "夜の窓に 小さな火": false, "𝙂𝘼𝙄𝙎𝙀 — café": false,
		"चाय की खुशबू": true, "سلام": true, "שלום": true, "the radio · नमस्ते": true,
	} {
		if needsShaping(s) != want {
			t.Errorf("%q: %v", s, !want)
		}
	}
	for s, want := range map[string]bool{
		"Turn the radio up": false, "चाय": false, "سلام عليكم": true, "שלום": true,
		"on repeat قلبي": false, "123 قلبي on repeat": true, "« سلام »": true,
	} {
		if rtlText(s) != want {
			t.Errorf("rtl %q: %v", s, !want)
		}
	}
}

// Devanagari: the vowel sign i is drawn before the consonant it follows,
// and k + virama + ss fuse into one conjunct.
func TestShapeDevanagari(t *testing.T) {
	needScript(t, 'क')
	f := face(fSemiBold, 17)
	st := shaped(f, "कि")
	if len(st.glyphs) != 2 || st.rtl {
		t.Fatalf("कि: %d glyphs, rtl %v", len(st.glyphs), st.rtl)
	}
	ka, _ := st.glyphs[1].face.NominalGlyph('क')
	if st.glyphs[1].gid != ka || st.glyphs[0].gid == ka || st.glyphs[0].x >= st.glyphs[1].x {
		t.Fatalf("the i sign isn't first: %+v (ka %d)", st.glyphs, ka)
	}
	if st := shaped(f, "क्ष"); len(st.glyphs) != 1 || st.runes != 3 {
		t.Fatalf("क्ष: %d glyphs for %d letters, want one conjunct", len(st.glyphs), st.runes)
	}
}

// Arabic: the letters join (initial, medial, final forms), right to left.
func TestShapeArabic(t *testing.T) {
	needScript(t, 'س')
	f := face(fSemiBold, 17)
	st := shaped(f, "سلام")
	if len(st.glyphs) != 4 || !st.rtl || st.mixed {
		t.Fatalf("%d glyphs, rtl %v, mixed %v", len(st.glyphs), st.rtl, st.mixed)
	}
	// left to right on screen: meem (alone: alef doesn't join to its left), alef (final), lam (medial), seen (initial)
	for i, c := range []struct {
		r      rune
		joined bool
	}{{'م', false}, {'ا', true}, {'ل', true}, {'س', true}} {
		g := st.glyphs[i]
		nom, _ := g.face.NominalGlyph(c.r)
		if (g.gid != nom) != c.joined {
			t.Errorf("glyph %d (%c): gid %d, isolated %d, want joined %v", i, c.r, g.gid, nom, c.joined)
		}
		if i > 0 && g.x <= st.glyphs[i-1].x {
			t.Errorf("glyph %d not right of %d", i, i-1)
		}
	}
}

// An Arabic line with English in it: the English reads left to right in
// its place, and the fill reaches it in reading order.
func TestShapeMixed(t *testing.T) {
	needScript(t, 'ق')
	f := face(fSemiBold, 17)
	s := "قلبي يغني on repeat"
	st := shaped(f, s)
	if !st.rtl || !st.mixed {
		t.Fatalf("rtl %v mixed %v", st.rtl, st.mixed)
	}
	at := func(r int) cluster {
		for _, c := range st.clusters {
			if c.r0 <= r && r < c.r1 {
				return c
			}
		}
		t.Fatalf("no cluster for letter %d", r)
		return cluster{}
	}
	o, tt, qaf := at(10), at(18), at(0) // "o" of on, the last "t", the first letter
	if !(o.x1 <= tt.x0 && tt.x1 <= qaf.x0) {
		t.Fatalf("visual order: o %v, t %v, qaf %v", o, tt, qaf)
	}
	sp, upto := shapedSpans(f, s, 0, 11.5) // the Arabic and the "o" sung, the "n" being sung
	if upto != 11 || len(sp) == 0 || sp[0].x0 < at(11).x0 || sp[0].x1 > at(11).x1 {
		t.Fatalf("filled out of reading order: upto %d, %+v (n at %+v)", upto, sp, at(11))
	}
	cv := newCanvas(400, 40)
	litM, all := cv.litMask(f, s, upto), cv.mask(f, s)
	ink := func(m *textMask, c cluster) (n int) {
		for y := 0; y < m.m.Rect.Dy(); y++ {
			for x := int(c.x0) + 2; x < int(c.x1); x++ {
				n += int(m.m.AlphaAt(x, y).A)
			}
		}
		return n
	}
	if ink(litM, qaf) != ink(all, qaf) || ink(litM, o) != ink(all, o) || ink(litM, tt) != 0 || ink(all, tt) == 0 {
		t.Fatal("the sung glyphs aren't the ones lit")
	}
	if cv.litMask(f, s, upto) != litM {
		t.Fatal("lit again")
	}
}

// A line read right to left fills from the right; others from the left.
func TestLitSpansRTL(t *testing.T) {
	ltr := litSpans(100, 50, 10, false)
	if ltr[0].x0 != 98 || ltr[0].x1 != 110 || ltr[1].x0 != 110 {
		t.Fatalf("ltr %+v", ltr)
	}
	rtl := litSpans(100, 50, 10, true)
	if rtl[0].x0 != 140 || rtl[0].x1 != 152 || rtl[1].x1 != 140 || rtl[2].x0 != 136 {
		t.Fatalf("rtl %+v", rtl)
	}
	if done := litSpans(100, 50, 60, true); len(done) != 1 || done[0].x0 != 100 {
		t.Fatalf("sung through: %+v", done)
	}
}

// A shaped line is shaped once and its mask drawn once; frames reuse them.
func TestShapedCached(t *testing.T) {
	needScript(t, 'क')
	f := face(fSemiBold, 17)
	s := "खिड़की पर बारिश धीरे गाती है"
	cv := newCanvas(400, 40)
	st, m := shaped(f, s), cv.mask(f, s)
	if again, m2 := shaped(f, s), cv.mask(f, s); again != st || m2 != m {
		t.Fatal("shaped again")
	}
	if w := measure(f, s); w != float32(shaped(f, s).w.Ceil()) || w != float32(cv.mask(f, s).w) {
		t.Fatalf("width %v, shaped %v", w, shaped(f, s).w.Ceil())
	}
	if n := testing.AllocsPerRun(50, func() { measure(f, s); cv.mask(f, s) }); n != 0 {
		t.Fatalf("%v allocations a frame", n)
	}
}

// A right-to-left line on stage: against the column's right edge, the words
// before it too; a wrapped piece starting with English keeps the line's direction.
func TestStageRTL(t *testing.T) {
	needScript(t, 'ل')
	lines := []proto.TimedLine{{TMs: 10000, Text: "الليل طويل والأغاني قريبة"}, {TMs: 14000, Text: "قلبي يغني on repeat طول الليل"}}
	g := stageScene(t, lines, mediaKey, "synced", "ready")
	st, _ := stageAt(g, 15*time.Second)
	if st.idx != 1 || st.fillK <= 0 {
		t.Fatalf("line %d fill %.2f", st.idx, st.fillK)
	}
	f := fCaption()
	if x := lineX(f, lines[0].Text, 100, 200); x <= 100 || x+measure(f, lines[0].Text) != 300 {
		t.Fatalf("not against the right edge: %v", x)
	}
	if lineX(f, "the kettle sings", 100, 200) != 100 {
		t.Fatal("a Latin line moved")
	}
	if keepDir("on repeat طول", true) == "on repeat طول" || !rtlText(keepDir("on repeat طول", true)) || keepDir("on repeat", false) != "on repeat" {
		t.Fatal("a piece of a right-to-left line turned around")
	}
}

var hindiTestLines = []proto.TimedLine{
	{TMs: 10000, Text: "चाय की खुशबू में सुबह जागी"},
	{TMs: 14000, Text: "खिड़की पर बारिश धीरे गाती है"},
	{TMs: 18000, Text: "हम छत पर नाचे सुबह तक"},
	{TMs: 22000, Text: "शहर ने फिर भी नहीं सुना"},
}

// BenchmarkStageSteadyHindi and BenchmarkStageSingingHindi: the stage's
// benchmarks with Hindi words.
func BenchmarkStageSteadyHindi(b *testing.B) {
	needScript(b, 'क')
	benchStage(b, hindiTestLines, 0)
}

func BenchmarkStageSingingHindi(b *testing.B) {
	needScript(b, 'क')
	benchStage(b, hindiTestLines, 14500*time.Millisecond)
}
