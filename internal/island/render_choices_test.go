//go:build render

package island

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fourWeeks is a month of reps ending Thu 29 Oct 2026: most nights, a skip, two misses.
func fourWeeks() string {
	end := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	var ds []string
	for i := 27; i >= 0; i-- {
		d := end.AddDate(0, 0, -i)
		lv, md := "1", "done"
		switch i % 9 {
		case 4:
			lv, md = "1", "wait"
		case 7:
			lv, md = "0", ""
		case 2:
			lv = "0.5"
		}
		ds = append(ds, `{"date":"`+d.Format("2006-01-02")+`","level":`+lv+`,"mood":"`+md+`"}`)
	}
	return "[" + strings.Join(ds, ",") + "]"
}

// EMBER_PREVIEW=dir go test -tags render -count=1 -run PreviewChoices ./internal/island:
// a plugin's card with choice chips (four, and six on two rows, with a
// button), at rest, under the pointer and held; and bars with interval
// bands, in rows and in columns, in a dark and a light wallpaper's palette.

// lightTheme writes a light wallpaper's terminal theme where Ember reads its
// palette (HOME is a temporary directory).
func lightTheme(t *testing.T) palette {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".config", "alacritty", "themes")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "noctalia.toml"), []byte(`[colors.primary]
background = "#f6f1e8"
foreground = "#2b2622"
[colors.normal]
green = "#3d8b4f"
blue = "#3466a8"
red = "#b3402f"
yellow = "#b07a1c"
`), 0o644)
	defer os.Remove(filepath.Join(dir, "noctalia.toml"))
	return loadPalette()
}

