package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/xdg"
	"github.com/Tejas242/ember/island"
)

// unitText is the systemd user unit that runs exe. A crash restarts it, but
// five starts within a minute stop trying, and exit 78 (no Wayland) never
// restarts.
func unitText(exe string) string {
	return `[Unit]
Description=Ember: a dynamic island for Claude Code, music and the clock
PartOf=graphical-session.target
After=graphical-session.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
ExecStart=` + exe + `
Restart=always
RestartSec=3
RestartPreventExitStatus=` + strconv.Itoa(exNoWayland) + `
Environment=PATH=%h/.local/bin:/usr/local/bin:/usr/bin
Environment=GOGC=50 GOMEMLIMIT=16MiB

[Install]
WantedBy=graphical-session.target
`
}

// transient says why exe won't last, or "": a binary under the temp dir, in
// Go's build cache or left by `go run` is gone by the next boot (or the next
// `go clean`), and a service and hooks pointing at it would break.
func transient(exe string) string {
	exe = filepath.Clean(exe)
	under := func(dir string) bool {
		rel, err := filepath.Rel(filepath.Clean(dir), exe)
		return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
	}
	for _, el := range strings.Split(exe, string(filepath.Separator)) {
		if strings.HasPrefix(el, "go-build") {
			return "it was built by go run or into Go's build cache"
		}
	}
	for _, d := range []string{os.TempDir(), "/tmp", "/var/tmp", "/dev/shm"} {
		if under(d) {
			return "it lives under " + d
		}
	}
	return ""
}

// executable is this binary's real path.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}

// systemctl runs `systemctl --user`; tests replace it.
var systemctl = func(args ...string) ([]byte, error) {
	return exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
}

// unitPath is where systemd looks for the user's own units.
func unitPath() string {
	return filepath.Join(xdg.ConfigHome(), "systemd", "user", "ember.service")
}

func settingsPath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".claude", "settings.json")
}

func install(exe string) error {
	if why := transient(exe); why != "" {
		return fmt.Errorf("won't install %s: %s, so it won't last.\n"+
			"Install a lasting copy instead: make install (it puts ember in ~/.local/bin and runs ember install from there)", exe, why)
	}
	if err := installHooks(settingsPath(), exe); err != nil {
		return err
	}

	if cfg := island.ConfigPath(); !fileExists(cfg) {
		if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
			return err
		}
		if err := xdg.AtomicWrite(cfg, []byte(island.DefaultConfigText), 0o644); err != nil {
			return err
		}
		fmt.Println("config →", cfg)
	} else {
		fmt.Println("config kept:", cfg, "(ember doctor lists any new settings)")
	}

	u := unitPath()
	if err := os.MkdirAll(filepath.Dir(u), 0o755); err != nil {
		return err
	}
	if err := xdg.AtomicWrite(u, []byte(unitText(exe)), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "ember.service"}, {"restart", "ember.service"}} {
		if o, err := systemctl(args...); err != nil {
			return fmt.Errorf("systemctl %v: %v: %s", args, err, o)
		}
	}
	fmt.Println("service → ember.service (running). Restart running Claude sessions to pick up the hooks.")
	return nil
}

// installHooks puts Ember's hooks in Claude Code's settings, creating the
// file if Claude hasn't yet, and backs the first version it saw up once.
// Without a ~/.claude at all there's no Claude Code to hook: it says so and
// skips them.
func installHooks(settings, exe string) error {
	if _, err := os.Stat(filepath.Dir(settings)); os.IsNotExist(err) {
		fmt.Println("hooks skipped: no", filepath.Dir(settings), "(no Claude Code yet). Run ember install again once it's set up.")
		return nil
	}
	raw, err := os.ReadFile(settings)
	switch {
	case os.IsNotExist(err):
		raw = []byte("{}")
	case err != nil:
		return err
	default:
		if !fileExists(settings + ".pre-ember") {
			if err := xdg.AtomicWrite(settings+".pre-ember", raw, 0o600); err != nil {
				return err
			}
		}
	}
	out, err := patchSettings(raw, exe, true)
	if err != nil {
		return fmt.Errorf("%s: %w (left as it was)", settings, err)
	}
	if err := xdg.AtomicWrite(settings, out, 0o644); err != nil {
		return err
	}
	fmt.Println("hooks →", settings, "(backup: settings.json.pre-ember)")
	return nil
}

