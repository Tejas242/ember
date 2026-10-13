package island

import (
	"strings"
	"testing"
	"time"
)

func TestDragSlider(t *testing.T) {
	clock := time.Date(2026, 10, 13, 21, 0, 0, 0, time.Local)
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.vol, u.light = level{v: 0.2, ok: true}, 0.5
	var keys []string
	u.onKey = func(k string) { keys = append(keys, k) }
	u.pinned, u.page = true, "system"
	settle(u, &clock)
	r := u.sliders["sys:vol"]
	if r.Dx() == 0 {
		t.Fatal("no volume slider")
	}
	y := float64(r.Min.Y+r.Max.Y) / 2
	at := func(f float64) float64 { return float64(r.Min.X) + f*float64(r.Dx()) }

	u.pointer(pointerEvent{kind: "enter", x: at(0.1), y: y})
	u.pointer(pointerEvent{kind: "press", x: at(0.1), y: y, button: 1})
	if u.drag != "sys:vol" || len(keys) != 1 {
		t.Fatalf("press starts a drag and sets the level: drag=%q keys=%v", u.drag, keys)
	}
	for i := 1; i <= 10; i++ { // 10 moves in 100 ms: throttled to every 50 ms
		clock = clock.Add(10 * time.Millisecond)
		u.pointer(pointerEvent{kind: "motion", x: at(0.1 + float64(i)*0.05), y: y})
	}
	if len(keys) > 4 {
		t.Fatalf("too many updates while dragging: %v", keys)
	}
	u.pointer(pointerEvent{kind: "leave"}) // dragged off the island: still ours
	u.pointer(pointerEvent{kind: "motion", x: at(1.4), y: y + 80})
	u.pointer(pointerEvent{kind: "release", x: at(1.4), y: y + 80, button: 1})
	if u.drag != "" || u.vol.v != 1 || !strings.HasPrefix(keys[len(keys)-1], "sys:vol:1.000") {
		t.Fatalf("release past the end sets exactly 100%%: v=%v last=%v", u.vol.v, keys[len(keys)-1])
	}
}

func TestDragSeekOnlyOnRelease(t *testing.T) {
	clock := time.Date(2026, 10, 13, 21, 0, 0, 0, time.Local)
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.track = &Track{Player: "p", Title: "T", Playing: true, Length: 4 * time.Minute, PosAt: clock}
	var keys []string
	u.onKey = func(k string) { keys = append(keys, k) }
	u.pinned, u.page = true, "music"
	settle(u, &clock)
	r := u.seekRect
	y := float64(r.Min.Y+r.Max.Y) / 2
	u.pointer(pointerEvent{kind: "press", x: float64(r.Min.X) + 0.25*float64(r.Dx()), y: y, button: 1})
	u.pointer(pointerEvent{kind: "motion", x: float64(r.Min.X) + 0.5*float64(r.Dx()), y: y})
	if len(keys) != 0 {
		t.Fatalf("no seeking mid-drag: %v", keys)
	}
	u.pointer(pointerEvent{kind: "release", x: float64(r.Min.X) + 0.5*float64(r.Dx()), y: y, button: 1})
	if len(keys) != 1 || !strings.HasPrefix(keys[0], "music:seek:") {
		t.Fatalf("one seek on release: %v", keys)
	}
	if d := u.track.pos(clock) - 2*time.Minute; d < -2*time.Second || d > 2*time.Second {
		t.Fatalf("playhead should be at the middle, at %v", u.track.pos(clock))
	}
}
