//go:build render

package island

import (
	"image"
	"os"
	"strconv"
	"testing"
	"time"
)

// EMBER_PREVIEW=dir go test -tags render -count=1 -run PreviewPressable ./internal/island:
// an agenda row that carries a key, at rest, under the pointer and held;
// and a plugin's card written in paragraphs.
func TestPreviewPressable(t *testing.T) {
	if os.Getenv("EMBER_PREVIEW") == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	t.Setenv("HOME", t.TempDir())
	page := func(label string, act func(g *galleryScene, cx, cy float64)) galleryShot {
		g := newScene()
		at := func(d time.Duration) string { return strconv.FormatInt(g.clock.Add(d).UnixMilli(), 10) }
		g.a.applyExt("edge", extMsg(t, `{"type":"dashboard","title":"edge","widgets":[
			{"kind":"agenda","title":"Coming","day_ends":"04:00","items":[
				{"title":"CF 1129 upsolves lapse","at_ms":`+at(30*time.Hour)+`,"tag":"2 owed","tag_mood":"nudge","key":"upsolve:cf:2261"},
				{"title":"ABC 482","at_ms":`+at(52*time.Hour)+`,"tag":"held","tag_mood":"quiet"}]},
			{"kind":"list","title":"Recent","items":[{"title":"2277C","sub":"WA on test 3","mood":"fail","icon":"warning","key":"open:cf:2277C"},{"title":"2270B","sub":"accepted","mood":"done","icon":"check"}]}]}`))
		g.a.ui.openDash("edge")
		g.run(2500 * time.Millisecond)
		var r image.Rectangle
		for _, ht := range g.a.ui.hits {
			if ht.key == "x:dbtn:edge\x00upsolve:cf:2261" {
				r = ht.r
			}
		}
		if r.Empty() {
			t.Fatal("no pressable row")
		}
		if act != nil {
			act(g, float64(r.Min.X+r.Dx()/2), float64(r.Min.Y+r.Dy()/2))
		}
		_, h := extDash{name: "edge"}.Size(g.a.ui)
		return galleryShot{label, onDesktop(g.a.ui, image.Rect(surfW/2-290, 0, surfW/2+290, min(surfH, int(h)+20)))}
	}
	sheet(t, "plugins-pressable", []galleryShot{
		page("a pressable row, at rest", nil),
		page("under the pointer", func(g *galleryScene, x, y float64) {
			g.a.ui.pointer(pointerEvent{kind: "enter", x: x, y: y})
			g.a.ui.pointer(pointerEvent{kind: "motion", x: x, y: y})
			g.run(600 * time.Millisecond)
		}),
		page("held", func(g *galleryScene, x, y float64) {
			g.a.ui.pointer(pointerEvent{kind: "enter", x: x, y: y})
			g.a.ui.pointer(pointerEvent{kind: "motion", x: x, y: y})
			g.run(600 * time.Millisecond)
			g.a.ui.pointer(pointerEvent{kind: "press", x: x, y: y, button: 1})
			g.run(300 * time.Millisecond)
		}),
	}, 1)

	g := newScene()
	g.a.applyExt("edge", extMsg(t, `{"type":"card","id":"giveup:cf:2277C","eyebrow":"edge","title":"2277C · 45 min in","body":"WA on test 3, twice.\nGive up and learn it, or keep going: either counts.\nThe hint ladder is spent.","mood":"nudge","ttl_s":600,"buttons":[{"key":"keep","label":"Keep going"},{"key":"giveup:cf:2277C","label":"Learn it","primary":true}]}`))
	g.a.ui.pinned = true
	g.run(2500 * time.Millisecond)
	h := g.a.ui.cardHeight(g.a.ui.top())
	sheet(t, "plugins-card-paragraphs", []galleryShot{{"a card in paragraphs", onDesktop(g.a.ui, image.Rect(surfW/2-210, 0, surfW/2+210, min(surfH, h+24)))}}, 1)
}
