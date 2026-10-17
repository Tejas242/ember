package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/config"
	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/wayland"
)

// A settings.json with Ember's hooks under other names (a stale temp build,
// the old Noctalia script) and other tools' hooks, some sharing an entry.
const settingsFixture = `{
  "model": "opus",
  "permissions": {"allow": ["Bash(ls:*)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "*", "hooks": [{"type": "command", "command": "/tmp/claude-1000/x/ember-pd hook PreToolUse", "timeout": 5, "async": true}]},
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "/home/x/.local/bin/edge hook"}]},
      {"matcher": "Edit", "hooks": [{"type": "command", "command": "/home/x/.local/bin/edge hook PreToolUse"}]},
      {"matcher": "*", "hooks": [
        {"type": "command", "command": "'/home/x/old ember/ember' hook PreToolUse"},
        {"type": "command", "command": "notify-send hi"}
      ]}
    ],
    "PermissionRequest": [
      {"matcher": "*", "hooks": [{"type": "command", "command": "/tmp/go-build1/b001/exe/ember hook PermissionRequest", "timeout": 100}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "sh '/home/x/.config/noctalia/plugins/ember/hook' Stop"}]},
      {"hooks": [{"type": "command", "command": "echo ember hook Stop"}]},
      {"hooks": [{"type": "command", "command": "/usr/bin/ember-notes hook sync"}]}
    ]
  }
}`

func TestEmberHook(t *testing.T) {
	for cmd, want := range map[string]string{
		"/home/x/.local/bin/ember hook Stop":                    "/home/x/.local/bin/ember",
		"/tmp/claude-1000/x/ember-pd hook PreToolUse":           "/tmp/claude-1000/x/ember-pd",
		"ember hook SessionStart":                               "ember",
		`"/home/x/my bin/ember" hook PostToolUse`:               "/home/x/my bin/ember",
		"sh '/home/x/.config/noctalia/plugins/ember/hook' Stop": "",
	} {
		if bin, ok := emberHook(cmd); !ok || bin != want {
			t.Errorf("emberHook(%q) = %q %v, want %q", cmd, bin, ok, want)
		}
	}
	for _, cmd := range []string{"/home/x/.local/bin/edge hook", "/home/x/.local/bin/edge hook PreToolUse",
		"echo ember hook Stop", "/usr/bin/ember-notes hook sync", "ember", "hook Stop", "notify-send hi"} {
		if _, ok := emberHook(cmd); ok {
			t.Errorf("emberHook(%q) claims another tool's hook", cmd)
		}
	}
}

func TestPatchSettingsFixture(t *testing.T) {
	raw := []byte(settingsFixture)
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
	if n, stale := hookCountIn(out, exe); n != len(emberHooks) || stale != 0 {
		t.Fatalf("want exactly %d Ember hooks, all this binary's; got %d (%d stale)", len(emberHooks), n, stale)
	}
	if n := strings.Count(s, exe+" hook "); n != len(emberHooks) {
		t.Fatalf("%d hooks run %s", n, exe)
	}
	for _, gone := range []string{"ember-pd", "go-build", "old ember", "noctalia"} {
		if strings.Contains(s, gone) {
			t.Errorf("stale Ember hook %q kept", gone)
		}
	}
	kept := []string{`"/home/x/.local/bin/edge hook"`, `"/home/x/.local/bin/edge hook PreToolUse"`, `"notify-send hi"`,
		`"echo ember hook Stop"`, `"/usr/bin/ember-notes hook sync"`}
	for _, k := range kept {
		if !strings.Contains(s, k) {
			t.Errorf("lost another tool's hook %s", k)
		}
	}
	if n, stale := hookCountIn(raw, exe); n != 4 || stale != 4 {
		t.Errorf("doctor on the fixture: %d hooks, %d stale; want 4, 4", n, stale)
	}

	gone, _ := patchSettings(out, exe, false)
	if n, _ := hookCountIn(gone, exe); n != 0 {
		t.Fatalf("uninstall left %d Ember hooks", n)
	}
	for _, k := range kept {
		if !strings.Contains(string(gone), k) {
			t.Errorf("uninstall lost another tool's hook %s", k)
		}
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
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "```toml\n"+config.DefaultText+"```") {
		t.Fatal("README's Config section differs from config.DefaultText")
	}
}

