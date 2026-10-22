package wayland

import (
	"image"
	"testing"
)

func TestMergeRects(t *testing.T) {
	area := func(rs []image.Rectangle) (n int) {
		for _, r := range rs {
			n += r.Dx() * r.Dy()
		}
		return
	}
	// Ember at one end of the card, a meter at the other: kept apart
	far := MergeRects([]image.Rectangle{image.Rect(101, 0, 154, 46), image.Rect(205, 157, 434, 180)})
	if len(far) != 2 || area(far) != 53*46+229*23 {
		t.Errorf("far apart: %v", far)
	}
	// overlapping, and touching: one
	if got := MergeRects([]image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(5, 5, 20, 20), image.Rect(20, 0, 30, 10)}); len(got) != 1 || got[0] != image.Rect(0, 0, 30, 20) {
		t.Errorf("overlapping: %v", got)
	}
	// empties go; many become their union
	if got := MergeRects([]image.Rectangle{{}, image.Rect(3, 3, 3, 9)}); len(got) != 0 {
		t.Errorf("empties: %v", got)
	}
	var many []image.Rectangle
	for i := 0; i < 12; i++ {
		many = append(many, image.Rect(i*100, i*100, i*100+2, i*100+2))
	}
	if got := MergeRects(many); len(got) != 1 || got[0] != image.Rect(0, 0, 1102, 1102) {
		t.Errorf("many: %v", got)
	}
}
