package island

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	proto "github.com/Tejas242/ember/pkg/plugin"
	"golang.org/x/image/vector"
)

// A stage (docs/PLUGINS.md, Stages): a page whose first widget is a stage.
// Ember stands big on the left on a floor of light and dances with its own
// sense of rhythm; Claude's line runs along the top, the song along the
// bottom, and the column beside Ember holds the words, timed to the song.
// Everything is drawn from Ember's own data (the track, its levels, the
// sessions): the plugin only says what goes where, and the words.
//
// It is made for waiting on Claude: rhythmic and predictable, nothing to
// think about, the work's state always in the corner of your eye, and a
// graceful hand-back at a natural stop (a song's end), never a timer. Nothing
// escalates over hours, nothing flashes, and a request from Claude replaces
// it at once.

const (
	stageH     = 256
	stageEmber = 88  // Ember's size on a stage
	stageCol   = 146 // the column, from the stage's left edge
	stageCY    = 110 // the column's centre line
	stageBase  = 186 // the floor Ember stands on
	newSongFor = 2400 * time.Millisecond
)

// stageWidgets are a stage page's widgets, by kind.
type stageWidgets struct {
	stage, text, media, sessions, audio, countdown *proto.Widget
}

// stageOf says whether a page is a stage, and its widgets.
func stageOf(db *proto.Dashboard) (stageWidgets, bool) {
	var sw stageWidgets
	if db == nil || len(db.Widgets) == 0 || db.Widgets[0].Kind != "stage" {
		return sw, false
	}
	for i := range db.Widgets {
		w := &db.Widgets[i]
		switch w.Kind {
		case "stage":
			sw.stage = w
		case "timed_text":
			sw.text = w
		case "media":
			sw.media = w
		case "sessions":
			sw.sessions = w
		case "audio":
			sw.audio = w
		case "countdown":
			sw.countdown = w
		}
	}
	return sw, true
}

// stageState is what an open stage keeps between frames.
type stageState struct {
	at      time.Time    // the opening it belongs to (u.dashAt)
	hist    [][3]float64 // the song's last few seconds, for the ribbon
	histAt  time.Time
	lvl     float64 // the low band, smoothed: the floor light breathes with it
	idx     int     // the line on stage (-2: not yet placed)
	idxAt   time.Time
	fillK   float64 // how far the current line is sung (-1: none): Ember sings along
	song    string  // the media key on stage
	songAt  time.Time
	lines   []stageLine // the timed text, as staged (fast pairs merged)
	linesOf *proto.Widget
	floor   *image.RGBA // the floor's pool of light, one sprite a song
	floorOf string
	nudgeAt time.Time // the words were nudged earlier or later (shown a moment)
}

// stageLine is a line as it goes on stage: two fast lines become one.
type stageLine struct {
	at    time.Duration
	text  string
	words []proto.TimedWord
	fast  bool // merged from lines under 0.9 s apart: it changes with a quick fade
}

// stageNow is the open stage's state and widgets, if a stage is what's open.
func (u *UI) stageNow() (*stageState, stageWidgets, bool) {
	if u.dash == "" || u.lastView != vDash {
		return nil, stageWidgets{}, false
	}
	sw, ok := stageOf(u.page())
	if !ok {
		return nil, sw, false
	}
	if u.stage == nil || !u.stage.at.Equal(u.dashAt) {
		u.stage = &stageState{at: u.dashAt, idx: -2, song: mediaKey(u.track)}
	}
	return u.stage, sw, true
}

// onStage: a stage is open and shown.
func (u *UI) onStage() bool {
	if u.dash == "" || u.lastView != vDash {
		return false
	}
	_, ok := stageOf(u.page())
	return ok
}

// stageStill: the stage asks Ember not to dance (it listens), or a turn
// just finished on it (it celebrates, then looks at you).
func (u *UI) stageStill() bool {
	if u.dash == "" || u.lastView != vDash {
		return false
	}
	sw, ok := stageOf(u.page())
	if !ok {
		return false
	}
	return (sw.stage.Ember != nil && sw.stage.Ember.Dance == "still") || u.stageOver() != nil
}

// stageOver is the session whose turn ended while the stage was open, if one did.
func (u *UI) stageOver() *Session {
	for _, s := range u.sessionList() {
		if !s.Unseen.IsZero() && s.Recap != nil {
			return s
		}
	}
	return nil
}

func (d extDash) stageSize(u *UI, db *proto.Dashboard) (float64, float64) {
	w := float64(extDashW)
	if db.Size == "card" {
		w = cardW
	}
	return w, stageH
}

// energyT is the song's temperature, 0 (a late-night tape) .. 1 (a
// floor-filler): how hard the low end goes and how sure Ember is of the
// beat, cooled at night. It warms the light and the words and quickens the
// line changes; it never escalates past what the song does.
func (u *UI) energyT() float64 {
	g := &u.groove
	e := math.Max(0, math.Min(1, (g.energy-0.25)/0.55))
	T := 0.25*e + 0.75*e*g.lock
	if !u.dancing {
		T = math.Min(T, 0.25)
	}
	if u.lateNight() {
		T *= 0.7
	}
	if u.cfg.Calm {
		T = math.Min(T, 0.3)
	}
	return T
}

func (u *UI) lateNight() bool { h := u.now().Hour(); return h >= 23 || h < 5 }

