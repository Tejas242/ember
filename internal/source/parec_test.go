package source

import (
	"math"
	"testing"
	"time"
)

// tone is a hop of a sine at f Hz, its left and right gains l and r, from sample n.
func tone(dst []int16, f, l, r float64, n int) {
	for i := 0; i < AudioHop; i++ {
		x := 8000 * math.Sin(2*math.Pi*f*float64(n+i)/AudioRate)
		dst[2*i], dst[2*i+1] = int16(x*l), int16(x*r)
	}
}

// The voice is heard in the middle of the stereo picture: a tone in the
// centre is loud, the same tone hard to one side is all but gone.
func TestVoiceCentre(t *testing.T) {
	level := func(l, r float64) float64 {
		a := &Audio{}
		an := newAnalyser()
		pcm := make([]int16, 2*AudioHop)
		var e float64
		for h := 0; h < 50; h++ {
			// a loud centred tone first sets the reference; then the test tone
			if h < 25 {
				tone(pcm, 600, 1, 1, h*AudioHop)
			} else {
				tone(pcm, 600, l, r, h*AudioHop)
			}
			a.hear(an, pcm, time.Now())
			e = float64(an.vf.Env)
		}
		return e
	}
	centre, side, half := level(1, 1), level(1, 0), level(1, 0.3)
	if centre < 0.9 || side > 0.05 || half > 0.6 {
		t.Fatalf("centre %.2f, hard left %.2f, panned %.2f: the centre should stand out", centre, side, half)
	}
}

// An onset: silence, then a centred tone, flux jumps on its first frames only.
func TestVoiceOnset(t *testing.T) {
	a := &Audio{}
	an := newAnalyser()
	pcm := make([]int16, 2*AudioHop)
	var flux []float32
	for h := 0; h < 40; h++ {
		if h < 20 {
			clear(pcm)
		} else {
			tone(pcm, 440, 1, 1, h*AudioHop)
		}
		a.hear(an, pcm, time.Now())
		flux = append(flux, an.vf.Flux)
	}
	if flux[20] < 0.5 && flux[21] < 0.5 {
		t.Fatalf("no onset where the tone starts: %v", flux[18:24])
	}
	for _, f := range flux[24:] {
		if f > 0.3 {
			t.Fatalf("flux while the tone holds: %v", flux[24:])
		}
	}
}

// Voice hands over what was heard since, oldest first, and no more than the ring.
func TestVoiceRing(t *testing.T) {
	a := &Audio{}
	pcm := make([]int16, 2*AudioHop)
	at := time.Now()
	a.Now = func() time.Time { return at }
	for i := 0; i < 100; i++ {
		a.FeedPCM(pcm, at.Add(time.Duration(i-100)*20*time.Millisecond))
	}
	var buf [VoiceFrames]VoiceFrame
	n, seq, fresh := a.Voice(&buf, 0)
	if n != VoiceFrames || seq != 100 || !fresh || !buf[0].At.Before(buf[n-1].At) {
		t.Fatalf("n %d seq %d fresh %v", n, seq, fresh)
	}
	if n, _, _ := a.Voice(&buf, seq); n != 0 {
		t.Fatal("nothing new, yet frames")
	}
}

// The analysis allocates nothing once it runs.
func TestAnalysisAllocs(t *testing.T) {
	a := &Audio{}
	an := newAnalyser()
	pcm := make([]int16, 2*AudioHop)
	tone(pcm, 300, 1, 0.8, 0)
	now := time.Now()
	if n := testing.AllocsPerRun(200, func() { a.hear(an, pcm, now) }); n != 0 {
		t.Fatalf("%v allocations a hop", n)
	}
}

// BenchmarkAnalysis: one 20 ms hop (two 256-point FFTs).
func BenchmarkAnalysis(b *testing.B) {
	a := &Audio{}
	an := newAnalyser()
	pcm := make([]int16, 2*AudioHop)
	tone(pcm, 300, 1, 0.8, 0)
	now := time.Now()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		a.hear(an, pcm, now)
	}
}
