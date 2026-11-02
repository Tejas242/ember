package adapter

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// edge says Accepted and the next problem in the same second; Ember takes
// one new card a second and keeps three open, dropping the rest unseen. The
// plugin spaces them, and the fourth open card takes the oldest's place.
func TestCardsWaitTheirTurn(t *testing.T) {
	run := func(p *plugin.Plugin) error {
		return Run(p, Options{StatsPath: filepath.Join(t.TempDir(), "none.json"), Now: func() time.Time { return clock }, Action: func(string) error { return nil }})
	}
	h := plugin.NewHarness("edge", plugin.Caps, run)
	defer h.Stop()
	card := func(id string) {
		raw, _ := json.Marshal(map[string]any{"type": "card", "card": map[string]any{"id": id, "title": id,
			"buttons": []map[string]any{{"key": "start", "label": "Start"}}}})
		h.Send(plugin.Event{Type: "tool", Payload: raw})
	}
	t0 := time.Now()
	for _, id := range []string{"ac", "offer", "health"} {
		card(id)
	}
	var at []time.Duration
	for _, id := range []string{"ac", "offer", "health"} {
		if _, err := h.WaitFor("card", 4*time.Second, func(m plugin.Message) bool { return m.Card.ID == id }); err != nil {
			t.Fatalf("%s never shown: %v", id, err)
		}
		at = append(at, time.Since(t0))
	}
	if at[1]-at[0] < time.Second || at[2]-at[1] < time.Second {
		t.Fatalf("cards %v apart: Ember would drop them", at)
	}
	card("hint") // a fourth open card: the oldest (ac) comes down first
	if _, err := h.WaitFor("card", 3*time.Second, func(m plugin.Message) bool { return m.Card.ID == "hint" }); err != nil {
		t.Fatal(err)
	}
	removed := false
	for _, m := range h.Messages() {
		if m.Type == "remove" && m.Remove != nil && m.Remove.ID == "ac" {
			removed = true
		}
	}
	if !removed {
		t.Fatal("a fourth open card without taking the oldest down")
	}
	// a card taken back before its turn never shows
	card("drift")
	raw, _ := json.Marshal(map[string]any{"type": "remove", "id": "drift"})
	h.Send(plugin.Event{Type: "tool", Payload: raw})
	time.Sleep(1500 * time.Millisecond)
	for _, m := range h.Messages() {
		if m.Type == "card" && m.Card.ID == "drift" {
			t.Fatal("a card removed while queued was shown")
		}
	}
}
