package island

import "testing"

func TestDisplayFoldsStyledLetters(t *testing.T) {
	if got := display("𝙂𝘼𝙄𝙎𝙀 𝙃𝙐𝙒𝘼"); got != "GAISE HUWA" {
		t.Fatalf("got %q", got)
	}
	if display("plain") != "plain" {
		t.Fatal("ASCII passes through")
	}
	if display("Slow… −3 · ½") != "Slow… −3 · ½" {
		t.Fatal("ordinary typography stays")
	}
}

func TestFallbackDrawsMissingScripts(t *testing.T) {
	f := face(fRegular, 12)
	if covers(f, 'क') {
		t.Skip("Geist has Devanagari now")
	}
	if fallbackFace(f, 'क') == nil {
		t.Skip("no Devanagari font on this machine")
	}
	c := newCanvas(10, 10)
	m := c.mask(f, "गाना")
	ink := 0
	for _, a := range m.m.Pix {
		if a > 128 {
			ink++
		}
	}
	if ink < 30 || measure(f, "गाना") < 15 {
		t.Fatalf("Devanagari should draw through the fallback (ink %d)", ink)
	}
}
