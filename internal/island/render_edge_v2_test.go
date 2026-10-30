//go:build render

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

	proto "github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/edge/adapter"
	xdraw "golang.org/x/image/draw"
)

// edgeSaysV2 is edgeSays with edge's v2 status behind the dashboard too.
func edgeSaysV2(u *UI, e *adapter.Status, st *adapter.Stats) {
	edgeSays(u, e, nil)
	if st != nil {
		b, _ := proto.Line("dashboard", adapter.DashboardFor(st, e, u.now()))
		m, err := proto.Decode(b)
		if err != nil {
			panic(err)
		}
		u.extOf("edge").dash = m.Dashboard
		u.dashboards["edge"] = extDash{name: "edge"}
	}
}

// EMBER_PREVIEW=dir go test -tags render -count=1 -run PreviewEdgeV2 ./internal/island:
// edge's contract v2 states: the pill (working, paused, judging, a wrong
// answer), the row, and the dashboard (working with give-up, owed,
// explain-back, no task).
func TestPreviewEdgeV2(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	clock := time.Date(2026, 10, 20, 21, 58, 0, 0, time.Local)
	task := func() *adapter.Status {
		return &adapter.Status{Today: 0, Need: 1, Streak: 4, Closed: true, Task: "2277C · Edge Addeleting", TaskID: "cf:2277C", Short: "2277C", Kind: "new",
			StartedMs: clock.Add(-time.Hour).UnixMilli(), WorkSec: 19 * 60, Working: true, AsOfMs: clock.UnixMilli(), Policy: "ladder",
			NextHintSec: 60, GiveUpInSec: 41 * 60, SeenMs: clock.UnixMilli()}
	}
	mk := func() *UI {
		u := NewUI(440, 280, 24)
		u.now = func() time.Time { return clock }
		u.mind = newMind(clock)
		u.start, u.lastBlink, u.bounceAt = clock, clock, clock.Add(-time.Hour)
		return u
	}
	shoot := func(name string, u *UI, e *adapter.Status) {
		edgeSays(u, e, nil)
		settle(u, &clock)
		const W, H = 640, 260
		bg := image.NewRGBA(image.Rect(0, 0, W, H))
		draw.Draw(bg, bg.Rect, image.NewUniform(color.NRGBA{0x22, 0x33, 0x26, 255}), image.Point{}, draw.Src)
		off := (W - 440) / 2
		draw.Draw(bg, image.Rect(off, 0, off+440, 280), u.cv.img, image.Point{}, draw.Over)
		big := image.NewRGBA(image.Rect(0, 0, W*2, H*2))
		xdraw.CatmullRom.Scale(big, big.Rect, bg, bg.Rect, xdraw.Src, nil)
		savePNG(t, filepath.Join(dir, name+".png"), big)
	}
	pills := []struct {
		name string
		edit func(e *adapter.Status)
	}{
		{"v1-working", func(e *adapter.Status) {}},
		{"v2-paused", func(e *adapter.Status) { e.Working, e.Paused, e.WorkSec = false, true, 18*60 }},
		{"v3-judging", func(e *adapter.Status) { e.Judging = "test 9" }},
		{"v4-wa", func(e *adapter.Status) { e.LastVerdict = "WA on test 3" }},
		{"v5-hint-ready", func(e *adapter.Status) { e.HintsReady, e.NextHintSec = true, 0 }},
		{"v6-unstarted", func(e *adapter.Status) { e.StartedMs, e.Pending, e.Working, e.WorkSec = 0, true, false, 0 }},
		{"v7-lc", func(e *adapter.Status) {
			e.Task, e.Short, e.TaskID = "3542. Minimum Operations to Convert All Elements to Zero", "LC 3542", "lc:3542"
		}},
	}
	for _, p := range pills {
		e := task()
		p.edit(e)
		shoot(p.name+"-pill", mk(), e)
		u := mk()
		u.pinned = true
		shoot(p.name+"-card", u, e)
	}

	dashes := []struct {
		name  string
		clock time.Time
		edit  func(st *adapter.Stats, e *adapter.Status)
	}{
		{"d1-working-giveup", clock, func(st *adapter.Stats, e *adapter.Status) {
			st.Today.StudyTime, st.Today.Gate, st.Today.GateWhy = true, "closed", ""
			st.Current = &adapter.Task{ID: "cf:2277C", Source: "cf", Kind: "new", Title: "Edge Addeleting", Rating: 1600, Started: clock.Add(-2 * time.Hour),
				WorkSec: 3700, Hints: 3, HintsAllowed: 3, Fails: 2}
			e.WorkSec, e.HintsReady, e.NextHintSec, e.GiveUpInSec, e.LastVerdict = 3700, false, 0, 0, "WA on test 3"
		}},
		{"d2-owed", clock, func(st *adapter.Stats, e *adapter.Status) {
			st.Today.StudyTime, st.Today.Gate, st.Today.GateWhy = true, "closed", ""
			st.Current = nil
			*e = adapter.Status{Today: 0, Need: 1, Closed: true, GiveUpInSec: -1, SeenMs: clock.UnixMilli(),
				Owed: []adapter.Owed{{Round: "cf:2261", Short: "CF 1129", Indexes: []string{"C", "D1"}, DueMs: clock.Add(50 * time.Hour).UnixMilli()},
					{Round: "cf:2262", Short: "CF 1130", Indexes: []string{"C", "D"}, DueMs: clock.Add(74 * time.Hour).UnixMilli()}}}
		}},
		{"d3-explain", clock, func(st *adapter.Stats, e *adapter.Status) {
			st.Today.Got, st.Today.Done, st.Today.WorkSec, st.Today.Gate, st.Today.GateWhy = 1, true, 2400, "open", "done"
			st.Current = nil
			st.Recent = append([]adapter.Task{{ID: "cf:2277C", Source: "cf", Kind: "new", Title: "Edge Addeleting", Outcome: "solo", WorkSec: 2400,
				Finished: clock.Add(-20 * time.Minute), Explain: -1}}, st.Recent...)
			*e = adapter.Status{Today: 1, Need: 1, Streak: 5, GiveUpInSec: -1, Explain: "cf:2277C", SeenMs: clock.UnixMilli()}
		}},
		{"d4-no-task", time.Date(2026, 10, 20, 10, 26, 0, 0, time.Local), func(st *adapter.Stats, e *adapter.Status) {
			st.Current, st.Rounds.Upsolves.Queue = nil, nil
			*e = adapter.Status{Today: 0, Need: 1, GiveUpInSec: -1}
		}},
	}
	for _, s := range dashes {
		dclock := s.clock
		d := NewUI(surfW, surfH, 22)
		d.now = func() time.Time { return dclock }
		st := edgeFixture()
		e := task()
		s.edit(st, e)
		edgeSaysV2(d, e, st)
		settle(d, &dclock)
		d.openDash("edge")
		for f := 0; f < 150; f++ {
			dclock = dclock.Add(time.Second / 30)
			d.renderDamage()
		}
		_, h := extDash{name: "edge"}.Size(d)
		img := image.NewRGBA(image.Rect(0, 0, surfW, int(h)+40))
		draw.Draw(img, img.Rect, image.NewUniform(color.NRGBA{0x22, 0x33, 0x26, 255}), image.Point{}, draw.Src)
		draw.Draw(img, img.Rect, d.cv.img, image.Point{}, draw.Over)
		f, _ := os.Create(filepath.Join(dir, s.name+".png"))
		png.Encode(f, img)
		f.Close()
	}
}
