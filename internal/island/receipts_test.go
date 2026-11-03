package island

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func rcpCard(t *testing.T, g *galleryScene, id string, ttl int) {
	t.Helper()
	g.a.applyExt("edge", extMsg(t, `{"type":"card","id":"`+id+`","title":"Today","body":"b","ttl_s":`+itoa(int64(ttl))+`}`))
}

// receipts: what the fake host heard, as "type card why".
func rcpHeard(h *fakeHost) []string {
	var out []string
	for _, ev := range h.sent {
		if ev.Type == "seen" || ev.Type == "expired" {
			out = append(out, strings.TrimSpace(ev.Type+" "+ev.Card+" "+ev.Why))
		}
	}
	return out
}

func rcpSetup() (*galleryScene, *fakeHost) {
	g := newScene()
	h := &fakeHost{}
	g.a.host = h
	return g, h
}

// "seen" goes out once, on the first frame that draws the card: not when it is
// queued, and not again as the card stays up.
func TestReceiptSeenOnFirstDraw(t *testing.T) {
	g, h := rcpSetup()
	rcpCard(t, g, "offer:1", 30)
	if got := rcpHeard(h); len(got) != 0 {
		t.Fatalf("heard %v before any frame", got)
	}
	g.a.ui.expire() // a timer wake isn't a draw
	if got := rcpHeard(h); len(got) != 0 {
		t.Fatalf("heard %v from a timer wake", got)
	}
	g.run(2 * time.Second)
	g.run(2 * time.Second)
	if got := rcpHeard(h); len(got) != 1 || got[0] != "seen offer:1" {
		t.Fatalf("heard %v", got)
	}
	// same card again (new words): still one card, no second receipt
	rcpCard(t, g, "offer:1", 30)
	g.run(time.Second)
	if got := rcpHeard(h); len(got) != 1 {
		t.Fatalf("heard %v after the card was re-sent", got)
	}
	// and once seen, running out is not an expiry
	g.clock = g.clock.Add(31 * time.Second)
	g.a.ui.expire()
	if got := rcpHeard(h); len(got) != 1 {
		t.Fatalf("a seen card expired: %v", got)
	}
}

// Held back, a card is not seen; running out says why.
func TestReceiptExpiredWhy(t *testing.T) {
	cases := []struct {
		name string
		hold func(g *galleryScene)
		why  string
	}{
		{"fullscreen", func(g *galleryScene) { g.a.ui.full = true }, "fullscreen"},
		{"away", func(g *galleryScene) { g.a.ui.mind.away = true }, "away"},
		{"osd", func(g *galleryScene) {
			g.a.ui.full = true
			g.a.ui.Moment(moment{key: "osd:vol", osd: true, until: g.clock.Add(time.Minute)})
		}, "osd"},
		{"unseen", func(g *galleryScene) {}, "unseen"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, h := rcpSetup()
			c.hold(g)
			rcpCard(t, g, "offer:2", 10)
			if c.name == "unseen" { // a card in front of it: this one never gets its turn
				rcpCard(t, g, "front", 60)
				g.a.ui.cards[0], g.a.ui.cards[1] = g.a.ui.cards[1], g.a.ui.cards[0]
				if g.a.ui.top().ID != "x:edge:front" {
					g.a.ui.cards[0], g.a.ui.cards[1] = g.a.ui.cards[1], g.a.ui.cards[0]
				}
			}
			g.run(time.Second)
			g.clock = g.clock.Add(11 * time.Second)
			g.a.ui.expire()
			want := "expired offer:2 " + c.why
			got := rcpHeard(h)
			if c.name == "unseen" {
				if len(got) != 2 || got[0] != "seen front" || got[1] != want {
					t.Fatalf("heard %v", got)
				}
				return
			}
			if len(got) != 1 || got[0] != want {
				t.Fatalf("heard %v, want %q", got, want)
			}
		})
	}
}

