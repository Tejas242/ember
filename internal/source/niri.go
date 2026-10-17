package source

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/Tejas242/ember/internal/xdg"
)

// The desktop as niri sees it: which window has focus, what each terminal
// is titled, whether the screen is being cast, and screenshots.

// NiriSocket is niri's IPC socket: $NIRI_SOCKET, else the one in the runtime
// dir, else "".
func NiriSocket() string {
	if s := os.Getenv("NIRI_SOCKET"); s != "" {
		return s
	}
	ms, _ := filepath.Glob(filepath.Join(xdg.RuntimeDir(), "niri.*.sock"))
	if len(ms) > 0 {
		return ms[len(ms)-1]
	}
	return ""
}

// Niri follows niri's event stream until it ends (niri restarted). It
// speaks niri's IPC directly (no `niri msg` process): ask for the event
// stream, then read one JSON event per line.
func Niri(emit Emit) error {
	sock := NiriSocket()
	if sock == "" {
		return errors.New("no niri socket (not niri?)")
	}
	c, err := net.Dial("unix", sock)
	if err != nil {
		return err
	}
	defer c.Close()
	defer emit(NiriDown{})
	if _, err := c.Write([]byte("\"EventStream\"\n")); err != nil {
		return err
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		if e := parseNiri(sc.Bytes()); e != nil {
			emit(e)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return errors.New("niri's event stream ended")
}

// parseNiri reads one line of the event stream: the event it means, or nil.
func parseNiri(line []byte) Event {
	var ev struct {
		WindowsChanged *struct {
			Windows []Window `json:"windows"`
		}
		WindowOpenedOrChanged *struct {
			Window Window `json:"window"`
		}
		WindowClosed *struct {
			ID int `json:"id"`
		}
		WindowFocusChanged *struct {
			ID *int `json:"id"`
		}
		CastsChanged *struct {
			Casts []json.RawMessage `json:"casts"`
		}
		ScreenshotCaptured *struct {
			Path *string `json:"path"`
		}
	}
	if json.Unmarshal(line, &ev) != nil {
		return nil
	}
	switch {
	case ev.WindowsChanged != nil:
		return NiriWindows{ev.WindowsChanged.Windows}
	case ev.WindowOpenedOrChanged != nil:
		return NiriWindow{ev.WindowOpenedOrChanged.Window}
	case ev.WindowClosed != nil:
		return NiriWindowClosed{ev.WindowClosed.ID}
	case ev.WindowFocusChanged != nil:
		id := 0
		if ev.WindowFocusChanged.ID != nil {
			id = *ev.WindowFocusChanged.ID
		}
		return NiriFocus{id}
	case ev.CastsChanged != nil:
		return NiriCasts{len(ev.CastsChanged.Casts)}
	case ev.ScreenshotCaptured != nil:
		path := ""
		if p := ev.ScreenshotCaptured.Path; p != nil {
			path = *p
		}
		return Screenshot{path}
	}
	return nil
}
