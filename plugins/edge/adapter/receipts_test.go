package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// fakeEdge puts an `edge` script first on PATH that records its arguments
// (and exits with code), and returns the file it writes to.
func fakeEdge(t *testing.T, code int) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$@\" >> " + out + "\nexit " + string(rune('0'+code)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "edge"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return out
}

func runWithReceipts(t *testing.T, fn func(h *plugin.Harness)) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stats.json")
	b, _ := os.ReadFile("testdata/stats.json")
	os.WriteFile(path, b, 0o644)
	h := plugin.NewHarness("edge", plugin.Caps, func(p *plugin.Plugin) error {
		return Run(p, Options{StatsPath: path, Now: func() time.Time { return clock }, Every: 20 * time.Millisecond,
			Action: func(string) error { return nil }})
	})
	if _, err := h.Wait("dashboard", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	fn(h)
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
}

func waitCalls(t *testing.T, file string, n int) []string {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if b, _ := os.ReadFile(file); strings.Count(string(b), "\n") >= n {
			return strings.Split(strings.TrimSpace(string(b)), "\n")
		}
	}
	b, _ := os.ReadFile(file)
	t.Fatalf("wanted %d edge calls, got %q", n, b)
	return nil
}

// Ember's seen / expired become `edge receipt …`, one process each, in order.
func TestReceiptsReachEdge(t *testing.T) {
	calls := fakeEdge(t, 0)
	runWithReceipts(t, func(h *plugin.Harness) {
		h.Send(plugin.Event{Type: "seen", Card: "offer:cf:1418C:2026-11-01:1"})
		h.Send(plugin.Event{Type: "expired", Card: "rep", Why: "fullscreen"})
		h.Send(plugin.Event{Type: "expired", Card: "late"}) // no why: unseen
		got := waitCalls(t, calls, 3)
		want := []string{"receipt seen offer:cf:1418C:2026-11-01:1", "receipt expired rep fullscreen", "receipt expired late unseen"}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("call %d: %q, want %q", i, got[i], want[i])
			}
		}
	})
}

// An older edge (no receipt command: it fails) or no edge at all is not an
// error, and the adapter keeps serving.
func TestReceiptsFailQuietly(t *testing.T) {
	calls := fakeEdge(t, 1)
	runWithReceipts(t, func(h *plugin.Harness) {
		h.Send(plugin.Event{Type: "seen", Card: "a"})
		waitCalls(t, calls, 1)
		t.Setenv("PATH", t.TempDir()) // and now no edge at all
		h.Send(plugin.Event{Type: "seen", Card: "b"})
		h.Send(plugin.Event{Type: "dash", State: "open"})
		if _, err := h.Wait("dashboard", 2*time.Second); err != nil {
			t.Fatal("the adapter stopped serving:", err)
		}
	})
}

// Receipts never block the adapter: with edge hanging, events still flow.
func TestReceiptsDontBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	b, _ := os.ReadFile("testdata/stats.json")
	os.WriteFile(path, b, 0o644)
	release := make(chan struct{})
	h := plugin.NewHarness("edge", plugin.Caps, func(p *plugin.Plugin) error {
		return Run(p, Options{StatsPath: path, Now: func() time.Time { return clock }, Every: 20 * time.Millisecond,
			Action: func(string) error { return nil }, Receipt: func(...string) error { <-release; return nil }})
	})
	if _, err := h.Wait("dashboard", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		h.Send(plugin.Event{Type: "seen", Card: "x"})
	}
	h.Send(plugin.Event{Type: "dash", State: "open"})
	if _, err := h.Wait("dashboard", 2*time.Second); err != nil {
		t.Fatal("blocked by a slow edge:", err)
	}
	close(release)
	h.Stop()
}
