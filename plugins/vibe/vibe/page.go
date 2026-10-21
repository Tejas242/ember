package vibe

import (
	"strings"

	"github.com/Tejas242/ember/pkg/plugin"
)

// What is playing, in a word: a song, an ad, or a talk. Ember never tells a
// plugin the song's levels, so this is read from what the player says.

// Kind is what a track is.
type Kind string

const (
	Song Kind = "song"
	Ad   Kind = "ad"   // Spotify's /ad/ track ids, "Advertisement", a short track without an artist
	Talk Kind = "talk" // a podcast's episode
	Long Kind = "long" // over twenty minutes, not marked as talk: a mix or a talk, no words looked up
)

// Classify says what m is.
func Classify(m *plugin.Media) Kind {
	id, u := strings.ToLower(m.TrackID), strings.ToLower(m.URL)
	switch {
	case strings.Contains(id, "/ad/") || strings.Contains(id, ":ad:") || strings.EqualFold(strings.TrimSpace(m.Title), "Advertisement"),
		m.Artist == "" && m.LengthMs > 0 && m.LengthMs <= 60_000 && strings.Contains(strings.ToLower(m.Player), "spotify"):
		return Ad
	case strings.Contains(id, "episode") || strings.Contains(u, "episode") || strings.Contains(u, "podcast"):
		return Talk
	case m.LengthMs > 20*60_000:
		return Long
	}
	return Song
}

// maxWordsBytes keeps a page's words well inside a line (64 KiB).
const maxWordsBytes = 56 << 10

// Page is the stage for what plays: the stage, Claude's line, the words (or
// the song's title over its ribbon), and the song. w is nil while the words
// are being looked up.
func Page(m *plugin.Media, w *Words) plugin.Dashboard {
	stage := plugin.Widget{Kind: "stage", Ember: &plugin.HeroEmber{Dance: "music", Sing: true}, Light: "audio", Haze: "cover"}
	text := plugin.Widget{Kind: "timed_text", State: "loading"}
	audio := plugin.Widget{Kind: "audio", Style: "ribbon", Tint: "cover"}
	ws := []plugin.Widget{}
	if m != nil {
		text.MediaKey = m.Key
	}
	switch k := kindOf(m); {
	case m == nil:
		text.State = "none"
	case k == Ad:
		stage.Ember = &plugin.HeroEmber{Dance: "still", Pose: "sleepy"}
		stage.Light = "still"
		text = plugin.Widget{Kind: "countdown", Title: "Ad", Text: "Back to the music", To: "media_end"}
	case k == Talk:
		stage.Ember = &plugin.HeroEmber{Dance: "still", Pose: "attentive"}
		stage.Light = "still"
		text.State, audio.Tint = "spoken", "quiet"
	case k == Long:
		text.State = "none"
	case w == nil:
	default:
		text.State = "ready"
		switch w.State {
		case "synced":
			text.Lines = timed(w.Lines)
			if m.LengthMs <= 0 { // no playhead to follow (some browsers): the words as stanzas
				text.Mode, text.Caption = "stanzas", "Words not timed for this player"
			}
		case "plain":
			text.Lines, text.Mode, text.Caption = timed(w.Lines), "stanzas", "Words not timed for this song"
		default:
			text.State = w.State // none | instrumental
		}
	}
	ws = append(ws, stage, plugin.Widget{Kind: "sessions", Style: "line"}, text, audio, plugin.Widget{Kind: "media"})
	return plugin.Dashboard{Title: "Vibe", Size: "card", Sticky: true,
		Ends: &plugin.Ends{MediaIdleS: 120, AfterClaude: "song_end"}, Widgets: ws}
}

func kindOf(m *plugin.Media) Kind {
	if m == nil {
		return ""
	}
	return Classify(m)
}

// timed is the words as timed text, within its limits.
func timed(ls []Line) []plugin.TimedLine {
	var out []plugin.TimedLine
	n := 0
	for _, l := range ls {
		t := plugin.Cut(l.Text, 160)
		if n += len(t) + 24; n > maxWordsBytes || len(out) >= plugin.MaxTimedLines {
			break
		}
		out = append(out, plugin.TimedLine{TMs: max(l.At, 0).Milliseconds(), Text: t})
	}
	return out
}

// newSong: two media events are different tracks.
func newSong(a, b *plugin.Media) bool {
	if a == nil || b == nil {
		return a != b
	}
	return a.Key != b.Key
}
