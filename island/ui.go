package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"golang.org/x/image/vector"
)

type Button struct {
	Key, Label      string
	Primary, Danger bool
}

type Option struct{ Label, Desc string }

// Card is anything that wants the user's eyes: a permission request, a
// question, an edge nudge, a finished session.
type Card struct {
	ID       string
	Kind     string // perm | ask | edge | done
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
func (s *spring) step(target, dt float64) {
	const k, d = 320.0, 31.0
	s.v += ((target-s.x)*k - s.v*d) * dt
	s.x += s.v * dt
}

func (s *spring) settled(target float64) bool {
	return math.Abs(target-s.x) < 0.35 && math.Abs(s.v) < 3
}

// live is a part of the frame that animates by itself (Ember, the equaliser):
// steady frames restore just these rectangles and redraw them.
type live struct {
	r    image.Rectangle
	draw func(t float64)
}

// UI owns all state; only the main loop touches it.
type UI struct {
	W, H     int
	BarH     float32
	TimeFmt  string
	cv       *canvas
	pal      palette
	sessions map[string]*Session
	cards    []*Card
	edge     *EdgeStatus
	track    *Track
	art      *albumArt

	hover, pinned      bool
	hoverAt, leftAt    time.Time
	mx, my             float64
	hoverKey           string
	hits               []hit
	onKey              func(key string)
	sw, sh, bub        spring
	lastView           view
	lastKey            string
	keyAt, settledAt   time.Time
	start, lastBlink   time.Time
	bounceAt, lastTick time.Time
	minute             time.Time
	dirty              bool
	base               *image.RGBA
	lives              []live
	shape              notch
	bubble             image.Rectangle
	lastBounds         image.Rectangle
	now                func() time.Time
}

func NewUI(w, h int, barH float32) *UI {
	u := &UI{W: w, H: h, BarH: barH, TimeFmt: "3:04 PM", cv: newCanvas(w, h), pal: loadPalette(),
		sessions: map[string]*Session{}, now: time.Now, dirty: true, base: image.NewRGBA(image.Rect(0, 0, w, h))}
	u.start, u.lastBlink = u.now(), u.now()
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
	return changed
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

const hoverDelay = 260 * time.Millisecond

func (u *UI) currentView() view {
	if u.top() != nil {
		return vCard
	}
	if u.pinned || (u.hover && u.now().Sub(u.hoverAt) >= hoverDelay) || (!u.hover && u.lastView == vExpanded && u.now().Sub(u.leftAt) < 320*time.Millisecond) {
		return vExpanded
	}
	return vCompact
}

// ---- compact line: what the pill says ----

type line struct {
	key         string
	mood        mood
	text        string
	plus, minus int
	more        int
	chip        string
	chipCol     color.NRGBA
	textCol     color.NRGBA
	lock        bool
	music       bool
}

func (u *UI) compactLine() line {
	p := u.pal
	ss := u.sessionList()
	busy := 0
	for _, s := range ss {
		if s.State != "idle" {
			busy++
		}
	}
	if len(ss) > 0 && ss[0].State == "wait" {
		s := ss[0]
		return line{key: "wait:" + s.ID, mood: moodWait, text: s.Project + " needs you", textCol: p.blue, more: busy - 1}
	}
	if len(ss) > 0 && ss[0].State == "work" {
		s := ss[0]
		l := line{key: "work:" + s.ID, mood: moodWork, text: s.Label, plus: s.Plus, minus: s.Minus, textCol: p.fg, more: busy - 1}
		if l.text == "" {
			l.text = "Thinking"
		}
		return l
	}
	if t := u.track; t != nil && t.Playing {
		txt := t.Title
		if t.Artist != "" {
			txt += " · " + t.Artist
		}
		return line{key: "music:" + t.Title, mood: moodIdle, text: txt, textCol: p.fg, music: true}
	}
	now := u.now()
	l := line{key: "clock", mood: moodIdle, text: now.Format(u.TimeFmt), textCol: p.fg}
	if e := u.edge; e != nil {
		l.lock = e.Closed
		switch {
		case e.Task != "" && !e.Pending && e.StartedMs > 0:
			l.chip = short(e.Task) + " · " + mins(now.Sub(time.UnixMilli(e.StartedMs)))
			l.chipCol = p.green
		case e.Task != "" && e.Pending && e.Closed:
			l.chip, l.chipCol = "practice due", p.yellow
		}
	}
	return l
}

func short(task string) string {
	if i := strings.Index(task, " · "); i > 0 {
		return task[:i]
	}
	return task
}

func mins(d time.Duration) string {
	m := int(d.Minutes())
	if m < 60 {
		return fmt.Sprintf("%dm", max(m, 0))
	}
	return fmt.Sprintf("%dh%02dm", m/60, m%60)
}

func diffText(p, m int) string {
	switch {
	case p > 0 && m > 0:
		return fmt.Sprintf("+%d −%d", p, m)
	case p > 0:
		return fmt.Sprintf("+%d", p)
	}
	return fmt.Sprintf("−%d", m)
}

// ---- layout ----

const (
	cardW   = 384
	expW    = 384
	padX    = 14
	rowH    = 30
	btnH    = 28
	maxComp = 400
)

var (
	fText  = func() fontFace { return face(fMedium, 12.5) }
	fSmall = func() fontFace { return face(fRegular, 11.5) }
)

func (u *UI) compactWidth(l line) float64 {
	w := 12 + 15 + 7 + float64(measure(fText(), l.text))
	if l.music {
		w += 2 + 20 // equaliser
	}
	if l.plus > 0 || l.minus > 0 {
		w += 7 + float64(measure(face(fMedium, 12), diffText(l.plus, l.minus)))
	}
	if l.more > 0 {
		w += 8 + float64(measure(fSmall(), fmt.Sprintf("+%d", l.more)))
	}
	if l.chip != "" {
		w += 14 + float64(measure(fSmall(), l.chip))
	}
	if l.lock {
		w += 15
	}
	return math.Min(maxComp, math.Max(64, w+12))
}

func (u *UI) measure(v view) (float64, float64) {
	switch v {
	case vCard:
		return cardW, float64(u.cardHeight(u.top()))
	case vExpanded:
		return expW, float64(u.expandedHeight())
	}
	return u.compactWidth(u.compactLine()), float64(u.BarH)
}

// showBubble: music keeps a small bubble beside the pill while something else talks.
func (u *UI) showBubble(v view) bool {
	return v == vCompact && u.track != nil && u.track.Playing && !u.compactLine().music
}

// ---- render ----

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
	key := fmt.Sprint(v)
	if v == vCompact {
		key += u.compactLine().key
	} else if c := u.top(); v == vCard && c != nil {
		key += c.ID + c.Title
	}
	if v != u.lastView || key != u.lastKey {
		u.lastView, u.lastKey, u.keyAt, u.dirty = v, key, now, true
	}
	if m := now.Truncate(time.Minute); !m.Equal(u.minute) {
		u.minute, u.dirty = m, true
	}
	tw, th := u.measure(v)
	if u.sw.x == 0 {
		u.sw.x, u.sh.x = tw, th
	}
	u.sw.step(tw, dt)
	u.sh.step(th, dt)
	bt := 0.0
	if u.showBubble(v) {
		bt = 1
	}
	u.bub.step(bt, dt)
	settled := u.sw.settled(tw) && u.sh.settled(th) && u.bub.settled(bt)
	if settled {
		u.sw.x, u.sh.x, u.sw.v, u.sh.v, u.bub.x, u.bub.v = tw, th, 0, 0, bt, 0
	}
	if !settled {
		u.settledAt = time.Time{}
	} else if u.settledAt.IsZero() {
		u.settledAt = now
	}
	// content fades in as the shape arrives (or right after a swap)
	fade := 0.0
	if settled {
		fade = math.Min(1, now.Sub(u.settledAt).Seconds()/0.12)
	} else if v == vCompact && math.Abs(u.sh.x-th) < 2 {
		fade = math.Min(1, now.Sub(u.keyAt).Seconds()/0.18) // width-only morph: keep text visible
	}
	t := now.Sub(u.start).Seconds()

	full := u.dirty || !settled || fade < 1
	if full {
		u.dirty = false
		u.cv.clear(u.cv.img.Rect)
		u.hits, u.lives = u.hits[:0], u.lives[:0]
		w, h := float32(u.sw.x), float32(u.sh.x)
		x := (float32(u.W) - w) / 2
		rb := min(h/2, 12)
		if v != vCompact {
			rb = min(h/2, 20)
		}
		u.shape = notch{x: x, w: w, h: h, rb: rb, ear: 6}
		u.cv.drawNotch(u.pal, u.shape, 1, int(u.BarH))
		u.drawBubble(t)
		a := float32(fade)
		switch v {
		case vCompact:
			u.drawCompact(x, w, h, a)
			u.hits = append(u.hits, hit{u.shape.bounds(), "pill"})
		case vCard:
			u.drawCard(u.top(), x, w, a)
		case vExpanded:
			u.drawExpanded(x, w, a)
			u.hits = append(u.hits, hit{u.shape.bounds(), "panel"})
		}
		copy(u.base.Pix, u.cv.img.Pix)
		b := u.shape.bounds().Union(u.bubble).Inset(-10)
		damage = b.Union(u.lastBounds).Intersect(u.cv.img.Rect)
		u.lastBounds = b
		for _, l := range u.lives {
			l.draw(t)
		}
		return true, damage
	}
	for _, l := range u.lives {
		r := l.r.Intersect(u.cv.img.Rect)
		for yy := r.Min.Y; yy < r.Max.Y; yy++ {
			o := yy*u.base.Stride + r.Min.X*4
			copy(u.cv.img.Pix[o:o+r.Dx()*4], u.base.Pix[o:o+r.Dx()*4])
		}
		l.draw(t)
		damage = damage.Union(r)
	}
	return u.blinking() || u.bouncing(), damage
}

func (u *UI) blinking() bool { return u.now().Sub(u.lastBlink) < 170*time.Millisecond }
func (u *UI) bouncing() bool { return u.now().Sub(u.bounceAt) < 700*time.Millisecond }

// continuous: something animates by itself (Ember flickering, music playing).
func (u *UI) continuous() bool {
	if u.lastView == vCard {
		return true
	}
	for _, s := range u.sessions {
		if s.State != "idle" {
			return true
		}
	}
	return u.track != nil && u.track.Playing && u.lastView != vExpanded
}

func (u *UI) nextBlink() time.Time { return u.lastBlink.Add(4300 * time.Millisecond) }

// emberLive registers Ember as a live element at (cx, cy).
func (u *UI) emberLive(cx, cy, s float32, m mood) {
	r := image.Rect(int(cx-s*0.8), int(cy-s*1.25), int(cx+s*0.8)+1, int(cy+s*0.75)+1)
	u.lives = append(u.lives, live{r, func(t float64) {
		now := u.now()
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
		u.cv.ember(u.pal, cx, cy, s, m, t, blink, bounce)
	}})
}

// eqLive registers the little equaliser at (x, y) (bottom-left), height h.
func (u *UI) eqLive(x, y, h float32, col color.NRGBA, playing bool) {
	r := image.Rect(int(x)-1, int(y-h)-1, int(x+16)+1, int(y)+1)
	u.lives = append(u.lives, live{r, func(t float64) {
		for i := 0; i < 3; i++ {
			k := 0.25
			if playing {
				k = 0.35 + 0.65*math.Abs(math.Sin(t*(4.1+float64(i)*1.7)+float64(i)*1.3))
			}
			bh := float32(k) * h
			u.cv.roundRect(x+float32(i)*5.5, y-bh, 3, bh, 1.5, col)
		}
	}})
}

func (u *UI) drawBubble(t float64) {
	u.bubble = image.Rectangle{}
	k := float32(u.bub.x)
	if k < 0.02 || u.track == nil {
		return
	}
	size := u.BarH * k
	cx := u.shape.x + u.shape.w + 8 + size/2
	n := notch{x: cx - size/2, w: size, h: u.BarH * (0.6 + 0.4*k), rb: size / 2, ear: 0}
	u.cv.drawNotch(u.pal, n, float64(k), 0)
	u.bubble = n.bounds()
	if u.art != nil && k > 0.9 {
		u.cv.drawArt(u.art.small, int(cx-9), int(n.h/2-9), 5, float64(k))
	} else if k > 0.6 {
		u.eqLive(cx-7.5, n.h/2+5, 10, u.pal.green, true)
	}
	u.hits = append(u.hits, hit{n.bounds(), "bubble"})
}

func (u *UI) drawCompact(x, w, h, a float32) {
	p := u.pal
	l := u.compactLine()
	cy := h / 2
	ix := x + 12
	if l.music && u.art != nil {
		u.cv.drawArt(u.art.small, int(ix-1), int(cy-9), 5, float64(a))
	} else {
		u.emberLive(ix+7, cy+1, 14, l.mood)
	}
	tx := ix + 22
	base := cy + 4.5
	avail := x + w - 12 - tx
	if l.lock {
		avail -= 15
	}
	if l.chip != "" {
		avail -= 14 + measure(fSmall(), l.chip)
	}
	if l.music {
		avail -= 22
	}
	diff := ""
	if l.plus > 0 || l.minus > 0 {
		diff = diffText(l.plus, l.minus)
		avail -= 7 + measure(face(fMedium, 12), diff)
	}
	if l.more > 0 {
		avail -= 8 + measure(fSmall(), fmt.Sprintf("+%d", l.more))
	}
	if a <= 0 {
		return
	}
	tx += u.cv.text(fText(), ellipsize(fText(), l.text, avail), tx, base, alpha(l.textCol, float64(a)))
	if diff != "" {
		tx += 7
		if l.plus > 0 {
			tx += u.cv.text(face(fMedium, 12), fmt.Sprintf("+%d", l.plus), tx, base, alpha(p.green, float64(a))) + 4
		}
		if l.minus > 0 {
			tx += u.cv.text(face(fMedium, 12), fmt.Sprintf("−%d", l.minus), tx, base, alpha(p.red, float64(a)))
		}
	}
	if l.more > 0 {
		tx += 8
		tx += u.cv.text(fSmall(), fmt.Sprintf("+%d", l.more), tx, base, alpha(p.muted, float64(a)))
	}
	if l.music {
		u.eqLive(tx+6, cy+5, 10, alpha(p.green, float64(a)), u.track != nil && u.track.Playing)
		tx += 22
	}
	rx := x + w - 12
	if l.lock {
		rx -= 11
		u.cv.lock(rx, cy-6, 11, alpha(p.yellow, float64(a)))
		rx -= 4
	}
	if l.chip != "" {
		cw := measure(fSmall(), l.chip)
		u.cv.circle(rx-cw-7, cy, 1.5, alpha(p.faint, float64(a)))
		u.cv.text(fSmall(), l.chip, rx-cw, base, alpha(l.chipCol, float64(a)))
	}
}

// ---- expanded: hover card ----

func (u *UI) expandedHeight() int {
	h := 14 + 22 // date header
	if u.track != nil {
		h += 10 + 46 + 14 + btnH
	}
	ss := min(len(u.sessions), 4)
	if ss > 0 {
		h += 10 + ss*rowH
	}
	if u.edge != nil {
		h += 8 + rowH
	}
	if u.track == nil && ss == 0 && u.edge == nil {
		h += 22
	}
	return min(h+14, u.H-12)
}

func (u *UI) drawExpanded(x, w, a float32) {
	p := u.pal
	if a <= 0 {
		return
	}
	A := float64(a)
	now := u.now()
	y := float32(14)
	u.emberLive(x+padX+7, y+9, 15, u.compactLine().mood)
	u.cv.text(face(fSemiBold, 13.5), now.Format(u.TimeFmt), x+padX+22, y+14, alpha(p.fg, A))
	date := now.Format("Monday, 2 January")
	dw := measure(fSmall(), date)
	u.cv.text(fSmall(), date, x+w-padX-dw, y+13.5, alpha(p.muted, A))
	y += 22

	if t := u.track; t != nil {
		y += 10
		ax := x + padX
		if u.art != nil {
			u.cv.drawArt(u.art.large, int(ax), int(y), 10, A)
		} else {
			u.cv.roundRect(ax, y, 46, 46, 10, alpha(p.raised, A))
			u.eqLive(ax+15, y+31, 16, alpha(p.green, A), t.Playing)
		}
		tx := ax + 58
		inner := x + w - padX - tx
		u.cv.text(face(fSemiBold, 13), ellipsize(face(fSemiBold, 13), t.Title, inner), tx, y+17, alpha(p.fg, A))
		u.cv.text(fSmall(), ellipsize(fSmall(), t.Artist, inner), tx, y+34, alpha(p.muted, A))
		y += 46 + 14
		// progress
		if t.Length > 0 {
			k := float32(math.Min(1, float64(t.pos(now))/float64(t.Length)))
			u.cv.roundRect(x+padX, y-6, w-2*padX, 3, 1.5, alpha(p.line, A))
			u.cv.roundRect(x+padX, y-6, (w-2*padX)*k, 3, 1.5, alpha(p.fg, A*0.9))
		}
		// controls, centred
		cx := x + w/2
		for i, k := range []string{"prev", "toggle", "next"} {
			bx := cx + float32(i-1)*44
			key := "music:" + k
			if u.hoverKey == key {
				u.cv.circle(bx, y+btnH/2, 15, alpha(p.raised, A))
			}
			u.mediaGlyph(k, bx, y+btnH/2, t.Playing, alpha(p.fg, A))
			u.hits = append(u.hits, hit{image.Rect(int(bx-16), int(y), int(bx+16), int(y+btnH)), key})
		}
		y += btnH
	}

	ss := u.sessionList()
	if len(ss) > 0 {
		y += 10
		for i, s := range ss {
			if i == 4 {
				break
			}
			key := "focus:" + s.ID
			if u.hoverKey == key {
				u.cv.roundRect(x+padX-6, y, w-2*padX+12, rowH, 10, alpha(p.raised, A))
			}
			dot := p.faint
			lbl := "idle"
			switch s.State {
			case "work":
				dot, lbl = p.green, s.Label
			case "wait":
				dot, lbl = p.blue, "waiting for you"
			}
			u.cv.circle(x+padX+4, y+rowH/2, 3.5, alpha(dot, A))
			pf := face(fSemiBold, 12.5)
			pw := u.cv.text(pf, ellipsize(pf, s.Project, 110), x+padX+16, y+rowH/2+4.5, alpha(p.fg, A))
			el := ""
			if !s.Since.IsZero() {
				el = mins(now.Sub(s.Since))
			}
			ew := measure(fSmall(), el)
			u.cv.text(fSmall(), el, x+w-padX-ew, y+rowH/2+4.5, alpha(p.faint, A))
			lf := face(fRegular, 12)
			u.cv.text(lf, ellipsize(lf, lbl, w-2*padX-pw-ew-36), x+padX+16+pw+8, y+rowH/2+4.5, alpha(p.muted, A))
			u.hits = append(u.hits, hit{image.Rect(int(x+padX-6), int(y), int(x+w-padX+6), int(y+rowH)), key})
			y += rowH
		}
	}

	if e := u.edge; e != nil {
		y += 8
		u.cv.roundRect(x+padX-6, y, w-2*padX+12, rowH, 10, alpha(p.raised, A))
		lx := x + padX + 2
		if e.Closed {
			u.cv.lock(lx, y+rowH/2-6, 11, alpha(p.yellow, A))
			lx += 17
		}
		label := fmt.Sprintf("today %d/%d · streak %d", e.Today, e.Need, e.Streak)
		if e.Task != "" {
			label = short(e.Task) + "  ·  " + label
		}
		right := x + w - padX
		if e.Task != "" && e.Pending {
			bf := face(fSemiBold, 12)
			bw := measure(bf, "Start") + 22
			bx := right - bw + 4
			key := "btn:edge:start"
			fill := p.green
			if u.hoverKey == key {
				fill = mix(p.green, p.fg, 0.25)
			}
			u.cv.roundRect(bx, y+4, bw, rowH-8, (rowH-8)/2, alpha(fill, A))
			u.cv.text(bf, "Start", bx+11, y+rowH/2+4.5, alpha(p.ink, A))
			u.hits = append(u.hits, hit{image.Rect(int(bx), int(y), int(bx+bw), int(y+rowH)), key})
			right = bx - 8
		} else if e.StartedMs > 0 {
			el := mins(now.Sub(time.UnixMilli(e.StartedMs)))
			ew := measure(fSmall(), el)
			u.cv.text(fSmall(), el, right-ew, y+rowH/2+4.5, alpha(p.green, A))
			right -= ew + 8
		}
		lf := face(fMedium, 12)
		u.cv.text(lf, ellipsize(lf, label, right-lx), lx, y+rowH/2+4.5, alpha(p.fg, A))
		y += rowH
	}
	if u.track == nil && len(ss) == 0 && u.edge == nil {
		u.cv.text(fSmall(), "Nothing playing · no Claude sessions", x+padX, y+16, alpha(p.faint, A))
	}
}

// mediaGlyph draws prev / play-pause / next centred at (cx, cy).
func (u *UI) mediaGlyph(kind string, cx, cy float32, playing bool, col color.NRGBA) {
	tri := func(x0, y0, x1, y1, x2, y2 float32) {
		u.cv.fillPath(min(x0, x1, x2), min(y0, y1, y2), max(x0, x1, x2), max(y0, y1, y2), col, func(r *vector.Rasterizer, ox, oy float32) {
			r.MoveTo(x0-ox, y0-oy)
			r.LineTo(x1-ox, y1-oy)
			r.LineTo(x2-ox, y2-oy)
			r.ClosePath()
		})
	}
	switch kind {
	case "toggle":
		if playing {
			u.cv.roundRect(cx-5, cy-7, 3.6, 14, 1.4, col)
			u.cv.roundRect(cx+1.4, cy-7, 3.6, 14, 1.4, col)
		} else {
			tri(cx-4, cy-7.5, cx-4, cy+7.5, cx+8, cy)
		}
	case "prev":
		u.cv.roundRect(cx-7, cy-5.5, 2.4, 11, 1, col)
		tri(cx+5, cy-5.5, cx+5, cy+5.5, cx-4, cy)
	case "next":
		u.cv.roundRect(cx+4.6, cy-5.5, 2.4, 11, 1, col)
		tri(cx-5, cy-5.5, cx-5, cy+5.5, cx+4, cy)
	}
}

// ---- attention cards ----

func (u *UI) cardHeight(c *Card) int {
	if c == nil {
		return 60
	}
	h := 14 + 22
	inner := float32(cardW - 2*padX)
	if c.Body != "" {
		h += 6 + 17*len(wrap(face(fRegular, 12.5), c.Body, inner, 3))
	}
	if c.Code != "" {
		h += 8 + 16 + 15*len(codeLines(c.Code, inner-16))
	}
	if len(c.Options) > 0 {
		h += 8 + len(c.Options)*(rowH+5) - 5
	}
	if len(c.Buttons) > 0 || c.Footer != "" {
		h += 12 + btnH
	}
	return min(h+padX, u.H-12)
}

func codeLines(code string, w float32) []string {
	f := face(fMono, 11.5)
	var out []string
	for _, ln := range strings.Split(strings.TrimSpace(code), "\n") {
		out = append(out, ellipsize(f, ln, w))
		if len(out) == 3 {
			break
		}
	}
	return out
}

func (u *UI) drawCard(c *Card, x, w, a float32) {
	if c == nil {
		return
	}
	p := u.pal
	A := float64(a)
	u.emberLive(x+padX+8, 14+10, 17, c.Mood)
	if a <= 0 {
		return
	}
	inner := w - 2*padX
	y := float32(14)
	metaW := float32(0)
	closable := c.Kind == "done" || c.Kind == "edge"
	if c.Meta != "" {
		mf := fSmall()
		meta := ellipsize(mf, c.Meta, 130)
		metaW = measure(mf, meta)
		mx := x + w - padX - metaW
		if closable {
			mx -= 22
		}
		u.cv.text(mf, meta, mx, y+13.5, alpha(p.muted, A))
	}
	tf := face(fSemiBold, 13.5)
	u.cv.text(tf, ellipsize(tf, c.Title, inner-26-metaW-30), x+padX+24, y+14, alpha(p.fg, A))
	if closable {
		cx, cy := x+w-padX-6, y+9
		key := "close:" + c.ID
		col := p.faint
		if u.hoverKey == key {
			u.cv.circle(cx, cy, 10, alpha(p.raised, A))
			col = p.fg
		}
		u.cv.text(face(fRegular, 14), "×", cx-4, cy+4.5, alpha(col, A))
		u.hits = append(u.hits, hit{image.Rect(int(cx-11), int(cy-11), int(cx+11), int(cy+11)), key})
	}
	cy := y + 22
	if c.Body != "" {
		cy += 6
		for _, ln := range wrap(face(fRegular, 12.5), c.Body, inner, 3) {
			cy += 17
			u.cv.text(face(fRegular, 12.5), ln, x+padX, cy-4, alpha(mix(p.fg, p.bg, 0.18), A))
		}
	}
	if c.Code != "" {
		lines := codeLines(c.Code, inner-16)
		bh := float32(16 + 15*len(lines))
		cy += 8
		u.cv.roundRect(x+padX, cy, inner, bh, 9, alpha(p.raised, A))
		for i, ln := range lines {
			u.cv.text(face(fMono, 11.5), ln, x+padX+8, cy+8+11+float32(i)*15, alpha(p.fg, A))
		}
		cy += bh
	}
	if len(c.Options) > 0 {
		cy += 8
		for i, o := range c.Options {
			key := fmt.Sprintf("opt:%d", i)
			bg := p.raised
			if u.hoverKey == key {
				bg = p.line
			}
			u.cv.roundRect(x+padX, cy, inner, rowH, 10, alpha(bg, A))
			lx := x + padX + 12
			if c.Multi {
				u.cv.roundRect(lx, cy+rowH/2-6, 12, 12, 4, alpha(p.line, A))
				if c.picked[i] {
					u.cv.roundRect(lx+2, cy+rowH/2-4, 8, 8, 2.5, alpha(p.green, A))
				}
				lx += 20
			}
			lf := face(fMedium, 12.5)
			lw := u.cv.text(lf, ellipsize(lf, o.Label, inner*0.55), lx, cy+rowH/2+4.5, alpha(p.fg, A))
			if o.Desc != "" {
				u.cv.text(fSmall(), ellipsize(fSmall(), o.Desc, x+padX+inner-12-(lx+lw+10)), lx+lw+10, cy+rowH/2+4.5, alpha(p.muted, A))
			}
			u.hits = append(u.hits, hit{image.Rect(int(x+padX), int(cy), int(x+padX+inner), int(cy+rowH)), key})
			cy += rowH + 5
		}
		cy -= 5
	}
	if len(c.Buttons) > 0 || c.Footer != "" {
		cy += 12
		bx := x + w - padX
		for i := len(c.Buttons) - 1; i >= 0; i-- {
			b := c.Buttons[i]
			bf := face(fMedium, 12.5)
			if b.Primary {
				bf = face(fSemiBold, 12.5)
			}
			bw := measure(bf, b.Label) + 28
			bx -= bw
			key := "btn:" + b.Key
			hov := u.hoverKey == key
			fill, txt := p.raised, p.fg
			switch {
			case b.Primary:
				fill, txt = p.green, p.ink
				if hov {
					fill = mix(p.green, p.fg, 0.25)
				}
			case b.Danger:
				txt = p.red
				if hov {
					fill = mix(p.raised, p.red, 0.16)
				}
			default:
				if hov {
					fill = p.line
				}
			}
			u.cv.roundRect(bx, cy, bw, btnH, btnH/2, alpha(fill, A))
			u.cv.text(bf, b.Label, bx+14, cy+btnH/2+4.5, alpha(txt, A))
			u.hits = append(u.hits, hit{image.Rect(int(bx), int(cy), int(bx+bw), int(cy+btnH)), key})
			bx -= 8
		}
		if c.Footer != "" {
			key := "btn:footer"
			col := p.muted
			if u.hoverKey == key {
				col = p.fg
			}
			fw := u.cv.text(face(fRegular, 12), c.Footer, x+padX+2, cy+btnH/2+4.5, alpha(col, A))
			u.hits = append(u.hits, hit{image.Rect(int(x+padX), int(cy), int(x+padX+fw+4), int(cy+btnH)), key})
		}
	}
}

// ---- input ----

func (u *UI) hitAt(x, y float64) string {
	for i := len(u.hits) - 1; i >= 0; i-- {
		if (image.Point{int(x), int(y)}).In(u.hits[i].r) {
			return u.hits[i].key
		}
	}
	return ""
}

func (u *UI) pointer(e pointerEvent) (redraw, hand bool) {
	switch e.kind {
	case "enter", "motion":
		u.mx, u.my = e.x, e.y
		if !u.hover {
			u.hover, u.hoverAt = true, u.now()
			redraw = true
		}
	case "leave":
		u.hover = false
		u.leftAt = u.now()
		u.hoverKey = ""
		return true, false
	case "press":
		u.press(u.hitAt(e.x, e.y))
		return true, false
	}
	k := u.hitAt(u.mx, u.my)
	if k != u.hoverKey {
		u.hoverKey = k
		redraw = true
	}
	for _, pre := range []string{"btn:", "opt:", "focus:", "close:", "music:"} {
		hand = hand || strings.HasPrefix(k, pre)
	}
	return redraw, hand
}

func (u *UI) press(k string) {
	c := u.top()
	switch {
	case k == "pill" || k == "panel":
		// a waiting session: go to it; otherwise pin / unpin the card
		if ss := u.sessionList(); k == "pill" && len(ss) > 0 && ss[0].State == "wait" {
			u.key("focus:" + ss[0].ID)
			return
		}
		u.pinned = !u.pinned
	case k == "bubble":
		u.pinned = true
	case strings.HasPrefix(k, "close:") && c != nil:
		u.RemoveCard(c.ID)
		if c.OnPick != nil {
			c.OnPick("dismiss", nil)
		}
	case strings.HasPrefix(k, "opt:") && c != nil:
		var i int
		fmt.Sscanf(k, "opt:%d", &i)
		if c.Multi {
			if c.picked == nil {
				c.picked = map[int]bool{}
			}
			c.picked[i] = !c.picked[i]
			return
		}
		u.RemoveCard(c.ID)
		if c.OnPick != nil {
			c.OnPick("option", []int{i})
		}
	case strings.HasPrefix(k, "btn:edge:"), strings.HasPrefix(k, "music:"), strings.HasPrefix(k, "focus:"):
		u.key(k)
	case strings.HasPrefix(k, "btn:") && c != nil:
		var picked []int
		for i := range c.Options {
			if c.picked[i] {
				picked = append(picked, i)
			}
		}
		u.RemoveCard(c.ID)
		if c.OnPick != nil {
			c.OnPick(strings.TrimPrefix(k, "btn:"), picked)
		}
	}
}

func (u *UI) key(k string) {
	if u.onKey != nil {
		u.onKey(k)
	}
}
