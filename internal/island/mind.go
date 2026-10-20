package island

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/xdg"
)

// Ember's mind: the few models that make the island feel alive. All of them
// are cheap, and all are integrated exactly over real elapsed time, so they
// behave the same at 4 frames a second or 60.
//
//   - affect: arousal (energy) and valence (mood), a first-order system that
//     relaxes toward a baseline set by the time of day, by what's running and
//     by whether you're at the machine; events kick it. It drives the flame:
//     flicker, breathing, eyelids, size, colour, the light it casts.
//   - life: blinks are a Poisson process (sometimes doubled), glances come at
//     random, the eyes jitter a little, Ember looks at what just appeared,
//     falls asleep when the desk has been quiet a while, wakes when things happen.
//   - attention: what the pill shows is scored by urgency, novelty (which
//     habituates) and whether you're already looking at it, with hysteresis.
//   - habits: which page you want when the card opens, learned per context.
//   - judgement: the most informative sentence of Claude's reply, notes that
//     arrive together are merged, what happened while you were away.

type mind struct {
	arousal, valence float64
	last             time.Time // when the affect was last integrated
	lastEvent        time.Time // anything happening: Claude, music, you
	rate             float64   // Claude tool calls per minute (exponentially weighted)
	rateAt           time.Time

	away      bool
	awaySince time.Time
	missed    []missedNote

	asleep  bool
	sleepAt time.Time // when it fell asleep or woke: lids ease over a second

	rng        *rand.Rand
	blinkAt    time.Time // start of the current/next blink
	blinkTwice bool
	glanceAt   time.Time // start of the current glance
	glanceLen  float64   // seconds
	glanceDir  float64
	noticeAt   time.Time // looking at something that just appeared
	noticeDir  float64

	choice      string // attention: what the pill shows
	choiceSince time.Time
	novel       map[string]time.Time // when an activity last did something new
}

type missedNote struct {
	kind    string // done | fail | wait
	project string
	session string
}

func newMind(now time.Time) *mind {
	m := &mind{arousal: 0.45, valence: 0.15, last: now, lastEvent: now, rateAt: now,
		rng: rand.New(rand.NewSource(now.UnixNano())), novel: map[string]time.Time{}}
	m.blinkAt = now.Add(m.exp(3.5))
	m.glanceAt = now.Add(m.exp(6))
	return m
}

// exp draws an exponentially distributed duration with the given mean (seconds).
func (m *mind) exp(mean float64) time.Duration {
	return time.Duration(m.rng.ExpFloat64() * mean * float64(time.Second))
}

// mood inputs, read from the UI every step
type senses struct {
	working, waiting, music bool
	hovered                 bool
}

// baseline is where arousal settles: higher while things run, a circadian
// curve (peak mid-afternoon, trough around 3 am), low while you're away.
func (m *mind) baseline(now time.Time, s senses) (arousal, valence float64) {
	h := float64(now.Hour()) + float64(now.Minute())/60
	circ := 0.8 + 0.2*math.Cos(2*math.Pi*(h-15)/24)
	a := 0.3
	if s.working {
		a += 0.35
	}
	if s.waiting {
		a += 0.25
	}
	if s.music {
		a += 0.1
	}
	if s.hovered {
		a += 0.25
	}
	a *= circ
	if m.away {
		a = 0.06
		if s.working { // keeping watch: Claude works while you're away, so Ember stays up
			a = 0.4
		}
	}
	v := 0.15
	if s.music {
		v += 0.15
	}
	return math.Min(a, 1), v
}

// step integrates the affect up to now: x(t) = b + (x0-b)·e^(-dt/τ).
func (m *mind) step(now time.Time, s senses) {
	dt := now.Sub(m.last).Seconds()
	if dt < -1 { // the clock went back (a time change): start the schedules over from here
		m.last, m.lastEvent, m.rateAt = now, now, now
		m.blinkAt, m.glanceAt = now.Add(m.exp(3.5)), now.Add(m.exp(6))
		return
	}
	if dt <= 0 {
		return
	}
	m.last = now
	ba, bv := m.baseline(now, s)
	m.arousal = ba + (m.arousal-ba)*math.Exp(-dt/25)
	m.valence = bv + (m.valence-bv)*math.Exp(-dt/90)
	m.rate *= math.Exp(-now.Sub(m.rateAt).Seconds() / 60)
	m.rateAt = now
	if s.hovered || s.working || s.waiting {
		m.lastEvent = now
	}
	quiet := now.Sub(m.lastEvent) > 3*time.Minute && !s.working && !s.waiting && !s.music
	switch {
	case !m.asleep && quiet && m.arousal < 0.2:
		m.asleep, m.sleepAt = true, now
	case m.asleep && (!quiet || m.arousal > 0.32):
		m.asleep, m.sleepAt = false, now
		m.noticeAt, m.noticeDir = now, 0 // a little start on waking
	}
}

