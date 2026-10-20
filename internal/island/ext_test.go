package island

import (
	"context"
	"image"
	"os"
	"path/filepath"
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
	if l.key != "x:pomodoro" || l.text != "Focus" || l.timer != "17:30" || !l.hasProg || l.m == nil || l.m.icon != "timer" {
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

// A plugin's cheer goes through Ember's own gate: once, then not again soon.
func TestExtCheerIsGated(t *testing.T) {
	g := newScene()
	g.a.ui.bond = loadBond(g.clock)
	g.run(2 * time.Second)
	g.a.applyExt("p", extMsg(t, `{"type":"cheer","kind":"hop"}`))
	if g.a.ui.act == nil || g.a.ui.act.kind != "hop" {
		t.Fatal("a calm Ember hops for a plugin's first cheer")
	}
	g.run(3 * time.Second)
	g.a.ui.act = nil
	g.a.applyExt("p", extMsg(t, `{"type":"cheer","kind":"hop"}`))
	if g.a.ui.act != nil {
		t.Fatal("a second cheer right away got through")
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
	os.MkdirAll(filepath.Join(xdg.ConfigHome(), "ember", "plugins", "edge"), 0o755) // a built-in's name: refused
	os.WriteFile(filepath.Join(xdg.ConfigHome(), "ember", "plugins", "edge", "plugin.toml"), []byte("name = \"edge\"\ncommand = \"true\"\n"), 0o644)

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
	if states["hi"] != "running" || states["edge"] != "broken" {
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
