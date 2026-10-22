package island

import (
	"fmt"
	"image"
	"math"
	"time"
)

// ---- render ----

// renderDamage draws a frame; it reports whether anything is still moving and
// what changed (all of it; u.dmg has it as separate rectangles). Steady
// frames restore only the live rectangles.
func (u *UI) renderDamage() (animating bool, damage image.Rectangle) {
	now := u.now()
	dt := now.Sub(u.lastTick).Seconds()
	if dt <= 0 || dt > 0.05 {
		dt = 1.0 / 60
	}
	u.lastTick = now
	u.dt = dt
	u.pageEnds(now)
	v := u.currentView()
	key := u.viewKey(v)
	if v != vExpanded {
		u.allRows, u.hero = false, "" // "Show all" and the hero you picked last as long as the card's open
	}
	for _, s := range u.sessions { // an outcome you've now seen leaves the pill
		if !s.Unseen.IsZero() && (u.watching[s.ID] || now.Sub(s.Unseen) > 30*time.Minute || (v == vExpanded && now.Sub(u.trAt) > time.Second)) {
			s.Unseen, u.dirty = time.Time{}, true
		}
	}
	if v != u.lastView || key != u.lastKey {
		u.transition(v, key, now)
		// keep what's on screen to cross-fade it out while the new view grows in,
		// from as visible as it had got (an open interrupted halfway is half there)
		copy(u.prev.Pix, u.content.img.Pix)
		u.prevAlive = !u.trAt.IsZero()
		u.prevK, u.prevAt = easeOut(float64(now.Sub(u.trAt))/float64(fadeIn)), now
		if v != u.lastView {
			u.resetMotion()
		}
		u.lastView, u.lastKey, u.trAt, u.dirty = v, key, now, true
	}
	if u.mind.asleep != u.wasAsleep { // falling asleep or waking dims the pill or lights it
		u.wasAsleep, u.dirty = u.mind.asleep, true
		if u.mind.asleep {
			u.play("sleep", 0) // a yawn, then it banks down to a coal
		} else {
			u.play("wake", 0)
		}
	}
	if v == vCompact { // a chip that counts minutes changes without any event
		if c := u.compactLine().tick(); c != u.lastChip {
			u.lastChip, u.dirty = c, true
		}
	}
	if m := now.Truncate(time.Minute); !m.Equal(u.minute) {
		u.minute, u.dirty = m, true
	}
	if sec := now.Truncate(time.Second); v == vDash && !sec.Equal(u.second) { // its clocks count seconds
		u.second, u.dirty = sec, true
	}
	tw, th := u.measure(v)
	swell := 0.0
	if v == vCompact && u.hover && !u.suppress { // anticipation: the pill swells under the pointer (its content stays put)
		swell = 1
	}
	u.swell.stepK(swell, dt, 700, 42)
	if u.sw.x == 0 {
		u.sw.x, u.sh.x = tw, th
	}
	// opening, the width leads without overshoot and the drop overshoots a
	// little; closing, the card folds up first, critically damped
	switch grow := th > u.sh.x+0.5; {
	case grow && v == vCard: // a request arrives: an overshoot you catch from the corner of your eye
		u.sw.stepK(tw, dt, 420, 39)
		u.sh.stepK(th, dt, 300, 26)
	case grow:
		u.sw.stepK(tw, dt, 420, 39)
		u.sh.stepK(th, dt, 300, 28)
	case th < u.sh.x-0.5:
		u.sw.stepK(tw, dt, 420, 41)
		u.sh.stepK(th, dt, 600, 49)
	default: // the same height: a pill changing width, with a little life
		u.sw.stepK(tw, dt, 380, 32)
		u.sh.stepK(th, dt, 600, 49)
	}
	bt := 0.0 // (the old bubble beside the pill: gone, the pill has a trailing slot)
	if on := v == vCompact && u.secondary(u.primary()) != ""; on != u.trailOn {
		u.trailOn = on
		if on {
			u.mind.notice(now, 1) // something new at the pill's end: Ember looks
		}
	}
	u.bub.step(bt, dt)
	mt := 0.0
	if m := u.moment; m != nil && m.meter {
		mt = m.level
		if m.key != u.meterKey { // a new meter starts where it is; changes glide
			u.meterKey, u.meter = m.key, spring{x: mt}
		}
	}
	u.meter.stepK(mt, dt, 420, 40)
	settled := u.sw.settled(tw) && u.sh.settled(th) && u.bub.settled(bt) && math.Abs(u.meter.x-mt) < 0.003 &&
		math.Abs(u.swell.x-swell) < 0.01 && math.Abs(u.swell.v) < 0.1
	if settled {
		u.sw.x, u.sh.x, u.sw.v, u.sh.v, u.bub.x, u.bub.v = tw, th, 0, 0, bt, 0
		u.swell.x, u.swell.v = swell, 0
		u.meter.x, u.meter.v = mt, 0
	}
	if v == vCompact && u.sh.x > th+24 { // closing: the pill's words wait until the card has nearly folded
		u.trAt = now
	}
	if v != vCompact && u.lastView == v && u.sw.x < tw*0.75 && now.Sub(u.prevAt) < 300*time.Millisecond {
		u.trAt = now // opening: the card's words wait until the shape is mostly open, so no line starts clipped
	}
	p := easeOut(float64(now.Sub(u.trAt)) / float64(fadeIn))
	leaving := u.prevK * (1 - math.Min(1, float64(now.Sub(u.prevAt))/float64(90*time.Millisecond)))
	sliding := now.Sub(u.slideAt) < slideTime
	flashing := now.Sub(u.pressAt) < 160*time.Millisecond
	t := now.Sub(u.start).Seconds()

	// Ember (blinks, glances, gliding to a new spot) and other live parts only
	// need their own rectangles repainted: that's the cheap path below.
	full := u.dirty || !settled || p < 1 || leaving > 0 || sliding || flashing || u.tweening
	u.lastFull = full
	if full {
		u.dirty, u.tweening = false, false
		u.frame++
		if u.frame%300 == 0 {
			u.gcTweens()
		}
		u.hits, u.lives = u.hits[:0], u.lives[:0]
		u.eqShown = false
		u.spot = emberSpot{}
		// the shape, at its current (springing) size
		w, h := float32(u.sw.x+8*u.swell.x), float32(u.sh.x+2*u.swell.x)
		x := (float32(u.W) - w) / 2
		rb := min(h/2, 12)
		if v != vCompact {
			rb = min(h/2, 20)
		}
		u.shape = notch{x: x, w: w, h: h, rb: rb, ear: 6}
		u.cv.clear(u.cv.img.Rect)
		if v == vCard && len(u.cards) > 1 { // more requests wait: their edges peek out under this one
			u.drawPeeks(u.shape, len(u.cards)-1)
		}
		u.sleepK = u.tw("sleep", map[bool]float64{true: 1}[u.mind.asleep && v == vCompact], 0)
		u.cv.drawNotch(u.pal, u.shape, 1-0.16*u.sleepK, int(u.BarH)) // asleep, the pill dims
		// the content, laid out at its final size, revealed by the growing shape
		u.content.clear(u.content.img.Rect)
		u.fadeK = p
		fx := (float32(u.W) - float32(tw)) / 2
		switch v {
		case vCompact: // the pill's ends ride the live shape: words never sit where the shape isn't
			u.hits = append(u.hits, hit{u.shape.bounds(), "pill"})
			u.drawCompact((float32(u.W)-float32(u.sw.x))/2, float32(u.sw.x), float32(th))
		case vCard:
			u.drawCard(u.top(), fx, float32(tw))
		case vExpanded:
			u.hits = append(u.hits, hit{u.shape.bounds(), "panel"})
			u.drawExpanded(fx, float32(tw))
		case vDash:
			u.hits = append(u.hits, hit{u.shape.bounds(), "dash"})
			if u.dashboards[u.dash].Draw(u, fx, float32(tw), now.Sub(u.dashAt)) {
				u.tweening = true
			}
		}
		if u.prevAlive && leaving > 0 { // the old view leaves quickly, drifting up
			composite(u.cv, u.prev, u.shape, leaving, 0, -int(math.Round(3*(1-leaving/math.Max(u.prevK, 0.01)))))
		}
		composite(u.cv, u.content.img, u.shape, p, 0, 0)
		copy(u.base.Pix, u.cv.img.Pix)
		u.stepEmber(dt, now)
		b := u.shape.bounds().Union(u.bubble).Inset(-14) // the shadow, a queued request's edges
		damage = b.Union(u.lastBounds).Intersect(u.cv.img.Rect)
		for _, l := range u.lives {
			if settled || l.outside || u.shape.contains(l.r) || u.bubble.Inset(-2).Intersect(l.r) == l.r { // appear once the shape reaches them
				l.draw(u.cv, t)
			}
		}
		u.drawEmber(t, now)
		// what's drawn after the damage was reckoned must be inside it too:
		// Ember (wherever it is, and where it was) and the live parts, some
		// of which live outside the shape (the halo). Pixels drawn outside
		// the damage reach a buffer anyway and linger on screen as ghosts.
		er := u.emberRect()
		drawn := b.Union(er)
		for _, l := range u.lives {
			drawn = drawn.Union(l.r)
		}
		damage = damage.Union(drawn).Union(u.lastEmber).Intersect(u.cv.img.Rect)
		u.lastBounds, u.lastEmber = drawn, er // the next full frame clears all of this
		u.dmg = append(u.dmg[:0], damage)
		return u.moving(settled, p, sliding, flashing) || leaving > 0, damage
	}
	u.stepEmber(dt, now)
	er := u.emberRect()
	u.dmg = append(u.dmg[:0], er.Union(u.lastEmber))
	for _, l := range u.lives {
		u.dmg = append(u.dmg, l.r)
	}
	for i, r := range u.dmg {
		r = r.Intersect(u.cv.img.Rect)
		u.dmg[i] = r
		for yy := r.Min.Y; yy < r.Max.Y; yy++ {
			o := yy*u.base.Stride + r.Min.X*4
			copy(u.cv.img.Pix[o:o+r.Dx()*4], u.base.Pix[o:o+r.Dx()*4])
		}
		damage = damage.Union(r)
	}
	for _, l := range u.lives {
		l.draw(u.cv, t)
	}
	u.drawEmber(t, now)
	u.lastEmber = er
	return u.moving(settled, p, sliding, flashing), damage
}

