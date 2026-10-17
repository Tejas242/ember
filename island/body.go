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
	gravity  = 320.0 // px/s²: a 2.6 px hop is in the air for a quarter of a second
)

// osc is a damped spring around a rest value: x” = −k(x − rest) − c·x' + force.
type osc struct{ x, v float64 }

func (o *osc) step(rest, k, zeta, force, h float64) {
	c := 2 * zeta * math.Sqrt(k)
	o.v += (-(o.x-rest)*k - c*o.v + force) * h
	o.x += o.v * h
}

func (o osc) still(rest, eps float64) bool {
	return math.Abs(o.x-rest) < eps && math.Abs(o.v) < eps*20
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
	y, vy    float64 // height above its floor, up
	sq       osc     // squash, rest 1
	lean     osc     // shear at the tip
	tip      osc     // the tip's lag behind the body
	tipH     osc     // the tongue's height, rest 0
	core     osc     // the inner light, rest 1
	coal     osc     // 0 a flame … 1 a coal
	sqRest   float64 // a held squash (compacting): 1 normally
	leanRest float64 // a held lean (resting its head on the wall asleep)
	coalRest float64

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
	b := &body{sqRest: 1, seed: 1}
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
	b.sq.v += b.vy * 0.06 // stretched as it leaves
	b.tipH.v -= b.vy * 0.02
	b.jumpH = 0
}

func (b *body) squash(v float64)   { b.sq.v -= v }
func (b *body) shove(v float64)    { b.lean.v += v }
func (b *body) gulp(k float64)     { b.tipH.v += 2.6 * k; b.core.v += 2.2 * k; b.sq.v += 0.9 * k }
func (b *body) brighten(v float64) { b.core.v += v }

// sparks: n embers off the tip, fanned from up-left to up-right. The pill's
// top is the screen's edge, so they fly sideways more than up.
func (b *body) sparks(n int, speed float64, from [2]float64) {
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
		if b.y <= 0 { // landing: it squashes by how hard it hit, and maybe bounces once, small
			impact := -b.vy
			b.y = 0
			b.sq.v -= impact * 0.12
			b.tipH.v -= impact * 0.02
			b.tip.v += impact * 0.02 * sign64(b.lean.x)
			if impact > 30 {
				b.vy = impact * 0.18
			} else {
				b.vy = 0
				if b.jumpH > 0 { // a jump asked for in the air goes now
					b.jump(b.jumpH, 0.05)
				}
			}
		}
	}
	sqTarget := b.sqRest
	if b.y > 0 { // in the air it stretches with its speed
		sqTarget += math.Min(math.Abs(b.vy)*0.0035, 0.18)
	}
	b.sq.step(sqTarget, 520, 0.36, 0, h)
	b.sq.x = math.Max(0.55, math.Min(1.6, b.sq.x))

	la := b.lean.v
	b.lean.step(b.leanRest, 150, 0.42, -b.slotA*0.03, h)
	b.tip.step(0, 80, 0.32, -(b.lean.v-la)/h*0.35, h)
	b.tipH.step(0, 260, 0.38, 0, h)
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
			ph.vy += (-ph.y*500 - ph.vy*2*0.35*math.Sqrt(500) - ay*0.02) * h
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

func sign64(x float64) float64 {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return 1
	}
	return 0
}

// busy: something moves enough to want full-rate frames (a hop, a lean
// swinging, particles, headphones landing). The small tail of a motion rides
// whatever rate the island already runs at.
func (b *body) busy() bool {
	if b.airborne() || math.Abs(b.lean.v) > 4 || math.Abs(b.sq.v) > 0.5 || math.Abs(b.tip.v) > 6 || math.Abs(b.tipH.v) > 0.6 ||
		math.Abs(b.core.v) > 0.6 || math.Abs(b.coal.v) > 0.15 {
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
	p.lift += float32(b.y)
	p.sq *= float32(b.sq.x)
	p.lean += float32(b.lean.x)
	p.tip += float32(b.tip.x)
	p.tipH += float32(b.tipH.x)
	p.core *= float32(b.core.x)
	p.coal = max(p.coal, float32(math.Max(0, math.Min(1, b.coal.x))))
}
