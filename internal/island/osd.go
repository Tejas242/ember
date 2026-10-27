package island

import (
	"image"
	"math"
	"time"
)

// ---- a key pressed while a card, the dashboard or a stage is open ----
//
// The pill answers a volume, brightness or microphone key with its OSD. An
// open view hasn't a pill, so the OSD drops in as a strip along its top,
// where the pill's OSD would be: the view's own top line steps aside under
// it, nothing below moves, and it rises away when the OSD is done.

const (
	stripIn   = 180 * time.Millisecond
	stripOut  = 160 * time.Millisecond
	stripFade = 14 // px under the band that fade into it
)

// osdLive is the OSD being shown, if a key was just pressed.
func (u *UI) osdLive() *moment {
	if m := u.moment; m != nil && m.osd && m.until.After(u.now()) {
		return m
	}
	return nil
}

// stripK is how far the strip is down, 0..1: it drops in from the press
// and rises away as the OSD ends.
func (u *UI) stripK(m *moment, now time.Time) float64 {
	in := easeOut(float64(now.Sub(m.at)) / float64(stripIn))
	out := easeOut(float64(m.until.Sub(now)) / float64(stripOut))
	return math.Max(0, math.Min(in, out))
}

// drawStrip draws the OSD into an open view's content, before it's
// composited. It reports whether the strip is still moving.
func (u *UI) drawStrip(now time.Time) bool {
	m := u.osdLive()
	if m == nil {
		return false
	}
	k := u.stripK(m, now)
	if k <= 0.003 {
		return true
	}
	p, c := u.pal, u.content
	H := u.BarH + 8
	band := float32(k) * H
	x0, x1 := u.shape.x+2, u.shape.x+u.shape.w-2
	// the view's top line steps aside: the band is the island itself
	c.clear(image.Rect(int(x0), 0, int(math.Ceil(float64(x1))), int(math.Ceil(float64(band)))))
	// and what's just under it dissolves into the band's edge, so a line
	// the band half covers reads as passing under it, not cut through
	for i := 0; i < stripFade; i++ {
		y := int(band) + i
		f := float64(i+1) / stripFade
		c.fade(image.Rect(int(x0), y, int(math.Ceil(float64(x1))), y+1), f*f)
	}
	c.roundRect(x0+14, band-1, x1-x0-28, 1, 0.5, alpha(p.line, 0.9*k))

	// what the pill's OSD says, centred where the pill would be
	dy := band - H // it slides down with the band
	cy := u.BarH/2 + 1 + dy
	base := cy + 4.5
	var vw, tw float32
	if m.right != "" {
		vw = max(tabWidth(fPillVal(), m.right), tabWidth(fPillVal(), "100"))
	}
	w := float32(16)
	switch {
	case m.meter:
		w += 10 + meterW
	case m.text != "":
		tw = measure(fPill(), m.text)
		w += 8 + tw
	}
	if vw > 0 {
		w += 10 + vw
	}
	x := u.shape.x + (u.shape.w-w)/2
	level := float32(math.Max(0, math.Min(1, u.meter.x)))
	switch {
	case m.key == "osd:volume" && m.icon != "mute":
		volumeGlyph(c, x+8, cy, 15, level, m.tint)
	case m.icon == "sun":
		sunGlyph(c, x+8, cy, 15, level, m.tint)
	default:
		c.icon(m.icon, x+8, cy, 15, m.tint)
	}
	x += 16
	switch {
	case m.meter:
		x += 10
		c.roundRect(x, cy-2, meterW, 4, 2, alpha(p.fg, 0.14))
		if level > 0 {
			c.roundRect(x, cy-2, max(meterW*level, 4), 4, 2, m.tint)
		}
		c.circle(x+meterW*level, cy, 4.5, m.tint) // a knob: Ember keeps its place in the view
		x += meterW
	case m.text != "":
		x += 8
		c.text(fPill(), m.text, x, base, p.fg)
		x += tw
	}
	if vw > 0 {
		col := p.fg
		if m.tint == p.muted {
			col = p.muted
		}
		u.rollText(c, "strip:value", m.right, fPillVal(), x+10+vw, base, col)
	}
	// the view's live parts under the band would draw over it: they wait
	bandR := image.Rect(int(x0), 0, int(x1), int(band)+stripFade)
	kept := u.lives[:0]
	for _, l := range u.lives {
		if !l.outside && l.r.Overlaps(bandR) {
			continue
		}
		kept = append(kept, l)
	}
	u.lives = kept
	return u.stripMoving(now)
}

// stripMoving: the strip is dropping in or rising away (in between it's
// steady, and steady frames needn't repaint it).
func (u *UI) stripMoving(now time.Time) bool {
	m := u.osdLive()
	return m != nil && (now.Sub(m.at) < stripIn || m.until.Sub(now) < stripOut)
}

// ---- over a fullscreen window ----
//
// Ember's layer (top) is under a fullscreen window, so a key pressed while
// you watch a video went unanswered. Now: under a fullscreen window the pill
// folds up into the top edge and rests (nothing to draw, nothing to wake
// for). A key brings the island up to the overlay layer, where its OSD drops
// out of the top edge alone (no satellite, no card), and once the OSD is
// done it folds up again and goes back under.

// foldAway is how long before the OSD ends the pill starts folding up
// into the top edge, the OSD still in it.
const foldAway = 220 * time.Millisecond

// hiddenUnderFull: a window is fullscreen and no key is being answered (or
// its answer is folding away).
func (u *UI) hiddenUnderFull() bool {
	if !u.full {
		return false
	}
	m := u.osdLive()
	return m == nil || m.until.Sub(u.now()) < foldAway
}

// wantOverlay says which layer the island should be on after this frame: up
// for a key's OSD over a fullscreen window, and until it has folded away.
// (Going fullscreen doesn't bring it up: it folds away underneath.)
func (u *UI) wantOverlay() bool {
	switch {
	case u.full && u.osdLive() != nil:
		u.overlay = true
	case !u.full || (u.sh.settled(0) && u.sh.x < 0.5):
		u.overlay = false
	}
	return u.overlay
}
