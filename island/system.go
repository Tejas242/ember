package island

import (
	"bufio"
	"bytes"
	"image"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/godbus/dbus/v5"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/sys/unix"
)

// System moments: battery, Bluetooth audio, volume, microphone, brightness,
// screenshots. Each watcher sleeps until the system tells it something
// changed, then posts to the main loop. Nothing polls.

const osdHold = 1600 * time.Millisecond // [moments] osd_ms

func startSystem(posts chan<- func(*app)) {
	go watchBattery(posts)
	go watchBluetooth(posts)
	go watchVolume(posts)
	go watchFiles(posts)
	go watchShell(posts)
}

// watchShell notices the desktop shell (Noctalia) starting again: its new
// bar is a newer surface on our layer and would cover the island, so the
// island raises itself once the bar is up. Event-driven: the shell's D-Bus
// names change owner when it restarts.
func watchShell(posts chan<- func(*app)) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	bus.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg0Namespace("dev.noctalia"))
	sig := make(chan *dbus.Signal, 8)
	bus.Signal(sig)
	var last time.Time
	for s := range sig {
		if len(s.Body) < 3 {
			continue
		}
		if owner, _ := s.Body[2].(string); owner == "" || time.Since(last) < 5*time.Second {
			continue // a name going away, or the same restart's other names
		}
		last = time.Now()
		go func() {
			for _, d := range []time.Duration{1500 * time.Millisecond, 3 * time.Second} { // the bar maps a moment after the names
				time.Sleep(d)
				posts <- func(a *app) { a.raise("the shell restarted") }
			}
		}()
	}
}

// ---- battery (UPower) ----

type power struct {
	pct      float64
	state    uint32 // 1 charging, 2 discharging, 4 full, 5 pending charge
	present  bool
	tte, ttf int64 // seconds to empty / to full, 0 if unknown
}

func readPower(bus *dbus.Conn) (power, bool) {
	obj := bus.Object("org.freedesktop.UPower", "/org/freedesktop/UPower/devices/DisplayDevice")
	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.freedesktop.UPower.Device").Store(&props); err != nil {
		return power{}, false
	}
	p := power{}
	p.pct, _ = props["Percentage"].Value().(float64)
	p.state, _ = props["State"].Value().(uint32)
	p.present, _ = props["IsPresent"].Value().(bool)
	p.tte, _ = props["TimeToEmpty"].Value().(int64)
	p.ttf, _ = props["TimeToFull"].Value().(int64)
	return p, p.present
}

func watchBattery(posts chan<- func(*app)) {
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return
	}
	last, ok := readPower(bus)
	if !ok {
		bus.Close()
		return // a desktop: nothing to say
	}
	posts <- func(a *app) { a.ui.power = &last }
	bus.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath("/org/freedesktop/UPower/devices/DisplayDevice"))
	sig := make(chan *dbus.Signal, 8)
	bus.Signal(sig)
	for range sig {
		p, ok := readPower(bus)
		if !ok {
			continue
		}
		prev := last
		last = p
		posts <- func(a *app) { a.ui.power = &p; a.powerChanged(prev, p) }
	}
}

func charging(s uint32) bool { return s == 1 || s == 4 || s == 5 }

func (a *app) powerChanged(prev, p power) {
	if !a.cfg.Battery {
		return
	}
	now := a.ui.now()
	switch {
	case charging(p.state) && !charging(prev.state):
		// the bar has the percentage: say what changed, and when it'll be full
		text := map[uint32]string{1: "Charging", 4: "Fully charged", 5: "Plugged in"}[p.state]
		sub := ""
		if p.state == 1 && p.ttf > 0 {
			sub = strings.ToLower(powerText(&p)[:1]) + powerText(&p)[1:]
		}
		a.ui.Moment(moment{key: "power", icon: "bolt", tint: a.ui.pal.green, text: text, sub: sub,
			batt: true, pct: p.pct / 100, until: now.Add(3500 * time.Millisecond)})
	case !charging(p.state) && charging(prev.state):
		sub := ""
		if p.tte > 0 {
			sub = powerText(&p)
		}
		a.ui.Moment(moment{key: "power", icon: "unplug", tint: a.ui.pal.fg, text: "On battery", sub: sub,
			batt: true, pct: p.pct / 100, until: now.Add(2500 * time.Millisecond)})
	case p.state == 4 && prev.state == 1:
		a.ui.Moment(moment{key: "power", icon: "check", tint: a.ui.pal.green, text: "Fully charged",
			batt: true, pct: 1, until: now.Add(4 * time.Second)})
	case p.state == 2:
		for _, at := range []float64{20, 10, 5} {
			if prev.pct > at && p.pct <= at {
				tint := a.ui.pal.yellow
				if at <= 10 {
					tint = a.ui.pal.red
				}
				a.ui.Moment(moment{key: "power", icon: "warning", tint: tint, text: "Battery low", right: pctText(p.pct),
					batt: true, pct: p.pct / 100, mood: moodNudge, until: now.Add(8 * time.Second)})
				break
			}
		}
	}
}

