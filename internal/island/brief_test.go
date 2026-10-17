package island

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/config"
	"github.com/Tejas242/ember/internal/ipc"
)

// hookAt feeds the app a Claude Code hook event, as the socket would.
func hookAt(a *app, event, session, project string, payload map[string]any) {
	payload["session_id"] = session
	payload["cwd"] = "/home/x/" + project
	b, _ := json.Marshal(payload)
	a.hook(ipc.Msg{Type: "hook", Event: event, Payload: b}, nil)
}

func newTestApp(clock *time.Time) *app {
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return *clock }
	u.mind = newMind(*clock)
	u.day = &Day{}
	u.day.roll(*clock)
	return &app{ui: u, cfg: config.Default()}
}

// A working session's card: a sentence in words, the facts, a live timeline
// in which a failed step says so.
func TestBriefWhileWorking(t *testing.T) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "ember", map[string]any{})
	clock = clock.Add(time.Minute)
	hookAt(a, "PreToolUse", "s", "ember", map[string]any{"tool_name": "Edit", "tool_input": map[string]any{
		"file_path": "/x/quota.go", "old_string": "a", "new_string": "a\nb\nc"}})
	hookAt(a, "PostToolUse", "s", "ember", map[string]any{"tool_name": "Edit"})
	clock = clock.Add(time.Minute)
	hookAt(a, "PreToolUse", "s", "ember", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{
		"command": "go test ./...", "description": "Run the tests"}})
	hookAt(a, "PostToolUseFailure", "s", "ember", map[string]any{"tool_name": "Bash"})
	clock = clock.Add(30 * time.Second)
	hookAt(a, "PreToolUse", "s", "ember", map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/store.go"}})

	b := a.ui.brief()
	if b.mode != "work" || b.head != "ember is reading store.go" { // no informative prompt: the step speaks
		t.Fatalf("mode %s, head %q", b.mode, b.head)
	}
	for _, want := range []string{"2m in", "1 file +3 −1", "1 retry"} {
		if !strings.Contains(b.sub, want) {
			t.Fatalf("facts %q lack %q", b.sub, want)
		}
	}
	st := a.ui.sessions["s"].Turn.Steps
	if len(st) != 3 || !st[1].Failed || st[1].Text != "Run the tests" || st[0].Text != "Edited quota.go" || st[2].Done {
		t.Fatalf("steps %+v", st)
	}
}

func TestGerundHeadline(t *testing.T) {
	for in, want := range map[string]string{
		"Run the store tests": "running the store tests",
		"Editing quota.go":    "editing quota.go",
		"Install deps":        "installing deps",
		"Git status":          "",
	} {
		got, ok := gerund(in)
		if want == "" && ok || want != "" && got != want {
			t.Errorf("gerund(%q) = %q, %v", in, got, ok)
		}
	}
}

// After it finishes: a recap in Claude's words, with what changed, and the
// day remembers it.
func TestBriefRecapThenDay(t *testing.T) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "kinvault", map[string]any{})
	hookAt(a, "PreToolUse", "s", "kinvault", map[string]any{"tool_name": "Write", "tool_input": map[string]any{
		"file_path": "/x/store.go", "content": "a\nb"}})
	clock = clock.Add(9 * time.Minute)
	hookAt(a, "Stop", "s", "kinvault", map[string]any{"last_assistant_message": "Let me summarise.\nFixed the race in store.go; all 42 tests pass."})
	b := a.ui.brief()
	if b.mode != "recap" || b.head != "kinvault is done" || b.sub != "Finished just now · took 9m · 1 file +2" {
		t.Fatalf("%+v", b)
	}
	r := a.ui.sessions["s"].Recap
	if !strings.Contains(r.Summary, "42 tests pass") || recapFacts(r) != "1 step · 1 file changed +2" {
		t.Fatalf("recap %q / %q", r.Summary, recapFacts(r))
	}
	clock = clock.Add(time.Hour)
	b = a.ui.brief()
	if b.mode != "day" || b.head != "Good afternoon" || !strings.HasPrefix(b.sub, "1 task done · 9m with Claude") {
		t.Fatalf("quiet: %+v", b)
	}
	if a.ui.day.Hours[15] != 1 {
		t.Fatalf("the 3 pm hour saw one call: %v", a.ui.day.Hours)
	}
}

// What you asked is the headline, when it says something.
func TestBriefHeadlineIsTheTask(t *testing.T) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "kinvault", map[string]any{"prompt": "fix the race in compaction. then run the tests"})
	hookAt(a, "PreToolUse", "s", "kinvault", map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/a.go"}})
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
		hookAt(a, "UserPromptSubmit", s, "p"+s, map[string]any{})
		for i := 0; i < 6; i++ {
			clock = clock.Add(time.Second)
			hookAt(a, "PreToolUse", s, "p"+s, map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/f.go"}})
		}
	}
	a.ui.track = &Track{Player: "p", Title: "T", Playing: true, PosAt: clock}
	b := a.ui.brief()
	rows, hidden := a.ui.board(b)
	if b.s.ID != "c" || len(rows)+hidden != 3 || !rows[0].open || (len(rows) > 1 && rows[1].open) {
		t.Fatalf("subject %s, %d rows (+%d)", b.s.ID, len(rows), hidden)
	}
	if h := a.ui.expandedHeight(); h > a.ui.H-14 {
		t.Fatalf("three sessions and music: %d px", h)
	}
}
