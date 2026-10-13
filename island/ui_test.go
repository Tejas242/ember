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
	u.page = "music"
	settle(u, &clock)
	for _, want := range []string{"music:toggle", "music:next", "tab:claude"} {
		found := false
		for _, h := range u.hits {
			if h.key == want {
				c := h.r.Min.Add(h.r.Max).Div(2)
				if k := u.hitAt(float64(c.X), float64(c.Y)); k != want {
					t.Fatalf("centre of %s resolves to %q", want, k)
				}
				u.press(want, 1)
				found = true
			}
		}
		if !found {
			t.Fatalf("no hit area for %s", want)
		}
	}
	settle(u, &clock) // now on the Claude page
	if u.pageFor() != "claude" {
		t.Fatalf("tab should switch page, on %s", u.pageFor())
	}
	for _, h := range u.hits {
		if h.key == "focus:s" {
			c := h.r.Min.Add(h.r.Max).Div(2)
			u.press(u.hitAt(float64(c.X), float64(c.Y)), 1)
		}
	}
	if strings.Join(got, ",") != "music:toggle,music:next,focus:s" {
		t.Fatalf("keys %v", got)
	}
	// clicking the card's background closes it and hover doesn't reopen it
	u.press("panel", 1)
	u.hover, u.hoverAt = true, clock.Add(-time.Second)
	if u.currentView() == vExpanded {
		t.Fatal("background click should close the card")
	}
	u.pointer(pointerEvent{kind: "leave"})
	u.pointer(pointerEvent{kind: "enter"})
	clock = clock.Add(u.cfg.HoverDelay + time.Millisecond)
	if u.currentView() != vExpanded {
		t.Fatal("hover should open it again after the pointer left and came back")
	}
}

// Opening the card must show content while it grows, not an empty shape.
func TestNoBlankWhileOpening(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.track = &Track{Player: "p", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: time.Minute, PosAt: clock}
	settle(u, &clock)
	u.pinned = true
	for i := 0; i < 3; i++ { // 50 ms into the morph
		clock = clock.Add(time.Second / 60)
		u.renderDamage()
	}
	if u.sh.x > 100 {
		t.Fatalf("expected to be mid-morph, height %.0f", u.sh.x)
	}
	bright := 0
	for y := 26; y < int(u.sh.x)-4; y++ {
		for x := int(u.shape.x) + 8; x < int(u.shape.x+u.shape.w)-8; x++ {
			i := y*u.cv.img.Stride + x*4
			if u.cv.img.Pix[i] > 70 && u.cv.img.Pix[i+1] > 70 { // text at ~half opacity, mid-fade
				bright++
			}
		}
	}
	if bright < 20 {
		t.Fatalf("card is blank 50ms into opening (%d bright pixels)", bright)
	}
}

// What the pill shows: requests first, then moments, then your pick, then live work.
func TestPrimaryPriority(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	if u.primary() != "clock" {
		t.Fatal("idle shows the clock")
	}
	u.track = &Track{Player: "p", Title: "T", Playing: true, PosAt: clock}
	if u.primary() != "music" {
		t.Fatal("music when it plays")
	}
	u.sessions["a"] = &Session{ID: "a", Project: "edge", State: "work", Seen: clock}
	if u.primary() != "claude" || u.secondary("claude") != "music" {
		t.Fatalf("claude working wins, music in the bubble: %s / %s", u.primary(), u.secondary(u.primary()))
	}
	u.cycle(1) // scroll: pick the next activity
	if u.primary() != "music" {
		t.Fatalf("scrolling picks music, got %s", u.primary())
	}
	u.Moment(moment{key: "done", act: "claude", text: "Done", until: clock.Add(5 * time.Second)})
	if u.primary() != "moment" {
		t.Fatal("a moment beats your pick")
	}
	u.sessions["b"] = &Session{ID: "b", Project: "kv", State: "wait", Seen: clock}
	if u.primary() != "claude" || u.compactLine().textCol != u.pal.blue {
		t.Fatal("a request waiting on you beats everything")
	}
	clock = clock.Add(6 * time.Second)
	u.expire()
	if u.moment != nil {
		t.Fatal("moments expire")
	}
}

// Scrolling over the open card switches pages; tabs exist for each activity.
func TestPagesAndTabs(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.track = &Track{Player: "p", Title: "T", Playing: true, PosAt: clock}
	u.sessions["a"] = &Session{ID: "a", Project: "edge", State: "work", Seen: clock}
	u.pinned = true
	settle(u, &clock)
	if u.pageFor() != "claude" {
		t.Fatalf("opens on what the pill showed, got %s", u.pageFor())
	}
	tabs := 0
	for _, h := range u.hits {
		if strings.HasPrefix(h.key, "tab:") {
			tabs++
		}
	}
	if tabs != 2 {
		t.Fatalf("want 2 tabs, got %d", tabs)
	}
	u.mx, u.my = 220, 120
	u.scroll(1)
	if u.pageFor() != "music" {
		t.Fatalf("scroll should go to music, got %s", u.pageFor())
	}
}

// A volume OSD over a "Done" note doesn't lose the note.
func TestOSDKeepsNote(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.Moment(moment{key: "done", text: "Done · ember", until: clock.Add(6 * time.Second)})
	u.Moment(moment{key: "osd:volume", osd: true, meter: true, level: 0.5, until: clock.Add(1600 * time.Millisecond)})
	if u.moment.key != "osd:volume" || u.primary() != "moment" {
		t.Fatal("the OSD shows first")
	}
	clock = clock.Add(2 * time.Second)
	u.expire()
	if u.moment == nil || u.moment.key != "done" {
		t.Fatalf("the note should come back, got %+v", u.moment)
	}
	clock = clock.Add(7 * time.Second)
	u.expire()
	if u.moment != nil {
		t.Fatal("and then go")
	}
}
