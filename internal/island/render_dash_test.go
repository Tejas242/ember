//go:build render

package island

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tejas242/ember/plugins/edge/adapter"
)

// EMBER_PREVIEW=dir go test -tags render -count=1 -run PreviewDash ./internal/island: edge's
// dashboard (its plugin's widgets) as it opens, settled, and from today's
// real `edge stats` if EMBER_EDGE_STATS names a file.
func TestPreviewDash(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	const W, H = surfW, surfH
	render := func(st *adapter.Stats, frames []int) *image.RGBA {
		clock := time.Date(2026, 10, 20, 21, 40, 0, 0, time.Local)
		u := NewUI(W, H, 22)
		u.now = func() time.Time { return clock }
		edgeSays(u, nil, st)
		settle(u, &clock)
		u.openDash("edge")
		strip := image.NewRGBA(image.Rect(0, 0, W, H*len(frames)))
		draw.Draw(strip, strip.Rect, image.NewUniform(color.NRGBA{0x22, 0x33, 0x26, 255}), image.Point{}, draw.Src)
		f := 0
		for i, n := range frames {
			for ; f < n; f++ {
				clock = clock.Add(time.Second / 60)
				u.renderDamage()
			}
			draw.Draw(strip, image.Rect(0, H*i, W, H*i+22), image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 255}), image.Point{}, draw.Over)
			draw.Draw(strip, image.Rect(0, H*i, W, H*i+H), u.cv.img, image.Point{}, draw.Over)
		}
		return strip
	}
	save := func(name string, img image.Image) {
		out, _ := os.Create(filepath.Join(dir, name))
		png.Encode(out, img)
		out.Close()
	}
	st := edgeFixture()
	st.Today.Gate, st.Today.GateWhy = "closed", ""
	save("dash-film.png", render(st, []int{8, 20, 40, 60, 140}))
	if p := os.Getenv("EMBER_EDGE_STATS"); p != "" {
		var real adapter.Stats
		b, err := os.ReadFile(p)
		if err != nil || json.Unmarshal(b, &real) != nil {
			t.Fatal("bad stats file", err)
		}
		save("dash-real.png", render(&real, []int{160}))
	}
}
