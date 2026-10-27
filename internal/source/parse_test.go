package source

import (
	"bufio"
	"net"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
)

// recorder is a fake Emit that keeps what it hears.
type recorder struct {
	mu sync.Mutex
	ev []Event
}

func (r *recorder) emit(e Event) { r.mu.Lock(); r.ev = append(r.ev, e); r.mu.Unlock() }

func (r *recorder) events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.ev...)
}

func TestParseNiri(t *testing.T) {
	for line, want := range map[string]Event{
		`{"WindowsChanged":{"windows":[{"id":3,"title":"✳ Fix it","app_id":"kitty","pid":42,"is_focused":true}]}}`:                      NiriWindows{[]Window{{ID: 3, Title: "✳ Fix it", AppID: "kitty", Pid: 42, Focused: true}}},
		`{"WindowOpenedOrChanged":{"window":{"id":5,"title":"vim","pid":7}}}`:                                                           NiriWindow{Window{ID: 5, Title: "vim", Pid: 7}},
		`{"WindowOpenedOrChanged":{"window":{"id":6,"workspace_id":2,"layout":{"tile_size":[1366.0,768.0],"window_size":[1366,768]}}}}`: NiriWindow{Window{ID: 6, Workspace: 2, Layout: WindowLayout{Size: [2]int{1366, 768}}}},
		`{"WindowLayoutsChanged":{"changes":[[6,{"window_size":[674,734]}]]}}`:                                                          NiriLayouts{map[int]WindowLayout{6: {Size: [2]int{674, 734}}}},
		`{"WorkspacesChanged":{"workspaces":[{"id":2,"output":"eDP-1"},{"id":5,"output":null}]}}`:                                       NiriWorkspaces{map[int]string{2: "eDP-1"}},
		`{"WindowClosed":{"id":5}}`:                                NiriWindowClosed{5},
		`{"WindowFocusChanged":{"id":3}}`:                          NiriFocus{3},
		`{"WindowFocusChanged":{"id":null}}`:                       NiriFocus{0},
		`{"CastsChanged":{"casts":[{"id":1},{"id":2}]}}`:           NiriCasts{2},
		`{"CastsChanged":{"casts":[]}}`:                            NiriCasts{0},
		`{"ScreenshotCaptured":{"path":"/home/x/Pictures/s.png"}}`: Screenshot{"/home/x/Pictures/s.png"},
		`{"ScreenshotCaptured":{"path":null}}`:                     Screenshot{""},
		`{"WorkspacesChanged":{"workspaces":[]}}`:                  NiriWorkspaces{map[int]string{}},
		`not json`: nil,
	} {
		if got := parseNiri([]byte(line)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %#v\nwant %#v", line, got, want)
		}
	}
}

// Niri speaks the real protocol: asks for the stream, emits each event, and
// says NiriDown when the stream ends.
func TestNiriStream(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "niri.test.sock")
	t.Setenv("NIRI_SOCKET", sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	asked := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		req, _ := bufio.NewReader(c).ReadString('\n')
		asked <- req
		c.Write([]byte(`{"WindowFocusChanged":{"id":9}}` + "\n" + `{"CastsChanged":{"casts":[{}]}}` + "\n"))
		c.Close()
	}()
	var r recorder
	if err := Niri(r.emit); err == nil {
		t.Fatal("a stream that ended should say so")
	}
	if req := <-asked; req != "\"EventStream\"\n" {
		t.Fatalf("asked %q", req)
	}
	if got, want := r.events(), []Event{NiriFocus{9}, NiriCasts{1}, NiriDown{}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	t.Setenv("NIRI_SOCKET", "")
	if err := Niri(r.emit); err == nil {
		t.Fatal("no niri: an error")
	}
}

func TestPactlLines(t *testing.T) {
	for ln, want := range map[string]bool{
		"Event 'change' on sink #57":          true,
		"Event 'change' on source #58":        true,
		"Event 'change' on server #0":         true,
		"Event 'new' on sink-input #120":      false,
		"Event 'remove' on source-output #33": false,
		"Event 'change' on sink-input #120":   false,
		"Event 'change' on client #9":         false,
		"":                                    false,
	} {
		if levelEvent(ln) != want {
			t.Errorf("levelEvent(%q) = %v", ln, !want)
		}
	}
}

func TestWpctlLevels(t *testing.T) {
	for out, want := range map[string]Volume{
		"Volume: 0.42\n":         {V: 0.42, OK: true},
		"Volume: 1.00 [MUTED]\n": {V: 1, Muted: true, OK: true},
		"Volume: 1.50\n":         {V: 1.5, OK: true},
		"Volume: 0.00 [MUTED]":   {Muted: true, OK: true},
		"wpctl: no such node\n":  {},
		"":                       {},
	} {
		if got := parseLevel([]byte(out)); got != want {
			t.Errorf("parseLevel(%q) = %+v, want %+v", out, got, want)
		}
	}
}

func TestUPowerValues(t *testing.T) {
	props := map[string]dbus.Variant{
		"Percentage":  dbus.MakeVariant(66.0),
		"State":       dbus.MakeVariant(uint32(2)),
		"IsPresent":   dbus.MakeVariant(true),
		"TimeToEmpty": dbus.MakeVariant(int64(9660)),
		"TimeToFull":  dbus.MakeVariant(int64(0)),
	}
	if p := powerOf(props); p != (Power{Pct: 66, State: 2, Present: true, TTE: 9660}) {
		t.Fatalf("%+v", p)
	}
	if p := powerOf(map[string]dbus.Variant{"Percentage": dbus.MakeVariant("x")}); p != (Power{}) {
		t.Fatalf("bad types must read as nothing: %+v", p)
	}
}

func TestNoctaliaShotDirs(t *testing.T) {
	cfg := "[bar]\ndirectory = \"/nope\"\n[general.screenshot]\ndirectory = \"~/Shots\"\n[other]\n"
	if got := noctaliaShotDirs(cfg, "/home/x"); !reflect.DeepEqual(got, []string{"/home/x/Shots"}) {
		t.Fatal(got)
	}
}

// Supervised, every source just reports why it can't run here (the test
// sandbox has no buses, no niri, no backlight), without panicking or hanging.
func TestSourcesInTheSandbox(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no pactl, no noctalia
	var r recorder
	for name, fn := range map[string]func(Emit) error{
		"volume": Levels, "niri": Niri, "screenshots": Screenshots, "shell settings": ShellSettings, "shell": Shell,
		"music": NewMPRIS().Run,
	} {
		if err := fn(r.emit); err == nil {
			t.Errorf("%s ran in the sandbox", name)
		}
	}
	if err := Levels(r.emit); err == nil || err.Error() != "pactl missing" {
		t.Errorf("volume: %v", err)
	}
}

func TestParseOutputs(t *testing.T) {
	got, err := parseOutputs([]byte(`{"Ok":{"Outputs":{"eDP-1":{"name":"eDP-1","logical":{"x":0,"y":0,"width":1366,"height":768,"scale":1.0}},"HDMI-A-1":{"logical":null}}}}`))
	if err != nil || !reflect.DeepEqual(got, map[string][2]int{"eDP-1": {1366, 768}}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := parseOutputs([]byte(`{"Err":"no"}`)); err == nil {
		t.Fatal("an error reply should say so")
	}
}
