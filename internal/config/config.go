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

	"github.com/Tejas242/ember/internal/xdg"
)

// Config is ~/.config/ember/config.toml. Every key is optional; the file is
// re-read when it changes.
type Config struct {
	Clock24    bool          // clock = "24h"
	AwayAfter  time.Duration // away_after_s: idle this long and you're away
	Pill       string        // [pill] idle: clock | date | both
	HoverDelay time.Duration // [card] hover_delay_ms (top-level before [card] existed; still read there)
	OSDFor     time.Duration // [moments] osd_ms
	Settle     time.Duration // [claude] settle_min: a silent session is taken as stopped
	AgentQuiet time.Duration // [claude] agent_quiet_min: a silent subagent is taken as gone
	BarHeight  float64       // [look] bar_height: the bar's thickness; 0 asks Noctalia (else 24)

	Calm  bool // [ember] motion = "calm": no hops, sparks, light bands, breathing halo or dance
	Dance bool // [ember] dance: to your music, in headphones

	Claude      bool // [show] claude
	Music       bool // [show] music
	Edge        bool // [show] edge (also turns the edge plugin off)
	Battery     bool // [show] battery: charging, low battery
	OSD         bool // [show] osd: volume, microphone, brightness
	Devices     bool // [show] devices: Bluetooth audio connecting
	Screenshots bool // [show] screenshots
	Privacy     bool // [privacy] dot (once [show] privacy): a dot while the screen is shared
	Done        bool // [show] done: "Done · project" when Claude finishes

	Lyrics bool // [vibe] lyrics: the vibe stage may look a song's words up (LRCLIB); false keeps it offline

	Plugins     map[string]map[string]string // [plugins.<name>] tables: raw key → value (quotes stripped)
	PluginsOnly []string                     // [plugins] enabled = [...]: only these run; nil means all
}

// The defaults the island's own code also uses.
const (
	DefaultOSDFor     = 1600 * time.Millisecond // [moments] osd_ms
	DefaultAgentQuiet = 20 * time.Minute        // [claude] agent_quiet_min: a silent agent was killed
)

