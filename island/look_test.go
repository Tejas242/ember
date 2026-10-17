package island

import (
	"image/color"
	"testing"

	"github.com/Tejas242/ember/internal/config"
)

// [look] colours and opacity go over the theme; the defaults change nothing.
func TestApplyLook(t *testing.T) {
	c := config.Default()
	c.Opacity, c.Green, c.Amber = 1, color.NRGBA{0, 255, 0, 255}, color.NRGBA{0x11, 0x22, 0x33, 255}
	p := applyLook(c, loadPalette())
	if p.green != c.Green || p.yellow != c.Amber || p.island.A != 255 || p.blue == (color.NRGBA{}) {
		t.Fatalf("look: %+v", p)
	}
	if p := applyLook(config.Default(), loadPalette()); p != loadPalette() {
		t.Fatal("the default look changes the theme's palette")
	}
}
