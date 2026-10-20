package config

import (
	"image/color"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// The file `ember install` writes is the defaults, word for word.
func TestDefaultConfigText(t *testing.T) {
	c, warns, keys := parse(DefaultText)
	if len(warns) > 0 {
		t.Fatalf("the shipped config warns: %v", warns)
	}
	if !reflect.DeepEqual(c, Default()) {
		t.Fatalf("the shipped config isn't the defaults:\n got %+v\nwant %+v", c, Default())
	}
	if len(keys) < 20 {
		t.Fatalf("only %d keys shipped: %v", len(keys), keys)
	}
	// the commented plugin example parses too, once uncommented
	on := strings.ReplaceAll(DefaultText, "# [plugins.edge]\n# enabled", "[plugins.edge]\nenabled")
	if c, warns, _ := parse(on); len(warns) > 0 || c.Plugins["edge"]["enabled"] != "auto" {
		t.Fatalf("plugin example: %v %v", warns, c.Plugins)
	}
}

func TestConfigKeys(t *testing.T) {
	c, warns, _ := parse(`
clock = "24"
away_after_s = 300
[pill]
idle = "both"
[card]
hover_delay_ms = 0
steps = 2
others = 0
[moments]
done_s = 2.5
osd_ms = 900
[claude]
answer = false
wait_s = 30
settle_min = 10
agent_quiet_min = 5
[look]
opacity = 1
green = "#00ff00"
amber = '0x112233'
`)
	if len(warns) > 0 {
		t.Fatal(warns)
	}
	want := Default()
	want.Clock24, want.AwayAfter, want.Pill = true, 5*time.Minute, "both"
	want.HoverDelay, want.CardSteps, want.CardOthers = 0, 2, 0
	want.DoneFor, want.OSDFor = 2500*time.Millisecond, 900*time.Millisecond
	want.Answer, want.Wait, want.Settle, want.AgentQuiet = false, 30*time.Second, 10*time.Minute, 5*time.Minute
	want.Opacity, want.Green, want.Amber = 1, color.NRGBA{0, 255, 0, 255}, color.NRGBA{0x11, 0x22, 0x33, 255}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("\n got %+v\nwant %+v", c, want)
	}
	if c.PillFormat() != "Mon 2 Jan · 15:04" {
		t.Fatal(c.PillFormat())
	}
}

// Bad lines never break loading: each keeps its default and says why.
func TestConfigWarnings(t *testing.T) {
	c, warns, _ := parse(`clock = "25h"
hover_delay_ms = soon
[card]
steps = 9
colour = "red"
[look]
opacity = NaN
red = "crimson"
[show]
music = yes
[future]
whatever = 1
just a line
[plugins.edge]
enabled = "sometimes"
`)
	want := []string{
		"config: line 1: clock must be 12h or 24h",
		"config: line 2: hover_delay_ms must be a number from 0 to 5000",
		"config: line 4: card.steps must be a whole number from 1 to 4",
		"config: line 5: unknown key card.colour",
		"config: line 7: look.opacity must be a number from 0.3 to 1",
		`config: line 8: look.red must be a colour like "#72dd71", or "" to follow the theme`,
		"config: line 10: show.music must be true or false",
		"config: line 11: unknown section [future]",
		`config: line 13: expected key = value, got "just a line"`,
		"config: line 15: plugins.edge.enabled must be auto, true or false",
	}
	if !reflect.DeepEqual(warns, want) {
		t.Fatalf("warnings:\n%s", strings.Join(warns, "\n"))
	}
	d := Default()
	if c.Clock24 || c.HoverDelay != d.HoverDelay || c.CardSteps != d.CardSteps || c.Opacity != d.Opacity || !c.Music || c.Red != d.Red {
		t.Fatalf("bad values didn't keep their defaults: %+v", c)
	}
	if !c.PluginOn("edge", true) || c.PluginOn("edge", false) {
		t.Fatal("an unreadable enabled means auto")
	}
}

