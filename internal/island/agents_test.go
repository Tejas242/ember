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
		ag := s.agent(fmt.Sprintf("ag%d", i), "Explore", now)
		ag.Desc, ag.Label = fmt.Sprintf("Map part %d", i+1), "Searching hook"
	}
}

func agentHookAt(a *app, event, agent string, payload map[string]any) {
	payload["agent_id"] = agent
	payload["agent_type"] = "Explore"
	hookAt(a, event, "s", "ember", payload)
}

// A subagent's tool calls are its own: the session's label and timeline
// stay on the Agent call that launched it.
func TestSubagentStepsStayTheirs(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "ember", map[string]any{"prompt": "map the hooks"})
	hookAt(a, "PreToolUse", "s", "ember", map[string]any{"tool_name": "Agent",
		"tool_input": map[string]any{"description": "Find hook handlers", "subagent_type": "Explore"}})
	agentHookAt(a, "SubagentStart", "x1", map[string]any{})
	clock = clock.Add(10 * time.Second)
	agentHookAt(a, "PreToolUse", "x1", map[string]any{"tool_name": "Grep", "tool_input": map[string]any{"pattern": "SubagentStart"}})
	s := a.ui.sessions["s"]
	if s.Label != "Delegating: Find hook handlers" || s.Turn.Count != 1 {
		t.Fatalf("session label %q, %d steps: a subagent's call leaked into the session", s.Label, s.Turn.Count)
	}
	ag := s.Agents["x1"]
	if ag == nil || ag.Desc != "Find hook handlers" || ag.Label != "Searching SubagentStart" {
		t.Fatalf("agent = %+v", ag)
	}
	b := a.ui.brief()
	if rows, _ := a.ui.board(b); b.mode != "work" || len(rows) != 1 || len(rows[0].agents) != 1 || !rows[0].open {
		t.Fatalf("brief %s, rows %+v", b.mode, rows)
	}
	if l := a.ui.mainLine(); !l.work {
		t.Fatalf("pill %q", l.text)
	}
}

// Background agents outlive the turn: the session stays busy, the pill says
// what they're doing, and "Done" waits for them.
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
	if l := a.ui.mainLine(); !l.work {
		t.Fatalf("pill %q", l.text)
	}
	b := a.ui.brief()
	if rows, _ := a.ui.board(b); b.mode != "work" || len(rows[0].agents) != 2 || !strings.Contains(b.head, "2 agents working") {
		t.Fatalf("brief %s %q", b.mode, b.head)
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

// Many agents, each opened: the card trims their steps and stays inside the
// surface, every agent still on the branch.
func TestManyAgentsFit(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "ember", map[string]any{"prompt": "audit everything"})
	for i := 0; i < 6; i++ {
		hookAt(a, "PreToolUse", "s", "ember", map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/a.go"}})
	}
	fakeAgents(a.ui.sessions["s"], 5, clock)
	for _, ag := range a.ui.sessions["s"].Agents {
		ag.Hist = []Step{{Kind: "read", Text: "Read a.go"}, {Kind: "read", Text: "Read b.go"}, {Kind: "search", Text: "Searched x"}}
		a.ui.opened["a:s:"+ag.ID] = true
	}
	rows, _ := a.ui.board(a.ui.brief())
	if len(rows) != 1 || !rows[0].open || len(rows[0].agents) != 5 {
		t.Fatalf("rows %+v", rows)
	}
	if h := a.ui.expandedHeight(); h > a.ui.H-14 {
		t.Fatalf("card %d px", h)
	}
}
