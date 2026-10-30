//go:build render

package island

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"testing"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/edge/adapter"
	xdraw "golang.org/x/image/draw"
)

// EMBER_FILM=dir go test -tags render -count=1 -run EdgeTrialFilm ./internal/island:
// a trial evening with edge, played through the real adapter and island:
// the cue card at the first unlock after office, the rep's line at its cue,
// the countdown working and paused, a wrong answer with its mistake chips,
// judging, the accept with the chain, then the dashboard in Trial 1 and in
// Trial 2. Numbered PNGs, for the demo page.
func TestEdgeTrialFilm(t *testing.T) {
	dir := os.Getenv("EMBER_FILM")
	if dir == "" {
		t.Skip("set EMBER_FILM")
	}
	t.Setenv("HOME", t.TempDir()) // the default palette
	t.Setenv("PATH", t.TempDir()) // no real edge behind the buttons
	os.MkdirAll(dir, 0o755)
	g := newScene()
	g.a.ui.pal = loadPalette()
	g.clock = time.Date(2026, 11, 24, 18, 52, 0, 0, time.Local) // a Tuesday in Trial 2
	plug := &edgeDriver{a: g.a}
	run := func(d time.Duration) { // as the app runs: moments end on their own
		for end := g.clock.Add(d); g.clock.Before(end); {
			g.clock = g.clock.Add(time.Second / 60)
			if g.a.ui.expire() {
				g.a.ui.Invalidate()
			}
			g.a.ui.renderDamage()
		}
	}
	bg := demoWallpaper()
	n := 0
	shoot := func(name string, h int) {
		u := g.a.ui
		const W = 640
		out := image.NewRGBA(image.Rect(0, 0, W, h))
		draw.Draw(out, out.Rect, bg, image.Pt((bg.Rect.Dx()-W)/2, 0), draw.Src)
		sx0 := (W - surfW) / 2
		for _, r := range u.shape.blurRects() {
			boxBlur(out, r.Add(image.Pt(sx0, 0)).Inset(-1), 9)
		}
		draw.Draw(out, image.Rect(sx0, 0, sx0+surfW, h), u.cv.img, image.Point{}, draw.Over)
		big := image.NewRGBA(image.Rect(0, 0, W*2, h*2))
		xdraw.CatmullRom.Scale(big, big.Rect, out, out.Rect, xdraw.Src, nil)
		for i := 3; i < len(big.Pix); i += 4 {
			big.Pix[i] = 255
		}
		n++
		savePNG(t, filepath.Join(dir, fmt.Sprintf("%02d-%s.png", n, name)), big)
	}
	say := func(v any) {
		raw, _ := json.Marshal(v)
		var old *adapter.Status
		if plug.status != nil {
			c := *plug.status
			old = &c
		}
		plug.say(raw)
		if m := adapter.Moment(old, plug.status); m != nil && plug.status != nil {
			g.a.applyExt("edge", proto.Message{Type: "moment", Moment: m})
		}
		g.a.ui.Invalidate()
	}
	status := func(e *adapter.Status) {
		e.SeenMs = g.clock.UnixMilli()
		say(map[string]any{"type": "status", "status": e})
	}
	card := func(c map[string]any) { say(map[string]any{"type": "card", "card": c}) }
	remove := func(id string) { say(map[string]any{"type": "remove", "id": id}) }
	chip := func(key, label string) map[string]any { return map[string]any{"key": key, "label": label} }
	btn := func(key, label string, primary bool) map[string]any {
		return map[string]any{"key": key, "label": label, "primary": primary}
	}
	cue := time.Date(2026, 11, 24, 19, 30, 0, 0, time.Local)
	base := func() *adapter.Status {
		return &adapter.Status{Need: 1, Closed: true, Task: "2779. Maximum Beauty of an Array After Applying Operation", TaskID: "lc:2779",
			Short: "LC 2779", Kind: "new", Pending: true, GiveUpInSec: -1, Phase: "trial2", Chain: 4, SkipTokens: 1, Pattern: "Sliding window"}
	}

	// 18:52: the first unlock after office: the cue card
	e := base()
	status(e)
	run(800 * time.Millisecond)
	card(map[string]any{"id": "cue", "mood": "nudge", "title": "Tonight's rep: LC 2779 · Sliding window. When?", "ttl_s": 3600,
		"chips": []map[string]any{chip("cue:now", "Now"), chip("cue:19:30", "19:30"), chip("cue:21:00", "21:00"), chip("skiptonight", "Skip tonight (1 left)")}})
	run(1500 * time.Millisecond)
	shoot("cue-card", 300)

	// the default holds: the rep's line, quiet until its time
	remove("cue")
	e.CueMs = cue.UnixMilli()
	status(e)
	run(1500 * time.Millisecond)
	shoot("rep-before-cue", 120)
	g.clock = cue.Add(time.Minute)
	status(e)
	run(1500 * time.Millisecond)
	shoot("rep-at-cue", 120)

	// 19:34: started; 12 minutes in, the countdown runs
	started := cue.Add(4 * time.Minute)
	g.clock = started.Add(12*time.Minute + 10*time.Second)
	e.Pending, e.StartedMs, e.Working, e.WorkSec, e.AsOfMs = false, started.UnixMilli(), true, 12*60, g.clock.UnixMilli()
	e.DeadlineMs = g.clock.Add(33 * time.Minute).UnixMilli()
	e.GiveUpInSec = 33 * 60
	status(e)
	run(1500 * time.Millisecond)
	shoot("countdown-working", 120)

	// away from the editor: it stands still
	g.clock = g.clock.Add(9 * time.Minute)
	e.Working, e.Paused, e.WorkSec, e.AsOfMs = false, true, 13*60+48, g.clock.Add(-7*time.Minute).UnixMilli()
	e.DeadlineMs = time.UnixMilli(e.AsOfMs).Add(31*time.Minute + 12*time.Second).UnixMilli()
	status(e)
	run(1500 * time.Millisecond)
	shoot("countdown-paused", 120)

	// back, a submission: wrong answer, and what was it?
	g.clock = g.clock.Add(14 * time.Minute)
	e.Working, e.Paused, e.WorkSec, e.AsOfMs = true, false, 27*60, g.clock.UnixMilli()
	e.DeadlineMs = g.clock.Add(18 * time.Minute).UnixMilli()
	e.LastVerdict, e.RepDone, e.Today, e.Chain = "WA on test 7", true, 1, 5 // a submit: the rep counts
	status(e)
	card(map[string]any{"id": "mistake", "mood": "error", "title": "WA on test 7. What was it?", "ttl_s": 1800,
		"chips": []map[string]any{chip("mistake:lc:2779:misread", "Misread"), chip("mistake:lc:2779:edge", "Edge case"),
			chip("mistake:lc:2779:overflow", "Overflow"), chip("mistake:lc:2779:approach", "Approach"), chip("mistake:lc:2779:bug", "Bug")}})
	run(1500 * time.Millisecond)
	shoot("wa-mistake-card", 300)
	shootPill := func(name string) { remove("mistake"); run(1200 * time.Millisecond); shoot(name, 120) }
	shootPill("rep-counted-moment")
	run(7 * time.Second)
	shoot("wa-pill", 120)

	// 20:05: resubmitted, judging
	g.clock = g.clock.Add(6 * time.Minute)
	e.WorkSec, e.AsOfMs, e.Judging = 33*60, g.clock.UnixMilli(), "test 12"
	e.DeadlineMs = g.clock.Add(12 * time.Minute).UnixMilli()
	status(e)
	run(1500 * time.Millisecond)
	shoot("judging", 120)

	// accepted: the chain in the moment
	g.clock = g.clock.Add(40 * time.Second)
	done := base()
	done.Task, done.TaskID, done.Short, done.Pending, done.Today, done.RepDone, done.Chain, done.Explain, done.Closed =
		"", "", "", false, 1, true, 5, "lc:2779", false
	done.CueMs = cue.UnixMilli()
	status(done)
	run(700 * time.Millisecond)
	shoot("accepted-moment", 120)
	card(map[string]any{"id": "ac", "mood": "happy", "title": "Accepted · chain 5", "body": "Maximum Beauty of an Array · 34m of work\nTonight's rep is done", "bounce": true,
		"buttons": []map[string]any{btn("next", "Another", false), btn("explain:lc:2779", "Explain-back (2 min)", true)}})
	run(1500 * time.Millisecond)
	shoot("accepted-card", 300)
	remove("ac")
	run(7 * time.Second)
	shoot("rep-done", 120)

	// the dashboard, in Trial 1 and in Trial 2
	dash := func(name, phase string, at time.Time, e *adapter.Status) {
		dclock := at
		d := NewUI(surfW, surfH, 22)
		d.now = func() time.Time { return dclock }
		st := filmStats(phase, at)
		e.Phase, e.SeenMs = phase, at.UnixMilli()
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
		n++
		savePNG(t, filepath.Join(dir, fmt.Sprintf("%02d-%s.png", n, name)), img)
	}
	t1 := time.Date(2026, 11, 10, 19, 12, 0, 0, time.Local)
	e1 := base()
	e1.Pattern, e1.CueMs, e1.Chain = "", time.Date(2026, 11, 10, 19, 30, 0, 0, time.Local).UnixMilli(), 3
	dash("dashboard-trial1", "trial1", t1, e1)
	e2 := base()
	e2.CueMs = cue.UnixMilli()
	dash("dashboard-trial2", "trial2", cue.Add(-10*time.Minute), e2)
	r := base()
	r.CueMs, r.Pending, r.StartedMs, r.Working, r.WorkSec, r.AsOfMs = cue.UnixMilli(), false, cue.UnixMilli(), true, 12*60, cue.Add(12*time.Minute).UnixMilli()
	r.DeadlineMs, r.GiveUpInSec = cue.Add(45*time.Minute).UnixMilli(), 33*60
	dash("dashboard-trial2-rep", "trial2", cue.Add(12*time.Minute), r)
	b := base()
	dash("dashboard-baseline", "baseline", time.Date(2026, 11, 2, 19, 40, 0, 0, time.Local), b)
}

