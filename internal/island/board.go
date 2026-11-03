package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"
)

// The card's layout: the hero (headline, and a way to its terminal when it
// asks), the other sessions a line each, then music and the plugins' rows,
// divided by hairlines. Layout first (what fits), then drawing. Every element
// arrives (rising and fading in, one after another) and glides when it moves.

const (
	lineH    = 30 // another session, one line
	moreH    = 24 // "N more"
	divH     = 13 // the space and hairline between sections
	waitBtnH = 36 // the hero's "Go to terminal"
	cardFoot = 8  // under the last section
)

type sessLayout struct {
	s    *Session
	hero bool
	mode string
}

// height of the hero's body (under its headline), or of a line.
func (l *sessLayout) height() float32 {
	if !l.hero {
		return lineH
	}
	if l.mode == "wait" {
		return waitBtnH
	}
	return 0
}

// footerH is what music and the plugins' rows take. Showing every session, they step aside.
func (u *UI) footerH() float32 {
	h := float32(0)
	if u.allRows {
		return h
	}
	if u.musicShown() {
		h += divH + musicH
	}
	h += float32(len(u.extRows())) * (divH + extH)
	return h
}

// musicShown: the song has a section while it plays, and for a quarter
// of an hour after you pause it; then it steps out of the way.
func (u *UI) musicShown() bool {
	t := u.track
	return t != nil && u.cfg.Music && (t.Playing || t.PausedAt.IsZero() || u.now().Sub(t.PausedAt) < 15*time.Minute)
}

// board lays out the sessions to fit the room the surface has: rows[0] is
// the hero, the rest are lines.
func (u *UI) board(b brief) (rows []sessLayout, hidden int) {
	if b.s == nil {
		return nil, 0
	}
	budget := float32(u.room()-14) - u.headH(b) - cardFoot - u.footerH()
	rows = append(rows, sessLayout{s: b.s, hero: true, mode: b.mode})
	var others []*Session
	for _, s := range b.rows {
		if s != b.s {
			others = append(others, s)
		}
	}
	if len(others) > cardOthers && !u.allRows {
		hidden, others = len(others)-cardOthers, others[:cardOthers]
	}
	for _, s := range others {
		rows = append(rows, sessLayout{s: s})
	}
	total := func() float32 {
		h := rows[0].height()
		if len(rows) > 1 || hidden > 0 || u.allRows {
			h += divH
		}
		for i := range rows[1:] {
			h += rows[i+1].height()
		}
		if hidden > 0 || u.allRows {
			h += moreH
		}
		return h
	}
	for total() > budget && len(rows) > 1 { // the last lines fold into "N more"
		rows = rows[:len(rows)-1]
		hidden++
	}
	return rows, hidden
}

func (u *UI) expandedHeight() int {
	b := u.brief()
	h := u.headH(b)
	rows, hidden := u.board(b)
	if len(rows) > 0 {
		h += rows[0].height()
		if len(rows) > 1 || hidden > 0 || u.allRows {
			h += divH
		}
		for i := range rows[1:] {
			h += rows[i+1].height()
		}
	}
	if hidden > 0 || u.allRows {
		h += moreH
	}
	h += u.footerH() + cardFoot
	return int(math.Min(float64(h), float64(u.room()-14)))
}

// room is the height a card may take: dashboards get the whole surface.
func (u *UI) room() int { return min(u.H, roomH) }

// ---- drawing ----

// item draws one element of the card: arriving (rising and fading in, the
// i-th after the ones before it) and gliding to its place when it moves.
// Its live parts wait until it has arrived.
func (u *UI) item(key string, i int, x, w, y, h float32, draw func(y float32)) {
	u.itemAs(key, key, i, x, w, y, h, draw)
}

