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

// goFullscreen plays niri's events for one window filling the 1366×768
// output (h 768), or stopping at the bar (h 740).
func goFullscreen(g *galleryScene, h int) {
	g.a.handle(source.NiriWorkspaces{Outputs: map[int]string{2: "eDP-1"}})
	g.a.handle(source.NiriOutputs{Sizes: map[string][2]int{"eDP-1": {1366, 768}}})
	g.a.handle(source.NiriWindows{Windows: []source.Window{{ID: 7, Focused: true, Workspace: 2, Layout: source.WindowLayout{Size: [2]int{1366, h}}}}})
}

// Over a fullscreen window Ember folds away and rests; a key brings it up
// over the window with the OSD alone, and it goes back under after.
func TestOSDOverFullscreen(t *testing.T) {
	g := newScene()
	g.work("k", "kinvault", "fix the race", 2)
	g.music(true)
	goFullscreen(g, 740) // maximized: the bar's still there
	if g.a.ui.full {
		t.Fatal("a maximized window isn't fullscreen")
	}
	goFullscreen(g, 768)
	u := g.a.ui
	if !u.full {
		t.Fatal("a window as big as its output is fullscreen")
	}
	u.AddCard(&Card{ID: "p", Kind: "perm", Mood: moodWait, Title: "Run the tests", Buttons: []Button{{Key: "allow", Label: "Allow", Primary: true}}})
	g.run(time.Second)
	if u.wantOverlay() || u.sh.x > 0.5 || u.continuous() {
		t.Fatalf("under a fullscreen window it should fold away and rest (h %.1f, overlay %v)", u.sh.x, u.overlay)
	}
	g.a.handle(source.Volume{V: 0.4, OK: true})
	if !u.wantOverlay() {
		t.Fatal("a key should bring it up over the fullscreen window")
	}
	g.run(400 * time.Millisecond)
	if u.lastView != vCompact || u.sh.x < float64(u.BarH)-1 || u.secondary(u.primary()) != "" {
		t.Fatalf("over the window: the pill's OSD alone (view %v, h %.1f)", u.lastView, u.sh.x)
	}
	g.run(u.cfg.OSDFor)
	u.expire()
	g.run(time.Second)
	if u.wantOverlay() || u.sh.x > 0.5 {
		t.Fatal("after the OSD it should fold away and go back under")
	}
	g.a.handle(source.NiriLayouts{Layouts: map[int]source.WindowLayout{7: {Size: [2]int{674, 734}}}})
	if u.full {
		t.Fatal("the window left fullscreen")
	}
	g.run(time.Second)
	if u.lastView != vCard || u.sh.x < 40 {
		t.Fatalf("the request should be back once the window isn't fullscreen (view %v)", u.lastView)
	}
}
