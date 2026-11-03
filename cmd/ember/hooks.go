package main

import "time"

// hookSpec is one of Ember's entries in Claude Code's settings.
type hookSpec struct {
	event   string // the settings' hook event
	matcher string // the tool matcher, "" for none
	arg     string // what `ember hook` is told
	timeout time.Duration
	async   bool // Claude doesn't wait for it
}

// emberHooks is every hook Ember installs, in the order it writes them.
// install, uninstall and doctor all read this table. Every one is async:
// Ember only listens, and Claude never waits for it. PermissionRequest and
// AskUserQuestion are there so Ember learns the question's text; they give
// no decision, so the terminal asks at once.
var emberHooks = []hookSpec{
	{event: "PermissionRequest", matcher: "*", arg: "PermissionRequest", timeout: 5 * time.Second, async: true},
	{event: "PreToolUse", matcher: "AskUserQuestion", arg: "AskUserQuestion", timeout: 5 * time.Second, async: true},
	{event: "SessionStart", arg: "SessionStart", timeout: 5 * time.Second, async: true},
	{event: "UserPromptSubmit", arg: "UserPromptSubmit", timeout: 5 * time.Second, async: true},
	{event: "Notification", arg: "Notification", timeout: 5 * time.Second, async: true},
	{event: "Stop", arg: "Stop", timeout: 5 * time.Second, async: true},
	{event: "StopFailure", arg: "StopFailure", timeout: 5 * time.Second, async: true},
	{event: "SessionEnd", arg: "SessionEnd", timeout: 5 * time.Second, async: true},
	{event: "SubagentStart", arg: "SubagentStart", timeout: 5 * time.Second, async: true},
	{event: "SubagentStop", arg: "SubagentStop", timeout: 5 * time.Second, async: true},
	{event: "PostToolUse", matcher: "*", arg: "PostToolUse", timeout: 5 * time.Second, async: true},
}

// retiredHooks are the `ember hook <arg>` commands earlier versions wrote
// and this one doesn't. install and uninstall still recognise them as
// Ember's, so an upgrade takes them out of Claude Code's settings.
var retiredHooks = []string{"PreToolUse", "PreCompact", "PostCompact", "PostToolUseFailure"}
