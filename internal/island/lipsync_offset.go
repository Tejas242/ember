package island

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// ---- each song's offset, learnt by ear ----
//
// A words file is often off by a few hundred ms (another cut of the song,
// a lead-in, a tapped guess), and then each line's first syllable lands a
// note late or early. Ember learns the offset itself:
//
//   - The voice's frames are watched for a voice coming back after a gap:
//     a sharp rise in the voice's spectrum (flux), out of a quiet 200 ms,
//     that holds for the next 240 ms. A drum is as sharp but dies away; a
//     syllable inside a phrase has no quiet before it. Score: flux ×
//     (held loudness − 0.3) × (1 − the quiet's loudness).
//   - Each line, once its time is 1.8 s past, offers its three best such
//     onsets within 1.5 s of where it now starts: where its first syllable
//     might be. One line alone is often wrong (a drum fill, the line
//     before's tail).
//   - The offset is where the lines agree (a vote, robust to the lines
//     that are wrong): around each proposal, the lines with one within
//     ±80 ms, each weighing its likeliest such proposal's weight (its score
//     over the line's best); the heaviest such window, its median.
//     It's believed once at least 4 lines agree and they outweigh any other
//     cluster (more than 160 ms away) by one line's worth. The line is
//     then set 40 ms before that onset: the flux peaks on the vowel, about
//     80 ms after a line's first consonant (the test song), and the
//     syllable follower forgives a line that's late better than one that's
//     early (20 ms early, a drum just before the voice takes the first
//     syllable in some lines; 30 ms late costs nothing). Clamped to ±1.5 s;
//     under 60 ms it's left alone.
//   - It eases in (a smoothstep, at most about half a second a second, so
//     the words' clock never runs back), is kept per song with the nudges
//     (words.json, "learned") so the next play starts right, and keeps
//     learning as the song plays.
//   - A nudge you gave a song always wins: its offset is neither learnt
//     nor applied.

const (
	offWin      = 1.5  // a line's first onset is looked for this far either side of its time (s)
	offAgree    = 0.08 // lines agree within this
	offBias     = 0.04 // the line is set this long before the gap onset (see below)
	offDead     = 0.06 // an offset this small is left alone
	offMinLines = 4    // lines agreeing before it's believed
	offMargin   = 1.0  // ... and outweighing any other cluster by this
	offProps    = 3    // proposals a line makes
	offLines    = 24   // lines remembered
	offRate     = 0.35 // s of offset a second, on average, as it eases in
	gapRing     = 24   // voice frames the gap finder looks over (480 ms)
	gapJudge    = 12   // it judges the frame this many back (240 ms held after it)
	gapMinScore = 0.15 // a gap onset that counts
	gapMinFlux  = 0.5  // its flux
	gapQuietLen = 9    // frames of quiet looked at before it
	gapHoldFrom = 2    // its held loudness: frames 2..11 after it
)

// gapFinder spots the voice coming back after a gap.
type gapFinder struct {
	lastT float64
	ring  [gapRing]struct{ raw, env, flux float64 }
	n     int
	cands [64]gapCand
	nc    int
}

type gapCand struct{ raw, score float64 }

// offProp is a line's proposal: how far its first onset may have come
// after its time (s), and how likely (its gap onset's score over the
// line's best).
type offProp struct{ r, w float64 }

// take: one voice frame at tf on the words' clock, off of which is the
// offset applied (the player's clock is tf − off).
func (g *gapFinder) take(tf, off float64, f source.VoiceFrame) {
	if tf <= g.lastT {
		return
	}
	if tf-g.lastT > 0.1 { // a hole in what was heard (a seek, a pause): start over
		g.n = 0
	}
	g.lastT = tf
	g.ring[g.n%gapRing] = struct{ raw, env, flux float64 }{tf - off, float64(f.Env), float64(f.Flux)}
	g.n++
	if g.n < gapRing {
		return
	}
	at := func(k int) int { return (g.n - 1 - k) % gapRing } // k frames back
	c := g.ring[at(gapJudge)]
	if c.flux < gapMinFlux || c.flux < g.ring[at(gapJudge+1)].flux || c.flux <= g.ring[at(gapJudge-1)].flux {
		return
	}
	var pre, hold float64
	for k := gapJudge + 2; k < gapJudge+2+gapQuietLen; k++ {
		pre += g.ring[at(k)].env
	}
	for k := gapJudge - gapHoldFrom; k > 0; k-- {
		hold += g.ring[at(k)].env
	}
	pre /= gapQuietLen
	hold /= float64(gapJudge - gapHoldFrom)
	score := math.Min(c.flux, 1.5) * math.Max(0, hold-0.3) * math.Max(0, 1-pre)
	if score >= gapMinScore {
		g.cands[g.nc%len(g.cands)] = gapCand{c.raw, score}
		g.nc++
	}
}

// propose: line at start (the words' clock), off the offset applied now:
// its best gap onsets near its start, as how far each came after the
// line's time on the player's clock.
func (g *gapFinder) propose(start, off float64, dst *[offProps]offProp) int {
	n := 0
	for i := max(0, g.nc-len(g.cands)); i < g.nc; i++ {
		c := g.cands[i%len(g.cands)]
		if math.Abs(c.raw+off-start) > offWin {
			continue
		}
		j := n
		if n < offProps {
			n++
		} else if c.score <= dst[n-1].w {
			continue
		} else {
			j = n - 1
		}
		for ; j > 0 && dst[j-1].w < c.score; j-- {
			dst[j] = dst[j-1]
		}
		dst[j] = offProp{c.raw - start, c.score}
	}
	for j := n - 1; j >= 0; j-- {
		dst[j].w /= dst[0].w
	}
	return n
}

