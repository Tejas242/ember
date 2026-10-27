package island

import (
	"image"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// A volume key pressed with the card open drops its OSD along the card's
// top, and when the OSD is done the card is as it was.
func TestOSDStripInOpenCard(t *testing.T) {
	scene := func(press bool) *galleryScene {
		g := newScene()
		g.work("k", "kinvault", "fix the race in the store's compaction path", 3)
		g.a.ui.pinned = true
		g.run(2 * time.Second)
		if press {
			g.a.handle(source.Volume{V: 0.6, OK: true})
		}
		return g
	}
	ref, g := scene(false), scene(true)
	band := func(g *galleryScene) *image.RGBA {
		u := g.a.ui
		cx := surfW / 2
		r := image.Rect(cx-60, 0, cx+60, int(u.BarH)+4)
		out := image.NewRGBA(r)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			copy(out.Pix[(y-r.Min.Y)*out.Stride:], u.cv.img.Pix[y*u.cv.img.Stride+r.Min.X*4:y*u.cv.img.Stride+r.Max.X*4])
		}
		return out
	}
	same := func(a, b *image.RGBA) bool { return string(a.Pix) == string(b.Pix) }
	ref.run(400 * time.Millisecond)
	g.run(400 * time.Millisecond)
	if g.a.ui.currentView() != vExpanded {
		t.Fatalf("the card closed on a key press")
	}
	if same(band(ref), band(g)) {
		t.Fatal("the OSD doesn't show in the open card")
	}
	if !g.a.ui.stripMoving(g.clock.Add(-300*time.Millisecond)) || g.a.ui.stripMoving(g.clock) {
		t.Fatal("the strip should be steady while it holds")
	}
	d := g.a.ui.cfg.OSDFor
	ref.run(d)
	g.run(d)
	g.a.ui.expire()
	ref.a.ui.expire()
	g.a.ui.Invalidate()
	ref.a.ui.Invalidate()
	ref.run(300 * time.Millisecond)
	g.run(300 * time.Millisecond)
	if !same(band(ref), band(g)) {
		t.Fatal("the card's top isn't back after the OSD")
	}
}

// Claude finishing while you press a volume key doesn't take the OSD from
// under your finger: the Done waits, then shows (and Ember cheers it then).
func TestNewsWaitsUnderTheOSD(t *testing.T) {
	g := newScene()
	g.work("k", "kinvault", "fix the race", 3)
	g.run(time.Second)
	g.a.handle(source.Volume{V: 0.6, OK: true})
	g.run(150 * time.Millisecond)
	hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "All 42 tests pass."})
	if m := g.a.ui.moment; m == nil || m.key != "osd:volume" {
		t.Fatalf("the Done took the OSD's place: %+v", m)
	}
	if u := g.a.ui.under; u == nil || u.verb != "Done" || !u.held {
		t.Fatalf("the Done should wait under the OSD, got %+v", u)
	}
	g.clock = g.clock.Add(g.a.ui.cfg.OSDFor)
	g.a.ui.expire()
	m := g.a.ui.moment
	if m == nil || m.verb != "Done" || m.held || !m.at.Equal(g.clock) {
		t.Fatalf("the Done should show once the OSD is done, got %+v", m)
	}
	if !m.until.After(g.clock.Add(time.Second)) {
		t.Fatal("the Done should stay long enough to read")
	}
}