// stageKind is what the column shows, in a word.
func (u *UI) stageKind(st *stageState, sw stageWidgets) string {
	t := u.track
	switch {
	case u.discreet():
		return "hidden"
	case sw.countdown != nil:
		return "countdown"
	case t != nil && !st.songAt.IsZero() && u.now().Sub(st.songAt) < newSongFor:
		return "change"
	case sw.text == nil:
		return "none"
	case sw.text.MediaKey != "" && sw.text.MediaKey != mediaKey(t):
		return "loading" // the words are another song's: they never show on this one
	case sw.text.State != "ready":
		return sw.text.State
	case sw.text.Mode == "stanzas" || (sw.text.MediaKey != "" && t != nil && (t.Length <= 0 || u.stuck == mediaKey(t))):
		return "stanzas"
	case len(sw.text.Lines) == 0:
		return "none"
	}
	return "synced"
}

func (d extDash) drawStage(u *UI, x, w float32, age time.Duration) bool {
	st, sw, ok := u.stageNow()
	if !ok {
		return false
	}
	t := u.track
	u.stageSong(st)
	k := phase(age, 260*time.Millisecond, 360*time.Millisecond) // the words wait for Ember to land on the stage
	right := x + w - pad
	kind := u.stageKind(st, sw)
	paused := t == nil || !t.Playing
	if u.art != nil && u.art.wash != nil && sw.stage.Haze == "cover" {
		strength := 0.55
		if u.lateNight() {
			strength = 0.36
		}
		if paused || kind == "countdown" {
			strength *= 0.6
		}
		u.washInside(u.art, image.Rect(int(x+2), 2, int(x+w-2), stageH-2), strength*k)
	}
	u.hits = append(u.hits, hit{image.Rect(int(x+stageCol-8), 40, int(right+6), stageBase+10), "page:words"})

	// Ember on the stage, its mood the work's
	ex := x + 74
	md := moodIdle
	if s := u.stageOver(); s != nil {
		md = map[bool]mood{true: moodError, false: moodHappy}[s.Recap.Failed]
	}
	u.spot = emberSpot{x: ex, y: stageBase - 0.45*stageEmber, s: stageEmber, mood: md, show: true}
	u.hits = append(u.hits, hit{image.Rect(int(ex-50), int(stageBase-stageEmber-6), int(ex+50), int(stageBase+6)), "ember"})

	// the floor: a pool of the cover's light that breathes with the low end, a ripple on the beat, Ember's shadow
	u.lives = append(u.lives, live{r: image.Rect(int(ex-84), stageBase-26, int(ex+84), stageBase+16), draw: func(cv *canvas, _ float64) {
		u.stageFloor(cv, st, sw, ex, k, kind, paused)
	}})

	// the top line: Claude at work on the left (what matters most comes first), the song's tempo on the right
	if sw.sessions != nil || sw.stage.Ember.Dance == "music" {
		u.lives = append(u.lives, live{r: image.Rect(int(x+pad-4), 12, int(right+4), 40), draw: func(cv *canvas, tt float64) {
			u.stageTop(cv, sw, x+pad, right, 30, tt, k, kind)
		}})
		if s := u.stageOver(); s != nil && sw.sessions != nil { // the way back: one small button, never a timer
			bw := measure(fCaption(), "Back to it") + 22
			u.hits = append(u.hits, hit{image.Rect(int(right-bw), 15, int(right), 37), "page:back"})
		}
	}

	// the column
	colX := x + stageCol
	u.lives = append(u.lives, live{r: image.Rect(int(colX-6), 40, int(right+6), stageBase+10), draw: func(cv *canvas, _ float64) {
		u.stageSong(st)
		u.stageColumn(cv, st, sw, u.stageKind(st, sw), colX, right-colX, k) // what it shows changes with time: worked out each frame
	}})

	// the song along the bottom
	if sw.media != nil && t != nil {
		u.stageMedia(x, w, right, k, kind, paused)
	}
	return age < 520*time.Millisecond
}

// stageSong notices a new song: its title takes the stage while the words
// load, and the beat is learned again.
func (u *UI) stageSong(st *stageState) {
	if key := mediaKey(u.track); key != st.song {
		st.song, st.songAt, st.idx = key, u.now(), -2
		st.hist = st.hist[:0]
		u.groove.lock, u.groove.counted = 0, 0
	}
}

// stageMedia is the song: the cover (a click plays or pauses), title,
// artist, time, a hairline of progress in the cover's colour.
func (u *UI) stageMedia(x, w, right float32, k float64, kind string, paused bool) {
	p, t := u.pal, u.track
	acc := u.accent(p.fg)
	by := float32(stageH - 52)
	cx := x + pad
	if u.art != nil && kind != "countdown" {
		u.dc.drawArt(u.art.square(30), int(cx), int(by), 8, k)
		if paused {
			u.dc.roundRect(cx, by, 30, 30, 8, alpha(colorBlack, 0.45*k))
			u.mediaGlyph("toggle", cx+15.5, by+15, false, alpha(p.fg, k), 0.6)
		}
	} else {
		u.dc.roundRect(cx, by, 30, 30, 8, alpha(p.fg, 0.08*k))
		u.dc.icon("note", cx+15, by+15, 13, alpha(p.muted, k))
	}
	u.hits = append(u.hits, hit{image.Rect(int(cx-2), int(by-2), int(cx+32), int(by+32)), "music:toggle"})
	tx := cx + 40
	tw := right - tx - 74
	u.dc.text(fBody(), ellipsize(fBody(), t.Title, tw), tx, by+13, alpha(p.fg, k))
	u.dc.text(fCaption(), ellipsize(fCaption(), t.Artist, tw), tx, by+28, alpha(p.muted, k))
	if t.Length <= 0 {
		return
	}
	f := fCaption()
	u.lives = append(u.lives, live{r: image.Rect(int(right-80), int(by), int(right+2), int(by+18)), draw: func(cv *canvas, _ float64) {
		txt := clock(max(t.Pos(u.now()), 0)) + " / " + clock(t.Length)
		if paused {
			txt = "Paused"
		}
		cv.text(f, txt, right-measure(f, txt), by+13, alpha(p.faint, k))
	}})
	if kind != "countdown" {
		u.progressLive(x+pad+2, stageH-14, w-2*pad-4, 2, alpha(acc, map[bool]float64{true: 0.5, false: 1}[paused]), 0)
	}
}

