package island

import (
	"math"
	"time"
)

// ---- groove: Ember's sense of rhythm ----
//
// Reacting to each kick after it lands is always a little late, and a jolt
// isn't a dance. So Ember counts, like a dancer does: it hears kicks (a
// sudden jump in the bass that stands over its recent level), learns the
// tempo from the gaps between them, and keeps its own beat going between
// them, a phase that runs at the tempo and is nudged toward each kick it
// hears (a phase-locked loop). It moves on the beat, not after it, and keeps
// going through a bar where the kick drops out.
//
// The dance is a dancer's, not a metronome's (see dancer below): its knees
// sink into each beat and rise off it, its weight steps from side to side,
// and the springs of its body carry the rest (the weight lands a hair after
// the beat, the tongue and the headphones lag the head). It keeps a move for
// a phrase and changes it with the song's sections, blending over a beat;
// the first beat of a bar lands heavier; nothing repeats exactly.

type groove struct {
	sampleAt  time.Time // the last 40 ms of the song it heard
	prev, env float64   // the low band, last sample and its recent level
	energy    float64   // the low band, slowly: how hard the song is going
	sect      float64   // low and high together over the last few seconds: this part of the song
	mean      float64   // the same over the last twenty-odd: the song on the whole
	heardN    int       // samples heard of this song
	hi        float64   // the high band (hi-hats), smoothed
	level     float64   // all three bands, slowly: is there music at all
	heardAt   time.Time // the last time it heard the song
	lastKick  time.Time
	period    float64 // seconds per beat; 0 while it hasn't found one
	phase     float64 // in beats: the whole part counts them, the fraction is where in the beat it is
	lock      float64 // 0..1: how sure it is of the tempo
	at        time.Time
	counted   int // beats counted since it locked on
	move      int // which move leads now (the dancer chooses it)
}

const (
	moveDip = iota
	moveSway
	moveNod
	moveBounce
)

// hear takes one sample of the song's bands (low, mid, high).
func (g *groove) hear(v [3]float64, now time.Time) {
	low := v[0]
	kick := low-g.prev > 0.12 && low > g.env*1.3 && now.Sub(g.lastKick) > 260*time.Millisecond
	g.env += (low - g.env) * 0.08
	g.energy += (low - g.energy) * 0.03
	g.hi += (v[2] - g.hi) * 0.2
	g.level += ((v[0]+v[1]+v[2])/3 - g.level) * 0.05
	e := 0.6*low + 0.4*v[2]
	g.heardN++                                                     // until it has heard a part (and a song) long enough, both are the average so far
	g.sect += (e - g.sect) * math.Max(0.012, 1/float64(g.heardN))  // ~3 s
	g.mean += (e - g.mean) * math.Max(0.0025, 1/float64(g.heardN)) // ~16 s
	g.heardAt = now
	g.prev = low
	if kick {
		g.kick(now)
	}
}

// kick: a kick drum, now. It teaches the tempo and pulls the count toward it.
func (g *groove) kick(now time.Time) {
	last := g.lastKick
	g.lastKick = now
	if last.IsZero() {
		return
	}
	iv := now.Sub(last).Seconds()
	if iv > 2.4 {
		return
	}
	// the gap may be one beat, two, or half of one: take the reading nearest the tempo it has
	c := iv
	if g.period > 0 {
		for _, m := range []float64{0.5, 2, 3, 1.0 / 3} {
			if math.Abs(iv*m-g.period) < math.Abs(c-g.period) {
				c = iv * m
			}
		}
	} else {
		for c < 0.4 {
			c *= 2
		}
		for c > 1.0 {
			c /= 2
		}
	}
	switch {
	case g.period > 0 && math.Abs(c-g.period)/g.period < 0.18: // it fits: settle the tempo, pull the count onto the kick
		g.period += (c - g.period) * 0.25
		g.lock = math.Min(1, g.lock+0.2)
		err := g.phase - math.Round(g.phase)
		g.phase -= err * (0.35 + 0.4*g.lock)
	case g.lock > 0.25: // a stray kick (a fill, a syncopation): doubt a little
		g.lock -= 0.15
	default: // a new tempo: start counting from this kick
		g.period, g.lock = c, 0.2
		g.phase = math.Round(g.phase)
	}
}

