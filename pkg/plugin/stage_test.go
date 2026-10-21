package plugin

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// A stage page: the widgets for it fit their limits, one of each, the stage first.
func TestDecodeStagePage(t *testing.T) {
	m, err := Decode([]byte(`{"type":"dashboard","title":"Vibe","size":"cinema","sticky":true,"ends":{"media_idle_s":9000,"after_claude":"forever"},"widgets":[
		{"kind":"stage","ember":{"dance":"breakdance","sing":true,"pose":"proud","react":"hop"},"light":"laser","haze":"none","fold":true},
		{"kind":"stage"},
		{"kind":"sessions","style":"grid"},
		{"kind":"timed_text","media_key":"k1","mode":"karaoke","state":"weird","caption":"` + strings.Repeat("c", 90) + `",
		 "lines":[{"t_ms":5000,"text":"second"},{"t_ms":-3,"text":"first\nline","words":[{"t_ms":-9,"n":99},{"t_ms":0,"n":5}]},{"t_ms":9000,"text":""}]},
		{"kind":"timed_text","media_key":"k2"},
		{"kind":"audio","tint":"neon"},
		{"kind":"countdown","title":"Ad","text":"Back to the music","to":"forever"},
		{"kind":"media"},
		{"kind":"columns","a":{"kind":"timed_text"},"b":{"kind":"note","text":"x"}}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	d := m.Dashboard
	if d.Size != "page" || !d.Sticky || d.Ends == nil || d.Ends.MediaIdleS != 600 || d.Ends.AfterClaude != "" {
		t.Fatalf("page fields: %+v %+v", d, d.Ends)
	}
	kinds := []string{}
	for _, w := range d.Widgets {
		kinds = append(kinds, w.Kind)
	}
	if strings.Join(kinds, " ") != "stage sessions timed_text audio countdown media" {
		t.Fatalf("kinds %v: a second stage, a second timed text and stage widgets in columns go", kinds)
	}
	st := d.Widgets[0]
	if st.Ember.Dance != "music" || !st.Ember.Sing || st.Ember.Pose != "calm" || st.Ember.React != "" || st.Light != "audio" || st.Haze != "none" || st.Folded != "no" {
		t.Fatalf("stage %+v %+v", st, st.Ember)
	}
	if s := d.Widgets[1]; s.Style != "line" {
		t.Fatalf("sessions %+v", s)
	}
	tt := d.Widgets[2]
	if tt.Mode != "synced" || tt.State != "ready" || utf8.RuneCountInString(tt.Caption) != 60 || tt.MediaKey != "k1" {
		t.Fatalf("timed text %+v", tt)
	}
	if len(tt.Lines) != 3 || tt.Lines[0].Text != "first line" || tt.Lines[0].TMs != 0 || tt.Lines[1].Text != "second" || tt.Lines[2].Text != "" {
		t.Fatalf("lines not sorted and clamped: %+v", tt.Lines)
	}
	if w := tt.Lines[0].Words; len(w) != 2 || w[0].TMs != 0 || w[0].N != 10 {
		t.Fatalf("words %+v", w)
	}
	if a := d.Widgets[3]; a.Style != "ribbon" || a.Tint != "cover" {
		t.Fatalf("audio %+v", a)
	}
	if c := d.Widgets[4]; c.To != "media_end" || c.Text != "Back to the music" || c.Title != "Ad" {
		t.Fatalf("countdown %+v", c)
	}
	if !d.Timed() {
		t.Fatal("a page with timed text is timed")
	}

	m, _ = Decode([]byte(`{"type":"dashboard","size":"card","ends":{"after_claude":"song_end"},"widgets":[{"kind":"note","text":"x"},{"kind":"stage"},{"kind":"hero","ember":{"dance":"music","sing":true}}]}`))
	d = m.Dashboard
	if d.Size != "card" || d.Ends.AfterClaude != "song_end" || len(d.Widgets) != 1 || d.Timed() {
		t.Fatalf("a stage after the first widget goes: %+v", d)
	}
	m, _ = Decode([]byte(`{"type":"dashboard","widgets":[{"kind":"hero","ember":{"dance":"music","sing":true},"light":"audio","lines":[{"t_ms":1,"text":"x"}],"style":"line"}]}`))
	if h := m.Dashboard.Widgets[0]; h.Ember.Dance != "" || h.Ember.Sing || h.Light != "" || h.Lines != nil || h.Style != "" {
		t.Fatalf("a hero keeps no stage fields: %+v %+v", h, h.Ember)
	}
	m, _ = Decode([]byte(`{"type":"dashboard","ends":{"media_idle_s":-4},"widgets":[]}`))
	if m.Dashboard.Ends != nil {
		t.Fatal("ends that end nothing are dropped")
	}
}

// Timed text keeps to 400 lines, 160 characters a line and 60 KiB of words.
func TestTimedTextLimits(t *testing.T) {
	var ls []TimedLine
	for i := 0; i < 500; i++ {
		ls = append(ls, TimedLine{TMs: int64(500 - i), Text: strings.Repeat("w", 200)})
	}
	w := Widget{Kind: "timed_text", Lines: ls, Clock: &Clock{PosMs: -1, Rate: 99}}
	if !w.fit(true) {
		t.Fatal("dropped")
	}
	if len(w.Lines) > MaxTimedLines || w.Clock.Rate != 4 || w.Clock.PosMs != 0 {
		t.Fatalf("%d lines, clock %+v", len(w.Lines), w.Clock)
	}
	total := 0
	for i, l := range w.Lines {
		total += len(l.Text)
		if utf8.RuneCountInString(l.Text) > 160 || (i > 0 && l.TMs < w.Lines[i-1].TMs) {
			t.Fatalf("line %d: %+v", i, l)
		}
	}
	if total > MaxTimedText {
		t.Fatalf("%d bytes of words", total)
	}
	w = Widget{Kind: "timed_text", Clock: &Clock{}}
	w.fit(true)
	if w.Clock.Rate != 1 {
		t.Fatal("a clock runs at 1 by default")
	}
}

// Events both ways: a media event carries the song, and a button's key isn't the media's.
func TestMediaEvent(t *testing.T) {
	ev := Event{Type: "media", Media: &Media{Key: "k", Player: "p", Title: "T", LengthMs: 1000, PosMs: 10, AtMs: 5, Playing: true}}
	b, err := Line("media", ev)
	if err != nil {
		t.Fatal(err)
	}
	var back Event
	if err := json.Unmarshal(b, &back); err != nil || back.Media == nil || back.Media.Key != "k" || back.Key != "" || !back.Playing || back.LengthMs != 1000 {
		t.Fatalf("%s → %+v %v", b, back, err)
	}
	b, _ = Line("button", Event{Type: "button", Card: "c", Key: "go"})
	back = Event{}
	json.Unmarshal(b, &back)
	if back.Key != "go" || back.Media != nil {
		t.Fatalf("a button: %s → %+v", b, back)
	}
	b, _ = Line("media", Event{Type: "media"})
	if string(b) != `{"type":"media"}`+"\n" {
		t.Fatalf("nothing playing: %s", b)
	}
	b, _ = Line("dash", Event{Type: "dash", State: "close", Reason: CloseSongEnd})
	if !strings.Contains(string(b), `"reason":"song_end"`) {
		t.Fatalf("%s", b)
	}
}

// The SDK hands media events to the plugin, and knows its grants.
func TestHarnessMedia(t *testing.T) {
	got := make(chan Event, 4)
	h := NewHarnessWith("vibe", Caps, []string{GrantMedia}, func(p *Plugin) error {
		if !p.Granted(GrantMedia) || p.Granted(GrantEntry) {
			t.Error("grants")
		}
		for ev := range p.Events() {
			got <- ev
		}
		return nil
	})
	h.Media(&Media{Key: "k", Title: "T"})
	h.DashClosed(CloseUser)
	if ev := <-got; ev.Type != "media" || ev.Media == nil || ev.Title != "T" {
		t.Fatalf("%+v", ev)
	}
	if ev := <-got; ev.Type != "dash" || ev.Reason != CloseUser {
		t.Fatalf("%+v", ev)
	}
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
}
