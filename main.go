// Ember: a dynamic island for the top of the screen. Claude Code sessions,
// permission requests and questions, music, system moments (volume,
// brightness, battery, devices, screenshots), the clock, and edge.
// `ember help` lists the commands.
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
	case "dash":
		name := ""
		if len(os.Args) > 2 {
			name = os.Args[2]
		}
		cmdOrFail(island.Cmd("dash", name))
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
	case "uninstall":
		if err := uninstall(); err != nil {
			fmt.Fprintln(os.Stderr, "ember:", err)
			os.Exit(1)
		}
	case "doctor":
		doctor()
	case "version", "--version", "-v":
		fmt.Println("ember", versionString())
	case "help", "--help", "-h":
		fmt.Print(help)
	default:
		usage()
	}
}

const help = `ember: a dynamic island for the top of your screen

  ember                  run the island (the service does this)
  ember install          systemd service, Claude Code hooks, default config
  ember uninstall        remove the hooks and the service (the config stays)
  ember doctor           check every piece and say what to fix
  ember version

  ember open             open the card
  ember close | toggle
  ember next | prev      switch what the pill shows
  ember play             play / pause
  ember dash [name]      open or close a plugin's dashboard (edge)

  ember allow | deny | always   answer the oldest permission request
  ember terminal                hand it back to Claude's own prompt
  ember answer opt:N | submit   answer the oldest question

  ember hook <Event>     Claude Code's hook (installed for you)
  ember edge '<json>'    a message for the edge plugin

Plugins: edge (on when edge is installed; [plugins.edge] in the config)

Config: ~/.config/ember/config.toml (re-read on change)
`

func usage() {
	fmt.Fprint(os.Stderr, help)
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
