package source

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

// The backlight: inotify on its brightness file says when it changed; logind
// sets it.

func backlights() []string {
	ds, _ := filepath.Glob("/sys/class/backlight/*")
	return ds
}

func readInt(path string) int {
	b, _ := os.ReadFile(path)
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

// level is a backlight's brightness, 0..1.
func level(dir string) (float64, bool) {
	cur, top := readInt(filepath.Join(dir, "brightness")), readInt(filepath.Join(dir, "max_brightness"))
	if top <= 0 {
		return 0, false
	}
	return float64(cur) / float64(top), true
}

// Backlight follows the screen's brightness. Without a backlight (an
// external monitor) it returns at once.
func Backlight(emit Emit) error {
	paths := map[string]uint32{}
	for _, d := range backlights() {
		paths[filepath.Join(d, "brightness")] = unix.IN_MODIFY
	}
	ready := func() {
		for _, d := range backlights() { // the starting level, quietly
			if v, ok := level(d); ok {
				emit(Brightness{V: v, First: true})
				break
			}
		}
	}
	var mu sync.Mutex
	due := false
	err := inotify(paths, ready, func(path, _ string) {
		mu.Lock()
		defer mu.Unlock()
		if due { // several writes per key press: show the last
			return
		}
		due = true
		go func(dir string) {
			time.Sleep(30 * time.Millisecond)
			mu.Lock()
			due = false
			mu.Unlock()
			if v, ok := level(dir); ok {
				emit(Brightness{V: v})
			}
		}(filepath.Dir(path))
	})
	if err == errNothing {
		return errors.New("no backlight (an external monitor?)")
	}
	return err
}

// SetBrightness goes through logind, which lets the seat's user set it
// without root. Never fully dark: the screen must stay readable.
func SetBrightness(frac float64) {
	ds := backlights()
	if len(ds) == 0 {
		return
	}
	top := readInt(filepath.Join(ds[0], "max_brightness"))
	if top <= 0 {
		return
	}
	v := uint32(max(0.03, min(1, frac)) * float64(top))
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return
	}
	defer bus.Close()
	bus.Object("org.freedesktop.login1", "/org/freedesktop/login1/session/auto").
		Call("org.freedesktop.login1.Session.SetBrightness", 0, "backlight", filepath.Base(ds[0]), v)
}

// NudgeBrightness moves it 5% up (dir > 0) or down.
func NudgeBrightness(dir int) {
	ds := backlights()
	if len(ds) == 0 {
		return
	}
	if v, ok := level(ds[0]); ok {
		SetBrightness(v + float64(dir)*0.05)
	}
}
