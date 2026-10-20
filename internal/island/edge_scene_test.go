package island

import (
	proto "github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/edge/adapter"
)

// edgeSays shows edge on u the way edge's plugin and the host would: the
// adapter's line, row and dashboard, each through the protocol's decoder,
// with the grants ember install gives edge.
func edgeSays(u *UI, e *adapter.Status, st *adapter.Stats) {
	now := u.now()
	if u.cfg.Plugins == nil {
		u.cfg.Plugins = map[string]map[string]string{}
	}
	u.cfg.Plugins["edge"] = map[string]string{"grants": `["critical", "badge", "focus"]`}
	x := u.extOf("edge")
	decode := func(typ string, v any) proto.Message {
		b, err := proto.Line(typ, v)
		if err != nil {
			panic(err)
		}
		m, err := proto.Decode(b)
		if err != nil {
			panic(err)
		}
		return m
	}
	x.status = nil
	if l := adapter.Line(e, now); l != nil {
		x.status = decode("status", l).Status
	}
	if e == nil && st != nil {
		e = adapter.FromStats(st)
	}
	if e != nil {
		x.row = decode("row", adapter.Row(e, now)).Row
	}
	if st != nil {
		x.dash = decode("dashboard", adapter.Dashboard(st, now)).Dashboard
		u.dashboards["edge"] = extDash{name: "edge"}
	}
}
