package main

import (
	"bytes"
	"strings"
	"testing"
)

// route never runs a command on arguments it didn't ask for: help is help,
// and anything unexpected is an error before the command starts.
func TestRoute(t *testing.T) {
	for _, tc := range []struct {
		argv     []string
		run      string // the command that would run, "" for none
		code     int
		out, err string // substrings of stdout / stderr
	}{
		{argv: nil, run: "run"},
		{argv: []string{"run"}, run: "run"},
		{argv: []string{"install"}, run: "install"},
		{argv: []string{"install", "--help"}, code: 0, out: "usage: ember install\n"},
		{argv: []string{"install", "-h"}, code: 0, out: "usage: ember install\n"},
		{argv: []string{"uninstall", "--help"}, code: 0, out: "usage: ember uninstall\n"},
		{argv: []string{"install", "--force"}, code: 2, err: `ember install: unexpected argument "--force" (see ember help install)`},
		{argv: []string{"uninstall", "now"}, code: 2, err: `ember uninstall: unexpected argument "now"`},
		{argv: []string{"allow", "x"}, code: 2, err: `ember allow: unexpected argument "x"`},
		{argv: []string{"open"}, run: "open"},
		{argv: []string{"open", "music"}, run: "open"},
		{argv: []string{"open", "music", "x"}, code: 2, err: `ember open: unexpected argument "x"`},
		{argv: []string{"answer"}, code: 2, err: "ember answer: needs an answer: opt:N or submit (see ember help answer)"},
		{argv: []string{"answer", "opt:1"}, run: "answer"},
		{argv: []string{"answer", "--help"}, code: 0, out: "usage: ember answer opt:N | submit"},
		{argv: []string{"hook"}, code: 2, err: "ember hook: needs a Claude Code hook event"},
		{argv: []string{"hook", "Stop"}, run: "hook"},
		{argv: []string{"hook", "Stop", "x"}, code: 2, err: `unexpected argument "x"`},
		{argv: []string{"edge"}, code: 2, err: "ember edge: needs a JSON message"},
		{argv: []string{"edge", `{"a":1}`}, run: "edge"},
		{argv: []string{"--version"}, run: "version"},
		{argv: []string{"-v"}, run: "version"},
		{argv: []string{"version", "x"}, code: 2, err: `ember version: unexpected argument "x"`},
		{argv: []string{"help"}, run: "help"},
		{argv: []string{"--help"}, run: "help"},
		{argv: []string{"help", "install"}, run: "help"},
		{argv: []string{"help", "a", "b"}, code: 2, err: `ember help: unexpected argument "b"`},
		{argv: []string{"bogus"}, code: 2, err: "ember: unknown command \"bogus\"\nrun ember help\n"},
		{argv: []string{"--bogus"}, code: 2, err: `ember: unknown command "--bogus"`},
	} {
		var out, errb bytes.Buffer
		c, _, _, code := route(tc.argv, &out, &errb)
		name := ""
		if c != nil {
			name = c.name
		}
		if name != tc.run || code != tc.code || !strings.Contains(out.String(), tc.out) || !strings.Contains(errb.String(), tc.err) {
			t.Errorf("%q: ran %q code %d out %q err %q; want %q %d %q %q", tc.argv, name, code, out.String(), errb.String(), tc.run, tc.code, tc.out, tc.err)
		}
		if tc.run == "" && tc.code == 0 && errb.Len() > 0 {
			t.Errorf("%q: help wrote to stderr: %q", tc.argv, errb.String())
		}
	}
}

// Every command has a run, a summary, sane bounds, and a need when it needs
// an argument; every name and alias is unique; help lists them all.
func TestCommandTable(t *testing.T) {
	seen := map[string]bool{}
	ov := overview()
	for _, c := range commands {
		if (c.run == nil && c.runf == nil) || c.summary == "" || c.min > c.max || (c.min > 0 && c.need == "") {
			t.Errorf("%s: incomplete entry", c.name)
		}
		for _, n := range append([]string{c.name}, c.aliases...) {
			if seen[n] {
				t.Errorf("%s declared twice", n)
			}
			seen[n] = true
		}
		if !c.hidden && !strings.Contains(ov, c.short()) {
			t.Errorf("help doesn't list %s", c.short())
		}
	}
}

// `ember help <cmd>` prints what `ember <cmd> --help` does.
func TestHelpCommand(t *testing.T) {
	for _, name := range []string{"install", "answer", "--version"} {
		var a, b, e bytes.Buffer
		if code := help([]string{name}, &a, &e); code != 0 || e.Len() > 0 {
			t.Fatalf("help %s: code %d err %q", name, code, e.String())
		}
		route([]string{lookup(name).name, "--help"}, &b, &e)
		if a.String() != b.String() || a.Len() == 0 {
			t.Errorf("help %s = %q, --help = %q", name, a.String(), b.String())
		}
	}
	var o, e bytes.Buffer
	if code := help([]string{"bogus"}, &o, &e); code != 2 || e.String() != "ember: unknown command \"bogus\"\nrun ember help\n" {
		t.Errorf("help bogus: %d %q", code, e.String())
	}
	if code := help(nil, &o, &e); code != 0 || !strings.Contains(o.String(), "ember install") {
		t.Errorf("help: %d %q", code, o.String())
	}
}
