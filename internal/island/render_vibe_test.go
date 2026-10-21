//go:build render

package island

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/plugin"
	"github.com/Tejas242/ember/internal/source"
	proto "github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/vibe/vibe"
)

// EMBER_VIBE=dir go test -tags render -count=1 -run 'TestVibe$' ./internal/island
// EMBER_VIBE_FILM=dir … -run TestVibeFilm (frames at 24 fps, 2×)
//
// The vibe stage in every case, as the vibe plugin (plugins/vibe, run in
// process) asks for it and the island draws it: its words found in an .lrc
// beside the song or from a fake LRCLIB, what plays told to it in media
// events. Every lyric here was written for these tests.

type vLine struct {
	at   time.Duration
	text string
}

var upbeatWords = []vLine{
	{44000, "Turn the radio up, the night is young"},
	{47400, "every streetlight humming the same old tune"},
	{50800, "we were running out of reasons to go home"},
	{54200, "so we danced on the roof till the morning came through"},
	{57600, ""},
	{63000, "hold the moment like a match before it burns"},
	{66400, "say it louder, let the whole block learn"},
	{69800, "Turn the radio up, the night is young"},
	{73200, "every streetlight humming the same old tune"},
}

var nightWords = []vLine{
	{44000, "the kettle's cold, the screen still glows"},
	{49500, "rain on the window, soft and slow"},
	{55000, "nothing to fix until the build turns green"},
	{60500, "so I'll stay here, somewhere in between"},
}

var longWords = []vLine{
	{44000, "and the radio knows"},
	{47000, "and if the morning finds us here still singing every word we swore we'd never learn, then let it find us"},
	{55000, "let it find us"},
}

var cjkWords = []vLine{
	{44000, "the window hums along"},
	{47400, "夜の窓に 小さな火がゆれる"},
	{51000, "a small fire, swaying"},
}

var hindiWords = []vLine{
	{44000, "चाय की खुशबू में सुबह जागी"},
	{47400, "खिड़की पर बारिश धीरे गाती है"},
	{51000, "हम छत पर नाचे सुबह तक"},
	{54600, "शहर ने फिर भी नहीं सुना"},
}

var arabicWords = []vLine{
	{44000, "الليل طويل والأغاني قريبة"},
	{47400, "قلبي يغني on repeat طول الليل"},
	{51000, "نرقص على السطح حتى الصباح"},
	{54600, "والمدينة لا تسمع"},
}

var plainWords = []string{"the kettle sings at seven", "a small flame keeps the time", "the window learns the morning", "nobody asked the radio",
	"it sang along all the same", "the floor still warm from dancing", "we said we'd sleep at two", "the city didn't listen"}

func init() {
	for _, ws := range [][]vLine{upbeatWords, nightWords, longWords, cjkWords, hindiWords, arabicWords} {
		for i := range ws {
			ws[i].at *= time.Millisecond
		}
	}
}

func coolArt() *albumArt {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dx, dy := float64(x)-40, float64(y)-22
			d := math.Sqrt(dx*dx+dy*dy) / 64
			r, g, b := 70+60*d, 80-30*d, 190-80*d
			if y > 44 {
				r, g, b = r*0.6, g*0.6, b*0.7
			}
			img.Set(x, y, color.NRGBA{uint8(r), uint8(math.Max(0, g)), uint8(b), 255})
		}
	}
	return makeArt(img, "cool")
}

type vScene struct {
	name   string
	kind   string // karaoke | plain | none | instrumental | loading | talk | ad | paused | change
	lines  []vLine
	title  string
	artist string
	pos    time.Duration
	length time.Duration
	night  bool
	feed   string // song | soft | talk | none
	setup  func(g *galleryScene)
	after  func(g *galleryScene)
	settle time.Duration
	crop   int // extra height below the stage (a request)
}

func feedLevels(g *galleryScene, kind string, frame int) {
	a := g.a.ui.audio
	lv := songLevels()
	n := frame * 25 / 60 // the frames are 60 a second; the levels 25
	switch kind {
	case "song":
		a.Feed(lv[n%len(lv)], time.Now())
	case "soft": // a late-night tape: the same song, smoothed and quiet, no kick to lock onto
		var v [3]float64
		for j := 0; j < 12; j++ {
			s := lv[(n+j)%len(lv)]
			for b := range v {
				v[b] += s[b] / 12
			}
		}
		a.Feed([3]float64{0.12 + 0.3*v[0], 0.12 + 0.3*v[1], 0.08 + 0.2*v[2]}, time.Now())
	case "talk": // a voice: syllables in the mids, no low end
		t := float64(n) / 25
		syl := math.Max(0, math.Sin(t*2*math.Pi*3.7)*math.Sin(t*2*math.Pi*0.43+1))
		a.Feed([3]float64{0.08, 0.15 + 0.5*syl, 0.1 + 0.2*syl}, time.Now())
	}
}

