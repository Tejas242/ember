package island

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// testLines are lines written for these tests.
var testLines = []proto.TimedLine{
	{TMs: 10000, Text: "the kettle sings at seven"},
	{TMs: 14000, Text: ""},
	{TMs: 20000, Text: "a small flame keeps the time"},
	{TMs: 20500, Text: "fast"},
	{TMs: 24000, Text: "the window learns the morning", Words: []proto.TimedWord{{TMs: 24000, N: 4}, {TMs: 25000, N: 7}, {TMs: 26000, N: 17}}},
}

// stageScene is a stage open on a song, its words sent by a plugin.
func stageScene(t testing.TB, lines []proto.TimedLine, key func(*Track) string, mode, state string) *galleryScene {
	g := newScene()
	g.work("k", "kinvault", "fix the race", 2)
	g.music(true)
	g.a.ui.audio = &source.Audio{}
	tt := map[string]any{"kind": "timed_text", "media_key": key(g.a.ui.track), "lines": lines, "mode": mode, "state": state}
	b, _ := json.Marshal(map[string]any{"type": "dashboard", "size": "card", "sticky": true, "widgets": []any{
		map[string]any{"kind": "stage", "ember": map[string]any{"dance": "music", "sing": true}},
		map[string]any{"kind": "sessions"}, tt, map[string]any{"kind": "audio"}, map[string]any{"kind": "media"}}})
	m, err := proto.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	g.a.ui.cfg.Plugins = map[string]map[string]string{"vibe": {"grants": `["media"]`}}
	g.a.applyExt("vibe", m)
	g.a.ui.openDash("vibe")
	g.run(600 * time.Millisecond)
	return g
}

func stageAt(g *galleryScene, pos time.Duration) (*stageState, stageWidgets) {
	g.a.ui.track.Position, g.a.ui.track.PosAt = pos, g.clock
	g.run(time.Second / 30)
	st, sw, _ := g.a.ui.stageNow()
	return st, sw
}

// The words follow the song's playhead, a little ahead; the words of another
// song never show on this one.
func TestTimedTextClock(t *testing.T) {
	g := stageScene(t, testLines, mediaKey, "synced", "ready")
	st, sw := stageAt(g, 11*time.Second)
	if k := g.a.ui.stageKind(st, sw); k != "synced" || st.idx != 0 || st.fillK <= 0 || st.fillK >= 1 {
		t.Fatalf("kind %s, line %d, fill %.2f", k, st.idx, st.fillK)
	}
	st, _ = stageAt(g, 13900*time.Millisecond) // 150 ms ahead: the break has begun
	if st.idx != 1 {
		t.Fatalf("line %d at 13.9 s", st.idx)
	}
	ls := st.stagedLines(sw.text)
	if len(ls) != 4 || !ls[2].fast || ls[2].text != "a small flame keeps the time fast" {
		t.Fatalf("lines under 0.9 s apart are sung as one: %+v", ls)
	}
	if f := lineFill(ls, 3, 25500*time.Millisecond); f < 0.25 || f > 0.35 { // half of the second word: (4 + 3.5) / 28
		t.Fatalf("by its words' times: %.2f", f)
	}

	g = stageScene(t, testLines, func(*Track) string { return "another-song" }, "synced", "ready")
	st, sw = stageAt(g, 11*time.Second)
	if k := g.a.ui.stageKind(st, sw); k != "loading" || st.idx != -2 || st.fillK != -1 {
		t.Fatalf("another song's words showed: %s line %d", k, st.idx)
	}
	if _, ok := g.a.ui.wordsPos(sw.text); ok {
		t.Fatal("a clock for another song's words")
	}

	// a song change while it's open: its title, then the words when they're its own
	g = stageScene(t, testLines, mediaKey, "synced", "ready")
	g.a.ui.track = &Track{Player: "p", Title: "Next", Artist: "Someone", Playing: true, Length: time.Minute, PosAt: g.clock}
	g.a.ui.Invalidate()
	st, sw = stageAt(g, 11*time.Second)
	if k := g.a.ui.stageKind(st, sw); k != "change" {
		t.Fatalf("a new song: %s", k)
	}
	g.run(3 * time.Second)
	if k := g.a.ui.stageKind(st, sw); k != "loading" {
		t.Fatalf("after its title card, the old words wait as loading: %s", k)
	}
}

// Its own clock, for words that don't follow a song.
func TestTimedTextOwnClock(t *testing.T) {
	g := newScene()
	w := &proto.Widget{Kind: "timed_text", Clock: &proto.Clock{PosMs: 1000, AtMs: g.clock.UnixMilli(), Rate: 2}}
	g.clock = g.clock.Add(time.Second)
	if pos, ok := g.a.ui.wordsPos(w); !ok || pos != 3*time.Second {
		t.Fatalf("%v %v", pos, ok)
	}
}

