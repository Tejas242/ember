package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
)

// A plugin's dashboard: the widgets it declared (docs/PLUGINS.md), laid out
// in a stack and drawn in Ember's language: sections under small capitals,
// hairlines between them, numbers that count up and lines that draw
// themselves as it opens. A hero, if there is one, is the page's head, with
// Ember standing in its ring. A section can fold to one line; "auto" ones
// fold from the bottom up until the page fits.

// extDash is a plugin's dashboard.
type extDash struct{ name string }

const (
	extDashW    = dashW
	extDashMaxH = 466 // what fits under the bar on a small screen
	heroH       = 94
	foldH       = 24
)

// wbox is one widget, measured and ready to draw at (x, y), w wide.
type wbox struct {
	h    float32
	draw func(x, y, w float32, age time.Duration)
}

func (d extDash) dash(u *UI) *proto.Dashboard {
	if x := u.ext[d.name]; x != nil {
		return x.dash
	}
	return nil
}

// layout measures the stack and decides what folds.
func (d extDash) layout(u *UI) []wbox {
	db := d.dash(u)
	if db == nil {
		return nil
	}
	ws := db.Widgets
	folded := make([]bool, len(ws))
	for i, w := range ws {
		folded[i] = w.Folded == "yes" && !u.unfolded[d.foldKey(i)]
	}
	boxes := func() []wbox {
		var out []wbox
		if len(ws) == 0 || ws[0].Kind != "hero" { // no hero: the page's title is its head
			title := db.Title
			if title == "" {
				title = d.name
			}
			out = append(out, wbox{h: 46, draw: func(x, y, w float32, age time.Duration) {
				u.dc.text(fTitle(), ellipsize(fTitle(), title, w), x, y+30, alpha(u.pal.fg, phase(age, 0, 300*time.Millisecond)))
			}})
		}
		for i := range ws {
			if folded[i] {
				out = append(out, d.fold(u, i, &ws[i]))
			} else {
				out = append(out, u.widget(d.name, &ws[i]))
			}
		}
		return out
	}
	total := func(bs []wbox) float32 {
		h := float32(6)
		for i, b := range bs {
			if i > 0 {
				h += dashDiv
			}
			h += b.h
		}
		return h
	}
	bs := boxes()
	for i := len(ws) - 1; i >= 0 && total(bs) > extDashMaxH; i-- { // "auto" folds, the bottom first
		if ws[i].Folded == "auto" && !folded[i] && !u.unfolded[d.foldKey(i)] {
			folded[i] = true
			bs = boxes()
		}
	}
	return bs
}

func (d extDash) foldKey(i int) string { return d.name + ":" + strconv.Itoa(i) }

// fold: a section reduced to one line, its title and its summary; a click unfolds it.
func (d extDash) fold(u *UI, i int, wd *proto.Widget) wbox {
	return wbox{h: foldH, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		k := phase(age, 300, 400*time.Millisecond)
		u.hits = append(u.hits, hit{image.Rect(int(x), int(y), int(x+w), int(y+foldH)), "x:unfold:" + d.foldKey(i)})
		u.chevron(x+3, y+12, 0, alpha(p.faint, k))
		title := wd.Title
		if title == "" {
			title = wd.Kind
		}
		ew := u.dc.eyebrow(title, x+14, y+15.5, alpha(p.faint, k))
		u.dc.text(fCaption(), ellipsize(fCaption(), wd.Summary, w-ew-26), x+ew+24, y+16, alpha(p.muted, k))
	}}
}

func (d extDash) Size(u *UI) (float64, float64) {
	if db := d.dash(u); db != nil && len(db.Widgets) > 0 && db.Widgets[0].Kind == "stage" {
		return d.stageSize(u, db)
	}
	h := float32(6)
	for i, b := range d.layout(u) {
		if i > 0 {
			h += dashDiv
		}
		h += b.h
	}
	w := float64(extDashW)
	if db := d.dash(u); db != nil && db.Size == "card" { // the card's width, in the card's place
		w = cardW
	}
	return w, float64(min(max(h, 96), extDashMaxH))
}

func (d extDash) Draw(u *UI, x, w float32, age time.Duration) bool {
	if db := d.dash(u); db != nil && len(db.Widgets) > 0 && db.Widgets[0].Kind == "stage" {
		return d.drawStage(u, x, w, age)
	}
	y := float32(0)
	for i, b := range d.layout(u) {
		if i > 0 {
			dashDivider(u, x, w, y, phase(age, time.Duration(80+60*i)*time.Millisecond, 400*time.Millisecond))
			y += dashDiv
		}
		b.draw(x+dashP, y, w-2*dashP, age)
		y += b.h
	}
	d.react(u, age)
	return age < dashEnter
}

