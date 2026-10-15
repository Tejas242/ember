package island

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"
)

// Rows of the open card (music, edge), buttons, and the decision cards:
// permission requests, questions, edge's nudges. The card's brief (headline
// and story) is in brief.go.

const (
	cardW   = 380
	expW    = 380
	musicH  = 52
	edgeH   = 34
	rowGapX = 6
	btnH    = 26
	optH    = 32
)

// rowHot reports how much the pointer is on a section (eased, 0..1). The
// section registers its own hit area, under its controls: moving onto it
// redraws.
func (u *UI) rowHot(key string, x, w, y, h float32) float64 {
	r := image.Rect(int(x+pad-8), int(y), int(x+w-pad+8), int(y+h))
	u.hits = append(u.hits, hit{r, "row:" + key})
	on := 0.0
	if (u.hover && (image.Point{int(u.mx), int(u.my)}).In(r)) || (u.drag != "" && strings.HasPrefix(u.drag, key)) {
		on = 1
	}
	return u.tw("row:"+key, on, 0)
}

// ---- music ----

// musicRow: the cover on the rail, the song beside it, its progress along
// the foot. The cover lights the section softly in its own colour; under the
// pointer the controls replace the equaliser and the line thickens.
func (u *UI) musicRow(x, w, y float32) {
	p, t := u.pal, u.track
	acc := u.accent(p.fg)
	on := u.rowHot("music", x, w, y, musicH)
	const art = 28
	ax, ay := x+railX-art/2, y+9
	if u.art != nil {
		if u.art.accent.A > 0 { // the cover's light on the island
			u.cv.glowInside(image.Rect(int(x+2), int(y-6), int(x+w/2), int(y+musicH+6)), ax+art/2, ay+art/2, 96,
				u.art.accent, (0.13+0.05*on)*u.fadeK*u.rowK)
		}
		img := u.art.row
		u.dc.drawArt(img, int(ax+art/2)-img.Rect.Dx()/2, int(ay+art/2)-img.Rect.Dy()/2, 7, 1)
	} else {
		u.dc.roundRect(ax, ay, art, art, 7, alpha(p.fg, 0.08))
		u.dc.icon("volume", ax+art/2, ay+art/2, 13, p.muted)
	}
	tx := x + textX
	right := x + w - pad
	inner := right - tx - 8 - float32(44+56*on)
	u.dc.text(fBody(), ellipsize(fBody(), t.Title, inner), tx, y+21, p.fg)
	u.dc.text(fSecond(), ellipsize(fSecond(), t.Artist, inner), tx, y+36, p.muted)
	if on < 0.99 {
		k := 1 - on
		if t.Playing {
			u.eqLive(right-14, y+21, 9, alpha(acc, k), true)
		}
		if t.Length > 0 { // time left ticks by itself: a live part, not baked into the frame
			ty := y + 36
			r := image.Rect(int(right-48), int(ty-11), int(right)+2, int(ty+4))
			u.lives = append(u.lives, live{r, func(c *canvas, _ float64) {
				left := "−" + clock(max(t.Length-t.pos(u.now()), 0))
				c.text(fCaption(), left, right-measure(fCaption(), left), ty, alpha(p.faint, k))
			}})
		}
	}
	if on > 0.01 {
		cy := y + 24
		for i, k := range []string{"prev", "toggle", "next"} {
			bx := right - 13 - float32(2-i)*32
			key := "music:" + k
			h, d := float32(u.hl(key)), float32(u.pressed(key))
			if k == "toggle" {
				u.dc.circle(bx, cy, (13-1.5*d)*float32(0.85+0.15*on), alpha(mix(p.fg, acc, 0.2), on))
				u.mediaGlyph(k, bx, cy, t.Playing, alpha(p.ink, on), 0.66)
			} else {
				if h > 0.01 {
					u.dc.circle(bx, cy, 12-d, alpha(p.fg, 0.1*float64(h)*on))
				}
				u.mediaGlyph(k, bx, cy, t.Playing, alpha(mix(p.muted, p.fg, 0.5+0.5*float64(h)), on), 0.66)
			}
			u.hits = append(u.hits, hit{image.Rect(int(bx-14), int(cy-14), int(bx+14), int(cy+14)), key})
		}
	}
	if t.Length > 0 { // the song, along the foot: thickens and drags under the pointer
		lx, lw := tx, right-tx
		ly := y + musicH - 6
		u.seekRect = image.Rect(int(lx), int(ly-5), int(lx+lw), int(ly+5))
		h := float32(u.hl("music:seek"))
		u.progressLive(lx, ly, lw, 2+1.5*h, acc, h)
		u.hits = append(u.hits, hit{u.seekRect, "music:seek"})
	}
}

