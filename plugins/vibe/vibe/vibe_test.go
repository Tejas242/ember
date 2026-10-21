package vibe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// Lines written for these tests (not a real song's).
const testLRC = `[ar:Nobody]
[offset:+500]
[00:12.00]the kettle sings at seven
[00:15.50][00:40.25]a small flame keeps the time
[00:19.00]
[00:21.75]and the window learns the morning
`

func TestParseLRC(t *testing.T) {
	ls := ParseLRC(testLRC)
	if len(ls) != 5 {
		t.Fatalf("%d lines, want 5: %+v", len(ls), ls)
	}
	if ls[0].At != 11500*time.Millisecond || ls[0].Text != "the kettle sings at seven" {
		t.Errorf("first line %+v: the offset pulls it 500 ms earlier", ls[0])
	}
	if ls[2].Text != "" {
		t.Errorf("a stamp with no words is a break, got %q", ls[2].Text)
	}
	if ls[4].Text != "a small flame keeps the time" || ls[4].At != 39750*time.Millisecond {
		t.Errorf("a line with two stamps is sung twice: %+v", ls[4])
	}
}

func TestSongOf(t *testing.T) {
	for _, c := range []struct {
		m             plugin.Media
		artist, title string
	}{
		{plugin.Media{Artist: "Kite Club, The Long Hallway", Title: `Neon Afternoon (From "A Film Nobody Made")`}, "Kite Club", "Neon Afternoon"},
		{plugin.Media{Artist: "Kite Club", Title: "Slow Rooms - Remastered 2021"}, "Kite Club", "Slow Rooms"},
		{plugin.Media{Artist: "Some Channel", Title: "Kite Club - Tidewater (Official Video)", Player: "firefox.instance_1"}, "Kite Club", "Tidewater"},
		{plugin.Media{Artist: "Kite Club - Topic", Title: "Morning Radio"}, "Kite Club", "Morning Radio"},
		{plugin.Media{Artist: "Kite Club", Title: "Half - Light"}, "Kite Club", "Half - Light"},
	} {
		if a, ti := SongOf(&c.m); a != c.artist || ti != c.title {
			t.Errorf("SongOf(%q, %q) = %q, %q; want %q, %q", c.m.Artist, c.m.Title, a, ti, c.artist, c.title)
		}
	}
}

// lrclib is a fake LRCLIB: it counts the asks, and can be down.
type lrclib struct {
	srv  *httptest.Server
	asks atomic.Int32
	down atomic.Bool
	hits map[string][]lrclibHit // by track_name: what a search finds
}

func fakeLRCLIB(t *testing.T) *lrclib {
	l := &lrclib{hits: map[string][]lrclibHit{}}
	l.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.asks.Add(1)
		if l.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/api/get":
			http.NotFound(w, r)
		case "/api/search":
			json.NewEncoder(w).Encode(l.hits[r.URL.Query().Get("track_name")])
		}
	}))
	t.Cleanup(l.srv.Close)
	return l
}

func (l *lrclib) finder(dir string) *Finder {
	return &Finder{Dir: dir, Base: l.srv.URL, Client: l.srv.Client()}
}

