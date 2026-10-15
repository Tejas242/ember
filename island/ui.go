package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"
)

type Button struct {
	Key, Label      string
	Primary, Danger bool
}

type Option struct{ Label, Desc string }

// Card is something that needs a decision: a permission request, a
// question, an actionable edge nudge.
type Card struct {
	ID       string
	Kind     string // perm | ask | edge
	Mood     mood
	Eyebrow  string // who wants what, in small capitals: "kinvault wants to run"
	Title    string
	Meta     string
	Body     string
	Code     string
	Plus     int
	Minus    int
	Buttons  []Button
	Options  []Option
	Multi    bool
	Footer   string
	Note     string // a quiet line under the detail: what "Always" would allow
	Expires  time.Time
	Priority int
	OnPick   func(key string, picked []int)
	OnExpire func()
	Born     time.Time // when it appeared: the countdown line starts here
	Shell    bool      // Code is a shell command: tint it

	picked map[int]bool
}

type Session struct {
	ID, Project, State, Label string // State: work | wait | idle
	Title                     string // the topic Claude put in its terminal's title
	Prompt                    string // what you asked this turn, first line
	Plus, Minus               int
	Agents                    map[string]*Agent    // subagents running, by agent_id
	Finished                  []*Agent             // subagents that finished this turn, oldest first
	asks                      []agentAsk           // Agent calls waiting for their SubagentStart
	ended                     map[string]time.Time // agents that stopped before their start arrived
	Since, Seen               time.Time
	StepAt                    time.Time // when the current step (Label) began
	Pids                      []int
	Turn                      Turn       // what this prompt has led to so far
	Recap                     *Recap     // the last finished turn
	Unseen                    time.Time  // it finished or stopped while you weren't looking at it
	Asking                    string     // what it waits on you for, when it asks in the terminal
	Bins                      [30]int64  // activity: 30 bins of 20 s (bin numbers)
	Counts                    [30]uint16 // tool calls in each bin
}

const binSecs = 20

// bump counts a tool call in the session's activity history.
func (s *Session) bump(now time.Time) {
	b := now.Unix() / binSecs
	i := b % int64(len(s.Bins))
	if s.Bins[i] != b {
		s.Bins[i], s.Counts[i] = b, 0
	}
	s.Counts[i]++
}

// activity is the last ten minutes of tool calls, oldest first.
func (s *Session) activity(now time.Time) []float64 {
	n := int64(len(s.Bins))
	cur := now.Unix() / binSecs
	out := make([]float64, n)
	for k := int64(0); k < n; k++ {
		b := cur - n + 1 + k
		if i := b % n; s.Bins[i] == b {
			out[k] = float64(s.Counts[i])
		}
	}
	return out
}

// moment is a short-lived compact line: Claude finished, a new track, the
// volume, a charger plugged in, a screenshot.
type moment struct {
	key   string
	act   string
	mood  mood
	verb  string // a word before the text in the mood's colour: Done, Stopped
	text  string
	sub   string
	until time.Time
	focus string   // session to jump to on click
	open  string   // file to open on click
	many  []string // the sessions a "Done" names

	icon  string      // drawn where Ember sits (Ember tucks away)
	tint  color.NRGBA // the icon's colour
	right string      // a value at the right end: "73%", "45"
	meter bool        // a level bar
	level float64     // 0..1
	batt  bool        // a battery glyph
	pct   float64     // its fill, 0..1
	thumb *image.RGBA // a screenshot preview instead of the icon
	osd   bool        // you just pressed a key: shows over everything but a card
	at    time.Time   // when it appeared
}

type view int

const (
	vCompact view = iota
	vCard
	vExpanded
	vDash // a plugin's dashboard
)

type hit struct {
	r   image.Rectangle
	key string
}

type spring struct{ x, v float64 }

// step advances a slightly under-damped spring: quick, with a whisper of overshoot.
func (s *spring) step(target, dt float64) { s.stepK(target, dt, 320, 31) }

func (s *spring) stepK(target, dt, k, d float64) {
	s.v += ((target-s.x)*k - s.v*d) * dt
	s.x += s.v * dt
}

func (s *spring) settled(target float64) bool {
	return math.Abs(target-s.x) < 0.5 && math.Abs(s.v) < 3
}

// live is a part of the frame that animates by itself (the equaliser, a
// progress bar). Steady frames restore just these rectangles and redraw them.
type live struct {
	r       image.Rectangle
	draw    func(c *canvas, t float64)
	outside bool // it lives outside the island (the halo): it needn't wait for the shape
}

// emberSpot is where the current view wants Ember.
type emberSpot struct {
	x, y, s float32
	mood    mood
	show    bool
}

const (
	fadeIn    = 160 * time.Millisecond
	slideTime = 260 * time.Millisecond
	focusHold = 90 * time.Second // a scroll-picked activity stays this long
)

