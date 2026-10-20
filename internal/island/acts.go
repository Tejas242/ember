package island

import (
	"image"
	"image/color"
	"math"
	"time"
)

// ---- what Ember does: acts, and how it holds itself ----
//
// An act is a short moment of acting for something real that just happened:
// impulses to the body at its beats (the physics does the rest) and a face
// over its timeline. A posture is how it holds itself for as long as a
// situation lasts: the mood's face, a held squash, headphones on, a coal.

type act struct {
	kind  string
	at    time.Time
	a     float64 // its size: how big a celebration
	beats int     // impulses already given
}

// actLen is how long each act's face lasts.
var actLen = map[string]float64{
	"ignite": 0.65, "needs": 0.9, "done": 1.3, "stopped": 1.2, "wince": 0.45, "nod": 0.5, "shake": 0.6,
	"sleep": 2.6, "wake": 1.1, "stretch": 1.9, "yawn": 1.2, "eyeroll": 0.75, "poke": 0.7, "puff": 1.6,
	"hide": 2.8, "relief": 0.7, "shutter": 0.55, "pop": 0.45, "charge": 1.0, "hello": 1.0, "hop": 0.6,
	"listen": 0.5,
}

// a flourish is an act Ember plays on its own: it goes through the gate.
func (u *UI) flourish(kind string, every time.Duration) bool {
	if !u.may(kind, every) {
		return false
	}
	u.play(kind, 0)
	return true
}

// play starts an act. A reaction you caused replaces whatever was playing;
// a quieter act doesn't cut a louder one short.
func (u *UI) play(kind string, a float64) {
	now := u.now()
	if cur := u.act; cur != nil && now.Sub(cur.at).Seconds() < actLen[cur.kind]*0.6 && rank(kind) < rank(cur.kind) {
		return
	}
	u.act = &act{kind: kind, at: now, a: a}
	u.actStep(now)
	u.dirty = true
}

func rank(kind string) int {
	switch kind {
	case "stopped", "needs", "done", "nod", "shake", "poke", "puff", "hide", "eyeroll", "stretch":
		return 3
	case "wince", "ignite", "wake", "charge", "hello", "shutter":
		return 2
	}
	return 1
}

// cheer is how Ember celebrates a Done: as big as the work was long, with a
// flare if it was earned (a long or hard-won turn that ended well), and an
// eye-roll first if Claude opened with "You're absolutely right".
type cheer struct {
	size        float64
	flare, roll bool
}

func (u *UI) cheer(c cheer) {
	a := c.size
	if a == 0 {
		a = 0.3
	}
	u.flare = c.flare && u.bond != nil && u.bond.cooled("flare", 20*time.Minute, u.now())
	if u.flare {
		a = math.Max(a, 0.85)
	}
	u.play("done", a)
}

// doneSize is how big a Done's celebration is, 0..1, from how long the work
// took: 20 s is small, 80 s middling, five minutes or more the whole thing.
func doneSize(took time.Duration) float64 {
	return math.Max(0, math.Min(1, math.Log2(math.Max(took.Seconds(), 1)/10)/5))
}

// hopFor is how long a hop h px high is in the air.
func hopFor(h float64) float64 { return 2 * math.Sqrt(2*h/gravity) }

var (
	tipFrom   = [2]float64{1.4, 14}
	smokeFrom = [2]float64{1, 13}
)

