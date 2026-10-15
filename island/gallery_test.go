package island

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

// EMBER_PREVIEW=dir [EMBER_BG=screenshot.png] go test -run Gallery ./island
//
// Every state of the island, composited over a real screenshot of the
// desktop (EMBER_BG, a 1366×768 capture without Ember) with the blur niri
// puts behind it, at 2× for looking closely: gallery-pills, -cards,
// -requests, and -motion (filmstrips of the transitions).

type galleryShot struct {
	label string
	img   *image.RGBA // the surface, 580×380
}

var galleryBG *image.RGBA

func galleryBackground() *image.RGBA {
	if galleryBG != nil {
		return galleryBG
	}
	bg := image.NewRGBA(image.Rect(0, 0, 1366, 768))
	if f, err := os.Open(os.Getenv("EMBER_BG")); err == nil {
		if im, _, err := image.Decode(f); err == nil {
			draw.Draw(bg, bg.Rect, im, im.Bounds().Min, draw.Src)
		}
		f.Close()
	} else {
		for y := 0; y < 768; y++ {
			for x := 0; x < 1366; x++ {
				bg.Set(x, y, color.NRGBA{uint8(20 + x/40), uint8(36 + y/10), uint8(28 + x/60), 255})
			}
		}
		draw.Draw(bg, image.Rect(0, 0, 1366, 22), image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 150}), image.Point{}, draw.Over)
	}
	galleryBG = bg
	return bg
}

// boxBlur blurs r of img in place (three box passes ≈ a gaussian).
func boxBlur(img *image.RGBA, r image.Rectangle, rad int) {
	r = r.Intersect(img.Rect)
	if r.Empty() {
		return
	}
	w, h := r.Dx(), r.Dy()
	buf := make([]float32, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := img.PixOffset(r.Min.X+x, r.Min.Y+y)
			for c := 0; c < 3; c++ {
				buf[(y*w+x)*3+c] = float32(img.Pix[i+c])
			}
		}
	}
	tmp := make([]float32, len(buf))
	pass := func(src, dst []float32, horiz bool) {
		n, m := w, h
		if !horiz {
			n, m = h, w
		}
		for j := 0; j < m; j++ {
			for c := 0; c < 3; c++ {
				at := func(k int) float32 {
					k = max(0, min(n-1, k))
					if horiz {
						return src[(j*w+k)*3+c]
					}
					return src[(k*w+j)*3+c]
				}
				var sum float32
				for k := -rad; k <= rad; k++ {
					sum += at(k)
				}
				for k := 0; k < n; k++ {
					v := sum / float32(2*rad+1)
					if horiz {
						dst[(j*w+k)*3+c] = v
					} else {
						dst[(k*w+j)*3+c] = v
					}
					sum += at(k+rad+1) - at(k-rad)
				}
			}
		}
	}
	for i := 0; i < 3; i++ {
		pass(buf, tmp, true)
		pass(tmp, buf, false)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := img.PixOffset(r.Min.X+x, r.Min.Y+y)
			for c := 0; c < 3; c++ {
				img.Pix[i+c] = uint8(buf[(y*w+x)*3+c])
			}
		}
	}
}

// onDesktop puts a rendered surface where it sits on screen, over the
// blurred desktop, and crops around it.
func onDesktop(u *UI, crop image.Rectangle) *image.RGBA {
	bg := galleryBackground()
	out := image.NewRGBA(image.Rect(0, 0, u.W, u.H))
	ox := (bg.Rect.Dx() - u.W) / 2
	draw.Draw(out, out.Rect, bg, image.Pt(ox, 0), draw.Src)
	for _, r := range u.shape.blurRects() {
		boxBlur(out, r.Inset(-1), 9)
	}
	draw.Draw(out, out.Rect, u.cv.img, image.Point{}, draw.Over)
	return out.SubImage(crop).(*image.RGBA)
}

func galleryArt() *albumArt {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dx, dy := float64(x)-22, float64(y)-20
			d := math.Sqrt(dx*dx+dy*dy) / 60
			r := 235 - 120*d
			g := 120 - 60*d + float64(y)
			b := 70 + 90*d
			if (x/8+y/8)%5 == 0 {
				r, g = r*0.85, g*0.9
			}
			img.Set(x, y, color.NRGBA{uint8(math.Min(255, r)), uint8(math.Min(255, math.Max(0, g))), uint8(math.Min(255, b)), 255})
		}
	}
	a := &albumArt{small: scaleSquare(img, 16), large: scaleSquare(img, 60), row: scaleSquare(img, 24)}
	a.accent = accentOf(a.large)
	return a
}

