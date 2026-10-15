package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"
)

// The card's layout: the hero (headline, and its timeline or recap), the
// other sessions a line each, then music and edge, divided by hairlines.
// Layout first (what fits), then drawing. Every element arrives (rising and
// fading in, one after another) and glides when it moves.

const (
	lineH      = 30 // another session, one line
	agentH     = 22 // a subagent on the branch
	agentStepH = 18 // one of its steps, when it's open
	recapLineH = 16 // a line of Claude's last word
	moreH      = 24 // "N more"
	divH       = 13 // the space and hairline between sections
	waitBtnH   = 36 // the hero's "Go to terminal"
	dayH       = 46 // the day, hour by hour
	cardFoot   = 8  // under the last section
)

type sessLayout struct {
	s       *Session
	hero    bool
	open    bool // the hero, showing its timeline
	steps   []Step
	agents  []*Agent
	openAg  map[string]bool
	agSteps int      // steps an open agent shows
	recap   []string // Claude's last word, wrapped (a finished hero)
	mode    string
}

// height of the hero's body (under its headline), or of a line.
func (l *sessLayout) height() float32 {
	if !l.open {
		if l.hero { // "Show all": the headline stands for it
			return 0
		}
		return lineH
	}
	h := float32(0)
	switch l.mode {
	case "wait":
		h += waitBtnH
	case "recap":
		return float32(max(len(l.recap), 1))*recapLineH + 10
	}
	h += float32(len(l.steps)) * stepH
	for _, ag := range l.agents {
		h += agentH
		if l.openAg[ag.ID] {
			h += float32(min(len(ag.Hist), l.agSteps)) * agentStepH
			if ag.Summary != "" {
				h += agentStepH
			}
		}
	}
	if h > 0 {
		h += 4
	}
	return h
}

