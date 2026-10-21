package vibe

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// A song's words come from, in order: an .lrc file beside the song (players
// that give a file:// url), the cache in ~/.cache/ember/lyrics, then LRCLIB
// (lrclib.net: free and open, no key), asked only while the stage is open and
// only for the song playing (its title, artist, album and length). An answer
// is cached, a "there are none" too; a failure (offline) is not, so the next
// play asks again.

// Words are what was found for a song.
type Words struct {
	State  string // synced | plain | instrumental | none
	Lines  []Line // synced: in order; plain: the lines, untimed
	Source string // file | cache | lrclib | ""
}

// Line is a line of words, and when it's sung ("" is a break).
type Line struct {
	At   time.Duration
	Text string
}

var (
	lrcTime   = regexp.MustCompile(`\[(\d+):(\d+(?:[.:]\d+)?)\]`)
	lrcOffset = regexp.MustCompile(`(?i)^\[offset:\s*([+-]?\d+)\]`)
	// what titles carry that a lyrics database doesn't: "(feat. X)", "- Remastered 2011", "(From "Film")"
	titleNoise = regexp.MustCompile(`(?i)\s*[\(\[][^\)\]]*(feat\.|ft\.|from |remaster|version|edit|live|mono|stereo|bonus|deluxe|official|video|audio|lyrics?)[^\)\]]*[\)\]]`)
	dashNoise  = regexp.MustCompile(`(?i)\s+-\s+.*(remaster|version|edit|live|mono|stereo|mix).*$`)
)

