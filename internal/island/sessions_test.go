package island

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// Several Claude Code sessions at once: each keeps its own story, requests
// go back to the session that asked, and nothing flickers between them.

// hookPids is hookAt from a given Claude process (its parent chain).
func hookPids(a *app, event, session, project string, pids []int, payload map[string]any) {
	payload["session_id"] = session
	payload["cwd"] = "/home/x/" + project
	b, _ := json.Marshal(payload)
	a.hook(ipc.Msg{Type: "hook", Event: event, Pids: pids, Payload: b}, nil)
}

// askAt is a hook that waits for an answer (a permission, a question).
func askAt(a *app, event, session, project string, payload map[string]any) (string, chan json.RawMessage) {
	payload["session_id"] = session
	payload["cwd"] = "/home/x/" + project
	b, _ := json.Marshal(payload)
	ch := make(chan json.RawMessage, 1)
	return a.hook(ipc.Msg{Type: "hook", Event: event, Wait: true, Payload: b}, ch), ch
}

func read(t *testing.T, name string, ch chan json.RawMessage) string {
	t.Helper()
	select {
	case b := <-ch:
		return string(b)
	default:
		t.Fatalf("%s got no answer", name)
	}
	return ""
}

func quiet(t *testing.T, name string, ch chan json.RawMessage) {
	t.Helper()
	select {
	case b := <-ch:
		t.Fatalf("%s was answered: %s", name, b)
	default:
	}
}

func step(a *app, session, project, file string) {
	hookAt(a, "PreToolUse", session, project, map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/" + file}})
}

// Two sessions working at once, both busy: the pill and the card keep to
// one of them (the newest task) instead of swapping on every tool call.
func TestTwoSessionsHoldStill(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "a", "edge", map[string]any{"prompt": "fix the streak counter across midnight"})
	clock = clock.Add(time.Minute)
	hookAt(a, "UserPromptSubmit", "b", "ember", map[string]any{"prompt": "make the pill hold still with two sessions"})
	for i := 0; i < 6; i++ {
		clock = clock.Add(2 * time.Second)
		s, p := "a", "edge"
		if i%2 == 1 {
			s, p = "b", "ember"
		}
		step(a, s, p, "f.go")
		l, b := a.ui.mainLine(), a.ui.brief()
		if l.text != "ember" || l.badge != "+1" || !l.work {
			t.Fatalf("after a step in %s: pill %q %q", s, l.text, l.badge)
		}
		if b.mode != "work" || b.s.ID != "b" || len(b.rows) != 2 || b.rows[1].ID != "a" {
			t.Fatalf("after a step in %s: card about %s, %d rows", s, b.s.ID, len(b.rows))
		}
	}
	// a new prompt in the other one is news: it takes the headline
	hookAt(a, "UserPromptSubmit", "a", "edge", map[string]any{"prompt": "now check the reset at midnight"})
	if b := a.ui.brief(); b.rows[0].ID != "a" {
		t.Fatalf("card leads with %q", b.rows[0].ID)
	}
}

// You're looking at one session's terminal: the pill tells you about the
// other one, which you can't see.
func TestPillShowsTheSessionYouCantSee(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "a", "edge", map[string]any{})
	step(a, "a", "edge", "a.go")
	clock = clock.Add(time.Minute)
	hookAt(a, "UserPromptSubmit", "b", "ember", map[string]any{})
	step(a, "b", "ember", "b.go")
	a.ui.watching["b"] = true
	clock = clock.Add(5 * time.Second)
	if p := a.ui.primary(); p != "claude" {
		t.Fatalf("a session works where you can't see it: the pill shows %s", p)
	}
	if b := a.ui.brief(); b.rows[0].ID != "a" {
		t.Fatalf("card leads with %q", b.rows[0].ID)
	}
}

