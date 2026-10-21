package island

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
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

// forgetIdle: an idle session silent this long is forgotten, its process or not.
const forgetIdle = 3 * time.Hour

// ---- Claude Code hooks ----

// isDemo: a session ember demo plays. It shows like any other, but isn't
// counted in the day, the habits or what Ember has learned.
func isDemo(s *Session) bool { return strings.HasPrefix(s.ID, ipc.DemoSession) }

func (a *app) hook(m ipc.Msg, answer chan<- json.RawMessage) string {
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
		if answer != nil {
			answer <- nil
		}
		return ""
	}
	var tin map[string]any
	if err := json.Unmarshal(in.ToolInput, &tin); err != nil && len(in.ToolInput) > 0 {
		logOnce("tool_input:"+in.Tool, "%s's tool_input Ember can't read: %v (this is said once)", in.Tool, err)
	}
	if in.AgentID != "" && a.agentHook(s, m.Event, in, tin, now) {
		return ""
	}

	switch m.Event {
	case "SessionEnd":
		a.end(in.SessionID, now)
	case "SessionStart":
		if in.Source == "startup" { // a compaction or a resume keeps its background agents
			s.Agents, s.asks = nil, nil
		}
		if in.Source == "clear" || in.Source == "resume" { // a new session in the same Claude: the old one is over
			for id, o := range a.ui.sessions {
				if o != s && sameProcess(o.Pids, m.Pids) {
					a.end(id, now)
				}
			}
		}
	case "UserPromptSubmit":
		s.State, s.Label, s.Plus, s.Minus, s.Since, s.StepAt = "work", "Thinking", 0, 0, now, now
		s.Turn, s.Finished, s.Recap, s.Unseen, s.Asking = Turn{}, nil, nil, time.Time{}, ""
		s.Prompt = firstLine(strings.TrimSpace(in.Prompt))
		a.ui.mind.kick(now, 0.25, 0.05)
		a.ui.play("ignite", 0) // it catches
		a.ui.mind.novel["claude"] = now
	case "PreCompact":
		s.State, s.Label, s.Plus, s.Minus = "work", "Compacting the conversation", 0, 0
	case "PostCompact":
		if s.Label == "Compacting the conversation" {
			s.Label = "Thinking"
			a.ui.play("pop", 0) // squeezed, and now it springs back
		}
	case "StopFailure":
		s.State, s.Label = "idle", ""
		msg := friendlyError(in.Error)
		took := now.Sub(s.Since)
		if s.Since.IsZero() {
			took = 0
		}
		s.Turn.finish(false)
		s.Recap = &Recap{Failed: true, Error: msg, At: now, Took: took, Steps: s.Turn.Count,
			Files: len(s.Turn.Files), Plus: s.Turn.Plus, Minus: s.Turn.Minus, Failures: s.Turn.Failures}
		if !a.looking(s) {
			s.Unseen = now
		}
		a.ui.mind.kick(now, 0.35, -0.6) // a start, and a low mood that takes a while to lift
		if a.ui.mind.away {
			a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"fail", a.ui.name(s), s.ID})
			break
		}
		a.ui.Moment(moment{key: "fail:" + s.ID, act: "claude", mood: moodError, verb: "Stopped", text: a.ui.short(s),
			sub: msg, until: now.Add(8 * time.Second), focus: s.ID})
	case "AskUserQuestion":
		if answer != nil {
			s.State = "wait"
			a.ui.play("needs", 0)
			return a.ask(s, in, tin, answer)
		}
	case "PreToolUse":
		if in.Tool == "AskUserQuestion" {
			break
		}
		s.State = "work"
		if in.Tool == "Agent" || in.Tool == "Task" {
			s.asked(tin)
		}
		label := s.Label
		s.Label, s.Plus, s.Minus = activity(in.Tool, tin)
		if s.Label != label || s.StepAt.IsZero() {
			s.StepAt = now
		}
		s.Turn.step(now, in.Tool, tin)
		if !isDemo(s) {
			a.ui.day.call(now)
		}
		a.ui.mind.toolCall(now)
		if ss := a.ui.sessionList(); len(ss) > 0 && ss[0] == s && now.Sub(a.ui.gulpAt) > 1200*time.Millisecond {
			a.ui.gulpAt = now
			a.ui.body.gulp(0.5) // a bite of fuel, for the session it's watching
		}
	case "PostToolUse":
		if s.State == "wait" {
			s.State = "work"
		}
		s.Asking = ""
		if s.Turn.streak >= 3 { // it was failing, and now it isn't: relief
			a.ui.play("relief", 0)
			a.ui.mind.kick(now, 0, 0.25)
		}
		s.Turn.finish(false)
	case "PostToolUseFailure":
		s.Turn.finish(true)
		a.ui.React("wince")
		a.ui.mind.kick(now, 0.1, -0.12) // a small wince
	case "PermissionRequest":
		if answer == nil {
			return ""
		}
		s.State = "wait"
		a.ui.play("needs", 0)
		return a.permission(s, in, tin, answer)
	case "Notification":
		// a prompt in the terminal (the island timed out, or a dialog it
		// can't show). idle_prompt is just "Claude is done": not a request.
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
		s.Turn.finish(false)
		took := now.Sub(s.Since)
		if s.Since.IsZero() {
			took = 0
		}
		s.Recap = &Recap{Summary: summarize(in.LastMessage), At: now, Took: took, Steps: s.Turn.Count,
			Files: len(s.Turn.Files), Plus: s.Turn.Plus, Minus: s.Turn.Minus, Failures: s.Turn.Failures}
		s.cheer = cheer{size: doneSize(took), roll: absolutelyRight.MatchString(in.LastMessage)}
		if b := a.ui.bond; b != nil && !isDemo(s) {
			long := b.turn(took)
			hard := s.Turn.Failures >= 2 || s.Turn.Count >= 40 // failures overcome, or a lot of work
			s.cheer.flare = (long || hard) && s.Turn.streak == 0 && !s.Recap.troubled()
		}
		if !isDemo(s) {
			a.ui.day.done(now, s.Project, took)
		}
		s.Asking = ""
		if !a.looking(s) {
			s.Unseen = now // it stays in the pill, quietly, until you've seen it
		}
		a.ui.mind.kick(now, 0.2, 0.35) // pleased
		if a.ui.mind.away {            // tell them when they're back
			a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"done", a.ui.name(s), s.ID})
			break
		}
		if !a.ui.cfg.Done || a.looking(s) || s.running() > 0 {
			break // they're looking at it, or it isn't done: its agents work on
		}
		a.done(s, summarize(in.LastMessage), now)
	}
	return ""
}

