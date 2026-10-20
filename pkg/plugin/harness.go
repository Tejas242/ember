package plugin

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"
)

// Harness is a fake Ember for testing a plugin written with Serve: it
// says hello, checks every line the plugin writes the way Ember does
// (Decode, the granted capabilities), keeps the messages, and sends events.
//
//	h := plugin.NewHarness("pomodoro", plugin.Caps, run)
//	defer h.Stop()
//	st, err := h.Wait("status", time.Second)
type Harness struct {
	Name string
	Caps []string

	in   *io.PipeWriter // the plugin's stdin
	mu   sync.Mutex
	got  []Message
	bad  []error
	new  chan struct{}
	end  chan error
	read chan struct{} // closed when the plugin's output has all been read
}

// NewHarness starts fn as plugin name, granted caps.
func NewHarness(name string, caps []string, fn func(p *Plugin) error) *Harness {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h := &Harness{Name: name, Caps: caps, in: inW, new: make(chan struct{}, 1), end: make(chan error, 1), read: make(chan struct{})}
	go func() {
		err := Serve(name, inR, outW, fn)
		outW.Close()
		h.end <- err
	}()
	go h.readAll(outR)
	b, _ := Line("hello", HelloEmber{V: Version, Ember: "test", Caps: caps})
	go inW.Write(b)
	return h
}

func (h *Harness) readAll(r io.Reader) {
	defer close(h.read)
	br := bufio.NewReaderSize(r, MaxLine+1)
	said := false
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		m, err := Decode(line)
		h.mu.Lock()
		switch {
		case errors.Is(err, ErrIgnored):
		case err != nil:
			h.bad = append(h.bad, err)
		case !said:
			if m.Type != "hello" || m.Hello.Name != h.Name {
				h.bad = append(h.bad, fmt.Errorf("first line isn't its hello: %s", line))
			}
			said = true
		case m.Cap() != "" && !slices.Contains(h.Caps, m.Cap()):
			h.bad = append(h.bad, fmt.Errorf("%s without the %q capability", m.Type, m.Cap()))
		default:
			h.got = append(h.got, m)
		}
		h.mu.Unlock()
		select {
		case h.new <- struct{}{}:
		default:
		}
	}
}

// Messages is everything the plugin sent (after its hello) that Ember would take.
func (h *Harness) Messages() []Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Message(nil), h.got...)
}

// Errors are the lines Ember would have refused (violations).
func (h *Harness) Errors() []error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]error(nil), h.bad...)
}

// Last is the plugin's latest message of a type, if any.
func (h *Harness) Last(typ string) (Message, bool) {
	ms := h.Messages()
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Type == typ {
			return ms[i], true
		}
	}
	return Message{}, false
}

// Wait waits up to timeout for a message of a type: the first one, which may
// have come already.
func (h *Harness) Wait(typ string, timeout time.Duration) (Message, error) {
	return h.WaitFor(typ, timeout, func(Message) bool { return true })
}

// WaitFor waits for a message of a type that ok accepts.
func (h *Harness) WaitFor(typ string, timeout time.Duration, ok func(Message) bool) (Message, error) {
	deadline := time.After(timeout)
	for {
		for _, m := range h.Messages() {
			if m.Type == typ && ok(m) {
				return m, nil
			}
		}
		select {
		case <-h.new:
		case <-deadline:
			return Message{}, fmt.Errorf("no %s within %v (got %d messages, %d refused: %v)", typ, timeout, len(h.Messages()), len(h.Errors()), h.Errors())
		}
	}
}

// Send tells the plugin about an event.
func (h *Harness) Send(ev Event) error {
	b, err := Line(ev.Type, ev)
	if err != nil {
		return err
	}
	_, err = h.in.Write(b)
	return err
}

// Click, Button and Dash send the plugin those events.
func (h *Harness) Click(target string) error { return h.Send(Event{Type: "click", Target: target}) }

// Button sends a button pick on a card.
func (h *Harness) Button(card, key string) error {
	return h.Send(Event{Type: "button", Card: card, Key: key})
}

// Dash sends its dashboard opening (true) or closing.
func (h *Harness) Dash(open bool) error {
	st := "close"
	if open {
		st = "open"
	}
	return h.Send(Event{Type: "dash", State: st})
}

// Stop says stop, as Ember does when it stops, and waits up to 2 seconds
// for the plugin to return: what it returned, or an error if it didn't.
func (h *Harness) Stop() error {
	b, _ := Line("stop", Event{Type: "stop"})
	go func() {
		h.in.Write(b)
		h.in.Close()
	}()
	select {
	case err := <-h.end:
		<-h.read // everything it wrote before returning counts
		return err
	case <-time.After(2 * time.Second):
		return errors.New("plugin: didn't stop within 2 seconds of being told to")
	}
}
