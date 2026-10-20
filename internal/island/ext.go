package island

import (
	"context"
	"fmt"
	"image"
	"log"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/plugin"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// External plugins: programs that speak the plugin protocol (docs/PLUGINS.md)
// through the host (package internal/plugin). What they send arrives here,
// checked and limited, and becomes the island's own things: a pill line
// that competes on salience, a moment, a row in the card, a card with
// buttons, a dashboard drawn from declared widgets, a cheer through Ember's
// own gate. Ember decides how each looks and when it shows.

// extPlugin is what one external plugin shows.
type extPlugin struct {
	name      string
	status    *proto.Status
	statusEnd time.Time // `ember status --ttl`: it goes then
	row       *proto.Row
	dash      *proto.Dashboard
}

// extAct is a plugin's activity name in the pill's competition.
func extAct(name string) string { return "x:" + name }

// extSink hands what the host checked to the island's loop.
type extSink struct {
	posts chan<- func(*app)
	ctx   context.Context
}

func (s extSink) Apply(name string, m proto.Message) {
	select {
	case s.posts <- func(a *app) { a.applyExt(name, m) }:
	case <-s.ctx.Done():
	}
}

func (s extSink) Gone(name string) {
	select {
	case s.posts <- func(a *app) { a.extGone(name) }:
	case <-s.ctx.Done():
	}
}

func extMood(s string) mood {
	switch s {
	case proto.Work:
		return moodWork
	case proto.Wait:
		return moodWait
	case proto.Nudge:
		return moodNudge
	case proto.Done:
		return moodHappy
	case proto.Fail:
		return moodError
	}
	return moodIdle
}

func (u *UI) extOf(name string) *extPlugin {
	if u.ext == nil {
		u.ext = map[string]*extPlugin{}
	}
	x := u.ext[name]
	if x == nil {
		x = &extPlugin{name: name}
		u.ext[name] = x
	}
	return x
}

// applyExt shows one message from a plugin, on the loop.
func (a *app) applyExt(name string, m proto.Message) {
	u := a.ui
	x := u.extOf(name)
	now := u.now()
	switch m.Type {
	case "status":
		x.status, x.statusEnd = m.Status, time.Time{}
		if m.Status.TTLs > 0 {
			x.statusEnd = now.Add(time.Duration(m.Status.TTLs * float64(time.Second)))
		}
	case "moment":
		mo := m.Moment
		md := extMood(mo.Mood)
		mm := moment{key: "x:" + name + ":" + mo.Text, mood: md, text: mo.Text, sub: mo.Sub, icon: mo.Icon,
			tint: u.pal.moodColor(md), until: now.Add(time.Duration(mo.TTLs * float64(time.Second))), open: mo.URL}
		if md == moodIdle {
			mm.tint = u.pal.fg
		}
		if mo.Target != "" {
			mm.ext = name + "\x00" + mo.Target
		}
		u.Moment(mm)
	case "row":
		x.row = m.Row
	case "card":
		a.extCard(name, m.Card)
	case "dashboard":
		x.dash = m.Dashboard
		if u.dashboards == nil {
			u.dashboards = map[string]Dashboard{}
		}
		u.dashboards[name] = extDash{name: name}
	case "cheer": // the host keeps the rates; Ember never cheers over something waiting on you
		if !u.waitingOnYou() {
			switch m.Cheer.Kind {
			case "hop", "nod": // reactions to something that just happened
				u.play(m.Cheer.Kind, 0)
			case "celebrate": // earned
				u.cheer(cheer{size: 0.9, flare: true})
			}
		}
	case "remove":
		switch m.Remove.What {
		case "status":
			x.status = nil
		case "row":
			x.row = nil
		case "dashboard":
			x.dash = nil
			delete(u.dashboards, name)
			if u.dash == name {
				u.closeDash()
			}
		case "card":
			u.RemoveCard("x:" + name + ":" + m.Remove.ID)
		}
	}
}

func (a *app) extCard(name string, w *proto.Card) {
	md := extMood(w.Mood)
	eyebrow := w.Eyebrow
	if eyebrow == "" {
		eyebrow = name
	}
	c := &Card{ID: "x:" + name + ":" + w.ID, Kind: "plugin", Mood: md, Eyebrow: eyebrow, Title: w.Title, Body: w.Body,
		Priority: 2, Expires: a.ui.now().Add(time.Duration(w.TTLs * float64(time.Second)))}
	if w.Urgent && a.ui.granted(name, proto.GrantCritical) {
		c.Priority = 5 // before Claude's own requests
	}
	for _, b := range w.Buttons {
		c.Buttons = append(c.Buttons, Button{Key: b.Key, Label: b.Label, Primary: b.Primary})
	}
	id := w.ID
	c.OnPick = func(k string, _ []int) {
		if a.host == nil {
			return
		}
		if k == "dismiss" {
			a.host.CardClosed(name, id)
			return
		}
		a.host.Send(name, proto.Event{Type: "button", Card: id, Key: k})
	}
	c.OnExpire = func() {
		if a.host != nil {
			a.host.CardClosed(name, id)
		}
	}
	a.ui.AddCard(c)
}

// extGone takes back everything a plugin showed: it stopped.
func (a *app) extGone(name string) {
	u := a.ui
	delete(u.ext, name)
	delete(u.dashboards, name)
	if u.dash == name {
		u.closeDash()
	}
	for _, c := range append([]*Card(nil), u.cards...) {
		if strings.HasPrefix(c.ID, "x:"+name+":") {
			u.RemoveCard(c.ID)
		}
	}
	if m := u.moment; m != nil && strings.HasPrefix(m.key, "x:"+name+":") {
		u.moment = nil
	}
}

// extKey handles a click on a plugin's row or moment ("x:click:<name>\x00<target>"),
// or a button on its row ("x:btn:<name>\x00<key>") or its dashboard ("x:dbtn:…").
func (a *app) extKey(k string) {
	if a.host == nil {
		return
	}
	switch {
	case strings.HasPrefix(k, "x:btn:"):
		name, key, _ := strings.Cut(strings.TrimPrefix(k, "x:btn:"), "\x00")
		a.host.Send(name, proto.Event{Type: "button", Card: "row", Key: key})
	case strings.HasPrefix(k, "x:dbtn:"):
		name, key, _ := strings.Cut(strings.TrimPrefix(k, "x:dbtn:"), "\x00")
		a.host.Send(name, proto.Event{Type: "button", Card: "dash", Key: key})
	default:
		name, target, _ := strings.Cut(strings.TrimPrefix(k, "x:click:"), "\x00")
		a.host.Click(name, target)
	}
}

// extDashChanged tells plugins their dashboard opened or closed.
func (a *app) extDashChanged() {
	if a.ui.dash == a.lastDash {
		return
	}
	if a.host != nil {
		if was := a.lastDash; a.ui.ext[was] != nil {
			a.host.Send(was, proto.Event{Type: "dash", State: "close"})
		}
		if now := a.ui.dash; a.ui.ext[now] != nil {
			a.host.Send(now, proto.Event{Type: "dash", State: "open"})
		}
	}
	a.lastDash = a.ui.dash
}

// ---- in the pill ----

// extNames is the plugins showing something, in a stable order.
func (u *UI) extNames() []string {
	var out []string
	for n := range u.ext {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// extStatus is a plugin's live pill line, if it has one (and it hasn't run out).
func (u *UI) extStatus(name string) *proto.Status {
	x := u.ext[name]
	if x == nil || x.status == nil || (!x.statusEnd.IsZero() && u.now().After(x.statusEnd)) {
		return nil
	}
	return x.status
}

// extActs are the activities plugins hold in the pill.
func (u *UI) extActs() []string {
	var out []string
	for _, n := range u.extNames() {
		if u.extStatus(n) != nil {
			out = append(out, extAct(n))
		}
	}
	return out
}

// granted says whether the user's config grants a plugin a power.
func (u *UI) granted(name, grant string) bool {
	return slices.Contains(u.cfg.PluginGrants(name), grant)
}

// waitingOnYou: a request from Claude, or a session that needs you.
func (u *UI) waitingOnYou() bool {
	if c := u.top(); c != nil && (c.Kind == "perm" || c.Kind == "ask") {
		return true
	}
	ss := u.sessionList()
	return len(ss) > 0 && ss[0].State == "wait"
}

// extSalience: what a plugin says it is (low, normal, high, or exactly), a
// little more when it needs you; never above Claude at work (0.75), unless
// the user granted it critical (0.95: still below anything waiting on you).
func (u *UI) extSalience(act string) float64 {
	name := strings.TrimPrefix(act, "x:")
	s := u.extStatus(name)
	if s == nil {
		return 0
	}
	top := 0.75
	if u.granted(name, proto.GrantCritical) {
		top = 0.95
	}
	if s.Salience != nil {
		return min(*s.Salience, top)
	}
	v := map[string]float64{"low": 0.3, "normal": 0.45, "high": 0.6, "critical": 0.95}[s.Urgency]
	if s.Mood == proto.Wait {
		v += 0.15
	}
	return min(v, top)
}

// extFocus: a plugin's line asks for Ember's game face (and is granted it).
func (u *UI) extFocus() bool {
	for _, n := range u.extNames() {
		if s := u.extStatus(n); s != nil && s.Focus && u.granted(n, proto.GrantFocus) {
			return true
		}
	}
	return false
}

// extClockBadge is the icon a plugin puts on the clock, if one is granted it.
func (u *UI) extClockBadge() string {
	for _, n := range u.extNames() {
		if s := u.extStatus(n); s != nil && s.BadgeClock != "" && u.granted(n, proto.GrantBadge) {
			return s.BadgeClock
		}
	}
	return ""
}

// extSatellite is a plugin whose timer may ride in the satellite.
func (u *UI) extSatellite(primary string) string {
	for _, n := range u.extNames() {
		if s := u.extStatus(n); s != nil && s.Satellite && extAct(n) != primary && extTimer(s, u.now()) != "" {
			return extAct(n)
		}
	}
	return ""
}

// extWarn: within the line's warn_ms of its until_ms.
func extWarn(s *proto.Status, now time.Time) bool {
	return s.UntilMs > 0 && s.WarnMs > 0 && time.UnixMilli(s.UntilMs).Sub(now) <= time.Duration(s.WarnMs)*time.Millisecond
}

// extTimer is the line's clock: time left ("17:59", "1h 05m"; "1:12" as
// h:mm) or time spent ("12m"), or none.
func extTimer(s *proto.Status, now time.Time) string {
	switch {
	case s.Timer == "none":
		return ""
	case s.Timer == "h:mm" && s.UntilMs > 0:
		return left(now, time.UnixMilli(s.UntilMs))
	case s.UntilMs > 0:
		d := max(0, time.UnixMilli(s.UntilMs).Sub(now))
		if d >= time.Hour {
			return mins(d)
		}
		return clock(d + time.Second - 1)
	case s.SinceMs > 0:
		return mins(now.Sub(time.UnixMilli(s.SinceMs)))
	}
	return ""
}

func (u *UI) extLine(act string) (line, bool) {
	s := u.extStatus(strings.TrimPrefix(act, "x:"))
	if s == nil {
		return line{}, false
	}
	p, now := u.pal, u.now()
	md := extMood(s.Mood)
	l := line{key: act + ":" + s.Mood, mood: md, text: s.Text, sub: s.Sub, textCol: p.fg, timer: extTimer(s, now), timerCol: p.muted,
		chip: s.Value, chipCol: p.muted, lock: s.Badge == "lock", flag: s.Badge, letters: chipLetters(s.Chips)}
	if l.lock {
		l.flag = ""
	}
	if s.UntilMs > 0 { // a countdown reads as a clock
		l.timerCol = p.fg
	}
	if extWarn(s, now) { // its last minutes
		l.timerCol, l.subCol = p.yellow, p.yellow
	}
	if md == moodWait {
		l.textCol = p.blue
	}
	if s.Icon != "" {
		tint := p.moodColor(md)
		if md == moodIdle {
			tint = p.fg
		}
		l.m = &moment{icon: s.Icon, tint: tint}
	}
	if s.Progress != nil {
		l.prog, l.hasProg = *s.Progress, true
	}
	return l, true
}

// ---- in the card ----

const extH = edgeH

// extRows are the plugins with a row in the card.
func (u *UI) extRows() []*extPlugin {
	var out []*extPlugin
	for _, n := range u.extNames() {
		if x := u.ext[n]; x.row != nil {
			out = append(out, x)
		}
	}
	return out
}

// extRow: a plugin on one line, the way edge's row looks: a ring or its
// icon on the rail, the title, its letters and badge, the words (a small
// flame before the streak), a thin bar for its progress, and its buttons or,
// when the line opens its dashboard, a chevron.
func (u *UI) extRow(x *extPlugin, rx, w, y float32) {
	p, r, now := u.pal, x.row, u.now()
	on := u.rowHot("x:"+x.name, rx, w, y, extH)
	_, hasDash := u.dashboards[x.name]
	key := "x:click:" + x.name + "\x00" + r.Target
	if hasDash {
		key = "dash:" + x.name
		if on > 0.01 {
			u.panel(rx+pad-8, y+2, w-2*pad+16, extH-4, radRow, alpha(p.fg, 0.05*on))
		}
	}
	u.hits = append(u.hits, hit{image.Rect(int(rx+pad-8), int(y), int(rx+w-pad+8), int(y+extH)), key})
	cy, md := y+extH/2, extMood(r.Mood)
	gx := rx + railX
	switch g := r.Ring; {
	case g != nil: // a ring on the rail: filled to its value, or emptying with its time
		col := p.moodColor(extMood(g.Mood))
		frac, k := 0.0, 1.0
		if g.Value != nil {
			frac = *g.Value
			k = u.tw("ring:"+x.name, frac, 0)
		} else if span := time.UnixMilli(g.EndMs).Sub(time.UnixMilli(g.StartMs)); span > 0 {
			frac = math.Max(0, math.Min(1, float64(time.UnixMilli(g.EndMs).Sub(now))/float64(span)))
		}
		u.dc.arc(gx, cy, 6, 1.8, 0, 2*math.Pi, alpha(p.fg, 0.12))
		if g.Value != nil {
			if v := frac * k; v > 0.01 {
				ringArc(u.dc, gx, cy, 6, 1.8, -math.Pi/2, -math.Pi/2+2*math.Pi*v, col)
			}
		} else if frac > 0.01 {
			ringArc(u.dc, gx, cy, 6, 1.8, -math.Pi/2, -math.Pi/2+2*math.Pi*frac, col)
		}
	case r.Icon != "":
		tint := p.moodColor(md)
		if md == moodIdle {
			tint = mix(p.muted, p.fg, 0.4)
		}
		u.dc.icon(r.Icon, gx, cy, 13, tint)
	default:
		u.dc.circle(gx, cy, 3, mix(p.muted, p.fg, 0.4))
	}
	tx, right := rx+textX, rx+w-pad
	for i := len(r.Buttons) - 1; i >= 0; i-- {
		b := r.Buttons[i]
		bw := u.pillButton(Button{Key: b.Key, Label: b.Label, Primary: b.Primary}, "x:btn:"+x.name+"\x00"+b.Key, right, cy-btnH/2, btnH, p.yellow)
		right -= bw + 8
	}
	switch {
	case len(r.Buttons) > 0:
		right -= 2
	case hasDash: // the line opens the dashboard: say so
		u.chevron(right-3, cy, 0, mix(p.faint, p.fg, 0.4*on))
		right -= 16
	}
	base := cy + 4.5
	nw := u.dc.text(fBody(), ellipsize(fBody(), r.Title, (right-tx)*0.55), tx, base, p.fg)
	sx := tx + nw + 8
	if ls := chipLetters(r.Chips); len(ls) > 0 && sx+lettersW(ls, 14)+60 < right {
		sx += u.letters(ls, sx, cy, 14, 1) + 8
	}
	switch r.Badge {
	case "":
	case "lock":
		u.dc.lock(sx, cy-5.5, 10, p.yellow)
		sx += 15
	default:
		u.dc.icon(r.Badge, sx+5, cy, 10, p.yellow)
		sx += 15
	}
	sub := r.Sub
	if f := r.Flame; f != "" && strings.HasSuffix(sub, f) && len(sub) > len(f) { // a small live flame before it
		sx += u.dc.text(fSecond(), sub[:len(sub)-len(f)], sx, base, p.muted)
		fx, fy := sx+4, cy+1
		u.lives = append(u.lives, live{r: image.Rect(int(fx-6), int(fy-10), int(fx+7), int(fy+6)), draw: func(c *canvas, t float64) {
			flame(c, fx, fy, 4.6, t, true, p)
		}})
		sx += 11
		sub = f
	}
	if sub != "" {
		u.dc.text(fSecond(), ellipsize(fSecond(), sub, right-sx), sx, base, p.muted)
	}
	if r.Progress != nil {
		bw := right - tx
		u.dc.roundRect(tx, y+extH-5, bw, 2, 1, alpha(p.fg, 0.08))
		if k := float32(*r.Progress); k > 0 {
			u.dc.roundRect(tx, y+extH-5, max(bw*k, 2), 2, 1, p.moodColor(md))
		}
	}
}

// ---- the dashboard ----

// extDash draws a plugin's declared widgets in the dashboard's language:
// a title, then sections under small capitals, divided by hairlines.
type extDash struct{ name string }

const extDashW = dashW

type block struct {
	h    float32
	draw func(y float32, age time.Duration)
}

func (d extDash) blocks(u *UI, x, w float32) []block {
	xp := u.ext[d.name]
	if xp == nil || xp.dash == nil {
		return nil
	}
	p := u.pal
	db := xp.dash
	title := db.Title
	if title == "" {
		title = d.name
	}
	inner, lx := w-2*dashP, x+dashP
	out := []block{{h: 46, draw: func(y float32, age time.Duration) {
		u.dc.text(fTitle(), ellipsize(fTitle(), title, inner), lx, y+30, alpha(p.fg, phase(age, 0, 300*time.Millisecond)))
	}}}
	for i, wd := range db.Widgets {
		wd, delay := wd, time.Duration(80+60*i)*time.Millisecond
		head := func(y float32, k float64) {
			if wd.Title != "" {
				u.dc.eyebrow(wd.Title, lx, y+14, alpha(p.faint, k))
			}
		}
		top := float32(0)
		if wd.Title != "" {
			top = 20
		}
		out = append(out, block{h: dashDiv, draw: func(y float32, age time.Duration) {
			dashDivider(u, x, w, y, phase(age, delay, 400*time.Millisecond))
		}})
		switch wd.Kind {
		case "stats":
			out = append(out, block{h: top + 52, draw: func(y float32, age time.Duration) {
				k := phase(age, delay, 500*time.Millisecond)
				head(y, k)
				n := max(len(wd.Items), 1)
				col := inner / float32(n)
				for j, it := range wd.Items {
					cx := lx + float32(j)*col
					u.dc.eyebrow(it.Label, cx, y+top+12, alpha(p.faint, k))
					f := face(fSemiBold, 20)
					vw := u.dc.text(f, ellipsize(f, it.Text, col-8), cx, y+top+36, alpha(p.fg, k))
					if it.Caption != "" {
						u.dc.text(fCaption(), ellipsize(fCaption(), it.Caption, col-vw-14), cx+vw+6, y+top+36, alpha(p.muted, k))
					}
				}
			}})
		case "sparkline":
			out = append(out, block{h: top + 62, draw: func(y float32, age time.Duration) {
				k := phase(age, delay, 700*time.Millisecond)
				head(y, k)
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
					pts := []pt{{lx, gy + gh}}
					for j, v := range vs[:shown] {
						pts = append(pts, pt{lx + inner*float32(j)/n, gy + gh - gh*float32((v-lo)/(hi-lo))})
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
					u.dc.text(fCaption(), ellipsize(fCaption(), wd.Caption, inner), lx, y+top+56, alpha(p.muted, k))
				}
			}})
		case "bars":
			rows := float32(len(wd.Items))
			out = append(out, block{h: top + rows*20 + 4, draw: func(y float32, age time.Duration) {
				head(y, phase(age, delay, 400*time.Millisecond))
				top0 := 0.0
				for _, it := range wd.Items {
					top0 = math.Max(top0, it.Num)
				}
				if top0 == 0 {
					top0 = 1
				}
				labelW, valueW := inner*0.32, float32(56)
				for j, it := range wd.Items {
					k := float32(phase(age, delay+time.Duration(50*j)*time.Millisecond, 600*time.Millisecond))
					ry := y + top + float32(j)*20
					u.dc.text(fSecond(), ellipsize(fSecond(), it.Label, labelW-8), lx, ry+14, p.muted)
					bx, bw := lx+labelW, inner-labelW-valueW
					u.dc.roundRect(bx, ry+7, bw, 6, 3, alpha(p.fg, 0.06))
					if fw := bw * float32(it.Num/top0) * k; fw > 0.5 {
						u.dc.roundRect(bx, ry+7, max(fw, 6), 6, 3, p.green)
					}
					v := it.Caption
					if v == "" {
						v = trimNum(it.Num)
					}
					f := face(fSemiBold, 11)
					u.dc.text(f, v, lx+inner-measure(f, v), ry+14, p.fg)
				}
			}})
		case "chips":
			out = append(out, block{h: top + 30, draw: func(y float32, age time.Duration) {
				k := phase(age, delay, 400*time.Millisecond)
				head(y, k)
				u.letters(chipsAsLetters(wd.Items), lx, y+top+13, 18, k)
				if wd.Caption != "" {
					cx := lx + lettersW(chipsAsLetters(wd.Items), 18) + 12
					u.dc.text(fCaption(), ellipsize(fCaption(), wd.Caption, lx+inner-cx), cx, y+top+17, alpha(p.muted, k))
				}
			}})
		case "list":
			out = append(out, block{h: top + float32(len(wd.Items))*24 + 2, draw: func(y float32, age time.Duration) {
				head(y, phase(age, delay, 400*time.Millisecond))
				for j, it := range wd.Items {
					k := phase(age, delay+time.Duration(40*j)*time.Millisecond, 400*time.Millisecond)
					ry := y + top + float32(j)*24
					md := extMood(it.Mood)
					tint := p.moodColor(md)
					if md == moodIdle {
						tint = p.muted
					}
					if it.Icon != "" {
						u.dc.icon(it.Icon, lx+6, ry+11, 11, alpha(tint, k))
					} else {
						u.dc.circle(lx+6, ry+11, 2.2, alpha(tint, k))
					}
					tw := u.dc.text(fBody(), ellipsize(fBody(), it.Title, inner*0.55), lx+20, ry+16, alpha(p.fg, k))
					if it.Sub != "" {
						u.dc.text(fSecond(), ellipsize(fSecond(), it.Sub, inner-tw-28), lx+28+tw, ry+16, alpha(p.muted, k))
					}
				}
			}})
		}
	}
	return append(out, block{h: 8})
}

// chipLetters speaks the protocol's chip states in the letters' own.
func chipLetters(cs []proto.Chip) []EdgeProblem {
	if len(cs) == 0 {
		return nil
	}
	state := map[string]string{"done": "solved", "tried": "tried", "owed": "upsolve", "fixed": "upsolved", "gone": "lapsed"}
	ps := make([]EdgeProblem, len(cs))
	for i, c := range cs {
		ps[i] = EdgeProblem{Index: c.Label, State: state[c.State]}
	}
	return ps
}

// chipsAsLetters speaks the round chips' states: the dashboard draws a
// plugin's chips the way it draws edge's letters.
func chipsAsLetters(items []proto.Item) []EdgeProblem {
	state := map[string]string{"done": "solved", "tried": "tried", "owed": "upsolve", "fixed": "upsolved", "gone": "lapsed"}
	ps := make([]EdgeProblem, len(items))
	for i, it := range items {
		ps[i] = EdgeProblem{Index: it.Label, State: state[it.State]}
	}
	return ps
}

func trimNum(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e9 {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}

func (d extDash) Size(u *UI) (float64, float64) {
	h := float32(0)
	for _, b := range d.blocks(u, 0, extDashW) {
		h += b.h
	}
	return extDashW, float64(max(h, 96))
}

func (d extDash) Draw(u *UI, x, w float32, age time.Duration) bool {
	y := float32(0)
	for _, b := range d.blocks(u, x, w) {
		if b.draw != nil {
			b.draw(y, age)
		}
		y += b.h
	}
	return age < 1500*time.Millisecond
}

var _ plugin.Sink = extSink{}

// ---- one-liners: `ember note`, `ember status` ----

// cliEvery is how often a one-liner may change what shows: notes are
// coalesced to one a second, each status line to ten.
var cliEvery = map[string]time.Duration{"note": time.Second, "line": 100 * time.Millisecond}

// cli shows a one-liner. Notes are the plugin "note"; each status line is
// its own "cli:<id>" (no plugin can have a colon in its name).
func (a *app) cli(m ipc.Msg) {
	key, name := m.Type, "note"
	if m.Type == "line" {
		if !proto.NameRe.MatchString(m.ID) {
			return
		}
		key, name = "status:"+m.ID, "cli:"+m.ID
		if m.Event == "clear" {
			if x := a.ui.ext[name]; x != nil {
				delete(a.ui.ext, name)
			}
			return
		}
	}
	now := a.ui.now()
	if a.cliLast == nil {
		a.cliLast = map[string]time.Time{}
	}
	if now.Sub(a.cliLast[key]) < cliEvery[m.Type] {
		a.quiet = true
		return
	}
	msg, err := proto.Decode(m.Payload)
	switch {
	case err != nil:
		log.Printf("ember %s: %v", map[string]string{"note": "note", "line": "status"}[m.Type], err)
		return
	case m.Type == "note" && msg.Type == "moment", m.Type == "line" && msg.Type == "status":
		a.cliLast[key] = now
		a.applyExt(name, msg)
	}
}
