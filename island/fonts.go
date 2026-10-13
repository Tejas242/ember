package island

import (
	"os"
	"os/exec"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/norm"
)

// Text from the outside world (track titles, window titles, Claude's words)
// isn't always Latin. Two things keep it readable:
//
//   - display folds compatibility characters: the "𝙂𝘼𝙄𝙎𝙀" styled letters
//     uploaders love, full-width forms, ligatures, become plain letters that
//     Geist draws crisply (Unicode NFKC);
//   - characters Geist doesn't have are drawn with a system font that does,
//     found once per Unicode block through fontconfig.

// display returns s as it should be drawn. ASCII, the common case, is free.
func display(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return norm.NFKC.String(s)
		}
	}
	return s
}

var (
	faceInfo  = map[font.Face]faceKey{}
	fallbacks = map[rune]*opentype.Font{} // by Unicode block (r>>7); nil = none found
	fbFaces   = map[fbKey]font.Face{}
	glyphBuf  sfnt.Buffer
)

type fbKey struct {
	f    *opentype.Font
	size float64
}

// covers reports whether the embedded font of face f has a glyph for r.
func covers(f font.Face, r rune) bool {
	if r < 0x80 || unicode.IsSpace(r) || unicode.IsControl(r) {
		return true
	}
	ft := fonts[faceInfo[f].name]
	if ft == nil {
		return true
	}
	i, err := ft.GlyphIndex(&glyphBuf, r)
	return err == nil && i != 0
}

// fallbackFace is a system font with r in it, at f's size; nil if none.
func fallbackFace(f font.Face, r rune) font.Face {
	block := r >> 7
	ft, tried := fallbacks[block]
	if !tried {
		ft = findFallback(r)
		fallbacks[block] = ft
	}
	if ft == nil {
		return nil
	}
	k := fbKey{ft, faceInfo[f].size}
	if fc := fbFaces[k]; fc != nil {
		return fc
	}
	fc, err := opentype.NewFace(ft, &opentype.FaceOptions{Size: k.size, DPI: 72, Hinting: font.HintingVertical})
	if err != nil {
		return nil
	}
	fbFaces[k] = fc
	return fc
}

func findFallback(r rune) *opentype.Font {
	out, err := exec.Command("fc-match", "-f", "%{file}", "sans-serif:charset="+hex(r)).Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	b, err := os.ReadFile(string(out))
	if err != nil {
		return nil
	}
	if c, err := opentype.ParseCollection(b); err == nil && c.NumFonts() > 0 { // .ttc
		for i := 0; i < c.NumFonts(); i++ {
			if ft, err := c.Font(i); err == nil {
				if gi, err := ft.GlyphIndex(&glyphBuf, r); err == nil && gi != 0 {
					return ft
				}
			}
		}
		return nil
	}
	ft, err := opentype.Parse(b)
	if err != nil {
		return nil
	}
	if gi, err := ft.GlyphIndex(&glyphBuf, r); err != nil || gi == 0 {
		return nil // fontconfig's best guess doesn't have it either
	}
	return ft
}

func hex(r rune) string {
	const digits = "0123456789abcdef"
	if r == 0 {
		return "0"
	}
	var b []byte
	for r > 0 {
		b = append([]byte{digits[r&15]}, b...)
		r >>= 4
	}
	return string(b)
}

// runs splits s into stretches drawn with the same face.
func runs(f font.Face, s string, each func(f font.Face, s string)) {
	start, cur := 0, f
	for i, r := range s {
		fc := f
		if !covers(f, r) {
			if fb := fallbackFace(f, r); fb != nil {
				fc = fb
			}
		}
		if fc != cur {
			if i > start {
				each(cur, s[start:i])
			}
			start, cur = i, fc
		}
	}
	if start < len(s) {
		each(cur, s[start:])
	}
}

// advance is the width of s in f, with fallbacks.
func advance(f font.Face, s string) fixed.Int26_6 {
	var w fixed.Int26_6
	runs(f, s, func(fc font.Face, part string) { w += font.MeasureString(fc, part) })
	return w
}