// react plays the hero's reaction once each time the page opens: a hop, a
// nod, a celebration (one an hour), a yawn for a sleepy pose. Never over
// something waiting on you.
func (d extDash) react(u *UI, age time.Duration) {
	db := d.dash(u)
	if db == nil || len(db.Widgets) == 0 || db.Widgets[0].Kind != "hero" || db.Widgets[0].Ember == nil {
		return
	}
	e := db.Widgets[0].Ember
	x := u.ext[d.name]
	if age < 700*time.Millisecond || x.reacted.Equal(u.dashAt) || u.waitingOnYou() {
		return
	}
	x.reacted = u.dashAt
	switch {
	case e.React == "celebrate":
		if u.bond == nil || u.bond.cooled("react:"+d.name, time.Hour, u.now()) {
			u.cheer(cheer{size: 0.95, flare: true})
		}
	case e.React != "":
		u.play(e.React, 0)
	case e.Pose == "sleepy":
		u.play("yawn", 0)
	}
}

// heroPose is the pose a plugin's hero asks of Ember, while its page is open.
func (u *UI) heroPose(ps *pose) {
	if u.lastView != vDash {
		return
	}
	db := extDash{name: u.dash}.dash(u)
	if db == nil || len(db.Widgets) == 0 || db.Widgets[0].Kind != "hero" || db.Widgets[0].Ember == nil {
		return
	}
	switch db.Widgets[0].Ember.Pose {
	case "attentive": // reading
		ps.eyeH, ps.lookX, ps.lookY = 4, 1, 0.5
	case "sidelong": // a look at the button
		ps.lookX, ps.lookY = 1, 0.5
	case "game": // eyes front, determined: the focus grant's
		if u.granted(u.dash, proto.GrantFocus) {
			ps.slant, ps.lid, ps.lookX, ps.lookY, ps.eyeH = 0.4, max(ps.lid, 0.15), 0, 0, 4
		}
	case "sleepy": // content and drowsy
		ps.lid = max(ps.lid, 0.55)
		ps.tipH -= 0.15
		ps.lookY = 1
	}
}

// widget measures one widget (not folded).
func (u *UI) widget(name string, wd *proto.Widget) wbox {
	switch wd.Kind {
	case "hero":
		return u.heroW(name, wd)
	case "columns":
		a, b := u.widget(name, wd.A), u.widget(name, wd.B)
		h := max(a.h, b.h)
		return wbox{h: h, draw: func(x, y, w float32, age time.Duration) {
			const gap = 33
			lw := (w - gap) * float32(wd.Split)
			a.draw(x, y, lw, age)
			u.dc.roundRect(x+lw+16, y+6, 1, h-12, 0.5, alpha(u.pal.hair(), phase(age, 200, 400*time.Millisecond)))
			b.draw(x+lw+gap, y, w-lw-gap, age)
		}}
	case "stats":
		return u.statsW(wd)
	case "sparkline":
		return u.sparkW(wd)
	case "line":
		return u.lineW(wd)
	case "bars":
		if wd.Layout == "columns" {
			return u.columnBarsW(wd)
		}
		return u.rowBarsW(wd)
	case "chips":
		return u.chipsW(wd)
	case "list":
		return u.listW(wd)
	case "heatmap":
		return u.heatW(wd)
	case "agenda":
		return u.agendaW(wd)
	case "note":
		return u.noteW(wd)
	}
	return wbox{draw: func(x, y, w float32, age time.Duration) {}}
}

// head draws a section's title (small capitals) and its caption at the right.
func (u *UI) head(wd *proto.Widget, x, y, w float32, k float64) {
	if wd.Title != "" {
		u.dc.eyebrow(wd.Title, x, y+15, alpha(u.pal.faint, k))
	}
	if wd.Caption != "" {
		u.dc.text(fCaption(), wd.Caption, x+w-measure(fCaption(), wd.Caption), y+15, alpha(u.pal.faint, k))
	}
}

// tint is a mood's colour; quiet is muted.
func (p palette) tint(m string) color.NRGBA {
	md := extMood(m)
	if md == moodIdle {
		return p.muted
	}
	return p.moodColor(md)
}

// ---- hero: Ember in the ring, the state in words, the next step with its button ----

