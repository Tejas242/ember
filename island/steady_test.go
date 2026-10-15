package island

import (
	"image"
	"testing"
	"time"
)

// A steady frame (only Ember and the live parts repainted) must look the same
// as a full repaint at the same instant: otherwise stale pixels pile up.
func TestSteadyFrameMatchesFull(t *testing.T) {
	cases := map[string]func(u *UI, clock time.Time){
		"music-card": func(u *UI, clock time.Time) {
			u.track = &Track{Player: "p", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: 5 * time.Minute, Position: 2 * time.Minute, PosAt: clock}
			u.pinned = true
		},
		"music-pill": func(u *UI, clock time.Time) {
			u.track = &Track{Player: "p", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: 5 * time.Minute, Position: 2 * time.Minute, PosAt: clock}
		},
		"volume-casting": func(u *UI, clock time.Time) {
			u.casting = true
			u.Moment(moment{key: "osd:volume", icon: "volume", tint: u.pal.fg, meter: true, level: 0.5, right: "50", osd: true, until: clock.Add(time.Hour)})
		},
		"agents": func(u *UI, clock time.Time) {
			working(u)
			fakeAgents(u.sessions["a"], 2, clock)
		},
		"claude-music": func(u *UI, clock time.Time) {
			working(u)
			u.track = &Track{Player: "p", Title: "Veridis Quo", Playing: true, Length: 5 * time.Minute, PosAt: clock}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			clock := time.Date(2026, 10, 11, 21, 30, 0, 0, time.Local)
			u := NewUI(440, 280, 22)
			u.now = func() time.Time { return clock }
			setup(u, clock)
			settle(u, &clock)
			for i := 0; i < 200; i++ { // seconds tick by on the fast path
				clock = clock.Add(time.Second / 12)
				u.renderDamage()
			}
			steady := image.NewRGBA(u.cv.img.Rect)
			copy(steady.Pix, u.cv.img.Pix)
			u.dirty = true
			u.renderDamage()
			skip := u.emberRect().Inset(-2) // Ember's eyes step a hair between the two frames
			bad := 0
			var at image.Point
			for y := 0; y < u.H; y++ {
				for x := 0; x < u.W; x++ {
					if (image.Point{x, y}).In(skip) {
						continue
					}
					i := y*steady.Stride + x*4
					for c := 0; c < 4; c++ {
						d := int(steady.Pix[i+c]) - int(u.cv.img.Pix[i+c])
						if d > 6 || d < -6 {
							if bad == 0 {
								at = image.Pt(x, y)
							}
							bad++
							break
						}
					}
				}
			}
			if bad > 0 {
				t.Fatalf("%d pixels differ from a full repaint (first at %v)", bad, at)
			}
		})
	}
}
