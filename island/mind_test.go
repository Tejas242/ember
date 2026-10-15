package island

import (
	"strings"
	"testing"
	"time"
)

func TestAffectSettlesAndSleeps(t *testing.T) {
	now := time.Date(2026, 10, 13, 2, 0, 0, 0, time.Local) // 2 am: the circadian trough
	m := newMind(now)
	m.kick(now, 0.8, 0.5)
	if m.arousal < 0.85 {
		t.Fatalf("a kick saturates toward 1, got %.2f", m.arousal)
	}
	quiet := senses{}
	for i := 0; i < 600; i++ { // ten quiet minutes, one step a second
		now = now.Add(time.Second)
		m.step(now, quiet)
	}
	if !m.asleep {
		t.Fatalf("a quiet desk at night puts Ember to sleep (arousal %.2f)", m.arousal)
	}
	if m.lids(now.Add(2*time.Second)) > 0.01 {
		t.Fatal("asleep means eyes shut")
	}
	m.kick(now, 0.5, 0)
	now = now.Add(time.Second)
	m.step(now, senses{working: true})
	if m.asleep {
		t.Fatal("work wakes it")
	}
}

func TestAffectIsFrameRateIndependent(t *testing.T) {
	a, b := newMind(time.Unix(0, 0)), newMind(time.Unix(0, 0))
	a.arousal, b.arousal = 0.9, 0.9
	s := senses{working: true}
	for i := 1; i <= 60; i++ {
		a.step(time.Unix(0, 0).Add(time.Duration(i)*time.Second), s)
	}
	b.step(time.Unix(60, 0), s)
	if d := a.arousal - b.arousal; d > 1e-3 || d < -1e-3 { // only the time-of-day baseline drifts within the minute
		t.Fatalf("60 one-second steps %.6f vs one 60-second step %.6f", a.arousal, b.arousal)
	}
}

func TestBlinksArePoisson(t *testing.T) {
	now := time.Unix(0, 0)
	m := newMind(now)
	blinks, was := 0, false
	for i := 0; i < 60*60; i++ { // a minute at 60 fps
		now = now.Add(time.Second / 60)
		on := m.blink(now) > 0.5
		if on && !was {
			blinks++
		}
		was = on
	}
	if blinks < 6 || blinks > 40 {
		t.Fatalf("%d blinks a minute: humans do ~15", blinks)
	}
}

func TestAttentionLooksAway(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.mind = newMind(clock)
	u.track = &Track{Player: "p", Title: "T", Playing: true, PosAt: clock}
	u.sessions["a"] = &Session{ID: "a", Project: "edge", State: "work", Seen: clock}
	if u.primary() != "claude" {
		t.Fatal("Claude working beats music you've been hearing")
	}
	u.watching["a"] = true // you're looking at its terminal
	clock = clock.Add(5 * time.Second)
	if u.primary() != "music" || u.secondary("music") != "claude" {
		t.Fatalf("looking at Claude's terminal: the pill shows what you can't see (%s), Claude goes to the bubble", u.primary())
	}
	u.watching["a"] = false
	clock = clock.Add(time.Second)
	if u.primary() != "claude" {
		t.Fatal("look away and Claude is back")
	}
}

func TestAttentionHysteresis(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.mind = newMind(clock)
	u.edge = &EdgeStatus{StartedMs: clock.UnixMilli(), Task: "x"}
	u.track = &Track{Player: "p", Title: "T", Playing: true, PosAt: clock}
	if u.primary() != "edge" { // 0.55 vs 0.40
		t.Fatalf("got %s", u.primary())
	}
	u.mind.novel["music"] = clock // a new track: music 0.9, a clear win
	if u.primary() != "music" {
		t.Fatal("a clear winner takes over at once")
	}
	clock = clock.Add(90 * time.Second) // novelty fades to ~0.58: a close call
	if u.primary() != "music" {
		t.Fatal("a close call doesn't flip the pill")
	}
}

func TestHabitsLearnAndForget(t *testing.T) {
	h := &habits{Counts: map[string]map[string]float64{}}
	pages := []string{"claude", "music", "system"}
	ctx := "work|playing"
	if h.predict(ctx, pages) != "" {
		t.Fatal("no guess without evidence")
	}
	for i := 0; i < 4; i++ {
		h.learn(ctx, "music")
	}
	if h.predict(ctx, pages) != "music" {
		t.Fatal("four times music while Claude works: open on music")
	}
	if h.predict("idle|none", pages) != "" {
		t.Fatal("habits are per context")
	}
	for i := 0; i < 6; i++ {
		h.learn(ctx, "claude")
	}
	if h.predict(ctx, pages) != "claude" {
		t.Fatal("a new habit replaces the old within a few opens")
	}
}

func TestSummarize(t *testing.T) {
	msg := "Let me explain what I did.\n\n## Summary\nI looked at the code.\n- Fixed the race in store.go; all 42 tests pass.\n- Updated the README."
	if got := summarize(msg); !strings.Contains(got, "42 tests pass") {
		t.Fatalf("picked %q", got)
	}
	if summarize("") != "" {
		t.Fatal("nothing from nothing")
	}
}

func TestDoneNotesMerge(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	a := &app{ui: u, cfg: DefaultConfig()}
	a.done(&Session{ID: "1", Project: "edge"}, "ok", clock)
	a.done(&Session{ID: "2", Project: "ember"}, "ok", clock.Add(time.Second))
	if u.moment.verb+" "+u.moment.text != "Done edge, ember" || u.moment.sub != "2 sessions finished" {
		t.Fatalf("got %q / %q", u.moment.text, u.moment.sub)
	}
}

func TestWelcomeBack(t *testing.T) {
	clock := time.Now()
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	a := &app{ui: u, cfg: DefaultConfig()}
	a.presence(true)
	u.mind.missed = append(u.mind.missed, missedNote{"done", "edge", "1"}, missedNote{"wait", "ember", "2"})
	clock = clock.Add(10 * time.Minute)
	a.presence(false)
	if u.moment == nil || u.moment.text != "While you were away" || u.moment.sub != "edge done · ember asked you" || u.moment.focus != "2" {
		t.Fatalf("got %+v", u.moment)
	}
}
