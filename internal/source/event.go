// Package source watches the desktop for the island: the media players
// (MPRIS), the battery (UPower), Bluetooth audio (BlueZ), the volume and
// microphone (PipeWire/PulseAudio), the backlight and the screenshots folder
// (inotify), the shell (Noctalia), niri's event stream, and the song's sound
// (parec). Each watcher sleeps until the system tells it something, and says
// what changed as a value Event through an Emit; it never touches the
// island's state. The island turns events into moments on its own loop.
package source

// Event is something a source noticed.
type Event interface{ event() }

// Emit hands an event to the island. It may block while the island is busy.
type Emit func(Event)

// ---- niri ----

// Window is a window as niri reports it.
type Window struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	AppID   string `json:"app_id"`
	Pid     int    `json:"pid"`
	Focused bool   `json:"is_focused"`
}

// NiriWindows is every window: the stream (re)started.
type NiriWindows struct{ Windows []Window }

// NiriWindow is a window opened or changed (its title, its focus).
type NiriWindow struct{ Window Window }

// NiriWindowClosed is a window gone.
type NiriWindowClosed struct{ ID int }

// NiriFocus is the focused window, 0 for none.
type NiriFocus struct{ ID int }

// NiriCasts is how many casts (screen shares, recordings) are running.
type NiriCasts struct{ N int }

// NiriDown is the stream ending: niri restarted, or isn't the compositor.
type NiriDown struct{}

// Screenshot is a screenshot taken: the file, or "" when it only went to
// the clipboard.
type Screenshot struct{ Path string }

func (NiriWindows) event()      {}
func (NiriWindow) event()       {}
func (NiriWindowClosed) event() {}
func (NiriFocus) event()        {}
func (NiriCasts) event()        {}
func (NiriDown) event()         {}
func (Screenshot) event()       {}

// ---- the system ----

// Power is the battery (UPower's display device). First is the reading at
// start, to show quietly.
type Power struct {
	Pct      float64
	State    uint32 // 1 charging, 2 discharging, 4 full, 5 pending charge
	Present  bool
	TTE, TTF int64 // seconds to empty / to full, 0 if unknown
	First    bool
}

// Device is a Bluetooth device connecting (On) or going; Pct is its
// battery, -1 if unknown.
type Device struct {
	Name, Kind string // Kind is BlueZ's icon name: audio-headset, …
	On         bool
	Pct        float64
}

// Volume is the default output's level (0..1, more when boosted). First is
// the reading at start, to show quietly; OK is false when it can't be read.
type Volume struct {
	V         float64
	Muted, OK bool
	First     bool
}

// Mic is the default input muted or not.
type Mic struct{ Muted bool }

// Brightness is the backlight, 0..1. First is the reading at start.
type Brightness struct {
	V     float64
	First bool
}

// ShellChanged: the desktop shell restarted or rebuilt its bar, which now
// covers the island: raise it.
type ShellChanged struct{ Why string }

func (Power) event()        {}
func (Device) event()       {}
func (Volume) event()       {}
func (Mic) event()          {}
func (Brightness) event()   {}
func (ShellChanged) event() {}
