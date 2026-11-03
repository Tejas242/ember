package main

import (
	"io"
	"os"

	"github.com/Tejas242/ember/internal/ipc"
)

// hook relays a Claude Code hook event to the island and returns at once.
// It never prints a decision: Claude is never held for Ember, and nothing
// waits when the island isn't running.
func hook(args []string) int {
	raw, _ := io.ReadAll(os.Stdin)
	ipc.Send(args[0], raw)
	return 0
}
