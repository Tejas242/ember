package island

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// hookAt feeds the app a Claude Code hook event, as the socket would.
func hookAt(a *app, event, session, project string, payload map[string]any) {
	payload["session_id"] = session
	payload["cwd"] = "/home/x/" + project
	b, _ := json.Marshal(payload)
	a.hook(ipc.Msg{Type: "hook", Event: event, Payload: b})
}

func newTestApp(clock *time.Time) *app {
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return *clock }
	u.mind = newMind(*clock)
	return &app{ui: u}
}

// A working session's card: who works, since when, and nothing else.
func TestBriefWhileWorking(t *testing.T) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "ember", map[string]any{})
	clock = clock.Add(2 * time.Minute)
	hookAt(a, "PostToolUse", "s", "ember", map[string]any{"tool_name": "Edit"})
	b := a.ui.brief()
	if b.mode != "work" || b.head != "ember is working" { // no informative prompt
		t.Fatalf("mode %s, head %q", b.mode, b.head)
	}
	if b.sub != "ember · 2m in" {
		t.Fatalf("facts %q", b.sub)
	}
}

// After it finishes: one line saying so; an hour later the card is the quiet one.
func TestBriefDoneThenQuiet(t *testing.T) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "kinvault", map[string]any{})
	clock = clock.Add(9 * time.Minute)
	hookAt(a, "Stop", "s", "kinvault", map[string]any{"last_assistant_message": "Fixed the race in store.go; all 42 tests pass."})
	b := a.ui.brief()
	if b.mode != "done" || b.head != "kinvault is done" || b.sub != "kinvault · finished just now" {
		t.Fatalf("%+v", b)
	}
	clock = clock.Add(time.Hour)
	b = a.ui.brief()
	if b.mode != "day" || b.head != "Good afternoon" || !strings.Contains(b.sub, "No Claude") {
		t.Fatalf("quiet: %+v", b)
	}
}

// What you asked is the headline, when it says something.
func TestBriefHeadlineIsTheTask(t *testing.T) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "kinvault", map[string]any{"prompt": "fix the race in compaction. then run the tests"})
	if b := a.ui.brief(); b.head != "Fix the race in compaction" || !strings.HasPrefix(b.sub, "kinvault · ") {
		t.Fatalf("%q / %q", b.head, b.sub)
	}
	hookAt(a, "UserPromptSubmit", "s", "kinvault", map[string]any{"prompt": "yes do it"})
	a.ui.sessions["s"].Title = "Compaction race"
	if b := a.ui.brief(); b.head != "Compaction race" {
		t.Fatalf("a terse prompt yields to Claude's topic: %q", b.head)
	}
}

// The others get one line each; the card stays small even when busy.
func TestBriefOthersAndHeight(t *testing.T) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	for _, s := range []string{"a", "b", "c"} {
		clock = clock.Add(time.Second)
		hookAt(a, "UserPromptSubmit", s, "p"+s, map[string]any{})
	}
	a.ui.track = &Track{Player: "p", Title: "T", Playing: true, PosAt: clock}
	b := a.ui.brief()
	rows, hidden := a.ui.board(b)
	if b.s.ID != "c" || len(rows)+hidden != 3 || !rows[0].hero || (len(rows) > 1 && rows[1].hero) {
		t.Fatalf("subject %s, %d rows (+%d)", b.s.ID, len(rows), hidden)
	}
	if h := a.ui.expandedHeight(); h > a.ui.H-14 {
		t.Fatalf("three sessions and music: %d px", h)
	}
}

// A session that asks says what for, from the permission request or the
// question; Ember answers nothing, and the terminal answering clears it.
func TestAsksAreReportedNotAnswered(t *testing.T) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "jobwatch", map[string]any{})
	hookAt(a, "PermissionRequest", "s", "jobwatch", map[string]any{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/store.go"}})
	if s := a.ui.sessions["s"]; s.State != "wait" || s.Asking != "Wants to edit store.go" || len(a.ui.cards) != 0 {
		t.Fatalf("%q %q, %d cards", s.State, s.Asking, len(a.ui.cards))
	}
	hookAt(a, "PostToolUse", "s", "jobwatch", map[string]any{"tool_name": "Edit"})
	hookAt(a, "AskUserQuestion", "s", "jobwatch", map[string]any{"tool_name": "AskUserQuestion", "tool_input": map[string]any{
		"questions": []any{map[string]any{"question": "Which store?", "options": []any{}}}}})
	if s := a.ui.sessions["s"]; s.State != "wait" || s.Asking != "Asks: Which store?" {
		t.Fatalf("%q %q", s.State, s.Asking)
	}
	b := a.ui.brief()
	if b.mode != "wait" || !strings.HasPrefix(b.sub, "Asks: Which store? · waiting") {
		t.Fatalf("%+v", b)
	}
	hookAt(a, "UserPromptSubmit", "s", "jobwatch", map[string]any{})
	if s := a.ui.sessions["s"]; s.State != "work" || s.Asking != "" {
		t.Fatalf("%q %q", s.State, s.Asking)
	}
}
