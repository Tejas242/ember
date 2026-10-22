package island

import (
	"math"
	"time"
)

// ---- the face over time ----
//
// The posture, the acts, the dance and the stage each say what face Ember
// should wear now. The face is how it gets there, as a face would:
//
//   - A change of eyes happens behind a blink. The old eyes close (60 ms),
//     stay shut a moment (30 ms), and the new ones open (120 ms): ^ ^ never
//     pops into ● ●. Only a flinch is instant.
//   - The eyes jump. Gaze moves in saccades: the eyes stay put until where
//     they should look is 0.3 away (a glance, the pointer, a line to read),
//     then go there in about 100 ms (critically damped, ω 45) and hold. A
//     big jump (1.2 or more) brings a blink with it half the time.
//   - Lids and eye height ease (lids 70 ms down, 100 up; height 45 ms).
//   - A mouth closes before it changes and opens into the new shape
//     (50 ms, then 80 ms). Singing, it sings the words (lipsync.go).
//
// All of it is a few numbers a frame, integrated over real elapsed time.

type faceState struct {
	at   time.Time
	init bool

	eyes             eyeKind // the eyes shown
	swapping         bool
	swapAt           time.Time
	swapFrom, swapTo eyeKind

	eyeH, lid float64

	fixX, fixY float64 // where the eyes are fixed
	x, y       float64 // where they are
	vx, vy     float64
	sacAt      time.Time
	offAt      time.Time // the target has been a little off since

	mouth int
	mk    float64 // how far the mouth is open into its shape, 0..1

	vox  float64 // the mid band, fast
	voxS float64 // slowly: how hard the song is going, for the lids
	sing singer  // the singing mouth (lipsync.go)

	// the dance face, chosen at each phrase
	phrase  int
	dstate  int
	winkAt  time.Time
	winked  time.Time // the last wink: rarely
	shutAt  time.Time // singing: when the eyes last closed or opened
	shut    bool
	readAt  time.Time // singing: a new line, a glance at the words
	lineIdx int
	beatN   int

	md   mood // the mood the face last saw, and since when
	mdAt time.Time
}

// moodSince is how long Ember's mood has been what it is.
func (u *UI) moodSince(now time.Time) time.Duration {
	f := &u.face
	if f.md != u.spot.mood || f.mdAt.IsZero() {
		f.md, f.mdAt = u.spot.mood, now
	}
	return now.Sub(f.mdAt)
}

const (
	swapClose = 0.06
	swapHold  = 0.03
	swapOpen  = 0.12
)

// busy: the face is moving on its own (a change of eyes, a saccade, the mouth).
func (f *faceState) busy(now time.Time) bool {
	if !f.init {
		return false
	}
	return f.swapping || math.Abs(f.x-f.fixX)+math.Abs(f.y-f.fixY) > 0.03 || (f.mk > 0 && f.mk < 1) || now.Sub(f.winkAt) < 600*time.Millisecond
}

// critical moves x toward target as a critically damped spring of rate w,
// exactly over dt (stable at any frame rate).
func critical(x, v *float64, target, w, dt float64) {
	d := *x - target
	e := math.Exp(-w * dt)
	c := *v + w*d
	*x = target + (d+c*dt)*e
	*v = (c - w*(d+c*dt)) * e
}

func approach(x, target, tau, dt float64) float64 {
	if dt <= 0 {
		return x
	}
	return target + (x-target)*math.Exp(-dt/tau)
}