// Two sessions in the same folder are told apart wherever both appear.
func TestTwinSessionsHaveNames(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "a", "ember", map[string]any{})
	hookAt(a, "UserPromptSubmit", "b", "ember", map[string]any{})
	hookAt(a, "UserPromptSubmit", "c", "edge", map[string]any{})
	u := a.ui
	if n := u.name(u.sessions["c"]); n != "edge" {
		t.Fatalf("a session alone in its folder is its project: %q", n)
	}
	if na, nb := u.name(u.sessions["a"]), u.name(u.sessions["b"]); na != "ember 1" || nb != "ember 2" {
		t.Fatalf("twins without a topic: %q, %q", na, nb)
	}
	u.sessions["a"].Title = "Fix the flaky test"
	if na := u.name(u.sessions["a"]); na != "ember (Fix the flaky test)" {
		t.Fatalf("a twin with a topic: %q", na)
	}
	// the requests say which one is asking
	id, _ := askAt(a, "PermissionRequest", "b", "ember", map[string]any{"tool_name": "Bash",
		"tool_input": map[string]any{"command": "rm -rf build", "description": "Clean the build"}})
	if c := u.cards[0]; c.ID != id || c.Eyebrow != "ember 2 wants to run" {
		t.Fatalf("card %q eyebrow %q", c.ID, c.Eyebrow)
	}
	if l := u.mainLine(); l.text != "ember 2 needs you" {
		t.Fatalf("pill %q", l.text)
	}
	delete(u.sessions, "b") // alone again: just the project
	if na := u.name(u.sessions["a"]); na != "ember" {
		t.Fatalf("%q", na)
	}
}

// Requests from two sessions at once: both stay on the island, each answer
// goes to the session that asked, and one cancelled in its terminal leaves
// the other alone.
func TestRequestsFromTwoSessions(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	u := a.ui
	hookAt(a, "UserPromptSubmit", "a", "edge", map[string]any{})
	hookAt(a, "UserPromptSubmit", "b", "ember", map[string]any{})
	hookAt(a, "UserPromptSubmit", "c", "kinvault", map[string]any{})
	pa, cha := askAt(a, "PermissionRequest", "a", "edge", map[string]any{"tool_name": "Bash",
		"tool_input": map[string]any{"command": "make", "description": "Build it"}})
	pb, chb := askAt(a, "PermissionRequest", "b", "ember", map[string]any{"tool_name": "Bash",
		"tool_input": map[string]any{"command": "go test ./...", "description": "Run the tests"}})
	qc, chc := askAt(a, "AskUserQuestion", "c", "kinvault", map[string]any{"tool_name": "AskUserQuestion",
		"tool_input": map[string]any{"questions": []any{map[string]any{"question": "Which store?",
			"options": []any{map[string]any{"label": "SQLite"}, map[string]any{"label": "Pebble"}}}}}})
	if len(u.cards) != 3 || pa == pb || !strings.Contains(qc, "c") {
		t.Fatalf("%d cards: %q %q %q", len(u.cards), pa, pb, qc)
	}
	// b gives up in its terminal (its hook exits): only b's card goes
	u.RemoveCard(pb)
	if len(u.cards) != 2 || u.top().ID != pa {
		t.Fatalf("cards after b's cancel: %d, top %q", len(u.cards), u.top().ID)
	}
	quiet(t, "b", chb)
	// allow goes to the top card: a's
	a.command("allow", "")
	if out := read(t, "a", cha); !strings.Contains(out, `"allow"`) {
		t.Fatalf("a got %s", out)
	}
	quiet(t, "c", chc)
	if u.sessions["a"].State != "work" || u.sessions["c"].State != "wait" {
		t.Fatalf("states a %s, c %s", u.sessions["a"].State, u.sessions["c"].State)
	}
	// then c's question, answered with its second option
	a.command("answer", "opt:2")
	if out := read(t, "c", chc); !strings.Contains(out, "Pebble") {
		t.Fatalf("c got %s", out)
	}
	if len(u.cards) != 0 {
		t.Fatalf("%d cards left", len(u.cards))
	}
}

