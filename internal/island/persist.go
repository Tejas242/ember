package island

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/xdg"
)

// shutdown saves everything that outlives the island and stops the song's
// listener.
func (a *app) shutdown() {
	if a.stopAll != nil { // the plugins first: they have two seconds to go
		a.stopAll()
		a.host.Stop()
	}
	a.saveSessions()
	a.ui.habits.save()
	a.ui.bond.save()
	a.ui.day.save()
	a.ui.audio.Set(false)
}

// ---- sessions survive a restart of the island ----

func sessionsPath() string { return filepath.Join(filepath.Dir(ipc.SockPath()), "ember-sessions.json") }

func (a *app) saveSessions() {
	b, err := json.Marshal(a.ui.sessions)
	if err == nil {
		xdg.AtomicWrite(sessionsPath(), b, 0o600)
	}
}

func (a *app) loadSessions() {
	b, err := os.ReadFile(sessionsPath())
	if err != nil {
		return
	}
	var ss map[string]*Session
	if json.Unmarshal(b, &ss) != nil {
		return
	}
	for id, s := range ss {
		if slices.ContainsFunc(s.Pids, ipc.Alive) {
			a.ui.sessions[id] = s
		}
	}
}
