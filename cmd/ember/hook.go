package main

import (
	"io"
	"os"

	"github.com/Tejas242/ember/internal/ipc"
)

// hook relays a Claude Code hook event. Only a permission request or a
// question waits for an answer (90 s at most); everything else returns at
// once, and nothing waits when the island isn't running.
func hook(args []string) int {
	event := args[0]
	raw, _ := io.ReadAll(os.Stdin)
	switch event {
	case "PermissionRequest", "AskUserQuestion":
		if out := ipc.Ask(event, raw); out != nil {
			os.Stdout.Write(out)
		}
	default:
		ipc.Send(event, raw)
	}
	return 0
}