func pctText(p float64) string { return strconv.Itoa(int(p+0.5)) + "%" }

// ---- Bluetooth audio (BlueZ) ----

func watchBluetooth(posts chan<- func(*app)) {
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return
	}
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
		posts <- func(a *app) { a.deviceChanged(name, kind, on, pct) }
	}
}

func (a *app) deviceChanged(name, kind string, on bool, pct float64) {
	if !a.cfg.Devices {
		return
	}
	icon := "bluetooth"
	if strings.HasPrefix(kind, "audio-head") || strings.HasPrefix(kind, "audio-card") {
		icon = "headphones"
	}
	m := moment{key: "device:" + name, icon: icon, tint: a.ui.pal.blue, text: name, until: a.ui.now().Add(3500 * time.Millisecond)}
	if on {
		m.sub = "Connected"
		if pct >= 0 {
			m.right, m.batt, m.pct = pctText(pct), true, pct/100
		}
	} else {
		m.sub, m.tint = "Disconnected", a.ui.pal.muted
		m.until = a.ui.now().Add(2500 * time.Millisecond)
	}
	a.ui.Moment(m)
}

// ---- volume and microphone (PipeWire / PulseAudio) ----

type level struct {
	v     float64
	muted bool
	ok    bool
}

var volRe = regexp.MustCompile(`Volume:\s*([0-9.]+)(\s*\[MUTED\])?`)

func readLevel(target string) level {
	out, err := exec.Command("wpctl", "get-volume", target).Output()
	if err != nil {
		return level{}
	}
	m := volRe.FindSubmatch(out)
	if m == nil {
		return level{}
	}
	v, _ := strconv.ParseFloat(string(m[1]), 64)
	return level{v: v, muted: len(m[2]) > 0, ok: true}
}

func watchVolume(posts chan<- func(*app)) {
	if _, err := exec.LookPath("pactl"); err != nil {
		return
	}
	if _, err := exec.LookPath("wpctl"); err != nil {
		return
	}
	for {
		cmd := exec.Command("pactl", "subscribe")
		out, err := cmd.StdoutPipe()
		if err != nil || cmd.Start() != nil {
			return
		}
		sink, src := readLevel("@DEFAULT_AUDIO_SINK@"), readLevel("@DEFAULT_AUDIO_SOURCE@")
		first := sink
		posts <- func(a *app) { a.ui.vol = first }
		events := make(chan struct{}, 1)
		go func() {
			sc := bufio.NewScanner(out)
			for sc.Scan() {
				ln := sc.Text()
				if strings.Contains(ln, "'change' on sink #") || strings.Contains(ln, "'change' on source #") || strings.Contains(ln, "on server") {
					select {
					case events <- struct{}{}:
					default:
					}
				}
			}
			close(events)
		}()
		for range events {
			time.Sleep(25 * time.Millisecond) // a key press sends a burst
			select {
			case <-events:
			default:
			}
			ns, nm := readLevel("@DEFAULT_AUDIO_SINK@"), readLevel("@DEFAULT_AUDIO_SOURCE@")
			if ns.ok && (ns.v != sink.v || ns.muted != sink.muted) {
				l := ns
				posts <- func(a *app) { a.volumeChanged(l) }
			}
			if nm.ok && src.ok && nm.muted != src.muted {
				muted := nm.muted
				posts <- func(a *app) { a.micChanged(muted) }
			}
			sink, src = ns, nm
		}
		cmd.Wait()
		time.Sleep(3 * time.Second) // the sound server restarted
	}
}

func (a *app) volumeChanged(l level) {
	if a.ui.drag == "sys:vol" {
		l.v = a.ui.vol.v // your hand wins over the echo of an older position
	}
	a.ui.vol = l
	if !a.cfg.OSD || a.ui.onSystemPage() {
		return
	}
	icon, tint := "volume", a.ui.pal.fg
	switch {
	case l.muted || l.v == 0:
		icon, tint = "mute", a.ui.pal.muted
	case l.v < 0.4:
		icon = "volume-low"
	}
	m := moment{key: "osd:volume", icon: icon, tint: tint, meter: true, level: min(l.v, 1), right: strconv.Itoa(int(l.v*100 + 0.5)),
		until: a.ui.now().Add(a.cfg.OSDFor), osd: true}
	if l.muted {
		m.right = "Muted"
	}
	a.ui.Moment(m)
}

