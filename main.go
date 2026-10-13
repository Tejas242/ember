// Ember: a dynamic island for the top of the screen. Claude Code sessions,
// permission requests and questions, music, the clock, and edge.
//
//	ember                     run the island (normally as a systemd user service)
//	ember hook <Event>        Claude Code's hook (installed by `ember install`)
//	ember allow|deny|always   answer the oldest permission request
//	ember terminal            hand the oldest request back to Claude's prompt
//	ember answer opt:2|submit answer the oldest question
//	ember toggle              open / close the island's card
//	ember open [page]         open the card (claude, music, edge, system)
//	ember close               close it
//	ember next|prev           show the next / previous activity in the pill
//	ember play                play / pause the music
//	ember edge '<json>'       a status or card from edge
//	ember install             systemd service + Claude Code hooks
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Tejas242/ember/island"
)

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "", "run":
		if err := island.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "ember:", err)
			os.Exit(1)
		}
	case "hook":
		hook()
	case "allow", "deny", "always", "terminal", "toggle", "close", "next", "prev", "play":
		cmdOrFail(island.Cmd(cmd, ""))
	case "open":
		page := ""
		if len(os.Args) > 2 {
			page = os.Args[2]
		}
		cmdOrFail(island.Cmd("open", page))
	case "answer":
		if len(os.Args) != 3 {
			usage()
		}
		cmdOrFail(island.Cmd("answer", os.Args[2]))
	case "edge":
		if len(os.Args) != 3 {
			usage()
		}
		cmdOrFail(island.Edge([]byte(os.Args[2])))
	case "install":
		if err := install(); err != nil {
			fmt.Fprintln(os.Stderr, "ember:", err)
			os.Exit(1)
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ember [run|hook <Event>|allow|deny|always|terminal|answer <key>|toggle|open [page]|close|next|prev|play|edge <json>|install]")
	os.Exit(2)
}

func cmdOrFail(ok bool) {
	if !ok {
		fmt.Fprintln(os.Stderr, "ember: the island isn't running")
		os.Exit(1)
	}
}

// hook relays a Claude Code hook event. Only a permission request or a
// question waits for an answer (90 s at most); everything else returns at
// once, and nothing waits when the island isn't running.
func hook() {
	if len(os.Args) < 3 {
		return
	}
	event := os.Args[2]
	raw, _ := io.ReadAll(os.Stdin)
	switch event {
	case "PermissionRequest", "AskUserQuestion":
		if out := island.Ask(event, raw); out != nil {
			os.Stdout.Write(out)
		}
	default:
		island.Send(event, raw)
	}
}
