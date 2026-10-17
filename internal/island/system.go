package island

import (
	"image"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/source"
	xdraw "golang.org/x/image/draw"
)

// System moments: battery, Bluetooth audio, volume, microphone, brightness,
// screenshots. The watchers are in package source; these turn what they
// say into moments.

func startSystem(emit source.Emit) {
	go source.Battery(emit)
	go source.Bluetooth(emit)
	go source.Levels(emit)
	go source.Backlight(emit)
	go source.Screenshots(emit)
	go source.Shell(emit)
	go source.ShellSettings(emit)
}

// ---- battery (UPower) ----

type power struct {
	pct      float64
	state    uint32 // 1 charging, 2 discharging, 4 full, 5 pending charge
	present  bool
	tte, ttf int64 // seconds to empty / to full, 0 if unknown
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

func (a *app) volumeChanged(l level) {
	if a.ui.drag == "sys:vol" {
		l.v = a.ui.vol.v // your hand wins over the echo of an older position
	}
	a.ui.vol = l
	if !a.cfg.OSD {
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

// ---- brightness and screenshots ----

func (a *app) brightnessChanged(v float64) {
	if a.ui.drag == "sys:light" {
		return
	}
	a.ui.light = v
	if !a.cfg.OSD {
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

// ---- controls (the System page) ----