// Released from the hold, the card is drawn and seen at last.
func TestReceiptSeenAfterHold(t *testing.T) {
	g, h := rcpSetup()
	g.a.ui.full = true
	rcpCard(t, g, "offer:3", 60)
	g.run(time.Second)
	g.a.ui.full = false
	g.run(time.Second)
	if got := rcpHeard(h); len(got) != 1 || got[0] != "seen offer:3" {
		t.Fatalf("heard %v", got)
	}
}

// The plugin taking a card back before it was drawn is "replaced".
func TestReceiptRemovedUnseen(t *testing.T) {
	g, h := rcpSetup()
	g.a.ui.full = true
	rcpCard(t, g, "offer:4", 60)
	g.run(time.Second)
	g.a.applyExt("edge", extMsg(t, `{"type":"remove","what":"card","id":"offer:4"}`))
	g.a.ui.expire()
	if got := rcpHeard(h); len(got) != 1 || got[0] != "expired offer:4 replaced" {
		t.Fatalf("heard %v", got)
	}
}

// The usage log gets one "card" line when a card ends: how long it was up
// and what came of it, with ts, the version and edge's phase on every line.
func TestUsageLogCard(t *testing.T) {
	g, _ := rcpSetup()
	dir := t.TempDir()
	stats := filepath.Join(dir, "stats.json")
	os.WriteFile(stats, []byte(`{"phase":"trial1"}`), 0o644)
	l := NewUsageLog(filepath.Join(dir, "events.jsonl"), "v1.3.0")
	l.statsPath, l.now = stats, func() time.Time { return g.clock }
	g.a.ui.usage = l
	g.a.ui.onKey = g.a.key
	rcpCard(t, g, "offer:5", 60)
	g.run(3 * time.Second)
	c := g.a.ui.top()
	g.a.ui.RemoveCard(c.ID)
	c.OnPick("start") // a button: removed, then picked
	g.a.ui.expire()
	b, _ := os.ReadFile(l.path)
	var m map[string]any
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"k":"surface"`) { // the card growing, then its end
		t.Fatalf("lines %q", lines)
	}
	if err := json.Unmarshal([]byte(lines[1]), &m); err != nil {
		t.Fatalf("%v: %q", err, b)
	}
	if m["k"] != "card" || m["res"] != "acted" || m["id"] != "x:edge:offer:5" || m["ember"] != "v1.3.0" || m["phase"] != "trial1" || m["ts"] == nil {
		t.Fatalf("%v", m)
	}
	if ms, _ := m["visible_ms"].(float64); ms < 2500 || ms > 3500 {
		t.Fatalf("visible_ms %v", m["visible_ms"])
	}
}

// No usage log, no files: the UI without one logs nothing and doesn't mind.
func TestUsageLogOffIsQuiet(t *testing.T) {
	g, _ := rcpSetup()
	rcpCard(t, g, "q", 10)
	g.run(time.Second)
	g.a.ui.Log("open", "via", "hover")
	g.clock = g.clock.Add(11 * time.Second)
	g.a.ui.expire()
}

// At 1 MB the file moves to events.jsonl.1 and a new one starts; the old
// one is replaced the next time, so the log stays about 2 MB at most.
func TestUsageLogRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "events.jsonl")
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	l := NewUsageLog(path, "v1.3.0")
	l.statsPath, l.now = filepath.Join(dir, "none"), func() time.Time { return now }
	pad := strings.Repeat("x", 1000)
	for i := 0; i < 1100; i++ {
		l.add("filler", []any{"pad", pad})
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() >= usageMax {
		t.Fatalf("the live file: %v %v", fi, err)
	}
	old, err := os.Stat(path + ".1")
	if err != nil || old.Size() > usageMax || old.Size() < usageMax/2 {
		t.Fatalf("the rotated file: %v %v", old, err)
	}
	for i := 0; i < 2200; i++ { // a second rotation replaces the first
		l.add("filler", []any{"pad", pad})
	}
	if _, err := os.Stat(path + ".2"); err == nil {
		t.Fatal("more than one old file is kept")
	}
	// a restarted Ember counts what is already there
	l2 := NewUsageLog(path, "v1.3.0")
	if l2.size != func() int64 { f, _ := os.Stat(path); return f.Size() }() {
		t.Fatal("a new log doesn't start from the file's size")
	}
}
