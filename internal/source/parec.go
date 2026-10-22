package source

import (
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// The equaliser listens. While a song plays, Ember reads the default
// output's monitor at 8 kHz in stereo (parec, about 32 KB/s) and hears it
// two ways:
//
//   - The bands. Each 40 ms of the mono mix splits into three with one-pole
//     filters: low under ~200 Hz, high over ~2 kHz, mid between. Each band
//     keeps its own slowly falling peak, so quiet songs move the bars as
//     much as loud ones.
//   - The voice. Every 20 ms a 256-point FFT (32 ms, Hann) of left and right
//     together keeps only what sits in the middle of the stereo picture
//     (each bin weighted by how alike its left and right are: a singer is
//     mixed in the centre, guitars and pads to the sides) and only the
//     voice's band, 300–3400 Hz. From that: how loud the voice is, how much
//     it just rose (spectral flux on a log scale: a syllable starting), and
//     how voiced it is (the autocorrelation's peak at a pitch's lag, by
//     Wiener–Khinchin). The singing mouth is placed by these.
//
// Nothing is kept beyond the last second of those numbers. It stops when the
// music does.

// Audio is the song's sound in three bands, while one plays, and the voice
// in it. Its zero value is ready; a nil *Audio hears nothing.
type Audio struct {
	mu   sync.Mutex
	v    [3]float64
	at   time.Time
	cmd  *exec.Cmd
	want bool

	voice [VoiceFrames]VoiceFrame // a ring: the last second of the voice
	seq   uint64                  // frames heard in all; the newest is voice[(seq-1)%VoiceFrames]
	an    *analyser               // FeedPCM's

	// Now is the clock freshness is judged by (nil: the wall's): for replays.
	Now func() time.Time
}

// VoiceFrame is 20 ms of the voice: when it was heard (its last sample),
// how loud (0..1 of the song's recent loudest), how much it rose (0 none,
// 1 a clear onset or more) and how voiced (0..1).
type VoiceFrame struct {
	At     time.Time
	Env    float32
	Flux   float32
	Voiced float32
}

const (
	// AudioRate is the rate Ember listens at, and AudioHop the samples (a
	// channel) in each voice frame.
	AudioRate   = 8000
	AudioHop    = 160 // 20 ms
	VoiceFrames = 64  // the ring: 1.28 s
	bandHops    = 2   // the bands are measured over 40 ms
)

func (a *Audio) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Set starts listening (on) or stops.
func (a *Audio) Set(on bool) {
	if a == nil {
		return
	}
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
	cmd := exec.Command(path, "--device=@DEFAULT_MONITOR@", "--format=s16le", "--rate=8000", "--channels=2",
		"--latency-msec=30", "--client-name=ember", "--stream-name=equaliser")
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM} // it goes when the island does, however it ends
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return
	}
	a.cmd = cmd
	go a.read(cmd, out)
}

func (a *Audio) read(cmd *exec.Cmd, r io.Reader) {
	defer cmd.Wait()
	buf := make([]byte, AudioHop*4)
	pcm := make([]int16, AudioHop*2)
	an := newAnalyser()
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		for i := range pcm {
			pcm[i] = int16(binary.LittleEndian.Uint16(buf[2*i:]))
		}
		a.mu.Lock()
		if a.cmd != cmd {
			a.mu.Unlock()
			return
		}
		a.mu.Unlock()
		a.hear(an, pcm, time.Now())
	}
}

// FeedPCM hears a hop of interleaved stereo (AudioHop frames, left then
// right) as if its last sample played at at: for tests and replays. It runs
// the same analysis as listening does.
func (a *Audio) FeedPCM(pcm []int16, at time.Time) {
	a.mu.Lock()
	if a.an == nil {
		a.an = newAnalyser()
	}
	an := a.an
	a.mu.Unlock()
	for len(pcm) >= 2*AudioHop {
		a.hear(an, pcm[:2*AudioHop], at)
		pcm = pcm[2*AudioHop:]
	}
}

// hear analyses a hop and publishes what it found.
func (a *Audio) hear(an *analyser, pcm []int16, at time.Time) {
	bands, fresh := an.hop(pcm)
	vf := an.vf
	vf.At = at
	a.mu.Lock()
	if fresh {
		a.v, a.at = bands, at
	}
	a.voice[a.seq%VoiceFrames] = vf
	a.seq++
	a.mu.Unlock()
}

