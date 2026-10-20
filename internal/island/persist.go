package island

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"

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
		save(sessionsPath(), b, 0o600)
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

// logged is what failed already: each failure is logged once, not on every
// retry (state saves every ten seconds, hooks on every tool call).
var logged sync.Map

// logOnce logs the first failure of its key.
func logOnce(key, format string, args ...any) {
	if _, dup := logged.LoadOrStore(key, true); !dup {
		log.Printf(format, args...)
	}
}

// save writes one of the island's state files, and says once if it can't.
func save(path string, data []byte, perm os.FileMode) {
	if err := xdg.AtomicWrite(path, data, perm); err != nil {
		logOnce("save:"+path, "can't save %s: %v (this is said once)", path, err)
	}
}
