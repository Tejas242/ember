//go:build render

package island

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tejas242/ember/plugins/edge/adapter"
	xdraw "golang.org/x/image/draw"
)

// EMBER_EDGE_STATS=stats.json EMBER_PREVIEW=dir go test -tags render -count=1 -run EdgeDashStates ./internal/island:
// edge's redesigned dashboard in its seven states, as edge's plugin declares
// it and the island draws it: 1x, 2x, Ember ×6, and a sheet of all of them.

func statsDay(st *adapter.Stats, date string) *adapter.Day {
	for i := range st.Calendar {
		if st.Calendar[i].Date == date {
			return &st.Calendar[i]
		}
	}
	return nil
}

// meet marks days met, in the calendar and the week.
func meet(st *adapter.Stats, dates ...string) {
	for _, d := range dates {
		if c := statsDay(st, d); c != nil {
			c.Credits, c.Met, c.WorkSec = c.Need, true, 1500+300*len(d)%900
		}
		for i := range st.Week.Days {
			if st.Week.Days[i].Date == d {
				st.Week.Days[i].Credits, st.Week.Days[i].Met, st.Week.Days[i].WorkSec = st.Week.Days[i].Need, true, 1700
			}
		}
	}
	st.Week.Credits = 0
	for _, d := range st.Week.Days {
		st.Week.Credits += d.Credits
	}
}

type dashState struct {
	name  string
	clock time.Time
	edit  func(st *adapter.Stats, now time.Time)
	at    time.Duration
}

func dashStates() []dashState {
	loc := time.Local
	return []dashState{
		{"1-quiet-morning", time.Date(2026, 10, 20, 10, 26, 0, 0, loc), func(st *adapter.Stats, now time.Time) {}, 2500 * time.Millisecond},
		{"2-behind-task-open", time.Date(2026, 10, 20, 21, 40, 0, 0, loc), func(st *adapter.Stats, now time.Time) {
			st.Today.StudyTime, st.Today.Gate, st.Today.GateWhy = true, "closed", ""
		}, 2500 * time.Millisecond},
		{"3-practising", time.Date(2026, 10, 20, 21, 58, 0, 0, loc), func(st *adapter.Stats, now time.Time) {
			st.Today.StudyTime, st.Today.Gate, st.Today.GateWhy, st.Today.WorkSec = true, "closed", "", 1380
			st.Current.Started, st.Current.WorkSec, st.Current.Fails = now.Add(-25*time.Minute), 1380, 1
			st.Current.HintsAllowed, st.Current.NextHintSec = 1, 0
			st.Week.Days[1].WorkSec = 1380
		}, 2500 * time.Millisecond},
		{"4-on-track", time.Date(2026, 10, 22, 20, 15, 0, 0, loc), func(st *adapter.Stats, now time.Time) {
			st.Today.Date = "2026-10-22"
			meet(st, "2026-10-19", "2026-10-20", "2026-10-21", "2026-10-22")
			st.Today.Got, st.Today.Done, st.Today.WorkSec, st.Today.Gate, st.Today.GateWhy = 1, true, 1560, "open", "done"
			st.Streak.Current, st.Streak.Best = 4, 4
			st.Current = nil
			st.Totals.Solved, st.Totals.Solo = 6, 5
			st.Rounds.Upsolves.Queue = st.Rounds.Upsolves.Queue[1:]
			st.Recent = append([]adapter.Task{{ID: "cf:2261B", Source: "cf", Title: "What's the Capital of France?", Outcome: "solo", WorkSec: 1560},
				{ID: "lc:x", Source: "lc", Title: "2092. Find All People With Secret", Outcome: "solo", WorkSec: 1980},
				{ID: "cf:1873E", Source: "cf", Title: "Building an Aquarium", Outcome: "assisted", WorkSec: 2400}}, st.Recent...)
		}, 2500 * time.Millisecond},
		{"5-live-round", time.Date(2026, 10, 24, 17, 28, 0, 0, loc), func(st *adapter.Stats, now time.Time) {
			st.Today.Date = "2026-10-24"
			meet(st, "2026-10-19", "2026-10-20", "2026-10-21", "2026-10-22", "2026-10-23")
			st.Today.Got, st.Today.Need, st.Streak.Current = 0, 3, 5
			st.Current = nil
			r := *st.Rounds.Next
			r.State, r.Joined = "live", "live"
			r.Problems = []adapter.Problem{{Index: "A", State: "solved"}, {Index: "B", State: "tried", Fails: 1}, {Index: "C"}, {Index: "D"}, {Index: "E"}, {Index: "F"}}
			st.Rounds.Live = &r
		}, 2500 * time.Millisecond},
		{"6-streak-milestone", time.Date(2026, 10, 26, 21, 5, 0, 0, loc), func(st *adapter.Stats, now time.Time) {
			st.Today.Date = "2026-10-26"
			for i := 20; i <= 26; i++ {
				d := fmt.Sprintf("2026-10-%d", i)
				if statsDay(st, d) == nil {
					st.Calendar = append(st.Calendar, adapter.Day{Date: d, Need: 1})
				}
			}
			ws := []adapter.Day{}
			for i := 26; i <= 32; i++ {
				ws = append(ws, adapter.Day{Date: time.Date(2026, 10, i, 0, 0, 0, 0, loc).Format("2006-01-02"), Need: map[bool]int{true: 3}[i == 31 || i == 32] + map[bool]int{true: 1}[i < 31]})
			}
			st.Week.Days, st.Week.Target = ws, 12
			meet(st, "2026-10-20", "2026-10-21", "2026-10-22", "2026-10-23", "2026-10-24", "2026-10-25", "2026-10-26")
			st.Today.Got, st.Today.Need, st.Today.Done, st.Today.WorkSec, st.Today.Gate, st.Today.GateWhy = 1, 1, true, 1900, "open", "done"
			st.Streak.Current, st.Streak.Best = 7, 7
			st.Current = nil
			st.Rounds.Upsolves.Queue = nil
			st.Rounds.Next = nil
			st.Rounds.Upcoming = nil
			st.Rounds.Record.Held, st.Rounds.Record.Sat = 2, 1
			st.Totals.Solved, st.Totals.Solo = 9, 7
		}, 1120 * time.Millisecond},
		{"7-late-night", time.Date(2026, 10, 21, 0, 40, 0, 0, loc), func(st *adapter.Stats, now time.Time) {
			meet(st, "2026-10-20")
			st.Today.Got, st.Today.Done, st.Today.WorkSec, st.Today.Gate, st.Today.GateWhy = 1, true, 1620, "open", "done"
			st.Streak.Current = 1
			st.Current = nil
			st.Totals.Solved, st.Totals.Solo = 3, 3
			st.Recent = append([]adapter.Task{{ID: "cf:2261B", Source: "cf", Title: "What's the Capital of France?", Outcome: "solo", WorkSec: 1620}}, st.Recent...)
			st.Rounds.Upsolves.Queue = st.Rounds.Upsolves.Queue[1:]
		}, 3200 * time.Millisecond},
	}
}

