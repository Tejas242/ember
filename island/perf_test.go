package island

import (
	"testing"
	"time"
)

// Frame costs for the states that matter. "full" repaints everything (a
// morph, a hover, a change); "steady" repaints only the live parts.

func briefApp(clock *time.Time) *app {
	a := newTestApp(clock)
	hookAt(a, "UserPromptSubmit", "s", "kinvault", map[string]any{"prompt": "fix the race in the store's compaction path"})
	for i := 0; i < 5; i++ {
		*clock = clock.Add(30 * time.Second)
		hookAt(a, "PreToolUse", "s", "kinvault", map[string]any{"tool_name": "Edit", "tool_input": map[string]any{
			"file_path": "/x/quota.go", "old_string": "a", "new_string": "a\nb"}})
	}
	a.ui.track = &Track{Player: "p", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: 5 * time.Minute, PosAt: *clock}
	return a
}

func benchFrames(b *testing.B, expanded, full bool) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := briefApp(&clock)
	u := a.ui
	u.pinned = expanded
	for i := 0; i < 120; i++ {
		clock = clock.Add(time.Second / 60)
		u.renderDamage()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clock = clock.Add(time.Second / 60)
		if full {
			u.dirty = true
		}
		u.renderDamage()
	}
}

func BenchmarkPillFull(b *testing.B)    { benchFrames(b, false, true) }
func BenchmarkPillSteady(b *testing.B)  { benchFrames(b, false, false) }
func BenchmarkBriefFull(b *testing.B)   { benchFrames(b, true, true) }
func BenchmarkBriefSteady(b *testing.B) { benchFrames(b, true, false) }

// opening the card: every frame of the morph is a full one
func BenchmarkOpenMorph(b *testing.B) {
	clock := time.Date(2026, 10, 13, 15, 0, 0, 0, time.Local)
	a := briefApp(&clock)
	u := a.ui
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u.pinned = i%40 < 20 // open, close, open...
		clock = clock.Add(time.Second / 60)
		u.renderDamage()
	}
}
