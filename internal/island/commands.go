package island

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// Commands: the keys the UI asks for (buttons, controls) and the ones that
// come over the socket (`ember allow`, `ember open`, …).

func (a *app) key(k string) {
	switch {
	case strings.HasPrefix(k, "focus:"):
		if s := a.ui.sessions[strings.TrimPrefix(k, "focus:")]; s != nil {
			a.focusSession(s.Pids)
		}
	case k == "sys:mute":
		go source.ToggleMute()
	case strings.HasPrefix(k, "sys:vol:nudge:"), strings.HasPrefix(k, "sys:light:nudge:"):
		dir, _ := strconv.Atoi(k[strings.LastIndexByte(k, ':')+1:])
		if strings.HasPrefix(k, "sys:vol") {
			go source.NudgeVolume(dir)
		} else {
			go source.NudgeBrightness(dir)
		}
	case strings.HasPrefix(k, "sys:vol:"):
		f, _ := strconv.ParseFloat(strings.TrimPrefix(k, "sys:vol:"), 64)
		go source.SetVolume(f)
	case strings.HasPrefix(k, "sys:light:"):
		f, _ := strconv.ParseFloat(strings.TrimPrefix(k, "sys:light:"), 64)
		go source.SetBrightness(f)
	case strings.HasPrefix(k, "x:click:"), strings.HasPrefix(k, "x:btn:"), strings.HasPrefix(k, "x:dbtn:"):
		a.extKey(k)
	case strings.HasPrefix(k, "open:"):
		go exec.Command("xdg-open", strings.TrimPrefix(k, "open:")).Run()
	case strings.HasPrefix(k, "music:seek:") && a.track != nil:
		d, _ := strconv.ParseInt(strings.TrimPrefix(k, "music:seek:"), 10, 64)
		a.music.Seek(a.track.Player, time.Duration(d))
	case strings.HasPrefix(k, "music:volume:") && a.track != nil:
		d, _ := strconv.Atoi(strings.TrimPrefix(k, "music:volume:"))
		a.music.NudgeVolume(a.track.Player, float64(d)*0.05)
	case strings.HasPrefix(k, "music:") && a.track != nil:
		if method := map[string]string{"music:prev": "Previous", "music:toggle": "PlayPause", "music:next": "Next"}[k]; method != "" {
			a.music.Control(a.track.Player, method)
			if method == "PlayPause" { // feel instant; the player confirms a moment later
				a.track.Playing = !a.track.Playing
				a.track.PosAt = a.ui.now()
			}
		}
	}
}

// command handles keyboard commands: answer the oldest request.
func (a *app) command(cmd, arg string) {
	switch cmd {
	case "toggle":
		a.ui.pinned, a.ui.suppress = !a.ui.pinned, false
		return
	case "open":
		a.ui.pinned, a.ui.suppress = true, false
		return
	case "close":
		a.ui.pinned, a.ui.suppress = false, a.ui.hover // only while the pointer is on it
		return
	case "dash":
		a.ui.toggleDash(arg)
		return
	case "next", "prev":
		dir := 1
		if cmd == "prev" {
			dir = -1
		}
		a.ui.cycle(dir)
		return
	case "play":
		a.key("music:toggle")
		return
	}
	for _, c := range a.ui.cards {
		if c.Kind != "perm" && c.Kind != "ask" {
			continue
		}
		key := cmd
		switch cmd {
		case "answer":
			key = arg
		case "terminal":
			key = "footer"
		}
		if c.Kind == "ask" {
			if strings.HasPrefix(key, "opt:") {
				i, _ := strconv.Atoi(strings.TrimPrefix(key, "opt:"))
				if c.Multi {
					if c.picked == nil {
						c.picked = map[int]bool{}
					}
					c.picked[i-1] = !c.picked[i-1]
					return
				}
				a.ui.RemoveCard(c.ID)
				c.OnPick("option", []int{i - 1})
				return
			}
			if key == "submit" {
				var picked []int
				for i := range c.Options {
					if c.picked[i] {
						picked = append(picked, i)
					}
				}
				a.ui.RemoveCard(c.ID)
				c.OnPick("submit", picked)
				return
			}
		}
		a.ui.RemoveCard(c.ID)
		c.OnPick(key, nil)
		return
	}
}
