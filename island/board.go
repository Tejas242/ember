package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"
)

// The board: the card's sessions, one row each, any of them opened into its
// short transcript. Layout first (what's open, how much of it fits), then
// drawing.

const (
	sessH      = 40 // a session's row
	sessGap    = 4
	promptH    = 20 // "what you asked"
	agentH     = 22 // a subagent on the branch
	agentStepH = 18 // one of its steps, when it's open
	recapH     = 15 // a line of the recap
	moreH      = 22 // "N more sessions"
)

type sessLayout struct {
	s       *Session
	open    bool
	steps   []Step // newest first
	agents  []*Agent
	openAg  map[string]bool
	agSteps int      // steps an open agent shows
	recap   []string // the recap's lines (a finished session)
}

func (l *sessLayout) height() float32 {
	h := float32(sessH)
	if !l.open {
		return h
	}
	h += 2
	if l.s.Prompt != "" {
		h += promptH
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
	h += float32(len(l.recap)) * recapH
	return h + 8
}

// footerH is what music and edge take under the sessions.
func (u *UI) footerH() float32 {
	h := float32(0)
	if u.track != nil && u.cfg.Music {
		h += musicRowH + rowGapX
	}
	if u.edgeRowShown() {
		h += edgeRowH + rowGapX
	}
	return h
}

// board lays out the sessions to fit the room the surface has.
func (u *UI) board(b brief) (rows []sessLayout, hidden int) {
	now := u.now()
	budget := float32(u.H-14-8) - briefHeadH - u.footerH()
	if b.mode == "day" {
		budget -= 46 + rowGapX
	}
	list := b.rows
	if n := 1 + u.cfg.CardOthers; len(list) > n { // [card] others
		hidden, list = len(list)-n, list[:n]
	}
	auto := map[string]bool{} // opened by Ember, not by you: these close first
	for i, s := range list {
		l := sessLayout{s: s, agSteps: agentHist, openAg: map[string]bool{}}
		open, set := u.opened["s:"+s.ID]
		if !set {
			open = s.State == "wait" || (i == 0 && (s.status() != "idle" || (s.Recap != nil && now.Sub(s.Recap.At) < recapFresh)))
			auto[s.ID] = open
		}
		l.open = open
		l.agents = append(s.agentList(), reversed(s.Finished)...)
		for _, ag := range l.agents {
			l.openAg[ag.ID] = u.opened["a:"+s.ID+":"+ag.ID]
		}
		l.steps = transcript(s, l.agents, b.keep)
		if s.status() == "idle" && s.Recap != nil && s.Recap.Summary != "" {
			l.recap = wrap(fCaption(), s.Recap.Summary, expW-2*pad-30, 2)
		}
		rows = append(rows, l)
	}
	total := func() float32 {
		h := float32(0)
		for i := range rows {
			h += rows[i].height() + sessGap
		}
		if hidden > 0 {
			h += moreH
		}
		return h
	}
	for total() > budget {
		switch {
		case shrink(rows, func(l *sessLayout) bool { // agents' own steps first
			if l.agSteps > 1 && anyOpen(l.openAg) {
				l.agSteps--
				return true
			}
			return false
		}):
		case shrink(rows, func(l *sessLayout) bool { // then the transcripts, from the bottom up
			if l.open && len(l.steps) > 1 {
				l.steps = l.steps[:len(l.steps)-1]
				return true
			}
			return false
		}):
		case shrink(rows, func(l *sessLayout) bool { // then rows Ember opened, but not the first
			if l.open && auto[l.s.ID] && l != &rows[0] {
				l.open = false
				return true
			}
			return false
		}):
		case len(rows) > 1: // then the last rows fold away
			rows = rows[:len(rows)-1]
			hidden++
		case rows[0].open && len(rows[0].steps) > 0:
			rows[0].steps = rows[0].steps[:len(rows[0].steps)-1]
		default:
			return rows, hidden
		}
	}
	return rows, hidden
}

// shrink applies f to the rows from the bottom up, until it does something.
func shrink(rows []sessLayout, f func(*sessLayout) bool) bool {
	for i := len(rows) - 1; i >= 0; i-- {
		if f(&rows[i]) {
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
	h := float32(briefHeadH)
	if b.mode == "day" {
		h += 46 + rowGapX
	}
	rows, hidden := u.board(b)
	for i := range rows {
		h += rows[i].height() + sessGap
	}
	if hidden > 0 {
		h += moreH
	}
	if len(rows) > 0 {
		h += rowGapX - sessGap
	}
	h += u.footerH()
	return int(math.Min(float64(h+8), float64(u.H-14)))
}

// ---- drawing ----

func (u *UI) drawExpanded(x, w float32) {
	p, now := u.pal, u.now()
	b := u.brief()
	u.lastPage = b.mode
	// the headline: Ember, a sentence, the facts under it
	u.spot = emberSpot{x: x + pad + 9, y: 26, s: 18, mood: b.mood, show: true}
	tm := now.Format(u.TimeFmt)
	tmw := measure(fCaption(), tm)
	u.dc.text(fCaption(), tm, x+w-pad-tmw, 24, p.faint)
	tx := x + pad + 30
	u.dc.text(fTitle(), ellipsize(fTitle(), b.head, x+w-pad-tmw-10-tx), tx, 24, p.fg)
	u.dc.text(fCaption(), ellipsize(fCaption(), b.sub, x+w-pad-tx), tx, 40, p.muted)
	if b.s != nil { // the headline is the way to that terminal
		u.hits = append(u.hits, hit{image.Rect(int(x+pad-4), 4, int(x+w-pad), briefHeadH-4), "focus:" + b.s.ID})
	}
	y := float32(briefHeadH)
	if b.mode == "day" {
		u.dayStory(x, w, y)
		y += 46 + rowGapX
	}
	rows, hidden := u.board(b)
	u.boardOpen = map[string]bool{}
	for i := range rows {
		l := &rows[i]
		u.boardOpen["s:"+l.s.ID] = l.open
		for id, o := range l.openAg {
			u.boardOpen["a:"+l.s.ID+":"+id] = o
		}
		u.drawSession(l, x, w, y)
		y += l.height() + sessGap
	}
	if hidden > 0 {
		t := plural(hidden, "more session")
		u.dc.text(fCaption(), t, x+(w-measure(fCaption(), t))/2, y+14, p.faint)
		y += moreH
	}
	if len(rows) > 0 {
		y += rowGapX - sessGap
	}
	if u.track != nil && u.cfg.Music {
		u.musicRow(x, w, y)
		y += musicRowH + rowGapX
	}
	if u.edgeRowShown() {
		u.edgeRow(x, w, y)
	}
}

// drawSession: the row, and when it's open, its transcript.
func (u *UI) drawSession(l *sessLayout, x, w, y float32) {
	p, now, s := u.pal, u.now(), l.s
	h := l.height()
	key := "s:" + s.ID
	k := u.hl(key)
	bg := 0.035 + 0.03*k
	if l.open {
		bg += 0.02
	}
	u.panel(x+pad-6, y, w-2*pad+12, h, 13, alpha(p.fg, bg))
	u.hits = append(u.hits, hit{image.Rect(int(x+pad-6), int(y), int(x+w-pad+6), int(y+sessH)), key})
	gx, gy := x+pad+8, y+sessH/2
	u.sessionGlyph(s, gx, gy)

	// the right end of the first line: agents, the last ten minutes, time, the chevron
	right := x + w - pad
	base := y + 17
	u.chevron(right-4, base-4, u.tw("chev:"+s.ID, map[bool]float64{true: 1}[l.open], 0), p.faint)
	rx := right - 14
	when := ""
	switch {
	case s.status() == "work" && !s.Since.IsZero():
		when = mins(now.Sub(s.Since))
	case s.State == "wait":
		when = strings.TrimSuffix(ago(now.Sub(s.Seen)), " ago")
	case s.Recap != nil:
		when = strings.TrimSuffix(ago(now.Sub(s.Recap.At)), " ago")
	}
	if when != "" {
		ww := measure(fCaption(), when)
		u.dc.text(fCaption(), when, rx-ww, base, p.faint)
		rx -= ww + 8
	}
	if act := s.activity(now); hasAny(act) {
		u.spark(act, rx-36, base-9, 36, 10, s.status() == "work")
		rx -= 36 + 8
	}
	if n := s.running(); n > 0 {
		t := fmt.Sprint(n)
		tw := measure(face(fSemiBold, 11), t)
		u.dc.text(face(fSemiBold, 11), t, rx-tw, base, p.yellow)
		u.dc.icon("agent", rx-tw-7, base-4, 10, p.yellow)
		rx -= tw + 22
	}
	tx := x + pad + 22
	nw := u.dc.text(fBody(), ellipsize(fBody(), u.name(s), rx-tx), tx, base, p.fg)
	_ = nw
	// the second line: what it's doing
	line, col := "Idle", p.faint
	switch {
	case s.State == "wait":
		line, col = "Waiting for your answer", p.blue
	case s.State == "work":
		line, col = s.Label, p.muted
		if line == "" {
			line = "Thinking"
		}
	case s.running() > 0:
		line, col = agentsText(s.running())+" working", p.muted
	case s.Recap != nil && now.Sub(s.Recap.At) < recapFresh:
		line, col = s.task(), p.muted
		if line == "" {
			line = "Done"
		}
	}
	lw := right - 14 - tx
	if l.open { // open, the transcript says what it's doing: this line gives the facts and the way there
		if s.State != "wait" {
			line, col = turnFacts(s, now), p.faint
			if s.status() == "idle" && s.Recap != nil {
				line = recapFacts(s.Recap)
			}
		}
		lw -= u.goThere(s, right, y+28) + 8
		u.dc.text(fCaption(), ellipsize(fCaption(), line, lw), tx, y+32, col)
	} else if s.status() == "work" {
		t := ellipsize(fCaption(), line, lw)
		u.dc.text(fCaption(), t, tx, y+32, col)
		u.shimmerLive(fCaption(), t, tx, y+32, mix(p.fg, colorWhite, 0.3))
	} else {
		u.dc.text(fCaption(), ellipsize(fCaption(), line, lw), tx, y+32, col)
	}
	if !l.open {
		return
	}
	u.drawTranscript(l, x, w, y+sessH+2)
}

// drawTranscript: what you asked, the latest steps on a rail, the agents on
// a branch off it, and a recap if it's done.
func (u *UI) drawTranscript(l *sessLayout, x, w, y float32) {
	p, s := u.pal, l.s
	k := float64(u.tw("body:"+s.ID, 1, 0)) // fades in as it opens
	gx := x + pad + 8
	tx := x + pad + 22
	right := x + w - pad
	railTop, railEnd := y-sessH/2+12, y
	if s.Prompt != "" {
		q := "“" + s.Prompt + "”"
		u.dc.text(fCaption(), ellipsize(fCaption(), q, right-tx), tx, y+13, alpha(mix(p.fg, p.bg, 0.25), k))
		y += promptH
	}
	for i, st := range l.steps {
		cy := y + stepH/2
		u.step(s, st, i == 0, gx, tx, right, cy, k)
		railEnd = cy
		y += stepH
	}
	for _, ag := range l.agents {
		cy := y + agentH/2
		u.agentRow(s, ag, l.openAg[ag.ID], gx, tx, right, cy, k)
		railEnd = cy - 6
		y += agentH
		if l.openAg[ag.ID] {
			n := min(len(ag.Hist), l.agSteps)
			for i := 0; i < n; i++ {
				st := ag.Hist[len(ag.Hist)-1-i]
				sy := y + agentStepH/2
				col := p.faint
				if st.Failed {
					col = p.red
				}
				u.dc.icon(st.Kind, tx+18, sy, 9, alpha(col, k))
				u.dc.text(face(fRegular, 10.5), ellipsize(face(fRegular, 10.5), st.Text, right-tx-30), tx+28, sy+3.5, alpha(mix(p.fg, p.bg, 0.4), k))
				y += agentStepH
			}
			if ag.Summary != "" {
				sy := y + agentStepH/2
				u.dc.text(face(fRegular, 10.5), ellipsize(face(fRegular, 10.5), "→ "+ag.Summary, right-tx-18), tx+18, sy+3.5, alpha(p.muted, k))
				y += agentStepH
			}
		}
	}
	for _, ln := range l.recap {
		u.dc.text(fCaption(), ln, tx, y+11, alpha(mix(p.fg, p.bg, 0.15), k))
		y += recapH
	}
	if railEnd > railTop+2 { // the rail the steps hang on
		u.dc.roundRect(gx-0.5, railTop, 1, railEnd-railTop, 0.5, alpha(p.fg, 0.1*k))
	}
}

// step draws one step of a transcript: its kind on the rail, its words, what
// it changed and when. The step in progress pulses and counts its seconds.
func (u *UI) step(s *Session, st Step, newest bool, gx, tx, right, cy float32, k float64) {
	p, now := u.pal, u.now()
	current := newest && !st.Done && s.State == "work"
	col, txtCol, f := p.muted, mix(p.fg, p.bg, 0.3), fCaption()
	switch {
	case st.Failed:
		col, txtCol = p.red, mix(p.red, p.fg, 0.3)
	case current:
		col, txtCol, f = p.green, p.fg, face(fMedium, 11.5)
	}
	u.dc.circle(gx, cy, 7, color2(p))
	u.dc.icon(st.Kind, gx, cy, 10, alpha(col, k))
	if current {
		u.lives = append(u.lives, live{image.Rect(int(gx-11), int(cy-11), int(gx+12), int(cy+12)), func(c *canvas, t float64) {
			q := math.Mod(t/1.4, 1)
			r := 5 + 4*float32(q)
			c.frame(gx-r, cy-r, 2*r, 2*r, r, 1.1, alpha(p.green, 0.5*(1-q)))
		}})
	}
	rx := right
	when := strings.TrimSuffix(ago(now.Sub(st.At)), " ago")
	switch {
	case current:
		when = mins(now.Sub(st.At))
		if now.Sub(st.At) < time.Minute {
			when = fmt.Sprintf("%ds", int(now.Sub(st.At).Seconds()))
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
	ww := measure(fCaption(), when)
	u.dc.text(fCaption(), when, rx-ww, cy+4, alpha(wc, k))
	rx -= ww + 8
	if st.Minus > 0 {
		m := fmt.Sprintf("−%d", st.Minus)
		rx -= measure(fCaption(), m)
		u.dc.text(fCaption(), m, rx, cy+4, alpha(p.red, k))
		rx -= 4
	}
	if st.Plus > 0 {
		pl := fmt.Sprintf("+%d", st.Plus)
		rx -= measure(fCaption(), pl)
		u.dc.text(fCaption(), pl, rx, cy+4, alpha(p.green, k))
		rx -= 4
	}
	text := st.Text
	if current && s.Label != "" { // the live step speaks in the present
		text = s.Label
	}
	t := ellipsize(f, text, rx-4-tx)
	u.dc.text(f, t, tx, cy+4, alpha(txtCol, k))
	if current {
		u.shimmerLive(f, t, tx, cy+4, mix(p.fg, colorWhite, 0.5))
	}
}

// agentRow: a subagent, on a branch off the rail: a spark that turns while
// it runs (a check once it's done), its task, what it's doing, how long.
func (u *UI) agentRow(s *Session, ag *Agent, open bool, gx, tx, right, cy float32, k float64) {
	p, now := u.pal, u.now()
	key := "a:" + s.ID + ":" + ag.ID
	hk := u.hl(key)
	if hk > 0.01 {
		u.dc.roundRect(tx-4, cy-agentH/2+1, right-tx+8, agentH-2, 8, alpha(p.fg, 0.05*hk*k))
	}
	// the branch: down the rail and round into the agent
	u.dc.arc(gx+7, cy-7, 7, 1, math.Pi/2, math.Pi, alpha(p.fg, 0.16*k))
	u.dc.roundRect(gx+7, cy-0.5, tx-gx-7-8, 1, 0.5, alpha(p.fg, 0.16*k))
	ix := tx + 2
	done := !ag.End.IsZero()
	if done {
		u.dc.icon("check", ix, cy, 10, alpha(p.green, 0.8*k))
	} else {
		u.lives = append(u.lives, live{image.Rect(int(ix-7), int(cy-7), int(ix+8), int(cy+8)), func(c *canvas, t float64) {
			sparkle(c, ix, cy, 5.5, t, p.yellow)
		}})
	}
	// right: how long, and the chevron when it has steps to show
	rx := right
	if len(ag.Hist) > 0 || ag.Summary != "" {
		u.chevron(rx-3, cy, u.tw("chev:"+key, map[bool]float64{true: 1}[open], 0), alpha(p.faint, k))
		rx -= 12
	}
	when := mins(now.Sub(ag.Start))
	if !done && now.Sub(ag.Start) < time.Minute {
		when = fmt.Sprintf("%ds", int(now.Sub(ag.Start).Seconds()))
	}
	if done {
		when = "done"
	}
	ww := measure(fCaption(), when)
	u.dc.text(fCaption(), when, rx-ww, cy+4, alpha(p.faint, k))
	rx -= ww + 8
	nf := face(fMedium, 11.5)
	name := ellipsize(nf, ag.name(), (rx-tx-14)*0.6)
	if ag.Label == "" || done {
		name = ellipsize(nf, ag.name(), rx-tx-14)
	}
	ncol := p.fg
	if done {
		ncol = p.muted
	}
	nw := u.dc.text(nf, name, tx+12, cy+4, alpha(ncol, k))
	if ag.Label != "" && !done {
		u.dc.text(fCaption(), ellipsize(fCaption(), ag.Label, rx-tx-12-nw-8), tx+12+nw+8, cy+4, alpha(p.muted, k))
	}
	u.hits = append(u.hits, hit{image.Rect(int(tx-4), int(cy-agentH/2), int(right+6), int(cy+agentH/2)), key})
}

// ---- the glyphs ----

// sessionGlyph says a session's state at a glance. Working: an orbit with a
// bead going round it, and a smaller satellite per subagent on an outer
// track. Waiting: a blue pulse. Done: a check. Idle: an empty ring.
func (u *UI) sessionGlyph(s *Session, gx, gy float32) {
	p, now := u.pal, u.now()
	switch {
	case s.State == "wait":
		u.dc.circle(gx, gy, 4.2, p.blue)
		u.lives = append(u.lives, live{image.Rect(int(gx-12), int(gy-12), int(gx+13), int(gy+13)), func(c *canvas, t float64) {
			q := math.Mod(t/1.3, 1)
			r := 5 + 6*float32(q)
			c.frame(gx-r, gy-r, 2*r, 2*r, r, 1.3, alpha(p.blue, 0.6*(1-q)))
		}})
	case s.status() == "work":
		u.dc.arc(gx, gy, 6.5, 1.2, 0, 2*math.Pi, alpha(p.green, 0.28))
		n := min(s.running(), 4)
		u.lives = append(u.lives, live{image.Rect(int(gx-12), int(gy-12), int(gx+13), int(gy+13)), func(c *canvas, t float64) {
			a := t * 2.6
			c.circle(gx+6.5*float32(math.Cos(a)), gy+6.5*float32(math.Sin(a)), 2.1, p.green)
			for i := 0; i < n; i++ { // the subagents, round the other way
				b := -t*1.7 + float64(i)*2*math.Pi/float64(n)
				c.circle(gx+10.5*float32(math.Cos(b)), gy+10.5*float32(math.Sin(b)), 1.4, p.yellow)
			}
		}})
	case s.Recap != nil && now.Sub(s.Recap.At) < recapFresh:
		col := p.green
		if s.Recap.Failures > 0 && strings.Contains(strings.ToLower(s.Recap.Summary), "fail") {
			col = p.yellow
		}
		u.dc.arc(gx, gy, 6.5, 1.2, 0, 2*math.Pi, alpha(col, 0.35))
		u.dc.icon("check", gx, gy, 9, col)
	default:
		u.dc.arc(gx, gy, 6, 1.2, 0, 2*math.Pi, alpha(p.fg, 0.18))
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

// spark draws the last ten minutes as a row of thin bars.
func (u *UI) spark(v []float64, x, y, w, h float32, live bool) {
	p := u.pal
	top := 1.0
	for _, x := range v {
		top = math.Max(top, x)
	}
	bw := w / float32(len(v))
	col := alpha(p.green, 0.6)
	if !live {
		col = alpha(p.fg, 0.25)
	}
	for i, c := range v {
		bh := float32(1)
		if c > 0 {
			bh = 2 + (h-2)*float32(math.Sqrt(c/top))
		}
		cc := col
		if c == 0 {
			cc = alpha(p.fg, 0.08)
		}
		u.dc.roundRect(x+float32(i)*bw, y+h-bh, max(bw-0.6, 0.6), bh, 0.4, cc)
	}
}

func hasAny(v []float64) bool {
	for _, x := range v {
		if x > 0 {
			return true
		}
	}
	return false
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
func color2(p palette) color.NRGBA { return mix(p.bg, p.fg, 0.07) }

// dayStory: the day, hour by hour: when Claude was busy, and now.
func (u *UI) dayStory(x, w, y float32) {
	p, now := u.pal, u.now()
	u.panel(x+pad-6, y, w-2*pad+12, 46, 13, alpha(p.fg, 0.04))
	bx, bw := x+pad+4, w-2*pad-8
	top := 1
	for _, c := range u.day.Hours {
		top = max(top, c)
	}
	slot := bw / 24
	for h := 0; h < 24; h++ {
		c := u.day.Hours[h]
		bh := float32(2)
		if c > 0 {
			bh = 3 + 17*float32(math.Sqrt(float64(c)/float64(top)))
		}
		col := alpha(p.fg, 0.12)
		switch {
		case h == now.Hour():
			col = p.green
		case c > 0:
			col = alpha(p.green, 0.55)
		case h > now.Hour():
			col = alpha(p.fg, 0.05)
		}
		u.dc.roundRect(bx+float32(h)*slot+1, y+26-bh, slot-2, bh, 1, col)
	}
	for _, h := range []int{0, 6, 12, 18} {
		lbl := map[int]string{0: "12a", 6: "6a", 12: "12p", 18: "6p"}[h]
		u.dc.text(face(fRegular, 9.5), lbl, bx+float32(h)*slot, y+40, p.faint)
	}
}

// goThere draws the way to a session's terminal, right-aligned at right and
// centred on cy, and reports its width. Blue and filled when the session is
// waiting on you; a quiet link otherwise.
func (u *UI) goThere(s *Session, right, cy float32) float32 {
	p := u.pal
	key := "focus:" + s.ID
	k := u.hl(key)
	f := face(fMedium, 11)
	t := "Go there"
	tw := measure(f, t)
	w := tw + 26
	x := right - w + 4
	bg, fg := alpha(p.blue, 0.1+0.12*k), mix(p.blue, p.fg, 0.25)
	if s.State == "wait" {
		bg, fg = mix(p.blue, colorWhite, 0.1*k), p.bg
	}
	u.dc.roundRect(x, cy-9, w, 18, 9, bg)
	u.dc.text(f, t, x+8, cy+4, fg)
	u.arrow(x+8+tw+5, cy, fg)
	u.hits = append(u.hits, hit{image.Rect(int(x), int(cy-10), int(x+w), int(cy+10)), key})
	return w
}