// UI owns all state; only the main loop touches it.
type UI struct {
	W, H       int
	BarH       float32
	TimeFmt    string
	cv         *canvas // the frame
	dc         *canvas // where views draw: the content layer
	content    *canvas
	prev       *image.RGBA // content of the view we're leaving
	pal        palette
	cfg        Config
	sessions   map[string]*Session
	cards      []*Card
	edge       *EdgeStatus // set by the edge plugin while it runs
	edgeStats  *EdgeStats
	second     time.Time
	opened     map[string]bool      // agents you opened or closed in the card: a:<session>:<agent>
	shownOpen  map[string]bool      // how the card last drew them (the room it had may have closed one)
	allRows    bool                 // the card shows every session, one line each ("Show all"), until it closes
	hero       string               // the session you made the card's hero, until it closes
	dash       string               // the open dashboard's plugin, if any
	dashAt     time.Time            // when it opened: its entrance plays from here
	dashIn     bool                 // the pointer has been on it: leaving closes it
	dashboards map[string]Dashboard // by plugin, while the plugin runs
	track      *Track
	art        *albumArt
	moment     *moment
	under      *moment // a note waiting under an OSD
	power      *power  // the battery, if there is one
	casting    bool    // the screen is being shared
	vol        level   // the default output
	light      float64 // screen brightness 0..1, -1 if there's no backlight

	// interaction
	hover, pinned, suppress bool
	hoverAt, leftAt         time.Time
	mx, my                  float64
	hoverKey, pressKey      string
	downKey                 string // the control under a button that's held down: it acts on release
	downBtn                 int
	staggerAt               time.Time // a click that rearranged the card: what arrives is staggered from here
	pressAt                 time.Time
	hits                    []hit
	onKey                   func(key string)
	focus                   string // activity picked by scrolling / the bubble
	focusAt                 time.Time
	seekRect                image.Rectangle
	seekPreview             float64                    // where a dragged playhead is, 0..1
	drag                    string                     // the control being dragged: sys:vol, sys:light, music:seek
	dragSent                time.Time                  // when the system last heard about the drag
	sliders                 map[string]image.Rectangle // System page: sys:vol, sys:light

	// animation
	sw, sh, bub spring
	swell       spring  // the pill's anticipation under the pointer, 0..1
	prevK       float64 // how visible the view we're leaving was when we left it
	prevAt      time.Time
	haloAt      time.Time  // the last frame something waited on you (the halo fades after)
	meterBump   time.Time  // a key pressed past the meter's end
	audio       *audio     // the song's levels, while one plays
	eqShown     bool       // an equaliser is on screen (it listens, so frames come quicker)
	eqLevel     [3]float64 // its bars, smoothed
	gesture     string     // Ember's reaction to what just happened: nod | shake | wince
	gestureAt   time.Time
	sleepK      float64 // how asleep the pill looks, 0..1
	wasAsleep   bool
	meter       spring // a moment's level bar
	meterKey    string
	tweens      map[string]*tween
	enterAt     map[string]time.Time // when each element of the current view began to arrive
	rolls       map[string]*roll
	frame       int     // full frames drawn: tweens step once per frame
	dt          float64 // this frame's time step
	tweening    bool    // a tween or roll is still moving
	fadeK       float64 // how far the content has faded in (panels fade with it)
	rowK        float64 // how far the element being drawn has arrived
	lastFull    bool    // the last frame was a full repaint (for EMBER_DEBUG)
	slideDir    float64 // pages slide sideways: -1 back, 1 forward
	lastPage    string
	mind        *mind
	day         *Day
	watching    map[string]bool // sessions whose terminal has your focus
	habits      *habits
	openCtx     string // the desk when the card opened
	openAt      time.Time
	lastChip    string
	trailOn     bool
	lastDone    struct {
		project string
		at      time.Time
	}
	ex, ey, es         spring // Ember's position and size, shared by every view
	look               spring
	emberCol           [3]float64
	spot               emberSpot
	lastView           view
	lastKey            string
	trAt               time.Time
	prevAlive          bool
	lineKey, lineText  string
	slideOld           string
	slideAt            time.Time
	start, lastBlink   time.Time
	bounceAt, lastTick time.Time
	glanceAt           time.Time
	minute             time.Time
	dirty              bool
	base               *image.RGBA
	lives              []live
	shape              notch
	bubble             image.Rectangle
	lastBounds         image.Rectangle
	lastEmber          image.Rectangle
	now                func() time.Time
}