// advance runs its count to now. It says whether a new beat began.
func (g *groove) advance(now time.Time) bool {
	dt := now.Sub(g.at).Seconds()
	g.at = now
	if g.period <= 0 || dt <= 0 || dt > 1 {
		return false
	}
	before := math.Floor(g.phase)
	g.phase += dt / g.period
	if now.Sub(g.lastKick).Seconds() > 4*g.period { // the beat has stopped: it stops being sure
		g.lock = math.Max(0, g.lock-dt*0.4)
	}
	if math.Floor(g.phase) == before {
		return false
	}
	if g.lock > 0.5 {
		g.counted++
	} else {
		g.counted = 0
	}
	return true
}

func (g *groove) dancing() bool { return g.lock > 0.5 }

func (g *groove) reset() { *g = groove{} }

// ---- the dancer ----
//
// Every part of the dance is a function of its own count, a smoothed copy of
// the groove's phase, so it is on the beat by construction; the body's
// springs turn those targets into weight.
//
//   - Knees: each beat is a bounce, a ball's arc: the weight drops into the
//     beat faster and faster, turns sharply at the bottom (the hit, on the
//     beat) and rebounds, slowing to a hang at the top just before the "and".
//     The target leads the count by the squash spring's lag (45 ms), so the
//     lowest point the eye sees lands on the beat, give or take 10 ms of
//     human; the spring rounds the turn and sinks a hair past it (the weight
//     settling), and the tongue and headphones lag the head.
//   - Weight: it steps from one side to the other, the transfer starting a
//     little before the beat and arriving on it, then it stays there (a
//     person's weight sits on one foot; a sine never rests). While it dances
//     its lean is held by a muscle (a stiffer spring), so the step arrives
//     when it's meant to instead of a fifth of a second late.
//   - Bars: the first beat of four lands heavier, the third a bit; the first
//     of sixteen (a phrase) heavier still. Each beat and step is ±10–12 % its
//     own size: nothing repeats exactly.
//   - Moves: a groove (bounce and two-step), a head-bob (deeper, little
//     weight), a half-time sway (a bounce every two beats, a ghost of one in
//     between, for a quieter part), a bounce with hops (for a loud one). On a
//     fast song it steps every other beat, as people do. It keeps a move at
//     least eight beats, chooses at a phrase from how this part of the song
//     compares to the whole, and blends into the new one over about a beat.
//   - In and out: it starts dancing small as it starts to find the beat and
//     grows over two beats as it's sure; when the song stops it eases out
//     over a third of a second, still on its count, and the springs settle.

type dancer struct {
	at     time.Time
	phase  float64 // its count, beats
	period float64
	in     float64    // 0..1: how much it dances to the beat
	free   float64    // 0..1: how much it sways without one
	inten  float64    // how big: the song's part against the whole
	w      [4]float64 // each move's share
	move   int
	moveAt int     // the beat the move began
	sway   float64 // the beatless sway's phase, radians
	hopFor int     // the beat a hop was launched for
	hit    int     // the phrase whose first beat threw sparks
	heard  time.Time
}

type moveShape struct {
	down, up float64 // the knees: how far it sinks on the beat, rises off it, px at its head
	peak     float64 // where in the bounce it hangs highest (share of it)
	every    float64 // beats a bounce takes (2: half time)
	ghost    float64 // half time: the little bounce on the beat between, share of down
	lift     float64 // px its heels leave the floor at the top
	step     float64 // the weight shift, px at its tip
	side     float64 // beats on each side
	xfer     float64 // beats the transfer takes
}

var moveShapes = [...]moveShape{
	moveDip:    {down: 1.1, up: 0.55, peak: 0.45, every: 1, lift: 0.1, step: 1.1, side: 1, xfer: 0.5},
	moveSway:   {down: 0.75, up: 0.45, peak: 0.5, every: 2, ghost: 0.3, step: 1.8, side: 2, xfer: 1.3},
	moveNod:    {down: 1.45, up: 0.4, peak: 0.4, every: 1, lift: 0.05, step: 0.45, side: 1, xfer: 0.4},
	moveBounce: {down: 1.25, up: 0.75, peak: 0.45, every: 1, lift: 0.35, step: 0.9, side: 1, xfer: 0.45},
}

// barAccent: how heavily each beat of a bar lands.
var barAccent = [4]float64{1.25, 0.85, 1.05, 0.9}

