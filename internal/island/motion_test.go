package island

import (
	"testing"
	"time"
)

// [ember] motion = "calm" keeps the colours but drops hops, sparks, bands of
// light, the halo and the dance; dance, flourishes and celebrate turn their
// own parts off.
func TestEmberSettings(t *testing.T) {
	clock := time.Date(2026, 10, 20, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	u := a.ui

	// full motion: a Done hops, Claude at work shimmers, music may be danced to
	hookAt(a, "UserPromptSubmit", "k", "kinvault", map[string]any{"prompt": "fix the race in the store"})
	u.stepEmber(1.0/60, clock)
	if !u.danceable() {
		t.Fatal("full motion: no dance")
	}
	u.play("done", 0.8)
	if !u.body.airborne() && u.body.launch == 0 {
		t.Fatal("full motion: a Done doesn't hop")
	}

	// calm
	u.cfg.Calm = true
	u.body = newBody()
	u.stepEmber(1.0/60, clock)
	u.act = nil
	u.play("done", 0.8)
	u.body.sparks(4, 30, tipFrom)
	if u.body.airborne() || u.body.launch != 0 || sparked(u.body) {
		t.Fatal("calm: it still hops or throws sparks")
	}
	if u.danceable() || u.shimmering() {
		t.Fatal("calm: it still dances or shimmers")
	}

	// each switch on its own
	u.cfg.Calm, u.cfg.Dance = false, false
	if u.danceable() {
		t.Fatal("dance = false, yet it dances")
	}
	u.cfg.Flourishes = false
	u.bond = &bond{Last: map[string]time.Time{}}
	if u.flourish("stretch", time.Hour) {
		t.Fatal("flourishes = false, yet it stretches")
	}
	u.cfg.Celebrate = false
	u.body, u.act = newBody(), nil
	u.stepEmber(1.0/60, clock)
	u.play("done", 0.8)
	clock = clock.Add(time.Second)
	u.actStep(clock)
	if u.body.airborne() || u.body.launch != 0 || sparked(u.body) {
		t.Fatal("celebrate = false, yet a Done hops")
	}
}

func sparked(b *body) bool {
	for _, p := range b.parts {
		if p.live {
			return true
		}
	}
	return false
}
