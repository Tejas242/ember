package island

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/metrics"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// The island's loop on a fake clock: a frame when the last one still moved
// (the compositor's callback, 60 a second), otherwise the timer nextWake
// asks for. It counts what the live loop would do for a span of each state:
// frames a second, full repaints, allocations, CPU.
//
// EMBER_SIM=1 go test -run TestSimStates -v ./internal/island

type simStats struct {
	frames, full int
	span         time.Duration
	allocs       uint64
	bytes        uint64
	cpu          time.Duration
	gcs          uint32
	gcCPU        float64 // seconds the GC took (the runtime's estimate)
	damage       int     // pixels sent to the compositor
}

func (s simStats) String() string {
	sec := s.span.Seconds()
	return fmt.Sprintf("%6.1f fps  %5.1f full/s  %7.0f allocs/s  %8.1f KB/s  %6.1f ms cpu/s  %5.1f allocs/frame  %d gc  %.1f ms gc/s  %6.0f px/frame",
		float64(s.frames)/sec, float64(s.full)/sec, float64(s.allocs)/sec, float64(s.bytes)/sec/1024,
		float64(s.cpu.Microseconds())/1000/sec, float64(s.allocs)/float64(max(s.frames, 1)), s.gcs, s.gcCPU*1000/sec, float64(s.damage)/float64(max(s.frames, 1)))
}

// simulate runs the loop for d of fake time, measured; each(el) runs
// before every frame (to feed the song).
func simulate(g *galleryScene, d time.Duration, each func(el time.Duration)) simStats {
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	gc0 := gcSeconds()
	t0 := time.Now()
	st := simLoop(g, d, each)
	st.cpu = time.Since(t0)
	st.gcCPU = gcSeconds() - gc0
	runtime.ReadMemStats(&m1)
	st.allocs, st.bytes, st.gcs = m1.Mallocs-m0.Mallocs, m1.TotalAlloc-m0.TotalAlloc, m1.NumGC-m0.NumGC
	return st
}

// simLoop is the loop itself: a frame, then the next one at the frame
// callback (still moving) or the timer nextWake sets.
func simLoop(g *galleryScene, d time.Duration, each func(el time.Duration)) simStats {
	u := g.a.ui
	start, end := g.clock, g.clock.Add(d)
	var st simStats
	for g.clock.Before(end) {
		if each != nil {
			each(g.clock.Sub(start))
		}
		animating, dmg := u.renderDamage()
		st.damage += dmg.Dx() * dmg.Dy()
		st.frames++
		if u.lastFull {
			st.full++
		}
		if animating {
			g.clock = g.clock.Add(time.Second / 60)
		} else {
			g.clock = g.clock.Add(g.a.nextWake(false))
		}
	}
	st.span = g.clock.Sub(start)
	return st
}

// songFeeder feeds the song's levels (looped) as parec would, one block per 40 ms.
func songFeeder(u *UI) func(el time.Duration) {
	lv, last := simSong(), -1
	return func(el time.Duration) {
		n := int(el / (40 * time.Millisecond))
		if n != last {
			last = n
			u.audio.Feed(lv[n%len(lv)], time.Now())
		}
	}
}

type simCase struct {
	name string
	set  func(g *galleryScene) func(el time.Duration)
}

func simCases() []simCase {
	song := func(g *galleryScene) func(el time.Duration) {
		g.a.ui.audio = &source.Audio{}
		g.music(true)
		return songFeeder(g.a.ui)
	}
	return []simCase{
		{"idle", func(g *galleryScene) func(time.Duration) { return nil }},
		{"claude-working", func(g *galleryScene) func(time.Duration) {
			g.work("s", "kinvault", "fix the race in the store's compaction path", 3)
			return nil
		}},
		{"music-paused", func(g *galleryScene) func(time.Duration) { g.music(false); return nil }},
		{"music-dance", song},
		{"music-dance+claude", func(g *galleryScene) func(time.Duration) {
			g.work("s", "kinvault", "fix the race in the store's compaction path", 3)
			return song(g)
		}},
		{"card-open+music", func(g *galleryScene) func(time.Duration) {
			g.a.ui.pinned = true
			return song(g)
		}},
		{"vibe-stage", func(g *galleryScene) func(time.Duration) {
			s := stageScene(&testing.T{}, testLines, mediaKey, "synced", "ready")
			g.clock, g.a = s.clock, s.a
			g.a.ui.now = func() time.Time { return g.clock } // the stage's clock is this scene's
			return songFeeder(g.a.ui)
		}},
		{"asleep", func(g *galleryScene) func(time.Duration) {
			g.a.ui.mind.asleep = true
			return nil
		}},
	}
}