func NewUI(w, h int, barH float32) *UI {
	u := &UI{W: w, H: h, BarH: barH, TimeFmt: "3:04 PM", cv: newCanvas(w, h), content: newCanvas(w, h),
		prev: image.NewRGBA(image.Rect(0, 0, w, h)), pal: loadPalette(), cfg: DefaultConfig(), sessions: map[string]*Session{}, dashboards: map[string]Dashboard{}, opened: map[string]bool{},
		now: time.Now, dirty: true, base: image.NewRGBA(image.Rect(0, 0, w, h))}
	u.dc = u.content
	u.light, u.rowK = -1, 1
	u.shownOpen = map[string]bool{}
	u.sliders = map[string]image.Rectangle{}
	u.tweens, u.rolls, u.watching = map[string]*tween{}, map[string]*roll{}, map[string]bool{}
	u.enterAt = map[string]time.Time{}
	u.mind = newMind(u.now())
	u.day = &Day{}
	u.day.roll(u.now())
	u.habits = &habits{Counts: map[string]map[string]float64{}}
	u.start, u.lastBlink, u.glanceAt = u.now(), u.now(), u.now()
	c := u.pal.muted
	u.emberCol = [3]float64{float64(c.R), float64(c.G), float64(c.B)}
	return u
}

func (u *UI) Invalidate() { u.dirty = true }

// React: Ember answers what just happened with its body. It nods when you
// allow, shakes its head when you deny, winces when a step fails.
func (u *UI) React(g string) { u.gesture, u.gestureAt = g, u.now() }

const gestureTime = 460 * time.Millisecond

func (u *UI) gesturing() bool { return u.gesture != "" && u.now().Sub(u.gestureAt) < gestureTime }

// gestureOffset is Ember's displacement and squash for the current reaction.
func (u *UI) gestureOffset(now time.Time) (dx, dy, scale float32) {
	scale = 1
	if u.gesture == "" {
		return
	}
	k := float64(now.Sub(u.gestureAt)) / float64(gestureTime)
	if k >= 1 {
		return
	}
	decay := float32(1 - k)
	switch u.gesture {
	case "nod": // two small dips, the second smaller
		dy = 2.2 * float32(math.Sin(k*2*math.Pi*2)) * decay * decay
		if dy < 0 {
			dy *= 0.4
		}
	case "shake": // side to side, settling
		dx = 2.4 * float32(math.Sin(k*2*math.Pi*3)) * decay
	case "wince": // a quick flinch: smaller, a touch lower, then back
		w := float32(math.Sin(math.Min(1, k*1.6) * math.Pi))
		scale, dy = 1-0.16*w, 1.2*w
	}
	return
}
func (u *UI) Bounce() { u.bounceAt = u.now() }

// ---- cards ----

func (u *UI) AddCard(c *Card) {
	if c.Born.IsZero() {
		c.Born = u.now()
	}
	for i, old := range u.cards {
		if old.ID == c.ID {
			u.cards[i] = c
			return
		}
	}
	u.cards = append(u.cards, c)
	sort.SliceStable(u.cards, func(i, j int) bool { return u.cards[i].Priority > u.cards[j].Priority })
}

func (u *UI) RemoveCard(id string) {
	for i, c := range u.cards {
		if c.ID == id {
			u.cards = append(u.cards[:i], u.cards[i+1:]...)
			return
		}
	}
}

func (u *UI) top() *Card {
	if len(u.cards) == 0 {
		return nil
	}
	return u.cards[0]
}

func (u *UI) expire() bool {
	now, changed := u.now(), false
	kept := u.cards[:0]
	for _, c := range u.cards {
		if !c.Expires.IsZero() && now.After(c.Expires) {
			if c.OnExpire != nil {
				c.OnExpire()
			}
			changed = true
			continue
		}
		kept = append(kept, c)
	}
	u.cards = kept
	if u.moment != nil && now.After(u.moment.until) {
		u.moment, changed = nil, true
		if m := u.under; m != nil {
			u.under = nil
			if m.until.Sub(now) < 2*time.Second {
				m.until = now.Add(2 * time.Second) // long enough to read
			}
			u.moment = m
		}
	}
	return changed
}

// Moment shows a short-lived line in the pill. A key press (osd) doesn't
// cost you a note: the note comes back when the OSD is done.
func (u *UI) Moment(m moment) {
	if cur := u.moment; m.osd && cur != nil && !cur.osd && cur.until.After(u.now()) {
		u.under = cur
	}
	if u.moment == nil || u.moment.key != m.key {
		u.mind.notice(u.now(), 0.6) // Ember looks at the news
	}
	if u.moment == nil || u.moment.key != m.key {
		m.at = u.now()
	} else {
		m.at = u.moment.at
		if m.meter && u.moment.level >= 0.999 && m.level >= 0.999 { // pressed again at the top
			u.meterBump = u.now()
		}
	}
	u.moment = &m
	if m.mood == moodHappy {
		u.Bounce()
	}
}

