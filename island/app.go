package island

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	surfW, surfH = 440, 280
	flickerEvery = time.Second / 12
	idleEvery    = time.Second / 6
	frameGrace   = 50 * time.Millisecond // a frame callback that never comes doesn't stall us
)

type app struct {
	ui    *UI
	win   *window
	music *music
	track *Track
	posts chan func(*app)
}

// barHeight reads Noctalia's bar thickness so the pill matches it exactly.
func barHeight() float32 {
	out, err := exec.Command("noctalia", "config", "export").Output()
	if err == nil {
		if m := regexp.MustCompile(`(?m)^\s*thickness\s*=\s*([0-9.]+)`).FindSubmatch(out); m != nil {
			if v, err := strconv.ParseFloat(string(m[1]), 32); err == nil && v >= 16 && v <= 64 {
				return float32(v)
			}
		}
	}
	return 24
}

// Run opens the island and serves until the compositor goes away.
func Run() error {
	posts := make(chan func(*app), 64)
	if _, err := listen(posts); err != nil {
		return err
	}
	win, err := openWindow(surfW, surfH, 0, layerTop)
	if err != nil {
		return err
	}
	a := &app{ui: NewUI(surfW, surfH, barHeight()), win: win, posts: posts}
	a.ui.onKey = a.key
	a.music = startMusic(posts)
	frameDue := false
	win.onPointer = func(e pointerEvent) {
		redraw, hand := a.ui.pointer(e)
		win.setCursor(hand)
		if redraw {
			a.ui.Invalidate()
		}
	}
	win.onFrame = func() { frameDue = true }
	log.Printf("ember up (bar %.0fpx)", a.ui.BarH)

	animating, waitingFrame, tick := false, false, false
	var waitingSince time.Time
	draw := func() {
		var damage image.Rectangle
		animating, damage = a.ui.renderDamage()
		if win.present(a.ui.cv.img, a.ui.shape.bounds().Union(a.ui.bubble), damage, animating) {
			waitingFrame, waitingSince = animating, time.Now()
		} else {
			a.ui.dirty = true // no free buffer: try again on the next wake
		}
		if b := a.ui.shape.blurRects(); !slices.Equal(b, win.lastBlur) {
			win.setBlur(b)
		}
	}
	draw()
	housekeep := time.NewTicker(20 * time.Second)
	defer housekeep.Stop()
	timer := time.NewTimer(time.Hour)
	lastPos := time.Time{}
	for {
		select {
		case m := <-win.c.msgs:
			win.c.dispatch(m)
		case err := <-win.c.readErr:
			return err
		case f := <-posts:
			f(a)
			a.ui.Invalidate()
		case <-housekeep.C:
			if a.sweep() {
				a.ui.Invalidate()
			}
			if p := loadPalette(); p != a.ui.pal { // the wallpaper palette changed
				a.ui.pal = p
				a.ui.Invalidate()
			}
		case <-timer.C:
			tick = true
			if a.ui.expire() {
				a.ui.Invalidate()
			}
		}
		if win.closed {
			return fmt.Errorf("layer surface closed")
		}
		// the open card shows a progress bar: re-read the playhead now and then
		if a.ui.lastView == vExpanded && a.track != nil && a.track.Playing && time.Since(lastPos) > 5*time.Second {
			lastPos = time.Now()
			a.music.position(a.track.Player)
		}
		if waitingFrame && time.Since(waitingSince) > frameGrace {
			waitingFrame = false // niri skipped the callback (nothing on screen changed)
		}
		switch {
		case frameDue:
			frameDue, waitingFrame = false, false
			if animating || a.ui.dirty {
				draw()
			}
		case (a.ui.dirty || tick || animating) && !waitingFrame:
			draw()
		}
		tick = false
		timer.Reset(a.nextWake(animating))
		win.c.flush()
	}
}