// square is the cover at n px, made once.
func (a *albumArt) square(n int) *image.RGBA {
	if a.sq == nil || a.sq.Rect.Dx() != n {
		a.sq = scaleSquare(a.large, n)
	}
	return a.sq
}

// stageFloor draws the floor: the pool of light (a sprite made once a song,
// laid down with the low end's breath), a ripple on the beat when the song
// runs hot, and Ember's shadow, smaller when it hops.
func (u *UI) stageFloor(cv *canvas, st *stageState, sw stageWidgets, ex float32, k float64, kind string, paused bool) {
	p := u.pal
	u.stageListen(st)
	T := u.energyT()
	if paused || kind == "countdown" || sw.stage.Ember.Dance == "still" {
		T = 0
	}
	if sw.stage.Light != "none" {
		lvl := st.lvl
		if sw.stage.Light == "still" || u.lowBattery() {
			lvl = 0.5 // a light that doesn't breathe
		}
		// the breath is capped: the light never swings more than ~6% in brightness
		a := (0.06 + 0.08*T + math.Min(0.16*T*math.Min(1, lvl), 0.045)) * k
		if u.lateNight() {
			a *= 0.75
		}
		light := mix(u.accent(p.fg), p.fg, 0.15)
		u.floorSprite(st, light)
		cv.blitAlpha(st.floor, int(ex)-st.floor.Rect.Dx()/2, stageBase+1-st.floor.Rect.Dy()/2, a)
		g := &u.groove
		if u.dancing && T > 0.45 && !u.cfg.Calm { // the floor ripples on the beat
			f := g.phase - math.Floor(g.phase)
			rx := float32(30 + 46*easeOut(f))
			vRing(cv, ex, stageBase+1, rx, rx*0.17, 1.4, alpha(light, 0.32*(1-f)*T*k))
		}
	}
	up := float32(math.Min(1, u.body.y/6))
	sw0 := 27 * (1 - 0.3*up)
	vEllipse(cv, ex, stageBase+2, sw0, 3.6*(1-0.3*up), alpha(colorBlack, 0.34*float64(1-0.6*up)*k))
}

// floorSprite makes the pool of light for the song's colour: three nested
// ellipses, drawn once at full strength.
func (u *UI) floorSprite(st *stageState, light color.NRGBA) {
	key := fmt.Sprint(light)
	if st.floor != nil && st.floorOf == key {
		return
	}
	c := newCanvas(160, 30)
	for i := 0; i < 3; i++ {
		rx := float32(78 - 22*i)
		vEllipse(c, 80, 15, rx, rx*0.17, alpha(light, 1/1.8))
	}
	st.floor, st.floorOf = c.img, key
}

// stageListen takes in the song's levels: the ribbon's history, the floor's breath.
func (u *UI) stageListen(st *stageState) {
	lv, at, ok := u.audio.Sample()
	if !ok || at == st.histAt {
		if !ok {
			st.lvl *= 0.97
		}
		return
	}
	st.histAt = at
	st.hist = append(st.hist, lv)
	if len(st.hist) > 120 {
		st.hist = st.hist[len(st.hist)-120:]
	}
	st.lvl += (lv[0] - st.lvl) * 0.25
}

// lowBattery: on battery and below a fifth.
func (u *UI) lowBattery() bool { pw := u.power; return pw != nil && pw.state == 2 && pw.pct < 20 }

// stageTop: the session at work, quietly; its outcome in its colour once
// it's over, with the way back. While the screen is shared, only the project.
func (u *UI) stageTop(cv *canvas, sw stageWidgets, x, right, base float32, t float64, k float64, kind string) {
	p, now := u.pal, u.now()
	f := fCaption()
	over := u.stageOver()
	if g := &u.groove; over == nil && u.dancing && g.period > 0 && sw.stage.Ember.Dance == "music" {
		bpm := fmt.Sprintf("%d bpm", int(math.Round(60/g.period)))
		bw := measure(f, bpm)
		ph := g.phase - math.Floor(g.phase)
		cv.text(f, bpm, right-bw, base, alpha(p.faint, k))
		cv.circle(right-bw-7, base-3.5, 2.2, alpha(u.accent(p.fg), (0.25+0.6*math.Exp(-ph*5))*k))
	}
	if sw.sessions == nil || !u.cfg.Claude {
		return
	}
	if ss := over; ss != nil {
		col, word := p.green, "done"
		if ss.Recap.Failed {
			col, word = p.red, "stopped"
		}
		cv.icon(map[bool]string{true: "warning", false: "check"}[ss.Recap.Failed], x+4, base-4, 10, alpha(col, k))
		head := u.short(ss) + " " + word
		hw := cv.text(fBody(), head, x+16, base, alpha(col, k))
		say := ss.Recap.Summary
		if ss.Recap.Failed {
			say = ss.Recap.Error
		}
		bl := "Back to it"
		bw := measure(f, bl) + 22
		hk := u.hl("page:back")
		cv.roundRect(right-bw, base-15, bw, 22, 11, alpha(p.fg, (0.12+0.08*hk)*k))
		cv.text(f, bl, right-bw+11, base-0.5, alpha(p.fg, k))
		if say != "" && !u.discreet() { // shared: the project only
			room := right - bw - 10 - (x + 16 + hw + 8)
			cv.text(f, ellipsize(f, "· "+firstLine(say), room), x+16+hw+6, base, alpha(p.muted, k))
		}
		return
	}
	for _, ss := range u.sessionList() {
		if ss.status() != "work" {
			continue
		}
		txt := u.short(ss)
		if d := now.Sub(ss.Since); !ss.Since.IsZero() && d >= time.Minute {
			txt += " · " + mins(d)
		}
		b := 0.5 + 0.5*math.Sin(t*2*math.Pi/beat) // breathing on Ember's own beat
		if u.cfg.Calm {
			b = 1
		}
		cv.circle(x+3, base-3.5, 2.6, alpha(p.green, (0.5+0.5*b)*k))
		cv.text(f, txt, x+12, base, alpha(p.muted, k))
		return
	}
}

