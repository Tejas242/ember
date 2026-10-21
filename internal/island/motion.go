package island

import (
	"math"
)

// Motion: critically damped springs and easing.

type spring struct{ x, v float64 }

// step advances a slightly under-damped spring: quick, with a whisper of overshoot.
func (s *spring) step(target, dt float64) { s.stepK(target, dt, 320, 31) }

// stepK advances the spring by dt. A step longer than a 60th of a second is
// taken in pieces: these springs are stiff (k up to 600), and one 24 fps step
// (41.7 ms) is past what explicit Euler can hold, so a slower frame would make
// them ring up and fly off instead of settling.
func (s *spring) stepK(target, dt, k, d float64) {
	n := max(int(math.Ceil(dt*60-1e-9)), 1)
	h := dt / float64(n)
	for range n {
		s.v += ((target-s.x)*k - s.v*d) * h
		s.x += s.v * h
	}
}

func (s *spring) settled(target float64) bool {
	return math.Abs(target-s.x) < 0.5 && math.Abs(s.v) < 3
}

func easeOut(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return 1 - math.Pow(1-x, 3)
}