func vibeScenes() []vScene {
	return []vScene{
		{name: "01-karaoke-upbeat", kind: "karaoke", lines: upbeatWords, title: "Neon Afternoon", artist: "Kite Club", pos: 51 * time.Second, feed: "song"},
		{name: "02-karaoke-late-night", kind: "karaoke", lines: nightWords, title: "Slow Rooms", artist: "tape hiss", pos: 51500 * time.Millisecond, night: true, feed: "soft"},
		{name: "03-break-in-the-words", kind: "karaoke", lines: upbeatWords, title: "Neon Afternoon", artist: "Kite Club", pos: 59800 * time.Millisecond, feed: "song"},
		{name: "04-a-very-long-line", kind: "karaoke", lines: longWords, title: "Neon Afternoon", artist: "Kite Club", pos: 50 * time.Second, feed: "song"},
		{name: "05-non-latin-line", kind: "karaoke", lines: cjkWords, title: "Yoru no Mado", artist: "Kite Club", pos: 46600 * time.Millisecond, feed: "song"},
		{name: "06-plain-words", kind: "plain", title: "Morning Radio", artist: "Kite Club", pos: 40 * time.Second, feed: "song"},
		{name: "07-finding-the-words", kind: "loading", title: "Neon Afternoon", artist: "Kite Club", pos: 52 * time.Second, feed: "song"},
		{name: "08-no-words", kind: "none", title: "Neon Afternoon (Club Edit)", artist: "Kite Club", pos: 52 * time.Second, feed: "song"},
		{name: "09-instrumental", kind: "instrumental", title: "Tidewater", artist: "The Long Hallway", pos: 52 * time.Second, feed: "song"},
		{name: "10-song-change", kind: "change", lines: upbeatWords, title: "Neon Afternoon", artist: "Kite Club", pos: 3*time.Minute + 22*time.Second, feed: "song"},
		{name: "11-talk-podcast", kind: "talk", title: "Why queues lie: backpressure, honestly", artist: "Signals & Noise", pos: 21 * time.Minute, length: 69 * time.Minute, feed: "talk"},
		{name: "12-an-ad", kind: "ad", title: "Advertisement", artist: "", pos: 7 * time.Second, length: 30 * time.Second, feed: "none"},
		{name: "13-paused", kind: "paused", lines: upbeatWords, title: "Neon Afternoon", artist: "Kite Club", pos: 52 * time.Second, feed: "none"},
		{name: "14-claude-done-mid-song", kind: "karaoke", lines: upbeatWords, title: "Neon Afternoon", artist: "Kite Club", pos: 52 * time.Second, feed: "song",
			after: func(g *galleryScene) {
				hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "Fixed the race in the store's compaction path; all 42 tests pass under -race."})
			}, settle: 1300 * time.Millisecond},
		{name: "15-claude-stopped", kind: "karaoke", lines: upbeatWords, title: "Neon Afternoon", artist: "Kite Club", pos: 52 * time.Second, feed: "song",
			after: func(g *galleryScene) {
				hookAt(g.a, "StopFailure", "k", "kinvault", map[string]any{"error": "rate_limit", "error_message": "Rate limited"})
			}, settle: 1300 * time.Millisecond},
		{name: "16-needs-you", kind: "karaoke", lines: upbeatWords, title: "Neon Afternoon", artist: "Kite Club", pos: 52 * time.Second, feed: "song",
			after: func(g *galleryScene) {
				askAt(g.a, "PermissionRequest", "k", "kinvault", map[string]any{"tool_name": "Bash",
					"tool_input": map[string]any{"command": "go test -race ./store/... -count=1", "description": "Run the store tests under the race detector"}})
			}, settle: 1200 * time.Millisecond, crop: 40},
		{name: "17-screen-shared", kind: "karaoke", lines: upbeatWords, title: "Neon Afternoon", artist: "Kite Club", pos: 52 * time.Second, feed: "song",
			setup: func(g *galleryScene) { g.a.ui.shared, g.a.ui.cfg.Discreet = true, true }},
		{name: "18-hindi-line", kind: "karaoke", lines: hindiWords, title: "Subah ki Chai", artist: "Kite Club", pos: 46400 * time.Millisecond, feed: "song"},
		{name: "19-arabic-line", kind: "karaoke", lines: arabicWords, title: "Layl Tawil", artist: "Kite Club", pos: 46400 * time.Millisecond, feed: "song"},
	}
}