func (a *app) micChanged(muted bool) {
	if !a.cfg.OSD {
		return
	}
	m := moment{key: "osd:mic", icon: "mic", tint: a.ui.pal.green, text: "Microphone on", until: a.ui.now().Add(a.cfg.OSDFor), osd: true}
	if muted {
		m.icon, m.tint, m.text = "mic-off", a.ui.pal.red, "Microphone muted"
	}
	a.ui.Moment(m)
}

// ---- brightness and screenshots (inotify) ----

func backlights() []string {
	ds, _ := filepath.Glob("/sys/class/backlight/*")
	return ds
}

func readInt(path string) int {
	b, _ := os.ReadFile(path)
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

// screenshotDirs is where screenshots land: Noctalia's setting, else the usual place.
func screenshotDirs() []string {
	h, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(h, "Pictures", "Screenshots")}
	if out, err := exec.Command("noctalia", "config", "export").Output(); err == nil {
		inShot := false
		for _, ln := range strings.Split(string(out), "\n") {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "[") {
				inShot = strings.HasSuffix(ln, "screenshot]")
				continue
			}
			if k, v, ok := strings.Cut(ln, "="); ok && inShot && strings.TrimSpace(k) == "directory" {
				d := strings.Trim(strings.TrimSpace(v), `"`)
				if strings.HasPrefix(d, "~/") {
					d = filepath.Join(h, d[2:])
				}
				dirs = append([]string{d}, dirs...)
			}
		}
	}
	return dirs
}

func watchFiles(posts chan<- func(*app)) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return
	}
	type watch struct {
		dir   string
		light bool
		shell bool // Noctalia's settings and config: a change can rebuild the bar
	}
	wds := map[int32]watch{}
	for _, d := range backlights() {
		if wd, err := unix.InotifyAddWatch(fd, filepath.Join(d, "brightness"), unix.IN_MODIFY); err == nil {
			wds[int32(wd)] = watch{dir: d, light: true}
		}
	}
	seen := map[string]bool{}
	for _, d := range screenshotDirs() {
		if seen[d] {
			continue
		}
		seen[d] = true
		if wd, err := unix.InotifyAddWatch(fd, d, unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO); err == nil {
			wds[int32(wd)] = watch{dir: d}
		}
	}
	h, _ := os.UserHomeDir()
	for _, d := range []string{filepath.Join(h, ".config", "noctalia"), filepath.Join(h, ".local", "state", "noctalia")} {
		if wd, err := unix.InotifyAddWatch(fd, d, unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO); err == nil {
			wds[int32(wd)] = watch{dir: d, shell: true}
		}
	}
	if len(wds) == 0 {
		unix.Close(fd)
		return
	}
	var raiseAt time.Time
	for _, d := range backlights() { // the starting level, quietly
		if cur, top := readInt(filepath.Join(d, "brightness")), readInt(filepath.Join(d, "max_brightness")); top > 0 {
			v := float64(cur) / float64(top)
			posts <- func(a *app) { a.ui.light = v }
			break
		}
	}
	var mu sync.Mutex
	lightDue := false
	buf := make([]byte, 64<<10)
	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			log.Println("inotify:", err)
			return
		}
		for off := 0; off+unix.SizeofInotifyEvent <= n; {
			ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
			name := ""
			if ev.Len > 0 {
				raw := buf[off+unix.SizeofInotifyEvent : off+unix.SizeofInotifyEvent+int(ev.Len)]
				name = string(bytes.TrimRight(raw, "\x00"))
			}
			off += unix.SizeofInotifyEvent + int(ev.Len)
			w, ok := wds[ev.Wd]
			if !ok {
				continue
			}
			if w.shell {
				// the settings window or a config edit: the bar may be rebuilt on top of us
				if (name == "settings.toml" || strings.HasSuffix(name, ".toml") && strings.Contains(w.dir, ".config")) && time.Since(raiseAt) > 2*time.Second {
					raiseAt = time.Now()
					go func() {
						for _, d := range []time.Duration{700 * time.Millisecond, 2 * time.Second} { // the bar rebuilds a moment later
							time.Sleep(d)
							posts <- func(a *app) { a.raise("the shell's settings changed") }
						}
					}()
				}
				continue
			}
			if w.light {
				mu.Lock()
				if !lightDue { // several writes per key press: show the last
					lightDue = true
					go func(dir string) {
						time.Sleep(30 * time.Millisecond)
						mu.Lock()
						lightDue = false
						mu.Unlock()
						cur, top := readInt(filepath.Join(dir, "brightness")), readInt(filepath.Join(dir, "max_brightness"))
						if top > 0 {
							v := float64(cur) / float64(top)
							posts <- func(a *app) { a.brightnessChanged(v) }
						}
					}(w.dir)
				}
				mu.Unlock()
				continue
			}
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
				go screenshotTaken(posts, filepath.Join(w.dir, name))
			}
		}
	}
}

