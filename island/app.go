package island

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"net/url"
	"os"
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
	ui      *UI
	win     *window
	music   *music
	track   *Track
	posts   chan func(*app)
	cfg     Config
	cfgAt   time.Time // the config file's mtime when read
	desk    desktop
	started time.Time
	quiet   bool // the current post changed nothing on screen
}

// Status is what `ember doctor` reports about the running island.
type Status struct {
	Pid         int     `json:"pid"`
	Uptime      string  `json:"uptime"`
	BarHeight   float32 `json:"barHeight"`
	Blur        bool    `json:"blur"`
	CursorShape bool    `json:"cursorShape"`
	Presence    bool    `json:"presence"`
	Niri        bool    `json:"niri"`
	Music       bool    `json:"music"`
	Track       string  `json:"track,omitempty"`
	Volume      bool    `json:"volume"`
	Backlight   bool    `json:"backlight"`
	Battery     bool    `json:"battery"`
	Sessions    int     `json:"sessions"`
	Pending     int     `json:"pending"`
	Config      string  `json:"config"`
}

func (a *app) status() Status {
	st := Status{Pid: os.Getpid(), Uptime: time.Since(a.started).Round(time.Second).String(), BarHeight: a.ui.BarH,
		Blur: a.win.blurMgr != 0, CursorShape: a.win.cursorMgr != 0, Presence: a.win.idleMgr != 0, Niri: a.desk.live, Music: a.music != nil,
		Volume: a.ui.vol.ok, Backlight: a.ui.light >= 0, Battery: a.ui.power != nil,
		Sessions: len(a.ui.sessions), Pending: len(a.ui.cards), Config: "defaults"}
	if a.track != nil {
		st.Track = a.track.Title
	}
	if !mtime(ConfigPath()).IsZero() {
		st.Config = ConfigPath()
	}
	return st
}

