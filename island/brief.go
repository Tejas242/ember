package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"
)

// The open card is a brief, not a window. It answers, in order, the three
// things you look up for: does anything need me; what is it doing and is it
// going well; what happened while I wasn't looking. It reads differently
// depending on which of those is true right now:
//
//	wait   a session is asking in its terminal: where, since when, go there
//	work   a session is working: a headline in words, then its live timeline
//	recap  a session just finished: what it did, how long, what changed
//	day    nothing's going on: your day so far, hour by hour
//
// Below the story, one line for each other session, then music and edge if
// they're live. No settings: volume and brightness speak up in the pill when
// they change, and that's enough.

const (
	briefHeadH = 52 // Ember, the headline, its facts
	stepH      = 22
	otherH     = 26
	keepShown  = 4 // timeline steps shown
	recapFresh = 45 * time.Minute
)

type brief struct {
	mode      string // wait | work | recap | day
	s         *Session
	head, sub string
	mood      mood
	others    []*Session
	agents    []*Agent // the working session's subagents
}

// brief decides what the card is about.
func (u *UI) brief() brief {
	now := u.now()
	ss := u.sessionList()
	if !u.cfg.Claude {
		ss = nil
	}
	var b brief
	var working []*Session // in the list's order: the same session as the pill, held still
	for _, s := range ss {
		if s.status() == "work" {
			working = append(working, s)
		}
	}
	switch {
	case len(ss) > 0 && ss[0].State == "wait":
		s := ss[0]
		sub := "It's asking in the terminal · " + ago(now.Sub(s.Seen))
		if t := s.task(); t != "" {
			sub = t + " · " + ago(now.Sub(s.Seen))
		}
		b = brief{mode: "wait", s: s, mood: moodWait, head: u.name(s) + " needs you", sub: sub}
	case len(working) > 0:
		s := working[0]
		label := s.Label
		if label == "" {
			label = "Thinking"
		}
		if s.State == "idle" { // its turn ended; its agents work on
			label = agentsText(s.running()) + " working"
		}
		// the headline is the task, in words; the timeline below says how it's going
		head, sub := s.task(), s.Project+" · "+turnFacts(s, now)
		if head == "" {
			head, sub = u.name(s)+" · "+label, turnFacts(s, now)
			if g, ok := gerund(label); ok {
				head = u.name(s) + " is " + g
			}
		}
		b = brief{mode: "work", s: s, mood: moodWork, head: head, sub: strings.TrimSuffix(sub, " · "), agents: s.agentList()}
	default:
		var last *Session
		for _, s := range ss {
			if s.Recap != nil && now.Sub(s.Recap.At) < recapFresh && (last == nil || s.Recap.At.After(last.Recap.At)) {
				last = s
			}
		}
		if last != nil {
			r := last.Recap
			m := moodHappy
			if r.Failures > 0 && strings.Contains(strings.ToLower(r.Summary), "fail") {
				m = moodNudge
			}
			sub := last.Project + " · done " + ago(now.Sub(r.At))
			if r.Took > 0 {
				sub += " · took " + mins(r.Took)
			}
			head := last.task()
			if head == "" {
				head, sub = u.name(last)+" is done", strings.TrimPrefix(sub, last.Project+" · d")
				sub = "D" + sub
			}
			b = brief{mode: "recap", s: last, mood: m, head: head, sub: sub}
		} else {
			b = brief{mode: "day", mood: moodIdle, head: greeting(now), sub: u.dayFacts()}
		}
	}
	for _, s := range ss { // everyone else, one line each: the busy, then the recently done
		if s == b.s || len(b.others) == 3 {
			continue
		}
		if s.status() != "idle" || (s.Recap != nil && now.Sub(s.Recap.At) < recapFresh) {
			b.others = append(b.others, s)
		}
	}
	return b
}

// shown is how many timeline steps fit: four alone, one fewer when other
// sessions need their lines, and fewer again for the agents' rows.
func (b brief) shown() int {
	n := keepShown
	if len(b.others) > 0 {
		n--
	}
	n -= (min(len(b.agents), agentRows) + 1) / 2
	return max(n, 1)
}

// timelineH is the height of the work story's timeline.
func (b brief) timelineH() float32 {
	n := min(len(b.steps()), b.shown())
	if n == 0 {
		return 0
	}
	return float32(10 + n*stepH)
}

// steps is the timeline: the turn's steps, less the launches of agents
// that have their own row below.
func (b brief) steps() []Step {
	all := b.s.Turn.Steps
	if len(b.agents) == 0 {
		return all
	}
	running := map[string]bool{}
	for _, ag := range b.agents {
		if ag.Desc != "" {
			running[ag.Desc] = true
		}
	}
	out := make([]Step, 0, len(all))
	for _, st := range all {
		if st.Kind == "agent" && running[strings.TrimPrefix(st.Text, "Asked an agent: ")] {
			continue
		}
		out = append(out, st)
	}
	return out
}

