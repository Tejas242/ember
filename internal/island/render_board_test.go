//go:build render

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

// EMBER_PREVIEW=dir go test -tags render -count=1 -run PreviewBoard ./island: the card with
// several sessions and their subagents, and the pill saying "Clauding…".
func TestPreviewBoard(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	steps := []map[string]any{
		{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/store.go"}},
		{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/quota.go", "old_string": "a\nb", "new_string": "a\nb\nc\nd\ne"}},
		{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test ./...", "description": "Run the store tests"}},
		{"tool_name": "Grep", "tool_input": map[string]any{"pattern": "segment.Lock"}},
	}
	work := func(a *app, clock *time.Time, id, project, prompt string) {
		hookAt(a, "UserPromptSubmit", id, project, map[string]any{"prompt": prompt})
		for i, st := range steps {
			*clock = clock.Add(31 * time.Second)
			hookAt(a, "PreToolUse", id, project, st)
			if i < len(steps)-1 {
				hookAt(a, "PostToolUse", id, project, map[string]any{"tool_name": st["tool_name"]})
			}
		}
	}
	agents := func(a *app, clock *time.Time, id, project string, descs ...string) {
		for i, d := range descs {
			hookAt(a, "PreToolUse", id, project, map[string]any{"tool_name": "Agent", "tool_input": map[string]any{"description": d, "subagent_type": "Explore"}})
			aid := fmt.Sprint(id, "ag", i)
			hookAt(a, "SubagentStart", id, project, map[string]any{"agent_id": aid, "agent_type": "Explore"})
			for _, st := range steps[:2+i%2] {
				*clock = clock.Add(9 * time.Second)
				p := map[string]any{"agent_id": aid, "agent_type": "Explore", "tool_name": st["tool_name"], "tool_input": st["tool_input"]}
				hookAt(a, "PreToolUse", id, project, p)
			}
		}
	}
	type shot struct {
		name string
		set  func(a *app, c *time.Time)
	}
	shots := []shot{
		{"two-working", func(a *app, c *time.Time) {
			work(a, c, "k", "kinvault", "fix the race in the store's compaction path")
			agents(a, c, "k", "kinvault", "Map the store's locking", "Find every compaction caller")
			a.ui.opened["a:k:kag0"] = true
			work(a, c, "e", "edge", "add a hint ladder to the daily problem flow")
		}},
		{"waiting", func(a *app, c *time.Time) {
			work(a, c, "k", "kinvault", "fix the race in the store's compaction path")
			agents(a, c, "k", "kinvault", "Map the store's locking")
			work(a, c, "e", "edge", "add a hint ladder to the daily problem flow")
			hookAt(a, "Notification", "e", "edge", map[string]any{"notification_type": "permission_prompt"})
			work(a, c, "m", "ember", "redesign the card")
			hookAt(a, "Stop", "m", "ember", map[string]any{"last_assistant_message": "The card now lists every session; 41 tests pass."})
		}},
		{"done", func(a *app, c *time.Time) {
			work(a, c, "m", "ember", "redesign the card around sessions")
			*c = c.Add(time.Minute)
			hookAt(a, "Stop", "m", "ember", map[string]any{"last_assistant_message": "Done. The card now lists every session with its subagents on a branch, and 41 tests pass."})
			work(a, c, "e", "edge", "add a hint ladder")
			hookAt(a, "Stop", "e", "edge", map[string]any{"last_assistant_message": "Added the ladder."})
		}},
		{"show-all", func(a *app, c *time.Time) {
			for i, n := range []string{"kinvault", "edge", "ember", "dots", "notes"} {
				work(a, c, fmt.Sprint(i), n, "work on "+n)
			}
			a.ui.allRows = true
		}},
		{"agents-done", func(a *app, c *time.Time) {
			work(a, c, "k", "kinvault", "audit the store")
			agents(a, c, "k", "kinvault", "Map the locking", "Audit the tests")
			hookAt(a, "SubagentStop", "k", "kinvault", map[string]any{"agent_id": "kag0", "last_assistant_message": "Locks are taken in segment order everywhere but compact()."})
			hookAt(a, "SubagentStop", "k", "kinvault", map[string]any{"agent_id": "kag1", "last_assistant_message": "Three tests race on the shared tmp dir."})
			a.ui.opened["a:k:kag0"] = true
		}},
	}
	const W, H = 580, 380
	sheet := image.NewRGBA(image.Rect(0, 0, W*2, H*((len(shots)+1)/2)))
	draw.Draw(sheet, sheet.Rect, image.NewUniform(color.NRGBA{0x22, 0x33, 0x26, 255}), image.Point{}, draw.Src)
	for i, sh := range shots {
		clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
		a := newTestApp(&clock)
		u := NewUI(W, H, 22)
		u.now, u.mind, u.day = a.ui.now, a.ui.mind, a.ui.day
		a.ui = u
		sh.set(a, &clock)
		a.ui.track = &Track{Player: "p", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: 5 * time.Minute, Position: time.Minute, PosAt: clock}
		a.ui.edge = &EdgeStatus{Today: 1, Need: 1, Streak: 4}
		a.ui.dashboards["edge"] = edgeDash{}
		all := a.ui.allRows
		a.ui.pinned = true
		settle(a.ui, &clock)
		a.ui.allRows = all
		a.ui.Invalidate()
		for f := 0; f < 90; f++ {
			clock = clock.Add(time.Second / 60)
			a.ui.renderDamage()
		}
		ox, oy := (i%2)*W, (i/2)*H
		draw.Draw(sheet, image.Rect(ox, oy, ox+W, oy+22), image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 255}), image.Point{}, draw.Over)
		draw.Draw(sheet, image.Rect(ox, oy, ox+W, oy+H), a.ui.cv.img, image.Point{}, draw.Over)
	}
	out, _ := os.Create(filepath.Join(dir, "board.png"))
	png.Encode(out, sheet)
	out.Close()

	// the pill, with two sessions at it
	clock := time.Date(2026, 10, 15, 15, 0, 0, 0, time.Local)
	a := newTestApp(&clock)
	work(a, &clock, "k", "kinvault", "fix it")
	work(a, &clock, "e", "edge", "fix it too")
	settle(a.ui, &clock)
	out, _ = os.Create(filepath.Join(dir, "pill.png"))
	pill := image.NewRGBA(image.Rect(0, 0, 440, 40))
	draw.Draw(pill, pill.Rect, image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 255}), image.Point{}, draw.Src)
	draw.Draw(pill, pill.Rect, a.ui.cv.img, image.Point{}, draw.Over)
	png.Encode(out, pill)
	out.Close()
}