// lrcOf writes a scene's words as an .lrc file.
func lrcOf(ls []vLine) string {
	var b strings.Builder
	for _, l := range ls {
		fmt.Fprintf(&b, "[%02d:%05.2f]%s\n", int(l.at.Minutes()), math.Mod(l.at.Seconds(), 60), l.text)
	}
	return b.String()
}

// fakeLRCLIB answers the plugin as LRCLIB would, for the scenes: plain words,
// an instrumental, nothing; and never, for a song on a slow network.
func fakeLRCLIB(t *testing.T) *httptest.Server {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("album_name") == "slow network" {
			select { // finding the words: still looking
			case <-block:
			case <-r.Context().Done():
			}
			return
		}
		if r.URL.Path == "/api/get" {
			http.NotFound(w, r)
			return
		}
		var hits []map[string]any
		switch q.Get("track_name") {
		case "Morning Radio":
			hits = append(hits, map[string]any{"duration": 204, "plainLyrics": strings.Join(plainWords, "\n")})
		case "Tidewater":
			hits = append(hits, map[string]any{"duration": 204, "instrumental": true})
		}
		json.NewEncoder(w).Encode(hits)
	}))
	t.Cleanup(func() { close(block); srv.Close() })
	return srv
}

// vibeBridge is the plugin host for the scenes: it runs plugins/vibe in
// process (the SDK's harness checks every line the way Ember does), hands
// it what the island sends, and applies what it answers on the island.
type vibeBridge struct {
	t       *testing.T
	h       *proto.Harness
	a       *app
	applied int
}

func (b *vibeBridge) Send(name string, ev proto.Event) { b.h.Send(ev) }
func (b *vibeBridge) CardClosed(string, string)        {}
func (b *vibeBridge) Click(string, string)             {}
func (b *vibeBridge) Has(string) bool                  { return true }
func (b *vibeBridge) Opener(c string) (string, string) {
	if c == "music" {
		return "vibe", "Vibe"
	}
	return "", ""
}
func (b *vibeBridge) Info() []plugin.Info { return nil }
func (b *vibeBridge) Scan()               {}
func (b *vibeBridge) Stop()               {}

// settle tells the plugin what the island would (its page opening or
// closing, the song) and applies its pages until one says what the island
// now plays, and done (not still looking) unless looking is what's wanted.
func (b *vibeBridge) settle(looking bool) {
	b.a.extDashChanged()
	b.a.extMedia()
	key := mediaKey(b.a.ui.track)
	if b.a.ui.dash == "" { // closed: the plugin hears it and says nothing more
		time.Sleep(20 * time.Millisecond)
		for ms := b.h.Messages(); b.applied < len(ms); b.applied++ {
			b.a.applyExt("vibe", ms[b.applied])
		}
		return
	}
	ok := func(d *proto.Dashboard) bool {
		for _, w := range d.Widgets {
			switch w.Kind {
			case "countdown":
				return true
			case "timed_text":
				return w.MediaKey == key && (looking || w.State != "loading")
			}
		}
		return false
	}
	time.Sleep(20 * time.Millisecond) // its answer to what it was just told, if it has one
	for end := time.Now().Add(3 * time.Second); ; time.Sleep(2 * time.Millisecond) {
		ms := b.h.Messages()
		done := b.a.ui.page() != nil && ok(b.a.ui.page()) // what it showed last may say it already (a page it doesn't send twice)
		for ; b.applied < len(ms); b.applied++ {
			b.a.applyExt("vibe", ms[b.applied])
			if ms[b.applied].Type == "dashboard" {
				done = ok(ms[b.applied].Dashboard)
			}
		}
		if done {
			break
		}
		if time.Now().After(end) {
			b.t.Fatalf("the plugin didn't answer for %q (%d messages, refused %v)", b.a.ui.track.Title, len(ms), b.h.Errors())
		}
	}
	if errs := b.h.Errors(); len(errs) > 0 {
		b.t.Fatalf("Ember would refuse the plugin's lines: %v", errs)
	}
}

