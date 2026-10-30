// Package plugin is the Ember plugin protocol (v1) in Go: the messages both
// ways as types, with the decoding and the limits Ember applies. A plugin
// is any program that speaks JSON lines on its stdin and stdout; this
// package is a convenience for plugins written in Go, and the definition
// Ember itself decodes with. docs/PLUGINS.md is the protocol.
package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Version is the protocol version this package speaks.
const Version = 1

// MaxLine is the longest line either side may send, newline included.
const MaxLine = 64 << 10

// KeyMax is the longest key a button or a pressable row may carry: room for
// an action and a long id ("giveup:lc:<a LeetCode slug>").
const KeyMax = 128

// A card's choice chips: at most ChoiceMax, each label at most ChoiceLabelMax
// characters (v2.1).
const (
	ChoiceMax      = 6
	ChoiceLabelMax = 18
)

// Moods.
const (
	Work  = "work"  // at it
	Wait  = "wait"  // needs you
	Nudge = "nudge" // worth a look
	Done  = "done"  // went well
	Fail  = "fail"  // went wrong
	Quiet = "quiet" // nothing to say
)

// Capabilities a manifest may declare.
const (
	CapPill      = "pill"
	CapMoment    = "moment"
	CapRow       = "row"
	CapCard      = "card"
	CapDashboard = "dashboard"
	CapCheer     = "cheer"
)

// Caps is every capability, in order.
var Caps = []string{CapPill, CapMoment, CapRow, CapCard, CapDashboard, CapCheer}

// Icons is Ember's icon set, the names a message may use.
var Icons = []string{"check", "warning", "bolt", "timer", "chart", "note", "read", "edit", "run", "search", "web",
	"agent", "camera", "screen", "cast", "headphones", "bluetooth", "battery", "unplug", "sun", "volume", "mic", "lock", "other"}

// Grants the user may give a plugin in Ember's config ([plugins.<name>] grants).
const (
	GrantCritical = "critical" // its line may outrank Claude at work; its cards come first
	GrantBadge    = "badge"    // its badge may sit on the clock
	GrantFocus    = "focus"    // its line may set Ember's game face
	GrantMedia    = "media"    // media events (the song playing) while its page is open
	GrantEntry    = "entry"    // its page opens from a click on Ember in a context ([opens] in its manifest)
)

// Grants is every grant, in order.
var Grants = []string{GrantCritical, GrantBadge, GrantFocus, GrantMedia, GrantEntry}

// Contexts a plugin's page may open from, on a click on Ember in the card
// ([opens] ember = "…" in its manifest, with the entry grant): music while a
// song plays, idle while none does.
var Contexts = []string{"music", "idle"}

// Close reasons a dash close event carries.
const (
	CloseUser      = "user"       // you closed it: a click on its background, a right-click, its way back, ember dash
	ClosePointer   = "pointer"    // the pointer left it (not on a sticky page)
	CloseMediaIdle = "media_idle" // ends.media_idle_s: the music paused that long, or its player went
	CloseSongEnd   = "song_end"   // ends.after_claude: Claude finished while it was open, and the song ended
	CloseOther     = "other"      // another page opened, or the plugin took it back
)

// ChipStates are a chip's states: done (filled green), tried (ringed red),
// owed (ringed amber), fixed (filled blue), gone (struck), todo (quiet).
var ChipStates = []string{"done", "tried", "owed", "fixed", "gone", "todo"}

// NameRe is what a plugin's name may be.
var NameRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// ---- plugin → Ember ----

// Hello is the plugin's answer to Ember's hello.
type Hello struct {
	V    int    `json:"v"`
	Name string `json:"name"`
}

// Status is the plugin's line in the pill.
type Status struct {
	Text     string   `json:"text"`
	Sub      string   `json:"sub,omitempty"`
	Icon     string   `json:"icon,omitempty"`
	Mood     string   `json:"mood,omitempty"`
	SinceMs  int64    `json:"since_ms,omitempty"` // a timer counting up from then
	UntilMs  int64    `json:"until_ms,omitempty"` // a timer counting down to then
	Progress *float64 `json:"progress,omitempty"` // 0..1
	Urgency  string   `json:"urgency,omitempty"`  // low | normal | high | critical (granted)
	Salience *float64 `json:"salience,omitempty"` // instead of urgency: 0..1, capped at 0.75 (0.95 granted)

	Timer      string `json:"timer,omitempty"`       // auto | none | h:mm
	WarnMs     int64  `json:"warn_ms,omitempty"`     // the last warn_ms before until_ms: the timer and sub turn amber
	Value      string `json:"value,omitempty"`       // a quiet value at the right end
	Chips      []Chip `json:"chips,omitempty"`       // letters in chips at the right end
	Badge      string `json:"badge,omitempty"`       // a small amber icon at the right end
	BadgeClock string `json:"badge_clock,omitempty"` // an icon on the clock (granted)
	Satellite  bool   `json:"satellite,omitempty"`   // its timer in the satellite when another line holds the pill
	Focus      bool   `json:"focus,omitempty"`       // Ember's game face while it holds the pill (granted)

	ID   string  `json:"id,omitempty"`    // `ember status` only: the line's name
	TTLs float64 `json:"ttl_s,omitempty"` // `ember status` only: seconds until it expires
}

