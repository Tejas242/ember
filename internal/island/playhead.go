package island

import "time"

// Some players (a browser's, often) say a song is playing but never say
// where it is: their position stays put, or stays at 0. Words timed to that
// clock would freeze on one line. Each time the player says its position
// (a change it signals, or the poll while timed words are open), it's
// compared with the time that passed: one that hasn't moved for more than
// stuckAfter while playing marks the song (u.stuck), and the stage pages
// its words by the wall clock instead. It's trusted again as soon as its
// position advances with time. A seek moves it too, but proves nothing
// either way.

const stuckAfter = 4 * time.Second

// playheadWatch is what the player has said of the song's playhead.
type playheadWatch struct {
	key      string        // the song (its media key)
	pos      time.Duration // the position it said last
	at       time.Time     // when
	moved    time.Time     // when it last moved with time (or started being watched, or playing)
	start    time.Time     // when the song started by the wall clock, pauses left out
	pausedAt time.Time     // zero while it plays
}

// heardPlayhead takes in what the player just said of the track (its
// Position, read at PosAt).
func (u *UI) heardPlayhead() {
	t, w := u.track, &u.playhead
	if t == nil {
		*w, u.stuck = playheadWatch{}, ""
		return
	}
	key, at := mediaKey(t), t.PosAt
	if w.key != key { // a new song: trusted until it shows otherwise
		*w = playheadWatch{key: key, pos: t.Position, at: at, moved: at, start: at.Add(-t.Position)}
		if !t.Playing {
			w.pausedAt = at
		}
		u.stuck = ""
		return
	}
	if !at.After(w.at) {
		return
	}
	d, wall := t.Position-w.pos, at.Sub(w.at)
	switch {
	case !t.Playing: // paused: nothing to judge
		if w.pausedAt.IsZero() {
			w.pausedAt = at
		}
		w.moved = at
	case !w.pausedAt.IsZero(): // playing again: the pause doesn't count
		w.start, w.pausedAt, w.moved = w.start.Add(at.Sub(w.pausedAt)), time.Time{}, at
	case d > 0 && d >= wall/2 && d <= wall*3/2+time.Second: // it moves with time
		w.moved = at
		if u.stuck == key {
			u.stuck = ""
		}
	case d != 0: // a seek: the song is somewhere else now
		w.moved, w.start = at, at.Add(-t.Position)
	case at.Sub(w.moved) > stuckAfter:
		u.stuck = key
	}
	w.pos, w.at = t.Position, at
}

// songElapsed is how far into the song it is by the wall clock (for a
// player that doesn't say), within its length.
func (u *UI) songElapsed(now time.Time) time.Duration {
	t, w := u.track, &u.playhead
	if t == nil || w.key != mediaKey(t) || w.start.IsZero() {
		return 0
	}
	end := now
	if !w.pausedAt.IsZero() {
		end = w.pausedAt
	}
	d := max(end.Sub(w.start), 0)
	if t.Length > 0 {
		d = min(d, t.Length)
	}
	return d
}
