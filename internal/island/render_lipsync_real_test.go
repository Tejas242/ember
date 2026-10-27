//go:build render

package island

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
	proto "github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/vibe/vibe"
)

// Real songs, replayed: what source's capture test recorded (the monitor as
// parec hands it over, and the player's playhead) goes through the real
// analysis on the vibe stage, with the song's words from the lyrics cache.
// Private and local only: it prints titles and numbers, never words.
//
//	EMBER_REAL=/home/screenager/.cache/ember-lipsync/real EMBER_REAL_LYRICS=$HOME/.cache/ember/lyrics \
//	  go test -tags render -count=1 -timeout 0 -v -run TestLipSyncReal ./internal/island
//
// EMBER_REAL_FETCH=1 asks LRCLIB for a song the cache hasn't (kept in
// EMBER_REAL/lyrics, to delete with the rest). EMBER_REAL_FACE=dir also films Ember's face (no sound, no words) for 20 s
// of each song, once its offset is learnt.

type realSample struct {
	At     int64   `json:"at"`
	Player string  `json:"player"`
	Status string  `json:"status"`
	Title  string  `json:"title"`
	Artist string  `json:"artist"`
	Album  string  `json:"album"`
	Len    float64 `json:"len"`
	Pos    float64 `json:"pos"`
}

type realCapture struct {
	pcm   []int16
	hops  []int64
	mpris []realSample
}

