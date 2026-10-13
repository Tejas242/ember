package island

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// EMBER_PREVIEW=dir go test -run PreviewMoments ./island: the system moments, stacked.
func TestPreviewMoments(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	clock := time.Date(2026, 10, 11, 21, 30, 0, 0, time.Local)
	shot := image.NewRGBA(image.Rect(0, 0, 1366, 768))
	for y := 0; y < 768; y++ {
		for x := 0; x < 1366; x++ {
			shot.Set(x, y, color.NRGBA{uint8(30 + x/12), uint8(50 + y/8), 90, 255})
		}
	}
	type state struct {
		name string
		set  func(u *UI)
	}
	art := func(u *UI) {
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				img.Set(x, y, color.NRGBA{uint8(120 + x*2), uint8(40 + y), uint8(160 - x), 255})
			}
		}
		u.art = &albumArt{small: scaleSquare(img, 18), large: scaleSquare(img, 46)}
		u.art.accent = accentOf(u.art.large)
	}
	states := []state{
		{"clock", func(u *UI) {}},
		{"claude", func(u *UI) { working(u) }},
		{"claude-music", func(u *UI) {
			working(u)
			u.track = &Track{Player: "x", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: 5 * time.Minute, PosAt: clock}
			art(u)
		}},
		{"waiting", func(u *UI) {
			u.sessions["b"] = &Session{ID: "b", Project: "kinvault", State: "wait"}
		}},
		{"done", func(u *UI) {
			u.Moment(moment{key: "done", act: "claude", mood: moodHappy, text: "Done · kinvault", sub: "All 42 tests pass", until: clock.Add(time.Hour)})
		}},
		{"music", func(u *UI) {
			u.track = &Track{Player: "x", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: 5 * time.Minute, Position: 2 * time.Minute, PosAt: clock}
			art(u)
		}},
		{"volume", func(u *UI) {
			u.Moment(moment{key: "osd:volume", icon: "volume", tint: u.pal.fg, meter: true, level: 0.64, right: "64", osd: true, until: clock.Add(time.Hour)})
		}},
		{"muted", func(u *UI) {
			u.Moment(moment{key: "osd:volume", icon: "mute", tint: u.pal.muted, meter: true, level: 0.4, right: "Muted", osd: true, until: clock.Add(time.Hour)})
		}},
		{"brightness", func(u *UI) {
			u.Moment(moment{key: "osd:brightness", icon: "sun", tint: u.pal.yellow, meter: true, level: 0.3, right: "30", osd: true, until: clock.Add(time.Hour)})
		}},
		{"mic", func(u *UI) {
			u.Moment(moment{key: "osd:mic", icon: "mic-off", tint: u.pal.red, text: "Microphone muted", osd: true, until: clock.Add(time.Hour)})
		}},
		{"charging", func(u *UI) {
			u.Moment(moment{key: "power", icon: "bolt", tint: u.pal.green, text: "Charging", right: "73%", batt: true, pct: 0.73, until: clock.Add(time.Hour)})
		}},
		{"low", func(u *UI) {
			u.Moment(moment{key: "power", icon: "battery", tint: u.pal.red, text: "Battery low", right: "9%", batt: true, pct: 0.09, until: clock.Add(time.Hour)})
		}},
		{"headphones", func(u *UI) {
			u.Moment(moment{key: "device", icon: "headphones", tint: u.pal.blue, text: "WH-1000XM4", sub: "Connected", right: "80%", batt: true, pct: 0.8, until: clock.Add(time.Hour)})
		}},
		{"screenshot", func(u *UI) {
			u.Moment(moment{key: "shot", icon: "camera", tint: u.pal.fg, text: "Screenshot saved", thumb: scaleThumb(shot, 30, 20), until: clock.Add(time.Hour)})
		}},
		{"agents-casting", func(u *UI) {
			working(u)
			u.sessions["a"].Agents = 3
			u.casting = true
		}},
	}
	const W, rowH = 520, 34
	sheet := image.NewRGBA(image.Rect(0, 0, W, rowH*len(states)))
	for i, st := range states {
		u := NewUI(440, 280, 24)
		u.now = func() time.Time { return clock }
		u.start, u.lastBlink, u.bounceAt = clock, clock, clock.Add(-time.Hour)
		st.set(u)
		for f := 0; f < 90; f++ {
			clock = clock.Add(time.Second / 60)
			u.renderDamage()
		}
		r := image.Rect(0, rowH*i, W, rowH*(i+1))
		draw.Draw(sheet, r, image.NewUniform(color.NRGBA{0x23, 0x3a, 0x2b, 255}), image.Point{}, draw.Src)
		draw.Draw(sheet, image.Rect(0, rowH*i, W, rowH*i+24), image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 200}), image.Point{}, draw.Over)
		draw.Draw(sheet, r, u.cv.img, image.Pt((440-W)/2, 0), draw.Over)
	}
	big := image.NewRGBA(image.Rect(0, 0, W*2, sheet.Rect.Dy()*2))
	for y := 0; y < big.Rect.Dy(); y++ {
		for x := 0; x < big.Rect.Dx(); x++ {
			big.Set(x, y, sheet.At(x/2, y/2))
		}
	}
	f, _ := os.Create(filepath.Join(dir, "moments.png"))
	png.Encode(f, big)
	f.Close()
}