func TestPluginOn(t *testing.T) {
	for _, tc := range []struct {
		text           string
		detected, want bool
	}{
		{"", true, true},
		{"", false, false},
		{"[plugins.edge]\nenabled = \"auto\"", false, false},
		{"[plugins.edge]\nenabled = true", false, true},
		{"[plugins.edge]\nenabled = false", true, false},
		{"[show]\nedge = false", true, false},                                 // the old switch still turns it off
		{"[show]\nedge = false\n[plugins.edge]\nenabled = true", true, false}, // and wins
		{"[plugins.other]\nenabled = false", true, true},                      // someone else's table
	} {
		c, _, _ := parse(tc.text)
		if got := c.PluginOn("edge", tc.detected); got != tc.want {
			t.Errorf("%q detected=%v: PluginOn = %v, want %v", tc.text, tc.detected, got, tc.want)
		}
	}
	c, _, _ := parse("[plugins.\"notes\"]\nenabled = false # off\ndir = '~/notes'")
	if c.PluginOn("notes", true) || c.Plugins["notes"]["dir"] != "~/notes" {
		t.Fatalf("%v", c.Plugins)
	}
}

// doctor names the shipped keys an older file doesn't have; the old
// top-level hover_delay_ms counts as [card]'s.
func TestCheckConfig(t *testing.T) {
	p := t.TempDir() + "/config.toml"
	writeFile(t, p, "clock = \"24h\"\nhover_delay_ms = 100\n[show]\nmusic = maybe\n")
	warns, missing := Check(p)
	if len(warns) != 1 || !strings.Contains(warns[0], "line 4: show.music") {
		t.Fatal(warns)
	}
	if slices.Contains(missing, "clock") || slices.Contains(missing, "card.hover_delay_ms") || slices.Contains(missing, "show.music") ||
		!slices.Contains(missing, "card.steps") || !slices.Contains(missing, "look.opacity") {
		t.Fatal(missing)
	}
	writeFile(t, p, DefaultText)
	if warns, missing := Check(p); warns != nil || missing != nil {
		t.Fatal(warns, missing)
	}
}

func TestConfigParse(t *testing.T) {
	p := t.TempDir() + "/config.toml"
	writeFile(t, p, "clock = \"24h\" # comment\nhover_delay_ms = 100\n[show]\nosd = false\nmusic=false\n")
	c, warns := Load(p)
	if len(warns) > 0 || !c.Clock24 || c.HoverDelay.Milliseconds() != 100 || c.OSD || c.Music || !c.Claude {
		t.Fatalf("%+v", c)
	}
	if c, warns := Load(p + ".missing"); !reflect.DeepEqual(c, Default()) || warns != nil {
		t.Fatal("a missing file means defaults")
	}
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPluginsEnabled(t *testing.T) {
	for text, want := range map[string]map[string]bool{
		"":                                    {"edge": true, "pomodoro": true},
		"[plugins]\nenabled = \"all\"":        {"edge": true, "pomodoro": true},
		"[plugins]\nenabled = \"none\"":       {"edge": false, "pomodoro": false},
		"[plugins]\nenabled = [\"pomodoro\"]": {"edge": false, "pomodoro": true},
		"[plugins]\nenabled = ['edge', 'ci']\n[plugins.edge]\nenabled = false": {"edge": false, "ci": true, "pomodoro": false},
	} {
		c, warns, _ := parse(text)
		if len(warns) > 0 {
			t.Fatalf("%q: %v", text, warns)
		}
		for name, on := range want {
			if c.PluginOn(name, true) != on {
				t.Errorf("%q: %s on = %v", text, name, !on)
			}
		}
	}
	if _, warns, _ := parse("[plugins]\nenabled = \"some\"\ncolour = 1"); len(warns) != 2 {
		t.Fatalf("%v", warns)
	}
}
