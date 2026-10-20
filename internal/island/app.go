package island

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tejas242/ember/internal/config"
	"github.com/Tejas242/ember/internal/ipc"
	"github.com/Tejas242/ember/internal/plugin"
	"github.com/Tejas242/ember/internal/source"
	"github.com/Tejas242/ember/internal/wayland"
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
	music     *source.MPRIS
	sup       *source.Supervisor   // runs the sources; knows how each is doing
	host      *plugin.Host         // runs the external plugins
	stopAll   func()               // ends the sources and the plugins
	lastDash  string               // the dashboard open last time we looked: plugins hear of a change
	cliLast   map[string]time.Time // when each one-liner last changed what shows
	version   string
	track     *Track
	posts     chan func(*app)
	cfg       config.Config
	cfgAt     time.Time // the config file's mtime when read
	desk      desktop
	started   time.Time
	quiet     bool                 // the current post changed nothing on screen
	ended     map[string]time.Time // sessions that ended: a late async hook mustn't bring one back

	detectedAt time.Time // when dormant plugins last looked for their tools
}

func (a *app) status() ipc.Status {
	st := ipc.Status{Version: a.version, Pid: os.Getpid(), Uptime: time.Since(a.started).Round(time.Second).String(), BarHeight: a.ui.BarH,
		Blur: a.win.Caps().Blur, CursorShape: a.win.Caps().CursorShape, Presence: a.win.Caps().Idle, Niri: a.desk.live, Music: a.music.Connected(),
		Volume: a.ui.vol.ok, Backlight: a.ui.light >= 0, Battery: a.ui.power != nil,
		Sessions: len(a.ui.sessions), Pending: len(a.ui.cards), Config: "defaults"}
	if a.track != nil {
		st.Track = a.track.Title
	}
	if !mtime(config.Path()).IsZero() {
		st.Config = config.Path()
	}
	if a.sup != nil {
		for _, h := range a.sup.Health() {
			st.Sources = append(st.Sources, ipc.Source{Name: h.Name, OK: h.OK, Detail: h.Detail})
		}
	}
	if a.host != nil {
		for _, p := range a.host.Info() {
			st.Extern = append(st.Extern, ipc.Plugin{Name: p.Name, Description: p.Description, State: p.State, Caps: p.Caps, Grants: p.Grants, Asks: p.Asks,
				Since: p.Since, Restarts: p.Restarts, Dropped: p.Dropped, Violations: p.Violations, LastError: p.LastError, Log: p.Log, Dir: p.Dir})
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

// Run opens the island and serves until the compositor goes away, or until
// SIGTERM or SIGINT, when it saves its state and stops cleanly. version is
// what it tells `ember doctor` it is.
func Run(version string) error {
	if err := wayland.WaitSocket(5 * time.Second); err != nil { // a session still starting gets a moment
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	posts := make(chan func(*app), 64)
	srv, err := listen(posts)
	if err != nil {
		return err
	}
	defer srv.close()
	win, err := wayland.Open(surfW, surfH, 0, wayland.LayerTop)
	if err != nil {
		return err
	}
	bar := readBar()
	a := &app{ui: NewUI(surfW, surfH, bar.h), win: win, posts: posts, started: time.Now(), version: version}
	a.ui.onKey = a.key
	a.ui.audio = &source.Audio{}
	a.ui.habits = loadHabits()
	a.ui.bond = loadBond(a.ui.now())
	if t := a.ui.bond.Active; !t.IsZero() && a.ui.now().Sub(t) > 4*time.Hour { // the machine was off: this is a return
		a.ui.returned(t, a.ui.now())
	}
	a.ui.day = loadDay(time.Now())
	win.OnIdle = func(idle bool) { a.presence(idle); a.ui.Invalidate() }
	a.reloadConfig()
	a.loadSessions()
	a.music, a.sup = source.NewMPRIS(), source.NewSupervisor()
	a.startSources(ctx, emitter(posts))
	a.startPlugins(ctx, stop)
	defer a.shutdown()
	win.OnPointer = func(e wayland.PointerEvent) {
		redraw, hand := a.ui.pointer(fromWayland(e))
		win.SetCursor(hand)
		if redraw {
			a.ui.Invalidate()
		}
	}
	return a.loop(ctx)
}

func (a *app) setTrack(t *Track) {
	prev := a.track
	if a.ui.audio != nil { // the equaliser listens only while a song plays
		a.ui.audio.Set(t != nil && t.Playing && a.cfg.Music)
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
