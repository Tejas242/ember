//go:build capture

package source

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// A dev tool, never built into Ember: it records what the live analysis
// hears (the default monitor, as parec gives it to Audio.Set) and where the
// player says it is, for replaying real songs offline. Nothing it writes is
// for committing; delete it once measured.
//
//	EMBER_CAPTURE=/home/screenager/.cache/ember-lipsync/real EMBER_CAPTURE_SONGS=4 \
//	EMBER_CAPTURE_ENV="$DBUS_SESSION_BUS_ADDRESS|$XDG_RUNTIME_DIR|$HOME" \
//	  go test -tags capture -count=1 -timeout 0 -v -run TestCapture ./internal/source
//
// (the tests run sandboxed: EMBER_CAPTURE_ENV hands over the real session
// bus, runtime dir and home, for the player and the sound server)
//
// Out come, per run: <t>.pcm (s16le stereo at AudioRate, as heard), <t>.hops
// (int64 unix ns: when each hop of AudioHop frames was read, as Audio.read
// stamps it) and <t>.mpris (JSON lines, five a second: the playing player's
// track, playhead and status, and when they were read). It stops after
// EMBER_CAPTURE_SONGS whole songs (default 4) or EMBER_CAPTURE_MAX (default
// 25m), or when nothing has played for two minutes.
func TestCapture(t *testing.T) {
	dir := os.Getenv("EMBER_CAPTURE")
	if dir == "" {
		t.Skip("set EMBER_CAPTURE")
	}
	songs := 4
	if v, err := strconv.Atoi(os.Getenv("EMBER_CAPTURE_SONGS")); err == nil {
		songs = v
	}
	maxD := 25 * time.Minute
	if v, err := time.ParseDuration(os.Getenv("EMBER_CAPTURE_MAX")); err == nil {
		maxD = v
	}
	os.MkdirAll(dir, 0o700)
	base := filepath.Join(dir, time.Now().Format("20060102-150405"))
	pcmF, _ := os.Create(base + ".pcm")
	hopF, _ := os.Create(base + ".hops")
	mpF, _ := os.Create(base + ".mpris")
	defer pcmF.Close()
	defer hopF.Close()
	defer mpF.Close()
	pcmW, hopW, mpW := bufio.NewWriter(pcmF), bufio.NewWriter(hopF), bufio.NewWriter(mpF)
	defer pcmW.Flush()
	defer hopW.Flush()
	defer mpW.Flush()

	real := strings.Split(os.Getenv("EMBER_CAPTURE_ENV"), "|")
	if len(real) != 3 {
		t.Fatal("set EMBER_CAPTURE_ENV")
	}
	cmd := exec.Command("parec", "--device=@DEFAULT_MONITOR@", "--format=s16le", "--rate=8000", "--channels=2",
		"--latency-msec=30", "--client-name=ember-capture", "--stream-name=capture")
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+real[1], "HOME="+real[2])
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		t.Fatal("parec", err)
	}
	defer cmd.Process.Kill()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { // as Audio.read: a hop at a time, stamped when it's read
		defer close(done)
		buf := make([]byte, AudioHop*4)
		var ts [8]byte
		for {
			if _, err := io.ReadFull(out, buf); err != nil {
				return
			}
			binary.LittleEndian.PutUint64(ts[:], uint64(time.Now().UnixNano()))
			select {
			case <-stop:
				return
			default:
			}
			pcmW.Write(buf)
			hopW.Write(ts[:])
		}
	}()

	conn, err := dbus.Connect(real[0])
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	type sample struct {
		At     int64   `json:"at"` // unix ns
		Player string  `json:"player"`
		Status string  `json:"status"`
		Title  string  `json:"title"`
		Artist string  `json:"artist"`
		Album  string  `json:"album"`
		Len    float64 `json:"len"` // s
		Pos    float64 `json:"pos"` // s
	}
	enc := json.NewEncoder(mpW)
	start, lastPlay := time.Now(), time.Now()
	var titles []string
	for time.Since(start) < maxD && time.Since(lastPlay) < 2*time.Minute {
		var names []string
		conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
		var s sample
		for _, n := range names {
			if !strings.HasPrefix(n, "org.mpris.MediaPlayer2.") {
				continue
			}
			o := conn.Object(n, "/org/mpris/MediaPlayer2")
			st, err := o.GetProperty("org.mpris.MediaPlayer2.Player.PlaybackStatus")
			if err != nil || st.Value() != "Playing" {
				continue
			}
			s.At = time.Now().UnixNano()
			pos, _ := o.GetProperty("org.mpris.MediaPlayer2.Player.Position")
			us, _ := pos.Value().(int64)
			s.Player, s.Status, s.Pos = n, "Playing", float64(us)/1e6
			if md, err := o.GetProperty("org.mpris.MediaPlayer2.Player.Metadata"); err == nil {
				m, _ := md.Value().(map[string]dbus.Variant)
				s.Title, _ = m["xesam:title"].Value().(string)
				s.Album, _ = m["xesam:album"].Value().(string)
				if a, ok := m["xesam:artist"].Value().([]string); ok && len(a) > 0 {
					s.Artist = a[0]
				}
				switch l := m["mpris:length"].Value().(type) {
				case int64:
					s.Len = float64(l) / 1e6
				case uint64:
					s.Len = float64(l) / 1e6
				}
			}
			break
		}
		if s.Player != "" {
			lastPlay = time.Now()
			enc.Encode(s)
			if len(titles) == 0 || titles[len(titles)-1] != s.Title {
				titles = append(titles, s.Title)
				t.Logf("%s now: %s", time.Now().Format("15:04:05"), s.Title)
			}
			if len(titles) > songs+1 { // the first may be caught partway: whole songs between
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	close(stop)
	cmd.Process.Kill()
	<-done
	t.Logf("captured %d tracks to %s.*", len(titles), base)
}
