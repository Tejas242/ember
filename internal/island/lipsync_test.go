package island

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// visemesOf is a line's shapes, syllables split by "|".
func visemesOf(line string) string {
	var b strings.Builder
	for i, s := range lineSyllables(line, nil) {
		if i > 0 {
			b.WriteString("|")
		}
		for _, v := range s.on[:s.nOn] {
			b.WriteString(v.String() + " ")
		}
		b.WriteString(s.v1.String())
		if s.v2 != s.v1 {
			b.WriteString(">" + s.v2.String())
		}
		for _, v := range s.coda[:s.nCoda] {
			b.WriteString(" " + v.String())
		}
	}
	return b.String()
}

func TestWordsToShapes(t *testing.T) {
	for in, want := range map[string]string{
		"maybe":        "MBP E|MBP E",
		"the moon":     "L/TH AI|MBP U/W etc",
		"brighter":     "MBP etc AI>E|etc AI",
		"love":         "L/TH AI FV",
		"window":       "U/W E etc|etc O>U/W",
		"holdin'":      "etc O>U/W L/TH|etc E etc",
		"fly away":     "FV L/TH AI>E|AI|U/W E",
		"मेरा मन":      "MBP E|etc AI|MBP AI etc",
		"कमरा":         "etc AI MBP|etc AI",
		"phir se gao":  "MBP E etc|etc E|etc AI|O",
		"الليل":        "AI|L/TH AI|L/TH E L/TH",
		"かな":           "etc AI|etc AI",
		"Ooh, yeah":    "U/W|etc E|AI",
		"singing slow": "etc E|etc E etc|etc L/TH O>U/W",
	} {
		if got := visemesOf(in); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
	if !romanHindi("tera mera pyaar") || romanHindi("hold me while the city sings") {
		t.Error("romanised Hindi told from English wrongly")
	}
}

// frames feeds the singer frames of a voice that sings at the given
// onsets: a dip before each (a consonant), a rise and a flux peak on it.
func singFrames(s *singer, onsets []float64, from, to float64) {
	for tf := from; tf < to; tf += 0.02 {
		env, flux := float32(0.1), float32(0)
		if tf >= onsets[0]-0.05 {
			env = 0.9
		}
		for _, o := range onsets {
			if d := tf - o; d > -0.07 && d < -0.005 {
				env = 0.25 // the consonant
			} else if d >= -0.005 && d < 0.015 {
				env, flux = 0.8, 1.2
			}
		}
		s.take(tf, source.VoiceFrame{Env: env, Flux: flux})
		s.voiceTrack(tf, float64(env))
	}
}

func TestSingerFollowsOnsets(t *testing.T) {
	var s singer
	s.plan(singLine{text: "paper boats are sailing past my window", at: 10 * time.Second, next: 16 * time.Second, hasNext: true})
	n := len(s.syls)
	var on []float64
	at := 10.12 // a little after the line's time, at a pace the plan doesn't know
	for i := 0; i < n; i++ {
		on = append(on, at)
		at += 0.22 + 0.2*float64(i%3) // uneven: eighths, dotted, quarters
	}
	singFrames(&s, on, 9.5, at+0.3)
	for i := 0; i < n; i++ {
		if math.Abs(s.onsets[i]-on[i]) > 0.045 {
			t.Errorf("syllable %d at %.3f, sung at %.3f", i, s.onsets[i], on[i])
		}
	}
}

// A held note holds the vowel: no onset, no moving on, until it's held far
// past its length; an onset hard on the last one's heels isn't a syllable.
func TestSingerHoldsAndIgnores(t *testing.T) {
	var s singer
	s.plan(singLine{text: "hold me", at: 10 * time.Second, next: 20 * time.Second, hasNext: true})
	singFrames(&s, []float64{10.05, 10.12}, 9.6, 10.6)
	if s.cur != 0 || math.Abs(s.curAt-10.12) > 0.03 {
		t.Fatalf("a second onset 70 ms on: cur %d at %.3f (want the first syllable, its start moved to the stronger)", s.cur, s.curAt)
	}
	held := s.curAt + 1.5*s.mu(0)
	for tf := 10.6; tf < held; tf += 0.02 {
		s.take(tf, source.VoiceFrame{Env: 0.9})
	}
	if s.cur != 0 {
		t.Fatalf("moved on in a held note, at %.2f", s.onsets[1])
	}
}

// Without audio the plan sings: even spacing over the line's time.
func TestSingerPlanAlone(t *testing.T) {
	var s singer
	l := singLine{text: "we were only humming", at: 10 * time.Second, next: 14 * time.Second, hasNext: true, now: time.Now()}
	prev := -1
	for p := 10.0; p < 13.5; p += 0.05 {
		l.posW = p
		s.step(l)
		if s.cur < prev {
			t.Fatal("went back")
		}
		prev = s.cur
	}
	if prev != len(s.syls)-1 {
		t.Fatalf("reached syllable %d of %d", prev, len(s.syls))
	}
}

// The lips close for m, b, p between two open vowels.
func TestLipsClose(t *testing.T) {
	var s singer
	s.plan(singLine{text: "ma ma", at: 10 * time.Second, next: 14 * time.Second, hasNext: true})
	s.cur, s.curAt = 0, 10
	s.timeline(10.1)
	if m := s.target(10.1); m.h < 0.6 {
		t.Fatalf("mid-vowel the mouth is open: %.2f", m.h)
	}
	s.heard = false // the plan: the next m at the vowel's end
	s.timeline(10.0)
	var closeAt float64
	for _, g := range s.segs[:s.nseg] {
		if g.v == vMBP && g.syl == 1 {
			closeAt = (g.a + g.b) / 2
		}
	}
	if closeAt == 0 {
		t.Fatal("no m for the second syllable")
	}
	if m := s.target(closeAt); m.h > 0.07 {
		t.Fatalf("at the m the mouth is %.2f open", m.h)
	}
}

// Singing allocates nothing frame to frame once the line is planned.
func TestSingerAllocs(t *testing.T) {
	a := &source.Audio{}
	now := time.Now()
	a.Now = func() time.Time { return now }
	pcm := make([]int16, 2*source.AudioHop)
	var s singer
	l := singLine{text: "maybe the moon will burn a little brighter", at: 10 * time.Second, next: 14 * time.Second, hasNext: true, audio: a, now: now, posW: 10}
	a.FeedPCM(pcm, now)
	s.step(l)
	n := testing.AllocsPerRun(200, func() {
		now = now.Add(20 * time.Millisecond)
		l.now, l.posW = now, l.posW+0.02
		a.FeedPCM(pcm, now)
		s.step(l)
	})
	if n != 0 {
		t.Fatalf("%v allocations a frame", n)
	}
}

func BenchmarkSingerStep(b *testing.B) {
	a := &source.Audio{}
	now := time.Now()
	a.Now = func() time.Time { return now }
	pcm := make([]int16, 2*source.AudioHop)
	var s singer
	l := singLine{text: "maybe the moon will burn a little brighter", at: 10 * time.Second, next: 14 * time.Second, hasNext: true, audio: a, now: now, posW: 10}
	for i := 0; i < b.N; i++ {
		now = now.Add(20 * time.Millisecond)
		l.now, l.posW = now, 10+math.Mod(float64(i)*0.02, 3)
		b.StopTimer()
		a.FeedPCM(pcm, now) // the analysis is the audio goroutine's, not the frame's
		b.StartTimer()
		s.step(l)
	}
}

// offLine: a line's proposals: the true lag first, then the decoys.
func offLine(lag float64, decoys ...float64) []offProp {
	ps := []offProp{{lag, 1}}
	for _, d := range decoys {
		ps = append(ps, offProp{d, 0.7})
	}
	return ps
}

func TestOffsetLearnerAgrees(t *testing.T) {
	var o offsetLearner
	// lines whose first onset came ~0.5 s late, each with decoys that don't agree
	lines := [][]offProp{
		offLine(0.52, 1.3, -0.4), offLine(0.48, 0.9), offLine(0.55, -1.1, 1.2), offLine(0.50, 0.2, 1.25), offLine(0.47),
	}
	for i, l := range lines {
		o.add(l)
		if i < 3 && o.conf {
			t.Fatalf("believed after %d lines", i+1)
		}
	}
	v, ok := o.target()
	if !ok || math.Abs(v-(-0.5+offBias)) > 0.03 {
		t.Fatalf("target %v %v (est %v, %d lines agree)", v, ok, o.est, o.support)
	}
	// a wrong line now and then doesn't move it
	o.add(offLine(-0.9, 1.1))
	if v2, _ := o.target(); math.Abs(v2-v) > 0.03 {
		t.Fatalf("an outlier moved it: %v -> %v", v, v2)
	}
	// two clusters as heavy as each other: not believed
	var p offsetLearner
	for i := 0; i < 6; i++ {
		p.add([]offProp{{0.3, 1}, {-0.6, 1}})
	}
	if p.conf {
		t.Fatal("believed with two clusters")
	}
	// small and huge
	var q offsetLearner
	for i := 0; i < 5; i++ {
		q.add(offLine(0.07))
	}
	if v, ok := q.target(); !ok || v != 0 {
		t.Fatalf("a few tens of ms should be left alone: %v", v)
	}
}

func TestOffsetGlides(t *testing.T) {
	var o offsetLearner
	t0 := time.Unix(1000, 0)
	o.glide(-1.0, t0)
	last := 0.0
	for ms := 0; ms <= 4000; ms += 20 {
		v := o.value(t0.Add(time.Duration(ms) * time.Millisecond))
		if d := last - v; d > 0.02 { // ~0.5 s a second at most: the words' clock never runs back
			t.Fatalf("at %d ms it moved %v in 20 ms", ms, d)
		}
		last = v
	}
	if last != -1 {
		t.Fatalf("didn't arrive: %v", last)
	}
}

func TestGapFinderProposes(t *testing.T) {
	var g gapFinder
	at := 10.0
	feed := func(env, flux float64) {
		g.take(at, 0.25, source.VoiceFrame{Env: float32(env), Flux: float32(flux)})
		at += 0.02
	}
	for i := 0; i < 40; i++ { // quiet, a drum that dies away, quiet
		switch {
		case i == 15:
			feed(0.8, 1.5)
		case i > 15 && i < 25:
			feed(0.8*math.Pow(0.75, float64(i-15)), 0)
		default:
			feed(0.05, 0)
		}
	}
	onset := at // the voice comes back and holds
	feed(0.9, 1.2)
	for i := 0; i < 30; i++ {
		feed(0.85, 0.1)
	}
	var ps [offProps]offProp
	// the line is at 0.3 s before the onset on the words' clock, which runs
	// 0.25 s ahead of the player's: on the player's, the voice came 0.05 s
	// after the line's time
	n := g.propose(onset-0.3, 0.25, &ps)
	if n != 1 || math.Abs(ps[0].r-0.05) > 0.001 || ps[0].w != 1 {
		t.Fatalf("%d %+v", n, ps[:n])
	}
}

func TestLearntOffsetAndNudges(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	g := stageScene(t, testLines, mediaKey, "synced", "ready")
	u := g.a.ui
	key := mediaKey(u.track)
	for i := 0; i < 5; i++ {
		u.learnOffset(key, offLine(-0.3, 1.0))
	}
	g.clock = g.clock.Add(5 * time.Second) // eased in
	want := time.Duration((0.3 + offBias) * float64(time.Second))
	if n := u.wordsNudge(key); (n - want).Abs() > time.Millisecond {
		t.Fatalf("learnt %v, want %v", n, want)
	}
	u.nudge, u.offset = nil, offsetLearner{} // the next play
	if n := u.wordsNudge(key); (n - want).Abs() > time.Millisecond {
		t.Fatalf("not kept: %v", n)
	}
	u.nudgeWords(-1) // you nudge from where it is, and yours wins
	if n := u.wordsNudge(key); (n - want - 250*time.Millisecond).Abs() > time.Millisecond {
		t.Fatalf("nudged %v", n)
	}
	for i := 0; i < 8; i++ {
		u.learnOffset(key, offLine(0.6))
	}
	g.clock = g.clock.Add(5 * time.Second)
	if n := u.wordsNudge(key); (n - want - 250*time.Millisecond).Abs() > time.Millisecond {
		t.Fatalf("learning overrode your nudge: %v", n)
	}
}
