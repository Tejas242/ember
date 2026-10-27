package island

import "github.com/Tejas242/ember/internal/source"

// Track is what the active media player plays.
type Track = source.Track

// emitter turns the sources' events into posts on the island's loop.
func emitter(posts chan<- func(*app)) source.Emit {
	return func(e source.Event) { posts <- func(a *app) { a.handle(e) } }
}

// handle applies one event from a source, on the loop.
func (a *app) handle(e source.Event) {
	switch e := e.(type) {
	case source.NiriWindows:
		a.desk.live, a.desk.windows, a.desk.focused = true, map[int]deskWindow{}, 0
		for _, w := range e.Windows {
			a.desk.windows[w.ID] = w
			if w.Focused {
				a.desk.focused = w.ID
			}
		}
		a.deskChanged()
	case source.NiriWindow:
		if a.desk.windows == nil {
			a.desk.windows = map[int]deskWindow{}
		}
		a.desk.windows[e.Window.ID] = e.Window
		if e.Window.Focused {
			a.desk.focused = e.Window.ID
		}
		a.deskChanged()
	case source.NiriWindowClosed:
		delete(a.desk.windows, e.ID)
		a.quiet = true
	case source.NiriFocus:
		a.desk.focused = e.ID
		a.deskChanged()
	case source.NiriCasts:
		a.castsChanged(e.N)
	case source.NiriDown:
		a.desk.live = false
	case source.Power:
		p := power{pct: e.Pct, state: e.State, present: e.Present, tte: e.TTE, ttf: e.TTF}
		prev := a.ui.power
		a.ui.power = &p
		if !e.First && prev != nil {
			a.powerChanged(*prev, p)
		}
	case source.Device:
		a.deviceChanged(e.Name, e.Kind, e.On, e.Pct)
	case source.Volume:
		l := level{v: e.V, muted: e.Muted, ok: e.OK}
		if e.First {
			a.ui.vol = l
		} else {
			a.volumeChanged(l)
		}
	case source.Mic:
		if e.Level {
			a.micLevel(level{v: e.V, muted: e.Muted, ok: true})
		} else {
			a.micChanged(e.Muted)
		}
	case source.Brightness:
		if e.First {
			a.ui.light = e.V
		} else {
			a.brightnessChanged(e.V)
		}
	case source.ShellChanged:
		a.raise(e.Why)
	case source.TrackChanged:
		a.setTrack(e.Track)
	case source.Playhead:
		if a.ui.track != nil && a.ui.track.Player == e.Player {
			a.ui.track.Position, a.ui.track.PosAt = e.Pos, e.At
			a.ui.heardPlayhead()
		}
	case source.Screenshot:
		if e.Path == "" {
			a.screenshot("", nil)
		} else {
			go screenshotTaken(a.posts, e.Path) // decoding is slow: off the loop
		}
	}
}
