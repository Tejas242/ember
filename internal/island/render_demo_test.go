//go:build render

package island

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

// EMBER_DEMO=dir go test -tags render -count=1 -run Demo ./island renders the README's demos:
// scripted scenes played on the real renderer, frame by frame, over a made-up
// wallpaper and bar, with a pointer, encoded to animated WebP by ffmpeg.
// Nothing comes from the machine they're made on: the default palette, a
// synthetic desktop, sample sessions and songs.

const (
	demoW, demoH = 760, 380 // the part of the screen a demo shows
	demoScale    = 2
	demoFPS      = 30
)

// demoWallpaper is a soft dusk gradient with a little grain, and a bar.
// EMBER_GROUND=site makes it the landing page's own dark ground instead, so
// the frames sit on the page without a wallpaper around them.
func demoWallpaper() *image.RGBA {
	const W, H = 1366, 768
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	ground := os.Getenv("EMBER_GROUND") == "site"
	type blob struct {
		x, y, r float64
		c       [3]float64
	}
	blobs := []blob{
		{430, 40, 420, [3]float64{24, 110, 120}},  // teal, top left of the island
		{980, 30, 420, [3]float64{120, 58, 140}},  // violet, top right
		{660, 470, 460, [3]float64{190, 92, 46}},  // an ember glow rising from below
		{1120, 400, 380, [3]float64{60, 34, 110}}, // indigo
		{200, 520, 420, [3]float64{16, 60, 70}},   // deep teal
		{690, 150, 260, [3]float64{36, 46, 74}},   // a little light where the island hangs
	}
	if ground {
		blobs = nil
	}
	rng := rand.New(rand.NewSource(7))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			c := [3]float64{10, 12, 18}
			if ground {
				c = [3]float64{8, 10, 9}
			}
			for _, b := range blobs {
				dx, dy := float64(x)-b.x, float64(y)-b.y
				k := math.Exp(-(dx*dx + dy*dy) / (b.r * b.r))
				for i := range c {
					c[i] += b.c[i] * k * 0.5 // dusk, not daylight: the island should stand out
				}
			}
			n := (rng.Float64() - 0.5) * 5 // grain: no banding
			if ground {
				n = 0
			}
			i := img.PixOffset(x, y)
			for ch := 0; ch < 3; ch++ {
				img.Pix[i+ch] = uint8(math.Max(0, math.Min(255, c[ch]+n)))
			}
			img.Pix[i+3] = 255
		}
	}
	// the bar: a glass strip, workspaces on the left, a few glyphs on the right
	cv := &canvas{img: img, masks: map[maskKey]*textMask{}}
	cv.r = newCanvas(1, 1).r
	bar := color.NRGBA{10, 12, 18, 150}
	if ground {
		bar = color.NRGBA{17, 20, 18, 255}
	}
	cv.roundRect(0, 0, W, 22, 0, bar)
	fg := color.NRGBA{220, 226, 236, 255}
	dim := color.NRGBA{220, 226, 236, 110}
	cv.roundRect(12, 7, 18, 8, 4, fg)
	cv.roundRect(36, 7, 8, 8, 4, dim)
	cv.roundRect(50, 7, 8, 8, 4, dim)
	cv.text(face(fMedium, 11.5), "kinvault — nvim", 72, 15, color.NRGBA{220, 226, 236, 170})
	f := face(fMedium, 11.5)
	cv.text(f, "84%", W-36, 15, fg)
	cv.battery(W-62, 11, 8, 0.84, alpha(fg, 0.6), fg)
	cv.icon("volume", W-80, 11, 12, fg)
	cv.icon("web", W-100, 11, 12, fg)
	return img
}

// pointerSprite draws an arrow with its tip at (x, y).
func pointerSprite(c *canvas, x, y float32, pressed float64) {
	s := float32(1 - 0.08*pressed)
	ps := []pt{{0, 0}, {0, 15}, {4, 11.5}, {6.8, 17.5}, {9, 16.5}, {6.3, 10.8}, {11, 10.8}}
	out := make([]pt, len(ps))
	in := make([]pt, len(ps))
	for i, p := range ps {
		out[i] = pt{x + p.x*s*1.12 - 0.9, y + p.y*s*1.12 - 1.2}
		in[i] = pt{x + p.x*s, y + p.y*s}
	}
	c.poly(color.NRGBA{0, 0, 0, 90}, shift(out, 0.8, 1.4)...) // a soft shadow
	c.poly(color.NRGBA{20, 20, 24, 255}, out...)
	c.poly(color.NRGBA{250, 250, 252, 255}, in...)
}

