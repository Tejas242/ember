package source

import (
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// Bluetooth follows BlueZ's devices connecting and disconnecting.
func Bluetooth(emit Emit) error {
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return fmt.Errorf("no system bus: %w", err)
	}
	defer bus.Close()
	bus.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchSender("org.bluez"), dbus.WithMatchPathNamespace("/org/bluez"), dbus.WithMatchArg(0, "org.bluez.Device1"))
	sig := make(chan *dbus.Signal, 8)
	bus.Signal(sig)
	for s := range sig {
		if len(s.Body) < 2 {
			continue
		}
		changed, _ := s.Body[1].(map[string]dbus.Variant)
		v, ok := changed["Connected"]
		if !ok {
			continue
		}
		on, _ := v.Value().(bool)
		dev := bus.Object("org.bluez", s.Path)
		str := func(prop string) string {
			v, err := dev.GetProperty("org.bluez.Device1." + prop)
			if err != nil {
				return ""
			}
			x, _ := v.Value().(string)
			return x
		}
		name, kind := str("Alias"), str("Icon")
		pct := -1.0
		if on { // the battery level shows up a moment after the link
			time.Sleep(1500 * time.Millisecond)
			if v, err := dev.GetProperty("org.bluez.Battery1.Percentage"); err == nil {
				if b, ok := v.Value().(byte); ok {
					pct = float64(b)
				}
			}
		}
		emit(Device{Name: name, Kind: kind, On: on, Pct: pct})
	}
	return errors.New("the system bus went away")
}
