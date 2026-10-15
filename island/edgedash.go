package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"
)

// edge's dashboard: the day, the week, the rating, the year, and the four
// numbers that say whether practice is working. Everything arrives in a
// short choreography (the ring sweeps, the bars grow one after another, the
// rating line draws itself, the year ripples in, the numbers count up); after
// that only the living parts move (the streak's flame, the pulse on a problem
// in progress), and those repaint on their own.

type edgeDash struct{}

const (
	dashW     = 560
	dashP     = 16 // inner padding
	dashHead  = 58
	dashCur   = 38 // the problem in progress, when there is one
	dashMid   = 104
	dashHeat  = 92
	dashTiles = 62
	dashGap   = 8
	dashEnter = 1700 * time.Millisecond // the whole entrance
)

func (edgeDash) Size(u *UI) (float64, float64) {
	st := u.edgeStats
	if st == nil {
		return dashW, 96
	}
	h := dashHead + dashGap + dashMid + dashGap + dashHeat + dashGap + dashTiles + dashP
	if st.Current != nil {
		h += dashCur + dashGap
	}
	if len(st.Health) > 0 {
		h += 20
	}
	return dashW, float64(h)
}

// phase is an entrance's progress: 0 before delay, eased to 1 over dur.
func phase(age, delay, dur time.Duration) float64 {
	return easeOut(float64(age-delay) / float64(dur))
}

func (d edgeDash) Draw(u *UI, x, w float32, age time.Duration) bool {
	st := u.edgeStats
	if st == nil {
		d.waiting(u, x, w)
		return false
	}
	y := float32(0)
	d.header(u, st, x, w, age)
	y += dashHead + dashGap
	if st.Current != nil {
		d.current(u, st, x, w, y, age)
		y += dashCur + dashGap
	}
	inner := w - 2*dashP
	lw := (inner - dashGap) * 0.42
	d.week(u, st, x+dashP, y, lw, age)
	d.rating(u, st, x+dashP+lw+dashGap, y, inner-lw-dashGap, age)
	y += dashMid + dashGap
	d.year(u, st, x+dashP, y, inner, age)
	y += dashHeat + dashGap
	d.tiles(u, st, x+dashP, y, inner, age)
	y += dashTiles + 6
	if len(st.Health) > 0 {
		h := st.Health[0]
		u.dc.icon("warning", x+dashP+6, y+9, 11, u.pal.red)
		u.dc.text(fCaption(), ellipsize(fCaption(), h.Msg, inner-20), x+dashP+18, y+13, mix(u.pal.red, u.pal.fg, 0.3))
	}
	return age < dashEnter
}

func (edgeDash) waiting(u *UI, x, w float32) {
	p := u.pal
	u.dc.text(fTitle(), "edge", x+dashP, 30, p.fg)
	u.dc.text(fBodyReg(), "Waiting for edge's stats. They come from its daemon:", x+dashP, 54, p.muted)
	u.dc.text(fCaption(), "systemctl --user status edge   ·   edge stats", x+dashP, 74, p.faint)
}

// ---- the day ----