// actStep gives the body the impulses of the act's beats that are due.
func (u *UI) actStep(now time.Time) {
	ac, b := u.act, u.body
	if ac == nil {
		return
	}
	t := now.Sub(ac.at).Seconds()
	beat := func(i int, at float64, f func()) {
		if ac.beats == i && t >= at {
			ac.beats++
			f()
		}
	}
	for n := -1; n != ac.beats; { // fire every beat that's due, in order
		n = ac.beats
		switch ac.kind {
		case "ignite": // gathers itself, catches, pops up throwing two sparks
			beat(0, 0, func() { b.squash(2.4) })
			beat(1, 0.08, func() { b.jump(1.4, 0); b.brighten(5); b.sparks(2, 34, tipFrom) })
		case "needs": // a crouch and a hop, then a smaller one: "hey"
			beat(0, 0, func() { b.jump(2.6, 0.08) })
			beat(1, 0.62, func() { b.jump(1.2, 0) })
		case "done": // a celebration as big as the work was long
			a := ac.a
			h := 1.2 + 1.7*a
			ant := 0.04 + 0.06*a
			land := ant + hopFor(h)
			beat(0, 0, func() { b.jump(h, ant); b.brighten(2 + 3*a) })
			beat(1, land, func() {
				if a > 0.2 {
					b.sparks(int(math.Round(2+4*a)), 36+10*a, tipFrom)
				}
				if u.flare { // earned: a flare off the tip, and it stands taller
					u.flare = false
					b.tipH.v += 5
					b.sparks(2, 48, [2]float64{tipFrom[0], tipFrom[1] + 1})
				}
			})
			beat(2, land+0.14, func() {
				if a > 0.45 {
					b.jump(0.5*h, 0.04)
				}
			})
		case "stopped": // a flinch back, the flame gutters, a puff of smoke
			beat(0, 0, func() { b.squash(3); b.shove(-26); b.tipH.v -= 3 })
			beat(1, 0.12, func() { b.smoke(2, smokeFrom) })
		case "wince":
			beat(0, 0, func() { b.squash(2); b.shove(-14) })
		case "nod": // two dips, the second smaller
			beat(0, 0, func() { b.squash(2.4) })
			beat(1, 0.2, func() { b.squash(1.5) })
		case "shake": // its head goes side to side, held by a muscle, its base planted: the eyes lead, the tip trails
		case "wake": // the coal catches: it shoots up and looks around
			beat(0, 0, func() { b.brighten(8); b.sq.v += 4; b.jump(1.2, 0.05) })
		case "stretch": // a big stretch, a tremble at the top, down again
			beat(0, 0, func() { b.sq.v += 5 })
			for i := 0; i < 5; i++ {
				beat(1+i, 0.3+float64(i)*0.11, func() { b.shove(float64(6 * (1 - 2*(i%2)))) })
			}
			beat(6, 0.9, func() { b.squash(3.5) })
		case "yawn":
			beat(0, 0.15, func() { b.sq.v += 2.6 })
		case "eyeroll":
		case "poke": // squashed by your finger, springs back, looks at you
			beat(0, 0, func() { b.squash(3.4); b.tipH.v += 2 })
		case "puff":
			beat(0, 0, func() { b.sq.v += 3; b.brighten(3) })
		case "hide":
		case "relief": // a slow blink and an exhale
			beat(0, 0, func() { b.squash(1.2) })
			beat(1, 0.25, func() { b.sq.v += 2.2 })
		case "shutter": // the flash makes it blink hard
			beat(0, 0, func() { b.squash(1.6); b.shove(-6) })
		case "pop": // the conversation is compacted: it springs back up
			beat(0, 0, func() { b.sq.v += 4.5 })
		case "charge": // a gulp of power, a few sparks
			beat(0, 0, func() { b.gulp(1.6); b.brighten(4) })
			beat(1, 0.18, func() { b.sparks(3, 30, tipFrom) })
		case "hello": // you're back
			beat(0, 0, func() { b.jump(1.5, 0.06) })
		case "hop":
			beat(0, 0, func() { b.jump(1.6, 0.05) })
		case "listen": // headphones on: it settles into the song
			beat(0, 0, func() { b.squash(1) })
		}
	}
	// held parts of an act: the shake is a muscle stepping the lean, faster than the body would swing by itself
	b.hold, b.leanK = 0, 0
	if ac.kind == "shake" && t < 0.6 {
		b.leanK = 420
		switch {
		case t < 0.14:
			b.hold = 1.8
		case t < 0.29:
			b.hold = -1.8
		case t < 0.43:
			b.hold = 1.2
		}
	}
	if t > actLen[ac.kind] {
		b.hold, b.leanK = 0, 0
		u.act = nil
		if ac.kind == "eyeroll" {
			c := u.afterRoll
			c.roll = false
			u.cheer(c)
		}
	}
}