// Moment is a short note in the pill.
type Moment struct {
	Text   string  `json:"text"`
	Sub    string  `json:"sub,omitempty"`
	Icon   string  `json:"icon,omitempty"`
	Mood   string  `json:"mood,omitempty"`
	TTLs   float64 `json:"ttl_s,omitempty"`
	Target string  `json:"target,omitempty"`
	URL    string  `json:"url,omitempty"` // `ember note --open` only
}

// Chip is a letter in a chip.
type Chip struct {
	Label string `json:"label"`
	State string `json:"state,omitempty"` // ChipStates
}

// Ring is a ring on the card's rail: filled to Value, or emptying from
// StartMs to EndMs.
type Ring struct {
	Value   *float64 `json:"value,omitempty"`
	StartMs int64    `json:"start_ms,omitempty"`
	EndMs   int64    `json:"end_ms,omitempty"`
	Mood    string   `json:"mood,omitempty"`
	Sweep   *bool    `json:"sweep,omitempty"` // a value ring sweeps in as it shows (the default); false: it just is
}

// Row is the plugin's row in the open card.
type Row struct {
	Title    string   `json:"title"`
	Sub      string   `json:"sub,omitempty"`
	Icon     string   `json:"icon,omitempty"`
	Mood     string   `json:"mood,omitempty"`
	Progress *float64 `json:"progress,omitempty"`
	Target   string   `json:"target,omitempty"`
	Ring     *Ring    `json:"ring,omitempty"`
	Buttons  []Button `json:"buttons,omitempty"` // at most 2; a press sends button with card "row"
	Chips    []Chip   `json:"chips,omitempty"`
	Badge    string   `json:"badge,omitempty"`
	Flame    string   `json:"flame,omitempty"` // the end of Sub that a small flame goes before
}

// Button is one of a card's buttons.
type Button struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Primary bool   `json:"primary,omitempty"`
}

// Card is a question or an offer, with buttons.
type Card struct {
	ID      string   `json:"id"`
	Eyebrow string   `json:"eyebrow,omitempty"`
	Title   string   `json:"title"`
	Body    string   `json:"body,omitempty"`
	Mood    string   `json:"mood,omitempty"`
	Buttons []Button `json:"buttons,omitempty"`
	Chips   []Button `json:"chips,omitempty"` // up to 6 small choice chips under the body; a press sends the chip's key (v2.1)
	TTLs    float64  `json:"ttl_s,omitempty"`
	Urgent  bool     `json:"urgent,omitempty"` // before Claude's requests (granted)
}

// Item is one entry of a widget: a stat, a bar, a chip, a list row or an
// agenda entry, as the widget's kind says.
type Item struct {
	Label   string   `json:"label,omitempty"`   // stats, bars, chips
	Value   any      `json:"value,omitempty"`   // stats: a string, or a number (with Unit, it can count up); bars: a number
	Unit    string   `json:"unit,omitempty"`    // stats, a number's: "" | "%" | "s" (seconds, shown as "1h 53m")
	Caption string   `json:"caption,omitempty"` // stats, bars
	State   string   `json:"state,omitempty"`   // chips: ChipStates; bars: "" | "future" | "excused"
	Title   string   `json:"title,omitempty"`   // list, agenda
	Sub     string   `json:"sub,omitempty"`     // list
	Icon    string   `json:"icon,omitempty"`    // list
	Mood    string   `json:"mood,omitempty"`    // list, stats (its dot), bars (its colour: done green, else amber)
	Mark    string   `json:"mark,omitempty"`    // list: done | skip | fail | none, an outcome instead of an icon
	Target  *float64 `json:"target,omitempty"`  // bars: a ghost of what it asks
	Today   bool     `json:"today,omitempty"`   // bars: this one is now
	Key     string   `json:"key,omitempty"`     // list, agenda: ≤ KeyMax, pressing the row sends it as a dashboard button
	Lo      *float64 `json:"lo,omitempty"`      // bars: an interval band's low end, same scale as Value (v2.1)
	Hi      *float64 `json:"hi,omitempty"`      // bars: its high end
	AtMs    int64    `json:"at_ms,omitempty"`   // agenda: when
	Tag     string   `json:"tag,omitempty"`     // agenda: a quiet word at the right
	TagMood string   `json:"tag_mood,omitempty"`
	Chips   []Chip   `json:"chips,omitempty"` // agenda: letters at the right

	Num  float64 `json:"-"` // stats, bars: Value as a number, after decoding
	Text string  `json:"-"` // stats: Value as text, after decoding
}

// Band is a shaded range across a line widget.
type Band struct {
	Lo    float64 `json:"lo"`
	Hi    float64 `json:"hi"`
	Label string  `json:"label,omitempty"`
}

// Day is one day of a heatmap.
type Day struct {
	Date  string  `json:"date"`            // 2006-01-02
	Level float64 `json:"level,omitempty"` // 0..1: 1 is the whole day's goal
	Mood  string  `json:"mood,omitempty"`  // done (green, the default), wait (blue: excused), quiet (a whisper)
}

// HeroRing is the hero's ring: Value of Goal filled, or a countdown that
// empties (from StartMs to EndMs, or Value of Goal left).
type HeroRing struct {
	Value   float64 `json:"value,omitempty"`
	Goal    float64 `json:"goal,omitempty"`
	Mode    string  `json:"mode,omitempty"` // fill | countdown
	StartMs int64   `json:"start_ms,omitempty"`
	EndMs   int64   `json:"end_ms,omitempty"`
	Mood    string  `json:"mood,omitempty"`
}