// sessionList ranks the sessions: waiting, working, idle. Within a rank, one
// whose terminal you aren't looking at comes first (you can see the other),
// then the newest turn. Not the last one heard from: two sessions working at
// once both speak every few seconds, and the pill would swap between them.
func (u *UI) sessionList() []*Session {
	var out []*Session
	for _, s := range u.sessions {
		out = append(out, s)
	}
	rank := map[string]int{"wait": 3, "work": 2, "idle": 1}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := rank[a.status()], rank[b.status()]; ra != rb {
			return ra > rb
		}
		if wa, wb := u.watching[a.ID], u.watching[b.ID]; wa != wb {
			return wb
		}
		if !a.Since.Equal(b.Since) {
			return a.Since.After(b.Since)
		}
		return a.ID < b.ID
	})
	return out
}

// name is what a session is called: its project, unless another session
// works in the same folder. Then the topic Claude gave it tells them apart
// ("ember (Fix the flaky test)"), or failing that a number ("ember 2").
func (u *UI) name(s *Session) string {
	twin, n := false, 1
	for _, o := range u.sessions {
		if o != s && o.Project == s.Project {
			twin = true
			if o.ID < s.ID {
				n++
			}
		}
	}
	switch {
	case !twin:
		return s.Project
	case s.Title != "":
		return s.Project + " (" + s.Title + ")"
	}
	return fmt.Sprintf("%s %d", s.Project, n)
}

// short is a session's name kept short: a twin's topic is cut so the pill
// and an eyebrow keep room for what matters ("ember (Redesign the…)").
func (u *UI) short(s *Session) string {
	n := u.name(s)
	if i := strings.Index(n, " ("); i > 0 && len([]rune(n)) > i+20 {
		t := []rune(strings.TrimSuffix(n[i+2:], ")"))
		if len(t) > 15 {
			t = append([]rune(strings.TrimRight(string(t[:15]), " ")), '…')
		}
		n = n[:i] + " (" + string(t) + ")"
	}
	return n
}

// ---- activities: what can be shown, and what wins ----

// acts lists the activities that have something to say, in a stable order.
func (u *UI) acts() []string {
	var out []string
	if len(u.sessions) > 0 && u.cfg.Claude {
		out = append(out, "claude")
	}
	if u.track != nil && u.cfg.Music {
		out = append(out, "music")
	}
	if e := u.edge; e != nil && (e.Task != "" || e.Closed || e.Need > 0) {
		out = append(out, "edge")
	}
	return out
}

func (u *UI) has(act string) bool {
	for _, a := range u.acts() {
		if a == act {
			return true
		}
	}
	return false
}

// primary decides what the pill shows. A request waiting on you always wins,
// then a moment, then what you picked, then whatever is live.
func (u *UI) primary() string {
	if u.moment != nil && u.moment.osd { // you're pressing a key: answer it
		return "moment"
	}
	ss := u.sessionList()
	if !u.cfg.Claude {
		ss = nil
	}
	if len(ss) > 0 && ss[0].State == "wait" {
		return "claude"
	}
	if u.moment != nil {
		return "moment"
	}
	if u.focus != "" && u.now().Sub(u.focusAt) < focusHold && (u.focus == "clock" || u.has(u.focus)) {
		return u.focus
	}
	return u.attend(u.now()) // the rest is a matter of salience: see mind.go
}

// secondary is what the bubble beside the pill shows, if anything.
func (u *UI) secondary(primary string) string {
	ss := u.sessionList()
	working := u.cfg.Claude && len(ss) > 0 && (ss[0].status() == "work" || ss[0].State == "wait")
	switch {
	case primary != "music" && u.track != nil && u.track.Playing && u.cfg.Music:
		return "music"
	case primary != "claude" && working:
		return "claude"
	case primary != "edge" && u.edge != nil && u.edge.StartedMs > 0:
		return "edge"
	}
	return ""
}

// cycle moves the pill's focus to the next / previous activity.
func (u *UI) cycle(dir int) {
	if u.moment != nil {
		u.moment = nil
	}
	acts := append(u.acts(), "clock")
	cur := u.primary()
	i := 0
	for j, a := range acts {
		if a == cur {
			i = j
		}
	}
	i = (i + dir + len(acts)) % len(acts)
	u.focus, u.focusAt = acts[i], u.now()
}

func (u *UI) currentView() view {
	if u.top() != nil {
		return vCard
	}
	if u.dash != "" && u.dashboards[u.dash] != nil {
		return vDash
	}
	if u.pinned || u.drag != "" || (u.hover && !u.suppress && u.now().Sub(u.hoverAt) >= u.cfg.HoverDelay) ||
		(!u.hover && u.lastView == vExpanded && u.now().Sub(u.leftAt) < 200*time.Millisecond) {
		return vExpanded
	}
	return vCompact
}

// ---- render ----

func easeOut(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return 1 - math.Pow(1-x, 3)
}