func (edgeDash) header(u *UI, st *EdgeStats, x, w float32, age time.Duration) {
	p, t := u.pal, st.Today
	cx, cy, r := x+dashP+20, float32(31), float32(17)
	frac := 1.0
	if t.Need > 0 {
		frac = math.Min(1, float64(t.Got)/float64(t.Need))
	}
	ring := p.yellow
	if t.Done {
		ring = p.green
	}
	u.dc.arc(cx, cy, r, 4, 0, 2*math.Pi, alpha(p.fg, 0.08))
	if sweep := frac * phase(age, 80, 900*time.Millisecond); sweep > 0.002 {
		u.dc.arc(cx, cy, r, 4, -math.Pi/2, float32(-math.Pi/2+2*math.Pi*sweep), ring)
	}
	n := fmt.Sprintf("%d/%d", t.Got, t.Need)
	f := face(fSemiBold, 12)
	u.dc.text(f, n, cx-measure(f, n)/2, cy+4.5, p.fg)
	if st.Current != nil && !st.Current.Started.IsZero() { // working on it: the ring breathes
		u.lives = append(u.lives, live{image.Rect(int(cx-r-9), int(cy-r-9), int(cx+r+10), int(cy+r+10)), func(c *canvas, tt float64) {
			k := math.Mod(tt/2.2, 1)
			rr := r + 3 + 5*float32(k)
			c.frame(cx-rr, cy-rr, 2*rr, 2*rr, rr, 1.2, alpha(ring, 0.45*(1-k)))
		}})
	}

	// the headline, in words
	head := "Today's done"
	switch {
	case st.Current != nil && !st.Current.Started.IsZero():
		head = "Practising · " + mins(time.Duration(st.Current.WorkSec)*time.Second)
	case !t.Done && t.Need > t.Got:
		head = fmt.Sprintf("%d to go today", t.Need-t.Got)
	}
	tx := cx + r + 14
	u.dc.text(fTitle(), head, tx, 27, p.fg)
	sub, subCol := gateWords(st), p.muted
	if t.Gate == "closed" {
		subCol = mix(p.yellow, p.fg, 0.35)
	}
	if t.WorkSec > 0 {
		sub += " · " + mins(time.Duration(t.WorkSec)*time.Second) + " focused"
	}
	u.dc.text(fCaption(), sub, tx, 44, subCol)

	// the streak, at the right: a flame and the count
	rx := x + w - dashP
	big := face(fSemiBold, 22)
	num := fmt.Sprint(int(math.Round(float64(st.Streak.Current) * phase(age, 200, 700*time.Millisecond))))
	nw := measure(big, num)
	u.dc.text(big, num, rx-nw, 33, p.fg)
	cap := fmt.Sprintf("day streak · best %d", st.Streak.Best)
	cw := measure(fCaption(), cap)
	u.dc.text(fCaption(), cap, rx-cw, 48, p.faint)
	fx, fy := rx-nw-12, float32(28)
	hot := st.Streak.Current > 0
	u.lives = append(u.lives, live{image.Rect(int(fx-10), int(fy-17), int(fx+10), int(fy+10)), func(c *canvas, tt float64) {
		flame(c, fx, fy, 9, tt, hot, p)
	}})
}

// gateWords says where Claude stands, the reason edge is there at all.
func gateWords(st *EdgeStats) string {
	t := st.Today
	switch t.Gate {
	case "closed":
		return "Claude is locked until you practise"
	case "off":
		return "The gate is off"
	}
	switch t.GateWhy {
	case "done":
		return "Claude is open: you did today's"
	case "pass":
		return "Claude is open: this week's pass"
	case "outside_study_time":
		return "Claude is open until " + t.StudyFrom
	case "edge_trouble":
		return "Claude is open: edge can't check right now"
	}
	return "Claude is open"
}

// flame draws a small living flame centred at (cx, cy): two tongues that
// sway out of step. A dead streak is a cold grey ember.
func flame(c *canvas, cx, cy, s float32, t float64, hot bool, p palette) {
	outer, inner := mix(p.yellow, p.red, 0.45), mix(p.yellow, colorWhite, 0.35)
	if !hot {
		outer, inner = alpha(p.fg, 0.25), alpha(p.fg, 0.12)
		t = 0
	}
	tongue := func(sc, sway float32, col color.NRGBA) {
		tipX := cx + sway*s
		var ps []pt
		const n = 14
		for i := 0; i <= n; i++ { // the bulb: the lower half circle
			a := float64(i) / n * math.Pi
			ps = append(ps, pt{cx + float32(math.Cos(a))*0.55*s*sc, cy + 0.3*s*sc + float32(math.Sin(a))*0.55*s*sc})
		}
		for i := 1; i < n; i++ { // up the left side to the tip, and down the right
			k := float32(i) / n
			ps = append(ps, pt{cx - 0.55*s*sc*float32(math.Pow(float64(1-k), 0.8)) + (tipX-cx)*k*k, cy + 0.3*s*sc - (1.75*s*sc)*k})
		}
		ps = append(ps, pt{tipX, cy + 0.3*s*sc - 1.75*s*sc})
		for i := n - 1; i >= 1; i-- {
			k := float32(i) / n
			ps = append(ps, pt{cx + 0.55*s*sc*float32(math.Pow(float64(1-k), 0.8)) + (tipX-cx)*k*k, cy + 0.3*s*sc - (1.75*s*sc)*k})
		}
		c.poly(col, ps...)
	}
	sway := float32(0.22*math.Sin(t*5.3) + 0.1*math.Sin(t*8.9))
	tongue(1+float32(0.05*math.Sin(t*7.1)), sway, outer)
	tongue(0.58, -sway*0.8, inner)
}

// ---- the problem in progress ----

