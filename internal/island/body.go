package island

import "math"

// ---- Ember's body: real physics, cheap ----
//
// Ember is a small mass. It jumps and lands under gravity. Its slot carries
// it across the pill, and the slot's acceleration is felt as inertia: the
// body shears back when it's pulled and forward when it stops, and the tip
// lags the body (follow-through). Landing squashes it by how hard it hit,
// leaving the ground stretches it. Sparks and smoke are particles, launched,
// buoyant and dragged by the air. Headphones are a second body that falls
// onto its head, bounces and rides it.
//
// Everything integrates in fixed 1/240 s steps, so it behaves the same at 6
// frames a second or at 60, and it rests (no frames at all) when nothing
// moves. Lengths are px at s = 16, the pill's size; time is seconds.

const (
	physStep = 1.0 / 240
	gravity  = 700.0 // px/s²: a 2.6 px hop is in the air for 170 ms (a 16 px creature falls fast)
)

// osc is a damped spring around a rest value: x” = −k(x − rest) − c·x' + force.
type osc struct{ x, v float64 }

func (o *osc) step(rest, k, zeta, force, h float64) {
	c := 2 * zeta * math.Sqrt(k)
	o.v += (-(o.x-rest)*k - c*o.v + force) * h
	o.x += o.v * h
}

type particle struct {
	x, y, vx, vy float64 // from Ember's base centre, y up
	age, life    float64 // a negative age waits to be born
	smoke        bool
	live         bool
}

type phones struct {
	want   bool    // it wants them on (a song is playing, headphones connected)
	shown  float64 // 0..1
	y, vy  float64 // height above where they sit on its head
	seated bool
	tilt   osc
}

type body struct {
	still    bool    // [ember] motion = "calm": no hops, no sparks
	y, vy    float64 // height above its floor, up
	sq       osc     // squash, rest 1
	lean     osc     // shear at the tip
	tip      osc     // the tip's lag behind the body
	tipH     osc     // the tongue's height, rest 0
	core     osc     // the inner light, rest 1
	coal     osc     // 0 a flame … 1 a coal
	sqRest   float64 // a held squash (compacting): 1 normally
	hold     float64 // a lean held by a muscle (a shake) or the dance's weight on one foot
	danceSq  float64 // the dance's knees: the height it is asked to stand at, 1 at rest
	danceUp  float64 // the dance's lift off its floor, px (a bounce leaves the ground a little)
	bob      osc     // that lift, through a spring: the weight lands a hair after the beat
	headA    float64 // how hard the top of its head is accelerating, px/s² up, smoothed: its tongue and headphones feel it
	leanK    float64 // its stiffness while a muscle holds it, 0 otherwise
	leanRest float64 // a held lean (resting its head on the wall asleep)
	coalRest float64
	breath   float64 // where it is in a breath, 0..1 (posture counts it)
	breathT  float64 // and when it last counted

	slotV  float64 // the slot's velocity at the last step: its change is the inertia felt
	slotA  float64 // and that acceleration, smoothed a little
	launch float64 // a launch waiting: seconds left (anticipation), 0 none
	jumpH  float64
	parts  [10]particle
	phones phones
	acc    float64
	seed   uint32
}

func newBody() *body {
	b := &body{sqRest: 1, danceSq: 1, seed: 1}
	b.sq.x, b.core.x = 1, 1
	return b
}

func (b *body) airborne() bool { return b.y > 0.01 || b.vy > 0.01 || b.launch > 0 }

// rnd is a tiny deterministic generator: sparks scatter, the same way each run.
func (b *body) rnd() float64 {
	b.seed = b.seed*1664525 + 1013904223
	return float64(b.seed>>8) / float64(1<<24)
}

// jump: crouch for ant seconds, then leave the ground for a hop h px high.
// Another jump while in the air waits until it lands.
func (b *body) jump(h, ant float64) {
	if b.still {
		return
	}
	if b.airborne() {
		b.jumpH = math.Max(b.jumpH, h)
		return
	}
	b.jumpH = h
	if ant <= 0 {
		b.takeOff()
		return
	}
	b.launch = ant
	b.sq.v -= 2 + h*0.5 // it gathers itself
}

func (b *body) takeOff() {
	b.launch = 0
	b.vy = math.Sqrt(2 * gravity * b.jumpH)
	b.sq.v += b.vy * 0.04 // stretched as it leaves
	b.tipH.v -= b.vy * 0.014
	b.jumpH = 0
}

