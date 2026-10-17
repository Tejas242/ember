package island

import "sort"

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
			u.closeDash()
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
		u.closeDash()
		return
	}
	u.openDash(name)
}

func (u *UI) openDash(name string) {
	if u.dashboards[name] == nil {
		return
	}
	u.dash, u.dashAt, u.dashIn = name, u.now(), u.hover
	u.pinned, u.suppress = false, false
	u.used(name)
}

func (u *UI) closeDash() {
	u.dash, u.dashIn = "", false
	u.pinned, u.suppress = false, u.hover // don't drop straight into the card under the pointer
}
