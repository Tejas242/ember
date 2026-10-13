package island

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// EMBER_PREVIEW=dir: a filmstrip of the pill opening into the music card.
func TestFilmstrip(t *testing.T) {
	dir := os.Getenv("EMBER_PREVIEW")
	if dir == "" {
		t.Skip()
	}
	clock := time.Date(2026, 10, 11, 21, 30, 0, 0, time.Local)
	u := NewUI(440, 280, 22)
	u.now = func() time.Time { return clock }
	u.track = &Track{Player: "org.mpris.MediaPlayer2.YoutubeMusic", Title: "Veridis Quo", Artist: "Daft Punk", Playing: true, Length: 5 * time.Minute, Position: 2 * time.Minute, PosAt: clock}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.NRGBA{uint8(120 + x*2), uint8(40 + y), uint8(160 - x), 255})
		}
	}
	u.art = &albumArt{small: scaleSquare(img, 18), large: scaleSquare(img, 60), row: scaleSquare(img, 30)}
	settle(u, &clock)
	u.pinned = true
	frames := []int{0, 2, 4, 6, 9, 14, 30}
	strip := image.NewRGBA(image.Rect(0, 0, 440, 230*len(frames)))
	draw.Draw(strip, strip.Rect, image.NewUniform(color.NRGBA{0x22, 0x33, 0x26, 255}), image.Point{}, draw.Src)
	f := 0
	for i, n := range frames {
		for ; f < n; f++ {
			clock = clock.Add(time.Second / 60)
			u.renderDamage()
		}
		draw.Draw(strip, image.Rect(0, 230*i, 440, 230*i+22), image.NewUniform(color.NRGBA{0x0f, 0x15, 0x0e, 255}), image.Point{}, draw.Over)
		draw.Draw(strip, image.Rect(0, 230*i, 440, 230*i+230), u.cv.img, image.Point{}, draw.Over)
	}
	out, _ := os.Create(filepath.Join(dir, "film.png"))
	png.Encode(out, strip)
	out.Close()
}
