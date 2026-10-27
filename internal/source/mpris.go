package source

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// Track is what the active media player plays.
type Track struct {
	Player   string // bus name
	Title    string
	Artist   string
	ArtURL   string
	Album    string
	URL      string // the track's own address (file:// for a local file)
	TrackID  string // the player's id for it (mpris:trackid)
	Playing  bool
	Length   time.Duration
	Position time.Duration
	PosAt    time.Time // when Position was read; it advances from there while playing
	PausedAt time.Time // when it stopped playing
}

// Pos is the playhead at now.
func (t Track) Pos(now time.Time) time.Duration {
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

// MPRIS watches the media players on the session bus, waking only on their
// signals, and controls them. Until Run is connected (and on a nil *MPRIS)
// the controls do nothing.
type MPRIS struct {
	mu     sync.Mutex
	conn   *dbus.Conn // written only by Run, under mu
	emit   Emit
	recent map[string]time.Time // player -> last time it was playing (Run's own)
}

// NewMPRIS is a player watcher waiting for Run.
func NewMPRIS() *MPRIS { return &MPRIS{recent: map[string]time.Time{}} }

// bus is the connection and the emit, for the controls.
func (m *MPRIS) bus() (*dbus.Conn, Emit) {
	if m == nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conn, m.emit
}

// Connected says Run is watching the session bus.
func (m *MPRIS) Connected() bool { c, _ := m.bus(); return c != nil }

// Run watches the players until the session bus goes away.
func (m *MPRIS) Run(emit Emit) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("no session bus: %w", err)
	}
	m.mu.Lock()
	m.conn, m.emit = conn, emit
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.conn = nil
		m.mu.Unlock()
		conn.Close()
	}()
	conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchObjectPath("/org/mpris/MediaPlayer2"))
	conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg0Namespace("org.mpris.MediaPlayer2"))
	conn.AddMatchSignal(dbus.WithMatchInterface("org.mpris.MediaPlayer2.Player"), dbus.WithMatchMember("Seeked"))
	sig := make(chan *dbus.Signal, 16)
	conn.Signal(sig)
	m.refresh()
	var pending <-chan time.Time
	for {
		select {
		case _, ok := <-sig:
			if !ok {
				return errors.New("the session bus went away")
			}
			if pending == nil { // coalesce bursts (players send several at once)
				pending = time.After(80 * time.Millisecond)
			}
		case <-pending:
			pending = nil
			m.refresh()
		}
	}
}

func (m *MPRIS) players() []string {
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

func (m *MPRIS) read(name string) (Track, bool) {
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
	if v, ok := md["xesam:album"]; ok {
		t.Album, _ = v.Value().(string)
	}
	if v, ok := md["xesam:url"]; ok {
		t.URL, _ = v.Value().(string)
	}
	switch id := md["mpris:trackid"].Value().(type) {
	case dbus.ObjectPath:
		t.TrackID = string(id)
	case string:
		t.TrackID = id
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
func (m *MPRIS) refresh() {
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
	m.emit(TrackChanged{best})
}

// Control calls a Player method (PlayPause, Next, Previous) on player.
func (m *MPRIS) Control(player, method string) {
	conn, _ := m.bus()
	if conn == nil || player == "" {
		return
	}
	go conn.Object(player, "/org/mpris/MediaPlayer2").Call("org.mpris.MediaPlayer2.Player."+method, 0)
}

// ReadPosition re-reads player's playhead and says it as a Playhead (only
// while the card shows a progress bar).
func (m *MPRIS) ReadPosition(player string) {
	conn, emit := m.bus()
	if conn == nil || player == "" {
		return
	}
	go func() {
		v, err := conn.Object(player, "/org/mpris/MediaPlayer2").GetProperty("org.mpris.MediaPlayer2.Player.Position")
		if err != nil {
			return
		}
		us, _ := v.Value().(int64)
		emit(Playhead{Player: player, Pos: time.Duration(us) * time.Microsecond, At: time.Now()})
	}()
}

// Seek moves the playhead by delta (MPRIS Seek takes microseconds).
func (m *MPRIS) Seek(player string, delta time.Duration) {
	conn, _ := m.bus()
	if conn == nil || player == "" {
		return
	}
	go conn.Object(player, "/org/mpris/MediaPlayer2").Call("org.mpris.MediaPlayer2.Player.Seek", 0, int64(delta/time.Microsecond))
}

// NudgeVolume changes the player's volume by d (0..1 scale), if it has
// one, and tells then the new volume (on its own goroutine).
func (m *MPRIS) NudgeVolume(player string, d float64, then func(v float64)) {
	conn, _ := m.bus()
	if conn == nil || player == "" {
		return
	}
	go func() {
		obj := conn.Object(player, "/org/mpris/MediaPlayer2")
		v, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.Volume")
		if err != nil {
			return
		}
		cur, _ := v.Value().(float64)
		nv := math.Max(0, math.Min(1, cur+d))
		if obj.SetProperty("org.mpris.MediaPlayer2.Player.Volume", dbus.MakeVariant(nv)) == nil && then != nil {
			then(nv)
		}
	}()
}