// bigFace: the largest size (17, 15.5, 14) at which the line fits in two
// lines; at 14 it may take three, then it's cut.
func bigFace(s string, w float32) (fontFace, []string) {
	for _, sz := range []float64{17, 15.5} {
		f := face(fSemiBold, sz)
		if ls := wrap(f, s, w, 3); len(ls) <= 2 {
			return f, ls
		}
	}
	f := face(fSemiBold, 14)
	return f, wrap(f, s, w, 3)
}

// stagedLines is the timed text as it goes on stage: two lines under 0.9 s
// apart are sung as one, so nothing flickers past unread.
func (st *stageState) stagedLines(w *proto.Widget) []stageLine {
	if st.linesOf == w {
		return st.lines
	}
	st.linesOf, st.lines = w, st.lines[:0]
	ls := w.Lines
	for i := 0; i < len(ls); i++ {
		l := stageLine{at: time.Duration(ls[i].TMs) * time.Millisecond, text: ls[i].Text, words: ls[i].Words}
		if w.Mode == "synced" && i+1 < len(ls) && l.text != "" && ls[i+1].Text != "" && ls[i+1].TMs-ls[i].TMs < 900 &&
			utf8.RuneCountInString(l.text)+utf8.RuneCountInString(ls[i+1].Text) < 150 {
			l.text, l.words, l.fast = l.text+" "+ls[i+1].Text, nil, true
			i++
		}
		st.lines = append(st.lines, l)
	}
	return st.lines
}

// lineAt is the line being sung at pos (-1 before the first).
func lineAt(ls []stageLine, pos time.Duration) int {
	i := -1
	for j, l := range ls {
		if l.at <= pos {
			i = j
		}
	}
	return i
}

// wordsPos is where the timed text is: the media playhead (a little ahead,
// as a singer breathes in first), nudged by what you set for this song; or
// its own clock.
func (u *UI) wordsPos(tw *proto.Widget) (time.Duration, bool) {
	now := u.now()
	if c := tw.Clock; tw.MediaKey == "" && c != nil {
		at := time.UnixMilli(c.AtMs)
		return time.Duration(c.PosMs)*time.Millisecond + time.Duration(float64(now.Sub(at))*c.Rate), true
	}
	t := u.track
	if t == nil || tw.MediaKey != mediaKey(t) {
		return 0, false
	}
	return t.Pos(now) + wordsLead + u.wordsNudge(tw.MediaKey), true
}

// wordsLead: the words are shown a little ahead of the song, as a singer breathes in first.
const wordsLead = 150 * time.Millisecond

