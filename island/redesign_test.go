package island

import (
	"encoding/json"
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
	if b.mode != "recap" || b.mood != moodError || !strings.Contains(b.sub, "stopped") {
		t.Fatalf("%+v", b)
	}
	if r := a.ui.sessions["s"].Recap; !r.Failed || r.Error != "Rate limited" || r.Summary != "" {
		t.Fatalf("recap %+v", r)
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

// A long command wraps and keeps its end: that's where the danger hides.
func TestCommandKeepsItsTail(t *testing.T) {
	cmd := "cd ~/Gems/kinvault && go build ./... && go test -race -count=1 ./store/... ./index/... ./wal/... && ./scripts/deploy.sh --prod && rm -rf /tmp/kv-build-cache"
	lines := codeLines(cmd, 300)
	if len(lines) > 4 || !strings.Contains(strings.Join(strings.Fields(strings.Join(lines[len(lines)-2:], " ")), " "), "rm -rf /tmp/kv-build-cache") {
		t.Fatalf("%q", lines)
	}
	for _, ln := range lines[1:] {
		if !strings.HasPrefix(ln, "  ") {
			t.Fatalf("continuation %q isn't indented", ln)
		}
	}
}

// "Always" says what it would allow, before you press it.
func TestAlwaysNote(t *testing.T) {
	raw, _ := json.Marshal([]map[string]any{{"type": "addRules", "behavior": "allow", "destination": "localSettings",
		"rules": []map[string]any{{"toolName": "Bash", "ruleContent": "go test:*"}}}})
	if n := alwaysNote(raw); n != "Always allows go test:* in this project" {
		t.Fatalf("%q", n)
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
