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
// The moves come from the beat's phase, through the body's springs (so the
// tip trails and everything has weight): it dips on the beat, sways over two,
// nods, or on an energetic song bounces, changing move every eight beats. How
// big it dances follows the song's energy; the hi-hats make its flame flicker.

type groove struct {
	sampleAt  time.Time // the last 40 ms of the song it heard
	prev, env float64   // the low band, last sample and its recent level
	energy    float64   // the low band, slowly: how hard the song is going
	hi        float64   // the high band (hi-hats), smoothed
	level     float64   // all three bands, slowly: is there music at all
	heardAt   time.Time // the last time it heard the song
	lastKick  time.Time
	period    float64 // seconds per beat; 0 while it hasn't found one
	phase     float64 // in beats: the whole part counts them, the fraction is where in the beat it is
	lock      float64 // 0..1: how sure it is of the tempo
	at        time.Time
	counted   int // beats counted since it locked on
	move      int // which move it's doing: changes every eight beats
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
		if g.counted%8 == 0 { // a new phrase: a new move
			g.move = (g.move + 1 + int(g.phase)%2) % 3
			if g.energy > 0.62 && g.counted%16 == 0 {
				g.move = moveBounce
			}
		}
	} else {
		g.counted = 0
	}
	return true
}

func (g *groove) dancing() bool { return g.lock > 0.5 }

func (g *groove) reset() { *g = groove{} }

// listen hears the song and dances to it, wherever Ember is, as long as it
// has its headphones on and nothing needs you or went wrong.
func (u *UI) listen(now time.Time) {
	b, g := u.body, &u.groove
	b.danceSq = 1
	if !b.phones.want || !b.phones.seated || !u.danceable() {
		if u.act == nil {
			b.hold = 0
		}
		if !b.phones.want {
			g.reset()
		}
		u.dancing = false
		if u.cfg.Calm && b.phones.seated && u.act == nil && u.mayDance() && u.onStage() { // calm, on a stage: a slow sway
			t := float64(now.UnixMilli()%1_000_000) / 1000
			b.hold = 0.8 * math.Sin(2*math.Pi*t/3.6)
		}
		return
	}
	if lv, at, heard := u.audio.Sample(); heard && at != g.sampleAt { // each 40 ms of the song once
		g.sampleAt = at
		g.hear(lv, now)
	}
	beat := g.advance(now)
	u.dancing = g.dancing()
	if u.act != nil { // an act has the body
		return
	}
	if !u.dancing { // music without a beat to hold on to (soft, acoustic, ambient): a slow sway, as big as the song is loud
		b.hold = 0
		if now.Sub(g.heardAt) < time.Second {
			t := float64(now.UnixMilli()%1_000_000) / 1000
			b.hold = (0.4 + 1.2*math.Min(1, g.level*1.6)) * math.Sin(2*math.Pi*t/3.6)
		}
		return
	}
	f := g.phase - math.Floor(g.phase) // where in the beat
	n := int(math.Floor(g.phase))
	amp := g.lock * (0.45 + 0.8*math.Min(1, g.energy))
	dip := math.Exp(-f * 7) // sharp on the beat, easing back up
	sway := math.Sin(math.Pi * (float64(n) + f))
	switch g.move {
	case moveDip: // down on every beat, the weight shifting a little
		b.danceSq = 1 - 0.11*amp*dip
		b.hold = 0.7 * amp * sway
	case moveSway: // side to side over two beats, a softer dip
		b.danceSq = 1 - 0.05*amp*dip
		b.hold = 1.9 * amp * sway
	case moveNod: // a nod on the beat
		b.danceSq = 1 - 0.08*amp*dip
		b.hold = 0.4 * amp * sway
	case moveBounce: // a small hop on every other beat
		b.danceSq = 1 - 0.06*amp*dip
		b.hold = 0.7 * amp * sway
		if beat && n%2 == 0 {
			b.jump(0.7+0.5*amp, 0.03)
		}
	}
	b.phones.tilt.v += (0.15*sway*amp - b.phones.tilt.x) * 0.2
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
	p.lift += float32(0.6 * g.lock * math.Min(1, g.energy+0.4) * math.Sin(math.Pi*fr)) // up between the beats, down on them
	p.tipH += float32(0.18 * g.hi * flick(float64(now.UnixMilli())/120, 7))            // the hi-hats in its flame

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