// actFace lays the act's face over the posture's.
func (u *UI) actFace(p *pose, now time.Time) {
	ac := u.act
	if ac == nil {
		return
	}
	t := now.Sub(ac.at).Seconds()
	in := func(a, b float64) bool { return t >= a && t < b }
	switch ac.kind {
	case "ignite":
		switch {
		case t < 0.09:
			p.eyeH = 1
		default:
			p.eyeH = 4 + float32(math.Exp(-(t-0.09)/0.25))
		}
		p.flash = max(p.flash, float32(0.6*math.Exp(-math.Max(0, t-0.08)/0.1)*b2f(t >= 0.08)))
	case "needs":
		p.eyeH, p.lookY, p.mouth = 5, -1, mouthO
		p.lookX = 0
		if in(0.46, 0.52) || in(0.6, 0.66) { // a double blink: "hey"
			p.eyeH = 1
		}
	case "done":
		p.eyes, p.lid, p.slant = eyeHappy, 0, 0
		if ac.a > 0.6 {
			p.cheeks = float32(math.Sin(math.Min(1, t/1.3) * math.Pi))
		}
	case "stopped":
		if t < 0.28 {
			p.eyes = eyeWince
		} else {
			p.slant, p.lid, p.lookY, p.mouth = -0.6, 0.2, 1, mouthFlat
		}
	case "wince", "shutter":
		if t < 0.22 {
			p.eyes = eyeWince
		}
	case "nod":
		p.lookY = 1
		if in(0.12, 0.42) {
			p.eyes = eyeHappy
		}
	case "shake":
		p.lid = max(p.lid, 0.3)
	case "sleep": // a yawn, then its eyes close
		mo := smooth01((t-0.25)/0.35) * (1 - smooth01((t-0.95)/0.25))
		if mo > 0.3 {
			p.mouth = mouthYawn
		}
		if t > 0.2 {
			p.eyes = eyeShut
		} else {
			p.lid = float32(t / 0.2)
		}
	case "wake":
		p.eyes, p.lid = eyeOpen, 0
		p.eyeH = 4 + float32(math.Exp(-math.Max(0, t-0.1)/0.3))
		switch {
		case in(0.45, 0.65):
			p.lookX = -2
		case in(0.7, 0.92):
			p.lookX = 2
		}
	case "stretch":
		switch {
		case t < 0.9:
			p.eyes = eyeShut
			if in(0.2, 0.75) {
				p.mouth = mouthYawn
			}
		case t < 1.1:
			p.eyeH = 1
		default:
			p.lookX, p.lookY = 2, 0 // a look at the clock
		}
	case "yawn":
		if in(0.25, 0.85) {
			p.mouth, p.eyes = mouthYawn, eyeShut
		}
	case "eyeroll": // half-lidded, the eyes sweep up and across
		p.lid = 0.45
		p.lookY = -1
		p.lookX = float32(-2 + 4*smooth01(t/0.5))
	case "poke":
		if t < 0.15 {
			p.eyes = eyeWince
		} else {
			p.eyeH, p.lookY, p.lookX = 5, -1, 0
		}
	case "puff":
		p.scale *= 1 + 0.12*float32(math.Sin(math.Min(1, t/1.6)*math.Pi))
		p.lid, p.slant = 0.35, 0.4
	case "hide": // shrinks to a coal; the eyes peek out first, then it grows back
		k := smooth01(t/0.25) * (1 - smooth01((t-2.2)/0.5))
		p.coal = max(p.coal, float32(k))
		if t > 1.9 && t < 2.3 {
			p.eyes, p.eyeH, p.lid = eyeOpen, 2, 0
		}
	case "relief":
		p.lid = max(p.lid, float32(0.6*math.Sin(math.Min(1, t/0.4)*math.Pi)))
	case "pop":
		p.eyeH = 5
	case "charge", "hello", "hop":
		p.eyes = eyeHappy
		if ac.kind == "hello" {
			p.cheeks = float32(math.Sin(math.Min(1, t/0.8) * math.Pi))
		}
	}
}

func smooth01(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return x * x * (3 - 2*x)
}

// ---- organic flicker: value noise at non-harmonic rates ----

func hash1(i int64, seed uint32) float64 {
	x := uint32(i)*0x9E3779B1 ^ seed*0x85EBCA77
	x ^= x >> 15
	x *= 0x2C1B3C6D
	x ^= x >> 12
	x *= 0x297A2D39
	x ^= x >> 15
	return float64(x)/float64(1<<32)*2 - 1
}

func vnoise(t float64, seed uint32) float64 {
	i := math.Floor(t)
	f := t - i
	a, b := hash1(int64(i), seed), hash1(int64(i)+1, seed)
	f = f * f * (3 - 2*f)
	return a + (b-a)*f
}

