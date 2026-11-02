package adapter

import (
	"testing"
	"time"
)

// Past the day's goal (in the trials: the rep), the problem he's still on
// keeps its controls on the dashboard: Hint, Give up, its clock.
func TestDoneKeepsTheTaskOnTheDashboard(t *testing.T) {
	st := &Stats{Version: 1}
	st.Today.Got, st.Today.Need, st.Today.Done, st.Today.DayEnds, st.Today.StudyFrom = 1, 1, true, "04:00", "19:00"
	st.Current = &Task{ID: "cf:1418C", Source: "cf", Kind: "new", Title: "Mortal Kombat Tower", Started: clock.Add(-30 * time.Minute), HintsAllowed: 1}
	e := practising()
	e.Today, e.Need, e.TaskID, e.Task, e.Kind, e.GiveUpInSec, e.HintsReady, e.RepDone = 1, 1, "cf:1418C", "1418C · Mortal Kombat Tower", "new", 0, true, true
	for _, ph := range []string{"baseline", "trial1"} {
		e.Phase, st.Phase = ph, ph
		h := DashboardFor(st, e, clock).Widgets[0]
		var keys []string
		for _, b := range h.Buttons {
			keys = append(keys, b.Key)
		}
		if h.Step == nil || h.Step.Text != "1418C · Mortal Kombat Tower" || len(keys) != 2 || keys[1] != "hint:cf:1418C" || keys[0] != "giveup:cf:1418C" {
			t.Fatalf("%s: the task is gone from the hero: %q %+v %v", ph, h.Title, h.Step, keys)
		}
	}
	// not started yet, nothing else waiting: offered as one more
	e = &Status{Today: 1, Need: 1, TaskID: "cf:1418C", Task: "1418C · Mortal Kombat Tower", Kind: "new", Pending: true, GiveUpInSec: -1}
	st.Current.Started = time.Time{}
	if h := DashboardFor(st, e, clock).Widgets[0]; len(h.Buttons) == 0 || h.Buttons[len(h.Buttons)-1].Key != "start:cf:1418C" {
		t.Fatalf("pending after the goal: %+v", h.Buttons)
	}
	// an explain-back still asks first
	e.Explain = "cf:1975B"
	if h := DashboardFor(st, e, clock).Widgets[0]; h.Buttons[0].Key != "explain:cf:1975B" {
		t.Fatalf("explain-back first: %+v", h.Buttons)
	}
}
