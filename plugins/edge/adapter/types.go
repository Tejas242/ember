// Package adapter is edge's plugin for Ember: it turns what edge (a practice
// coach, github.com/Tejas242/edge) says over Ember's socket, and the stats
// file it keeps, into protocol v1 messages: the pill's line, the card's row,
// edge's cards, Ember's reactions, and the dashboard. Its buttons run `edge
// action <key>`. Ember draws all of it in its own style; nothing here knows
// a pixel.
package adapter

import (
	"strings"
	"time"
)

// Status is what edge reports about the day (its "status" message).
type Status struct {
	Today     int    `json:"today"`
	Need      int    `json:"need"`
	Streak    int    `json:"streak"`
	Closed    bool   `json:"closed"`
	Task      string `json:"task"`
	URL       string `json:"url"`
	StartedMs int64  `json:"startedMs"`
	Pending   bool   `json:"pending"`
	Round     *Round `json:"round"` // a contest round that matters now
}

// Round is the round in edge's status.
type Round struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Short    string    `json:"short"` // "CF 1128", "ABC 480", "Weekly 524"
	URL      string    `json:"url"`
	StartMs  int64     `json:"startMs"`
	EndMs    int64     `json:"endMs"`
	Plan     string    `json:"plan"`   // in | out | "" (open)
	Joined   string    `json:"joined"` // live | virtual | ""
	Locked   bool      `json:"locked"` // Claude is off for it
	Settled  bool      `json:"settled"`
	Problems []Problem `json:"problems"`
}

// Problem is one letter of a round.
type Problem struct {
	Index string `json:"index"`
	State string `json:"state"` // "" | tried | solved | upsolve | upsolved | taught | dropped | lapsed
	AtSec int    `json:"at_sec"`
	Fails int    `json:"fails"`
}

func (r *Round) start() time.Time { return time.UnixMilli(r.StartMs) }
func (r *Round) end() time.Time   { return time.UnixMilli(r.EndMs) }

// phase: soon | live | over.
func (r *Round) phase(now time.Time) string {
	switch {
	case now.Before(r.start()):
		return "soon"
	case now.Before(r.end()):
		return "live"
	}
	return "over"
}

