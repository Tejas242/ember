package island

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

// EMBER_MASCOT=dir [EMBER_BG=screenshot.png] go test -run MascotPreview ./island
//
// Ember up close: every pill state at its real 1× pixels (enlarged without
// smoothing, so what you see is what the screen gets), and each act as a
// sequence of frames at 60 fps (dir/<act>/0000.png …) for an animation.

type mascotScene struct {
	name string
	set  func(g *galleryScene)
	run  time.Duration
}

func mascotScenes() []mascotScene {
	vol := func(level float64) func(g *galleryScene) {
		return func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "osd:volume", icon: "volume", tint: g.a.ui.pal.fg, meter: true, level: level, right: fmt.Sprint(int(level * 100)), until: g.clock.Add(time.Hour), osd: true})
		}
	}
	return []mascotScene{
		{"quiet", func(g *galleryScene) {}, 2500 * time.Millisecond},
		{"asleep", func(g *galleryScene) {
			g.a.ui.mind.asleep, g.a.ui.mind.arousal, g.a.ui.mind.sleepAt = true, 0.08, g.clock.Add(-time.Hour)
			g.a.ui.mind.lastEvent = g.clock.Add(-time.Hour)
		}, 2500 * time.Millisecond},
		{"working", func(g *galleryScene) { g.work("k", "kinvault", "fix the race", 5) }, 2500 * time.Millisecond},
		{"needs you", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the race", 2)
			hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
		}, 2500 * time.Millisecond},
		{"done", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "done:k", act: "claude", mood: moodHappy, verb: "Done", text: "kinvault", sub: "All tests pass", until: g.clock.Add(time.Hour)})
		}, 2500 * time.Millisecond},
		{"stopped", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "fail:k", act: "claude", mood: moodError, verb: "Stopped", text: "kinvault", sub: "Rate limited", until: g.clock.Add(time.Hour)})
		}, 2500 * time.Millisecond},
		{"gone quiet", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the race", 3)
			g.clock = g.clock.Add(4 * time.Minute)
		}, 2500 * time.Millisecond},
		{"music", func(g *galleryScene) { g.music(true) }, 2500 * time.Millisecond},
		{"volume 64", vol(0.64), 2500 * time.Millisecond},
		{"charging", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "power", icon: "bolt", tint: g.a.ui.pal.green, text: "Charging", sub: "full in 40m", batt: true, pct: 0.73, until: g.clock.Add(time.Hour)})
		}, 2500 * time.Millisecond},
		{"headphones", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "device:x", icon: "headphones", tint: g.a.ui.pal.blue, text: "WH-1000XM4", sub: "Connected", right: "80%", batt: true, pct: 0.8, until: g.clock.Add(time.Hour)})
		}, 2500 * time.Millisecond},
		{"edge round live", func(g *galleryScene) {
			now := g.clock
			g.a.ui.edge = &EdgeStatus{Today: 1, Need: 2, Streak: 4, Round: &EdgeRound{ID: "r", Short: "CF 1128", StartMs: now.Add(-30 * time.Minute).UnixMilli(),
				EndMs: now.Add(90 * time.Minute).UnixMilli(), Joined: "live", Locked: true, Problems: []EdgeProblem{{Index: "A", State: "solved"}, {Index: "B", State: "tried"}, {Index: "C"}}}}
		}, 2500 * time.Millisecond},
	}
}

// upscale enlarges img n times, nearest neighbour: every screen pixel a block.
func upscale(img image.Image, n int) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx()*n, b.Dy()*n))
	xdraw.NearestNeighbor.Scale(out, out.Rect, img, b, xdraw.Src, nil)
	return out
}

func savePNG(t *testing.T, path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, img)
	f.Close()
}

// emberCrop is the pill around Ember at 1×.
func emberCrop(u *UI, w int) image.Rectangle {
	x := int(u.shape.x)
	return image.Rect(x-6, 0, x+w, 34)
}

func TestMascotPreview(t *testing.T) {
	dir := os.Getenv("EMBER_MASCOT")
	if dir == "" {
		t.Skip("set EMBER_MASCOT")
	}
	if os.Getenv("EMBER_BG") == "" {
		t.Setenv("HOME", t.TempDir())
	}
	// the states: the whole pill at 2× above, Ember's corner at 6× below
	var shots []galleryShot
	for _, sc := range mascotScenes() {
		g := newScene()
		sc.set(g)
		g.run(sc.run)
		u := g.a.ui
		full := onDesktop(u, image.Rect(int(u.shape.x)-12, 0, int(u.shape.x+u.shape.w)+12, 40))
		near := upscale(onDesktop(u, emberCrop(u, 64)), 6)
		c := image.NewRGBA(image.Rect(0, 0, max(full.Rect.Dx()*2, near.Rect.Dx()), 80+near.Rect.Dy()))
		draw.Draw(c, c.Rect, image.NewUniform(color.NRGBA{0x10, 0x12, 0x10, 255}), image.Point{}, draw.Src)
		xdraw.CatmullRom.Scale(c, image.Rect(0, 0, full.Rect.Dx()*2, 80), full, full.Rect, xdraw.Src, nil)
		draw.Draw(c, near.Rect.Add(image.Pt(0, 80)), near, image.Point{}, draw.Src)
		shots = append(shots, galleryShot{sc.name, c})
	}
	mascotSheet(t, filepath.Join(dir, "states.png"), shots, 3)

	// the acts, frame by frame
	for _, f := range mascotFilms() {
		g := newScene()
		f.set(g)
		g.run(f.settle)
		sub := filepath.Join(dir, f.name)
		os.MkdirAll(sub, 0o755)
		f.act(g)
		for i := 0; i < f.frames; i++ {
			g.run(time.Second / 60)
			u := g.a.ui
			if f.each != nil {
				f.each(g, i)
			}
			w := f.w
			if w == 0 {
				w = 120
			}
			crop := emberCrop(u, w)
			if f.whole {
				crop = image.Rect(int(u.shape.x)-14, 0, int(u.shape.x+u.shape.w)+14, 40)
			}
			img := onDesktop(u, crop)
			savePNG(t, filepath.Join(sub, fmt.Sprintf("%04d.png", i)), upscale(img, f.zoom))
		}
	}
}