// ---- edge ----

// edgeRowShown: edge's row is there whenever the plugin runs: it's the way
// to its dashboard.
func (u *UI) edgeRowShown() bool {
	return u.cfg.Edge && (u.edge != nil || u.edgeStats != nil || u.dashboards["edge"] != nil)
}

// edgeRow: edge on one line: its target on the rail, the task or the day,
// and its button when it has one. The line opens edge's dashboard.
func (u *UI) edgeRow(x, w, y float32) {
	p, now, e := u.pal, u.now(), u.edge
	if e == nil { // before edge's first status: what its stats say
		e = &EdgeStatus{}
		if st := u.edgeStats; st != nil {
			e.Today, e.Need, e.Streak, e.Closed = st.Today.Got, st.Today.Need, st.Streak.Current, st.Today.Gate == "closed"
		}
	}
	on := u.rowHot("edge", x, w, y, edgeH)
	if u.dashboards["edge"] != nil { // the line opens edge's dashboard; its button still acts
		if on > 0.01 {
			u.panel(x+pad-8, y+2, w-2*pad+16, edgeH-4, radRow, alpha(p.fg, 0.05*on))
		}
		u.hits = append(u.hits, hit{image.Rect(int(x+pad-8), int(y), int(x+w-pad+8), int(y+edgeH)), "dash:edge"})
	}
	cy := y + edgeH/2
	gx := x + railX
	u.dc.frame(gx-6.5, cy-6.5, 13, 13, 6.5, 1.3, p.yellow)
	u.dc.circle(gx, cy, 2.3, p.yellow)
	tx, right := x+textX, x+w-pad
	var btn Button
	sub := fmt.Sprintf("%d/%d today · %d-day streak", e.Today, e.Need, e.Streak)
	switch {
	case e.StartedMs > 0 && !e.Pending:
		sub = "Practising · " + mins(now.Sub(time.UnixMilli(e.StartedMs)))
		btn = Button{Key: "hint", Label: "Hint", Primary: true}
	case e.Task != "":
		btn = Button{Key: "start", Label: "Start", Primary: true}
		if e.Closed {
			sub = "Claude is locked until you practise"
		}
	}
	if btn.Key != "" {
		bw := u.pillButton(btn, "btn:edge:"+btn.Key, right, cy-11, 22, p.yellow)
		right -= bw + 10
	} else if u.dashboards["edge"] != nil { // the line opens the dashboard: say so
		u.dc.icon("chart", right-6, cy, 12, mix(p.faint, p.yellow, 0.3+0.7*on))
		right -= 20
	}
	task := e.Task
	if task == "" {
		task = "Practice"
	}
	base := cy + 4.5
	nw := u.dc.text(fBody(), ellipsize(fBody(), task, (right-tx)*0.55), tx, base, p.fg)
	u.dc.text(fSecond(), ellipsize(fSecond(), sub, right-tx-nw-8), tx+nw+8, base, p.muted)
}

