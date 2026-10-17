package island

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// A steady kick: it finds the tempo, locks on, and its beats land on the kicks.
func TestGrooveLocksOnAKick(t *testing.T) {
	var g groove
	start := time.Date(2026, 10, 17, 12, 0, 0, 0, time.Local)
	const period = 0.7
	var off []float64 // how far its own beats are from the kicks, once locked
	for i := 0; i < 20/0.04; i++ {
		now := start.Add(time.Duration(float64(i) * 0.04 * float64(time.Second)))
		el := float64(i) * 0.04
		low := 0.25
		if math.Mod(el, period) < 0.04 {
			low = 0.95
		}
		g.hear([3]float64{low, 0.3, 0.3}, now)
		if g.advance(now) && el > 8 {
			off = append(off, math.Abs(math.Mod(el+period/2, period)-period/2))
		}
	}
	if !g.dancing() || math.Abs(g.period-period) > 0.03 {
		t.Fatalf("not locked on: period %.3f lock %.2f", g.period, g.lock)
	}
	for _, d := range off {
		if d > 0.06 {
			t.Errorf("a beat %.0f ms off the kick", d*1000)
		}
	}
}

// It keeps the beat going through two bars with no kick.
func TestGrooveKeepsTimeThroughADropout(t *testing.T) {
	var g groove
	start := time.Date(2026, 10, 17, 12, 0, 0, 0, time.Local)
	for i := 0; i < 16/0.04; i++ {
		now := start.Add(time.Duration(float64(i) * 0.04 * float64(time.Second)))
		el := float64(i) * 0.04
		low := 0.25
		if math.Mod(el, 0.6) < 0.04 && (el < 8 || el > 12) {
			low = 0.95
		}
		g.hear([3]float64{low, 0.3, 0.3}, now)
		g.advance(now)
		if el > 10 && el < 11 && !g.dancing() {
			t.Fatalf("lost the beat %.1f s into a 4 s dropout", el-8)
		}
	}
}

// A real song (the low, mid and high levels Ember measured, 40 ms apart):
// with Claude working, it locks on and dances most of the time.
func TestGrooveOnARealSong(t *testing.T) {
	data, err := os.ReadFile("testdata/song-levels.json")
	if err != nil {
		t.Fatal(err)
	}
	var lv [][3]float64
	json.Unmarshal(data, &lv)
	g := newScene()
	g.work("k", "kinvault", "fix it", 2)
	g.music(true)
	g.a.ui.audio = &source.Audio{}
	g.run(1500 * time.Millisecond)
	danced := 0
	for _, v := range lv {
		g.a.ui.audio.Feed(v, time.Now())
		for f := 0; f < 2; f++ { // two 20 ms frames a sample, so the song's time is the clock's
			g.clock = g.clock.Add(20 * time.Millisecond)
			g.a.ui.renderDamage()
		}
		if g.a.ui.dancing {
			danced++
		}
	}
	if p := g.a.ui.groove.period; p < 0.6 || p > 0.8 {
		t.Errorf("tempo %.2f s a beat, want the song's ~0.7", p)
	}
	if danced < len(lv)*6/10 {
		t.Errorf("danced in %d of %d samples", danced, len(lv))
	}
}

// Soft music with no kick: no beat to lock onto, so it sways slowly instead of standing still.
func TestGrooveSwaysWithoutABeat(t *testing.T) {
	g := newScene()
	g.music(true)
	g.a.ui.audio = &source.Audio{}
	g.run(1500 * time.Millisecond)
	lo, hi := 0.0, 0.0
	for i := 0; i < 200; i++ { // 8 s of an even, quiet wash
		g.a.ui.audio.Feed([3]float64{0.4, 0.45, 0.35}, time.Now())
		for f := 0; f < 2; f++ {
			g.clock = g.clock.Add(20 * time.Millisecond)
			g.a.ui.renderDamage()
		}
		lo, hi = math.Min(lo, g.a.ui.body.hold), math.Max(hi, g.a.ui.body.hold)
	}
	if g.a.ui.dancing {
		t.Error("found a beat in a song without one")
	}
	if lo > -0.5 || hi < 0.5 {
		t.Errorf("swayed only %.2f..%.2f", lo, hi)
	}
}
