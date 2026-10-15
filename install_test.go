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
