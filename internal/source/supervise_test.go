package source

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSupervisor(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	s := NewSupervisor()
	s.Min, s.Max, s.Reset = time.Millisecond, 4*time.Millisecond, time.Hour
	s.Logf = func(f string, a ...any) { mu.Lock(); logs = append(logs, fmt.Sprintf(f, a...)); mu.Unlock() }

	runs := 0
	gate := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx, "volume", func(Emit) error {
			runs++
			switch {
			case runs <= 5:
				return errors.New("pactl missing")
			case runs == 6:
				return errors.New("pactl subscribe ended")
			}
			close(gate)
			<-ctx.Done() // running, until the island stops
			return nil
		}, func(Event) {})
	}()
	<-gate
	if h := s.Health(); len(h) != 1 || !h[0].OK || h[0].Name != "volume" {
		t.Fatalf("running: %+v", h)
	}
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if runs != 7 {
		t.Fatalf("%d runs", runs)
	}
	if len(logs) != 2 || !strings.Contains(logs[0], "volume: pactl missing") || !strings.Contains(logs[1], "pactl subscribe ended") {
		t.Fatalf("one line per cause, got %q", logs)
	}
}

func TestSupervisorHealth(t *testing.T) {
	s := NewSupervisor()
	s.Min, s.Logf = time.Hour, func(string, ...any) {}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx, "battery", func(Emit) error { return errors.New("no battery") }, func(Event) {})
	for i := 0; i < 200; i++ {
		if h := s.Health(); len(h) == 1 && !h[0].OK {
			if h[0].Detail != "no battery" {
				t.Fatal(h)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("health never said why")
}