func shift(ps []pt, dx, dy float32) []pt {
	out := make([]pt, len(ps))
	for i, p := range ps {
		out[i] = pt{p.x + dx, p.y + dy}
	}
	return out
}

// demo plays a script on a scene and writes its frames.
type demo struct {
	t      *testing.T
	g      *galleryScene
	bg     *image.RGBA
	name   string
	ff     *exec.Cmd
	pipe   io.WriteCloser
	n      int
	px, py float64 // the pointer, in screen coordinates (-1: not shown)
	press  float64
	cropW  int
	cropH  int
	sx0    int // where the surface's left edge is on the crop
}

func newDemo(t *testing.T, name string, cropW, cropH int) *demo {
	g := newScene()
	g.a.ui.pal = loadPalette()
	d := &demo{t: t, g: g, bg: demoWallpaper(), name: name, px: -1, cropW: cropW, cropH: cropH, sx0: (cropW - surfW) / 2}
	out := filepath.Join(os.Getenv("EMBER_DEMO"), name+".webp")
	d.ff = exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgba",
		"-s", fmt.Sprintf("%dx%d", cropW*demoScale, cropH*demoScale), "-framerate", fmt.Sprint(demoFPS), "-i", "-",
		"-c:v", "libwebp_anim", "-lossless", "0", "-q:v", "82", "-compression_level", "6", "-loop", "0", out)
	var err error
	if d.pipe, err = d.ff.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := d.ff.Start(); err != nil {
		t.Fatal(err)
	}
	return d
}

// run advances the scene by d, writing a frame every 1/30 s.
func (d *demo) run(dur time.Duration) {
	steps := int(dur.Seconds() * 60)
	for i := 0; i < steps; i++ {
		d.g.clock = d.g.clock.Add(time.Second / 60)
		if d.g.a.ui.expire() { // moments end, as the app's timer ends them
			d.g.a.ui.Invalidate()
		}
		d.g.a.ui.renderDamage()
		if i%(60/demoFPS) == 0 {
			d.frame()
		}
	}
}

func (d *demo) frame() {
	u := d.g.a.ui
	out := image.NewRGBA(image.Rect(0, 0, d.cropW, d.cropH))
	ox := (d.bg.Rect.Dx() - d.cropW) / 2
	draw.Draw(out, out.Rect, d.bg, image.Pt(ox, 0), draw.Src)
	for _, r := range u.shape.blurRects() {
		boxBlur(out, r.Add(image.Pt(d.sx0, 0)).Inset(-1), 9)
	}
	draw.Draw(out, image.Rect(d.sx0, 0, d.sx0+surfW, d.cropH), u.cv.img, image.Point{}, draw.Over)
	big := newCanvas(d.cropW*demoScale, d.cropH*demoScale)
	xdraw.CatmullRom.Scale(big.img, big.img.Rect, out, out.Rect, xdraw.Src, nil)
	if d.px >= 0 {
		pointerSprite(big, float32(d.px*demoScale), float32(d.py*demoScale), d.press)
	}
	for i := 3; i < len(big.img.Pix); i += 4 { // opaque: straight and premultiplied agree
		big.img.Pix[i] = 255
	}
	if _, err := d.pipe.Write(big.img.Pix); err != nil {
		d.t.Fatal(err)
	}
	d.n++
}

// move glides the pointer to (x, y) on the crop over dur, easing in and out,
// telling the island where it is.
func (d *demo) move(x, y float64, dur time.Duration) {
	x0, y0 := d.px, d.py
	if x0 < 0 {
		x0, y0 = x, float64(d.cropH)+20
	}
	steps := int(dur.Seconds() * 60)
	u := d.g.a.ui
	for i := 1; i <= steps; i++ {
		k := easeInOut(float64(i) / float64(steps))
		d.px, d.py = x0+(x-x0)*k, y0+(y-y0)*k
		lx, ly := d.px-float64(d.sx0), d.py
		inside := u.hitAt(lx, ly) != ""
		switch {
		case inside:
			if redraw, _ := u.pointer(pointerEvent{kind: "motion", x: lx, y: ly}); redraw {
				u.Invalidate()
			}
		case u.hover:
			u.pointer(pointerEvent{kind: "leave"})
			u.Invalidate()
		}
		d.g.clock = d.g.clock.Add(time.Second / 60)
		u.renderDamage()
		if i%(60/demoFPS) == 0 {
			d.frame()
		}
	}
}