// done shows "Done · project". If another session's note is still up, the
// two merge into one ("Done · edge, ember") rather than one hiding the other.
func (a *app) done(s *Session, summary string, now time.Time) {
	name := a.ui.short(s)
	m := moment{key: "done:" + s.ID, act: "claude", mood: moodHappy, verb: "Done", text: name,
		sub: summary, until: now.Add(a.ui.cfg.DoneFor), focus: s.ID, many: []string{name}, cheer: s.cheer}
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
			m.until = now.Add(a.ui.cfg.DoneFor + time.Second)
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

func (a *app) permission(s *Session, in hookInput, tin map[string]any, answer chan<- json.RawMessage) string {
	a.ui.mind.kick(a.ui.now(), 0.4, -0.05)
	if a.ui.mind.away {
		a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"wait", a.ui.name(s), s.ID})
	}
	str := func(k string) string { v, _ := tin[k].(string); return v }
	if !a.ui.cfg.Answer { // [claude] answer = false: the terminal asks
		answer <- nil
		return ""
	}
	c := &Card{ID: "perm:" + s.ID + ":" + fmt.Sprint(time.Now().UnixNano()), Kind: "perm", Mood: moodWait,
		Eyebrow: a.ui.short(s) + " wants to " + wants(in.Tool), Priority: 10, Footer: "Answer in terminal", Expires: a.ui.now().Add(a.ui.cfg.Wait)}
	switch in.Tool {
	case "Bash": // Claude says what it wants in words: that's the title; the command is the detail
		c.Title, c.Code, c.Shell = str("description"), str("command"), true
		if c.Title == "" {
			c.Title = "Run a command"
		}
	case "Edit", "MultiEdit", "Write":
		verb := map[string]string{"Write": "Create"}[in.Tool]
		if verb == "" {
			verb = "Edit"
		}
		_, p, m := activity(in.Tool, tin)
		c.Title = verb + " " + filepath.Base(str("file_path"))
		c.Body = fmt.Sprintf("%s · %s", where(str("file_path")), diffText(max(p, 0), m))
		if p == 0 && m == 0 {
			c.Body = where(str("file_path"))
		}
	case "WebFetch":
		u, _ := url.Parse(str("url"))
		c.Title, c.Code = "Fetch "+u.Host, str("url")
	default:
		label, _, _ := activity(in.Tool, tin)
		c.Title = label
		if b, err := json.Marshal(tin); err == nil && len(b) > 2 {
			c.Code = string(b)
		}
	}
	c.Buttons = []Button{{Key: "deny", Label: "Deny", Danger: true}}
	if len(in.Suggestions) > 2 && string(in.Suggestions) != "null" {
		c.Buttons = append(c.Buttons, Button{Key: "always", Label: "Always"})
		c.Note = alwaysNote(in.Suggestions)
	}
	c.Buttons = append(c.Buttons, Button{Key: "allow", Label: "Allow", Primary: true})
	decide := func(decision map[string]any) {
		b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": "PermissionRequest", "decision": decision}})
		answer <- b
	}
	c.OnPick = func(k string, _ []int) {
		s.State = "work"
		switch k {
		case "allow":
			decide(map[string]any{"behavior": "allow"})
		case "always":
			decide(map[string]any{"behavior": "allow", "updatedPermissions": in.Suggestions})
		case "deny":
			decide(map[string]any{"behavior": "deny", "message": "Denied from Ember."})
		default: // In terminal: no decision, Claude asks there
			s.Asking = asking(c)
			answer <- nil
		}
	}
	c.OnExpire = func() { s.Asking = asking(c); answer <- nil }
	a.ui.AddCard(c)
	return c.ID
}