// footerH is what music and edge take. Showing every session, they step aside.
func (u *UI) footerH() float32 {
	h := float32(0)
	if u.allRows {
		return h
	}
	if u.musicShown() {
		h += divH + musicH
	}
	if u.edgeRowShown() {
		h += divH + edgeH
	}
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
	budget := float32(u.H-14) - heroHeadH - cardFoot - u.footerH()
	hero := sessLayout{s: b.s, hero: true, open: !u.allRows, mode: b.mode, agSteps: agentHist, openAg: map[string]bool{}}
	if b.mode == "work" || b.mode == "wait" {
		hero.agents = append(b.s.agentList(), reversed(b.s.Finished)...)
		for _, ag := range hero.agents {
			hero.openAg[ag.ID] = u.opened["a:"+b.s.ID+":"+ag.ID]
		}
		keep := b.keep
		if b.mode == "wait" {
			keep = min(keep, 2)
		}
		hero.steps = transcript(b.s, hero.agents, keep)
	}
	if b.mode == "recap" {
		say := b.s.Recap.Summary
		if b.s.Recap.Failed {
			say = b.s.Recap.Error
		}
		if say != "" {
			hero.recap = wrap(fSecond(), say, expW-textX-pad, 3)
		}
	}
	rows = append(rows, hero)
	var others []*Session
	for _, s := range b.rows {
		if s != b.s {
			others = append(others, s)
		}
	}
	if n := u.cfg.CardOthers; len(others) > n && !u.allRows { // [card] others
		hidden, others = len(others)-n, others[:n]
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
	for total() > budget {
		h := &rows[0]
		switch {
		case h.agSteps > 1 && anyOpen(h.openAg): // agents' own steps first
			h.agSteps--
		case len(h.steps) > 1: // then the timeline, oldest first
			h.steps = h.steps[:len(h.steps)-1]
		case len(rows) > 1: // then the last lines fold into "N more"
			rows = rows[:len(rows)-1]
			hidden++
		case closeLast(h):
		case len(h.agents) > 0:
			h.agents = h.agents[:len(h.agents)-1]
		case len(h.steps) > 0:
			h.steps = h.steps[:len(h.steps)-1]
		default:
			return rows, hidden
		}
	}
	return rows, hidden
}

// closeLast closes the lowest agent that's open on the hero's branch.
func closeLast(l *sessLayout) bool {
	for i := len(l.agents) - 1; i >= 0; i-- {
		if id := l.agents[i].ID; l.openAg[id] {
			l.openAg[id] = false
			return true
		}
	}
	return false
}

func anyOpen(m map[string]bool) bool {
	for _, v := range m {
		if v {
			return true
		}
	}
	return false
}

func reversed(a []*Agent) []*Agent {
	out := make([]*Agent, len(a))
	for i, x := range a {
		out[len(a)-1-i] = x
	}
	return out
}

// transcript is a session's latest steps, newest first, less the launches
// of agents that have a row on the branch.
func transcript(s *Session, agents []*Agent, keep int) []Step {
	launched := map[string]bool{}
	for _, ag := range agents {
		if ag.Desc != "" {
			launched[ag.Desc] = true
		}
	}
	var out []Step
	for i := len(s.Turn.Steps) - 1; i >= 0 && len(out) < keep; i-- {
		st := s.Turn.Steps[i]
		if st.Kind == "agent" && launched[strings.TrimPrefix(st.Text, "Asked an agent: ")] {
			continue
		}
		out = append(out, st)
	}
	return out
}

func (u *UI) expandedHeight() int {
	b := u.brief()
	h := float32(heroHeadH)
	if b.mode == "day" {
		h += dayH
	}
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
	return int(math.Min(float64(h), float64(u.H-14)))
}

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
	u.item(heroKey, i, x, w, 0, heroHeadH, func(y float32) { u.heroHead(b, x, w, y) })
	i++
	y := float32(heroHeadH)
	if b.mode == "day" {
		u.item("day", i, x, w, y, dayH, func(y float32) { u.dayStory(x, w, y) })
		y += dayH
		i++
	}
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
	if u.edgeRowShown() {
		u.divider(x, w, y)
		y += divH
		u.item("edge", i, x, w, y, edgeH, func(y float32) { u.edgeRow(x, w, y) })
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
	u.spot = emberSpot{x: x + railX, y: y + 25, s: 18, mood: b.mood, show: true}
	right := x + w - pad
	tm := now.Format(u.TimeFmt)
	tmw := measure(fCaption(), tm)
	u.dc.text(fCaption(), tm, right-tmw, y+23, p.faint)
	tx := x + textX
	k := float64(0)
	if b.s != nil {
		key := "focus:" + b.s.ID
		k = u.hl(key)
		u.hits = append(u.hits, hit{image.Rect(int(x+pad-6), int(y+6), int(x+w-pad+6), int(y+heroHeadH-4)), key})
	}
	head := ellipsize(fTitle(), b.head, right-tmw-12-tx)
	hw := u.dc.text(fTitle(), head, tx, y+23, p.fg)
	if k > 0.01 { // the way there shows itself under the pointer
		u.arrow(tx+hw+6, y+19, alpha(p.muted, k))
	}
	u.facts(b.sub, tx, y+39, right-tx, p.muted)
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
		for j, word := range strings.Split(part, " ") {
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
			x += u.dc.text(f, word, x, base, c)
		}
	}
	return x - x0
}