// Sample is the three bands and when they were measured, if fresh.
func (a *Audio) Sample() ([3]float64, time.Time, bool) {
	if a == nil {
		return [3]float64{}, time.Time{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v, a.at, a.now().Sub(a.at) < 250*time.Millisecond
}

// Levels are the three bands, if fresh.
func (a *Audio) Levels() ([3]float64, bool) {
	if a == nil {
		return [3]float64{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v, a.now().Sub(a.at) < 250*time.Millisecond
}

// Feed sets the bands as if heard at at: for tests and replays.
func (a *Audio) Feed(v [3]float64, at time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.v, a.at = v, at
}

// Voice copies the voice frames heard after the first since (a count of
// frames, as returned before) into dst, oldest first, and says how many and
// the count now. Frames older than the ring are lost; fresh is false when
// nothing has been heard for 250 ms.
func (a *Audio) Voice(dst *[VoiceFrames]VoiceFrame, since uint64) (n int, seq uint64, fresh bool) {
	if a == nil {
		return 0, since, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	seq = a.seq
	if seq == 0 {
		return 0, 0, false
	}
	if since > seq {
		since = seq
	}
	if seq-since > VoiceFrames {
		since = seq - VoiceFrames
	}
	for s := since; s < seq; s++ {
		dst[n] = a.voice[s%VoiceFrames]
		n++
	}
	return n, seq, a.now().Sub(a.voice[(seq-1)%VoiceFrames].At) < 250*time.Millisecond
}

// ---- the analysis ----

const (
	fftN   = 256
	binHz  = float64(AudioRate) / fftN // 31.25
	binLo  = 10                        // 312 Hz
	binHi  = 109                       // 3406 Hz
	nBins  = binHi - binLo
	lagLo  = 10  // 800 Hz
	lagHi  = 100 // 80 Hz
	logRef = 0.15
)

// analyser is the state the analysis keeps between hops: nothing is
// allocated after it's made.
type analyser struct {
	// the bands
	lp1, lp2 float64
	peak     [3]float64
	e        [3]float64
	hops     int

	// the voice
	l, r    [fftN]float64 // the last fftN samples of each channel, a ring
	w       int           // where the next sample goes
	re, im  [fftN]float64
	prev    [nBins]float64 // last frame's log magnitudes
	envPeak float64        // dB, falling slowly
	fluxM   float64        // flux's running mean and mean deviation
	fluxD   float64
	frames  int
	vf      VoiceFrame
	pw      [fftN]float64
}

var (
	hann    [fftN]float64
	winAC   [lagHi + 1]float64 // the window's own autocorrelation, normalised
	twRe    [fftN / 2]float64
	twIm    [fftN / 2]float64
	bitRev  [fftN]uint16
	tabOnce sync.Once
)

func tables() {
	for i := range hann {
		hann[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/fftN)
	}
	var w2 float64
	for i := range hann {
		w2 += hann[i] * hann[i]
	}
	for lag := range winAC {
		var s float64
		for i := 0; i+lag < fftN; i++ {
			s += hann[i] * hann[i+lag]
		}
		winAC[lag] = s / w2
	}
	for k := range twRe {
		a := -2 * math.Pi * float64(k) / fftN
		twRe[k], twIm[k] = math.Cos(a), math.Sin(a)
	}
	bits := 0
	for 1<<bits < fftN {
		bits++
	}
	for i := range bitRev {
		r := 0
		for b := 0; b < bits; b++ {
			if i&(1<<b) != 0 {
				r |= 1 << (bits - 1 - b)
			}
		}
		bitRev[i] = uint16(r)
	}
}

func newAnalyser() *analyser {
	tabOnce.Do(tables)
	return &analyser{peak: [3]float64{300, 300, 300}, envPeak: -40}
}

// fft transforms re, im in place (radix 2, forward; inverse with conj before and after).
func fft(re, im *[fftN]float64) {
	for i := 0; i < fftN; i++ {
		if j := int(bitRev[i]); j > i {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for size := 2; size <= fftN; size <<= 1 {
		half, step := size/2, fftN/size
		for start := 0; start < fftN; start += size {
			for k := 0; k < half; k++ {
				wr, wi := twRe[k*step], twIm[k*step]
				a, b := start+k, start+k+half
				tr := re[b]*wr - im[b]*wi
				ti := re[b]*wi + im[b]*wr
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a], im[a] = re[a]+tr, im[a]+ti
			}
		}
	}
}

// hop takes AudioHop stereo frames: the bands once every 40 ms (fresh), the
// voice frame (an.vf) every time.
func (an *analyser) hop(pcm []int16) (bands [3]float64, fresh bool) {
	const k1, k2 = 0.145, 0.79 // 1 - exp(-2π·f/8000) for 200 Hz and 2 kHz
	for i := 0; i < AudioHop; i++ {
		l, r := float64(pcm[2*i]), float64(pcm[2*i+1])
		x := math.Trunc((l + r) / 2) // the mono mix, as parec downmixes
		an.lp1 += (x - an.lp1) * k1
		an.lp2 += (x - an.lp2) * k2
		lo, mid, hi := an.lp1, an.lp2-an.lp1, x-an.lp2
		an.e[0], an.e[1], an.e[2] = an.e[0]+lo*lo, an.e[1]+mid*mid, an.e[2]+hi*hi
		an.l[an.w], an.r[an.w] = l, r
		an.w = (an.w + 1) & (fftN - 1)
	}
	an.hops++
	if an.hops == bandHops {
		an.hops, fresh = 0, true
		for b := range an.e {
			rms := math.Sqrt(an.e[b] / (AudioHop * bandHops))
			an.peak[b] = math.Max(rms, an.peak[b]*0.995) // the reference falls slowly: quiet songs move too
			an.peak[b] = math.Max(an.peak[b], 300)
			bands[b] = math.Min(1, math.Pow(rms/an.peak[b], 1.4)) // spread: the bars use their whole height
			an.e[b] = 0
		}
	}
	an.voiceFrame()
	return bands, fresh
}

// voiceFrame: the centre of the stereo picture in the voice's band.
func (an *analyser) voiceFrame() {
	re, im := &an.re, &an.im
	for i := 0; i < fftN; i++ { // left real, right imaginary: one FFT for both
		j := (an.w + i) & (fftN - 1)
		re[i], im[i] = an.l[j]*hann[i]/32768, an.r[j]*hann[i]/32768
	}
	fft(re, im)
	var pow, flux float64
	ref := logRef * math.Sqrt(math.Pow(10, an.envPeak/10)/nBins) // a bin 40 dB under the voice's recent loudest
	for i := range an.pw {
		an.pw[i] = 0
	}
	for k := binLo; k < binHi; k++ {
		zr, zi := re[k], im[k]
		cr, ci := re[fftN-k], -im[fftN-k] // conj(Z[N-k])
		lr, li := (zr+cr)/2, (zi+ci)/2    // L = (Z + conj Z[N-k]) / 2
		rr, ri := (zi-ci)/2, -(zr-cr)/2   // R = (Z − conj Z[N-k]) / 2i
		mr, mi := (lr+rr)/2, (li+ri)/2
		ll, rp := lr*lr+li*li, rr*rr+ri*ri
		psi := 0.0 // how alike: 1 in the centre, 0 hard to one side or out of phase
		if s := ll + rp; s > 1e-20 {
			psi = math.Max(0, 2*(lr*rr+li*ri)/s)
		}
		p2 := psi * psi
		w := p2 * p2 * p2 // ψ⁶: a centred voice keeps all, a half-panned guitar loses ~12 dB
		p := w * (mr*mr + mi*mi)
		pow += p
		an.pw[k], an.pw[fftN-k] = p, p
		lc := math.Log1p(math.Sqrt(p) / ref)
		if d := lc - an.prev[k-binLo]; d > 0 {
			flux += d
		}
		an.prev[k-binLo] = lc
	}
	flux /= nBins

	// voicing: the autocorrelation of the centre's power spectrum
	for i := range an.pw {
		re[i], im[i] = an.pw[i], 0
	}
	fft(re, im) // the spectrum is real and even: forward is the inverse, up to scale
	voiced := 0.0
	if r0 := re[0]; r0 > 1e-12 {
		for lag := lagLo; lag <= lagHi; lag++ {
			voiced = math.Max(voiced, re[lag]/r0/winAC[lag])
		}
	}

	db := 10 * math.Log10(pow+1e-12)
	an.envPeak = math.Max(db, an.envPeak-3.0/50) // falls 3 dB a second
	an.envPeak = math.Max(an.envPeak, -50)
	env := math.Max(0, math.Min(1, 1+(db-an.envPeak)/30))

	// flux against its own running level: an onset stands out from the song's busy-ness
	an.frames++
	if an.frames == 1 {
		an.fluxM, an.fluxD = flux, flux*0.5+1e-3
	}
	z := (flux - an.fluxM) / (3*an.fluxD + 0.02)
	an.fluxM += (flux - an.fluxM) * 0.03
	an.fluxD += (math.Abs(flux-an.fluxM) - an.fluxD) * 0.03
	an.vf = VoiceFrame{Env: float32(env), Flux: float32(math.Max(0, math.Min(1.5, z))), Voiced: float32(math.Max(0, math.Min(1, voiced)))}
}
