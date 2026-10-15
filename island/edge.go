package island

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// The edge plugin: edge (github.com/Tejas242/edge) is a practice coach. It
// pushes its day's status and its cards over Ember's socket, keeps a live
// stats file (STATS.md in edge's repo), and its buttons call `edge action`.

type edgePlugin struct {
	stop chan struct{}
}

func (*edgePlugin) Name() string { return "edge" }

func (*edgePlugin) Detect() bool {
	if _, err := exec.LookPath("edge"); err == nil {
		return true
	}
	_, err := os.Stat(edgeStatsPath())
	return err == nil
}

func (e *edgePlugin) Start(a *app) {
	e.stop = make(chan struct{})
	go watchFile(edgeStatsPath(), 3*time.Second, e.stop, func(b []byte) {
		var st EdgeStats
		if json.Unmarshal(b, &st) == nil && st.Version >= 1 {
			a.posts <- func(a *app) { a.ui.edgeStats = &st }
		}
	})
}

func (e *edgePlugin) Stop(a *app) {
	if e.stop != nil {
		close(e.stop)
		e.stop = nil
	}
	a.ui.edge, a.ui.edgeStats = nil, nil
	for _, c := range append([]*Card(nil), a.ui.cards...) {
		if c.Kind == "edge" {
			a.ui.RemoveCard(c.ID)
		}
	}
}

func (*edgePlugin) Dash() Dashboard { return edgeDash{} }

// edgeCardLife: edge never takes a card back, so one you answered in the
// terminal or ignored would hold the island forever. A note with buttons
// stays this long; edge says it again if it still matters.
const edgeCardLife = 3 * time.Minute

func (*edgePlugin) Message(a *app, raw json.RawMessage) {
	var msg struct {
		Type   string      `json:"type"`
		Status *EdgeStatus `json:"status"`
		Stats  *EdgeStats  `json:"stats"`
		ID     string      `json:"id"`
		Card   *edgeCard   `json:"card"`
	}
	if json.Unmarshal(raw, &msg) != nil {
		return
	}
	switch msg.Type {
	case "status":
		a.ui.edge = msg.Status
	case "stats":
		if msg.Stats != nil && msg.Stats.Version >= 1 {
			a.ui.edgeStats = msg.Stats
		}
	case "remove":
		a.ui.RemoveCard("edge:" + msg.ID)
	case "card":
		w := msg.Card
		if w == nil {
			return
		}
		moods := map[string]mood{"work": moodWork, "wait": moodWait, "happy": moodHappy, "error": moodError, "nudge": moodNudge}
		c := &Card{ID: "edge:" + w.ID, Kind: "edge", Mood: moods[w.Mood], Eyebrow: "Practice", Title: w.Title, Body: w.Body, Priority: 2}
		for _, b := range w.Buttons {
			c.Buttons = append(c.Buttons, Button{Key: b.Key, Label: b.Label, Primary: b.Primary})
		}
		if w.Urgent {
			c.Priority = 5
		}
		c.Expires = a.ui.now().Add(edgeCardLife)
		if len(c.Buttons) == 0 {
			c.Expires = a.ui.now().Add(7 * time.Second)
		}
		c.OnPick = func(k string, _ []int) {
			if k != "dismiss" {
				edgeAction(k)
			}
		}
		a.ui.AddCard(c)
		if w.Bounce {
			a.ui.Bounce()
		}
	}
}

// edgeAction runs `edge action <key>` (edge's buttons), if edge is installed.
func edgeAction(key string) {
	if _, err := exec.LookPath("edge"); err == nil {
		go exec.Command("edge", "action", key).Run()
	}
}

func edgeStatsPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "edge", "stats.json")
}

