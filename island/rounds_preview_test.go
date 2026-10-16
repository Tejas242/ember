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

// sampleRound is CF 1128, 20:05 to 23:05, with A and B solved and C tried.
func sampleRound(clock time.Time, start time.Time) *EdgeRound {
	return &EdgeRound{ID: "cf:2261", Name: "Codeforces Round 1128 (Div. 1 + Div. 2)", Short: "CF 1128",
		StartMs: start.UnixMilli(), EndMs: start.Add(3 * time.Hour).UnixMilli(), Plan: "in",
		Problems: []EdgeProblem{{Index: "A"}, {Index: "B"}, {Index: "C"}, {Index: "D"}, {Index: "E"}, {Index: "F"}}}
}

// sampleRounds is the rounds part of a stats document: a live or upcoming
// round, and three finished ones.
func sampleRounds(now time.Time, live bool) *EdgeRounds {
	rs := &EdgeRounds{Enabled: true, PerWeek: 2}
	rs.Weeks.Current, rs.Record.Held, rs.Record.Sat = 3, 5, 4
	rs.Upsolves.Done, rs.Upsolves.Lapsed = 5, 1
	ps := func(states ...string) []EdgeProblem {
		var out []EdgeProblem
		for i, s := range states {
			out = append(out, EdgeProblem{Index: string(rune('A' + i)), State: s})
		}
		return out
	}
	r := EdgeStatsRound{ID: "cf:2261", Short: "CF 1128", Kind: "div12", Plan: "in", State: "upcoming",
		Start: now.Add(26 * time.Hour), End: now.Add(29 * time.Hour)}
	if live {
		r.State, r.Start, r.End, r.Joined = "live", now.Add(-48*time.Minute), now.Add(132*time.Minute), "live"
		r.Problems = ps("solved", "solved", "tried", "", "", "")
		rs.Live = &r
	} else {
		rs.Next = &r
	}
	rs.Recent = []EdgeStatsRound{
		{Short: "ABC 479", Kind: "abc", State: "over", Joined: "live", Rated: true, Delta: 64, Lesson: "reread the constraints before coding",
			Problems: ps("solved", "solved", "solved", "upsolved", "upsolve", "", "")},
		{Short: "Weekly 523", Kind: "weekly", State: "over", Plan: "in", Problems: ps("", "", "upsolved", "lapsed")},
		{Short: "CF 1127", Kind: "div2", State: "over", Joined: "live", Rated: true, Delta: -31, Problems: ps("solved", "solved", "taught", "upsolved", "", "")},
	}
	for i := range rs.Recent[1].Problems {
		rs.Recent[1].Problems[i].Index = "Q" + string(rune('1'+i))
	}
	return rs
}

// EMBER_PREVIEW=dir go test -run PreviewRounds ./island: the pill before,
// during and after a round, the card's row, and the dashboard's section.
func TestPreviewRounds(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	clock := time.Date(2026, 10, 17, 18, 23, 0, 0, time.Local)
	start := time.Date(2026, 10, 17, 20, 5, 0, 0, time.Local)
	mk := func() *UI {
		u := NewUI(440, 280, 24)
		u.now = func() time.Time { return clock }
		u.mind = newMind(clock)
		u.start, u.lastBlink, u.bounceAt = clock, clock, clock.Add(-time.Hour)
		u.edge = &EdgeStatus{Today: 0, Need: 3, Streak: 4}
		return u
	}
	shoot := func(name string, u *UI) {
		settle(u, &clock)
		const W, H = 640, 260
		bg := image.NewRGBA(image.Rect(0, 0, W, H))
		for y := 0; y < H; y++ {
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

	u := mk()
	u.edge.Round = sampleRound(clock, start)
	shoot("r1-soon", u)

	clock = start.Add(-12 * time.Minute)
	u = mk()
	u.edge.Round = sampleRound(clock, start)
	shoot("r2-starting", u)

	clock = start.Add(108 * time.Minute)
	u = mk()
	r := sampleRound(clock, start)
	r.Joined, r.Locked = "live", true
	r.Problems[0].State, r.Problems[1].State, r.Problems[2].State = "solved", "solved", "tried"
	u.edge.Round = r
	shoot("r3-live", u)
	u.pinned = true
	shoot("r4-live-card", u)

	clock = start.Add(3*time.Hour + 20*time.Minute)
	u = mk()
	r = sampleRound(clock, start)
	r.Joined, r.Settled = "live", true
	r.Problems[0].State, r.Problems[1].State, r.Problems[2].State, r.Problems[3].State = "solved", "solved", "upsolve", "upsolve"
	u.edge.Round = r
	u.edge.Today = 3
	shoot("r5-over", u)
	u.pinned = true
	shoot("r6-over-card", u)

	// the dashboard
	for i, live := range []bool{false, true} {
		dclock := time.Date(2026, 10, 15, 21, 30, 0, 0, time.Local)
		d := NewUI(surfW, surfH, 22)
		d.now = func() time.Time { return dclock }
		st := sampleStats(dclock)
		st.Rounds = sampleRounds(dclock, live)
		d.edgeStats = st
		d.dashboards["edge"] = edgeDash{}
		settle(d, &dclock)
		d.openDash("edge")
		for f := 0; f < 150; f++ {
			dclock = dclock.Add(time.Second / 30)
			d.renderDamage()
		}
		_, h := edgeDash{}.Size(d)
		img := image.NewRGBA(image.Rect(0, 0, surfW, int(h)+40))
		draw.Draw(img, img.Rect, image.NewUniform(color.NRGBA{0x22, 0x33, 0x26, 255}), image.Point{}, draw.Src)
		draw.Draw(img, img.Rect, d.cv.img, image.Point{}, draw.Over)
		f, _ := os.Create(filepath.Join(dir, []string{"r7-dash.png", "r8-dash-live.png"}[i]))
		png.Encode(f, img)
		f.Close()
	}
}
