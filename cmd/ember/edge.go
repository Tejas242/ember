package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Tejas242/ember/internal/config"
	"github.com/Tejas242/ember/internal/xdg"
	"github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/edge/adapter"
	"github.com/Tejas242/ember/plugins/vibe/vibe"
)

// edge's and vibe's plugins ship inside Ember's binary (`ember plugin
// edge`, `ember plugin vibe`), so one static binary is still all there is
// to install.

// builtins are the plugins this binary can be.
var builtins = map[string]func(*plugin.Plugin) error{
	"edge": func(p *plugin.Plugin) error { return adapter.Run(p, adapter.Options{}) },
	"vibe": func(p *plugin.Plugin) error { return vibe.Run(p, vibe.Options{}) },
}

func runPlugin(args []string) int {
	run := builtins[args[0]]
	if run == nil {
		fmt.Fprintf(os.Stderr, "ember plugin: no plugin %q in this binary\n", args[0])
		return 2
	}
	if err := plugin.Run(args[0], run); err != nil {
		fmt.Fprintln(os.Stderr, "ember plugin:", err)
		return 1
	}
	return 0
}

// edgeInstalled: edge's command is on PATH, or its daemon keeps stats.
func edgeInstalled() bool {
	if _, err := exec.LookPath("edge"); err == nil {
		return true
	}
	_, err := os.Stat(adapter.StatsPath())
	return err == nil
}

// edgeManifest is edge's plugin.toml, running this binary.
func edgeManifest(exe string) string {
	return fmt.Sprintf(`# edge, the practice coach, on Ember's island: written by ember install.
name         = "edge"
description  = "Practice: the day's problem, contest rounds, and the record"
command      = %q
args         = ["plugin", "edge"]
icon         = "chart"
capabilities = ["pill", "moment", "row", "card", "dashboard", "cheer"]
grants       = ["critical", "badge", "focus"]
`, exe)
}

// installEdge sets up edge's plugin when edge is installed: its manifest,
// and the grants that keep edge as it was (a live round outranks Claude,
// the padlock on the clock, Ember's game face), unless the config already
// says something about them.
func installEdge(exe string) error {
	if !edgeInstalled() {
		return nil
	}
	dir := filepath.Join(pluginsDir(), "edge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := xdg.AtomicWrite(filepath.Join(dir, "plugin.toml"), []byte(edgeManifest(exe)), 0o644); err != nil {
		return err
	}
	fmt.Println("edge →", filepath.Join(dir, "plugin.toml"))
	return grantOnce("edge", `["critical", "badge", "focus"]`,
		"edge keeps what it could do before plugins: a live round outranks Claude, the\n# padlock on the clock, Ember's game face. Take a grant out to take it back.")
}

// grantOnce writes a plugin's grants into the config, with a comment
// saying why, unless the config already says something about them.
func grantOnce(name, grants, why string) error {
	cfg := config.Path()
	c, _ := config.Load(cfg)
	if _, ok := c.Plugins[name]["grants"]; ok {
		return nil
	}
	b, _ := os.ReadFile(cfg)
	text := string(b)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n# " + why + "\n[plugins." + name + "]\ngrants = " + grants + "\n"
	if err := xdg.AtomicWrite(cfg, []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Println(name+"'s grants →", cfg)
	return nil
}