// itemAs is item with its own glide key: one thing in two roles (a
// session as the hero's headline and as a line) glides between them.
func (u *UI) itemAs(key, glideKey string, i int, x, w, y, h float32, draw func(y float32)) {
	known := u.gliding(glideKey) && u.enterAt[key].IsZero()
	e := u.enter(key, i, known)
	gy := float32(math.Round(float64(u.glide(glideKey, y) + rise(e)))) // whole pixels: text and shapes move together
	nl := len(u.lives)
	u.rowK = e
	draw(gy)
	u.rowK = 1
	if e < 0.999 {
		for j := nl; j < len(u.lives); j++ { // its live parts arrive with it
			f := u.lives[j].draw
			u.lives[j].draw = func(c *canvas, t float64) { c.k = math.Max(e, 0.001); f(c, t); c.k = 0 }
		}
		u.dc.fade(image.Rect(int(x), int(gy)-2, int(x+w), int(gy+h)+2), e)
	}
}

func (u *UI) drawExpanded(x, w float32) {
	b := u.brief()
	u.lastPage = b.mode
	i := 0
	heroKey := "hero:day"
	if b.s != nil {
		heroKey = "hero:" + b.s.ID
	}
	hh := u.headH(b)
	u.item(heroKey, i, x, w, 0, hh, func(y float32) { u.heroHead(b, x, w, y) })
	i++
	y := hh
	rows, hidden := u.board(b)
	if len(rows) > 0 {
		hero := &rows[0]
		y = u.heroBody(hero, x, w, y, &i)
		if len(rows) > 1 || hidden > 0 || u.allRows {
			u.divider(x, w, y)
			y += divH
		}
		for k := range rows[1:] {
			l := &rows[k+1]
			u.item("line:"+l.s.ID, i, x, w, y, lineH, func(y float32) { u.sessionLine(l.s, x, w, y) })
			y += lineH
			i++
		}
	}
	if hidden > 0 || u.allRows { // a link: every session, a line each, or back
		u.item("more", i, x, w, y, moreH, func(y float32) { u.moreLink(hidden, x, w, y) })
		y += moreH
		i++
	}
	if u.allRows {
		return
	}
	if u.musicShown() {
		u.divider(x, w, y)
		y += divH
		u.item("music", i, x, w, y, musicH, func(y float32) { u.musicRow(x, w, y) })
		y += musicH
		i++
	}
	for _, xp := range u.extRows() { // the plugins' rows, by name
		u.divider(x, w, y)
		y += divH
		u.item("x:"+xp.name, i, x, w, y, extH, func(y float32) { u.extRow(xp, x, w, y) })
		y += extH
		i++
	}
}

// divider: a hairline between sections, inside the margins.
func (u *UI) divider(x, w, y float32) {
	u.dc.roundRect(x+pad, y+float32(divH)/2, w-2*pad, 1, 0.5, u.pal.hair())
}

// heroHead: Ember, the headline, its facts, and the time. The headline is
// the way to that session's terminal.
func (u *UI) heroHead(b brief, x, w, y float32) {
	p, now := u.pal, u.now()
	u.spot = emberSpot{x: x + railX, y: y + 25, s: emberCard, mood: b.mood, show: true}
	defer func() { // after the headline's own hit: resting the pointer on Ember pets it
		u.hits = append(u.hits, hit{image.Rect(int(x+railX-12), int(y+8), int(x+railX+12), int(y+40)), "ember"})
	}()
	right := x + w - pad
	tm := now.Format(u.TimeFmt)
	tmw := measure(fCaption(), tm)
	u.dc.text(fCaption(), tm, right-tmw, y+23, p.faint)
	tx := x + textX
	k := float64(0)
	if b.s != nil {
		key := "focus:" + b.s.ID
		k = u.hl(key)
		u.hits = append(u.hits, hit{image.Rect(int(x+pad-6), int(y+6), int(x+w-pad+6), int(y+u.headH(b)-4)), key})
	}
	lines := u.headLines(b)
	hw := float32(0)
	for j, ln := range lines {
		hw = u.dc.text(fTitle(), ln, tx, y+23+float32(j)*headLineH, p.fg)
	}
	extra := float32(len(lines)-1) * headLineH
	if k > 0.01 { // the way there shows itself under the pointer
		u.arrow(tx+hw+6, y+19+extra, alpha(p.muted, k))
	}
	u.facts(b.sub, tx, y+39+extra, right-tx, p.muted)
	if u.showHint() { // once ever: a page opens from Ember
		u.hintLine(x, y+heroHeadH+extra-4)
	}
	u.hoverLabel(x, y)
}

