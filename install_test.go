package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tejas242/ember/island"
)

func TestPatchSettingsOnRealCopy(t *testing.T) {
	h, _ := os.UserHomeDir()
	raw, err := os.ReadFile(filepath.Join(h, ".claude", "settings.json"))
	if err != nil {
		t.Skip("no settings")
	}
	exe := "/home/x/.local/bin/ember"
	out, err := patchSettings(raw, exe, true)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := patchSettings(out, exe, true)
	if string(again) != string(out) {
		t.Fatal("not idempotent")
	}
	s := string(out)
	if n := strings.Count(s, exe+" hook "); n != 15 || strings.Contains(s, "ember/hook'") {
		t.Fatalf("want exactly 15 Ember entries and no old plugin ones, got %d", n)
	}
	if !strings.Contains(s, "edge hook") {
		t.Fatal("must keep edge's gate hooks")
	}
	gone, _ := patchSettings(out, exe, false)
	if strings.Contains(string(gone), "ember hook") || !strings.Contains(string(gone), "edge hook") {
		t.Fatal("uninstall must remove Ember's hooks and only those")
	}
	var before, after map[string]any
	json.Unmarshal(raw, &before)
	json.Unmarshal(out, &after)
	for k, v := range before {
		if k != "hooks" && fmt.Sprint(after[k]) != fmt.Sprint(v) {
			t.Errorf("changed unrelated key %s", k)
		}
	}
}

// The README shows the config file exactly as install writes it.
func TestReadmeConfig(t *testing.T) {
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "```toml\n"+island.DefaultConfigText+"```") {
		t.Fatal("README's Config section differs from island.DefaultConfigText")
	}
}

// install refuses a binary that won't survive a reboot or a go clean.
func TestTransient(t *testing.T) {
	sandbox(t)
	t.Setenv("TMPDIR", "/scratch/tmp")
	for exe, bad := range map[string]bool{
		"/tmp/go-build1234/b001/exe/ember":        true,
		"/home/x/.cache/go-build/ab/ember":        true,
		"/scratch/tmp/ember-pd":                   true,
		"/tmp/claude-1000/scratchpad/ember":       true,
		"/home/x/work/go-build555/b001/exe/ember": true,
		"/home/x/.local/bin/ember":                false,
		"/usr/local/bin/ember":                    false,
		"/home/x/Gems/ember/ember":                false,
		"/tmpfoo/ember":                           false,
	} {
		if got := transient(exe) != ""; got != bad {
			t.Errorf("transient(%s) = %v, want %v", exe, got, bad)
		}
	}
	err := install("/tmp/go-build1/b001/exe/ember")
	if err == nil || !strings.Contains(err.Error(), "make install") || !strings.Contains(err.Error(), "~/.local/bin") {
		t.Fatalf("install from go run: %v", err)
	}
}

// sandbox points every directory install could touch at a fresh temp dir, so
// a test can never reach the real settings, service or state.
func sandbox(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	t.Setenv("HOME", d)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(d, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(d, ".local", "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(d, "run"))
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(d, "run", "bus"))
	return d
}
