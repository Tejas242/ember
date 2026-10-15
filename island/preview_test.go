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

	xdraw "golang.org/x/image/draw"
)

// EDGE_PREVIEW=dir go test -run Preview ./island renders each state over a mock bar.
func TestPreview(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	clock := time.Date(2026, 10, 11, 21, 30, 0, 0, time.Local)
	mk := func() *UI {
		u := NewUI(440, 280, 24)
		u.now = func() time.Time { return clock }
		u.mind = newMind(clock)
		u.start, u.lastBlink, u.bounceAt = clock, clock, clock.Add(-time.Hour)
		return u
	}
	music := func(u *UI, playing bool) {
		u.track = &Track{Player: "x", Title: "Veridis Quo", Artist: "Daft Punk", Playing: playing, Length: 5 * time.Minute, Position: 2 * time.Minute, PosAt: clock}
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				img.Set(x, y, color.NRGBA{uint8(120 + x*2), uint8(40 + y), uint8(160 - x), 255})
			}
		}
		u.art = &albumArt{small: scaleSquare(img, 18), large: scaleSquare(img, 60), row: scaleSquare(img, 30)}
		u.art.accent = accentOf(u.art.large)
	}
	save := func(name string, u *UI) {
		for i := 0; i < 150; i++ {
			clock = clock.Add(time.Second / 60)
			u.renderDamage()
		}
		const W, H = 700, 300
		bg := image.NewRGBA(image.Rect(0, 0, W, H))
		for y := 0; y < H; y++ { // a dark green wallpaper-ish gradient
			for x := 0; x < W; x++ {
				bg.Set(x, y, color.NRGBA{uint8(20 + x/30), uint8(38 + y/8), uint8(28 + x/40), 255})
			}
		}
		draw.Draw(bg, image.Rect(0, 0, W, 24), image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 170}), image.Point{}, draw.Over)
		off := (W - 440) / 2
		draw.Draw(bg, image.Rect(off, 0, off+440, 280), u.cv.img, image.Point{}, draw.Over)
		big := image.NewRGBA(image.Rect(0, 0, W*2, H*2))
		xdraw.CatmullRom.Scale(big, big.Rect, bg, bg.Rect, xdraw.Src, nil)
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		png.Encode(f, big)
		f.Close()
	}

	save("1-clock", mk())

	u := mk()
	u.edge = &EdgeStatus{Today: 0, Need: 1, Streak: 3, Closed: true, Task: "1360E · Polygon", Pending: true}
	save("2-clock-locked", u)

	u = mk()
	working(u)
	save("3-claude", u)

	u = mk()
	working(u)
	music(u, true)
	save("4-claude-music", u)

	u = mk()
	music(u, true)
	save("5-music", u)

	u = mk()
	working(u)
	u.sessions["b"] = &Session{ID: "b", Project: "kinvault", State: "wait", Since: clock.Add(-31 * time.Minute)}
	music(u, true)
	u.sessions["a"].Since, u.sessions["a"].Title = clock.Add(-9*time.Minute), "Quota enforcement"
	for i := 0; i < 40; i++ {
		u.sessions["a"].bump(clock.Add(-time.Duration(i*13) * time.Second))
	}
	u.vol, u.light = level{v: 0.38, ok: true}, 0.7
	u.power = &power{pct: 66, state: 2, present: true, tte: 2*3600 + 41*60}
	u.lastDone.project, u.lastDone.at = "airspace", clock.Add(-12*time.Minute)
	u.pinned = true
	save("6-expanded", u)
	u.edge = &EdgeStatus{Today: 0, Need: 1, Streak: 3, Closed: true, Task: "1360E · Polygon", Pending: true}

	save("6b-expanded-claude", u)

	u.edge.StartedMs = clock.Add(-23 * time.Minute).UnixMilli()
	u.edge.Pending = false
	save("6c-expanded-edge", u)

	u = mk()
	u.vol = level{v: 0.38, ok: true}
	u.light = 0.7
	u.power = &power{pct: 66, state: 2, present: true, tte: 2*3600 + 41*60}
	u.sessions["c"] = &Session{ID: "c", Project: "ember", State: "idle", Title: "Ember end-to-end product", Since: clock.Add(-2 * time.Hour)}
	u.pinned = true
	save("6d-expanded-system", u)

	save("6e-expanded-claude-titles", u)

	u = mk()
	u.AddCard(&Card{ID: "p", Kind: "perm", Mood: moodWait, Title: "Run the store tests with the race detector", Eyebrow: "kinvault wants to run", Shell: true,
		Code:    "go test -race ./store/... -run TestRepair -count=1",
		Buttons: []Button{{Key: "deny", Label: "Deny", Danger: true}, {Key: "always", Label: "Always"}, {Key: "allow", Label: "Allow", Primary: true}},
		Footer:  "In terminal"})
	save("7-permission", u)

	u = mk()
	u.AddCard(&Card{ID: "q", Kind: "ask", Mood: moodWait, Title: "Which storage engine?", Eyebrow: "kinvault asks", Meta: "1 of 2",
		Options: []Option{{"Pebble", "LSM, battle tested"}, {"bbolt", "B+tree, single file"}, {"Roll our own", "Append-only log + index"}},
		Footer:  "In terminal"})
	save("8-question", u)
}