// faceStep turns the face Ember should wear into the one it wears this frame.
func (u *UI) faceStep(ps *pose, now time.Time) {
	f, m := &u.face, u.mind
	dt := now.Sub(f.at).Seconds()
	if !f.init || dt < 0 { // the first frame, or the clock went back: the face is what it should be
		md, mdAt := f.md, f.mdAt
		*f = faceState{md: md, mdAt: mdAt, init: true, eyes: ps.eyes, eyeH: float64(ps.eyeH), lid: float64(ps.lid), fixX: float64(ps.lookX), fixY: float64(ps.lookY),
			x: float64(ps.lookX), y: float64(ps.lookY), mouth: ps.mouth, mk: 1, lineIdx: -3, winked: now.Add(-time.Hour)}
		dt = 0
	}
	f.at = now
	dt = math.Min(dt, 0.25)

	// eyes: a change goes behind a blink
	want := ps.eyes
	over := 0.0 // the lid the change itself brings
	switch {
	case want == eyeWince && f.eyes != eyeWince: // a flinch is instant
		f.eyes, f.swapping = want, false
	case !f.swapping && want != f.eyes:
		f.swapping, f.swapAt, f.swapFrom, f.swapTo = true, now, f.eyes, want
	case f.swapping:
		f.swapTo = want
	}
	shown := f.eyes
	if f.swapping {
		ts := now.Sub(f.swapAt).Seconds()
		switch {
		case ts < swapClose:
			shown = f.swapFrom
			if shown == eyeOpen {
				k := ts / swapClose
				over = k * k
			} else {
				shown, over = eyeOpen, 1 // a glyph closes to a line
			}
		case ts < swapClose+swapHold:
			shown, over = eyeOpen, 1
		default:
			f.eyes, shown = f.swapTo, f.swapTo
			k := math.Min(1, (ts-swapClose-swapHold)/swapOpen)
			if shown == eyeOpen {
				over = 1 - easeOut(k)
			}
			if k >= 1 {
				f.swapping = false
			}
		}
	}
	ps.eyes = shown

	// height and lids ease
	f.eyeH = approach(f.eyeH, float64(ps.eyeH), 0.045, dt)
	ps.eyeH = float32(f.eyeH)
	tau := 0.1
	if float64(ps.lid) > f.lid {
		tau = 0.07
	}
	f.lid = approach(f.lid, float64(ps.lid), tau, dt)
	if ps.lid >= 0.95 { // an act's blink: down at once, up at the lids' pace
		f.lid = 1
	}
	lid := math.Max(f.lid, over)
	if shown == eyeOpen {
		lid = math.Max(lid, m.blink(now))
	}
	ps.lid = float32(math.Min(lid, 1))
	if t := now.Sub(f.winkAt).Seconds(); t >= 0 && t < 0.5 && shown == eyeOpen { // a wink: the right eye, a beat
		w := 0.0
		switch {
		case t < 0.06:
			w = t / 0.06
		case t < 0.36:
			w = 1
		default:
			w = 1 - easeOut((t-0.36)/0.14)
		}
		ps.lidAsym = float32(math.Max(float64(ps.lidAsym), w))
	}

	// gaze: saccades
	tx, ty := float64(ps.lookX), float64(ps.lookY)
	d := math.Hypot(tx-f.fixX, ty-f.fixY)
	since := now.Sub(f.sacAt).Seconds()
	if d > 0.06 {
		if f.offAt.IsZero() {
			f.offAt = now
		}
	} else {
		f.offAt = time.Time{}
	}
	if (d > 0.3 && since > 0.09) || d > 1.2 || (d > 0.06 && now.Sub(f.offAt) > 400*time.Millisecond) {
		f.fixX, f.fixY, f.sacAt, f.offAt = tx, ty, now, time.Time{}
		if d >= 1.2 && m.rng.Float64() < 0.5 {
			m.blinkSoon(now)
		}
	}
	critical(&f.x, &f.vx, f.fixX, 45, dt)
	critical(&f.y, &f.vy, f.fixY, 45, dt)
	ps.lookX, ps.lookY = float32(f.x), float32(f.y)

	// the voice, for the singing mouth and the lids
	if lv, ok := u.audio.Levels(); ok {
		tau := 0.11
		if lv[1] > f.vox {
			tau = 0.035
		}
		f.vox = approach(f.vox, lv[1], tau, dt)
		f.voxS = approach(f.voxS, 0.5*lv[0]+0.5*lv[1], 0.6, dt)
	} else {
		f.vox = approach(f.vox, 0, 0.11, dt)
		f.voxS = approach(f.voxS, 0, 0.6, dt)
	}

	// mouth: closes before it changes, then opens into the new shape
	if ps.mouth != f.mouth {
		if f.mouth == mouthNone || f.mk <= 0 {
			f.mouth, f.mk = ps.mouth, 0
		} else {
			f.mk = math.Max(0, f.mk-dt/0.05)
		}
	} else if f.mk < 1 {
		f.mk = math.Min(1, f.mk+dt/0.08)
	}
	ps.mouth = f.mouth
	ps.mouthK = float32(easeOut(f.mk))
	if f.mouth == mouthNone {
		ps.mouthK = 0
	}
}
