package island

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/xdg"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

func extMsg(t *testing.T, line string) proto.Message {
	t.Helper()
	m, err := proto.Decode([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func painted(img *image.RGBA) int {
	n := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 0 {
			n++
		}
	}
	return n
}

// A plugin's line competes on salience: it takes the resting pill, never
// Claude at work, and goes when the plugin does.
func TestExtPillLine(t *testing.T) {
	g := newScene()
	g.a.applyExt("pomodoro", extMsg(t, `{"type":"status","text":"Focus","sub":"session 3","icon":"timer","mood":"work","until_ms":`+
		itoa(g.clock.Add(17*time.Minute+30*time.Second).UnixMilli())+`,"progress":0.3}`))
	g.run(600 * time.Millisecond)
	l := g.a.ui.compactLine()
	if l.key != "x:pomodoro:work" || l.text != "Focus" || l.timer != "17:30" || !l.hasProg || l.m == nil || l.m.icon != "timer" {
		t.Fatalf("%+v", l)
	}
	g.work("k", "kinvault", "fix it", 2) // Claude at work outranks it
	g.run(5 * time.Second)
	if p := g.a.ui.primary(); p != "claude" {
		t.Fatalf("primary %s while Claude works", p)
	}
	g.a.extGone("pomodoro")
	if len(g.a.ui.extActs()) != 0 {
		t.Fatal("a gone plugin keeps its line")
	}
}

// `ember status --ttl` lines go by themselves.
func TestExtStatusExpires(t *testing.T) {
	g := newScene()
	g.a.applyExt("status-deploy", extMsg(t, `{"type":"status","text":"Deploying","ttl_s":60}`))
	if len(g.a.ui.extActs()) != 1 {
		t.Fatal("no line")
	}
	g.clock = g.clock.Add(61 * time.Second)
	if len(g.a.ui.extActs()) != 0 {
		t.Fatal("the line outlived its ttl")
	}
}

func TestExtMomentRowCardDashboard(t *testing.T) {
	g := newScene()
	g.a.applyExt("ci", extMsg(t, `{"type":"moment","text":"Build passed","icon":"check","mood":"done","target":"docs"}`))
	if m := g.a.ui.moment; m == nil || m.text != "Build passed" || m.ext != "ci\x00docs" || !m.until.Equal(g.clock.Add(5*time.Second)) {
		t.Fatalf("%+v", g.a.ui.moment)
	}
	g.a.ui.moment = nil
	g.a.applyExt("ci", extMsg(t, `{"type":"row","title":"Build","sub":"step 3 of 5","icon":"run","mood":"work","progress":0.6}`))
	g.a.applyExt("ci", extMsg(t, `{"type":"card","id":"deploy","title":"Deploy to prod?","body":"All green.","buttons":[{"key":"no","label":"Later"},{"key":"yes","label":"Deploy","primary":true}]}`))
	if c := g.a.ui.top(); c == nil || c.ID != "x:ci:deploy" || c.Kind != "plugin" || c.Eyebrow != "ci" || len(c.Buttons) != 2 {
		t.Fatalf("%+v", c)
	}
	g.run(1200 * time.Millisecond)
	if painted(g.a.ui.cv.img) < 2000 {
		t.Fatal("the card isn't drawn")
	}
	g.a.applyExt("ci", extMsg(t, `{"type":"remove","what":"card","id":"deploy"}`))
	if g.a.ui.top() != nil {
		t.Fatal("remove card")
	}
	// the row, in the open card
	g.a.ui.pinned = true
	g.run(1200 * time.Millisecond)
	found := false
	for _, h := range g.a.ui.hits {
		if strings.HasPrefix(h.key, "x:click:ci") {
			found = true
		}
	}
	if !found {
		t.Fatal("the row isn't in the card")
	}
	// the dashboard: every widget kind draws
	g.a.applyExt("ci", extMsg(t, `{"type":"dashboard","title":"CI","widgets":[
		{"kind":"stats","title":"Today","items":[{"label":"Builds","value":12},{"label":"Green","value":"92%","caption":"of 12"}]},
		{"kind":"sparkline","title":"Minutes a build","values":[4,5,3,6,4,7,5],"caption":"this week"},
		{"kind":"bars","title":"By repo","items":[{"label":"ember","value":7},{"label":"edge","value":5}]},
		{"kind":"chips","title":"Stages","items":[{"label":"B","state":"done"},{"label":"T","state":"tried"},{"label":"D","state":"todo"}]},
		{"kind":"list","title":"Recent","items":[{"title":"main #81","sub":"4m · passed","icon":"check","mood":"done"}]}]}`))
	if _, ok := g.a.ui.dashboards["ci"]; !ok {
		t.Fatal("no dashboard")
	}
	_, h := extDash{name: "ci"}.Size(g.a.ui)
	if h < 300 {
		t.Fatalf("dashboard height %v", h)
	}
	g.a.ui.openDash("ci")
	g.run(2 * time.Second)
	if painted(g.a.ui.cv.img) < 20000 {
		t.Fatal("the dashboard isn't drawn")
	}
	g.a.extGone("ci")
	if g.a.ui.dash != "" || len(g.a.ui.dashboards) != 0 && g.a.ui.dashboards["ci"] != nil || g.a.ui.ext["ci"] != nil {
		t.Fatal("gone: the dashboard and the row must go")
	}
}

// A plugin's cheer (the host keeps its rate) never plays over something
// waiting on you.
func TestExtCheerWaitsForYou(t *testing.T) {
	g := newScene()
	g.run(2 * time.Second)
	g.a.applyExt("p", extMsg(t, `{"type":"cheer","kind":"hop"}`))
	if g.a.ui.act == nil || g.a.ui.act.kind != "hop" {
		t.Fatal("a reaction plays")
	}
	g.run(3 * time.Second)
	g.a.ui.act = nil
	g.work("k", "kinvault", "fix it", 2)
	hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
	g.a.applyExt("p", extMsg(t, `{"type":"cheer","kind":"celebrate"}`))
	if g.a.ui.act != nil && g.a.ui.act.kind == "done" {
		t.Fatal("a celebration over a session that needs you")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// Every icon the protocol names is one Ember draws.
func TestProtocolIcons(t *testing.T) {
	for _, name := range proto.Icons {
		c := newCanvas(24, 24)
		c.icon(name, 12, 12, 15, colorWhite)
		if painted(c.img) < 12 {
			t.Errorf("icon %q draws nothing", name)
		}
	}
}

// End to end: a shell-script plugin in ~/.config/ember/plugins, started by
// the host, its status on the island's loop, and gone when it stops.
func TestExtPluginEndToEnd(t *testing.T) {
	dir := filepath.Join(xdg.ConfigHome(), "ember", "plugins", "hi")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "run.sh"), []byte(`#!/bin/sh
read hello
echo '{"type":"hello","v":1,"name":"hi"}'
echo '{"type":"status","text":"Hello from sh","mood":"done"}'
while read line; do case "$line" in *stop*) exit 0;; esac; done
`), 0o755)
	os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte("name = \"hi\"\ncommand = \"./run.sh\"\ncapabilities = [\"pill\"]\n"), 0o644)

	clock := time.Now()
	a := newTestApp(&clock)
	a.posts = make(chan func(*app), 64)
	ctx, cancel := context.WithCancel(context.Background())
	a.startPlugins(ctx, cancel)
	pump := func(until func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
			select {
			case f := <-a.posts:
				f(a)
			case <-time.After(10 * time.Millisecond):
			}
			if until() {
				return
			}
		}
		t.Fatalf("timed out: %+v", a.host.Info())
	}
	pump(func() bool { return a.ui.extStatus("hi") != nil })
	if s := a.ui.extStatus("hi"); s.Text != "Hello from sh" {
		t.Fatalf("%+v", s)
	}
	states := map[string]string{}
	for _, i := range a.host.Info() {
		states[i.Name] = i.State
	}
	if states["hi"] != "running" {
		t.Fatalf("%v", states)
	}
	go func() {
		for f := range a.posts { // the loop, while the island stops
			f(a)
		}
	}()
	a.shutdown()
	if i := a.host.Info(); i[len(i)-1].Name != "hi" || i[len(i)-1].State != "stopped" {
		t.Fatalf("%+v", i)
	}
}