// drawPeeks draws the edges of the requests waiting behind the one shown,
// like a stack of cards: narrower, a little lower, a little lighter.
func (u *UI) drawPeeks(n notch, behind int) {
	for i := min(behind, 2); i >= 1; i-- {
		drop := float32(u.tw(fmt.Sprint("peek:", i), 6*float64(i), 0))
		inset := 14 * float32(i)
		pk := notch{x: n.x + inset, w: n.w - 2*inset, h: n.h + drop, rb: n.rb, ear: 0}
		top := int(n.h - n.rb)
		u.cv.fillNotch(pk, 1, alpha(u.pal.line, 0.9-0.25*float64(i)), &pk, 0, top) // a rim, so it reads as a card
		u.cv.fillNotch(pk, 0, alpha(mix(u.pal.island, u.pal.fg, 0.07+0.03*float64(i)), 0.95-0.2*float64(i)), nil, 0, top)
	}
}

func (u *UI) moving(settled bool, p float64, sliding, flashing bool) bool {
	now := u.now()
	if !settled || p < 1 || sliding || flashing || u.tweening || u.act != nil || u.body.flying() || u.travelling || !u.emberSettled() {
		return true
	}
	if u.dancing && u.continuous() { // a dance repaints Ember at its own rate (24 a second): its swing, its face and its blinks ride it
		return false
	}
	return u.mind.blinking(now) || u.face.busy(now) || u.body.swinging()
}

