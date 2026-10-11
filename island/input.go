package island

import (
	"fmt"
	"image"
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
		if !u.hover {
			u.hover, u.hoverAt = true, u.now()
			redraw = true
		}
	case "leave":
		u.hover, u.pinned, u.suppress = false, false, false
		u.leftAt = u.now()
		u.hoverKey = ""
		return true, false
	case "press":
		u.mx, u.my = e.x, e.y
		u.press(u.hitAt(e.x, e.y), e.button)
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
	for _, pre := range []string{"btn:", "opt:", "focus:", "close:", "music:", "tab:", "bubble"} {
		hand = hand || strings.HasPrefix(k, pre)
	}
	return redraw, hand
}

// scroll: over the pill it switches what the pill shows; over the card it
// switches pages, except on the music page where it's volume.
func (u *UI) scroll(steps float64) {
	dir := 1
	if steps < 0 {
		dir = -1
	}
	switch u.currentView() {
	case vCompact:
		u.cycle(dir)
	case vExpanded:
		if u.pageFor() == "music" && strings.HasPrefix(u.hitAt(u.mx, u.my), "music:") {
			u.key(fmt.Sprintf("music:volume:%d", -dir))
			return
		}
		acts := u.acts()
		cur := u.pageFor()
		for i, a := range acts {
			if a == cur && len(acts) > 0 {
				u.page = acts[(i+dir+len(acts))%len(acts)]
				return
			}
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
		if u.moment != nil && u.moment.focus != "" {
			u.key("focus:" + u.moment.focus)
			u.moment = nil
			return
		}
		u.pinned, u.suppress = true, false
	case k == "panel": // the card's background closes it
		u.pinned, u.suppress = false, true
	case k == "bubble": // bring the bubble's activity to the front
		if s := u.secondary(u.primary()); s != "" {
			u.focus, u.focusAt = s, u.now()
		}
	case strings.HasPrefix(k, "tab:"):
		u.page = strings.TrimPrefix(k, "tab:")
	case k == "music:seek" && u.track != nil && u.track.Length > 0 && u.seekRect.Dx() > 0:
		frac := (u.mx - float64(u.seekRect.Min.X)) / float64(u.seekRect.Dx())
		target := time.Duration(frac * float64(u.track.Length))
		delta := target - u.track.pos(u.now())
		u.track.Position, u.track.PosAt = target, u.now() // move now; the player catches up
		u.key(fmt.Sprintf("music:seek:%d", delta))
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
