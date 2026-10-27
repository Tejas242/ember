package island

import (
	"math"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// ---- the singing mouth ----
//
// Ember sings the words: the right shape at the right moment.
//
//   - Each line, once: its syllables and their shapes (lipsync_g2p.go), and
//     how long each is likely sung (long vowels and closed syllables longer,
//     the line's last held), spread over the line's time.
//   - Each 20 ms of the song (source/parec.go): the centre of the stereo
//     picture in the voice's band, how loud and how sharply its spectrum
//     rose. A syllable starts where the voice rises out of a dip (a
//     consonant, a breath) as its spectrum changes: onset strength is the
//     flux times that rise, so a drum in the gap, a guitar off to the side
//     or vibrato in a held note barely counts. Its peaks are followed
//     through the line's syllables: the first may start a little before the
//     line's time; a peak is the next syllable's when it comes at least a
//     quarter of the way into the one being sung, against that one's
//     expected length (fully believed past 55%); a stronger peak within
//     130 ms of a syllable's start moves its start there (a voiced consonant
//     rose first); a note held well past twice its length moves on without
//     one; the pace is learnt as it goes. With no audio it's the plan alone:
//     even spacing at the pace the line's time allows. (A forward Viterbi
//     over the same syllables was tried first: its online decisions slipped
//     a whole note at a time on the test song, median 150-300 ms off,
//     against 25 ms for this.)
//   - Each frame: a short timeline of shapes around now: the vowel being
//     sung (from its onset to where the voice dips, or held while it
//     doesn't), its coda, the next syllable's consonants and vowel at the
//     predicted time, a rest in silence. The shapes blend by Cohen–Massaro
//     dominance (each shape's pull falls off exponentially either side of
//     it, consonants faster than vowels; lips closing for m, b, p pull six
//     times as hard as a vowel, so they close even between two open
//     vowels). Openness follows the centre voice's loudness. A critically
//     damped spring (about 25 ms) takes the mouth there.

// mouthShape is a mouth: width (1 a middling one), how open (0..1), how
// round, upper teeth showing, tongue showing, the lower lip tucked under
// the teeth (f, v) and a resting smile.
type mouthShape struct {
	w, h, round, teeth, tongue, tuck, smile float32
}

var visemeShapes = [nVisemes]mouthShape{
	vRest: {w: 0.9, smile: 1},
	vMBP:  {w: 0.85, round: 0.1},
	vFV:   {w: 1, h: 0.25, teeth: 1, tuck: 1},
	vTH:   {w: 1, h: 0.42, round: 0.1, teeth: 0.6, tongue: 1},
	vCH:   {w: 0.75, h: 0.38, round: 0.7, teeth: 1},
	vEtc:  {w: 1.05, h: 0.26, round: 0.1, teeth: 0.8},
	vAI:   {w: 1.2, h: 1, round: 0.15, teeth: 0.35, tongue: 0.4},
	vE:    {w: 1.4, h: 0.55, teeth: 0.75, tongue: 0.1},
	vO:    {w: 0.85, h: 0.85, round: 1, tongue: 0.1},
	vU:    {w: 0.55, h: 0.55, round: 1},
}

// dominance: how hard each shape pulls (α), and how far its pull reaches
// before it (anticipation) and after it (carry-over), in seconds.
var visemeDom = [nVisemes]struct{ a, before, after float64 }{
	vRest: {1.2, 0.08, 0.08},
	vMBP:  {20, 0.02, 0.012},
	vFV:   {4, 0.03, 0.02},
	vTH:   {1.2, 0.035, 0.03},
	vCH:   {2, 0.04, 0.03},
	vEtc:  {0.35, 0.03, 0.03},
	vAI:   {1, 0.06, 0.07},
	vE:    {1, 0.06, 0.07},
	vO:    {1, 0.06, 0.07},
	vU:    {1, 0.06, 0.07},
}

const (
	voiceLag    = 0.046 // a voice frame's centre is this long before it's heard (parec's 30 ms, half the window)
	consDur     = 0.065 // a consonant's length
	minSyl      = 0.14  // the shortest syllable
	onsetSettle = 0.13  // a stronger onset this soon after a syllable's moves its start there
	maxStates   = 96
)

// segment is a shape on the timeline: full pull over [a, b], fading either side.
type segment struct {
	v    viseme
	syl  int16 // whose (-1: a rest)
	a, b float64
	open float32
}

// singer is the singing mouth's state.
type singer struct {
	// the line
	key        string
	keyAt      time.Duration
	syls       []syl
	start, end float64 // the line's window, seconds of the words' clock
	rate       float64 // seconds a unit of syllable weight
	pace       float64 // the plan's pace: rate at the line's start

	// the alignment: state 0 is before the line, 1..n its syllables
	cur     int     // the syllable being sung, -1 before the first
	curAt   float64 // when it began
	onsets  []float64
	peak    float64    // the voice's loudest since the syllable began
	dipAt   float64    // when the voice dipped from that (0: it hasn't)
	quietAt float64    // when the voice went quiet (0: it isn't)
	lastT   float64    // the newest frame taken in
	recent  [6]float64 // the voice's last 120 ms
	oHist   [3]float64 // the onset strength, the last three frames
	fHist   [3]float64 // the flux, the same
	curO    float64    // the onset the syllable began on
	nrec    int
	seq     uint64
	frames  [source.VoiceFrames]source.VoiceFrame
	heard   bool    // the voice is being heard
	byEar   int     // syllables begun on an onset heard, in all
	byClock int     // ... and by the clock alone (none heard in time, or no audio)
	env     float64 // the voice, smoothed

	segs [16]segment
	nseg int

	// the song's offset (lipsync_offset.go): a line's likeliest first onsets
	gaps     gapFinder
	asked    bool // this line has proposed
	proposed bool // a line's proposals wait to be taken
	props    [offProps]offProp
	rise     riseCurve
	nprops   int

	// the mouth: where it is and how fast it's going
	m, mv mouthShape
	at    time.Time
	init  bool
}

// singLine is the line being sung and its window on the words' clock.
type singLine struct {
	text     string
	at, next time.Duration
	hasNext  bool
	posW     float64 // the words' clock now, seconds (without the display lead)
	off      float64 // the offset in that clock (nudged or learnt), seconds: the player's is posW − off
	audio    *source.Audio
	now      time.Time
}

// plan readies the line: its syllables, its pace, the alignment's start.
func (s *singer) plan(l singLine) {
	if s.key != "" && !s.asked {
		s.propose(l.off)
	}
	s.key, s.keyAt, s.asked = l.text, l.at, false
	s.syls = lineSyllables(l.text, s.syls[:0])
	if len(s.syls) > maxStates-2 {
		s.syls = s.syls[:maxStates-2]
	}
	s.start = l.at.Seconds()
	s.end = s.start + 6
	if l.hasNext {
		s.end = l.next.Seconds()
	}
	var tw float64
	for _, y := range s.syls {
		tw += float64(y.w)
	}
	tw = math.Max(tw, 0.5)
	d := math.Min((s.end-s.start)*0.85, tw*0.3)
	d = math.Max(d, tw*0.13)
	s.rate, s.pace = d/tw, d/tw
	s.onsets = s.onsets[:0]
	for range s.syls {
		s.onsets = append(s.onsets, math.NaN())
	}
	s.cur, s.curAt, s.peak, s.dipAt, s.quietAt = -1, s.start, 0, 0, 0
	s.lastT = s.start - 0.6 // the singer may start a little before the line's time
	s.nrec, s.oHist, s.fHist, s.curO = 0, [3]float64{}, [3]float64{}, 0
}

// propose: the line's likeliest first onsets, for the song's offset.
func (s *singer) propose(off float64) {
	s.asked, s.proposed = true, true
	s.nprops = s.gaps.propose(s.start, off, &s.props)
	s.gaps.rise(s.start, &s.rise)
}

// mu is syllable j's expected length.
func (s *singer) mu(j int) float64 { return float64(s.syls[j].w) * s.rate }

// take runs one voice frame through the alignment.
func (s *singer) take(tf float64, f source.VoiceFrame) {
	if tf <= s.lastT {
		return
	}
	s.lastT = tf
	env := float64(f.Env)
	gate := math.Max(0, math.Min(1, (env-0.05)/0.2))
	// a syllable starts as the voice rises out of a dip (a consonant, a
	// breath) and its spectrum changes; flux alone in a held note is vibrato
	lo := env
	for k := 0; k < min(s.nrec, len(s.recent)); k++ {
		lo = math.Min(lo, s.recent[k])
	}
	s.recent[s.nrec%len(s.recent)] = env
	s.nrec++
	rise := math.Max(0, math.Min(1, (env-lo-0.05)/0.35))
	o := math.Min(1, float64(f.Flux)) * gate * math.Min(1, 3*rise)
	s.follow(tf, o, math.Min(1, float64(f.Flux))*gate, env)
	s.voiceTrack(tf, env)
}

// voiceTrack follows the voice through the syllable: its loudest, a dip
// from that (a consonant or the note's end), silence.
func (s *singer) voiceTrack(tf, env float64) {
	if env > s.peak {
		s.peak = env
	}
	if s.cur >= 0 && s.dipAt == 0 && tf-s.curAt > 0.08 && env < s.peak-0.15 {
		s.dipAt = tf
	}
	if env < 0.25 {
		if s.quietAt == 0 {
			s.quietAt = tf
		}
	} else {
		s.quietAt = 0
	}
}

// listen takes in the voice frames heard since last time.
func (s *singer) listen(l singLine) {
	n, seq, fresh := l.audio.Voice(&s.frames, s.seq)
	s.seq = seq
	s.heard = fresh
	if !fresh {
		return
	}
	for _, f := range s.frames[:n] {
		tf := l.posW - l.now.Sub(f.At).Seconds() - voiceLag
		s.gaps.take(tf, l.off, f)
		s.take(tf, f)
		a := 0.015
		if float64(f.Env) < s.env {
			a = 0.06
		}
		s.env += (float64(f.Env) - s.env) * (1 - math.Exp(-0.02/a))
	}
}

// replay runs the frames still in the ring through a new line's alignment.
func (s *singer) replay(l singLine) {
	s.seq = 0
	s.listen(l)
}

// planned: with no audio, the syllable sung at t is where the plan puts it.
func (s *singer) planned(t float64) {
	at := s.start
	j := -1
	for k, y := range s.syls {
		if at > t {
			break
		}
		j = k
		if k > s.cur {
			s.onsets[k] = at
		}
		at += float64(y.w) * s.rate
	}
	if j > s.cur {
		s.byClock += j - s.cur
		s.cur, s.curAt = j, at-float64(s.syls[j].w)*s.rate
	}
}

// timeline lays out the shapes around t: the syllable being sung, its
// end, the next one.
func (s *singer) timeline(t float64) {
	s.nseg = 0
	who := int16(-1)
	add := func(v viseme, a, b float64, open float32) {
		if s.nseg < len(s.segs) {
			s.segs[s.nseg] = segment{v, who, a, b, open}
			s.nseg++
		}
	}
	n := len(s.syls)
	j := s.cur
	if j < 0 { // before the line: at rest, the first syllable coming
		first := s.start
		if s.heard {
			first = math.Max(first, t+0.12)
		}
		add(vRest, math.Inf(-1), first-0.15, 0)
		if n > 0 {
			who = 0
			y := &s.syls[0]
			c := first - float64(y.nOn)*consDur
			for k := 0; k < int(y.nOn); k++ {
				add(y.on[k], c+float64(k)*consDur, c+float64(k+1)*consDur, visemeShapes[y.on[k]].h)
			}
			add(y.v1, first, math.Inf(1), y.o1)
		}
		return
	}
	y := &s.syls[j]
	// the end of the vowel: where the voice dipped, or where the plan says, but never while it's held
	var next *syl
	if j+1 < n {
		next = &s.syls[j+1]
	}
	cons := float64(y.nCoda)
	if next != nil {
		cons += float64(next.nOn)
	}
	end := s.curAt + math.Max(minSyl, s.mu(j)-cons*consDur)
	if s.heard {
		switch {
		case s.dipAt > 0:
			end = math.Min(end, s.dipAt)
		default:
			end = math.Max(end, t+0.1)
		}
	}
	// the syllable before, as it leaves
	c := s.curAt - float64(y.nOn)*consDur
	if j > 0 {
		who = int16(j - 1)
		p := &s.syls[j-1]
		pc := c - float64(p.nCoda)*consDur
		add(p.v2, pc-0.25, pc, p.o2)
		for k := 0; k < int(p.nCoda); k++ {
			add(p.coda[k], pc+float64(k)*consDur, pc+float64(k+1)*consDur, visemeShapes[p.coda[k]].h)
		}
	}
	who = int16(j)
	for k := 0; k < int(y.nOn); k++ {
		add(y.on[k], c+float64(k)*consDur, c+float64(k+1)*consDur, visemeShapes[y.on[k]].h)
	}
	if y.v2 != y.v1 { // a diphthong glides at the end of the note
		g := math.Max(s.curAt+0.1, end-0.16)
		add(y.v1, s.curAt, g, y.o1)
		add(y.v2, g, end, y.o2)
	} else {
		add(y.v1, s.curAt, end, y.o1)
	}
	at := end
	for k := 0; k < int(y.nCoda); k++ {
		add(y.coda[k], at, at+consDur, visemeShapes[y.coda[k]].h)
		at += consDur
	}
	if next != nil {
		who = int16(j + 1)
		for k := 0; k < int(next.nOn); k++ {
			add(next.on[k], at, at+consDur, visemeShapes[next.on[k]].h)
			at += consDur
		}
		if s.heard { // its vowel waits for its onset to be heard
			at = math.Max(at, t+0.06)
		}
		add(next.v1, at, math.Inf(1), next.o1)
	} else {
		who = -1
		add(vRest, at+0.05, math.Inf(1), 0)
	}
	who = -1
	if s.heard && s.quietAt > 0 && t-s.quietAt > 0.06 { // the voice stopped: the mouth rests
		add(vRest, s.quietAt+0.06, math.Inf(1), 0)
	}
}

// target blends the timeline's shapes at t (Cohen–Massaro).
func (s *singer) target(t float64) mouthShape {
	var sum mouthShape
	var wsum float64
	for _, g := range s.segs[:s.nseg] {
		d := visemeDom[g.v]
		w := d.a
		switch {
		case t < g.a:
			w *= math.Exp(-(g.a - t) / d.before)
		case t > g.b:
			w *= math.Exp(-(t - g.b) / d.after)
		}
		if w < 1e-6 {
			continue
		}
		sh := visemeShapes[g.v]
		sh.h = g.open
		fw := float32(w)
		sum.w += fw * sh.w
		sum.h += fw * sh.h
		sum.round += fw * sh.round
		sum.teeth += fw * sh.teeth
		sum.tongue += fw * sh.tongue
		sum.tuck += fw * sh.tuck
		sum.smile += fw * sh.smile
		wsum += w
	}
	if wsum <= 0 {
		return visemeShapes[vRest]
	}
	k := float32(1 / wsum)
	return mouthShape{sum.w * k, sum.h * k, sum.round * k, sum.teeth * k, sum.tongue * k, sum.tuck * k, sum.smile * k}
}

// step: the mouth this frame, singing line l.
func (s *singer) step(l singLine) mouthShape {
	if l.text != s.key || l.at != s.keyAt {
		s.plan(l)
		if l.audio != nil {
			s.replay(l)
		}
	} else if l.audio != nil {
		s.listen(l)
	}
	t := l.posW
	if !s.asked && t > s.start+offWin+0.3 { // its onsets near its time are all judged
		s.propose(l.off)
	}
	if !s.heard {
		s.planned(t)
	}
	s.timeline(t)
	tg := s.target(t)
	gain := float32(0.8)
	if s.heard {
		gain = float32(math.Max(0.3, math.Min(1, 0.25+0.95*s.env)))
	}
	tg.h *= gain

	dt := 0.0
	if s.init {
		dt = math.Max(0, math.Min(0.1, l.now.Sub(s.at).Seconds()))
	} else {
		s.m, s.init = tg, true
	}
	s.at = l.now
	spring := func(x, v *float32, to float32, w float64) {
		xx, vv := float64(*x), float64(*v)
		critical(&xx, &vv, float64(to), w, dt)
		*x, *v = float32(xx), float32(vv)
	}
	wj := 70.0 // the jaw: quick, and quicker to close (lips meet in ~40 ms)
	if tg.h < s.m.h {
		wj = 110
	}
	spring(&s.m.h, &s.mv.h, tg.h, wj)
	spring(&s.m.w, &s.mv.w, tg.w, 55)
	spring(&s.m.round, &s.mv.round, tg.round, 55)
	spring(&s.m.teeth, &s.mv.teeth, tg.teeth, 60)
	spring(&s.m.tongue, &s.mv.tongue, tg.tongue, 60)
	spring(&s.m.tuck, &s.mv.tuck, tg.tuck, 60)
	spring(&s.m.smile, &s.mv.smile, tg.smile, 40)
	m := s.m
	m.h = max(m.h, 0)
	return m
}

// singing says whether the line is still being sung: syllables to go, or
// the voice still carrying the last (a held note).
func (s *singer) singing(l singLine) bool {
	if s.key != l.text || l.at != s.keyAt {
		return false
	}
	if !s.heard {
		return s.cur < len(s.syls)-1 || l.posW-s.curAt < 2*s.mu(len(s.syls)-1)
	}
	return s.cur < len(s.syls)-1 || s.quietAt == 0 || l.posW-s.quietAt < 0.4
}

// follow is the onset follower: an onset (a peak of o) is the next
// syllable's when it comes late enough into the one being sung, against
// that one's expected length; one soon after the last moves its start
// there (a voiced consonant rose first); a note held far past its length
// moves on without one.
func (s *singer) follow(tf float64, o, flux, env float64) {
	s.oHist[2], s.oHist[1], s.oHist[0] = s.oHist[1], s.oHist[0], o
	s.fHist[2], s.fHist[1], s.fHist[0] = s.fHist[1], s.fHist[0], flux
	n := len(s.syls)
	peakAt, po := tf-0.02, s.oHist[1]
	isPeak := po >= followThr && po >= s.oHist[2] && po > s.oHist[0]
	advance := func(at float64, ear bool) {
		j := s.cur + 1
		if j >= n {
			return
		}
		if ear {
			s.byEar++
		} else {
			s.byClock++
		}
		if s.cur >= 0 {
			d := at - s.curAt
			mu := s.mu(s.cur)
			if d > 0.5*mu && d < 2*mu {
				s.rate = 0.7*s.rate + 0.3*d/float64(s.syls[s.cur].w)
			}
		}
		s.onsets[j] = at
		s.cur, s.curAt, s.curO, s.peak, s.dipAt = j, at, po, env, 0
	}
	switch {
	case s.cur < 0:
		if isPeak && peakAt >= s.start-0.1 && peakAt <= s.start+1.5 {
			advance(peakAt, true)
		} else if tf > s.start+0.6 {
			advance(s.start+0.6, false)
		}
	case isPeak:
		d := peakAt - s.curAt
		mu := s.mu(s.cur)
		if d < onsetSettle || (s.cur == 0 && d < 0.32 && po > 1.4*s.curO) { // the line's first: its time is rough
			if po > s.curO {
				s.curAt, s.curO, s.onsets[s.cur] = peakAt, po, peakAt
			}
			return
		}
		k := math.Max(0, math.Min(1, (d/mu-0.25)/0.3))
		if po*k >= followThr {
			advance(peakAt, true)
		}
	case s.fHist[1] >= 0.6 && s.fHist[1] >= s.fHist[2] && s.fHist[1] > s.fHist[0] && peakAt-s.curAt > 1.1*s.mu(s.cur):
		advance(peakAt, true) // overdue, and the spectrum turned without a dip: vowel to vowel (ga-o)
	default:
		if tf-s.curAt > 2.2*s.mu(s.cur)+0.25 && s.cur+1 < n {
			advance(tf, false)
		}
	}
}

// followThr: an onset strength that counts.
const followThr = 0.3