// Every case's column.
func TestStageKinds(t *testing.T) {
	for _, c := range []struct {
		mode, state, want string
		set               func(g *galleryScene)
	}{
		{"synced", "loading", "loading", nil},
		{"synced", "none", "none", nil},
		{"synced", "instrumental", "instrumental", nil},
		{"synced", "spoken", "spoken", nil},
		{"stanzas", "ready", "stanzas", nil},
		{"synced", "ready", "stanzas", func(g *galleryScene) { g.a.ui.track.Length = 0; g.a.ui.stage = nil }}, // a browser: no playhead to follow
		{"synced", "ready", "hidden", func(g *galleryScene) { g.a.ui.shared, g.a.ui.cfg.Discreet = true, true }},
	} {
		g := stageScene(t, testLines, mediaKey, c.mode, c.state)
		if c.set != nil {
			c.set(g)
			g.a.applyExt("vibe", extMsg(t, mustLine(t, g)))
		}
		st, sw := stageAt(g, 11*time.Second)
		if k := g.a.ui.stageKind(st, sw); k != c.want {
			t.Errorf("%s/%s: %s, want %s", c.mode, c.state, k, c.want)
		}
	}
}

// mustLine is the page re-sent for the scene's current song (its key may change with it).
func mustLine(t *testing.T, g *galleryScene) string {
	b, _ := json.Marshal(map[string]any{"type": "dashboard", "widgets": []any{map[string]any{"kind": "stage"},
		map[string]any{"kind": "timed_text", "media_key": mediaKey(g.a.ui.track), "lines": testLines}}})
	return string(b)
}

// Ember on a still stage doesn't dance; on a music stage it may.
func TestStageStill(t *testing.T) {
	g := stageScene(t, testLines, mediaKey, "synced", "ready")
	if g.a.ui.stageStill() {
		t.Fatal("a music stage is still")
	}
	g.a.applyExt("vibe", extMsg(t, `{"type":"dashboard","widgets":[{"kind":"stage","ember":{"dance":"still","pose":"sleepy"}}]}`))
	g.run(time.Second / 30)
	if !g.a.ui.stageStill() || g.a.ui.danceable() {
		t.Fatal("a still stage lets Ember dance")
	}
}

// The frame rate follows the song: 24 on a beat, 15 swaying, 8 paused, 12
// calm, less on battery.
func TestStageFPS(t *testing.T) {
	u := NewUI(100, 100, 24)
	u.track = &Track{Playing: true}
	u.dancing = true
	for _, c := range []struct {
		set  func()
		want int
	}{
		{func() {}, 24},
		{func() { u.dancing = false }, 15},
		{func() { u.track.Playing = false }, 8},
		{func() { u.track.Playing, u.dancing, u.cfg.Calm = true, true, true }, 12},
		{func() { u.cfg.Calm, u.power = false, &power{state: 2, pct: 50} }, 15},
		{func() { u.power.pct = 12 }, 10},
	} {
		c.set()
		if got := u.stageFPS(); got != c.want {
			t.Fatalf("%d fps, want %d", got, c.want)
		}
	}
}

// Scrolling over the words moves them a quarter second, remembered for the song.
func TestNudgeWords(t *testing.T) {
	g := stageScene(t, testLines, mediaKey, "synced", "ready")
	key := mediaKey(g.a.ui.track)
	g.a.ui.nudgeWords(-1)
	g.a.ui.nudgeWords(-1)
	if n := g.a.ui.wordsNudge(key); n != 500*time.Millisecond {
		t.Fatalf("%v", n)
	}
	for i := 0; i < 40; i++ {
		g.a.ui.nudgeWords(1)
	}
	if n := g.a.ui.wordsNudge(key); n != -5*time.Second {
		t.Fatalf("capped at five seconds: %v", n)
	}
	if _, err := os.Stat(nudgesPath()); err != nil {
		t.Fatal("not remembered")
	}
	g.a.ui.nudge = nil
	if n := g.a.ui.wordsNudge(key); n != -5*time.Second {
		t.Fatalf("read back: %v", n)
	}
}

// BenchmarkStageSteady: a stage's steady frames, at 24 a second, on a song.
func BenchmarkStageSteady(b *testing.B) { benchStage(b, testLines, 0) }

// BenchmarkStageSinging: the same, a line being sung (the playhead kept
// within it), its fill moving every frame.
func BenchmarkStageSinging(b *testing.B) { benchStage(b, testLines, 10500*time.Millisecond) }

// benchStage: a stage's frames at 24 a second; with sing, the playhead loops
// over the three seconds from there.
func benchStage(b *testing.B, lines []proto.TimedLine, sing time.Duration) {
	g := stageScene(b, lines, mediaKey, "synced", "ready")
	lv := stageLevels()
	for i := 0; i < 240; i++ {
		g.a.ui.audio.Feed(lv[i*25/60%len(lv)], time.Now())
		g.run(time.Second / 60)
	}
	b.ResetTimer()
	full := 0
	for i := 0; i < b.N; i++ {
		g.a.ui.audio.Feed(lv[(240+i)*25/24%len(lv)], time.Now())
		g.clock = g.clock.Add(time.Second / 24)
		if sing > 0 {
			g.a.ui.track.Position, g.a.ui.track.PosAt = sing+time.Duration(i%72)*time.Second/24, g.clock
		}
		g.a.ui.renderDamage()
		if g.a.ui.lastFull {
			full++
		}
	}
	b.ReportMetric(float64(full)/float64(b.N), "full/op")
}

// stageLevels are a real song's levels (testdata), for the benchmark.
func stageLevels() [][3]float64 {
	var lv [][3]float64
	data, _ := os.ReadFile("testdata/song-levels.json")
	json.Unmarshal(data, &lv)
	if len(lv) == 0 {
		lv = [][3]float64{{0.5, 0.4, 0.3}}
	}
	return lv
}