// HeroEmber is how Ember stands in the hero's ring: its mood, a pose
// (calm | attentive | sidelong | game | sleepy | proud), and a reaction when
// the dashboard opens (hop | nod | celebrate, through Ember's gate).
//
// On a stage it is Ember on the stage instead: Dance (music: with the song,
// on the host's own beat; still: it listens) and Sing (along with the
// timed text's line, while it dances), and Pose (calm, attentive: listening
// to words; sleepy: waiting something out).
type HeroEmber struct {
	Mood  string `json:"mood,omitempty"`
	Pose  string `json:"pose,omitempty"`
	React string `json:"react,omitempty"`
	Dance string `json:"dance,omitempty"` // stage: music | still
	Sing  bool   `json:"sing,omitempty"`  // stage: sings along with the timed text
}

// TimedLine is one line of timed text: when it starts, its words (≤ 160;
// "" is a break), and optionally when each word starts, as N characters of
// the line from TMs.
type TimedLine struct {
	TMs   int64       `json:"t_ms"`
	Text  string      `json:"text"`
	Words []TimedWord `json:"words,omitempty"`
}

// TimedWord is one word of a timed line: N characters, from TMs.
type TimedWord struct {
	TMs int64 `json:"t_ms"`
	N   int   `json:"n"`
}

// Clock is a timed text's own playhead, when it doesn't follow the media:
// at AtMs it was at PosMs, and it runs at Rate (0..4, default 1).
type Clock struct {
	PosMs int64   `json:"pos_ms"`
	AtMs  int64   `json:"at_ms"`
	Rate  float64 `json:"rate,omitempty"`
}

// Ends are when a page closes itself: the music paused (or its player gone)
// MediaIdleS seconds (≤ 600), and, with AfterClaude "song_end", at the end
// of the song playing when Claude finishes a turn while the page is open.
type Ends struct {
	MediaIdleS  float64 `json:"media_idle_s,omitempty"`
	AfterClaude string  `json:"after_claude,omitempty"`
}

// HeroStat is the hero's number at the top right, and its caption.
type HeroStat struct {
	Value   string `json:"value"`
	Caption string `json:"caption,omitempty"`
}

// Dots are a ladder: used, ready, the next one filling.
type Dots struct {
	Used  int     `json:"used"`
	Ready int     `json:"ready"`
	Total int     `json:"total"`
	Next  float64 `json:"next,omitempty"`
}

// Step is the next thing to do: a tag, the thing, quiet facts, its dots.
type Step struct {
	Tag     string `json:"tag,omitempty"`
	TagMood string `json:"tag_mood,omitempty"`
	Text    string `json:"text"`
	Meta    string `json:"meta,omitempty"`  // quieter, after the text ("1400")
	Facts   string `json:"facts,omitempty"` // under the text
	Dots    *Dots  `json:"dots,omitempty"`
}

// Widget is one section of a dashboard. Its kind says which fields count:
//
//   - hero: Ring, Ember, Title, Aside, Sub, SubIcon, SubMood, Chips, Stat, Step, Buttons (first, at most one)
//   - stats: Items (label, value, unit, caption, mood), Cols, CountUp
//   - sparkline: Values, Caption
//   - line: Values, Value, Delta, DeltaMood, Extra, Band
//   - bars: Items (label, value, caption, target, mood, lo, hi; columns: today, state), Layout, Value, Caption
//   - chips: Items (label, state), Caption
//   - list: Items (title, sub, icon, mood, mark), Caption
//   - heatmap: Days, Weeks, Caption
//   - agenda: Items (at_ms, title, tag, tag_mood, chips), DayEnds, Empty, Caption
//   - columns: Split, A, B (neither a hero nor columns)
//   - note: Icon, Text, Mood
//   - stage: Ember (dance, sing, pose), Light (audio | still | none), Haze (cover | none): first, at most one
//   - timed_text: MediaKey or Clock, Lines, Mode (synced | stanzas), State (loading | ready | none | instrumental | spoken), Caption
//   - media: the song playing, from Ember's own data, with play/pause
//   - sessions: Style (line): Claude's state, from Ember's own data
//   - audio: Style (ribbon), Tint (cover | quiet): the song's levels, which never leave Ember
//   - countdown: Title, Text, To (media_end): a hairline draining to the end of the media playing
//
// The kinds after note are for stage pages (docs/PLUGINS.md, Stages); at
// most one of each on a page, never in columns, never folded.
//
// Every kind but hero may fold to one line: Fold true, false or "auto"
// (folded from the bottom up until the dashboard fits), showing Summary.
type Widget struct {
	Kind    string    `json:"kind"`
	Title   string    `json:"title,omitempty"`
	Caption string    `json:"caption,omitempty"`
	Items   []Item    `json:"items,omitempty"`
	Values  []float64 `json:"values,omitempty"`
	Summary string    `json:"summary,omitempty"`
	Fold    any       `json:"fold,omitempty"` // true | false | "auto"

	Cols    int    `json:"cols,omitempty"`     // stats
	CountUp bool   `json:"count_up,omitempty"` // stats
	Layout  string `json:"layout,omitempty"`   // bars: rows | columns
	Value   string `json:"value,omitempty"`    // bars, line: the number by the title

	Band      *Band  `json:"band,omitempty"` // line
	Delta     string `json:"delta,omitempty"`
	DeltaMood string `json:"delta_mood,omitempty"`
	Extra     string `json:"extra,omitempty"`

	Weeks int   `json:"weeks,omitempty"` // heatmap
	Days  []Day `json:"days,omitempty"`

	DayEnds string `json:"day_ends,omitempty"` // agenda: when the day turns, HH:MM ("04:00")
	Empty   string `json:"empty,omitempty"`    // agenda: said when nothing's coming

	Split float64 `json:"split,omitempty"` // columns
	A     *Widget `json:"a,omitempty"`
	B     *Widget `json:"b,omitempty"`

	Icon string `json:"icon,omitempty"` // note
	Text string `json:"text,omitempty"`
	Mood string `json:"mood,omitempty"`

	Ring    *HeroRing  `json:"ring,omitempty"` // hero
	Ember   *HeroEmber `json:"ember,omitempty"`
	Aside   string     `json:"aside,omitempty"`
	Sub     string     `json:"sub,omitempty"`
	SubIcon string     `json:"sub_icon,omitempty"`
	SubMood string     `json:"sub_mood,omitempty"`
	Chips   []Chip     `json:"chips,omitempty"`
	Stat    *HeroStat  `json:"stat,omitempty"`
	Step    *Step      `json:"step,omitempty"`
	Buttons []Button   `json:"buttons,omitempty"`

	Light    string      `json:"light,omitempty"`     // stage
	Haze     string      `json:"haze,omitempty"`      // stage
	MediaKey string      `json:"media_key,omitempty"` // timed_text: the media event's key it belongs to
	Lines    []TimedLine `json:"lines,omitempty"`     // timed_text
	Mode     string      `json:"mode,omitempty"`      // timed_text
	State    string      `json:"state,omitempty"`     // timed_text
	Clock    *Clock      `json:"clock,omitempty"`     // timed_text
	Style    string      `json:"style,omitempty"`     // sessions, audio
	Tint     string      `json:"tint,omitempty"`      // audio
	To       string      `json:"to,omitempty"`        // countdown

	Folded string `json:"-"` // after decoding: yes | no | auto
}

