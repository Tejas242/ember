package island

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/config"
	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/source"
	"github.com/Tejas242/ember/internal/wayland"
	"github.com/Tejas242/ember/internal/xdg"
)

const (
	surfW, surfH = 580, 480 // room for a dashboard; the input region is only the shape
	roomH        = 380      // the most a card or the open island may take
	flickerEvery = time.Second / 12
	idleEvery    = time.Second / 6
	frameGrace   = 34 * time.Millisecond // a frame callback that never comes doesn't stall us
)

type app struct {
	lastHouse time.Time // the last housekeeping tick: a long gap means the machine slept
	ui        *UI
	win       *wayland.Window
	music     *music
	track     *Track
	posts     chan func(*app)
	cfg       config.Config
	cfgAt     time.Time // the config file's mtime when read
	desk      desktop
	started   time.Time
	quiet     bool                 // the current post changed nothing on screen
	ended     map[string]time.Time // sessions that ended: a late async hook mustn't bring one back

	allPlugins []Plugin
	plugins    map[string]Plugin // the running ones, by name
	detectedAt time.Time         // when dormant plugins last looked for their tools
}

func (a *app) status() ipc.Status {
	st := ipc.Status{Pid: os.Getpid(), Uptime: time.Since(a.started).Round(time.Second).String(), BarHeight: a.ui.BarH,
		Blur: a.win.Caps().Blur, CursorShape: a.win.Caps().CursorShape, Presence: a.win.Caps().Idle, Niri: a.desk.live, Music: a.music != nil,
		Volume: a.ui.vol.ok, Backlight: a.ui.light >= 0, Battery: a.ui.power != nil,
		Sessions: len(a.ui.sessions), Pending: len(a.ui.cards), Config: "defaults"}
	if a.track != nil {
		st.Track = a.track.Title
	}
	if !mtime(config.Path()).IsZero() {
		st.Config = config.Path()
	}
	for _, p := range a.allPlugins {
		switch q := a.plugins[p.Name()]; {
		case q == nil:
			st.Plugins = append(st.Plugins, p.Name()+": off (its tool isn't installed, or the config turns it off)")
		case p.Name() == "edge" && a.ui.edgeStats == nil:
			st.Plugins = append(st.Plugins, "edge: on, waiting for edge's stats (is its daemon running?)")
		default:
			st.Plugins = append(st.Plugins, p.Name()+": on")
		}
	}
	return st
}

// raise puts the island back above a bar that was just re-created.
func (a *app) raise(why string) {
	log.Println(why + ": raising the island above the bar")
	a.win.Raise()
	a.ui.Invalidate()
	go func() { // the bar may have changed too: take its new look
		b := readBar()
		a.posts <- func(a *app) {
			if b.h != a.ui.BarH && a.cfg.BarHeight == 0 {
				a.ui.BarH = b.h
				a.ui.Invalidate()
			}
		}
	}()
}

// reloadConfig applies ~/.config/ember/config.toml if it changed.
func (a *app) reloadConfig() bool {
	at := mtime(config.Path())
	if !a.cfgAt.IsZero() && at.Equal(a.cfgAt) {
		return false
	}
	a.cfgAt = at
	if at.IsZero() {
		a.cfgAt = time.Unix(1, 0) // no file: defaults, and don't look again until one appears
	}
	var warns []string
	a.cfg, warns = config.Load(config.Path())
	for _, w := range warns {
		log.Println(w)
	}
	a.ui.cfg = a.cfg
	a.ui.TimeFmt = a.cfg.TimeFormat()
	if h := float32(a.cfg.BarHeight); h > 0 && h != a.ui.BarH { // a bar that isn't Noctalia's says its height here
		a.ui.BarH = h
	}
	a.ui.pal = applyLook(a.cfg, loadPalette())
	if a.win != nil {
		a.win.WatchIdle(a.cfg.Away())
	}
	a.ui.casting = a.desk.casts > 0 && a.cfg.Privacy
	a.reconcile()
	return true
}

// barLook is what the island takes from Noctalia's bar: its thickness, so
// the pill sits on the bar's line and dips just below it.
type barLook struct{ h float32 }

// readBar reads [bar.default] from Noctalia's merged config.
func readBar() barLook {
	b := barLook{24}
	out, err := exec.Command("noctalia", "config", "export").Output()
	if err != nil {
		return b
	}
	return parseBar(string(out), b)
}

