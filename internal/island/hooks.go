package island

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// sweep forgets sessions whose Claude process is gone, and settles ones
// that went silent: a Claude that was killed never says Stop, so after
// [claude] settle_min (30) without a word "working" becomes idle (a long
// build still shows its minutes in the pill until then).
func (a *app) sweep() bool {
	changed, now := false, a.ui.now()
	for id, s := range a.ui.sessions {
		quiet := now.Sub(s.Seen)
		switch {
		case !slices.ContainsFunc(s.Pids, ipc.Alive) || (s.State == "idle" && quiet > forgetIdle):
			a.forget(id)
			changed = true
		case s.State != "idle" && quiet > a.ui.cfg.Settle && !a.hasCard(id):
			s.State, s.Label = "idle", ""
			changed = true
		}
		if s.sweepAgents(now, a.ui.cfg.AgentQuiet) {
			changed = true
		}
	}
	return changed
}

// forget drops a session and what the island knew about it.
func (a *app) forget(id string) {
	delete(a.ui.sessions, id)
	delete(a.ui.watching, id)
}

// end forgets a session that's over, and keeps it over: hooks are async, so
// one can still arrive after its SessionEnd.
func (a *app) end(id string, now time.Time) {
	a.forget(id)
	a.ended = remember(a.ended, id, now)
}

// sameProcess: two hooks' parent chains come from one Claude process. A
// chain starts at the hook's parent, which is Claude itself or a shell it ran
// the hook in; that shell has exited by now. So the older chain, less its
// dead head, is the newer one, give or take the newer one's shell. Sessions
// in the same terminal share only the chain's tail, and a Claude started
// from inside another adds more than a shell.
func sameProcess(old, cur []int) bool {
	i := 0
	for i < len(old) && !ipc.Alive(old[i]) {
		i++
	}
	old = old[i:]
	d := len(cur) - len(old)
	return len(old) > 0 && (d == 0 || d == 1) && slices.Equal(cur[d:], old)
}

// hasCard: a request from this session is still on the island.
func (a *app) hasCard(session string) bool {
	for _, c := range a.ui.cards {
		if strings.Contains(c.ID, ":"+session) {
			return true
		}
	}
	return false
}

// doneFor is how long "Done · project" stays up.
const doneFor = 6 * time.Second

// forgetIdle: an idle session silent this long is forgotten, its process or not.
const forgetIdle = 3 * time.Hour

// ---- Claude Code hooks ----

// isDemo: a session ember demo plays. It shows like any other, but isn't
// counted in the day, the habits or what Ember has learned.
func isDemo(s *Session) bool { return strings.HasPrefix(s.ID, ipc.DemoSession) }

