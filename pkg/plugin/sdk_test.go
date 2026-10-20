package plugin_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// A plugin written with the SDK, round trip through the harness: it shows
// its things, hears events, answers them, and stops when told.
func TestSDKRoundTrip(t *testing.T) {
	run := func(p *plugin.Plugin) error {
		if p.Ember != "test" || !p.Can(plugin.CapPill) || p.Can(plugin.CapCheer) {
			return errors.New("hello not understood")
		}
		if err := p.Cheer("hop"); !errors.Is(err, plugin.ErrNotGranted) {
			return errors.New("a cheer without the capability must not go")
		}
		p.Status(plugin.Status{Text: "Focus", Icon: "timer", Mood: plugin.Work, UntilMs: time.Now().Add(25 * time.Minute).UnixMilli(), Progress: plugin.Float(0)})
		p.Row(plugin.Row{Title: "Focus", Sub: "25 min", Progress: plugin.Float(0.5)})
		p.Dashboard(plugin.Dashboard{Title: "Pomodoro", Widgets: []plugin.Widget{
			plugin.Stats("Today", plugin.Stat("Done", "3", "of 8")),
			plugin.Sparkline("Week", []float64{1, 2, 3}, "a day"),
			plugin.Bars("Projects", plugin.Bar("ember", 5, "")),
			plugin.Chips("Today", "", plugin.ChipItem("1", "done"), plugin.ChipItem("2", "todo")),
			plugin.List("Recent", plugin.ListItem("Plugin host", "25 min", "check", plugin.Done)),
		}})
		p.Card(plugin.Card{ID: "break", Title: "Break?", Buttons: []plugin.Button{{Key: "yes", Label: "Yes", Primary: true}}})
		for ev := range p.Events() {
			switch ev.Type {
			case "button":
				p.RemoveCard(ev.Card)
				p.Moment(plugin.Moment{Text: "Break time", Mood: plugin.Done})
			case "click":
				p.Logf("clicked %s", ev.Target)
			case "dash":
				p.Logf("dash %s", ev.State)
			}
		}
		p.ClearStatus()
		return nil
	}
	h := plugin.NewHarness("pomodoro", []string{"pill", "row", "moment", "card", "dashboard"}, run)
	st, err := h.Wait("status", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status.Text != "Focus" || st.Status.UntilMs == 0 || *st.Status.Progress != 0 {
		t.Fatalf("%+v", st.Status)
	}
	d, err := h.Wait("dashboard", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	w := d.Dashboard.Widgets
	if len(w) != 5 || w[0].Items[0].Text != "3" || w[2].Items[0].Num != 5 || w[3].Items[0].State != "done" || w[4].Items[0].Title != "Plugin host" {
		t.Fatalf("%+v", w)
	}
	if _, err := h.Wait("card", time.Second); err != nil {
		t.Fatal(err)
	}
	h.Button("break", "yes")
	h.Click("row")
	h.Dash(true)
	if _, err := h.Wait("moment", time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := h.WaitFor("log", time.Second, func(m plugin.Message) bool { return m.Log.Text == "dash open" }); err != nil {
		t.Fatal(err)
	}
	if _, err := h.WaitFor("remove", time.Second, func(m plugin.Message) bool { return m.Remove.ID == "break" }); err != nil {
		t.Fatal(err)
	}
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	if m, ok := h.Last("remove"); !ok || m.Remove.What != "status" {
		t.Fatal("it clears its line on stop")
	}
	if len(h.Errors()) != 0 {
		t.Fatal(h.Errors())
	}
}

func TestHarnessCatchesViolations(t *testing.T) {
	h := plugin.NewHarness("p", []string{"pill"}, func(p *plugin.Plugin) error {
		p.Log("info", "hi")
		p.Caps = append(p.Caps, plugin.CapRow) // cheating: Ember still refuses it
		p.Row(plugin.Row{Title: "x"})
		<-p.Context().Done()
		return nil
	})
	if _, err := h.Wait("log", time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if len(h.Errors()) != 1 {
		t.Fatalf("%v", h.Errors())
	}
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
}
