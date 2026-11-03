package island

import (
	"testing"

	"github.com/Tejas242/ember/internal/config"
)

// The island's body is a little translucent over the theme's palette; the
// colours are the theme's own.
func TestApplyLook(t *testing.T) {
	base := loadPalette()
	p := applyLook(config.Default(), base)
	if p.island.A != 222 || p.green != base.green || p.blue != base.blue || p.red != base.red || p.yellow != base.yellow {
		t.Fatalf("look: %+v", p)
	}
}