func (u *UI) heroW(name string, h *proto.Widget) wbox {
	ht := float32(heroH)
	if u.stepWraps(h, extDashW-2*dashP) {
		ht += 14
	}
	return wbox{h: ht, draw: func(x, y, w float32, age time.Duration) {
		p, now := u.pal, u.now()
		cx, cy, r := x+31, y+46, float32(29)
		u.dc.arc(cx, cy, r, 3.5, 0, 2*math.Pi, alpha(p.fg, 0.08))
		if g := h.Ring; g != nil { // the day's goal (or a round's clock), Ember standing in it
			frac, count := 0.0, g.Mode == "countdown"
			switch {
			case g.EndMs > g.StartMs && g.StartMs > 0:
				span := time.UnixMilli(g.EndMs).Sub(time.UnixMilli(g.StartMs))
				frac = math.Max(0, math.Min(1, float64(time.UnixMilli(g.EndMs).Sub(now))/float64(span)))
			case g.Goal > 0:
				frac = math.Min(1, g.Value/g.Goal)
			}
			if sw := frac * phase(age, 80, 900*time.Millisecond); sw > 0.002 {
				col := p.tint(g.Mood)
				if count {
					ringArc(u.dc, cx, cy, r, 3.5, -math.Pi/2+2*math.Pi*(1-sw), 3*math.Pi/2, col)
				} else {
					ringArc(u.dc, cx, cy, r, 3.5, -math.Pi/2, -math.Pi/2+2*math.Pi*sw, col)
				}
			}
		}
		md := moodIdle
		if h.Ember != nil {
			md = extMood(h.Ember.Mood)
		}
		u.spot = emberSpot{x: cx, y: cy + 1, s: 40, mood: md, show: true}

		tx, right := x+78, x+w
		k := phase(age, 60, 400*time.Millisecond)
		tw := u.dc.text(face(fSemiBold, 15), ellipsize(face(fSemiBold, 15), h.Title, w-160), tx, y+27, alpha(p.fg, k))
		if h.Aside != "" {
			u.dc.text(fSecond(), h.Aside, tx+tw+8, y+27, alpha(p.muted, k))
		}
		if s := h.Stat; s != nil {
			cw := measure(fCaption(), s.Caption)
			u.dc.text(fCaption(), s.Caption, right-cw, y+26, alpha(p.faint, k))
			f := face(fSemiBold, 15)
			u.dc.text(f, s.Value, right-cw-5-measure(f, s.Value), y+27, alpha(p.fg, k))
		}
		subCol := p.muted
		if h.SubMood == proto.Nudge {
			subCol = mix(p.yellow, p.fg, 0.35)
		} else if h.SubMood != proto.Quiet {
			subCol = p.tint(h.SubMood)
		}
		sx := tx
		switch h.SubIcon {
		case "":
		case "lock":
			u.dc.lock(sx, y+36, 9, subCol)
			sx += 14
		default:
			u.dc.icon(h.SubIcon, sx+5, y+41, 10, subCol)
			sx += 14
		}
		if ls := chipLetters(h.Chips); len(ls) > 0 { // a round: its letters, then the words
			sx += u.letters(ls, sx, y+41, 15, k) + 10
		}
		u.dc.text(fCaption(), ellipsize(fCaption(), h.Sub, right-sx), sx, y+45, alpha(subCol, k))
		u.heroStep(name, h, tx, y+72, right, age)
	}}
}

// stepRoom is what the step's text has beside its buttons and dots.
func (u *UI) stepRoom(h *proto.Widget, tx, right float32) (lx, room float32) {
	bx := right
	for _, b := range h.Buttons {
		f := face(fMedium, 12)
		bx -= measure(f, b.Label) + 28 + 8
	}
	bx -= 4
	if s := h.Step; s != nil && s.Dots != nil {
		bx -= 11*float32(s.Dots.Total) + 6
	}
	lx = tx
	if s := h.Step; s != nil && s.Tag != "" {
		lx += eyebrowW(s.Tag) + 14 + 8
	}
	return lx, bx - lx
}

// stepWraps: the step's text and its facts don't share a line; the facts go under the text.
func (u *UI) stepWraps(h *proto.Widget, w float32) bool {
	s := h.Step
	if s == nil || s.Facts == "" {
		return false
	}
	_, room := u.stepRoom(h, 78, w)
	return measure(fBody(), s.Text)+10+measure(fCaption(), s.Facts) > room
}

