package plugin

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseManifest(t *testing.T) {
	m, err := parseManifest(`
# a timer
name         = "pomodoro"
description  = "A focus timer # not a comment"
command      = './pomodoro'
args         = ["--minutes", "25", "say \"hi\""]
icon         = "timer"
capabilities = ["pill", "moment", "teleport", "pill"]
future_key   = "fine"

[targets]
docs  = "https://example.com/pomodoro"
again = ["pomodoro-cli", "restart"]
`)
	if err != nil {
		t.Fatal(err)
	}
	want := &Manifest{Name: "pomodoro", Description: "A focus timer # not a comment", Command: "./pomodoro",
		Args: []string{"--minutes", "25", `say "hi"`}, Icon: "timer", Caps: []string{"pill", "moment"},
		Targets: map[string][]string{"docs": {"https://example.com/pomodoro"}, "again": {"pomodoro-cli", "restart"}}}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("\n got %+v\nwant %+v", m, want)
	}
	if !IsURL(m.Targets["docs"]) || IsURL(m.Targets["again"]) {
		t.Fatal("IsURL")
	}
	for text, why := range map[string]string{
		`command = "x"`: "name",
		`name = "Bad Name"` + "\ncommand = \"x\"": "name",
		`name = "ok"`:                         "no command",
		`name = "ok"` + "\ncommand = [\"x\"]": "a string",
		`name = "ok"` + "\ncommand = \"x\"\n[targets]\nrun = \"make\"":    "a command is an array",
		`name = "ok"` + "\ncommand = \"x\"\n[other]":                      "unknown table",
		`name = "ok"` + "\ncommand = \"x\"\n[opens]\nember = \"party\"":   "one of music, idle",
		`name = "ok"` + "\ncommand = \"x\"\n[opens]\nember = [\"music\"]": "a string",
		`name = "ok` + "\ncommand = \"x\"":                                "unclosed",
		`name "ok"`:                                                       "key = value",
	} {
		if _, err := parseManifest(text); err == nil || !strings.Contains(err.Error(), why) {
			t.Errorf("%q: %v, want %q", text, err, why)
		}
	}
}

func TestParseManifestOpens(t *testing.T) {
	m, err := parseManifest("name = \"vibe\"\ncommand = \"ember\"\ngrants = [\"media\", \"entry\", \"root\"]\n[opens]\nember = \"music\"\nhint = \"Vibe along with the song\"\nlater = \"fine\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if m.Opens != "music" || m.Hint != "Vibe along with…" || !reflect.DeepEqual(m.Asks, []string{"media", "entry"}) {
		t.Fatalf("%+v", m)
	}
}
