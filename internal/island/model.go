package island

import (
	"image"
	"image/color"
	"time"
)

// The island's model: the cards that need a decision, the Claude sessions,
// the moments in the pill.

type Button struct {
	Key, Label      string
	Primary, Danger bool
}

// Card is something that needs a decision: a plugin's offer.
type Card struct {
	ID       string
	Kind     string // plugin
	Mood     mood
	Eyebrow  string // who wants what, in small capitals: "Practice"
	Title    string
	Meta     string
	Body     string
	Plus     int
	Minus    int
	Buttons  []Button
	Chips    []Button // small choice chips under the body: a press picks the chip's key, like a button
	Expires  time.Time
	Priority int
	OnPick   func(key string)
	OnExpire func()
	Born     time.Time // when it appeared: the countdown line starts here

	chipKeys []string // the chips' hit keys, "chip:"+key
}

type Session struct {
	ID, Project, State, Label string               // State: work | wait | idle
	Title                     string               // the topic Claude put in its terminal's title
	Prompt                    string               // what you asked this turn, first line
	cheer                     cheer                // how Ember will celebrate this turn's Done
	Agents                    map[string]*Agent    // subagents running, by agent_id
	ended                     map[string]time.Time // agents that stopped before their start arrived
	Since, Seen               time.Time
	Pids                      []int
	Outcome                   *Outcome  `json:"Recap"` // how the last turn ended (the old key: sessions saved before 1.3 still load)
	Unseen                    time.Time // it finished or stopped while you weren't looking at it
	Asking                    string    // what it waits on you for: the permission or question the terminal shows
}

// Outcome is how a turn ended: done, or stopped on an error. That is all
// Ember keeps of it; the words are in the terminal.
type Outcome struct {
	At     time.Time `json:"at"`
	Failed bool      `json:"failed,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// moment is a short-lived compact line: Claude finished, a new track, the
// volume, a charger plugged in, a screenshot.
type moment struct {
	key   string
	act   string
	mood  mood
	verb  string // a word before the text in the mood's colour: Done, Stopped
	text  string
	sub   string
	until time.Time
	focus string   // session to jump to on click
	open  string   // file to open on click
	many  []string // the sessions a "Done" names

	icon  string      // drawn where Ember sits (Ember tucks away)
	tint  color.NRGBA // the icon's colour
	right string      // a value at the right end: "73%", "45"
	meter bool        // a level bar
	level float64     // 0..1
	batt  bool        // a battery glyph
	pct   float64     // its fill, 0..1
	thumb *image.RGBA // a screenshot preview instead of the icon
	osd   bool        // you just pressed a key: shows over everything, a strip in an open view
	held  bool        // news that arrived during an OSD: it waits under it, and Ember answers it when it shows
	at    time.Time   // when it appeared
	cheer cheer       // a Done's celebration: how big, earned, an eye-roll first
	ext   string      // a plugin's: "<name>\x00<target>", for a click
}