func agentsH(n int) float32 { return float32(8 + min(n, agentRows)*stepH) }

// turnFacts: "4m in · 9 steps · 3 files +58 −12 · 1 failed".
func turnFacts(s *Session, now time.Time) string {
	t := s.Turn
	var parts []string
	if !s.Since.IsZero() {
		parts = append(parts, mins(now.Sub(s.Since))+" in")
	}
	if t.Count > 0 {
		parts = append(parts, plural(t.Count, "step"))
	}
	if n := len(t.Files); n > 0 {
		f := plural(n, "file")
		if t.Plus > 0 || t.Minus > 0 {
			f += " " + diffText(t.Plus, t.Minus)
		}
		parts = append(parts, f)
	}
	if t.Failures > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", t.Failures))
	}
	return strings.Join(parts, " · ")
}

func recapFacts(r *Recap) string {
	var parts []string
	if r.Steps > 0 {
		parts = append(parts, plural(r.Steps, "step"))
	}
	if r.Files > 0 {
		f := plural(r.Files, "file") + " changed"
		if r.Plus > 0 || r.Minus > 0 {
			f += " " + diffText(r.Plus, r.Minus)
		}
		parts = append(parts, f)
	} else if r.Steps > 0 {
		parts = append(parts, "no files changed")
	}
	if r.Failures > 0 {
		parts = append(parts, plural(r.Failures, "failed step"))
	}
	return strings.Join(parts, " · ")
}

func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return fmt.Sprintf("%d %ss", n, w)
}

// dayFacts: "6 tasks done · 2h 14m with Claude · mostly ember".
func (u *UI) dayFacts() string {
	d := u.day
	if d == nil || d.Tasks == 0 {
		return "Nothing finished with Claude yet today"
	}
	parts := []string{plural(d.Tasks, "task") + " done"}
	if d.Busy >= time.Minute {
		parts = append(parts, mins(d.Busy)+" with Claude")
	}
	top, n := "", 0
	for p, c := range d.Projects {
		if c > n || (c == n && p < top) {
			top, n = p, c
		}
	}
	if len(d.Projects) > 1 {
		parts = append(parts, "mostly "+top)
	} else if top != "" {
		parts = append(parts, "in "+top)
	}
	return strings.Join(parts, " · ")
}

// ---- layout ----

func (u *UI) storyHeight(b brief) float32 {
	switch b.mode {
	case "wait":
		return 34
	case "work":
		h := b.timelineH()
		if len(b.agents) > 0 {
			if h > 0 {
				h += rowGapX
			}
			h += agentsH(len(b.agents))
		}
		return h
	case "recap":
		r := b.s.Recap
		h := float32(14 + 15) // padding, facts line
		if r.Summary != "" {
			h += 16 * float32(len(wrap(fBodyReg(), r.Summary, cardW-2*pad-24, 3)))
		}
		return h + 6
	case "day":
		return 46
	}
	return 0
}

func (u *UI) expandedHeight() int {
	b := u.brief()
	h := float32(briefHeadH)
	if sh := u.storyHeight(b); sh > 0 {
		h += sh + rowGapX
	}
	h += float32(len(b.others)) * (otherH + 2)
	if len(b.others) > 0 {
		h += rowGapX - 2
	}
	if u.track != nil && u.cfg.Music {
		h += musicRowH + rowGapX
	}
	if e := u.edge; e != nil && u.cfg.Edge && (e.Task != "" || e.Closed) {
		h += edgeRowH + rowGapX
	}
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
	if sh := u.storyHeight(b); sh > 0 {
		switch b.mode {
		case "wait":
			u.waitStory(b.s, x, w, y)
		case "work":
			th := b.timelineH()
			if th > 0 {
				u.timeline(b.s, b.steps(), b.shown(), x, w, y, th)
				th += rowGapX
			}
			if len(b.agents) > 0 {
				u.agentsStory(b.s, b.agents, x, w, y+th)
			}
		case "recap":
			u.recapStory(b.s, x, w, y, sh)
		case "day":
			u.dayStory(x, w, y)
		}
		y += sh + rowGapX
	}
	for _, s := range b.others {
		u.otherRow(s, x, w, y)
		y += otherH + 2
	}
	if len(b.others) > 0 {
		y += rowGapX - 2
	}
	if u.track != nil && u.cfg.Music {
		u.musicRow(x, w, y)
		y += musicRowH + rowGapX
	}
	if e := u.edge; e != nil && u.cfg.Edge && (e.Task != "" || e.Closed) {
		u.edgeRow(x, w, y)
	}
}

