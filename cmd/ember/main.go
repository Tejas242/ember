// Ember: a dynamic island for Wayland, made for Claude Code. Claude Code
// sessions, permission requests and questions, music, system moments
// (volume, brightness, battery, devices, screenshots), the clock, and plugins.
// `ember help` lists the commands.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/island"
	"github.com/Tejas242/ember/internal/wayland"
)

// command is one subcommand: its usage, how many arguments it takes, and
// what it does. Every argument rule lives here, so no command ever runs on
// arguments it didn't ask for (`ember install --help` prints help).
type command struct {
	name    string
	aliases []string
	args    string // the usage after the name: "<Event>", "[page]"
	summary string
	need    string // what a missing argument is, for the error
	min     int    // positional arguments
	max     int
	group   int // help's paragraph: groupSetup, groupControl, groupPlugins
	hidden  bool
	run     func(args []string) int
	flags   []string                                     // --name value flags it takes
	bools   []string                                     // --name flags without a value
	runf    func(args []string, f map[string]string) int // run, for a command with flags
}

var commands []*command

// help's paragraphs, in order.
const (
	groupSetup = iota
	groupControl
	groupPlugins
)

var groupNames = []string{"Setup", "Control", "Plugins"}

func init() {
	send := func(name string) func([]string) int {
		return func([]string) int { return cmdOrFail(ipc.Cmd(name, "")) }
	}
	withArg := func(name string) func([]string) int {
		return func(a []string) int {
			arg := ""
			if len(a) > 0 {
				arg = a[0]
			}
			return cmdOrFail(ipc.Cmd(name, arg))
		}
	}
	commands = []*command{
		{name: "install", summary: "set up the service, Claude Code's hooks and a config", run: runInstall},
		{name: "doctor", summary: "check every piece and say what to fix", run: func([]string) int { doctor(); return 0 }},
		{name: "uninstall", args: "[--purge]", summary: "remove the hooks and the service (--purge: the config and Ember's memory too)",
			bools: []string{"purge"}, runf: runUninstall},
		{name: "demo", summary: "play a Claude Code session on the island, to see what Ember does", run: runDemo},
		{name: "version", aliases: []string{"--version", "-v"}, summary: "print Ember's version",
			run: func([]string) int { fmt.Println("ember", versionString()); return 0 }},
		{name: "help", aliases: []string{"--help", "-h"}, args: "[command]", max: 1, summary: "this list, or one command's help"},
		{name: "run", summary: "run the island (the service does this)", run: runIsland},
		{name: "hook", args: "<Event>", min: 1, max: 1, need: "a Claude Code hook event",
			summary: "Claude Code's hook (install sets it up)", run: hook},

		{group: groupControl, name: "open", args: "[page]", max: 1, summary: "open the card", run: withArg("open")},
		{group: groupControl, name: "close", summary: "fold the card", run: send("close")},
		{group: groupControl, name: "toggle", summary: "open or fold the card", run: send("toggle")},
		{group: groupControl, name: "next", summary: "switch what the pill shows", run: send("next")},
		{group: groupControl, name: "prev", summary: "switch back", run: send("prev")},
		{group: groupControl, name: "play", summary: "play / pause the music", run: send("play")},
		{group: groupControl, name: "allow", summary: "allow the oldest permission request", run: send("allow")},
		{group: groupControl, name: "deny", summary: "deny it", run: send("deny")},
		{group: groupControl, name: "always", summary: "allow it, and what Claude suggests from now on", run: send("always")},
		{group: groupControl, name: "terminal", summary: "hand it back to Claude's own prompt", run: send("terminal")},
		{group: groupControl, name: "answer", args: "opt:N | submit", min: 1, max: 1, need: "an answer: opt:N or submit",
			summary: "answer the oldest question", run: withArg("answer")},

		{group: groupPlugins, name: "plugins", args: "[name]", max: 1, summary: "each plugin: its state, its health, its last error", run: runPlugins},
		{group: groupPlugins, name: "dash", args: "[name]", max: 1, summary: "open or close a plugin's dashboard", run: withArg("dash")},
		{group: groupPlugins, name: "note", args: "TEXT [--sub S] [--icon I] [--mood M] [--ttl SECONDS] [--open URL]", min: 1, max: 1,
			need: "the note's text", summary: "show a moment: a short note in the pill",
			flags: []string{"sub", "icon", "mood", "ttl", "open"}, runf: runNote},
		{group: groupPlugins, name: "status", args: "ID [TEXT] [--sub S] [--icon I] [--mood M] [--progress 0..1] [--since now|5m] [--urgency U] [--ttl 10m] [--clear]",
			min: 1, max: 2, need: "the line's id", summary: "hold a line in the pill until --ttl or --clear",
			flags: []string{"sub", "icon", "mood", "progress", "since", "urgency", "ttl"}, bools: []string{"clear"}, runf: runStatus},
		{group: groupPlugins, name: "plugin", args: "<name>", min: 1, max: 1, need: "a plugin's name", hidden: true,
			summary: "run one of the plugins built into this binary (Ember starts it)", run: runPlugin},
		{group: groupPlugins, name: "edge", args: "'<json>'", min: 1, max: 1, need: "a JSON message", hidden: true,
			summary: "a message for the edge plugin", run: func(a []string) int { return cmdOrFail(ipc.Edge([]byte(a[0]))) }},
	}
	lookup("help").run = func(a []string) int { return help(a, os.Stdout, os.Stderr) }
}