func (u *UI) heroStep(name string, h *proto.Widget, tx, ly, right float32, age time.Duration) {
	p, s := u.pal, h.Step
	k2 := phase(age, 160, 450*time.Millisecond)
	ly += 4 * float32(1-k2)
	bx := right
	for i := len(h.Buttons) - 1; i >= 0; i-- {
		b := h.Buttons[i]
		key := "x:dbtn:" + name + "\x00" + b.Key
		var bw float32
		if b.Primary {
			bw = u.pillButton(Button{Key: b.Key, Label: b.Label, Primary: true}, key, bx, ly-btnH/2, btnH, p.yellow)
		} else {
			f := face(fMedium, 12)
			bw = measure(f, b.Label) + 28
			u.dc.roundRect(bx-bw, ly-btnH/2, bw, btnH, btnH/2, alpha(p.fg, 0.08+0.06*u.hl(key)))
			u.dc.text(f, b.Label, bx-bw+14, ly+4.2, p.fg)
			u.hits = append(u.hits, hit{image.Rect(int(bx-bw), int(ly-btnH/2), int(bx), int(ly+btnH/2)), key})
		}
		bx -= bw + 8
	}
	if s == nil {
		return
	}
	bx -= 4
	if d := s.Dots; d != nil { // a ladder: used, ready, the next one filling
		for i := d.Total - 1; i >= 0; i-- {
			dx := bx - 4
			switch {
			case i < d.Used:
				u.dc.circle(dx, ly, 3.2, alpha(p.muted, k2))
			case i < d.Ready:
				u.dc.circle(dx, ly, 3.2, alpha(p.yellow, k2))
			default:
				u.dc.arc(dx, ly, 2.8, 1.1, 0, 2*math.Pi, alpha(p.fg, 0.2*k2))
				if i == d.Ready && d.Next > 0 {
					ringArc(u.dc, dx, ly, 2.8, 1.1, -math.Pi/2, -math.Pi/2+2*math.Pi*d.Next, alpha(p.yellow, 0.85*k2))
				}
			}
			bx -= 11
		}
		bx -= 6
	}
	lx := tx
	if s.Tag != "" {
		tc := p.tint(s.TagMood)
		tw := eyebrowW(s.Tag) + 14
		u.dc.roundRect(lx, ly-8, tw, 16, 8, alpha(tc, 0.16*k2))
		u.dc.eyebrow(s.Tag, lx+7, ly+3.5, alpha(tc, k2))
		lx += tw + 8
	}
	wraps := u.stepWraps(h, right-tx+78)
	fw := float32(0)
	if s.Facts != "" && !wraps {
		fw = measure(fCaption(), s.Facts)
	}
	room := bx - lx - fw - 10
	lw := u.dc.text(fBody(), ellipsize(fBody(), s.Text, room), lx, ly+4.5, alpha(p.fg, k2))
	if s.Meta != "" && lw+measure(fCaption(), s.Meta)+6 < room {
		u.dc.text(fCaption(), s.Meta, lx+lw+6, ly+4, alpha(p.faint, k2))
	}
	switch {
	case s.Facts != "" && wraps: // under the text, so the text keeps its room
		u.dc.text(fCaption(), ellipsize(fCaption(), s.Facts, bx-lx), lx, ly+20, alpha(p.muted, k2))
	case s.Facts != "":
		u.dc.text(fCaption(), s.Facts, bx-fw, ly+4, alpha(p.muted, k2))
	}
}

// ---- stats: a grid of numbers (cols × rows), counting up as it opens ----

func statText(it proto.Item, k float64) string {
	if it.Unit == "" && it.Text != "" && (it.Num == 0 || k >= 1) {
		return it.Text
	}
	v := it.Num * k
	switch it.Unit {
	case "%":
		return fmt.Sprintf("%d%%", int(math.Round(v)))
	case "s":
		return mins(time.Duration(v) * time.Second)
	}
	if it.Text != "" && k >= 1 {
		return it.Text
	}
	return trimNum(math.Round(v))
}

func (u *UI) statsW(wd *proto.Widget) wbox {
	cols := max(wd.Cols, 1)
	rows := (len(wd.Items) + cols - 1) / cols
	top := float32(0)
	if wd.Title != "" {
		top = 20
	}
	h := top + 8 + 44*float32(max(rows, 1))
	return wbox{h: h, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		u.head(wd, x, y, w, phase(age, 0, 400*time.Millisecond))
		cw := w / float32(cols)
		for i, it := range wd.Items {
			cx, cy := x+float32(i%cols)*cw, y+top+6+float32(i/cols)*44
			k := phase(age, time.Duration(700+90*i)*time.Millisecond, 600*time.Millisecond)
			if it.Mood != proto.Quiet {
				u.dc.circle(cx+3, cy+10, 3, alpha(p.tint(it.Mood), k))
			}
			lx := cx + 11
			if it.Mood == proto.Quiet {
				lx = cx
			}
			u.dc.eyebrow(it.Label, lx, cy+13.5, alpha(p.faint, k))
			count := 1.0
			if wd.CountUp {
				count = phase(age, time.Duration(750+90*i)*time.Millisecond, 800*time.Millisecond)
			}
			v := statText(it, count)
			bf := face(fSemiBold, 16)
			vw := u.dc.text(bf, v, cx, cy+34, alpha(p.fg, k))
			if room := cw - vw - 14; it.Caption != "" && measure(fCaption(), it.Caption) <= room {
				u.dc.text(fCaption(), it.Caption, cx+vw+6, cy+33, alpha(p.faint, k))
			}
		}
	}}
}

