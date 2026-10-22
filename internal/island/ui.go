package island

import (
	"fmt"
	"image"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/config"
	"github.com/Tejas242/ember/internal/source"
)

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
	travel  bool // it rides something that moves (a meter's knob): a spring with weight, not a snap
}

const (
	fadeIn    = 160 * time.Millisecond
	slideTime = 260 * time.Millisecond
	focusHold = 90 * time.Second // a scroll-picked activity stays this long
)

// UI owns all state; only the main loop touches it.
type UI struct {
	W, H    int
	BarH    float32
	TimeFmt string
	pal     palette
	cfg     config.Config
	now     func() time.Time

	worldState
	pointerState
	motionState
	emberState
	frameState
}

// worldState is what the island knows: Claude's sessions and requests, the
// plugins, the music, the system, and what Ember remembers.
type worldState struct {
	sessions   map[string]*Session
	cards      []*Card
	ext        map[string]*extPlugin // external plugins, by name: what each shows
	dashboards map[string]Dashboard  // by plugin, while the plugin runs
	track      *Track
	art        *albumArt
	moment     *moment
	under      *moment // a note waiting under an OSD
	power      *power  // the battery, if there is one
	casting    bool    // the screen is being shared, and [privacy] dot says so
	shared     bool    // the screen is being shared, dot or not
	vol        level   // the default output
	light      float64 // screen brightness 0..1, -1 if there's no backlight
	bond       *bond   // what Ember remembers about you
	mind       *mind
	day        *Day
	watching   map[string]bool // sessions whose terminal has your focus
	habits     *habits
	lastDone   struct {
		project string
		at      time.Time
	}
}

// pointerState is where the pointer is and what you've done with it: hover,
// presses and drags, and what you opened or picked in the card.
type pointerState struct {
	unfolded                map[string]bool                          // dashboard sections you unfolded: "<plugin>:<index>"
	opened                  map[string]bool                          // agents you opened or closed in the card: a:<session>:<agent>
	shownOpen               map[string]bool                          // how the card last drew them (the room it had may have closed one)
	allRows                 bool                                     // the card shows every session, one line each ("Show all"), until it closes
	hero                    string                                   // the session you made the card's hero, until it closes
	dash                    string                                   // the open dashboard's plugin, if any
	dashAt                  time.Time                                // when it opened: its entrance plays from here
	dashIn                  bool                                     // the pointer has been on it: leaving closes it
	dashWhy                 string                                   // why the last page closed, until its plugin hears it
	idleSince               time.Time                                // the open page's music has been paused (or gone) since
	finishedSong            string                                   // Claude finished while the page was open, during this song (its media key)
	openers                 func(context string) (name, hint string) // the plugin a click on Ember opens in a context
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
	stage                   *stageState                // the open stage's own state
	nudge                   *wordsNudges               // timed words you moved earlier or later, per song
	stuck                   string                     // the song whose player's playhead doesn't advance (its media key)
	playhead                playheadWatch              // what the player has said of its playhead, to tell when it's stuck
	openCtx                 string                     // the desk when the card opened
	openAt                  time.Time
}

// motionState is the island's own motion: its shape's springs, tweens and
// the cross-fade between views.
type motionState struct {
	sw, sh, bub spring
	swell       spring  // the pill's anticipation under the pointer, 0..1
	prevK       float64 // how visible the view we're leaving was when we left it
	prevAt      time.Time
	haloAt      time.Time // the last frame something waited on you (the halo fades after)
	meterBump   time.Time // a key pressed past the meter's end
	meter       spring    // a moment's level bar
	meterKey    string
	tweens      map[string]*tween
	enterAt     map[string]time.Time // when each element of the current view began to arrive
	rolls       map[string]*roll
	tweening    bool    // a tween or roll is still moving
	fadeK       float64 // how far the content has faded in (panels fade with it)
	rowK        float64 // how far the element being drawn has arrived
	trAt        time.Time
	prevAlive   bool
	slideOld    string
	slideAt     time.Time
}

// emberState is Ember's body, its acts and its character.
type emberState struct {
	audio            *source.Audio // the song's levels, while one plays
	eqShown          bool          // an equaliser is on screen (it listens, so frames come quicker)
	eqLevel          [3]float64    // its bars, smoothed
	body             *body         // Ember's physical self
	act              *act          // what it's acting out, if anything
	dancing          bool          // it has caught the song's beat
	groove           groove        // its sense of rhythm
	dancer           dancer        // how it moves to it
	travelTo         float32       // where its slot last was: a jump in it is a trip, taken with a hop
	travelling       bool
	tripFrom         float64   // where the trip left from
	tripAt           time.Time // when it left the ground (after its crouch)
	tripAir          float64   // how long it's in the air
	flare            bool      // the Done playing now was earned: a long or hard-won turn
	afterRoll        cheer     // the Done that follows the eye-roll
	gulpAt           time.Time
	greetAt          time.Time // a return found at startup or after a sleep: greet once it's calm
	pokes            int
	pokeAt           time.Time
	petSince         time.Time // the pointer has rested on Ember (in the card) since
	yawnCheck        time.Time
	sleepK           float64 // how asleep the pill looks, 0..1
	wasAsleep        bool
	ex, ey, es       spring // Ember's position and size, shared by every view
	look             spring
	face             faceState     // the face over time: blinks between expressions, saccades, the mouth
	emberCol         [3][3]float64 // its fire: hot, body, tip
	spot             emberSpot
	start, lastBlink time.Time
	bounceAt         time.Time
	glanceAt         time.Time
}

