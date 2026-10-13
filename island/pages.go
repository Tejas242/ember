package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"
)

// The card that drops from the pill: a header (Ember, a title, a line of
// context, the page switcher) over one page. And the decision cards:
// permission requests, questions, edge's nudges.

const (
	cardW    = 384
	expW     = 384
	btnH     = 30
	capH     = 32 // the System page's capsule sliders
	sessRowH = 46
	musicH   = 150
	optH     = 36
)

// ---- the header ----

func (u *UI) pageTitle(page string) (title, sub string, m mood) {
	now := u.now()
	switch page {
	case "claude":
		n := map[string]int{}
		for _, s := range u.sessions {
			n[s.State]++
		}
		var parts []string
		for _, k := range []struct{ state, word string }{{"work", "working"}, {"wait", "waiting"}, {"idle", "idle"}} {
			if n[k.state] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n[k.state], k.word))
			}
		}
		m = moodIdle
		if ss := u.sessionList(); len(ss) > 0 {
			m = map[string]mood{"work": moodWork, "wait": moodWait, "idle": moodIdle}[ss[0].State]
		}
		return "Claude", strings.Join(parts, " · "), m
	case "music":
		if t := u.track; t != nil {
			title = "Now playing"
			if !t.Playing {
				title = "Paused"
			}
			return title, playerName(t.Player), moodIdle
		}
	case "edge":
		if e := u.edge; e != nil {
			switch {
			case e.StartedMs > 0 && !e.Pending:
				sub = "Practising for " + mins(now.Sub(time.UnixMilli(e.StartedMs)))
			case e.Closed:
				sub = "Claude is locked until you practise"
			case e.Task != "":
				sub = "Today's problem is ready"
			default:
				sub = "Nothing due"
			}
			return "edge", sub, moodNudge
		}
	}
	return now.Format(u.TimeFmt), now.Format("Monday, 2 January"), moodIdle
}

func segWidth(n int) float32 { return float32(n)*30 + float32(n-1)*2 + 4 }

func (u *UI) header(x, w float32, page string) {
	p := u.pal
	title, sub, m := u.pageTitle(page)
	u.spot = emberSpot{x: x + pad + 9, y: 25, s: 18, mood: m, show: true}
	tx, right := x+pad+30, x+w-pad
	if pages := u.pages(); len(pages) >= 2 {
		u.segmented(pages, page, right, 11)
		right -= segWidth(len(pages)) + 12
	}
	u.dc.text(fTitle(), ellipsize(fTitle(), title, right-tx), tx, 23, p.fg)
	u.dc.text(fCaption(), ellipsize(fCaption(), sub, right-tx), tx, 38.5, p.muted)
}

// ---- the expanded card ----

func (u *UI) expandedHeight() int {
	h := headH + 6
	switch u.pageFor() {
	case "music":
		h += musicH
	case "claude":
		n := min(len(u.sessions), 4)
		h += n*(sessRowH+rowGap) - rowGap
		if len(u.sessions) > 4 {
			h += 20
		}
	case "edge":
		h += 40 + 40 + 12 + btnH
	case "system":
		h += u.systemRows()*(capH+10) - 10
	default:
		h += 24
	}
	return min(h+pad, u.H-14)
}

func (u *UI) drawExpanded(x, w float32) {
	page := u.pageFor()
	u.lastPage = page
	u.header(x, w, page)
	y := float32(headH + 6)
	switch page {
	case "music":
		u.musicPage(x, w, y)
	case "claude":
		u.claudePage(x, w, y)
	case "edge":
		u.edgePage(x, w, y)
	case "system":
		u.systemPage(x, w, y)
	default:
		u.dc.text(fCaption(), "Quiet. Nothing playing, no Claude sessions.", x+pad, y+14, u.pal.faint)
	}
}

// ---- Claude ----