// ---- sparkline: a line through values, oldest first ----

func (u *UI) sparkW(wd *proto.Widget) wbox {
	top := float32(0)
	if wd.Title != "" {
		top = 20
	}
	return wbox{h: top + 62, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		k := phase(age, 80, 700*time.Millisecond)
		if wd.Title != "" {
			u.dc.eyebrow(wd.Title, x, y+14, alpha(p.faint, k))
		}
		vs := wd.Values
		gy, gh := y+top+6, float32(36)
		if len(vs) >= 2 {
			lo, hi := vs[0], vs[0]
			for _, v := range vs {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
			if hi-lo < 1e-9 {
				hi = lo + 1
			}
			n := float32(len(vs) - 1)
			shown := 1 + int(float64(len(vs)-1)*k) // the line draws itself in
			pts := []pt{{x, gy + gh}}
			for j, v := range vs[:shown] {
				pts = append(pts, pt{x + w*float32(j)/n, gy + gh - gh*float32((v-lo)/(hi-lo))})
			}
			last := pts[len(pts)-1]
			pts = append(pts, pt{last.x, gy + gh})
			u.dc.poly(alpha(p.green, 0.16), pts...)
			for j := 1; j < len(pts)-2; j++ {
				u.dc.line(pts[j], pts[j+1], 1.4, p.green)
			}
			u.dc.circle(last.x, last.y, 2.4, p.green)
		}
		if wd.Caption != "" {
			u.dc.text(fCaption(), ellipsize(fCaption(), wd.Caption, w), x, y+top+56, alpha(p.muted, k))
		}
	}}
}

// ---- line: a series over a shaded band, the last point lit ----

func (u *UI) lineW(wd *proto.Widget) wbox {
	const h = 96
	return wbox{h: h, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		u.dc.eyebrow(wd.Title, x, y+15, p.faint)
		rx := x + w
		if wd.Extra != "" {
			ew := measure(fCaption(), wd.Extra)
			u.dc.text(fCaption(), wd.Extra, rx-ew, y+15, p.faint)
			rx -= ew + 10
		}
		f := face(fSemiBold, 12)
		vw := measure(f, wd.Value)
		u.dc.text(f, wd.Value, rx-vw, y+15, p.fg)
		if wd.Delta != "" {
			u.dc.text(fCaption(), wd.Delta, rx-vw-6-measure(fCaption(), wd.Delta), y+15, alpha(p.tint(wd.DeltaMood), phase(age, 900, 400*time.Millisecond)))
		}
		vs := wd.Values
		gx, gy, gw, gh := x, y+27, w, float32(h-27-10)
		if len(vs) < 2 {
			return
		}
		lo, hi := vs[0], vs[0]
		for _, r := range vs {
			lo, hi = math.Min(lo, r), math.Max(hi, r)
		}
		b := wd.Band
		if b != nil {
			lo, hi = math.Min(lo, b.Lo), math.Max(hi, b.Hi)
		}
		// a few points of noise on a big number (a rating) don't fill the whole height
		span := float32(math.Max(hi-lo, math.Max(0.04*math.Max(math.Abs(hi), math.Abs(lo)), 1e-9)))
		yOf := func(r float64) float32 { return gy + gh - gh*float32(r-lo)/span }
		if b != nil {
			k := phase(age, 200, 500*time.Millisecond)
			y0, y1 := yOf(b.Hi), yOf(b.Lo)
			u.dc.roundRect(gx, y0, gw, y1-y0, 4, alpha(p.yellow, 0.07*k))
			u.dc.text(face(fRegular, 9.5), b.Label, gx+4, y0+10, alpha(p.yellow, 0.6*k))
		}
		pts := make([]pt, len(vs))
		for i, r := range vs {
			pts[i] = pt{gx + gw*float32(i)/float32(len(vs)-1), yOf(r)}
		}
		reveal := float32(phase(age, 350, 1100*time.Millisecond)) * float32(len(pts)-1)
		if fk := phase(age, 700, 700*time.Millisecond); fk > 0 {
			area := append([]pt{{pts[0].x, gy + gh}}, pts...)
			area = append(area, pt{pts[len(pts)-1].x, gy + gh})
			u.dc.poly(alpha(p.blue, 0.1*fk), area...)
		}
		for i := 1; i < len(pts); i++ {
			a, c := pts[i-1], pts[i]
			seg := min(max(reveal-float32(i-1), 0), 1)
			if seg <= 0 {
				break
			}
			u.dc.line(a, pt{a.x + (c.x-a.x)*seg, a.y + (c.y-a.y)*seg}, 1.8, p.blue)
		}
		if reveal >= float32(len(pts)-1) {
			last := pts[len(pts)-1]
			u.dc.circle(last.x, last.y, 3, p.blue)
		}
	}}
}

