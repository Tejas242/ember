package island

import (
	"math"
	"time"
)

// The frame budget. Drawing is the cheap part of Ember, but every frame
// still wakes the compositor, so each state gets a rate and no more:
//
//	state             on AC    on battery   under 20 %
//	quiet             4        3            1.5
//	asleep            2.5      1.7          0.7
//	Claude working    6        4            2        (+ blinks and glances)
//	card open         12       12           12       (60 only while it morphs)
//	dance, equaliser  24       15           none     (static headphones)
//	full screen, away 0        0            0
//
// Blinks, glances and swings are bursts: they ride a 24 a second tick instead
// of the compositor's 60. A steady frame whose flame moved less than half a
// pixel is not sent at all. TestSimBudget holds the numbers.

// burstEvery is the tick of a blink, a glance or a swing: 24 a second, 8 under
// a fifth of battery.
func (u *UI) burstEvery() time.Duration { return u.tick(24, 20, 8) }

// awayGate: away this long (idle time included) and nothing is drawn.
const awayGate = 2 * time.Minute

// budgetState is what the budget remembers between frames.
type budgetState struct {
	on      bool      // the live loop (and the sim) set it; render tests draw every frame as before
	burst   bool      // the last frame was a blink, a glance or a swing: tick fast
	skipped bool      // the last frame had nothing worth sending
	last    pose      // the pose last drawn
	lastAt  time.Time // and when
	ps      pose      // the pose this frame computed for the skip check ...
	psOK    bool      // ... for drawEmber to use instead of computing it again
}

// tier: 0 on AC (or no battery), 1 on battery, 2 on battery under a fifth.
func (u *UI) tier() int {
	switch pw := u.power; {
	case pw == nil || pw.state != 2:
		return 0
	case pw.pct < 20:
		return 2
	}
	return 1
}

// tick picks a state's interval, in frames a second, for the power tier.
func (u *UI) tick(ac, battery, low float64) time.Duration {
	return time.Duration(float64(time.Second) / []float64{ac, battery, low}[u.tier()])
}

// gated: nobody can see Ember (you've been away a while, or a full-screen
// window covers it and it has folded away), so nothing is drawn. The mind
// keeps no frame-by-frame state: it integrates over elapsed time, and the
// first frame back shows where it got to.
func (u *UI) gated() bool {
	if !u.bud.on {
		return false
	}
	m := u.mind
	if m.away && u.now().Sub(m.awaySince)+u.cfg.Away() >= awayGate {
		return true
	}
	return u.hiddenUnderFull() && u.sh.settled(0)
}

// blinkLive: the lids are moving (a blink lasts 0.22 s, a double one 0.52 s).
func (m *mind) blinkLive(now time.Time) bool {
	t := now.Sub(m.blinkAt).Seconds()
	if m.blinkTwice {
		return t >= 0 && t < 0.52
	}
	return t >= 0 && t < 0.22
}

// eyesMoving: the gaze is travelling to a new spot (the faint jitter of a
// resting eye is at most 0.05 and isn't travel).
func (u *UI) eyesMoving() bool {
	return math.Abs(u.look.x-u.lookTarget(u.now())) > 0.08 || u.face.busy(u.now())
}

// baseTick is how often a state with something always going on repaints.
func (u *UI) baseTick() time.Duration {
	switch {
	case u.lastView != vCompact && (u.eqShown || u.dancing):
		return u.tick(24, 15, 8)
	case u.lastView != vCompact:
		return time.Second / 12
	case u.eqShown || u.dancing || (u.body.phones.seated && u.danceable()):
		return u.tick(24, 15, 8) // a dance needs every beat
	}
	return u.tick(6, 4, 2)
}

// quietFrame: Ember is steady and its flame moved less than half a pixel
// since the last frame sent, so this one need not be. Anything that isn't
// the pose (a spark, the equaliser, z's) never skips.
func (u *UI) quietFrame(t float64, now time.Time) bool {
	b := &u.bud
	if !b.on || len(u.lives) > 0 || !u.spot.show || u.mind.asleep || u.body.flying() || u.dirty {
		return false
	}
	if b.lastAt.IsZero() || now.Sub(b.lastAt) > time.Second || now.Before(b.lastAt) {
		return false // now and then, whatever slow thing the pose doesn't hold (colours, light)
	}
	b.ps, b.psOK = u.emberPose(t, now), true
	if poseMoved(b.last, b.ps, float32(u.es.x)) {
		return false
	}
	b.psOK = false // not drawn: the next frame computes its own (the breath is counted, so that's safe)
	return true
}

// poseMoved: does anything a viewer could see differ by half a pixel (or by
// a visible step, for faces and light)? s is Ember's size in px.
func poseMoved(a, b pose, s float32) bool {
	abs := func(x float32) float32 { return float32(math.Abs(float64(x))) }
	k := s / 16
	switch {
	case a.eyes != b.eyes || a.mouth != b.mouth || a.sing != b.sing:
		return true
	case abs(a.sq-b.sq)*s*0.5 >= 0.5, abs(a.scale-b.scale)*s*0.5 >= 0.5, abs(a.lift-b.lift)*k >= 0.5, abs(a.lean-b.lean) >= 0.5,
		abs(a.tip-b.tip)*k >= 0.5, abs(a.tipH-b.tipH)*s*0.4 >= 0.5, abs(a.sideH-b.sideH)*s*0.4 >= 0.5, abs(a.coal-b.coal)*s*0.5 >= 0.5:
		return true
	case abs(a.lookX-b.lookX)*k >= 0.35, abs(a.lookY-b.lookY)*k >= 0.35, abs(a.eyeH-b.eyeH)*k >= 0.35:
		return true
	}
	return abs(a.lid-b.lid) > 0.04 || abs(a.lidAsym-b.lidAsym) > 0.04 || abs(a.lidFine-b.lidFine) > 0.04 || abs(a.slant-b.slant) > 0.04 ||
		abs(a.mouthK-b.mouthK) > 0.04 || abs(a.cheeks-b.cheeks) > 0.04 || abs(a.core-b.core) > 0.05 || abs(a.flash-b.flash) > 0.03 || abs(a.dim-b.dim) > 0.03
}

// syncAudio: the equaliser and the dance listen to the song only while one
// plays, and not at all on a nearly empty battery (unless a stage needs the
// voice). parec is a process: stopping it saves a wake-up every 20 ms.
func (a *app) syncAudio() {
	u := a.ui
	if u.audio == nil {
		return
	}
	u.audio.Set(u.track != nil && u.track.Playing && u.cfg.Music && (u.tier() < 2 || u.onStage()))
}
