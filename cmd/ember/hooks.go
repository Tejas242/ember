package main

import (
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// hookSpec is one of Ember's entries in Claude Code's settings.
type hookSpec struct {
	event   string // the settings' hook event
	matcher string // the tool matcher, "" for none
	arg     string // what `ember hook` is told
	timeout time.Duration
	async   bool // Claude doesn't wait for it
}

// askTimeout: a request waits AskTimeout on the island; Claude must wait
// longer than that (the client gives up at AskTimeout+5s), or it kills the
// hook while the island still shows the card.
const askTimeout = ipc.AskTimeout + 10*time.Second

// emberHooks is every hook Ember installs, in the order it writes them.
// install, uninstall and doctor all read this table.
var emberHooks = []hookSpec{
	{event: "PermissionRequest", matcher: "*", arg: "PermissionRequest", timeout: askTimeout},
	{event: "PreToolUse", matcher: "AskUserQuestion", arg: "AskUserQuestion", timeout: askTimeout},
	{event: "SessionStart", arg: "SessionStart", timeout: 5 * time.Second, async: true},
	{event: "UserPromptSubmit", arg: "UserPromptSubmit", timeout: 5 * time.Second, async: true},
	{event: "Notification", arg: "Notification", timeout: 5 * time.Second, async: true},
	{event: "Stop", arg: "Stop", timeout: 5 * time.Second, async: true},
	{event: "StopFailure", arg: "StopFailure", timeout: 5 * time.Second, async: true},
	{event: "SessionEnd", arg: "SessionEnd", timeout: 5 * time.Second, async: true},
	{event: "SubagentStart", arg: "SubagentStart", timeout: 5 * time.Second, async: true},
	{event: "SubagentStop", arg: "SubagentStop", timeout: 5 * time.Second, async: true},
	{event: "PreCompact", arg: "PreCompact", timeout: 5 * time.Second, async: true},
	{event: "PostCompact", arg: "PostCompact", timeout: 5 * time.Second, async: true},
	{event: "PreToolUse", matcher: "*", arg: "PreToolUse", timeout: 5 * time.Second, async: true},
	{event: "PostToolUse", matcher: "*", arg: "PostToolUse", timeout: 5 * time.Second, async: true},
	{event: "PostToolUseFailure", matcher: "*", arg: "PostToolUseFailure", timeout: 5 * time.Second, async: true},
}