// Default is the config with no file.
func Default() Config {
	return Config{AwayAfter: 2 * time.Minute, Pill: "clock", HoverDelay: 150 * time.Millisecond,
		OSDFor: DefaultOSDFor, Settle: 30 * time.Minute, AgentQuiet: DefaultAgentQuiet,
		Dance:  true,
		Claude: true, Music: true, Edge: true, Battery: true,
		OSD: true, Devices: true, Screenshots: true, Privacy: true, Done: true, Lyrics: true}
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

[moments]
osd_ms = 1600  # how long a volume or brightness level stays up

[claude]
settle_min      = 30    # a session silent this long is taken as stopped (a killed Claude never says so)
agent_quiet_min = 20    # the same for a subagent

[look]
bar_height = 0  # your bar's thickness in px, so the pill sits on its line; 0 asks Noctalia, else 24 (16-64)

[ember]
motion = "full"  # full | calm: calm keeps the colours but drops hops, sparks, bands of light, the breathing halo, the little things it does on its own and the dance (reduced motion, and less CPU)
dance  = true    # dance to your music, in headphones

[show]
claude      = true  # Claude Code sessions: who is working, who asks, who is done
done        = true  # "Done · project" when a session finishes and you're elsewhere
music       = true  # the playing track (MPRIS)
edge        = true  # edge's practice status, if edge runs
osd         = true  # volume, microphone and brightness changes
battery     = true  # charging, fully charged, battery low
devices     = true  # Bluetooth audio connecting and disconnecting
screenshots = true  # a preview when a screenshot is saved

[privacy]
dot      = true  # a dot while your screen is being shared or recorded (once [show] privacy, still read)

[vibe]
lyrics = true  # click Ember in the card while a song plays: its words, from an .lrc beside the file, a cache, or LRCLIB; false: no network

# Plugins run when their tool is installed (edge) or when they're in
# ~/.config/ember/plugins (see docs/PLUGINS.md); ember plugins lists them.
# [plugins]
# enabled = "all"   # all | none | ["pomodoro", "edge"]: only these
# [plugins.edge]
# enabled = "auto"  # auto | true | false — edge's practice status, cards and dashboard
[plugins.vibe]
grants = ["media", "entry"]  # vibe hears the song only while its stage is open; a click on Ember opens it
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

// Paste is the shipped lines for missing keys (as Check names them), each
// under its section header, ready to paste into the file: the defaults with
// their comments. Top-level keys come first, as they must.
func Paste(missing []string) string {
	var top, rest []string
	section, header := "", ""
	for _, raw := range strings.Split(DefaultText, "\n") {
		ln := strings.TrimSpace(raw)
		if strings.HasPrefix(ln, "[") {
			section, header = strings.Trim(ln, "[]"), ln
			continue
		}
		k, _, ok := strings.Cut(stripComment(ln), "=")
		if !ok || strings.HasPrefix(ln, "#") {
			continue
		}
		key := strings.TrimSpace(k)
		if section != "" {
			key = section + "." + key
		}
		if !slices.Contains(missing, key) {
			continue
		}
		if section == "" {
			top = append(top, raw)
			continue
		}
		if header != "" {
			rest = append(rest, header)
			header = ""
		}
		rest = append(rest, raw)
	}
	return strings.Join(append(top, rest...), "\n")
}

var configSections = []string{"", "pill", "card", "moments", "claude", "look", "ember", "show", "privacy", "vibe", "plugins"}

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
			if g, ok := pluginList(v); k == "grants" && (!ok || g == nil) {
				warn(`plugins.%s.grants must be a list: ["critical", "badge", "focus"]`, name)
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
		canon := key
		switch key {
		case "hover_delay_ms":
			canon = "card.hover_delay_ms" // where it lived before [card]
		case "show.privacy":
			canon = "privacy.dot" // where it lived before [privacy]
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
		case "moments.osd_ms":
			span(&c.OSDFor, time.Millisecond, 300, 10000)
		case "claude.settle_min":
			span(&c.Settle, time.Minute, 1, 1440)
		case "claude.agent_quiet_min":
			span(&c.AgentQuiet, time.Minute, 1, 1440)
		case "look.bar_height":
			if n, ok := number(0, 64); ok && (n == 0 || n >= 16) {
				c.BarHeight = n
			}
		case "ember.motion":
			switch v {
			case "full", "calm":
				c.Calm = v == "calm"
			default:
				warn(`ember.motion must be "full" or "calm"`)
			}
		case "ember.dance":
			flag(&c.Dance)
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
		case "privacy.dot":
			flag(&c.Privacy)
		case "vibe.lyrics":
			flag(&c.Lyrics)
		case "show.done":
			flag(&c.Done)
		default:
			if retired[canon] {
				warn("%s is no longer used in Ember 1.3 (ignored; you can delete the line)", key)
				continue
			}
			warn("unknown key %s", key)
			continue
		}
		keys = append(keys, canon)
	}
	return c, warns, keys
}

// retired are keys earlier versions read. An old config that still has them
// loads fine: they do nothing, and only `ember doctor` says so.
var retired = map[string]bool{
	"claude.answer": true, "claude.wait_s": true,
	"card.steps": true, "card.others": true, "moments.done_s": true,
	"look.opacity": true, "look.green": true, "look.blue": true, "look.red": true, "look.amber": true,
	"ember.flourishes": true, "ember.celebrate": true, "privacy.discreet": true,
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

// PluginGrants is what the user granted a plugin: [plugins.<name>] grants =
// ["critical", "badge", "focus"].
func (c Config) PluginGrants(name string) []string {
	g, _ := pluginList(c.Plugins[name]["grants"])
	return g
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