// stageColumn draws the column beside Ember: the words, or the song's title
// over its ribbon when there are none to show.
func (u *UI) stageColumn(cv *canvas, st *stageState, sw stageWidgets, kind string, x, w float32, k float64) {
	p, t, now := u.pal, u.track, u.now()
	acc := u.accent(p.fg)
	cy := float32(stageCY)
	st.fillK = -1
	if t == nil {
		return
	}
	titleBlock := func(head, sub string, subCol color.NRGBA, top float32) float32 {
		f := face(fSemiBold, 17)
		y := top
		for _, ln := range wrap(f, head, w, 2) {
			cv.text(f, ln, x, y, alpha(p.fg, k))
			y += 22
		}
		if sub != "" {
			cv.text(fSecond(), ellipsize(fSecond(), sub, w), x, y-2, alpha(subCol, k))
			y += 16
		}
		return y
	}
	ribbon := func(y, h float32) {
		if sw.audio == nil {
			return
		}
		col := acc
		if sw.audio.Tint == "quiet" {
			col, h = p.muted, min(h, 9)
		}
		st.ribbon(cv, x, w, y, h, col, k)
	}
	caption := func(s string, y float32, a float64) {
		if s != "" {
			cv.text(fCaption(), ellipsize(fCaption(), s, w), x, y, alpha(p.faint, a*k))
		}
	}
	capOf := func(def string) string {
		if sw.text != nil && sw.text.Caption != "" {
			return sw.text.Caption
		}
		return def
	}
	switch kind {
	case "hidden": // the screen is shared: no words
		y := titleBlock(t.Title, t.Artist, p.muted, cy-30)
		ribbon(y+20, 16)
		caption("Words hidden while your screen is shared", cy+68, 1)
		return
	case "countdown": // an ad: the way back to the music, draining
		c := sw.countdown
		if c.Title != "" {
			cv.eyebrow(c.Title, x, cy-30, alpha(p.faint, k))
		}
		left := max(t.Length-t.Pos(now), 0)
		f := face(fSemiBold, 17)
		text := c.Text
		if text == "" {
			text = "Back in"
		}
		cv.text(f, ellipsize(f, text, w), x, cy-6, alpha(p.fg, k))
		cv.text(f, "in "+clock(left+time.Second-1), x, cy+16, alpha(p.fg, 0.62*k))
		cv.roundRect(x, cy+34, w, 2, 1, alpha(p.line, 0.9*k))
		if t.Length > 0 {
			cv.roundRect(x, cy+34, w*float32(float64(left)/float64(t.Length)), 2, 1, alpha(p.muted, k))
		}
		return
	case "change": // a new song: its title, while its words are found
		cv.eyebrow("New song", x, cy-36, alpha(mix(acc, p.fg, 0.4), k))
		titleBlock(t.Title, t.Artist, p.muted, cy-12)
		return
	case "loading":
		y := titleBlock(t.Title, t.Artist, p.muted, cy-34)
		ribbon(y+20, 16)
		b := 0.5 + 0.35*math.Sin(float64(now.UnixMilli())/320)
		if u.cfg.Calm {
			b = 0.6
		}
		caption(capOf("Finding the words…"), cy+64, b)
		return
	case "none":
		y := titleBlock(t.Title, t.Artist, p.muted, cy-34)
		ribbon(y+22, 18)
		caption(capOf(""), cy+68, 1)
		return
	case "instrumental":
		y := titleBlock(t.Title, "Instrumental", mix(acc, p.fg, 0.4), cy-34)
		ribbon(y+22, 18)
		return
	case "spoken": // a talk: its title, the time left, a thin grey ribbon of speech
		sub := t.Artist
		if t.Length > 0 {
			sub = strings.TrimPrefix(sub+" · "+mins(max(t.Length-t.Pos(now), 0))+" left", " · ")
		}
		y := titleBlock(t.Title, sub, p.muted, cy-34)
		if sw.audio != nil {
			st.ribbon(cv, x, w, y+20, 9, p.muted, k)
		}
		return
	case "stanzas": // words without times: four lines at a time, paged through the song
		ls := st.stagedLines(sw.text)
		n := len(ls)
		pg := u.stanzaPage(st, max((n+3)/4, 1))
		f := face(fMedium, 14)
		y := cy - 30
		for i := pg * 4; i < min(n, pg*4+4); i++ {
			ln := ellipsize(f, ls[i].text, w)
			cv.text(f, ln, lineX(f, ln, x, w), y, alpha(p.fg, 0.82*k))
			y += 21
		}
		caption(u.stanzaCaption(sw), y+10, 1)
		return
	}
	u.karaoke(cv, st, sw, x, w, k)
}

// stanzaPage is the page of words to show, of pages: through the song by
// its playhead, or by how long it has played when its player doesn't say
// where it is; without a length, one every sixteen seconds.
func (u *UI) stanzaPage(st *stageState, pages int) int {
	t, now := u.track, u.now()
	pos := t.Pos(now)
	if u.stuck == mediaKey(t) {
		pos = u.songElapsed(now)
	}
	pg := 0
	switch {
	case t.Length > 0:
		pg = int(float64(pos) / float64(t.Length) * float64(pages))
	default: // no playhead to follow
		pg = int(now.Sub(st.at) / (16 * time.Second))
	}
	return min(max(pg, 0), pages-1)
}

// stanzaCaption is what's said under words shown as stanzas: the plugin's
// caption, or, for timed words whose player's playhead doesn't move, why
// they're not.
func (u *UI) stanzaCaption(sw stageWidgets) string {
	if u.stuck == mediaKey(u.track) && sw.text.Mode != "stanzas" {
		return "This player doesn't say where the song is"
	}
	return sw.text.Caption
}

