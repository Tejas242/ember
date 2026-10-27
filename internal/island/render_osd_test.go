//go:build render

package island

import (
	"fmt"
	"image"
	"image/draw"
	"os"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// EMBER_PREVIEW=dir go test -tags render -count=1 -run OSD ./internal/island
//
// A volume key pressed in every state the island can be in, 400 ms after the
// press (the OSD is up): osd-cases.png. Each shot is what you'd see on
// screen, so a state where the change doesn't show looks like nothing
// happened.

type osdCase struct {
	label string
	setup func(t *testing.T, g *galleryScene)
	build func(t *testing.T) *galleryScene // a scene made its own way (a stage)
	// unheard: a change Ember didn't hear before; EMBER_OSD_BEFORE=1 leaves
	// it out, to picture what you saw then (nothing)
	unheard bool
	press   func(g *galleryScene) // the change; a volume key by default
	crop    func(u *UI) image.Rectangle
}

func pressVolume(g *galleryScene) { g.a.handle(source.Volume{V: 0.64, OK: true}) }

func osdPillCrop(*UI) image.Rectangle { return image.Rect(surfW/2-230, 0, surfW/2+230, 46) }

// osdTopCrop: the top of an open view, where the change would show.
func osdTopCrop(u *UI) image.Rectangle {
	b := u.shape.bounds()
	cx := (b.Min.X + b.Max.X) / 2
	hw := max(235, (b.Dx()+28)/2)
	return image.Rect(cx-hw, 0, cx+hw, min(b.Max.Y+16, 150))
}

func osdCases() []osdCase {
	return []osdCase{
		{label: "pill (works today)", setup: func(t *testing.T, g *galleryScene) {}, crop: osdPillCrop},
		{label: "pill, Claude needs you", setup: func(t *testing.T, g *galleryScene) {
			g.work("k", "kinvault", "fix the race", 2)
			hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
		}, crop: osdPillCrop},
		{label: "pill, asleep", setup: func(t *testing.T, g *galleryScene) {
			g.a.ui.mind.asleep, g.a.ui.mind.arousal, g.a.ui.mind.sleepAt = true, 0.08, g.clock.Add(-time.Hour)
			g.a.ui.mind.lastEvent = g.clock.Add(-time.Hour)
		}, crop: osdPillCrop},
		{label: "pill, screen shared (discreet)", setup: func(t *testing.T, g *galleryScene) {
			g.a.castsChanged(1)
			g.a.ui.moment = nil
		}, crop: osdPillCrop},
		{label: "Done arrives while you press", setup: func(t *testing.T, g *galleryScene) {
			g.work("k", "kinvault", "fix the race", 3)
		}, press: func(g *galleryScene) {
			pressVolume(g)
			g.run(150 * time.Millisecond)
			hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "All 42 tests pass."})
		}, crop: osdPillCrop},
		{label: "microphone level (Ember never heard it)", setup: func(t *testing.T, g *galleryScene) {}, unheard: true,
			press: func(g *galleryScene) { g.a.handle(source.Mic{V: 0.45, Level: true}) }, crop: osdPillCrop},
		{label: "card open, scrolling the music row (the player's volume)", setup: func(t *testing.T, g *galleryScene) {
			g.music(true)
			g.a.ui.pinned = true
		}, unheard: true, press: func(g *galleryScene) { g.a.playerVolume(0.35) }, crop: osdTopCrop},
		{label: "card open", setup: func(t *testing.T, g *galleryScene) {
			g.work("k", "kinvault", "fix the race in the store's compaction path and make the tests pass", 4)
			g.music(true)
			g.a.ui.pinned = true
		}, crop: osdTopCrop},
		{label: "a request (Claude asks)", setup: func(t *testing.T, g *galleryScene) {
			g.work("k", "kinvault", "fix the race", 2)
			askAt(g.a, "PermissionRequest", "k", "kinvault", map[string]any{"tool_name": "Bash",
				"tool_input": map[string]any{"command": "go test -race ./store/... -count=1", "description": "Run the store tests under the race detector"}})
		}, crop: osdTopCrop},
		{label: "edge's dashboard (the gate lock)", setup: func(t *testing.T, g *galleryScene) {
			st := edgeFixture()
			st.Today.Gate, st.Today.GateWhy = "closed", ""
			edgeSays(g.a.ui, nil, st)
			g.run(500 * time.Millisecond)
			g.a.ui.openDash("edge")
		}, crop: osdTopCrop},
		{label: "vibe stage", build: func(t *testing.T) *galleryScene {
			g, _ := buildVibe(t, vibeScenes()[0])
			return g
		}, crop: osdTopCrop},
	}
}