// kick: something happened. Arousal saturates toward 1, valence is clamped.
func (m *mind) kick(now time.Time, da, dv float64) {
	m.arousal += da * (1 - m.arousal)
	m.valence = math.Max(-1, math.Min(1, m.valence+dv))
	m.lastEvent = now
}

// toolCall feeds the activity rate (events per minute).
func (m *mind) toolCall(now time.Time) {
	m.rate *= math.Exp(-now.Sub(m.rateAt).Seconds() / 60)
	m.rateAt = now
	m.rate += 1
	m.kick(now, 0.03, 0.005)
}

// notice: something just appeared to one side (-1 left … 1 right); Ember looks.
func (m *mind) notice(now time.Time, dir float64) {
	m.noticeAt, m.noticeDir = now, dir
}

// ---- the face: lids, blinks, gaze ----

// lids is how open the eyes are, 0..1: drowsy when energy is low, shut asleep.
func (m *mind) lids(now time.Time) float64 {
	open := math.Max(0.35, math.Min(1, (m.arousal-0.08)/0.3))
	k := math.Min(1, now.Sub(m.sleepAt).Seconds()/1.2) // ease into and out of sleep
	if m.asleep {
		return open * (1 - k)
	}
	return open * (0.3 + 0.7*k)
}

// blink is the blink amount now (1 = shut) and schedules the next one.
// Blinks come as a Poisson process; one in five is a double blink.
func (m *mind) blink(now time.Time) float64 {
	const dur = 0.15
	if m.asleep {
		return 0
	}
	t := now.Sub(m.blinkAt).Seconds()
	switch {
	case t < 0:
		return 0
	case t < dur:
		return math.Sin(t / dur * math.Pi)
	case m.blinkTwice && t < 2.2*dur:
		return 0
	case m.blinkTwice && t < 3.2*dur:
		return math.Sin((t - 2.2*dur) / dur * math.Pi)
	}
	m.blinkTwice = m.rng.Float64() < 0.2
	m.blinkAt = now.Add(m.exp(2.6 + 2.5*m.arousal))
	return 0
}

func (m *mind) blinking(now time.Time) bool {
	t := now.Sub(m.blinkAt).Seconds()
	return t >= 0 && t < 0.5
}

// gaze is where the eyes point (-1 … 1), and whether they're moving.
// Something that just appeared wins, then the pointer, then idle glances,
// with a faint jitter on top (eyes are never perfectly still).
func (m *mind) gaze(now time.Time, pointer float64, hovered bool) (float64, bool) {
	jitter := 0.05 * math.Sin(float64(now.UnixMilli())/310) * math.Sin(float64(now.UnixMilli())/770)
	if t := now.Sub(m.noticeAt).Seconds(); t < 1.3 && !m.noticeAt.IsZero() {
		return m.noticeDir*math.Sin(math.Min(1, t/0.25)*math.Pi/2) + jitter, true
	}
	if hovered {
		return pointer + jitter, true
	}
	if m.asleep {
		return 0, false
	}
	t := now.Sub(m.glanceAt).Seconds()
	if t > m.glanceLen {
		if t > m.glanceLen+0.1 { // schedule the next glance
			m.glanceAt = now.Add(m.exp(5 + 4*(1-m.arousal)))
			m.glanceLen = 0.5 + m.rng.Float64()*0.9
			m.glanceDir = (0.45 + 0.55*m.rng.Float64()) * float64(1-2*m.rng.Intn(2))
		}
		return jitter, false
	}
	if t < 0 {
		return jitter, false
	}
	return m.glanceDir*math.Sin(t/m.glanceLen*math.Pi) + jitter, true
}

// ---- attention: what the pill shows ----

// salience scores an activity now: urgency and novelty, less if you're
// already looking at it. Novelty habituates with a 90 s time constant.
func (u *UI) salience(act string, now time.Time) float64 {
	nov := func() float64 {
		if t, ok := u.mind.novel[act]; ok {
			return math.Exp(-now.Sub(t).Seconds() / 90)
		}
		return 0
	}
	if strings.HasPrefix(act, "x:") {
		return u.extSalience(act)
	}
	switch act {
	case "claude":
		ss := u.sessionList()
		if len(ss) == 0 || ss[0].status() != "work" {
			return 0
		}
		s := 0.85 + 0.15*nov()
		if u.watching[ss[0].ID] { // its terminal has your focus: you can see it there
			s = 0.15
		}
		return s
	case "music":
		if u.track == nil || !u.track.Playing {
			return 0
		}
		return 0.4 + 0.5*nov()
	case "edge":
		if e := u.edge; e != nil && e.Round != nil {
			return roundSalience(e.Round, now)
		}
		if e := u.edge; e != nil && (e.StartedMs > 0 || e.Closed) {
			return 0.55
		}
		return 0
	case "clock":
		return 0.25
	}
	return 0
}

