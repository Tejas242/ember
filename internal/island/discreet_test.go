package island

import (
	"image"
	"testing"
	"time"
)

// While the screen is shared, Ember is discreet: Done without Claude's
// words, no screenshot preview, no card on hover; requests still show.
func TestDiscreet(t *testing.T) {
	clock := time.Date(2026, 10, 20, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	u := a.ui
	done := moment{key: "done:k", act: "claude", mood: moodHappy, verb: "Done", text: "kinvault",
		sub: "All 42 tests pass under -race", until: clock.Add(time.Hour)}
	u.Moment(done)
	if l := u.mainLine(); l.sub == "" {
		t.Fatal("not shared: Claude's words should show")
	}

	a.castsChanged(1)
	if !u.discreet() {
		t.Fatal("sharing, yet not discreet")
	}
	u.Moment(done) // after "Sharing your screen"
	if l := u.mainLine(); l.sub != "" || l.verb != "Done" || l.text != "kinvault" {
		t.Fatalf("shared: %q %q %q", l.verb, l.text, l.sub)
	}
	u.Moment(moment{key: "shot", icon: "camera", text: "Screenshot saved", thumb: image.NewRGBA(image.Rect(0, 0, 30, 18)), until: clock.Add(time.Hour)})
	if l := u.mainLine(); l.m == nil || l.m.thumb != nil || l.m.icon != "camera" {
		t.Fatal("shared: a screenshot's preview shows")
	}
	u.moment = nil

	u.hover, u.hoverAt = true, clock.Add(-time.Second)
	if u.currentView() != vCompact {
		t.Fatal("shared: the card opened on hover")
	}
	u.pinned = true // a click
	if u.currentView() != vExpanded {
		t.Fatal("shared: a click doesn't open the card")
	}
	u.pinned = false
	u.AddCard(&Card{ID: "p", Kind: "perm", Mood: moodWait, Title: "Run the tests", Expires: clock.Add(time.Minute)})
	if u.currentView() != vCard {
		t.Fatal("shared: a request doesn't show")
	}
	u.cards = nil

	// the dot off doesn't make it less discreet; discreet off does
	u.cfg.Privacy = false
	a.castsChanged(0)
	a.castsChanged(1)
	if u.casting || !u.discreet() {
		t.Fatal("dot off: still discreet, without the dot")
	}
	u.cfg.Discreet = false
	if u.discreet() || u.currentView() != vExpanded {
		t.Fatal("discreet = false: hover opens the card again")
	}
}