// karaoke: the line being sung, filling with light at its pace with a soft
// leading edge; the one before shrinks upward into the past; the next two
// wait below. A break in the words is three dots filling over it.
func (u *UI) karaoke(cv *canvas, st *stageState, sw stageWidgets, x, w float32, k float64) {
	p, t, now := u.pal, u.track, u.now()
	acc := u.accent(p.fg)
	cy := float32(stageCY)
	small := face(fMedium, 12.5)
	T := u.energyT()
	dimK := 1.0
	if !t.Playing {
		dimK = 0.55 // paused: everything waits, the line frozen
	}
	pos, ok := u.wordsPos(sw.text)
	if !ok {
		return
	}
	ls := st.stagedLines(sw.text)
	idx := lineAt(ls, pos)
	if st.idx == -2 { // opening: the line already being sung is just there
		st.idx, st.idxAt = idx, now.Add(-time.Hour)
	}
	if idx != st.idx {
		st.idx, st.idxAt = idx, now
	}
	slide := time.Duration(float64(560*time.Millisecond) - 260*T*float64(time.Millisecond)) // quicker as the song runs hotter
	e := easeOut(math.Min(1, float64(now.Sub(st.idxAt))/float64(slide)))
	fade := u.cfg.Calm || (idx >= 0 && ls[idx].fast)
	if fade { // calm, or a fast pair: a quick cross-fade instead of the slide
		d := 200 * time.Millisecond
		if !u.cfg.Calm {
			d = 120 * time.Millisecond
		}
		e = math.Min(1, float64(now.Sub(st.idxAt))/float64(d))
	}
	at := func(i int) string {
		if i < 0 || i >= len(ls) {
			return ""
		}
		return ls[i].text
	}
	cur := at(idx)
	shift := float32(1-e) * 30
	if fade {
		shift = 0
	}
	lit := mix(acc, p.fg, 0.86-0.2*T) // warmer as the song gets hotter
	dim := alpha(p.fg, 0.3)

	if cur == "" && idx >= 0 { // a break: three dots fill up over the gap, so you know when to come back in
		start, end := ls[idx].at, ls[idx].at+6*time.Second
		if idx+1 < len(ls) {
			end = ls[idx+1].at
		}
		f := float64(pos-start) / float64(max(end-start, time.Millisecond))
		g := &u.groove
		ph := g.phase - math.Floor(g.phase)
		pulse := float32(math.Exp(-ph*5)) * float32(T)
		if u.cfg.Calm {
			pulse = 0
		}
		for i := 0; i < 3; i++ {
			fi := math.Max(0, math.Min(1, f*3-float64(i)))
			cv.circle(x+5+float32(i)*15, cy-5+shift, 3.6+0.8*pulse, alpha(p.fg, (0.22+0.72*fi)*k*dimK))
		}
		if nx := at(idx + 1); nx != "" {
			nx = ellipsize(small, nx, w)
			cv.text(small, nx, lineX(small, nx, x, w), cy+26+shift, alpha(p.muted, 0.6*k*dimK))
		}
		if pv := at(idx - 1); pv != "" {
			pv = ellipsize(small, pv, w)
			cv.text(small, pv, lineX(small, pv, x, w), cy-34+shift-8*float32(e), alpha(p.muted, 0.4*k*dimK*(1-0.5*e)))
		}
		return
	}

	bf, lines := bigFace(cur, w)
	lh := float32(bf.Metrics().Height.Round()) + 4
	top := cy - float32(len(lines)-1)*lh/2 + 5 + shift
	fill := -1.0
	if idx >= 0 {
		fill = lineFill(ls, idx, pos)
		if fill < 1 && t.Playing {
			st.fillK = fill
		}
	}
	total := float32(0)
	for _, ln := range lines {
		total += measure(bf, ln)
	}
	before := float32(0)
	y := top
	asc := int(bf.Metrics().Ascent.Ceil()) + 2
	rtl := rtlText(cur) // a line read right to left: against the right edge, lit from the right
	byLetter, letters, beforeL := needsShaping(cur), 0, 0
	if byLetter { // shaped: lit by its letters, in reading order
		for _, ln := range lines {
			letters += utf8.RuneCountInString(display(keepDir(ln, rtl)))
		}
	}
	for _, ln := range lines {
		ln = keepDir(ln, rtl)
		lw := measure(bf, ln)
		lx := x
		if rtl {
			lx = x + w - lw
		}
		cv.text(bf, ln, lx, y, alpha(dim, e*k*dimK))
		var spans []litSpan
		if byLetter {
			ds := display(ln)
			if sung := float32(fill)*float32(letters) - float32(beforeL); sung > 0 {
				var upto int
				if spans, upto = shapedSpans(bf, ds, lx, sung); upto > 0 {
					cv.drawMask(cv.litMask(bf, ds, upto), lx, y, alpha(lit, e*k*dimK))
				}
			}
			beforeL += utf8.RuneCountInString(ds)
		} else if sung := float32(fill)*total - before; sung > 0 {
			spans = litSpans(lx, lw, sung, rtl)
		}
		if len(spans) > 0 {
			ct, cb := int(y)-asc, int(y)+6
			if needsShaping(ln) { // its marks may reach further than Ember's font
				m := cv.mask(bf, ln)
				ct, cb = min(ct, int(y)-m.asc-1), max(cb, int(y)-m.asc+m.m.Rect.Dy()+1)
			}
			for _, sp := range spans {
				cv.clipR = image.Rect(int(sp.x0), ct, int(sp.x1), cb)
				cv.text(bf, ln, lx, y, alpha(lit, sp.a*e*k*dimK))
			}
			cv.clipR = image.Rectangle{}
		}
		before += lw
		y += lh
	}
	// the line just sung: on its way up, it shrinks into the past
	if pv := at(idx - 1); pv != "" {
		if e < 1 && !fade {
			pf, pl := bigFace(pv, w)
			yy := top - shift - float32(e)*34 - float32(len(pl)-1)*lh/2
			for _, ln := range pl {
				ln = keepDir(ln, rtlText(pv))
				cv.text(pf, ln, lineX(pf, ln, x, w), yy, alpha(lit, (1-e)*0.8*k*dimK))
				yy += lh
			}
		}
		pv = ellipsize(small, pv, w)
		cv.text(small, pv, lineX(small, pv, x, w), top-34+(1-float32(e))*8, alpha(p.muted, 0.42*e*k*dimK))
	}
	yb := top + float32(len(lines)-1)*lh + 28
	for i, a := range []float64{0.6, 0.3} {
		nx := at(idx + 1 + i)
		if nx == "" {
			break
		}
		nx = ellipsize(small, nx, w)
		cv.text(small, nx, lineX(small, nx, x, w), yb+float32(i)*19, alpha(p.muted, a*k*dimK))
	}
	if d := now.Sub(st.nudgeAt); d < 1500*time.Millisecond { // you nudged the words: by how much, a moment
		n := u.wordsNudge(sw.text.MediaKey)
		s := fmt.Sprintf("words %+.2f s", n.Seconds())
		f := fCaption()
		cv.text(f, s, x+w-measure(f, s), 52, alpha(p.faint, (1-float64(d)/float64(1500*time.Millisecond))*k))
	}
}

// lineX is where a line of the words starts: at the column's left edge x,
// or against its right edge when the line reads right to left.
func lineX(f fontFace, s string, x, w float32) float32 {
	if rtlText(s) {
		return x + w - measure(f, s)
	}
	return x
}

// keepDir keeps a wrapped piece of a line read right to left reading that
// way when it starts with a word in a left-to-right script (a right-to-left
// mark before it).
func keepDir(ln string, rtl bool) string {
	if rtl && !rtlText(ln) {
		return "\u200f" + ln
	}
	return ln
}