func (b *body) squash(v float64)   { b.sq.v -= v }
func (b *body) shove(v float64)    { b.lean.v += v }
func (b *body) gulp(k float64)     { b.tipH.v += 2.6 * k; b.core.v += 2.2 * k; b.sq.v += 0.9 * k }
func (b *body) brighten(v float64) { b.core.v += v }

// sparks: n embers off the tip, fanned from up-left to up-right. The pill's
// top is the screen's edge, so they fly sideways more than up.
func (b *body) sparks(n int, speed float64, from [2]float64) {
	if b.still {
		return
	}
	for i := 0; i < n; i++ {
		f := (float64(i) + 0.5) / float64(n)
		ang := math.Pi*(0.12+0.76*f) + (b.rnd()-0.5)*0.3
		v := speed * (0.8 + 0.4*b.rnd())
		b.add(particle{x: from[0], y: from[1], vx: math.Cos(ang) * v * 1.4, vy: math.Sin(ang) * v * 0.55,
			life: 0.55 + 0.25*b.rnd(), age: -0.03 * float64(i%2)})
	}
}

// smoke: n puffs off the tip, rising slowly, swelling, thinning.
func (b *body) smoke(n int, from [2]float64) {
	for i := 0; i < n; i++ {
		b.add(particle{x: from[0] + (b.rnd()-0.5)*1.5, y: from[1], vx: (b.rnd() - 0.5) * 6, vy: 6,
			life: 1.1, age: -0.2 * float64(i), smoke: true})
	}
}

func (b *body) add(p particle) {
	p.live = true
	for i := range b.parts {
		if !b.parts[i].live {
			b.parts[i] = p
			return
		}
	}
	b.parts[0] = p // full: the oldest goes
}

// step advances the body by dt. slotV is how fast its slot moves sideways
// (px/s, at s = 16): a change in it is felt as inertia.
func (b *body) step(dt, slotV float64) {
	b.acc += math.Min(dt, 0.5)
	for b.acc >= physStep-1e-9 { // a hair of slack: 1/6 s is 40 steps, not 39 and a remainder
		b.acc -= physStep
		b.tick(physStep, slotV)
	}
}