// transition: the view is about to change. Opening the card notes the
// situation, so what you do in it can be learned for next time.
func (u *UI) transition(v view, key string, now time.Time) {
	switch {
	case v == vExpanded && u.lastView != vExpanded:
		u.openCtx, u.openAt = u.habitContext(), now
	}
}

// composite draws src (premultiplied) over dst at strength k, only inside n,
// shifted dx pixels sideways and dy down. Integer arithmetic throughout; transparent
// source pixels (most of them) cost one load.
func composite(dst *canvas, src *image.RGBA, n notch, k float64, dx, dy int) {
	if k <= 0.003 {
		return
	}
	kk := uint32(k*256 + 0.5)
	b := dst.img.Rect
	for py := 0; py < min(int(math.Ceil(float64(n.h))), b.Max.Y); py++ {
		lx, rx, ok := n.span(float32(py) + 0.5)
		if !ok {
			continue
		}
		x0, x1 := max(int(math.Ceil(float64(lx))), 0), min(int(rx), b.Max.X)
		sy := py - dy
		if sy < 0 || sy >= b.Max.Y {
			continue
		}
		o, so := py*dst.img.Stride, sy*dst.img.Stride
		s, d := words(src.Pix[so:so+dst.img.Stride]), words(dst.img.Pix[o:o+dst.img.Stride])
		for px := x0; px < x1; px++ {
			sx := px - dx
			if sx < 0 || sx >= b.Max.X {
				continue
			}
			v := s[sx]
			if v == 0 {
				continue
			}
			if kk < 256 {
				v = scale(v, kk)
			}
			d[px] = over(v, d[px])
		}
	}
}

func (u *UI) liveRects() []image.Rectangle {
	rs := make([]image.Rectangle, len(u.lives))
	for i, l := range u.lives {
		rs[i] = l.r
	}
	return rs
}
