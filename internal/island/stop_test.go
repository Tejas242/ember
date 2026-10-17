package island

import (
	"os"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// A stop leaves the sessions, habits, bond and day on disk, and no socket.
func TestCleanStop(t *testing.T) {
	posts := make(chan func(*app), 4)
	srv, err := listen(posts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ipc.SockPath()); err != nil {
		t.Fatal("no socket while listening")
	}
	clock := time.Now()
	a := newTestApp(&clock)
	a.ui.sessions["a"] = &Session{ID: "a", Project: "kinvault", Pids: []int{os.Getpid()}}
	a.ui.habits = &habits{Counts: map[string]map[string]float64{"x": {"music": 1}}, dirty: true}
	a.ui.bond = loadBond(clock)
	a.ui.day.dirty = true

	a.shutdown()
	srv.close()

	for _, p := range []string{sessionsPath(), habitsPath(), bondPath(), dayPath()} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s not saved: %v", p, err)
		}
	}
	if _, err := os.Stat(ipc.SockPath()); !os.IsNotExist(err) {
		t.Fatalf("socket left behind: %v", err)
	}
	b := &app{ui: NewUI(surfW, surfH, 22)}
	b.loadSessions()
	if b.ui.sessions["a"] == nil {
		t.Fatal("the saved sessions don't load")
	}
}