func beatAccent(k int) float64 {
	a := barAccent[k&3] * (1 + 0.1*hash1(int64(k), 31))
	if k&15 == 0 {
		a *= 1.3
	}
	return a
}

// knees is the height a move asks for at count q, px at its head (− sunk),
// and its lift off the floor. slow: bounce every other beat (a fast song).
func (m moveShape) knees(q float64, slow bool) (y, lift float64) {
	ev := m.every
	if slow {
		ev *= 2
	}
	x := q / ev
	n := math.Floor(x)
	f := x - n
	bottom := func(k float64) float64 { return -m.down * beatAccent(int(k*ev)) }
	if f < m.peak { // the rebound: quick off the bottom, slowing to a hang
		k := 1 - f/m.peak
		y = bottom(n) + (m.up-bottom(n))*(1-k*k)
	} else { // the drop into the next beat, faster and faster
		k := (f - m.peak) / (1 - m.peak)
		y = m.up + (bottom(n+1)-m.up)*k*k
	}
	if ev > 1 && m.ghost > 0 { // the beats between: a ghost of a bounce
		d := math.Abs(q-math.Floor(q/ev)*ev-ev/2) / 0.35
		if d < 1 {
			y -= m.ghost * m.down * (1 - d*d) * (1 - d*d)
		}
	}
	if y > 0 {
		lift = m.lift * smooth01(y/m.up)
	}
	return y, lift
}

// weight is where its weight is at count q, px at its tip: −1 … 1 (a little more on the right) times step.
func (m moveShape) weight(q float64, slow bool) float64 {
	every, xfer := m.side, m.xfer
	if slow && every < 2 {
		every, xfer = 2, xfer*1.6
	}
	lead := 0.6 * xfer
	x := (q + lead) / every
	k := math.Floor(x)
	g := (x - k) * every
	side := func(k float64) float64 {
		s := 1 - 2*float64(int64(k)&1)
		if s < 0 {
			s *= 0.92
		}
		return s * (1 + 0.12*hash1(int64(k), 47))
	}
	return m.step * (side(k-1) + (side(k)-side(k-1))*smooth01(g/xfer))
}

// listen hears the song and dances to it, wherever Ember is, as long as it
// has its headphones on and nothing needs you or went wrong.
const (
	kneesLead = 0.045 // s: the squash spring's lag to the bottom of a bounce
	stepLead  = 0.05  // s: the lean's, held by the dance's muscle
)

