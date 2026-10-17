package island

import (
	"math"
	"testing"
	"time"
)

// The body integrates in fixed steps: at 6 frames a second or at 60 it ends
// up in the same place.
func TestBodySameAtAnyFrameRate(t *testing.T) {
	run := func(fps float64) *body {
		b := newBody()
		b.jump(2.6, 0.08)
		b.shove(30)
		dt := 1 / fps
		for i := 0; i < int(fps); i++ { // one second
			b.step(dt, 0)
		}
		return b
	}
	a, c := run(6), run(60)
	for _, d := range []struct {
		name string
		x, y float64
	}{{"y", a.y, c.y}, {"sq", a.sq.x, c.sq.x}, {"lean", a.lean.x, c.lean.x}, {"tip", a.tip.x, c.tip.x}} {
		if math.Abs(d.x-d.y) > 1e-3 {
			t.Errorf("%s differs: %v at 6 fps, %v at 60", d.name, d.x, d.y)
		}
	}
}

// A hop goes up about as high as asked, lands, squashes, and comes to rest.
func TestBodyHopLandsAndRests(t *testing.T) {
	b := newBody()
	b.jump(2.6, 0)
	top, squashed := 0.0, 1.0
	for i := 0; i < 240*2; i++ {
		b.step(physStep, 0)
		top = math.Max(top, b.y)
		squashed = math.Min(squashed, b.sq.x)
	}
	if top < 2.3 || top > 2.9 {
		t.Errorf("hop peaked at %.2f px, want about 2.6", top)
	}
	if squashed > 0.95 {
		t.Errorf("landing barely squashed it (%.3f)", squashed)
	}
	if b.busy() {
		t.Errorf("still busy two seconds after a hop: y %.3f vy %.3f sq %.3f/%.3f", b.y, b.vy, b.sq.x, b.sq.v)
	}
}

// Its slot speeding up is felt as inertia: the body leans back, then settles.
func TestBodyLeansAgainstAcceleration(t *testing.T) {
	b := newBody()
	v := 0.0
	minLean := 0.0
	for i := 0; i < 60; i++ { // the slot accelerates to the right for a quarter second
		v += 4000 * physStep
		b.step(physStep, v)
		minLean = math.Min(minLean, b.lean.x)
	}
	if minLean > -0.5 {
		t.Errorf("pulled right, it leant %.2f px: want it to lag behind (negative)", minLean)
	}
}

// Headphones fall onto its head, bounce, and sit.
func TestPhonesLandAndSit(t *testing.T) {
	b := newBody()
	b.phones.want = true
	for i := 0; i < 240*2; i++ {
		b.step(physStep, 0)
	}
	if !b.phones.seated || math.Abs(b.phones.y) > 0.6 || b.phones.shown < 1 {
		t.Errorf("headphones didn't settle: seated %v y %.2f shown %.2f", b.phones.seated, b.phones.y, b.phones.shown)
	}
	b.phones.want = false
	for i := 0; i < 240; i++ {
		b.step(physStep, 0)
	}
	if b.phones.shown > 0 {
		t.Errorf("taken off, they're still there (%.2f)", b.phones.shown)
	}
}

func TestDoneSize(t *testing.T) {
	for _, c := range []struct {
		took time.Duration
		lo   float64
		hi   float64
	}{{5 * time.Second, 0, 0.01}, {20 * time.Second, 0.15, 0.25}, {80 * time.Second, 0.55, 0.65}, {10 * time.Minute, 1, 1}} {
		if a := doneSize(c.took); a < c.lo || a > c.hi {
			t.Errorf("doneSize(%v) = %.2f, want %.2f..%.2f", c.took, a, c.lo, c.hi)
		}
	}
}

// The gate: a flourish only when calm, one per twenty minutes, five a day.
func TestFlourishGate(t *testing.T) {
	clock := time.Date(2026, 10, 17, 10, 0, 0, 0, time.Local)
	u := NewUI(surfW, surfH, 22)
	u.now = func() time.Time { return clock }
	u.bond = &bond{Last: map[string]time.Time{}}
	if !u.may("yawn", time.Hour) {
		t.Fatal("a calm island refused the first flourish")
	}
	if u.may("stretch", time.Hour) {
		t.Error("two flourishes within twenty minutes")
	}
	clock = clock.Add(25 * time.Minute)
	if u.may("yawn", time.Hour) {
		t.Error("a flourish within its own cooldown")
	}
	u.casting = true
	if u.may("stretch", time.Hour) {
		t.Error("a flourish while the screen is shared")
	}
	u.casting = false
	n := 1
	for i := 0; i < 10; i++ {
		clock = clock.Add(21 * time.Minute)
		if u.may("f"+string(rune('a'+i)), time.Hour) {
			n++
		}
	}
	if n != flourishDaily {
		t.Errorf("%d flourishes in a day, want %d", n, flourishDaily)
	}
}

// Poking: the third puffs it up, the sixth hides it in its coal.
func TestPokes(t *testing.T) {
	clock := time.Date(2026, 10, 17, 10, 0, 0, 0, time.Local)
	u := NewUI(surfW, surfH, 22)
	u.now = func() time.Time { return clock }
	var got []string
	for i := 0; i < 6; i++ {
		u.act = nil
		u.poke()
		got = append(got, u.act.kind)
		clock = clock.Add(300 * time.Millisecond)
	}
	want := []string{"poke", "poke", "puff", "poke", "poke", "hide"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pokes played %v, want %v", got, want)
		}
	}
}

func TestBedtimeLearned(t *testing.T) {
	b := &bond{Last: map[string]time.Time{}}
	night := time.Date(2026, 10, 16, 1, 30, 0, 0, time.Local)
	b.back(night, night.Add(7*time.Hour))
	if !b.nearBedtime(night.Add(20 * time.Minute)) {
		t.Errorf("bedtime %v: 1:50 should be near it", b.Bedtime)
	}
	if b.nearBedtime(night.Add(-3 * time.Hour)) {
		t.Error("22:30 shouldn't be near a 1:30 bedtime")
	}
}
