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
