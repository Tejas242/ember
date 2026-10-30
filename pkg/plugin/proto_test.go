package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDecode(t *testing.T) {
	long := strings.Repeat("é", 100)
	m, err := Decode([]byte(`{"type":"status","text":"` + long + "\",\"sub\":\"a\\nb\",\"mood\":\"ecstatic\",\"icon\":\"rocket\",\"progress\":7,\"urgency\":\"panic\",\"until_ms\":-5,\"new_field\":{}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := m.Status
	if utf8.RuneCountInString(s.Text) != 40 || !strings.HasSuffix(s.Text, "…") || s.Sub != "a b" || s.Mood != Work || s.Icon != "" ||
		*s.Progress != 1 || s.Urgency != "normal" || s.UntilMs != 0 {
		t.Fatalf("%+v", s)
	}
	if m.Cap() != CapPill {
		t.Fatal(m.Cap())
	}

	m, err = Decode([]byte(`{"type":"card","id":"c","title":"T","buttons":[{"key":"a","label":"A"},{"key":"","label":"B"},{"key":"c","label":"C"},{"key":"d","label":"D"},{"key":"e","label":"E"}],"ttl_s":9999}`))
	if err != nil || len(m.Card.Buttons) != 2 || m.Card.TTLs != 600 || m.Card.Mood != Nudge {
		t.Fatalf("%v %+v", err, m.Card)
	}
	m, _ = Decode([]byte(`{"type":"card","id":"c","title":"T"}`))
	if m.Card.TTLs != 7 {
		t.Fatal("a card without buttons goes after 7 s")
	}

	m, err = Decode([]byte(`{"type":"dashboard","widgets":[{"kind":"hologram"},{"kind":"bars","items":[{"label":"a","value":3},{"label":"b","value":-1},{"label":"c","value":"x"}]},{"kind":"stats","items":[{"label":"Today","value":3},{"label":"Best","value":"5 days"}]},{"kind":"chips","items":[{"label":"ABCDE","state":"weird"}]}]}`))
	if err != nil || len(m.Dashboard.Widgets) != 3 {
		t.Fatalf("%v %+v", err, m.Dashboard)
	}
	bars, stats, chips := m.Dashboard.Widgets[0], m.Dashboard.Widgets[1], m.Dashboard.Widgets[2]
	if bars.Items[0].Num != 3 || bars.Items[1].Num != 0 || bars.Items[2].Num != 0 || stats.Items[0].Text != "3" ||
		stats.Items[1].Text != "5 days" || chips.Items[0].Label != "ABC…" || chips.Items[0].State != "todo" {
		t.Fatalf("%+v %+v %+v", bars, stats, chips)
	}

	for line, want := range map[string]string{
		`not json`:                            "not a JSON object",
		`[1,2]`:                               "not a JSON object",
		`{"text":"x"}`:                        "no type",
		`{"type":"status"}`:                   "needs text",
		`{"type":"status","text":5}`:          "status",
		`{"type":"cheer","kind":"dance"}`:     "hop, nod or celebrate",
		`{"type":"remove","what":"card"}`:     "needs its id",
		`{"type":"remove","what":"pill"}`:     "status, row, card or dashboard",
		`{"type":"hello","v":0,"name":"x"}`:   "v ≥ 1",
		`{"type":"hello","v":1,"name":"Bad"}`: "a name",
		`{"type":"card","title":"no id"}`:     "an id and a title",
	} {
		if _, err := Decode([]byte(line)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", line, err, want)
		}
	}
	if _, err := Decode([]byte(`{"type":"teleport"}`)); !errors.Is(err, ErrIgnored) {
		t.Fatal("an unknown type is ignored")
	}
	if _, err := Decode([]byte(`{"type":"log","text":"` + strings.Repeat("x", MaxLine) + `"}`)); err == nil {
		t.Fatal("a line over 64 KiB")
	}
}

// Every example in docs/PLUGINS.md decodes.
func TestDocExamples(t *testing.T) {
	b, err := os.ReadFile("../../docs/PLUGINS.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```json\n(.*?)```").FindAllStringSubmatch(string(b), -1)
	n := 0
	for _, bl := range blocks {
		text := strings.ReplaceAll(bl[1], "\n ", " ")
		text = strings.ReplaceAll(text, "\n  {", " {")
		text = strings.ReplaceAll(text, "\n]}", "]}")
		for _, ln := range strings.Split(strings.TrimSpace(text), "\n") {
			var head struct{ Type string }
			if json.Unmarshal([]byte(ln), &head) != nil {
				t.Errorf("not one JSON line: %s", ln)
				continue
			}
			switch head.Type {
			case "hello":
				var h HelloEmber
				if json.Unmarshal([]byte(ln), &h); h.Ember != "" {
					continue // Ember's side
				}
			}
			if _, err := Decode([]byte(ln)); err != nil {
				t.Errorf("%s: %v", ln, err)
			}
			n++
		}
	}
	if n < 6 {
		t.Fatalf("only %d examples", n)
	}
}

// The schema's maxLengths are the decoder's.
func TestSchemaLimits(t *testing.T) {
	b, err := os.ReadFile("../../docs/plugin-protocol.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				MaxLength int `json:"maxLength"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &schema); err != nil {
		t.Fatal(err)
	}
	base := map[string]map[string]any{
		"status": {"text": "x"}, "moment": {"text": "x"}, "row": {"title": "x"}, "card": {"id": "x", "title": "x"},
		"log": {"text": "x"}, "dashboard": {"widgets": []any{}},
	}
	checked := 0
	for typ, fields := range base {
		for prop, p := range schema.Defs[typ].Properties {
			if p.MaxLength == 0 || prop == "type" {
				continue
			}
			msg := map[string]any{"type": typ}
			for k, v := range fields {
				msg[k] = v
			}
			msg[prop] = strings.Repeat("a", p.MaxLength+20)
			if prop == "icon" {
				continue // an enum in disguise
			}
			line, _ := json.Marshal(msg)
			m, err := Decode(line)
			if err != nil {
				t.Errorf("%s.%s: %v", typ, prop, err)
				continue
			}
			var back map[string]any
			out, _ := json.Marshal(map[string]any{"status": m.Status, "moment": m.Moment, "row": m.Row, "card": m.Card, "log": m.Log, "dashboard": m.Dashboard}[typ])
			json.Unmarshal(out, &back)
			got, _ := back[prop].(string)
			if n := utf8.RuneCountInString(got); n != p.MaxLength {
				t.Errorf("%s.%s: schema says %d, the decoder keeps %d", typ, prop, p.MaxLength, n)
			}
			checked++
		}
	}
	if checked < 15 {
		t.Fatalf("checked %d", checked)
	}
}

func TestLine(t *testing.T) {
	b, err := Line("click", Event{Type: "click", Target: "docs"})
	if err != nil || string(b) != `{"target":"docs","type":"click"}`+"\n" {
		t.Fatalf("%s %v", b, err)
	}
}

func TestDecodeDashboardWidgets(t *testing.T) {
	m, err := Decode([]byte(`{"type":"dashboard","widgets":[
		{"kind":"hero","title":"1 to go today","ring":{"value":0,"goal":1,"mode":"spin"},"ember":{"pose":"dancing","react":"hop"},
		 "step":{"tag":"Upsolve","text":"2261B","dots":{"used":9,"ready":2,"total":12}},"buttons":[{"key":"a","label":"A"},{"key":"b","label":"B"},{"key":"c","label":"C"}],"fold":true},
		{"kind":"hero","title":"a second hero"},
		{"kind":"columns","split":0.9,"a":{"kind":"bars","layout":"columns","items":[{"label":"M","value":1,"target":2,"today":true,"state":"future"}]},
		 "b":{"kind":"agenda","items":[{"at_ms":1792234567000,"title":"CF Div. 2","chips":[{"label":"B","state":"owed"},{"label":"C"},{"label":"D"},{"label":"E"},{"label":"F"}]},{"title":"no time"}]},"fold":"auto","summary":"the week"},
		{"kind":"columns","a":{"kind":"columns","a":{"kind":"note","text":"x"},"b":{"kind":"note","text":"y"}},"b":{"kind":"note","text":"z"}},
		{"kind":"stats","cols":9,"count_up":true,"items":[{"label":"Solo","value":83.3,"unit":"%"},{"label":"Focus","value":6780,"unit":"s"}]},
		{"kind":"heatmap","weeks":200,"days":[{"date":"2026-10-20","level":3,"mood":"wait"}]},
		{"kind":"line","values":[1,2],"band":{"lo":5,"hi":1}},
		{"kind":"note","text":""}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	ws := m.Dashboard.Widgets
	if len(ws) != 5 {
		t.Fatalf("%d widgets: the second hero, nested columns and an empty note go", len(ws))
	}
	h := ws[0]
	if h.Ring.Mode != "fill" || h.Ember.Pose != "calm" || h.Ember.React != "hop" || h.Step.Dots.Used != 5 || h.Step.Dots.Total != 5 || len(h.Buttons) != 2 || h.Folded != "no" {
		t.Fatalf("hero %+v %+v %+v", h.Ring, h.Ember, h.Step.Dots)
	}
	c := ws[1]
	if c.Split != 0.7 || c.Folded != "auto" || c.A.Layout != "columns" || *c.A.Items[0].Target != 2 || !c.A.Items[0].Today || c.A.Items[0].State != "future" ||
		len(c.B.Items) != 1 || len(c.B.Items[0].Chips) != 4 || c.B.Items[0].Chips[0].State != "owed" {
		t.Fatalf("columns %+v %+v %+v", c, c.A, c.B)
	}
	if s := ws[2]; s.Cols != 4 || !s.CountUp || s.Items[0].Num != 83.3 || s.Items[1].Unit != "s" {
		t.Fatalf("stats %+v", s)
	}
	if hm := ws[3]; hm.Weeks != 53 || hm.Days[0].Level != 1 || hm.Days[0].Mood != "wait" {
		t.Fatalf("heatmap %+v", hm)
	}
	if ln := ws[4]; ln.Band != nil || len(ln.Values) != 2 {
		t.Fatalf("line %+v", ln)
	}
}

// A card's body keeps its paragraphs; every other text field is one line.
func TestCardBodyKeepsLines(t *testing.T) {
	m, err := Decode([]byte(`{"type":"card","id":"x","title":"A\nB","body":"WA on test 3.\r\nGive up\tand learn it?\n"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Card.Body != "WA on test 3.\nGive up and learn it?\n" || m.Card.Title != "A B" {
		t.Fatalf("%q %q", m.Card.Body, m.Card.Title)
	}
}

// A list's and an agenda's rows carry a key; other widgets' items don't.
func TestItemKeys(t *testing.T) {
	m, err := Decode([]byte(`{"type":"dashboard","widgets":[
		{"kind":"list","items":[{"title":"a","key":"open:1"}]},
		{"kind":"agenda","items":[{"title":"b","at_ms":1,"key":"upsolve:cf:2261` + strings.Repeat("9", 140) + `"}]},
		{"kind":"bars","items":[{"label":"c","value":1,"key":"x"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	ws := m.Dashboard.Widgets
	if ws[0].Items[0].Key != "open:1" || utf8.RuneCountInString(ws[1].Items[0].Key) != KeyMax || ws[2].Items[0].Key != "" {
		t.Fatalf("%q %q %q", ws[0].Items[0].Key, ws[1].Items[0].Key, ws[2].Items[0].Key)
	}
}
