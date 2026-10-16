package island

import (
	"os"
	"reflect"
	"testing"
)

func TestSessionTitle(t *testing.T) {
	for in, want := range map[string]string{
		"✳ Ember end-to-end product": "Ember end-to-end product",
		"◐ Fix the flaky test":       "Fix the flaky test",
		"⠂ Claude Code":              "",
		"user@host:~":                "",
		"~/src/ember":                "",
		"nvim main.go":               "",
		"":                           "",
	} {
		if got := sessionTitle(in); got != want {
			t.Errorf("sessionTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigParse(t *testing.T) {
	p := t.TempDir() + "/config.toml"
	writeFile(t, p, "clock = \"24h\" # comment\nhover_delay_ms = 100\n[show]\nosd = false\nmusic=false\n")
	c, warns := loadConfig(p)
	if len(warns) > 0 || !c.Clock24 || c.HoverDelay.Milliseconds() != 100 || c.OSD || c.Music || !c.Claude {
		t.Fatalf("%+v", c)
	}
	if c, warns := loadConfig(p + ".missing"); !reflect.DeepEqual(c, DefaultConfig()) || warns != nil {
		t.Fatal("a missing file means defaults")
	}
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
