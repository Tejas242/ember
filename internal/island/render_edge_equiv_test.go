//go:build render

package island

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tejas242/ember/plugins/edge/adapter"
)

// Until the built-in edge goes: every edge state through the plugin must
// draw the very pixels the built-in drew.
func TestEdgeThroughThePlugin(t *testing.T) {
	clock := time.Date(2026, 10, 17, 18, 23, 0, 0, time.Local)
	start := time.Date(2026, 10, 17, 20, 5, 0, 0, time.Local)
	toA := func(e *EdgeStatus) *adapter.Status {
		if e == nil {
			return nil
		}
		out := &adapter.Status{Today: e.Today, Need: e.Need, Streak: e.Streak, Closed: e.Closed, Task: e.Task, URL: e.URL, StartedMs: e.StartedMs, Pending: e.Pending}
		if r := e.Round; r != nil {
			ar := &adapter.Round{ID: r.ID, Name: r.Name, Short: r.Short, URL: r.URL, StartMs: r.StartMs, EndMs: r.EndMs, Plan: r.Plan, Joined: r.Joined, Locked: r.Locked, Settled: r.Settled}
			for _, p := range r.Problems {
				ar.Problems = append(ar.Problems, adapter.Problem{Index: p.Index, State: p.State, AtSec: p.AtSec, Fails: p.Fails})
			}
			out.Round = ar
		}
		return out
	}
	round := func(phase string, now time.Time) *EdgeRound {
		r := sampleRound(now, start)
		switch phase {
		case "live":
			r.Joined, r.Locked = "live", true
			r.Problems[0].State, r.Problems[1].State, r.Problems[2].State = "solved", "solved", "tried"
		case "over":
			r.Joined, r.Settled = "live", true
			r.Problems[0].State, r.Problems[1].State, r.Problems[2].State, r.Problems[3].State = "solved", "solved", "upsolve", "upsolve"
		case "unjoined":
			r.Plan = ""
		}
		return r
	}
	type scene struct {
		name   string
		at     time.Time
		e      *EdgeStatus
		pinned bool
		claude bool
	}
	var scenes []scene
	for _, s := range []struct {
		name string
		at   time.Time
		ph   string
	}{{"soon", clock, "soon"}, {"starting", start.Add(-12 * time.Minute), "soon"}, {"live", start.Add(108 * time.Minute), "live"},
		{"live-unjoined", start.Add(30 * time.Minute), "unjoined"}, {"over", start.Add(3*time.Hour + 20*time.Minute), "over"}} {
		for _, pinned := range []bool{false, true} {
			for _, claude := range []bool{false, true} {
				scenes = append(scenes, scene{s.name, s.at, &EdgeStatus{Need: 3, Streak: 4, Round: round(s.ph, s.at)}, pinned, claude})
			}
		}
	}
	for _, e := range []*EdgeStatus{
		{Today: 0, Need: 1, Streak: 3, Closed: true, Task: "1360E · Polygon", Pending: true},
		{Today: 0, Need: 1, Streak: 3, Closed: true, Task: "1360E · Polygon", StartedMs: clock.Add(-23 * time.Minute).UnixMilli()},
		{Today: 1, Need: 2, Streak: 4},
		{Today: 2, Need: 2, Streak: 0},
		{Today: 0, Need: 1, Closed: true},
	} {
		for _, pinned := range []bool{false, true} {
			for _, claude := range []bool{false, true} {
				scenes = append(scenes, scene{"day", clock, e, pinned, claude})
			}
		}
	}
	dir := os.Getenv("EMBER_PREVIEW")
	bad := 0
	for i, s := range scenes {
		render := func(builtin bool) *image.RGBA {
			at := s.at
			g := &galleryScene{clock: at}
			g.a = newTestApp(&g.clock)
			u := NewUI(surfW, surfH, 22)
			u.now, u.mind, u.day = g.a.ui.now, g.a.ui.mind, g.a.ui.day
			u.start, u.lastBlink, u.bounceAt = g.clock, g.clock, g.clock.Add(-time.Hour)
			g.a.ui = u
			if s.claude {
				g.work("k", "kinvault", "fix the race", 3)
			}
			if builtin {
				e := *s.e
				u.edge = &e
			} else {
				edgeSays(u, toA(s.e), nil)
				g.a.cfg = u.cfg
			}
			u.pinned = s.pinned
			g.run(2500 * time.Millisecond)
			return u.cv.img
		}
		a, b := render(true), render(false)
		practising := s.e.StartedMs > 0 && !s.pinned && !s.claude
		if string(a.Pix) != string(b.Pix) && !practising { // the built-in drew practise's timer in no colour at all: the plugin's shows it
			bad++
			t.Errorf("scene %d (%s, pinned %v, claude %v): the plugin's pixels differ", i, s.name, s.pinned, s.claude)
			if dir != "" {
				savePNG(t, filepath.Join(dir, "equiv-"+s.name+"-a.png"), a)
				savePNG(t, filepath.Join(dir, "equiv-"+s.name+"-b.png"), b)
			}
		}
	}
	t.Logf("%d scenes, %d differ", len(scenes), bad)

	// edge's cards
	card := func(builtin bool) *image.RGBA {
		g := newScene()
		raw := []byte(`{"type":"card","card":{"id":"offer","mood":"work","title":"Today's problem is waiting","body":"1360E · Polygon, rated 1400. Claude unlocks when it's solved.","buttons":[{"key":"later","label":"Later"},{"key":"start","label":"Start","primary":true}]}}`)
		if builtin {
			(&edgePlugin{}).Message(g.a, raw)
		} else {
			var m adapter.Message
			json.Unmarshal(raw, &m)
			g.a.edgeCard(m.Card)
		}
		g.run(2500 * time.Millisecond)
		return g.a.ui.cv.img
	}
	if string(card(true).Pix) != string(card(false).Pix) {
		t.Error("edge's card through the plugin differs")
	}
}
