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

// edge's dashboard: the day, the problem in progress, the week, the rating,
// the year, and the four numbers that say whether practice is working. One
// dark surface, like the card: sections divided by hairlines and named in
// small capitals, no boxes in boxes. Everything arrives in a short
// choreography (the ring sweeps, the bars grow one after another, the rating
// line draws itself, the year ripples in, the numbers count up); after that
// only the living parts move (the streak's flame, the pulses), and those
// repaint on their own.

type edgeDash struct{}

const (
	dashW     = 560
	dashP     = 20 // inner margin
	dashHead  = 56
	dashCur   = 34 // the problem in progress, when there is one
	dashMid   = 96
	dashHeat  = 92
	dashTiles = 46
	dashDiv   = 9                       // the space and hairline between sections
	dashEnter = 1700 * time.Millisecond // the whole entrance
)

func (edgeDash) Size(u *UI) (float64, float64) {
	st := u.edgeStats
	if st == nil {
		return dashW, 96
	}
	h := dashHead + dashDiv + dashMid + dashDiv + dashHeat + dashDiv + dashTiles + 4
	if st.Current != nil {
		h += dashCur + dashDiv
	}
	if len(st.Health) > 0 {
		h += 22
	}
	return dashW, float64(h)
}

// phase is an entrance's progress: 0 before delay, eased to 1 over dur.
func phase(age, delay, dur time.Duration) float64 {
	return easeOut(float64(age-delay) / float64(dur))
}

// dashDivider: a hairline between sections, faded in with the entrance.
func dashDivider(u *UI, x, w, y float32, k float64) {
	u.dc.roundRect(x+dashP, y+dashDiv/2, w-2*dashP, 1, 0.5, alpha(u.pal.hair(), k))
}

