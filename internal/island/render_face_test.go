//go:build render

package island

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// EMBER_FACE=dir go test -tags render -count=1 -run TestFaceLab ./internal/island
//
// Ember's face in every state over time, at its three sizes from the same
// pose: the pill's 16 px (1× and 4× without smoothing), the card's 24 px (1×
// and 3×) and the stage's 88 px. 60 fps, every other frame kept (30 fps).

type faceScene struct {
	name string
	set  func(t *testing.T, g *galleryScene) func(i int)
	secs float64
}

func feedSong(g *galleryScene, i int) {
	lv := songLevels()
	n := int(float64(i) / 60 / 0.04)
	a := g.a.ui.audio
	if v, at, _ := a.Sample(); v != lv[n%len(lv)] || time.Since(at) > 200*time.Millisecond {
		a.Feed(lv[n%len(lv)], time.Now())
	}
}

func faceScenes() []faceScene {
	nop := func(i int) {}
	return []faceScene{
		{"idle", func(t *testing.T, g *galleryScene) func(int) { g.run(time.Second); return nop }, 8},
		{"working", func(t *testing.T, g *galleryScene) func(int) {
			g.work("k", "kinvault", "fix the race", 3)
			g.run(time.Second)
			return func(i int) {
				if i%70 == 0 {
					hookAt(g.a, "PreToolUse", "k", "kinvault", map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/a.go"}})
				}
			}
		}, 6},
		{"needs", func(t *testing.T, g *galleryScene) func(int) {
			g.work("k", "kinvault", "fix the race", 2)
			g.run(time.Second)
			return func(i int) {
				if i == 20 {
					hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
				}
			}
		}, 4},
		{"done", func(t *testing.T, g *galleryScene) func(int) {
			g.work("k", "kinvault", "fix the race", 3)
			g.run(time.Second)
			return func(i int) {
				if i == 20 {
					g.clock = g.clock.Add(5 * time.Minute)
					hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "All 42 tests pass under -race."})
				}
			}
		}, 4},
		{"stopped", func(t *testing.T, g *galleryScene) func(int) {
			g.work("k", "kinvault", "fix the race", 3)
			g.run(time.Second)
			return func(i int) {
				if i == 20 {
					hookAt(g.a, "StopFailure", "k", "kinvault", map[string]any{"error": "rate_limit"})
				}
			}
		}, 4},
		{"asleep-wake", func(t *testing.T, g *galleryScene) func(int) {
			m := g.a.ui.mind
			m.asleep, m.arousal, m.sleepAt, m.lastEvent = true, 0.08, g.clock.Add(-time.Hour), g.clock.Add(-time.Hour)
			g.run(time.Second)
			return func(i int) {
				if i == 150 {
					g.a.ui.mind.asleep = false
					g.a.ui.mind.sleepAt = g.clock
					g.a.ui.play("wake", 0)
				}
			}
		}, 5},
		{"hovered", func(t *testing.T, g *galleryScene) func(int) {
			g.run(time.Second)
			u := g.a.ui
			x0 := u.ex.x
			return func(i int) {
				// the pointer drifts left and right past Ember, then rests on it
				var dx float64
				switch {
				case i < 60:
					dx = 120
				case i < 300:
					dx = 120 - float64(i-60)
				default:
					dx = -120 + float64(i-300)*0.8
				}
				u.pointer(pointerEvent{kind: "motion", x: x0 + dx, y: 12})
			}
		}, 7},
		{"dance-pill", func(t *testing.T, g *galleryScene) func(int) {
			g.music(true)
			g.run(1500 * time.Millisecond)
			g.a.ui.audio = &source.Audio{}
			for i := 0; i < 240; i++ { // it has heard the song a few seconds
				feedSong(g, i)
				g.run(time.Second / 60)
			}
			return func(i int) { feedSong(g, 240+i) }
		}, 16},
		{"stage-sing", func(t *testing.T, g *galleryScene) func(int) {
			return nil // built by buildVibe
		}, 10},
	}
}