func TestSimStates(t *testing.T) {
	if os.Getenv("EMBER_SIM") == "" {
		t.Skip("set EMBER_SIM")
	}
	t.Setenv("HOME", t.TempDir())
	for _, c := range simCases() {
		g := newScene()
		each := c.set(g)
		g.run(2 * time.Second) // settle into the state
		simulate(g, 20*time.Second, each)
		t.Logf("%-20s %v", c.name, simulate(g, 60*time.Second, each))
		t.Logf("%-20s %s", "", memLine())
	}
}

func simSong() [][3]float64 {
	var lv [][3]float64
	data, _ := os.ReadFile("testdata/song-levels.json")
	json.Unmarshal(data, &lv)
	return lv
}

func gcSeconds() float64 {
	m := []metrics.Sample{{Name: "/cpu/classes/gc/total:cpu-seconds"}}
	metrics.Read(m)
	return m[0].Value.Float64()
}

// memLine is the runtime's view of memory: the live heap, the goal, and all
// it holds (what GOMEMLIMIT counts).
func memLine() string {
	m := []metrics.Sample{{Name: "/gc/heap/live:bytes"}, {Name: "/gc/heap/goal:bytes"}, {Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"}, {Name: "/gc/gomemlimit:bytes"}}
	metrics.Read(m)
	mb := func(i int) float64 { return float64(m[i].Value.Uint64()) / (1 << 20) }
	return fmt.Sprintf("live %.1f MiB, goal %.1f MiB, runtime total %.1f MiB (released %.1f), limit %.0f MiB", mb(0), mb(1), mb(2), mb(3), mb(4))
}

// TestSimWhy: what keeps frames coming in each state (a debugging aid).
func TestSimWhy(t *testing.T) {
	if os.Getenv("EMBER_SIM") == "" {
		t.Skip("set EMBER_SIM")
	}
	t.Setenv("HOME", t.TempDir())
	for _, c := range simCases() {
		g := newScene()
		each := c.set(g)
		g.run(2 * time.Second)
		u := g.a.ui
		why := map[string]int{}
		start := g.clock
		for g.clock.Before(start.Add(30 * time.Second)) {
			if each != nil {
				each(g.clock.Sub(start))
			}
			animating, _ := u.renderDamage()
			now := u.now()
			if animating {
				for k, v := range map[string]bool{"blink": u.mind.blinking(now), "face": u.face.busy(now), "act": u.act != nil, "body": u.body.busy(),
					"travel": u.travelling, "ember": !u.emberSettled(), "tween": u.tweening} {
					if v {
						why["anim:"+k]++
					}
				}
				g.clock = g.clock.Add(time.Second / 60)
				continue
			}
			w := g.a.nextWake(false)
			switch {
			case u.continuous():
				why["cont"]++
			case u.mind.asleep:
				why["asleep"]++
			case u.glancing():
				why["glance"]++
			default:
				why["idle"]++
			}
			g.clock = g.clock.Add(w)
		}
		t.Logf("%-20s %v", c.name, why)
	}
}

// BenchmarkSim: each state's loop for one second of fake time per op, so
// allocs/op is allocations a second, and ns/op the CPU a second costs.
func BenchmarkSim(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	for _, c := range simCases() {
		b.Run(c.name, func(b *testing.B) {
			g := newScene()
			each := c.set(g)
			g.run(2 * time.Second)
			simulate(g, 10*time.Second, each)
			b.ReportAllocs()
			b.ResetTimer()
			var st simStats
			for i := 0; i < b.N; i++ {
				s := simLoop(g, time.Second, each)
				st.frames += s.frames
				st.damage += s.damage
			}
			b.ReportMetric(float64(st.frames)/float64(b.N), "frames/s")
			b.ReportMetric(float64(st.damage)/float64(b.N), "px/s")
		})
	}
}