// pillButton draws a small round button right-aligned at right, and
// reports its width.
func (u *UI) pillButton(b Button, key string, right, y, h float32, tint color.NRGBA) float32 {
	p := u.pal
	f := face(fSemiBold, 11.5)
	bw := measure(f, b.Label) + 22
	x := right - bw
	k, d := u.hl(key), u.pressed(key)
	fill, txt := alpha(tint, 0.16+0.1*k), mix(tint, p.fg, 0.2)
	if b.Primary {
		fill, txt = mix(tint, colorWhite, 0.12*k+0.2*d), p.ink
	}
	in := float32(1.2 * d)
	u.dc.roundRect(x+in, y+in, bw-2*in, h-2*in, h/2, fill)
	u.dc.text(f, b.Label, x+11, y+h/2+4, txt)
	u.hits = append(u.hits, hit{image.Rect(int(x), int(y), int(x+bw), int(y+h)), key})
	return bw
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
	c.poly(alpha(col, 0.25), ps...)
	for i := 1; i < n; i++ { // the top edge, over the fill
		c.line(ps[i], ps[i+1], 1.1, col)
	}
}

// ---- buttons ----

// buttonRow draws right-aligned round buttons, and an optional quiet link on
// the left. The primary one is filled in the card's colour; hover and press
// ease.
func (u *UI) buttonRow(btns []Button, prefix string, x, w, y float32, footer string, primary color.NRGBA) {
	p := u.pal
	bx := x + w - pad
	for i := len(btns) - 1; i >= 0; i-- {
		b := btns[i]
		bf := face(fMedium, 12)
		if b.Primary {
			bf = face(fSemiBold, 12)
		}
		bw := measure(bf, b.Label) + 28
		bx -= bw
		key := prefix + b.Key
		h, d := u.hl(key), u.pressed(key)
		var fill, txt color.NRGBA
		switch {
		case b.Primary:
			fill, txt = mix(primary, colorWhite, 0.14*h), p.ink
		case b.Danger:
			fill, txt = alpha(p.red, 0.09+0.09*h), mix(p.red, p.fg, 0.1)
		default:
			fill, txt = alpha(p.fg, 0.07+0.06*h), p.fg
		}
		if d > 0 {
			fill = mix(fill, colorWhite, 0.2*d)
		}
		in := float32(1.5 * d)
		u.dc.roundRect(bx+in, y+in, bw-2*in, btnH-2*in, btnH/2, fill)
		u.dc.text(bf, b.Label, bx+14, y+btnH/2+4.2, txt)
		u.hits = append(u.hits, hit{image.Rect(int(bx), int(y), int(bx+bw), int(y+btnH)), key})
		bx -= 6
	}
	if footer != "" {
		key := prefix + "footer"
		col := mix(p.faint, p.fg, 0.2+0.6*u.hl(key))
		fw := u.dc.text(fCaption(), footer, x+pad, y+btnH/2+4, col)
		u.hits = append(u.hits, hit{image.Rect(int(x+pad-4), int(y), int(x+pad+fw+4), int(y+btnH)), key})
	}
}

// ---- decision cards ----

// cardMeta is the quiet note at the eyebrow's right: a question's place in
// its series, and how many more requests wait behind this one.
func (u *UI) cardMeta(c *Card) string {
	meta := c.Meta
	if n := len(u.cards) - 1; n > 0 { // more requests are queued behind this one
		meta = strings.TrimPrefix(fmt.Sprintf("%s · %d more", meta, n), " · ")
	}
	return meta
}

// eyebrowOf says who wants what: "kinvault wants to run", "edge".
func eyebrowOf(c *Card) string {
	if c.Eyebrow != "" {
		return c.Eyebrow
	}
	switch c.Kind {
	case "ask":
		return "Question"
	case "edge":
		return "edge"
	}
	return "Permission"
}

func (u *UI) cardTitle(c *Card) []string {
	return wrap(fTitle(), c.Title, cardW-textX-pad, 2)
}

func codeLines(code string, w float32) []string {
	f := fMonoS()
	var out []string
	for _, ln := range strings.Split(strings.TrimSpace(code), "\n") {
		out = append(out, ellipsize(f, ln, w))
		if len(out) == 3 {
			break
		}
	}
	return out
}

const (
	cardTitleTop = 25 // the title's first line box
	titleLineH   = 19
)

