//go:build render

package island

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// EMBER_DEMO=dir go test -tags render -count=1 -run DemoEdge ./island: an evening with edge on the
// island, played from the same messages edge sends over the socket: a
// problem offered and started, a wrong answer, the accept, then a contest
// round counting down, running and settling into upsolves.
func TestDemoEdge(t *testing.T) {
	if os.Getenv("EMBER_DEMO") == "" {
		t.Skip("set EMBER_DEMO")
	}
	t.Setenv("HOME", t.TempDir())
	d := newDemo(t, "edge", demoW, 300)
	t.Setenv("PATH", t.TempDir()) // the buttons must not reach a real edge
	d.g.clock = time.Date(2026, 10, 17, 19, 0, 0, 0, time.Local)
	plug := &edgePlugin{}
	send := func(msg any) {
		raw, _ := json.Marshal(msg)
		d.event(func(g *galleryScene) { plug.Message(g.a, raw) })
	}
	status := func(st EdgeStatus) { send(map[string]any{"type": "status", "status": st}) }
	card := func(id, mood, title, body string, bounce bool, buttons ...map[string]any) {
		send(map[string]any{"type": "card", "card": map[string]any{"id": id, "mood": mood, "title": title,
			"body": body, "bounce": bounce, "buttons": buttons}})
	}
	btn := func(key, label string, primary bool) map[string]any {
		return map[string]any{"key": key, "label": label, "primary": primary}
	}
	jump := func(dur time.Duration) { d.g.clock = d.g.clock.Add(dur) }

	// 19:00: the problem is offered
	status(EdgeStatus{Need: 3, Streak: 4, Closed: true, Task: "2257C · Spying on the Beaver", Pending: true})
	d.run(900 * time.Millisecond)
	card("offer", "work", "Next problem is ready", "Spying on the Beaver · rating 1400\nToday 0/3 · streak 4", false,
		btn("snooze", "In 30 min", false), btn("start", "Start", true))
	d.run(2200 * time.Millisecond)
	d.to("btn:start", 700*time.Millisecond)
	d.run(200 * time.Millisecond)
	d.click()
	started := d.g.clock
	status(EdgeStatus{Need: 3, Streak: 4, Closed: true, Task: "2257C · Spying on the Beaver", StartedMs: started.UnixMilli()})
	d.move(demoW/2+180, 240, 500*time.Millisecond)
	d.run(1000 * time.Millisecond)

	// 19:23: a wrong answer
	jump(23 * time.Minute)
	card("fail", "error", "Wrong answer on test 4", "Spying on the Beaver", false)
	d.run(2200 * time.Millisecond)

	// 19:41: accepted
	jump(18 * time.Minute)
	status(EdgeStatus{Today: 1, Need: 3, Streak: 4, Closed: true})
	card("ac", "happy", "Accepted (solo)", "Spying on the Beaver · 41m of work\ntoday 1/3", true,
		btn("next", "Next problem", false), btn("explain", "Explain-back (2 min)", true))
	d.run(2600 * time.Millisecond)
	d.event(func(g *galleryScene) { g.a.ui.RemoveCard("edge:ac") })

	// 20:03: a round about to start
	jump(22 * time.Minute)
	start := time.Date(2026, 10, 17, 20, 5, 0, 0, time.Local)
	round := &EdgeRound{ID: "cf:2261", Short: "CF 1128", StartMs: start.UnixMilli(), EndMs: start.Add(3 * time.Hour).UnixMilli(),
		Plan: "in", Problems: []EdgeProblem{{Index: "A"}, {Index: "B"}, {Index: "C"}, {Index: "D"}, {Index: "E"}, {Index: "F"}}}
	status(EdgeStatus{Today: 1, Need: 3, Streak: 4, Round: round})
	d.run(1600 * time.Millisecond)

	// live: verdicts land
	jump(14 * time.Minute)
	round.Joined, round.Locked = "live", true
	status(EdgeStatus{Today: 3, Need: 3, Streak: 5, Round: round})
	d.run(900 * time.Millisecond)
	for _, v := range []struct {
		skip  time.Duration
		i     int
		state string
	}{{9 * time.Minute, 0, "solved"}, {31 * time.Minute, 1, "tried"}, {12 * time.Minute, 1, "solved"}, {58 * time.Minute, 2, "tried"}} {
		jump(v.skip)
		round.Problems[v.i].State = v.state
		status(EdgeStatus{Today: 3, Need: 3, Streak: 5, Round: round})
		d.run(1100 * time.Millisecond)
	}

	// 23:15: over, and what's owed
	d.g.clock = start.Add(3*time.Hour + 10*time.Minute)
	round.Settled, round.Locked = true, false
	round.Problems[2].State, round.Problems[3].State = "upsolve", "upsolve"
	status(EdgeStatus{Today: 3, Need: 3, Streak: 5, Round: round})
	card("roundover", "happy", "CF 1128 · solved A B", "Counts for the day. Upsolve next: C D.", true,
		btn("debrief", "Debrief (3 min)", false), btn("upsolve", "Upsolve now", true))
	d.run(2600 * time.Millisecond)
	d.event(func(g *galleryScene) { g.a.ui.RemoveCard("edge:roundover") })
	d.run(1800 * time.Millisecond)
	d.encode("edge")
}
