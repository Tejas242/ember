// Package config reads ~/.config/ember/config.toml.
package config

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/xdg"
)

// Config is ~/.config/ember/config.toml. Every key is optional; the file is
// re-read when it changes.
type Config struct {
	Clock24    bool          // clock = "24h"
	AwayAfter  time.Duration // away_after_s: idle this long and you're away
	Pill       string        // [pill] idle: clock | date | both
	HoverDelay time.Duration // [card] hover_delay_ms (top-level before [card] existed; still read there)
	CardSteps  int           // [card] steps: timeline steps, at most
	CardOthers int           // [card] others: other sessions listed under the story
	DoneFor    time.Duration // [moments] done_s
	OSDFor     time.Duration // [moments] osd_ms
	Answer     bool          // [claude] answer: requests on the island, not only in the terminal
	Wait       time.Duration // [claude] wait_s: before a request goes back to the terminal
	Settle     time.Duration // [claude] settle_min: a silent session is taken as stopped
	AgentQuiet time.Duration // [claude] agent_quiet_min: a silent subagent is taken as gone
	Opacity    float64       // [look] opacity of the island's body
	BarHeight  float64       // [look] bar_height: the bar's thickness; 0 asks Noctalia (else 24)

	Green, Blue, Red, Amber color.NRGBA // [look] colours; zero follows the Noctalia theme

	Claude      bool // [show] claude
	Music       bool // [show] music
	Edge        bool // [show] edge (also turns the edge plugin off)
	Battery     bool // [show] battery: charging, low battery
	OSD         bool // [show] osd: volume, microphone, brightness
	Devices     bool // [show] devices: Bluetooth audio connecting
	Screenshots bool // [show] screenshots
	Privacy     bool // [show] privacy: screen sharing
	Done        bool // [show] done: "Done · project" when Claude finishes

	Plugins     map[string]map[string]string // [plugins.<name>] tables: raw key → value (quotes stripped)
	PluginsOnly []string                     // [plugins] enabled = [...]: only these run; nil means all
}

// The defaults the island's own code also uses.
const (
	DefaultCardSteps  = 4                       // [card] steps: timeline steps shown, at most
	DefaultOSDFor     = 1600 * time.Millisecond // [moments] osd_ms
	DefaultAgentQuiet = 20 * time.Minute        // [claude] agent_quiet_min: a silent agent was killed
)

// Default is the config with no file.
func Default() Config {
	return Config{AwayAfter: 2 * time.Minute, Pill: "clock", HoverDelay: 150 * time.Millisecond,
		CardSteps: DefaultCardSteps, CardOthers: 3, DoneFor: 6 * time.Second, OSDFor: DefaultOSDFor,
		Answer: true, Wait: ipc.AskTimeout - 5*time.Second, Settle: 30 * time.Minute, AgentQuiet: DefaultAgentQuiet, Opacity: 0.87,
		Claude: true, Music: true, Edge: true, Battery: true,
		OSD: true, Devices: true, Screenshots: true, Privacy: true, Done: true}
}

// Path is the config file.
func Path() string {
	return filepath.Join(xdg.ConfigHome(), "ember", "config.toml")
}

// DefaultText is what `ember install` writes when there's no config
// yet. It must parse to exactly Default().
const DefaultText = `# Ember: changes apply within ten seconds, no restart needed.
# A value Ember can't use keeps its default; ` + "`ember doctor`" + ` says which.

clock        = "12h"  # 12h | 24h
away_after_s = 120    # idle this long and you're away: news waits and is summed up when you're back

[pill]
idle = "clock"  # what the pill shows when nothing's going on: clock | date | both

[card]
hover_delay_ms = 150  # how long the pointer rests on the pill before the card drops
steps          = 4    # timeline steps, at most (1-4; fewer when other rows need the room)
others         = 3    # sessions listed besides the first; the rest fold into "N more" (0-3)

[moments]
done_s = 6     # how long "Done · project" stays up
osd_ms = 1600  # how long a volume or brightness level stays up

[claude]
answer          = true  # answer permission requests and questions on the island; false: only the terminal asks
wait_s          = 85    # how long a request waits on the island before the terminal asks (5-85)
settle_min      = 30    # a session silent this long is taken as stopped (a killed Claude never says so)
agent_quiet_min = 20    # the same for a subagent

[look]
opacity    = 0.87  # the island's body: 1 is solid, lower lets the blur through (0.3-1)
bar_height = 0     # your bar's thickness in px, so the pill sits on its line; 0 asks Noctalia, else 24 (16-64)
# colours are hex, like "#72dd71"; "" follows Noctalia's theme
green   = ""    # working, done
blue    = ""    # waiting on you
red     = ""    # errors
amber   = ""    # nudges, low battery

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

# Plugins run when their tool is installed (edge) or when they're in
# ~/.config/ember/plugins (see docs/PLUGINS.md); ember plugins lists them.
# [plugins]
# enabled = "all"   # all | none | ["pomodoro", "edge"]: only these
# [plugins.edge]
# enabled = "auto"  # auto | true | false — edge's practice status, cards and dashboard
`