func (u *UI) claudePage(x, w, y float32) {
	p, now := u.pal, u.now()
	ss := u.sessionList()
	for i, s := range ss {
		if i == 4 {
			u.dc.text(fCaption(), fmt.Sprintf("+%d more", len(ss)-4), x+pad, y+13, p.faint)
			break
		}
		key := "focus:" + s.ID
		h := u.hl(key)
		bg := alpha(p.fg, 0.04+0.05*h)
		orb, line2, line2Col := p.faint, "Idle", p.muted
		switch s.State {
		case "work":
			orb, line2, line2Col = p.green, s.Label, mix(p.fg, p.bg, 0.25)
			if line2 == "" {
				line2 = "Thinking"
			}
		case "wait":
			orb, line2, line2Col = p.blue, "Waiting for you", p.blue
			bg = alpha(p.blue, 0.10+0.06*h)
		}
		if s.State == "idle" && !s.Seen.IsZero() {
			line2 = "Idle · " + ago(now.Sub(s.Seen))
		}
		rx0, rw := x+pad-6, w-2*pad+12
		u.dc.roundRect(rx0, y, rw, sessRowH, radRow, bg)
		ox, oy := x+pad+5, y+sessRowH/2
		u.dc.circle(ox, oy, 3.6, orb)
		if s.State != "idle" { // a slow halo: alive, not alarming
			col := orb
			u.lives = append(u.lives, live{image.Rect(int(ox-10), int(oy-10), int(ox+11), int(oy+11)), func(c *canvas, t float64) {
				k := math.Mod(t/1.6, 1)
				c.frame(ox-3.6-6*float32(k), oy-3.6-6*float32(k), 7.2+12*float32(k), 7.2+12*float32(k), 3.6+6*float32(k), 1.2, alpha(col, 0.55*(1-k)))
			}})
		}
		tx, right := x+pad+20, x+w-pad
		// right column: how long, and the last ten minutes of activity
		el := mins(now.Sub(s.Since))
		if s.Since.IsZero() {
			el = ""
		}
		ew := measure(fCaption(), el)
		u.dc.text(fCaption(), el, right-ew, y+19, p.faint)
		if act := s.activity(now); hasActivity(act) {
			u.dc.spark(right-58, y+25, 58, 12, act, alpha(orb, 0.9))
		}
		pf := fBody()
		pw := u.dc.text(pf, ellipsize(pf, s.Project, 120), tx, y+19, p.fg)
		if s.Title != "" {
			u.dc.text(fCaption(), ellipsize(fCaption(), s.Title, right-ew-10-(tx+pw+8)), tx+pw+8, y+19, p.muted)
		}
		l2w := right - 66 - tx
		diff := ""
		if s.State == "work" && (s.Plus > 0 || s.Minus > 0) {
			diff = diffText(s.Plus, s.Minus)
			l2w -= measure(fCaption(), diff) + 8
		}
		if s.Agents > 0 && s.State != "idle" {
			line2 += " · " + agentsText(s.Agents)
		}
		lw := u.dc.text(fCaption(), ellipsize(fCaption(), line2, l2w), tx, y+35, line2Col)
		if diff != "" {
			dx := tx + lw + 8
			if s.Plus > 0 {
				dx += u.dc.text(fCaption(), fmt.Sprintf("+%d", s.Plus), dx, y+35, p.green) + 4
			}
			if s.Minus > 0 {
				u.dc.text(fCaption(), fmt.Sprintf("−%d", s.Minus), dx, y+35, p.red)
			}
		}
		u.hits = append(u.hits, hit{image.Rect(int(rx0), int(y), int(rx0+rw), int(y+sessRowH)), key})
		y += sessRowH + rowGap
	}
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

func hasActivity(v []float64) bool {
	for _, x := range v {
		if x > 0 {
			return true
		}
	}
	return false
}

// spark draws a filled sparkline of v (oldest first) in w×h, bottom at y+h.
func (c *canvas) spark(x, y, w, h float32, v []float64, col color.NRGBA) {
	top := 1.0
	for _, a := range v {
		top = math.Max(top, a)
	}
	n := len(v)
	ps := []pt{{x, y + h}}
	for i, a := range v {
		// a square-root scale: one call still shows, a burst doesn't flatten the rest
		k := float32(math.Sqrt(a / top))
		ps = append(ps, pt{x + w*float32(i)/float32(n-1), y + h - h*k})
	}
	ps = append(ps, pt{x + w, y + h})
	c.poly(alpha(col, 0.28), ps...)
	for i := 1; i < n; i++ { // the top edge, over the fill
		c.line(ps[i], ps[i+1], 1.1, col)
	}
}

// ---- music ----

func (u *UI) musicPage(x, w, y float32) {
	p, t := u.pal, u.track
	if t == nil {
		return
	}
	acc := u.accent(p.fg)
	ax := x + pad
	if a := u.accent(color.NRGBA{}); a.A > 0 { // the card takes on the cover's light
		u.dc.glow(image.Rect(int(x), 0, int(x+w), u.H), ax+30, y+30, 260, a, 0.22)
	}
	const art = 60
	if u.art != nil {
		u.dc.roundRect(ax+1, y+4, art, art, 13, color.NRGBA{0, 0, 0, 70}) // a soft drop shadow
		u.dc.drawArt(u.art.large, int(ax), int(y+2), 13, 1)
	} else {
		u.dc.roundRect(ax, y+2, art, art, 13, alpha(p.fg, 0.08))
		u.dc.icon("volume", ax+art/2, y+2+art/2, 22, p.muted)
	}
	tx := ax + art + 14
	inner := x + w - pad - tx
	u.dc.text(fTitle(), ellipsize(fTitle(), t.Title, inner), tx, y+21, p.fg)
	u.dc.text(fBodyReg(), ellipsize(fBodyReg(), t.Artist, inner), tx, y+39, p.muted)
	if t.Playing {
		u.eqLive(x+w-pad-16, y+57, 10, acc, true)
	}
	y += 80
	// the seek bar: thickens and grows a knob as you come near, drags
	bw := w - 2*pad
	u.seekRect = image.Rect(int(x+pad), int(y-9), int(x+pad+bw), int(y+9))
	if t.Length > 0 {
		h := float32(u.hl("music:seek"))
		u.progressLive(x+pad, y, bw, 4+2*h, acc, h)
		u.hits = append(u.hits, hit{u.seekRect, "music:seek"})
		ty := y + 18 // y moves on below; the closure must not see that
		tr := image.Rect(int(x+pad)-3, int(ty-12), int(x+w-pad)+3, int(ty+5))
		u.lives = append(u.lives, live{tr, func(c *canvas, _ float64) {
			pos := t.pos(u.now())
			if u.drag == "music:seek" {
				pos = time.Duration(u.seekPreview * float64(t.Length))
			}
			f := face(fRegular, 10.5)
			el, tot := clock(pos), "−"+clock(max(t.Length-pos, 0))
			c.text(f, el, x+pad, ty, p.faint)
			c.text(f, tot, x+w-pad-tabWidth(f, tot), ty, p.faint)
		}})
	}
	y += 28
	cx, cy := x+w/2, y+20
	for i, k := range []string{"prev", "toggle", "next"} {
		bx := cx + float32(i-1)*64
		key := "music:" + k
		h := float32(u.hl(key))
		d := float32(u.pressed(key))
		if k == "toggle" { // the one big, filled button
			r := 19 + 1.5*h - 2.5*d
			u.dc.circle(bx, cy, r, mix(p.fg, acc, 0.25))
			u.mediaGlyph(k, bx, cy, t.Playing, p.ink)
			u.hits = append(u.hits, hit{image.Rect(int(bx-21), int(cy-21), int(bx+21), int(cy+21)), key})
			continue
		}
		if h > 0.01 || d > 0 {
			u.dc.circle(bx, cy, 16-1.5*d, alpha(p.fg, 0.10*float64(h)+0.08*float64(d)))
		}
		u.mediaGlyph(k, bx, cy, t.Playing, mix(p.muted, p.fg, 0.5+0.5*float64(h)))
		u.hits = append(u.hits, hit{image.Rect(int(bx-18), int(cy-18), int(bx+18), int(cy+18)), key})
	}
}

// ---- edge ----

func (u *UI) edgePage(x, w, y float32) {
	p, now, e := u.pal, u.now(), u.edge
	if e == nil {
		return
	}
	big := face(fSemiBold, 22)
	tx := x + pad
	tx += u.dc.text(big, fmt.Sprintf("%d/%d", e.Today, e.Need), tx, y+26, p.fg)
	tx += u.dc.text(fCaption(), "  today", tx, y+25, p.muted) + 18
	tx += u.dc.text(big, fmt.Sprint(e.Streak), tx, y+26, p.fg)
	u.dc.text(fCaption(), "  day streak", tx, y+25, p.muted)
	if e.Closed {
		lw := measure(fCaption(), "Claude locked")
		u.dc.lock(x+w-pad-lw-16, y+15, 11, p.yellow)
		u.dc.text(fCaption(), "Claude locked", x+w-pad-lw, y+25, p.yellow)
	}
	y += 40
	if e.Task != "" {
		u.dc.roundRect(x+pad-6, y, w-2*pad+12, 40, radRow, alpha(p.fg, 0.05))
		right := x + w - pad
		if e.StartedMs > 0 && !e.Pending {
			el := mins(now.Sub(time.UnixMilli(e.StartedMs)))
			ew := measure(fNum(), el)
			u.dc.text(fNum(), el, right-ew, y+25, p.green)
			right -= ew + 10
		}
		u.dc.text(fBody(), ellipsize(fBody(), e.Task, right-(x+pad+4)), x+pad+4, y+25, p.fg)
	}
	y += 40 + 12
	var btns []Button
	switch {
	case e.Task != "" && e.Pending:
		btns = []Button{{Key: "snooze", Label: "Later"}, {Key: "start", Label: "Start", Primary: true}}
	case e.Task != "":
		btns = []Button{{Key: "teach", Label: "Give up"}, {Key: "hint", Label: "Hint", Primary: true}}
	}
	u.buttonRow(btns, "btn:edge:", x, w, y, "")
}

// ---- system ----

func (u *UI) systemRows() int {
	n := 0
	if u.vol.ok {
		n++
	}
	if u.light >= 0 {
		n++
	}
	if u.power != nil {
		n++
	}
	return n
}

// capsule is a Control Centre slider: the fill is a pill that always holds
// its icon, the value sits at the far end, the whole thing drags.
func (u *UI) capsule(key, icon, iconKey string, v float64, tint color.NRGBA, value string, x, w, y float32) {
	p := u.pal
	sx, sw := x+pad, w-2*pad
	h := float32(u.hl(key))
	u.dc.roundRect(sx, y, sw, capH, capH/2, alpha(p.fg, 0.07+0.03*float64(h)))
	fw := capH + (sw-capH)*float32(math.Max(0, math.Min(1, v)))
	u.dc.roundRect(sx, y, fw, capH, capH/2, tint)
	ih := float32(0)
	if iconKey != "" {
		ih = float32(u.hl(iconKey))
		if ih > 0.01 {
			u.dc.circle(sx+capH/2, y+capH/2, capH/2-3, alpha(p.ink, 0.18*float64(ih)))
		}
	}
	u.dc.icon(icon, sx+capH/2, y+capH/2, 16, p.ink)
	vf := fNum()
	vx := sx + sw - 14
	col := p.fg
	if sx+fw > vx-tabWidth(vf, value)-4 { // the fill has reached the number
		col = p.ink
	}
	u.rollText(u.dc, "cap:"+key, value, vf, vx, y+capH/2+4.5, col)
	r := image.Rect(int(sx+capH/2), int(y), int(sx+sw-capH/2), int(y+capH))
	u.sliders[key] = r
	u.hits = append(u.hits, hit{image.Rect(int(sx), int(y-3), int(sx+sw), int(y+capH+3)), key})
	if iconKey != "" {
		u.hits = append(u.hits, hit{image.Rect(int(sx), int(y), int(sx+capH), int(y+capH)), iconKey})
	}
}

func (u *UI) systemPage(x, w, y float32) {
	p := u.pal
	if v := u.vol; v.ok {
		icon, tint, val := "volume", mix(p.fg, p.bg, 0.16), fmt.Sprint(int(v.v*100+0.5))
		switch {
		case v.muted:
			icon, tint, val = "mute", p.muted, "Off"
		case v.v < 0.4:
			icon = "volume-low"
		}
		u.capsule("sys:vol", icon, "sys:mute", v.v, tint, val, x, w, y)
		y += capH + 10
	}
	if u.light >= 0 {
		u.capsule("sys:light", "sun", "", u.light, p.yellow, fmt.Sprint(int(u.light*100+0.5)), x, w, y)
		y += capH + 10
	}
	if pw := u.power; pw != nil {
		cy := y + capH/2
		fill := p.fg
		switch {
		case charging(pw.state):
			fill = p.green
		case pw.pct <= 10:
			fill = p.red
		case pw.pct <= 20:
			fill = p.yellow
		}
		u.dc.battery(x+pad+5, cy, 10, float32(pw.pct/100), alpha(p.fg, 0.45), fill)
		tx := x + pad + capH + 6
		tx += u.rollText(u.dc, "batt", pctText(pw.pct), fNum(), tx+tabWidth(fNum(), pctText(pw.pct)), cy+4.5, p.fg)
		if charging(pw.state) && pw.state != 4 {
			u.dc.icon("bolt", tx+9, cy, 12, p.green)
			tx += 16
		}
		if s := powerText(pw); s != "" {
			u.dc.text(fCaption(), s, tx+8, cy+4.5, p.muted)
		}
	}
}

// ---- buttons ----

// buttonRow draws right-aligned pill buttons, and an optional quiet link on
// the left. Hover and press ease.
func (u *UI) buttonRow(btns []Button, prefix string, x, w, y float32, footer string) {
	p := u.pal
	bx := x + w - pad
	for i := len(btns) - 1; i >= 0; i-- {
		b := btns[i]
		bf := fBody()
		if b.Primary {
			bf = face(fSemiBold, 12.5)
		}
		bw := measure(bf, b.Label) + 30
		bx -= bw
		key := prefix + b.Key
		h, d := u.hl(key), u.pressed(key)
		var fill, txt color.NRGBA
		switch {
		case b.Primary:
			fill, txt = mix(p.green, p.fg, 0.18*h), p.ink
		case b.Danger:
			fill, txt = alpha(p.red, 0.10+0.10*h), p.red
		default:
			fill, txt = alpha(p.fg, 0.08+0.07*h), p.fg
		}
		if d > 0 {
			fill = mix(fill, p.fg, 0.25*d)
		}
		in := float32(1.5 * d)
		u.dc.roundRect(bx+in, y+in, bw-2*in, btnH-2*in, btnH/2, fill)
		u.dc.text(bf, b.Label, bx+15, y+btnH/2+4.5, txt)
		u.hits = append(u.hits, hit{image.Rect(int(bx), int(y), int(bx+bw), int(y+btnH)), key})
		bx -= 8
	}
	if footer != "" {
		key := prefix + "footer"
		col := mix(p.muted, p.fg, u.hl(key))
		fw := u.dc.text(fCaption(), footer, x+pad, y+btnH/2+4, col)
		u.hits = append(u.hits, hit{image.Rect(int(x+pad-4), int(y), int(x+pad+fw+4), int(y+btnH)), key})
	}
}

// ---- decision cards ----

func (u *UI) cardMeta(c *Card) string {
	meta := c.Meta
	if n := len(u.cards) - 1; n > 0 { // more requests are queued behind this one
		meta = strings.TrimPrefix(fmt.Sprintf("%s · %d more", meta, n), " · ")
	}
	return meta
}

func (u *UI) cardTitle(c *Card) []string {
	metaW := min(measure(fCaption(), u.cardMeta(c)), 150)
	return wrap(fTitle(), c.Title, cardW-2*pad-30-metaW-12, 2)
}

func codeLines(code string, w float32) []string {
	f := fMonoS()
	var out []string
	for _, ln := range strings.Split(strings.TrimSpace(code), "\n") {
		out = append(out, ellipsize(f, ln, w))
		if len(out) == 4 {
			break
		}
	}
	return out
}

func (u *UI) cardHeight(c *Card) int {
	if c == nil {
		return 60
	}
	inner := float32(cardW - 2*pad)
	h := 16 + 20*len(u.cardTitle(c)) + 4
	if c.Body != "" {
		h += 4 + 18*len(wrap(fBodyReg(), c.Body, inner, 3))
	}
	if c.Code != "" {
		h += 10 + 18 + 16*len(codeLines(c.Code, inner-20))
	}
	if len(c.Options) > 0 {
		h += 10 + len(c.Options)*(optH+rowGap) - rowGap
	}
	if len(c.Buttons) > 0 || c.Footer != "" {
		h += 14 + btnH
	}
	return min(h+pad, u.H-14)
}

func (u *UI) drawCard(c *Card, x, w float32) {
	if c == nil {
		return
	}
	p := u.pal
	u.spot = emberSpot{x: x + pad + 9, y: 25, s: 18, mood: c.Mood, show: true}
	inner := w - 2*pad
	meta := u.cardMeta(c)
	closable := c.Kind == "edge"
	right := x + w - pad
	if closable {
		cx, cy := right-7, float32(25)
		key := "close:" + c.ID
		h := u.hl(key)
		if h > 0.01 {
			u.dc.circle(cx, cy, 11, alpha(p.fg, 0.1*h))
		}
		col := mix(p.faint, p.fg, h)
		u.dc.line(pt{cx - 3.5, cy - 3.5}, pt{cx + 3.5, cy + 3.5}, 1.4, col)
		u.dc.line(pt{cx + 3.5, cy - 3.5}, pt{cx - 3.5, cy + 3.5}, 1.4, col)
		u.hits = append(u.hits, hit{image.Rect(int(cx-12), int(cy-12), int(cx+12), int(cy+12)), key})
		right -= 24
	}
	if meta != "" {
		mm := ellipsize(fCaption(), meta, 150)
		u.dc.text(fCaption(), mm, right-measure(fCaption(), mm), 29.5, p.muted)
	}
	y := float32(16)
	for _, ln := range u.cardTitle(c) {
		u.dc.text(fTitle(), ln, x+pad+30, y+14, p.fg)
		y += 20
	}
	y += 4
	if c.Body != "" {
		y += 4
		for _, ln := range wrap(fBodyReg(), c.Body, inner, 3) {
			u.dc.text(fBodyReg(), ln, x+pad, y+13, mix(p.fg, p.bg, 0.2))
			y += 18
		}
	}
	if c.Code != "" {
		lines := codeLines(c.Code, inner-20)
		bh := float32(18 + 16*len(lines))
		y += 10
		u.dc.roundRect(x+pad, y, inner, bh, 10, alpha(p.fg, 0.06))
		shell := strings.HasPrefix(c.Title, "Run ")
		for i, ln := range lines {
			by := y + 9 + 12 + float32(i)*16
			if !shell {
				u.dc.text(fMonoS(), ln, x+pad+10, by, p.fg)
				continue
			}
			cx := x + pad + 10
			for _, r := range shellTokens(p, ln) {
				cx += u.dc.text(fMonoS(), r.s, cx, by, r.col)
			}
		}
		y += bh
	}
	if len(c.Options) > 0 {
		y += 10
		for i, o := range c.Options {
			key := fmt.Sprintf("opt:%d", i)
			h, d := u.hl(key), u.pressed(key)
			picked := c.picked[i]
			bg := alpha(p.fg, 0.05+0.06*h+0.06*d)
			if picked {
				bg = alpha(p.green, 0.16)
			}
			u.dc.roundRect(x+pad, y, inner, optH, 11, bg)
			bx, by := x+pad+18, y+optH/2
			if c.Multi {
				u.dc.frame(bx-7, by-7, 14, 14, 4.5, 1.3, alpha(p.fg, 0.4))
				if picked {
					u.dc.roundRect(bx-7, by-7, 14, 14, 4.5, p.green)
					u.dc.icon("check", bx, by, 11, p.ink)
				}
			} else { // the number you can press: `ember answer opt:N`
				u.dc.circle(bx, by, 9, alpha(p.fg, 0.08+0.08*h))
				n := fmt.Sprint(i + 1)
				nf := face(fSemiBold, 10.5)
				u.dc.text(nf, n, bx-measure(nf, n)/2+0.5, by+3.8, mix(p.muted, p.fg, h))
			}
			lx := bx + 18
			lw := u.dc.text(fBody(), ellipsize(fBody(), o.Label, inner*0.5), lx, by+4.5, p.fg)
			if o.Desc != "" {
				u.dc.text(fCaption(), ellipsize(fCaption(), o.Desc, x+pad+inner-12-(lx+lw+10)), lx+lw+10, by+4, p.muted)
			}
			u.hits = append(u.hits, hit{image.Rect(int(x+pad), int(y), int(x+pad+inner), int(y+optH)), key})
			y += optH + rowGap
		}
		y -= rowGap
	}
	if len(c.Buttons) > 0 || c.Footer != "" {
		u.buttonRow(c.Buttons, "btn:", x, w, y+14, c.Footer)
		y += 14 + btnH
	}
	// how long it will wait for you: a hairline that drains along the bottom
	if !c.Expires.IsZero() && !c.Born.IsZero() {
		total := c.Expires.Sub(c.Born).Seconds()
		bottom := float32(u.cardHeight(c)) - 2.5
		lx, lw := x+radCard, w-2*radCard
		u.lives = append(u.lives, live{image.Rect(int(lx)-2, int(bottom)-3, int(lx+lw)+2, int(bottom)+3), func(cv *canvas, _ float64) {
			left := c.Expires.Sub(u.now()).Seconds()
			k := float32(math.Max(0, math.Min(1, left/total)))
			col := alpha(p.fg, 0.22)
			if left < 15 {
				col = alpha(p.yellow, 0.8)
			}
			cv.roundRect(lx+lw*(1-k)/2, bottom-1, lw*k, 2, 1, col)
		}})
	}
}