func (b *body) tick(h, slotV float64) {
	a := (slotV - b.slotV) / h
	b.slotV = slotV
	b.slotA += (a - b.slotA) * 0.35 // a hard edge to a new slot is still felt, not as one spike

	if b.launch > 0 {
		if b.launch -= h; b.launch <= 0 {
			b.takeOff()
		}
	}
	vy0 := b.vy
	if b.y > 0 || b.vy > 0 {
		b.vy -= gravity * h
		b.y += b.vy * h
		if b.y <= 0 { // landing: it squashes by how hard it hit (the energy goes into the squash, not a bounce)
			impact := -b.vy
			b.y, b.vy = 0, 0
			b.sq.v -= impact * 0.082
			b.tipH.v -= impact * 0.014
			b.tip.v += impact * 0.02 * math.Max(-1, math.Min(1, b.lean.x)) // the tip whips the way it leant
			if b.jumpH > 0 {                                               // a jump asked for in the air goes now
				b.jump(b.jumpH, 0.05)
			}
		}
	}
	sqTarget := b.sqRest * b.danceSq
	if b.y > 0 { // in the air it stretches with its speed
		sqTarget += math.Min(math.Abs(b.vy)*0.0024, 0.18)
	}
	sqV, bobV := b.sq.v, b.bob.v
	b.sq.step(sqTarget, 520, 0.36, 0, h)
	b.sq.x = math.Max(0.55, math.Min(1.6, b.sq.x))
	b.bob.step(b.danceUp, 420, 0.55, 0, h)
	if b.bob.x < 0 {
		b.bob.x, b.bob.v = 0, math.Max(0, b.bob.v)
	}
	// the top of its head: the body stretching (≈14 px tall at s = 16) and lifting.
	// Whatever is loose on it (the tongue, the headphones) lags that.
	b.headA += (((b.sq.v-sqV)*14+(b.bob.v-bobV))/h - b.headA) * 0.25

	la := b.lean.v
	lk, lz := 150.0, 0.42
	if b.leanK > 0 { // a muscle holds it (a head shake): stiff and quick
		lk, lz = b.leanK, 0.5
	}
	b.lean.step(b.leanRest+b.hold, lk, lz, -b.slotA*0.03, h)
	b.tip.step(0, 80, 0.32, -(b.lean.v-la)/h*0.35, h)
	b.tipH.step(0, 260, 0.38, -b.headA*0.012, h)
	b.core.step(1, 200, 0.55, 0, h)
	b.coal.step(b.coalRest, 90, 0.7, 0, h)

	for i := range b.parts {
		p := &b.parts[i]
		if !p.live {
			continue
		}
		p.age += h
		if p.age < 0 {
			continue
		}
		if p.age > p.life {
			p.live = false
			continue
		}
		if p.smoke { // buoyant, slow, curling
			p.vy += 14 * h
			p.vx += math.Sin(p.age*7+p.x) * 10 * h
			p.vx -= p.vx * 1.8 * h
			p.vy -= p.vy * 1.2 * h
		} else { // a spark: light, it floats up a little, the air slows it
			p.vy += 30 * h
			p.vx -= p.vx * 3.2 * h
			p.vy -= p.vy * 3.2 * h
		}
		p.x += p.vx * h
		p.y += p.vy * h
	}

	ph := &b.phones
	switch {
	case ph.want && ph.shown == 0: // they appear above its head and fall
		ph.shown, ph.y, ph.vy, ph.seated = 0.01, 9, 0, false
		ph.tilt = osc{x: 0.25}
	case ph.want:
		ph.shown = math.Min(1, ph.shown+h/0.12)
		if !ph.seated {
			ph.vy -= gravity * h
			ph.y += ph.vy * h
			if ph.y <= 0 {
				impact := -ph.vy
				ph.y = 0
				b.sq.v -= impact * 0.03 // it feels them land
				if impact > 12 {
					ph.vy = impact * 0.32
					ph.tilt.v += (b.rnd() - 0.5) * 6
				} else {
					ph.vy, ph.seated = 0, true
				}
			}
		} else { // they ride its head: a little behind when it moves, held by the band
			ay := (b.vy - vy0) / h
			ph.vy += (-ph.y*500 - ph.vy*2*0.35*math.Sqrt(500) - ay*0.02 - b.headA*0.55) * h
			ph.y += ph.vy * h
			if ph.y < -0.6 {
				ph.y, ph.vy = -0.6, 0
			}
		}
		ph.tilt.step(0, 160, 0.3, -(b.lean.v-la)/h*0.004, h)
	case ph.shown > 0: // taken off: they lift away and fade
		if ph.seated {
			ph.seated, ph.vy = false, 40
		}
		ph.vy -= gravity * 0.3 * h
		ph.y += ph.vy * h
		ph.shown = math.Max(0, ph.shown-h/0.3)
	}
}

// busy: something moves enough to want full-rate frames (a hop, a lean
// swinging, particles, headphones landing). The small tail of a motion rides
// whatever rate the island already runs at.
func (b *body) busy() bool { return b.swinging() || b.flying() }

// swinging: the body's springs move fast (a lean, a squash, a tip). A dance
// swings all the time, and rides the dance's own rate.
func (b *body) swinging() bool {
	return math.Abs(b.lean.v) > 4 || math.Abs(b.sq.v) > 0.5 || math.Abs(b.tip.v) > 6 || math.Abs(b.tipH.v) > 0.6 ||
		math.Abs(b.core.v) > 0.6 || math.Abs(b.coal.v) > 0.15 || math.Abs(b.bob.v) > 1
}

// flying: a hop, sparks or smoke, the headphones landing or leaving.
func (b *body) flying() bool {
	if b.airborne() {
		return true
	}
	for _, p := range b.parts {
		if p.live {
			return true
		}
	}
	ph := b.phones
	return (ph.want && (!ph.seated || ph.shown < 1)) || (!ph.want && ph.shown > 0)
}

// pose writes the body's state into p.
func (b *body) pose(p *pose) {
	p.lift += float32(b.y + b.bob.x)
	p.sq *= float32(b.sq.x)
	p.lean += float32(b.lean.x)
	p.tip += float32(b.tip.x)
	p.tipH += float32(b.tipH.x)
	p.core *= float32(b.core.x)
	p.coal = max(p.coal, float32(math.Max(0, math.Min(1, b.coal.x))))
}
