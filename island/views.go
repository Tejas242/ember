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

const maxComp = 400

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
	work        bool    // Claude is working: the label shimmers
	trail       string  // the second activity, as a satellite at the pill's end
	m           *moment // a system moment: icon, meter, value, battery
}

const meterW = 86

func (u *UI) compactLine() line {
	l := u.mainLine()
	l.trail = u.secondary(u.primary())
	return l
}

// trailW is the satellite's room: gap, hairline, gap, glyph.
const trailW = 8 + 1 + 7 + 18

func (u *UI) mainLine() line {
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
			if x.status() != "idle" {
				busy++
			}
		}
		if s.State == "idle" && s.running() > 0 { // the turn ended; its agents work on
			l := line{key: "claude:agents:" + s.ID, mood: moodWork, textCol: p.fg, more: busy - 1, work: true}
			ags := s.agentList()
			if len(ags) == 1 {
				l.text = ags[0].Label
				if l.text == "" {
					l.text = ags[0].name()
				}
				l.chip = mins(now.Sub(ags[0].Start))
			} else {
				l.text = agentsText(len(ags)) + " working"
				l.chip = s.Project
			}
			l.chipCol = p.muted
			return l
		}
		switch s.State {
		case "wait":
			return line{key: "claude:wait:" + s.ID, mood: moodWait, text: s.Project + " needs you", textCol: p.blue, more: busy - 1}
		case "work":
			l := line{key: "claude:" + s.ID, mood: moodWork, text: s.Label, plus: s.Plus, minus: s.Minus, textCol: p.fg, more: busy - 1, work: true}
			if l.text == "" {
				l.text = "Thinking"
			}
			var chips []string
			if n := s.running(); n > 0 {
				chips = append(chips, agentsText(n))
			}
			if !s.StepAt.IsZero() && now.Sub(s.StepAt) >= time.Minute { // one step running long: say how long
				chips = append(chips, mins(now.Sub(s.StepAt)))
			}
			l.chip, l.chipCol = strings.Join(chips, " · "), p.muted
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
			w += 10 + float64(tabWidth(fValue(), m.right))
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
		w += 7 + float64(tabWidth(face(fMedium, 12), diffText(l.plus, l.minus)))
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
	if l.trail != "" {
		w += trailW
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
	case vDash:
		w, h := u.dashboards[u.dash].Size(u)
		return w, math.Min(h, float64(u.H-14))
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

	// the right end, laid out first: the satellite, privacy dot, battery,
	// value, meter, lock, chip
	rx := x + w - 12
	if l.trail != "" {
		u.drawTrail(l.trail, rx-18, cy, h)
		u.dc.roundRect(rx-18-7-1, cy-6, 1, 12, 0.5, alpha(p.fg, 0.14))
		rx -= trailW
	}
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
		if m.right != "" { // the value rolls as it changes
			col := p.fg
			if m.tint == p.muted {
				col = p.muted
			}
			rx -= u.rollText(u.dc, "pill:value", m.right, fValue(), rx, base, col) + 10
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
		avail -= 7 + tabWidth(face(fMedium, 12), diff)
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
	} else if l.work { // lit from inside while Claude works
		u.dc.text(fText(), text, tx, base, mix(l.textCol, p.bg, 0.22))
		u.shimmerLive(fText(), text, tx, base, mix(l.textCol, colorWhite, 0.55))
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
		df := face(fMedium, 12)
		if l.plus > 0 {
			s := fmt.Sprintf("+%d", l.plus)
			tx += u.rollText(u.dc, "pill:plus", s, df, tx+tabWidth(df, s), base, p.green) + 4
		}
		if l.minus > 0 {
			s := fmt.Sprintf("−%d", l.minus)
			tx += u.rollText(u.dc, "pill:minus", s, df, tx+tabWidth(df, s), base, p.red)
		}
	}
	if l.more > 0 {
		tx += 8
		tx += u.dc.text(fSmall(), fmt.Sprintf("+%d", l.more), tx, base, p.muted)
	}
	if l.music && u.track != nil {
		u.eqLive(tx+8, cy+5, 10, u.accent(p.green), u.track.Playing)
		u.progressLive(x+16, h-2.5, w-32, 1.5, u.accent(p.fg), 0) // a hairline of the song under the pill
	}
}

// accent is the playing cover's colour, else fallback.
func (u *UI) accent(fallback color.NRGBA) color.NRGBA {
	if u.art != nil && u.art.accent.A > 0 {
		return u.art.accent
	}
	return fallback
}

// drawTrail draws the second activity as a small satellite glyph in an 18 px
// slot at x: the cover (or an equaliser), a tiny flickering flame, a timer.
// Clicking it swaps it with the pill's main line.
func (u *UI) drawTrail(act string, x, cy, h float32) {
	p := u.pal
	u.hits = append(u.hits, hit{image.Rect(int(x-9), 0, int(x+20), int(h)), "trail"})
	k := float32(u.hl("trail"))
	switch act {
	case "music":
		if u.art != nil {
			u.dc.drawArt(u.art.small, int(x), int(cy-9), 9, 1) // a round disc
			if u.track != nil && u.track.Playing {
				u.lives = append(u.lives, live{image.Rect(int(x)-2, int(cy-11), int(x+20), int(cy+11)), func(c *canvas, t float64) {
					a := t * 2.2 // a dot orbiting the disc: it's spinning
					c.circle(x+9+7.2*float32(math.Cos(a)), cy+7.2*float32(math.Sin(a)), 1.3, alpha(colorWhite, 0.85))
				}})
			}
		} else {
			u.eqLive(x+1, cy+5, 10, u.accent(p.green), u.track != nil && u.track.Playing)
		}
	case "claude":
		m := moodWork
		if ss := u.sessionList(); len(ss) > 0 && ss[0].State == "wait" {
			m = moodWait
		}
		col := p.moodColor(m)
		u.lives = append(u.lives, live{image.Rect(int(x), 0, int(x+19), int(h)), func(c *canvas, t float64) {
			c.ember(p, x+9, cy+1, 11+k, m, t, 0, 0, col, 1, 0)
		}})
	case "edge":
		if e := u.edge; e != nil {
			txt := mins(u.now().Sub(time.UnixMilli(e.StartedMs)))
			f := face(fSemiBold, 10)
			u.dc.text(f, txt, x+9-measure(f, txt)/2, cy+3.5, p.green)
		}
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
func (u *UI) progressLive(x, y, w, th float32, col color.NRGBA, knob float32) {
	t := u.track
	if t == nil || t.Length <= 0 {
		return
	}
	r := image.Rect(int(x)-8, int(y-th)-8, int(x+w)+9, int(y+th)+9)
	u.lives = append(u.lives, live{r, func(c *canvas, _ float64) {
		k := float32(math.Min(1, float64(t.pos(u.now()))/float64(t.Length)))
		if u.drag == "music:seek" {
			k = float32(u.seekPreview)
		}
		c.roundRect(x, y-th/2, w, th, th/2, alpha(u.pal.line, 0.9))
		c.roundRect(x, y-th/2, max(w*k, th), th, th/2, alpha(col, 0.9))
		if knob > 0.02 { // a knob grows on the big bar as you come near
			c.circle(x+w*k, y, 6.5*knob, u.pal.fg)
		}
	}})
}

// mediaGlyph draws prev / play-pause / next centred at (cx, cy), scaled by k.
func (u *UI) mediaGlyph(kind string, cx, cy float32, playing bool, col color.NRGBA, k float32) {
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
			u.dc.roundRect(cx-5.5*k, cy-7.5*k, 4*k, 15*k, 1.5*k, col)
			u.dc.roundRect(cx+1.5*k, cy-7.5*k, 4*k, 15*k, 1.5*k, col)
		} else {
			tri(cx-4.5*k, cy-8*k, cx-4.5*k, cy+8*k, cx+8.5*k, cy)
		}
	case "prev":
		u.dc.roundRect(cx-7*k, cy-5.5*k, 2.4*k, 11*k, 1*k, col)
		tri(cx+5*k, cy-5.5*k, cx+5*k, cy+5.5*k, cx-4*k, cy)
	case "next":
		u.dc.roundRect(cx+4.6*k, cy-5.5*k, 2.4*k, 11*k, 1*k, col)
		tri(cx-5*k, cy-5.5*k, cx-5*k, cy+5.5*k, cx+4*k, cy)
	}
}

// powerText: "2h 41m left", "full in 1h 05m", "fully charged".
func powerText(pw *power) string {
	dur := func(sec int64) string {
		d := time.Duration(sec) * time.Second
		if d < time.Hour {
			return fmt.Sprintf("%dm", int(d.Minutes()))
		}
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	switch {
	case pw.state == 4:
		return "Fully charged"
	case pw.state == 1 && pw.ttf > 0:
		return "Full in " + dur(pw.ttf)
	case pw.state == 1:
		return "Charging"
	case pw.state == 2 && pw.tte > 0:
		return dur(pw.tte) + " left"
	case charging(pw.state):
		return "Plugged in"
	}
	return ""
}