func TestFaceLab(t *testing.T) {
	dir := os.Getenv("EMBER_FACE")
	if dir == "" {
		t.Skip("set EMBER_FACE")
	}
	t.Setenv("HOME", t.TempDir())
	only := os.Getenv("EMBER_FACE_ONLY")
	for _, sc := range faceScenes() {
		if only != "" && only != sc.name {
			continue
		}
		var g *galleryScene
		var each func(int)
		if sc.name == "stage-sing" {
			vs := vibeScenes()[0]
			g, _ = buildVibe(t, vs)
			each = func(i int) { feedLevels(g, vs.feed, 300+i) }
		} else {
			g = newScene()
			each = sc.set(t, g)
		}
		sub := filepath.Join(dir, sc.name)
		os.MkdirAll(sub, 0o755)
		trace, _ := os.Create(filepath.Join(sub, "trace.csv"))
		fmt.Fprintln(trace, "frame,t,eyes,eyeH,lid,lidR,slant,lookX,lookY,mouth,open,lift,lean,dancing,phase")
		n := int(sc.secs * 60)
		var strip []*image.RGBA
		for i := 0; i < n; i++ {
			each(i)
			g.run(time.Second / 60)
			u := g.a.ui
			now := u.now()
			tt := now.Sub(u.start).Seconds()
			ps := u.emberPose(tt, now)
			fmt.Fprintf(trace, "%d,%.3f,%d,%.2f,%.2f,%.2f,%.2f,%.2f,%.2f,%d,%.2f,%.2f,%.2f,%v,%.2f\n", i, float64(i)/60, ps.eyes, ps.eyeH, ps.lid, faceLidR(ps), ps.slant, ps.lookX, ps.lookY, ps.mouth, faceOpen(ps), ps.lift, ps.lean, u.dancing, u.groove.phase)
			if i%2 != 0 {
				continue
			}
			img := faceFrame(u, ps)
			savePNG(t, filepath.Join(sub, fmt.Sprintf("%04d.png", i/2)), img)
			if (i/2)%15 == 0 && len(strip) < 12 {
				strip = append(strip, img)
			}
		}
		trace.Close()
		if len(strip) > 0 {
			w, h := strip[0].Rect.Dx(), strip[0].Rect.Dy()
			out := image.NewRGBA(image.Rect(0, 0, w*3, h*((len(strip)+2)/3)))
			for j, s := range strip {
				draw.Draw(out, s.Rect.Add(image.Pt((j%3)*w, (j/3)*h)), s, image.Point{}, draw.Src)
			}
			savePNG(t, filepath.Join(dir, sc.name+"-strip.png"), out)
		}
	}
}

// faceFrame draws the pose at 16, 24 and 88 px: [16 1×][16 4×][24 1×][24 3×][88 1×].
func faceFrame(u *UI, ps pose) *image.RGBA {
	bg := mix(u.pal.island, color.NRGBA{0, 0, 0, 255}, 0)
	bg.A = 255
	cell := func(s float32, w, h int) *image.RGBA {
		c := newCanvas(w, h)
		draw.Draw(c.img, c.img.Rect, image.NewUniform(bg), image.Point{}, draw.Src)
		cx := float32(w / 2)
		base := float32(h) - 0.22*s - 2
		f := u.emberFire()
		old := u.cv
		u.cv = c
		g := newGeo(cx, base, s, ps)
		c.drawFlame(u.pal, cx, base, s, ps, f, 1)
		u.drawPhones(g)
		u.drawParts(g, f)
		u.cv = old
		return c.img
	}
	c16, c24, c88 := cell(16, 30, 34), cell(24, 44, 48), cell(88, 150, 160)
	H := 160
	out := image.NewRGBA(image.Rect(0, 0, 30+120+44+132+150+40, H))
	draw.Draw(out, out.Rect, image.NewUniform(color.NRGBA{0x10, 0x12, 0x10, 255}), image.Point{}, draw.Src)
	x := 6
	put := func(img image.Image) {
		b := img.Bounds()
		draw.Draw(out, image.Rect(x, H-b.Dy(), x+b.Dx(), H), img, b.Min, draw.Src)
		x += b.Dx() + 6
	}
	put(c16)
	put(upscale(c16, 4))
	put(c24)
	put(upscale(c24, 3))
	put(c88)
	return out
}