func (a *app) hook(m ipc.Msg) {
	var in hookInput
	if err := json.Unmarshal(m.Payload, &in); err != nil && len(m.Payload) > 0 {
		logOnce("hook:"+m.Event, "a %s hook Ember can't read: %v (this is said once)", m.Event, err)
	}
	now := a.ui.now()
	s := a.ui.sessions[in.SessionID]
	if m.Event == "SessionStart" {
		delete(a.ended, in.SessionID) // resumed: it's back
	}
	_, over := a.ended[in.SessionID]
	if s == nil && in.SessionID != "" && m.Event != "SessionEnd" && !over {
		s = &Session{ID: in.SessionID, State: "idle", Since: now}
		a.ui.sessions[in.SessionID] = s
		if !isDemo(s) {
			a.ui.bond.meet()
		}
	}
	if s != nil {
		s.Seen = now
		if in.Cwd != "" {
			s.Project = project(in.Cwd)
		}
		if len(m.Pids) > 0 {
			s.Pids = m.Pids
		}
	}
	if s == nil {
		if m.Event == "SessionEnd" && in.SessionID != "" {
			a.end(in.SessionID, now)
		}
		return
	}
	var tin map[string]any
	if err := json.Unmarshal(in.ToolInput, &tin); err != nil && len(in.ToolInput) > 0 {
		logOnce("tool_input:"+in.Tool, "%s's tool_input Ember can't read: %v (this is said once)", in.Tool, err)
	}
	if in.AgentID != "" && s.agentHook(m.Event, in, now) {
		return
	}

	before := s.Asking
	defer func() { a.ui.logAsk(s, before, m.Event, now) }() // usage log

	switch m.Event {
	case "SessionEnd":
		a.end(in.SessionID, now)
	case "SessionStart":
		if in.Source == "startup" { // a compaction or a resume keeps its background agents
			s.Agents = nil
		}
		if in.Source == "clear" || in.Source == "resume" { // a new session in the same Claude: the old one is over
			for id, o := range a.ui.sessions {
				if o != s && sameProcess(o.Pids, m.Pids) {
					a.end(id, now)
				}
			}
		}
	case "UserPromptSubmit":
		s.State, s.Label, s.Since = "work", "Thinking", now
		s.Outcome, s.Unseen, s.Asking = nil, time.Time{}, ""
		s.Prompt = firstLine(strings.TrimSpace(in.Prompt))
		a.ui.mind.kick(now, 0.25, 0.05)
		a.ui.play("ignite", 0) // it catches
		a.ui.mind.novel["claude"] = now
	case "StopFailure":
		s.State, s.Label = "idle", ""
		msg := friendlyError(in.Error)
		s.Outcome = &Outcome{At: now, Failed: true, Error: msg}
		if !a.looking(s) {
			s.Unseen = now
		}
		a.ui.mind.kick(now, 0.35, -0.6) // a start, and a low mood that takes a while to lift
		if a.ui.mind.away {
			a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"fail", a.ui.name(s), s.ID})
			return
		}
		a.ui.Moment(moment{key: "fail:" + s.ID, act: "claude", mood: moodError, verb: "Stopped", text: a.ui.short(s),
			sub: msg, until: now.Add(8 * time.Second), focus: s.ID})
	case "AskUserQuestion":
		// reported, not answered: the terminal is already asking
		a.asks(s, "Asks: "+firstQuestion(tin), now)
	case "PermissionRequest":
		a.asks(s, wanting(in.Tool, tin), now)
	case "PostToolUse":
		// answered in the terminal, or just the next step: it isn't asking now
		if s.State == "wait" {
			s.State = "work"
		}
		s.Asking = ""
		a.ui.mind.toolCall(now)
		if ss := a.ui.sessionList(); len(ss) > 0 && ss[0] == s && now.Sub(a.ui.gulpAt) > 1200*time.Millisecond {
			a.ui.gulpAt = now
			a.ui.body.gulp(0.5) // a bite of fuel, for the session it's watching
		}
	case "Notification":
		// a prompt in the terminal (a dialog the hooks above don't cover).
		// idle_prompt is just "Claude is done": not a request.
		if in.NotifyType == "permission_prompt" || in.NotifyType == "elicitation_dialog" {
			if s.State != "wait" {
				a.ui.play("needs", 0)
			}
			s.State = "wait"
			if s.Asking == "" {
				s.Asking = in.Message
			}
		}
	case "Stop":
		s.State, s.Label = "idle", ""
		a.ui.lastDone.project, a.ui.lastDone.at = s.Project, now
		took := now.Sub(s.Since)
		if s.Since.IsZero() {
			took = 0
		}
		s.Outcome = &Outcome{At: now}
		s.cheer = cheer{size: doneSize(took), roll: absolutelyRight.MatchString(in.LastMessage)}
		if b := a.ui.bond; b != nil && !isDemo(s) {
			s.cheer.flare = b.turn(took)
		}
		s.Asking = ""
		if !a.looking(s) {
			s.Unseen = now // it stays in the pill, quietly, until you've seen it
		}
		a.ui.mind.kick(now, 0.2, 0.35) // pleased
		if a.ui.mind.away {            // tell them when they're back
			a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"done", a.ui.name(s), s.ID})
			return
		}
		if !a.ui.cfg.Done || a.looking(s) || s.running() > 0 {
			return // they're looking at it, or it isn't done: its agents work on
		}
		a.done(s, now)
	}
}

// done shows "Done · project". If another session's note is still up, the
// two merge into one ("Done · edge, ember") rather than one hiding the other.
func (a *app) done(s *Session, now time.Time) {
	name := a.ui.short(s)
	m := moment{key: "done:" + s.ID, act: "claude", mood: moodHappy, verb: "Done", text: name,
		until: now.Add(doneFor), focus: s.ID, many: []string{name}, cheer: s.cheer}
	s.cheer = cheer{}
	if cur := a.ui.moment; cur != nil && strings.HasPrefix(cur.key, "done") && now.Before(cur.until) {
		names := cur.many
		if !slices.Contains(names, name) { // one that finishes again is named once
			names = append(slices.Clip(names), name)
		}
		if len(names) > 1 {
			m.key, m.many = "done:many", names
			m.text = strings.Join(names, ", ")
			m.sub = fmt.Sprintf("%d sessions finished", len(names))
			m.until = now.Add(doneFor + time.Second)
		}
	}
	a.ui.Moment(m)
}