// filmStats is the fixture in a phase: the last 28 days of reps, and (in
// Trial 2) the readiness model.
func filmStats(phase string, now time.Time) *adapter.Stats {
	st := edgeFixture()
	st.Phase = phase
	st.Today.Date, st.Today.Need, st.Today.StudyTime, st.Today.Gate, st.Today.GateWhy = now.Format("2006-01-02"), 1, true, "closed", ""
	st.Current = &adapter.Task{ID: "lc:2779", Source: "lc", Kind: "new", Title: "2779. Maximum Beauty of an Array After Applying Operation"}
	if phase == "baseline" {
		st.Current.Source, st.Current.ID, st.Current.Title, st.Current.Rating = "cf", "cf:2277C", "Edge Addeleting", 1400
		return st
	}
	c := &adapter.Consistency{From: "2026-11-06", To: "2026-11-19", Chain: 3, SkipTokens: 1, ShowUp: 10.0 / 14, CueToStart: new(float64)}
	*c.CueToStart = 9
	for i := 27; i >= 1; i-- {
		d := now.AddDate(0, 0, -i)
		day := adapter.RepDay{Date: d.Format("2006-01-02")}
		on := !d.Before(time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local))
		switch {
		case on && (i%5 == 2):
			day.Token = true
		case on && i%4 != 1:
			day.Rep = true
		case !on && i%3 == 0:
			day.Rep = true
		}
		c.Days = append(c.Days, day)
	}
	c.Days = append(c.Days, adapter.RepDay{Date: now.Format("2006-01-02")})
	st.Consistency = c
	if phase != "trial2" {
		return st
	}
	r := &adapter.Readiness{R: 0.58, Lo: 0.49, Hi: 0.66, Drills: 1, Brier: new(float64)}
	*r.Brier = 0.21
	r.Mistakes.Last30 = map[string]int{"edge": 4, "bug": 2, "misread": 1, "approach": 1}
	for _, p := range []struct {
		name string
		p, r float64
	}{{"Two pointers", 0.86, 0.95}, {"Sliding window", 0.55, 0.9}, {"Binary search", 0.78, 0.84}, {"Prefix sums", 0.82, 0.92},
		{"Hashing", 0.9, 0.97}, {"Monotonic stack", 0.48, 0.7}, {"Heap", 0.7, 0.88}, {"Intervals", 0.62, 0.75}, {"Greedy", 0.66, 0.9},
		{"Graphs BFS/DFS", 0.72, 0.8}, {"Shortest paths", 0.41, 0.62}, {"DSU", 0.5, 0.55}, {"Trees", 0.74, 0.86}, {"Topo sort", 0.58, 0.66},
		{"DP 1D", 0.6, 0.85}, {"DP 2D", 0.38, 0.7}, {"Backtracking", 0.57, 0.8}} {
		v := p.p * p.r
		r.Patterns = append(r.Patterns, adapter.Pattern{Name: p.name, Weight: 1, P: p.p, R: p.r, Lo: v - 0.12, Hi: min(1, v+0.1)})
	}
	st.Readiness = r
	return st
}
