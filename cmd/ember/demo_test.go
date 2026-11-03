package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// The demo plays a whole session on the island and holds nothing: its
// question is only reported.
func TestDemo(t *testing.T) {
	demoPace = 0
	defer func() { demoPace = 1 }()
	if code := runDemo(nil); code != 1 {
		t.Fatalf("no island: exit %d, want 1", code)
	}
	os.MkdirAll(strings.TrimSuffix(ipc.SockPath(), "/ember.sock"), 0o700)
	ln, err := net.Listen("unix", ipc.SockPath())
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var mu sync.Mutex
	var events []string
	handle := func(c net.Conn) {
		line, _ := bufio.NewReader(c).ReadBytes('\n')
		var m ipc.Msg
		json.Unmarshal(line, &m)
		mu.Lock()
		defer mu.Unlock()
		switch {
		case m.Type == "status":
			b, _ := json.Marshal(ipc.Status{Version: "test"})
			c.Write(append(b, '\n'))
		case m.Type == "hook":
			var in struct {
				SessionID string `json:"session_id"`
			}
			json.Unmarshal(m.Payload, &in)
			if !strings.HasPrefix(in.SessionID, ipc.DemoSession) || len(m.Pids) != 0 {
				t.Errorf("%s: session %q pids %v: want a demo session with no terminal of its own", m.Event, in.SessionID, m.Pids)
			}
			events = append(events, m.Event)
		}
		c.Close()
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(c)
		}
	}()
	if code := runDemo(nil); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for i := 0; i < 200; i++ { // the last hooks are on their way: the fake island records them in its own time
		mu.Lock()
		all := strings.Join(events, " ")
		end := strings.Contains(all, "Stop") && strings.Contains(all, "SessionEnd")
		mu.Unlock()
		if end {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	got := strings.Join(events, " ")
	for _, want := range []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "PermissionRequest", "Stop", "SessionEnd"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in %s", want, got)
		}
	}
}