// renderDamage draws a frame; it reports whether anything is still moving and
// what changed. Steady frames restore only the live rectangles.
func (u *UI) renderDamage() (animating bool, damage image.Rectangle) {
	now := u.now()
	dt := now.Sub(u.lastTick).Seconds()
	if dt <= 0 || dt > 0.05 {
		dt = 1.0 / 60
	}
	u.lastTick = now
	u.dt = dt
	v := u.currentView()
	key := u.viewKey(v)
	if v != vExpanded {
		u.allRows, u.hero = false, "" // "Show all" and the hero you picked last as long as the card's open
	}
	for _, s := range u.sessions { // an outcome you've now seen leaves the pill
		if !s.Unseen.IsZero() && (u.watching[s.ID] || now.Sub(s.Unseen) > 30*time.Minute || (v == vExpanded && now.Sub(u.trAt) > time.Second)) {
			s.Unseen, u.dirty = time.Time{}, true
		}
	}
	if v != u.lastView || key != u.lastKey {
		u.transition(v, key, now)
		// keep what's on screen to cross-fade it out while the new view grows in,
		// from as visible as it had got (an open interrupted halfway is half there)
		copy(u.prev.Pix, u.content.img.Pix)
		u.prevAlive = !u.trAt.IsZero()
		u.prevK, u.prevAt = easeOut(float64(now.Sub(u.trAt))/float64(fadeIn)), now
		if v != u.lastView {
			u.resetMotion()
		}
		u.lastView, u.lastKey, u.trAt, u.dirty = v, key, now, true
	}
	if u.mind.asleep != u.wasAsleep { // falling asleep or waking dims the pill or lights it
		u.wasAsleep, u.dirty = u.mind.asleep, true
	}
	if v == vCompact { // a chip that counts minutes changes without any event
		if c := u.compactLine().tick(); c != u.lastChip {
			u.lastChip, u.dirty = c, true
		}
	}
	if m := now.Truncate(time.Minute); !m.Equal(u.minute) {
		u.minute, u.dirty = m, true
	}
	if sec := now.Truncate(time.Second); v == vDash && !sec.Equal(u.second) { // its clocks count seconds
		u.second, u.dirty = sec, true
	}
	tw, th := u.measure(v)
	swell := 0.0
	if v == vCompact && u.hover && !u.suppress { // anticipation: the pill swells under the pointer (its content stays put)
		swell = 1
	}
	u.swell.stepK(swell, dt, 700, 42)
	if u.sw.x == 0 {
		u.sw.x, u.sh.x = tw, th
	}
	// opening, the width leads without overshoot and the drop overshoots a
	// little; closing, the card folds up first, critically damped
	switch grow := th > u.sh.x+0.5; {
	case grow && v == vCard: // a request arrives: an overshoot you catch from the corner of your eye
		u.sw.stepK(tw, dt, 420, 39)
		u.sh.stepK(th, dt, 300, 26)
	case grow:
		u.sw.stepK(tw, dt, 420, 39)
		u.sh.stepK(th, dt, 300, 28)
	case th < u.sh.x-0.5:
		u.sw.stepK(tw, dt, 420, 41)
		u.sh.stepK(th, dt, 600, 49)
	default: // the same height: a pill changing width, with a little life
		u.sw.stepK(tw, dt, 380, 32)
		u.sh.stepK(th, dt, 600, 49)
	}
	bt := 0.0 // (the old bubble beside the pill: gone, the pill has a trailing slot)
	if on := v == vCompact && u.secondary(u.primary()) != ""; on != u.trailOn {
		u.trailOn = on
		if on {
			u.mind.notice(now, 1) // something new at the pill's end: Ember looks
		}
	}
	u.bub.step(bt, dt)
	mt := 0.0
	if m := u.moment; m != nil && m.meter {
		mt = m.level
		if m.key != u.meterKey { // a new meter starts where it is; changes glide
			u.meterKey, u.meter = m.key, spring{x: mt}
		}
	}
	u.meter.stepK(mt, dt, 420, 40)
	settled := u.sw.settled(tw) && u.sh.settled(th) && u.bub.settled(bt) && math.Abs(u.meter.x-mt) < 0.003 &&
		math.Abs(u.swell.x-swell) < 0.01 && math.Abs(u.swell.v) < 0.1
	if settled {
		u.sw.x, u.sh.x, u.sw.v, u.sh.v, u.bub.x, u.bub.v = tw, th, 0, 0, bt, 0
		u.swell.x, u.swell.v = swell, 0
		u.meter.x, u.meter.v = mt, 0
	}
	if v == vCompact && u.sh.x > th+24 { // closing: the pill's words wait until the card has nearly folded
		u.trAt = now
	}
	if v != vCompact && u.lastView == v && u.sw.x < tw*0.75 && now.Sub(u.prevAt) < 300*time.Millisecond {
		u.trAt = now // opening: the card's words wait until the shape is mostly open, so no line starts clipped
	}
	p := easeOut(float64(now.Sub(u.trAt)) / float64(fadeIn))
	leaving := u.prevK * (1 - math.Min(1, float64(now.Sub(u.prevAt))/float64(90*time.Millisecond)))
	sliding := now.Sub(u.slideAt) < slideTime
	flashing := now.Sub(u.pressAt) < 160*time.Millisecond
	t := now.Sub(u.start).Seconds()

	// Ember (blinks, glances, gliding to a new spot) and other live parts only
	// need their own rectangles repainted: that's the cheap path below.
	full := u.dirty || !settled || p < 1 || leaving > 0 || sliding || flashing || u.tweening
	u.lastFull = full
	if full {
		u.dirty, u.tweening = false, false
		u.frame++
		if u.frame%300 == 0 {
			u.gcTweens()
		}
		u.hits, u.lives = u.hits[:0], u.lives[:0]
		u.eqShown = false
		u.spot = emberSpot{}
		// the shape, at its current (springing) size
		w, h := float32(u.sw.x+8*u.swell.x), float32(u.sh.x+2*u.swell.x)
		x := (float32(u.W) - w) / 2
		rb := min(h/2, 12)
		if v != vCompact {
			rb = min(h/2, 20)
		}
		u.shape = notch{x: x, w: w, h: h, rb: rb, ear: 6}
		u.cv.clear(u.cv.img.Rect)
		if v == vCard && len(u.cards) > 1 { // more requests wait: their edges peek out under this one
			u.drawPeeks(u.shape, len(u.cards)-1)
		}
		u.sleepK = u.tw("sleep", map[bool]float64{true: 1}[u.mind.asleep && v == vCompact], 0)
		u.cv.drawNotch(u.pal, u.shape, 1-0.16*u.sleepK, int(u.BarH)) // asleep, the pill dims
		// the content, laid out at its final size, revealed by the growing shape
		u.content.clear(u.content.img.Rect)
		u.fadeK = p
		fx := (float32(u.W) - float32(tw)) / 2
		switch v {
		case vCompact: // the pill's ends ride the live shape: words never sit where the shape isn't
			u.hits = append(u.hits, hit{u.shape.bounds(), "pill"})
			u.drawCompact((float32(u.W)-float32(u.sw.x))/2, float32(u.sw.x), float32(th))
		case vCard:
			u.drawCard(u.top(), fx, float32(tw))
		case vExpanded:
			u.hits = append(u.hits, hit{u.shape.bounds(), "panel"})
			u.drawExpanded(fx, float32(tw))
		case vDash:
			u.hits = append(u.hits, hit{u.shape.bounds(), "dash"})
			if u.dashboards[u.dash].Draw(u, fx, float32(tw), now.Sub(u.dashAt)) {
				u.tweening = true
			}
		}
		if u.prevAlive && leaving > 0 { // the old view leaves quickly, drifting up
			composite(u.cv, u.prev, u.shape, leaving, 0, -int(math.Round(3*(1-leaving/math.Max(u.prevK, 0.01)))))
		}
		composite(u.cv, u.content.img, u.shape, p, 0, 0)
		copy(u.base.Pix, u.cv.img.Pix)
		u.stepEmber(dt, now)
		b := u.shape.bounds().Union(u.bubble).Inset(-10)
		damage = b.Union(u.lastBounds).Intersect(u.cv.img.Rect)
		u.lastBounds = b
		for _, l := range u.lives {
			if settled || l.outside || u.shape.contains(l.r) || u.bubble.Inset(-2).Intersect(l.r) == l.r { // appear once the shape reaches them
				l.draw(u.cv, t)
			}
		}
		u.drawEmber(t, now)
		u.lastEmber = u.emberRect()
		return u.moving(settled, p, sliding, flashing) || leaving > 0, damage
	}
	u.stepEmber(dt, now)
	er := u.emberRect()
	for _, r := range append([]image.Rectangle{er.Union(u.lastEmber)}, u.liveRects()...) {
		r = r.Intersect(u.cv.img.Rect)
		for yy := r.Min.Y; yy < r.Max.Y; yy++ {
			o := yy*u.base.Stride + r.Min.X*4
			copy(u.cv.img.Pix[o:o+r.Dx()*4], u.base.Pix[o:o+r.Dx()*4])
		}
		damage = damage.Union(r)
	}
	for _, l := range u.lives {
		l.draw(u.cv, t)
	}
	u.drawEmber(t, now)
	u.lastEmber = er
	return u.moving(settled, p, sliding, flashing), damage
}