func loadCapture(t *testing.T, base string) realCapture {
	var c realCapture
	b, err := os.ReadFile(base + ".pcm")
	if err != nil {
		t.Fatal(err)
	}
	c.pcm = make([]int16, len(b)/2)
	for i := range c.pcm {
		c.pcm[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	h, _ := os.ReadFile(base + ".hops")
	for i := 0; i+8 <= len(h); i += 8 {
		c.hops = append(c.hops, int64(binary.LittleEndian.Uint64(h[i:])))
	}
	f, _ := os.Open(base + ".mpris")
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var s realSample
		if json.Unmarshal(sc.Bytes(), &s) == nil {
			c.mpris = append(c.mpris, s)
		}
	}
	return c
}

// realWords finds title's synced words in the lyrics cache.
func realWords(dir, title, artist string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	best := ""
	for _, f := range files {
		var e struct {
			Hit struct {
				TrackName, ArtistName, SyncedLyrics string
			} `json:"hit"`
		}
		b, _ := os.ReadFile(f)
		if json.Unmarshal(b, &e) != nil || e.Hit.SyncedLyrics == "" || !strings.EqualFold(strings.TrimSpace(e.Hit.TrackName), strings.TrimSpace(title)) {
			continue
		}
		if best == "" || strings.Contains(strings.ToLower(e.Hit.ArtistName), strings.ToLower(artist)) {
			best = e.Hit.SyncedLyrics
		}
	}
	return best
}

func TestLipSyncReal(t *testing.T) {
	dir, lyr := os.Getenv("EMBER_REAL"), os.Getenv("EMBER_REAL_LYRICS")
	if dir == "" || lyr == "" {
		t.Skip("set EMBER_REAL and EMBER_REAL_LYRICS")
	}
	t.Setenv("HOME", t.TempDir())
	bases, _ := filepath.Glob(filepath.Join(dir, "*.pcm"))
	var rep strings.Builder
	for _, p := range bases {
		c := loadCapture(t, strings.TrimSuffix(p, ".pcm"))
		if len(c.hops) == 0 || len(c.mpris) == 0 {
			continue
		}
		for i := 0; i < len(c.mpris); { // one song at a time
			j := i
			for j < len(c.mpris) && c.mpris[j].Title == c.mpris[i].Title {
				j++
			}
			seg := c.mpris[i:j]
			i = j
			span := float64(seg[len(seg)-1].At-seg[0].At) / 1e9
			if span < 60 {
				continue
			}
			words := realWords(lyr, seg[0].Title, seg[0].Artist)
			if words == "" {
				words = realWords(filepath.Join(dir, "lyrics"), seg[0].Title, seg[0].Artist)
			}
			if words == "" && os.Getenv("EMBER_REAL_FETCH") != "" { // not in Ember's cache: ask LRCLIB, kept beside the capture
				own := filepath.Join(dir, "lyrics")
				f := &vibe.Finder{Dir: own, Base: "https://lrclib.net", Client: &http.Client{Timeout: 20 * time.Second}}
				f.Find(context.Background(), &proto.Media{Player: seg[0].Player, Title: seg[0].Title, Artist: seg[0].Artist, Album: seg[0].Album, LengthMs: int64(seg[0].Len * 1000)})
				words = realWords(own, seg[0].Title, seg[0].Artist)
			}
			if words == "" {
				fmt.Fprintf(&rep, "%s: no synced words in the cache\n", seg[0].Title)
				continue
			}
			rep.WriteString(replaySong(t, c, seg, words))
		}
	}
	os.WriteFile(filepath.Join(dir, "report.txt"), []byte(rep.String()), 0o600)
	t.Log("\n" + rep.String())
}

type realFrame struct{ pos, env float64 } // the player's clock, the voice's loudness

func replaySong(t *testing.T, c realCapture, seg []realSample, words string) string {
	g := newScene()
	g.work("k", "kinvault", "fix the race in the store", 3)
	g.music(true)
	tr := g.a.ui.track
	tr.Title, tr.Artist, tr.Album = seg[0].Title, seg[0].Artist, seg[0].Album
	tr.Length = time.Duration(seg[0].Len * float64(time.Second))
	sd := t.TempDir()
	os.WriteFile(filepath.Join(sd, "song.lrc"), []byte(words), 0o600)
	tr.URL = "file://" + filepath.Join(sd, "song.flac")
	w0 := seg[0].At
	tr.Position, tr.PosAt, tr.Playing, tr.Player = time.Duration(seg[0].Pos*float64(time.Second)), g.clock, true, seg[0].Player
	lastRead := w0
	g.a.ui.audio = &source.Audio{Now: func() time.Time { return g.clock }}
	if os.Getenv("EMBER_REAL_NOLEARN") != "" { // a nudge of 0 by hand: nothing learnt, to compare
		g.a.ui.nudges().Ms[mediaKey(tr)] = 0
	}
	b := startVibe(t, g)
	g.a.ui.openDash("vibe")
	b.settle(false)
	tr = g.a.ui.track
	u := g.a.ui
	s0 := g.clock
	scene := func(wall int64) time.Time { return s0.Add(time.Duration(wall - w0)) }

	var lines []float64 // the words' line times
	for _, ln := range strings.Split(words, "\n") {
		var m int
		var s float64
		if _, err := fmt.Sscanf(ln, "[%d:%f]", &m, &s); err == nil && strings.TrimSpace(ln[strings.Index(ln, "]")+1:]) != "" {
			lines = append(lines, float64(m)*60+s)
		}
	}
	hop := sort.Search(len(c.hops), func(k int) bool { return c.hops[k] >= w0 })
	ms := 1
	var frames []realFrame
	var vf [source.VoiceFrames]source.VoiceFrame
	var seq uint64
	type lineRec struct {
		at, off, first float64 // its time, the offset applied as it began (s), its first syllable (player's clock)
		key            string
	}
	recs := map[int]*lineRec{}
	believedAt := -1
	var face *lsVideo
	var faceR image.Rectangle
	faceFrom := -1.0
	faceDir := os.Getenv("EMBER_REAL_FACE")
	end := seg[len(seg)-1].At
	for i := 0; ; i++ {
		wall := w0 + int64(i+1)*int64(time.Second)/60
		if wall > end {
			break
		}
		g.clock = scene(wall)
		for ; hop < len(c.hops) && c.hops[hop] <= wall; hop++ {
			lsFeed(u.audio, c.pcm[hop*2*source.AudioHop:(hop+1)*2*source.AudioHop], scene(c.hops[hop]))
		}
		for ; ms < len(seg) && seg[ms].At <= wall; ms++ { // the player's playhead, read as Ember reads it: every five seconds
			if seg[ms].At-lastRead >= int64(5*time.Second) {
				lastRead = seg[ms].At
				g.a.handle(source.Playhead{Player: tr.Player, Pos: time.Duration(seg[ms].Pos * float64(time.Second)), At: scene(seg[ms].At)})
			}
		}
		n, sq, _ := u.audio.Voice(&vf, seq)
		seq = sq
		for _, f := range vf[:n] {
			frames = append(frames, realFrame{tr.Pos(f.At).Seconds() - voiceLag, float64(f.Env)})
		}
		u.renderDamage()
		key := mediaKey(tr)
		off := u.wordsNudge(key).Seconds()
		s := &u.face.sing
		if s.key != "" {
			k := sort.SearchFloat64s(lines, s.keyAt.Seconds()+1e-6) - 1
			if k >= 0 {
				r := recs[k]
				if r == nil {
					r = &lineRec{at: lines[k], off: off, first: math.NaN(), key: s.key}
					recs[k] = r
				}
				if len(s.onsets) > 0 && !math.IsNaN(s.onsets[0]) {
					r.first = s.onsets[0] - off
				}
			}
			if believedAt < 0 && u.offset.conf {
				believedAt = k
			}
		}
		if faceDir != "" && believedAt >= 0 && faceFrom < 0 && s.key != "" {
			faceFrom = tr.Pos(g.clock).Seconds()
			faceR = image.Rect(int(u.spot.x)-30, stageBase-47, int(u.spot.x)+30, stageBase+3)
			face = newSilentVideo(t, filepath.Join(faceDir, safeName(seg[0].Title)+".mp4"), faceR.Dx()*5, faceR.Dy()*5)
		}
		if face != nil && (i%5 == 0 || i%5 == 2) { // 24 a second
			face.frame(upscale(onDesktop(u, image.Rect(0, 0, u.W, u.H)).SubImage(faceR), 5))
			if tr.Pos(g.clock).Seconds() > faceFrom+20 {
				face.close()
				face = nil
			}
		}
	}
	if face != nil {
		face.close()
	}

	// the numbers
	o := &u.offset
	var out strings.Builder
	fmt.Fprintf(&out, "%s — %d lines\n", seg[0].Title, len(lines))
	fmt.Fprintf(&out, "  offset: lines heard %d, believed %v after line %d, learnt %+.0f ms (onsets' lag %+.0f ms, %d lines agree)\n",
		o.lines, o.conf, believedAt, o.to*1000, o.est*1000, o.support)
	fmt.Fprintf(&out, "  syllables begun on an onset heard %d, by the clock %d (%.0f%% by ear)\n", u.face.sing.byEar, u.face.sing.byClock,
		100*float64(u.face.sing.byEar)/math.Max(1, float64(u.face.sing.byEar+u.face.sing.byClock)))
	var all, gap []float64
	for k, r := range recs {
		if math.IsNaN(r.first) || k >= len(lines) {
			continue
		}
		t0 := r.at - r.off // the line's time on the player's clock, the offset applied
		e := r.first - t0
		all = append(all, e)
		var q, nq float64 // the voice 600..100 ms before it
		for _, f := range frames {
			if f.pos >= t0-0.6 && f.pos < t0-0.1 {
				q += f.env
				nq++
			}
		}
		if nq > 10 && q/nq < 0.25 {
			gap = append(gap, e)
		}
	}
	stat := func(name string, e []float64) {
		if len(e) == 0 {
			fmt.Fprintf(&out, "  %s: none\n", name)
			return
		}
		a := make([]float64, len(e))
		in := 0
		for i, v := range e {
			a[i] = math.Abs(v)
			if a[i] <= 0.1 {
				in++
			}
		}
		sort.Float64s(a)
		sort.Float64s(e)
		fmt.Fprintf(&out, "  %s: n %d, median %+.0f ms, median|e| %.0f ms, within 100 ms %.0f%%\n", name, len(e), e[len(e)/2]*1000, a[len(a)/2]*1000, 100*float64(in)/float64(len(e)))
	}
	// an independent check, from all lines at once: how much the voice rises
	// across each line's time moved by d (the 400 ms after minus the 400 ms
	// before, on the player's clock), averaged; it peaks at the voice's lag
	bestD, bestC, base := 0.0, math.Inf(-1), 0.0
	envAt := func(a, b float64) float64 {
		lo := sort.Search(len(frames), func(i int) bool { return frames[i].pos >= a })
		var sum, n float64
		for i := lo; i < len(frames) && frames[i].pos < b; i++ {
			sum += frames[i].env
			n++
		}
		return sum / math.Max(1, n)
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].pos < frames[j].pos })
	var curve []float64
	for d := -1.5; d <= 1.5001; d += 0.05 {
		var c float64
		for _, at := range lines {
			c += envAt(at+d, at+d+0.4) - envAt(at+d-0.4, at+d)
		}
		c /= float64(len(lines))
		curve = append(curve, c)
		if math.Abs(d) < 0.001 {
			base = c
		}
		if c > bestC {
			bestC, bestD = c, d
		}
	}
	sc := append([]float64(nil), curve...)
	sort.Float64s(sc)
	fmt.Fprintf(&out, "  the voice rises most across the lines' times moved %+.0f ms (rise %.3f; at 0: %.3f; median over ±1.5 s: %.3f)\n", bestD*1000, bestC, base, sc[len(sc)/2])
	stat("first syllable vs the line's time, offset applied (lines after a clear vocal gap)", gap)
	stat("... all lines", all)
	if os.Getenv("EMBER_REAL_PROPS") != "" {
		out.WriteString("    the learner's rise curve (every 100 ms from -1.5 s):")
		for k := 0; k < riseN; k += 2 {
			var sum, n float32
			for i := 0; i < min(o.lines, offLines); i++ {
				if v := o.rises[i][k]; v == v {
					sum, n = sum+v, n+1
				}
			}
			fmt.Fprintf(&out, " %.2f", sum/max(n, 1))
		}
		out.WriteString("\n")
		for i := 0; i < min(o.lines, offLines); i++ {
			fmt.Fprintf(&out, "    proposal set %2d:", i)
			for _, p := range o.props[i][:o.np[i]] {
				fmt.Fprintf(&out, " %+5.0f/%.2f", p.r*1000, p.w)
			}
			out.WriteString("\n")
		}
		ks := make([]int, 0, len(recs))
		for k := range recs {
			ks = append(ks, k)
		}
		sort.Ints(ks)
		for _, k := range ks {
			r := recs[k]
			fmt.Fprintf(&out, "    line %2d at %6.2f off %+5.0f first-at %+5.0f\n", k, r.at, r.off*1000, (r.first-(r.at-r.off))*1000)
		}
	}
	return out.String()
}

// newSilentVideo: a film with no soundtrack.
func newSilentVideo(t *testing.T, out string, w, h int) *lsVideo {
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", w, h), "-r", "24", "-i", "-",
		"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p", out)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return &lsVideo{cmd, in, w, h}
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, s)
}
