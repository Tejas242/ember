package island

import (
	"image"
	"image/color"
	"testing"
)

func TestAccentOf(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 46, 46))
	for i := 0; i < len(img.Pix); i += 4 { // mostly dark grey, a red third
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 40, 40, 40, 255
		if i%12 == 0 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = 200, 30, 40
		}
	}
	a := accentOf(img)
	if a.R < 200 || a.G > 160 || a.B > 160 {
		t.Fatalf("want a light red, got %v", a)
	}
	grey := image.NewRGBA(image.Rect(0, 0, 46, 46))
	for i := range grey.Pix {
		grey.Pix[i] = 128
	}
	if accentOf(grey) != (color.NRGBA{}) {
		t.Fatal("a grey cover has no accent")
	}
}