// heroBody: under the headline, by mode. Returns where it ends.
func (u *UI) heroBody(l *sessLayout, x, w, y float32, i *int) float32 {
	p, s := u.pal, l.s
	if !l.open {
		return y
	}
	top := y
	gx := x + railX
	tx := x + textX
	right := x + w - pad
	switch l.mode {
	case "wait":
		u.item("wait:"+s.ID, *i, x, w, y, waitBtnH, func(y float32) { u.goThere(s, tx, y+4) })
		y += waitBtnH
		*i++
	case "recap":
		r := s.Recap
		col := p.green
		switch {
		case r.Failed:
			col = p.red
		case r.troubled():
			col = p.yellow
		}
		h := l.height()
		u.item("recap:"+s.ID, *i, x, w, y, h, func(y float32) {
			lines := l.recap
			if len(lines) == 0 {
				lines = []string{"Nothing to report"}
			}
			// the rail turns into a bar beside Claude's words: green, amber if it admits a failure, red if it stopped
			u.dc.roundRect(gx-1, y+1, 2, float32(len(lines))*recapLineH-2, 1, alpha(col, 0.75))
			tc := p.secondary()
			if r.Failed {
				tc = mix(p.red, p.fg, 0.35)
			}
			for j, ln := range lines {
				u.dc.text(fSecond(), ln, tx, y+12+float32(j)*recapLineH, tc)
			}
		})
		*i++
		return top + h
	}
	railTop := top - heroHeadH + 38 // just under Ember
	if l.mode == "wait" {
		railTop = y + 4
	}
	railEnd := float32(0)
	for j, st := range l.steps {
		key := fmt.Sprintf("st:%s:%d", s.ID, st.At.UnixNano())
		cy := y + stepH/2
		u.item(key, *i, x, w, y, stepH, func(yy float32) { u.step(s, st, j == 0, gx, tx, right, yy+stepH/2) })
		railEnd = cy
		y += stepH
		*i++
	}
	for _, ag := range l.agents {
		open := l.openAg[ag.ID]
		u.shownOpen["a:"+s.ID+":"+ag.ID] = open
		n := 0
		if open {
			n = min(len(ag.Hist), l.agSteps)
		}
		h := float32(agentH) + float32(n)*agentStepH
		if open && ag.Summary != "" {
			h += agentStepH
		}
		u.item("ag:"+s.ID+":"+ag.ID, *i, x, w, y, h, func(yy float32) {
			u.agentRow(s, ag, open, gx, tx, right, yy+agentH/2)
			yy += agentH
			for k := 0; k < n; k++ {
				st := ag.Hist[len(ag.Hist)-1-k]
				sy := yy + agentStepH/2
				col := p.faint
				if st.Failed {
					col = p.red
				}
				u.dc.icon(st.Kind, tx+4, sy, 9, col)
				u.dc.text(fCaption(), ellipsize(fCaption(), st.Text, right-tx-14), tx+14, sy+3.5, p.muted)
				yy += agentStepH
			}
			if open && ag.Summary != "" {
				u.dc.text(fCaption(), ellipsize(fCaption(), "→ "+ag.Summary, right-tx-14), tx+14, yy+agentStepH/2+3.5, p.muted)
			}
		})
		railEnd = y + agentH/2 - 7 // the branch leaves the rail here
		y += h
		*i++
	}
	if railEnd > railTop+2 { // the thread the steps hang on, down from Ember
		u.dc.roundRect(gx-0.5, railTop, 1, railEnd-railTop, 0.5, alpha(p.fg, 0.14))
	}
	if len(l.steps) > 0 || len(l.agents) > 0 {
		y += 4
	}
	return y
}

