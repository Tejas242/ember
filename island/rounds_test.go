package island

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// edge's status carries the round; the pill follows it through its phases.
func TestRoundPillPhases(t *testing.T) {
	clock := time.Date(2026, 10, 17, 18, 23, 0, 0, time.Local)
	start := time.Date(2026, 10, 17, 20, 5, 0, 0, time.Local)
	u := NewUI(440, 280, 24)
	u.now = func() time.Time { return clock }
	u.mind = newMind(clock)
	var st EdgeStatus
	raw := `{"today":0,"need":3,"streak":4,"round":{"id":"cf:2261","short":"CF 1128","startMs":` +
		jsonInt(start.UnixMilli()) + `,"endMs":` + jsonInt(start.Add(3*time.Hour).UnixMilli()) +
		`,"plan":"in","problems":[{"index":"A"},{"index":"B"},{"index":"C"}]}}`
	if err := json.Unmarshal([]byte(raw), &st); err != nil || st.Round == nil {
		t.Fatal("edge's status should carry the round", err)
	}
	u.edge = &st

	if p := u.primary(); p != "edge" {
		t.Fatalf("a held round two hours out leads the resting pill: %s", p)
	}
	if l := u.mainLine(); l.text != "CF 1128" || l.sub != "in 1h 42m" || len(l.letters) != 0 {
		t.Fatalf("soon: %+v", l)
	}

	clock = start.Add(108 * time.Minute)
	st.Round.Joined, st.Round.Locked = "live", true
	st.Round.Problems[0].State, st.Round.Problems[1].State = "solved", "tried"
	l := u.mainLine()
	if l.timer != "1:12" || !l.lock || len(l.letters) != 3 || l.key != "edge:round:live" {
		t.Fatalf("live: %+v", l)
	}
	u.sessions["s"] = &Session{ID: "s", Project: "x", State: "work", Since: clock, Seen: clock}
	if p := u.attend(clock.Add(10 * time.Second)); p != "edge" {
		t.Fatalf("the round you're in outranks a background session: %s", p)
	}

	clock = start.Add(3*time.Hour + 15*time.Minute)
	st.Round.Settled = true
	st.Round.Problems[1].State, st.Round.Problems[2].State = "upsolve", "upsolve"
	if p := u.attend(clock.Add(time.Minute)); p != "claude" {
		t.Fatalf("over, a working session leads again: %s", p)
	}
	delete(u.sessions, "s")
	if l := u.mainLine(); l.sub != "upsolve B" || l.timer != "" {
		t.Fatalf("over: %+v", l)
	}
}

// The card's row offers the round's one action: open it while it's close,
// upsolve once it's over.
func TestRoundRowButtons(t *testing.T) {
	clock := time.Date(2026, 10, 17, 19, 52, 0, 0, time.Local)
	start := time.Date(2026, 10, 17, 20, 5, 0, 0, time.Local)
	u := NewUI(440, 280, 24)
	u.now = func() time.Time { return clock }
	u.edge = &EdgeStatus{Need: 3, Round: &EdgeRound{ID: "cf:2261", Short: "CF 1128", StartMs: start.UnixMilli(),
		EndMs: start.Add(3 * time.Hour).UnixMilli(), Plan: "in"}}
	u.pinned = true
	settle(u, &clock)
	keys := func() string {
		var k []string
		for _, h := range u.hits {
			k = append(k, h.key)
		}
		return strings.Join(k, " ")
	}
	if k := keys(); !strings.Contains(k, "btn:edge:openround:cf:2261") || !strings.Contains(k, "btn:edge:skipround:cf:2261") {
		t.Fatalf("13 minutes out: open it, or can't make it: %s", k)
	}
	clock = start.Add(3*time.Hour + 20*time.Minute)
	u.edge.Round.Joined, u.edge.Round.Settled = "live", true
	u.edge.Round.Problems = []EdgeProblem{{Index: "A", State: "solved"}, {Index: "B", State: "upsolve"}}
	u.Invalidate()
	settle(u, &clock)
	if k := keys(); !strings.Contains(k, "btn:edge:upsolve") || strings.Contains(k, "openround") {
		t.Fatalf("over: upsolve: %s", k)
	}
}

func jsonInt(n int64) string { b, _ := json.Marshal(n); return string(b) }
