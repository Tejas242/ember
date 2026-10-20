package island

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// hookInput is the subset of Claude Code's hook payload the island uses.
type hookInput struct {
	SessionID   string          `json:"session_id"`
	Cwd         string          `json:"cwd"`
	Tool        string          `json:"tool_name"`
	ToolInput   json.RawMessage `json:"tool_input"`
	Suggestions json.RawMessage `json:"permission_suggestions"`
	NotifyType  string          `json:"notification_type"`
	Message     string          `json:"message"` // Notification
	LastMessage string          `json:"last_assistant_message"`
	Error       string          `json:"error"`      // StopFailure
	Prompt      string          `json:"prompt"`     // UserPromptSubmit
	Source      string          `json:"source"`     // SessionStart: startup | resume | clear | compact
	AgentID     string          `json:"agent_id"`   // set on every hook a subagent fires
	AgentType   string          `json:"agent_type"` // Explore, general-purpose, …
}

type server struct {
	ln    *net.UnixListener
	posts chan<- func(*app)
}

func listen(posts chan<- func(*app)) (*server, error) {
	p := ipc.SockPath()
	if c, err := net.DialTimeout("unix", p, 200*time.Millisecond); err == nil {
		c.Close()
		return nil, fmt.Errorf("another island is running on %s", p)
	}
	os.Remove(p)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: p, Net: "unix"})
	if err != nil {
		return nil, err
	}
	os.Chmod(p, 0o600)
	s := &server{ln: ln, posts: posts}
	go s.accept()
	return s, nil
}

// close stops listening; the socket file goes with it.
func (s *server) close() { s.ln.Close() }

func (s *server) accept() {
	for {
		c, err := s.ln.AcceptUnix()
		if err != nil {
			return
		}
		go s.serve(c)
	}
}

func (s *server) serve(c *net.UnixConn) {
	defer c.Close()
	r := bufio.NewReader(c)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	var m ipc.Msg
	if json.Unmarshal(line, &m) != nil {
		return
	}
	switch m.Type {
	case "hook":
		if !m.Wait {
			s.posts <- func(a *app) { a.hook(m, nil) }
			return
		}
		answer := make(chan json.RawMessage, 1)
		gone := make(chan struct{})
		go func() { // the hook process exits if Claude cancels: drop the card
			r.ReadByte()
			close(gone)
		}()
		cardID := ""
		s.posts <- func(a *app) { cardID = a.hook(m, answer) }
		var out json.RawMessage
		select {
		case out = <-answer:
		case <-gone:
			s.posts <- func(a *app) { a.ui.RemoveCard(cardID); a.ui.Invalidate() }
			return
		case <-time.After(ipc.AskTimeout):
		}
		b, _ := json.Marshal(ipc.Reply{Output: out})
		c.Write(append(b, '\n'))
	case "edge": // edge's own address, from before plugins
		s.posts <- func(a *app) { a.toPlugin("edge", m.Payload) }
	case "plugin": // a tool talking to its plugin
		s.posts <- func(a *app) { a.toPlugin(m.Name, m.Payload) }
	case "note", "line": // `ember note`, `ember status`: a moment, a pill line
		s.posts <- func(a *app) { a.cli(m) }
	case "cmd": // keyboard: allow / deny / always / terminal / answer <key>
		s.posts <- func(a *app) { a.command(m.Event, m.ID) }
	case "status": // `ember doctor` asks what the island is connected to
		st := make(chan ipc.Status, 1)
		s.posts <- func(a *app) { st <- a.status() }
		select {
		case v := <-st:
			b, _ := json.Marshal(v)
			c.Write(append(b, '\n'))
		case <-time.After(2 * time.Second):
		}
	}
}

// ---- hook → UI ----

func project(cwd string) string {
	h, _ := os.UserHomeDir()
	if cwd == "" || cwd == h {
		return "~"
	}
	return filepath.Base(cwd)
}

func lines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// activity turns a tool call into a short present-tense label.
func activity(tool string, in map[string]any) (label string, plus, minus int) {
	str := func(k string) string { v, _ := in[k].(string); return v }
	base := filepath.Base(str("file_path"))
	switch tool {
	case "Read":
		return "Reading " + base, 0, 0
	case "Edit":
		return "Editing " + base, lines(str("new_string")), lines(str("old_string"))
	case "MultiEdit":
		edits, _ := in["edits"].([]any)
		for _, e := range edits {
			if m, ok := e.(map[string]any); ok {
				n, _ := m["new_string"].(string)
				o, _ := m["old_string"].(string)
				plus, minus = plus+lines(n), minus+lines(o)
			}
		}
		return "Editing " + base, plus, minus
	case "Write":
		return "Writing " + base, lines(str("content")), 0
	case "Bash":
		if d := str("description"); d != "" {
			return d, 0, 0
		}
		return "Running " + str("command"), 0, 0
	case "Grep", "Glob":
		return "Searching " + str("pattern"), 0, 0
	case "WebFetch":
		if u, err := url.Parse(str("url")); err == nil {
			return "Reading " + u.Host, 0, 0
		}
	case "WebSearch":
		return "Searching the web", 0, 0
	case "Task", "Agent":
		return "Delegating: " + str("description"), 0, 0
	case "Skill":
		return "Using " + str("skill"), 0, 0
	case "TodoWrite", "EnterPlanMode":
		return "Planning", 0, 0
	case "ExitPlanMode":
		return "Presenting a plan", 0, 0
	case "NotebookEdit":
		return "Editing " + filepath.Base(str("notebook_path")), 0, 0
	case "ToolSearch":
		return "Loading tools", 0, 0
	case "Artifact":
		return "Publishing a page", 0, 0
	case "Monitor":
		if d := str("description"); d != "" {
			return "Watching " + d, 0, 0
		}
		return "Watching", 0, 0
	case "ScheduleWakeup":
		return "Scheduling a check-in", 0, 0
	case "SendMessage":
		return "Messaging an agent", 0, 0
	case "LSP":
		return "Asking the language server", 0, 0
	}
	if strings.HasPrefix(tool, "mcp__") {
		parts := strings.Split(tool, "__")
		return "Using " + strings.ReplaceAll(parts[len(parts)-1], "_", " "), 0, 0
	}
	return tool, 0, 0
}