const headLineH = 18

// headLines is the headline, wrapped to two lines before it's cut short.
func (u *UI) headLines(b brief) []string {
	tmw := measure(fCaption(), u.now().Format(u.TimeFmt))
	return wrap(fTitle(), b.head, expW-textX-pad-tmw-12, 2)
}

// headH is the hero's head: one line of headline, or two.
func (u *UI) headH(b brief) float32 {
	h := heroHeadH + float32(len(u.headLines(b))-1)*headLineH
	if u.showHint() {
		h += hintH
	}
	return h
}

// facts draws "kinvault · 4m in · 3 files +58 −12 · 1 failed": quiet words,
// the diff in its colours, a failure in red.
func (u *UI) facts(s string, x, base, maxW float32, col color.NRGBA) float32 {
	p := u.pal
	f := fCaption()
	s = ellipsize(f, s, maxW)
	x0 := x
	parts := strings.Split(s, " · ")
	for i, part := range parts {
		if i > 0 {
			x += u.dc.text(f, " · ", x, base, p.faint)
		}
		words := strings.Split(part, " ")
		for j, word := range words {
			if j > 0 {
				x += measure(f, " ")
			}
			c := col
			switch {
			case len(word) > 1 && word[0] == '+' && word[1] >= '0' && word[1] <= '9':
				c = mix(p.green, col, 0.25)
			case strings.HasPrefix(word, "−"):
				c = mix(p.red, col, 0.25)
			case strings.HasPrefix(word, "failed") || (strings.HasSuffix(part, "failed") || strings.Contains(part, "failed step")) && word != "":
				c = mix(p.red, col, 0.2)
			}
			if strings.ContainsAny(word, "0123456789") { // numbers roll when they change
				ww := measure(f, word)
				// keyed by the words before it: two lines of a card can share x, part and position
				key := fmt.Sprintf("facts:%.0f:%d:%s", x0, i, strings.Join(words[:j], " "))
				u.rollText(u.dc, key, word, f, x+ww, base, c)
				x += ww
				continue
			}
			x += u.dc.text(f, word, x, base, c)
		}
	}
	return x - x0
}

// heroBody: under the headline, only when the session asks: the way to its
// terminal. Returns where it ends.
func (u *UI) heroBody(l *sessLayout, x, w, y float32, i *int) float32 {
	if l.mode != "wait" || u.allRows {
		return y
	}
	s := l.s
	u.item("wait:"+s.ID, *i, x, w, y, waitBtnH, func(y float32) { u.goThere(s, x+textX, y+4) })
	*i++
	return y + waitBtnH
}

