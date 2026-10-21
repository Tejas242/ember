package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// demoPace scales every pause in the demo (a test sets it to 0).
var demoPace = 1.0

// runDemo plays a short Claude Code session on the running island, so you
// can see what Ember does without starting one: it works for a few steps,
// asks to run a command (answering itself after a moment if you don't), and
// finishes. It never touches Claude, and what Ember learns about you (your
// day, your habits) leaves it out.
func runDemo([]string) int {
	if _, ok := ipc.QueryStatus(); !ok {
		fmt.Fprintln(os.Stderr, "ember demo: the island isn't running (ember doctor says why)")
		return 1
	}
	id := ipc.DemoSession + strconv.FormatInt(time.Now().UnixNano(), 36)
	pause := func(s float64) { time.Sleep(time.Duration(s * demoPace * float64(time.Second))) }
	hook := func(ev string, m map[string]any) []byte {
		m["session_id"], m["cwd"] = id, "/demo"
		b, _ := json.Marshal(m)
		if ev == "PermissionRequest" {
			return ipc.AskHook(ev, b, nil)
		}
		ipc.SendHook(ev, b, nil)
		return nil
	}
	tool := func(name string, in map[string]any) {
		hook("PreToolUse", map[string]any{"tool_name": name, "tool_input": in})
		pause(1.6)
		hook("PostToolUse", map[string]any{"tool_name": name, "tool_input": in})
	}

	fmt.Println("Watch the top of your screen: a Claude Code session, played on the island.")
	hook("SessionStart", map[string]any{"source": "startup"})
	hook("UserPromptSubmit", map[string]any{"prompt": "make the store's tests pass"})
	pause(1.5)
	tool("Read", map[string]any{"file_path": "/demo/store/store.go"})
	tool("Grep", map[string]any{"pattern": "func Compact"})
	tool("Edit", map[string]any{"file_path": "/demo/store/store.go", "old_string": "a\nb", "new_string": "a\nb\nc\nd"})

	fmt.Println("It asks to run a command. Answer it on the island; in 8 seconds it answers itself.")
	cmd := map[string]any{"command": "go test ./store/...", "description": "Run the store's tests"}
	hook("PreToolUse", map[string]any{"tool_name": "Bash", "tool_input": cmd})
	answered := make(chan []byte, 1)
	go func() { answered <- hook("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": cmd}) }()
	select {
	case out := <-answered:
		fmt.Println(verdict(out, "you"))
	case <-time.After(time.Duration(8 * demoPace * float64(time.Second))):
		// only the demo's own request, never a real one; again until the island has it
		tick := time.NewTicker(300 * time.Millisecond)
		defer tick.Stop()
		ipc.Cmd("allow", id)
	wait:
		for {
			select {
			case out := <-answered:
				fmt.Println(verdict(out, "the demo"))
				break wait
			case <-tick.C:
				ipc.Cmd("allow", id)
			}
		}
	}
	pause(2)
	hook("PostToolUse", map[string]any{"tool_name": "Bash", "tool_input": cmd})
	pause(1)
	hook("Stop", map[string]any{"last_assistant_message": "All 12 tests in the store pass now."})
	fmt.Println("Done. That's what a session looks like; the demo leaves in a few seconds.")
	pause(8)
	hook("SessionEnd", map[string]any{})
	return 0
}

// verdict says how the demo's request was answered.
func verdict(out []byte, who string) string {
	var r struct {
		HookSpecificOutput struct {
			Decision struct {
				Behavior string `json:"behavior"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	if json.Unmarshal(out, &r) != nil || r.HookSpecificOutput.Decision.Behavior == "" {
		return "The request went back to Claude's own prompt (answering from the island is off)."
	}
	return fmt.Sprintf("%s answered: %s.", capital(who), r.HookSpecificOutput.Decision.Behavior)
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}