// `ember note` and `ember status`, as the island hears them.
func TestOneLiners(t *testing.T) {
	g := newScene()
	note := func(text string) ipc.Msg {
		b, _ := proto.Line("moment", proto.Moment{Text: text, Icon: "check", Mood: "done", URL: "https://ci.example.com"})
		return ipc.Msg{Type: "note", Payload: b}
	}
	g.a.cli(note("Build passed"))
	if m := g.a.ui.moment; m == nil || m.text != "Build passed" || m.open != "https://ci.example.com" {
		t.Fatalf("%+v", g.a.ui.moment)
	}
	g.a.cli(note("Flood")) // within a second: coalesced away
	if g.a.ui.moment.text != "Build passed" {
		t.Fatal("notes come one a second at most")
	}
	line := func(id, text string, ttl float64) ipc.Msg {
		b, _ := proto.Line("status", proto.Status{ID: id, Text: text, TTLs: ttl})
		return ipc.Msg{Type: "line", ID: id, Payload: b}
	}
	g.a.cli(line("deploy", "Deploying", 600))
	g.a.cli(line("tests", "Testing", 60))
	if acts := g.a.ui.extActs(); len(acts) != 2 || acts[0] != "x:cli:deploy" {
		t.Fatalf("%v", acts)
	}
	g.a.cli(ipc.Msg{Type: "line", ID: "deploy", Event: "clear"})
	g.clock = g.clock.Add(2 * time.Minute) // tests' ttl runs out
	if acts := g.a.ui.extActs(); len(acts) != 0 {
		t.Fatalf("cleared and expired: %v", acts)
	}
	g.a.cli(ipc.Msg{Type: "line", ID: "Bad Id!", Payload: []byte(`{"type":"status","text":"x"}`)})
	g.a.cli(ipc.Msg{Type: "line", ID: "x", Payload: []byte(`{"type":"moment","text":"wrong type"}`)})
	if len(g.a.ui.extActs()) != 0 {
		t.Fatal("bad lines must be refused")
	}
}