func (d edgeDash) Draw(u *UI, x, w float32, age time.Duration) bool {
	st := u.edgeStats
	if st == nil {
		d.waiting(u, x, w)
		return false
	}
	y := float32(0)
	d.header(u, st, x, w, age)
	y += dashHead
	if st.Current != nil {
		dashDivider(u, x, w, y, phase(age, 80, 400*time.Millisecond))
		y += dashDiv
		d.current(u, st, x, w, y, age)
		y += dashCur
	}
	dashDivider(u, x, w, y, phase(age, 120, 400*time.Millisecond))
	y += dashDiv
	inner := w - 2*dashP
	lw := (inner - 33) * 0.42
	d.week(u, st, x+dashP, y, lw, age)
	u.dc.roundRect(x+dashP+lw+16, y+4, 1, dashMid-8, 0.5, alpha(u.pal.hair(), phase(age, 200, 400*time.Millisecond)))
	d.rating(u, st, x+dashP+lw+33, y, inner-lw-33, age)
	y += dashMid
	dashDivider(u, x, w, y, phase(age, 300, 400*time.Millisecond))
	y += dashDiv
	d.year(u, st, x+dashP, y, inner, age)
	y += dashHeat
	dashDivider(u, x, w, y, phase(age, 700, 400*time.Millisecond))
	y += dashDiv
	d.tiles(u, st, x+dashP, y, inner, age)
	y += dashTiles + 4
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

// ringArc strokes an arc with round ends: progress rings.
func ringArc(c *canvas, cx, cy, r, w float32, a0, a1 float64, col color.NRGBA) {
	c.arc(cx, cy, r, w, float32(a0), float32(a1), col)
	for _, a := range []float64{a0, a1} {
		c.circle(cx+r*float32(math.Cos(a)), cy+r*float32(math.Sin(a)), w/2, col)
	}
}

// ---- the day ----

func (edgeDash) header(u *UI, st *EdgeStats, x, w float32, age time.Duration) {
	p, t := u.pal, st.Today
	cx, cy, r := x+dashP+18, float32(29), float32(16)
	frac := 1.0
	if t.Need > 0 {
		frac = math.Min(1, float64(t.Got)/float64(t.Need))
	}
	ring := p.yellow
	if t.Done {
		ring = p.green
	}
	u.dc.arc(cx, cy, r, 3.5, 0, 2*math.Pi, alpha(p.fg, 0.08))
	if sweep := frac * phase(age, 80, 900*time.Millisecond); sweep > 0.002 {
		ringArc(u.dc, cx, cy, r, 3.5, -math.Pi/2, -math.Pi/2+2*math.Pi*sweep, ring)
	}
	n := fmt.Sprintf("%d/%d", t.Got, t.Need)
	f := face(fSemiBold, 12)
	u.dc.text(f, n, cx-measure(f, n)/2, cy+4.5, p.fg)
	if st.Current != nil && !st.Current.Started.IsZero() { // working on it: the ring breathes on the island's beat
		u.lives = append(u.lives, live{r: image.Rect(int(cx-r-10), int(cy-r-10), int(cx+r+11), int(cy+r+11)), draw: func(c *canvas, tt float64) {
			k := math.Mod(tt/(2*beat), 1)
			rr := r + 4 + 5*float32(easeOut(k))
			c.frame(cx-rr, cy-rr, 2*rr, 2*rr, rr, 1.1, alpha(ring, 0.4*(1-k)*math.Min(1, k*6)))
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
	tx := cx + r + 16
	u.dc.text(face(fSemiBold, 15), head, tx, 25, p.fg)
	sub, subCol := gateWords(st), p.muted
	if t.Gate == "closed" {
		subCol = mix(p.yellow, p.fg, 0.35)
	}
	sx := tx
	if t.Gate == "closed" { // a small padlock says it before the words do
		u.dc.lock(sx, 33, 9, subCol)
		sx += 14
	}
	if t.WorkSec > 0 {
		sub += " · " + mins(time.Duration(t.WorkSec)*time.Second) + " focused"
	}
	u.dc.text(fCaption(), sub, sx, 42, subCol)

	// the streak, at the right: a living flame and the count
	rx := x + w - dashP
	big := face(fSemiBold, 24)
	num := fmt.Sprint(int(math.Round(float64(st.Streak.Current) * phase(age, 200, 700*time.Millisecond))))
	nw := measure(big, num)
	u.dc.text(big, num, rx-nw, 31, p.fg)
	cap := fmt.Sprintf("day streak · best %d", st.Streak.Best)
	cw := measure(fCaption(), cap)
	u.dc.text(fCaption(), cap, rx-cw, 46, p.faint)
	fx, fy := rx-nw-13, float32(26)
	hot := st.Streak.Current > 0
	if hot { // the flame lights the island around it
		u.cv.glowInside(image.Rect(int(fx-40), 4, int(fx+40), 60), fx, fy-4, 34, mix(p.yellow, p.red, 0.4), 0.12*phase(age, 200, 600*time.Millisecond))
	}
	u.lives = append(u.lives, live{r: image.Rect(int(fx-10), int(fy-17), int(fx+10), int(fy+10)), draw: func(c *canvas, tt float64) {
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
	cy := y + dashCur/2
	kind := map[string]string{"new": "New", "resolve": "Re-solve", "reentry": "Again"}[c.Kind]
	if kind == "" {
		kind = c.Kind
	}
	kw := eyebrowW(kind) + 14
	cx := x + dashP
	u.dc.roundRect(cx, cy-8, kw, 16, 8, alpha(p.yellow, 0.16*k))
	u.dc.eyebrow(kind, cx+7, cy+3.5, alpha(p.yellow, k))
	right := x + w - dashP
	// the hint ladder: a dot a hint, used, ready, then the next one filling
	// as its time comes
	hx := right
	next := 0.0
	if c.NextHintSec > 0 {
		next = math.Max(0.08, 1-float64(c.NextHintSec)/1800)
	}
	dots := c.HintsAllowed + map[bool]int{true: 1}[c.NextHintSec > 0]
	dots = max(dots, c.Hints, 1)
	for i := dots - 1; i >= 0; i-- {
		dx := hx - 5
		switch {
		case i < c.Hints: // used
			u.dc.circle(dx, cy, 3.5, alpha(p.muted, k))
		case i < c.HintsAllowed: // ready to take
			u.dc.circle(dx, cy, 3.5, alpha(p.yellow, k))
		default: // coming: its ring fills as the wait runs down
			u.dc.arc(dx, cy, 3, 1.1, 0, 2*math.Pi, alpha(p.fg, 0.18*k))
			if next > 0 {
				ringArc(u.dc, dx, cy, 3, 1.1, -math.Pi/2, -math.Pi/2+2*math.Pi*next, alpha(p.yellow, 0.8*k))
			}
		}
		hx -= 12
	}
	u.dc.text(fCaption(), "hints", hx-measure(fCaption(), "hints")+2, cy+4, alpha(p.faint, k))
	hx -= measure(fCaption(), "hints") + 12
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
		facts = append(facts, "next in "+mins(time.Duration(c.NextHintSec)*time.Second))
	}
	if c.Fails > 0 {
		facts = append(facts, plural(c.Fails, "fail"))
	}
	ft := strings.Join(facts, " · ")
	fw := measure(fCaption(), ft)
	u.dc.text(fCaption(), ft, hx-fw, cy+4, alpha(p.muted, k))
	tx := cx + kw + 10
	title := strings.TrimPrefix(c.ID, "cf:") + " · " + c.Title
	if c.Source == "lc" {
		title = c.Title
	}
	tw := u.dc.text(fBody(), ellipsize(fBody(), title, hx-fw-50-tx), tx, cy+4.5, alpha(p.fg, k))
	if c.Rating > 0 {
		u.dc.text(fCaption(), fmt.Sprint(c.Rating), tx+tw+8, cy+4, alpha(p.faint, k))
	}
}

// ---- the week ----

func (edgeDash) week(u *UI, st *EdgeStats, x, y, w float32, age time.Duration) {
	p := u.pal
	u.dc.eyebrow("This week", x, y+14, p.faint)
	tot := fmt.Sprintf("%d/%d", st.Week.Credits, st.Week.Target)
	f := face(fSemiBold, 12)
	u.dc.text(f, tot, x+w-measure(f, tot), y+14, p.fg)
	days := st.Week.Days
	if len(days) == 0 {
		return
	}
	top := 1
	for _, d := range days {
		top = max(top, d.Credits, d.Need)
	}
	slot := w / float32(len(days))
	base, maxH := y+dashMid-22, float32(dashMid-22-28)
	today := st.Today.Date
	future := false
	for i, d := range days {
		bx := x + float32(i)*slot + slot/2
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
				if isToday && !d.Met { // today's bar breathes until it's met
					bh := max(h, bw*0.5)
					u.lives = append(u.lives, live{r: image.Rect(int(bx-bw/2)-3, int(base-bh)-3, int(bx+bw/2)+4, int(base)+3), draw: func(c *canvas, t float64) {
						q := 0.5 + 0.5*math.Sin(t*2*math.Pi/(2*beat))
						c.frame(bx-bw/2-1.5, base-bh-1.5, bw+3, bh+3, bw/2.6+1.5, 1, alpha(col, 0.15+0.3*q))
					}})
				}
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
	name, src := "Codeforces", "cf"
	s, ok := st.Sources["cf"]
	if !ok || (s.Rating == 0 && len(s.RatingHistory) == 0) {
		if l, ok2 := st.Sources["lc"]; ok2 && (l.Rating > 0 || len(l.RatingHistory) > 0) {
			s, name, src = l, "LeetCode", "lc"
		}
	}
	_ = src
	u.dc.eyebrow(name, x, y+14, p.faint)
	h := s.RatingHistory
	rx := x + w
	if s.Rating > 0 {
		f := face(fSemiBold, 12)
		r := fmt.Sprint(int(math.Round(float64(s.Rating) * phase(age, 250, 900*time.Millisecond))))
		rw := measure(f, r)
		u.dc.text(f, r, rx-rw, y+14, p.fg)
		if n := len(h); n >= 2 {
			dlt := h[n-1].Rating - h[n-2].Rating
			ds, dc := fmt.Sprintf("+%d", dlt), p.green
			if dlt < 0 {
				ds, dc = fmt.Sprintf("−%d", -dlt), p.red
			}
			u.dc.text(fCaption(), ds, rx-rw-6-measure(fCaption(), ds), y+14, alpha(dc, phase(age, 900, 400*time.Millisecond)))
		}
	}
	gx, gy, gw, gh := x, y+26, w, float32(dashMid-26-8)
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
	// the line draws itself from the first contest to the last, a soft
	// fill rising under it
	reveal := float32(phase(age, 350, 1100*time.Millisecond)) * float32(len(pts)-1)
	if fk := phase(age, 700, 700*time.Millisecond); fk > 0 {
		area := append([]pt{{pts[0].x, gy + gh}}, pts...)
		area = append(area, pt{pts[len(pts)-1].x, gy + gh})
		u.dc.poly(alpha(p.blue, 0.08*fk), area...)
		for j := 1; j <= 3; j++ { // denser toward the line: a gradient in a few layers
			up := make([]pt, 0, len(pts)+2)
			sh := gh * float32(j) * 0.18
			up = append(up, pt{pts[0].x, min(pts[0].y+sh, gy+gh)})
			for _, q := range pts {
				up = append(up, pt{q.x, q.y})
			}
			for i := len(pts) - 1; i >= 0; i-- {
				up = append(up, pt{pts[i].x, min(pts[i].y+sh, gy+gh)})
			}
			u.dc.poly(alpha(p.blue, 0.035*fk), up...)
		}
	}
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
		u.lives = append(u.lives, live{r: image.Rect(int(last.x-9), int(last.y-9), int(last.x+10), int(last.y+10)), draw: func(c *canvas, t float64) {
			k := math.Mod(t/1.8, 1)
			rr := 3 + 5*float32(k)
			c.frame(last.x-rr, last.y-rr, 2*rr, 2*rr, rr, 1, alpha(p.blue, 0.5*(1-k)))
		}})
	}
}

// ---- the year ----

func (edgeDash) year(u *UI, st *EdgeStats, x, y, w float32, age time.Duration) {
	p := u.pal
	cal := st.Calendar
	met := 0
	for _, d := range cal {
		if d.Met {
			met++
		}
	}
	// the caption: the year, or the day under the pointer
	caption, capCol := "", p.muted
	right := plural(met, "day") + " met"
	hovered := -1
	if strings.HasPrefix(u.hoverKey, "heat:") {
		fmt.Sscan(strings.TrimPrefix(u.hoverKey, "heat:"), &hovered)
	}
	if hovered >= 0 && hovered < len(cal) {
		caption, capCol, right = dayWords(cal[hovered]), p.fg, ""
	}
	if caption == "" {
		u.dc.eyebrow("The last 12 months", x, y+14, p.faint)
	} else { // the day under the pointer
		u.dc.text(fCaption(), caption, x, y+14, capCol)
	}
	if right != "" {
		u.dc.text(fCaption(), right, x+w-measure(fCaption(), right), y+14, p.faint)
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
	pitch := min(float32(8.8), w/float32(cols))
	cell := pitch - 2
	gx := x + (w-pitch*float32(cols))/2
	gy := y + 22
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
	mf := fCaption()
	for col, m := range months {
		u.dc.text(mf, m, gx+float32(col)*pitch, gy+7*pitch+11, alpha(p.faint, phase(age, 700, 500*time.Millisecond)))
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

	tw := w / float32(len(ts)) // a row of four, hairlines between: numbers, not boxes
	for i, tl := range ts {
		tx := x + float32(i)*tw
		if i > 0 {
			tx += 14
			u.dc.roundRect(tx-14, y+2, 1, dashTiles-6, 0.5, alpha(p.hair(), phase(age, 900, 400*time.Millisecond)))
		}
		k := phase(age, time.Duration(900+90*i)*time.Millisecond, 600*time.Millisecond)
		ty := y + 6*float32(1-k)
		u.dc.circle(tx+3, ty+8, 3, alpha(tl.col, k))
		u.dc.eyebrow(tl.label, tx+11, ty+11.5, alpha(p.faint, k))
		v := tl.format(tl.value * phase(age, time.Duration(950+90*i)*time.Millisecond, 800*time.Millisecond))
		bf := face(fSemiBold, 17)
		u.dc.text(bf, ellipsize(bf, v, tw-20), tx, ty+31, alpha(p.fg, k))
		u.dc.text(fCaption(), ellipsize(fCaption(), tl.sub, tw-20), tx, ty+44, alpha(p.muted, k))
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