// reloadConfig applies ~/.config/ember/config.toml if it changed.
func (a *app) reloadConfig() bool {
	at := mtime(ConfigPath())
	if !a.cfgAt.IsZero() && at.Equal(a.cfgAt) {
		return false
	}
	a.cfgAt = at
	if at.IsZero() {
		a.cfgAt = time.Unix(1, 0) // no file: defaults, and don't look again until one appears
	}
	a.cfg = loadConfig(ConfigPath())
	a.ui.cfg = a.cfg
	a.ui.TimeFmt = a.cfg.timeFormat()
	a.ui.casting = a.desk.casts > 0 && a.cfg.Privacy
	return true
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
	a := &app{ui: NewUI(surfW, surfH, barHeight()), win: win, posts: posts, started: time.Now()}
	a.ui.onKey = a.key
	a.ui.habits = loadHabits()
	win.onIdle = func(idle bool) { a.presence(idle); a.ui.Invalidate() }
	a.reloadConfig()
	a.loadSessions()
	a.music = startMusic(posts)
	startSystem(posts)
	go watchNiri(posts)
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
	debug := os.Getenv("EMBER_DEBUG") != ""
	var nFull, nFast, nFail, nPost, nWake int
	statAt := time.Now()
	draw := func() {
		var damage image.Rectangle
		wasDirty := a.ui.dirty
		animating, damage = a.ui.renderDamage()
		if wasDirty || damage.Dx() > 60 {
			nFull++
		} else {
			nFast++
		}
		if win.present(a.ui.cv.img, a.ui.shape.bounds().Union(a.ui.bubble), damage, animating) {
			waitingFrame, waitingSince = animating, time.Now()
		} else {
			nFail++
			a.ui.dirty = true // no free buffer: try again on the next wake
		}
		if b := a.ui.shape.blurRects(); !slices.Equal(b, win.lastBlur) {
			win.setBlur(b)
		}
	}
	draw()
	housekeep := time.NewTicker(10 * time.Second)
	defer housekeep.Stop()
	timer := time.NewTimer(time.Hour)
	lastPos := time.Time{}
	saveDue := false
	for {
		select {
		case m := <-win.c.msgs:
			win.c.dispatch(m)
		case err := <-win.c.readErr:
			return err
		case f := <-posts:
			a.quiet = false
			f(a)
			if !a.quiet { // a post that changed nothing visible says so
				a.ui.Invalidate()
			}
			nPost++
			saveDue = true
		case <-housekeep.C:
			if a.sweep() {
				a.ui.Invalidate()
			}
			if saveDue {
				a.saveSessions()
				saveDue = false
			}
			a.ui.habits.save()
			if p := loadPalette(); p != a.ui.pal { // the wallpaper palette changed
				a.ui.pal = p
				a.ui.Invalidate()
			}
			if a.reloadConfig() {
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
		nWake++
		if debug && time.Since(statAt) > 5*time.Second {
			log.Printf("5s: wakes %d, full %d, fast %d, failed %d, posts %d, view %v, cont %v", nWake, nFull, nFast, nFail, nPost, a.ui.lastView, a.ui.continuous())
			nFull, nFast, nFail, nPost, nWake, statAt = 0, 0, 0, 0, 0, time.Now()
		}
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
		if u.shimmering() {
			next = now.Add(time.Second / 20) // the label's light moves smoothly
		}
	} else if u.mind.asleep {
		next = now.Add(time.Second / 4) // slow breaths, drifting z's
	} else if u.glancing() {
		next = now.Add(time.Second / 30) // eyes move smoothly; nothing else needs the rate
	} else {
		next = minTime(next, now.Add(idleEvery)) // Ember breathes even when nothing happens
	}

	next = minTime(next, now.Truncate(time.Minute).Add(time.Minute))
	for _, c := range u.cards {
		if !c.Expires.IsZero() {
			next = minTime(next, c.Expires)
		}
	}
	if u.hover && now.Sub(u.hoverAt) < u.cfg.HoverDelay {
		next = minTime(next, u.hoverAt.Add(u.cfg.HoverDelay))
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

// sweep forgets sessions whose Claude process is gone, and settles ones
// that went silent: a Claude that was killed never says Stop, so after 30
// minutes without a word "working" becomes idle (a long build still shows
// its minutes in the pill until then).
func (a *app) sweep() bool {
	changed, now := false, a.ui.now()
	for id, s := range a.ui.sessions {
		quiet := now.Sub(s.Seen)
		switch {
		case !slices.ContainsFunc(s.Pids, alive) || (s.State == "idle" && quiet > 3*time.Hour):
			delete(a.ui.sessions, id)
			changed = true
		case s.State != "idle" && quiet > 30*time.Minute && !a.hasCard(id):
			s.State, s.Label = "idle", ""
			changed = true
		}
	}
	return changed
}

// hasCard: a request from this session is still on the island.
func (a *app) hasCard(session string) bool {
	for _, c := range a.ui.cards {
		if strings.Contains(c.ID, ":"+session) {
			return true
		}
	}
	return false
}

func (a *app) setTrack(t *Track) {
	prev := a.track
	a.track, a.ui.track = t, t
	if t != nil && t.Playing && (prev == nil || prev.Title != t.Title) {
		a.ui.mind.novel["music"] = a.ui.now()
		a.ui.mind.kick(a.ui.now(), 0.1, 0.1)
	}
	if t != nil && t.Playing && a.cfg.Music && (prev == nil || prev.Title != t.Title) && a.ui.primary() != "music" && a.ui.primary() != "moment" {
		a.ui.Moment(moment{key: "track:" + t.Title, act: "music", mood: moodIdle, text: t.Title, sub: t.Artist,
			until: a.ui.now().Add(4 * time.Second)})
	}
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
			a.focusSession(s.Pids)
		}
	case k == "sys:mute":
		go toggleMute()
	case strings.HasPrefix(k, "sys:vol:nudge:"), strings.HasPrefix(k, "sys:light:nudge:"):
		dir, _ := strconv.Atoi(k[strings.LastIndexByte(k, ':')+1:])
		if strings.HasPrefix(k, "sys:vol") {
			go nudgeVolume(dir)
		} else {
			go nudgeBrightness(dir)
		}
	case strings.HasPrefix(k, "sys:vol:"):
		f, _ := strconv.ParseFloat(strings.TrimPrefix(k, "sys:vol:"), 64)
		go setVolume(f)
	case strings.HasPrefix(k, "sys:light:"):
		f, _ := strconv.ParseFloat(strings.TrimPrefix(k, "sys:light:"), 64)
		go setBrightness(f)
	case strings.HasPrefix(k, "open:"):
		go exec.Command("xdg-open", strings.TrimPrefix(k, "open:")).Run()
	case strings.HasPrefix(k, "music:seek:") && a.track != nil:
		d, _ := strconv.ParseInt(strings.TrimPrefix(k, "music:seek:"), 10, 64)
		a.music.seek(a.track.Player, time.Duration(d))
	case strings.HasPrefix(k, "music:volume:") && a.track != nil:
		d, _ := strconv.Atoi(strings.TrimPrefix(k, "music:volume:"))
		a.music.nudgeVolume(a.track.Player, float64(d)*0.05)
	case strings.HasPrefix(k, "music:") && a.track != nil:
		if method := map[string]string{"music:prev": "Previous", "music:toggle": "PlayPause", "music:next": "Next"}[k]; method != "" {
			a.music.control(a.track.Player, method)
			if method == "PlayPause" { // feel instant; the player confirms a moment later
				a.track.Playing = !a.track.Playing
				a.track.PosAt = a.ui.now()
			}
		}
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
	switch cmd {
	case "toggle":
		a.ui.pinned, a.ui.suppress = !a.ui.pinned, false
		return
	case "open": // open the card, on a page if one is named
		a.ui.pinned, a.ui.suppress = true, false
		if arg != "" {
			a.ui.page = arg
		}
		return
	case "close":
		a.ui.pinned, a.ui.suppress = false, a.ui.hover // only while the pointer is on it
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
	case "SessionStart":
		s.Agents = 0
	case "UserPromptSubmit":
		s.State, s.Label, s.Plus, s.Minus, s.Since, s.StepAt = "work", "Thinking", 0, 0, now, now
		a.ui.mind.kick(now, 0.25, 0.05)
		a.ui.mind.novel["claude"] = now
	case "SubagentStart":
		s.Agents++
	case "SubagentStop":
		s.Agents = max(s.Agents-1, 0)
	case "PreCompact":
		s.State, s.Label, s.Plus, s.Minus = "work", "Compacting the conversation", 0, 0
	case "PostCompact":
		if s.Label == "Compacting the conversation" {
			s.Label = "Thinking"
		}
	case "StopFailure":
		s.State, s.Label = "idle", ""
		msg := in.Error
		if msg == "" {
			msg = "unknown error"
		}
		a.ui.mind.kick(now, 0.35, -0.6) // a start, and a low mood that takes a while to lift
		if a.ui.mind.away {
			a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"fail", s.Project, s.ID})
			break
		}
		a.ui.Moment(moment{key: "fail:" + s.ID, act: "claude", mood: moodError, text: "Stopped · " + s.Project,
			sub: strings.ReplaceAll(msg, "_", " "), until: now.Add(8 * time.Second), focus: s.ID})
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
		label := s.Label
		s.Label, s.Plus, s.Minus = activity(in.Tool, tin)
		if s.Label != label || s.StepAt.IsZero() {
			s.StepAt = now
		}
		s.bump(now)
		a.ui.mind.toolCall(now)
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
		// a prompt in the terminal (the island timed out, or a dialog it
		// can't show). idle_prompt is just "Claude is done": not a request.
		if in.NotifyType == "permission_prompt" || in.NotifyType == "elicitation_dialog" {
			s.State = "wait"
		}
	case "Stop":
		s.State, s.Label = "idle", ""
		a.ui.mind.kick(now, 0.2, 0.35) // pleased
		if a.ui.mind.away {            // tell them when they're back
			a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"done", s.Project, s.ID})
			break
		}
		if !a.cfg.Done || slices.Contains(s.Pids, a.focusedPid()) {
			break // they're looking at it
		}
		a.done(s, summarize(in.LastMessage), now)
	}
	return ""
}

