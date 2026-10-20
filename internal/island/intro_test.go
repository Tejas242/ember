package island

import (
	"testing"
	"time"
)

// The first time ever, two seconds in, Ember says hello in the pill, once;
// until it has met Claude, the quiet card says how to start.
func TestIntroduce(t *testing.T) {
	clock := time.Date(2026, 10, 20, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	u := a.ui
	u.start = clock
	u.bond = &bond{Born: clock, Last: map[string]time.Time{}}

	u.introduce(clock.Add(time.Second))
	if u.moment != nil || !u.bond.Introduced.IsZero() {
		t.Fatal("hello before two seconds")
	}
	clock = clock.Add(2 * time.Second)
	u.introduce(clock)
	if u.moment == nil || u.moment.text != "Hi, I'm Ember" || u.moment.until.Sub(clock) != 5*time.Second {
		t.Fatalf("moment %+v", u.moment)
	}
	if u.act == nil || u.act.kind != "hello" || u.bond.Introduced != clock {
		t.Fatal("no hello act, or not remembered")
	}
	clock = clock.Add(5*time.Second + time.Millisecond)
	if !u.expire() || u.moment != nil {
		t.Fatal("hello outstayed its five seconds")
	}
	u.act = nil
	u.introduce(clock.Add(time.Hour))
	if u.moment != nil {
		t.Fatal("said hello twice")
	}

	if got := u.dayFacts(); got != "Start a Claude Code session and I'll follow it here" {
		t.Fatalf("quiet card before Claude: %q", got)
	}
	hookAt(a, "SessionStart", "s", "kinvault", map[string]any{})
	if !u.bond.MetClaude || u.dayFacts() != "Nothing finished with Claude yet today" {
		t.Fatalf("after a session: %v %q", u.bond.MetClaude, u.dayFacts())
	}

	// someone who used Ember before it kept count has met Claude
	if !(&bond{Turns: 3}).metClaude() || (&bond{}).metClaude() {
		t.Fatal("metClaude from turns")
	}
}