// litSpan is a stretch of a line drawn lit, and how strongly.
type litSpan struct {
	x0, x1 float32
	a      float64
}

// litSpans is the part of a line (drawn from lx, lw wide) lit once sung
// pixels of it are sung: from the left with a soft leading edge two steps
// wide; a line read right to left fills from the right.
func litSpans(lx, lw, sung float32, rtl bool) []litSpan {
	if !rtl {
		edge := lx + min(sung, lw)
		sp := []litSpan{{lx - 2, edge, 1}}
		if sung < lw { // a soft leading edge, not a hard cut
			sp = append(sp, litSpan{edge, edge + 2, 0.6}, litSpan{edge + 2, edge + 4, 0.3})
		}
		return sp
	}
	edge := lx + lw - min(sung, lw)
	sp := []litSpan{{edge, lx + lw + 2, 1}}
	if sung < lw {
		sp = append(sp, litSpan{edge - 2, edge, 0.6}, litSpan{edge - 4, edge - 2, 0.3})
	}
	return sp
}

// lineFill is how far line idx is sung at pos, 0..1: by its words' times
// when it has them, else at a singer's pace (68 ms a character) until the
// next line.
func lineFill(ls []stageLine, idx int, pos time.Duration) float64 {
	l := ls[idx]
	end := l.at + 4*time.Second
	if idx+1 < len(ls) {
		end = ls[idx+1].at
	}
	n := utf8.RuneCountInString(l.text)
	if len(l.words) > 0 && n > 0 {
		chars := 0.0
		done := 0
		for j, wd := range l.words {
			wAt := time.Duration(wd.TMs) * time.Millisecond
			if wAt > pos {
				break
			}
			next := end
			if j+1 < len(l.words) {
				next = time.Duration(l.words[j+1].TMs) * time.Millisecond
			}
			f := math.Min(1, float64(pos-wAt)/float64(max(next-wAt, time.Millisecond)))
			chars = float64(done) + f*float64(wd.N)
			done += wd.N
		}
		return math.Max(0, math.Min(1, chars/float64(n)))
	}
	dur := min(end-l.at, time.Duration(n)*68*time.Millisecond+600*time.Millisecond)
	return math.Max(0, math.Min(1, float64(pos-l.at)/float64(max(dur, time.Millisecond))))
}

// ribbon: the song's last four seconds flowing past, mirrored bars, newest
// on the right and brightest.
func (st *stageState) ribbon(cv *canvas, x, w, cy, hmax float32, col color.NRGBA, k float64) {
	const step = 5
	n := int(w / step)
	h := st.hist
	for i := 0; i < n; i++ {
		j := len(h) - n + i
		v := 0.0
		if j >= 0 && j < len(h) {
			v = 0.6*h[j][0] + 0.4*h[j][1]
		}
		bh := float32(1.5 + v*float64(hmax))
		age := float64(i) / float64(n)
		cv.roundRect(x+float32(i)*step, cy-bh, 3, 2*bh, 1.5, alpha(col, (0.12+0.75*age*age)*k))
	}
}

// stagePose: Ember on a stage. Singing along on a line it's dancing to,
// listening to a talk, waiting an ad out, looking at the cover while
// paused, looking at you once a turn has finished.
func (u *UI) stagePose(ps *pose) {
	st, sw, ok := u.stageNow()
	if !ok || u.act != nil {
		return
	}
	e := sw.stage.Ember
	switch {
	case u.track != nil && !u.track.Playing:
		ps.eyes, ps.eyeH, ps.lookX, ps.lookY, ps.mouth = eyeOpen, 2.8, -0.6, 1.2, mouthNone
	case u.stageOver() != nil:
		if u.stageOver().Recap.Failed {
			return // slumped: its mood says it
		}
		ps.eyes, ps.lookX, ps.lookY = eyeOpen, 0, -0.3 // done: it looks at you
	case e.Pose == "attentive": // listening to words
		ps.eyes, ps.eyeH, ps.lid, ps.lookX, ps.lookY, ps.mouth = eyeOpen, 3, 0, 1.6, 0.2, mouthNone
	case e.Pose == "sleepy": // waiting something out
		ps.eyes, ps.eyeH, ps.lid, ps.lookX, ps.lookY, ps.mouth = eyeOpen, 3, 0.45, 1.4, 0.4, mouthNone
	case e.Sing && u.dancing && !u.discreet() && st.idx >= 0 && st.idx < len(st.lines) && sw.text != nil: // it sings along
		if l, ok := u.singLineOf(st, sw.text); ok {
			m := u.face.sing.step(l) // it listens all through the line
			if st.fillK >= 0 || u.face.sing.singing(l) {
				u.singFace(ps, st, m)
			}
		}
	}
}

// singLineOf is the line on stage as the singing mouth needs it.
func (u *UI) singLineOf(st *stageState, tw *proto.Widget) (singLine, bool) {
	pos, ok := u.wordsPos(tw)
	if !ok || st.idx < 0 || st.idx >= len(st.lines) {
		return singLine{}, false
	}
	l := st.lines[st.idx]
	if tw.MediaKey != "" || tw.Clock == nil {
		pos -= wordsLead
	}
	sl := singLine{text: l.text, at: l.at, posW: pos.Seconds(), audio: u.audio, now: u.now()}
	if st.idx+1 < len(st.lines) {
		sl.next, sl.hasNext = st.lines[st.idx+1].at, true
	}
	return sl, l.text != ""
}

