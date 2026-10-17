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