// to glides the pointer to the centre of the control under key.
func (d *demo) to(key string, dur time.Duration) {
	for _, h := range d.g.a.ui.hits {
		if h.key == key {
			c := h.r.Min.Add(h.r.Max).Div(2)
			d.move(float64(c.X+d.sx0), float64(c.Y), dur)
			return
		}
	}
	d.t.Fatalf("no control %q", key)
}

// click presses and lets go where the pointer is.
func (d *demo) click() {
	u := d.g.a.ui
	lx, ly := d.px-float64(d.sx0), d.py
	u.pointer(pointerEvent{kind: "press", x: lx, y: ly, button: 1})
	u.Invalidate()
	for i := 0; i < 6; i++ {
		d.press = math.Min(1, float64(i+1)/3)
		d.run(time.Second / 60)
	}
	u.pointer(pointerEvent{kind: "release", x: lx, y: ly, button: 1})
	u.Invalidate()
	d.press = 0
}

func (d *demo) event(f func(g *galleryScene)) {
	f(d.g)
	d.g.a.ui.Invalidate()
}

// encode finishes the WebP.
func (d *demo) encode(name string) {
	out := filepath.Join(os.Getenv("EMBER_DEMO"), name+".webp")
	d.pipe.Close()
	if err := d.ff.Wait(); err != nil {
		d.t.Fatalf("ffmpeg: %v", err)
	}
	if st, err := os.Stat(out); err == nil {
		d.t.Logf("%s: %d frames, %.1f MB", out, d.n, float64(st.Size())/1e6)
	}
}