// done shows "Done · project". If another session's note is still up, the
// two merge into one ("Done · edge, ember") rather than one hiding the other.
func (a *app) done(s *Session, summary string, now time.Time) {
	m := moment{key: "done:" + s.ID, act: "claude", mood: moodHappy, text: "Done · " + s.Project,
		sub: summary, until: now.Add(6 * time.Second), focus: s.ID}
	if cur := a.ui.moment; cur != nil && strings.HasPrefix(cur.key, "done") && cur.focus != s.ID && now.Before(cur.until) {
		names := strings.TrimPrefix(cur.text, "Done · ")
		n := strings.Count(names, ",") + 2
		m.key = "done:many"
		m.text = "Done · " + names + ", " + s.Project
		m.sub = fmt.Sprintf("%d sessions finished", n)
		m.until = now.Add(7 * time.Second)
	}
	a.ui.Moment(m)
}

// welcomeBack: you were away; here's what happened, in one line.
func (a *app) welcomeBack(now time.Time) {
	ms := a.ui.mind.missed
	a.ui.mind.missed = nil
	if len(ms) == 0 {
		return
	}
	var parts []string
	seen := map[string]bool{}
	md, focus := moodHappy, ms[len(ms)-1].session
	for _, n := range ms {
		k := n.kind + n.project
		if seen[k] {
			continue
		}
		seen[k] = true
		switch n.kind {
		case "done":
			parts = append(parts, n.project+" done")
		case "fail":
			parts, md = append(parts, n.project+" stopped"), moodError
		case "wait":
			parts, md, focus = append(parts, n.project+" asked you"), moodWait, n.session
		}
	}
	a.ui.Moment(moment{key: "away", act: "claude", mood: md, text: "While you were away",
		sub: strings.Join(parts, " · "), until: now.Add(10 * time.Second), focus: focus})
}