// startVibe runs plugins/vibe for g, as Ember would with its manifest's grants.
func startVibe(t *testing.T, g *galleryScene) *vibeBridge {
	srv := fakeLRCLIB(t)
	cache := t.TempDir()
	h := proto.NewHarnessWith("vibe", []string{proto.CapDashboard}, []string{proto.GrantMedia, proto.GrantEntry}, func(p *proto.Plugin) error {
		return vibe.Run(p, vibe.Options{Finder: &vibe.Finder{Dir: cache, Base: srv.URL, Client: srv.Client()}, Lyrics: func() bool { return true }})
	})
	t.Cleanup(func() { h.Stop() })
	b := &vibeBridge{t: t, h: h, a: g.a}
	g.a.host, g.a.ui.openers = b, b.Opener
	g.a.ui.cfg.Plugins = map[string]map[string]string{"vibe": {"grants": `["media", "entry"]`}}
	if _, err := h.Wait("dashboard", 3*time.Second); err != nil {
		t.Fatal(err)
	}
	b.settle(true) // its first page, to open
	return b
}

func buildVibe(t *testing.T, sc vScene) (*galleryScene, *vibeBridge) {
	g := newScene()
	if sc.night {
		g.clock = time.Date(2026, 10, 16, 2, 10, 0, 0, time.Local)
		g.a.ui.start, g.a.ui.lastBlink = g.clock, g.clock
	}
	g.work("k", "kinvault", "fix the race in the store", 3)
	g.music(true)
	tr := g.a.ui.track
	tr.Title, tr.Artist, tr.Album, tr.Position, tr.PosAt = sc.title, sc.artist, "", sc.pos, g.clock
	tr.Length = 3*time.Minute + 24*time.Second
	if sc.length > 0 {
		tr.Length = sc.length
	}
	if len(sc.lines) > 0 { // a local file, its words beside it
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "song.lrc"), []byte(lrcOf(sc.lines)), 0o644)
		tr.URL = "file://" + filepath.Join(dir, "song.flac")
	}
	switch sc.kind {
	case "loading":
		tr.Album = "slow network"
	case "talk":
		tr.TrackID = "spotify:episode:4ccd1e"
	}
	if sc.night {
		g.a.ui.art = coolArt()
	}
	g.a.ui.audio = &source.Audio{}
	if sc.kind == "paused" {
		tr.Playing, tr.PausedAt = false, g.clock
	}
	if sc.setup != nil {
		sc.setup(g)
	}
	b := startVibe(t, g)
	// warm up: Ember hears the song a while (it has learnt the beat by the time you open vibe)
	for i := 0; i < 300; i++ {
		feedLevels(g, sc.feed, i)
		g.run(time.Second / 60)
	}
	g.a.ui.track.Position, g.a.ui.track.PosAt = sc.pos, g.clock
	g.a.ui.openDash("vibe")
	b.settle(sc.kind == "loading")
	return g, b
}

func vibeCrop(u *UI, extra int) image.Rectangle {
	b := u.shape.bounds()
	cx := (b.Min.X + b.Max.X) / 2
	return image.Rect(cx-cardW/2-14, 0, cx+cardW/2+14, stageH+16+extra)
}

func TestVibe(t *testing.T) {
	dir := os.Getenv("EMBER_VIBE")
	if dir == "" {
		t.Skip("set EMBER_VIBE")
	}
	t.Setenv("HOME", t.TempDir())
	os.MkdirAll(dir, 0o755)
	var shots []galleryShot
	only := os.Getenv("EMBER_VIBE_ONLY")
	for _, sc := range vibeScenes() {
		if only != "" && only != sc.name {
			continue
		}
		g, b := buildVibe(t, sc)
		frames := 120
		if sc.kind == "change" { // a new song arrives while the stage is open
			g.run(time.Second)
			tr := *g.a.ui.track
			tr.Title, tr.Artist, tr.URL, tr.Position, tr.PosAt = "Tidewater", "The Long Hallway", "", 0, g.clock
			g.a.ui.track = &tr
			g.a.ui.Invalidate()
			b.settle(false)
			frames = 80
		}
		for i := 0; i < frames; i++ {
			feedLevels(g, sc.feed, 300+i)
			g.run(time.Second / 60)
		}
		if sc.after != nil {
			sc.after(g)
			n := int(sc.settle / (time.Second / 60))
			for i := 0; i < n; i++ {
				feedLevels(g, sc.feed, 420+i)
				g.run(time.Second / 60)
			}
		}
		img := onDesktop(g.a.ui, vibeCrop(g.a.ui, sc.crop))
		one := image.NewRGBA(image.Rect(0, 0, img.Rect.Dx(), img.Rect.Dy()))
		draw.Draw(one, one.Rect, img, img.Rect.Min, draw.Src)
		savePNG(t, filepath.Join(dir, sc.name+"-1x.png"), one)
		savePNG(t, filepath.Join(dir, sc.name+"-2x.png"), upscale(one, 2))
		shots = append(shots, galleryShot{sc.name, upscale(one, 2)})
	}
	if only == "" {
		mascotSheet(t, filepath.Join(dir, "states.png"), shots, 3)
	}
}