func TestEdgeDashStates(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	b, err := os.ReadFile(os.Getenv("EMBER_EDGE_STATS"))
	if dir == "" || err != nil {
		t.Skip("set EMBER_PREVIEW and EMBER_EDGE_STATS")
	}
	var shots []galleryShot
	for _, s := range dashStates() {
		var st adapter.Stats
		if err := json.Unmarshal(b, &st); err != nil {
			t.Fatal(err)
		}
		s.edit(&st, s.clock)
		g := &galleryScene{clock: s.clock}
		g.a = newTestApp(&g.clock)
		u := NewUI(surfW, surfH, 22)
		u.now, u.mind, u.day = g.a.ui.now, g.a.ui.mind, g.a.ui.day
		u.start, u.lastBlink, u.bounceAt = g.clock, g.clock, g.clock.Add(-time.Hour)
		g.a.ui = u
		edgeSays(u, nil, &st)
		g.run(500 * time.Millisecond)
		u.openDash("edge")
		g.run(s.at)
		_, h := extDash{name: "edge"}.Size(u)
		img := onDesktop(u, image.Rect(surfW/2-290, 0, surfW/2+290, min(surfH, int(h)+16)))
		one := image.NewRGBA(image.Rect(0, 0, img.Rect.Dx(), img.Rect.Dy()))
		draw.Draw(one, one.Rect, img, img.Rect.Min, draw.Src)
		savePNG(t, filepath.Join(dir, s.name+"-1x.png"), one)
		big := image.NewRGBA(image.Rect(0, 0, one.Rect.Dx()*2, one.Rect.Dy()*2))
		xdraw.CatmullRom.Scale(big, big.Rect, one, one.Rect, xdraw.Src, nil)
		savePNG(t, filepath.Join(dir, s.name+"-2x.png"), big)
		ex, ey := int(u.ex.x), int(u.ey.x)
		savePNG(t, filepath.Join(dir, s.name+"-ember6x.png"), upscale(onDesktop(u, image.Rect(ex-34, ey-40, ex+34, ey+36)), 6))
		shots = append(shots, galleryShot{s.name + fmt.Sprintf("  (%d px tall)", int(h)), one})
	}
	sheet(t, "edge-dash-all-states", shots, 2)
}