func (a *app) brightnessChanged(v float64) {
	if a.ui.drag == "sys:light" {
		return
	}
	a.ui.light = v
	if !a.cfg.OSD || a.ui.onSystemPage() {
		return
	}
	a.ui.Moment(moment{key: "osd:brightness", icon: "sun", tint: a.ui.pal.yellow, meter: true, level: v,
		right: strconv.Itoa(int(v*100 + 0.5)), until: a.ui.now().Add(a.cfg.OSDFor), osd: true})
}

// screenshotTaken loads a small preview and shows it.
func screenshotTaken(posts chan<- func(*app), path string) {
	var thumb *image.RGBA
	if f, err := os.Open(path); err == nil {
		if src, _, err := image.Decode(f); err == nil {
			thumb = scaleThumb(src, 30, 20)
		}
		f.Close()
		debug.FreeOSMemory() // a decoded screenshot is several MB; give it back
	}
	posts <- func(a *app) { a.screenshot(path, thumb) }
}

func (a *app) screenshot(path string, thumb *image.RGBA) {
	if !a.cfg.Screenshots {
		return
	}
	key := "shot:" + path
	if m := a.ui.moment; m != nil && m.key == key {
		return // niri and the folder both told us
	}
	m := moment{key: key, icon: "camera", tint: a.ui.pal.fg, text: "Screenshot saved", thumb: thumb,
		open: path, until: a.ui.now().Add(5 * time.Second)}
	if path == "" {
		m.text, m.sub, m.open = "Screenshot copied", "", ""
	}
	a.ui.Moment(m)
}

// scaleThumb fits src in w×h (keeping its shape) with rounded corners baked in.
func scaleThumb(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil
	}
	tw, th := w, int(float64(w)*float64(b.Dy())/float64(b.Dx())+0.5)
	if th > h {
		tw, th = int(float64(h)*float64(b.Dx())/float64(b.Dy())+0.5), h
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(tw, 1), max(th, 1)))
	xdraw.ApproxBiLinear.Scale(dst, dst.Rect, src, b, xdraw.Src, nil)
	m := newCanvas(dst.Rect.Dx(), dst.Rect.Dy())
	m.roundRect(0, 0, float32(dst.Rect.Dx()), float32(dst.Rect.Dy()), 3.5, colorWhite)
	for i := 0; i < len(dst.Pix); i += 4 { // premultiply by the rounded mask
		k := uint32(m.img.Pix[i+3])
		for c := 0; c < 4; c++ {
			dst.Pix[i+c] = uint8(uint32(dst.Pix[i+c]) * k / 255)
		}
	}
	return dst
}

// onSystemPage: nothing in the card shows levels any more; the pill says it.
func (u *UI) onSystemPage() bool { return false }

// ---- controls (the System page) ----

func setVolume(frac float64) {
	exec.Command("wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", strconv.FormatFloat(max(0, min(1, frac)), 'f', 3, 64)).Run()
}

func nudgeVolume(dir int) {
	step := "5%+"
	if dir < 0 {
		step = "5%-"
	}
	exec.Command("wpctl", "set-volume", "-l", "1", "@DEFAULT_AUDIO_SINK@", step).Run()
}

func toggleMute() { exec.Command("wpctl", "set-mute", "@DEFAULT_AUDIO_SINK@", "toggle").Run() }

// setBrightness goes through logind, which lets the seat's user set it
// without root. Never fully dark: the screen must stay readable.
func setBrightness(frac float64) {
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

func nudgeBrightness(dir int) {
	ds := backlights()
	if len(ds) == 0 {
		return
	}
	cur, top := readInt(filepath.Join(ds[0], "brightness")), readInt(filepath.Join(ds[0], "max_brightness"))
	if top > 0 {
		setBrightness(float64(cur)/float64(top) + float64(dir)*0.05)
	}
}
