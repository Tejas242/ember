package island

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"slices"
	"strings"
	"time"

	"golang.org/x/image/vector"
)

const maxComp = 400


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
	return fmt.Sprintf("%dh %02dm", m/60, m%60)
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

// line is what the pill says. It changes when the situation changes
// (working → done), not with every tool call: the card has the detail.
type line struct {
	key      string
	mood     mood
	verb     string // a word before the label in the mood's colour: Done, Stopped
	text     string
	sub      string // secondary words after the label
	textCol  color.NRGBA
	badge    string // "+2": more sessions than the one named
	timer    string // how long it has been at it, at the right end
	timerCol color.NRGBA
	subCol   color.NRGBA // the secondary words' colour, if not the usual
	agents   int         // subagents running: an amber spark and a count
	chip     string // a quiet value at the right end (edge's minutes, its task)
	chipCol  color.NRGBA
	lock     bool
	music    bool
	work     bool    // Claude is working: the label shimmers
	halo     bool    // waiting on you: a blue light breathes around the pill
	trail    string  // the second activity, as a satellite at the pill's end
	m        *moment // a system moment: icon, meter, value, battery
}

// tick is the part of the line that changes with time alone.
func (l line) tick() string { return l.timer + "|" + l.chip }

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
		l := line{key: "moment:" + m.key, mood: m.mood, verb: m.verb, text: m.text, sub: m.sub, textCol: p.fg}
		if m.icon != "" || m.thumb != nil {
			l.m = m
		}
		return l
	case "claude":
		if len(ss) == 0 {
			break
		}
		// the pill names a session and says how long it has been at it; what
		// it's doing step by step is the card's job
		var waiting, working []*Session
		for _, x := range ss {
			switch {
			case x.State == "wait":
				waiting = append(waiting, x)
			case x.status() == "work":
				working = append(working, x)
			}
		}
		switch {
		case len(waiting) == 1:
			return line{key: "claude:wait", mood: moodWait, text: u.short(waiting[0]) + " needs you", textCol: p.blue, halo: true}
		case len(waiting) > 1:
			return line{key: "claude:wait", mood: moodWait, text: fmt.Sprintf("%d sessions need you", len(waiting)), textCol: p.blue, halo: true}
		case len(working) > 0:
			s := working[0]
			// subagents are detail of the same work: the card has them
			l := line{key: "claude:work", mood: moodWork, text: u.short(s), textCol: p.fg, work: true, timerCol: p.muted}
			if len(working) > 1 {
				l.badge = fmt.Sprintf("+%d", len(working)-1)
			}
			if d := now.Sub(s.Since); !s.Since.IsZero() && d >= time.Minute {
				l.timer = mins(d)
			}
			if stalled(s, now) { // nothing heard for a while: it may be stuck
				l.timerCol = p.yellow
				if l.timer == "" {
					l.timer = mins(now.Sub(s.Since))
				}
			}
			return l
		}
		return line{key: "claude:idle", mood: moodIdle, text: fmt.Sprintf("Claude · %d idle", len(ss)), textCol: p.muted}
	case "music":
		if t := u.track; t != nil {
			col := p.fg
			if !t.Playing {
				col = p.muted
			}
			return line{key: "music", mood: moodIdle, text: t.Title, sub: t.Artist, textCol: col, music: true}
		}
	case "edge":
		if e := u.edge; e != nil {
			if e.StartedMs > 0 && !e.Pending {
				return line{key: "edge", mood: moodWork, text: short(e.Task), textCol: p.fg,
					timer: mins(now.Sub(time.UnixMilli(e.StartedMs)))}
			}
			l := line{key: "edge", mood: moodNudge, text: fmt.Sprintf("Today %d/%d", e.Today, e.Need), textCol: p.fg, lock: e.Closed}
			if e.Task != "" {
				l.chip, l.chipCol = short(e.Task), p.muted
			}
			return l
		}
	}
	l := line{key: "clock", mood: moodIdle, text: now.Format(u.cfg.pillFormat()), textCol: p.fg}
	if e := u.edge; e != nil && e.Closed {
		l.lock = true
	}
	// what finished or stopped while you weren't looking stays, quietly,
	// beside the time until you've seen it
	var done, failed []*Session
	for _, s := range ss {
		if !s.Unseen.IsZero() && s.Recap != nil && u.cfg.Claude {
			if s.Recap.Failed {
				failed = append(failed, s)
			} else {
				done = append(done, s)
			}
		}
	}
	switch {
	case len(failed) == 1:
		l.sub, l.subCol = u.short(failed[0])+" stopped", mix(p.red, p.muted, 0.2)
	case len(failed) > 1:
		l.sub, l.subCol = fmt.Sprintf("%d stopped", len(failed)), mix(p.red, p.muted, 0.2)
	case len(done) == 1:
		l.sub, l.subCol = u.short(done[0])+" done", mix(p.green, p.muted, 0.35)
	case len(done) > 1:
		l.sub, l.subCol = fmt.Sprintf("%d done", len(done)), mix(p.green, p.muted, 0.35)
	}
	if l.sub != "" {
		l.key = "clock:" + l.sub
	}
	return l
}