// singFace: singing a line. The mouth sings the words (lipsync.go). A new line starts with
// a breath (a blink) and a glance at the words; then it sings out, eyes open
// and a little up, closing them while the song holds loud (shut 1.4 s at
// least, open 0.8 s).
func (u *UI) singFace(ps *pose, st *stageState, m mouthShape) {
	f, now := &u.face, u.now()
	ps.mouth = mouthSing
	ps.sing = m
	e := math.Max(0, math.Min(1, (f.voxS-0.35)/0.3))
	if st.idx != f.lineIdx {
		f.lineIdx, f.readAt = st.idx, now
		u.mind.blinkSoon(now)
	}
	hold := 800 * time.Millisecond // eyes open a while before they close again
	if f.shut {
		hold = 1400 * time.Millisecond // closed into a held note: long enough not to read as a slow blink
	}
	if want := e > 0.6; want != f.shut && now.Sub(f.shutAt) > hold && now.Sub(f.readAt) > 900*time.Millisecond {
		f.shut, f.shutAt = want, now
	}
	ps.eyes, ps.lid, ps.lidAsym, ps.lookX, ps.lookY = eyeOpen, float32(0.12+0.25*e), 0, 0, -0.4
	switch {
	case now.Sub(f.readAt) < 450*time.Millisecond: // the words
		ps.lid, ps.lookX, ps.lookY = 0.05, 1.0, 0.1
	case f.shut:
		ps.eyes = eyeShut
	}
	ps.lidFine = 0
}

// stageFPS is how often a stage's live parts repaint: 24 a second on a
// locked beat, 15 for a sway or a talk, 8 paused, 12 calm; on battery 15,
// under a fifth 10.
func (u *UI) stageFPS() int {
	fps := 24
	if !u.dancing {
		fps = 15
	}
	if t := u.track; t == nil || !t.Playing {
		fps = 8
	}
	if u.cfg.Calm {
		fps = min(fps, 12)
	}
	if pw := u.power; pw != nil && pw.state == 2 {
		fps = min(fps, 15)
		if pw.pct < 20 {
			fps = min(fps, 10)
		}
	}
	return fps
}

// ---- nudging the words: a quarter second a scroll step, remembered per song ----

const maxNudges = 200

type wordsNudges struct {
	Ms    map[string]int64 `json:"ms"`
	Order []string         `json:"order"` // oldest first: the oldest is forgotten past maxNudges
}

func nudgesPath() string { return filepath.Join(filepath.Dir(habitsPath()), "words.json") }

func (u *UI) nudges() *wordsNudges {
	if u.nudge == nil {
		u.nudge = &wordsNudges{Ms: map[string]int64{}}
		if b, err := os.ReadFile(nudgesPath()); err == nil {
			json.Unmarshal(b, u.nudge)
			if u.nudge.Ms == nil {
				u.nudge.Ms = map[string]int64{}
			}
		}
	}
	return u.nudge
}

// wordsNudge is how much earlier (+) the words of a song are shown.
func (u *UI) wordsNudge(key string) time.Duration {
	if key == "" {
		return 0
	}
	return time.Duration(u.nudges().Ms[key]) * time.Millisecond
}

// nudgeWords moves the open stage's words a quarter second (dir -1: earlier,
// sooner; +1 later), up to five seconds either way, and remembers it.
func (u *UI) nudgeWords(dir int) {
	st, sw, ok := u.stageNow()
	if !ok || sw.text == nil || sw.text.MediaKey == "" {
		return
	}
	n := u.nudges()
	key := sw.text.MediaKey
	v := max(-5000, min(5000, n.Ms[key]-int64(dir)*250))
	if _, had := n.Ms[key]; !had {
		n.Order = append(n.Order, key)
		if len(n.Order) > maxNudges {
			delete(n.Ms, n.Order[0])
			n.Order = n.Order[1:]
		}
	}
	n.Ms[key] = v
	st.nudgeAt = u.now()
	if b, err := json.Marshal(n); err == nil {
		os.MkdirAll(filepath.Dir(nudgesPath()), 0o755)
		save(nudgesPath(), b, 0o644)
	}
}

// ---- shapes ----

func vEllipsePath(r *vector.Rasterizer, cx, cy, rx, ry float32, rev bool) {
	const kap = 0.5523
	kx, ky := rx*kap, ry*kap
	r.MoveTo(cx+rx, cy)
	if !rev {
		r.CubeTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry)
		r.CubeTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy)
		r.CubeTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry)
		r.CubeTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy)
	} else {
		r.CubeTo(cx+rx, cy-ky, cx+kx, cy-ry, cx, cy-ry)
		r.CubeTo(cx-kx, cy-ry, cx-rx, cy-ky, cx-rx, cy)
		r.CubeTo(cx-rx, cy+ky, cx-kx, cy+ry, cx, cy+ry)
		r.CubeTo(cx+kx, cy+ry, cx+rx, cy+ky, cx+rx, cy)
	}
	r.ClosePath()
}

func vEllipse(cv *canvas, cx, cy, rx, ry float32, col color.NRGBA) {
	cv.fillPath(cx-rx-1, cy-ry-1, cx+rx+1, cy+ry+1, col, func(r *vector.Rasterizer, ox, oy float32) {
		vEllipsePath(r, cx-ox, cy-oy, rx, ry, false)
	})
}

func vRing(cv *canvas, cx, cy, rx, ry, w float32, col color.NRGBA) {
	cv.fillPath(cx-rx-w, cy-ry-w, cx+rx+w, cy+ry+w, col, func(r *vector.Rasterizer, ox, oy float32) {
		vEllipsePath(r, cx-ox, cy-oy, rx+w/2, ry+w/2, false)
		vEllipsePath(r, cx-ox, cy-oy, rx-w/2, max(ry-w/2, 0.1), true)
	})
}