// /clear starts a new session in the same Claude process: the old one is
// gone, not an idle twin for the next three hours. Other Claude processes,
// even in the same terminal, keep theirs.
func TestClearReplacesTheSession(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	other := exec.Command("sleep", "30") // another Claude, a sibling in the same terminal
	if err := other.Start(); err != nil {
		t.Skip(err)
	}
	defer other.Process.Kill()
	const gone = 1 << 30 // the shell Claude ran the hook in: exited
	claude, term := os.Getpid(), os.Getppid()
	hookPids(a, "UserPromptSubmit", "old", "ember", []int{gone, claude, term}, map[string]any{})
	hookPids(a, "Stop", "old", "ember", []int{gone - 1, claude, term}, map[string]any{"last_assistant_message": "Done."})
	hookPids(a, "UserPromptSubmit", "sib", "ember", []int{gone - 2, other.Process.Pid, term}, map[string]any{})
	hookPids(a, "SessionStart", "new", "ember", []int{gone - 3, claude, term}, map[string]any{"source": "clear"})
	if _, ok := a.ui.sessions["old"]; ok {
		t.Fatal("the cleared session lingers")
	}
	if _, ok := a.ui.sessions["sib"]; !ok {
		t.Fatal("a sibling Claude in the same terminal was dropped")
	}
	// a straggling async hook of the old session doesn't bring it back
	hookPids(a, "PostToolUse", "old", "ember", []int{gone - 4, claude, term}, map[string]any{})
	if _, ok := a.ui.sessions["old"]; ok {
		t.Fatal("a late hook resurrected the cleared session")
	}
	// Claude ran the hook itself (no shell in between): same chain, same process
	hookPids(a, "SessionStart", "newer", "ember", []int{claude, term}, map[string]any{"source": "clear"})
	if _, ok := a.ui.sessions["new"]; ok || len(a.ui.sessions) != 2 {
		t.Fatalf("sessions %v", keys(a.ui.sessions))
	}
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A session that ended stays ended against late async hooks, until it's
// resumed.
func TestEndedSessionStaysEnded(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "a", "edge", map[string]any{})
	hookAt(a, "UserPromptSubmit", "b", "ember", map[string]any{})
	a.ui.watching["a"] = true
	hookAt(a, "SessionEnd", "a", "edge", map[string]any{})
	hookAt(a, "PostToolUse", "a", "edge", map[string]any{})
	if _, ok := a.ui.sessions["a"]; ok || a.ui.watching["a"] {
		t.Fatal("a late hook brought an ended session back")
	}
	if _, ok := a.ui.sessions["b"]; !ok {
		t.Fatal("the other session went too")
	}
	hookAt(a, "SessionStart", "a", "edge", map[string]any{"source": "resume"})
	if _, ok := a.ui.sessions["a"]; !ok {
		t.Fatal("a resumed session is back")
	}
}

// Two sessions finish together: one note names both, once each.
func TestDoneMergesAcrossSessions(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	a.desk.live = true // nothing focused
	for _, s := range [][2]string{{"a", "edge"}, {"b", "ember"}} {
		hookAt(a, "UserPromptSubmit", s[0], s[1], map[string]any{})
	}
	hookAt(a, "Stop", "a", "edge", map[string]any{"last_assistant_message": "Fixed."})
	clock = clock.Add(2 * time.Second)
	hookAt(a, "Stop", "b", "ember", map[string]any{"last_assistant_message": "Fixed too."})
	m := a.ui.moment
	if m == nil || m.verb+" "+m.text != "Done edge, ember" || m.sub != "2 sessions finished" || m.focus != "b" {
		t.Fatalf("%+v", m)
	}
	hookAt(a, "UserPromptSubmit", "a", "edge", map[string]any{})
	clock = clock.Add(time.Second)
	hookAt(a, "Stop", "a", "edge", map[string]any{"last_assistant_message": "Again."})
	if m := a.ui.moment; m.verb+" "+m.text != "Done edge, ember" || m.sub != "2 sessions finished" {
		t.Fatalf("a session finishing twice is named once: %q / %q", m.text, m.sub)
	}
}

// Away while two sessions moved on: one line says what each did, and the
// click goes to the one that's asking.
func TestWelcomeBackWithTwoSessions(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "a", "ember", map[string]any{})
	hookAt(a, "UserPromptSubmit", "b", "ember", map[string]any{})
	hookAt(a, "UserPromptSubmit", "c", "edge", map[string]any{})
	a.ui.mind.away = true
	hookAt(a, "Stop", "a", "ember", map[string]any{})
	hookAt(a, "Stop", "b", "ember", map[string]any{})
	askAt(a, "PermissionRequest", "c", "edge", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}})
	a.welcomeBack(clock)
	m := a.ui.moment
	if m == nil || m.sub != "ember 1 done · ember 2 done · edge asked you" || m.focus != "c" || m.mood != moodWait {
		t.Fatalf("%+v", m)
	}
}

