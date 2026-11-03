package island

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// hookInput is the subset of Claude Code's hook payload the island uses.
type hookInput struct {
	SessionID   string          `json:"session_id"`
	Cwd         string          `json:"cwd"`
	Tool        string          `json:"tool_name"`
	ToolInput   json.RawMessage `json:"tool_input"`
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
		s.posts <- func(a *app) { a.hook(m) }
	case "edge": // edge's own address, from before plugins: it hears whether the plugin got it
		ok := make(chan bool, 1)
		s.posts <- func(a *app) { ok <- a.toPlugin("edge", m.Payload) }
		select {
		case v := <-ok:
			b, _ := json.Marshal(ipc.Ack{OK: v})
			c.Write(append(b, '\n'))
		case <-time.After(2 * time.Second):
		}
	case "plugin": // a tool talking to its plugin
		s.posts <- func(a *app) { a.toPlugin(m.Name, m.Payload) }
	case "note", "line": // `ember note`, `ember status`: a moment, a pill line
		s.posts <- func(a *app) { a.cli(m) }
	case "cmd": // keyboard: open, close, toggle, next, prev, play, dash <name>
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