// stalled: a session working that hasn't been heard from in three minutes,
// a long build, or a hang. Either way, worth a look.
func stalled(s *Session, now time.Time) bool {
	return s.State == "work" && now.Sub(s.Seen) > 3*time.Minute
}

func agentsText(n int) string {
	if n == 1 {
		return "1 agent"
	}
	return fmt.Sprintf("%d agents", n)
}

func battW() float32 { return 9 * 1.9 }

// seg is one element of the pill: its room, the space before it, and how
// to draw it with its left edge at x. Measuring and drawing share the list,
// so the pill is always exactly as wide as what it shows.
type seg struct {
	w, gap float32
	draw   func(x float32)
}

var (
	fPill    = func() fontFace { return face(fMedium, 12.5) }
	fPillSub = func() fontFace { return face(fRegular, 12) }
	fPillVal = func() fontFace { return face(fSemiBold, 12) }
	fTimer   = func() fontFace { return face(fMedium, 11.5) }
	fBadge   = func() fontFace { return face(fSemiBold, 10) }
)

const (
	pillEnd  = 10 // the pill's ends
	leadW    = 16 // the leading glyph's slot
	leadGap  = 8
	maxSub   = 160 // secondary words, at most
	minLabel = 40  // the label shrinks to this before the pill stops growing
)

