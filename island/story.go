package island

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// What the card tells: each session's story (the steps of the current turn,
// what it changed, and a recap when it's done) and the day's (how much got
// done, when). Kept small: six steps a session, one day of hours.

// Step is one tool call, written as it reads in a log: past tense.
type Step struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"` // read | edit | run | search | web | agent | other
	Text   string    `json:"text"`
	Plus   int       `json:"plus,omitempty"`
	Minus  int       `json:"minus,omitempty"`
	Done   bool      `json:"done,omitempty"`
	Failed bool      `json:"failed,omitempty"`
}

// Turn is what one prompt has led to so far.
type Turn struct {
	Steps       []Step          `json:"steps"`
	Count       int             `json:"count"` // all steps, not just the kept six
	Files       map[string]bool `json:"files"`
	Plus, Minus int
	Failures    int
}

// Recap is a finished turn, for the card to tell afterwards.
type Recap struct {
	Summary     string        `json:"summary"`
	Failed      bool          `json:"failed,omitempty"` // it stopped on an error (Error says which)
	Error       string        `json:"error,omitempty"`
	At          time.Time     `json:"at"`
	Took        time.Duration `json:"took"`
	Steps       int           `json:"steps"`
	Files       int           `json:"files"`
	Plus, Minus int
	Failures    int
}

const keepSteps = 6

// step records the start of a tool call; the previous one is over.
func (t *Turn) step(now time.Time, tool string, in map[string]any) {
	if n := len(t.Steps); n > 0 {
		t.Steps[n-1].Done = true
	}
	kind, text := pastStep(tool, in)
	_, plus, minus := activity(tool, in)
	t.Steps = append(t.Steps, Step{At: now, Kind: kind, Text: text, Plus: plus, Minus: minus})
	if len(t.Steps) > keepSteps {
		t.Steps = t.Steps[len(t.Steps)-keepSteps:]
	}
	t.Count++
	if kind == "edit" {
		if f, _ := in["file_path"].(string); f != "" {
			if t.Files == nil {
				t.Files = map[string]bool{}
			}
			t.Files[f] = true
		}
		t.Plus, t.Minus = t.Plus+plus, t.Minus+minus
	}
}

// finish marks the current step over; failed if the tool said so.
func (t *Turn) finish(failed bool) {
	if n := len(t.Steps); n > 0 && !t.Steps[n-1].Done {
		t.Steps[n-1].Done, t.Steps[n-1].Failed = true, failed
		if failed {
			t.Failures++
		}
	}
}