// flick: gusts, licks and a shiver (1.7, 4.3, 9.7 Hz at speed 1), -1 … 1.
func flick(t float64, seed uint32) float64 {
	return 0.55*vnoise(t*1.7, seed) + 0.3*vnoise(t*4.3, seed+7) + 0.15*vnoise(t*9.7, seed+13)
}

// ---- postures ----

// posture is how Ember holds itself now, from the situation: the mood's face,
// the breath and the flicker (driven by its energy and Claude's pace), and the
// held parts of the body (headphones, a coal, a squash held while compacting).
func (u *UI) posture(p *pose, now time.Time, t float64) {
	m, b := u.mind, u.body
	en := m.arousal
	heat := math.Min(1, m.rate/15)
	md := u.spot.mood

	// breathing: in 40 %, out 60 %, its pace following its energy
	period := 5.2 - 3.2*en
	ph := math.Mod(t/period, 1)
	var br float64
	if ph < 0.4 {
		br = smooth01(ph / 0.4)
	} else {
		br = 1 - smooth01((ph-0.4)/0.6)
	}
	p.sq *= float32(1 + 0.045*(br-0.5))
	p.lift += float32(0.3 * br)

	// the flicker: noise, faster and taller the more Claude does; the two tongues on their own seeds
	sp, amp := 1+1.4*heat, 0.06+0.12*heat
	switch md {
	case moodWork:
		sp, amp = sp*1.15, amp+0.04
	case moodError:
		sp, amp = sp*0.6, amp*0.6
	}
	p.tipH += float32(amp * flick(t*sp, 1))
	p.sideH += float32(amp * 1.2 * flick(t*sp*1.13, 40))
	p.tip += float32((0.5 + 0.9*heat) * flick(t*sp*0.8, 90))
	p.core *= float32(1 + 0.1*heat + 0.05*flick(t*sp*1.3, 200))

	// the mood's face
	p.eyeH = 3
	look := float32(u.look.x) * 1.5
	p.lookX = look
	switch md {
	case moodWork:
		p.eyeH = 4
		if math.Abs(u.look.x) < 0.2 {
			p.lookX = 1 // reading the label
		}
		if heat > 0.5 {
			p.slant = float32(0.35 * (heat - 0.5) * 2)
		}
	case moodWait:
		p.eyeH, p.lookY, p.mouth = 4, -1, mouthO
	case moodHappy:
		p.eyes = eyeHappy
		p.tipH += 0.15
	case moodError:
		p.slant, p.lookY, p.mouth = -0.6, 1, mouthFlat // slumped: the posture carries it, not darkness
		p.tipH -= 0.18
		p.sideH -= 0.4 // a guttering flame, not two horns
		p.scale *= 0.94
	case moodNudge: // gone quiet: heavy lids, a side-eye at the timer
		p.lid, p.lookX = 0.45, 2
		p.tipH -= 0.1
	}
	// tired: drowsy lids at low energy and late at night
	p.lid = max(p.lid, float32((1-math.Min(m.lids(now), 1))*0.8))
	if h := now.Hour(); (h >= 23 || h < 5) && !m.away && md == moodIdle {
		p.lid = max(p.lid, 0.4)
		p.tipH -= 0.12
	}
	// hovered: it perks up (eyes wide for a moment), then follows the pointer
	if u.hover {
		if now.Sub(u.hoverAt) < 250*time.Millisecond {
			p.eyeH = 5
		}
		p.tipH += 0.12
	}
	// a blink: the lids come down in whole pixels; the eye isn't squashed
	if bl := m.blink(now); bl > 0 && p.eyes == eyeOpen {
		p.lid = max(p.lid, float32(math.Min(1, bl*1.2)))
	}

	// held parts
	b.sqRest, b.leanRest, b.coalRest = 1, 0, 0
	if u.compacting() { // squashed while Claude squeezes its context
		b.sqRest = 0.82
		p.lid = max(p.lid, 0.4)
	}
	yawning := u.act != nil && u.act.kind == "sleep" && now.Sub(u.act.at) < 1100*time.Millisecond
	if m.asleep && u.lastView == vCompact && !yawning { // asleep: a coal, resting its head on the pill's wall
		b.coalRest, b.leanRest = 0.55, -1.4 // banked low, a little tongue left: a flame asleep, not a cookie
		p.eyes = eyeShut
		p.core *= float32(1 + 0.3*(0.5-0.5*math.Cos(2*math.Pi*t/4.2))) // its light breathes
	}
	if u.extFocus() { // a plugin asks for focus (edge's contest round): game face, eyes front
		p.slant, p.lid, p.lookX = 0.35, max(p.lid, 0.15), 0
	}
	if u.worried() {
		p.scale *= 0.93
		p.lid = max(p.lid, 0.3)
	}
	if pw := u.power; pw != nil && pw.state == 2 && pw.pct < 15 { // it runs on your battery
		p.scale *= float32(0.85 + 0.15*pw.pct/15)
	}
	b.phones.want = u.wantPhones()
	u.danceFace(p)
	u.heroPose(p) // a plugin's dashboard: how its hero asks Ember to stand
	// petted: the pointer resting on it in the card
	if u.hoverKey == "ember" && u.lastView != vCompact {
		if u.petSince.IsZero() {
			u.petSince = now
		}
		if now.Sub(u.petSince) > 1500*time.Millisecond {
			p.eyes, p.cheeks = eyeHappy, 0.8
			p.core *= 1.15
		}
	} else {
		u.petSince = time.Time{}
	}
	// a moment beside it: it looks at what it's showing you
	if mo := u.moment; mo != nil && u.lastView == vCompact && (mo.icon != "" || mo.thumb != nil) && u.act == nil {
		if !mo.meter { // riding a meter it looks ahead; otherwise at the icon beside it
			p.lookX = 2
		}
		switch {
		case mo.icon == "mic-off", mo.key == "osd:volume" && mo.icon == "mute": // shh
			p.mouth, p.lid = mouthFlat, max(p.lid, 0.5)
		case mo.icon == "warning":
			p.slant = -0.5
		}
	}
}