func (edgeDash) current(u *UI, st *EdgeStats, x, w float32, y float32, age time.Duration) {
	p, c := u.pal, st.Current
	k := phase(age, 120, 500*time.Millisecond)
	y += 6 * float32(1-k)
	u.panel(x+dashP-4, y, w-2*dashP+8, dashCur, 12, alpha(p.yellow, 0.08*k))
	kind := map[string]string{"new": "NEW", "resolve": "RE-SOLVE", "reentry": "AGAIN"}[c.Kind]
	if kind == "" {
		kind = strings.ToUpper(c.Kind)
	}
	cf := face(fSemiBold, 9.5)
	kw := measure(cf, kind) + 12
	cx := x + dashP + 6
	u.dc.roundRect(cx, y+dashCur/2-8, kw, 16, 8, alpha(p.yellow, 0.18*k))
	u.dc.text(cf, kind, cx+6, y+dashCur/2+3.5, alpha(p.yellow, k))
	right := x + w - dashP - 4
	// right: how long, and what the hints say
	var facts []string
	if c.Started.IsZero() {
		facts = append(facts, "not started")
	} else {
		facts = append(facts, mins(time.Duration(c.WorkSec)*time.Second))
	}
	switch {
	case c.HintsAllowed > c.Hints:
		facts = append(facts, "hint ready")
	case c.NextHintSec > 0:
		facts = append(facts, "hint in "+mins(time.Duration(c.NextHintSec)*time.Second))
	}
	if c.Fails > 0 {
		facts = append(facts, plural(c.Fails, "fail"))
	}
	ft := strings.Join(facts, " · ")
	fw := measure(fCaption(), ft)
	u.dc.text(fCaption(), ft, right-fw, y+dashCur/2+4, alpha(p.muted, k))
	tx := cx + kw + 10
	title := strings.TrimPrefix(c.ID, "cf:") + " · " + c.Title
	if c.Source == "lc" {
		title = c.Title
	}
	tw := u.dc.text(fBody(), ellipsize(fBody(), title, right-fw-60-tx), tx, y+dashCur/2+4.5, alpha(p.fg, k))
	if c.Rating > 0 {
		u.dc.text(fCaption(), fmt.Sprint(c.Rating), tx+tw+8, y+dashCur/2+4, alpha(p.faint, k))
	}
}

// ---- the week ----

func (edgeDash) week(u *UI, st *EdgeStats, x, y, w float32, age time.Duration) {
	p := u.pal
	u.panel(x-4, y, w+8, dashMid, 13, alpha(p.fg, 0.04))
	u.dc.text(fCaption(), "This week", x+8, y+18, p.muted)
	tot := fmt.Sprintf("%d/%d", st.Week.Credits, st.Week.Target)
	f := face(fSemiBold, 11.5)
	u.dc.text(f, tot, x+w-8-measure(f, tot), y+18, p.fg)
	days := st.Week.Days
	if len(days) == 0 {
		return
	}
	top := 1
	for _, d := range days {
		top = max(top, d.Credits, d.Need)
	}
	slot := (w - 16) / float32(len(days))
	base, maxH := y+dashMid-24, float32(dashMid-24-30)
	today := st.Today.Date
	future := false
	for i, d := range days {
		bx := x + 8 + float32(i)*slot + slot/2
		bw := min(slot*0.46, 16)
		isToday := d.Date == today
		k := float32(phase(age, time.Duration(150+70*i)*time.Millisecond, 650*time.Millisecond))
		// the day's need: a faint ghost of the bar it asks for
		needH := maxH * float32(d.Need) / float32(top)
		u.dc.roundRect(bx-bw/2, base-needH, bw, needH, bw/2.6, alpha(p.fg, 0.05))
		if !future {
			h := maxH * float32(d.Credits) / float32(top) * k
			col := p.yellow
			if d.Met {
				col = p.green
			}
			switch {
			case d.Pass && d.Credits == 0:
				u.dc.frame(bx-bw/2, base-needH, bw, needH, bw/2.6, 1.2, alpha(p.blue, 0.55*float64(k)))
			case h > 0.5:
				u.dc.roundRect(bx-bw/2, base-max(h, bw*0.5), bw, max(h, bw*0.5), bw/2.6, col)
			}
		}
		lbl := []string{"M", "T", "W", "T", "F", "S", "S"}[i%7]
		lc, lf := p.faint, fCaption()
		if isToday {
			lc, lf = p.fg, face(fSemiBold, 11)
			u.dc.circle(bx, base+17, 1.6, p.fg)
		}
		u.dc.text(lf, lbl, bx-measure(lf, lbl)/2, base+13, lc)
		if isToday {
			future = true
		}
	}
}