// A plugin that floods: the island takes at most ten of its lines a second,
// keeps drawing at its usual cost, and the host suspends it within seconds;
// what it showed goes with it.
func TestFloodingPluginStaysOutOfTheWay(t *testing.T) {
	dir := filepath.Join(xdg.ConfigHome(), "ember", "plugins", "flood")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "run.sh"), []byte(`#!/bin/sh
read hello
echo '{"type":"hello","v":1,"name":"flood"}'
exec yes '{"type":"status","text":"flood","mood":"wait","urgency":"high"}'
`), 0o755)
	os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte("name = \"flood\"\ncommand = \"./run.sh\"\ncapabilities = [\"pill\"]\n"), 0o644)
	defer os.RemoveAll(dir)

	g := newScene()
	g.a.posts = make(chan func(*app), 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.a.startPlugins(ctx, cancel)
	start, applied := time.Now(), 0
	var frames []time.Duration
	suspended := false
	for time.Since(start) < 8*time.Second && !suspended {
		select {
		case f := <-g.a.posts:
			f(g.a)
			applied++
		default:
		}
		t0 := time.Now() // a frame, while the plugin floods
		g.clock = g.clock.Add(time.Second / 60)
		g.a.ui.renderDamage()
		frames = append(frames, time.Since(t0))
		for _, i := range g.a.host.Info() {
			suspended = suspended || i.State == "suspended"
		}
		time.Sleep(2 * time.Millisecond)
	}
	took := time.Since(start)
	if !suspended {
		t.Fatalf("not suspended after %v: %+v", took, g.a.host.Info())
	}
	if limit := int(took.Seconds()*10) + 4; applied > limit {
		t.Fatalf("%d updates reached the loop in %v", applied, took)
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i] < frames[j] })
	if med := frames[len(frames)/2]; med > 4*time.Millisecond {
		t.Fatalf("frames slowed to %v (median) while it flooded", med)
	}
	for drain := time.After(time.Second); ; { // its Gone
		select {
		case f := <-g.a.posts:
			f(g.a)
			continue
		case <-drain:
		}
		break
	}
	if g.a.ui.ext["flood"] != nil {
		t.Fatal("a suspended plugin's line stays")
	}
	t.Logf("suspended after %v; %d updates applied; frames median %v, max %v", took.Round(time.Millisecond), applied, frames[len(frames)/2], frames[len(frames)-1])
}

