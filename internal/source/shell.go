package source

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

// The desktop shell (Noctalia): when it starts again, or a setting rebuilds
// its bar, the new bar is a newer surface on the island's layer and covers
// it, so the island raises itself once the bar is up.

// raiseSoon says ShellChanged after each delay (the bar maps a moment after
// the event).
func raiseSoon(emit Emit, why string, delays ...time.Duration) {
	go func() {
		for _, d := range delays {
			time.Sleep(d)
			emit(ShellChanged{Why: why})
		}
	}()
}

// Shell notices the shell restarting: its D-Bus names change owner.
func Shell(emit Emit) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	bus.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg0Namespace("dev.noctalia"))
	sig := make(chan *dbus.Signal, 8)
	bus.Signal(sig)
	var last time.Time
	for s := range sig {
		if len(s.Body) < 3 {
			continue
		}
		if owner, _ := s.Body[2].(string); owner == "" || time.Since(last) < 5*time.Second {
			continue // a name going away, or the same restart's other names
		}
		last = time.Now()
		raiseSoon(emit, "the shell restarted", 1500*time.Millisecond, 3*time.Second)
	}
}

// ShellSettings notices the shell's settings window or a config edit, which
// can rebuild the bar. Without Noctalia's directories it returns.
func ShellSettings(emit Emit) {
	h, _ := os.UserHomeDir()
	paths := map[string]uint32{}
	for _, d := range []string{filepath.Join(h, ".config", "noctalia"), filepath.Join(h, ".local", "state", "noctalia")} {
		paths[d] = unix.IN_CLOSE_WRITE | unix.IN_MOVED_TO
	}
	var raiseAt time.Time
	inotify(paths, nil, func(dir, name string) {
		if (name == "settings.toml" || strings.HasSuffix(name, ".toml") && strings.Contains(dir, ".config")) && time.Since(raiseAt) > 2*time.Second {
			raiseAt = time.Now()
			raiseSoon(emit, "the shell's settings changed", 700*time.Millisecond, 2*time.Second)
		}
	})
}
