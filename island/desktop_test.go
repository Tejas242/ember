package island

import (
	"os"
	"testing"
)

func TestSessionTitle(t *testing.T) {
	for in, want := range map[string]string{
		"✳ Ember end-to-end product": "Ember end-to-end product",
		"◐ Fix the flaky test":       "Fix the flaky test",
		"⠂ Claude Code":              "",
		"screenager@creeper:~":       "",
		"~/Gems/ember":               "",
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
	c := loadConfig(p)
	if !c.Clock24 || c.HoverDelay.Milliseconds() != 100 || c.OSD || c.Music || !c.Claude {
		t.Fatalf("%+v", c)
	}
	if loadConfig(p+".missing") != DefaultConfig() {
		t.Fatal("a missing file means defaults")
	}
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
