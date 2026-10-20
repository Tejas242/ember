package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/config"
	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/plugin"
	"github.com/Tejas242/ember/internal/xdg"
)

// `ember plugins [name]`: each external plugin, its state, its health and
// its last error; with a name, everything about that one, its log too.

func pluginsDir() string { return filepath.Join(xdg.ConfigHome(), "ember", "plugins") }

func runPlugins(args []string) int {
	st, up := ipc.QueryStatus()
	ps := st.Extern
	if !up {
		ps = pluginsOnDisk()
	}
	if len(args) == 1 {
		for _, p := range ps {
			if p.Name == args[0] {
				pluginDetail(os.Stdout, p, up)
				return 0
			}
		}
		fmt.Fprintf(os.Stderr, "ember plugins: no plugin %q in %s\n", args[0], pluginsDir())
		return 1
	}
	listPlugins(os.Stdout, ps, up, time.Now())
	return 0
}

// pluginsOnDisk is what's installed, when the island isn't running to say more.
func pluginsOnDisk() []ipc.Plugin {
	var out []ipc.Plugin
	entries, _ := os.ReadDir(pluginsDir())
	for _, e := range entries {
		dir := filepath.Join(pluginsDir(), e.Name())
		if _, err := os.Stat(filepath.Join(dir, "plugin.toml")); !e.IsDir() || err != nil {
			continue
		}
		m, err := plugin.LoadManifest(dir)
		if err != nil {
			out = append(out, ipc.Plugin{Name: e.Name(), State: plugin.Broken, LastError: err.Error(), Dir: dir})
			continue
		}
		c, _ := config.Load(config.Path())
		out = append(out, ipc.Plugin{Name: m.Name, Description: m.Description, State: "installed", Caps: m.Caps, Asks: m.Asks,
			Grants: c.PluginGrants(m.Name), Dir: dir})
	}
	return out
}

// health is a plugin's state in a few words.
func health(p ipc.Plugin, now time.Time) string {
	s := p.State
	switch p.State {
	case plugin.Running:
		s += " for " + span(now.Sub(p.Since))
	case plugin.Waiting, plugin.Suspended, plugin.Broken:
		if p.LastError != "" {
			s += ": " + strings.TrimPrefix(p.LastError, p.State+": ")
		}
	}
	var n []string
	if p.Restarts > 0 {
		n = append(n, fmt.Sprintf("%d restarts", p.Restarts))
	}
	if p.Dropped > 0 {
		n = append(n, fmt.Sprintf("%d dropped", p.Dropped))
	}
	if p.Violations > 0 {
		n = append(n, fmt.Sprintf("%d refused", p.Violations))
	}
	if len(n) > 0 {
		s += " (" + strings.Join(n, ", ") + ")"
	}
	return s
}

// grantWords says what a plugin was granted, and what it asks for that it wasn't.
func grantWords(p ipc.Plugin) string {
	var asked []string
	for _, a := range p.Asks {
		if !slices.Contains(p.Grants, a) {
			asked = append(asked, a)
		}
	}
	var s []string
	if len(p.Grants) > 0 {
		s = append(s, "granted "+strings.Join(p.Grants, ", "))
	}
	if len(asked) > 0 {
		s = append(s, "asks for "+strings.Join(asked, ", ")+" ([plugins."+p.Name+"] grants in the config)")
	}
	return strings.Join(s, "; ")
}

func listPlugins(w io.Writer, ps []ipc.Plugin, up bool, now time.Time) {
	if !up {
		fmt.Fprintln(w, "The island isn't running: this is what's installed.")
	}
	if len(ps) == 0 {
		fmt.Fprintf(w, "No plugins in %s. See docs/PLUGINS.md to write one.\n", pluginsDir())
		return
	}
	for _, p := range ps {
		fmt.Fprintf(w, "%-16s %s\n", p.Name, health(p, now))
		if p.Description != "" {
			fmt.Fprintf(w, "%-16s %s\n", "", p.Description)
		}
		if g := grantWords(p); g != "" {
			fmt.Fprintf(w, "%-16s %s\n", "", g)
		}
	}
}

func pluginDetail(w io.Writer, p ipc.Plugin, up bool) {
	fmt.Fprintf(w, "%s: %s\n", p.Name, health(p, time.Now()))
	if p.Description != "" {
		fmt.Fprintf(w, "  %s\n", p.Description)
	}
	fmt.Fprintf(w, "  dir:  %s\n", p.Dir)
	if len(p.Caps) > 0 {
		fmt.Fprintf(w, "  caps: %s\n", strings.Join(p.Caps, ", "))
	}
	if g := grantWords(p); g != "" {
		fmt.Fprintf(w, "  %s\n", g)
	}
	if p.LastError != "" && !strings.Contains(health(p, time.Now()), p.LastError) {
		fmt.Fprintf(w, "  last error: %s\n", p.LastError)
	}
	if len(p.Log) > 0 {
		fmt.Fprintln(w, "  log:")
		for _, l := range p.Log {
			fmt.Fprintf(w, "    %s\n", l)
		}
	}
	if !up {
		fmt.Fprintln(w, "  (the island isn't running)")
	}
}

// span is a while, roughly: "a moment", "12m", "3h 05m", "2 days".
func span(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "a moment"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}