// An .lrc beside a local file wins; then the cache; then LRCLIB, exact first
// then by search (the version of the song's length). Offline is not cached:
// the next play asks again. "None" is.
func TestFind(t *testing.T) {
	dir := t.TempDir()
	l := fakeLRCLIB(t)
	f := l.finder(filepath.Join(dir, "cache"))
	ctx := context.Background()

	song := filepath.Join(dir, "song.flac")
	os.WriteFile(filepath.Join(dir, "song.lrc"), []byte(testLRC), 0o644)
	if w := f.Find(ctx, &plugin.Media{Title: "x", Artist: "y", URL: "file://" + song}); w.State != "synced" || w.Source != "file" || l.asks.Load() != 0 {
		t.Fatalf("the .lrc beside the song: %s from %s after %d asks", w.State, w.Source, l.asks.Load())
	}

	l.hits["Kettle"] = []lrclibHit{
		{TrackName: "Kettle", ArtistName: "Nobody", Duration: 300, PlainLyrics: "another version"},
		{TrackName: "Kettle", ArtistName: "Nobody", Duration: 181, SyncedLyrics: testLRC},
	}
	m := &plugin.Media{Title: "Kettle", Artist: "Nobody", LengthMs: 180_000}
	if w := f.Find(ctx, m); w.State != "synced" || w.Source != "lrclib" || l.asks.Load() != 2 {
		t.Fatalf("got %s from %s after %d asks, want synced words from the search", w.State, w.Source, l.asks.Load())
	}
	if w := f.Find(ctx, m); w.Source != "cache" || l.asks.Load() != 2 {
		t.Errorf("the second time comes from the cache (%s, %d asks)", w.Source, l.asks.Load())
	}

	l.down.Store(true)
	gone := &plugin.Media{Title: "Unknown", Artist: "Nobody", LengthMs: 100_000}
	if w := f.Find(ctx, gone); w.State != "none" {
		t.Fatalf("offline: %s", w.State)
	}
	l.down.Store(false)
	asks := l.asks.Load()
	if w := f.Find(ctx, gone); w.State != "none" || w.Source != "lrclib" || l.asks.Load() == asks {
		t.Fatalf("offline wasn't remembered, so it asks again: %s %s", w.State, w.Source)
	}
	asks = l.asks.Load()
	if w := f.Find(ctx, gone); w.Source != "cache" || l.asks.Load() != asks {
		t.Fatalf("none is remembered: %s", w.Source)
	}

	l.hits["Hum"] = []lrclibHit{{Duration: 100, Instrumental: true}}
	l.hits["Notes"] = []lrclibHit{{Duration: 100, PlainLyrics: "one\n\ntwo\n"}}
	if w := f.Find(ctx, &plugin.Media{Title: "Hum", Artist: "Nobody", LengthMs: 100_000}); w.State != "instrumental" {
		t.Fatalf("instrumental: %s", w.State)
	}
	if w := f.Find(ctx, &plugin.Media{Title: "Notes", Artist: "Nobody", LengthMs: 100_000}); w.State != "plain" || len(w.Lines) != 2 {
		t.Fatalf("plain: %+v", w)
	}

	off := &Finder{Dir: f.Dir} // no network ([vibe] lyrics = false): the cache only
	asks = l.asks.Load()
	if w := off.Find(ctx, m); w.Source != "cache" {
		t.Fatalf("offline finder, cached song: %s", w.Source)
	}
	if w := off.Find(ctx, &plugin.Media{Title: "Never asked", LengthMs: 1}); w.State != "none" || l.asks.Load() != asks {
		t.Fatal("an offline finder asked the network")
	}
}

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		m    plugin.Media
		want Kind
	}{
		{plugin.Media{Title: "Neon Afternoon", Artist: "Kite Club", LengthMs: 204_000}, Song},
		{plugin.Media{Title: "Something", TrackID: "/com/spotify/ad/123", LengthMs: 30_000}, Ad},
		{plugin.Media{Title: "Advertisement", Artist: "", LengthMs: 30_000}, Ad},
		{plugin.Media{Title: "Buy this", Player: "spotify", LengthMs: 15_000}, Ad},
		{plugin.Media{Title: "Why queues lie", TrackID: "spotify:episode:abc", LengthMs: 4_140_000}, Talk},
		{plugin.Media{Title: "Three hours of house", Artist: "Kite Club", LengthMs: 3 * 3_600_000}, Long},
	} {
		if got := Classify(&c.m); got != c.want {
			t.Errorf("%q: %s, want %s", c.m.Title, got, c.want)
		}
	}
}

