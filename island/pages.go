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
	cardW     = 380
	expW      = 380
	musicRowH = 44
	edgeRowH  = 40
	rowGapX   = 6
	btnH      = 26
	optH      = 30
)

// rowBox draws a row's rounded background and reports how much the
// pointer is on it (eased, 0..1).
func (u *UI) rowBox(key string, x, w, y, h float32, tint color.NRGBA, strength float64) float64 {
	r := image.Rect(int(x+pad-6), int(y), int(x+w-pad+6), int(y+h))
	if !strings.Contains(key, ":") { // its own hit area, under its controls: moving onto it redraws
		u.hits = append(u.hits, hit{r, "row:" + key})
	}
	on := 0.0
	if (u.hover && (image.Point{int(u.mx), int(u.my)}).In(r)) || (u.drag != "" && strings.HasPrefix(u.drag, key)) {
		on = 1
	}
	k := u.tw("row:"+key, on, 0)
	u.panel(x+pad-6, y, w-2*pad+12, h, 13, alpha(tint, strength+0.04*k))
	return k
}

// ---- music ----

func (u *UI) musicRow(x, w, y float32) {
	p, t := u.pal, u.track
	acc := u.accent(p.fg)
	on := u.rowBox("music", x, w, y, musicRowH, acc, 0.06)
	const art = 30
	ax, ay := x+pad, y+(musicRowH-art)/2-1
	if u.art != nil {
		u.dc.drawArt(u.art.row, int(ax), int(ay), 8, 1)
	} else {
		u.dc.roundRect(ax, ay, art, art, 8, alpha(p.fg, 0.08))
		u.dc.icon("volume", ax+art/2, ay+art/2, 14, p.muted)
	}
	tx := ax + art + 10
	right := x + w - pad
	// the right end: equaliser and time left, or the controls under your pointer
	inner := right - tx - 8 - float32(52+44*on)
	u.dc.text(fBody(), ellipsize(fBody(), t.Title, inner), tx, y+18, p.fg)
	u.dc.text(fCaption(), ellipsize(fCaption(), t.Artist, inner), tx, y+31, p.muted)
	if on < 0.99 {
		k := 1 - on
		if t.Playing {
			u.eqLive(right-16, y+19, 9, alpha(acc, k), true)
		}
		if t.Length > 0 { // time left ticks by itself: a live part, not baked into the frame
			ty := y + 32
			r := image.Rect(int(right-48), int(ty-11), int(right)+2, int(ty+4))
			u.lives = append(u.lives, live{r, func(c *canvas, _ float64) {
				left := "−" + clock(max(t.Length-t.pos(u.now()), 0))
				c.text(fCaption(), left, right-measure(fCaption(), left), ty, alpha(p.faint, k))
			}})
		}
	}
	if on > 0.01 {
		cy := y + 20
		for i, k := range []string{"prev", "toggle", "next"} {
			bx := right - 13 - float32(2-i)*31
			key := "music:" + k
			h, d := float32(u.hl(key)), float32(u.pressed(key))
			if k == "toggle" {
				u.dc.circle(bx, cy, 13-1.5*d, alpha(mix(p.fg, acc, 0.25), on))
				u.mediaGlyph(k, bx, cy, t.Playing, alpha(p.ink, on), 0.7)
			} else {
				if h > 0.01 {
					u.dc.circle(bx, cy, 12-d, alpha(p.fg, 0.1*float64(h)*on))
				}
				u.mediaGlyph(k, bx, cy, t.Playing, alpha(mix(p.muted, p.fg, 0.5+0.5*float64(h)), on), 0.7)
			}
			u.hits = append(u.hits, hit{image.Rect(int(bx-14), int(cy-14), int(bx+14), int(cy+14)), key})
		}
	}
	// the song, as a line along the row's foot: thickens and drags under the pointer
	if t.Length > 0 {
		lx, lw := tx, right-tx
		ly := y + musicRowH - 5
		u.seekRect = image.Rect(int(lx), int(ly-5), int(lx+lw), int(ly+5))
		h := float32(u.hl("music:seek"))
		u.progressLive(lx, ly, lw, 2+2*h, acc, h)
		u.hits = append(u.hits, hit{u.seekRect, "music:seek"})
	}
}