// watchFile calls got with the file's contents now and whenever its mtime
// changes, until stop closes. edge rewrites the file atomically and only
// when something changed, so a cheap stat every few seconds is plenty.
func watchFile(path string, every time.Duration, stop <-chan struct{}, got func([]byte)) {
	var seen time.Time
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if m := mtime(path); !m.IsZero() && !m.Equal(seen) {
			if b, err := os.ReadFile(path); err == nil {
				seen = m
				got(b)
			}
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

// EdgeStatus is what edge reports about the day, for the pill and the card.
type EdgeStatus struct {
	Today     int    `json:"today"`
	Need      int    `json:"need"`
	Streak    int    `json:"streak"`
	Closed    bool   `json:"closed"`
	Task      string `json:"task"`
	URL       string `json:"url"`
	StartedMs int64  `json:"startedMs"`
	Pending   bool   `json:"pending"`
}

type edgeCard struct {
	ID      string `json:"id"`
	Mood    string `json:"mood"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Urgent  bool   `json:"urgent"`
	Bounce  bool   `json:"bounce"`
	Buttons []struct {
		Key     string `json:"key"`
		Label   string `json:"label"`
		Primary bool   `json:"primary"`
	} `json:"buttons"`
}

// EdgeStats is the part of edge's stats document (version 1) the dashboard
// draws. Fields are only ever added within a version; unknown ones are
// ignored.
type EdgeStats struct {
	Version   int       `json:"version"`
	Generated time.Time `json:"generated"`
	Today     struct {
		Date      string `json:"date"`
		Got       int    `json:"got"`
		Need      int    `json:"need"`
		Done      bool   `json:"done"`
		WorkSec   int    `json:"work_sec"`
		StudyTime bool   `json:"study_time"`
		StudyFrom string `json:"study_from"`
		Gate      string `json:"gate"`     // closed | open | off
		GateWhy   string `json:"gate_why"` // done | pass | outside_study_time | not_enforcing | edge_trouble | disabled
	} `json:"today"`
	Current *struct {
		ID           string    `json:"id"`
		Source       string    `json:"source"`
		Kind         string    `json:"kind"` // new | resolve | reentry
		Title        string    `json:"title"`
		Rating       int       `json:"rating"`
		Started      time.Time `json:"started"`
		WorkSec      int       `json:"work_sec"`
		Hints        int       `json:"hints"`
		HintsAllowed int       `json:"hints_allowed"`
		NextHintSec  int       `json:"next_hint_sec"`
		GiveUpInSec  int       `json:"give_up_in_sec"`
		Fails        int       `json:"fails"`
	} `json:"current"`
	Streak struct {
		Current int `json:"current"`
		Best    int `json:"best"`
	} `json:"streak"`
	Week struct {
		Credits int       `json:"credits"`
		Target  int       `json:"target"`
		Days    []EdgeDay `json:"days"`
	} `json:"week"`
	Calendar []EdgeDay `json:"calendar"`
	Totals   struct {
		Tasks    int `json:"tasks"`
		Solved   int `json:"solved"`
		Solo     int `json:"solo"`
		Assisted int `json:"assisted"`
		GaveUp   int `json:"gave_up"`
		WorkSec  int `json:"work_sec"`
		DaysMet  int `json:"days_met"`
	} `json:"totals"`
	Sources map[string]struct {
		Rating        int `json:"rating"`
		BandLo        int `json:"band_lo"`
		BandHi        int `json:"band_hi"`
		RatingHistory []struct {
			At      time.Time `json:"at"`
			Rating  int       `json:"rating"`
			Contest string    `json:"contest"`
		} `json:"rating_history"`
	} `json:"sources"`
	Topics []struct {
		Tag         string `json:"tag"`
		Source      string `json:"source"`
		FocusWeight int    `json:"focus_weight"`
		Tasks       int    `json:"tasks"`
		Solo        int    `json:"solo"`
	} `json:"topics"`
	Resolves struct {
		Due           int `json:"due"`
		Scheduled     int `json:"scheduled"`
		Graduated     int `json:"graduated"`
		FirstAttempts int `json:"first_attempts"`
		FirstRetained int `json:"first_retained"`
	} `json:"resolves"`
	Health []struct {
		Key string `json:"key"`
		Msg string `json:"msg"`
	} `json:"health"`
}

type EdgeDay struct {
	Date     string `json:"date"`
	Credits  int    `json:"credits"`
	Need     int    `json:"need"`
	Met      bool   `json:"met"`
	Pass     bool   `json:"pass"`
	WorkSec  int    `json:"work_sec"`
	CFSolved int    `json:"cf_solved"`
}