// sessionLine: another session, on one line. Click it and it's the hero.
func (u *UI) sessionLine(s *Session, x, w, y float32) {
	p, now := u.pal, u.now()
	key := "s:" + s.ID
	k := u.hl(key)
	if k > 0.01 {
		u.panel(x+pad-8, y+2, w-2*pad+16, lineH-4, radRow, alpha(p.fg, 0.055*k))
	}
	u.hits = append(u.hits, hit{image.Rect(int(x+pad-8), int(y+2), int(x+w-pad+8), int(y+lineH-2)), key})
	cy := y + lineH/2
	u.sessionGlyph(s, x+railX, cy)
	right := x + w - pad
	base := cy + 4.5
	rx := right
	when := ""
	switch {
	case s.status() == "work" && !s.Since.IsZero():
		when = mins(now.Sub(s.Since))
		if now.Sub(s.Since) < time.Minute {
			when = "<1m"
		}
	case s.State == "wait":
		when = ago(now.Sub(s.Seen))
	case s.Outcome != nil:
		when = ago(now.Sub(s.Outcome.At))
	}
	if k > 0.01 { // under the pointer, a chevron says "open this here"
		u.chevron(right-3, cy, 0, alpha(mix(p.faint, p.fg, 0.5), k))
	}
	if when != "" {
		ww := measure(fCaption(), when)
		u.dc.text(fCaption(), when, rx-ww-12*float32(k), base-0.5, alpha(p.faint, 1-0.6*k))
		rx -= ww + 10
	}
	if n := s.running(); n > 0 {
		t := fmt.Sprint(n)
		tw := measure(fBadge(), t)
		u.dc.text(fBadge(), t, rx-tw, base-1, p.yellow)
		u.dc.icon("agent", rx-tw-7, cy, 10, p.yellow)
		rx -= tw + 22
	}
	tx := x + textX
	nw := u.dc.text(fBody(), ellipsize(fBody(), u.short(s), (rx-tx)*0.45), tx, base, p.fg)
	verb, verbCol := "", p.fg
	what, col := "Idle", p.faint
	switch {
	case s.State == "wait":
		what, col = "Needs you", p.blue
		if s.Asking != "" {
			verb, verbCol, what, col = "Needs you", p.blue, s.Asking, p.muted
		}
	case s.State == "work":
		what, col = s.Label, p.muted
		if what == "" {
			what = "Thinking"
		}
	case s.running() > 0:
		what, col = agentsText(s.running())+" working", p.muted
	case s.Outcome != nil && s.Outcome.Failed:
		verb, verbCol, what, col = "Stopped", p.red, s.Outcome.Error, p.muted
	case s.Outcome != nil && now.Sub(s.Outcome.At) < outcomeFresh:
		verb, verbCol, what, col = "Done", p.green, s.task(), p.muted
	}
	ax := tx + nw + 8
	if verb != "" {
		ax += u.dc.text(fSecond(), verb, ax, base, verbCol)
		if what != "" {
			ax += u.dc.text(fSecond(), " · ", ax, base, p.faint)
		}
	}
	t := ellipsize(fSecond(), what, rx-ax)
	u.dc.text(fSecond(), t, ax, base, col)
	if s.State == "work" {
		u.shimmerLive(fSecond(), t, ax, base, mix(p.fg, colorWhite, 0.25))
	}
}

// moreLink: "2 more" / "Show less", centred, quiet.
func (u *UI) moreLink(hidden int, x, w, y float32) {
	p := u.pal
	t := plural(hidden, "more session")
	if u.allRows {
		t = "Show less"
		if hidden > 0 { // more than the surface can hold even so
			t = plural(hidden, "more session") + " · Show less"
		}
	}
	k := u.hl("rows:all")
	tw := measure(fCaption(), t)
	tx := x + (w-tw)/2
	if k > 0.01 {
		u.panel(tx-10, y+3, tw+20, moreH-6, 9, alpha(p.fg, 0.06*k))
	}
	u.dc.text(fCaption(), t, tx, y+15.5, mix(p.faint, p.fg, 0.6*k))
	u.hits = append(u.hits, hit{image.Rect(int(tx-10), int(y), int(tx+tw+10), int(y+moreH)), "rows:all"})
}

// ---- the glyphs ----

// beat is the one rhythm every pulse keeps (rings, the halo, orbits), so
// the island breathes together instead of flickering in many tempos.
const beat = 1.6

