package island

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"time"

	xdraw "golang.org/x/image/draw"
)

// albumArt holds a track's cover pre-scaled to the sizes the island draws.
type albumArt struct {
	url          string
	small, large *image.RGBA // 18 px (pill), 46 px (card)
}

var artClient = &http.Client{Timeout: 8 * time.Second}

// loadArt fetches and scales a cover off the main loop.
func loadArt(raw string, done func(*albumArt)) {
	go func() {
		var data []byte
		u, err := url.Parse(raw)
		if err != nil {
			return
		}
		switch u.Scheme {
		case "file":
			data, err = os.ReadFile(u.Path)
		case "http", "https":
			var resp *http.Response
			if resp, err = artClient.Get(raw); err == nil {
				data, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
				resp.Body.Close()
			}
		default:
			return
		}
		if err != nil {
			return
		}
		src, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		art := &albumArt{url: raw, small: scaleSquare(src, 18), large: scaleSquare(src, 46)}
		src, data = nil, nil
		debug.FreeOSMemory() // a decoded cover can be tens of MB; give it back now
		done(art)
	}()
}

// scaleSquare centre-crops to a square and scales to n×n.
func scaleSquare(src image.Image, n int) *image.RGBA {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	crop := image.Rect(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2, 0, 0)
	crop.Max = crop.Min.Add(image.Pt(side, side))
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	xdraw.CatmullRom.Scale(dst, dst.Rect, src, crop, xdraw.Src, nil)
	return dst
}

// drawArt draws a cover with rounded corners at (x, y).
func (c *canvas) drawArt(img *image.RGBA, x, y int, rad float32, alpha float64) {
	if img == nil {
		return
	}
	n := img.Rect.Dx()
	mask := roundMask(n, rad, alpha)
	draw.DrawMask(c.img, image.Rect(x, y, x+n, y+n), img, image.Point{}, mask, image.Point{}, draw.Over)
}

type maskKey2 struct {
	n     int
	rad   float32
	alpha uint8
}

var roundMasks = map[maskKey2]*image.Alpha{}

func roundMask(n int, rad float32, alpha float64) *image.Alpha {
	k := maskKey2{n, rad, uint8(alpha * 255)}
	if m := roundMasks[k]; m != nil {
		return m
	}
	tmp := newCanvas(n, n)
	tmp.roundRect(0, 0, float32(n), float32(n), rad, mixAlpha(colorWhite, alpha))
	m := image.NewAlpha(image.Rect(0, 0, n, n))
	for i := 0; i < n*n; i++ {
		m.Pix[i] = tmp.img.Pix[i*4+3]
	}
	roundMasks[k] = m
	return m
}
