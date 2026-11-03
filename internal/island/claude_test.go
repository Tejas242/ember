package island

import (
	"strings"
	"testing"
	"time"
)

func TestGoPickOrder(t *testing.T) {
	t0 := time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	mk := func(id, state string, since, ask, unseen time.Time) *Session {
		return &Session{ID: id, State: state, Since: since, AskAt: ask, Unseen: unseen, Outcome: &Outcome{At: unseen}}
	}
	cases := []struct {
		name string
		ss   []*Session
		want string
	}{
		{"nothing", nil, ""},
		{"idle only", []*Session{mk("a", "idle", t0, time.Time{}, time.Time{})}, ""},
		{"oldest ask first", []*Session{
			mk("a", "wait", t0, t0.Add(2*time.Minute), time.Time{}),
			mk("b", "wait", t0, t0.Add(time.Minute), time.Time{}),
			mk("c", "idle", t0, time.Time{}, t0.Add(3*time.Minute))}, "b"},
		{"ask beats done beats work", []*Session{
			mk("w", "work", t0, time.Time{}, time.Time{}),
			mk("d", "idle", t0, time.Time{}, t0)}, "d"},
		{"newest unseen done", []*Session{
			mk("d1", "idle", t0, time.Time{}, t0),
			mk("d2", "idle", t0, time.Time{}, t0.Add(time.Minute))}, "d2"},
		{"newest working", []*Session{
			mk("w1", "work", t0, time.Time{}, time.Time{}),
			mk("w2", "work", t0.Add(time.Minute), time.Time{}, time.Time{})}, "w2"},
	}
	for _, c := range cases {
		u := NewUI(440, 280, 22)
		u.sessions = map[string]*Session{}
		for _, s := range c.ss {
			u.sessions[s.ID] = s
		}
		got := ""
		if s := u.goTarget(); s != nil {
			got = s.ID
		}
		if got != c.want {
			t.Errorf("%s: went to %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAskGrowsThenSettles(t *testing.T) {
	clock := time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	hookAt(a, "UserPromptSubmit", "s", "jobwatch", map[string]any{})
	hookAt(a, "PermissionRequest", "s", "jobwatch", map[string]any{"tool_name": "Bash",
		"tool_input": map[string]any{"command": strings.Repeat("go test ./... ", 8)}})
	l := a.ui.mainLine()
	if l.text != "jobwatch asks" || !l.halo || len([]rune(l.grow)) != growChars || !strings.HasSuffix(l.grow, "…") {
		t.Fatalf("pill %q grow %q (%d)", l.text, l.grow, len([]rune(l.grow)))
	}
	if _, h := a.ui.measure(vCompact); h != float64(a.ui.BarH)+growH {
		t.Fatalf("height %v", h)
	}
	clock = clock.Add(growTime + time.Second)
	if l := a.ui.mainLine(); l.grow != "" || l.text != "jobwatch asks" {
		t.Fatalf("after 5 s: %+v", l)
	}
	if _, h := a.ui.measure(vCompact); h != float64(a.ui.BarH) {
		t.Fatalf("settled height %v", h)
	}
}

func TestBeadsAndCounter(t *testing.T) {
	clock := time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		hookAt(a, "UserPromptSubmit", id, "p"+id, map[string]any{})
	}
	hookAt(a, "Stop", "f", "pf", map[string]any{})
	bs := a.ui.beads(a.ui.sessionList())
	if len(bs) != 6 || beadsW(bs) <= beadsW(bs[:4]) {
		t.Fatalf("beads %v", bs)
	}
	if bs[5] != beadDone {
		t.Fatalf("the finished one is %v", bs[5])
	}
	if beadsW(bs[:4]) != 4*beadD+3*beadGap {
		t.Fatalf("4 beads are %v wide", beadsW(bs[:4]))
	}
}

func TestLimitLine(t *testing.T) {
	now := time.Date(2026, 11, 3, 14, 10, 0, 0, time.Local)
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return now }
	pct := func(v float64) *float64 { return &v }
	reset := now.Add(2 * time.Hour)
	line := func(f limitsFile) limitView { u.lim.f = f; return u.limitLine() }
	if l := line(limitsFile{H5: pct(79), H5At: reset.Unix(), T: now.Unix()}); l.text != "" {
		t.Errorf("79%% shows %q", l.text)
	}
	l := line(limitsFile{H5: pct(84), H5At: reset.Unix(), T: now.Unix()})
	if l.text != "5h 84% · resets 4:10 PM" || l.full {
		t.Errorf("84%%: %+v", l)
	}
	if l := line(limitsFile{H5: pct(100), H5At: reset.Unix(), T: now.Unix()}); !l.full {
		t.Errorf("100%% not red: %+v", l)
	}
	if l := line(limitsFile{H5: pct(90), H5At: now.Add(-time.Minute).Unix(), T: now.Unix()}); l.text != "" {
		t.Errorf("after its reset: %q", l.text)
	}
	l = line(limitsFile{H5: pct(90), H5At: reset.Unix(), T: now.Add(-20 * time.Minute).Unix()})
	if !strings.HasSuffix(l.text, "as of 1:50 PM") {
		t.Errorf("stale: %q", l.text)
	}
	if l := line(limitsFile{H5: pct(50), H5At: reset.Unix(), D7: pct(95), D7At: now.Add(48 * time.Hour).Unix(), T: now.Unix()}); !strings.HasPrefix(l.text, "7d 95% · resets Thu") {
		t.Errorf("7d: %q", l.text)
	}
}

func TestClaudeIsBack(t *testing.T) {
	now := time.Date(2026, 11, 3, 14, 10, 0, 0, time.Local)
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return now }
	full, reset := 100.0, now.Add(time.Hour)
	u.lim.f = limitsFile{H5: &full, H5At: reset.Unix(), T: now.Unix()}
	u.limitsTick(now)
	now = now.Add(30 * time.Minute)
	if u.limitsTick(now); u.moment != nil {
		t.Fatal("announced before the reset")
	}
	now = reset.Add(3 * time.Second)
	u.limitsTick(now)
	if m := u.moment; m == nil || m.text != "Claude's back" || m.until.Sub(now) != backFor {
		t.Fatalf("%+v", m)
	}
	u.moment = nil
	u.limitsTick(now.Add(time.Second))
	if u.moment != nil {
		t.Fatal("announced twice")
	}
}