// ---- bars ----

// columnBarsW: a column a bar, each with a ghost of its target; today's is marked.
func (u *UI) columnBarsW(wd *proto.Widget) wbox {
	const h = 98
	return wbox{h: h, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		u.dc.eyebrow(wd.Title, x, y+15, p.faint)
		f := face(fSemiBold, 12)
		u.dc.text(f, wd.Value, x+w-measure(f, wd.Value), y+15, p.fg)
		if wd.Caption != "" {
			u.dc.text(fCaption(), wd.Caption, x+w-measure(f, wd.Value)-6-measure(fCaption(), wd.Caption), y+15, p.faint)
		}
		if len(wd.Items) == 0 {
			return
		}
		top := 1.0
		for _, d := range wd.Items {
			t := 0.0
			if d.Target != nil {
				t = *d.Target
			}
			top = math.Max(top, math.Max(d.Num, t))
		}
		slot := w / float32(len(wd.Items))
		base, maxH := y+h-22, float32(h-22-28)
		for i, d := range wd.Items {
			bx := x + float32(i)*slot + slot/2
			bw := min(slot*0.46, 16)
			k := float32(phase(age, time.Duration(150+70*i)*time.Millisecond, 650*time.Millisecond))
			needH := float32(0)
			if d.Target != nil {
				needH = maxH * float32(*d.Target/top)
				u.dc.roundRect(bx-bw/2, base-needH, bw, needH, bw/2.6, alpha(p.fg, 0.05))
			}
			if d.State != "future" {
				hh := maxH * float32(d.Num/top) * k
				col := p.yellow
				if d.Mood == proto.Done {
					col = p.green
				}
				switch {
				case d.State == "excused" && d.Num == 0:
					u.dc.frame(bx-bw/2, base-needH, bw, needH, bw/2.6, 1.2, alpha(p.blue, 0.55*float64(k)))
				case hh > 0.5:
					u.dc.roundRect(bx-bw/2, base-max(hh, bw*0.5), bw, max(hh, bw*0.5), bw/2.6, col)
				case d.Today && needH > 0:
					u.dc.frame(bx-bw/2, base-needH, bw, needH, bw/2.6, 1.1, alpha(p.yellow, 0.45))
				}
			}
			lc, lf := p.faint, fCaption()
			if d.Today {
				lc, lf = p.fg, face(fSemiBold, 11)
				u.dc.circle(bx, base+17, 1.6, p.fg)
			}
			u.dc.text(lf, d.Label, bx-measure(lf, d.Label)/2, base+13, lc)
		}
	}}
}

// rowBarsW: horizontal bars, a label at the left and the value at the right.
func (u *UI) rowBarsW(wd *proto.Widget) wbox {
	top := float32(0)
	if wd.Title != "" {
		top = 20
	}
	return wbox{h: top + float32(len(wd.Items))*20 + 4, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		u.head(wd, x, y, w, phase(age, 0, 400*time.Millisecond))
		top0 := 0.0
		for _, it := range wd.Items {
			top0 = math.Max(top0, it.Num)
		}
		if top0 == 0 {
			top0 = 1
		}
		labelW, valueW := w*0.32, float32(56)
		for j, it := range wd.Items {
			k := float32(phase(age, time.Duration(50*j)*time.Millisecond, 600*time.Millisecond))
			ry := y + top + float32(j)*20
			u.dc.text(fSecond(), ellipsize(fSecond(), it.Label, labelW-8), x, ry+14, p.muted)
			bx, bw := x+labelW, w-labelW-valueW
			u.dc.roundRect(bx, ry+7, bw, 6, 3, alpha(p.fg, 0.06))
			if fw := bw * float32(it.Num/top0) * k; fw > 0.5 {
				u.dc.roundRect(bx, ry+7, max(fw, 6), 6, 3, p.green)
			}
			v := it.Caption
			if v == "" {
				v = trimNum(it.Num)
			}
			f := face(fSemiBold, 11)
			u.dc.text(f, v, x+w-measure(f, v), ry+14, p.fg)
		}
	}}
}

// ---- chips ----

func (u *UI) chipsW(wd *proto.Widget) wbox {
	top := float32(0)
	if wd.Title != "" {
		top = 20
	}
	return wbox{h: top + 30, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		k := phase(age, 0, 400*time.Millisecond)
		if wd.Title != "" {
			u.dc.eyebrow(wd.Title, x, y+14, alpha(p.faint, k))
		}
		ls := chipsAsLetters(wd.Items)
		u.letters(ls, x, y+top+13, 18, k)
		if wd.Caption != "" {
			cx := x + lettersW(ls, 18) + 12
			u.dc.text(fCaption(), ellipsize(fCaption(), wd.Caption, x+w-cx), cx, y+top+17, alpha(p.muted, k))
		}
	}}
}

