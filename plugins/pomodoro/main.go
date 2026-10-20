// Pomodoro is an example Ember plugin: focus for a while, then a break.
// The pill counts the session down, the card's row shows where you are (a
// click on it starts a session), a card offers the break and the next
// session, and the dashboard has today's sessions. It uses pkg/plugin.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

func main() {
	focus := flag.Duration("focus", 25*time.Minute, "a focus session")
	rest := flag.Duration("break", 5*time.Minute, "a break")
	goal := flag.Int("goal", 8, "sessions a day")
	flag.Parse()
	if err := plugin.Run("pomodoro", func(p *plugin.Plugin) error {
		return (&pomodoro{p: p, focus: *focus, rest: *rest, goal: *goal, now: time.Now}).run()
	}); err != nil {
		log.Fatal(err)
	}
}

type pomodoro struct {
	p           *plugin.Plugin
	focus, rest time.Duration
	goal        int
	now         func() time.Time

	phase string    // idle | focus | offer (focus done, a break offered) | break | ready (break done)
	end   time.Time // when the focus or the break ends
	done  []time.Time
}

func (s *pomodoro) run() error {
	s.idle()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case ev, ok := <-s.p.Events():
			if !ok { // Ember said stop
				return nil
			}
			s.event(ev)
		case <-tick.C:
			s.tick()
		}
	}
}

func (s *pomodoro) event(ev plugin.Event) {
	switch {
	case ev.Type == "click" && (s.phase == "idle" || s.phase == "ready"):
		s.start()
	case ev.Type == "button" && ev.Key == "break":
		s.takeBreak()
	case ev.Type == "button" && (ev.Key == "start" || ev.Key == "skip"):
		s.start()
	case ev.Type == "button" && ev.Key == "later":
		s.idle()
	case ev.Type == "dash" && ev.State == "open":
		s.dashboard()
	}
}

func (s *pomodoro) tick() {
	if s.end.IsZero() {
		return
	}
	now := s.now()
	if now.Before(s.end) {
		if s.phase == "focus" && now.Second() == 0 { // the row's bar, a minute at a time
			s.row()
		}
		return
	}
	switch s.phase {
	case "focus":
		s.finished(now)
	case "break":
		s.breakOver()
	}
}

func (s *pomodoro) today() int {
	n, now := 0, s.now()
	for _, t := range s.done {
		if t.YearDay() == now.YearDay() && t.Year() == now.Year() {
			n++
		}
	}
	return n
}

func (s *pomodoro) idle() {
	s.phase, s.end = "idle", time.Time{}
	s.p.ClearStatus()
	s.row()
}

func (s *pomodoro) start() {
	s.p.RemoveCard("next")
	s.p.RemoveCard("break")
	s.phase, s.end = "focus", s.now().Add(s.focus)
	s.p.Status(plugin.Status{Text: "Focus", Sub: fmt.Sprintf("session %d", s.today()+1), Icon: "timer", Mood: plugin.Work,
		UntilMs: s.end.UnixMilli(), Urgency: "low"})
	s.row()
}

func (s *pomodoro) finished(now time.Time) {
	s.done = append(s.done, now)
	s.phase, s.end = "offer", time.Time{}
	n := s.today()
	s.p.Moment(plugin.Moment{Text: "Focus done", Sub: fmt.Sprintf("%d of %d today", n, s.goal), Icon: "check", Mood: plugin.Done})
	s.p.Status(plugin.Status{Text: "Break?", Icon: "timer", Mood: plugin.Wait, Urgency: "normal"})
	s.p.Card(plugin.Card{ID: "break", Eyebrow: "Pomodoro", Title: "Take a break?",
		Body: fmt.Sprintf("%s of focus done. A few minutes away from the screen help the next one.", short(s.focus)),
		Mood: plugin.Done, Buttons: []plugin.Button{{Key: "skip", Label: "Keep going"}, {Key: "break", Label: "Break", Primary: true}}})
	if n == s.goal {
		s.p.Cheer("celebrate")
	}
	s.row()
	s.dashboard()
}

func (s *pomodoro) takeBreak() {
	s.phase, s.end = "break", s.now().Add(s.rest)
	s.p.Status(plugin.Status{Text: "Break", Icon: "timer", Mood: plugin.Quiet, UntilMs: s.end.UnixMilli(), Urgency: "low"})
	s.row()
}

func (s *pomodoro) breakOver() {
	s.phase, s.end = "ready", time.Time{}
	s.p.ClearStatus()
	s.p.Moment(plugin.Moment{Text: "Break's over", Icon: "timer", Mood: plugin.Quiet})
	s.p.Card(plugin.Card{ID: "next", Eyebrow: "Pomodoro", Title: "Ready for another?", Mood: plugin.Work,
		Buttons: []plugin.Button{{Key: "later", Label: "Not now"}, {Key: "start", Label: "Start", Primary: true}}})
	s.row()
}

// row is where you are, in the card.
func (s *pomodoro) row() {
	r := plugin.Row{Title: "Pomodoro", Icon: "timer", Mood: plugin.Quiet, Sub: fmt.Sprintf("%d of %d today", s.today(), s.goal)}
	switch s.phase {
	case "idle", "ready":
		if s.today() == 0 {
			r.Sub = "click to focus for " + short(s.focus)
		}
	case "focus":
		left := s.end.Sub(s.now())
		r.Title, r.Mood, r.Sub = "Focus", plugin.Work, fmt.Sprintf("%s left · %d of %d today", short(left), s.today(), s.goal)
		r.Progress = plugin.Float(1 - left.Seconds()/s.focus.Seconds())
	case "offer":
		r.Title, r.Mood = "Focus done", plugin.Done
	case "break":
		r.Title = "Break"
	}
	s.p.Row(r)
}

// dashboard is today's sessions.
func (s *pomodoro) dashboard() {
	if len(s.done) == 0 {
		return
	}
	var chips []plugin.Item
	for i := 1; i <= s.goal; i++ {
		st := "todo"
		if i <= s.today() {
			st = "done"
		}
		chips = append(chips, plugin.Chip(fmt.Sprint(i), st))
	}
	var recent []plugin.Item
	for i := len(s.done) - 1; i >= 0 && len(recent) < 5; i-- {
		recent = append(recent, plugin.ListItem("Focus", short(s.focus)+" · "+s.done[i].Format("15:04"), "check", plugin.Done))
	}
	s.p.Dashboard(plugin.Dashboard{Title: "Pomodoro", Widgets: []plugin.Widget{
		plugin.Stats("", plugin.Stat("Today", fmt.Sprint(s.today()), fmt.Sprintf("of %d", s.goal)),
			plugin.Stat("Focused", short(time.Duration(s.today())*s.focus), "")),
		plugin.Chips("Sessions", "", chips...),
		plugin.List("Recent", recent...),
	}})
}

func short(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}
