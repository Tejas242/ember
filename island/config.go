package island

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is ~/.config/ember/config.toml. Every key is optional; the file is
// re-read when it changes.
type Config struct {
	Clock24     bool          // clock = "24h"
	HoverDelay  time.Duration // hover_delay_ms
	Claude      bool          // [show] claude
	Music       bool          // [show] music
	Edge        bool          // [show] edge
	Battery     bool          // [show] battery: charging, low battery
	OSD         bool          // [show] osd: volume, microphone, brightness
	Devices     bool          // [show] devices: Bluetooth audio connecting
	Screenshots bool          // [show] screenshots
	Privacy     bool          // [show] privacy: screen sharing
	Done        bool          // [show] done: "Done · project" when Claude finishes
}

func DefaultConfig() Config {
	return Config{HoverDelay: 150 * time.Millisecond, Claude: true, Music: true, Edge: true, Battery: true,
		OSD: true, Devices: true, Screenshots: true, Privacy: true, Done: true}
}

func ConfigPath() string {
	d := os.Getenv("XDG_CONFIG_HOME")
	if d == "" {
		h, _ := os.UserHomeDir()
		d = filepath.Join(h, ".config")
	}
	return filepath.Join(d, "ember", "config.toml")
}

// DefaultConfigText is what `ember install` writes when there's no config yet.
const DefaultConfigText = `# Ember: changes apply within ten seconds, no restart needed.

clock          = "12h"   # 12h | 24h
hover_delay_ms = 150     # how long the pointer rests on the pill before the card drops

[show]
claude      = true  # Claude Code sessions, permission requests, questions
done        = true  # "Done · project" when a session finishes and you're elsewhere
music       = true  # the playing track (MPRIS)
edge        = true  # edge's practice status, if edge runs
osd         = true  # volume, microphone and brightness changes
battery     = true  # charging, fully charged, battery low
devices     = true  # Bluetooth audio connecting and disconnecting
screenshots = true  # a preview when a screenshot is saved
privacy     = true  # a dot while your screen is being shared or recorded
`

// loadConfig reads path over the defaults; a missing file means defaults.
func loadConfig(path string) Config {
	c := DefaultConfig()
	f, err := os.Open(path)
	if err != nil {
		return c
	}
	defer f.Close()
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ln := strings.TrimSpace(stripComment(sc.Text()))
		if strings.HasPrefix(ln, "[") {
			section = strings.Trim(ln, "[] ")
			continue
		}
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		b, _ := strconv.ParseBool(v)
		switch section + "." + k {
		case ".clock":
			c.Clock24 = v == "24h" || v == "24"
		case ".hover_delay_ms":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n < 5000 {
				c.HoverDelay = time.Duration(n) * time.Millisecond
			}
		case "show.claude":
			c.Claude = b
		case "show.music":
			c.Music = b
		case "show.edge":
			c.Edge = b
		case "show.battery":
			c.Battery = b
		case "show.osd":
			c.OSD = b
		case "show.devices":
			c.Devices = b
		case "show.screenshots":
			c.Screenshots = b
		case "show.privacy":
			c.Privacy = b
		case "show.done":
			c.Done = b
		}
	}
	return c
}

// stripComment cuts a # comment that isn't inside quotes.
func stripComment(ln string) string {
	quote := rune(0)
	for i, c := range ln {
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && c == '#':
			return ln[:i]
		}
	}
	return ln
}

func (c Config) enabled(act string) bool {
	switch act {
	case "claude":
		return c.Claude
	case "music":
		return c.Music
	case "edge":
		return c.Edge
	}
	return true
}

func (c Config) timeFormat() string {
	if c.Clock24 {
		return "15:04"
	}
	return "3:04 PM"
}

func mtime(path string) time.Time {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}