// welcomeBack: you were away; here's what happened, in one line.
func (a *app) welcomeBack(now time.Time) {
	ms := a.ui.mind.missed
	a.ui.mind.missed = nil
	if len(ms) == 0 {
		return
	}
	var parts []string
	seen := map[string]bool{}
	md, focus := moodHappy, ms[len(ms)-1].session
	for _, n := range ms {
		k := n.kind + n.project
		if seen[k] {
			continue
		}
		seen[k] = true
		switch n.kind {
		case "done":
			parts = append(parts, n.project+" done")
		case "fail":
			parts, md = append(parts, n.project+" stopped"), moodError
		case "wait":
			parts, md, focus = append(parts, n.project+" asked you"), moodWait, n.session
		}
	}
	a.ui.Moment(moment{key: "away", act: "claude", mood: md, text: "While you were away",
		sub: strings.Join(parts, " · "), until: now.Add(10 * time.Second), focus: focus})
}

// asks records that a session waits on you, and what for: the permission or
// the question the terminal is showing. Ember only reports it.
func (a *app) asks(s *Session, what string, now time.Time) {
	if s.State != "wait" {
		a.ui.play("needs", 0)
	}
	s.State = "wait"
	s.Asking = what
	a.ui.mind.kick(now, 0.4, -0.05)
	if a.ui.mind.away {
		a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"wait", a.ui.name(s), s.ID})
	}
}

// wanting is what a permission request is about, in the session's words:
// "Wants to run go test ./...", "Wants to edit board.go".
func wanting(tool string, tin map[string]any) string {
	str := func(k string) string { v, _ := tin[k].(string); return v }
	switch tool {
	case "Bash":
		if c := str("command"); c != "" {
			return "Wants to run " + firstLine(c)
		}
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		if f := filepath.Base(str("file_path") + str("notebook_path")); f != "." {
			return "Wants to " + wants(tool) + " " + f
		}
	case "WebFetch":
		if u, err := url.Parse(str("url")); err == nil && u.Host != "" {
			return "Wants to fetch " + u.Host
		}
	}
	return "Wants to " + wants(tool)
}

// firstQuestion is the first question of an AskUserQuestion call.
func firstQuestion(tin map[string]any) string {
	qs, _ := tin["questions"].([]any)
	if len(qs) > 0 {
		if q, ok := qs[0].(map[string]any); ok {
			if t, _ := q["question"].(string); t != "" {
				return t
			}
		}
	}
	return "A question"
}

// friendlyError turns StopFailure's code into words, the same everywhere.
func friendlyError(code string) string {
	switch c := strings.ToLower(code); {
	case strings.Contains(c, "rate"):
		return "Rate limited"
	case strings.Contains(c, "overload"), strings.Contains(c, "server"), strings.Contains(c, "api"):
		return "API error, try again"
	case strings.Contains(c, "auth"):
		return "Signed out, run /login"
	case strings.Contains(c, "billing"), strings.Contains(c, "credit"):
		return "Billing problem"
	case strings.Contains(c, "max_output"), strings.Contains(c, "token"):
		return "Reply hit the length limit"
	case c == "":
		return "Stopped unexpectedly"
	}
	return strings.ToUpper(code[:1]) + strings.ReplaceAll(code[1:], "_", " ")
}

// wants is what a tool asks to do, for a request's eyebrow.
func wants(tool string) string {
	switch tool {
	case "Bash":
		return "run"
	case "Edit", "MultiEdit", "NotebookEdit":
		return "edit"
	case "Write":
		return "create"
	case "WebFetch":
		return "fetch"
	case "WebSearch":
		return "search"
	}
	if strings.HasPrefix(tool, "mcp__") { // mcp__server__tool: the tool's own name
		parts := strings.Split(tool, "__")
		tool = strings.ReplaceAll(parts[len(parts)-1], "_", " ")
	}
	return "use " + tool
}
