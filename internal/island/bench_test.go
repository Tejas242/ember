package island

import (
	"testing"
	"time"
)

func benchUI(b *testing.B, step time.Duration, setup func(*UI)) {
	u := NewUI(440, 280, 24)
	clock := time.Now()
	u.now = func() time.Time { return clock }
	setup(u)
	for i := 0; i < 120; i++ {
		clock = clock.Add(time.Second / 60)
		u.renderDamage()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clock = clock.Add(step)
		u.renderDamage()
	}
}

func working(u *UI) {
	u.sessions["a"] = &Session{ID: "a", Project: "edge", State: "work", Label: "Editing quota.go", Plus: 12, Minus: 3}
}

// steady frames while Claude works: only Ember's square is redrawn
func BenchmarkFlickerFrame(b *testing.B) { benchUI(b, time.Second/12, working) }

// a full repaint (state changed)
func BenchmarkFullCard(b *testing.B) {
	benchUI(b, time.Second/60, func(u *UI) {
		u.AddCard(&Card{ID: "p", Kind: "perm", Mood: moodWait, Title: "Run this command?", Meta: "kinvault", Body: "Run tests",
			Code: "go test -race ./...", Buttons: []Button{{Key: "deny", Label: "Deny"}, {Key: "allow", Label: "Allow", Primary: true}}})
		u.dirty = true
	})
}

// benchRepaint repaints every frame, as when something in the view changes.
func benchRepaint(b *testing.B, setup func(*UI)) {
	u := NewUI(440, 280, 24)
	clock := time.Now()
	u.now = func() time.Time { return clock }
	setup(u)
	for i := 0; i < 120; i++ {
		clock = clock.Add(time.Second / 60)
		u.renderDamage()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clock = clock.Add(time.Second / 60)
		u.Invalidate()
		u.renderDamage()
	}
}

func choicesCard(chips bool) func(*UI) {
	return func(u *UI) {
		c := &Card{ID: "x:edge:cue", Kind: "plugin", Mood: moodNudge, Eyebrow: "edge", Title: "Tonight's rep: LC 2779",
			Body: "Sliding window. When?", Buttons: []Button{{Key: "open", Label: "Open"}}}
		if chips {
			c.Chips = []Button{{Key: "cue:now", Label: "Now", Primary: true}, {Key: "cue:19:30", Label: "19:30"}, {Key: "cue:21:00", Label: "21:00"},
				{Key: "skiptonight", Label: "Skip (1 left)"}, {Key: "a", Label: "Another choice"}, {Key: "b", Label: "And one more"}}
		}
		u.AddCard(c)
	}
}

// a plugin's card repainted, with six choice chips and without: the chips' own cost
func BenchmarkCardChipsFull(b *testing.B) { benchRepaint(b, choicesCard(true)) }
func BenchmarkCardPlainFull(b *testing.B) { benchRepaint(b, choicesCard(false)) }

// the card with chips at rest: nothing to redraw
func BenchmarkCardChipsSteady(b *testing.B) { benchUI(b, time.Second/12, choicesCard(true)) }
