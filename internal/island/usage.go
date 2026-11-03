package island

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Tejas242/ember/internal/xdg"
)

// The private usage log: one JSON line per thing worth measuring, appended to
// ~/.local/state/ember/events.jsonl. It exists so the user can look at how Ember
// is really used (what he opens, what he ignores, what it costs). It never
// leaves the machine (no network code here), shows nothing on screen, and
// holds no text from Claude or from a plugin: kinds, ids, counts and times.
//
// Every line carries ts, the Ember version and edge's phase (baseline,
// trial1, trial2: read from edge's stats file, so the lines of a trial can be
// told from the baseline). Use it from anywhere with one call:
//
//	u.Log("open", "via", "hover", "dwell_ms", 1800)

const (
	usageMax   = 1 << 20          // rotate at 1 MB
	phaseEvery = 30 * time.Second // how often the phase is looked up again
)

// UsageLog appends to a file. The zero *UsageLog does nothing (tests, and an
// Ember whose state directory can't be written).
type UsageLog struct {
	path, version string
	statsPath     string // edge's stats.json, for the phase
	size          int64
	phase         string
	phaseAt       time.Time
	now           func() time.Time
}

// NewUsageLog logs to path (events.jsonl in Ember's state directory when empty).
func NewUsageLog(path, version string) *UsageLog {
	if path == "" {
		path = filepath.Join(xdg.StateHome(), "ember", "events.jsonl")
	}
	l := &UsageLog{path: path, version: version, now: time.Now,
		statsPath: filepath.Join(xdg.RuntimeDir(), "edge", "stats.json")}
	if fi, err := os.Stat(path); err == nil {
		l.size = fi.Size()
	}
	return l
}

// Log adds one line: kind, then key/value pairs. Failures are silent: a log
// must never cost the island anything.
func (u *UI) Log(kind string, kv ...any) {
	if u.usage != nil {
		u.usage.add(kind, kv)
	}
}

func (l *UsageLog) add(kind string, kv []any) {
	now := l.now()
	m := map[string]any{"ts": now.UTC().Format("2006-01-02T15:04:05.000Z"), "ember": l.version, "phase": l.phaseOf(now), "k": kind}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	b = append(b, '\n')
	if l.size+int64(len(b)) > usageMax {
		os.Rename(l.path, l.path+".1") // one old file is kept; the one before it goes
		l.size = 0
	}
	if os.MkdirAll(filepath.Dir(l.path), 0o700) != nil {
		return
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	n, _ := f.Write(b)
	f.Close()
	l.size += int64(n)
}

// phaseOf is edge's phase, looked up at most every phaseEvery ("" when edge
// isn't running or has none).
func (l *UsageLog) phaseOf(now time.Time) string {
	if !l.phaseAt.IsZero() && now.Sub(l.phaseAt) < phaseEvery {
		return l.phase
	}
	l.phaseAt, l.phase = now, ""
	if b, err := os.ReadFile(l.statsPath); err == nil {
		var s struct {
			Phase string `json:"phase"`
		}
		if json.Unmarshal(b, &s) == nil {
			l.phase = s.Phase
		}
	}
	return l.phase
}
