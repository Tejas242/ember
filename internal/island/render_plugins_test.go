//go:build render

package island

import (
	"image"
	"os"
	"strconv"
	"testing"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
)

// EMBER_PREVIEW=dir go test -tags render -count=1 -run PreviewPlugins ./internal/island:
// what external plugins can show, through the same path the host feeds:
// pill lines, a moment, a row and a card, and a dashboard of every widget.
func TestPreviewPlugins(t *testing.T) {
	if os.Getenv("EMBER_PREVIEW") == "" {
		t.Skip("set EMBER_PREVIEW")
	}
	t.Setenv("HOME", t.TempDir())
	send := func(g *galleryScene, name, line string) {
		m, err := proto.Decode([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		g.a.applyExt(name, m)
	}
	ms := func(g *galleryScene, d time.Duration) string {
		return strconv.FormatInt(g.clock.Add(d).UnixMilli(), 10)
	}
	pillCrop := image.Rect(surfW/2-230, 0, surfW/2+230, 46)
	pill := func(label string, set func(g *galleryScene)) galleryShot {
		g := newScene()
		set(g)
		g.run(2500 * time.Millisecond)
		return galleryShot{label, onDesktop(g.a.ui, pillCrop)}
	}
	sheet(t, "plugins-pill", []galleryShot{
		pill("a timer, counting down", func(g *galleryScene) {
			send(g, "pomodoro", `{"type":"status","text":"Focus","sub":"session 3","icon":"timer","mood":"work","until_ms":`+ms(g, 17*time.Minute+30*time.Second)+`}`)
		}),
		pill("progress", func(g *galleryScene) {
			send(g, "status-deploy", `{"type":"status","text":"Deploying","sub":"eu-west","mood":"work","progress":0.4}`)
		}),
		pill("needs you", func(g *galleryScene) {
			send(g, "ci", `{"type":"status","text":"Review the release","icon":"warning","mood":"wait","since_ms":`+ms(g, -12*time.Minute)+`,"urgency":"high"}`)
		}),
		pill("a moment", func(g *galleryScene) {
			send(g, "ci", `{"type":"moment","text":"Build passed","sub":"42 tests","icon":"check","mood":"done"}`)
		}),
	}, 2)

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
	sheet(t, "plugins-cards", []galleryShot{
		card("rows in the card", func(g *galleryScene) {
			g.work("k", "kinvault", "fix the flaky test", 3)
			g.music(true)
			send(g, "pomodoro", `{"type":"row","title":"Focus","sub":"17 min left · 3 today","icon":"timer","mood":"work","progress":0.3}`)
			send(g, "ci", `{"type":"row","title":"main #81","sub":"passed in 4m","icon":"check","mood":"done"}`)
			send(g, "ci", `{"type":"dashboard","widgets":[]}`)
		}),
		card("a card with buttons", func(g *galleryScene) {
			send(g, "pomodoro", `{"type":"card","id":"break","eyebrow":"Pomodoro","title":"Take a break?","body":"25 minutes of focus done. Five minutes away from the screen will help the next one.","mood":"done","buttons":[{"key":"skip","label":"Keep going"},{"key":"break","label":"Break","primary":true}]}`)
		}),
	}, 2)

	g := newScene()
	send(g, "ci", `{"type":"dashboard","title":"CI","widgets":[
		{"kind":"stats","title":"Today","items":[{"label":"Builds","value":12},{"label":"Green","value":"92%"},{"label":"Median","value":"4m","caption":"a build"}]},
		{"kind":"sparkline","title":"Minutes a build","values":[4,5,3,6,4,7,5,4,3,5,6,4],"caption":"the last twelve"},
		{"kind":"bars","title":"By repo","items":[{"label":"ember","value":7},{"label":"edge","value":5},{"label":"notes","value":2}]},
		{"kind":"chips","title":"Stages","items":[{"label":"B","state":"done"},{"label":"T","state":"tried"},{"label":"L","state":"owed"},{"label":"D","state":"todo"}],"caption":"build, test, lint, deploy"},
		{"kind":"list","title":"Recent","items":[{"title":"main #81","sub":"4m · passed","icon":"check","mood":"done"},{"title":"pr-212 #80","sub":"6m · a flaky test","icon":"warning","mood":"fail"}]}]}`)
	g.run(500 * time.Millisecond)
	g.a.ui.openDash("ci")
	g.run(2500 * time.Millisecond)
	_, h := extDash{name: "ci"}.Size(g.a.ui)
	sheet(t, "plugins-dash", []galleryShot{{"a dashboard of declared widgets", onDesktop(g.a.ui, image.Rect(surfW/2-290, 0, surfW/2+290, min(surfH, int(h)+20)))}}, 1)
}