func (u *UI) compacting() bool {
	for _, s := range u.sessions {
		if s.State == "work" && s.Label == "Compacting the conversation" {
			return true
		}
	}
	return false
}

// worried: steps keep failing in the turn Ember is watching.
func (u *UI) worried() bool {
	for _, s := range u.sessions {
		if s.status() == "work" && s.Turn.streak >= 3 {
			return true
		}
	}
	return false
}

// wantPhones: a song is playing (and Ember is listening), or headphones just connected.
func (u *UI) wantPhones() bool {
	if u.casting {
		return false
	}
	if m := u.moment; m != nil && m.icon == "headphones" && m.sub == "Connected" {
		return true
	}
	return u.track != nil && u.track.Playing && u.cfg.Music
}

// ---- drawing Ember ----

func (u *UI) emberFire() fire {
	var f fire
	cs := []*color.NRGBA{&f.hot, &f.mid, &f.tip}
	for i, c := range cs {
		*c = color.NRGBA{uint8(u.emberCol[i][0]), uint8(u.emberCol[i][1]), uint8(u.emberCol[i][2]), 255}
	}
	return f
}

// blendFire moves Ember's colours toward its mood's fire. A stop is felt at
// once (green to red through brown is mud); other moods blend over ~150 ms.
func (u *UI) blendFire(dt float64) {
	tgt := fireOf(u.spot.mood)
	k := math.Min(1, dt*7)
	if u.spot.mood == moodError || u.spot.mood == moodWait || u.emberCol[1][0] == 0 { // orange to blue through grey reads as a fault
		k = 1
	}
	for i, c := range []color.NRGBA{tgt.hot, tgt.mid, tgt.tip} {
		u.emberCol[i][0] += (float64(c.R) - u.emberCol[i][0]) * k
		u.emberCol[i][1] += (float64(c.G) - u.emberCol[i][1]) * k
		u.emberCol[i][2] += (float64(c.B) - u.emberCol[i][2]) * k
	}
}

// emberRect covers Ember and everything it throws: sparks, smoke, its
// headphones landing, its z's when asleep.
func (u *UI) emberRect() image.Rectangle {
	if !u.spot.show {
		return image.Rectangle{}
	}
	x, y, s := float32(u.ex.x), float32(u.ey.x), float32(u.es.x)
	if s < 1 {
		return image.Rectangle{}
	}
	k := max(s/16, 0.7)
	r := image.Rect(int(x-14*k-4), int(y-s*0.9-12*k), int(x+16*k+4), int(y+s*0.75+3))
	if u.mind.asleep {
		r = r.Union(image.Rect(int(x), 0, int(x+s+10), int(y)))
	}
	return r
}