type mascotFilm struct {
	name   string
	set    func(g *galleryScene)
	settle time.Duration
	act    func(g *galleryScene)
	each   func(g *galleryScene, i int)
	frames int
	zoom   int
	w      int
	whole  bool
}

func mascotFilms() []mascotFilm {
	play := func(kind string, a float64) func(g *galleryScene) {
		return func(g *galleryScene) { g.a.ui.play(kind, a) }
	}
	none := func(g *galleryScene) {}
	working := func(g *galleryScene) { g.work("k", "kinvault", "fix the race", 3) }
	return []mascotFilm{
		{name: "ignite", set: none, settle: time.Second, act: func(g *galleryScene) {
			hookAt(g.a, "UserPromptSubmit", "k", "kinvault", map[string]any{"prompt": "fix the race"})
		}, frames: 60, zoom: 4},
		{name: "needs", set: working, settle: time.Second, act: func(g *galleryScene) {
			hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
		}, frames: 120, zoom: 4},
		{name: "done", set: working, settle: time.Second, act: func(g *galleryScene) {
			g.clock = g.clock.Add(5 * time.Minute)
			hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "All 42 tests pass under -race."})
		}, frames: 100, zoom: 4, whole: true},
		{name: "stopped", set: working, settle: time.Second, act: func(g *galleryScene) {
			hookAt(g.a, "StopFailure", "k", "kinvault", map[string]any{"error": "rate_limit"})
		}, frames: 90, zoom: 4},
		{name: "work", set: working, settle: time.Second, act: none, each: func(g *galleryScene, i int) {
			if i%20 == 0 {
				hookAt(g.a, "PreToolUse", "k", "kinvault", map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/a.go"}})
			}
		}, frames: 120, zoom: 4},
		{name: "nod", set: working, settle: time.Second, act: play("nod", 0), frames: 40, zoom: 4},
		{name: "shake", set: working, settle: time.Second, act: play("shake", 0), frames: 50, zoom: 4},
		{name: "sleep", set: none, settle: time.Second, act: func(g *galleryScene) {
			m := g.a.ui.mind
			m.arousal, m.lastEvent = 0.1, g.clock.Add(-time.Hour)
			m.asleep, m.sleepAt = true, g.clock
			g.a.ui.play("sleep", 0)
		}, frames: 170, zoom: 4},
		{name: "wake", set: func(g *galleryScene) {
			m := g.a.ui.mind
			m.asleep, m.arousal, m.sleepAt, m.lastEvent = true, 0.08, g.clock.Add(-time.Hour), g.clock.Add(-time.Hour)
		}, settle: 2 * time.Second, act: func(g *galleryScene) {
			g.a.ui.mind.asleep = false
			g.a.ui.play("wake", 0)
		}, frames: 70, zoom: 4},
		{name: "volume", set: none, settle: time.Second, act: none, each: func(g *galleryScene, i int) {
			lv := map[int]float64{0: 0.35, 50: 0.6, 70: 0.85, 90: 0.4}
			if v, ok := lv[i]; ok {
				g.a.ui.Moment(moment{key: "osd:volume", icon: "volume", tint: g.a.ui.pal.fg, meter: true, level: v, right: fmt.Sprint(int(v * 100)), until: g.clock.Add(1200 * time.Millisecond), osd: true})
			}
		}, frames: 200, zoom: 3, whole: true},
		{name: "music", set: none, settle: time.Second, act: func(g *galleryScene) { g.music(true) }, frames: 120, zoom: 3, whole: true},
		{name: "poke", set: none, settle: time.Second, act: play("poke", 0), frames: 45, zoom: 4},
		{name: "stretch", set: none, settle: time.Second, act: play("stretch", 0), frames: 120, zoom: 4},
		{name: "eyeroll", set: none, settle: time.Second, act: play("eyeroll", 0), frames: 50, zoom: 4},
	}
}

func mascotSheet(t *testing.T, path string, shots []galleryShot, cols int) {
	cw, ch := 0, 0
	for _, s := range shots {
		cw, ch = max(cw, s.img.Rect.Dx()), max(ch, s.img.Rect.Dy())
	}
	const lab = 22
	rows := (len(shots) + cols - 1) / cols
	out := newCanvas(cols*(cw+12), rows*(ch+lab))
	draw.Draw(out.img, out.img.Rect, image.NewUniform(color.NRGBA{0x10, 0x12, 0x10, 255}), image.Point{}, draw.Src)
	for i, s := range shots {
		x, y := (i%cols)*(cw+12), (i/cols)*(ch+lab)
		out.text(face(fMedium, 13), s.label, float32(x+8), float32(y+16), color.NRGBA{0x9a, 0xa0, 0x98, 255})
		draw.Draw(out.img, s.img.Rect.Add(image.Pt(x, y+lab)), s.img, image.Point{}, draw.Src)
	}
	savePNG(t, path, out.img)
}