// pillLayout lays out the pill for line l: the left group (lead, verb,
// label, words, badge) and the right group (timer, values, satellite).
func (u *UI) pillLayout(l line, h float32) (left, right []seg, w float32) {
	p, now := u.pal, u.now()
	cy := h / 2
	base := cy + 4.5
	m := l.m

	// the lead: Ember, the cover, a moment's icon or a screenshot
	leadSlot := float32(leadW)
	if m != nil && m.thumb != nil {
		leadSlot = float32(m.thumb.Rect.Dx())
	}
	left = append(left, seg{w: leadSlot, draw: func(x float32) {
		switch {
		case m != nil && m.thumb != nil:
			th := m.thumb.Rect
			ty := int(cy+0.5) - th.Dy()/2
			draw.Draw(u.dc.img, th.Add(image.Pt(int(x), ty)), m.thumb, image.Point{}, draw.Over)
			u.dc.frame(x, float32(ty), float32(th.Dx()), float32(th.Dy()), 3.5, 1, alpha(p.fg, 0.22))
			u.spot = emberSpot{x: x + float32(th.Dx())/2, y: cy, s: 0, mood: moodIdle, show: true}
		case m != nil:
			u.dc.icon(m.icon, x+8, cy, 15, m.tint)
			u.spot = emberSpot{x: x + 8, y: cy + 1, s: 0, mood: l.mood, show: true} // Ember tucks into the icon
		case l.music && u.art != nil:
			u.dc.drawArt(u.art.small, int(x-1), int(cy-9), 5, 1)
			u.spot = emberSpot{x: x + 8, y: cy + 1, s: 0, mood: moodIdle, show: true} // Ember tucks into the cover
		default:
			u.spot = emberSpot{x: x + 8, y: cy + 1, s: 14, mood: l.mood, show: true}
		}
	}})

	// the right group, laid out first: it's fixed, the words take what's left
	if l.timer != "" {
		tw := tabWidth(fTimer(), l.timer)
		right = append(right, seg{w: tw, gap: 8, draw: func(x float32) {
			u.rollText(u.dc, "pill:timer", l.timer, fTimer(), x+tw, base, l.timerCol)
		}})
	}
	if l.agents > 0 {
		t := fmt.Sprint(l.agents)
		tw := measure(fBadge(), t)
		right = append(right, seg{w: 12 + tw, gap: 10, draw: func(x float32) {
			u.dc.icon("agent", x+5, cy, 10, p.yellow)
			u.dc.text(fBadge(), t, x+12, base-0.5, p.yellow)
		}})
	}
	if l.chip != "" {
		cw := min(measure(fPillSub(), l.chip), 120)
		right = append(right, seg{w: cw + 8, gap: 8, draw: func(x float32) {
			u.dc.circle(x+1.5, cy, 1.5, p.faint)
			u.dc.text(fPillSub(), ellipsize(fPillSub(), l.chip, cw), x+8, base, l.chipCol)
		}})
	}
	if l.lock {
		right = append(right, seg{w: 11, gap: 8, draw: func(x float32) { u.dc.lock(x, cy-6, 11, p.yellow) }})
	}
	if m != nil && m.meter {
		right = append(right, seg{w: meterW, gap: 10, draw: func(x float32) {
			k := float32(math.Max(0, math.Min(1, u.meter.x)))
			u.dc.roundRect(x, cy-2, meterW, 4, 2, alpha(p.fg, 0.14))
			if k > 0 {
				u.dc.roundRect(x, cy-2, max(meterW*k, 4), 4, 2, m.tint)
			}
		}})
	}
	if m != nil && m.right != "" { // the value rolls as it changes
		vw := max(tabWidth(fPillVal(), m.right), tabWidth(fPillVal(), "100"))
		if m.batt {
			vw = tabWidth(fPillVal(), m.right)
		}
		right = append(right, seg{w: vw, gap: 10, draw: func(x float32) {
			col := p.fg
			if m.tint == p.muted {
				col = p.muted
			}
			u.rollText(u.dc, "pill:value", m.right, fPillVal(), x+vw, base, col)
		}})
	}
	if m != nil && m.batt {
		bw := battW() + 3
		right = append(right, seg{w: bw, gap: 6, draw: func(x float32) {
			u.dc.battery(x, cy, 9, float32(m.pct), alpha(p.fg, 0.45), m.tint)
		}})
	}
	if u.casting { // someone can see your screen
		right = append(right, seg{w: 7, gap: 8, draw: func(x float32) { u.dc.circle(x+3.5, cy, 3.2, mix(p.red, p.yellow, 0.35)) }})
	}
	if l.music && u.track != nil {
		right = append(right, seg{w: 14, gap: 10, draw: func(x float32) {
			u.eqLive(x, cy+5, 10, u.accent(p.green), u.track.Playing)
		}})
	}
	if l.trail != "" {
		right = append(right, seg{w: trailW - 8, gap: 8, draw: func(x float32) {
			u.dc.roundRect(x, cy-6, 1, 12, 0.5, alpha(p.fg, 0.14))
			u.drawTrail(l.trail, x+8, cy, h)
		}})
	}
	slices.Reverse(right) // laid out from the right end inwards
	fixed := float32(2 * pillEnd)
	for _, s := range append(append([]seg{}, left...), right...) {
		fixed += s.w + s.gap
	}

	// the words: verb, label, secondary words, badge
	var verbW, badgeW float32
	if l.verb != "" {
		verbW = measure(fPillVal(), l.verb) + 5
	}
	if l.badge != "" {
		badgeW = measure(fBadge(), l.badge) + 10 + 6
	}
	labelW := measure(fPill(), l.text)
	subW := float32(0)
	if l.sub != "" {
		subW = min(maxSub, measure(fPillSub(), l.sub)) + 8
	}
	gap := float32(leadGap)
	if l.text == "" && l.verb == "" {
		gap, labelW = 0, 0
	}
	room := float32(maxComp) - fixed - gap - verbW - badgeW
	if labelW+subW > room { // too long: the secondary words give way first, then the label
		subW = max(0, room-labelW)
		if subW < 40 {
			subW = 0
		}
		labelW = max(minLabel, min(labelW, room-subW))
	}
	words := verbW + labelW + subW + badgeW
	if words > 0 {
		slideKey := l.key
		left = append(left, seg{w: words, gap: gap, draw: func(x float32) {
			if l.verb != "" {
				x += u.dc.text(fPillVal(), l.verb, x, base, p.moodColor(l.mood)) + 5
			}
			text := ellipsize(fPill(), l.text, labelW)
			// a label that changes in place slides: the old one up and out, the new one in from below
			if slideKey == u.lineKey && l.text != u.lineText && u.lineText != "" {
				u.slideOld, u.slideAt = u.lineText, now
			}
			u.lineKey, u.lineText = slideKey, l.text
			if k := float64(now.Sub(u.slideAt)) / float64(slideTime); k < 1 {
				e := float32(easeOut(k))
				u.dc.text(fPill(), ellipsize(fPill(), u.slideOld, labelW), x, base-9*e, alpha(l.textCol, 1-float64(e)))
				u.dc.text(fPill(), text, x, base+9*(1-e), alpha(l.textCol, float64(e)))
			} else if l.work { // lit from inside while Claude works
				u.dc.text(fPill(), text, x, base, mix(l.textCol, p.bg, 0.18))
				u.shimmerLive(fPill(), text, x, base, mix(l.textCol, colorWhite, 0.6))
			} else {
				u.dc.text(fPill(), text, x, base, l.textCol)
			}
			x += measure(fPill(), text)
			if subW > 0 {
				col := p.muted
				if l.subCol.A > 0 {
					col = l.subCol
				}
				u.dc.text(fPillSub(), ellipsize(fPillSub(), l.sub, subW-8), x+8, base, col)
				x += subW
			}
			if l.badge != "" { // more sessions than the one named
				bw := badgeW - 6
				u.dc.roundRect(x+6, cy-7, bw, 14, 7, alpha(p.fg, 0.09))
				u.dc.text(fBadge(), l.badge, x+6+5, cy+3.5, p.muted)
			}
		}})
	}
	w = fixed + gap + words
	lo := float32(70)
	if m != nil && m.meter {
		lo = 168 // the bar shouldn't jump as the number changes
	}
	return left, right, min(maxComp, max(lo, w))
}

