package island

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/config"
)

// fakeAgents gives a session n running subagents.
func fakeAgents(s *Session, n int, now time.Time) {
	for i := 0; i < n; i++ {
		s.agent(fmt.Sprintf("ag%d", i), "Explore", now)
	}
}

func agentHookAt(a *app, event, agent string, payload map[string]any) {
	payload["agent_id"] = agent
	payload["agent_type"] = "Explore"
	hookAt(a, event, "s", "ember", payload)
}

// A subagent's tool calls are its own: they keep it alive, and never touch
// the session's state (a question the session asks stays asked).
func TestSubagentCallsStayTheirs(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "ember", map[string]any{"prompt": "map the hooks"})
	agentHookAt(a, "SubagentStart", "x1", map[string]any{})
	hookAt(a, "PermissionRequest", "s", "ember", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test ./..."}})
	clock = clock.Add(10 * time.Second)
	agentHookAt(a, "PostToolUse", "x1", map[string]any{"tool_name": "Grep"})
	s := a.ui.sessions["s"]
	if s.State != "wait" || s.Asking != "Wants to run go test ./..." {
		t.Fatalf("state %q asking %q: a subagent's call cleared the session's question", s.State, s.Asking)
	}
	if ag := s.Agents["x1"]; ag == nil || !ag.Seen.Equal(clock) {
		t.Fatalf("agent = %+v", ag)
	}
	hookAt(a, "PostToolUse", "s", "ember", map[string]any{"tool_name": "Bash"})
	if s.State != "work" || s.Asking != "" {
		t.Fatalf("answered in the terminal, still %q %q", s.State, s.Asking)
	}
	b := a.ui.brief()
	if rows, _ := a.ui.board(b); b.mode != "work" || len(rows) != 1 || !strings.Contains(b.sub, "1 agent") {
		t.Fatalf("brief %s %q, rows %+v", b.mode, b.sub, rows)
	}
	if l := a.ui.mainLine(); len(l.beads) != 1 || l.beads[0] != beadWork {
		t.Fatalf("pill %q: beads %v", l.text, l.beads)
	}
}

// Background agents outlive the turn: the session stays busy, the pill says
// so, and "Done" waits for them.
func TestAgentsOutliveTheTurn(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "ember", map[string]any{})
	agentHookAt(a, "SubagentStart", "x1", map[string]any{})
	agentHookAt(a, "SubagentStart", "x2", map[string]any{})
	hookAt(a, "Stop", "s", "ember", map[string]any{"last_assistant_message": "Two agents are on it."})
	s := a.ui.sessions["s"]
	if s.status() != "work" || a.ui.moment != nil {
		t.Fatalf("status %s, moment %+v", s.status(), a.ui.moment)
	}
	if l := a.ui.mainLine(); len(l.beads) != 1 || l.agentN != 2 {
		t.Fatalf("pill %q: %d beads, %d agents", l.text, len(l.beads), l.agentN)
	}
	b := a.ui.brief()
	if b.mode != "work" || !strings.Contains(b.sub, "2 agents") {
		t.Fatalf("brief %s %q", b.mode, b.sub)
	}
	agentHookAt(a, "SubagentStop", "x1", map[string]any{})
	agentHookAt(a, "SubagentStop", "x2", map[string]any{})
	if s.status() != "idle" {
		t.Fatalf("status %s after both stopped", s.status())
	}
}

// Async hooks race: a stop that beats its start, or a tool call after the
// stop, must not leave a ghost agent behind. A killed agent goes quiet and
// is swept.
func TestAgentRaces(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "ember", map[string]any{})
	agentHookAt(a, "SubagentStop", "x1", map[string]any{})
	agentHookAt(a, "SubagentStart", "x1", map[string]any{})
	agentHookAt(a, "PostToolUse", "x1", map[string]any{"tool_name": "Read"})
	s := a.ui.sessions["s"]
	if s.running() != 0 {
		t.Fatalf("%d ghost agents", s.running())
	}
	agentHookAt(a, "SubagentStart", "x2", map[string]any{})
	clock = clock.Add(config.DefaultAgentQuiet + time.Minute)
	s.sweepAgents(clock, config.DefaultAgentQuiet)
	if s.running() != 0 {
		t.Fatal("a silent agent was never swept")
	}
}

// Many agents are a count, not rows: the card stays small and inside the surface.
func TestManyAgentsFit(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "ember", map[string]any{"prompt": "audit everything"})
	fakeAgents(a.ui.sessions["s"], 5, clock)
	rows, _ := a.ui.board(a.ui.brief())
	if len(rows) != 1 || a.ui.sessions["s"].running() != 5 {
		t.Fatalf("rows %+v", rows)
	}
	if h := a.ui.expandedHeight(); h > a.ui.H-14 {
		t.Fatalf("card %d px", h)
	}
}