// The page for each case decodes the way Ember will read it.
func TestPage(t *testing.T) {
	m := &plugin.Media{Key: "k1", Title: "Neon Afternoon", Artist: "Kite Club", LengthMs: 204_000}
	decode := func(d plugin.Dashboard) *plugin.Dashboard {
		b, err := plugin.Line("dashboard", d)
		if err != nil {
			t.Fatal(err)
		}
		msg, err := plugin.Decode(b)
		if err != nil {
			t.Fatal(err)
		}
		return msg.Dashboard
	}
	d := decode(Page(m, nil))
	if d.Size != "card" || !d.Sticky || d.Ends.MediaIdleS != 120 || d.Ends.AfterClaude != "song_end" || d.Widgets[0].Kind != "stage" {
		t.Fatalf("%+v", d)
	}
	if tt := d.Widgets[2]; tt.Kind != "timed_text" || tt.State != "loading" || tt.MediaKey != "k1" {
		t.Fatalf("looking: %+v", tt)
	}
	d = decode(Page(m, &Words{State: "synced", Lines: ParseLRC(testLRC)}))
	if tt := d.Widgets[2]; tt.State != "ready" || tt.Mode != "synced" || len(tt.Lines) != 5 || tt.Lines[0].TMs != 11500 {
		t.Fatalf("synced: %+v", tt)
	}
	d = decode(Page(m, &Words{State: "plain", Lines: []Line{{Text: "a"}, {Text: "b"}}}))
	if tt := d.Widgets[2]; tt.Mode != "stanzas" || tt.Caption == "" {
		t.Fatalf("plain: %+v", tt)
	}
	if tt := decode(Page(m, &Words{State: "instrumental"})).Widgets[2]; tt.State != "instrumental" {
		t.Fatalf("instrumental: %+v", tt)
	}
	ad := decode(Page(&plugin.Media{Key: "a", Title: "Advertisement", LengthMs: 30_000}, nil))
	if ad.Widgets[0].Ember.Dance != "still" || ad.Widgets[2].Kind != "countdown" {
		t.Fatalf("ad: %+v", ad.Widgets)
	}
	talk := decode(Page(&plugin.Media{Key: "t", Title: "Why queues lie", TrackID: "spotify:episode:x", LengthMs: 4_000_000}, nil))
	if talk.Widgets[0].Ember.Pose != "attentive" || talk.Widgets[2].State != "spoken" || talk.Widgets[3].Tint != "quiet" {
		t.Fatalf("talk: %+v", talk.Widgets)
	}
	browser := decode(Page(&plugin.Media{Key: "b", Title: "x"}, &Words{State: "synced", Lines: ParseLRC(testLRC)}))
	if browser.Widgets[2].Mode != "stanzas" {
		t.Fatal("no playhead: stanzas")
	}
	var many []Line
	for i := 0; i < 2000; i++ {
		many = append(many, Line{At: time.Duration(i) * time.Second, Text: strings.Repeat("la ", 50)})
	}
	if _, err := plugin.Line("dashboard", Page(m, &Words{State: "synced", Lines: many})); err != nil {
		t.Fatal(err)
	}
	if b, _ := plugin.Line("dashboard", Page(m, &Words{State: "synced", Lines: many})); len(b) >= plugin.MaxLine {
		t.Fatalf("a page of %d bytes", len(b))
	}
}

// The plugin, under a fake Ember: a page to open at once; nothing looked up
// until a media event (Ember sends those only while the stage is open); the
// words once found; none of the network when lyrics are off.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	l := fakeLRCLIB(t)
	l.hits["Kettle"] = []lrclibHit{{Duration: 180, SyncedLyrics: testLRC}}
	lyrics := atomic.Bool{}
	lyrics.Store(true)
	h := plugin.NewHarnessWith("vibe", plugin.Caps, []string{plugin.GrantMedia, plugin.GrantEntry}, func(p *plugin.Plugin) error {
		return Run(p, Options{Finder: l.finder(dir), Lyrics: lyrics.Load})
	})
	if _, err := h.Wait("dashboard", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if l.asks.Load() != 0 {
		t.Fatal("asked before the stage opened")
	}
	h.Dash(true)
	h.Media(&plugin.Media{Key: "k1", Title: "Kettle", Artist: "Nobody", LengthMs: 180_000, Playing: true})
	ready := func(key string) func(plugin.Message) bool {
		return func(m plugin.Message) bool {
			tt := m.Dashboard.Widgets[2]
			return tt.MediaKey == key && tt.State == "ready"
		}
	}
	got, err := h.WaitFor("dashboard", 2*time.Second, ready("k1"))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Dashboard.Widgets[2].Lines); n != 5 {
		t.Fatalf("%d lines", n)
	}
	lyrics.Store(false)
	asks := l.asks.Load()
	h.Media(&plugin.Media{Key: "k2", Title: "Kettle 2", Artist: "Nobody", LengthMs: 180_000, Playing: true})
	if _, err := h.WaitFor("dashboard", 2*time.Second, func(m plugin.Message) bool {
		tt := m.Dashboard.Widgets[2]
		return tt.MediaKey == "k2" && tt.State == "none"
	}); err != nil {
		t.Fatal(err)
	}
	if l.asks.Load() != asks {
		t.Fatal("asked the network with lyrics off")
	}
	h.Media(&plugin.Media{Key: "k1", Title: "Kettle", Artist: "Nobody", LengthMs: 180_000, Playing: true}) // played again: from memory
	if _, err := h.WaitFor("dashboard", 2*time.Second, func(m plugin.Message) bool {
		return m.Dashboard.Widgets[2].MediaKey == "k1" && len(h.Messages()) > 3
	}); err != nil {
		t.Fatal(err)
	}
	if len(h.Errors()) > 0 {
		t.Fatalf("Ember would refuse: %v", h.Errors())
	}
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
}