// Subagents in two sessions at once stay in their own session.
func TestSubagentsOfTwoSessions(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "a", "edge", map[string]any{})
	hookAt(a, "UserPromptSubmit", "b", "ember", map[string]any{})
	for _, s := range [][3]string{{"a", "edge", "x"}, {"b", "ember", "y"}, {"b", "ember", "z"}} {
		hookAt(a, "SubagentStart", s[0], s[1], map[string]any{"agent_id": s[2], "agent_type": "Explore"})
		hookAt(a, "PreToolUse", s[0], s[1], map[string]any{"agent_id": s[2], "agent_type": "Explore",
			"tool_name": "Grep", "tool_input": map[string]any{"pattern": s[2]}})
	}
	sa, sb := a.ui.sessions["a"], a.ui.sessions["b"]
	if sa.running() != 1 || sb.running() != 2 || sa.Agents["x"].Label != "Searching x" || sb.Agents["x"] != nil {
		t.Fatalf("a %d agents, b %d", sa.running(), sb.running())
	}
	hookAt(a, "SubagentStop", "a", "edge", map[string]any{"agent_id": "y"}) // a's hook can't stop b's agent
	if sb.running() != 2 {
		t.Fatal("an agent stopped in the wrong session")
	}
}

// The terminal of a session is the window of the nearest process in its
// chain: a terminal launched from another terminal's shell has that one
// further up, and the map's order mustn't decide.
func TestSessionWindowIsTheNearest(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	a.desk = desktop{live: true, windows: map[int]deskWindow{
		1: {ID: 1, Pid: 100, Title: "✳ Outer work"},
		2: {ID: 2, Pid: 200, Title: "✳ Inner work"},
	}, focused: 1}
	a.ui.sessions["in"] = &Session{ID: "in", Project: "ember", Pids: []int{210, 205, 200, 105, 100}}
	a.ui.sessions["out"] = &Session{ID: "out", Project: "edge", Pids: []int{110, 105, 100}}
	for i := 0; i < 20; i++ {
		if w, ok := a.sessionWindow(a.ui.sessions["in"].Pids); !ok || w.ID != 2 {
			t.Fatalf("the inner session's terminal is window %d", w.ID)
		}
	}
	a.deskChanged()
	if a.ui.watching["in"] || !a.ui.watching["out"] {
		t.Fatalf("watching %v", a.ui.watching)
	}
	if a.ui.sessions["in"].Title != "Inner work" || a.ui.sessions["out"].Title != "Outer work" {
		t.Fatalf("titles %q, %q", a.ui.sessions["in"].Title, a.ui.sessions["out"].Title)
	}
}

// When the card runs out of room it folds the last sessions into a link,
// never out of reach: "Show all" lists every session on one line, music and
// edge stepping aside, until the card closes.
func TestShowAllSessions(t *testing.T) {
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	for _, s := range []string{"a", "b", "c", "d"} {
		clock = clock.Add(time.Second)
		hookAt(a, "UserPromptSubmit", s, "p"+s, map[string]any{"prompt": "fix the thing in " + s})
		for i := 0; i < 4; i++ {
			step(a, s, "p"+s, s+".go")
		}
	}
	a.ui.track = &Track{Player: "p", Title: "T", Playing: true, PosAt: clock}
	a.ui.edge = &EdgeStatus{Today: 1, Need: 1}
	if _, hidden := a.ui.board(a.ui.brief()); hidden == 0 {
		t.Fatal("expected the card to fold some sessions away at this size")
	}
	found := false
	a.ui.pinned = true
	settle(a.ui, &clock)
	for _, h := range a.ui.hits {
		found = found || h.key == "rows:all"
	}
	if !found {
		t.Fatal("the folded sessions have no link")
	}
	a.ui.press("rows:all", 1)
	rows, hidden := a.ui.board(a.ui.brief())
	if hidden != 0 || len(rows) != 4 {
		t.Fatalf("show all: %d rows, %d still hidden", len(rows), hidden)
	}
	for _, r := range rows {
		if r.open {
			t.Fatalf("%s is open in the full list", r.s.ID)
		}
	}
	if h := a.ui.expandedHeight(); h > a.ui.H-14 {
		t.Fatalf("card %d px", h)
	}
	a.ui.pinned = false
	settle(a.ui, &clock)
	if a.ui.allRows {
		t.Fatal("show all should end when the card closes")
	}
}

// A sessions file from before the activity bins went still loads.
func TestOldSessionsFileLoads(t *testing.T) {
	var ss map[string]*Session
	old := `{"a":{"ID":"a","Project":"kinvault","State":"work","Bins":[1,2,3],"Counts":[4,5,6]}}`
	if err := json.Unmarshal([]byte(old), &ss); err != nil || ss["a"] == nil || ss["a"].Project != "kinvault" {
		t.Fatalf("old file: %v %+v", err, ss)
	}
}
