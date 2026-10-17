package island

import (
	"image"
	"image/color"
	"math"
	"time"
)

// A scripted island for tests and the render tools alike: a fixed clock, a
// test app, and helpers that play Claude Code hooks, music and edge into it.

type galleryScene struct {
	clock time.Time
	a     *app
}

func newScene() *galleryScene {
	g := &galleryScene{clock: time.Date(2026, 10, 15, 17, 24, 0, 0, time.Local)}
	g.a = newTestApp(&g.clock)
	u := NewUI(surfW, surfH, 22)
	u.now, u.mind, u.day = g.a.ui.now, g.a.ui.mind, g.a.ui.day
	u.start, u.lastBlink, u.bounceAt = g.clock, g.clock, g.clock.Add(-time.Hour)
	g.a.ui = u
	return g
}

func (g *galleryScene) run(d time.Duration) {
	for end := g.clock.Add(d); g.clock.Before(end); {
		g.clock = g.clock.Add(time.Second / 60)
		g.a.ui.renderDamage()
	}
}

var gallerySteps = []map[string]any{
	{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/store/compact.go"}},
	{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/store/quota.go", "old_string": "a\nb\nc", "new_string": "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no\np\nq\nr"}},
	{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test ./store/...", "description": "Run the store tests"}, "fail": true},
	{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/store/store.go", "old_string": "a\nb\nc", "new_string": "a\nx\ny\nz\nw"}},
	{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test -race ./...", "description": "Run the whole suite under the race detector"}},
}

func (g *galleryScene) work(id, project, prompt string, n int) {
	hookAt(g.a, "UserPromptSubmit", id, project, map[string]any{"prompt": prompt})
	for i, st := range gallerySteps[:n] {
		g.clock = g.clock.Add(41 * time.Second)
		hookAt(g.a, "PreToolUse", id, project, map[string]any{"tool_name": st["tool_name"], "tool_input": st["tool_input"]})
		if i < n-1 {
			ev := "PostToolUse"
			if st["fail"] == true {
				ev = "PostToolUseFailure"
			}
			hookAt(g.a, ev, id, project, map[string]any{"tool_name": st["tool_name"]})
		}
	}
}

func (g *galleryScene) music(playing bool) {
	g.a.ui.track = &Track{Player: "p", Title: "Instant Crush", Artist: "Daft Punk, Julian Casablancas", Playing: playing,
		Length: 5*time.Minute + 37*time.Second, Position: 2 * time.Minute, PosAt: g.clock}
	g.a.ui.art = galleryArt()
}

func galleryArt() *albumArt {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dx, dy := float64(x)-22, float64(y)-20
			d := math.Sqrt(dx*dx+dy*dy) / 60
			r := 235 - 120*d
			g := 120 - 60*d + float64(y)
			b := 70 + 90*d
			if (x/8+y/8)%5 == 0 {
				r, g = r*0.85, g*0.9
			}
			img.Set(x, y, color.NRGBA{uint8(math.Min(255, r)), uint8(math.Min(255, math.Max(0, g))), uint8(math.Min(255, b)), 255})
		}
	}
	return makeArt(img, "gallery")
}