// critical, the clock's badge and the game face only work when granted.
func TestExtGrants(t *testing.T) {
	g := newScene()
	end := g.clock.Add(70 * time.Minute)
	g.a.applyExt("edge", extMsg(t, `{"type":"status","text":"CF 1128","mood":"work","urgency":"critical","badge":"lock","badge_clock":"lock",
		"focus":true,"satellite":true,"timer":"h:mm","warn_ms":600000,"until_ms":`+itoa(end.UnixMilli())+`,"chips":[{"label":"A","state":"done"},{"label":"B","state":"tried"}]}`))
	if s := g.a.ui.extSalience("x:edge"); s != 0.75 {
		t.Fatalf("critical without the grant: %v", s)
	}
	if g.a.ui.extFocus() || g.a.ui.extClockBadge() != "" {
		t.Fatal("focus and the clock's badge without grants")
	}
	g.a.ui.cfg.Plugins = map[string]map[string]string{"edge": {"grants": `["critical", "badge", "focus"]`}}
	if s := g.a.ui.extSalience("x:edge"); s != 0.95 || !g.a.ui.extFocus() || g.a.ui.extClockBadge() != "lock" {
		t.Fatalf("granted: %v %v %q", s, g.a.ui.extFocus(), g.a.ui.extClockBadge())
	}
	l, _ := g.a.ui.extLine("x:edge")
	if l.timer != "1:10" || !l.lock || len(l.letters) != 2 || l.letters[0].state != "done" || l.timerCol != g.a.ui.pal.fg {
		t.Fatalf("%+v", l)
	}
	g.clock = end.Add(-9 * time.Minute) // its last ten minutes
	if l, _ := g.a.ui.extLine("x:edge"); l.timerCol != g.a.ui.pal.yellow || l.timer != "0:09" {
		t.Fatalf("warn: %+v", l)
	}
	g.work("k", "kinvault", "fix it", 2) // Claude holds the pill: the plugin's clock rides the satellite
	g.run(5 * time.Second)
	if p := g.a.ui.primary(); p != "x:edge" {
		t.Fatalf("critical outranks Claude at work: %s", p)
	}
	g.a.ui.cfg.Plugins = nil
	g.run(5 * time.Second)
	if p, s := g.a.ui.primary(), g.a.ui.secondary("claude"); p != "claude" || s != "x:edge" {
		t.Fatalf("primary %s, satellite %s", p, s)
	}
}

// A post that changes nothing on screen doesn't redraw: the same line again,
// a row while the card is closed, a page that isn't open or didn't change.
func TestExtQuietPosts(t *testing.T) {
	g := newScene()
	post := func(line string) bool { // reports whether it redraws
		g.a.quiet = false
		g.a.applyExt("edge", extMsg(t, line))
		return !g.a.quiet
	}
	st := `{"type":"status","text":"2277C","sub":"18m","mood":"work"}`
	if !post(st) || post(st) || !post(`{"type":"status","text":"2277C","sub":"19m","mood":"work"}`) {
		t.Fatal("status")
	}
	row := `{"type":"row","title":"2277C","sub":"18m"}`
	if post(row) || post(`{"type":"row","title":"2277C","sub":"19m"}`) {
		t.Fatal("a row while the card is closed")
	}
	dash := `{"type":"dashboard","widgets":[{"kind":"note","text":"a"}]}`
	if !post(dash) {
		t.Fatal("the first page: its row gets a chevron")
	}
	if post(`{"type":"dashboard","widgets":[{"kind":"note","text":"b"}]}`) {
		t.Fatal("a page while it's closed")
	}
	g.a.ui.pinned = true
	g.run(600 * time.Millisecond)
	if !post(row) || post(row) {
		t.Fatal("a row in the open card")
	}
	g.a.ui.openDash("edge")
	g.run(600 * time.Millisecond)
	if !post(dash) || post(dash) {
		t.Fatal("the open page")
	}
	if post(`{"type":"remove","what":"status"}`) == false || post(`{"type":"remove","what":"status"}`) {
		t.Fatal("remove")
	}
}

