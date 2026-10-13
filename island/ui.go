package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
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
	Expires  time.Time
	Priority int
	OnPick   func(key string, picked []int)
	OnExpire func()

	picked map[int]bool
}

type Session struct {
	ID, Project, State, Label string // State: work | wait | idle
	Plus, Minus               int
	Since, Seen               time.Time
	Pids                      []int
}

// EdgeStatus is what edge reports about the day (optional).
type EdgeStatus struct {
	Today     int    `json:"today"`
	Need      int    `json:"need"`
	Streak    int    `json:"streak"`
	Closed    bool   `json:"closed"`
	Task      string `json:"task"`
	URL       string `json:"url"`
	StartedMs int64  `json:"startedMs"`
	Pending   bool   `json:"pending"`
}

// moment is a short-lived compact line: Claude finished, a new track.
type moment struct {
	key   string
	act   string
	mood  mood
	text  string
	sub   string
	until time.Time
	focus string // session to jump to on click
}

type view int

const (
	vCompact view = iota
	vCard
	vExpanded
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
	return math.Abs(target-s.x) < 0.35 && math.Abs(s.v) < 3
}

// live is a part of the frame that animates by itself (the equaliser, a
// progress bar). Steady frames restore just these rectangles and redraw them.
type live struct {
	r    image.Rectangle
	draw func(c *canvas, t float64)
}

// emberSpot is where the current view wants Ember.
type emberSpot struct {
	x, y, s float32
	mood    mood
	show    bool
}

const (
	hoverDelay = 240 * time.Millisecond
	fadeIn     = 230 * time.Millisecond
	slideTime  = 260 * time.Millisecond
	focusHold  = 90 * time.Second // a scroll-picked activity stays this long
)

// UI owns all state; only the main loop touches it.
type UI struct {
	W, H     int
	BarH     float32
	TimeFmt  string
	cv       *canvas // the frame
	dc       *canvas // where views draw: the content layer
	content  *canvas
	prev     *image.RGBA // content of the view we're leaving
	pal      palette
	sessions map[string]*Session
	cards    []*Card
	edge     *EdgeStatus
	track    *Track
	art      *albumArt
	moment   *moment

	// interaction
	hover, pinned, suppress bool
	hoverAt, leftAt         time.Time
	mx, my                  float64
	hoverKey, pressKey      string
	pressAt                 time.Time
	hits                    []hit
	onKey                   func(key string)
	focus                   string // activity picked by scrolling / the bubble
	focusAt                 time.Time
	page                    string // expanded card page
	seekRect                image.Rectangle

	// animation
	sw, sh, bub        spring
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
		prev: image.NewRGBA(image.Rect(0, 0, w, h)), pal: loadPalette(), sessions: map[string]*Session{},
		now: time.Now, dirty: true, base: image.NewRGBA(image.Rect(0, 0, w, h))}
	u.dc = u.content
	u.start, u.lastBlink, u.glanceAt = u.now(), u.now(), u.now()
	c := u.pal.muted
	u.emberCol = [3]float64{float64(c.R), float64(c.G), float64(c.B)}
	return u
}

func (u *UI) Invalidate() { u.dirty = true }
func (u *UI) Bounce()     { u.bounceAt = u.now() }

// ---- cards ----

func (u *UI) AddCard(c *Card) {
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
	}
	return changed
}

// Moment shows a short-lived line in the pill.
func (u *UI) Moment(m moment) {
	u.moment = &m
	if m.mood == moodHappy {
		u.Bounce()
	}
}

func (u *UI) sessionList() []*Session {
	var out []*Session
	for _, s := range u.sessions {
		out = append(out, s)
	}
	rank := map[string]int{"wait": 3, "work": 2, "idle": 1}
	sort.Slice(out, func(i, j int) bool {
		if rank[out[i].State] != rank[out[j].State] {
			return rank[out[i].State] > rank[out[j].State]
		}
		return out[i].Seen.After(out[j].Seen)
	})
	return out
}

