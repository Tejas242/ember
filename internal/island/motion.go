package island

import (
	"math"
)

// Motion: critically damped springs and easing.

type spring struct{ x, v float64 }

// step advances a slightly under-damped spring: quick, with a whisper of overshoot.
func (s *spring) step(target, dt float64) { s.stepK(target, dt, 320, 31) }

func (s *spring) stepK(target, dt, k, d float64) {
	s.v += ((target-s.x)*k - s.v*d) * dt
	s.x += s.v * dt
}

func (s *spring) settled(target float64) bool {
	return math.Abs(target-s.x) < 0.5 && math.Abs(s.v) < 3
}

func easeOut(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return 1 - math.Pow(1-x, 3)
}
