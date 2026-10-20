package main

import (
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// A whole session through the harness: start from the row, the countdown,
// the end, the break offered and taken, the next one offered.
func TestPomodoro(t *testing.T) {
	h := plugin.NewHarness("pomodoro", plugin.Caps, func(p *plugin.Plugin) error {
		return (&pomodoro{p: p, focus: 1500 * time.Millisecond, rest: 1500 * time.Millisecond, goal: 1, now: time.Now}).run()
	})
	if r, err := h.Wait("row", time.Second); err != nil || r.Row.Sub != "click to focus for 0 min" {
		t.Fatalf("%v %+v", err, r.Row)
	}
	h.Click("row")
	st, err := h.WaitFor("status", time.Second, func(m plugin.Message) bool { return m.Status.Text == "Focus" })
	if err != nil || st.Status.UntilMs == 0 || st.Status.Icon != "timer" {
		t.Fatalf("%v %+v", err, st.Status)
	}
	if _, err := h.WaitFor("moment", 4*time.Second, func(m plugin.Message) bool { return m.Moment.Text == "Focus done" }); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Wait("cheer", time.Second); err != nil {
		t.Fatal("the day's goal reached: a celebration", err)
	}
	if _, err := h.Wait("dashboard", time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := h.WaitFor("card", time.Second, func(m plugin.Message) bool { return m.Card.ID == "break" }); err != nil {
		t.Fatal(err)
	}
	h.Button("break", "break")
	if _, err := h.WaitFor("status", time.Second, func(m plugin.Message) bool { return m.Status.Text == "Break" }); err != nil {
		t.Fatal(err)
	}
	if _, err := h.WaitFor("card", 4*time.Second, func(m plugin.Message) bool { return m.Card.ID == "next" }); err != nil {
		t.Fatal(err)
	}
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	if len(h.Errors()) > 0 {
		t.Fatal(h.Errors())
	}
}