// Dashboard is the plugin's page.
type Dashboard struct {
	Title   string   `json:"title,omitempty"`
	Widgets []Widget `json:"widgets"`
	Size    string   `json:"size,omitempty"`   // page (the default) | card: the card's width, in the card's place
	Sticky  bool     `json:"sticky,omitempty"` // the pointer leaving doesn't close it
	Ends    *Ends    `json:"ends,omitempty"`
}

// Timed says whether the page carries timed text (its updates go at most
// once a second).
func (d *Dashboard) Timed() bool {
	for _, w := range d.Widgets {
		if w.Kind == "timed_text" {
			return true
		}
	}
	return false
}

// Cheer asks Ember to react: hop | nod | celebrate.
type Cheer struct {
	Kind string `json:"kind"`
}

// Log is a line for the plugin's log.
type Log struct {
	Level string `json:"level,omitempty"` // info | warn | error
	Text  string `json:"text"`
}

// Remove takes back what the plugin showed: status | row | dashboard | card (with ID).
type Remove struct {
	What string `json:"what"`
	ID   string `json:"id,omitempty"`
}

// ---- Ember → plugin ----

// HelloEmber is Ember's first line.
type HelloEmber struct {
	V      int      `json:"v"`
	Ember  string   `json:"ember"`
	Caps   []string `json:"caps"`
	Grants []string `json:"grants,omitempty"` // what the user's config grants it
}

// Media is the song playing, as a media event tells a plugin granted media
// (only while its page is open, and when it changes). Key is Ember's name
// for this track on this player: a timed_text's media_key. An event without
// one says nothing plays.
type Media struct {
	Key      string `json:"media_key"`
	Player   string `json:"player,omitempty"`
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	URL      string `json:"url,omitempty"`      // the track's own address: file:// for a local file
	TrackID  string `json:"track_id,omitempty"` // the player's id for it (mpris:trackid)
	LengthMs int64  `json:"length_ms"`
	PosMs    int64  `json:"pos_ms"` // the playhead at AtMs
	AtMs     int64  `json:"at_ms"`
	Playing  bool   `json:"playing"`
}

// Event is something Ember tells a plugin: Type is click, button, dash,
// tool, media or stop, and the fields that go with it are set.
type Event struct {
	Type   string `json:"type"`
	Target string `json:"target,omitempty"` // click
	Card   string `json:"card,omitempty"`   // button
	Key    string `json:"key,omitempty"`    // button
	State  string `json:"state,omitempty"`  // dash: open | close
	Reason string `json:"reason,omitempty"` // dash close: user | pointer | media_idle | song_end | other

	Payload json.RawMessage `json:"payload,omitempty"` // tool: what the plugin's tool sent

	*Media // media: the song (nil: nothing plays)
}

// ---- decoding ----

// Message is one decoded message from a plugin: Type says which field is set.
type Message struct {
	Type      string
	Hello     *Hello
	Status    *Status
	Moment    *Moment
	Row       *Row
	Card      *Card
	Dashboard *Dashboard
	Cheer     *Cheer
	Log       *Log
	Remove    *Remove
}

// Cap is the capability a message needs, "" for none.
func (m Message) Cap() string {
	switch m.Type {
	case "status":
		return CapPill
	case "moment":
		return CapMoment
	case "row":
		return CapRow
	case "card":
		return CapCard
	case "dashboard":
		return CapDashboard
	case "cheer":
		return CapCheer
	}
	return ""
}

// ErrIgnored is a line of a type this version doesn't know: not an error,
// it's skipped (a newer plugin).
var ErrIgnored = errors.New("plugin: unknown message type, ignored")

