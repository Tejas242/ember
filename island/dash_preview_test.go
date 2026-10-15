package island

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sampleStats is a few months of practice, shaped like `edge stats`.
func sampleStats(now time.Time) *EdgeStats {
	var st EdgeStats
	st.Version = 1
	st.Today.Date = now.Format("2006-01-02")
	st.Today.Got, st.Today.Need, st.Today.Gate, st.Today.WorkSec = 0, 1, "closed", 1260
	st.Streak.Current, st.Streak.Best = 4, 9
	cur := &struct {
		ID           string    `json:"id"`
		Source       string    `json:"source"`
		Kind         string    `json:"kind"`
		Title        string    `json:"title"`
		Rating       int       `json:"rating"`
		Started      time.Time `json:"started"`
		WorkSec      int       `json:"work_sec"`
		Hints        int       `json:"hints"`
		HintsAllowed int       `json:"hints_allowed"`
		NextHintSec  int       `json:"next_hint_sec"`
		GiveUpInSec  int       `json:"give_up_in_sec"`
		Fails        int       `json:"fails"`
	}{ID: "cf:2257C", Source: "cf", Kind: "new", Title: "Spying on the Beaver", Rating: 1400,
		Started: now.Add(-25 * time.Minute), WorkSec: 1260, NextHintSec: 840, Fails: 2}
	st.Current = cur
	for i := 364; i >= 0; i-- {
		d := now.AddDate(0, 0, -i)
		day := EdgeDay{Date: d.Format("2006-01-02"), Need: 1}
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			day.Need = 3
		}
		switch h := (i*7 + i/3) % 11; {
		case i > 150:
			day.CFSolved = map[bool]int{true: 1}[h < 3]
		case h < 6:
			day.Credits = 1 + h%3
			day.Met = day.Credits >= day.Need
			day.WorkSec = 1800 + 600*h
		case h == 7:
			day.Pass = true
		}
		if i == 0 {
			day.Credits, day.Met = 0, false
		}
		st.Calendar = append(st.Calendar, day)
	}
	st.Week.Days = st.Calendar[len(st.Calendar)-4:]
	for i := 1; i <= 3; i++ {
		st.Week.Days = append(st.Week.Days, EdgeDay{Date: now.AddDate(0, 0, i).Format("2006-01-02"), Need: 1})
	}
	st.Week.Credits, st.Week.Target = 3, 12
	st.Totals.Solved, st.Totals.Solo = 31, 19
	st.Resolves.Due, st.Resolves.FirstAttempts, st.Resolves.FirstRetained = 1, 10, 7
	type src = struct {
		Rating        int `json:"rating"`
		BandLo        int `json:"band_lo"`
		BandHi        int `json:"band_hi"`
		RatingHistory []struct {
			At      time.Time `json:"at"`
			Rating  int       `json:"rating"`
			Contest string    `json:"contest"`
		} `json:"rating_history"`
	}
	cf := src{Rating: 1208, BandLo: 1200, BandHi: 1500}
	for i, r := range []int{0, 1050, 1120, 1094, 1163, 1150, 1185, 1172, 1208} {
		if i == 0 {
			continue
		}
		cf.RatingHistory = append(cf.RatingHistory, struct {
			At      time.Time `json:"at"`
			Rating  int       `json:"rating"`
			Contest string    `json:"contest"`
		}{now.AddDate(0, 0, -14*(9-i)), r, fmt.Sprint("Round ", i)})
	}
	st.Sources = map[string]src{"cf": cf}
	for _, t := range []struct {
		tag         string
		tasks, solo int
	}{{"dp", 6, 2}, {"greedy", 8, 6}, {"graphs", 3, 1}, {"math", 5, 4}} {
		st.Topics = append(st.Topics, struct {
			Tag         string `json:"tag"`
			Source      string `json:"source"`
			FocusWeight int    `json:"focus_weight"`
			Tasks       int    `json:"tasks"`
			Solo        int    `json:"solo"`
		}{t.tag, "cf", 2, t.tasks, t.solo})
	}
	return &st
}

// EMBER_PREVIEW=dir go test -run PreviewDash ./island: edge's dashboard as
// it opens, settled, with a day under the pointer, and from today's real
// `edge stats` if EMBER_EDGE_STATS names a file.
func TestPreviewDash(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	const W, H = 580, 410
	render := func(st *EdgeStats, frames []int, hover string) *image.RGBA {
		clock := time.Date(2026, 10, 15, 21, 30, 0, 0, time.Local)
		u := NewUI(W, H, 22)
		u.now = func() time.Time { return clock }
		u.edgeStats = st
		u.dashboards["edge"] = edgeDash{}
		settle(u, &clock)
		u.openDash("edge")
		strip := image.NewRGBA(image.Rect(0, 0, W, H*len(frames)))
		draw.Draw(strip, strip.Rect, image.NewUniform(color.NRGBA{0x22, 0x33, 0x26, 255}), image.Point{}, draw.Src)
		f := 0
		for i, n := range frames {
			for ; f < n; f++ {
				clock = clock.Add(time.Second / 60)
				if f == n-2 && hover != "" {
					u.hover, u.hoverKey = true, hover
					u.Invalidate()
				}
				u.renderDamage()
			}
			draw.Draw(strip, image.Rect(0, H*i, W, H*i+22), image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 255}), image.Point{}, draw.Over)
			draw.Draw(strip, image.Rect(0, H*i, W, H*i+H), u.cv.img, image.Point{}, draw.Over)
		}
		return strip
	}
	save := func(name string, img image.Image) {
		out, _ := os.Create(filepath.Join(dir, name))
		png.Encode(out, img)
		out.Close()
	}
	now := time.Date(2026, 10, 15, 21, 30, 0, 0, time.Local)
	save("dash-film.png", render(sampleStats(now), []int{8, 20, 40, 60, 140}, ""))
	save("dash-hover.png", render(sampleStats(now), []int{160}, "heat:300"))
	if p := os.Getenv("EMBER_EDGE_STATS"); p != "" {
		var st EdgeStats
		b, err := os.ReadFile(p)
		if err != nil || json.Unmarshal(b, &st) != nil {
			t.Fatal("bad stats file", err)
		}
		save("dash-real.png", render(&st, []int{160}, ""))
	}
}
