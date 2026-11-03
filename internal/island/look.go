package island

import (
	"math"
	"os"
	"time"

	"github.com/Tejas242/ember/internal/config"
)

// islandOpacity is how solid the island's body is: lower lets the blur through.
const islandOpacity = 0.87

// applyLook sets the island body's translucency on the theme's palette.
func applyLook(_ config.Config, p palette) palette {
	p.island.A = uint8(math.Round(islandOpacity * 255))
	return p
}

func mtime(path string) time.Time {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}
