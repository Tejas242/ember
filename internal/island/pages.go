package island

import (
	"fmt"
	xdraw "golang.org/x/image/draw"
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
	musicH  = 70
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

// musicRow: now playing. The cover, a little larger than anything else in
// the card, sits on a haze of its own colours that washes across the
// section; the equaliser rides on its corner. Beside it the song, quiet
// controls that brighten under the pointer, and a scrubber with the time
// at both ends. A new song fades in; pausing dims the cover and morphs the
// button.
func (u *UI) musicRow(x, w, y float32) {
	p, t, now := u.pal, u.track, u.now()
	acc := u.accent(p.fg)
	on := u.rowHot("music", x, w, y, musicH)
	playK := u.tw("music:playing", b2f(t.Playing), b2f(t.Playing))
	if u.art != nil && u.art.wash != nil { // the cover's light across the section
		u.washInside(u.art, image.Rect(int(x+2), int(y-4), int(x+w-2), int(y+musicH+4)), (0.3+0.08*on)*u.fadeK*u.rowK)
	}
	cx, cy0 := x+pad, y+(musicH-coverS)/2
	if u.art != nil { // a new cover arrives: it fades in
		ak := u.tw("art:"+u.art.url, 1, 0)
		u.dc.roundRect(cx-1, cy0+2, coverS+2, coverS+1, 10, alpha(colorBlack, 0.35*ak)) // a soft shadow under it
		u.dc.drawArt(u.art.row, int(cx), int(cy0), 9, ak*(0.55+0.45*playK))
	} else {
		u.dc.roundRect(cx, cy0, coverS, coverS, 9, alpha(p.fg, 0.07))
		u.dc.icon("note", cx+coverS/2, cy0+coverS/2, 18, p.muted)
	}
	// the equaliser on the cover's corner, on a small dark lozenge
	ex, ey := cx+coverS-21, cy0+coverS-15
	u.dc.roundRect(ex, ey, 18, 12, 6, alpha(colorBlack, 0.5))
	u.eqLiveS(ex+3.5, ey+10, 8, 2.2, 4, mix(acc, colorWhite, 0.45), t.Playing)

	tx := cx + coverS + 12
	right := x + w - pad
	// the controls: always there, quiet, brighter under the pointer
	ccy := y + 27
	for i, k := range []string{"prev", "toggle", "next"} {
		bx := right - 12 - float32(2-i)*32
		key := "music:" + k
		h, d := float32(u.hl(key)), float32(u.pressed(key))
		if k == "toggle" { // a disc; pause and play morph into each other
			r := (14 - 1.6*d) * (1 + 0.04*h)
			u.dc.circle(bx, ccy, r, mix(mix(p.fg, acc, 0.15), colorWhite, 0.1*float64(h)))
			gk := 0.6 * (1 - 0.06*d)
			u.mediaGlyph(k, bx, ccy, true, alpha(p.ink, playK), gk*float32(0.7+0.3*playK))
			u.mediaGlyph(k, bx+1, ccy, false, alpha(p.ink, 1-playK), gk*float32(0.7+0.3*(1-playK)))
		} else {
			if h > 0.01 || d > 0 {
				u.dc.circle(bx, ccy, 12-d, alpha(p.fg, 0.1*float64(h)+0.08*float64(d)))
			}
			u.mediaGlyph(k, bx, ccy, t.Playing, mix(p.muted, p.fg, 0.35*float64(on)+0.65*float64(h)), 0.6*(1-0.08*d))
		}
		u.hits = append(u.hits, hit{image.Rect(int(bx-15), int(ccy-15), int(bx+15), int(ccy+15)), key})
	}
	// the song: a new one rises in
	e := u.enter("song:"+t.Title+"\x00"+t.Artist, 0, false)
	ty := rise(e)
	tw := right - 12 - 2*32 - 22 - tx
	u.dc.text(fBody(), ellipsize(fBody(), t.Title, tw), tx, y+23+ty, alpha(p.fg, e))
	u.dc.text(fSecond(), ellipsize(fSecond(), t.Artist, tw), tx, y+38+ty, alpha(p.muted, e))
	// the scrubber, the time at both ends: it thickens and grows a knob under the pointer, and drags
	if t.Length > 0 {
		ly := y + musicH - 13
		f := fCaption()
		tw0 := measure(f, "00:00")
		lx, lw := tx+tw0+6, right-tx-2*tw0-12
		u.seekRect = image.Rect(int(lx), int(ly-6), int(lx+lw), int(ly+6))
		h := float32(u.hl("music:seek"))
		u.progressLive(lx, ly, lw, 3+1.5*h, acc, h)
		u.hits = append(u.hits, hit{u.seekRect, "music:seek"})
		r := image.Rect(int(tx)-1, int(ly-10), int(right)+2, int(ly+5))
		u.lives = append(u.lives, live{r: r, draw: func(c *canvas, _ float64) {
			pos := min(t.Pos(u.now()), t.Length)
			if u.drag == "music:seek" { // dragging: the times follow the knob
				pos = time.Duration(u.seekPreview * float64(t.Length))
			}
			el, left := clock(max(pos, 0)), "−"+clock(max(t.Length-pos, 0))
			c.text(f, el, tx+tw0-measure(f, el), ly+3.5, p.faint)
			c.text(f, left, right-measure(f, left), ly+3.5, p.faint)
		}})
	}
	_ = now
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

var colorBlack = color.NRGBA{0, 0, 0, 255}

// eqLiveS is the equaliser at a given size: bar height h, width bw, pitch.
func (u *UI) eqLiveS(x, y, h, bw, pitch float32, col color.NRGBA, playing bool) {
	r := image.Rect(int(x)-1, int(y-h)-1, int(x+3*pitch)+1, int(y)+1)
	u.eqShown = u.eqShown || playing
	u.lives = append(u.lives, live{r: r, draw: func(c *canvas, t float64) {
		lv, heard := u.audio.Levels()
		for i := 0; i < 3; i++ {
			k := []float64{0.4, 0.75, 0.55}[i]
			switch {
			case playing && heard:
				target := 0.18 + 0.82*lv[i]
				if target > u.eqLevel[i] {
					u.eqLevel[i] += (target - u.eqLevel[i]) * 0.7
				} else {
					u.eqLevel[i] += (target - u.eqLevel[i]) * 0.25
				}
				k = u.eqLevel[i]
			case playing:
				k = 0.3 + 0.7*math.Abs(math.Sin(t*(3.7+float64(i)*1.9)+float64(i)*1.3))*(0.75+0.25*math.Sin(t*1.3+float64(i)))
			}
			bh := float32(k) * h
			cc := col
			if !playing {
				cc = alpha(col, 0.5)
			}
			c.roundRect(x+float32(i)*pitch, y-bh, bw, bh, bw/2, cc)
		}
	}})
}

// washInside lays the cover's haze over the island's body in r: strongest
// behind the cover, fading toward the right and at every edge.
func (u *UI) washInside(a *albumArt, r image.Rectangle, strength float64) {
	r = r.Intersect(u.cv.img.Rect)
	if r.Empty() || strength <= 0 {
		return
	}
	key := fmt.Sprint(a.url, r.Dx(), r.Dy())
	if u.washKey != key { // scaled once a cover and a size
		img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		xdraw.ApproxBiLinear.Scale(img, img.Rect, a.wash, a.wash.Rect, xdraw.Src, nil)
		u.washImg, u.washKey = img, key
	}
	img := u.washImg
	edge := func(d int, span float32) float32 {
		f := min(float32(d)/span, 1)
		return f * f * (3 - 2*f)
	}
	w := float32(r.Dx())
	for yy := 0; yy < r.Dy(); yy++ {
		row := u.cv.img.Pix[(r.Min.Y+yy)*u.cv.img.Stride:]
		src := img.Pix[yy*img.Stride:]
		fy := edge(min(yy, r.Dy()-1-yy), 22) // feathered well in from every edge: light, not a box
		for xx := 0; xx < r.Dx(); xx++ {
			i := (r.Min.X + xx) * 4
			a := row[i+3]
			if a < 180 {
				continue
			}
			fx := (1 - 0.85*float32(xx)/w) * edge(min(xx, r.Dx()-1-xx), 24)
			k := float32(strength) * fx * fy
			if k <= 0.003 {
				continue
			}
			for c := 0; c < 3; c++ {
				tgt := float32(src[xx*4+c]) * 0.75 * float32(a) / 255 // a little darker than the cover: text stays readable
				cur := float32(row[i+c])
				row[i+c] = uint8(cur + (tgt-cur)*k)
			}
		}
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
	if e.Round != nil { // a round outranks the day's task
		u.roundRow(e.Round, x, w, y)
		return
	}
	cy := y + edgeH/2
	gx := x + railX
	// today's goal as a ring, filled as far as you've got
	goal := 0.0
	if e.Need > 0 {
		goal = math.Min(1, float64(e.Today)/float64(e.Need))
	}
	rc := p.yellow
	if goal >= 1 {
		rc = p.green
	}
	u.dc.arc(gx, cy, 6, 1.8, 0, 2*math.Pi, alpha(p.fg, 0.12))
	if g := goal * u.tw("goal", goal, 0); g > 0.01 {
		ringArc(u.dc, gx, cy, 6, 1.8, -math.Pi/2, -math.Pi/2+2*math.Pi*g, rc)
	}
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
		bw := u.pillButton(btn, "btn:edge:"+btn.Key, right, cy-btnH/2, btnH, p.yellow)
		right -= bw + 10
	} else if u.dashboards["edge"] != nil { // the line opens the dashboard: say so
		u.chevron(right-3, cy, 0, mix(p.faint, p.fg, 0.4*on))
		right -= 16
	}
	task := e.Task
	if task == "" {
		task = "Practice"
	}
	base := cy + 4.5
	nw := u.dc.text(fBody(), ellipsize(fBody(), task, (right-tx)*0.55), tx, base, p.fg)
	sx := tx + nw + 8
	if day, streak, ok := strings.Cut(sub, " · "); ok && strings.HasSuffix(streak, "-day streak") && e.Streak > 0 {
		// the streak gets its little flame
		sx += u.dc.text(fSecond(), day+" · ", sx, base, p.muted)
		fx, fy, hot := sx+4, cy+1, true
		u.lives = append(u.lives, live{r: image.Rect(int(fx-6), int(fy-10), int(fx+7), int(fy+6)), draw: func(c *canvas, t float64) {
			flame(c, fx, fy, 4.6, t, hot, p)
		}})
		sx += 11
		sub = streak
	}
	u.dc.text(fSecond(), ellipsize(fSecond(), sub, right-sx), sx, base, p.muted)
}

// pillButton draws a small round button right-aligned at right, and
// reports its width.
func (u *UI) pillButton(b Button, key string, right, y, h float32, tint color.NRGBA) float32 {
	p := u.pal
	f := face(fSemiBold, 12)
	bw := measure(f, b.Label) + 28
	x := right - bw
	k, d := u.hl(key), u.pressed(key)
	fill, txt := alpha(tint, 0.16+0.1*k), mix(tint, p.fg, 0.2)
	if b.Primary {
		fill, txt = mix(tint, colorWhite, 0.12*k+0.2*d), p.ink
	}
	in := float32(1.2 * d)
	u.dc.roundRect(x+in, y+in, bw-2*in, h-2*in, h/2, fill)
	u.dc.text(f, b.Label, x+14, y+h/2+4.2, txt)
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
			fill, txt = alpha(p.fg, 0.07+0.06*h), mix(p.red, p.fg, 0.1)
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
		bx -= 8
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
		return "Practice"
	}
	return "Permission"
}

func (u *UI) cardTitle(c *Card) []string {
	return wrap(fTitle(), c.Title, cardW-textX-pad, 2)
}

// codeLines wraps a command to its box: at spaces, continuation lines
// indented, four lines at most. Longer, it keeps the start and the end (the
// end is where a "&& rm -rf" would hide) and says the middle is left out.
func codeLines(code string, w float32) []string {
	f := fMonoS()
	var out []string
	for _, ln := range strings.Split(strings.TrimSpace(code), "\n") {
		cur, ind := "", ""
		for _, word := range strings.Fields(ln) {
			try := word
			if cur != "" {
				try = cur + " " + word
			}
			if measure(f, ind+try) <= w || cur == "" {
				cur = try
				continue
			}
			out = append(out, ind+cur)
			cur, ind = word, "  "
		}
		if cur != "" || ind == "" {
			out = append(out, ind+cur)
		}
	}
	for i := range out {
		out[i] = ellipsize(f, out[i], w)
	}
	if len(out) > 4 {
		out = []string{out[0], "  …", out[len(out)-2], out[len(out)-1]}
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
	if c.Note != "" {
		h += 18
	}
	if len(c.Options) > 0 {
		h += 10 + len(c.Options)*optH
	}
	if len(c.Buttons) > 0 || c.Footer != "" {
		h += 12 + btnH
	}
	return min(h+14, u.room()-14)
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
	u.spot = emberSpot{x: x + railX, y: 28, s: emberCard, mood: c.Mood, show: true}
	inner := w - 2*pad
	right := x + w - pad
	tx := x + textX
	i := 0
	// the eyebrow: who wants what; the meta and, for edge's notes, a close
	u.item("c:eyebrow", i, x, w, 0, cardTitleTop, func(y float32) {
		r := right
		if c.Kind == "edge" || c.Kind == "plugin" { // edge's notes and plugins' cards can be closed
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
				for _, r := range shellTokens(p, ln, !strings.HasPrefix(ln, "  ")) {
					cx += u.dc.text(fMonoS(), r.s, cx, by, r.col)
				}
			}
		})
		y += bh
		i++
	}
	if c.Note != "" { // what "Always" would allow, before you press it
		note := ellipsize(fCaption(), c.Note, right-x-pad)
		u.item("c:note", i, x, w, y, 18, func(y float32) { u.dc.text(fCaption(), note, x+pad+2, y+14, p.faint) })
		y += 18
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
					u.panel(x+pad, y, inner, optH, radRow, alpha(u.primaryOf(c), 0.07+0.03*h))
				} else if h > 0.01 || d > 0 {
					u.panel(x+pad, y, inner, optH, radRow, alpha(p.fg, 0.05*h+0.06*d))
				}
				if j > 0 {
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
	if len(c.Buttons) > 0 || c.Footer != "" {
		y += 12
		u.item("c:buttons", i, x, w, y, btnH, func(y float32) {
			u.buttonRow(c.Buttons, "btn:", x, w, y, c.Footer, u.primaryOf(c))
		})
	}
	// how long it will wait for you: a hairline that drains along the bottom
	if !c.Expires.IsZero() && !c.Born.IsZero() {
		total := c.Expires.Sub(c.Born).Seconds()
		bottom := float32(u.cardHeight(c)) - 1.5
		lx, lw := x+radCard, w-2*radCard
		u.lives = append(u.lives, live{r: image.Rect(int(lx)-2, int(bottom)-3, int(lx+lw)+2, int(bottom)+3), draw: func(cv *canvas, _ float64) {
			left := c.Expires.Sub(u.now()).Seconds()
			k := float32(math.Max(0, math.Min(1, left/total)))
			late := math.Max(0, math.Min(1, (15-left)/1.5)) // the last fifteen seconds turn amber, easing in
			col := mix(alpha(p.fg, 0.12), alpha(p.yellow, 0.8), late)
			th := 1 + 0.5*float32(late)
			cv.roundRect(lx+lw*(1-k)/2, bottom+1-th, lw*k, th, th/2, col)
		}})
	}
}