// Card is one of edge's cards.
type Card struct {
	ID      string `json:"id"`
	Mood    string `json:"mood"` // work | wait | happy | error | nudge | idle
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

// Message is one of edge's messages over Ember's socket.
type Message struct {
	Type   string  `json:"type"` // status | stats | card | remove
	Status *Status `json:"status"`
	Stats  *Stats  `json:"stats"`
	ID     string  `json:"id"`
	Card   *Card   `json:"card"`
}

// Day is one day in edge's stats.
type Day struct {
	Date     string `json:"date"`
	Credits  int    `json:"credits"`
	Need     int    `json:"need"`
	Met      bool   `json:"met"`
	Pass     bool   `json:"pass"`
	WorkSec  int    `json:"work_sec"`
	CFSolved int    `json:"cf_solved"`
}

// Task is a problem in edge's stats.
type Task struct {
	ID           string    `json:"id"`
	Source       string    `json:"source"`
	Kind         string    `json:"kind"` // new | resolve | reentry | upsolve
	Title        string    `json:"title"`
	Rating       int       `json:"rating"`
	Started      time.Time `json:"started"`
	Finished     time.Time `json:"finished"`
	WorkSec      int       `json:"work_sec"`
	Hints        int       `json:"hints"`
	HintsAllowed int       `json:"hints_allowed"`
	NextHintSec  int       `json:"next_hint_sec"`
	Fails        int       `json:"fails"`
	Outcome      string    `json:"outcome"` // solo | assisted | gaveup | skipped | ""
}

// StatsRound is a round in edge's stats.
type StatsRound struct {
	ID       string    `json:"id"`
	Short    string    `json:"short"`
	Kind     string    `json:"kind"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Plan     string    `json:"plan"`
	Joined   string    `json:"joined"`
	State    string    `json:"state"`
	Problems []Problem `json:"problems"`
}

// Stats is the part of edge's stats document (version 1, STATS.md in
// edge's repo) the dashboard and the row use. Unknown fields are ignored.
type Stats struct {
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
		DayEnds   string `json:"day_ends"`
		Gate      string `json:"gate"`     // closed | open | off
		GateWhy   string `json:"gate_why"` // done | pass | outside_study_time | not_enforcing | edge_trouble | disabled
	} `json:"today"`
	Current *Task `json:"current"`
	Streak  struct {
		Current int `json:"current"`
		Best    int `json:"best"`
	} `json:"streak"`
	Week struct {
		Credits int   `json:"credits"`
		Target  int   `json:"target"`
		Days    []Day `json:"days"`
	} `json:"week"`
	Calendar []Day `json:"calendar"`
	Totals   struct {
		Solved  int `json:"solved"`
		Solo    int `json:"solo"`
		WorkSec int `json:"work_sec"`
	} `json:"totals"`
	Sources map[string]struct {
		Rating        int `json:"rating"`
		BandLo        int `json:"band_lo"`
		BandHi        int `json:"band_hi"`
		RatingHistory []struct {
			Rating int `json:"rating"`
		} `json:"rating_history"`
	} `json:"sources"`
	Resolves struct {
		Due           int `json:"due"`
		FirstAttempts int `json:"first_attempts"`
		FirstRetained int `json:"first_retained"`
		Upcoming      []struct {
			Title string    `json:"title"`
			Due   time.Time `json:"due"`
		} `json:"upcoming"`
	} `json:"resolves"`
	Experiment struct {
		Status string `json:"status"`
		Target int    `json:"target_per_arm"`
		Arms   map[string]struct {
			New int `json:"new"`
		} `json:"arms"`
		Verdict string `json:"verdict"`
	} `json:"experiment"`
	Recent []Task `json:"recent"`
	Health []struct {
		Msg string `json:"msg"`
	} `json:"health"`
	Rounds *struct {
		Live     *StatsRound  `json:"live"`
		Next     *StatsRound  `json:"next"`
		Upcoming []StatsRound `json:"upcoming"`
		Upsolves struct {
			Queue []struct {
				Short string    `json:"short"`
				Index string    `json:"index"`
				Due   time.Time `json:"due"`
			} `json:"queue"`
		} `json:"upsolves"`
		Record struct {
			Held int `json:"held"`
			Sat  int `json:"sat"`
		} `json:"record"`
	} `json:"rounds"`
}

// ---- words ----

func short(task string) string {
	if i := strings.Index(task, " · "); i > 0 {
		return task[:i]
	}
	return task
}

func mins(d time.Duration) string {
	m := int(d.Minutes())
	if m < 60 {
		return itoa(max(m, 0)) + "m"
	}
	return itoa(m/60) + "h " + pad2(m%60) + "m"
}

// until says how far off t is: "in 42m", "in 3h 05m", "Sat 20:05".
func until(now, t time.Time) string {
	d := t.Sub(now)
	switch {
	case d < time.Minute:
		return "now"
	case d < 12*time.Hour:
		return "in " + mins(d+time.Minute-1)
	}
	return t.Format("Mon 15:04")
}

// left is a live round's time left: "1:12", "0:09".
func left(now, end time.Time) string {
	d := max(0, end.Sub(now)).Round(time.Minute)
	return itoa(int(d.Hours())) + ":" + pad2(int(d.Minutes())%60)
}

func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return itoa(n) + " " + w + "s"
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func roundKind(kind string) string {
	return map[string]string{"div2": "Div. 2", "div3": "Div. 3", "div4": "Div. 4", "edu": "Educational",
		"div12": "Div. 1 + 2", "abc": "Beginner", "weekly": "Weekly", "biweekly": "Biweekly"}[kind]
}

func roundName(short, kind string) string {
	if k := roundKind(kind); k != "" && !strings.Contains(short, k) {
		return short + " · " + k
	}
	return short
}