func (u *UI) cardHeight(c *Card) int {
	if c == nil {
		return 60
	}
	inner := float32(cardW - 2*pad)
	h := cardTitleTop + titleLineH*len(u.cardTitle(c))
	if c.Body != "" {
		h += 2 + 15*len(wrap(fCaption(), c.Body, cardW-textX-pad, 2))
	}
	if c.Code != "" {
		h += 10 + 12 + 15*len(codeLines(c.Code, inner-20))
	}
	if len(c.Options) > 0 {
		h += 10 + len(c.Options)*optH
	}
	if len(c.Buttons) > 0 {
		h += 12 + btnH
	}
	return min(h+14, u.H-14)
}

// primaryOf is a card's action colour: blue when Claude waits on you.
func (u *UI) primaryOf(c *Card) color.NRGBA {
	switch c.Mood {
	case moodWait:
		return u.pal.blue
	case moodNudge:
		return u.pal.yellow
	}
	return u.pal.green
}

func (u *UI) drawCard(c *Card, x, w float32) {
	if c == nil {
		return
	}
	p := u.pal
	u.spot = emberSpot{x: x + railX, y: 28, s: 16, mood: c.Mood, show: true}
	inner := w - 2*pad
	right := x + w - pad
	tx := x + textX
	i := 0
	// the eyebrow: who wants what; the meta and, for edge's notes, a close
	u.item("c:eyebrow", i, x, w, 0, cardTitleTop, func(y float32) {
		r := right
		if c.Kind == "edge" { // edge's notes can be closed
			cx, cy := r-5, y+15
			key := "close:" + c.ID
			h := u.hl(key)
			if h > 0.01 {
				u.dc.circle(cx, cy, 10, alpha(p.fg, 0.1*h))
			}
			col := mix(p.faint, p.fg, h)
			u.dc.line(pt{cx - 3, cy - 3}, pt{cx + 3, cy + 3}, 1.3, col)
			u.dc.line(pt{cx + 3, cy - 3}, pt{cx - 3, cy + 3}, 1.3, col)
			u.hits = append(u.hits, hit{image.Rect(int(cx-11), int(cy-11), int(cx+11), int(cy+11)), key})
			r -= 22
		}
		if len(c.Buttons) == 0 && c.Footer != "" { // no buttons: "In terminal" sits up here
			key := "btn:footer"
			fw := measure(fCaption(), c.Footer)
			u.dc.text(fCaption(), c.Footer, r-fw, y+19, mix(p.faint, p.fg, 0.2+0.6*u.hl(key)))
			u.hits = append(u.hits, hit{image.Rect(int(r-fw-4), int(y+6), int(r+4), int(y+26)), key})
			r -= fw + 12
		}
		if meta := u.cardMeta(c); meta != "" {
			mm := ellipsize(fCaption(), meta, 140)
			mw := measure(fCaption(), mm)
			u.dc.text(fCaption(), mm, r-mw, y+19, p.faint)
			r -= mw + 10
		}
		eb := eyebrowOf(c)
		col := mix(p.moodColor(c.Mood), p.fg, 0.35)
		if c.Mood == moodIdle {
			col = p.faint
		}
		for eyebrowW(eb) > r-tx && len(eb) > 4 {
			eb = strings.TrimSuffix(eb[:len(eb)-2], " ") + "…"
		}
		u.dc.eyebrow(eb, tx, y+18.5, col)
	})
	i++
	y := float32(cardTitleTop)
	lines := u.cardTitle(c)
	th := float32(titleLineH * len(lines))
	u.item("c:title", i, x, w, y, th, func(y float32) {
		for j, ln := range lines {
			u.dc.text(fTitle(), ln, tx, y+14+float32(j)*titleLineH, p.fg)
		}
	})
	i++
	y += th
	if c.Body != "" {
		body := wrap(fCaption(), c.Body, right-tx, 2)
		bh := float32(2 + 15*len(body))
		u.item("c:body", i, x, w, y, bh, func(y float32) {
			for j, ln := range body {
				u.facts(ln, tx, y+13+float32(j)*15, right-tx, p.muted) // a diff in its colours
			}
		})
		y += bh
		i++
	}
	if c.Code != "" {
		code := codeLines(c.Code, inner-20)
		bh := float32(12 + 15*len(code))
		y += 10
		u.item("c:code", i, x, w, y, bh, func(y float32) {
			u.panel(x+pad, y, inner, bh, radRow, alpha(p.fg, 0.055))
			for j, ln := range code {
				by := y + 6 + 11 + float32(j)*15
				if !c.Shell {
					u.dc.text(fMonoS(), ln, x+pad+10, by, mix(p.fg, p.bg, 0.12))
					continue
				}
				cx := x + pad + 10
				for _, r := range shellTokens(p, ln) {
					cx += u.dc.text(fMonoS(), r.s, cx, by, r.col)
				}
			}
		})
		y += bh
		i++
	}
	if len(c.Options) > 0 { // one grouped list, hairlines between, a number to press
		y += 10
		u.panel(x+pad, y, inner, float32(len(c.Options))*optH, radRow, alpha(p.fg, 0.045))
		for j, o := range c.Options {
			key := fmt.Sprintf("opt:%d", j)
			u.item("c:"+key, i, x, w, y, optH, func(y float32) {
				h, d := u.hl(key), u.pressed(key)
				picked := c.picked[j]
				if picked {
					u.panel(x+pad, y, inner, optH, radRow, alpha(u.primaryOf(c), 0.16))
				} else if h > 0.01 || d > 0 {
					u.panel(x+pad, y, inner, optH, radRow, alpha(p.fg, 0.05*h+0.06*d))
				}
				if j > 0 && !picked && !c.picked[j-1] {
					u.dc.roundRect(x+pad+32, y, inner-32-10, 1, 0.5, p.hair())
				}
				bx, by := x+pad+16, y+optH/2
				if c.Multi {
					u.dc.frame(bx-6.5, by-6.5, 13, 13, 4, 1.3, alpha(p.fg, 0.35+0.2*h))
					if picked {
						u.dc.roundRect(bx-6.5, by-6.5, 13, 13, 4, u.primaryOf(c))
						u.dc.icon("check", bx, by, 10, p.ink)
					}
				} else { // the number you can press: `ember answer opt:N`
					n := fmt.Sprint(j + 1)
					nf := face(fSemiBold, 10)
					u.dc.circle(bx, by, 8, alpha(p.fg, 0.07+0.1*h))
					u.dc.text(nf, n, bx-measure(nf, n)/2+0.5, by+3.6, mix(p.muted, p.fg, h))
				}
				lx := bx + 16
				lw := u.dc.text(fBody(), ellipsize(fBody(), o.Label, inner*0.5), lx, by+4.5, p.fg)
				if o.Desc != "" {
					u.dc.text(fCaption(), ellipsize(fCaption(), o.Desc, x+pad+inner-12-(lx+lw+10)), lx+lw+10, by+4, p.muted)
				}
				u.hits = append(u.hits, hit{image.Rect(int(x+pad), int(y), int(x+pad+inner), int(y+optH)), key})
			})
			y += optH
			i++
		}
	}
	if len(c.Buttons) > 0 {
		y += 12
		u.item("c:buttons", i, x, w, y, btnH, func(y float32) {
			u.buttonRow(c.Buttons, "btn:", x, w, y, c.Footer, u.primaryOf(c))
		})
	}
	// how long it will wait for you: a hairline that drains along the bottom
	if !c.Expires.IsZero() && !c.Born.IsZero() {
		total := c.Expires.Sub(c.Born).Seconds()
		bottom := float32(u.cardHeight(c)) - 2.5
		lx, lw := x+radCard, w-2*radCard
		u.lives = append(u.lives, live{image.Rect(int(lx)-2, int(bottom)-3, int(lx+lw)+2, int(bottom)+3), func(cv *canvas, _ float64) {
			left := c.Expires.Sub(u.now()).Seconds()
			k := float32(math.Max(0, math.Min(1, left/total)))
			col := alpha(p.fg, 0.2)
			if left < 15 {
				col = alpha(p.yellow, 0.8)
			}
			cv.roundRect(lx+lw*(1-k)/2, bottom-1, lw*k, 2, 1, col)
		}})
	}
}