// Decode reads one line from a plugin and makes it fit the limits: text is
// cut, lists are cut, numbers clamped, unknown enum values read as their
// default. It fails (a violation) on a line that is too long, not a JSON
// object, without a type, with a field of the wrong JSON type, or without a
// required field; it returns ErrIgnored for a type it doesn't know.
func Decode(line []byte) (Message, error) {
	line = bytes.TrimSpace(line)
	if len(line) >= MaxLine {
		return Message{}, fmt.Errorf("line of %d bytes: over %d", len(line), MaxLine)
	}
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return Message{}, fmt.Errorf("not a JSON object: %v", err)
	}
	m := Message{Type: head.Type}
	var err error
	switch head.Type {
	case "":
		return m, errors.New("no type")
	case "hello":
		m.Hello, err = into[Hello](line)
		if err == nil && (m.Hello.V < 1 || !NameRe.MatchString(m.Hello.Name)) {
			err = errors.New("hello needs v ≥ 1 and a name")
		}
	case "status":
		m.Status, err = into[Status](line)
		if err == nil {
			err = m.Status.fit()
		}
	case "moment":
		m.Moment, err = into[Moment](line)
		if err == nil {
			err = m.Moment.fit()
		}
	case "row":
		m.Row, err = into[Row](line)
		if err == nil {
			err = m.Row.fit()
		}
	case "card":
		m.Card, err = into[Card](line)
		if err == nil {
			err = m.Card.fit()
		}
	case "dashboard":
		m.Dashboard, err = into[Dashboard](line)
		if err == nil {
			err = m.Dashboard.fit()
		}
	case "cheer":
		m.Cheer, err = into[Cheer](line)
		if err == nil && !slices.Contains([]string{"hop", "nod", "celebrate"}, m.Cheer.Kind) {
			err = fmt.Errorf("cheer %q: hop, nod or celebrate", m.Cheer.Kind)
		}
	case "log":
		m.Log, err = into[Log](line)
		if err == nil {
			m.Log.Text = Cut(m.Log.Text, 500)
			m.Log.Level = enum(m.Log.Level, "info", "info", "warn", "error")
		}
	case "remove":
		m.Remove, err = into[Remove](line)
		if err == nil {
			r := m.Remove
			r.ID = Cut(r.ID, 64)
			switch {
			case !slices.Contains([]string{"status", "row", "card", "dashboard"}, r.What):
				err = fmt.Errorf("remove %q: status, row, card or dashboard", r.What)
			case r.What == "card" && r.ID == "":
				err = errors.New("remove card needs its id")
			}
		}
	default:
		return m, ErrIgnored
	}
	if err != nil {
		return Message{}, fmt.Errorf("%s: %w", head.Type, err)
	}
	return m, nil
}

func into[T any](line []byte) (*T, error) {
	v := new(T)
	if err := json.Unmarshal(line, v); err != nil {
		return nil, err
	}
	return v, nil
}

// Cut shortens s to n characters, the last an ellipsis, and makes it valid
// UTF-8 without control characters.
func Cut(s string, n int) string { return cut(s, n, false) }

// CutLines is Cut for text that keeps its line breaks: a card's body, in
// paragraphs. "\r\n" reads as "\n".
func CutLines(s string, n int) string {
	return cut(strings.ReplaceAll(s, "\r\n", "\n"), n, true)
}

func cut(s string, n int, lines bool) string {
	if !utf8.ValidString(s) {
		s = string([]rune(s)) // invalid bytes become U+FFFD
	}
	rs := []rune(s)
	out := rs[:0]
	for _, r := range rs {
		switch {
		case r == '\n' && lines:
			out = append(out, r)
			continue
		case r == '\n' || r == '\t' || r == '\r':
			r = ' '
		}
		if r >= 0x20 && r != 0x7f {
			out = append(out, r)
		}
	}
	if len(out) > n {
		out = append(out[:n-1], '…')
	}
	return string(out)
}

func enum(v, def string, ok ...string) string {
	if slices.Contains(ok, v) {
		return v
	}
	return def
}

func icon(v string) string { return enum(v, "", Icons...) }

func mood(v, def string) string { return enum(v, def, Work, Wait, Nudge, Done, Fail, Quiet) }

func chips(cs []Chip) []Chip {
	if len(cs) > 9 {
		cs = cs[:9]
	}
	out := cs[:0]
	for _, c := range cs {
		if c.Label = Cut(c.Label, 4); c.Label != "" {
			c.State = enum(c.State, "todo", ChipStates...)
			out = append(out, c)
		}
	}
	return out
}

func buttons(bs []Button, n int) []Button {
	if len(bs) > n {
		bs = bs[:n]
	}
	out := bs[:0]
	for _, b := range bs {
		if b.Key, b.Label = Cut(b.Key, KeyMax), Cut(b.Label, 24); b.Key != "" && b.Label != "" {
			out = append(out, b)
		}
	}
	return out
}

