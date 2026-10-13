package island

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode"
)

// The desktop as niri sees it, kept current by `niri msg -j event-stream`:
// which window has focus (so "Done" stays quiet while you're looking), what
// each terminal is titled (Claude names its sessions there), and whether
// the screen is being cast.

type deskWindow struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	AppID   string `json:"app_id"`
	Pid     int    `json:"pid"`
	Focused bool   `json:"is_focused"`
}

type desktop struct {
	live    bool // the stream is up
	windows map[int]deskWindow
	focused int
	casts   int
}

func watchNiri(posts chan<- func(*app)) {
	if _, err := exec.LookPath("niri"); err != nil {
		return
	}
	for {
		streamNiri(posts)
		posts <- func(a *app) { a.desk.live = false }
		time.Sleep(5 * time.Second) // niri restarted, or isn't the compositor
	}
}

func streamNiri(posts chan<- func(*app)) {
	cmd := exec.Command("niri", "msg", "-j", "event-stream")
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return
	}
	defer cmd.Wait()
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var ev struct {
			WindowsChanged *struct {
				Windows []deskWindow `json:"windows"`
			}
			WindowOpenedOrChanged *struct {
				Window deskWindow `json:"window"`
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
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch {
		case ev.WindowsChanged != nil:
			ws := ev.WindowsChanged.Windows
			posts <- func(a *app) {
				a.desk.live, a.desk.windows, a.desk.focused = true, map[int]deskWindow{}, 0
				for _, w := range ws {
					a.desk.windows[w.ID] = w
					if w.Focused {
						a.desk.focused = w.ID
					}
				}
				a.deskChanged()
			}
		case ev.WindowOpenedOrChanged != nil:
			w := ev.WindowOpenedOrChanged.Window
			posts <- func(a *app) {
				if a.desk.windows == nil {
					a.desk.windows = map[int]deskWindow{}
				}
				a.desk.windows[w.ID] = w
				if w.Focused {
					a.desk.focused = w.ID
				}
				a.deskChanged()
			}
		case ev.WindowClosed != nil:
			id := ev.WindowClosed.ID
			posts <- func(a *app) { delete(a.desk.windows, id) }
		case ev.WindowFocusChanged != nil:
			id := 0
			if ev.WindowFocusChanged.ID != nil {
				id = *ev.WindowFocusChanged.ID
			}
			posts <- func(a *app) { a.desk.focused = id; a.deskChanged() }
		case ev.CastsChanged != nil:
			n := len(ev.CastsChanged.Casts)
			posts <- func(a *app) { a.castsChanged(n) }
		case ev.ScreenshotCaptured != nil:
			path := ""
			if p := ev.ScreenshotCaptured.Path; p != nil {
				path = *p
			}
			if path != "" {
				go screenshotTaken(posts, path)
			} else {
				posts <- func(a *app) { a.screenshot("", nil) }
			}
		}
	}
}

// sessionTitle is the topic Claude Code puts in its terminal's title
// ("✳ Fix the flaky test", or a spinner while it works). A plain shell
// title doesn't count.
func sessionTitle(title string) string {
	t := strings.TrimSpace(title)
	r := []rune(t)
	if len(r) < 3 || unicode.IsLetter(r[0]) || unicode.IsDigit(r[0]) || r[0] == '~' || r[0] == '/' {
		return ""
	}
	t = strings.TrimSpace(strings.TrimLeftFunc(t, func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsDigit(c) }))
	if t == "" || strings.EqualFold(t, "claude") || strings.EqualFold(t, "claude code") {
		return ""
	}
	return t
}

// sessionWindow finds the terminal window that hosts one of pids.
func (a *app) sessionWindow(pids []int) (deskWindow, bool) {
	for _, w := range a.desk.windows {
		if slices.Contains(pids, w.Pid) {
			return w, true
		}
	}
	return deskWindow{}, false
}

// deskChanged: titles may have changed, and looking at a session's
// terminal answers its "Done".
func (a *app) deskChanged() {
	for _, s := range a.ui.sessions {
		if w, ok := a.sessionWindow(s.Pids); ok {
			if t := sessionTitle(w.Title); t != "" {
				s.Title = t
			}
			if w.ID == a.desk.focused && a.ui.moment != nil && a.ui.moment.focus == s.ID {
				a.ui.moment = nil
			}
		}
	}
}

func (a *app) castsChanged(n int) {
	was := a.desk.casts
	a.desk.casts, a.ui.casting = n, n > 0 && a.cfg.Privacy
	if n > 0 && was == 0 && a.cfg.Privacy {
		a.ui.Moment(moment{key: "cast", icon: "cast", tint: a.ui.pal.red, text: "Sharing your screen",
			until: a.ui.now().Add(3 * time.Second)})
	}
}

// focusedPid is the pid of the focused window, 0 if unknown.
func (a *app) focusedPid() int {
	if a.desk.live {
		return a.desk.windows[a.desk.focused].Pid
	}
	out, err := exec.Command("niri", "msg", "-j", "focused-window").Output()
	if err != nil {
		return 0
	}
	var w struct {
		Pid int `json:"pid"`
	}
	json.Unmarshal(out, &w)
	return w.Pid
}

// focusSession raises the terminal hosting a session: niri, Hyprland or Sway.
func (a *app) focusSession(pids []int) {
	if w, ok := a.sessionWindow(pids); ok {
		go exec.Command("niri", "msg", "action", "focus-window", "--id", fmt.Sprint(w.ID)).Run()
		return
	}
	go focusTerminal(pids)
}

func focusTerminal(pids []int) {
	if out, err := exec.Command("niri", "msg", "-j", "windows").Output(); err == nil {
		var wins []deskWindow
		json.Unmarshal(out, &wins)
		for _, p := range pids {
			for _, w := range wins {
				if w.Pid == p {
					exec.Command("niri", "msg", "action", "focus-window", "--id", fmt.Sprint(w.ID)).Run()
					return
				}
			}
		}
		return
	}
	if out, err := exec.Command("hyprctl", "-j", "clients").Output(); err == nil {
		var cs []struct {
			Pid     int    `json:"pid"`
			Address string `json:"address"`
		}
		json.Unmarshal(out, &cs)
		for _, p := range pids {
			for _, c := range cs {
				if c.Pid == p {
					exec.Command("hyprctl", "dispatch", "focuswindow", "address:"+c.Address).Run()
					return
				}
			}
		}
		return
	}
	if _, err := exec.LookPath("swaymsg"); err == nil {
		for _, p := range pids {
			if exec.Command("swaymsg", fmt.Sprintf("[pid=%d] focus", p)).Run() == nil {
				return
			}
		}
	}
}