func parseBar(cfg string, b barLook) barLook {
	section := ""
	for _, ln := range strings.Split(cfg, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "[") {
			section = strings.Trim(ln, "[]")
			continue
		}
		if section != "bar.default" {
			continue
		}
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 32)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(k) {
		case "thickness":
			if f >= 16 && f <= 64 {
				b.h = float32(f)
			}
		}
	}
	return b
}

// Run opens the island and serves until the compositor goes away.
func Run() error {
	if err := wayland.WaitSocket(5 * time.Second); err != nil { // a session still starting gets a moment
		return err
	}
	posts := make(chan func(*app), 64)
	if _, err := listen(posts); err != nil {
		return err
	}
	win, err := wayland.Open(surfW, surfH, 0, wayland.LayerTop)
	if err != nil {
		return err
	}
	bar := readBar()
	a := &app{ui: NewUI(surfW, surfH, bar.h), win: win, posts: posts, started: time.Now()}
	a.ui.onKey = a.key
	a.ui.audio = &audio{}
	a.ui.habits = loadHabits()
	a.ui.bond = loadBond(a.ui.now())
	if t := a.ui.bond.Active; !t.IsZero() && a.ui.now().Sub(t) > 4*time.Hour { // the machine was off: this is a return
		a.ui.returned(t, a.ui.now())
	}
	a.ui.day = loadDay(time.Now())
	win.OnIdle = func(idle bool) { a.presence(idle); a.ui.Invalidate() }
	a.reloadConfig()
	a.loadSessions()
	a.music = startMusic(posts)
	startSystem(emitter(posts))
	go source.Niri(emitter(posts))
	frameDue := false
	win.OnPointer = func(e wayland.PointerEvent) {
		redraw, hand := a.ui.pointer(fromWayland(e))
		win.SetCursor(hand)
		if redraw {
			a.ui.Invalidate()
		}
	}
	win.OnFrame = func() { frameDue = true }
	log.Printf("ember up (bar %.0fpx)", a.ui.BarH)

	animating, waitingFrame, tick := false, false, false
	var waitingSince time.Time
	debug := os.Getenv("EMBER_DEBUG") != ""
	var nFull, nFast, nFail, nPost, nWake int
	var lost image.Rectangle // damage of frames the compositor had no buffer for: the next frame carries it
	statAt := time.Now()
	draw := func() {
		var damage image.Rectangle
		animating, damage = a.ui.renderDamage()
		if a.ui.lastFull {
			nFull++
		} else {
			nFast++
		}
		damage = damage.Union(lost)
		if win.Present(a.ui.cv.img, a.ui.shape.bounds().Union(a.ui.bubble), damage, animating) {
			waitingFrame, waitingSince = animating, time.Now()
			lost = image.Rectangle{}
		} else {
			nFail++
			lost = damage     // what changed never reached the screen: keep it for the next frame
			a.ui.dirty = true // no free buffer: try again on the next wake
		}
		win.SetBlur(a.ui.shape.blurRects())
	}
	draw()
	housekeep := time.NewTicker(10 * time.Second)
	defer housekeep.Stop()
	timer := time.NewTimer(time.Hour)
	lastPos := time.Time{}
	saveDue := false
	for {
		select {
		case m := <-win.Messages():
			win.Dispatch(m)
		case err := <-win.Errors():
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
			if gap := a.ui.now().Sub(a.lastHouse); !a.lastHouse.IsZero() && gap > 4*time.Hour { // it slept (a lid closed): you're back
				a.ui.returned(a.lastHouse, a.ui.now())
			}
			a.lastHouse = a.ui.now()
			if !a.ui.mind.away {
				a.ui.bond.present(a.ui.now())
			}
			a.ui.habits.save()
			a.ui.bond.save()
			a.ui.day.save()
			if p := applyLook(a.cfg, loadPalette()); p != a.ui.pal { // the wallpaper palette changed
				a.ui.pal = p
				a.ui.Invalidate()
			}
			if a.reloadConfig() {
				a.ui.Invalidate()
			}
			if a.ui.now().Sub(a.detectedAt) > redetectEvery { // a tool installed or removed meanwhile
				a.reconcile()
			}
		case <-timer.C:
			tick = true
			if a.ui.expire() {
				a.ui.Invalidate()
			}
		}
		if win.Closed() {
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
		win.Flush()
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
		if u.lastView != vCompact || u.eqShown || u.dancing || (u.body.phones.seated && u.danceable()) { // a dance needs every beat
			next = now.Add(time.Second / 24) // the card's pulses, a listening equaliser: only their squares repaint
		}
		if u.shimmering() { // a band of light crosses smoothly, then rests
			if t := now.Sub(u.start).Seconds(); shimmerPassing(t) {
				next = now.Add(time.Second / 40)
			} else {
				rest := time.Duration((shimmerEvery - math.Mod(t, shimmerEvery)) * float64(time.Second))
				next = minTime(next, now.Add(rest))
			}
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
	if !u.hover && now.Sub(u.leftAt) < 300*time.Millisecond {
		next = minTime(next, u.leftAt.Add(210*time.Millisecond))
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
// that went silent: a Claude that was killed never says Stop, so after
// [claude] settle_min (30) without a word "working" becomes idle (a long
// build still shows its minutes in the pill until then).
func (a *app) sweep() bool {
	changed, now := false, a.ui.now()
	for id, s := range a.ui.sessions {
		quiet := now.Sub(s.Seen)
		switch {
		case !slices.ContainsFunc(s.Pids, ipc.Alive) || (s.State == "idle" && quiet > 3*time.Hour):
			a.forget(id)
			changed = true
		case s.State != "idle" && quiet > a.cfg.Settle && !a.hasCard(id):
			s.State, s.Label = "idle", ""
			changed = true
		}
		if s.sweepAgents(now, a.cfg.AgentQuiet) {
			changed = true
		}
	}
	return changed
}

// forget drops a session and what the island knew about it.
func (a *app) forget(id string) {
	delete(a.ui.sessions, id)
	delete(a.ui.watching, id)
}

// end forgets a session that's over, and keeps it over: hooks are async, so
// one can still arrive after its SessionEnd.
func (a *app) end(id string, now time.Time) {
	a.forget(id)
	a.ended = remember(a.ended, id, now)
}

// sameProcess: two hooks' parent chains come from one Claude process. A
// chain starts at the hook's parent, which is Claude itself or a shell it ran
// the hook in; that shell has exited by now. So the older chain, less its
// dead head, is the newer one, give or take the newer one's shell. Sessions
// in the same terminal share only the chain's tail, and a Claude started
// from inside another adds more than a shell.
func sameProcess(old, cur []int) bool {
	i := 0
	for i < len(old) && !ipc.Alive(old[i]) {
		i++
	}
	old = old[i:]
	d := len(cur) - len(old)
	return len(old) > 0 && (d == 0 || d == 1) && slices.Equal(cur[d:], old)
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
	if a.ui.audio != nil { // the equaliser listens only while a song plays
		a.ui.audio.set(t != nil && t.Playing && a.cfg.Music)
	}
	if t != nil && !t.Playing { // when it stopped: a song paused long ago leaves the card
		t.PausedAt = a.ui.now()
		if prev != nil && !prev.Playing && prev.Title == t.Title && !prev.PausedAt.IsZero() {
			t.PausedAt = prev.PausedAt
		}
	}
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

// ---- Claude Code hooks ----

func (a *app) hook(m ipc.Msg, answer chan<- json.RawMessage) string {
	var in hookInput
	json.Unmarshal(m.Payload, &in)
	now := a.ui.now()
	s := a.ui.sessions[in.SessionID]
	if m.Event == "SessionStart" {
		delete(a.ended, in.SessionID) // resumed: it's back
	}
	_, over := a.ended[in.SessionID]
	if s == nil && in.SessionID != "" && m.Event != "SessionEnd" && !over {
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
		if m.Event == "SessionEnd" && in.SessionID != "" {
			a.end(in.SessionID, now)
		}
		if answer != nil {
			answer <- nil
		}
		return ""
	}
	var tin map[string]any
	json.Unmarshal(in.ToolInput, &tin)
	if in.AgentID != "" && a.agentHook(s, m.Event, in, tin, now) {
		return ""
	}

	switch m.Event {
	case "SessionEnd":
		a.end(in.SessionID, now)
	case "SessionStart":
		if in.Source == "startup" { // a compaction or a resume keeps its background agents
			s.Agents, s.asks = nil, nil
		}
		if in.Source == "clear" || in.Source == "resume" { // a new session in the same Claude: the old one is over
			for id, o := range a.ui.sessions {
				if o != s && sameProcess(o.Pids, m.Pids) {
					a.end(id, now)
				}
			}
		}
	case "UserPromptSubmit":
		s.State, s.Label, s.Plus, s.Minus, s.Since, s.StepAt = "work", "Thinking", 0, 0, now, now
		s.Turn, s.Finished, s.Recap, s.Unseen, s.Asking = Turn{}, nil, nil, time.Time{}, ""
		s.Prompt = firstLine(strings.TrimSpace(in.Prompt))
		a.ui.mind.kick(now, 0.25, 0.05)
		a.ui.play("ignite", 0) // it catches
		a.ui.mind.novel["claude"] = now
	case "PreCompact":
		s.State, s.Label, s.Plus, s.Minus = "work", "Compacting the conversation", 0, 0
	case "PostCompact":
		if s.Label == "Compacting the conversation" {
			s.Label = "Thinking"
			a.ui.play("pop", 0) // squeezed, and now it springs back
		}
	case "StopFailure":
		s.State, s.Label = "idle", ""
		msg := friendlyError(in.Error)
		took := now.Sub(s.Since)
		if s.Since.IsZero() {
			took = 0
		}
		s.Turn.finish(false)
		s.Recap = &Recap{Failed: true, Error: msg, At: now, Took: took, Steps: s.Turn.Count,
			Files: len(s.Turn.Files), Plus: s.Turn.Plus, Minus: s.Turn.Minus, Failures: s.Turn.Failures}
		if !a.looking(s) {
			s.Unseen = now
		}
		a.ui.mind.kick(now, 0.35, -0.6) // a start, and a low mood that takes a while to lift
		if a.ui.mind.away {
			a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"fail", a.ui.name(s), s.ID})
			break
		}
		a.ui.Moment(moment{key: "fail:" + s.ID, act: "claude", mood: moodError, verb: "Stopped", text: a.ui.short(s),
			sub: msg, until: now.Add(8 * time.Second), focus: s.ID})
	case "AskUserQuestion":
		if answer != nil {
			s.State = "wait"
			a.ui.play("needs", 0)
			return a.ask(s, in, tin, answer)
		}
	case "PreToolUse":
		if in.Tool == "AskUserQuestion" {
			break
		}
		s.State = "work"
		if in.Tool == "Agent" || in.Tool == "Task" {
			s.asked(tin)
		}
		label := s.Label
		s.Label, s.Plus, s.Minus = activity(in.Tool, tin)
		if s.Label != label || s.StepAt.IsZero() {
			s.StepAt = now
		}
		s.Turn.step(now, in.Tool, tin)
		a.ui.day.call(now)
		a.ui.mind.toolCall(now)
		if ss := a.ui.sessionList(); len(ss) > 0 && ss[0] == s && now.Sub(a.ui.gulpAt) > 1200*time.Millisecond {
			a.ui.gulpAt = now
			a.ui.body.gulp(0.5) // a bite of fuel, for the session it's watching
		}
	case "PostToolUse":
		if s.State == "wait" {
			s.State = "work"
		}
		s.Asking = ""
		if s.Turn.streak >= 3 { // it was failing, and now it isn't: relief
			a.ui.play("relief", 0)
			a.ui.mind.kick(now, 0, 0.25)
		}
		s.Turn.finish(false)
	case "PostToolUseFailure":
		s.Turn.finish(true)
		a.ui.React("wince")
		a.ui.mind.kick(now, 0.1, -0.12) // a small wince
	case "PermissionRequest":
		if answer == nil {
			return ""
		}
		s.State = "wait"
		a.ui.play("needs", 0)
		return a.permission(s, in, tin, answer)
	case "Notification":
		// a prompt in the terminal (the island timed out, or a dialog it
		// can't show). idle_prompt is just "Claude is done": not a request.
		if in.NotifyType == "permission_prompt" || in.NotifyType == "elicitation_dialog" {
			if s.State != "wait" {
				a.ui.play("needs", 0)
			}
			s.State = "wait"
			if s.Asking == "" {
				s.Asking = in.Message
			}
		}
	case "Stop":
		s.State, s.Label = "idle", ""
		a.ui.lastDone.project, a.ui.lastDone.at = s.Project, now
		s.Turn.finish(false)
		took := now.Sub(s.Since)
		if s.Since.IsZero() {
			took = 0
		}
		s.Recap = &Recap{Summary: summarize(in.LastMessage), At: now, Took: took, Steps: s.Turn.Count,
			Files: len(s.Turn.Files), Plus: s.Turn.Plus, Minus: s.Turn.Minus, Failures: s.Turn.Failures}
		s.cheer = cheer{size: doneSize(took), roll: absolutelyRight.MatchString(in.LastMessage)}
		if b := a.ui.bond; b != nil {
			long := b.turn(took)
			hard := s.Turn.Failures >= 2 || s.Turn.Count >= 40 // failures overcome, or a lot of work
			s.cheer.flare = (long || hard) && s.Turn.streak == 0 && !s.Recap.troubled()
		}
		a.ui.day.done(now, s.Project, took)
		s.Asking = ""
		if !a.looking(s) {
			s.Unseen = now // it stays in the pill, quietly, until you've seen it
		}
		a.ui.mind.kick(now, 0.2, 0.35) // pleased
		if a.ui.mind.away {            // tell them when they're back
			a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"done", a.ui.name(s), s.ID})
			break
		}
		if !a.cfg.Done || a.looking(s) || s.running() > 0 {
			break // they're looking at it, or it isn't done: its agents work on
		}
		a.done(s, summarize(in.LastMessage), now)
	}
	return ""
}

// done shows "Done · project". If another session's note is still up, the
// two merge into one ("Done · edge, ember") rather than one hiding the other.
func (a *app) done(s *Session, summary string, now time.Time) {
	name := a.ui.short(s)
	m := moment{key: "done:" + s.ID, act: "claude", mood: moodHappy, verb: "Done", text: name,
		sub: summary, until: now.Add(a.cfg.DoneFor), focus: s.ID, many: []string{name}, cheer: s.cheer}
	s.cheer = cheer{}
	if cur := a.ui.moment; cur != nil && strings.HasPrefix(cur.key, "done") && now.Before(cur.until) {
		names := cur.many
		if !slices.Contains(names, name) { // one that finishes again is named once
			names = append(slices.Clip(names), name)
		}
		if len(names) > 1 {
			m.key, m.many = "done:many", names
			m.text = strings.Join(names, ", ")
			m.sub = fmt.Sprintf("%d sessions finished", len(names))
			m.until = now.Add(a.cfg.DoneFor + time.Second)
		}
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
	a.ui.returned(m.awaySince, now)
	if now.Sub(m.awaySince) > 30*time.Second {
		a.welcomeBack(now)
	}
}

func (a *app) permission(s *Session, in hookInput, tin map[string]any, answer chan<- json.RawMessage) string {
	a.ui.mind.kick(a.ui.now(), 0.4, -0.05)
	if a.ui.mind.away {
		a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"wait", a.ui.name(s), s.ID})
	}
	str := func(k string) string { v, _ := tin[k].(string); return v }
	if !a.cfg.Answer { // [claude] answer = false: the terminal asks
		answer <- nil
		return ""
	}
	c := &Card{ID: "perm:" + s.ID + ":" + fmt.Sprint(time.Now().UnixNano()), Kind: "perm", Mood: moodWait,
		Eyebrow: a.ui.short(s) + " wants to " + wants(in.Tool), Priority: 10, Footer: "Answer in terminal", Expires: a.ui.now().Add(a.cfg.Wait)}
	switch in.Tool {
	case "Bash": // Claude says what it wants in words: that's the title; the command is the detail
		c.Title, c.Code, c.Shell = str("description"), str("command"), true
		if c.Title == "" {
			c.Title = "Run a command"
		}
	case "Edit", "MultiEdit", "Write":
		verb := map[string]string{"Write": "Create"}[in.Tool]
		if verb == "" {
			verb = "Edit"
		}
		_, p, m := activity(in.Tool, tin)
		c.Title = verb + " " + filepath.Base(str("file_path"))
		c.Body = fmt.Sprintf("%s · %s", where(str("file_path")), diffText(max(p, 0), m))
		if p == 0 && m == 0 {
			c.Body = where(str("file_path"))
		}
	case "WebFetch":
		u, _ := url.Parse(str("url"))
		c.Title, c.Code = "Fetch "+u.Host, str("url")
	default:
		label, _, _ := activity(in.Tool, tin)
		c.Title = label
		if b, err := json.Marshal(tin); err == nil && len(b) > 2 {
			c.Code = string(b)
		}
	}
	c.Buttons = []Button{{Key: "deny", Label: "Deny", Danger: true}}
	if len(in.Suggestions) > 2 && string(in.Suggestions) != "null" {
		c.Buttons = append(c.Buttons, Button{Key: "always", Label: "Always"})
		c.Note = alwaysNote(in.Suggestions)
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
			s.Asking = asking(c)
			answer <- nil
		}
	}
	c.OnExpire = func() { s.Asking = asking(c); answer <- nil }
	a.ui.AddCard(c)
	return c.ID
}