func (u *UI) drawEmber(t float64, now time.Time) {
	if !u.spot.show {
		return
	}
	x, y, s := float32(u.ex.x), float32(u.ey.x), float32(u.es.x)
	if s < 2 {
		return
	}
	m := u.mind
	ps := restPose()
	u.posture(&ps, now, t)
	u.actFace(&ps, now)
	u.body.pose(&ps)
	ps.lookX += float32(u.body.lean.x*0.45 + u.body.lean.v*0.02) // the eyes lead the head
	ps.scale *= 1 + 0.05*float32(m.arousal-0.5)                  // a touch bigger when lively
	f := u.emberFire()
	base := y + 0.45*s
	if !m.asleep { // the light it casts on the island, brighter with its energy
		gy, gr := base-s*0.45, s*1.5
		r := image.Rect(int(x-gr), int(gy-gr), int(x+gr)+1, int(gy+gr)+1).Intersect(u.emberRect())
		u.cv.glowInside(r, x, gy, gr, f.mid, 0.08+0.16*m.arousal+0.25*float64(ps.flash))
	}
	g := newGeo(x, base, s, ps)
	u.cv.drawFlame(u.pal, x, base, s, ps, f, 1)
	u.drawPhones(g)
	u.drawParts(g, f)
	if m.asleep && s > 6 && u.lastView == vCompact { // z's drift up and fade
		fz := face(fSemiBold, 7)
		for i := 0; i < 2; i++ {
			k := math.Mod(t/2.4+float64(i)*0.5, 1)
			zx := x + s*0.4 + float32(k)*5
			zy := y - s*0.25 - float32(k)*s*0.7
			u.cv.text(fz, "z", zx, zy, alpha(u.pal.muted, math.Sin(k*math.Pi)*0.9))
		}
	}
}

// drawParts draws the sparks and the smoke, from the body's particles.
func (u *UI) drawParts(g flameGeo, f fire) {
	k := g.k
	for _, p := range u.body.parts {
		if !p.live || p.age < 0 {
			continue
		}
		q := p.age / p.life
		px, py := g.cx+float32(p.x)*k, g.base-float32(p.y)*k
		if p.smoke {
			a := 0.55 * math.Pow(1-q, 1.3) * math.Min(1, q*6)
			u.cv.circle(px, py, (1+2.2*float32(q))*k, alpha(u.pal.muted, a))
			continue
		}
		// a spark is one bright pixel, hot whatever the mood, cooling as it flies
		col := mix(rgb(0xffe7a0), rgb(0xff9a3c), math.Min(1, q*1.4))
		if u.spot.mood == moodWait {
			col = mix(rgb(0xe6f3ff), rgb(0x6cb0ff), math.Min(1, q*1.4))
		}
		a := math.Min(1, 1.25*math.Pow(1-q, 1.2))
		if k < 1.3 {
			u.cv.roundRect(float32(math.Round(float64(px))), float32(math.Round(float64(py))), 1, 1, 0, alpha(col, a))
		} else {
			u.cv.circle(px, py, 0.6*k, alpha(col, a))
		}
	}
}

// drawPhones draws Ember's headphones: a band across the flame, a cup on
// each side. They fall onto its head, bounce and ride it.
func (u *UI) drawPhones(g flameGeo) {
	ph := u.body.phones
	if ph.shown <= 0 || g.ps.coal > 0.5 {
		return
	}
	c, r := g.headAt()
	k := g.k
	c.y -= float32(ph.y) * k
	tilt := float32(ph.tilt.x)
	a := ph.shown
	// one cool colour against the warm fire: a warm headset melts into the face (monkey ears)
	col := alpha(phonesCol, a)
	rr := r + 0.4*k
	u.cv.arc(c.x, c.y+0.4*k, rr, max(k, 1), math.Pi+0.05+tilt*0.3, 2*math.Pi-0.05+tilt*0.3, col)
	snap := func(v float32) float32 { return float32(math.Round(float64(v))) }
	w, h := max(snap(2*k), 2), max(snap(4*k), 4)
	for _, side := range []float32{-1, 1} {
		cx := c.x + side*rr
		cy := c.y + 0.6*k + side*tilt*1.5*k
		u.cv.roundRect(snap(cx-w/2), snap(cy-h/2), w, h, w*0.3, col)
	}
}