// ---- the rating ----

func (edgeDash) rating(u *UI, st *EdgeStats, x, y, w float32, age time.Duration) {
	p := u.pal
	u.panel(x-4, y, w+8, dashMid, 13, alpha(p.fg, 0.04))
	name, src := "Codeforces", "cf"
	s, ok := st.Sources["cf"]
	if !ok || (s.Rating == 0 && len(s.RatingHistory) == 0) {
		if l, ok2 := st.Sources["lc"]; ok2 && (l.Rating > 0 || len(l.RatingHistory) > 0) {
			s, name, src = l, "LeetCode", "lc"
		}
	}
	_ = src
	u.dc.text(fCaption(), name, x+8, y+18, p.muted)
	h := s.RatingHistory
	rx := x + w - 8
	if s.Rating > 0 {
		f := face(fSemiBold, 11.5)
		r := fmt.Sprint(int(math.Round(float64(s.Rating) * phase(age, 250, 900*time.Millisecond))))
		rw := measure(f, r)
		u.dc.text(f, r, rx-rw, y+18, p.fg)
		if n := len(h); n >= 2 {
			dlt := h[n-1].Rating - h[n-2].Rating
			ds, dc := fmt.Sprintf("+%d", dlt), p.green
			if dlt < 0 {
				ds, dc = fmt.Sprintf("−%d", -dlt), p.red
			}
			u.dc.text(fCaption(), ds, rx-rw-6-measure(fCaption(), ds), y+18, alpha(dc, phase(age, 900, 400*time.Millisecond)))
		}
	}
	gx, gy, gw, gh := x+8, y+30, w-16, float32(dashMid-30-12)
	if len(h) < 2 {
		msg := "No rated contests yet"
		if len(h) == 1 {
			msg = "One contest so far"
		}
		u.dc.text(fCaption(), msg, gx+(gw-measure(fCaption(), msg))/2, gy+gh/2+4, p.faint)
		return
	}
	if len(h) > 24 {
		h = h[len(h)-24:]
	}
	lo, hi := h[0].Rating, h[0].Rating
	for _, r := range h {
		lo, hi = min(lo, r.Rating), max(hi, r.Rating)
	}
	if s.BandLo > 0 {
		lo, hi = min(lo, s.BandLo), max(hi, s.BandHi)
	}
	span := float32(max(hi-lo, 50))
	yOf := func(r int) float32 { return gy + gh - gh*float32(r-lo)/span }
	if s.BandLo > 0 && s.BandHi > s.BandLo { // the band edge picks problems from
		k := phase(age, 200, 500*time.Millisecond)
		y0, y1 := yOf(s.BandHi), yOf(s.BandLo)
		u.dc.roundRect(gx, y0, gw, y1-y0, 4, alpha(p.yellow, 0.07*k))
		bl := fmt.Sprintf("%d–%d", s.BandLo, s.BandHi)
		u.dc.text(face(fRegular, 9.5), bl, gx+3, y0+10, alpha(p.yellow, 0.55*k))
	}
	pts := make([]pt, len(h))
	for i, r := range h {
		pts[i] = pt{gx + gw*float32(i)/float32(len(h)-1), yOf(r.Rating)}
	}
	// the line draws itself from the first contest to the last
	reveal := float32(phase(age, 350, 1100*time.Millisecond)) * float32(len(pts)-1)
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		seg := min(max(reveal-float32(i-1), 0), 1)
		if seg <= 0 {
			break
		}
		b = pt{a.x + (b.x-a.x)*seg, a.y + (b.y-a.y)*seg}
		u.dc.line(a, b, 2, p.blue)
	}
	for i, q := range pts[:len(pts)-1] {
		if float32(i) <= reveal {
			u.dc.circle(q.x, q.y, 1.8, mix(p.blue, p.bg, 0.25))
		}
	}
	if reveal >= float32(len(pts)-1) { // the latest contest glows
		last := pts[len(pts)-1]
		u.dc.circle(last.x, last.y, 3, p.blue)
		u.lives = append(u.lives, live{image.Rect(int(last.x-9), int(last.y-9), int(last.x+10), int(last.y+10)), func(c *canvas, t float64) {
			k := math.Mod(t/1.8, 1)
			rr := 3 + 5*float32(k)
			c.frame(last.x-rr, last.y-rr, 2*rr, 2*rr, rr, 1, alpha(p.blue, 0.5*(1-k)))
		}})
	}
}

