// Package testenv makes a test binary hermetic: it points HOME and every XDG
// directory at a fresh temp dir before any test runs, so no test reads this
// machine's theme, config, state or Claude settings, or reaches the running
// island, the compositor, niri or the session bus. Only tests import it.
package testenv

import (
	"os"
	"path/filepath"
	"testing"
)

// Main runs m in a sandboxed environment and exits with its code. A package
// uses it as its TestMain:
//
//	func TestMain(m *testing.M) { testenv.Main(m) }
func Main(m *testing.M) {
	dir, err := os.MkdirTemp("", "ember-test-")
	if err != nil {
		panic(err)
	}
	Sandbox(dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// Sandbox sets the environment to live under dir.
func Sandbox(dir string) {
	run := filepath.Join(dir, "run")
	os.MkdirAll(run, 0o700)
	for k, v := range map[string]string{
		"HOME":                     dir,
		"XDG_CONFIG_HOME":          filepath.Join(dir, ".config"),
		"XDG_STATE_HOME":           filepath.Join(dir, ".local", "state"),
		"XDG_DATA_HOME":            filepath.Join(dir, ".local", "share"),
		"XDG_CACHE_HOME":           filepath.Join(dir, ".cache"),
		"XDG_RUNTIME_DIR":          run,
		"WAYLAND_DISPLAY":          "wayland-test-none",
		"NIRI_SOCKET":              filepath.Join(run, "niri-none.sock"),
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=" + filepath.Join(run, "bus-none"),
	} {
		os.Setenv(k, v)
	}
}