// ParseLRC reads LRC text into timed lines, in order. A line may carry
// several stamps; an [offset:] tag shifts them all.
func ParseLRC(s string) []Line {
	var out []Line
	offset := time.Duration(0)
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if m := lrcOffset.FindStringSubmatch(line); m != nil {
			ms, _ := strconv.Atoi(m[1])
			offset = time.Duration(ms) * time.Millisecond
			continue
		}
		stamps := lrcTime.FindAllStringSubmatchIndex(line, -1)
		if len(stamps) == 0 {
			continue
		}
		text := strings.Trim(strings.TrimSpace(line[stamps[len(stamps)-1][1]:]), "♪ ")
		for _, st := range stamps {
			mins, _ := strconv.Atoi(line[st[2]:st[3]])
			secs, _ := strconv.ParseFloat(strings.Replace(line[st[4]:st[5]], ":", ".", 1), 64)
			at := time.Duration(mins)*time.Minute + time.Duration(secs*float64(time.Second)) - offset
			out = append(out, Line{At: max(at, 0), Text: text})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// cleanTitle drops what a lyrics database won't have in a title.
func cleanTitle(s string) string {
	s = titleNoise.ReplaceAllString(s, "")
	s = dashNoise.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// browser: the player is a web browser (its tabs' titles are "Artist - Title").
func browser(player string) bool {
	for _, b := range []string{"firefox", "chromium", "chrome", "zen", "brave", "vivaldi", "librewolf", "plasma-browser"} {
		if strings.Contains(strings.ToLower(player), b) {
			return true
		}
	}
	return false
}

// SongOf works out the artist and title to ask for. Browser players often
// put "Artist - Title" in the title and a channel in the artist.
func SongOf(m *plugin.Media) (artist, title string) {
	artist, title = m.Artist, m.Title
	if i := strings.Index(artist, ", "); i > 0 { // the first artist is the one a database files it under
		artist = artist[:i]
	}
	artist = strings.TrimSuffix(artist, " - Topic")
	if a, ti, ok := strings.Cut(title, " - "); ok && (artist == "" || browser(m.Player) || strings.EqualFold(strings.TrimSpace(a), artist)) {
		artist, title = strings.TrimSpace(a), ti
	}
	return artist, cleanTitle(title)
}

// CacheDir is where found words are kept: $XDG_CACHE_HOME/ember/lyrics.
func CacheDir() string {
	d := os.Getenv("XDG_CACHE_HOME")
	if d == "" {
		h, _ := os.UserHomeDir()
		d = filepath.Join(h, ".cache")
	}
	return filepath.Join(d, "ember", "lyrics")
}

type lrclibHit struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

// cached is what's kept on disk for a song: the hit, or that there was none.
type cached struct {
	Hit  *lrclibHit `json:"hit,omitempty"`
	None bool       `json:"none,omitempty"`
	At   time.Time  `json:"at"`
}

// noneFor: how long "there are no words" is believed before asking again.
const noneFor = 7 * 24 * time.Hour

func songKey(m *plugin.Media) string {
	a, t := SongOf(m)
	return strings.ToLower(a + "\x00" + t + "\x00" + strconv.FormatInt(m.LengthMs/1000, 10))
}

func cachePath(dir string, m *plugin.Media) string {
	h := sha1.Sum([]byte(songKey(m)))
	return filepath.Join(dir, hex.EncodeToString(h[:8])+".json")
}

// Finder looks words up.
type Finder struct {
	Dir    string       // the cache
	Base   string       // LRCLIB's address
	Client *http.Client // nil: no network (the cache and .lrc files only)
}

// Find looks up m's words. It blocks on the network: run it off the plugin's loop.
func (f *Finder) Find(ctx context.Context, m *plugin.Media) Words {
	none := Words{State: "none"}
	if u, err := url.Parse(m.URL); err == nil && u.Scheme == "file" { // an .lrc beside the file wins
		p := strings.TrimSuffix(u.Path, filepath.Ext(u.Path)) + ".lrc"
		if b, err := os.ReadFile(p); err == nil {
			if ls := ParseLRC(string(b)); len(ls) > 0 {
				return Words{State: "synced", Lines: ls, Source: "file"}
			}
		}
	}
	cp := cachePath(f.Dir, m)
	var c cached
	if b, err := os.ReadFile(cp); err == nil && json.Unmarshal(b, &c) == nil && (c.Hit != nil || time.Since(c.At) < noneFor) {
		w := from(c.Hit)
		w.Source = "cache"
		return w
	}
	if f.Client == nil {
		return none
	}
	hit, err := f.ask(ctx, m)
	if err != nil { // offline, or the service is down: nothing kept, the next play asks again
		return none
	}
	c = cached{Hit: hit, None: hit == nil, At: time.Now()}
	if b, err := json.Marshal(c); err == nil {
		os.MkdirAll(filepath.Dir(cp), 0o755)
		tmp := cp + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			os.Rename(tmp, cp)
		}
	}
	w := from(hit)
	w.Source = "lrclib"
	return w
}

func from(h *lrclibHit) Words {
	w := Words{State: "none"}
	switch {
	case h == nil:
	case h.SyncedLyrics != "":
		if ls := ParseLRC(h.SyncedLyrics); len(ls) > 0 {
			w.State, w.Lines = "synced", ls
		}
	case h.Instrumental:
		w.State = "instrumental"
	case h.PlainLyrics != "":
		for _, s := range strings.Split(h.PlainLyrics, "\n") {
			if s = strings.TrimSpace(s); s != "" {
				w.Lines = append(w.Lines, Line{Text: s})
			}
		}
		if len(w.Lines) > 0 {
			w.State = "plain"
		}
	}
	return w
}

// ask asks LRCLIB for the song: exactly first (its length tells versions
// apart), then by search. A nil hit with no error: there are no words to be had.
func (f *Finder) ask(ctx context.Context, m *plugin.Media) (*lrclibHit, error) {
	artist, title := SongOf(m)
	if title == "" {
		return nil, nil
	}
	get := func(path string, q url.Values, into any) (int, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", f.Base+path+"?"+q.Encode(), nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("User-Agent", "ember (https://github.com/Tejas242/ember)")
		resp, err := f.Client.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound:
			return http.StatusNotFound, nil
		case resp.StatusCode != http.StatusOK:
			return resp.StatusCode, fmt.Errorf("lrclib: %s", resp.Status)
		}
		return http.StatusOK, json.NewDecoder(resp.Body).Decode(into)
	}
	secs := float64(m.LengthMs) / 1000
	q := url.Values{"track_name": {title}, "artist_name": {artist}}
	if m.Album != "" {
		q.Set("album_name", m.Album)
	}
	if secs > 0 {
		q.Set("duration", fmt.Sprint(int(secs+0.5)))
	}
	var h lrclibHit
	code, err := get("/api/get", q, &h)
	if err != nil {
		return nil, err
	}
	if code == http.StatusOK && (h.SyncedLyrics != "" || h.PlainLyrics != "" || h.Instrumental) {
		return &h, nil
	}
	var hits []lrclibHit
	if _, err := get("/api/search", url.Values{"track_name": {title}, "artist_name": {artist}}, &hits); err != nil {
		return nil, err
	}
	// the best hit: synced words, then the length nearest the song's
	var best *lrclibHit
	score := func(x *lrclibHit) float64 {
		s := 0.0
		if x.SyncedLyrics != "" {
			s += 100
		}
		if secs > 0 {
			s -= math.Abs(x.Duration - secs)
		}
		return s
	}
	for i := range hits {
		x := &hits[i]
		if secs > 0 && math.Abs(x.Duration-secs) > 8 { // another version of the song
			continue
		}
		if best == nil || score(x) > score(best) {
			best = x
		}
	}
	return best, nil
}