func (u *UI) compactWidth(l line) float64 {
	_, _, w := u.pillLayout(l, u.BarH)
	return float64(w)
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
	left, right, _ := u.pillLayout(l, h)
	lx := x + pillEnd
	for _, s := range left {
		lx += s.gap
		s.draw(lx)
		lx += s.w
	}
	rx := x + w - pillEnd
	for _, s := range right {
		rx -= s.w
		s.draw(rx)
		rx -= s.gap
	}
	if l.music && u.track != nil { // the song, as a hairline along the pill's rim
		u.progressLive(x+16, h-2.5, w-32, 1.5, u.accent(p.fg), 0)
	}
	if l.halo {
		u.haloLive(p.blue)
	}
	if m := u.moment; m != nil && l.m == nil && m.mood == moodHappy && now.Sub(m.at) < sweepTime {
		u.sweepLive(m.at, p.green)
	}
}

const sweepTime = 900 * time.Millisecond

// haloLive: a soft light breathes around the pill while something waits
// on you. It lives outside the body, so it repaints alone.
func (u *UI) haloLive(col color.NRGBA) {
	n := u.shape
	since := u.trAt
	u.lives = append(u.lives, live{n.bounds().Inset(-12), func(c *canvas, t float64) {
		in := math.Min(1, u.now().Sub(since).Seconds()/0.6) // it comes up gently
		b := 0.4 + 0.6*(0.5+0.5*math.Sin(t*2*math.Pi/1.6))
		for i := 8; i >= 2; i-- { // the light spilling out
			a := 70 * math.Exp(-math.Pow(float64(i)/3.6, 2)) * b * in
			c.fillNotch(n, float32(i)*1.4, alpha(col, a/255), &n, 0, 0)
		}
		c.fillNotch(n, 1.1, alpha(col, (0.35+0.5*b)*in), &n, 0, 0) // and a lit rim
	}})
}

// sweepLive: one band of light crosses the pill when something succeeds.
func (u *UI) sweepLive(at time.Time, col color.NRGBA) {
	n := u.shape
	u.tweening = true // a single pass at the full frame rate
	u.lives = append(u.lives, live{n.interior(), func(c *canvas, _ float64) {
		k := u.now().Sub(at).Seconds() / sweepTime.Seconds()
		if k >= 1 {
			return
		}
		e := easeOut(k)
		cx := float64(n.x) - 40 + e*float64(n.w+80)
		r := n.interior().Intersect(c.img.Rect)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			row := c.img.Pix[y*c.img.Stride:]
			for x := max(r.Min.X, int(cx-36)); x < min(r.Max.X, int(cx+36)); x++ {
				d := math.Abs(float64(x)-cx) / 18
				a := 0.16 * math.Exp(-d*d) * (1 - k)
				i := x * 4
				for ch, v := range [3]uint8{col.R, col.G, col.B} {
					row[i+ch] = uint8(float64(row[i+ch])*(1-a) + float64(v)*a)
				}
			}
		}
	}})
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