// install refuses a binary that won't survive a reboot or a go clean.
func TestTransient(t *testing.T) {
	sandbox(t)
	fakeSystemctl(t)
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

// Claude waits on a request longer than the island and its client do, and
// never on anything else for long.
func TestHookTable(t *testing.T) {
	seen := map[string]bool{}
	for _, h := range emberHooks {
		key := h.event + "/" + h.matcher
		if seen[key] {
			t.Errorf("%s twice", key)
		}
		seen[key] = true
		if !h.async && h.timeout <= ipc.AskTimeout+5*time.Second {
			t.Errorf("%s: Claude would kill the hook (%v) before the island answers", key, h.timeout)
		}
		if h.async && h.timeout > 10*time.Second {
			t.Errorf("%s: async hook with a %v timeout", key, h.timeout)
		}
	}
	out, err := patchSettings([]byte(`{}`), "/opt/ember", true)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := hookCountIn(out, "/opt/ember"); n != len(emberHooks) {
		t.Fatalf("install wrote %d hooks, doctor counts %d", len(emberHooks), n)
	}
	if !strings.Contains(string(out), `"timeout": 100`) {
		t.Fatal("the request hooks' timeout isn't AskTimeout+10s")
	}
}

// fakeSystemctl stands in for systemctl --user and records what it was asked.
func fakeSystemctl(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	old := systemctl
	systemctl = func(args ...string) ([]byte, error) { calls = append(calls, strings.Join(args, " ")); return nil, nil }
	t.Cleanup(func() { systemctl = old })
	return &calls
}

func TestInstallNoClaude(t *testing.T) {
	h := sandbox(t)
	calls := fakeSystemctl(t)
	exe := "/opt/ember/bin/ember"
	if err := install(exe); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(h, ".claude")) {
		t.Fatal("install made a ~/.claude for a Claude Code that isn't there")
	}
	u, err := os.ReadFile(filepath.Join(h, ".config", "systemd", "user", "ember.service"))
	if err != nil || !strings.Contains(string(u), "ExecStart="+exe+"\n") {
		t.Fatalf("service: %v %q", err, u)
	}
	if c, err := os.ReadFile(filepath.Join(h, ".config", "ember", "config.toml")); err != nil || string(c) != config.DefaultText {
		t.Fatalf("config: %v", err)
	}
	if strings.Join(*calls, ";") != "daemon-reload;enable ember.service;restart ember.service" {
		t.Fatalf("systemctl calls %q", *calls)
	}
}

func TestInstallCreatesSettings(t *testing.T) {
	h := sandbox(t)
	fakeSystemctl(t)
	os.MkdirAll(filepath.Join(h, ".claude"), 0o755)
	exe := "/opt/ember/bin/ember"
	if err := install(exe); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(h, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if n, stale := hookCountIn(raw, exe); n != len(emberHooks) || stale != 0 {
		t.Fatalf("%d hooks (%d stale)", n, stale)
	}
	if fileExists(filepath.Join(h, ".claude", "settings.json.pre-ember")) {
		t.Fatal("backed up a file that didn't exist")
	}
}

func TestInstallKeepsSettings(t *testing.T) {
	h := sandbox(t)
	fakeSystemctl(t)
	settings := filepath.Join(h, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(settings), 0o755)
	os.WriteFile(settings, []byte(settingsFixture), 0o600)
	cfg := filepath.Join(h, ".config", "ember", "config.toml")
	os.MkdirAll(filepath.Dir(cfg), 0o755)
	os.WriteFile(cfg, []byte("clock = \"24h\"\n"), 0o644)

	exe := "/opt/ember/bin/ember"
	if err := install(exe); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(settings); fi.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode became %v", fi.Mode().Perm())
	}
	if b, _ := os.ReadFile(settings + ".pre-ember"); string(b) != settingsFixture {
		t.Fatal("no backup of the original")
	}
	raw, _ := os.ReadFile(settings)
	if n, stale := hookCountIn(raw, exe); n != len(emberHooks) || stale != 0 || !strings.Contains(string(raw), "edge hook") {
		t.Fatalf("%d hooks (%d stale): %s", n, stale, raw)
	}
	if b, _ := os.ReadFile(cfg); string(b) != "clock = \"24h\"\n" {
		t.Fatal("overwrote the config")
	}
	if es, _ := os.ReadDir(filepath.Dir(settings)); len(es) != 2 {
		t.Fatalf("left temp files: %v", es)
	}

	os.WriteFile(settings, []byte("{not json"), 0o600) // a broken file is left alone
	if err := install(exe); err == nil {
		t.Fatal("installed over settings it couldn't read")
	}
	if b, _ := os.ReadFile(settings); string(b) != "{not json" {
		t.Fatal("touched a settings file it couldn't parse")
	}
	os.WriteFile(settings, raw, 0o600)

	calls := fakeSystemctl(t)
	if err := uninstall(); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(settings)
	if n, _ := hookCountIn(raw, exe); n != 0 || !strings.Contains(string(raw), "edge hook") {
		t.Fatalf("uninstall left %d hooks: %s", n, raw)
	}
	if fileExists(filepath.Join(h, ".config", "systemd", "user", "ember.service")) || !fileExists(cfg) {
		t.Fatal("uninstall must remove the service and keep the config")
	}
	if len(*calls) != 2 {
		t.Fatalf("systemctl calls %q", *calls)
	}
}

func TestUnitText(t *testing.T) {
	u := unitText("/opt/ember/bin/ember")
	unitSec, serviceSec, _ := strings.Cut(u, "[Service]")
	for _, l := range []string{"StartLimitIntervalSec=60", "StartLimitBurst=5"} {
		if !strings.Contains(unitSec, "\n"+l+"\n") {
			t.Errorf("[Unit] lacks %s", l)
		}
	}
	for _, l := range []string{"ExecStart=/opt/ember/bin/ember", "Restart=always", "RestartSec=3",
		"RestartPreventExitStatus=78", "Environment=PATH=%h/.local/bin:/usr/local/bin:/usr/bin"} {
		if !strings.Contains(serviceSec, "\n"+l+"\n") {
			t.Errorf("[Service] lacks %s", l)
		}
	}
	if !strings.Contains(u, "[Install]\nWantedBy=graphical-session.target\n") {
		t.Error("no [Install]")
	}
}

// No Wayland is exit 78, the code the unit won't restart on.
func TestRunExit(t *testing.T) {
	if runExit(nil) != 0 || runExit(errors.New("boom")) != 1 ||
		runExit(fmt.Errorf("%w at /run/x", wayland.ErrNoWayland)) != 78 || exNoWayland != 78 {
		t.Fatal("wrong exit codes")
	}
}