// A list's or an agenda's row with a key is pressable like a button: under
// the pointer it lights, and a press sends its key as a button on the page.
func TestPressableRows(t *testing.T) {
	g := newScene()
	h := &fakeHost{}
	g.a.host = h
	g.a.ui.onKey = g.a.key
	at := strconv.FormatInt(g.clock.Add(26*time.Hour).UnixMilli(), 10)
	g.a.applyExt("edge", extMsg(t, `{"type":"dashboard","widgets":[
		{"kind":"agenda","title":"Coming","items":[{"title":"CF 1129 upsolves","at_ms":`+at+`,"key":"upsolve:cf:2261"},{"title":"ABC 482","at_ms":`+at+`}]},
		{"kind":"list","title":"Recent","items":[{"title":"2277C","sub":"WA on test 3","key":"open:cf:2277C"}]}]}`))
	g.a.ui.openDash("edge")
	g.run(2500 * time.Millisecond)
	find := func(key string) (image.Rectangle, bool) {
		for _, ht := range g.a.ui.hits {
			if ht.key == "x:dbtn:edge\x00"+key {
				return ht.r, true
			}
		}
		return image.Rectangle{}, false
	}
	n := 0
	for _, ht := range g.a.ui.hits {
		if strings.HasPrefix(ht.key, "x:dbtn:edge\x00") {
			n++
		}
	}
	r, ok := find("upsolve:cf:2261")
	if _, ok2 := find("open:cf:2277C"); !ok || !ok2 || n != 2 {
		t.Fatalf("the keyed rows' hits: %v %v, %d in all", ok, ok2, n)
	}
	cx, cy := float64(r.Min.X+r.Dx()/2), float64(r.Min.Y+r.Dy()/2)
	if redraw, _ := g.a.ui.pointer(pointerEvent{kind: "motion", x: cx, y: cy}); !redraw {
		t.Fatal("moving onto a row doesn't light it")
	}
	g.a.ui.pointer(pointerEvent{kind: "press", x: cx, y: cy, button: 1})
	g.a.ui.pointer(pointerEvent{kind: "release", x: cx, y: cy, button: 1})
	bs := h.of("button")
	if len(bs) != 1 || bs[0].Card != "dash" || bs[0].Key != "upsolve:cf:2261" {
		t.Fatalf("%+v", bs)
	}
}

// A card's choice chips: laid out by the one helper that sizes the card,
// wrapping to a second row, each hit-tested, lit under the pointer, and a
// press sends its key as a button and closes the card. Buttons stay.
func TestCardChoiceChips(t *testing.T) {
	g := newScene()
	h := &fakeHost{}
	g.a.host = h
	g.a.applyExt("edge", extMsg(t, `{"type":"card","id":"cue","title":"Tonight's rep: LC 2779","body":"Sliding window. When?",
		"chips":[{"key":"cue:now","label":"Now","primary":true},{"key":"cue:19:30","label":"19:30"},{"key":"cue:21:00","label":"21:00"},
		{"key":"skiptonight","label":"Skip tonight (1 left)"},{"key":"a","label":"Another choice"},{"key":"b","label":"And one more"}],
		"buttons":[{"key":"open","label":"Open"}]}`))
	c := g.a.ui.top()
	if c == nil || len(c.Chips) != 6 || len(c.Buttons) != 1 {
		t.Fatalf("%+v", c)
	}
	w := float32(cardW - textX - pad)
	bs, rows := layoutChoices(c.Chips, w)
	if rows != 2 {
		t.Fatalf("six chips take %d rows", rows)
	}
	for i := range c.Chips {
		if bs[i].x+bs[i].w > w || (i > 0 && bs[i].row == bs[i-1].row && bs[i].x <= bs[i-1].x+bs[i-1].w) {
			t.Fatalf("chip %d: %+v", i, bs[:len(c.Chips)])
		}
	}
	plain := &Card{Kind: "plugin", Title: c.Title, Body: c.Body, Buttons: c.Buttons}
	if d := g.a.ui.cardHeight(c) - g.a.ui.cardHeight(plain); d != int(choicesH(c.Chips, w)) || d != choiceTop+2*choiceH+choiceGap {
		t.Fatalf("the chips add %d px", d)
	}
	if choicesH(nil, w) != 0 {
		t.Fatal("no chips, no room")
	}
	if n := testing.AllocsPerRun(50, func() { choicesH(c.Chips, w) }); n != 0 {
		t.Fatalf("measuring the chips allocates %v times", n)
	}
	g.run(1500 * time.Millisecond)
	hits := map[string]image.Rectangle{}
	for _, ht := range g.a.ui.hits {
		if strings.HasPrefix(ht.key, "chip:") || strings.HasPrefix(ht.key, "btn:") {
			hits[ht.key] = ht.r
		}
	}
	if len(hits) != 7 {
		t.Fatalf("%d hits: %v", len(hits), hits)
	}
	if hits["chip:a"].Min.Y <= hits["chip:cue:now"].Max.Y || hits["btn:open"].Min.Y <= hits["chip:b"].Max.Y {
		t.Fatalf("rows out of place: %v", hits)
	}
	r := hits["chip:cue:19:30"]
	cx, cy := float64(r.Min.X+r.Dx()/2), float64(r.Min.Y+r.Dy()/2)
	if redraw, hand := g.a.ui.pointer(pointerEvent{kind: "motion", x: cx, y: cy}); !redraw || !hand {
		t.Fatalf("a chip under the pointer: redraw %v, hand %v", redraw, hand)
	}
	g.a.ui.pointer(pointerEvent{kind: "press", x: cx, y: cy, button: 1})
	g.a.ui.pointer(pointerEvent{kind: "release", x: cx, y: cy, button: 1})
	bsent := h.of("button")
	if len(bsent) != 1 || bsent[0].Card != "cue" || bsent[0].Key != "cue:19:30" {
		t.Fatalf("%+v", bsent)
	}
	if g.a.ui.top() != nil {
		t.Fatal("a picked chip closes its card")
	}
}