func (u *UI) listen(now time.Time) {
	b, g, d := u.body, &u.groove, &u.dancer
	dt := math.Max(0, math.Min(0.1, now.Sub(d.at).Seconds()))
	d.at = now
	t := float64(now.UnixMilli()%1_000_000) / 1000
	ease := func(x *float64, to, tau float64) { *x += (to - *x) * math.Min(1, dt/tau) }

	inTo, freeTo, swayA := 0.0, 0.0, 0.0
	listening := b.phones.want && b.phones.seated && u.danceable()
	if !listening {
		if !b.phones.want {
			g.reset()
			d.heard = time.Time{}
		}
		u.dancing = false
		if u.cfg.Calm && b.phones.seated && u.act == nil && u.mayDance() && u.onStage() { // calm, on a stage: a slow sway
			freeTo, swayA = 1, 0.8
		}
	} else {
		if lv, at, heard := u.audio.Sample(); heard && at != g.sampleAt { // each 40 ms of the song once
			g.sampleAt = at
			g.hear(lv, now)
			if d.heard.IsZero() {
				d.heard = now
			}
		}
		g.advance(now)
		u.dancing = g.dancing()
		if g.period > 0 { // it dances small as it starts to find the beat, fully once it's sure
			inTo = smooth01((g.lock - 0.15) / 0.45)
		}
		if now.Sub(g.heardAt) < time.Second && g.lock < 0.3 && !d.heard.IsZero() && now.Sub(d.heard) > 2500*time.Millisecond {
			freeTo, swayA = 1, 0.4+1.2*math.Min(1, g.level*1.6) // music without a beat to hold on to: a slow sway, as big as the song is loud
		}
	}

	// its count: the groove's, smoothed (a kick's correction is felt over ~120 ms, not as a jump); on its own when the groove has none
	if g.period > 0 {
		d.period = g.period
		d.phase += dt / g.period
		if err := g.phase - d.phase; math.Abs(err) > 0.5 {
			d.phase = g.phase
		} else {
			d.phase += err * math.Min(1, dt*8)
		}
	} else if d.period > 0 {
		d.phase += dt / d.period
	}

	if u.act != nil { // an act has the body; the dance comes back in when it's over
		d.in, d.free = 0, 0
		b.danceSq, b.danceUp = 1, 0
		return
	}
	if inTo > d.in && d.period > 0 {
		ease(&d.in, inTo, 2*d.period)
	} else {
		ease(&d.in, inTo, 0.3)
	}
	ease(&d.free, freeTo, 1.5)
	if d.free < 0.01 {
		d.sway = 0 // a sway begins from the middle
	}

	// how big: this part of the song against the whole, and how hard it goes
	rel := 1.0
	if g.mean > 0.05 {
		rel = g.sect / g.mean
	}
	inten := (0.7 + 0.5*math.Min(1, g.energy)) * math.Max(0.6, math.Min(1.3, 1+1.2*(rel-1)))
	if u.onStage() {
		inten *= 1.15
	}
	if d.inten == 0 {
		d.inten = inten
	}
	ease(&d.inten, inten, 1.5)

	// a move for a phrase, chosen by the song's part; at least eight beats each
	// the knees lead the count by the squash spring's lag, so the eye sees the weight land on the beat;
	// the step leads by the lean's; a dancer is never on the same hair twice (±10 ms)
	var q, qw float64
	if d.period > 0 {
		hum := 0.010 * vnoise(d.phase*0.7, 11)
		q = d.phase + (kneesLead+hum)/d.period
		qw = d.phase + (stepLead+hum)/d.period
	}
	slow := d.period > 0 && d.period < 0.47 // past ~128 bpm: it bounces and steps every other beat
	n := int(math.Floor(q))
	if d.in > 0.5 && n%8 == 0 && n-d.moveAt >= 8 {
		want := d.move
		switch {
		case rel < 0.9:
			want = moveSway
		case rel > 1.1 && g.energy > 0.55:
			want = moveBounce
		case d.move == moveSway || d.move == moveBounce:
			want = moveDip
		case n-d.moveAt >= 16: // a groove held for a while turns into a head-bob, and back
			want = moveDip + moveNod - d.move
		}
		if want != d.move {
			d.move, d.moveAt = want, n
		}
	} else if d.in < 0.05 {
		d.moveAt = n - 8
	}
	g.move = d.move
	var sum float64
	for i := range d.w {
		to := 0.0
		if i == d.move {
			to = 1
		}
		if d.period > 0 {
			ease(&d.w[i], to, 0.8*d.period)
		}
		sum += d.w[i]
	}
	if sum < 1e-6 {
		d.w[d.move], sum = 1, 1
	}

	var y, up, lean float64
	for i, m := range moveShapes {
		if d.w[i] < 0.002 {
			continue
		}
		yi, li := m.knees(q, slow)
		y += d.w[i] / sum * yi
		up += d.w[i] / sum * li
		lean += d.w[i] / sum * m.weight(qw, slow)
	}
	a := d.in * d.inten
	b.danceSq = 1 + a*y/14
	b.danceUp = a * up

	// without a beat: a slow sway that rests a moment at each side and dips as it passes the middle
	d.sway += dt * 2 * math.Pi / (3.6 * (1 + 0.15*vnoise(t/5, 61)))
	sw := math.Sin(d.sway)
	sway := math.Copysign(math.Pow(math.Abs(sw), 0.7), sw)
	b.danceSq -= d.free * (1 - d.in) * 0.025 * math.Cos(2*d.sway)

	idle := 0.0
	if !u.mind.asleep { // even standing still, its weight drifts
		idle = 0.3 * vnoise(t/6, 71)
	}
	b.hold = a*lean + (1-d.in)*(d.free*swayA*sway+(1-d.free)*idle)
	b.leanK = 0
	if d.in > 0.01 { // dancing, its weight is held by a muscle: it arrives on the beat, not after it
		b.leanK = 150 + 250*d.in
	}

	// the bounce: a small hop on every other beat, launched so it touches down 20 ms before
	// the beat and the landing's squash bottoms out on it
	if d.w[moveBounce] > 0.6 && d.in > 0.8 && !b.still && d.period > 0 {
		h := math.Max(0.5, math.Min(1.0, 0.6+0.5*(d.inten-0.8)))
		const ant = 0.05
		next := int(math.Floor(d.phase)) + 1
		if next%2 == 0 && next != d.hopFor && d.phase+0.02/d.period >= float64(next)-(hopFor(h)+ant)/d.period {
			d.hopFor = next
			b.jump(h, ant)
		}
	}
	// a phrase's first beat on a stage, a song running hot: two sparks off its tip as the weight lands
	if u.onStage() && n%16 == 0 && n != d.hit && q-float64(n) < 0.1 && d.in > 0.8 && d.inten > 1 && u.energyT() > 0.45 {
		d.hit = n
		b.sparks(2, 30, tipFrom)
	}
}