// frameState is drawing: the canvases, what the last frame drew and where,
// and what changed since.
type frameState struct {
	cv                *canvas // the frame
	dc                *canvas // where views draw: the content layer
	content           *canvas
	prev              *image.RGBA // content of the view we're leaving
	second            time.Time
	washImg           *image.RGBA // the cover's haze, scaled to the music section
	washKey           string
	frame             int               // full frames drawn: tweens step once per frame
	dt                float64           // this frame's time step
	lastFull          bool              // the last frame was a full repaint (for EMBER_DEBUG)
	dmg               []image.Rectangle // the last frame's damage, rectangle by rectangle
	lastPage          string
	lastChip          string
	trailOn           bool
	lastView          view
	lastKey           string
	lineKey, lineText string
	lastTick          time.Time
	minute            time.Time
	dirty             bool
	base              *image.RGBA
	lives             []live
	shape             notch
	bubble            image.Rectangle
	lastBounds        image.Rectangle
	lastEmber         image.Rectangle
}

func NewUI(w, h int, barH float32) *UI {
	u := &UI{W: w, H: h, BarH: barH, TimeFmt: "3:04 PM", pal: loadPalette(), cfg: config.Default(), now: time.Now}
	u.cv, u.content, u.prev, u.base = newCanvas(w, h), newCanvas(w, h), image.NewRGBA(image.Rect(0, 0, w, h)), image.NewRGBA(image.Rect(0, 0, w, h))
	u.sessions, u.dashboards, u.opened, u.dirty = map[string]*Session{}, map[string]Dashboard{}, map[string]bool{}, true
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
	u.body = newBody()
	return u
}

func (u *UI) Invalidate() { u.dirty = true }

// React: Ember answers what you just did with its body. It nods when you
// allow, shakes its head when you deny, winces when a step fails.
func (u *UI) React(g string) { u.play(g, 0) }

// Bounce: a small happy hop (good news).
func (u *UI) Bounce() { u.play("hop", 0) }

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
	fresh := u.moment == nil || u.moment.key != m.key
	u.moment = &m
	if fresh {
		u.momentAct(m)
	}
}

// momentAct: Ember answers the news with its body.
func (u *UI) momentAct(m moment) {
	switch {
	case m.key == "hello": // the first time ever: a hello
		if u.cfg.Flourishes {
			u.play("hello", 0)
		}
	case m.key == "away": // you're back: a stretch may be playing; otherwise a small hello
		if u.cfg.Flourishes && (u.act == nil || u.act.kind != "stretch") {
			u.play("hello", 0)
		}
	case m.mood == moodHappy && m.cheer.roll && !u.casting && u.cfg.Flourishes && u.bond != nil && u.bond.cooled("eyeroll", 20*time.Hour, u.now()):
		u.afterRoll = m.cheer
		u.play("eyeroll", 0) // then the Done, when it's done rolling
	case m.mood == moodHappy:
		u.cheer(m.cheer)
	case m.mood == moodError:
		u.play("stopped", 0)
	case m.icon == "bolt":
		u.play("charge", 0)
	case m.icon == "check":
		u.play("hop", 0)
	case m.thumb != nil:
		u.play("shutter", 0)
	case m.icon == "headphones" && m.sub == "Connected":
		u.play("listen", 0)
	}
}

