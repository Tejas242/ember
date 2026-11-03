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
// asks to run a command, and finishes. It never touches Claude, and what Ember learns about you (your
// day, your habits) leaves it out.
func runDemo([]string) int {
	if _, ok := ipc.QueryStatus(); !ok {
		fmt.Fprintln(os.Stderr, "ember demo: the island isn't running (ember doctor says why)")
		return 1
	}
	id := ipc.DemoSession + strconv.FormatInt(time.Now().UnixNano(), 36)
	pause := func(s float64) { time.Sleep(time.Duration(s * demoPace * float64(time.Second))) }
	hook := func(ev string, m map[string]any) {
		m["session_id"], m["cwd"] = id, "/demo"
		b, _ := json.Marshal(m)
		ipc.SendHook(ev, b, nil)
	}
	tool := func(name string, in map[string]any) {
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

	fmt.Println("It asks to run a command; the island says who asks and what.")
	cmd := map[string]any{"command": "go test ./store/...", "description": "Run the store's tests"}
	hook("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": cmd})
	pause(6)
	pause(2)
	hook("PostToolUse", map[string]any{"tool_name": "Bash", "tool_input": cmd})
	pause(1)
	hook("Stop", map[string]any{"last_assistant_message": "All 12 tests in the store pass now."})
	fmt.Println("Done. That's what a session looks like; the demo leaves in a few seconds.")
	pause(8)
	hook("SessionEnd", map[string]any{})
	return 0
}
