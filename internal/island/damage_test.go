package island

import (
	"image"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// What reaches the screen is only each frame's damage. After any sequence of
// views, the screen built from those rectangles must equal the frame: a
// pixel drawn outside its damage stays on screen as a ghost (Ember left
// behind beside the pill).
func TestDamageLeavesNoGhosts(t *testing.T) {
	type step struct {
		after time.Duration
		do    func(g *galleryScene)
	}
	cases := map[string][]step{
		"done moment ends": {
			{0, func(g *galleryScene) { g.work("k", "kinvault", "fix it", 2) }},
			{time.Second, func(g *galleryScene) {
				g.a.ui.Moment(moment{key: "done:k", act: "claude", mood: moodHappy, verb: "Done", text: "kinvault", sub: "All 42 tests pass under -race and more words", until: g.clock.Add(2 * time.Second)})
			}},
			{3 * time.Second, func(g *galleryScene) { g.a.ui.expire() }},
		},
		"card opens and closes": {
			{0, func(g *galleryScene) { g.work("k", "kinvault", "fix the race in the compaction path", 3) }},
			{time.Second, func(g *galleryScene) { g.a.ui.pinned = true }},
			{2 * time.Second, func(g *galleryScene) { g.a.ui.pinned = false }},
		},
		"request comes and goes": {
			{0, func(g *galleryScene) { g.work("k", "kinvault", "fix it", 2) }},
			{time.Second, func(g *galleryScene) {
				g.a.ui.AddCard(&Card{ID: "p", Kind: "perm", Mood: moodWait, Eyebrow: "kinvault wants to run", Title: "Run the tests", Buttons: []Button{{Key: "allow", Label: "Allow", Primary: true}}})
			}},
			{2 * time.Second, func(g *galleryScene) { g.a.ui.RemoveCard("p") }},
		},
		"needs you, then answered": {
			{0, func(g *galleryScene) {
				g.work("k", "kinvault", "fix it", 2)
				hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
			}},
			{2 * time.Second, func(g *galleryScene) {
				hookAt(g.a, "PostToolUse", "k", "kinvault", map[string]any{"tool_name": "Read"})
			}},
		},
		"wide music to a narrow pill": {
			{0, func(g *galleryScene) { g.music(true); g.a.ui.focus, g.a.ui.focusAt = "music", g.clock }},
			{time.Second, func(g *galleryScene) { g.a.ui.focus, g.a.ui.focusAt = "clock", g.clock }},
		},
		"a volume key with the card open": {
			{0, func(g *galleryScene) {
				g.work("k", "kinvault", "fix the race in the compaction path", 3)
				g.a.ui.pinned = true
			}},
			{time.Second, func(g *galleryScene) { g.a.handle(source.Volume{V: 0.5, OK: true}) }},
			{1200 * time.Millisecond, func(g *galleryScene) { g.a.handle(source.Volume{V: 0.55, OK: true}) }},
		},
		"a volume key over a request": {
			{0, func(g *galleryScene) {
				g.a.ui.AddCard(&Card{ID: "p", Kind: "perm", Mood: moodWait, Eyebrow: "kinvault wants to run", Title: "Run the tests", Buttons: []Button{{Key: "allow", Label: "Allow", Primary: true}}})
			}},
			{time.Second, func(g *galleryScene) { g.a.handle(source.Brightness{V: 0.3}) }},
		},
		"hover swell": {
			{0, func(g *galleryScene) { g.music(true) }},
			{time.Second, func(g *galleryScene) { g.a.ui.pointer(pointerEvent{kind: "enter", x: surfW / 2, y: 10}) }},
			{1100 * time.Millisecond, func(g *galleryScene) { g.a.ui.pointer(pointerEvent{kind: "leave"}) }},
		},
	}
	for name, steps := range cases {
		t.Run(name, func(t *testing.T) {
			g := newScene()
			u := g.a.ui
			screen := image.NewRGBA(u.cv.img.Rect)
			frame := func() {
				_, d := u.renderDamage()
				d = d.Intersect(screen.Rect)
				// everything drawn this frame must be inside its damage: Ember, and live parts like the halo
				for _, r := range append([]image.Rectangle{u.emberRect()}, u.liveRects()...) {
					if r = r.Intersect(screen.Rect); !r.Empty() && !r.In(d) {
						t.Fatalf("drew %v outside the frame's damage %v", r, d)
					}
				}
				for y := d.Min.Y; y < d.Max.Y; y++ {
					o := y*screen.Stride + d.Min.X*4
					copy(screen.Pix[o:o+d.Dx()*4], u.cv.img.Pix[o:o+d.Dx()*4])
				}
			}
			start := g.clock
			for _, st := range steps {
				for g.clock.Before(start.Add(st.after)) {
					g.clock = g.clock.Add(time.Second / 60)
					frame()
				}
				st.do(g)
				u.Invalidate()
			}
			for i := 0; i < 240; i++ {
				g.clock = g.clock.Add(time.Second / 60)
				frame()
			}
			bad, at := 0, image.Point{}
			for i := 0; i < len(screen.Pix); i += 4 {
				for c := 0; c < 4; c++ {
					if d := int(screen.Pix[i+c]) - int(u.cv.img.Pix[i+c]); d > 8 || d < -8 {
						if bad == 0 {
							at = image.Pt((i/4)%screen.Stride*0+(i%screen.Stride)/4, i/screen.Stride)
						}
						bad++
						break
					}
				}
			}
			if bad > 0 {
				t.Fatalf("%d ghost pixels on screen (first at %v)", bad, at)
			}
		})
	}
}
