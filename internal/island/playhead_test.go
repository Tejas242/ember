package island

import (
	"fmt"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// A browser's player that says it plays but never where: after more than
// four seconds of the same position, the stage pages the words by the wall
// clock with a caption saying why; once the position moves with time again,
// the words follow it again. A seek proves nothing either way; a pause
// isn't a stuck playhead.
func TestFrozenPlayhead(t *testing.T) {
	g := stageScene(t, testLines, mediaKey, "synced", "ready")
	a := g.a
	key := mediaKey(a.ui.track)
	says := func(pos time.Duration) { // the player answers the poll
		a.handle(source.Playhead{Player: "p", Pos: pos, At: g.clock})
		g.run(time.Second / 30)
	}
	kind := func() string { st, sw, _ := a.ui.stageNow(); return a.ui.stageKind(st, sw) }

	says(11 * time.Second)
	g.run(2500 * time.Millisecond)
	says(11 * time.Second)
	if a.ui.stuck != "" || kind() != "synced" {
		t.Fatalf("stuck after 2.5 s: %q %s", a.ui.stuck, kind())
	}
	g.run(2500 * time.Millisecond)
	says(11 * time.Second)
	if a.ui.stuck != key || kind() != "stanzas" {
		t.Fatalf("not stuck after 5 s of 0:11: %q %s", a.ui.stuck, kind())
	}
	if el := a.ui.songElapsed(g.clock); el < 16*time.Second || el > 17*time.Second {
		t.Fatalf("by the wall clock it's at %v, want ~16 s", el)
	}
	g.run(5 * time.Second)
	says(80 * time.Second) // a seek in a stuck player: still stuck, the wall clock from there
	if a.ui.stuck != key {
		t.Fatal("a seek made it trusted")
	}
	if el := a.ui.songElapsed(g.clock.Add(3 * time.Second)); el < 82*time.Second || el > 84*time.Second {
		t.Fatalf("after the seek: %v", el)
	}

	// it moves with time again: the words follow it
	g.run(2 * time.Second)
	says(82 * time.Second)
	if a.ui.stuck != "" || kind() != "synced" {
		t.Fatalf("still stuck once moving: %q %s", a.ui.stuck, kind())
	}

	// a moving player that seeks, then plays on: never stuck
	g.run(3 * time.Second)
	says(20 * time.Second)
	for i := 0; i < 4; i++ {
		g.run(3 * time.Second)
		says(time.Duration(23+3*i) * time.Second)
	}
	if a.ui.stuck != "" {
		t.Fatal("a seek made it stuck")
	}

	// paused: the position stays, rightly
	a.ui.track.Playing = false
	for i := 0; i < 4; i++ {
		says(32 * time.Second)
		g.run(3 * time.Second)
	}
	if a.ui.stuck != "" {
		t.Fatal("a pause made it stuck")
	}

	// the same through the player's own changes (TrackChanged), stuck at 0, as some browsers are
	for i := 0; i < 4; i++ {
		tr := *a.ui.track
		tr.Playing, tr.Position, tr.PosAt = true, 0, g.clock
		a.handle(source.TrackChanged{Track: &tr})
		g.run(2 * time.Second)
	}
	if a.ui.stuck != key {
		t.Fatal("stuck at 0 through its own changes, not noticed")
	}
	// another song: trusted until it shows otherwise
	tr := *a.ui.track
	tr.Title, tr.PosAt = "Another", g.clock
	a.handle(source.TrackChanged{Track: &tr})
	if a.ui.stuck != "" {
		t.Fatal("a new song inherited the old one's stuck playhead")
	}
}

// Stuck, the words are paged through the song by how long it has played,
// with the caption; words the plugin sent as stanzas keep its own.
func TestFrozenPlayheadStanzas(t *testing.T) {
	var lines []proto.TimedLine
	for i := 0; i < 12; i++ { // three pages
		lines = append(lines, proto.TimedLine{TMs: int64(10000 + 4000*i), Text: fmt.Sprintf("line %d", i)})
	}
	g := stageScene(t, lines, mediaKey, "synced", "ready")
	a := g.a
	a.handle(source.Playhead{Player: "p", Pos: 0, At: g.clock}) // a browser stuck at 0 from the start
	for i := 0; i < 30; i++ {
		g.run(5 * time.Second)
		a.handle(source.Playhead{Player: "p", Pos: 0, At: g.clock})
	}
	st, sw, _ := a.ui.stageNow()
	if k := a.ui.stageKind(st, sw); k != "stanzas" {
		t.Fatalf("%s", k)
	}
	if pg := a.ui.stanzaPage(st, 3); pg != 1 { // 150 s of 5:37
		t.Fatalf("page %d after 150 s", pg)
	}
	if c := a.ui.stanzaCaption(sw); c != "This player doesn't say where the song is" {
		t.Fatalf("caption %q", c)
	}
	sw.text.Mode, sw.text.Caption = "stanzas", "Words not timed for this song"
	if c := a.ui.stanzaCaption(sw); c != "Words not timed for this song" {
		t.Fatalf("the plugin's caption: %q", c)
	}
}