// sessionList ranks the sessions: waiting, working, idle. Within a rank, one
// whose terminal you aren't looking at comes first (you can see the other),
// then the newest turn. Not the last one heard from: two sessions working at
// once both speak every few seconds, and the pill would swap between them.
func (u *UI) sessionList() []*Session {
	out := make([]*Session, 0, len(u.sessions))
	for _, s := range u.sessions {
		out = append(out, s)
	}
	rank := func(s *Session) int {
		switch s.status() {
		case "wait":
			return 3
		case "work":
			return 2
		case "idle":
			return 1
		}
		return 0
	}
	slices.SortFunc(out, func(a, b *Session) int { // a total order: the same result as any sort
		if ra, rb := rank(a), rank(b); ra != rb {
			return rb - ra
		}
		if wa, wb := u.watching[a.ID], u.watching[b.ID]; wa != wb {
			if wb { // (as it always sorted: the one you're not watching first)
				return -1
			}
			return 1
		}
		if !a.Since.Equal(b.Since) {
			return b.Since.Compare(a.Since)
		}
		return strings.Compare(a.ID, b.ID)
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
	return append(out, u.extActs()...)
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
	}
	return u.extSatellite(primary) // a plugin's clock (edge's round, its practice)
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

// leaveGrace: the open card stays this long after the pointer leaves it, so
// a pointer that slips off its edge and back doesn't fold it.
const leaveGrace = 200 * time.Millisecond

func (u *UI) currentView() view {
	if u.top() != nil {
		return vCard
	}
	if u.dash != "" && u.dashboards[u.dash] != nil {
		return vDash
	}
	if u.pinned || u.drag != "" || (u.hover && !u.suppress && !u.discreet() && u.now().Sub(u.hoverAt) >= u.cfg.HoverDelay) ||
		(!u.hover && u.lastView == vExpanded && u.now().Sub(u.leftAt) < leaveGrace) {
		return vExpanded
	}
	return vCompact
}

// discreet: the screen is shared and [privacy] discreet is on: Done and
// Stopped keep Claude's words to themselves, screenshots show no preview, and
// the card opens only on a click. Requests still show: they need you.
func (u *UI) discreet() bool { return u.shared && u.cfg.Discreet }

// used: you did something with a row in the open card; in this situation
// that row is the one you reach for.
func (u *UI) used(kind string) {
	if u.lastView == vExpanded && u.openCtx != "" && (kind == "music" || u.ext[kind] != nil) {
		u.habits.learn(u.openCtx, kind)
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
	return moving || math.Abs(u.look.x-u.lookTarget(u.now())) > 0.03 || u.face.busy(u.now())
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
	u.actStep(now)
	u.listen(now)
	u.body.still = u.cfg.Calm
	u.greet(now)
	u.introduce(now)
	if now.Sub(u.yawnCheck) > time.Minute { // around your bedtime, on the quiet clock, a yawn (twice a night at most)
		u.yawnCheck = now
		if u.bond.nearBedtime(now) && !u.mind.away && !u.mind.asleep && (u.primary() == "" || u.primary() == "clock") && !u.senses().working {
			u.flourish("yawn", 45*time.Minute)
		}
	}
	if !u.spot.show {
		u.body.step(dt, 0)
		return
	}
	sp := u.spot
	rel := sp.x - u.shape.x // where its slot is in the pill: the pill's own edges move it too
	if u.ex.x == 0 {
		u.ex.x, u.ey.x, u.es.x = float64(sp.x), float64(sp.y), float64(sp.s)
		u.travelTo = rel
	}
	// a trip: its slot jumped somewhere else in the pill (onto a meter, back
	// home). It jumps there: a ballistic hop as high as the trip is long,
	// crossing at an even pace and landing on the slot (wherever the morphing
	// pill has taken it by then), so it never skates after it lands.
	if u.lastView == vCompact && abs32(rel-u.travelTo) > 14 && math.Abs(u.ey.x-float64(sp.y)) < 8 && !u.cfg.Calm {
		h := math.Max(1.6, math.Min(3.2, 1.2+math.Abs(float64(rel-u.travelTo))/40))
		const ant = 0.04
		u.travelling, u.tripFrom = true, u.ex.x
		u.tripAt, u.tripAir = now.Add(time.Duration(ant*float64(time.Second))), hopFor(h)
		u.body.jump(h, ant)
	}
	u.travelTo = rel
	x0 := u.ex.x
	switch {
	case u.travelling:
		k := math.Max(0, math.Min(1, now.Sub(u.tripAt).Seconds()/u.tripAir))
		u.ex.x = u.tripFrom + (float64(sp.x)-u.tripFrom)*k
		u.ex.v = (u.ex.x - x0) / math.Max(dt, 1e-3)
		if k >= 1 {
			u.travelling = false
		}
	case sp.travel: // riding a knob: it stands on it; the knob's moves are felt as lean
		u.ex.x = float64(sp.x)
		u.ex.v = (u.ex.x - x0) / math.Max(dt, 1e-3)
	case u.lastView == vCompact && math.Abs(u.ey.x-float64(sp.y)) < 1 && math.Abs(u.ex.x-float64(sp.x)) < 60:
		u.ex.x, u.ex.v = float64(sp.x), 0 // in the pill Ember sits in its slot, which rides the shape's edge
	default:
		u.ex.stepK(float64(sp.x), dt, 600, 49) // critically damped and quick: Ember keeps up with the shape
	}
	u.ey.stepK(float64(sp.y), dt, 600, 49)
	u.es.stepK(float64(sp.s), dt, 600, 49)
	u.look.stepK(u.lookTarget(now), dt, 160, 22)
	k := float64(u.es.x) / 16
	u.body.step(dt, u.ex.v/math.Max(k, 0.5))
	u.blendFire(dt)
}

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
	if u.cfg.Calm {
		return false
	}
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
