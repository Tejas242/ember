package island

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"
)

func (u *UI) hitAt(x, y float64) string {
	for i := len(u.hits) - 1; i >= 0; i-- {
		if (image.Point{int(x), int(y)}).In(u.hits[i].r) {
			return u.hits[i].key
		}
	}
	return ""
}

// pointer handles a pointer event; it reports whether to redraw and whether
// the cursor should be a hand.
func (u *UI) pointer(e pointerEvent) (redraw, hand bool) {
	switch e.kind {
	case "enter", "motion":
		u.mx, u.my = e.x, e.y
		if u.drag != "" { // the pointer may be far outside the island: the grab keeps it ours
			u.dragTo(false)
			return true, true
		}
		if u.dash != "" {
			u.dashIn = true
		}
		if !u.hover {
			u.hover, u.hoverAt = true, u.now()
			u.mind.kick(u.now(), 0.15, 0.02) // it notices you
			redraw = true
		}
	case "leave":
		if u.drag != "" {
			return false, true // released outside: the release still comes to us
		}
		u.hover, u.pinned, u.suppress = false, false, false
		u.leftAt = u.now()
		if u.dash != "" && u.dashIn { // you looked and moved on
			u.closeDash()
			u.suppress = false
		}
		u.hoverKey = ""
		return true, false
	case "press":
		u.mx, u.my = e.x, e.y
		k := u.hitAt(e.x, e.y)
		if e.button == 1 && draggable(k) {
			u.drag, u.dragSent = k, time.Time{}
			u.flash(k)
			u.dragTo(false)
			return true, true
		}
		u.press(k, e.button)
		return true, false
	case "release":
		u.mx, u.my = e.x, e.y
		if u.drag != "" {
			u.dragTo(true)
			u.drag = ""
			if k := u.hitAt(e.x, e.y); k == "" { // let go outside the island
				u.hover, u.hoverKey, u.leftAt = false, "", u.now()
			}
		}
		return true, false
	case "scroll":
		u.scroll(e.steps)
		return true, false
	}
	k := u.hitAt(u.mx, u.my)
	if k != u.hoverKey {
		u.hoverKey = k
		redraw = true
	}
	for _, pre := range []string{"btn:", "opt:", "focus:", "close:", "music:", "trail", "sys:"} {
		hand = hand || strings.HasPrefix(k, pre)
	}
	return redraw, hand
}

// scroll: over the pill it switches what the pill shows; over the card it
// changes the level under the pointer (a chip, the music row).
func (u *UI) scroll(steps float64) {
	dir := 1
	if steps < 0 {
		dir = -1
	}
	switch u.currentView() {
	case vCompact:
		u.cycle(dir)
	case vExpanded:
		k := u.hitAt(u.mx, u.my)
		switch {
		case k == "sys:vol" || k == "sys:light" || k == "sys:mute":
			u.key(fmt.Sprintf("%s:nudge:%d", strings.TrimSuffix(k, ":mute"), -dir))
		case strings.HasPrefix(k, "music:") || u.tweens["row:music"] != nil && u.tweens["row:music"].x > 0.5:
			u.key(fmt.Sprintf("music:volume:%d", -dir)) // on the music row: the player's own volume
			u.used("music")
		}
	}
}

