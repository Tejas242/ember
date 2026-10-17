package island

import "github.com/Tejas242/ember/internal/wayland"

// pointerEvent is the pointer over the island, in surface pixels.
type pointerEvent struct {
	kind   string // enter | motion | leave | press | release | scroll
	x, y   float64
	button int     // press: 1 left, 2 middle, 3 right
	steps  float64 // scroll: wheel notches, positive = down
}

func fromWayland(e wayland.PointerEvent) pointerEvent {
	return pointerEvent{kind: e.Kind, x: e.X, y: e.Y, button: e.Button, steps: e.Steps}
}
