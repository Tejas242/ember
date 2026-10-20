package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sync"
)

// Plugin is a running plugin's side of the conversation: what Ember said in
// its hello, the events it sends, and the messages to send it. Its methods
// are safe to call from any goroutine.
type Plugin struct {
	Name  string   // this plugin's name, as its manifest has it
	Ember string   // Ember's version
	Caps  []string // the capabilities Ember granted

	mu     sync.Mutex
	w      io.Writer
	events chan Event
	ctx    context.Context
}

// Run is Serve on stdin and stdout: what a plugin's main calls. It returns
// when fn does, or when Ember says stop (fn's context ends; give it a
// moment to return).
//
//	func main() {
//		err := plugin.Run("pomodoro", func(p *plugin.Plugin) error { … })
//		if err != nil {
//			log.Fatal(err) // stderr is the plugin's log
//		}
//	}
func Run(name string, fn func(p *Plugin) error) error {
	return Serve(name, os.Stdin, os.Stdout, fn)
}

// Serve speaks the protocol over in and out: it waits for Ember's hello,
// answers it, then runs fn while it hands Ember's events to p.Events().
func Serve(name string, in io.Reader, out io.Writer, fn func(p *Plugin) error) error {
	if !NameRe.MatchString(name) {
		return fmt.Errorf("plugin: bad name %q", name)
	}
	r := bufio.NewReaderSize(in, 64<<10)
	var hello HelloEmber
	for { // Ember speaks first
		line, err := r.ReadBytes('\n')
		if err != nil {
			return fmt.Errorf("plugin: no hello from Ember: %w", err)
		}
		var head struct{ Type string }
		if json.Unmarshal(line, &head) == nil && head.Type == "hello" {
			if err := json.Unmarshal(line, &hello); err != nil || hello.V < 1 {
				return fmt.Errorf("plugin: a hello without a version: %s", line)
			}
			break
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &Plugin{Name: name, Ember: hello.Ember, Caps: hello.Caps, w: out, events: make(chan Event, 64), ctx: ctx}
	if err := p.send("hello", Hello{V: Version, Name: name}); err != nil {
		return err
	}
	go func() {
		defer close(p.events)
		defer cancel()
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var ev Event
			if json.Unmarshal(line, &ev) != nil {
				continue
			}
			switch ev.Type {
			case "stop":
				return
			case "click", "button", "dash":
				select {
				case p.events <- ev:
				default: // the plugin isn't reading its events: it misses this one
				}
			}
		}
	}()
	return fn(p)
}

// Events are what Ember tells the plugin: clicks, buttons, its dashboard
// opening and closing. The channel closes when Ember says stop.
func (p *Plugin) Events() <-chan Event { return p.events }

// Context ends when Ember says stop (or stops listening).
func (p *Plugin) Context() context.Context { return p.ctx }

// Can says whether Ember granted a capability. A message that needs one
// Ember didn't grant is dropped by Ember (and counted against the plugin).
func (p *Plugin) Can(capability string) bool { return slices.Contains(p.Caps, capability) }

// ErrNotGranted: the message needs a capability Ember didn't grant.
var ErrNotGranted = errors.New("plugin: capability not granted")

func (p *Plugin) send(typ string, v any) error {
	b, err := Line(typ, v)
	if err != nil {
		return err
	}
	if len(b) > MaxLine {
		return fmt.Errorf("plugin: %s message of %d bytes is over %d", typ, len(b), MaxLine)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err = p.w.Write(b)
	return err
}

func (p *Plugin) sendCap(capability, typ string, v any) error {
	if !p.Can(capability) {
		return fmt.Errorf("%w: %s needs %q", ErrNotGranted, typ, capability)
	}
	return p.send(typ, v)
}

// Status sets the plugin's line in the pill.
func (p *Plugin) Status(s Status) error { return p.sendCap(CapPill, "status", s) }

// Moment shows a short note in the pill.
func (p *Plugin) Moment(m Moment) error { return p.sendCap(CapMoment, "moment", m) }

// Row sets the plugin's row in the open card.
func (p *Plugin) Row(r Row) error { return p.sendCap(CapRow, "row", r) }

// Card shows a card with buttons (or replaces the one with its ID).
func (p *Plugin) Card(c Card) error { return p.sendCap(CapCard, "card", c) }

// Dashboard sets the plugin's dashboard.
func (p *Plugin) Dashboard(d Dashboard) error { return p.sendCap(CapDashboard, "dashboard", d) }

// Cheer asks Ember to react: "hop", "nod" or "celebrate". Ember may not.
func (p *Plugin) Cheer(kind string) error { return p.sendCap(CapCheer, "cheer", Cheer{Kind: kind}) }

// Log writes a line to the plugin's log (level info, warn or error).
func (p *Plugin) Log(level, text string) error { return p.send("log", Log{Level: level, Text: text}) }

// Logf is Log at info, formatted.
func (p *Plugin) Logf(format string, a ...any) error { return p.Log("info", fmt.Sprintf(format, a...)) }

// ClearStatus takes the pill line back.
func (p *Plugin) ClearStatus() error { return p.send("remove", Remove{What: "status"}) }

// ClearRow takes the row back.
func (p *Plugin) ClearRow() error { return p.send("remove", Remove{What: "row"}) }

// ClearDashboard takes the dashboard back.
func (p *Plugin) ClearDashboard() error { return p.send("remove", Remove{What: "dashboard"}) }

// RemoveCard takes a card back.
func (p *Plugin) RemoveCard(id string) error { return p.send("remove", Remove{What: "card", ID: id}) }

// ---- building messages ----

// Float is a pointer to v: for Status.Progress and Row.Progress.
func Float(v float64) *float64 { return &v }

// Stats is a row of big numbers.
func Stats(title string, items ...Item) Widget {
	return Widget{Kind: "stats", Title: title, Items: items}
}

// Stat is one of them: "Today" "3" "of 8".
func Stat(label, value, caption string) Item {
	return Item{Label: label, Value: value, Caption: caption}
}

// Sparkline is a line through values, oldest first.
func Sparkline(title string, values []float64, caption string) Widget {
	return Widget{Kind: "sparkline", Title: title, Values: values, Caption: caption}
}

// Bars are horizontal bars.
func Bars(title string, items ...Item) Widget {
	return Widget{Kind: "bars", Title: title, Items: items}
}

// Bar is one: a label and a value (caption, if set, is shown instead of the number).
func Bar(label string, value float64, caption string) Item {
	return Item{Label: label, Value: value, Caption: caption}
}

// Chips are letters in chips.
func Chips(title, caption string, items ...Item) Widget {
	return Widget{Kind: "chips", Title: title, Caption: caption, Items: items}
}

// Chip is one: a label of up to 4 characters and a state: done, tried, owed, fixed, gone or todo.
func Chip(label, state string) Item { return Item{Label: label, State: state} }

// List is rows of a title and words.
func List(title string, items ...Item) Widget {
	return Widget{Kind: "list", Title: title, Items: items}
}

// ListItem is one row.
func ListItem(title, sub, icon, mood string) Item {
	return Item{Title: title, Sub: sub, Icon: icon, Mood: mood}
}