// choices are a card's choice chips: up to ChoiceMax with a key (at most
// KeyMax) and a label, labels cut to ChoiceLabelMax. A chip is never primary-filled; its
// Primary only tints it, as the one that leads.
func choices(bs []Button) []Button {
	out := bs[:0]
	for _, b := range bs {
		if len(out) == ChoiceMax {
			break
		}
		if b.Key, b.Label = Cut(b.Key, KeyMax+1), Cut(b.Label, ChoiceLabelMax); b.Key != "" && len([]rune(b.Key)) <= KeyMax && b.Label != "" {
			out = append(out, b) // a key too long is dropped, not cut: a cut key would send another action
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	return math.Max(lo, math.Min(hi, v))
}

func unit(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := clamp(*p, 0, 1)
	return &v
}

func (s *Status) fit() error {
	if s.Text = Cut(s.Text, 40); s.Text == "" {
		return errors.New("needs text")
	}
	s.Sub, s.Icon, s.Mood = Cut(s.Sub, 40), icon(s.Icon), mood(s.Mood, Work)
	s.Progress, s.Urgency = unit(s.Progress), enum(s.Urgency, "normal", "low", "normal", "high", "critical")
	s.Salience = unit(s.Salience)
	s.SinceMs, s.UntilMs, s.WarnMs = max(s.SinceMs, 0), max(s.UntilMs, 0), max(s.WarnMs, 0)
	s.Timer, s.Value, s.Chips, s.Badge = enum(s.Timer, "auto", "auto", "none", "h:mm"), Cut(s.Value, 20), chips(s.Chips), icon(s.Badge)
	s.BadgeClock = icon(s.BadgeClock)
	s.ID, s.TTLs = Cut(s.ID, 32), clamp(s.TTLs, 0, 86400)
	return nil
}

func (m *Moment) fit() error {
	if m.Text = Cut(m.Text, 40); m.Text == "" {
		return errors.New("needs text")
	}
	m.Sub, m.Icon, m.Mood = Cut(m.Sub, 40), icon(m.Icon), mood(m.Mood, Quiet)
	m.Target, m.URL = Cut(m.Target, 32), Cut(m.URL, 2048)
	if m.TTLs == 0 {
		m.TTLs = 5
	}
	m.TTLs = clamp(m.TTLs, 1, 10)
	return nil
}

func (r *Row) fit() error {
	if r.Title = Cut(r.Title, 40); r.Title == "" {
		return errors.New("needs a title")
	}
	r.Sub, r.Icon, r.Mood = Cut(r.Sub, 60), icon(r.Icon), mood(r.Mood, Quiet)
	r.Progress, r.Target = unit(r.Progress), Cut(r.Target, 32)
	r.Buttons, r.Chips, r.Badge, r.Flame = buttons(r.Buttons, 2), chips(r.Chips), icon(r.Badge), Cut(r.Flame, 40)
	if g := r.Ring; g != nil {
		g.Value, g.Mood = unit(g.Value), mood(g.Mood, Work)
		g.StartMs, g.EndMs = max(g.StartMs, 0), max(g.EndMs, 0)
		if g.Value == nil && (g.EndMs <= g.StartMs) {
			r.Ring = nil // neither a value nor a span: no ring
		}
	}
	return nil
}

func (c *Card) fit() error {
	c.ID, c.Title = Cut(c.ID, 64), Cut(c.Title, 60)
	if c.ID == "" || c.Title == "" {
		return errors.New("needs an id and a title")
	}
	c.Eyebrow, c.Body, c.Mood = Cut(c.Eyebrow, 32), CutLines(c.Body, 280), mood(c.Mood, Nudge)
	c.Buttons, c.Chips = buttons(c.Buttons, 3), choices(c.Chips)
	switch {
	case c.TTLs == 0 && len(c.Buttons)+len(c.Chips) > 0:
		c.TTLs = 180
	case c.TTLs == 0:
		c.TTLs = 7
	}
	c.TTLs = clamp(c.TTLs, 5, 600)
	return nil
}

func (d *Dashboard) fit() error {
	d.Title = Cut(d.Title, 32)
	d.Size = enum(d.Size, "page", "page", "card")
	if e := d.Ends; e != nil {
		e.MediaIdleS, e.AfterClaude = clamp(e.MediaIdleS, 0, 600), enum(e.AfterClaude, "", "", "song_end")
		if e.MediaIdleS == 0 && e.AfterClaude == "" {
			d.Ends = nil
		}
	}
	if len(d.Widgets) > 12 {
		d.Widgets = d.Widgets[:12]
	}
	ws := d.Widgets[:0]
	seen := map[string]bool{}
	for i, w := range d.Widgets {
		if (w.Kind == "hero" || w.Kind == "stage") && i > 0 {
			continue // a hero or a stage comes first, once
		}
		if slices.Contains(stageKinds, w.Kind) {
			if seen[w.Kind] {
				continue // one of each on a page
			}
			seen[w.Kind] = true
		}
		if w.fit(true) {
			ws = append(ws, w)
		}
	}
	d.Widgets = ws
	return nil
}

var itemLimit = map[string]int{"stats": 4, "bars": 12, "chips": 12, "list": 8, "agenda": 5,
	"sparkline": 0, "line": 0, "heatmap": 0, "columns": 0, "note": 0, "hero": 0,
	"stage": 0, "timed_text": 0, "media": 0, "sessions": 0, "audio": 0, "countdown": 0}

// stageKinds are the widgets for stage pages: one of each at most, at the
// top level, never folded.
var stageKinds = []string{"stage", "timed_text", "media", "sessions", "audio", "countdown"}

// Timed text's limits.
const (
	MaxTimedLines = 400
	MaxTimedText  = 60 << 10 // bytes of words in all
)

// fit makes a widget fit; false for a kind this version doesn't draw. top
// is false for a column's children: no hero, no columns there.
func (w *Widget) fit(top bool) bool {
	n, ok := itemLimit[w.Kind]
	staged := slices.Contains(stageKinds, w.Kind)
	if !ok || (!top && (w.Kind == "hero" || w.Kind == "columns" || staged)) {
		return false
	}
	capN := 40
	if w.Kind == "timed_text" {
		capN = 60
	}
	w.Title, w.Caption, w.Summary = Cut(w.Title, 32), Cut(w.Caption, capN), Cut(w.Summary, 80)
	switch f := w.Fold.(type) {
	case bool:
		w.Folded = map[bool]string{true: "yes", false: "no"}[f]
	case string:
		w.Folded = enum(f, "no", "auto", "yes", "no")
	default:
		w.Folded = "no"
	}
	if w.Kind == "hero" || !top || staged {
		w.Folded = "no"
	}
	w.Fold = nil
	if len(w.Items) > n {
		w.Items = w.Items[:n]
	}
	if len(w.Values) > 120 {
		w.Values = w.Values[len(w.Values)-120:]
	}
	for i := range w.Values {
		w.Values[i] = clamp(w.Values[i], -1e12, 1e12)
	}
	if w.Kind != "sparkline" && w.Kind != "line" {
		w.Values = nil
	}
	w.Value, w.Delta, w.Extra = Cut(w.Value, 12), Cut(w.Delta, 12), Cut(w.Extra, 12)
	w.DeltaMood = mood(w.DeltaMood, Quiet)
	switch w.Kind {
	case "stats":
		w.Cols = int(clamp(float64(w.Cols), 0, 4))
		if w.Cols == 0 {
			w.Cols = max(len(w.Items), 1)
		}
	case "bars":
		w.Layout = enum(w.Layout, "rows", "rows", "columns")
	case "line":
		if b := w.Band; b != nil {
			b.Label = Cut(b.Label, 32)
			if !(b.Hi > b.Lo) {
				w.Band = nil
			}
		}
	case "heatmap":
		w.Weeks = int(clamp(float64(w.Weeks), 4, 53))
		if len(w.Days) > 53*7 {
			w.Days = w.Days[len(w.Days)-53*7:]
		}
		for i := range w.Days {
			d := &w.Days[i]
			d.Date, d.Level, d.Mood = Cut(d.Date, 10), clamp(d.Level, 0, 1), mood(d.Mood, Done)
		}
	case "agenda":
		w.DayEnds, w.Empty = Cut(w.DayEnds, 5), Cut(w.Empty, 60)
	case "columns":
		if w.Split == 0 {
			w.Split = 0.5
		}
		w.Split = clamp(w.Split, 0.3, 0.7)
		if w.A == nil || w.B == nil || !w.A.fit(false) || !w.B.fit(false) {
			return false
		}
	case "note":
		w.Icon, w.Text, w.Mood = icon(w.Icon), Cut(w.Text, 80), mood(w.Mood, Quiet)
		if w.Text == "" {
			return false
		}
	case "stage":
		if w.Ember == nil {
			w.Ember = &HeroEmber{}
		}
		e := w.Ember
		e.Mood, e.Pose, e.React = mood(e.Mood, Quiet), enum(e.Pose, "calm", "calm", "attentive", "sleepy"), ""
		e.Dance = enum(e.Dance, "music", "music", "still")
		w.Light, w.Haze = enum(w.Light, "audio", "audio", "still", "none"), enum(w.Haze, "cover", "cover", "none")
	case "timed_text":
		w.fitTimed()
	case "sessions":
		w.Style = enum(w.Style, "line", "line")
	case "audio":
		w.Style, w.Tint = enum(w.Style, "ribbon", "ribbon"), enum(w.Tint, "cover", "cover", "quiet")
	case "countdown":
		w.Text, w.To = Cut(w.Text, 40), enum(w.To, "media_end", "media_end")
	case "hero":
		w.Aside, w.Sub, w.SubIcon, w.SubMood = Cut(w.Aside, 12), Cut(w.Sub, 60), icon(w.SubIcon), mood(w.SubMood, Quiet)
		w.Chips, w.Buttons = chips(w.Chips), buttons(w.Buttons, 2)
		if r := w.Ring; r != nil {
			r.Mode, r.Mood = enum(r.Mode, "fill", "fill", "countdown"), mood(r.Mood, Nudge)
			r.Value, r.Goal = clamp(r.Value, 0, 1e12), clamp(r.Goal, 0, 1e12)
		}
		if e := w.Ember; e != nil {
			e.Mood = mood(e.Mood, Quiet)
			e.Pose = enum(e.Pose, "calm", "calm", "attentive", "sidelong", "game", "sleepy", "proud")
			e.React = enum(e.React, "", "hop", "nod", "celebrate")
			e.Dance, e.Sing = "", false
		}
		if st := w.Stat; st != nil {
			st.Value, st.Caption = Cut(st.Value, 6), Cut(st.Caption, 16)
		}
		if sp := w.Step; sp != nil {
			sp.Tag, sp.TagMood, sp.Text = Cut(sp.Tag, 12), mood(sp.TagMood, Nudge), Cut(sp.Text, 48)
			sp.Meta, sp.Facts = Cut(sp.Meta, 12), Cut(sp.Facts, 32)
			if d := sp.Dots; d != nil {
				d.Total = int(clamp(float64(d.Total), 0, 5))
				d.Used, d.Ready = int(clamp(float64(d.Used), 0, float64(d.Total))), int(clamp(float64(d.Ready), 0, float64(d.Total)))
				d.Next = clamp(d.Next, 0, 1)
			}
		}
	}
	if w.Kind != "hero" {
		w.Ring, w.Stat, w.Step, w.Chips, w.Buttons = nil, nil, nil, nil, nil
		if w.Kind != "stage" {
			w.Ember = nil
		}
	}
	if w.Kind != "stage" {
		w.Light, w.Haze = "", ""
	}
	if w.Kind != "timed_text" {
		w.MediaKey, w.Lines, w.Mode, w.State, w.Clock = "", nil, "", "", nil
	}
	if w.Kind != "sessions" && w.Kind != "audio" {
		w.Style = ""
	}
	if w.Kind != "audio" {
		w.Tint = ""
	}
	if w.Kind != "countdown" {
		w.To = ""
	}
	if w.Kind != "columns" {
		w.A, w.B = nil, nil
	}
	items := w.Items[:0]
	for _, it := range w.Items {
		num, isNum := it.Value.(float64)
		key, lo, hi := it.Key, it.Lo, it.Hi
		it.Key, it.Lo, it.Hi = "", nil, nil // only a list's or an agenda's rows press; only bars have bands
		switch w.Kind {
		case "stats":
			it.Label, it.Caption, it.Mood = Cut(it.Label, 20), Cut(it.Caption, 24), mood(it.Mood, Quiet)
			it.Unit = enum(it.Unit, "", "", "%", "s")
			switch v := it.Value.(type) {
			case string:
				it.Text = Cut(v, 12)
			case float64:
				it.Num, it.Text = clamp(num, -1e12, 1e12), Cut(fmt.Sprint(v), 12)
			}
			if it.Label == "" && it.Text == "" {
				continue
			}
		case "bars":
			it.Label, it.Caption, it.Mood = Cut(it.Label, 20), Cut(it.Caption, 12), mood(it.Mood, "") // none: as the layout draws it
			it.State = enum(it.State, "", "", "future", "excused")
			if isNum {
				it.Num = clamp(num, 0, 1e12)
			}
			if t := it.Target; t != nil {
				*t = clamp(*t, 0, 1e12)
			}
			if lo != nil && hi != nil { // a band needs both ends, low first
				l, h := clamp(*lo, 0, 1e12), clamp(*hi, 0, 1e12)
				l, h = min(l, h), max(l, h)
				it.Lo, it.Hi = &l, &h
			}
		case "chips":
			it.Label, it.State = Cut(it.Label, 4), enum(it.State, "todo", ChipStates...)
			if it.Label == "" {
				continue
			}
		case "list":
			it.Title, it.Sub, it.Icon, it.Mood = Cut(it.Title, 40), Cut(it.Sub, 60), icon(it.Icon), mood(it.Mood, Quiet)
			it.Key = Cut(key, KeyMax)
			it.Mark = enum(it.Mark, "", "", "done", "skip", "fail", "none")
			if it.Title == "" {
				continue
			}
		case "agenda":
			it.Title, it.Tag, it.TagMood, it.Chips = Cut(it.Title, 40), Cut(it.Tag, 14), mood(it.TagMood, Quiet), chips(it.Chips)
			it.Key = Cut(key, KeyMax)
			if len(it.Chips) > 4 {
				it.Chips = it.Chips[:4]
			}
			if it.Title == "" || it.AtMs <= 0 {
				continue
			}
		}
		it.Value = nil
		items = append(items, it)
	}
	w.Items = items
	return true
}

// fitTimed makes timed text fit: its lines sorted by time and cut to their
// limits (400 lines, 160 characters each, 40 timed words a line, 60 KiB of
// words in all), a word never before its line.
func (w *Widget) fitTimed() {
	w.MediaKey = Cut(w.MediaKey, 64)
	w.Mode = enum(w.Mode, "synced", "synced", "stanzas")
	w.State = enum(w.State, "ready", "loading", "ready", "none", "instrumental", "spoken")
	if c := w.Clock; c != nil {
		c.PosMs, c.AtMs = max(c.PosMs, 0), max(c.AtMs, 0)
		if c.Rate == 0 {
			c.Rate = 1
		}
		c.Rate = clamp(c.Rate, 0, 4)
	}
	if len(w.Lines) > MaxTimedLines {
		w.Lines = w.Lines[:MaxTimedLines]
	}
	total := 0
	for i := range w.Lines {
		l := &w.Lines[i]
		l.TMs, l.Text = max(l.TMs, 0), Cut(l.Text, 160)
		if total += len(l.Text); total > MaxTimedText {
			w.Lines = w.Lines[:i]
			break
		}
		if len(l.Words) > 40 {
			l.Words = l.Words[:40]
		}
		n := utf8.RuneCountInString(l.Text)
		for j := range l.Words {
			wd := &l.Words[j]
			wd.TMs, wd.N = max(wd.TMs, l.TMs), int(clamp(float64(wd.N), 0, float64(n)))
		}
		slices.SortStableFunc(l.Words, func(a, b TimedWord) int { return cmpInt(a.TMs, b.TMs) })
		if len(l.Words) == 0 {
			l.Words = nil
		}
	}
	slices.SortStableFunc(w.Lines, func(a, b TimedLine) int { return cmpInt(a.TMs, b.TMs) })
	if len(w.Lines) == 0 {
		w.Lines = nil
	}
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Line encodes v as one protocol line, newline included.
func Line(typ string, v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["type"] = typ
	b, err = json.Marshal(m)
	return append(b, '\n'), err
}