// ---- list: rows, each with its icon or an outcome mark ----

func (u *UI) listW(wd *proto.Widget) wbox {
	marked := false
	for _, it := range wd.Items {
		marked = marked || it.Mark != ""
	}
	if marked {
		return u.markedListW(wd)
	}
	top := float32(0)
	if wd.Title != "" {
		top = 20
	}
	return wbox{h: top + float32(len(wd.Items))*24 + 2, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		u.head(wd, x, y, w, phase(age, 0, 400*time.Millisecond))
		for j, it := range wd.Items {
			k := phase(age, time.Duration(40*j)*time.Millisecond, 400*time.Millisecond)
			ry := y + top + float32(j)*24
			tint := p.tint(it.Mood)
			if it.Icon != "" {
				u.dc.icon(it.Icon, x+6, ry+11, 11, alpha(tint, k))
			} else {
				u.dc.circle(x+6, ry+11, 2.2, alpha(tint, k))
			}
			tw := u.dc.text(fBody(), ellipsize(fBody(), it.Title, w*0.55), x+20, ry+16, alpha(p.fg, k))
			if it.Sub != "" {
				u.dc.text(fSecond(), ellipsize(fSecond(), it.Sub, w-tw-28), x+28+tw, ry+16, alpha(p.muted, k))
			}
		}
	}}
}

// markedListW: a few rows, an honest outcome mark each, the detail at the right.
func (u *UI) markedListW(wd *proto.Widget) wbox {
	n := min(len(wd.Items), 4)
	return wbox{h: 22 + 16*float32(max(n, 1)), draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		u.head(wd, x, y, w, 1)
		for i, it := range wd.Items[:n] {
			k := phase(age, time.Duration(500+70*i)*time.Millisecond, 450*time.Millisecond)
			cy := y + 34 + float32(i)*16
			switch it.Mark {
			case "done":
				u.dc.circle(x+4, cy, 3.2, alpha(p.green, k))
			case "fail":
				u.dc.circle(x+4, cy, 3.2, alpha(p.red, k))
			case "skip":
				u.dc.arc(x+4, cy, 2.8, 1.2, 0, 2*math.Pi, alpha(p.yellow, k))
			default:
				u.dc.arc(x+4, cy, 2.8, 1.1, 0, 2*math.Pi, alpha(p.faint, 0.8*k))
			}
			rw := measure(fCaption(), it.Sub)
			u.dc.text(fCaption(), it.Sub, x+w-rw, cy+4, alpha(p.faint, k))
			u.dc.text(fSecond(), ellipsize(fSecond(), it.Title, w-rw-24), x+14, cy+4.5, alpha(p.fg, k))
		}
	}}
}

// ---- heatmap: weeks of days, Monday on top, today ringed ----

func (u *UI) heatW(wd *proto.Widget) wbox {
	return wbox{h: 86, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		u.head(wd, x, y, w, 1)
		cal := wd.Days
		if len(cal) == 0 {
			return
		}
		last, err := time.Parse("2006-01-02", cal[len(cal)-1].Date)
		if err != nil {
			return
		}
		lastRow := (int(last.Weekday()) + 6) % 7
		cols := wd.Weeks
		n := min(len(cal), (cols-1)*7+lastRow+1)
		cal = cal[len(cal)-n:]
		pitch := min(float32(8), w/float32(cols))
		cell := pitch - 2
		gx, gy := x+w-pitch*float32(cols)+2, y+24
		months := map[int]string{}
		for i, d := range cal {
			back := len(cal) - 1 - i
			pos := lastRow - back
			col := cols - 1 + floorDiv(pos, 7)
			row := pos - 7*floorDiv(pos, 7)
			cx, cy := gx+float32(col)*pitch, gy+float32(row)*pitch
			k := float32(phase(age, time.Duration(380+col*13+row*9)*time.Millisecond, 420*time.Millisecond))
			if k <= 0 {
				continue
			}
			sz := cell * (0.4 + 0.6*k)
			off := (cell - sz) / 2
			c := alpha(p.fg, 0.09)
			switch {
			case d.Level <= 0:
			case d.Mood == proto.Wait:
				c = alpha(p.blue, 0.5)
			case d.Mood == proto.Quiet:
				c = alpha(p.green, 0.2)
			case d.Level >= 1:
				c = alpha(p.green, 0.95)
			default:
				c = alpha(p.green, 0.3+0.45*d.Level)
			}
			u.dc.roundRect(cx+off, cy+off, sz, sz, 1.6, alpha(c, float64(k)))
			if i == len(cal)-1 {
				u.dc.frame(cx-1.5, cy-1.5, cell+3, cell+3, 3, 1.1, alpha(p.fg, 0.6*float64(k)))
			}
			if t, err := time.Parse("2006-01-02", d.Date); err == nil && t.Day() <= 7 && row == 0 {
				months[col] = t.Format("Jan")
			}
		}
		for col, m := range months {
			u.dc.text(fCaption(), m, gx+float32(col)*pitch, gy+7*pitch+10, alpha(p.faint, phase(age, 700, 500*time.Millisecond)))
		}
	}}
}