// nextWake: animation runs on frame callbacks; otherwise sleep until the next
// blink, flicker tick, minute, hover delay or expiry.
func (a *app) nextWake(animating bool) time.Duration {
	u := a.ui
	now := u.now()
	if animating || u.dirty {
		return 16 * time.Millisecond // normally the frame callback comes first
	}
	next := u.nextBlink()
	if u.continuous() {
		next = now.Add(flickerEvery)
	} else if u.lively() {
		next = minTime(next, now.Add(idleEvery))
	}
	if u.lastView == vExpanded && a.track != nil && a.track.Playing {
		next = minTime(next, now.Add(time.Second)) // the progress bar moves
		u.dirty = true
	}
	next = minTime(next, now.Truncate(time.Minute).Add(time.Minute))
	for _, c := range u.cards {
		if !c.Expires.IsZero() {
			next = minTime(next, c.Expires)
		}
	}
	if u.hover && now.Sub(u.hoverAt) < hoverDelay {
		next = minTime(next, u.hoverAt.Add(hoverDelay))
	}
	if !u.hover && now.Sub(u.leftAt) < 400*time.Millisecond {
		next = minTime(next, u.leftAt.Add(330*time.Millisecond))
	}
	return max(next.Sub(now), 5*time.Millisecond)
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// sweep forgets sessions whose Claude process is gone.
func (a *app) sweep() bool {
	changed := false
	for id, s := range a.ui.sessions {
		if !slices.ContainsFunc(s.Pids, alive) || (s.State == "idle" && a.ui.now().Sub(s.Seen) > 3*time.Hour) {
			delete(a.ui.sessions, id)
			changed = true
		}
	}
	return changed
}

func (a *app) setTrack(t *Track) {
	prev := a.track
	a.track, a.ui.track = t, t
	if t == nil {
		a.ui.art = nil
		return
	}
	if prev == nil || prev.ArtURL != t.ArtURL {
		a.ui.art = nil
		if t.ArtURL != "" {
			loadArt(t.ArtURL, func(art *albumArt) {
				a.posts <- func(a *app) {
					if a.track != nil && a.track.ArtURL == art.url {
						a.ui.art = art
					}
				}
			})
		}
	}
}

func (a *app) key(k string) {
	switch {
	case strings.HasPrefix(k, "focus:"):
		if s := a.ui.sessions[strings.TrimPrefix(k, "focus:")]; s != nil {
			go focusTerminal(s.Pids)
		}
	case strings.HasPrefix(k, "music:") && a.track != nil:
		method := map[string]string{"music:prev": "Previous", "music:toggle": "PlayPause", "music:next": "Next"}[k]
		a.music.control(a.track.Player, method)
	case strings.HasPrefix(k, "btn:edge:"):
		edgeAction(strings.TrimPrefix(k, "btn:edge:"))
	}
}

// edgeAction runs `edge action <key>` (edge's buttons), if edge is installed.
func edgeAction(key string) {
	if _, err := exec.LookPath("edge"); err == nil {
		go exec.Command("edge", "action", key).Run()
	}
}

// command handles keyboard commands: answer the oldest request.
func (a *app) command(cmd, arg string) {
	if cmd == "toggle" {
		a.ui.pinned = !a.ui.pinned
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

// ---- edge ----

type edgeCard struct {
	ID      string `json:"id"`
	Mood    string `json:"mood"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Urgent  bool   `json:"urgent"`
	Bounce  bool   `json:"bounce"`
	Buttons []struct {
		Key     string `json:"key"`
		Label   string `json:"label"`
		Primary bool   `json:"primary"`
	} `json:"buttons"`
}

func (a *app) fromEdge(raw json.RawMessage) {
	var msg struct {
		Type   string      `json:"type"`
		Status *EdgeStatus `json:"status"`
		ID     string      `json:"id"`
		Card   *edgeCard   `json:"card"`
	}
	if json.Unmarshal(raw, &msg) != nil {
		return
	}
	switch msg.Type {
	case "status":
		a.ui.edge = msg.Status
	case "remove":
		a.ui.RemoveCard("edge:" + msg.ID)
	case "card":
		w := msg.Card
		if w == nil {
			return
		}
		moods := map[string]mood{"work": moodWork, "wait": moodWait, "happy": moodHappy, "error": moodError, "nudge": moodNudge}
		c := &Card{ID: "edge:" + w.ID, Kind: "edge", Mood: moods[w.Mood], Title: w.Title, Body: w.Body, Priority: 2}
		for _, b := range w.Buttons {
			c.Buttons = append(c.Buttons, Button{Key: b.Key, Label: b.Label, Primary: b.Primary})
		}
		if w.Urgent {
			c.Priority = 5
		}
		if len(c.Buttons) == 0 {
			c.Expires = a.ui.now().Add(7 * time.Second)
		}
		c.OnPick = func(k string, _ []int) {
			if k != "dismiss" {
				edgeAction(k)
			}
		}
		a.ui.AddCard(c)
		if w.Bounce {
			a.ui.Bounce()
		}
	}
}

var _ = url.Parse // used by the hook handlers below

// ---- Claude Code hooks ----

func (a *app) hook(m inMsg, answer chan<- json.RawMessage) string {
	var in hookInput
	json.Unmarshal(m.Payload, &in)
	now := a.ui.now()
	s := a.ui.sessions[in.SessionID]
	if s == nil && in.SessionID != "" && m.Event != "SessionEnd" {
		s = &Session{ID: in.SessionID, State: "idle", Since: now}
		a.ui.sessions[in.SessionID] = s
	}
	if s != nil {
		s.Seen = now
		if in.Cwd != "" {
			s.Project = project(in.Cwd)
		}
		if len(m.Pids) > 0 {
			s.Pids = m.Pids
		}
	}
	if s == nil {
		if answer != nil {
			answer <- nil
		}
		return ""
	}
	var tin map[string]any
	json.Unmarshal(in.ToolInput, &tin)

	switch m.Event {
	case "SessionEnd":
		delete(a.ui.sessions, in.SessionID)
	case "UserPromptSubmit":
		s.State, s.Label, s.Plus, s.Minus, s.Since = "work", "Thinking", 0, 0, now
	case "AskUserQuestion":
		if answer != nil {
			s.State = "wait"
			return a.ask(s, in, tin, answer)
		}
	case "PreToolUse":
		if in.Tool == "AskUserQuestion" {
			break
		}
		s.State = "work"
		s.Label, s.Plus, s.Minus = activity(in.Tool, tin)
	case "PostToolUse":
		if s.State == "wait" {
			s.State = "work"
		}
	case "PermissionRequest":
		if answer == nil {
			return ""
		}
		s.State = "wait"
		return a.permission(s, in, tin, answer)
	case "Notification":
		if in.NotifyType == "idle_prompt" {
			s.State = "wait"
		}
	case "Stop":
		s.State, s.Label = "idle", ""
		if slices.Contains(s.Pids, focusedPid()) {
			break // they're looking at it
		}
		body := strings.TrimSpace(in.LastMessage)
		if i := strings.IndexByte(body, '\n'); i > 0 {
			body = body[:i]
		}
		id := "done:" + s.ID
		a.ui.AddCard(&Card{ID: id, Kind: "done", Mood: moodHappy, Title: "Done", Meta: s.Project,
			Body: strings.TrimLeft(body, "#*- "), Expires: now.Add(9 * time.Second), Priority: 1,
			Buttons: []Button{{Key: "open", Label: "Open terminal"}},
			OnPick: func(k string, _ []int) {
				if k == "open" {
					go focusTerminal(s.Pids)
				}
			}})
		a.ui.Bounce()
	}
	return ""
}

func (a *app) permission(s *Session, in hookInput, tin map[string]any, answer chan<- json.RawMessage) string {
	str := func(k string) string { v, _ := tin[k].(string); return v }
	c := &Card{ID: "perm:" + s.ID + ":" + fmt.Sprint(time.Now().UnixNano()), Kind: "perm", Mood: moodWait,
		Meta: s.Project, Priority: 10, Footer: "In terminal", Expires: a.ui.now().Add(AskTimeout - 5*time.Second)}
	switch in.Tool {
	case "Bash":
		c.Title, c.Body, c.Code = "Run this command?", str("description"), str("command")
	case "Edit", "MultiEdit", "Write":
		verb := map[string]string{"Write": "Create"}[in.Tool]
		if verb == "" {
			verb = "Edit"
		}
		_, p, m := activity(in.Tool, tin)
		c.Title = verb + " " + filepath.Base(str("file_path")) + "?"
		c.Body = fmt.Sprintf("%s  ·  %s", rel(str("file_path"), s), diffText(max(p, 0), m))
		if p == 0 && m == 0 {
			c.Body = rel(str("file_path"), s)
		}
	case "WebFetch":
		u, _ := url.Parse(str("url"))
		c.Title, c.Code = "Fetch "+u.Host+"?", str("url")
	default:
		label, _, _ := activity(in.Tool, tin)
		c.Title = label + "?"
		if b, err := json.Marshal(tin); err == nil && len(b) > 2 {
			c.Code = string(b)
		}
	}
	c.Buttons = []Button{{Key: "deny", Label: "Deny", Danger: true}}
	if len(in.Suggestions) > 2 && string(in.Suggestions) != "null" {
		c.Buttons = append(c.Buttons, Button{Key: "always", Label: "Always"})
	}
	c.Buttons = append(c.Buttons, Button{Key: "allow", Label: "Allow", Primary: true})
	decide := func(decision map[string]any) {
		b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": "PermissionRequest", "decision": decision}})
		answer <- b
	}
	c.OnPick = func(k string, _ []int) {
		s.State = "work"
		switch k {
		case "allow":
			decide(map[string]any{"behavior": "allow"})
		case "always":
			decide(map[string]any{"behavior": "allow", "updatedPermissions": in.Suggestions})
		case "deny":
			decide(map[string]any{"behavior": "deny", "message": "Denied from the edge island."})
		default: // In terminal: no decision, Claude asks there
			answer <- nil
		}
	}
	c.OnExpire = func() { answer <- nil }
	a.ui.AddCard(c)
	return c.ID
}

func rel(p string, s *Session) string {
	if p == "" {
		return ""
	}
	return strings.TrimPrefix(p, filepath.Dir(p)+"/")
}

type question struct {
	Question    string `json:"question"`
	Header      string `json:"header"`
	MultiSelect bool   `json:"multiSelect"`
	Options     []struct {
		Label       string `json:"label"`
		Description string `json:"description"`
	} `json:"options"`
}

// ask walks the AskUserQuestion questions one card at a time.
func (a *app) ask(s *Session, in hookInput, tin map[string]any, answer chan<- json.RawMessage) string {
	var qs []question
	b, _ := json.Marshal(tin["questions"])
	json.Unmarshal(b, &qs)
	if len(qs) == 0 {
		answer <- nil
		return ""
	}
	answers := map[string]string{}
	deadline := a.ui.now().Add(AskTimeout - 5*time.Second)
	id := "ask:" + s.ID
	var show func(i int)
	show = func(i int) {
		q := qs[i]
		c := &Card{ID: id, Kind: "ask", Mood: moodWait, Title: q.Question, Meta: s.Project, Priority: 9,
			Multi: q.MultiSelect, Footer: "In terminal", Expires: deadline}
		if len(qs) > 1 {
			c.Meta = fmt.Sprintf("%s · %d/%d", s.Project, i+1, len(qs))
		}
		for _, o := range q.Options {
			if len(c.Options) == 4 {
				break
			}
			c.Options = append(c.Options, Option{o.Label, o.Description})
		}
		if q.MultiSelect {
			c.Buttons = []Button{{Key: "submit", Label: "Submit", Primary: true}}
		}
		c.OnPick = func(k string, picked []int) {
			if k == "footer" || k == "dismiss" || (k == "submit" && len(picked) == 0) {
				answer <- nil
				return
			}
			var labels []string
			for _, p := range picked {
				labels = append(labels, q.Options[p].Label)
			}
			answers[q.Question] = strings.Join(labels, ", ")
			if i+1 < len(qs) {
				show(i + 1)
				return
			}
			s.State = "work"
			b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
				"hookEventName": "PreToolUse", "permissionDecision": "allow",
				"updatedInput": map[string]any{"questions": tin["questions"], "answers": answers}}})
			answer <- b
		}
		c.OnExpire = func() { answer <- nil }
		a.ui.AddCard(c)
	}
	show(0)
	return id
}
