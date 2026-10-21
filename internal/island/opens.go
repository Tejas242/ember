package island

import (
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
)

// Pages that open from Ember, follow the music and close themselves: a
// plugin's page may say it is card-sized and sticky, and when it ends (the
// music paused a while, or the song Claude finished in is over); a plugin
// granted entry may own a click on Ember in the card in one context (music,
// idle); one granted media hears about the song, only while its page is
// open. docs/PLUGINS.md has the protocol; stage.go draws a stage.

// page is the open page's declaration, if a plugin's page is open.
func (u *UI) page() *proto.Dashboard {
	if u.dash == "" {
		return nil
	}
	return extDash{name: u.dash}.dash(u)
}

// pageSticky: the open page stays when the pointer leaves it.
func (u *UI) pageSticky() bool {
	p := u.page()
	return p != nil && p.Sticky
}

// entryCtx is the context a click on Ember is in: music while a song plays.
func (u *UI) entryCtx() string {
	if t := u.track; t != nil && t.Playing && u.cfg.Music {
		return "music"
	}
	return "idle"
}

// emberBusy: Ember's click has a job of its own (a session waiting on you, a
// Done you haven't seen): no page opens from it.
func (u *UI) emberBusy() bool {
	if u.waitingOnYou() {
		return true
	}
	for _, s := range u.sessions {
		if !s.Unseen.IsZero() {
			return true
		}
	}
	return false
}

// opener is the plugin whose page a click on Ember would open now, and its
// hint, if there is one and its page is there to open.
func (u *UI) opener() (name, hint string) {
	if u.openers == nil || u.lastView != vExpanded || u.emberBusy() {
		return "", ""
	}
	name, hint = u.openers(u.entryCtx())
	if name == "" || u.dashboards[name] == nil {
		return "", ""
	}
	return name, hint
}

// enterPage opens the page a click on Ember in the card opens, if any.
func (u *UI) enterPage() bool {
	name, _ := u.opener()
	if name == "" {
		return false
	}
	u.openDash(name)
	return true
}

// pageEnds closes the open page when its ends say: the music paused (or
// gone) media_idle_s, or, once Claude has finished a turn while it was open,
// the song then playing is over. Never on a timer of its own.
func (u *UI) pageEnds(now time.Time) {
	p := u.page()
	if p == nil || p.Ends == nil {
		return
	}
	e := p.Ends
	if e.MediaIdleS > 0 {
		if t := u.track; t == nil || !t.Playing {
			if u.idleSince.IsZero() {
				u.idleSince = now
			}
			if now.Sub(u.idleSince) >= time.Duration(e.MediaIdleS*float64(time.Second)) {
				u.closeDash(proto.CloseMediaIdle)
				return
			}
		} else {
			u.idleSince = time.Time{}
		}
	}
	if e.AfterClaude == "song_end" {
		if u.finishedSong == "" {
			for _, s := range u.sessions {
				if !s.Unseen.IsZero() && s.Unseen.After(u.dashAt) {
					u.finishedSong = mediaKey(u.track)
					if u.finishedSong == "" {
						u.finishedSong = "-" // nothing playing: it ends with the next one's change
					}
				}
			}
		}
		if u.finishedSong != "" && mediaKey(u.track) != u.finishedSong && !(u.finishedSong == "-" && u.track == nil) {
			u.closeDash(proto.CloseSongEnd)
		}
	}
}

// mediaKey is Ember's name for a track on a player: a timed text's media_key.
func mediaKey(t *Track) string {
	if t == nil {
		return ""
	}
	h := fnv.New64a()
	for _, s := range []string{t.Player, t.Title, t.Artist, t.Album, strconv.FormatInt(t.Length.Milliseconds()/1000, 10)} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// media is the song as a media event tells it: nil when none plays (or
// music is off).
func (u *UI) media() *proto.Media {
	t := u.track
	if t == nil || !u.cfg.Music {
		return nil
	}
	now := u.now()
	return &proto.Media{Key: mediaKey(t), Player: strings.TrimPrefix(t.Player, "org.mpris.MediaPlayer2."), Title: t.Title, Artist: t.Artist,
		Album: t.Album, URL: t.URL, TrackID: t.TrackID, LengthMs: t.Length.Milliseconds(), PosMs: max(t.Pos(now), 0).Milliseconds(),
		AtMs: now.UnixMilli(), Playing: t.Playing}
}

// extMedia tells the open page's plugin about the song, if it's granted
// media: once as it opens, then when it changes (another song, play or
// pause, a seek). Nothing reaches a closed page.
func (a *app) extMedia() {
	u := a.ui
	name := u.dash
	if a.host == nil || name == "" || u.ext[name] == nil || !u.granted(name, proto.GrantMedia) {
		a.mediaTo, a.mediaLast = "", nil
		return
	}
	m := u.media()
	if name == a.mediaTo && sameMedia(a.mediaLast, m) {
		return
	}
	a.mediaTo, a.mediaLast = name, m
	a.host.Send(name, proto.Event{Type: "media", Media: m})
}

// sameMedia: b is a told: the same song, playing or not alike, its playhead
// where a would have it (within a second).
func sameMedia(a, b *proto.Media) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Key != b.Key || a.Playing != b.Playing {
		return false
	}
	want := a.PosMs
	if a.Playing {
		want += b.AtMs - a.AtMs
	}
	return math.Abs(float64(b.PosMs-want)) < 1000
}

// followsPlayhead: the open page shows something timed to the song, so the
// player's playhead is read now and then.
func (u *UI) followsPlayhead() bool {
	p := u.page()
	return p != nil && u.lastView == vDash && p.Timed()
}