// Load reads path over the defaults; a missing file means defaults.
// The warnings say which lines it couldn't use.
func Load(path string) (Config, []string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Default(), nil
	}
	c, warns, _ := parse(string(b))
	return c, warns
}

// Check is for `ember doctor`: what's wrong in the config file, and
// which of the shipped keys it doesn't have yet (Ember grew new ones since
// it was written; install never overwrites it).
func Check(path string) (warns, missing []string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	_, warns, have := parse(string(b))
	_, _, all := parse(DefaultText)
	for _, k := range all {
		if !slices.Contains(have, k) {
			missing = append(missing, k)
		}
	}
	return warns, missing
}

var configSections = []string{"", "pill", "card", "moments", "claude", "look", "show", "plugins"}

// parse reads the small part of TOML Ember needs: sections and
// key = value lines. It never fails: an unknown key is skipped and a value
// it can't use keeps its default, each with a warning. keys are the known
// keys set, as section.key.
func parse(text string) (c Config, warns, keys []string) {
	c = Default()
	section, known := "", true
	for i, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(stripComment(raw))
		if ln == "" {
			continue
		}
		warn := func(f string, a ...any) {
			warns = append(warns, fmt.Sprintf("config: line %d: ", i+1)+fmt.Sprintf(f, a...))
		}
		if strings.HasPrefix(ln, "[") {
			section = strings.TrimSpace(strings.Trim(ln, "[]"))
			known = slices.Contains(configSections, section) || strings.HasPrefix(section, "plugins.")
			if !known {
				warn("unknown section [%s]", section)
			}
			continue
		}
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			warn("expected key = value, got %q", ln)
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if section == "plugins" {
			if k != "enabled" {
				warn("unknown key plugins.%s", k)
				continue
			}
			if only, ok := pluginList(v); ok {
				c.PluginsOnly = only
				keys = append(keys, "plugins.enabled")
			} else {
				warn(`plugins.enabled must be "all", "none" or a list: ["pomodoro", "edge"]`)
			}
			continue
		}
		if name, ok := strings.CutPrefix(section, "plugins."); ok {
			name = strings.Trim(name, `"'`)
			if c.Plugins == nil {
				c.Plugins = map[string]map[string]string{}
			}
			if c.Plugins[name] == nil {
				c.Plugins[name] = map[string]string{}
			}
			c.Plugins[name][k] = v
			if _, err := strconv.ParseBool(v); k == "enabled" && v != "auto" && err != nil {
				warn("plugins.%s.enabled must be auto, true or false", name)
			}
			continue
		}
		if !known {
			continue // warned at its header
		}
		key := k
		if section != "" {
			key = section + "." + k
		}

		flag := func(dst *bool) {
			if b, err := strconv.ParseBool(v); err == nil {
				*dst = b
			} else {
				warn("%s must be true or false", key)
			}
		}
		count := func(dst *int, lo, hi int) {
			if n, err := strconv.Atoi(v); err == nil && n >= lo && n <= hi {
				*dst = n
			} else {
				warn("%s must be a whole number from %d to %d", key, lo, hi)
			}
		}
		number := func(lo, hi float64) (float64, bool) {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(n) || n < lo || n > hi {
				warn("%s must be a number from %g to %g", key, lo, hi)
				return 0, false
			}
			return n, true
		}
		span := func(dst *time.Duration, unit time.Duration, lo, hi float64) {
			if n, ok := number(lo, hi); ok {
				*dst = time.Duration(n * float64(unit))
			}
		}
		hex := func(dst *color.NRGBA) {
			if v == "" {
				*dst = color.NRGBA{}
			} else if col, ok := HexColor(v); ok {
				*dst = col
			} else {
				warn(`%s must be a colour like "#72dd71", or "" to follow the theme`, key)
			}
		}

		canon := key
		if key == "hover_delay_ms" {
			canon = "card.hover_delay_ms" // where it lived before [card]
		}
		switch canon {
		case "clock":
			switch v {
			case "12h", "12":
				c.Clock24 = false
			case "24h", "24":
				c.Clock24 = true
			default:
				warn("clock must be 12h or 24h")
			}
		case "away_after_s":
			span(&c.AwayAfter, time.Second, 30, 3600)
		case "pill.idle":
			if v == "clock" || v == "date" || v == "both" {
				c.Pill = v
			} else {
				warn("pill.idle must be clock, date or both")
			}
		case "card.hover_delay_ms":
			span(&c.HoverDelay, time.Millisecond, 0, 5000)
		case "card.steps":
			count(&c.CardSteps, 1, DefaultCardSteps)
		case "card.others":
			count(&c.CardOthers, 0, 3)
		case "moments.done_s":
			span(&c.DoneFor, time.Second, 1, 60)
		case "moments.osd_ms":
			span(&c.OSDFor, time.Millisecond, 300, 10000)
		case "claude.answer":
			flag(&c.Answer)
		case "claude.wait_s":
			span(&c.Wait, time.Second, 5, (ipc.AskTimeout - 5*time.Second).Seconds()) // the hook gives up at ipc.AskTimeout
		case "claude.settle_min":
			span(&c.Settle, time.Minute, 1, 1440)
		case "claude.agent_quiet_min":
			span(&c.AgentQuiet, time.Minute, 1, 1440)
		case "look.opacity":
			if n, ok := number(0.3, 1); ok {
				c.Opacity = n
			}
		case "look.bar_height":
			if n, ok := number(0, 64); ok && (n == 0 || n >= 16) {
				c.BarHeight = n
			}
		case "look.green":
			hex(&c.Green)
		case "look.blue":
			hex(&c.Blue)
		case "look.red":
			hex(&c.Red)
		case "look.amber":
			hex(&c.Amber)
		case "show.claude":
			flag(&c.Claude)
		case "show.music":
			flag(&c.Music)
		case "show.edge":
			flag(&c.Edge)
		case "show.battery":
			flag(&c.Battery)
		case "show.osd":
			flag(&c.OSD)
		case "show.devices":
			flag(&c.Devices)
		case "show.screenshots":
			flag(&c.Screenshots)
		case "show.privacy":
			flag(&c.Privacy)
		case "show.done":
			flag(&c.Done)
		default:
			warn("unknown key %s", key)
			continue
		}
		keys = append(keys, canon)
	}
	return c, warns, keys
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

