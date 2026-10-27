package source

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"

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
	sizes := map[string][2]int{}
	for sc.Scan() {
		e := parseNiri(sc.Bytes())
		if e == nil {
			continue
		}
		if ws, ok := e.(NiriWorkspaces); ok && unknownOutput(ws, sizes) { // a new output: ask its size (once)
			if s, err := niriOutputs(sock); err == nil {
				sizes = s
				emit(NiriOutputs{s})
			}
		}
		emit(e)
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
		WindowLayoutsChanged *struct {
			Changes [][2]json.RawMessage `json:"changes"`
		}
		WorkspacesChanged *struct {
			Workspaces []struct {
				ID     int     `json:"id"`
				Output *string `json:"output"`
			} `json:"workspaces"`
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
	case ev.WindowLayoutsChanged != nil:
		ls := map[int]WindowLayout{}
		for _, ch := range ev.WindowLayoutsChanged.Changes {
			var id int
			var l WindowLayout
			if json.Unmarshal(ch[0], &id) == nil && json.Unmarshal(ch[1], &l) == nil {
				ls[id] = l
			}
		}
		return NiriLayouts{ls}
	case ev.WorkspacesChanged != nil:
		outs := map[int]string{}
		for _, w := range ev.WorkspacesChanged.Workspaces {
			if w.Output != nil {
				outs[w.ID] = *w.Output
			}
		}
		return NiriWorkspaces{outs}
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

func unknownOutput(ws NiriWorkspaces, sizes map[string][2]int) bool {
	for _, o := range ws.Outputs {
		if _, ok := sizes[o]; !ok {
			return true
		}
	}
	return false
}

// niriOutputs asks niri each output's logical size (a request of its own:
// the event stream doesn't carry them).
func niriOutputs(sock string) (map[string][2]int, error) {
	c, err := net.Dial("unix", sock)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write([]byte("\"Outputs\"\n")); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	if !sc.Scan() {
		return nil, errors.New("niri didn't answer Outputs")
	}
	return parseOutputs(sc.Bytes())
}

func parseOutputs(line []byte) (map[string][2]int, error) {
	var r struct {
		Ok *struct {
			Outputs map[string]struct {
				Logical *struct {
					W int `json:"width"`
					H int `json:"height"`
				} `json:"logical"`
			}
		}
	}
	if err := json.Unmarshal(line, &r); err != nil {
		return nil, err
	}
	if r.Ok == nil {
		return nil, errors.New("niri: Outputs failed")
	}
	sizes := map[string][2]int{}
	for name, o := range r.Ok.Outputs {
		if o.Logical != nil {
			sizes[name] = [2]int{o.Logical.W, o.Logical.H}
		}
	}
	return sizes, nil
}
