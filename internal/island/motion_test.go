package island

import (
	"math"
	"testing"
	"time"
)

// Ember's springs are stiff; a frame as slow as the card's 24 fps tick must
// still settle them, not ring them up until Ember flies off the island.
func TestSpringsSettleAtAnyFrameRate(t *testing.T) {
	for _, fps := range []float64{60, 30, 24, 12, 6} {
		s := spring{x: 1}
		for i := 0; i < int(fps*3); i++ {
			s.stepK(0, 1/fps, 600, 49)
		}
		if math.Abs(s.x) > 0.01 || math.IsNaN(s.x) {
			t.Errorf("%v fps: still %.3g px off after 3 s", fps, s.x)
		}
	}
}

// The card held open on its steady 24 fps tick: Ember stays where it is.
func TestCardSteadyAt24fps(t *testing.T) {
	g := newScene()
	g.work("k", "kinvault", "fix the race", 3)
	g.a.ui.pinned = true
	g.run(time.Second) // opens, settles
	u := g.a.ui
	x0, y0 := u.ex.x, u.ey.x
	for i := 0; i < 240; i++ { // 10 s of steady frames
		g.clock = g.clock.Add(time.Second / 24)
		u.renderDamage()
	}
	if math.Abs(u.ex.x-x0) > 0.5 || math.Abs(u.ey.x-y0) > 0.5 || math.IsNaN(u.ex.x) {
		t.Fatalf("Ember drifted from (%.1f, %.1f) to (%.1f, %.1f) at 24 fps", x0, y0, u.ex.x, u.ey.x)
	}
}
