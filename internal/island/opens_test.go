package island

import (
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/plugin"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// fakeHost records what the island sends its plugins.
type fakeHost struct {
	sent  []proto.Event
	to    []string
	opens map[string]string // context → plugin
}

func (h *fakeHost) Send(name string, ev proto.Event) {
	h.to, h.sent = append(h.to, name), append(h.sent, ev)
}
func (h *fakeHost) CardClosed(name, id string)       {}
func (h *fakeHost) Click(name, target string)        {}
func (h *fakeHost) Has(name string) bool             { return true }
func (h *fakeHost) Opener(c string) (string, string) { return h.opens[c], "Vibe" }
func (h *fakeHost) Info() []plugin.Info              { return nil }
func (h *fakeHost) Scan()                            {}
func (h *fakeHost) Stop()                            {}

func (h *fakeHost) of(typ string) []proto.Event {
	var out []proto.Event
	for _, ev := range h.sent {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

// stagePage gives a plugin a sticky card-sized page and the grants it asks for.
func stagePage(t *testing.T, g *galleryScene, name string, grants string, page string) *fakeHost {
	h := &fakeHost{opens: map[string]string{"music": name}}
	g.a.host, g.a.ui.openers = h, h.Opener
	if g.a.ui.cfg.Plugins == nil {
		g.a.ui.cfg.Plugins = map[string]map[string]string{}
	}
	g.a.ui.cfg.Plugins[name] = map[string]string{"grants": grants}
	g.a.applyExt(name, extMsg(t, page))
	return h
}

const vibePage = `{"type":"dashboard","title":"Vibe","size":"card","sticky":true,"ends":{"media_idle_s":120,"after_claude":"song_end"},"widgets":[{"kind":"stage"},{"kind":"timed_text","state":"loading"},{"kind":"media"}]}`

// loopOnce is what the loop does after a frame: plugins hear of their page and the song.
func loopOnce(g *galleryScene) {
	g.run(time.Second / 30)
	g.a.extDashChanged()
	g.a.extMedia()
}

// A plugin granted media hears about the song only while its page is open:
// once as it opens, then on a change; nothing once it's closed.
func TestMediaWhileOpen(t *testing.T) {
	g := newScene()
	g.music(true)
	h := stagePage(t, g, "vibe", `["media"]`, vibePage)
	for i := 0; i < 5; i++ {
		loopOnce(g)
	}
	if n := len(h.sent); n != 0 {
		t.Fatalf("%d events before the page opened", n)
	}
	g.a.ui.openDash("vibe")
	for i := 0; i < 30; i++ {
		loopOnce(g)
	}
	ms := h.of("media")
	if len(ms) != 1 || ms[0].Media == nil || ms[0].Title != "Instant Crush" || ms[0].Media.Key != mediaKey(g.a.ui.track) || !ms[0].Playing {
		t.Fatalf("on opening: %+v", ms)
	}
	g.a.ui.track.Playing, g.a.ui.track.Position, g.a.ui.track.PosAt = false, g.a.ui.track.Pos(g.clock), g.clock // paused
	loopOnce(g)
	g.a.ui.track = &Track{Player: "p", Title: "Another", Playing: true, Length: time.Minute, PosAt: g.clock} // a new song
	loopOnce(g)
	g.a.ui.track.Position = 30 * time.Second // a seek
	loopOnce(g)
	if ms = h.of("media"); len(ms) != 4 || ms[1].Playing || ms[2].Title != "Another" || ms[3].PosMs < 30000 {
		t.Fatalf("changes: %+v", ms)
	}
	g.a.ui.track = nil
	loopOnce(g)
	if ms = h.of("media"); len(ms) != 5 || ms[4].Media != nil {
		t.Fatalf("nothing plays: %+v", ms[len(ms)-1])
	}
	g.a.ui.press("dash", 1)
	loopOnce(g)
	g.music(true)
	for i := 0; i < 5; i++ {
		loopOnce(g)
	}
	if ms = h.of("media"); len(ms) != 5 {
		t.Fatalf("media reached a closed page: %d", len(ms))
	}
	if ds := h.of("dash"); len(ds) != 2 || ds[1].State != "close" || ds[1].Reason != proto.CloseUser {
		t.Fatalf("dash events %+v", ds)
	}

	// without the grant: the page opens, the song stays Ember's
	g2 := newScene()
	g2.music(true)
	h2 := stagePage(t, g2, "vibe", `["entry"]`, vibePage)
	g2.a.ui.openDash("vibe")
	for i := 0; i < 5; i++ {
		loopOnce(g2)
	}
	if len(h2.of("media")) != 0 || len(h2.of("dash")) != 1 {
		t.Fatalf("not granted media: %+v", h2.sent)
	}
}

// A click on Ember in the card opens the page that owns the context, unless
// its click has a job (something waits on you); never from the pill.
func TestEntryFromEmber(t *testing.T) {
	g := newScene()
	g.music(true)
	stagePage(t, g, "vibe", `["media","entry"]`, vibePage)
	g.a.ui.press("ember", 1) // in the pill: Ember's own
	if g.a.ui.dash != "" {
		t.Fatal("the pill's Ember opened a page")
	}
	g.a.ui.pinned = true
	g.run(time.Second)
	g.a.ui.press("ember", 1)
	if g.a.ui.dash != "vibe" {
		t.Fatalf("the card's Ember while a song plays: %q", g.a.ui.dash)
	}
	g.a.ui.closeDash(proto.CloseUser)
	g.a.ui.track.Playing = false // idle: nobody owns it
	g.a.ui.pinned = true
	g.run(time.Second)
	g.a.ui.press("ember", 1)
	if g.a.ui.dash != "" {
		t.Fatal("idle opened the music page")
	}
	g.a.ui.track.Playing = true
	g.work("k", "kinvault", "fix it", 2)
	hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
	g.a.ui.pinned = true
	g.run(time.Second)
	if name, _ := g.a.ui.opener(); name != "" {
		t.Fatal("Ember's click opens a page while a session waits on you")
	}
}

// A sticky page stays when the pointer leaves it; a plain one closes, and says so.
func TestStickyPages(t *testing.T) {
	for _, sticky := range []bool{true, false} {
		g := newScene()
		g.music(true)
		page := vibePage
		if !sticky {
			page = `{"type":"dashboard","widgets":[{"kind":"note","text":"x"}]}`
		}
		h := stagePage(t, g, "vibe", `["media"]`, page)
		g.a.ui.openDash("vibe")
		loopOnce(g)
		g.a.ui.pointer(pointerEvent{kind: "motion", x: 290, y: 40})
		g.a.ui.pointer(pointerEvent{kind: "leave"})
		loopOnce(g)
		if open := g.a.ui.dash == "vibe"; open != sticky {
			t.Fatalf("sticky %v: open %v after the pointer left", sticky, open)
		}
		if !sticky {
			if ds := h.of("dash"); len(ds) != 2 || ds[1].Reason != proto.ClosePointer {
				t.Fatalf("%+v", ds)
			}
		}
	}
}

// A page ends itself: the music paused for media_idle_s; and once Claude
// has finished while it's open, when that song is over.
func TestPageEnds(t *testing.T) {
	g := newScene()
	g.music(true)
	h := stagePage(t, g, "vibe", `["media"]`, vibePage)
	g.a.ui.openDash("vibe")
	loopOnce(g)
	g.a.ui.track.Playing = false
	g.run(119 * time.Second)
	if g.a.ui.dash == "" {
		t.Fatal("closed before media_idle_s")
	}
	g.a.ui.track.Playing = true // it came back: the count starts over
	g.run(time.Second)
	g.a.ui.track.Playing = false
	g.run(100 * time.Second)
	if g.a.ui.dash == "" {
		t.Fatal("the pause before counted")
	}
	g.run(21 * time.Second)
	loopOnce(g)
	if ds := h.of("dash"); g.a.ui.dash != "" || ds[len(ds)-1].Reason != proto.CloseMediaIdle {
		t.Fatalf("after media_idle_s: %q %+v", g.a.ui.dash, ds)
	}

	g = newScene()
	g.music(true)
	h = stagePage(t, g, "vibe", `["media"]`, vibePage)
	g.work("k", "kinvault", "fix it", 2)
	g.a.ui.openDash("vibe")
	loopOnce(g)
	g.clock = g.clock.Add(time.Second)
	hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "Fixed it."})
	g.run(30 * time.Second)
	if g.a.ui.dash == "" {
		t.Fatal("folded mid-song when Claude finished")
	}
	g.a.ui.track = &Track{Player: "p", Title: "Next song", Playing: true, Length: time.Minute, PosAt: g.clock}
	loopOnce(g)
	if ds := h.of("dash"); g.a.ui.dash != "" || ds[len(ds)-1].Reason != proto.CloseSongEnd {
		t.Fatalf("at the song's end: %q %+v", g.a.ui.dash, ds)
	}
}

// Finding it: the hint once ever (the first card opened while Claude works
// and a song plays), the label under Ember with the pointer on it, the music
// row's control; none of it without a page to open.
func TestDiscover(t *testing.T) {
	g := newScene()
	g.a.ui.bond = &bond{Last: map[string]time.Time{}}
	g.music(true)
	g.work("k", "kinvault", "fix the race", 2)
	stagePage(t, g, "vibe", `["media","entry"]`, vibePage)
	u := g.a.ui
	u.pinned = true
	g.run(time.Second)
	if !u.showHint() || u.bond.Hinted.IsZero() {
		t.Fatal("no hint the first time")
	}
	h1 := u.expandedHeight()
	u.pinned = false
	g.run(time.Second)
	u.pinned = true
	g.run(time.Second)
	if u.showHint() || u.expandedHeight() >= h1 {
		t.Fatal("the hint came back")
	}
	if u.musicOpener() != "vibe" {
		t.Fatal("no control in the music row")
	}
	found := false
	for _, h := range u.hits {
		found = found || h.key == "music:page"
	}
	if !found {
		t.Fatal("the control isn't there to press")
	}
	u.press("music:page", 1)
	if u.dash != "vibe" {
		t.Fatal("the control didn't open it")
	}

	g2 := newScene() // no page that opens from Ember: nothing
	g2.a.ui.bond = &bond{Last: map[string]time.Time{}}
	g2.music(true)
	g2.work("k", "kinvault", "fix the race", 2)
	g2.a.ui.pinned = true
	g2.run(time.Second)
	if g2.a.ui.showHint() || g2.a.ui.musicOpener() != "" {
		t.Fatal("a hint with nothing to open")
	}
}