// uninstall removes Ember's hooks and its service; the config stays.
func uninstall() error {
	settings := settingsPath()
	if raw, err := os.ReadFile(settings); err == nil {
		out, err := patchSettings(raw, "", false)
		if err != nil {
			return fmt.Errorf("%s: %w (left as it was)", settings, err)
		}
		if err := xdg.AtomicWrite(settings, out, 0o644); err != nil {
			return err
		}
		fmt.Println("hooks removed from", settings)
	}
	systemctl("disable", "--now", "ember.service")
	os.Remove(unitPath())
	systemctl("daemon-reload")
	fmt.Println("service stopped and removed. Your config stays at", island.ConfigPath())
	return nil
}

// emberHook says whether a hook command is one of Ember's, from any version
// and whatever its binary is called (ember, ember-pd, a temp build), and which
// binary it runs: "<bin> hook <Event>" with an event Ember installs and a
// binary whose name starts with "ember", or the old Noctalia plugin's sh
// script (bin ""). Other tools' hooks (`edge hook`) are never Ember's.
func emberHook(cmd string) (bin string, ok bool) {
	cmd = strings.TrimSpace(cmd)
	if strings.Contains(cmd, "ember/hook'") {
		return "", true
	}
	i := strings.LastIndex(cmd, " hook ")
	if i < 0 || !knownEvent(cmd[i+len(" hook "):]) {
		return "", false
	}
	bin = strings.TrimSpace(cmd[:i])
	if len(bin) >= 2 && (bin[0] == '\'' || bin[0] == '"') && bin[len(bin)-1] == bin[0] {
		bin = bin[1 : len(bin)-1]
	}
	if bin == "" || !strings.HasPrefix(filepath.Base(bin), "ember") {
		return "", false
	}
	return bin, true
}

func knownEvent(ev string) bool {
	for _, h := range emberHooks {
		if ev == h.arg || ev == h.event {
			return true
		}
	}
	return false
}

// withoutEmber is a settings hook entry with Ember's hooks taken out, or nil
// when nothing else was in it.
func withoutEmber(entry any) any {
	m, ok := entry.(map[string]any)
	if !ok {
		return entry
	}
	hs, ok := m["hooks"].([]any)
	if !ok {
		return entry
	}
	kept := []any{}
	for _, h := range hs {
		hm, _ := h.(map[string]any)
		c, _ := hm["command"].(string)
		if _, ok := emberHook(c); !ok {
			kept = append(kept, h)
		}
	}
	switch {
	case len(kept) == len(hs):
		return entry
	case len(kept) == 0:
		return nil
	}
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	out["hooks"] = kept
	return out
}

// patchSettings replaces Ember's hooks in Claude Code's settings and leaves
// everything else as written.
func patchSettings(raw []byte, exe string, add bool) ([]byte, error) {
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
			if e = withoutEmber(e); e != nil {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			delete(hooks, ev)
		} else {
			hooks[ev] = kept
		}
	}
	for _, hs := range emberHooks {
		if !add {
			break
		}
		h := map[string]any{"type": "command", "command": exe + " hook " + hs.arg, "timeout": int(hs.timeout / time.Second)}
		if hs.async {
			h["async"] = true
		}
		e := map[string]any{"hooks": []any{h}}
		if hs.matcher != "" {
			e["matcher"] = hs.matcher
		}
		list, _ := hooks[hs.event].([]any)
		hooks[hs.event] = append(list, e)
	}
	cfg["hooks"] = hooks
	if len(hooks) == 0 {
		delete(cfg, "hooks")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
