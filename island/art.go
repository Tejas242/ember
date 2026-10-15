package island

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
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
	small, large *image.RGBA // 14 px (pill: a disc in a progress ring), 60 px (source of the rest)
	row          *image.RGBA // 24 px (the card's music section)
	accent       color.NRGBA // the cover's colour, lifted to read on the dark island; zero if it's grey
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
		art := &albumArt{url: raw, small: scaleSquare(src, 14), large: scaleSquare(src, 60), row: scaleSquare(src, 24)}
		art.accent = accentOf(art.large)
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

// accentOf finds a cover's most prominent vivid hue (by saturation-weighted
// area) and brings it to a lightness that reads on the dark island.
func accentOf(img *image.RGBA) color.NRGBA {
	const buckets = 24
	var weight [buckets]float64
	var sum [buckets][3]float64
	for i := 0; i+3 < len(img.Pix); i += 4 {
		r, g, b := float64(img.Pix[i])/255, float64(img.Pix[i+1])/255, float64(img.Pix[i+2])/255
		h, s, v := rgbToHSV(r, g, b)
		if s < 0.25 || v < 0.22 {
			continue // greys, near-black
		}
		k := int(h*buckets) % buckets
		w := s * s * v
		weight[k] += w
		sum[k][0] += r * w
		sum[k][1] += g * w
		sum[k][2] += b * w
	}
	best, total := 0, 0.0
	for k := range weight {
		// a hue's neighbours count a little: covers are rarely one flat colour
		w := weight[k] + 0.5*(weight[(k+1)%buckets]+weight[(k+buckets-1)%buckets])
		if w > total {
			best, total = k, w
		}
	}
	if total < 0.02*float64(len(img.Pix)/4) {
		return color.NRGBA{}
	}
	r, g, b := sum[best][0]/weight[best], sum[best][1]/weight[best], sum[best][2]/weight[best]
	h, s, l := rgbToHSL(r, g, b)
	s = math.Min(math.Max(s, 0.5), 0.85)
	l = math.Min(math.Max(l, 0.64), 0.76)
	r, g, b = hslToRGB(h, s, l)
	return color.NRGBA{uint8(r*255 + 0.5), uint8(g*255 + 0.5), uint8(b*255 + 0.5), 255}
}

func rgbToHSV(r, g, b float64) (h, s, v float64) {
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	v = mx
	if mx == 0 {
		return 0, 0, 0
	}
	s = (mx - mn) / mx
	h, _, _ = rgbToHSL(r, g, b)
	return h, s, v
}

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	d := mx - mn
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h /= 6
	if h < 0 {
		h++
	}
	return h, s, l
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h*6, 2)-1))
	m := l - c/2
	switch int(h*6) % 6 {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}
