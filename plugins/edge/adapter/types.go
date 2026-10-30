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

	// contract v2 (docs/EDGE-CONTRACT.md); an older edge leaves them zero
	TaskID      string `json:"taskId"`      // "cf:2277C": actions name it (start:<id>, giveup:<id> …)
	Short       string `json:"short"`       // the pill's label: "2277C", "LC 3542"
	Kind        string `json:"kind"`        // new | resolve | reentry | upsolve | self
	WorkSec     int    `json:"workSec"`     // focused work on the task, as of AsOfMs
	TodaySec    int    `json:"todaySec"`    // focused work today
	Working     bool   `json:"working"`     // work is being counted right now: the clock runs
	Paused      bool   `json:"paused"`      // started, but no work for a while
	AsOfMs      int64  `json:"asOfMs"`      // when WorkSec was measured
	Policy      string `json:"policy"`      // ladder | instant | cold | ""
	HintsReady  bool   `json:"hintsReady"`  // a hint can be taken now
	NextHintSec int    `json:"nextHintSec"` // work until the next hint, 0 = none coming
	GiveUpInSec int    `json:"giveUpInSec"` // work until give-up unlocks: 0 now, -1 never
	CanSkip     bool   `json:"canSkip"`     // skip:<id> is accepted
	DueMs       int64  `json:"dueMs"`       // an upsolve's lapse
	Lapsed      bool   `json:"lapsed"`      // an upsolve past it
	Judging     string `json:"judging"`     // "" | "queue" | "test 9"
	LastVerdict string `json:"lastVerdict"` // "WA on test 3"
	Owed        []Owed `json:"owed"`        // upsolves still owed, oldest first
	Explain     string `json:"explain"`     // a task waiting for its explain-back
	Debrief     string `json:"debrief"`     // a round waiting for its debrief
	Health      string `json:"health"`      // one plain sentence when a judge is down
	SeenMs      int64  `json:"seenMs"`      // edge's heartbeat

	// contract v2.1 (the trials); an older edge leaves them zero
	Phase      string `json:"phase"`      // baseline | trial1 | trial2
	CueMs      int64  `json:"cueMs"`      // tonight's rep time, once set
	RepDone    bool   `json:"repDone"`    // today's rep counts
	Chain      int    `json:"chain"`      // days in the chain (one miss doesn't break it)
	Missed     int    `json:"missed"`     // misses in a row right now: 0 or 1
	SkipTokens int    `json:"skipTokens"` // skips left this week
	DeadlineMs int64  `json:"deadlineMs"` // the rep's interview countdown ends then (as of AsOfMs while not working)
	Pattern    string `json:"pattern"`    // the task's interview pattern
	Drill      bool   `json:"drill"`      // the task is the weekly drill
	ReadyPct   int    `json:"readyPct"`   // interview readiness, 0..100 (trial2)
	ReadyLo    int    `json:"readyLo"`    // its 80% interval
	ReadyHi    int    `json:"readyHi"`

	Stopped bool `json:"-"` // the adapter's: edge hasn't been heard from in 90 s
}

// Owed is one round's upsolves still owed.
type Owed struct {
	Round   string   `json:"round"` // upsolve:<round> starts its next one
	Short   string   `json:"short"`
	Indexes []string `json:"indexes"`
	DueMs   int64    `json:"dueMs"`
}

// active: the task is started and not finished.
func (e *Status) active() bool { return e.StartedMs > 0 && !e.Pending }

// workAt is the task's work as of now: it runs on only while edge counts it.
func (e *Status) workAt(now time.Time) time.Duration {
	w := time.Duration(e.WorkSec) * time.Second
	if e.ticking() {
		w += max(0, now.Sub(time.UnixMilli(e.AsOfMs)))
	}
	return w
}

// ticking: the clock runs (work counted now, edge alive, a time to count from).
func (e *Status) ticking() bool { return e.Working && !e.Paused && !e.Stopped && e.AsOfMs > 0 }

// key is an action scoped to the task or round it acts on.
func key(verb, id string) string {
	if id == "" {
		return verb
	}
	return verb + ":" + id
}

// label is the task's name in the pill: edge's Short, else the task cut
// short, a LeetCode title as "LC 3542".
func (e *Status) label() string {
	if e.Short != "" {
		return e.Short
	}
	return shortTask(e.Task)
}

func shortTask(task string) string {
	if i := strings.Index(task, ". "); i > 0 && strings.Trim(task[:i], "0123456789") == "" {
		return "LC " + task[:i]
	}
	return short(task)
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
	TTLs  float64 `json:"ttl_s"` // how long its buttons stay valid
	Chips []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	} `json:"chips"` // v2.1: small choice chips under the body
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
	URL          string    `json:"url"`
	Kind         string    `json:"kind"` // new | resolve | reentry | upsolve
	Title        string    `json:"title"`
	Rating       int       `json:"rating"`
	Started      time.Time `json:"started"`
	Finished     time.Time `json:"finished"`
	WorkSec      int       `json:"work_sec"`
	Hints        int       `json:"hints"`
	HintsAllowed int       `json:"hints_allowed"`
	NextHintSec  int       `json:"next_hint_sec"`
	GiveUpInSec  int       `json:"give_up_in_sec"` // current task: work until giving up is allowed
	Fails        int       `json:"fails"`
	Explain      int       `json:"explain"` // 0..2, -1 = not done
	Outcome      string    `json:"outcome"` // solo | assisted | gaveup | skipped | ""
}

// started: begun and not finished.
func (t *Task) started() bool { return !t.Started.IsZero() && t.Outcome == "" }

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
			At     time.Time `json:"at"`
			Rating int       `json:"rating"`
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
	Daemon struct {
		Seen time.Time `json:"seen"`
	} `json:"daemon"`
	Phase       string       `json:"phase"`       // v2.1: baseline | trial1 | trial2
	Consistency *Consistency `json:"consistency"` // Trial 1's numbers
	Readiness   *Readiness   `json:"readiness"`   // Trial 2's model (in shadow before it)
	Rounds      *struct {
		Live     *StatsRound  `json:"live"`
		Next     *StatsRound  `json:"next"`
		Upcoming []StatsRound `json:"upcoming"`
		Recent   []StatsRound `json:"recent"`
		Upsolves struct {
			Queue []Upsolve `json:"queue"`
		} `json:"upsolves"`
		Record struct {
			Held int `json:"held"`
			Sat  int `json:"sat"`
		} `json:"record"`
	} `json:"rounds"`
}

// Upsolve is one owed problem in edge's stats, oldest round first.
type Upsolve struct {
	ID    string    `json:"id"`    // the task's: "cf:2261C"
	Round string    `json:"round"` // the round's: "cf:2261"
	Short string    `json:"short"`
	Index string    `json:"index"`
	Due   time.Time `json:"due"`
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
