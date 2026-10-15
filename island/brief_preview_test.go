package island

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// EMBER_PREVIEW=dir go test -run PreviewBrief ./island: the card in each of its moods.
func TestPreviewBrief(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	cover := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			cover.Set(x, y, color.NRGBA{uint8(120 + x*2), uint8(40 + y), uint8(160 - x), 255})
		}
	}
	music := func(a *app, clock time.Time) {
		a.ui.track = &Track{Player: "org.mpris.MediaPlayer2.YoutubeMusic", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: 5 * time.Minute, Position: 2 * time.Minute, PosAt: clock}
		a.ui.art = &albumArt{small: scaleSquare(cover, 18), large: scaleSquare(cover, 60), row: scaleSquare(cover, 30)}
		a.ui.art.accent = accentOf(a.ui.art.large)
	}
	work := func(a *app, clock *time.Time, id, project string, fail bool) {
		hookAt(a, "UserPromptSubmit", id, project, map[string]any{"prompt": map[string]string{
			"kinvault": "fix the race in the store's compaction path and make the tests pass under -race",
			"edge":     "add a hint ladder to the daily problem flow",
			"airspace": "draw the radar view for parallel sessions"}[project]})
		steps := []map[string]any{
			{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/store.go"}},
			{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/quota.go", "old_string": "a\nb\nc", "new_string": "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no"}},
			{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test ./store/...", "description": "Run the store tests"}},
			{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/store.go", "old_string": "x\ny", "new_string": "x\ny\nz\nw"}},
			{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test -race ./...", "description": "Run the whole suite under the race detector"}},
		}
		for i, st := range steps {
			*clock = clock.Add(47 * time.Second)
			hookAt(a, "PreToolUse", id, project, st)
			if i < len(steps)-1 {
				ev := "PostToolUse"
				if fail && i == 2 {
					ev = "PostToolUseFailure"
				}
				hookAt(a, ev, id, project, map[string]any{"tool_name": st["tool_name"]})
			}
		}
		*clock = clock.Add(12 * time.Second)
	}
	const W, H = 520, 300
	type shot struct {
		name string
		set  func(a *app, clock *time.Time)
	}
	shots := []shot{
		{"work", func(a *app, c *time.Time) {
			work(a, c, "s", "kinvault", true)
			a.ui.sessions["s"].Title = "Quota enforcement"
			music(a, *c)
		}},
		{"work-busy", func(a *app, c *time.Time) {
			work(a, c, "o", "airspace", false)
			hookAt(a, "Stop", "o", "airspace", map[string]any{"last_assistant_message": "Added the radar view; 18 tests pass."})
			*c = c.Add(4 * time.Minute)
			work(a, c, "e", "edge", false)
			work(a, c, "s", "kinvault", true)
			music(a, *c)
		}},
		{"agents", func(a *app, c *time.Time) {
			work(a, c, "s", "kinvault", false)
			for i, d := range []string{"Map the store's locking", "Find every compaction caller", "Audit the quota tests"} {
				hookAt(a, "PreToolUse", "s", "kinvault", map[string]any{"tool_name": "Agent",
					"tool_input": map[string]any{"description": d, "subagent_type": "Explore"}})
				hookAt(a, "SubagentStart", "s", "kinvault", map[string]any{"agent_id": fmt.Sprint("x", i), "agent_type": "Explore"})
				*c = c.Add(20 * time.Second)
			}
			for i, tl := range []map[string]any{
				{"tool_name": "Grep", "tool_input": map[string]any{"pattern": "segment.Lock"}},
				{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/compact.go"}},
				{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test ./quota", "description": "Run the quota tests"}}} {
				hookAt(a, "PreToolUse", "s", "kinvault", map[string]any{"agent_id": fmt.Sprint("x", i), "agent_type": "Explore", "tool_name": tl["tool_name"], "tool_input": tl["tool_input"]})
			}
			hookAt(a, "Stop", "s", "kinvault", map[string]any{"last_assistant_message": "Three agents are mapping it."})
			*c = c.Add(50 * time.Second)
			music(a, *c)
		}},
		{"recap", func(a *app, c *time.Time) {
			work(a, c, "s", "kinvault", true)
			*c = c.Add(40 * time.Second)
			hookAt(a, "Stop", "s", "kinvault", map[string]any{"last_assistant_message": "Done.\n\nFixed the race in the store's compaction path: writers now take the segment lock before rotating, and all 42 tests pass under -race."})
			*c = c.Add(3 * time.Minute)
			music(a, *c)
		}},
		{"day", func(a *app, c *time.Time) {
			for h, n := range map[int]int{9: 14, 10: 30, 11: 22, 14: 8, 15: 40, 16: 25, 17: 6} {
				a.ui.day.Hours[h] = n
			}
			a.ui.day.Tasks, a.ui.day.Busy = 7, 2*time.Hour+14*time.Minute
			a.ui.day.Projects = map[string]int{"ember": 4, "kinvault": 2, "edge": 1}
			music(a, *c)
		}},
		{"wait", func(a *app, c *time.Time) {
			work(a, c, "s", "kinvault", false)
			hookAt(a, "Notification", "s", "kinvault", map[string]any{"notification_type": "permission_prompt"})
		}},
	}
	sheet := image.NewRGBA(image.Rect(0, 0, W, H*len(shots)))
	for i, sh := range shots {
		clock := time.Date(2026, 10, 13, 17, 20, 0, 0, time.Local)
		a := newTestApp(&clock)
		a.ui.start, a.ui.bounceAt = clock, clock.Add(-time.Hour)
		sh.set(a, &clock)
		a.ui.pinned = true
		for f := 0; f < 120; f++ {
			clock = clock.Add(time.Second / 60)
			a.ui.renderDamage()
		}
		r := image.Rect(0, H*i, W, H*(i+1))
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := 0; x < W; x++ {
				sheet.Set(x, y, color.NRGBA{uint8(20 + x/30), uint8(38 + (y-r.Min.Y)/8), uint8(40 + x/20), 255})
			}
		}
		draw.Draw(sheet, image.Rect(0, H*i, W, H*i+22), image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 200}), image.Point{}, draw.Over)
		draw.Draw(sheet, r, a.ui.cv.img, image.Pt((440-W)/2, 0), draw.Over)
	}
	big := image.NewRGBA(image.Rect(0, 0, W*2, sheet.Rect.Dy()*2))
	for y := 0; y < big.Rect.Dy(); y++ {
		for x := 0; x < big.Rect.Dx(); x++ {
			big.Set(x, y, sheet.At(x/2, y/2))
		}
	}
	f, _ := os.Create(filepath.Join(dir, "brief.png"))
	png.Encode(f, big)
	f.Close()
}
