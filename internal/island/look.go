package island

import (
	"image/color"
	"math"
	"os"
	"time"

	"github.com/Tejas242/ember/internal/config"
)

// applyLook puts the [look] overrides over the theme's palette.
func applyLook(c config.Config, p palette) palette {
	for _, o := range []struct {
		dst *color.NRGBA
		v   color.NRGBA
	}{{&p.green, c.Green}, {&p.blue, c.Blue}, {&p.red, c.Red}, {&p.yellow, c.Amber}} {
		if o.v.A != 0 {
			*o.dst = o.v
		}
	}
	p.island.A = uint8(math.Round(c.Opacity * 255))
	return p
}

func mtime(path string) time.Time {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}