// drawPeeks draws the edges of the requests waiting behind the one shown,
// like a stack of cards: narrower, a little lower, a little lighter.
func (u *UI) drawPeeks(n notch, behind int) {
	for i := min(behind, 2); i >= 1; i-- {
		drop := float32(u.tw(fmt.Sprint("peek:", i), 6*float64(i), 0))
		inset := 14 * float32(i)
		pk := notch{x: n.x + inset, w: n.w - 2*inset, h: n.h + drop, rb: n.rb, ear: 0}
		top := int(n.h - n.rb)
		u.cv.fillNotch(pk, 1, alpha(u.pal.line, 0.9-0.25*float64(i)), &pk, 0, top) // a rim, so it reads as a card
		u.cv.fillNotch(pk, 0, alpha(mix(u.pal.island, u.pal.fg, 0.07+0.03*float64(i)), 0.95-0.2*float64(i)), nil, 0, top)
	}
}

func (u *UI) moving(settled bool, p float64, sliding, flashing bool) bool {
	return !settled || p < 1 || sliding || flashing || u.tweening || u.mind.blinking(u.now()) || u.bouncing() || u.gesturing() || !u.emberSettled()
}

// transition: the view is about to change. Opening the card notes the
// situation, so what you do in it can be learned for next time.
func (u *UI) transition(v view, key string, now time.Time) {
	switch {
	case v == vExpanded && u.lastView != vExpanded:
		u.openCtx, u.openAt = u.habitContext(), now
	}
}