// TestVibeFilm: the main state, frame by frame at 2× (24 fps, like the
// stage), from opening the stage out of the card, on the real song levels.
func TestVibeFilm(t *testing.T) {
	dir := os.Getenv("EMBER_VIBE_FILM")
	if dir == "" {
		t.Skip("set EMBER_VIBE_FILM")
	}
	t.Setenv("HOME", t.TempDir())
	os.MkdirAll(dir, 0o755)
	sc := vibeScenes()[0]
	if n := os.Getenv("EMBER_VIBE_SCENE"); n != "" {
		for _, s := range vibeScenes() {
			if s.name == n {
				sc = s
			}
		}
	}
	sc.pos = 48500 * time.Millisecond
	g, b := buildVibe(t, sc)
	g.a.ui.closeDash(proto.CloseUser) // first the card, as he sees it, then the click on Ember
	b.settle(false)
	g.a.ui.pinned = true
	for i := 0; i < 30; i++ {
		feedLevels(g, sc.feed, 300+i)
		g.run(time.Second / 60)
	}
	g.a.ui.press("ember", 1)
	b.settle(false)
	frame := 0
	for i := 0; i < 60*10; i++ { // ten seconds
		feedLevels(g, sc.feed, 330+i)
		g.run(time.Second / 60)
		if i%5 == 0 || i%5 == 2 { // 24 fps
			img := onDesktop(g.a.ui, vibeCrop(g.a.ui, 0))
			savePNG(t, filepath.Join(dir, fmt.Sprintf("%04d.png", frame)), upscale(img, 2))
			frame++
		}
	}
}

// TestVibeFind: how you find the stage: the one-time hint under the
// headline, the label under Ember with the pointer on it, the music row's
// control under the pointer.
func TestVibeFind(t *testing.T) {
	dir := os.Getenv("EMBER_VIBE")
	if dir == "" {
		t.Skip("set EMBER_VIBE")
	}
	t.Setenv("HOME", t.TempDir())
	os.MkdirAll(dir, 0o755)
	var shots []galleryShot
	for _, c := range []struct {
		name   string
		hinted bool
		mx, my float64
	}{{"find-hint", false, -1, -1}, {"find-label", true, 0, 0}, {"find-control", true, 1, 1}} {
		sc := vibeScenes()[0]
		g, b := buildVibe(t, sc)
		g.a.ui.closeDash(proto.CloseUser)
		b.settle(false)
		g.a.ui.bond = &bond{Last: map[string]time.Time{}}
		if c.hinted {
			g.a.ui.bond.Hinted = g.clock.Add(-time.Hour)
		}
		g.a.ui.pinned = true
		g.run(time.Second)
		if c.mx >= 0 {
			key := "ember"
			if c.mx > 0 {
				key = "row:music"
			}
			for _, h := range g.a.ui.hits {
				if h.key == key {
					cx, cy := float64(h.r.Min.X+h.r.Max.X)/2, float64(h.r.Min.Y+h.r.Max.Y)/2
					if key == "row:music" {
						cx = float64(h.r.Min.X) + 60
					}
					g.a.ui.pointer(pointerEvent{kind: "motion", x: cx, y: cy})
				}
			}
			g.a.ui.Invalidate()
			g.run(time.Second)
		}
		r := g.a.ui.shape.bounds()
		img := onDesktop(g.a.ui, image.Rect(r.Min.X-14, 0, r.Max.X+14, r.Max.Y+16))
		one := image.NewRGBA(image.Rect(0, 0, img.Rect.Dx(), img.Rect.Dy()))
		draw.Draw(one, one.Rect, img, img.Rect.Min, draw.Src)
		shots = append(shots, galleryShot{c.name, upscale(one, 2)})
	}
	mascotSheet(t, filepath.Join(dir, "find.png"), shots, 3)
}