// asking is what a request handed to the terminal was about, for the card.
func asking(c *Card) string {
	if c.Shell && c.Code != "" {
		return "Wants to run " + firstLine(c.Code)
	}
	return c.Title
}

// alwaysNote says what "Always" would allow from now on, from the rules
// Claude suggests: "Always allows go test:* in this project".
func alwaysNote(raw json.RawMessage) string {
	var sugg []struct {
		Type  string `json:"type"`
		Rules []struct {
			Tool    string `json:"toolName"`
			Content string `json:"ruleContent"`
		} `json:"rules"`
		Destination string `json:"destination"`
	}
	if json.Unmarshal(raw, &sugg) != nil {
		return ""
	}
	var what []string
	where := ""
	for _, sg := range sugg {
		for _, r := range sg.Rules {
			switch {
			case r.Content != "" && r.Tool == "Bash":
				what = append(what, r.Content)
			case r.Content != "":
				what = append(what, r.Tool+" "+r.Content)
			default:
				what = append(what, "every "+r.Tool+" call")
			}
		}
		switch sg.Destination {
		case "userSettings":
			where = " everywhere"
		case "projectSettings", "localSettings":
			where = " in this project"
		case "session":
			where = " this session"
		}
	}
	if len(what) == 0 {
		if len(sugg) > 0 && sugg[0].Type == "setMode" {
			return "Always switches to accepting edits"
		}
		return ""
	}
	return "Always allows " + strings.Join(what, ", ") + where
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

// where is a file's folder, short: "~/Gems/edge/store".
func where(p string) string {
	if p == "" {
		return ""
	}
	d := filepath.Dir(p)
	if h, _ := os.UserHomeDir(); h != "" && strings.HasPrefix(d, h) {
		d = "~" + strings.TrimPrefix(d, h)
	}
	return d
}

type question struct {
	Question    string `json:"question"`
	Header      string `json:"header"`
	MultiSelect bool   `json:"multiSelect"`
	Options     []struct {
		Label       string `json:"label"`
		Description string `json:"description"`
	} `json:"options"`
}

// ask walks the AskUserQuestion questions one card at a time.
func (a *app) ask(s *Session, in hookInput, tin map[string]any, answer chan<- json.RawMessage) string {
	a.ui.mind.kick(a.ui.now(), 0.4, 0)
	if a.ui.mind.away {
		a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"wait", a.ui.name(s), s.ID})
	}
	var qs []question
	b, _ := json.Marshal(tin["questions"])
	json.Unmarshal(b, &qs)
	if len(qs) == 0 || !a.ui.cfg.Answer {
		answer <- nil
		return ""
	}
	answers := map[string]string{}
	deadline := a.ui.now().Add(a.ui.cfg.Wait)
	id := "ask:" + s.ID
	var show func(i int)
	show = func(i int) {
		q := qs[i]
		c := &Card{ID: id, Kind: "ask", Mood: moodWait, Title: q.Question, Eyebrow: a.ui.short(s) + " asks", Priority: 9,
			Multi: q.MultiSelect, Footer: "Answer in terminal", Expires: deadline}
		if len(qs) > 1 {
			c.Meta = fmt.Sprintf("%d of %d", i+1, len(qs))
		}
		for _, o := range q.Options {
			if len(c.Options) == 4 {
				break
			}
			c.Options = append(c.Options, Option{o.Label, o.Description})
		}
		if q.MultiSelect {
			c.Buttons = []Button{{Key: "submit", Label: "Submit", Primary: true}}
		}
		c.OnPick = func(k string, picked []int) {
			if k == "footer" || k == "dismiss" || (k == "submit" && len(picked) == 0) {
				s.Asking = "Asks: " + q.Question
				answer <- nil
				return
			}
			var labels []string
			for _, p := range picked {
				labels = append(labels, q.Options[p].Label)
			}
			answers[q.Question] = strings.Join(labels, ", ")
			if i+1 < len(qs) {
				show(i + 1)
				return
			}
			s.State = "work"
			b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
				"hookEventName": "PreToolUse", "permissionDecision": "allow",
				"updatedInput": map[string]any{"questions": tin["questions"], "answers": answers}}})
			answer <- b
		}
		c.OnExpire = func() { s.Asking = "Asks: " + q.Question; answer <- nil }
		a.ui.AddCard(c)
	}
	show(0)
	return id
}