// step: one step of the timeline. Its kind sits on the rail, its words
// beside it, what it changed and when at the right. The step in progress
// glows green, pulses, shimmers and counts its seconds.
func (u *UI) step(s *Session, st Step, newest bool, gx, tx, right, cy float32) {
	p, now := u.pal, u.now()
	current := newest && !st.Done && s.State == "work"
	col, txtCol, f := p.muted, p.secondary(), fSecond()
	cur := u.tw(fmt.Sprintf("cur:%s:%d", s.ID, st.At.UnixNano()), map[bool]float64{true: 1}[current], map[bool]float64{true: 1}[current])
	switch {
	case st.Failed:
		col, txtCol = p.red, mix(p.red, p.fg, 0.35)
	case cur > 0.001: // the live step's colours ease away when it's over
		col, txtCol = mix(col, p.green, cur), mix(txtCol, p.fg, cur)
		if current {
			f = fBody()
		}
	}
	u.dc.circle(gx, cy, 7.5, color2(p))
	u.dc.icon(st.Kind, gx, cy, 11, col)
	if current {
		u.lives = append(u.lives, live{r: image.Rect(int(gx-12), int(cy-12), int(gx+13), int(cy+13)), draw: func(c *canvas, t float64) {
			q := math.Mod(t/beat, 1)
			r := 7.5 + 4.5*float32(easeOut(q))
			c.frame(gx-r, cy-r, 2*r, 2*r, r, 1, alpha(p.green, 0.45*(1-q)*math.Min(1, q*6)))
		}})
	}
	rx := right
	when := strings.TrimSuffix(ago(now.Sub(st.At)), " ago")
	switch {
	case current && stalled(s, now):
		when = "quiet " + mins(now.Sub(s.Seen))
	case current:
		d := now.Sub(st.At)
		when = mins(d)
		if d < time.Minute {
			when = fmt.Sprintf("%ds", int(d.Seconds()))
		}
	case st.Failed:
		when = "failed"
	case when == "just now":
		when = "now"
	}
	wc := p.faint
	if st.Failed {
		wc = p.red
	}
	if current && !strings.HasSuffix(when, "s") { // minutes roll; seconds just tick
		wc := p.muted
		if stalled(s, now) {
			wc = p.yellow
		}
		rx -= u.rollText(u.dc, "step:"+s.ID, when, fCaption(), rx, cy+4, wc) + 10
	} else if current {
		ww := measure(fCaption(), when)
		u.dc.text(fCaption(), when, rx-ww, cy+4, p.muted)
		rx -= ww + 10
	} else {
		ww := measure(fCaption(), when)
		u.dc.text(fCaption(), when, rx-ww, cy+4, wc)
		rx -= ww + 10
	}
	if st.Minus > 0 {
		m := fmt.Sprintf("−%d", st.Minus)
		rx -= measure(fCaption(), m)
		u.dc.text(fCaption(), m, rx, cy+4, alpha(p.red, 0.85))
		rx -= 5
	}
	if st.Plus > 0 {
		pl := fmt.Sprintf("+%d", st.Plus)
		rx -= measure(fCaption(), pl)
		u.dc.text(fCaption(), pl, rx, cy+4, alpha(p.green, 0.85))
		rx -= 5
	}
	text := st.Text
	if current && s.Label != "" { // the live step speaks in the present
		text = s.Label
	}
	t := ellipsize(f, text, rx-6-tx)
	u.dc.text(f, t, tx, cy+4.5, txtCol)
	if current {
		u.shimmerLive(f, t, tx, cy+4.5, mix(p.fg, colorWhite, 0.55))
	}
}

// agentRow: a subagent on a branch off the rail: a spark that turns while
// it runs (a check once it's done), its task, what it's doing, how long.
func (u *UI) agentRow(s *Session, ag *Agent, open bool, gx, tx, right, cy float32) {
	p, now := u.pal, u.now()
	key := "a:" + s.ID + ":" + ag.ID
	hk := u.hl(key)
	if hk > 0.01 {
		u.panel(tx-8, cy-agentH/2+1, right-tx+14, agentH-2, radRow, alpha(p.fg, 0.05*hk))
	}
	// the branch: down the rail and round into the agent
	u.dc.arc(gx+7, cy-7, 7, 1, math.Pi/2, math.Pi, alpha(p.fg, 0.16))
	u.dc.roundRect(gx+7, cy-0.5, tx-gx-7-3, 1, 0.5, alpha(p.fg, 0.16))
	ix := tx + 4
	done := !ag.End.IsZero()
	if done {
		u.dc.icon("check", ix, cy, 10, alpha(p.green, 0.8))
	} else {
		u.lives = append(u.lives, live{r: image.Rect(int(ix-7), int(cy-7), int(ix+8), int(cy+8)), draw: func(c *canvas, t float64) {
			sparkle(c, ix, cy, 5.5, t, p.yellow)
		}})
	}
	rx := right
	if len(ag.Hist) > 0 || ag.Summary != "" {
		u.chevron(rx-3, cy, u.tw("chev:"+key, map[bool]float64{true: 1}[open], 0), mix(p.faint, p.fg, 0.4*hk))
		rx -= 13
	}
	when := mins(now.Sub(ag.Start))
	if !done && now.Sub(ag.Start) < time.Minute {
		when = fmt.Sprintf("%ds", int(now.Sub(ag.Start).Seconds()))
	}
	if done {
		when = "done"
	}
	ww := measure(fCaption(), when)
	u.dc.text(fCaption(), when, rx-ww, cy+4, p.faint)
	rx -= ww + 10
	nf := fBody()
	nx := tx + 14
	name := ellipsize(nf, ag.name(), (rx-nx)*0.6)
	if ag.Label == "" || done {
		name = ellipsize(nf, ag.name(), rx-nx)
	}
	ncol := p.fg
	if done {
		ncol = p.muted
	}
	nw := u.dc.text(nf, name, nx, cy+4, ncol)
	if ag.Label != "" && !done {
		u.dc.text(fCaption(), ellipsize(fCaption(), ag.Label, rx-nx-nw-8), nx+nw+8, cy+4, p.muted)
	}
	u.hits = append(u.hits, hit{image.Rect(int(tx-8), int(cy-agentH/2), int(right+6), int(cy+agentH/2)), key})
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
	case s.State == "wait":
		when = ago(now.Sub(s.Seen))
	case s.Recap != nil:
		when = ago(now.Sub(s.Recap.At))
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
	case s.Recap != nil && s.Recap.Failed:
		verb, verbCol, what, col = "Stopped", p.red, s.Recap.Error, p.muted
	case s.Recap != nil && now.Sub(s.Recap.At) < recapFresh:
		verb, verbCol, what, col = "Done", p.green, firstClause(s.Recap.Summary), p.muted
		if what == "" {
			what = s.task()
		}
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

// firstClause: a summary's opening, up to its first comma, colon or stop.
func firstClause(s string) string {
	if i := strings.IndexAny(s, ",:;."); i > 12 {
		return s[:i]
	}
	return s
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
	case s.Recap != nil && s.Recap.Failed:
		u.dc.line(pt{gx - 3, gy - 3}, pt{gx + 3, gy + 3}, 1.6, p.red)
		u.dc.line(pt{gx + 3, gy - 3}, pt{gx - 3, gy + 3}, 1.6, p.red)
	case s.Recap != nil && now.Sub(s.Recap.At) < recapFresh:
		col := p.green
		if s.Recap.troubled() {
			col = p.yellow
		}
		u.dc.icon("check", gx, gy, 10, col)
	default:
		u.dc.arc(gx, gy, 5, 1.1, 0, 2*math.Pi, alpha(p.fg, 0.2))
	}
}