// asking is what a request handed to the terminal was about, for the card.
func asking(c *Card) string {
	if c.Shell && c.Code != "" {
		return "Wants to run " + firstLine(c.Code)
	}
	return c.Title
}

// alwaysNote says what "Always" would allow from now on, from the rules
// Claude suggests: "Always allows go test:* in this project".
func alwaysNote(raw json.RawMessage) string {
	var sugg []struct {
		Type  string `json:"type"`
		Rules []struct {
			Tool    string `json:"toolName"`
			Content string `json:"ruleContent"`
		} `json:"rules"`
		Destination string `json:"destination"`
	}
	if json.Unmarshal(raw, &sugg) != nil {
		return ""
	}
	var what []string
	where := ""
	for _, sg := range sugg {
		for _, r := range sg.Rules {
			switch {
			case r.Content != "" && r.Tool == "Bash":
				what = append(what, r.Content)
			case r.Content != "":
				what = append(what, r.Tool+" "+r.Content)
			default:
				what = append(what, "every "+r.Tool+" call")
			}
		}
		switch sg.Destination {
		case "userSettings":
			where = " everywhere"
		case "projectSettings", "localSettings":
			where = " in this project"
		case "session":
			where = " this session"
		}
	}
	if len(what) == 0 {
		if len(sugg) > 0 && sugg[0].Type == "setMode" {
			return "Always switches to accepting edits"
		}
		return ""
	}
	return "Always allows " + strings.Join(what, ", ") + where
}

