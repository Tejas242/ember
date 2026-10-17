package island

import (
	"strings"
	"testing"
	"time"
)

func TestDragSeekOnlyOnRelease(t *testing.T) {
	clock := time.Date(2026, 10, 13, 21, 0, 0, 0, time.Local)
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.track = &Track{Player: "p", Title: "T", Playing: true, Length: 4 * time.Minute, PosAt: clock}
	var keys []string
	u.onKey = func(k string) { keys = append(keys, k) }
	u.pinned = true
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
	if d := u.track.Pos(clock) - 2*time.Minute; d < -2*time.Second || d > 2*time.Second {
		t.Fatalf("playhead should be at the middle, at %v", u.track.Pos(clock))
	}
}