// pastStep describes a tool call as a log line, and what kind of thing it was.
func pastStep(tool string, in map[string]any) (kind, text string) {
	str := func(k string) string { v, _ := in[k].(string); return v }
	base := filepath.Base(str("file_path"))
	switch tool {
	case "Read":
		return "read", "Read " + base
	case "Edit", "MultiEdit":
		return "edit", "Edited " + base
	case "Write":
		return "edit", "Wrote " + base
	case "NotebookEdit":
		return "edit", "Edited " + filepath.Base(str("notebook_path"))
	case "Bash":
		if d := str("description"); d != "" {
			return "run", d
		}
		return "run", "Ran " + firstLine(str("command"))
	case "Grep", "Glob":
		return "search", "Searched for " + str("pattern")
	case "WebFetch":
		if u, err := url.Parse(str("url")); err == nil && u.Host != "" {
			return "web", "Read " + u.Host
		}
		return "web", "Read a page"
	case "WebSearch":
		if q := str("query"); q != "" {
			return "web", "Searched the web for " + q
		}
		return "web", "Searched the web"
	case "Task", "Agent":
		return "agent", "Asked an agent: " + str("description")
	case "Skill":
		return "other", "Used " + str("skill")
	case "TodoWrite", "EnterPlanMode":
		return "other", "Planned"
	}
	label, _, _ := activity(tool, in)
	return "other", label
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ---- the day ----

// Day is today's record: tasks finished, time Claude spent on them, tool
// calls per hour, and where.
type Day struct {
	Date     string         `json:"date"`
	Tasks    int            `json:"tasks"`
	Busy     time.Duration  `json:"busy"`
	Hours    [24]int        `json:"hours"`
	Projects map[string]int `json:"projects"`
	dirty    bool
}

func dayPath() string { return filepath.Join(filepath.Dir(habitsPath()), "today.json") }

func loadDay(now time.Time) *Day {
	d := &Day{}
	if b, err := os.ReadFile(dayPath()); err == nil {
		json.Unmarshal(b, d)
	}
	d.roll(now)
	return d
}

// roll starts a new day at midnight.
func (d *Day) roll(now time.Time) {
	if today := now.Format("2006-01-02"); d.Date != today {
		*d = Day{Date: today, Projects: map[string]int{}, dirty: true}
	}
	if d.Projects == nil {
		d.Projects = map[string]int{}
	}
}

func (d *Day) call(now time.Time) {
	d.roll(now)
	d.Hours[now.Hour()]++
	d.dirty = true
}

func (d *Day) done(now time.Time, project string, took time.Duration) {
	d.roll(now)
	d.Tasks++
	d.Busy += took
	d.Projects[project]++
	d.dirty = true
}

func (d *Day) save() {
	if !d.dirty {
		return
	}
	d.dirty = false
	os.MkdirAll(filepath.Dir(dayPath()), 0o755)
	if b, err := json.Marshal(d); err == nil {
		os.WriteFile(dayPath(), b, 0o644)
	}
}

// greeting knows what time it is.
func greeting(now time.Time) string {
	switch h := now.Hour(); {
	case h < 5:
		return "Up late"
	case h < 12:
		return "Good morning"
	case h < 17:
		return "Good afternoon"
	case h < 22:
		return "Good evening"
	}
	return "Late night"
}

// gerund turns an imperative step ("Run the tests") into what's happening
// now ("running the tests") for the headline. Only well-known verbs: a
// wrong guess reads worse than none.
func gerund(label string) (string, bool) {
	verb, rest, _ := strings.Cut(label, " ")
	lv := strings.ToLower(verb)
	if strings.HasSuffix(lv, "ing") && len(lv) > 4 {
		return strings.ToLower(verb[:1]) + verb[1:] + " " + rest, true
	}
	irregular := map[string]string{"run": "running", "set": "setting", "get": "getting", "stop": "stopping",
		"put": "putting", "plan": "planning", "commit": "committing", "write": "writing", "make": "making",
		"remove": "removing", "rename": "renaming", "update": "updating", "create": "creating", "use": "using",
		"move": "moving", "compare": "comparing", "configure": "configuring", "measure": "measuring",
		"prepare": "preparing", "save": "saving", "resolve": "resolving", "merge": "merging", "rebase": "rebasing",
		"restore": "restoring", "type": "typing", "debug": "debugging"}
	regular := "add build check clean count delete deploy download edit export fetch find fix format generate grep " +
		"inspect install launch lint list load look open patch print profile pull push read refactor reload render " +
		"replace request reset restart revert scan search show sort start test trace verify view watch"
	g, ok := irregular[lv]
	if !ok && strings.Contains(" "+regular+" ", " "+lv+" ") {
		g, ok = lv+"ing", true
	}
	if !ok {
		return "", false
	}
	if rest == "" {
		return g, true
	}
	return g + " " + rest, true
}

// task is what a session is about, in words, for the headline: what you
// asked this turn if that says something ("yes", "go on" don't), else the
// topic Claude gave the session, else nothing.
func (s *Session) task() string {
	p := strings.Join(strings.Fields(s.Prompt), " ")
	if words := strings.Fields(p); len(words) >= 4 && len(p) >= 18 && !strings.HasPrefix(p, "/") {
		if i := strings.IndexAny(p, ".?!"); i >= 18 { // the first sentence is enough
			p = p[:i]
		}
		return strings.ToUpper(p[:1]) + p[1:]
	}
	return s.Title
}