// help is `ember help [command]`.
func help(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, overview())
		return 0
	}
	c := lookup(args[0])
	if c == nil {
		return unknown(stderr, args[0])
	}
	fmt.Fprint(stdout, c.usage())
	return 0
}

func lookup(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
		for _, a := range c.aliases {
			if a == name {
				return c
			}
		}
	}
	return nil
}

func (c *command) line() string {
	s := "ember " + c.name
	if c.args != "" {
		s += " " + c.args
	}
	return s
}

// short is the line for the list: the flags as "[flags]".
func (c *command) short() string {
	l := c.line()
	if i := strings.Index(l, " [--"); i > 0 {
		l = l[:i] + " [flags]"
	}
	return l
}

func (c *command) usage() string {
	return fmt.Sprintf("usage: %s\n\n%s\n", c.line(), c.summary)
}

func overview() string {
	var b strings.Builder
	b.WriteString("ember: a dynamic island for Wayland, made for Claude Code\n")
	w := 0
	for _, c := range commands {
		if !c.hidden {
			w = max(w, len(c.short()))
		}
	}
	group := -1
	for _, c := range commands {
		if c.hidden {
			continue
		}
		if c.group != group {
			group = c.group
			fmt.Fprintf(&b, "\n%s\n", groupNames[group])
		}
		fmt.Fprintf(&b, "  %-*s  %s\n", w, c.short(), c.summary)
	}
	b.WriteString("\nember help <command> says more about one.\n")
	b.WriteString("Config ~/.config/ember/config.toml (applies within 10 s) · docs: github.com/Tejas242/ember\n")
	return b.String()
}

func unknown(w io.Writer, name string) int {
	fmt.Fprintf(w, "ember: unknown command %q\nrun ember help\n", name)
	return 2
}

// route finds the command for argv (os.Args[1:]) and checks its arguments.
// It returns the command to run with its positional arguments and flags,
// or nil and an exit code once it has printed help or an error itself.
func route(argv []string, stdout, stderr io.Writer) (*command, []string, map[string]string, int) {
	name, args := "run", []string(nil)
	if len(argv) > 0 {
		name, args = argv[0], argv[1:]
	}
	c := lookup(name)
	if c == nil {
		return nil, nil, nil, unknown(stderr, name)
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Fprint(stdout, c.usage())
			return nil, nil, nil, 0
		}
	}
	args, flags, bad := c.parseFlags(args)
	if bad != "" {
		fmt.Fprintf(stderr, "ember %s: %s (see ember help %s)\n", c.name, bad, c.name)
		return nil, nil, nil, 2
	}
	switch {
	case len(args) < c.min:
		fmt.Fprintf(stderr, "ember %s: needs %s (see ember help %s)\n", c.name, c.need, c.name)
		return nil, nil, nil, 2
	case len(args) > c.max:
		fmt.Fprintf(stderr, "ember %s: unexpected argument %q (see ember help %s)\n", c.name, args[c.max], c.name)
		return nil, nil, nil, 2
	}
	return c, args, flags, 0
}

// parseFlags takes the command's --flags out of args: "--name value",
// "--name=value", or a bare "--name" for a bool. An unknown flag, or one
// without its value, is an error; "--" ends the flags.
func (c *command) parseFlags(args []string) (pos []string, flags map[string]string, bad string) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return append(pos, args[i+1:]...), flags, ""
		}
		if !strings.HasPrefix(a, "--") || len(c.flags)+len(c.bools) == 0 {
			pos = append(pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(a[2:], "=")
		switch {
		case slices.Contains(c.bools, name) && !hasVal:
			flags[name] = "true"
		case slices.Contains(c.flags, name) && hasVal:
			flags[name] = val
		case slices.Contains(c.flags, name) && i+1 < len(args):
			i++
			flags[name] = args[i]
		case slices.Contains(c.flags, name):
			return nil, nil, fmt.Sprintf("--%s needs a value", name)
		default:
			return nil, nil, fmt.Sprintf("unexpected argument %q", a)
		}
	}
	return pos, flags, ""
}

func main() {
	c, args, flags, code := route(os.Args[1:], os.Stdout, os.Stderr)
	switch {
	case c != nil && c.runf != nil:
		code = c.runf(args, flags)
	case c != nil:
		code = c.run(args)
	}
	os.Exit(code)
}

func runIsland([]string) int {
	err := island.Run(versionString())
	if err != nil {
		fmt.Fprintln(os.Stderr, "ember:", err)
	}
	return runExit(err)
}

// exNoWayland (EX_CONFIG) is the unit's RestartPreventExitStatus: with no
// compositor, systemd stops instead of restarting the island forever.
const exNoWayland = 78

func runExit(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, wayland.ErrNoWayland):
		return exNoWayland
	}
	return 1
}

func runInstall([]string) int {
	exe, err := executable()
	if err == nil {
		err = install(exe)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ember:", err)
		return 1
	}
	return 0
}

func runUninstall(_ []string, f map[string]string) int {
	_, purge := f["purge"]
	if err := uninstall(purge); err != nil {
		fmt.Fprintln(os.Stderr, "ember:", err)
		return 1
	}
	return 0
}

func cmdOrFail(ok bool) int {
	if !ok {
		fmt.Fprintln(os.Stderr, "ember: the island isn't running")
		return 1
	}
	return 0
}
