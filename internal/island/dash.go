package island

import (
	"sort"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
)

// Dashboards: a plugin's full page. It opens on demand (`ember dash`, or a
// click on the plugin's row in the card) and closes the same way, on a click
// on its background, a right-click, or when the pointer, having been on it,
// leaves. A request from Claude still comes first: its card shows over the
// dashboard, which is there again once the request is answered.

// toggleDash opens name's dashboard, or closes the one that's open. An empty
// name means the first dashboard there is.
func (u *UI) toggleDash(name string) {
	if name == "" {
		if u.dash != "" {
			u.closeDash(proto.CloseUser)
			return
		}
		var names []string
		for n := range u.dashboards {
			names = append(names, n)
		}
		sort.Strings(names)
		if len(names) == 0 {
			return
		}
		name = names[0]
	}
	if u.dash == name {
		u.closeDash(proto.CloseUser)
		return
	}
	u.openDash(name)
}

func (u *UI) openDash(name string) {
	if u.dashboards[name] == nil {
		return
	}
	if u.dash != "" && u.dash != name {
		u.dashWhy = proto.CloseOther // the page it replaces hears why it closed
	}
	u.dash, u.dashAt, u.dashIn = name, u.now(), u.hover
	u.pinned, u.suppress = false, false
	u.idleSince, u.finishedSong = time.Time{}, ""
	u.used(name)
}

// closeDash closes the open page, and says why (proto.CloseUser, …): the
// plugin hears it.
func (u *UI) closeDash(why string) {
	if u.dash != "" {
		u.dashWhy = why
	}
	u.dash, u.dashIn = "", false
	u.pinned, u.suppress = false, u.hover // don't drop straight into the card under the pointer
}