func renderOSDCase(t *testing.T, c osdCase) galleryShot {
	var g *galleryScene
	if c.build != nil {
		g = c.build(t)
	} else {
		g = newScene()
		c.setup(t, g)
	}
	g.run(2500 * time.Millisecond)
	press := c.press
	if press == nil {
		press = pressVolume
	}
	if !c.unheard || os.Getenv("EMBER_OSD_BEFORE") == "" {
		press(g)
	}
	g.run(400 * time.Millisecond)
	return galleryShot{c.label, onDesktop(g.a.ui, c.crop(g.a.ui))}
}

func TestOSD(t *testing.T) {
	if os.Getenv("EMBER_PREVIEW") == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	t.Setenv("HOME", t.TempDir()) // the default palette
	var shots []galleryShot
	for _, c := range osdCases() {
		shots = append(shots, renderOSDCase(t, c))
	}
	shots = append(shots, galleryShot{"fullscreen video, a volume key", fullscreenPress(t, 400*time.Millisecond)})
	sheet(t, "osd-cases", shots, 2)
}

// fullscreenPress: a video fullscreen, music playing and Claude at work
// behind it; a volume key, and what's on screen d later.
func fullscreenPress(t *testing.T, d time.Duration) *image.RGBA {
	g := newScene()
	g.work("k", "kinvault", "fix the race", 2)
	g.music(true)
	goFullscreen(g, 768)
	g.run(time.Second)
	g.a.handle(source.Volume{V: 0.64, OK: true})
	g.run(d)
	u := g.a.ui
	if !u.wantOverlay() || os.Getenv("EMBER_OSD_BEFORE") != "" { // on the top layer: under the window
		return fullscreenShot(nil)
	}
	return fullscreenShot(u)
}

// TestOSDMotion: filmstrips of the strip dropping into an open card and of
// the OSD over a fullscreen window, from the press to after it's gone.
func TestOSDMotion(t *testing.T) {
	if os.Getenv("EMBER_PREVIEW") == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	t.Setenv("HOME", t.TempDir())
	at := []time.Duration{0, 50, 100, 180, 600, 1480, 1540, 1600, 1700}
	var card, full []galleryShot
	for _, ms := range at {
		d := ms * time.Millisecond
		g := newScene()
		g.work("k", "kinvault", "fix the race in the store's compaction path and make the tests pass", 4)
		g.a.ui.pinned = true
		g.run(2500 * time.Millisecond)
		g.a.handle(source.Volume{V: 0.64, OK: true})
		g.run(d)
		g.a.ui.expire()
		card = append(card, galleryShot{fmt.Sprintf("+%dms", ms), onDesktop(g.a.ui, image.Rect(surfW/2-expW/2-14, 0, surfW/2+expW/2+14, 70))})
		full = append(full, galleryShot{fmt.Sprintf("+%dms", ms), fullscreenPressExpiring(d)})
	}
	sheet(t, "osd-motion-card", card, 3)
	sheet(t, "osd-motion-fullscreen", full, 3)
}

func fullscreenPressExpiring(d time.Duration) *image.RGBA {
	g := newScene()
	g.work("k", "kinvault", "fix the race", 2)
	g.music(true)
	goFullscreen(g, 768)
	g.run(time.Second)
	g.a.handle(source.Volume{V: 0.64, OK: true})
	for end := g.clock.Add(d); g.clock.Before(end); {
		g.run(time.Second / 60)
		g.a.ui.expire()
	}
	if !g.a.ui.wantOverlay() {
		return fullscreenShot(nil)
	}
	return fullscreenShot(g.a.ui)
}

// fullscreenFrame stands in for a fullscreen video: no bar, a sunset.
func fullscreenFrame() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, surfW, 46))
	for y := 0; y < 46; y++ {
		for x := 0; x < surfW; x++ {
			img.Pix[img.PixOffset(x, y)+0] = uint8(150 + x/6)
			img.Pix[img.PixOffset(x, y)+1] = uint8(90 + y + x/12)
			img.Pix[img.PixOffset(x, y)+2] = uint8(70 + x/10)
			img.Pix[img.PixOffset(x, y)+3] = 255
		}
	}
	return img
}

// fullscreenShot: a fullscreen window, and the island over it if it's on a
// layer above it (u nil: it isn't).
func fullscreenShot(u *UI) *image.RGBA {
	out := fullscreenFrame()
	if u != nil {
		for _, r := range u.shape.blurRects() {
			boxBlur(out, r.Inset(-1).Intersect(out.Rect), 9)
		}
		draw.Draw(out, out.Rect, u.cv.img, image.Point{}, draw.Over)
	}
	return out.SubImage(image.Rect(surfW/2-230, 0, surfW/2+230, 46)).(*image.RGBA)
}