// friendlyError turns StopFailure's code into words, the same everywhere.
func friendlyError(code string) string {
	switch c := strings.ToLower(code); {
	case strings.Contains(c, "rate"):
		return "Rate limited"
	case strings.Contains(c, "overload"), strings.Contains(c, "server"), strings.Contains(c, "api"):
		return "API error, try again"
	case strings.Contains(c, "auth"):
		return "Signed out, run /login"
	case strings.Contains(c, "billing"), strings.Contains(c, "credit"):
		return "Billing problem"
	case strings.Contains(c, "max_output"), strings.Contains(c, "token"):
		return "Reply hit the length limit"
	case c == "":
		return "Stopped unexpectedly"
	}
	return strings.ToUpper(code[:1]) + strings.ReplaceAll(code[1:], "_", " ")
}

// wants is what a tool asks to do, for a request's eyebrow.
func wants(tool string) string {
	switch tool {
	case "Bash":
		return "run"
	case "Edit", "MultiEdit", "NotebookEdit":
		return "edit"
	case "Write":
		return "create"
	case "WebFetch":
		return "fetch"
	case "WebSearch":
		return "search"
	}
	if strings.HasPrefix(tool, "mcp__") { // mcp__server__tool: the tool's own name
		parts := strings.Split(tool, "__")
		tool = strings.ReplaceAll(parts[len(parts)-1], "_", " ")
	}
	return "use " + tool
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
		a.ui.mind.missed = append(a.ui.mind.missed, missedNote{"wait", a.ui.name(s), s.ID})
	}
	var qs []question
	b, _ := json.Marshal(tin["questions"])
	json.Unmarshal(b, &qs)
	if len(qs) == 0 || !a.cfg.Answer {
		answer <- nil
		return ""
	}
	answers := map[string]string{}
	deadline := a.ui.now().Add(a.cfg.Wait)
	id := "ask:" + s.ID
	var show func(i int)
	show = func(i int) {
		q := qs[i]
		c := &Card{ID: id, Kind: "ask", Mood: moodWait, Title: q.Question, Eyebrow: a.ui.short(s) + " asks", Priority: 9,
			Multi: q.MultiSelect, Footer: "Answer in terminal", Expires: deadline}
		if len(qs) > 1 {
			c.Meta = fmt.Sprintf("%d of %d", i+1, len(qs))
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
				s.Asking = "Asks: " + q.Question
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
		c.OnExpire = func() { s.Asking = "Asks: " + q.Question; answer <- nil }
		a.ui.AddCard(c)
	}
	show(0)
	return id
}

// ---- sessions survive a restart of the island ----

func sessionsPath() string { return filepath.Join(filepath.Dir(ipc.SockPath()), "ember-sessions.json") }

func (a *app) saveSessions() {
	b, err := json.Marshal(a.ui.sessions)
	if err == nil {
		xdg.AtomicWrite(sessionsPath(), b, 0o600)
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
		if slices.ContainsFunc(s.Pids, ipc.Alive) {
			a.ui.sessions[id] = s
		}
	}
}
