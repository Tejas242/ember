package island

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Tejas242/ember/internal/source"
)

// The desktop as niri sees it, kept current by its event stream (package
// source): which window has focus (so "Done" stays quiet while you're
// looking), what each terminal is titled (Claude names its sessions there),
// and whether the screen is being cast.

type deskWindow = source.Window

type desktop struct {
	live    bool // the stream is up
	windows map[int]deskWindow
	focused int
	casts   int
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

// sessionWindow finds the terminal window that hosts a session: the window
// of the nearest process in its chain (a terminal opened from another one's
// shell has that one further up). None if that terminal runs several windows
// in one process: which of them is the session's is anyone's guess.
func (a *app) sessionWindow(pids []int) (deskWindow, bool) {
	for _, p := range pids {
		var found []deskWindow
		for _, w := range a.desk.windows {
			if w.Pid == p {
				found = append(found, w)
			}
		}
		if len(found) == 1 {
			return found[0], true
		}
		if len(found) > 1 {
			break
		}
	}
	return deskWindow{}, false
}

// looking: the session's terminal has your focus, so you see it finish.
func (a *app) looking(s *Session) bool {
	if a.desk.live {
		w, ok := a.sessionWindow(s.Pids)
		return ok && w.ID == a.desk.focused
	}
	return slices.Contains(s.Pids, a.focusedPid())
}

// deskChanged: titles may have changed, and looking at a session's
// terminal answers its "Done".
func (a *app) deskChanged() {
	changed := false // Claude's title spinner ticks every second: only a real change costs a frame
	for _, s := range a.ui.sessions {
		w, ok := a.sessionWindow(s.Pids)
		if watching := ok && w.ID == a.desk.focused; watching != a.ui.watching[s.ID] {
			a.ui.watching[s.ID], changed = watching, true // attention: you can see this one already
		}
		if ok {
			if t := sessionTitle(w.Title); t != "" && t != s.Title {
				s.Title, changed = t, true
			}
			if w.ID == a.desk.focused && a.ui.moment != nil && a.ui.moment.focus == s.ID {
				a.ui.moment, changed = nil, true
			}
		}
	}
	a.quiet = !changed
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
