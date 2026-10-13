package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchSettingsOnRealCopy(t *testing.T) {
	h, _ := os.UserHomeDir()
	raw, err := os.ReadFile(filepath.Join(h, ".claude", "settings.json"))
	if err != nil {
		t.Skip("no settings")
	}
	exe := "/home/x/.local/bin/ember"
	out, err := patchSettings(raw, exe)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := patchSettings(out, exe)
	if string(again) != string(out) {
		t.Fatal("not idempotent")
	}
	s := string(out)
	if n := strings.Count(s, exe+" hook "); n != 14 || strings.Contains(s, "ember/hook'") {
		t.Fatalf("want exactly 14 Ember entries and no old plugin ones, got %d", n)
	}
	if !strings.Contains(s, "edge hook") {
		t.Fatal("must keep edge's gate hooks")
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