// waitStory: one line and the way there.
func (u *UI) waitStory(s *Session, x, w, y float32) {
	p := u.pal
	u.panel(x+pad-6, y, w-2*pad+12, 34, 12, alpha(p.blue, 0.12))
	u.dc.icon("run", x+pad+8, y+17, 13, p.blue)
	u.dc.text(fBody(), "Answer it in the terminal", x+pad+24, y+21.5, p.fg)
	u.buttonRow([]Button{{Key: s.ID, Label: "Go there", Primary: true}}, "focus:", x, w-4, y+4, "")
}

// timeline: the turn so far, newest first, on a thin rail. The step in
// progress pulses and counts its seconds; a failed step says so in red.
func (u *UI) timeline(s *Session, steps []Step, shown int, x, w, y, h float32) {
	p, now := u.pal, u.now()
	u.panel(x+pad-6, y, w-2*pad+12, h, 13, alpha(p.fg, 0.04))
	n := min(len(steps), shown)
	rail := x + pad + 7
	if n > 1 {
		u.dc.roundRect(rail-0.5, y+5+stepH/2, 1, float32(n-1)*stepH, 0.5, alpha(p.fg, 0.12))
	}
	right := x + w - pad
	for i := 0; i < n; i++ {
		st := steps[len(steps)-1-i]
		cy := y + 5 + float32(i)*stepH + stepH/2
		current := i == 0 && !st.Done && s.State == "work"
		col, txtCol, f := p.muted, mix(p.fg, p.bg, 0.3), fCaption()
		switch {
		case st.Failed:
			col, txtCol = p.red, mix(p.red, p.fg, 0.3)
		case current:
			col, txtCol, f = p.green, p.fg, fBody()
		}
		u.dc.circle(rail, cy, 8, color2(p))
		u.dc.icon(st.Kind, rail, cy, 12, col)
		if current { // a soft pulse on the live step
			u.lives = append(u.lives, live{image.Rect(int(rail-11), int(cy-11), int(rail+12), int(cy+12)), func(c *canvas, t float64) {
				k := math.Mod(t/1.4, 1)
				r := 6 + 4*float32(k)
				c.frame(rail-r, cy-r, 2*r, 2*r, r, 1.1, alpha(p.green, 0.5*(1-k)))
			}})
		}
		// right: what it changed, and when
		rx := right
		when := ago(now.Sub(st.At))
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
		default:
			when = strings.TrimSuffix(when, " ago")
		}
		wc := p.faint
		if st.Failed {
			wc = p.red
		}
		ww := measure(fCaption(), when)
		u.dc.text(fCaption(), when, rx-ww, cy+4, wc)
		rx -= ww + 8
		if st.Plus > 0 || st.Minus > 0 {
			if st.Minus > 0 {
				m := fmt.Sprintf("−%d", st.Minus)
				rx -= measure(fCaption(), m)
				u.dc.text(fCaption(), m, rx, cy+4, p.red)
				rx -= 4
			}
			if st.Plus > 0 {
				pl := fmt.Sprintf("+%d", st.Plus)
				rx -= measure(fCaption(), pl)
				u.dc.text(fCaption(), pl, rx, cy+4, p.green)
			}
			rx -= 8
		}
		tx := rail + 13
		text := st.Text
		if current && s.Label != "" { // the live step speaks in the present
			text = s.Label
		}
		if current {
			u.dc.text(f, ellipsize(f, text, rx-tx), tx, cy+4.5, mix(txtCol, p.bg, 0.2))
			u.shimmerLive(f, ellipsize(f, text, rx-tx), tx, cy+4.5, mix(p.fg, colorWhite, 0.5))
		} else {
			u.dc.text(f, ellipsize(f, text, rx-tx), tx, cy+4, txtCol)
		}
	}
	u.hits = append(u.hits, hit{image.Rect(int(x+pad-6), int(y), int(x+w-pad+6), int(y+h)), "focus:" + s.ID})
}

// color2 is the card's own colour, to sit an icon on the rail cleanly.
func color2(p palette) color.NRGBA { return mix(p.bg, p.fg, 0.07) }

// recapStory: Claude's own most informative sentence, then the facts.
func (u *UI) recapStory(s *Session, x, w, y, h float32) {
	p := u.pal
	r := s.Recap
	key := "focus:" + s.ID
	k := u.hl(key)
	u.panel(x+pad-6, y, w-2*pad+12, h, 13, alpha(p.fg, 0.04+0.03*k))
	bar := p.green
	if r.Failures > 0 && strings.Contains(strings.ToLower(r.Summary), "fail") {
		bar = p.yellow
	}
	cy := y + 8
	if r.Summary != "" {
		lines := wrap(fBodyReg(), r.Summary, w-2*pad-24, 3)
		u.dc.roundRect(x+pad+2, cy+2, 2, float32(len(lines))*16-3, 1, bar)
		for _, ln := range lines {
			u.dc.text(fBodyReg(), ln, x+pad+14, cy+13, p.fg)
			cy += 16
		}
		cy += 4
	}
	u.dc.text(fCaption(), ellipsize(fCaption(), recapFacts(r), w-2*pad-14), x+pad+14, cy+11, p.muted)
	u.hits = append(u.hits, hit{image.Rect(int(x+pad-6), int(y), int(x+w-pad+6), int(y+h)), key})
}

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

