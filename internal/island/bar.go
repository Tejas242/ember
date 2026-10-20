package island

import (
	"os/exec"
	"strconv"
	"strings"
)

// The bar the island sits on: its height, from Noctalia's settings or the
// config.

// barLook is what the island takes from Noctalia's bar: its thickness, so
// the pill sits on the bar's line and dips just below it.
type barLook struct{ h float32 }

// readBar reads [bar.default] from Noctalia's merged config.
func readBar() barLook {
	b := barLook{24}
	out, err := exec.Command("noctalia", "config", "export").Output()
	if err != nil {
		return b
	}
	return parseBar(string(out), b)
}

func parseBar(cfg string, b barLook) barLook {
	section := ""
	for _, ln := range strings.Split(cfg, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "[") {
			section = strings.Trim(ln, "[]")
			continue
		}
		if section != "bar.default" {
			continue
		}
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 32)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(k) {
		case "thickness":
			if f >= 16 && f <= 64 {
				b.h = float32(f)
			}
		}
	}
	return b
}