// sheet lays shots out in a grid with a label over each, at 2×.
func sheet(t *testing.T, name string, shots []galleryShot, cols int) {
	if len(shots) == 0 {
		return
	}
	cw, ch := shots[0].img.Rect.Dx(), shots[0].img.Rect.Dy()
	for _, s := range shots {
		cw, ch = max(cw, s.img.Rect.Dx()), max(ch, s.img.Rect.Dy())
	}
	const lab = 20
	rows := (len(shots) + cols - 1) / cols
	out := newCanvas(cols*cw, rows*(ch+lab))
	draw.Draw(out.img, out.img.Rect, image.NewUniform(color.NRGBA{0x10, 0x12, 0x10, 255}), image.Point{}, draw.Src)
	for i, s := range shots {
		x, y := (i%cols)*cw, (i/cols)*(ch+lab)
		out.text(face(fMedium, 11), s.label, float32(x+8), float32(y+14), color.NRGBA{0x9a, 0xa0, 0x98, 255})
		draw.Draw(out.img, s.img.Rect.Sub(s.img.Rect.Min).Add(image.Pt(x, y+lab)), s.img, s.img.Rect.Min, draw.Src)
	}
	big := image.NewRGBA(image.Rect(0, 0, out.img.Rect.Dx()*2, out.img.Rect.Dy()*2))
	xdraw.CatmullRom.Scale(big, big.Rect, out.img, out.img.Rect, xdraw.Src, nil)
	f, err := os.Create(filepath.Join(os.Getenv("EMBER_PREVIEW"), name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, big)
	f.Close()
}

type galleryScene struct {
	clock time.Time
	a     *app
}

func newScene() *galleryScene {
	g := &galleryScene{clock: time.Date(2026, 10, 15, 17, 24, 0, 0, time.Local)}
	g.a = newTestApp(&g.clock)
	u := NewUI(surfW, surfH, 22)
	u.now, u.mind, u.day = g.a.ui.now, g.a.ui.mind, g.a.ui.day
	u.start, u.lastBlink, u.bounceAt = g.clock, g.clock, g.clock.Add(-time.Hour)
	g.a.ui = u
	return g
}

func (g *galleryScene) run(d time.Duration) {
	for end := g.clock.Add(d); g.clock.Before(end); {
		g.clock = g.clock.Add(time.Second / 60)
		g.a.ui.renderDamage()
	}
}

var gallerySteps = []map[string]any{
	{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/store/compact.go"}},
	{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/store/quota.go", "old_string": "a\nb\nc", "new_string": "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no\np\nq\nr"}},
	{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test ./store/...", "description": "Run the store tests"}, "fail": true},
	{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "/x/store/store.go", "old_string": "a\nb\nc", "new_string": "a\nx\ny\nz\nw"}},
	{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test -race ./...", "description": "Run the whole suite under the race detector"}},
}

func (g *galleryScene) work(id, project, prompt string, n int) {
	hookAt(g.a, "UserPromptSubmit", id, project, map[string]any{"prompt": prompt})
	for i, st := range gallerySteps[:n] {
		g.clock = g.clock.Add(41 * time.Second)
		hookAt(g.a, "PreToolUse", id, project, map[string]any{"tool_name": st["tool_name"], "tool_input": st["tool_input"]})
		if i < n-1 {
			ev := "PostToolUse"
			if st["fail"] == true {
				ev = "PostToolUseFailure"
			}
			hookAt(g.a, ev, id, project, map[string]any{"tool_name": st["tool_name"]})
		}
	}
}

func (g *galleryScene) agent(id, project, aid, desc string, steps int) {
	hookAt(g.a, "PreToolUse", id, project, map[string]any{"tool_name": "Agent", "tool_input": map[string]any{"description": desc, "subagent_type": "Explore"}})
	hookAt(g.a, "SubagentStart", id, project, map[string]any{"agent_id": aid, "agent_type": "Explore"})
	for _, st := range []map[string]any{
		{"tool_name": "Grep", "tool_input": map[string]any{"pattern": "segment.Lock"}},
		{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/x/store/segment.go"}},
	}[:steps] {
		g.clock = g.clock.Add(12 * time.Second)
		hookAt(g.a, "PreToolUse", id, project, map[string]any{"agent_id": aid, "agent_type": "Explore", "tool_name": st["tool_name"], "tool_input": st["tool_input"]})
	}
}

func (g *galleryScene) music(playing bool) {
	g.a.ui.track = &Track{Player: "p", Title: "Instant Crush", Artist: "Daft Punk, Julian Casablancas", Playing: playing,
		Length: 5*time.Minute + 37*time.Second, Position: 2 * time.Minute, PosAt: g.clock}
	g.a.ui.art = galleryArt()
}

func (g *galleryScene) edge() {
	g.a.ui.edge = &EdgeStatus{Today: 1, Need: 2, Streak: 4}
	g.a.ui.dashboards["edge"] = edgeDash{}
}

func TestGallery(t *testing.T) {
	if os.Getenv("EMBER_PREVIEW") == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	pillCrop := image.Rect(surfW/2-230, 0, surfW/2+230, 46)
	pill := func(label string, set func(g *galleryScene)) galleryShot {
		g := newScene()
		set(g)
		g.run(2500 * time.Millisecond)
		return galleryShot{label, onDesktop(g.a.ui, pillCrop)}
	}
	var pills []galleryShot
	pills = append(pills,
		pill("quiet", func(g *galleryScene) {}),
		pill("asleep", func(g *galleryScene) {
			g.a.ui.mind.asleep, g.a.ui.mind.arousal, g.a.ui.mind.sleepAt = true, 0.08, g.clock.Add(-time.Hour)
			g.a.ui.mind.lastEvent = g.clock.Add(-time.Hour)
		}),
		pill("working", func(g *galleryScene) { g.work("k", "kinvault", "fix the race in the store's compaction path", 5) }),
		pill("working: three sessions, subagents", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the race in the store's compaction path", 5)
			g.agent("k", "kinvault", "a1", "Map the store's locking", 2)
			g.agent("k", "kinvault", "a2", "Find every compaction caller", 1)
			g.work("e", "edge", "add a hint ladder", 2)
			g.work("m", "ember", "redesign the island", 3)
		}),
		pill("working, music as the satellite", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the race in the store's compaction path", 5)
			g.music(true)
		}),
		pill("needs you (the halo breathes)", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the race", 2)
			hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
		}),
		pill("done", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "done:k", act: "claude", mood: moodHappy, verb: "Done", text: "kinvault", sub: "All 42 tests pass under -race", until: g.clock.Add(time.Hour), focus: "k"})
		}),
		pill("stopped", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "fail:k", act: "claude", mood: moodError, verb: "Stopped", text: "kinvault", sub: "rate limit", until: g.clock.Add(time.Hour)})
		}),
		pill("music", func(g *galleryScene) { g.music(true) }),
		pill("music, paused", func(g *galleryScene) { g.music(false); g.a.ui.focus, g.a.ui.focusAt = "music", g.clock }),
		pill("volume", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "osd:volume", icon: "volume", tint: g.a.ui.pal.fg, meter: true, level: 0.64, right: "64", until: g.clock.Add(time.Hour), osd: true})
		}),
		pill("muted", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "osd:volume", icon: "mute", tint: g.a.ui.pal.muted, text: "Muted", meter: true, level: 0.4, until: g.clock.Add(time.Hour), osd: true})
		}),
		pill("brightness", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "osd:brightness", icon: "sun", tint: g.a.ui.pal.yellow, meter: true, level: 0.3, right: "30", until: g.clock.Add(time.Hour), osd: true})
		}),
		pill("charging", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "power", icon: "bolt", tint: g.a.ui.pal.green, text: "Charging", right: "73%", batt: true, pct: 0.73, until: g.clock.Add(time.Hour)})
		}),
		pill("battery low", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "power", icon: "warning", tint: g.a.ui.pal.red, text: "Battery low", right: "9%", batt: true, pct: 0.09, until: g.clock.Add(time.Hour)})
		}),
		pill("headphones", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "device:x", icon: "headphones", tint: g.a.ui.pal.blue, text: "WH-1000XM4", sub: "Connected", right: "80%", batt: true, pct: 0.8, until: g.clock.Add(time.Hour)})
		}),
		pill("screenshot", func(g *galleryScene) {
			g.a.ui.Moment(moment{key: "shot", icon: "camera", tint: g.a.ui.pal.fg, text: "Screenshot saved", thumb: scaleThumb(galleryBackground().SubImage(image.Rect(200, 100, 1166, 668)), 30, 18), until: g.clock.Add(time.Hour)})
		}),
		pill("sharing the screen, quiet", func(g *galleryScene) { g.a.ui.casting = true }),
	)
	sheet(t, "gallery-pills", pills, 2)

	card := func(label string, set func(g *galleryScene)) galleryShot {
		g := newScene()
		set(g)
		g.a.ui.pinned = true
		g.run(2500 * time.Millisecond)
		h := g.a.ui.expandedHeight()
		if g.a.ui.top() != nil {
			h = g.a.ui.cardHeight(g.a.ui.top())
		}
		return galleryShot{label, onDesktop(g.a.ui, image.Rect(surfW/2-210, 0, surfW/2+210, min(surfH, h+24)))}
	}
	cards := []galleryShot{
		card("working: the timeline hangs from Ember", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the race in the store's compaction path and make the tests pass", 5)
			g.agent("k", "kinvault", "a1", "Map the store's locking", 2)
			g.music(true)
			g.edge()
		}),
		card("three sessions: the one you can't see is the hero", func(g *galleryScene) {
			g.work("e", "edge", "add a hint ladder to the daily problem flow", 3)
			g.work("m", "ember", "redesign the island", 2)
			hookAt(g.a, "Stop", "m", "ember", map[string]any{"last_assistant_message": "The island is redesigned and 41 tests pass."})
			g.work("k", "kinvault", "fix the race in the store's compaction path", 4)
			g.agent("k", "kinvault", "a1", "Map the store's locking", 2)
			g.music(true)
			g.edge()
		}),
		card("needs you, in the terminal", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the race in the store's compaction path", 4)
			hookAt(g.a, "Notification", "k", "kinvault", map[string]any{"notification_type": "permission_prompt"})
			g.work("e", "edge", "add a hint ladder", 2)
			g.music(true)
		}),
		card("just finished", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the race in the store's compaction path", 5)
			g.clock = g.clock.Add(30 * time.Second)
			hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "Fixed the race in the store's compaction path: writers now take the segment lock before rotating, and all 42 tests pass under -race."})
			g.clock = g.clock.Add(3 * time.Minute)
			g.music(false)
			g.edge()
		}),
		card("a quiet day", func(g *galleryScene) {
			for h, c := range []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 14, 31, 22, 4, 0, 18, 40, 52, 26, 0} {
				g.a.ui.day.Hours[h] = c
			}
			g.a.ui.day.Tasks, g.a.ui.day.Busy = 7, 2*time.Hour+14*time.Minute
			g.a.ui.day.Projects = map[string]int{"ember": 5, "kinvault": 2}
			g.music(true)
			g.edge()
		}),
	}
	// point at the music section of the last card, once it's laid out
	{
		g := newScene()
		cards = append(cards, func() galleryShot {
			g.work("k", "kinvault", "audit the store for lock ordering bugs", 3)
			g.agent("k", "kinvault", "a1", "Map the store's locking", 2)
			g.agent("k", "kinvault", "a2", "Audit the tests", 1)
			hookAt(g.a, "SubagentStop", "k", "kinvault", map[string]any{"agent_id": "a1", "last_assistant_message": "Locks are taken in segment order everywhere but compact()."})
			g.a.ui.opened["a:k:a1"] = true
			g.music(true)
			g.a.ui.pinned = true
			g.run(time.Second)
			r := g.a.ui.seekRect
			g.a.ui.pointer(pointerEvent{kind: "enter", x: float64(r.Max.X - 40), y: float64(r.Min.Y - 18)})
			g.a.ui.Invalidate()
			g.run(1500 * time.Millisecond)
			return galleryShot{"subagents, one opened; music under the pointer", onDesktop(g.a.ui, image.Rect(surfW/2-210, 0, surfW/2+210, min(surfH, g.a.ui.expandedHeight()+24)))}
		}())
	}
	sheet(t, "gallery-cards", cards, 2)

	req := func(label string, c *Card, extra func(g *galleryScene)) galleryShot {
		g := newScene()
		if extra != nil {
			extra(g)
		}
		c.Born = g.clock
		if c.Expires.IsZero() {
			c.Expires = g.clock.Add(85 * time.Second)
		}
		g.a.ui.AddCard(c)
		g.run(2500 * time.Millisecond)
		return galleryShot{label, onDesktop(g.a.ui, image.Rect(surfW/2-210, 0, surfW/2+210, min(surfH, g.a.ui.cardHeight(g.a.ui.top())+24)))}
	}
	perm := func() []Button {
		return []Button{{Key: "deny", Label: "Deny", Danger: true}, {Key: "always", Label: "Always"}, {Key: "allow", Label: "Allow", Primary: true}}
	}
	reqs := []galleryShot{
		req("a command", &Card{ID: "p", Kind: "perm", Mood: moodWait, Eyebrow: "kinvault wants to run", Title: "Run the store tests with the race detector",
			Code: "go test -race ./store/... -run TestRepair -count=1", Shell: true, Buttons: perm(), Footer: "Answer in terminal"}, nil),
		req("an edit, two more waiting", &Card{ID: "e", Kind: "perm", Mood: moodWait, Eyebrow: "ember wants to edit", Title: "Edit board.go",
			Body: "~/Gems/ember/island  ·  +48 −112", Buttons: perm(), Footer: "Answer in terminal"}, func(g *galleryScene) {
			g.a.ui.AddCard(&Card{ID: "x1", Priority: -1})
			g.a.ui.AddCard(&Card{ID: "x2", Priority: -1})
		}),
		req("a question", &Card{ID: "q", Kind: "ask", Mood: moodWait, Eyebrow: "kinvault asks", Meta: "1 of 2", Title: "Which storage engine should the index use?",
			Options: []Option{{"Pebble", "LSM, battle tested"}, {"bbolt", "B+tree, one file"}, {"Roll our own", "Append-only log and an index"}}, Footer: "Answer in terminal"}, nil),
		req("several answers, the last seconds", &Card{ID: "m", Kind: "ask", Mood: moodWait, Eyebrow: "edge asks", Title: "Which topics should the ladder cover?", Multi: true,
			Options: []Option{{"Graphs", ""}, {"Dynamic programming", ""}, {"Binary search", ""}}, Buttons: []Button{{Key: "submit", Label: "Submit", Primary: true}}, Footer: "Answer in terminal",
			picked: map[int]bool{0: true, 2: true}, Expires: time.Date(2026, 10, 15, 17, 24, 12, 0, time.Local)}, nil),
		req("edge's nudge", &Card{ID: "edge:1", Kind: "edge", Mood: moodNudge, Eyebrow: "edge", Title: "Today's problem is waiting",
			Body: "1360E · Polygon, rated 1400. Claude unlocks when it's solved.", Buttons: []Button{{Key: "later", Label: "Later"}, {Key: "start", Label: "Start", Primary: true}}}, nil),
	}
	sheet(t, "gallery-requests", reqs, 2)

	// motion: filmstrips, one frame every few sixtieths
	var motion []galleryShot
	strip := func(name, label string, set func(g *galleryScene), act func(g *galleryScene), frames []int, crop image.Rectangle) {
		g := newScene()
		set(g)
		g.run(2 * time.Second)
		act(g)
		f := 0
		for _, n := range frames {
			for ; f < n; f++ {
				g.clock = g.clock.Add(time.Second / 60)
				g.a.ui.renderDamage()
			}
			motion = append(motion, galleryShot{fmt.Sprintf("%s · %d ms", label, n*1000/60), onDesktop(g.a.ui, crop)})
		}
		sheet(t, "gallery-motion-"+name, motion, 4)
		motion = nil
	}
	open := []int{0, 3, 6, 9, 13, 18, 26, 40}
	cardCrop := image.Rect(surfW/2-210, 0, surfW/2+210, 300)
	strip("open", "the card opens", func(g *galleryScene) {
		g.work("k", "kinvault", "fix the race in the store's compaction path and make the tests pass", 5)
		g.music(true)
		g.edge()
	}, func(g *galleryScene) { g.a.ui.pinned = true }, open, cardCrop)
	strip("done", "done arrives", func(g *galleryScene) { g.work("k", "kinvault", "fix the race", 3); g.a.ui.watching["k"] = false }, func(g *galleryScene) {
		g.a.ui.mind.away = false
		hookAt(g.a, "Stop", "k", "kinvault", map[string]any{"last_assistant_message": "All 42 tests pass under -race."})
	}, []int{0, 4, 8, 12, 18, 26, 36, 50}, pillCrop)
	strip("hero", "another session becomes the hero", func(g *galleryScene) {
		g.work("e", "edge", "add a hint ladder to the daily problem flow", 3)
		g.work("k", "kinvault", "fix the race in the store's compaction path", 4)
		g.a.ui.pinned = true
	}, func(g *galleryScene) { g.a.ui.press("s:e", 1); g.a.ui.Invalidate() }, open, cardCrop)
}