// used: you did something with a row in the open card; in this situation
// that row is the one you reach for.
func (u *UI) used(kind string) {
	if u.lastView == vExpanded && u.openCtx != "" && (kind == "music" || kind == "edge") {
		u.habits.learn(u.openCtx, kind)
	}
}

func sign(x int) int {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return 1
	}
	return 0
}

func (u *UI) liveRects() []image.Rectangle {
	rs := make([]image.Rectangle, len(u.lives))
	for i, l := range u.lives {
		rs[i] = l.r
	}
	return rs
}

// div255 is x/255 for x ≤ 255·255, without a divide.
func div255(x uint32) uint32 { return (x + 1 + x>>8) >> 8 }

// composite draws src (premultiplied) over dst at strength k, only inside n,
// shifted dx pixels sideways and dy down. Integer arithmetic throughout; transparent
// source pixels (most of them) cost one load.
func composite(dst *canvas, src *image.RGBA, n notch, k float64, dx, dy int) {
	if k <= 0.003 {
		return
	}
	kk := uint32(k*256 + 0.5)
	b := dst.img.Rect
	for py := 0; py < min(int(math.Ceil(float64(n.h))), b.Max.Y); py++ {
		lx, rx, ok := n.span(float32(py) + 0.5)
		if !ok {
			continue
		}
		x0, x1 := max(int(math.Ceil(float64(lx))), 0), min(int(rx), b.Max.X)
		sy := py - dy
		if sy < 0 || sy >= b.Max.Y {
			continue
		}
		o, so := py*dst.img.Stride, sy*dst.img.Stride
		s, d := words(src.Pix[so:so+dst.img.Stride]), words(dst.img.Pix[o:o+dst.img.Stride])
		for px := x0; px < x1; px++ {
			sx := px - dx
			if sx < 0 || sx >= b.Max.X {
				continue
			}
			v := s[sx]
			if v == 0 {
				continue
			}
			if kk < 256 {
				v = scale(v, kk)
			}
			d[px] = over(v, d[px])
		}
	}
}

// ---- Ember: one element shared by every view ----

func (u *UI) emberSettled() bool {
	if !u.spot.show {
		return true
	}
	return u.ex.settled(float64(u.spot.x)) && u.ey.settled(float64(u.spot.y)) && u.es.settled(float64(u.spot.s))
}

// glancing: Ember's eyes are moving (a glance, a notice, the pointer).
func (u *UI) glancing() bool {
	_, moving := u.mind.gaze(u.now(), 0, u.hover)
	return moving || math.Abs(u.look.x-u.lookTarget(u.now())) > 0.03
}

func (u *UI) lookTarget(now time.Time) float64 {
	ptr := math.Max(-1, math.Min(1, (u.mx-u.ex.x)/90)) // watch the pointer
	g, _ := u.mind.gaze(now, ptr, u.hover)
	return g
}

// senses is what the mind takes in each frame.
func (u *UI) senses() senses {
	s := senses{music: u.track != nil && u.track.Playing, hovered: u.hover}
	for _, x := range u.sessions {
		s.working = s.working || x.status() == "work"
		s.waiting = s.waiting || x.State == "wait"
	}
	return s
}