// attend picks what the pill shows among the running activities. Leaving
// the resting clock, or a clear win (by 0.3), is immediate; a close call must
// win by 0.12 and the current choice holds at least 4 s, so the pill never
// flickers between two near-equals.
func (u *UI) attend(now time.Time) string {
	best, bestS := "clock", u.salience("clock", now)
	for _, a := range append([]string{"claude", "music", "edge"}, u.extActs()...) {
		if !u.cfg.Enabled(a) {
			continue
		}
		if s := u.salience(a, now); s > bestS {
			best, bestS = a, s
		}
	}
	m := u.mind
	cur := m.choice
	curS := u.salience(cur, now)
	near := bestS < curS+0.3
	if cur != "" && cur != "clock" && curS > 0 && best != cur && near && (bestS < curS+0.12 || now.Sub(m.choiceSince) < 4*time.Second) {
		return cur
	}
	if best != cur {
		m.choice, m.choiceSince = best, now
	}
	return best
}

// ---- habits: which page you want ----

type habits struct {
	Counts map[string]map[string]float64 `json:"counts"`
	dirty  bool
}

func habitsPath() string {
	return filepath.Join(xdg.StateHome(), "ember", "habits.json")
}

func loadHabits() *habits {
	h := &habits{Counts: map[string]map[string]float64{}}
	if b, err := os.ReadFile(habitsPath()); err == nil {
		json.Unmarshal(b, h)
	}
	if h.Counts == nil {
		h.Counts = map[string]map[string]float64{}
	}
	return h
}

func (h *habits) save() {
	if !h.dirty {
		return
	}
	h.dirty = false
	os.MkdirAll(filepath.Dir(habitsPath()), 0o755)
	if b, err := json.Marshal(h); err == nil {
		xdg.AtomicWrite(habitsPath(), b, 0o644)
	}
}

// learn: in this context you ended up on page. Older evidence fades (×0.85
// per observation), so a change in habit shows within a few opens.
func (h *habits) learn(ctx, page string) {
	c := h.Counts[ctx]
	if c == nil {
		c = map[string]float64{}
		h.Counts[ctx] = c
	}
	for k := range c {
		c[k] *= 0.85
	}
	c[page]++
	h.dirty = true
}

// predict returns the page you usually want here, if the evidence is clear:
// enough of it, and one page well ahead.
func (h *habits) predict(ctx string, pages []string) string {
	c := h.Counts[ctx]
	best, bestN, total := "", 0.0, 0.0
	for _, p := range pages {
		total += c[p]
		if c[p] > bestN {
			best, bestN = p, c[p]
		}
	}
	if total < 2.5 || bestN/total < 0.6 {
		return ""
	}
	return best
}

// context is what the desk looks like, coarsely: Claude's state and the music's.
func (u *UI) habitContext() string {
	cl := "none"
	if ss := u.sessionList(); len(ss) > 0 {
		cl = ss[0].status()
	}
	mu := "none"
	if u.track != nil {
		mu = map[bool]string{true: "playing", false: "paused"}[u.track.Playing]
	}
	return cl + "|" + mu
}

// ---- judgement ----

var (
	mdNoise   = regexp.MustCompile("[*_`#>]+")
	outcomeRe = regexp.MustCompile(`(?i)\b(pass(es|ed)?|fail(s|ed)?|fixed|added|removed|renamed|built|creat(ed|es)|updat(ed|es)|merged|works?|ready|done|deployed|install(ed)?|green|broke|error|found|wrote|refactor(ed)?|commit(ted)?)\b`)
	hedgeRe   = regexp.MustCompile(`(?i)^(i'll|i will|let me|sure|okay|ok|here('s| is)|great|now|first|next)\b`)
)

// summarize picks the most informative sentence of Claude's last reply for
// the "Done" note: outcomes and numbers score up, preamble scores down.
func summarize(msg string) string {
	var cands []string
	for _, ln := range strings.Split(msg, "\n") {
		ln = strings.TrimSpace(mdNoise.ReplaceAllString(ln, ""))
		ln = strings.TrimLeft(ln, "-•0123456789. ")
		if ln == "" || strings.HasPrefix(ln, "|") {
			continue
		}
		for _, s := range regexp.MustCompile(`[.!?](\s+|$)`).Split(ln, -1) {
			if s = strings.TrimSpace(s); len(s) > 3 {
				cands = append(cands, s)
			}
		}
		if len(cands) > 24 {
			break
		}
	}
	if len(cands) == 0 {
		return ""
	}
	type scored struct {
		s     string
		score float64
		i     int
	}
	var ss []scored
	for i, c := range cands {
		sc := 0.0
		if outcomeRe.MatchString(c) {
			sc += 2
		}
		if strings.ContainsAny(c, "0123456789") {
			sc += 1.2
		}
		if hedgeRe.MatchString(c) {
			sc -= 2.5
		}
		if n := len(c); n >= 18 && n <= 90 {
			sc += 0.8
		} else if n > 140 {
			sc -= 1
		}
		if strings.HasSuffix(c, ":") {
			sc -= 2
		}
		sc -= 0.15 * float64(i) // earlier is a little better
		ss = append(ss, scored{c, sc, i})
	}
	sort.SliceStable(ss, func(a, b int) bool { return ss[a].score > ss[b].score })
	return ss[0].s
}
