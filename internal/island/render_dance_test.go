//go:build render

package island

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// EMBER_DANCE=dir go test -tags render -count=1 -run TestDanceFilm ./internal/island
//
// Ember dancing to the real song's levels at each size, from the same input:
// the pill (16 px), the card (24 px), the vibe stage (88 px). Frames at 60 a
// second, every other one saved (30 fps), and a trace of the pose each frame.
// Pill and card: 2 s quiet, the song starts (headphones on), 20 s of it, it
// pauses (headphones off), 3 s quiet. The stage: 20 s of the song.

func TestDanceFilm(t *testing.T) {
	dir := os.Getenv("EMBER_DANCE")
	if dir == "" {
		t.Skip("set EMBER_DANCE")
	}
	t.Setenv("HOME", t.TempDir())
	only := os.Getenv("EMBER_DANCE_ONLY")
	for _, size := range []string{"pill", "card", "stage"} {
		if only != "" && only != size {
			continue
		}
		sub := filepath.Join(dir, size)
		os.MkdirAll(sub, 0o755)
		danceFilm(t, size, sub)
	}
}

func danceFilm(t *testing.T, size, dir string) {
	var g *galleryScene
	lead, songLen, tail := 2*time.Second, 20*time.Second, 3*time.Second
	if size == "stage" {
		sc := vibeScenes()[0]
		sc.pos = 48500 * time.Millisecond
		var b *vibeBridge
		g, b = buildVibe(t, sc)
		_ = b
		g.a.ui.groove.reset() // it hears the song from its start, as in the pill
		lead, tail = 0, 0
	} else {
		g = newScene()
		if size == "card" {
			g.a.ui.pinned = true
		}
		g.run(time.Second)
		g.a.ui.audio = &source.Audio{}
	}
	_ = proto.CloseUser
	u := g.a.ui
	var ps pose
	debugPose = &ps
	defer func() { debugPose = nil }()
	tr, _ := os.Create(filepath.Join(dir, "trace.csv"))
	defer tr.Close()
	fmt.Fprintln(tr, "t,dancing,lock,phase,move,energy,hold,danceSq,leanx,sqx,y,tipx,tipHx,phy,tilt,plift,psq,plean,ptip,ptipH,eyes,ex,ey,es,din,dinten,dfree,headA")
	lv := songLevels()
	total := lead + songLen + tail
	frames := int(total.Seconds() * 60)
	lastN, saved := -1, 0
	for i := 0; i < frames; i++ {
		el := time.Duration(i) * time.Second / 60
		switch {
		case size != "stage" && i == int(lead.Seconds()*60):
			g.music(true)
		case size != "stage" && i == int((lead+songLen).Seconds()*60):
			u.track.Playing, u.track.PausedAt = false, g.clock
		}
		if el >= lead && el < lead+songLen {
			n := int((el - lead).Seconds() / 0.04)
			if n != lastN && n < len(lv) {
				lastN = n
				u.audio.Feed(lv[n], time.Now())
			}
		}
		g.run(time.Second / 60)
		b, gr := u.body, &u.groove
		fmt.Fprintf(tr, "%.4f,%v,%.3f,%.4f,%d,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%d,%.2f,%.2f,%.2f,%.3f,%.3f,%.3f,%.1f\n",
			el.Seconds(), u.dancing, gr.lock, gr.phase, gr.move, gr.energy, b.hold, b.danceSq, b.lean.x, b.sq.x, b.y, b.tip.x, b.tipH.x,
			b.phones.y, b.phones.tilt.x, ps.lift, ps.sq, ps.lean, ps.tip, ps.tipH, ps.eyes, u.ex.x, u.ey.x, u.es.x, u.dancer.in, u.dancer.inten, u.dancer.free, b.headA)
		if i%2 != 1 {
			continue
		}
		var crop image.Rectangle
		x, y := int(u.ex.x), int(u.ey.x)
		switch size {
		case "pill":
			crop = image.Rect(x-18, 0, x+22, 34)
		case "card":
			crop = image.Rect(x-26, y-34, x+26, y+20)
		case "stage":
			crop = image.Rect(x-80, y-100, x+80, y+60)
		}
		img := onDesktop(u, crop)
		savePNG(t, filepath.Join(dir, fmt.Sprintf("%04d.png", saved)), img)
		saved++
	}
}
