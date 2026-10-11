package island

import (
	"strings"
	"testing"
	"time"
)

func settle(u *UI, clock *time.Time) {
	for i := 0; i < 150; i++ {
		*clock = clock.Add(time.Second / 60)
		u.renderDamage()
	}
}

// Buttons inside the open card must win over the card's own click area.
func TestExpandedButtonsWinHits(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.track = &Track{Player: "p", Title: "T", Artist: "A", Playing: true, Length: time.Minute, PosAt: clock}
	u.sessions["s"] = &Session{ID: "s", Project: "edge", State: "work", Label: "x", Since: clock}
	var got []string
	u.onKey = func(k string) { got = append(got, k) }
	u.pinned = true
	settle(u, &clock)
	for _, want := range []string{"music:toggle", "music:next", "focus:s"} {
		found := false
		for _, h := range u.hits {
			if h.key == want {
				c := h.r.Min.Add(h.r.Max).Div(2)
				if k := u.hitAt(float64(c.X), float64(c.Y)); k != want {
					t.Fatalf("centre of %s resolves to %q", want, k)
				}
				u.press(want)
				found = true
			}
		}
		if !found {
			t.Fatalf("no hit area for %s", want)
		}
	}
	if strings.Join(got, ",") != "music:toggle,music:next,focus:s" {
		t.Fatalf("keys %v", got)
	}
	// clicking the card's background closes it and hover doesn't reopen it
	u.press("panel")
	u.hover, u.hoverAt = true, clock.Add(-time.Second)
	if u.currentView() == vExpanded {
		t.Fatal("background click should close the card")
	}
	u.pointer(pointerEvent{kind: "leave"})
	u.pointer(pointerEvent{kind: "enter"})
	clock = clock.Add(hoverDelay + time.Millisecond)
	if u.currentView() != vExpanded {
		t.Fatal("hover should open it again after the pointer left and came back")
	}
}