// ---- activities: what can be shown, and what wins ----

// acts lists the activities that have something to say, in a stable order.
func (u *UI) acts() []string {
	var out []string
	if len(u.sessions) > 0 {
		out = append(out, "claude")
	}
	if u.track != nil {
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
	ss := u.sessionList()
	if len(ss) > 0 && ss[0].State == "wait" {
		return "claude"
	}
	if u.moment != nil {
		return "moment"
	}
	if u.focus != "" && u.now().Sub(u.focusAt) < focusHold && (u.focus == "clock" || u.has(u.focus)) {
		return u.focus
	}
	if len(ss) > 0 && ss[0].State == "work" {
		return "claude"
	}
	if u.track != nil && u.track.Playing {
		return "music"
	}
	if e := u.edge; e != nil && (e.StartedMs > 0 || e.Closed) {
		return "edge"
	}
	return "clock"
}

// secondary is what the bubble beside the pill shows, if anything.
func (u *UI) secondary(primary string) string {
	ss := u.sessionList()
	working := len(ss) > 0 && (ss[0].State == "work" || ss[0].State == "wait")
	switch {
	case primary != "music" && u.track != nil && u.track.Playing:
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
	if u.pinned || (u.hover && !u.suppress && u.now().Sub(u.hoverAt) >= hoverDelay) ||
		(!u.hover && u.lastView == vExpanded && u.now().Sub(u.leftAt) < 300*time.Millisecond) {
		return vExpanded
	}
	return vCompact
}

// pageFor picks the expanded card's page: the one you chose, else the pill's.
func (u *UI) pageFor() string {
	if u.page != "" && u.has(u.page) {
		return u.page
	}
	switch p := u.primary(); p {
	case "claude", "music", "edge":
		return p
	case "moment":
		if u.moment != nil && u.has(u.moment.act) {
			return u.moment.act
		}
	}
	if a := u.acts(); len(a) > 0 {
		return a[0]
	}
	return ""
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
	v := u.currentView()
	key := u.viewKey(v)
	if v != u.lastView || key != u.lastKey {
		// keep what's on screen to cross-fade it out while the new view grows in
		copy(u.prev.Pix, u.content.img.Pix)
		u.prevAlive = !u.trAt.IsZero()
		u.lastView, u.lastKey, u.trAt, u.dirty = v, key, now, true
	}
	if m := now.Truncate(time.Minute); !m.Equal(u.minute) {
		u.minute, u.dirty = m, true
	}
	tw, th := u.measure(v)
	if v == vCompact && u.hover && !u.suppress { // anticipation: swell a little under the pointer
		tw, th = tw+10, th+2
	}
	if u.sw.x == 0 {
		u.sw.x, u.sh.x = tw, th
	}
	u.sw.step(tw, dt)
	u.sh.step(th, dt)
	bt := 0.0
	if v == vCompact && u.secondary(u.primary()) != "" {
		bt = 1
	}
	u.bub.step(bt, dt)
	settled := u.sw.settled(tw) && u.sh.settled(th) && u.bub.settled(bt)
	if settled {
		u.sw.x, u.sh.x, u.sw.v, u.sh.v, u.bub.x, u.bub.v = tw, th, 0, 0, bt, 0
	}
	p := easeOut(float64(now.Sub(u.trAt)) / float64(fadeIn))
	sliding := now.Sub(u.slideAt) < slideTime
	flashing := now.Sub(u.pressAt) < 160*time.Millisecond
	t := now.Sub(u.start).Seconds()

	// Ember (blinks, glances, gliding to a new spot) and other live parts only
	// need their own rectangles repainted: that's the cheap path below.
	full := u.dirty || !settled || p < 1 || sliding || flashing
	moving := !settled || p < 1 || sliding || flashing || u.blinking() || u.bouncing() || !u.emberSettled()
	if full {
		u.dirty = false
		u.hits, u.lives = u.hits[:0], u.lives[:0]
		u.spot = emberSpot{}
		// the shape, at its current (springing) size
		w, h := float32(u.sw.x), float32(u.sh.x)
		x := (float32(u.W) - w) / 2
		rb := min(h/2, 12)
		if v != vCompact {
			rb = min(h/2, 20)
		}
		u.shape = notch{x: x, w: w, h: h, rb: rb, ear: 6}
		u.cv.clear(u.cv.img.Rect)
		u.cv.drawNotch(u.pal, u.shape, 1, int(u.BarH))
		u.drawBubble(t)
		// the content, laid out at its final size, revealed by the growing shape
		u.content.clear(u.content.img.Rect)
		fx := (float32(u.W) - float32(tw)) / 2
		switch v {
		case vCompact:
			u.hits = append(u.hits, hit{u.shape.bounds(), "pill"})
			u.drawCompact(fx, float32(tw), float32(th))
		case vCard:
			u.drawCard(u.top(), fx, float32(tw))
		case vExpanded:
			u.hits = append(u.hits, hit{u.shape.bounds(), "panel"})
			u.drawExpanded(fx, float32(tw))
		}
		if u.prevAlive && p < 1 {
			composite(u.cv, u.prev, u.shape, 1-math.Min(1, p/0.45))
		}
		composite(u.cv, u.content.img, u.shape, p)
		copy(u.base.Pix, u.cv.img.Pix)
		u.stepEmber(dt, now)
		b := u.shape.bounds().Union(u.bubble).Inset(-10)
		damage = b.Union(u.lastBounds).Intersect(u.cv.img.Rect)
		u.lastBounds = b
		for _, l := range u.lives {
			if settled || u.shape.contains(l.r) || u.bubble.Inset(-2).Intersect(l.r) == l.r { // appear once the shape reaches them
				l.draw(u.cv, t)
			}
		}
		u.drawEmber(t, now)
		u.lastEmber = u.emberRect()
		return moving, damage
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
	return moving, damage
}

func (u *UI) liveRects() []image.Rectangle {
	rs := make([]image.Rectangle, len(u.lives))
	for i, l := range u.lives {
		rs[i] = l.r
	}
	return rs
}

// composite draws src (premultiplied) over dst at strength k, only inside n.
func composite(dst *canvas, src *image.RGBA, n notch, k float64) {
	if k <= 0.003 {
		return
	}
	kk := uint32(k * 256)
	b := dst.img.Rect
	for py := 0; py < min(int(math.Ceil(float64(n.h))), b.Max.Y); py++ {
		lx, rx, ok := n.span(float32(py) + 0.5)
		if !ok {
			continue
		}
		x0, x1 := max(int(math.Ceil(float64(lx))), 0), min(int(rx), b.Max.X)
		o := py * dst.img.Stride
		s, d := src.Pix[o:], dst.img.Pix[o:]
		for px := x0; px < x1; px++ {
			i := px * 4
			sa := uint32(s[i+3]) * kk >> 8
			if sa == 0 {
				continue
			}
			inv := 255 - min(sa, 255)
			d[i] = uint8(min(255, uint32(s[i])*kk>>8+uint32(d[i])*inv/255))
			d[i+1] = uint8(min(255, uint32(s[i+1])*kk>>8+uint32(d[i+1])*inv/255))
			d[i+2] = uint8(min(255, uint32(s[i+2])*kk>>8+uint32(d[i+2])*inv/255))
			d[i+3] = uint8(min(255, sa+uint32(d[i+3])*inv/255))
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

// glancing: Ember's eyes are moving (an idle glance, or following the pointer).
func (u *UI) glancing() bool {
	return u.now().Sub(u.glanceAt) < 900*time.Millisecond || math.Abs(u.look.x-u.lookTarget(u.now())) > 0.05
}

func (u *UI) lookTarget(now time.Time) float64 {
	if u.hover {
		return math.Max(-1, math.Min(1, (u.mx-u.ex.x)/90)) // watch the pointer
	}
	if g := now.Sub(u.glanceAt).Seconds(); g < 0.8 { // an idle glance, left or right
		dir := 1.0
		if u.glanceAt.UnixMilli()/1000%2 == 0 {
			dir = -1
		}
		return 0.9 * math.Sin(g/0.8*math.Pi) * dir
	}
	return 0
}

func (u *UI) stepEmber(dt float64, now time.Time) {
	if !u.spot.show {
		return
	}
	if u.ex.x == 0 {
		u.ex.x, u.ey.x, u.es.x = float64(u.spot.x), float64(u.spot.y), float64(u.spot.s)
	}
	u.ex.stepK(float64(u.spot.x), dt, 260, 28)
	u.ey.stepK(float64(u.spot.y), dt, 260, 28)
	u.es.stepK(float64(u.spot.s), dt, 260, 28)
	u.look.stepK(u.lookTarget(now), dt, 120, 18)
	if now.Sub(u.glanceAt) > 9*time.Second && !u.hover {
		u.glanceAt = now
	}
	target := u.pal.moodColor(u.spot.mood)
	k := math.Min(1, dt*7) // colours blend over ~150 ms
	u.emberCol[0] += (float64(target.R) - u.emberCol[0]) * k
	u.emberCol[1] += (float64(target.G) - u.emberCol[1]) * k
	u.emberCol[2] += (float64(target.B) - u.emberCol[2]) * k
}

func (u *UI) emberRect() image.Rectangle {
	if !u.spot.show {
		return image.Rectangle{}
	}
	x, y, s := float32(u.ex.x), float32(u.ey.x), float32(u.es.x)
	return image.Rect(int(x-s*0.85), int(y-s*1.3), int(x+s*0.85)+2, int(y+s*0.8)+2)
}

func (u *UI) drawEmber(t float64, now time.Time) {
	if !u.spot.show {
		return
	}
	blink := float32(0)
	if bs := now.Sub(u.lastBlink).Seconds(); bs < 0.16 {
		blink = float32(math.Sin(bs / 0.16 * math.Pi))
	} else if bs > 4.3 {
		u.lastBlink = now
	}
	bounce := float32(0)
	if bt := now.Sub(u.bounceAt).Seconds(); bt < 0.7 {
		bounce = float32(math.Abs(math.Sin(bt/0.7*2*math.Pi)) * (1 - bt/0.7))
	}
	col := color.NRGBA{uint8(u.emberCol[0]), uint8(u.emberCol[1]), uint8(u.emberCol[2]), 255}
	u.cv.ember(u.pal, float32(u.ex.x), float32(u.ey.x), float32(u.es.x), u.spot.mood, t, blink, bounce, col, 1, float32(u.look.x))
}

func (u *UI) blinking() bool { return u.now().Sub(u.lastBlink) < 170*time.Millisecond }
func (u *UI) bouncing() bool { return u.now().Sub(u.bounceAt) < 700*time.Millisecond }

// continuous: something animates at full rate (Ember flickering, music, a card).
func (u *UI) continuous() bool {
	if u.lastView != vCompact {
		return true
	}
	for _, s := range u.sessions {
		if s.State != "idle" {
			return true
		}
	}
	return (u.track != nil && u.track.Playing) || u.moment != nil
}

func (u *UI) nextBlink() time.Time { return u.lastBlink.Add(4300 * time.Millisecond) }

// viewKey changes when what's on screen changes identity (a cross-fade).
func (u *UI) viewKey(v view) string {
	switch v {
	case vCard:
		if c := u.top(); c != nil {
			return fmt.Sprint("card:", c.ID, c.Title)
		}
	case vExpanded:
		return "exp:" + u.pageFor()
	}
	return "pill:" + u.compactLine().key
}
