package island

import (
	"image"
	"strings"
)

// Finding a page that opens from Ember (opens.go): in the card, with the
// pointer on Ember, its hint in a small label under it; a quiet control in
// the music row for a page that opens with the music; and once ever, the
// first time the card opens while Claude works and the context holds, a
// line under the headline saying so ("Click me to vibe"). The pill never
// hints: Ember dancing in its headphones is the hint.

const hintH = 18 // the one-time hint's line

// hoverLabel draws the opener's hint under Ember while the pointer is on it.
func (u *UI) hoverLabel(x, y float32) {
	_, hint := u.opener()
	k := u.hl("ember")
	if hint == "" || k < 0.01 {
		return
	}
	p := u.pal
	f := fCaption()
	w := measure(f, hint) + 14
	lx, ly := x+railX-w/2, y+41
	u.dc.roundRect(lx, ly, w, 16, 8, alpha(mix(p.island, p.fg, 0.14), k))
	u.dc.text(f, hint, lx+7, ly+11.5, alpha(p.fg, k))
}

// showHint: the one-time hint shows in this card (and is remembered as shown).
func (u *UI) showHint() bool {
	if u.bond == nil || u.lastView != vExpanded {
		return false
	}
	if u.bond.Hinted.Equal(u.openAt) && !u.openAt.IsZero() { // shown in this opening: it stays until the card closes
		return true
	}
	if !u.bond.Hinted.IsZero() || !u.senses().working || u.entryCtx() != "music" {
		return false
	}
	if name, _ := u.opener(); name == "" {
		return false
	}
	u.bond.Hinted, u.bond.dirty = u.openAt, true
	return true
}

// hintLine is the hint's words: "Click me to vibe".
func (u *UI) hintLine(x, y float32) {
	_, hint := u.opener()
	if hint == "" {
		return
	}
	p := u.pal
	u.dc.circle(x+railX, y+9, 8, p.island) // on the rail, like a step's glyph
	u.dc.icon("headphones", x+railX, y+9, 11, alpha(p.muted, u.rowK))
	u.dc.text(fCaption(), "Click me to "+strings.ToLower(hint), x+textX, y+13, alpha(p.muted, u.rowK))
}

// musicOpener is the page the music row's control opens, if any.
func (u *UI) musicOpener() string {
	if u.openers == nil || u.track == nil || !u.track.Playing {
		return ""
	}
	name, _ := u.openers("music")
	if u.dashboards[name] == nil {
		return ""
	}
	return name
}

// vibeControl draws the music row's control for its page, at bx (its
// centre), quiet until the pointer is on the row.
func (u *UI) vibeControl(bx, cy float32, on float64) {
	p := u.pal
	key := "music:page"
	h, d := float32(u.hl(key)), float32(u.pressed(key))
	if h > 0.01 || d > 0 {
		u.dc.circle(bx, cy, 12-d, alpha(p.fg, 0.1*float64(h)+0.08*float64(d)))
	}
	u.dc.icon("headphones", bx, cy, 13, alpha(mix(p.muted, p.fg, 0.65*float64(h)), 0.25+0.75*on))
	u.hits = append(u.hits, hit{image.Rect(int(bx-15), int(cy-15), int(bx+15), int(cy+15)), key})
}