// danceFace: how Ember's face rides the song. Not one face for the whole
// song: at each phrase (eight beats) it picks one, by a roll weighted with
// how hard the song is going: lost in it (^ ^ and a smile), grooving (eyes
// open, lids relaxed and a little down, the lids dipping on the beat where a
// pixel can show it, a soft smile), or beaming at you (eyes open and
// up, a smile, a blush, now and then a wink). The first phrase after it
// finds the beat it grooves, eyes open, still listening. Its blinks land on
// beats, and a phrase begins with one.
func (u *UI) danceFace(p *pose) {
	g, f := &u.groove, &u.face
	if u.act != nil || !u.body.phones.seated {
		return
	}
	now := u.now()
	if !u.dancing { // swaying to something soft: eyes closed, listening; now and then they open a while, heavy, and close again
		if now.Sub(g.heardAt) < time.Second && g.level > 0.15 {
			t := float64(now.UnixMilli()%1_000_000) / 1000
			if math.Mod(t+4*vnoise(t/23, 5), 9) < 6.5 {
				p.eyes = eyeShut
			} else {
				p.lid, p.lookX, p.lookY = 0.4, -0.7, 0.6
			}
		}
		return
	}
	fr := g.phase - math.Floor(g.phase)
	p.tipH += float32(0.18 * g.hi * flick(float64(now.UnixMilli())/120, 7)) // the hi-hats in its flame

	e := math.Max(0, math.Min(1, (f.voxS-0.35)/0.3)) // how hard the song is going
	onBeat := int(math.Floor(g.phase)) != f.beatN    // a new beat began (whatever the frame rate)
	f.beatN = int(math.Floor(g.phase))
	if ph := g.counted / 8; ph != f.phrase {
		f.phrase = ph
		prev := f.dstate
		switch r := (hash1(int64(ph), 77) + 1) / 2; {
		case ph == 0:
			f.dstate = danceGroove
		case r < 0.25+0.5*e:
			f.dstate = danceLost
		case r < 0.47+0.5*e && prev != danceBeam:
			f.dstate = danceBeam
			if now.Sub(f.winked) > 30*time.Second && (hash1(int64(ph), 91)+1)/2 < 0.4 {
				f.winkAt, f.winked = now, now
			}
		default:
			f.dstate = danceGroove
		}
		if f.dstate == prev && prev != danceLost {
			u.mind.blinkSoon(now)
		}
	}
	if g.counted%8 == 4 { // half way: the song can change its mind for it
		switch {
		case f.dstate == danceLost && e < 0.15:
			f.dstate = danceGroove
		case f.dstate == danceGroove && e > 0.85:
			f.dstate = danceLost
		}
	}
	switch f.dstate {
	case danceLost:
		p.eyes, p.mouth = eyeHappy, mouthSmile
	case danceGroove: // relaxed, not sleepy: light lids, a soft smile, a look to one side then the other each half phrase
		side := float32(1 - 2*((g.counted/4)%2))
		p.eyes, p.lid, p.mouth = eyeOpen, float32(0.12+0.18*e), mouthSmile
		p.lookX, p.lookY = 0.6*side, 0.3
		p.lidFine = float32(0.2 * math.Exp(-fr*7))
	case danceBeam:
		p.eyes, p.eyeH, p.lid, p.lookX, p.lookY, p.mouth = eyeOpen, 3.4, 0.05, 0, -0.5, mouthSmile
	}
	if g.move == moveNod && fr < 0.3 {
		p.lookY = 1
	}
	if p.eyes == eyeOpen && onBeat && u.mind.blinkDue(now, 600*time.Millisecond) { // a blink about due: on this beat
		u.mind.blinkSoon(now)
	}
}

const (
	danceGroove = iota
	danceLost
	danceBeam
)
