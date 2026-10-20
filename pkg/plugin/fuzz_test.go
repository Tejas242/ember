package plugin

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzDecode: no line makes Decode panic; whatever it accepts is within the
// limits, and decodes again to the same thing once written back out.
func FuzzDecode(f *testing.F) {
	if b, err := os.ReadFile("../../docs/PLUGINS.md"); err == nil {
		for _, m := range regexp.MustCompile(`\{"type":[^\n]*\}`).FindAllString(string(b), -1) {
			f.Add([]byte(m))
		}
	}
	for _, s := range []string{
		`{"type":"status","text":"` + strings.Repeat("é", 50) + `","progress":1e309}`,
		`{"type":"dashboard","widgets":[{"kind":"bars","items":[{"label":"a","value":"NaN"}]},{"kind":"sparkline","values":[1e308,-1e308]}]}`,
		`{"type":"card","id":"x","title":"t","buttons":[{"key":"\u0000","label":"\ud800"}]}`,
		`{"type":"remove","what":"card","id":""}`, `{"type":"hello","v":1,"name":"a"}`, `{}`, `null`, `[]`, `"x"`, "{\"type\":\"log\",\"text\":\"\xff\xfe\"}",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, line []byte) {
		m, err := Decode(line)
		if err != nil { // refused or ignored: fine, as long as it didn't panic
			return
		}
		var v any
		switch m.Type {
		case "status":
			s := m.Status
			if runes(s.Text) > 40 || runes(s.Sub) > 40 || s.Text == "" || (s.Progress != nil && (*s.Progress < 0 || *s.Progress > 1)) {
				t.Fatalf("status out of limits: %+v", s)
			}
			v = s
		case "moment":
			if mo := m.Moment; runes(mo.Text) > 40 || mo.TTLs < 1 || mo.TTLs > 10 {
				t.Fatalf("moment out of limits: %+v", mo)
			}
			v = m.Moment
		case "row":
			v = m.Row
		case "card":
			c := m.Card
			if len(c.Buttons) > 3 || runes(c.Body) > 280 || c.TTLs < 5 || c.TTLs > 600 {
				t.Fatalf("card out of limits: %+v", c)
			}
			v = c
		case "dashboard":
			d := m.Dashboard
			if len(d.Widgets) > 12 {
				t.Fatalf("%d widgets", len(d.Widgets))
			}
			for _, w := range d.Widgets {
				if len(w.Items) > 12 || len(w.Values) > 120 {
					t.Fatalf("widget out of limits: %+v", w)
				}
			}
			return // Item's decoded Num/Text don't write back out: checked above
		case "log":
			if runes(m.Log.Text) > 500 {
				t.Fatal("log over 500")
			}
			v = m.Log
		default:
			return
		}
		for _, s := range strings.Fields(strings.ReplaceAll(string(mustJSON(t, v)), `"`, " ")) {
			if !utf8.ValidString(s) {
				t.Fatalf("invalid UTF-8 kept: %q", s)
			}
		}
		again, err := Decode(append(mustLine(t, m.Type, v), '\n'))
		if err != nil {
			t.Fatalf("what Decode accepted doesn't decode again: %v", err)
		}
		if a, b := string(mustJSON(t, v)), string(mustJSON(t, field(again))); a != b {
			t.Fatalf("not stable:\n%s\n%s", a, b)
		}
	})
}

func runes(s string) int { return utf8.RuneCountInString(s) }

func mustJSON(t *testing.T, v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustLine(t *testing.T, typ string, v any) []byte {
	b, err := Line(typ, v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func field(m Message) any {
	switch m.Type {
	case "status":
		return m.Status
	case "moment":
		return m.Moment
	case "row":
		return m.Row
	case "card":
		return m.Card
	case "log":
		return m.Log
	}
	return nil
}
