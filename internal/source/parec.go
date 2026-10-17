package source

import (
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"sync"
	"time"
)

// The equaliser listens. While a song plays, Ember reads the default
// output's monitor at 8 kHz, mono (parec, about 16 KB/s), and splits each
// 40 ms into three bands with one-pole filters: low under ~200 Hz, high over
// ~2 kHz, mid between. Each band keeps its own slowly falling peak, so quiet
// songs move the bars as much as loud ones. Nothing is kept: three numbers,
// overwritten 25 times a second. It stops when the music does.

// Audio is the song's sound in three bands, while one plays. Its zero
// value is ready; a nil *Audio hears nothing.
type Audio struct {
	mu   sync.Mutex
	v    [3]float64
	at   time.Time
	cmd  *exec.Cmd
	want bool
}

const audioChunk = 320 // samples: 40 ms at 8 kHz

// Set starts listening (on) or stops.
func (a *Audio) Set(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if on == a.want {
		return
	}
	a.want = on
	if !on {
		if a.cmd != nil && a.cmd.Process != nil {
			a.cmd.Process.Kill()
		}
		a.cmd = nil
		return
	}
	path, err := exec.LookPath("parec")
	if err != nil {
		return
	}
	cmd := exec.Command(path, "--device=@DEFAULT_MONITOR@", "--format=s16le", "--rate=8000", "--channels=1",
		"--latency-msec=30", "--client-name=ember", "--stream-name=equaliser")
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return
	}
	a.cmd = cmd
	go a.read(cmd, out)
}

func (a *Audio) read(cmd *exec.Cmd, r io.Reader) {
	defer cmd.Wait()
	buf := make([]byte, audioChunk*2)
	var lp1, lp2 float64
	peak := [3]float64{300, 300, 300}
	const k1, k2 = 0.145, 0.79 // 1 - exp(-2π·f/8000) for 200 Hz and 2 kHz
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		var e [3]float64
		for i := 0; i < audioChunk; i++ {
			x := float64(int16(binary.LittleEndian.Uint16(buf[2*i:])))
			lp1 += (x - lp1) * k1
			lp2 += (x - lp2) * k2
			lo, mid, hi := lp1, lp2-lp1, x-lp2
			e[0], e[1], e[2] = e[0]+lo*lo, e[1]+mid*mid, e[2]+hi*hi
		}
		var v [3]float64
		for b := range e {
			rms := math.Sqrt(e[b] / audioChunk)
			peak[b] = math.Max(rms, peak[b]*0.995) // the reference falls slowly: quiet songs move too
			peak[b] = math.Max(peak[b], 300)
			v[b] = math.Min(1, math.Pow(rms/peak[b], 1.4)) // spread: the bars use their whole height
		}
		a.mu.Lock()
		if a.cmd != cmd {
			a.mu.Unlock()
			return
		}
		a.v, a.at = v, time.Now()
		a.mu.Unlock()
	}
}

// Sample is the three bands and when they were measured, if fresh.
func (a *Audio) Sample() ([3]float64, time.Time, bool) {
	if a == nil {
		return [3]float64{}, time.Time{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v, a.at, time.Since(a.at) < 250*time.Millisecond
}

// Levels are the three bands, if fresh.
func (a *Audio) Levels() ([3]float64, bool) {
	if a == nil {
		return [3]float64{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v, time.Since(a.at) < 250*time.Millisecond
}

// Feed sets the bands as if heard at at: for tests and replays.
func (a *Audio) Feed(v [3]float64, at time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.v, a.at = v, at
}