// Bars with an interval band draw it (rows and columns); without one they
// draw just as before.
func TestBarBandsDrawn(t *testing.T) {
	shot := func(items string) []byte {
		g := newScene()
		g.a.applyExt("edge", extMsg(t, `{"type":"dashboard","widgets":[{"kind":"bars","title":"Readiness","items":[`+items+`]},
			{"kind":"bars","layout":"columns","title":"Nightly","items":[`+items+`]}]}`))
		g.a.ui.openDash("edge")
		g.run(2500 * time.Millisecond)
		return append([]byte(nil), g.a.ui.cv.img.Pix...)
	}
	plain := shot(`{"label":"Two pointers","value":0.6,"target":0.75},{"label":"DSU","value":0.4,"target":0.75}`)
	again := shot(`{"label":"Two pointers","value":0.6,"target":0.75},{"label":"DSU","value":0.4,"target":0.75}`)
	banded := shot(`{"label":"Two pointers","value":0.6,"lo":0.45,"hi":0.8,"target":0.75},{"label":"DSU","value":0.4,"lo":0.2,"hi":0.6,"target":0.75}`)
	if !bytes.Equal(plain, again) {
		t.Skip("the page isn't steady enough to compare")
	}
	if bytes.Equal(plain, banded) {
		t.Fatal("the bands aren't drawn")
	}
}

// Row bars scale to their largest value; with a target or a band, to 0..1
// or 0..100 as the numbers fit, else the largest. And a bar's mood is its
// colour, green without one, never red.
func TestRowBarsScaleAndMood(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		items []proto.Item
		want  float64
	}{
		{nil, 1},
		{[]proto.Item{{Num: 7}, {Num: 5}}, 7},
		{[]proto.Item{{Num: 0.5, Target: f(0.75)}, {Num: 0.4}}, 1},
		{[]proto.Item{{Num: 0.4, Lo: f(0.2), Hi: f(0.6)}}, 1},
		{[]proto.Item{{Num: 52, Target: f(75)}}, 100},
		{[]proto.Item{{Num: 300, Target: f(200)}}, 300},
		{[]proto.Item{{Num: 0.9, Lo: f(0.8), Hi: f(1.2)}}, 100},
	} {
		if got := rowBarsScale(c.items); got != c.want {
			t.Errorf("%+v: scale %v, want %v", c.items, got, c.want)
		}
	}
	shot := func(mood string) []byte {
		g := newScene()
		g.a.applyExt("edge", extMsg(t, `{"type":"dashboard","widgets":[{"kind":"bars","items":[{"label":"Heap","value":0.5,"target":0.75,"mood":"`+mood+`"}]}]}`))
		g.a.ui.openDash("edge")
		g.run(2500 * time.Millisecond)
		return append([]byte(nil), g.a.ui.cv.img.Pix...)
	}
	none, done, nudge, fail := shot(""), shot("done"), shot("nudge"), shot("fail")
	if !bytes.Equal(none, done) || bytes.Equal(none, nudge) || !bytes.Equal(nudge, fail) {
		t.Fatal("a row bar's colour: green without a mood or done, amber for nudge and fail")
	}
}

// A heatmap of 8 weeks or fewer gets bigger cells, and room for them; a
// longer one is as before.
func TestShortHeatmapGrows(t *testing.T) {
	u := newScene().a.ui
	if short, long := u.heatW(&proto.Widget{Kind: "heatmap", Weeks: 4}).h, u.heatW(&proto.Widget{Kind: "heatmap", Weeks: 26}).h; long != 86 || short != 24+7*16+18 {
		t.Fatalf("heights %v and %v", short, long)
	}
}