// ---- edge ----

func (u *UI) edgeRow(x, w, y float32) {
	p, now, e := u.pal, u.now(), u.edge
	u.rowBox("edge", x, w, y, edgeRowH, p.yellow, 0.06)
	u.dc.frame(x+pad, y+edgeRowH/2-7, 14, 14, 7, 1.4, p.yellow)
	u.dc.circle(x+pad+7, y+edgeRowH/2, 2.5, p.yellow)
	tx, right := x+pad+22, x+w-pad
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
		u.buttonRow([]Button{btn}, "btn:edge:", x, w, y+(edgeRowH-btnH)/2, "")
		right -= measure(face(fSemiBold, 12), btn.Label) + 26 + 10
	}
	task := e.Task
	if task == "" {
		task = "Practice"
	}
	u.dc.text(fBody(), ellipsize(fBody(), task, right-tx), tx, y+16, p.fg)
	u.dc.text(fCaption(), ellipsize(fCaption(), sub, right-tx), tx, y+30.5, p.muted)
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

// buttonRow draws right-aligned pill buttons, and an optional quiet link on
// the left. Hover and press ease.
func (u *UI) buttonRow(btns []Button, prefix string, x, w, y float32, footer string) {
	p := u.pal
	bx := x + w - pad
	for i := len(btns) - 1; i >= 0; i-- {
		b := btns[i]
		bf := face(fMedium, 12)
		if b.Primary {
			bf = face(fSemiBold, 12)
		}
		bw := measure(bf, b.Label) + 26
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
		u.dc.text(bf, b.Label, bx+13, y+btnH/2+4.2, txt)
		u.hits = append(u.hits, hit{image.Rect(int(bx), int(y), int(bx+bw), int(y+btnH)), key})
		bx -= 6
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

// answerHere: a single-choice question has no buttons, so "In terminal"
// sits up top beside the meta instead of taking a row.
func answerHere(c *Card) bool { return len(c.Options) > 0 && !c.Multi && c.Footer != "" }

func (u *UI) cardTitle(c *Card) []string {
	side := min(measure(fCaption(), u.cardMeta(c)), 140)
	if answerHere(c) {
		side += measure(fCaption(), c.Footer) + 12
	}
	return wrap(fTitle(), c.Title, cardW-2*pad-24-side-10, 2)
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

func (u *UI) cardHeight(c *Card) int {
	if c == nil {
		return 60
	}
	inner := float32(cardW - 2*pad)
	h := 12 + 19*len(u.cardTitle(c))
	if c.Body != "" {
		h += 2 + 15*len(wrap(fCaption(), c.Body, inner-24, 2))
	}
	if c.Code != "" {
		h += 8 + 10 + 15*len(codeLines(c.Code, inner-16))
	}
	if len(c.Options) > 0 {
		h += 8 + len(c.Options)*(optH+4) - 4
	}
	if len(c.Buttons) > 0 || (c.Footer != "" && !answerHere(c)) {
		h += 10 + btnH
	}
	return min(h+12, u.H-14)
}

func (u *UI) drawCard(c *Card, x, w float32) {
	if c == nil {
		return
	}
	p := u.pal
	u.spot = emberSpot{x: x + pad + 7, y: 21, s: 15, mood: c.Mood, show: true}
	inner := w - 2*pad
	right := x + w - pad
	if c.Kind == "edge" { // edge's notes can be closed
		cx, cy := right-6, float32(21)
		key := "close:" + c.ID
		h := u.hl(key)
		if h > 0.01 {
			u.dc.circle(cx, cy, 10, alpha(p.fg, 0.1*h))
		}
		col := mix(p.faint, p.fg, h)
		u.dc.line(pt{cx - 3, cy - 3}, pt{cx + 3, cy + 3}, 1.3, col)
		u.dc.line(pt{cx + 3, cy - 3}, pt{cx - 3, cy + 3}, 1.3, col)
		u.hits = append(u.hits, hit{image.Rect(int(cx-11), int(cy-11), int(cx+11), int(cy+11)), key})
		right -= 22
	}
	if answerHere(c) {
		key := "btn:footer"
		fw := measure(fCaption(), c.Footer)
		u.dc.text(fCaption(), c.Footer, right-fw, 25.5, mix(p.muted, p.fg, u.hl(key)))
		u.hits = append(u.hits, hit{image.Rect(int(right-fw-4), 10, int(right+4), 32), key})
		right -= fw + 12
	}
	if meta := u.cardMeta(c); meta != "" {
		mm := ellipsize(fCaption(), meta, 140)
		u.dc.text(fCaption(), mm, right-measure(fCaption(), mm), 25.5, p.faint)
	}
	y := float32(12)
	for _, ln := range u.cardTitle(c) {
		u.dc.text(fTitle(), ln, x+pad+24, y+14, p.fg)
		y += 19
	}
	if c.Body != "" {
		y += 2
		for _, ln := range wrap(fCaption(), c.Body, inner-24, 2) {
			u.dc.text(fCaption(), ln, x+pad+24, y+11, p.muted)
			y += 15
		}
	}
	if c.Code != "" {
		lines := codeLines(c.Code, inner-16)
		bh := float32(10 + 15*len(lines))
		y += 8
		u.panel(x+pad, y, inner, bh, 9, alpha(p.fg, 0.06))
		for i, ln := range lines {
			by := y + 5 + 11 + float32(i)*15
			if !c.Shell {
				u.dc.text(fMonoS(), ln, x+pad+8, by, p.fg)
				continue
			}
			cx := x + pad + 8
			for _, r := range shellTokens(p, ln) {
				cx += u.dc.text(fMonoS(), r.s, cx, by, r.col)
			}
		}
		y += bh
	}
	if len(c.Options) > 0 {
		y += 8
		for i, o := range c.Options {
			key := fmt.Sprintf("opt:%d", i)
			h, d := u.hl(key), u.pressed(key)
			picked := c.picked[i]
			bg := alpha(p.fg, 0.05+0.06*h+0.06*d)
			if picked {
				bg = alpha(p.green, 0.16)
			}
			u.panel(x+pad, y, inner, optH, 10, bg)
			bx, by := x+pad+15, y+optH/2
			if c.Multi {
				u.dc.frame(bx-6.5, by-6.5, 13, 13, 4, 1.3, alpha(p.fg, 0.4))
				if picked {
					u.dc.roundRect(bx-6.5, by-6.5, 13, 13, 4, p.green)
					u.dc.icon("check", bx, by, 10, p.ink)
				}
			} else { // the number you can press: `ember answer opt:N`
				n := fmt.Sprint(i + 1)
				nf := face(fSemiBold, 10.5)
				u.dc.circle(bx, by, 8, alpha(p.fg, 0.08+0.1*h))
				u.dc.text(nf, n, bx-measure(nf, n)/2+0.5, by+3.8, mix(p.muted, p.fg, h))
			}
			lx := bx + 16
			lw := u.dc.text(fBody(), ellipsize(fBody(), o.Label, inner*0.5), lx, by+4.5, p.fg)
			if o.Desc != "" {
				u.dc.text(fCaption(), ellipsize(fCaption(), o.Desc, x+pad+inner-10-(lx+lw+9)), lx+lw+9, by+4, p.muted)
			}
			u.hits = append(u.hits, hit{image.Rect(int(x+pad), int(y), int(x+pad+inner), int(y+optH)), key})
			y += optH + 4
		}
		y -= 4
	}
	if len(c.Buttons) > 0 || (c.Footer != "" && !answerHere(c)) {
		footer := c.Footer
		if answerHere(c) {
			footer = ""
		}
		u.buttonRow(c.Buttons, "btn:", x, w, y+10, footer)
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
