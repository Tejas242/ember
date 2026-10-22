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
