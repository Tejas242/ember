package wayland

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
	if err := WaitSocket(0); !errors.Is(err, ErrNoWayland) {
		t.Fatalf("no socket: %v", err)
	}

	go func() { // a compositor still starting
		time.Sleep(150 * time.Millisecond)
		os.WriteFile(filepath.Join(d, "wayland-test"), nil, 0o600)
	}()
	if err := WaitSocket(3 * time.Second); err != nil {
		t.Fatalf("socket appeared: %v", err)
	}
}
