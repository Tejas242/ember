package island

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"time"

	"github.com/Tejas242/ember/internal/plugin"
	"github.com/Tejas242/ember/internal/xdg"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// Plugins are the parts of Ember that belong to a tool not everyone has
// (edge, today). Each is compiled in but dormant until its tool turns up, or
// until the config says otherwise ([plugins.<name>] enabled = auto | true |
// false): a dormant plugin watches nothing and shows nothing. A plugin talks
// to its tool however suits the tool. Tools reach theirs over Ember's socket
// as {"type":"plugin","name":"<name>","payload":…}, and a plugin may own a
// dashboard: a full page the island opens on demand (`ember dash <name>`).

type Plugin interface {
	Name() string
	Detect() bool // is its tool installed?
	Start(a *app) // it's on: begin watching
	Stop(a *app)  // it's off: take back everything it showed
	Message(a *app, payload json.RawMessage)
	Dash() Dashboard // nil if it has none
}

// Dashboard is a plugin's page. Draw lays it out at x on the island's
// content canvas; age is how long it has been open, for its entrance. It
// reports whether it is still animating.
type Dashboard interface {
	Size(u *UI) (w, h float64)
	Draw(u *UI, x, w float32, age time.Duration) (animating bool)
}

// plugins is every plugin Ember knows, in the order they're checked.
func plugins() []Plugin { return []Plugin{&edgePlugin{}} }

// redetectEvery: how often a dormant plugin looks for its tool again, so
// installing edge needs no restart.
const redetectEvery = time.Minute

// reconcile starts and stops plugins to match the config and what's
// installed. It runs at start, on config changes and now and then.
func (a *app) reconcile() {
	if a.plugins == nil {
		a.plugins = map[string]Plugin{}
		a.allPlugins = plugins()
	}
	for _, p := range a.allPlugins {
		name := p.Name()
		_, running := a.plugins[name]
		on := a.cfg.PluginOn(name, p.Detect())
		switch {
		case on && !running:
			log.Printf("plugin %s: on", name)
			a.plugins[name] = p
			p.Start(a)
			if d := p.Dash(); d != nil {
				a.ui.dashboards[name] = d
			}
		case !on && running:
			log.Printf("plugin %s: off", name)
			p.Stop(a)
			delete(a.plugins, name)
			delete(a.ui.dashboards, name)
			if a.ui.dash == name {
				a.ui.closeDash()
			}
		}
	}
	if a.host != nil {
		a.host.Scan()
	}
	a.detectedAt = a.ui.now()
}

// toPlugin hands a message from a tool to its plugin, if that plugin runs:
// an external one gets it as a tool event.
func (a *app) toPlugin(name string, payload json.RawMessage) {
	if a.host != nil && a.host.Has(name) {
		if len(payload) <= 60<<10 {
			a.host.Send(name, proto.Event{Type: "tool", Payload: payload})
		}
		a.quiet = true
		return
	}
	if p := a.plugins[name]; p != nil {
		p.Message(a, payload)
	}
}

// startPlugins runs the external plugins in ~/.config/ember/plugins until
// ctx ends (stop ends it).
func (a *app) startPlugins(ctx context.Context, stop func()) {
	a.host = plugin.NewHost(filepath.Join(xdg.ConfigHome(), "ember", "plugins"), a.version, extSink{posts: a.posts, ctx: ctx})
	a.host.Enabled = func(name string) bool { return a.cfg.PluginOn(name, true) } // Scan runs on the loop
	a.host.Reserved = func(name string) bool {
		for _, p := range plugins() {
			if p.Name() == name {
				return true
			}
		}
		return false
	}
	a.stopAll = stop
	a.host.Start(ctx)
}