// offsetLearner learns the playing song's offset from its lines' proposals.
type offsetLearner struct {
	key   string
	props [offLines][offProps]offProp
	np    [offLines]int
	lines int // lines heard in all

	est     float64 // the onsets' lag behind the lines' times (s)
	support int     // lines agreeing on it
	conf    bool

	from, to float64 // the applied offset (s, + the words earlier) eases from → to
	at       time.Time
	dur      float64
}

// agree: the lines with a proposal within offAgree of c, how much they
// weigh (each line's likeliest such proposal), and the median of those.
func (o *offsetLearner) agree(c float64) (n int, w, med float64) {
	var near [offLines]float64
	for i := 0; i < min(o.lines, offLines); i++ {
		best := offProp{w: -1}
		for _, p := range o.props[i][:o.np[i]] {
			if math.Abs(p.r-c) <= offAgree && p.w > best.w {
				best = p
			}
		}
		if best.w >= 0 {
			near[n] = best.r
			w += best.w
			n++
		}
	}
	if n == 0 {
		return 0, 0, c
	}
	s := near[:n]
	for i := 1; i < n; i++ { // a few numbers: insertion sort, nothing allocated
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	if n%2 == 1 {
		return n, w, s[n/2]
	}
	return n, w, (s[n/2-1] + s[n/2]) / 2
}

// estimate: where the lines agree most, and whether that's believed. The
// windows tried are centred on each proposal and half a window either
// side of it.
func (o *offsetLearner) estimate() {
	bestC, bestW, bestN, med := 0.0, -1.0, 0, 0.0
	for i := 0; i < min(o.lines, offLines); i++ {
		for _, p := range o.props[i][:o.np[i]] {
			for _, c := range [3]float64{p.r - offAgree/2, p.r, p.r + offAgree/2} {
				if n, w, m := o.agree(c); w > bestW {
					bestC, bestW, bestN, med = c, w, n, m
				}
			}
		}
	}
	other := 0.0 // the next cluster, well away from it
	for i := 0; i < min(o.lines, offLines); i++ {
		for _, p := range o.props[i][:o.np[i]] {
			if math.Abs(p.r-bestC) > 2*offAgree {
				if _, w, _ := o.agree(p.r); w > other {
					other = w
				}
			}
		}
	}
	o.est, o.support = med, bestN
	o.conf = bestN >= offMinLines && bestW >= other+offMargin
}

// add takes in a line's proposals.
func (o *offsetLearner) add(props []offProp) {
	i := o.lines % offLines
	o.np[i] = copy(o.props[i][:], props)
	o.lines++
	o.estimate()
}

// target is the offset to apply (s, + the words earlier), when believed.
func (o *offsetLearner) target() (float64, bool) {
	if !o.conf {
		return 0, false
	}
	v := -(o.est - offBias)
	if math.Abs(v) < offDead {
		v = 0
	}
	return math.Max(-offWin, math.Min(offWin, v)), true
}

// value is the offset applied at now: it eases from → to.
func (o *offsetLearner) value(now time.Time) float64 {
	if o.dur <= 0 {
		return o.to
	}
	k := now.Sub(o.at).Seconds() / o.dur
	if k >= 1 {
		return o.to
	}
	k = math.Max(0, k)
	return o.from + (o.to-o.from)*k*k*(3-2*k)
}

// glide eases the applied offset to v from now.
func (o *offsetLearner) glide(v float64, now time.Time) {
	o.from, o.to, o.at = o.value(now), v, now
	o.dur = math.Max(0.5, math.Abs(v-o.from)/offRate)
}

// learnedOffset is the offset Ember has learnt for the song (+ the words earlier).
func (u *UI) learnedOffset(key string) time.Duration {
	if o := &u.offset; o.key == key {
		return time.Duration(o.value(u.now()) * float64(time.Second))
	}
	return time.Duration(u.nudges().Learned[key]) * time.Millisecond
}

// learnOffset takes a sung line's proposals for the song key.
func (u *UI) learnOffset(key string, props []offProp) {
	n := u.nudges()
	if _, manual := n.Ms[key]; manual || key == "" {
		return // you set this song's: yours wins
	}
	o, now := &u.offset, u.now()
	if o.key != key {
		v := float64(n.Learned[key]) / 1000
		*o = offsetLearner{key: key, from: v, to: v}
	}
	o.add(props)
	v, ok := o.target()
	if !ok {
		return
	}
	if math.Abs(v-o.to) >= 0.02 {
		o.glide(v, now)
	}
	ms := int64(math.Round(v * 1000))
	if old, had := n.Learned[key]; (had && math.Abs(float64(ms-old)) >= 20) || (!had && ms != 0) {
		n.remember(key)
		if n.Learned == nil {
			n.Learned = map[string]int64{}
		}
		n.Learned[key] = ms
		n.save()
	}
}

// ---- kept with the nudges ----

func (n *wordsNudges) has(key string) bool {
	_, a := n.Ms[key]
	_, b := n.Learned[key]
	return a || b
}

// remember notes key as a song the nudges know, forgetting the oldest
// past maxNudges.
func (n *wordsNudges) remember(key string) {
	if n.has(key) {
		return
	}
	n.Order = append(n.Order, key)
	if len(n.Order) > maxNudges {
		delete(n.Ms, n.Order[0])
		delete(n.Learned, n.Order[0])
		n.Order = n.Order[1:]
	}
}

func (n *wordsNudges) save() {
	if b, err := json.Marshal(n); err == nil {
		os.MkdirAll(filepath.Dir(nudgesPath()), 0o755)
		save(nudgesPath(), b, 0o644)
	}
}