// sessionGlyph says a session's state at a glance. Working: an orbit with a
// bead going round it, and a smaller satellite per subagent on an outer
// track. Waiting: a blue pulse. Done: a check. Idle: an empty ring.
func (u *UI) sessionGlyph(s *Session, gx, gy float32) {
	p, now := u.pal, u.now()
	switch {
	case s.State == "wait":
		u.dc.circle(gx, gy, 3.6, p.blue)
		u.lives = append(u.lives, live{r: image.Rect(int(gx-12), int(gy-12), int(gx+13), int(gy+13)), draw: func(c *canvas, t float64) {
			q := math.Mod(t/beat, 1)
			r := 4 + 6*float32(easeOut(q))
			c.frame(gx-r, gy-r, 2*r, 2*r, r, 1.2, alpha(p.blue, 0.6*(1-q)*math.Min(1, q*6)))
		}})
	case s.status() == "work":
		u.dc.arc(gx, gy, 6, 1.1, 0, 2*math.Pi, alpha(p.green, 0.4))
		n := min(s.running(), 4)
		u.lives = append(u.lives, live{r: image.Rect(int(gx-12), int(gy-12), int(gx+13), int(gy+13)), draw: func(c *canvas, t float64) {
			a := t * 2 * math.Pi / (2 * beat) // one turn every two beats
			c.circle(gx+6*float32(math.Cos(a)), gy+6*float32(math.Sin(a)), 1.9, p.green)
			for i := 0; i < n; i++ { // the subagents, round the other way
				b := -t*1.7 + float64(i)*2*math.Pi/float64(n)
				c.circle(gx+10*float32(math.Cos(b)), gy+10*float32(math.Sin(b)), 1.3, p.yellow)
			}
		}})
	case s.Outcome != nil && s.Outcome.Failed:
		u.dc.line(pt{gx - 3, gy - 3}, pt{gx + 3, gy + 3}, 1.6, p.red)
		u.dc.line(pt{gx + 3, gy - 3}, pt{gx - 3, gy + 3}, 1.6, p.red)
	case s.Outcome != nil && now.Sub(s.Outcome.At) < outcomeFresh:
		u.dc.icon("check", gx, gy, 10, p.green)
	default:
		u.dc.arc(gx, gy, 5, 1.1, 0, 2*math.Pi, alpha(p.fg, 0.2))
	}
}

// chevron points right when k is 0 and down when it's 1.
func (u *UI) chevron(cx, cy float32, k float64, col color.NRGBA) {
	a := k * math.Pi / 2
	rot := func(x, y float32) pt {
		c, s := float32(math.Cos(a)), float32(math.Sin(a))
		return pt{cx + x*c - y*s, cy + x*s + y*c}
	}
	u.dc.line(rot(-1.5, -3.2), rot(1.7, 0), 1.3, col)
	u.dc.line(rot(1.7, 0), rot(-1.5, 3.2), 1.3, col)
}

// arrow: a small arrow up and out, for "go there".
func (u *UI) arrow(x, cy float32, col color.NRGBA) {
	u.dc.line(pt{x, cy + 3}, pt{x + 6, cy - 3}, 1.3, col)
	u.dc.line(pt{x + 1.5, cy - 3}, pt{x + 6, cy - 3}, 1.3, col)
	u.dc.line(pt{x + 6, cy - 3}, pt{x + 6, cy + 1.5}, 1.3, col)
}

// goThere: the way to a session's terminal, a filled blue button at x.
func (u *UI) goThere(s *Session, x, y float32) float32 {
	p := u.pal
	key := "focus:" + s.ID
	k, d := float32(u.hl(key)), float32(u.pressed(key))
	f := face(fSemiBold, 12)
	t := "Answer in terminal"
	tw := measure(f, t)
	w := tw + 26 + 12
	in := 1.5 * d
	u.dc.roundRect(x+in, y+in, w-2*in, btnH-2*in, btnH/2, mix(p.blue, colorWhite, float64(0.12*k+0.2*d)))
	u.dc.text(f, t, x+13, y+btnH/2+4.2, p.ink)
	u.arrow(x+13+tw+6, y+btnH/2, p.ink)
	u.hits = append(u.hits, hit{image.Rect(int(x), int(y), int(x+w), int(y+btnH)), key})
	return w
}
