package island

import (
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Track is what the island shows for the active media player.
type Track struct {
	Player   string // bus name
	Title    string
	Artist   string
	ArtURL   string
	Playing  bool
	Length   time.Duration
	Position time.Duration
	PosAt    time.Time // when Position was read; it advances from there while playing
}

func (t Track) pos(now time.Time) time.Duration {
	p := t.Position
	if t.Playing {
		p += now.Sub(t.PosAt)
	}
	if t.Length > 0 && p > t.Length {
		p = t.Length
	}
	return p
}

const mprisPrefix = "org.mpris.MediaPlayer2."

// music watches MPRIS players on the session bus. It wakes only on signals.
type music struct {
	conn   *dbus.Conn
	posts  chan<- func(*app)
	recent map[string]time.Time // player -> last time it was playing
}

func startMusic(posts chan<- func(*app)) *music {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Println("music: no session bus:", err)
		return nil
	}
	m := &music{conn: conn, posts: posts, recent: map[string]time.Time{}}
	conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchObjectPath("/org/mpris/MediaPlayer2"))
	conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg0Namespace("org.mpris.MediaPlayer2"))
	conn.AddMatchSignal(dbus.WithMatchInterface("org.mpris.MediaPlayer2.Player"), dbus.WithMatchMember("Seeked"))
	sig := make(chan *dbus.Signal, 16)
	conn.Signal(sig)
	go func() {
		m.refresh()
		var pending <-chan time.Time
		for {
			select {
			case _, ok := <-sig:
				if !ok {
					return
				}
				if pending == nil { // coalesce bursts (players send several at once)
					pending = time.After(80 * time.Millisecond)
				}
			case <-pending:
				pending = nil
				m.refresh()
			}
		}
	}()
	return m
}

func (m *music) players() []string {
	var names []string
	if err := m.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return nil
	}
	var out []string
	for _, n := range names {
		if strings.HasPrefix(n, mprisPrefix) {
			out = append(out, n)
		}
	}
	return out
}

func (m *music) read(name string) (Track, bool) {
	obj := m.conn.Object(name, "/org/mpris/MediaPlayer2")
	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.mpris.MediaPlayer2.Player").Store(&props); err != nil {
		return Track{}, false
	}
	t := Track{Player: name, PosAt: time.Now()}
	if v, ok := props["PlaybackStatus"]; ok {
		s, _ := v.Value().(string)
		t.Playing = s == "Playing"
		if s == "Stopped" {
			return t, false
		}
	}
	if v, ok := props["Position"]; ok {
		if us, ok := v.Value().(int64); ok {
			t.Position = time.Duration(us) * time.Microsecond
		}
	}
	md, _ := props["Metadata"].Value().(map[string]dbus.Variant)
	if v, ok := md["xesam:title"]; ok {
		t.Title, _ = v.Value().(string)
	}
	if v, ok := md["xesam:artist"]; ok {
		if as, ok := v.Value().([]string); ok {
			t.Artist = strings.Join(as, ", ")
		}
	}
	if v, ok := md["mpris:artUrl"]; ok {
		t.ArtURL, _ = v.Value().(string)
	}
	if v, ok := md["mpris:length"]; ok {
		switch n := v.Value().(type) {
		case int64:
			t.Length = time.Duration(n) * time.Microsecond
		case uint64:
			t.Length = time.Duration(n) * time.Microsecond
		}
	}
	return t, t.Title != ""
}

// refresh picks the player worth showing: one that's playing, else the one
// that played most recently.
func (m *music) refresh() {
	var tracks []Track
	for _, n := range m.players() {
		if t, ok := m.read(n); ok {
			if t.Playing {
				m.recent[n] = time.Now()
			}
			tracks = append(tracks, t)
		}
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		if tracks[i].Playing != tracks[j].Playing {
			return tracks[i].Playing
		}
		return m.recent[tracks[i].Player].After(m.recent[tracks[j].Player])
	})
	var best *Track
	if len(tracks) > 0 {
		best = &tracks[0]
	}
	m.posts <- func(a *app) { a.setTrack(best) }
}

func (m *music) control(player, method string) {
	if m == nil || player == "" {
		return
	}
	go m.conn.Object(player, "/org/mpris/MediaPlayer2").Call("org.mpris.MediaPlayer2.Player."+method, 0)
}

// position re-reads the playhead (only while the card shows a progress bar).
func (m *music) position(player string) {
	if m == nil || player == "" {
		return
	}
	go func() {
		v, err := m.conn.Object(player, "/org/mpris/MediaPlayer2").GetProperty("org.mpris.MediaPlayer2.Player.Position")
		if err != nil {
			return
		}
		us, _ := v.Value().(int64)
		at := time.Now()
		m.posts <- func(a *app) {
			if a.track != nil && a.track.Player == player {
				a.track.Position, a.track.PosAt = time.Duration(us)*time.Microsecond, at
			}
		}
	}()
}

// seek moves the playhead by delta (MPRIS Seek takes microseconds).
func (m *music) seek(player string, delta time.Duration) {
	if m == nil || player == "" {
		return
	}
	go m.conn.Object(player, "/org/mpris/MediaPlayer2").Call("org.mpris.MediaPlayer2.Player.Seek", 0, int64(delta/time.Microsecond))
}

// nudgeVolume changes the player's volume by d (0..1 scale), if it has one.
func (m *music) nudgeVolume(player string, d float64) {
	if m == nil || player == "" {
		return
	}
	go func() {
		obj := m.conn.Object(player, "/org/mpris/MediaPlayer2")
		v, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.Volume")
		if err != nil {
			return
		}
		cur, _ := v.Value().(float64)
		obj.SetProperty("org.mpris.MediaPlayer2.Player.Volume", dbus.MakeVariant(math.Max(0, math.Min(1, cur+d))))
	}()
}

// playerName is the friendly name of a bus name ("org.mpris.MediaPlayer2.YoutubeMusic" -> "YoutubeMusic").
func playerName(bus string) string {
	n := strings.TrimPrefix(bus, mprisPrefix)
	if i := strings.IndexByte(n, '.'); i > 0 {
		n = n[:i]
	}
	return n
}
