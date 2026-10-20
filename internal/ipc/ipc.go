// Package ipc is how the ember command talks to the running island: one
// JSON line over a unix socket in the runtime dir, and, for a request that
// waits, one JSON line back.
package ipc

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/Tejas242/ember/internal/xdg"
)

// AskTimeout is how long a permission or question waits on the island
// before Claude falls back to its own terminal prompt.
const AskTimeout = 90 * time.Second

// SockPath is the island's socket.
func SockPath() string {
	return filepath.Join(xdg.RuntimeDir(), "ember.sock")
}

// Msg is one message to the island: a JSON object on one line.
type Msg struct {
	Type    string          `json:"type"`           // hook | plugin | edge | cmd | status | note | line
	Name    string          `json:"name,omitempty"` // plugin: which one
	Event   string          `json:"event,omitempty"`
	Wait    bool            `json:"wait,omitempty"`
	Pids    []int           `json:"pids,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	ID      string          `json:"id,omitempty"`
}

// Reply answers a hook that waits.
type Reply struct {
	Output json.RawMessage `json:"output,omitempty"`
}

// Status is what `ember doctor` reports about the running island.
type Status struct {
	Version     string   `json:"version"` // the running island's own version
	Pid         int      `json:"pid"`
	Uptime      string   `json:"uptime"`
	BarHeight   float32  `json:"barHeight"`
	Blur        bool     `json:"blur"`
	CursorShape bool     `json:"cursorShape"`
	Presence    bool     `json:"presence"`
	Niri        bool     `json:"niri"`
	Music       bool     `json:"music"`
	Track       string   `json:"track,omitempty"`
	Volume      bool     `json:"volume"`
	Backlight   bool     `json:"backlight"`
	Battery     bool     `json:"battery"`
	Sessions    int      `json:"sessions"`
	Pending     int      `json:"pending"`
	Config      string   `json:"config"`
	Sources     []Source `json:"sources"` // each watcher: running, or why not
	Extern      []Plugin `json:"extern"`  // the plugins in ~/.config/ember/plugins
}

// Plugin is how an external plugin is doing.
type Plugin struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	State       string    `json:"state"` // starting | running | waiting | suspended | stopped | off | broken
	Caps        []string  `json:"caps,omitempty"`
	Since       time.Time `json:"since"`
	Restarts    int       `json:"restarts"`
	Dropped     int       `json:"dropped"`
	Violations  int       `json:"violations"`
	LastError   string    `json:"lastError,omitempty"`
	Log         []string  `json:"log,omitempty"`
	Dir         string    `json:"dir,omitempty"`
}

// Source is one of the island's watchers (volume, battery, niri, …).
type Source struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}