func TestPreviewChoices(t *testing.T) {
	if os.Getenv("EMBER_PREVIEW") == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	t.Setenv("HOME", t.TempDir())
	dark, light := loadPalette(), lightTheme(t)
	four := `{"type":"card","id":"cue","eyebrow":"edge","title":"Tonight's rep: LC 2779","body":"Sliding window. When?","mood":"nudge",
		"chips":[{"key":"cue:now","label":"Now"},{"key":"cue:19:30","label":"19:30","primary":true},{"key":"cue:21:00","label":"21:00"},{"key":"skiptonight","label":"Skip (1 left)"}]}`
	six := `{"type":"card","id":"wa","eyebrow":"edge","title":"WA on test 7. What was it?","body":"2277C · a quick word helps the next one.","mood":"nudge",
		"chips":[{"key":"mistake:1:misread","label":"Misread"},{"key":"mistake:1:edge","label":"Edge case"},{"key":"mistake:1:overflow","label":"Overflow"},
		{"key":"mistake:1:approach","label":"Approach"},{"key":"mistake:1:bug","label":"Bug"},{"key":"mistake:1:other","label":"Something else"}],
		"buttons":[{"key":"open:cf:2277C","label":"Open"}]}`
	card := func(label, msg string, p palette, chip string, held bool) galleryShot {
		g := newScene()
		g.a.ui.pal = p
		g.a.applyExt("edge", extMsg(t, msg))
		g.a.ui.pinned = true
		g.run(2500 * time.Millisecond)
		if chip != "" {
			var r image.Rectangle
			for _, ht := range g.a.ui.hits {
				if ht.key == "chip:"+chip {
					r = ht.r
				}
			}
			if r.Empty() {
				t.Fatalf("no chip %s", chip)
			}
			x, y := float64(r.Min.X+r.Dx()/2), float64(r.Min.Y+r.Dy()/2)
			g.a.ui.pointer(pointerEvent{kind: "enter", x: x, y: y})
			g.a.ui.pointer(pointerEvent{kind: "motion", x: x, y: y})
			g.a.ui.Invalidate() // as the loop does when the pointer asks for a redraw
			g.run(600 * time.Millisecond)
			if held {
				g.a.ui.pointer(pointerEvent{kind: "press", x: x, y: y, button: 1})
				g.a.ui.Invalidate()
				g.run(300 * time.Millisecond)
			}
		}
		h := g.a.ui.cardHeight(g.a.ui.top())
		return galleryShot{label, onDesktop(g.a.ui, image.Rect(surfW/2-210, 0, surfW/2+210, min(surfH, h+24)))}
	}
	var shots []galleryShot
	for _, th := range []struct {
		name string
		p    palette
	}{{"dark", dark}, {"light", light}} {
		shots = append(shots,
			card("four chips, at rest · "+th.name, four, th.p, "", false),
			card("under the pointer · "+th.name, four, th.p, "cue:21:00", false),
			card("held · "+th.name, four, th.p, "cue:21:00", true),
			card("six chips and a button · "+th.name, six, th.p, "", false),
			card("under the pointer · "+th.name, six, th.p, "mistake:1:other", false),
			card("held · "+th.name, six, th.p, "mistake:1:other", true))
	}
	sheet(t, "plugins-card-chips", shots, 3)

	bars := `{"type":"dashboard","title":"edge","widgets":[
		{"kind":"bars","title":"Readiness by pattern","caption":"80% interval","items":[
			{"label":"Two pointers","value":0.82,"lo":0.71,"hi":0.9,"target":0.75,"caption":"82%"},
			{"label":"Sliding window","value":0.64,"lo":0.48,"hi":0.78,"target":0.75,"caption":"64%"},
			{"label":"Binary search","value":0.71,"lo":0.6,"hi":0.8,"target":0.75,"caption":"71%"},
			{"label":"DP 2D / knapsack","value":0.31,"lo":0.12,"hi":0.55,"target":0.75,"caption":"31%"},
			{"label":"Graphs BFS/DFS","value":0.9,"lo":0.84,"hi":0.95,"target":0.75,"caption":"90%"},
			{"label":"DSU","value":0.5,"target":0.75,"caption":"50%"}]},
		{"kind":"bars","title":"Weakest, by mood","caption":"0.75 asked","items":[
			{"label":"Heap","value":0.52,"lo":0.4,"hi":0.6,"target":0.75,"mood":"nudge","caption":"52%"},
			{"label":"Topo sort","value":0.44,"lo":0.3,"hi":0.58,"target":0.75,"mood":"nudge","caption":"44%"},
			{"label":"Intervals","value":0.48,"target":0.75,"mood":"wait","caption":"48%"},
			{"label":"Backtracking","value":0.4,"lo":0.22,"hi":0.55,"target":0.75,"mood":"quiet","caption":"40%"},
			{"label":"Trees","value":0.51,"lo":0.45,"hi":0.57,"target":0.75,"mood":"done","caption":"51%"}]},
		{"kind":"bars","layout":"columns","title":"Readiness, nightly","value":"64%","caption":"52–75%","items":[
			{"label":"M","value":0.55,"lo":0.4,"hi":0.68,"target":0.75},{"label":"T","value":0.58,"lo":0.45,"hi":0.7,"target":0.75},
			{"label":"W","value":0.6,"lo":0.48,"hi":0.71,"target":0.75,"mood":"done"},{"label":"T","value":0.64,"lo":0.52,"hi":0.75,"target":0.75,"today":true},
			{"label":"F","value":0,"state":"future","target":0.75},{"label":"S","value":0,"state":"future","target":0.75},{"label":"S","value":0,"state":"future","target":0.75}]}]}`
	page := func(label string, p palette) galleryShot {
		g := newScene()
		g.a.ui.pal = p
		g.a.applyExt("edge", extMsg(t, bars))
		g.a.ui.openDash("edge")
		g.run(2500 * time.Millisecond)
		_, h := extDash{name: "edge"}.Size(g.a.ui)
		return galleryShot{label, onDesktop(g.a.ui, image.Rect(surfW/2-290, 0, surfW/2+290, min(surfH, int(h)+20)))}
	}
	sheet(t, "plugins-bars-bands", []galleryShot{page("bands · dark", dark), page("bands · light", light)}, 2)
	bars = `{"type":"dashboard","title":"edge","widgets":[
		{"kind":"columns","split":0.5,"a":{"kind":"heatmap","title":"Last 4 weeks","weeks":4,"days":` + fourWeeks() + `},
			"b":{"kind":"note","text":"A month of reps: cells grow for a few weeks."}},
		{"kind":"heatmap","title":"Last 4 weeks, full width","weeks":4,"days":` + fourWeeks() + `},
		{"kind":"heatmap","title":"Last 6 months","weeks":26,"days":` + fourWeeks() + `}]}`
	sheet(t, "plugins-heatmap-short", []galleryShot{page("a short heatmap · dark", dark), page("light", light)}, 2)
}
