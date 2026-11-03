package island

import (
	"strings"
	"testing"
	"time"
)

// A turn that stops on an error is told as one: never the last turn's
// success, and the outcome waits by the clock until you've seen it.
func TestFailedTurnIsNotASuccess(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "kinvault", map[string]any{"prompt": "fix the race in the compaction path"})
	hookAt(a, "Stop", "s", "kinvault", map[string]any{"last_assistant_message": "All tests pass."})
	hookAt(a, "UserPromptSubmit", "s", "kinvault", map[string]any{"prompt": "now make the index faster please"})
	clock = clock.Add(time.Minute)
	hookAt(a, "StopFailure", "s", "kinvault", map[string]any{"error": "rate_limit"})
	b := a.ui.brief()
	if b.mode != "done" || b.mood != moodError || !strings.Contains(b.sub, "stopped") {
		t.Fatalf("%+v", b)
	}
	if o := a.ui.sessions["s"].Outcome; !o.Failed || o.Error != "Rate limited" {
		t.Fatalf("outcome %+v", o)
	}
	a.ui.moment = nil
	if l := a.ui.mainLine(); l.key != "clock:kinvault stopped" {
		t.Fatalf("pill %q / %q", l.text, l.sub)
	}
	a.ui.watching["s"] = true // you look at its terminal: it's seen
	settle(a.ui, &clock)
	if l := a.ui.mainLine(); l.sub != "" {
		t.Fatalf("still says %q after you looked", l.sub)
	}
}

// Text in the pill keeps a twin's topic short, so the verb survives.
func TestShortTwinNames(t *testing.T) {
	u := NewUI(440, 280, 22)
	u.sessions["a"] = &Session{ID: "a", Project: "ember", Title: "Redesign the card around every session"}
	u.sessions["b"] = &Session{ID: "b", Project: "ember"}
	if n := u.short(u.sessions["a"]); n != "ember (Redesign the ca…)" {
		t.Fatalf("%q", n)
	}
}