// ---- the year ----

func (edgeDash) year(u *UI, st *EdgeStats, x, y, w float32, age time.Duration) {
	p := u.pal
	u.panel(x-4, y, w+8, dashHeat, 13, alpha(p.fg, 0.04))
	cal := st.Calendar
	met := 0
	for _, d := range cal {
		if d.Met {
			met++
		}
	}
	// the caption: the year, or the day under the pointer
	caption, capCol := "The last 12 months", p.muted
	right := plural(met, "day") + " met"
	hovered := -1
	if strings.HasPrefix(u.hoverKey, "heat:") {
		fmt.Sscan(strings.TrimPrefix(u.hoverKey, "heat:"), &hovered)
	}
	if hovered >= 0 && hovered < len(cal) {
		caption, capCol, right = dayWords(cal[hovered]), p.fg, ""
	}
	u.dc.text(fCaption(), caption, x+8, y+17, capCol)
	if right != "" {
		u.dc.text(fCaption(), right, x+w-8-measure(fCaption(), right), y+17, p.faint)
	}
	if len(cal) == 0 {
		return
	}
	// weeks run down, Monday on top; today sits in the last column
	last, err := time.Parse("2006-01-02", cal[len(cal)-1].Date)
	if err != nil {
		return
	}
	lastRow := (int(last.Weekday()) + 6) % 7
	cols := (len(cal) - 1 - lastRow + 6) / 7 // whole weeks before this one
	cols++
	pitch := min(float32(9), (w-16)/float32(cols))
	cell := pitch - 2
	gx := x + 8 + (w-16-pitch*float32(cols))/2
	gy := y + 26
	months := map[int]string{}
	for i, d := range cal {
		back := len(cal) - 1 - i // days before today
		pos := lastRow - back    // rows from this week's Monday, negative into earlier weeks
		col := cols - 1 + floorDiv(pos, 7)
		row := pos - 7*floorDiv(pos, 7)
		cx, cy := gx+float32(col)*pitch, gy+float32(row)*pitch
		k := float32(phase(age, time.Duration(380+col*13+row*9)*time.Millisecond, 420*time.Millisecond))
		if k <= 0 {
			continue
		}
		col2 := heatColor(p, d)
		sz := cell * (0.4 + 0.6*k)
		off := (cell - sz) / 2
		u.dc.roundRect(cx+off, cy+off, sz, sz, 2, alpha(col2, float64(k)))
		if i == len(cal)-1 { // today
			u.dc.frame(cx-1.5, cy-1.5, cell+3, cell+3, 3, 1.1, alpha(p.fg, 0.6*float64(k)))
		}
		if i == hovered {
			u.dc.frame(cx-1.5, cy-1.5, cell+3, cell+3, 3, 1.3, p.fg)
		}
		u.hits = append(u.hits, hit{image.Rect(int(cx-1), int(cy-1), int(cx+pitch), int(cy+pitch)), fmt.Sprint("heat:", i)})
		if t, err := time.Parse("2006-01-02", d.Date); err == nil && t.Day() == 1 {
			months[col] = t.Format("Jan")
		}
	}
	mf := face(fRegular, 9)
	for col, m := range months {
		u.dc.text(mf, m, gx+float32(col)*pitch, gy+7*pitch+9, alpha(p.faint, phase(age, 700, 500*time.Millisecond)))
	}
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// heatColor: green as deep as the day's work against its need, blue for a
// pass, a whisper of green for Codeforces solves before edge, nothing else.
func heatColor(p palette, d EdgeDay) color.NRGBA {
	switch {
	case d.Met:
		return alpha(p.green, 0.95)
	case d.Credits > 0:
		k := float64(d.Credits) / float64(max(d.Need, 1))
		return alpha(p.green, 0.3+0.45*math.Min(k, 1))
	case d.Pass:
		return alpha(p.blue, 0.5)
	case d.CFSolved > 0:
		return alpha(p.green, 0.2)
	}
	return alpha(p.fg, 0.09)
}

// dayWords: "Thu 15 Oct · 1/1 · 45m focused · 2 CF solves".
func dayWords(d EdgeDay) string {
	t, err := time.Parse("2006-01-02", d.Date)
	if err != nil {
		return d.Date
	}
	parts := []string{t.Format("Mon 2 Jan"), fmt.Sprintf("%d/%d", d.Credits, d.Need)}
	if d.Pass {
		parts = append(parts, "pass")
	}
	if d.WorkSec >= 60 {
		parts = append(parts, mins(time.Duration(d.WorkSec)*time.Second)+" focused")
	}
	if d.CFSolved > 0 {
		parts = append(parts, plural(d.CFSolved, "CF solve"))
	}
	return strings.Join(parts, " · ")
}

// ---- the four numbers ----

func (edgeDash) tiles(u *UI, st *EdgeStats, x, y, w float32, age time.Duration) {
	p := u.pal
	type tile struct {
		label, sub string
		value      float64 // what the big number counts up to
		format     func(v float64) string
		col        color.NRGBA
	}
	pct := func(v float64) string { return fmt.Sprintf("%d%%", int(math.Round(v))) }
	t := st.Totals
	var ts []tile
	solo := tile{label: "Solo", sub: "nothing solved yet", format: pct, col: p.green}
	if t.Solved > 0 {
		solo.value = 100 * float64(t.Solo) / float64(t.Solved)
		solo.sub = fmt.Sprintf("%d of %d solved", t.Solo, t.Solved)
	} else {
		solo.format = func(float64) string { return "–" }
	}
	ts = append(ts, solo)
	r := st.Resolves
	ret := tile{label: "Retained", format: pct, col: p.blue}
	if r.FirstAttempts > 0 {
		ret.value = 100 * float64(r.FirstRetained) / float64(r.FirstAttempts)
	} else {
		ret.format = func(float64) string { return "–" }
	}
	ret.sub = fmt.Sprintf("%d re-solve%s due", r.Due, map[bool]string{true: "", false: "s"}[r.Due == 1])
	ts = append(ts, ret)
	wk := 0
	for _, d := range st.Week.Days {
		wk += d.WorkSec
	}
	ts = append(ts, tile{label: "Focus", sub: "this week", value: float64(wk), col: p.yellow,
		format: func(v float64) string { return mins(time.Duration(v) * time.Second) }})
	weak := tile{label: "Weakest", sub: "no focus topics", col: p.red, format: func(float64) string { return "–" }}
	if tp := weakest(st); tp != "" {
		weak.format = func(float64) string { return tp }
		weak.sub = weakSub(st, tp)
	}
	ts = append(ts, weak)

	tw := (w - float32(len(ts)-1)*dashGap) / float32(len(ts))
	for i, tl := range ts {
		tx := x + float32(i)*(tw+dashGap)
		k := phase(age, time.Duration(900+90*i)*time.Millisecond, 600*time.Millisecond)
		ty := y + 8*float32(1-k)
		u.panel(tx-4, ty, tw+8, dashTiles, 12, alpha(p.fg, 0.04*k))
		u.dc.roundRect(tx+6, ty+12, 3, 12, 1.5, alpha(tl.col, k))
		u.dc.text(fCaption(), tl.label, tx+14, ty+22, alpha(p.muted, k))
		v := tl.format(tl.value * phase(age, time.Duration(950+90*i)*time.Millisecond, 800*time.Millisecond))
		bf := face(fSemiBold, 15)
		u.dc.text(bf, ellipsize(bf, v, tw-16), tx+8, ty+40, alpha(p.fg, k))
		sf := face(fRegular, 9.5)
		u.dc.text(sf, ellipsize(sf, tl.sub, tw-12), tx+8, ty+54, alpha(p.faint, k))
	}
}

// weakest is the focus topic with the lowest solo rate (untried ones first).
func weakest(st *EdgeStats) string {
	type tp struct {
		tag  string
		rate float64
		n    int
	}
	var ts []tp
	for _, t := range st.Topics {
		if t.FocusWeight <= 0 {
			continue
		}
		r := -1.0
		if t.Tasks > 0 {
			r = float64(t.Solo) / float64(t.Tasks)
		}
		ts = append(ts, tp{t.Tag, r, t.Tasks})
	}
	if len(ts) == 0 {
		return ""
	}
	sort.SliceStable(ts, func(i, j int) bool {
		if ts[i].rate != ts[j].rate {
			return ts[i].rate < ts[j].rate
		}
		return ts[i].tag < ts[j].tag
	})
	return ts[0].tag
}

func weakSub(st *EdgeStats, tag string) string {
	for _, t := range st.Topics {
		if t.Tag == tag && t.FocusWeight > 0 {
			if t.Tasks == 0 {
				return "not tried yet"
			}
			return fmt.Sprintf("%d of %d solo", t.Solo, t.Tasks)
		}
	}
	return ""
}