// sparkle is a running subagent: a four-pointed spark that turns and
// breathes.
func sparkle(c *canvas, cx, cy, r float32, t float64, col color.NRGBA) {
	rot := t * 1.8
	k := float32(0.82 + 0.18*math.Sin(t*4.1))
	var ps []pt
	for i := 0; i < 8; i++ {
		a := rot + float64(i)*math.Pi/4
		rr := r * k
		if i%2 == 1 {
			rr = r * 0.32
		}
		ps = append(ps, pt{cx + rr*float32(math.Cos(a)), cy + rr*float32(math.Sin(a))})
	}
	c.poly(col, ps...)
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

// color2 is the card's own colour, to sit an icon on the rail cleanly.
func color2(p palette) color.NRGBA { return mix(p.bg, p.island, 0.5) }

// dayStory: the day, hour by hour: when Claude was busy, and now.
func (u *UI) dayStory(x, w, y float32) {
	p, now := u.pal, u.now()
	bx, bw := x+textX, w-textX-pad
	top := 1
	for _, c := range u.day.Hours {
		top = max(top, c)
	}
	slot := bw / 24
	bwid := max(slot-4, 3)
	floor := y + 26
	for h := 0; h < 24; h++ {
		c := u.day.Hours[h]
		bh := float32(2)
		if c > 0 {
			bh = 3 + 19*float32(math.Sqrt(float64(c)/float64(top)))
		}
		col := alpha(p.fg, 0.12)
		switch {
		case h == now.Hour():
			col = p.green
		case c > 0:
			col = alpha(p.green, 0.5)
		case h > now.Hour():
			col = alpha(p.fg, 0.05)
		}
		bx0 := bx + float32(h)*slot + (slot-bwid)/2
		u.dc.roundRect(bx0, floor-bh, bwid, bh, min(1.5, bwid/2), col)
	}
	for _, h := range []int{0, 6, 12, 18} {
		lbl := map[int]string{0: "12a", 6: "6a", 12: "12p", 18: "6p"}[h]
		u.dc.text(fCaption(), lbl, bx+float32(h)*slot+(slot-bwid)/2, y+40, p.faint)
	}
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
