package source

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

// Health is how a source is doing, for `ember doctor`.
type Health struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`               // running now
	Detail string `json:"detail,omitempty"` // why it isn't: "pactl missing"
}

// Supervisor runs sources for good: one that returns (its tool missing,
// its bus gone, a pipe closed) runs again after a pause that doubles from
// Min to Max, and back to Min once a run has lasted Reset. Each cause is
// logged once, not on every retry.
type Supervisor struct {
	Min, Max, Reset time.Duration
	Logf            func(format string, args ...any)

	mu     sync.Mutex
	health map[string]Health
}

// NewSupervisor retries after 1 s, then 2, 4, … up to 5 min.
func NewSupervisor() *Supervisor {
	return &Supervisor{Min: time.Second, Max: 5 * time.Minute, Reset: time.Minute, Logf: log.Printf, health: map[string]Health{}}
}

func (s *Supervisor) set(h Health) {
	s.mu.Lock()
	s.health[h.Name] = h
	s.mu.Unlock()
}

// Health is every source's state, by name.
func (s *Supervisor) Health() []Health {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Health, 0, len(s.health))
	for _, h := range s.health {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Run runs fn as the source name until ctx ends. It blocks: start it with go.
func (s *Supervisor) Run(ctx context.Context, name string, fn func(Emit) error, emit Emit) {
	wait, last := s.Min, ""
	for {
		s.set(Health{Name: name, OK: true})
		start := time.Now()
		err := fn(emit)
		if ctx.Err() != nil {
			return
		}
		cause := "stopped"
		if err != nil {
			cause = err.Error()
		}
		s.set(Health{Name: name, Detail: cause})
		if time.Since(start) >= s.Reset {
			wait, last = s.Min, "" // it ran a good while: a new failure is news
		}
		if cause != last {
			s.Logf("%s: %s (trying again)", name, cause)
			last = cause
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, s.Max)
	}
}
