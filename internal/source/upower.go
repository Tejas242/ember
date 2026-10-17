package source

import "github.com/godbus/dbus/v5"

func readPower(bus *dbus.Conn) (Power, bool) {
	obj := bus.Object("org.freedesktop.UPower", "/org/freedesktop/UPower/devices/DisplayDevice")
	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.freedesktop.UPower.Device").Store(&props); err != nil {
		return Power{}, false
	}
	p := powerOf(props)
	return p, p.Present
}

// powerOf reads UPower's device properties.
func powerOf(props map[string]dbus.Variant) Power {
	p := Power{}
	p.Pct, _ = props["Percentage"].Value().(float64)
	p.State, _ = props["State"].Value().(uint32)
	p.Present, _ = props["IsPresent"].Value().(bool)
	p.TTE, _ = props["TimeToEmpty"].Value().(int64)
	p.TTF, _ = props["TimeToFull"].Value().(int64)
	return p
}

// Battery follows UPower's display device. A desktop has none: it returns.
func Battery(emit Emit) {
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return
	}
	first, ok := readPower(bus)
	if !ok {
		bus.Close()
		return // a desktop: nothing to say
	}
	first.First = true
	emit(first)
	bus.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath("/org/freedesktop/UPower/devices/DisplayDevice"))
	sig := make(chan *dbus.Signal, 8)
	bus.Signal(sig)
	for range sig {
		if p, ok := readPower(bus); ok {
			emit(p)
		}
	}
}
