package island

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitWayland(t *testing.T) {
	d := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", d)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	old := waylandWait
	t.Cleanup(func() { waylandWait = old })

	waylandWait = 0
	if err := waitWayland(); !errors.Is(err, ErrNoWayland) {
		t.Fatalf("no socket: %v", err)
	}

	waylandWait = 3 * time.Second // a compositor still starting
	go func() {
		time.Sleep(150 * time.Millisecond)
		os.WriteFile(filepath.Join(d, "wayland-test"), nil, 0o600)
	}()
	if err := waitWayland(); err != nil {
		t.Fatalf("socket appeared: %v", err)
	}
}
