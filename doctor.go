package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/Tejas242/ember/island"
)

const version = "1.0.0"

// versionString is "1.0.0 (9daceba)", with "+dirty" for an uncommitted build.
func versionString() string {
	v := version
	if bi, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", false
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if len(rev) >= 7 {
			v += " (" + rev[:7]
			if dirty {
				v += "+dirty"
			}
			v += ")"
		}
	}
	return v
}

// doctor checks every piece Ember depends on and says what to do about
// whatever is missing.
func doctor() {
	bad := 0
	green, red, yellow := "\033[32m✓\033[0m", "\033[31m✗\033[0m", "\033[33m·\033[0m"
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 { // piped: plain marks
		green, red, yellow = "✓", "✗", "·"
	}
	line := func(ok bool, what, detail, fix string) {
		mark := green
		if !ok {
			mark, bad = red, bad+1
		}
		fmt.Printf("%s %-14s %s\n", mark, what, detail)
		if !ok && fix != "" {
			fmt.Printf("  %-14s → %s\n", "", fix)
		}
	}
	note := func(what, detail string) { fmt.Printf("%s %-14s %s\n", yellow, what, detail) }

	exe, _ := os.Executable()
	fmt.Printf("ember %s  %s\n\n", versionString(), exe)

	active := exec.Command("systemctl", "--user", "is-active", "--quiet", "ember.service").Run() == nil
	enabled := exec.Command("systemctl", "--user", "is-enabled", "--quiet", "ember.service").Run() == nil
	line(active && enabled, "service", fmt.Sprintf("ember.service active=%v enabled=%v", active, enabled), "ember install")

	st, up := island.QueryStatus()
	line(up, "island", map[bool]string{true: fmt.Sprintf("running (pid %d, up %s, bar %.0fpx)", st.Pid, st.Uptime, st.BarHeight),
		false: "not answering on " + island.SockPath()}[up], "journalctl --user -u ember -n 30")
	if up {
		line(st.Blur, "blur", map[bool]string{true: "ext-background-effect-v1", false: "compositor has no ext-background-effect-v1; the island is drawn without blur"}[st.Blur], "")
		line(st.CursorShape, "cursor", map[bool]string{true: "wp-cursor-shape-v1", false: "no wp-cursor-shape-v1: no hand cursor over buttons"}[st.CursorShape], "")
		line(st.Presence, "presence", map[bool]string{true: "ext-idle-notify-v1: knows when you're away",
			false: "no ext-idle-notify-v1: no \"while you were away\""}[st.Presence], "")
		line(st.Niri, "niri", map[bool]string{true: "event stream: focus, terminal titles, screen sharing",
			false: "no niri event stream: Done notes can't tell where you're looking"}[st.Niri], "")
		line(st.Music, "music", map[bool]string{true: "MPRIS on the session bus" + map[bool]string{true: " · " + st.Track, false: ""}[st.Track != ""],
			false: "no session bus"}[st.Music], "")
		line(st.Volume, "volume", map[bool]string{true: "wpctl / pactl", false: "no PipeWire volume: no volume OSD"}[st.Volume], "install wireplumber and libpulse (pactl)")
		if st.Backlight {
			line(true, "brightness", "backlight via inotify, set via logind", "")
		} else {
			note("brightness", "no backlight (an external monitor?): no brightness OSD")
		}
		if st.Battery {
			line(true, "battery", "UPower", "")
		} else {
			note("battery", "no battery reported by UPower")
		}
		for _, p := range st.Plugins {
			note("plugin", p)
		}
		note("now", fmt.Sprintf("%d Claude session(s), %d request(s) waiting", st.Sessions, st.Pending))
	}

	h, _ := os.UserHomeDir()
	settings := filepath.Join(h, ".claude", "settings.json")
	n, stale := hookCount(settings, exe)
	switch {
	case n == 0:
		line(false, "claude hooks", "none in "+settings, "ember install")
	case stale > 0:
		line(false, "claude hooks", fmt.Sprintf("%d point at another ember binary", stale), "ember install")
	case n < 15:
		line(false, "claude hooks", fmt.Sprintf("%d of 15: from an older Ember", n), "ember install, then restart Claude sessions")
	default:
		line(true, "claude hooks", fmt.Sprintf("%d hooks in %s", n, settings), "")
	}

	if cfg := island.ConfigPath(); fileExists(cfg) {
		line(true, "config", cfg, "")
		warns, missing := island.CheckConfig(cfg)
		for _, w := range warns { // a bad value keeps its default: worth a look, not a failure
			note("config", strings.TrimPrefix(w, "config: "))
		}
		if len(missing) > 0 { // install never overwrites the file, so new keys don't reach it
			more := ""
			if len(missing) > 6 {
				missing, more = missing[:6], fmt.Sprintf(" and %d more", len(missing)-6)
			}
			note("config", "new settings your file doesn't have: "+strings.Join(missing, ", ")+more+" (see the README's Config)")
		}
	} else {
		note("config", "defaults (ember install writes "+cfg+")")
	}
	if p := filepath.Join(h, ".config", "alacritty", "themes", "noctalia.toml"); fileExists(p) {
		line(true, "palette", "follows Noctalia's wallpaper colours", "")
	} else {
		note("palette", "built-in colours (no Noctalia theme found)")
	}
	for _, t := range []string{"niri", "wpctl", "pactl", "xdg-open"} {
		if _, err := exec.LookPath(t); err != nil {
			note(t, "not found")
		}
	}
	if bad == 0 {
		fmt.Println("\nAll good.")
	} else {
		fmt.Printf("\n%d problem(s).\n", bad)
		os.Exit(1)
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// hookCount counts Ember's hooks in Claude's settings, and how many of them
// run some other binary than this one.
func hookCount(settings, exe string) (n, stale int) {
	raw, err := os.ReadFile(settings)
	if err != nil {
		return 0, 0
	}
	var cfg struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return 0, 0
	}
	for _, es := range cfg.Hooks {
		for _, e := range es {
			for _, h := range e.Hooks {
				if bin, _, ok := strings.Cut(h.Command, " hook "); ok && strings.HasSuffix(bin, "ember") {
					n++
					if bin != exe {
						stale++
					}
				}
			}
		}
	}
	return n, stale
}
