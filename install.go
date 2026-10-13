package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const unit = `[Unit]
Description=Ember: a dynamic island for Claude Code, music and the clock
PartOf=graphical-session.target
After=graphical-session.target

[Service]
ExecStart=%s
Restart=always
RestartSec=3
Environment=PATH=%%h/.local/bin:/usr/local/bin:/usr/bin

[Install]
WantedBy=graphical-session.target
`

func install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	h, _ := os.UserHomeDir()
	settings := filepath.Join(h, ".claude", "settings.json")
	raw, err := os.ReadFile(settings)
	if err != nil {
		return err
	}
	if _, err := os.Stat(settings + ".pre-ember"); os.IsNotExist(err) {
		os.WriteFile(settings+".pre-ember", raw, 0o644)
	}
	out, err := patchSettings(raw, exe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(settings, out, 0o644); err != nil {
		return err
	}
	fmt.Println("hooks →", settings, "(backup: settings.json.pre-ember)")

	dir := filepath.Join(h, ".config", "systemd", "user")
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, "ember.service"), []byte(fmt.Sprintf(unit, exe)), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "ember.service"}, {"restart", "ember.service"}} {
		if o, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %v: %v: %s", args, err, o)
		}
	}
	fmt.Println("service → ember.service (running). Restart running Claude sessions to pick up the hooks.")
	return nil
}

// isEmber matches Ember's hook entries from any version (the Go binary, or
// the older Noctalia plugin's sh script).
func isEmber(entry any) bool {
	m, _ := entry.(map[string]any)
	hs, _ := m["hooks"].([]any)
	for _, h := range hs {
		hm, _ := h.(map[string]any)
		c, _ := hm["command"].(string)
		if strings.Contains(c, "ember hook ") || strings.Contains(c, "ember/hook'") {
			return true
		}
	}
	return false
}

// patchSettings replaces Ember's hooks in Claude Code's settings and leaves
// everything else as written.
func patchSettings(raw []byte, exe string) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	hooks, _ := cfg["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for ev, v := range hooks {
		list, _ := v.([]any)
		kept := []any{}
		for _, e := range list {
			if !isEmber(e) {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			delete(hooks, ev)
		} else {
			hooks[ev] = kept
		}
	}
	add := func(event, matcher, arg string, timeout int, async bool) {
		h := map[string]any{"type": "command", "command": exe + " hook " + arg, "timeout": timeout}
		if async {
			h["async"] = true
		}
		e := map[string]any{"hooks": []any{h}}
		if matcher != "" {
			e["matcher"] = matcher
		}
		list, _ := hooks[event].([]any)
		hooks[event] = append(list, e)
	}
	add("PermissionRequest", "*", "PermissionRequest", 100, false)
	add("PreToolUse", "AskUserQuestion", "AskUserQuestion", 100, false)
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "Notification", "Stop", "StopFailure", "SessionEnd",
		"SubagentStart", "SubagentStop", "PreCompact", "PostCompact"} {
		add(ev, "", ev, 5, true)
	}
	add("PreToolUse", "*", "PreToolUse", 5, true)
	add("PostToolUse", "*", "PostToolUse", 5, true)
	cfg["hooks"] = hooks
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
