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
	"unicode/utf8"
)

// Version is the protocol version this package speaks.
const Version = 1

// MaxLine is the longest line either side may send, newline included.
const MaxLine = 64 << 10

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
	BadgeClock bool   `json:"badge_clock,omitempty"` // the badge on the clock too (granted)
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
	TTLs    float64  `json:"ttl_s,omitempty"`
	Urgent  bool     `json:"urgent,omitempty"` // before Claude's requests (granted)
}

// Item is one entry of a widget: a stat, a bar, a chip or a list row, as
// the widget's kind says.
type Item struct {
	Label   string  `json:"label,omitempty"`   // stats, bars, chips
	Value   any     `json:"value,omitempty"`   // stats: a string; bars: a number
	Caption string  `json:"caption,omitempty"` // stats, bars
	State   string  `json:"state,omitempty"`   // chips
	Title   string  `json:"title,omitempty"`   // list
	Sub     string  `json:"sub,omitempty"`     // list
	Icon    string  `json:"icon,omitempty"`    // list
	Mood    string  `json:"mood,omitempty"`    // list
	Num     float64 `json:"-"`                 // bars: Value as a number, after decoding
	Text    string  `json:"-"`                 // stats: Value as text, after decoding
}

// Widget is one section of a dashboard.
type Widget struct {
	Kind    string    `json:"kind"` // stats | sparkline | bars | chips | list
	Title   string    `json:"title,omitempty"`
	Items   []Item    `json:"items,omitempty"`
	Values  []float64 `json:"values,omitempty"` // sparkline
	Caption string    `json:"caption,omitempty"`
}

// Dashboard is the plugin's page.
type Dashboard struct {
	Title   string   `json:"title,omitempty"`
	Widgets []Widget `json:"widgets"`
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
	V     int      `json:"v"`
	Ember string   `json:"ember"`
	Caps  []string `json:"caps"`
}

// Event is something Ember tells a plugin: Type is click, button, dash or
// stop, and the fields that go with it are set.
type Event struct {
	Type   string `json:"type"`
	Target string `json:"target,omitempty"` // click
	Card   string `json:"card,omitempty"`   // button
	Key    string `json:"key,omitempty"`    // button
	State  string `json:"state,omitempty"`  // dash: open | close

	Payload json.RawMessage `json:"payload,omitempty"` // tool: what the plugin's tool sent
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
func Cut(s string, n int) string {
	if !utf8.ValidString(s) {
		s = string([]rune(s)) // invalid bytes become U+FFFD
	}
	rs := []rune(s)
	out := rs[:0]
	for _, r := range rs {
		if r == '\n' || r == '\t' || r == '\r' {
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
		if b.Key, b.Label = Cut(b.Key, 32), Cut(b.Label, 24); b.Key != "" && b.Label != "" {
			out = append(out, b)
		}
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
	c.Eyebrow, c.Body, c.Mood = Cut(c.Eyebrow, 32), Cut(c.Body, 280), mood(c.Mood, Work)
	c.Buttons = buttons(c.Buttons, 3)
	switch {
	case c.TTLs == 0 && len(c.Buttons) > 0:
		c.TTLs = 180
	case c.TTLs == 0:
		c.TTLs = 7
	}
	c.TTLs = clamp(c.TTLs, 5, 600)
	return nil
}

func (d *Dashboard) fit() error {
	d.Title = Cut(d.Title, 32)
	if len(d.Widgets) > 12 {
		d.Widgets = d.Widgets[:12]
	}
	ws := d.Widgets[:0]
	for _, w := range d.Widgets {
		if w.fit() {
			ws = append(ws, w)
		}
	}
	d.Widgets = ws
	return nil
}

// fit makes a widget fit; false for a kind this version doesn't draw.
func (w *Widget) fit() bool {
	w.Title, w.Caption = Cut(w.Title, 32), Cut(w.Caption, 40)
	limit := map[string]int{"stats": 4, "bars": 12, "chips": 12, "list": 8, "sparkline": 0}
	n, ok := limit[w.Kind]
	if !ok {
		return false
	}
	if len(w.Items) > n {
		w.Items = w.Items[:n]
	}
	if len(w.Values) > 120 {
		w.Values = w.Values[len(w.Values)-120:]
	}
	for i := range w.Values {
		w.Values[i] = clamp(w.Values[i], -1e12, 1e12)
	}
	if w.Kind != "sparkline" {
		w.Values = nil
	}
	items := w.Items[:0]
	for _, it := range w.Items {
		switch w.Kind {
		case "stats":
			it.Label, it.Caption = Cut(it.Label, 20), Cut(it.Caption, 24)
			switch v := it.Value.(type) {
			case string:
				it.Text = Cut(v, 12)
			case float64:
				it.Text = Cut(fmt.Sprint(v), 12)
			}
			if it.Label == "" && it.Text == "" {
				continue
			}
		case "bars":
			it.Label, it.Caption = Cut(it.Label, 20), Cut(it.Caption, 12)
			if v, ok := it.Value.(float64); ok {
				it.Num = clamp(v, 0, 1e12)
			}
		case "chips":
			it.Label, it.State = Cut(it.Label, 4), enum(it.State, "todo", "done", "tried", "owed", "fixed", "gone", "todo")
			if it.Label == "" {
				continue
			}
		case "list":
			it.Title, it.Sub, it.Icon, it.Mood = Cut(it.Title, 40), Cut(it.Sub, 60), icon(it.Icon), mood(it.Mood, Quiet)
			if it.Title == "" {
				continue
			}
		}
		it.Value = nil
		items = append(items, it)
	}
	w.Items = items
	return true
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
