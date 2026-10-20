package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

func TestListPlugins(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	ps := []ipc.Plugin{
		{Name: "pomodoro", Description: "Focus sessions", State: "running", Since: now.Add(-95 * time.Minute), Dropped: 3},
		{Name: "ci", State: "suspended", LastError: "suspended: exited 5 times within 10m0s (exit status 1)", Restarts: 4},
		{Name: "bad", State: "broken", LastError: "plugin.toml: no command"},
		{Name: "quiet", State: "off"},
	}
	var b bytes.Buffer
	listPlugins(&b, ps, true, now)
	want := `pomodoro         running for 1h 35m (3 dropped)
                 Focus sessions
ci               suspended: exited 5 times within 10m0s (exit status 1) (4 restarts)
bad              broken: plugin.toml: no command
quiet            off
`
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
	b.Reset()
	listPlugins(&b, nil, false, now)
	if !strings.Contains(b.String(), "isn't running") || !strings.Contains(b.String(), "No plugins in") {
		t.Fatal(b.String())
	}
	b.Reset()
	pluginDetail(&b, ipc.Plugin{Name: "ci", State: "waiting", LastError: "exit status 2", Caps: []string{"pill"}, Log: []string{"error: no token"}, Dir: "/x/ci"}, true)
	if s := b.String(); !strings.Contains(s, "ci: waiting: exit status 2") || !strings.Contains(s, "caps: pill") || !strings.Contains(s, "    error: no token") {
		t.Fatal(s)
	}
}

func TestPluginsOnDisk(t *testing.T) {
	for name, toml := range map[string]string{
		"pomodoro": "name = \"pomodoro\"\ncommand = \"./p\"\ncapabilities = [\"pill\"]\n",
		"bad":      "name = \"bad\"\n",
	} {
		dir := filepath.Join(pluginsDir(), name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(toml), 0o644)
	}
	defer os.RemoveAll(pluginsDir())
	ps := pluginsOnDisk()
	if len(ps) != 2 || ps[0].Name != "bad" || ps[0].State != "broken" || ps[1].State != "installed" || ps[1].Caps[0] != "pill" {
		t.Fatalf("%+v", ps)
	}
}