// pluginList reads [plugins] enabled: "all" (nil), "none" (empty), or a
// list of names.
func pluginList(v string) ([]string, bool) {
	switch v {
	case "all":
		return nil, true
	case "none", "[]":
		return []string{}, true
	}
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil, false
	}
	out := []string{}
	for _, n := range strings.Split(v[1:len(v)-1], ",") {
		if n = strings.Trim(strings.TrimSpace(n), `"'`); n != "" {
			out = append(out, n)
		}
	}
	return out, true
}

// PluginOn reports whether a plugin should run: in [plugins] enabled (all,
// by default), and then [plugins.<name>] enabled = "auto" (the default: run
// if its tool is installed, i.e. detected), true or false.
func (c Config) PluginOn(name string, detected bool) bool {
	if name == "edge" && !c.Edge {
		return false // [show] edge = false, from before plugins
	}
	if c.PluginsOnly != nil && !slices.Contains(c.PluginsOnly, name) {
		return false
	}
	if b, err := strconv.ParseBool(c.Plugins[name]["enabled"]); err == nil {
		return b
	}
	return detected
}

func (c Config) Enabled(act string) bool {
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

func (c Config) TimeFormat() string {
	if c.Clock24 {
		return "15:04"
	}
	return "3:04 PM"
}

// pillFormat is what the resting pill shows: the time, the date, or both.
func (c Config) PillFormat() string {
	switch c.Pill {
	case "date":
		return "Mon 2 Jan"
	case "both":
		return "Mon 2 Jan · " + c.TimeFormat()
	}
	return c.TimeFormat()
}

// Away is away_after_s, unless EMBER_AWAY_AFTER (a Go duration, for
// testing) says otherwise.
func (c Config) Away() time.Duration {
	if v, err := time.ParseDuration(os.Getenv("EMBER_AWAY_AFTER")); err == nil && v > 0 {
		return v
	}
	return c.AwayAfter
}

// HexColor reads "#rrggbb", "0xrrggbb" or "rrggbb", quoted or not.
func HexColor(s string) (color.NRGBA, bool) {
	s = strings.Trim(strings.TrimSpace(s), `'"`)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "#"), "0x")
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	var v [3]uint8
	for i := range v {
		var n uint8
		for _, ch := range s[i*2 : i*2+2] {
			n <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				n |= uint8(ch - '0')
			case ch >= 'a' && ch <= 'f':
				n |= uint8(ch-'a') + 10
			case ch >= 'A' && ch <= 'F':
				n |= uint8(ch-'A') + 10
			default:
				return color.NRGBA{}, false
			}
		}
		v[i] = n
	}
	return color.NRGBA{v[0], v[1], v[2], 255}, true
}