// ---- agenda: what's coming, in time order, said the way you'd say it ----

// dayOf is the day a time belongs to when days end at ends ("04:00"): a
// time before it is the night before.
func dayOf(t time.Time, ends string) time.Time {
	hm, err := time.Parse("15:04", ends)
	if err != nil {
		return t
	}
	if t.Hour()*60+t.Minute() <= hm.Hour()*60+hm.Minute() {
		return t.AddDate(0, 0, -1)
	}
	return t
}

// when says how far off t is: "in 1h 07m", "18:35", "Sun night", "Mon 20:05", "Thu 29".
func when(now, t time.Time, ends string) string {
	d := t.Sub(now)
	day := func(x time.Time) time.Time { return dayOf(x, ends) }
	switch {
	case d < 0:
		return "now"
	case d < 3*time.Hour:
		return "in " + mins(d)
	case day(t).YearDay() == day(now).YearDay() && day(t).Year() == day(now).Year():
		return t.Format("15:04")
	case d < 6*24*time.Hour:
		if ends != "" && day(t).YearDay() != t.YearDay() {
			return day(t).Format("Mon") + " night"
		}
		return t.Format("Mon 15:04")
	}
	return t.Format("Mon 2")
}

func (u *UI) agendaW(wd *proto.Widget) wbox {
	now := u.now()
	var items []proto.Item
	for _, it := range wd.Items {
		if !time.UnixMilli(it.AtMs).Before(now) {
			items = append(items, it)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].AtMs < items[j].AtMs })
	h := 26 + 18*float32(max(len(items), 4))
	return wbox{h: h, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		u.head(wd, x, y, w, 1)
		if len(items) == 0 {
			u.dc.text(fCaption(), wd.Empty, x, y+44, p.faint)
			return
		}
		for i, it := range items {
			k := phase(age, time.Duration(200+70*i)*time.Millisecond, 450*time.Millisecond)
			cy := y + 35 + float32(i)*18 + 4*float32(1-k)
			at := time.UnixMilli(it.AtMs)
			wc := p.muted
			if at.Sub(now) < 24*time.Hour { // within a day: it brightens
				wc = p.fg
			}
			u.dc.text(fCaption(), when(now, at, wd.DayEnds), x, cy+4, alpha(wc, k))
			tx, r := x+62, x+w
			if ls := chipLetters(it.Chips); len(ls) > 0 {
				cw := lettersW(ls, 14)
				u.letters(ls, r-cw, cy, 14, k)
				r -= cw + 8
			}
			if it.Tag != "" {
				tw := measure(fCaption(), it.Tag)
				tc := p.tint(it.TagMood)
				if it.TagMood == proto.Quiet {
					tc = p.faint
				}
				u.dc.text(fCaption(), it.Tag, r-tw, cy+4, alpha(tc, k))
				r -= tw + 8
			}
			u.dc.text(fSecond(), ellipsize(fSecond(), it.Title, r-tx), tx, cy+4.5, alpha(p.fg, k))
		}
	}}
}

// ---- note: one quiet line ----

func (u *UI) noteW(wd *proto.Widget) wbox {
	return wbox{h: 22, draw: func(x, y, w float32, age time.Duration) {
		p := u.pal
		k := phase(age, 900, 400*time.Millisecond)
		col := p.muted
		if wd.Mood != proto.Quiet {
			col = p.tint(wd.Mood)
		}
		tx := x
		if wd.Icon != "" {
			u.dc.icon(wd.Icon, x+5, y+11, 11, alpha(col, k))
			tx += 16
		}
		u.dc.text(fCaption(), ellipsize(fCaption(), wd.Text, w-(tx-x)-12), tx, y+15, alpha(mix(col, p.fg, 0.25), k))
	}}
}

// chipsAsLetters are a chips widget's items, to draw.
func chipsAsLetters(items []proto.Item) []chip {
	out := make([]chip, len(items))
	for i, it := range items {
		out[i] = chip{label: it.Label, state: it.State}
	}
	return out
}

func trimNum(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e9 {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}
