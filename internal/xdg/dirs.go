package xdg

import (
	"os"
	"path/filepath"
)

// ConfigHome is $XDG_CONFIG_HOME, else ~/.config.
func ConfigHome() string { return dir("XDG_CONFIG_HOME", ".config") }

// StateHome is $XDG_STATE_HOME, else ~/.local/state.
func StateHome() string { return dir("XDG_STATE_HOME", filepath.Join(".local", "state")) }

// RuntimeDir is $XDG_RUNTIME_DIR, else the temp dir.
func RuntimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}

func dir(env, fallback string) string {
	if d := os.Getenv(env); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, fallback)
}