// presence: the compositor says you've stopped (or started) using the machine.
func (a *app) presence(idle bool) {
	m, now := a.ui.mind, a.ui.now()
	log.Printf("presence: away=%v (%d note(s) waiting)", idle, len(m.missed))
	if idle {
		m.away, m.awaySince = true, now
		return
	}
	m.away = false
	m.kick(now, 0.45, 0.1) // you're back: Ember perks up
	if now.Sub(m.awaySince) > 30*time.Second {
		a.ui.Bounce()
		a.welcomeBack(now)
	}
}

func (a *app) permission(s *Session, in hookInput, tin map[string]any, answer chan<- json.RawMessage) string {
	a.ui.mind.kick(a.ui.now(), 0.4, -0.05)
	if a.ui.mind.away {
		a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"wait", s.Project, s.ID})
	}
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
		c.Body = fmt.Sprintf("%s  ·  %s", where(str("file_path")), diffText(max(p, 0), m))
		if p == 0 && m == 0 {
			c.Body = where(str("file_path"))
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
			decide(map[string]any{"behavior": "deny", "message": "Denied from Ember."})
		default: // In terminal: no decision, Claude asks there
			answer <- nil
		}
	}
	c.OnExpire = func() { answer <- nil }
	a.ui.AddCard(c)
	return c.ID
}

// where is a file's folder, short: "~/Gems/edge/store".
func where(p string) string {
	if p == "" {
		return ""
	}
	d := filepath.Dir(p)
	if h, _ := os.UserHomeDir(); h != "" && strings.HasPrefix(d, h) {
		d = "~" + strings.TrimPrefix(d, h)
	}
	return d
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
	a.ui.mind.kick(a.ui.now(), 0.4, 0)
	if a.ui.mind.away {
		a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"wait", s.Project, s.ID})
	}
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

// ---- sessions survive a restart of the island ----

func sessionsPath() string { return filepath.Join(filepath.Dir(SockPath()), "ember-sessions.json") }

func (a *app) saveSessions() {
	b, err := json.Marshal(a.ui.sessions)
	if err == nil {
		os.WriteFile(sessionsPath(), b, 0o600)
	}
}

func (a *app) loadSessions() {
	b, err := os.ReadFile(sessionsPath())
	if err != nil {
		return
	}
	var ss map[string]*Session
	if json.Unmarshal(b, &ss) != nil {
		return
	}
	for id, s := range ss {
		if slices.ContainsFunc(s.Pids, alive) {
			a.ui.sessions[id] = s
		}
	}
}
