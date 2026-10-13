package island

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"

	"golang.org/x/image/vector"
)

const (
	cardW   = 384
	expW    = 384
	padX    = 14
	rowH    = 30
	btnH    = 28
	maxComp = 400
	headH   = 40 // expanded header: Ember, time, tabs
)

var (
	fText  = func() fontFace { return face(fMedium, 12.5) }
	fSmall = func() fontFace { return face(fRegular, 11.5) }
	fValue = func() fontFace { return face(fSemiBold, 12) }
)

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

func clock(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
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

// ---- compact: the pill ----

type line struct {
	key         string
	mood        mood
	text, sub   string
	textCol     color.NRGBA
	plus, minus int
	more        int
	chip        string
	chipCol     color.NRGBA
	lock        bool
	music       bool
	m           *moment // a system moment: icon, meter, value, battery
}

const meterW = 86

func (u *UI) compactLine() line {
	p, now := u.pal, u.now()
	ss := u.sessionList()
	switch u.primary() {
	case "moment":
		m := u.moment
		l := line{key: "moment:" + m.key, mood: m.mood, text: m.text, sub: m.sub, textCol: p.fg}
		if m.icon != "" || m.thumb != nil {
			l.m = m
		}
		return l
	case "claude":
		if len(ss) == 0 {
			break
		}
		s := ss[0]
		busy := 0
		for _, x := range ss {
			if x.State != "idle" {
				busy++
			}
		}
		switch s.State {
		case "wait":
			return line{key: "claude:wait:" + s.ID, mood: moodWait, text: s.Project + " needs you", textCol: p.blue, more: busy - 1}
		case "work":
			l := line{key: "claude:" + s.ID, mood: moodWork, text: s.Label, plus: s.Plus, minus: s.Minus, textCol: p.fg, more: busy - 1}
			if l.text == "" {
				l.text = "Thinking"
			}
			if s.Agents > 0 {
				l.chip, l.chipCol = agentsText(s.Agents), p.muted
			}
			return l
		}
		return line{key: "claude:idle", mood: moodIdle, text: fmt.Sprintf("Claude · %d idle", len(ss)), textCol: p.muted}
	case "music":
		if t := u.track; t != nil {
			txt := t.Title
			if t.Artist != "" {
				txt += " · " + t.Artist
			}
			col := p.fg
			if !t.Playing {
				col = p.muted
			}
			return line{key: "music", mood: moodIdle, text: txt, textCol: col, music: true}
		}
	case "edge":
		if e := u.edge; e != nil {
			if e.StartedMs > 0 && !e.Pending {
				return line{key: "edge", mood: moodWork, text: short(e.Task), textCol: p.fg,
					chip: mins(now.Sub(time.UnixMilli(e.StartedMs))), chipCol: p.green}
			}
			l := line{key: "edge", mood: moodNudge, text: fmt.Sprintf("Today %d/%d", e.Today, e.Need), textCol: p.fg, lock: e.Closed}
			if e.Task != "" {
				l.chip, l.chipCol = short(e.Task), p.muted
			}
			return l
		}
	}
	l := line{key: "clock", mood: moodIdle, text: now.Format(u.TimeFmt), textCol: p.fg}
	if e := u.edge; e != nil && e.Closed {
		l.lock = true
	}
	return l
}

func agentsText(n int) string {
	if n == 1 {
		return "1 agent"
	}
	return fmt.Sprintf("%d agents", n)
}

// thumbW is how much wider than Ember's slot a screenshot preview is.
func (l line) thumbW() float32 {
	if l.m != nil && l.m.thumb != nil {
		return float32(l.m.thumb.Rect.Dx()) - 15
	}
	return 0
}

func battW() float32 { return 9 * 1.9 }

func (u *UI) compactWidth(l line) float64 {
	w := 12 + 15 + 8 + float64(measure(fText(), l.text)) + float64(l.thumbW())
	if m := l.m; m != nil {
		if l.text == "" {
			w -= 8
		}
		if m.meter {
			w += 8 + meterW
		}
		if m.right != "" {
			w += 10 + float64(measure(fValue(), m.right))
		}
		if m.batt {
			w += 6 + float64(battW()) + 3
		}
	}
	if l.sub != "" {
		w += 8 + math.Min(150, float64(measure(fSmall(), l.sub)))
	}
	if l.music {
		w += 24
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
	if u.casting {
		w += 12
	}
	lo := 70.0
	if l.m != nil && l.m.meter {
		lo = 168 // the bar shouldn't jump as the number changes
	}
	return math.Min(maxComp, math.Max(lo, w+12))
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

func (u *UI) drawCompact(x, w, h float32) {
	p, now := u.pal, u.now()
	l := u.compactLine()
	cy := h / 2
	ix := x + 12
	// a label that changes in place slides: the old one up and out, the new one in from below
	if l.key == u.lineKey && l.text != u.lineText && u.lineText != "" {
		u.slideOld, u.slideAt = u.lineText, now
	}
	u.lineKey, u.lineText = l.key, l.text

	m := l.m
	switch {
	case m != nil && m.thumb != nil:
		th := m.thumb.Rect
		ty := int(cy+0.5) - th.Dy()/2
		draw.Draw(u.dc.img, th.Add(image.Pt(int(ix-1), ty)), m.thumb, image.Point{}, draw.Over)
		u.dc.frame(ix-1, float32(ty), float32(th.Dx()), float32(th.Dy()), 3.5, 1, alpha(p.fg, 0.22))
		u.spot = emberSpot{x: ix + float32(th.Dx())/2, y: cy, s: 0, mood: moodIdle, show: true}
	case m != nil:
		u.dc.icon(m.icon, ix+7, cy, 15, m.tint)
		u.spot = emberSpot{x: ix + 7, y: cy + 1, s: 0, mood: l.mood, show: true} // Ember tucks into the icon
	case l.music && u.art != nil:
		u.dc.drawArt(u.art.small, int(ix-1), int(cy-9), 5, 1)
		u.spot = emberSpot{x: ix + 8, y: cy + 1, s: 0, mood: moodIdle, show: true} // Ember tucks into the cover
	default:
		u.spot = emberSpot{x: ix + 7, y: cy + 1, s: 14, mood: l.mood, show: true}
	}
	tx := ix + 22 + l.thumbW()
	base := cy + 4.5

	// the right end, laid out first: privacy dot, battery, value, meter, lock, chip
	rx := x + w - 12
	if u.casting { // someone can see your screen
		u.dc.circle(rx-3, cy, 3.2, mix(p.red, p.yellow, 0.35))
		rx -= 12
	}
	if m != nil {
		if m.batt {
			bw := battW()
			u.dc.battery(rx-bw-3, cy, 9, float32(m.pct), alpha(p.fg, 0.45), m.tint)
			rx -= bw + 3 + 6
		}
		if m.right != "" {
			vw := measure(fValue(), m.right)
			col := p.fg
			if m.tint == p.muted {
				col = p.muted
			}
			u.dc.text(fValue(), m.right, rx-vw, base, col)
			rx -= vw + 10
		}
		if m.meter {
			mx := tx
			if l.text != "" {
				mx = rx - meterW
			}
			mw := rx - mx
			k := float32(math.Max(0, math.Min(1, u.meter.x)))
			u.dc.roundRect(mx, cy-2.5, mw, 5, 2.5, alpha(p.fg, 0.16))
			if k > 0 {
				u.dc.roundRect(mx, cy-2.5, max(mw*k, 5), 5, 2.5, m.tint)
			}
			rx = mx - 8
		}
	}
	if l.lock {
		rx -= 11
		u.dc.lock(rx, cy-6, 11, p.yellow)
		rx -= 4
	}
	if l.chip != "" {
		cw := measure(fSmall(), l.chip)
		u.dc.circle(rx-cw-7, cy, 1.5, p.faint)
		u.dc.text(fSmall(), l.chip, rx-cw, base, l.chipCol)
		rx -= cw + 14
	}
	if l.text == "" {
		return
	}

	avail := rx - tx
	if l.music {
		avail -= 24
	}
	diff := ""
	if l.plus > 0 || l.minus > 0 {
		diff = diffText(l.plus, l.minus)
		avail -= 7 + measure(face(fMedium, 12), diff)
	}
	if l.more > 0 {
		avail -= 8 + measure(fSmall(), fmt.Sprintf("+%d", l.more))
	}
	subW := float32(0)
	if l.sub != "" {
		subW = min(150, measure(fSmall(), l.sub)+8)
		avail -= subW
	}
	text := ellipsize(fText(), l.text, avail)
	if k := float64(now.Sub(u.slideAt)) / float64(slideTime); k < 1 {
		e := float32(easeOut(k))
		u.dc.text(fText(), ellipsize(fText(), u.slideOld, avail), tx, base-9*e, alpha(l.textCol, 1-float64(e)))
		u.dc.text(fText(), text, tx, base+9*(1-e), alpha(l.textCol, float64(e)))
		tx += measure(fText(), text)
	} else {
		tx += u.dc.text(fText(), text, tx, base, l.textCol)
	}
	if l.sub != "" {
		u.dc.text(fSmall(), ellipsize(fSmall(), l.sub, subW-8), tx+8, base, p.muted)
		tx += subW
	}
	if diff != "" {
		tx += 7
		if l.plus > 0 {
			tx += u.dc.text(face(fMedium, 12), fmt.Sprintf("+%d", l.plus), tx, base, p.green) + 4
		}
		if l.minus > 0 {
			tx += u.dc.text(face(fMedium, 12), fmt.Sprintf("−%d", l.minus), tx, base, p.red)
		}
	}
	if l.more > 0 {
		tx += 8
		tx += u.dc.text(fSmall(), fmt.Sprintf("+%d", l.more), tx, base, p.muted)
	}
	if l.music && u.track != nil {
		u.eqLive(tx+8, cy+5, 10, p.green, u.track.Playing)
		u.progressLive(x+16, h-2.5, w-32, 1.5) // a hairline of the song under the pill
	}
}

// eqLive registers the little equaliser at (x, y) (bottom-left), height h.
func (u *UI) eqLive(x, y, h float32, col color.NRGBA, playing bool) {
	r := image.Rect(int(x)-1, int(y-h)-1, int(x+16)+1, int(y)+1)
	u.lives = append(u.lives, live{r, func(c *canvas, t float64) {
		for i := 0; i < 3; i++ {
			k := 0.22
			if playing {
				k = 0.3 + 0.7*math.Abs(math.Sin(t*(3.7+float64(i)*1.9)+float64(i)*1.3))*(0.75+0.25*math.Sin(t*1.3+float64(i)))
			}
			bh := float32(k) * h
			c.roundRect(x+float32(i)*5.5, y-bh, 3, bh, 1.5, col)
		}
	}})
}

// progressLive registers a progress bar that moves on its own while playing.
func (u *UI) progressLive(x, y, w, th float32) {
	t := u.track
	if t == nil || t.Length <= 0 {
		return
	}
	r := image.Rect(int(x)-6, int(y-th)-6, int(x+w)+7, int(y+th)+7)
	u.lives = append(u.lives, live{r, func(c *canvas, _ float64) {
		k := float32(math.Min(1, float64(t.pos(u.now()))/float64(t.Length)))
		c.roundRect(x, y-th/2, w, th, th/2, alpha(u.pal.line, 0.9))
		c.roundRect(x, y-th/2, max(w*k, th), th, th/2, alpha(u.pal.fg, 0.85))
		if th > 2 { // a knob on the big bar
			c.circle(x+w*k, y, th+2, u.pal.fg)
		}
	}})
}

func (u *UI) drawBubble(t float64) {
	u.bubble = image.Rectangle{}
	k := float32(u.bub.x)
	if k < 0.02 {
		return
	}
	act := u.secondary(u.primary())
	size := u.BarH * k
	cx := u.shape.x + u.shape.w + 8 + size/2
	n := notch{x: cx - size/2, w: size, h: u.BarH * (0.6 + 0.4*k), rb: size / 2}
	u.cv.drawNotch(u.pal, n, float64(k), 0)
	u.bubble = n.bounds()
	cy := n.h / 2
	if k > 0.85 {
		switch act {
		case "music":
			if u.art != nil {
				u.cv.drawArt(u.art.small, int(cx-9), int(cy-9), 9, float64(k))
			} else {
				u.eqLive(cx-7.5, cy+5, 10, u.pal.green, true)
			}
		case "claude":
			m := moodWork
			if ss := u.sessionList(); len(ss) > 0 && ss[0].State == "wait" {
				m = moodWait
			}
			u.lives = append(u.lives, live{image.Rect(int(cx-9), 0, int(cx+9), int(n.h)), func(c *canvas, t float64) {
				c.ember(u.pal, cx, cy+1, 11, m, t, 0, 0, u.pal.moodColor(m), 1, 0)
			}})
		case "edge":
			if e := u.edge; e != nil {
				txt := mins(u.now().Sub(time.UnixMilli(e.StartedMs)))
				f := face(fSemiBold, 9.5)
				u.cv.text(f, txt, cx-measure(f, txt)/2, cy+3.5, u.pal.green)
			}
		}
	}
	u.hits = append(u.hits, hit{n.bounds(), "bubble"})
}

// ---- expanded: the card that drops from the pill ----

func (u *UI) expandedHeight() int {
	h := headH
	switch u.pageFor() {
	case "music":
		h += 56 + 16 + 18 + 36 + 8
	case "claude":
		h += min(len(u.sessions), 4)*(rowH+2) + 8
	case "edge":
		h += 34 + rowH + 10 + btnH + 10
	default:
		h += 30
	}
	return min(h+6, u.H-12)
}

func (u *UI) drawExpanded(x, w float32) {
	p, now := u.pal, u.now()
	page := u.pageFor()
	// header: Ember · time   …   tabs (or the date)
	em := moodIdle
	switch page {
	case "claude":
		em = moodWork
		if ss := u.sessionList(); len(ss) > 0 && ss[0].State != "work" {
			em = map[string]mood{"wait": moodWait, "idle": moodIdle}[ss[0].State]
		}
	case "edge":
		em = moodNudge
	}
	u.spot = emberSpot{x: x + padX + 8, y: 20, s: 17, mood: em, show: true}
	u.dc.text(face(fSemiBold, 13.5), now.Format(u.TimeFmt), x+padX+24, 25, p.fg)
	acts := u.acts()
	if len(acts) >= 2 {
		rx := x + w - padX
		for i := len(acts) - 1; i >= 0; i-- {
			a := acts[i]
			label := map[string]string{"claude": "Claude", "music": "Music", "edge": "edge"}[a]
			f := face(fMedium, 11.5)
			tw := measure(f, label) + 18
			rx -= tw
			key := "tab:" + a
			fill := color.NRGBA{}
			col := p.muted
			if a == page {
				fill, col = p.raised, p.fg
			} else if u.hoverKey == key {
				fill = mixAlpha(p.raised, 0.6)
			}
			if fill.A > 0 {
				u.dc.roundRect(rx, 10, tw, 21, 10.5, fill)
			}
			u.dc.text(f, label, rx+9, 24.5, col)
			u.hits = append(u.hits, hit{image.Rect(int(rx), 6, int(rx+tw), 35), key})
			rx -= 4
		}
	} else {
		date := now.Format("Monday, 2 January")
		u.dc.text(fSmall(), date, x+w-padX-measure(fSmall(), date), 24.5, p.muted)
	}
	y := float32(headH)
	switch page {
	case "music":
		u.musicPage(x, w, y)
	case "claude":
		u.claudePage(x, w, y)
	case "edge":
		u.edgePage(x, w, y)
	default:
		u.dc.text(fSmall(), "Nothing playing · no Claude sessions", x+padX, y+18, p.faint)
	}
}

func (u *UI) musicPage(x, w, y float32) {
	p, t := u.pal, u.track
	if t == nil {
		return
	}
	ax := x + padX
	if u.art != nil {
		u.dc.drawArt(u.art.large, int(ax), int(y), 11, 1)
	} else {
		u.dc.roundRect(ax, y, 46, 46, 11, p.raised)
		u.eqLive(ax+15, y+31, 16, p.green, t.Playing)
	}
	tx := ax + 60
	inner := x + w - padX - tx
	u.dc.text(face(fSemiBold, 13.5), ellipsize(face(fSemiBold, 13.5), t.Title, inner), tx, y+15, p.fg)
	u.dc.text(face(fRegular, 12), ellipsize(face(fRegular, 12), t.Artist, inner), tx, y+31, p.muted)
	u.dc.text(face(fRegular, 10.5), playerName(t.Player), tx, y+45, p.faint)
	if t.Playing {
		u.eqLive(x+w-padX-16, y+45, 9, p.green, true)
	}
	y += 56 + 10
	// progress with times; click to seek
	bw := w - 2*padX
	u.seekRect = image.Rect(int(x+padX), int(y-6), int(x+padX+bw), int(y+8))
	if t.Length > 0 {
		u.progressLive(x+padX, y, bw, 3)
		u.hits = append(u.hits, hit{u.seekRect, "music:seek"})
		ty := y + 17                                                            // y moves on below; the closure must not see that
		tr := image.Rect(int(x+padX)-3, int(ty-12), int(x+w-padX)+3, int(ty+5)) // glyphs reach a pixel past their box
		u.lives = append(u.lives, live{tr, func(c *canvas, _ float64) {
			f := face(fRegular, 10.5)
			el, tot := clock(t.pos(u.now())), clock(t.Length)
			c.text(f, el, x+padX, ty, p.faint)
			c.text(f, tot, x+w-padX-measure(f, tot), ty, p.faint)
		}})
	}
	y += 20
	cx := x + w/2
	for i, k := range []string{"prev", "toggle", "next"} {
		bx := cx + float32(i-1)*48
		key := "music:" + k
		r := float32(15)
		if k == "toggle" {
			r = 17
		}
		switch {
		case u.pressKey == key && u.now().Sub(u.pressAt) < 160*time.Millisecond:
			u.dc.circle(bx, y+btnH/2+2, r, p.line)
		case u.hoverKey == key:
			u.dc.circle(bx, y+btnH/2+2, r, p.raised)
		}
		u.mediaGlyph(k, bx, y+btnH/2+2, t.Playing, p.fg)
		u.hits = append(u.hits, hit{image.Rect(int(bx-r), int(y), int(bx+r), int(y+btnH+4)), key})
	}
}

func (u *UI) claudePage(x, w, y float32) {
	p, now := u.pal, u.now()
	for i, s := range u.sessionList() {
		if i == 4 {
			break
		}
		key := "focus:" + s.ID
		dot, lbl := p.faint, "idle"
		switch s.State {
		case "work":
			dot, lbl = p.green, s.Label
		case "wait":
			dot, lbl = p.blue, "waiting for you"
		}
		if s.State == "wait" {
			u.dc.roundRect(x+padX-6, y, w-2*padX+12, rowH, 10, mixAlpha(p.blue, 0.12))
		} else if u.hoverKey == key {
			u.dc.roundRect(x+padX-6, y, w-2*padX+12, rowH, 10, p.raised)
		}
		u.dc.circle(x+padX+4, y+rowH/2, 3.5, dot)
		pf := face(fSemiBold, 12.5)
		pw := u.dc.text(pf, ellipsize(pf, s.Project, 110), x+padX+16, y+rowH/2+4.5, p.fg)
		right := x + w - padX
		if !s.Since.IsZero() {
			el := mins(now.Sub(s.Since))
			ew := measure(fSmall(), el)
			u.dc.text(fSmall(), el, right-ew, y+rowH/2+4.5, p.faint)
			right -= ew + 8
		}
		if s.State == "work" && (s.Plus > 0 || s.Minus > 0) {
			f := face(fMedium, 11.5)
			if s.Minus > 0 {
				m := fmt.Sprintf("−%d", s.Minus)
				right -= measure(f, m)
				u.dc.text(f, m, right, y+rowH/2+4.5, p.red)
				right -= 4
			}
			if s.Plus > 0 {
				pl := fmt.Sprintf("+%d", s.Plus)
				right -= measure(f, pl)
				u.dc.text(f, pl, right, y+rowH/2+4.5, p.green)
			}
			right -= 8
		}
		lf := face(fRegular, 12)
		u.dc.text(lf, ellipsize(lf, lbl, right-(x+padX+16+pw+8)), x+padX+16+pw+8, y+rowH/2+4.5, p.muted)
		u.hits = append(u.hits, hit{image.Rect(int(x+padX-6), int(y), int(x+w-padX+6), int(y+rowH)), key})
		y += rowH + 2
	}
}

func (u *UI) edgePage(x, w, y float32) {
	p, now, e := u.pal, u.now(), u.edge
	if e == nil {
		return
	}
	// the day, big
	big := face(fSemiBold, 20)
	tx := x + padX
	tx += u.dc.text(big, fmt.Sprintf("%d/%d", e.Today, e.Need), tx, y+22, p.fg)
	tx += u.dc.text(fSmall(), "  today", tx, y+21, p.muted) + 16
	tx += u.dc.text(big, fmt.Sprint(e.Streak), tx, y+22, p.fg)
	u.dc.text(fSmall(), "  day streak", tx, y+21, p.muted)
	if e.Closed {
		u.dc.lock(x+w-padX-60, y+10, 11, p.yellow)
		u.dc.text(fSmall(), "locked", x+w-padX-44, y+21, p.yellow)
	}
	y += 34
	if e.Task != "" {
		u.dc.roundRect(x+padX-6, y, w-2*padX+12, rowH, 10, p.raised)
		right := x + w - padX
		if e.StartedMs > 0 && !e.Pending {
			el := mins(now.Sub(time.UnixMilli(e.StartedMs)))
			ew := measure(fSmall(), el)
			u.dc.text(fSmall(), el, right-ew, y+rowH/2+4.5, p.green)
			right -= ew + 8
		}
		lf := face(fMedium, 12.5)
		u.dc.text(lf, ellipsize(lf, e.Task, right-(x+padX+2)), x+padX+2, y+rowH/2+4.5, p.fg)
	}
	y += rowH + 10
	var btns []Button
	switch {
	case e.Task != "" && e.Pending:
		btns = []Button{{Key: "snooze", Label: "Later"}, {Key: "start", Label: "Start", Primary: true}}
	case e.Task != "":
		btns = []Button{{Key: "teach", Label: "Give up"}, {Key: "hint", Label: "Hint", Primary: true}}
	}
	u.buttonRow(btns, "btn:edge:", x, w, y, "")
}

// buttonRow draws right-aligned pill buttons (and an optional ghost footer on the left).
func (u *UI) buttonRow(btns []Button, prefix string, x, w, y float32, footer string) {
	p := u.pal
	bx := x + w - padX
	for i := len(btns) - 1; i >= 0; i-- {
		b := btns[i]
		bf := face(fMedium, 12.5)
		if b.Primary {
			bf = face(fSemiBold, 12.5)
		}
		bw := measure(bf, b.Label) + 28
		bx -= bw
		key := prefix + b.Key
		hov := u.hoverKey == key
		pressed := u.pressKey == key && u.now().Sub(u.pressAt) < 160*time.Millisecond
		fill, txt := p.raised, p.fg
		switch {
		case b.Primary:
			fill, txt = p.green, p.ink
			if hov {
				fill = mix(p.green, p.fg, 0.22)
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
		if pressed {
			fill = mix(fill, p.fg, 0.35)
		}
		u.dc.roundRect(bx, y, bw, btnH, btnH/2, fill)
		u.dc.text(bf, b.Label, bx+14, y+btnH/2+4.5, txt)
		u.hits = append(u.hits, hit{image.Rect(int(bx), int(y), int(bx+bw), int(y+btnH)), key})
		bx -= 8
	}
	if footer != "" {
		key := prefix + "footer"
		col := p.muted
		if u.hoverKey == key {
			col = p.fg
		}
		fw := u.dc.text(face(fRegular, 12), footer, x+padX+2, y+btnH/2+4.5, col)
		u.hits = append(u.hits, hit{image.Rect(int(x+padX), int(y), int(x+padX+fw+4), int(y+btnH)), key})
	}
}

// mediaGlyph draws prev / play-pause / next centred at (cx, cy).
func (u *UI) mediaGlyph(kind string, cx, cy float32, playing bool, col color.NRGBA) {
	tri := func(x0, y0, x1, y1, x2, y2 float32) {
		u.dc.fillPath(min(x0, x1, x2), min(y0, y1, y2), max(x0, x1, x2), max(y0, y1, y2), col, func(r *vector.Rasterizer, ox, oy float32) {
			r.MoveTo(x0-ox, y0-oy)
			r.LineTo(x1-ox, y1-oy)
			r.LineTo(x2-ox, y2-oy)
			r.ClosePath()
		})
	}
	switch kind {
	case "toggle":
		if playing {
			u.dc.roundRect(cx-5.5, cy-7.5, 4, 15, 1.5, col)
			u.dc.roundRect(cx+1.5, cy-7.5, 4, 15, 1.5, col)
		} else {
			tri(cx-4.5, cy-8, cx-4.5, cy+8, cx+8.5, cy)
		}
	case "prev":
		u.dc.roundRect(cx-7, cy-5.5, 2.4, 11, 1, col)
		tri(cx+5, cy-5.5, cx+5, cy+5.5, cx-4, cy)
	case "next":
		u.dc.roundRect(cx+4.6, cy-5.5, 2.4, 11, 1, col)
		tri(cx-5, cy-5.5, cx-5, cy+5.5, cx+4, cy)
	}
}

// ---- cards: decisions ----

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

func (u *UI) drawCard(c *Card, x, w float32) {
	if c == nil {
		return
	}
	p := u.pal
	u.spot = emberSpot{x: x + padX + 8, y: 23, s: 17, mood: c.Mood, show: true}
	inner := w - 2*padX
	y := float32(14)
	metaW := float32(0)
	closable := c.Kind == "edge"
	if c.Meta != "" {
		meta := ellipsize(fSmall(), c.Meta, 130)
		metaW = measure(fSmall(), meta)
		mx := x + w - padX - metaW
		if closable {
			mx -= 22
		}
		u.dc.text(fSmall(), meta, mx, y+13.5, p.muted)
	}
	tf := face(fSemiBold, 13.5)
	u.dc.text(tf, ellipsize(tf, c.Title, inner-26-metaW-30), x+padX+24, y+14, p.fg)
	if closable {
		cx, cy := x+w-padX-6, y+9
		key := "close:" + c.ID
		col := p.faint
		if u.hoverKey == key {
			u.dc.circle(cx, cy, 10, p.raised)
			col = p.fg
		}
		u.dc.text(face(fRegular, 14), "×", cx-4, cy+4.5, col)
		u.hits = append(u.hits, hit{image.Rect(int(cx-11), int(cy-11), int(cx+11), int(cy+11)), key})
	}
	cy := y + 22
	if c.Body != "" {
		cy += 6
		for _, ln := range wrap(face(fRegular, 12.5), c.Body, inner, 3) {
			cy += 17
			u.dc.text(face(fRegular, 12.5), ln, x+padX, cy-4, mix(p.fg, p.bg, 0.18))
		}
	}
	if c.Code != "" {
		lines := codeLines(c.Code, inner-16)
		bh := float32(16 + 15*len(lines))
		cy += 8
		u.dc.roundRect(x+padX, cy, inner, bh, 9, p.raised)
		for i, ln := range lines {
			u.dc.text(face(fMono, 11.5), ln, x+padX+8, cy+8+11+float32(i)*15, p.fg)
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
			u.dc.roundRect(x+padX, cy, inner, rowH, 10, bg)
			lx := x + padX + 12
			if c.Multi {
				u.dc.roundRect(lx, cy+rowH/2-6, 12, 12, 4, p.line)
				if c.picked[i] {
					u.dc.roundRect(lx+2, cy+rowH/2-4, 8, 8, 2.5, p.green)
				}
				lx += 20
			}
			lf := face(fMedium, 12.5)
			lw := u.dc.text(lf, ellipsize(lf, o.Label, inner*0.55), lx, cy+rowH/2+4.5, p.fg)
			if o.Desc != "" {
				u.dc.text(fSmall(), ellipsize(fSmall(), o.Desc, x+padX+inner-12-(lx+lw+10)), lx+lw+10, cy+rowH/2+4.5, p.muted)
			}
			u.hits = append(u.hits, hit{image.Rect(int(x+padX), int(cy), int(x+padX+inner), int(cy+rowH)), key})
			cy += rowH + 5
		}
		cy -= 5
	}
	if len(c.Buttons) > 0 || c.Footer != "" {
		u.buttonRow(c.Buttons, "btn:", x, w, cy+12, c.Footer)
	}
}