func (u *UI) press(k string, button int) {
	if button == 2 { // middle: play / pause, anywhere on the island
		u.flash("music:toggle")
		u.key("music:toggle")
		return
	}
	if button == 3 { // right: dismiss — a moment, the open card, an edge note
		switch {
		case u.currentView() == vDash:
			u.closeDash()
		case u.moment != nil:
			u.moment = nil
		case u.currentView() == vExpanded:
			u.pinned, u.suppress = false, true
		case u.top() != nil && u.top().Kind == "edge":
			c := u.top()
			u.RemoveCard(c.ID)
			if c.OnPick != nil {
				c.OnPick("dismiss", nil)
			}
		}
		return
	}
	c := u.top()
	u.flash(k)
	switch {
	case k == "pill":
		// a waiting session or a "done" moment: go to it; otherwise open now
		if ss := u.sessionList(); len(ss) > 0 && ss[0].State == "wait" {
			u.key("focus:" + ss[0].ID)
			return
		}
		if m := u.moment; m != nil {
			switch {
			case m.focus != "":
				u.key("focus:" + m.focus)
				u.moment = nil
				return
			case m.open != "":
				u.key("open:" + m.open)
				u.moment = nil
				return
			case m.osd || m.batt || m.key == "power": // volume, brightness, battery: their controls
				u.moment = nil
			}
		}
		u.pinned, u.suppress = true, false
	case k == "panel": // the card's background closes it
		u.pinned, u.suppress = false, true
	case k == "dash": // so does a dashboard's
		u.closeDash()
	case strings.HasPrefix(k, "dash:"): // a plugin's row in the card: its dashboard
		u.openDash(strings.TrimPrefix(k, "dash:"))
	case k == "trail": // the satellite at the pill's end: swap it in
		if s := u.secondary(u.primary()); s != "" {
			u.focus, u.focusAt = s, u.now()
		}
	case k == "sys:mute":
		u.vol.muted = !u.vol.muted
		u.key(k)
	case strings.HasPrefix(k, "close:") && c != nil:
		u.RemoveCard(c.ID)
		if c.OnPick != nil {
			c.OnPick("dismiss", nil)
		}
	case strings.HasPrefix(k, "opt:") && c != nil:
		var i int
		fmt.Sscanf(k, "opt:%d", &i)
		if c.Multi {
			if c.picked == nil {
				c.picked = map[int]bool{}
			}
			c.picked[i] = !c.picked[i]
			return
		}
		u.RemoveCard(c.ID)
		if c.OnPick != nil {
			c.OnPick("option", []int{i})
		}
	case strings.HasPrefix(k, "btn:edge:"), strings.HasPrefix(k, "music:"), strings.HasPrefix(k, "focus:"):
		u.key(k)
		if strings.HasPrefix(k, "music:") {
			u.used("music")
		} else if strings.HasPrefix(k, "btn:edge:") {
			u.used("edge")
		}
	case strings.HasPrefix(k, "btn:") && c != nil:
		var picked []int
		for i := range c.Options {
			if c.picked[i] {
				picked = append(picked, i)
			}
		}
		u.RemoveCard(c.ID)
		if c.OnPick != nil {
			c.OnPick(strings.TrimPrefix(k, "btn:"), picked)
		}
	}
}

// flash gives a pressed control a brief highlight.
func (u *UI) flash(k string) { u.pressKey, u.pressAt = k, u.now() }

func (u *UI) key(k string) {
	if u.onKey != nil {
		u.onKey(k)
	}
}

// ---- dragging: sliders and the seek bar ----

func draggable(k string) bool { return k == "music:seek" }

// dragFrac is where the pointer is along the dragged control, 0..1.
func (u *UI) dragFrac() (float64, bool) {
	r := u.sliders[u.drag]
	if u.drag == "music:seek" {
		r = u.seekRect
	}
	if r.Dx() <= 0 {
		return 0, false
	}
	return math.Max(0, math.Min(1, (u.mx-float64(r.Min.X))/float64(r.Dx()))), true
}

// dragTo follows the pointer: the control moves every frame, the system
// hears about it at most every 50 ms (a process per change would pile up),
// and once more, exactly, on release. A seek happens only on release: the
// playhead previews until then.
func (u *UI) dragTo(final bool) {
	frac, ok := u.dragFrac()
	if !ok {
		return
	}
	now := u.now()
	switch u.drag {
	case "sys:vol":
		u.vol.v = frac
	case "sys:light":
		u.light = frac
	case "music:seek":
		u.seekPreview = frac
		if final && u.track != nil && u.track.Length > 0 {
			target := time.Duration(frac * float64(u.track.Length))
			delta := target - u.track.pos(now)
			u.track.Position, u.track.PosAt = target, now // move now; the player catches up
			u.key(fmt.Sprintf("music:seek:%d", delta))
		}
		return
	}
	if final || now.Sub(u.dragSent) >= 50*time.Millisecond {
		u.dragSent = now
		u.key(fmt.Sprintf("%s:%.3f", u.drag, frac))
	}
}