// otherRow: another session, in one line: what it's doing or what it did.
func (u *UI) otherRow(s *Session, x, w, y float32) {
	p, now := u.pal, u.now()
	key := "focus:" + s.ID
	k := u.hl(key)
	if k > 0.01 {
		u.panel(x+pad-6, y, w-2*pad+12, otherH, 10, alpha(p.fg, 0.05*k))
	}
	cy := y + otherH/2
	dot, text, when := p.green, s.Label, ""
	switch {
	case s.State == "wait":
		dot, text = p.blue, "Waiting for you"
	case s.State == "work":
		if text == "" {
			text = "Thinking"
		}
		if n := s.running(); n > 0 {
			text = agentsText(n) + " · " + text
		}
		if !s.Since.IsZero() {
			when = mins(now.Sub(s.Since))
		}
	case s.running() > 0: // the turn ended; its agents work on
		ags := s.agentList()
		text, when = agentsText(len(ags))+" working", mins(now.Sub(ags[0].Start))
	case s.Recap != nil:
		dot, text, when = p.faint, s.Recap.Summary, ago(now.Sub(s.Recap.At))
		if text == "" {
			text = "Done"
		}
	}
	u.dc.circle(x+pad+5, cy, 3, dot)
	tx, right := x+pad+18, x+w-pad
	ww := measure(fCaption(), when)
	u.dc.text(fCaption(), when, right-ww, cy+4, p.faint)
	pw := u.dc.text(fBody(), ellipsize(fBody(), u.name(s), 110), tx, cy+4.5, p.fg)
	u.dc.text(fCaption(), ellipsize(fCaption(), text, right-ww-10-(tx+pw+8)), tx+pw+8, cy+4, p.muted)
	u.hits = append(u.hits, hit{image.Rect(int(x+pad-6), int(y), int(x+w-pad+6), int(y+otherH)), key})
}

// agentsStory: the session's subagents, one row each: what it was asked,
// what it's doing now, how long it's been at it. Past three, the last row
// counts the rest.
func (u *UI) agentsStory(s *Session, ags []*Agent, x, w, y float32) {
	p, now := u.pal, u.now()
	h := agentsH(len(ags))
	u.panel(x+pad-6, y, w-2*pad+12, h, 13, alpha(p.fg, 0.04))
	rows := min(len(ags), agentRows)
	icon, right := x+pad+7, x+w-pad
	for i := 0; i < rows; i++ {
		cy := y + 4 + float32(i)*stepH + stepH/2
		if i == rows-1 && len(ags) > agentRows { // the rest, counted
			rest := len(ags) - i
			u.dc.icon("agent", icon, cy, 11, p.faint)
			u.dc.text(fCaption(), fmt.Sprintf("%d more agents", rest), icon+13, cy+4, p.muted)
			break
		}
		ag := ags[i]
		u.dc.icon("agent", icon, cy, 11, p.green)
		u.lives = append(u.lives, live{image.Rect(int(icon-11), int(cy-11), int(icon+12), int(cy+12)), func(c *canvas, t float64) {
			k := math.Mod(t/1.6+float64(i)*0.33, 1) // staggered, so they breathe in turn
			r := 5 + 4*float32(k)
			c.frame(icon-r, cy-r, 2*r, 2*r, r, 1, alpha(p.green, 0.4*(1-k)))
		}})
		d := now.Sub(ag.Start)
		when := mins(d)
		if d < time.Minute {
			when = fmt.Sprintf("%ds", int(d.Seconds()))
		}
		ww := measure(fCaption(), when)
		u.dc.text(fCaption(), when, right-ww, cy+4, p.faint)
		tx, room := icon+13, right-ww-8-(icon+13)
		name := ellipsize(fBody(), ag.name(), room*0.6)
		if ag.Label == "" {
			name = ellipsize(fBody(), ag.name(), room)
		}
		nw := u.dc.text(fBody(), name, tx, cy+4.5, p.fg)
		if ag.Label != "" {
			u.dc.text(fCaption(), ellipsize(fCaption(), ag.Label, room-nw-8), tx+nw+8, cy+4, p.muted)
		}
	}
	u.hits = append(u.hits, hit{image.Rect(int(x+pad-6), int(y), int(x+w-pad+6), int(y+h)), "focus:" + s.ID})
}