func TestDemo(t *testing.T) {
	if os.Getenv("EMBER_DEMO") == "" {
		t.Skip("set EMBER_DEMO")
	}
	t.Setenv("HOME", t.TempDir()) // the default palette, not this machine's wallpaper

	// 1. Claude at work: the pill, the card with its live timeline, a
	// request answered on the island, and the work done.
	t.Run("claude", func(t *testing.T) {
		d := newDemo(t, "claude", demoW, 330)
		d.run(1200 * time.Millisecond)
		steps := []map[string]any{
			{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/store/compact.go"}},
			{"tool_name": "Grep", "tool_input": map[string]any{"pattern": "segment.Lock"}},
			{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/store/compact.go", "old_string": "a\nb", "new_string": "a\nb\nc\nd\ne\nf"}},
			{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test ./store/...", "description": "Run the store tests"}},
			{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/store/segment.go", "old_string": "a\nb\nc", "new_string": "a\nx\ny\nz"}},
		}
		stepAt := func(i int) {
			d.event(func(g *galleryScene) {
				if i > 0 {
					hookAt(g.a, "PostToolUse", "k", "kinvault", map[string]any{"tool_name": steps[i-1]["tool_name"]})
				}
				hookAt(g.a, "PreToolUse", "k", "kinvault", steps[i])
			})
		}
		d.event(func(g *galleryScene) {
			hookAt(g.a, "UserPromptSubmit", "k", "kinvault", map[string]any{"prompt": "fix the race in the store's compaction path"})
		})
		d.run(700 * time.Millisecond)
		stepAt(0)
		d.run(600 * time.Millisecond)
		d.move(demoW/2+10, 11, 700*time.Millisecond) // over the pill: the card drops
		d.run(500 * time.Millisecond)
		for i := 1; i < len(steps); i++ {
			stepAt(i)
			d.run(900 * time.Millisecond)
		}
		d.move(demoW/2+60, 345, 600*time.Millisecond) // away: it folds
		d.run(700 * time.Millisecond)
		var id string
		d.event(func(g *galleryScene) {
			id, _ = askAt(g.a, "PermissionRequest", "k", "kinvault", map[string]any{"tool_name": "Bash",
				"tool_input": map[string]any{"command": "go test -race -count=1 ./store/...", "description": "Run the store tests with the race detector"}})
		})
		_ = id
		d.run(1100 * time.Millisecond)
		d.to("btn:allow", 700*time.Millisecond)
		d.run(250 * time.Millisecond)
		d.click()
		d.run(500 * time.Millisecond)
		d.move(demoW/2+140, 330, 500*time.Millisecond)
		d.event(func(g *galleryScene) {
			hookAt(g.a, "PostToolUse", "k", "kinvault", map[string]any{"tool_name": "Bash"})
			g.clock = g.clock.Add(time.Second)
			hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "Fixed the race: writers take the segment lock before rotating, and all 42 tests pass under -race."})
		})
		d.run(2800 * time.Millisecond)
		d.encode("claude")
	})

	// 2. Music: the cover in its ring, the card's now playing, the controls.
	t.Run("music", func(t *testing.T) {
		d := newDemo(t, "music", 440, 210)
		d.g.a.ui.track = &Track{Player: "p", Title: "Instant Crush", Artist: "Daft Punk, Julian Casablancas", Playing: true,
			Length: 5*time.Minute + 37*time.Second, Position: 98 * time.Second, PosAt: d.g.clock}
		d.g.a.ui.art = galleryArt()
		copy(d.g.a.ui.day.Hours[:], []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 14, 31, 22, 4, 0, 18, 40, 52, 26})
		d.g.a.ui.day.Tasks, d.g.a.ui.day.Busy = 7, 2*time.Hour+14*time.Minute
		d.g.a.ui.day.Projects = map[string]int{"kinvault": 5, "notes": 2}
		d.run(1500 * time.Millisecond)
		d.move(float64(d.cropW)/2, 11, 700*time.Millisecond)
		d.run(900 * time.Millisecond)
		d.to("music:toggle", 600*time.Millisecond)
		d.run(300 * time.Millisecond)
		d.click()
		d.event(func(g *galleryScene) {
			g.a.ui.track.Playing, g.a.ui.track.Position, g.a.ui.track.PosAt = false, g.a.ui.track.Pos(g.clock), g.clock
		})
		d.run(900 * time.Millisecond)
		d.click()
		d.event(func(g *galleryScene) { g.a.ui.track.Playing, g.a.ui.track.PosAt = true, g.clock })
		d.run(800 * time.Millisecond)
		d.move(float64(d.cropW)/2+40, 230, 500*time.Millisecond)
		d.run(400 * time.Millisecond)
		for i, v := range []float64{0.52, 0.64, 0.78, 0.9, 1, 1, 1} {
			v := v
			d.event(func(g *galleryScene) {
				g.a.ui.Moment(moment{key: "osd:volume", icon: "volume", tint: g.a.ui.pal.fg, meter: true, level: v,
					right: fmt.Sprint(int(v * 100)), until: g.clock.Add(1600 * time.Millisecond), osd: true})
			})
			d.run(time.Duration(220+60*min(i, 3)) * time.Millisecond)
		}
		d.run(2200 * time.Millisecond)
		d.encode("music")
	})

	// 3. Moments: what the desktop tells you, for a moment.
	t.Run("moments", func(t *testing.T) {
		d := newDemo(t, "moments", 400, 48)
		d.run(800 * time.Millisecond)
		p := d.g.a.ui.pal
		show := func(m moment, dur time.Duration) {
			d.event(func(g *galleryScene) { m.until = g.clock.Add(dur); g.a.ui.Moment(m) })
			d.run(dur + 450*time.Millisecond)
		}
		show(moment{key: "power", icon: "bolt", tint: p.green, text: "Charging", sub: "full in 40m", batt: true, pct: 0.73}, 1800*time.Millisecond)
		show(moment{key: "device:x", icon: "headphones", tint: p.blue, text: "WH-1000XM4", sub: "Connected", right: "80%", batt: true, pct: 0.8}, 1800*time.Millisecond)
		thumb := scaleThumb(d.bg.SubImage(image.Rect(200, 100, 1166, 668)), 30, 18)
		show(moment{key: "shot:x", icon: "camera", tint: p.fg, text: "Screenshot saved", thumb: thumb}, 1800*time.Millisecond)
		for _, v := range []float64{0.7, 0.55, 0.4, 0.3} {
			v := v
			d.event(func(g *galleryScene) {
				g.a.ui.Moment(moment{key: "osd:brightness", icon: "sun", tint: p.yellow, meter: true, level: v,
					right: fmt.Sprint(int(v * 100)), until: g.clock.Add(1500 * time.Millisecond), osd: true})
			})
			d.run(260 * time.Millisecond)
		}
		d.run(1800 * time.Millisecond)
		d.event(func(g *galleryScene) {
			g.work("k", "kinvault", "fix it", 2)
			hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
		})
		d.run(3 * time.Second)
		d.encode("moments")
	})

	// 4. edge's dashboard opening.
	t.Run("dash", func(t *testing.T) {
		d := newDemo(t, "dash", demoW, 380)
		d.g.clock = time.Date(2026, 10, 20, 21, 40, 0, 0, time.Local)
		st := edgeFixture()
		st.Today.Gate, st.Today.GateWhy = "closed", ""
		edgeSays(d.g.a.ui, nil, st)
		d.run(600 * time.Millisecond)
		d.event(func(g *galleryScene) { g.a.ui.openDash("edge") })
		d.run(2400 * time.Millisecond)
		d.move(demoW/2+60, 250, 700*time.Millisecond) // a day in the year
		d.run(1200 * time.Millisecond)
		d.encode("dash")
	})
}

// tracked draws s with its letters set track px apart beyond their kerned
// advance (negative tightens), and returns its width.
func tracked(c *canvas, f fontFace, s string, x, base, track float32, col color.NRGBA, draw bool) float32 {
	x0 := x
	prev := rune(-1)
	for _, r := range s {
		if prev >= 0 {
			x += float32(f.Kern(prev, r)) / 64
		}
		if draw {
			c.text(f, string(r), x, base, col)
		}
		adv, _ := f.GlyphAdvance(r)
		x += float32(adv)/64 + track
		prev = r
	}
	return x - x0 - track
}

// TestBanner draws the README's header and the social preview in Ember's own
// language: the dark island hanging from a bar, the flame, tracked small
// capitals, hairlines, and the real card, drawn by the real renderer.
func TestBanner(t *testing.T) {
	if os.Getenv("EMBER_DEMO") == "" {
		t.Skip("set EMBER_DEMO")
	}
	t.Setenv("HOME", t.TempDir())
	for _, sz := range []struct {
		name string
		w, h int // at 1x; drawn at 2x
	}{{"banner", 840, 300}, {"social", 640, 320}} {
		W, H := sz.w*2, sz.h*2
		big := newCanvas(W, H)
		p := loadPalette()
		// the ground: near black, a faint grain, the island's light pooling under it
		bg := color.NRGBA{9, 11, 10, 255}
		draw.Draw(big.img, big.img.Rect, image.NewUniform(bg), image.Point{}, draw.Src)
		rng := rand.New(rand.NewSource(3))
		cardX := float32(W) - 880
		if sz.name == "social" {
			cardX = float32(W) - 640
		}
		big.glow(big.img.Rect, cardX+380, 40, 760, p.green, 0.07)
		big.glow(big.img.Rect, float32(W)*0.12, float32(H)*1.05, 620, mix(p.yellow, p.red, 0.4), 0.06)
		for i := 0; i < len(big.img.Pix); i += 4 {
			n := uint8(rng.Intn(5))
			for c := 0; c < 3; c++ {
				big.img.Pix[i+c] += n
			}
		}
		// a dot grid, the pixels it's drawn in, fading out from the card
		for y := 30; y < H; y += 24 {
			for x := 18; x < W; x += 24 {
				dx, dy := float64(x)-float64(cardX+380), float64(y)-60
				k := math.Exp(-(dx*dx + dy*dy) / (900 * 900))
				big.circle(float32(x), float32(y), 1.1, alpha(colorWhite, 0.06*k))
			}
		}
		// the bar, a glass strip with the island hanging from it
		big.roundRect(0, 0, float32(W), 44, 0, color.NRGBA{22, 25, 24, 255})
		big.roundRect(0, 44, float32(W), 1, 0, alpha(colorWhite, 0.06))

		// the real card: a session at work, rendered at 1x and set in place
		g := newScene()
		g.a.ui.pal = p
		g.work("k", "kinvault", "fix the race in the store's compaction path", 4)
		g.agent("k", "kinvault", "a1", "Map the store's locking", 2)
		g.a.ui.pinned = true
		g.run(2 * time.Second)
		u := g.a.ui
		sh := u.shape
		src := image.NewRGBA(image.Rect(0, 0, int(sh.w)+40, int(sh.h)+30))
		draw.Draw(src, src.Rect, u.cv.img, image.Pt(int(sh.x)-20, 0), draw.Src)
		cs := 2.0 // the card's scale: crisp at 2x, smaller on the social card
		if sz.name == "social" {
			cs = 1.45
		}
		card := image.NewRGBA(image.Rect(0, 0, int(float64(src.Rect.Dx())*cs), int(float64(src.Rect.Dy())*cs)))
		xdraw.CatmullRom.Scale(card, card.Rect, src, src.Rect, xdraw.Over, nil)
		cx := W - card.Rect.Dx() - 36 // inside the right margin
		draw.Draw(big.img, card.Rect.Add(image.Pt(cx, 0)), card, image.Point{}, draw.Over)

		// the name, on the left (on the social card, centred under the island)
		eyebrow := face(fSemiBold, 19)
		title := face(fSemiBold, 148)
		tag := face(fRegular, 31)
		mono := face(fMono, 21)
		lx, top := float32(104), float32(176)
		lines := []string{"Knows what Claude Code is doing, asks before", "it runs anything, and plays your music."}
		if sz.name == "social" {
			title, tag = face(fSemiBold, 128), face(fRegular, 29)
			lx, top = 84, 230
			lines = []string{"Knows what Claude Code is doing,", "asks before it runs anything,", "and plays your music."}
		}
		center := func(float32) float32 { return lx }
		eb := "A DYNAMIC ISLAND FOR WAYLAND"
		ebw := tracked(big, eyebrow, eb, 0, 0, 3.2, p.fg, false)
		ex := center(ebw + 26)
		strokeGlyph := func(x, y float32) { // the flame, small, before the eyebrow
			ps := restPose()
			ps.eyeH = 4
			big.drawFlame(p, x+9, y-6+0.45*22, 22, ps, fireOf(moodWork), 1)
		}
		strokeGlyph(ex, top)
		tracked(big, eyebrow, eb, ex+26, top, 3.2, mix(p.green, p.fg, 0.45), true)
		tw := tracked(big, title, "Ember", 0, 0, -5, colorWhite, false)
		tb := float32(150)
		if sz.name == "social" {
			tb = 132
		}
		tracked(big, title, "Ember", center(tw)-6, top+tb, -5, color.NRGBA{244, 247, 242, 255}, true)
		y := top + 214
		if sz.name == "social" {
			y = top + 192
		}
		for _, ln := range lines {
			lw := tracked(big, tag, ln, 0, 0, -0.3, colorWhite, false)
			tracked(big, tag, ln, center(lw), y, -0.3, color.NRGBA{190, 198, 190, 255}, true)
			y += 44
		}
		if sz.name == "banner" {
			// the facts, in mono, between hairlines
			y += 20
			big.roundRect(lx, y, 520, 1, 0.5, alpha(colorWhite, 0.09))
			x := lx
			for i, fct := range []string{"one go binary", "drawn in software", "~9 MB", "no polling"} {
				if i > 0 {
					big.circle(x+12, y+34, 2.2, alpha(p.green, 0.7))
					x += 26
				}
				x += big.text(mono, fct, x, y+41, color.NRGBA{140, 150, 142, 255})
			}
		}
		f, err := os.Create(filepath.Join(os.Getenv("EMBER_DEMO"), "..", sz.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, big.img)
		f.Close()
	}
}

// glow fills r with a soft radial light of col centred on (cx, cy), fading
// out by radius rad: the music card takes on the cover's colour.
func (c *canvas) glow(r image.Rectangle, cx, cy, rad float32, col color.NRGBA, strength float64) {
	r = r.Intersect(c.img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := c.img.Pix[y*c.img.Stride:]
		dy := float32(y) + 0.5 - cy
		for x := r.Min.X; x < r.Max.X; x++ {
			dx := float32(x) + 0.5 - cx
			d := (dx*dx + dy*dy) / (rad * rad)
			if d >= 1 {
				continue
			}
			k := (1 - d) * (1 - d) * float32(strength)
			i := x * 4
			fi := 1 - k
			row[i] = uint8(float32(col.R)*k + float32(row[i])*fi)
			row[i+1] = uint8(float32(col.G)*k + float32(row[i+1])*fi)
			row[i+2] = uint8(float32(col.B)*k + float32(row[i+2])*fi)
			row[i+3] = uint8(255*k + float32(row[i+3])*fi)
		}
	}
}
