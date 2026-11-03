package island

import (
	"math"
	"testing"
	"time"
)

// TestSimBudget fails when a state draws more frames a second than its row
// of the budget (budget.go) allows, on AC, on battery and under a fifth, or
// when anything is drawn with nobody to see it. The caps carry 5 % for the
// edges of the measured minute.
func TestSimBudget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ac, bat, low := (*power)(nil), &power{state: 2, pct: 60, present: true}, &power{state: 2, pct: 10, present: true}
	caps := []struct {
		state       string
		ac, bat, lo float64
	}{
		{"idle", 4, 3, 2},
		{"music-paused", 4, 3, 2},
		{"asleep", 3, 2, 1},
		{"claude-working", 12, 8, 4},
		{"music-dance", 24, 15, 4}, // under a fifth the dance stops: a song playing is just a lively pill
		{"music-dance+claude", 24, 15, 4},
		{"card-open+music", 24, 15, 12},
		{"vibe-stage", 28, 15, 10},
	}
	byName := map[string]simCase{}
	for _, c := range simCases() {
		byName[c.name] = c
	}
	for _, c := range caps {
		for i, tier := range []struct {
			name string
			pw   *power
			cap  float64
		}{{"ac", ac, c.ac}, {"battery", bat, c.bat}, {"low", low, c.lo}} {
			t.Run(c.state+"/"+tier.name, func(t *testing.T) {
				g := newScene()
				g.a.ui.power = tier.pw
				each := byName[c.state].set(g)
				g.a.ui.power = tier.pw // (the stage scene swaps the app)
				g.run(2 * time.Second)
				simulate(g, 20*time.Second, each)
				st := simulate(g, 60*time.Second, each)
				fps := float64(st.frames) / st.span.Seconds()
				t.Logf("%.1f fps (cap %.0f)", fps, tier.cap)
				if fps > tier.cap*1.05 {
					t.Errorf("%s on %s draws %.1f fps, over %.0f", c.state, tier.name, fps, tier.cap)
				}
				if i == 2 && c.state == "music-dance" && g.a.ui.dancing {
					t.Error("it still dances under a fifth of battery")
				}
			})
		}
	}
}

// Under a full-screen window, and once you've been away a while, nothing is
// drawn at all, for as long as it lasts.
func TestSimBudgetGated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for name, gate := range map[string]func(g *galleryScene){
		"fullscreen": func(g *galleryScene) {
			g.a.ui.full = true
			g.run(2 * time.Second) // it folds up into the edge
		},
		"away": func(g *galleryScene) {
			m := g.a.ui.mind
			m.away, m.awaySince = true, g.clock.Add(-3*time.Minute)
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := newScene()
			g.a.ui.bud.on = true
			g.work("s", "kinvault", "fix the race", 3)
			g.run(2 * time.Second)
			gate(g)
			if !g.a.ui.gated() {
				t.Fatal("not gated")
			}
			if st := simulate(g, 5*time.Minute, nil); st.frames != 0 {
				t.Errorf("%d frames drawn while nobody could see them", st.frames)
			}
		})
	}
}

// The mind is not stepped frame by frame, so a long gap with no frames ends
// where drawing every frame would have: one step over the whole gap.
func TestMindIntegratesOverGaps(t *testing.T) {
	start := time.Date(2026, 10, 15, 17, 24, 0, 0, time.Local)
	fine, coarse := newMind(start), newMind(start)
	for _, m := range []*mind{fine, coarse} {
		m.arousal, m.away = 0.9, true
	}
	s := senses{}
	for now := start; now.Before(start.Add(3 * time.Minute)); now = now.Add(time.Second / 10) {
		fine.step(now, s)
	}
	end := start.Add(3 * time.Minute)
	fine.step(end, s)
	coarse.step(end, s)
	if d := math.Abs(fine.arousal - coarse.arousal); d > 0.01 {
		t.Errorf("arousal %.4f stepped finely, %.4f in one step", fine.arousal, coarse.arousal)
	}
	if fine.asleep != coarse.asleep {
		t.Errorf("asleep %v finely, %v at once", fine.asleep, coarse.asleep)
	}
}
