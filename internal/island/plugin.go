package island

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/Tejas242/ember/internal/plugin"
	"github.com/Tejas242/ember/internal/xdg"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// Plugins are programs that speak the plugin protocol (docs/PLUGINS.md):
// edge's among them, built into Ember's binary. The host (package
// internal/plugin) runs them; ext.go shows what they send. A plugin may own
// a dashboard: a full page the island opens on demand (`ember dash <name>`).

// Dashboard is a plugin's page. Draw lays it out at x on the island's
// content canvas; age is how long it has been open, for its entrance. It
// reports whether it is still animating.
type Dashboard interface {
	Size(u *UI) (w, h float64)
	Draw(u *UI, x, w float32, age time.Duration) (animating bool)
}

// extHost is what the island asks of the plugin host (package
// internal/plugin's Host; a test's own).
type extHost interface {
	Send(name string, ev proto.Event) bool
	CardClosed(name, id string)
	Click(name, target string)
	Has(name string) bool
	Opener(context string) (name, hint string)
	Info() []plugin.Info
	Scan()
	Stop()
}

var _ extHost = (*plugin.Host)(nil)

// redetectEvery: how often the host looks for new plugins and manifests
// that changed, so installing one needs no restart.
const redetectEvery = time.Minute

// reconcile matches the running plugins to their manifests and the config.
// It runs at start, on config changes and now and then.
func (a *app) reconcile() {
	if a.host != nil {
		a.host.Scan()
	}
	a.detectedAt = a.ui.now()
}

// maxToolPayload: a tool event's line must stay within the protocol's 64 KiB.
const maxToolPayload = 60 << 10

// toPlugin hands a message from a tool to its plugin as a tool event, and
// reports whether the plugin's running process took it. A payload over the
// limit is said once in the log, and not sent.
func (a *app) toPlugin(name string, payload json.RawMessage) bool {
	a.quiet = true
	if a.host == nil || !a.host.Has(name) {
		return false
	}
	if len(payload) > maxToolPayload {
		logOnce("toolsize:"+name, "ember: a %d-byte message for the %s plugin is over %d bytes, so it isn't sent (this is said once)", len(payload), name, maxToolPayload)
		return false
	}
	return a.host.Send(name, proto.Event{Type: "tool", Payload: payload})
}

// startPlugins runs the plugins in ~/.config/ember/plugins until ctx ends
// (stop ends it).
func (a *app) startPlugins(ctx context.Context, stop func()) {
	h := plugin.NewHost(filepath.Join(xdg.ConfigHome(), "ember", "plugins"), a.version, extSink{posts: a.posts, ctx: ctx})
	h.Enabled = func(name string) bool { return a.ui.cfg.PluginOn(name, true) } // Scan runs on the loop
	h.Grants = func(name string) []string { return a.ui.cfg.PluginGrants(name) }
	a.host, a.stopAll = h, stop
	a.ui.openers = h.Opener
	h.Start(ctx)
}