func (u *UI) stepEmber(dt float64, now time.Time) {
	u.mind.step(now, u.senses())
	if !u.spot.show {
		return
	}
	if u.ex.x == 0 {
		u.ex.x, u.ey.x, u.es.x = float64(u.spot.x), float64(u.spot.y), float64(u.spot.s)
	}
	if u.lastView == vCompact && math.Abs(u.ey.x-float64(u.spot.y)) < 1 && math.Abs(u.ex.x-float64(u.spot.x)) < 60 {
		u.ex.x, u.ex.v = float64(u.spot.x), 0 // in the pill Ember sits in its slot, which rides the shape's edge
	}
	u.ex.stepK(float64(u.spot.x), dt, 600, 49) // critically damped and quick: Ember keeps up with the shape
	u.ey.stepK(float64(u.spot.y), dt, 600, 49)
	u.es.stepK(float64(u.spot.s), dt, 600, 49)
	u.look.stepK(u.lookTarget(now), dt, 160, 22)
	// colour: the mood's, pulled toward red by a bad mood, lifted by a good one
	target := u.pal.moodColor(u.spot.mood)
	if v := u.mind.valence; v < 0 {
		target = mix(target, u.pal.red, math.Min(0.6, -v*0.6))
	} else if v > 0.45 {
		target = mix(target, colorWhite, (v-0.45)*0.25)
	}
	k := math.Min(1, dt*7) // colours blend over ~150 ms
	u.emberCol[0] += (float64(target.R) - u.emberCol[0]) * k
	u.emberCol[1] += (float64(target.G) - u.emberCol[1]) * k
	u.emberCol[2] += (float64(target.B) - u.emberCol[2]) * k
}

// emberRect covers Ember, the light it casts, and its z's when asleep.
func (u *UI) emberRect() image.Rectangle {
	if !u.spot.show {
		return image.Rectangle{}
	}
	x, y, s := float32(u.ex.x), float32(u.ey.x), float32(u.es.x)
	g := s*1.5 + 4 // room for a nod or a shake
	r := image.Rect(int(x-g), int(y-g), int(x+g)+2, int(y+g)+2)
	if u.mind.asleep {
		r = r.Union(image.Rect(int(x), 0, int(x+s+10), int(y)))
	}
	return r
}

func (u *UI) drawEmber(t float64, now time.Time) {
	if !u.spot.show {
		return
	}
	m := u.mind
	bounce := float32(0)
	if bt := now.Sub(u.bounceAt).Seconds(); bt < 0.7 {
		bounce = float32(math.Abs(math.Sin(bt/0.7*2*math.Pi)) * (1 - bt/0.7))
	}
	col := color.NRGBA{uint8(u.emberCol[0]), uint8(u.emberCol[1]), uint8(u.emberCol[2]), 255}
	x, y, s := float32(u.ex.x), float32(u.ey.x), float32(u.es.x)
	gdx, gdy, gs := u.gestureOffset(now)
	x, y, s = x+gdx, y+gdy, s*gs
	s *= 1 + 0.06*float32(m.arousal-0.5) // a touch bigger when lively
	if s > 2 && !m.asleep {              // the light it casts on the island, brighter with energy
		u.cv.glowInside(u.emberRect(), x, y-s*0.2, s*1.5, col, 0.08+0.16*m.arousal)
	}
	u.cv.emberX(u.pal, x, y, s, u.spot.mood, t, expr{blink: float32(m.blink(now)), bounce: bounce,
		look: float32(u.look.x), lids: float32(m.lids(now)), energy: float32(m.arousal)}, col, 1)
	if m.asleep && s > 6 { // z's drift up and fade
		f := face(fSemiBold, 7)
		for i := 0; i < 2; i++ {
			k := math.Mod(t/2.4+float64(i)*0.5, 1)
			zx := x + s*0.45 + float32(k)*5
			zy := y - s*0.3 - float32(k)*s*0.75
			u.cv.text(f, "z", zx, zy, alpha(u.pal.muted, math.Sin(k*math.Pi)*0.9))
		}
	}
}

func (u *UI) bouncing() bool { return u.now().Sub(u.bounceAt) < 700*time.Millisecond }

// continuous: something animates at full rate (Ember flickering, music, a card).
func (u *UI) continuous() bool {
	if u.lastView != vCompact {
		return true
	}
	for _, s := range u.sessions {
		if s.status() != "idle" {
			return true
		}
	}
	return (u.track != nil && u.track.Playing) || u.moment != nil
}

func (u *UI) nextBlink() time.Time { return u.mind.blinkAt }

// shimmering: the pill's label is lit while Claude works.
func (u *UI) shimmering() bool {
	if u.lastView != vCompact {
		return u.lastView == vExpanded && u.senses().working
	}
	return u.primary() == "claude" && u.compactLine().work
}

// viewKey changes when what's on screen changes identity (a cross-fade).
func (u *UI) viewKey(v view) string {
	switch v {
	case vCard:
		if c := u.top(); c != nil {
			return fmt.Sprint("card:", c.ID, c.Title)
		}
	case vExpanded: // a hero you pick cross-fades the card: the lines that stay glide
		return "exp:" + u.hero
	case vDash:
		return "dash:" + u.dash
	}
	return "pill:" + u.compactLine().key
}
