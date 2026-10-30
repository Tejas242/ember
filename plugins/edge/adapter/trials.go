package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Tejas242/ember/pkg/plugin"
)

// edge's v2 trials (contract v2.1): Trial 1, the consistency loop (a rep
// each evening at a cue, a chain, skip tokens), and Trial 2, the readiness
// engine (interview patterns, a model of each, a weekly drill, a mistake
// log). What's drawn follows the fields edge fills: an older edge, or the
// baseline weeks, leave them zero and nothing here shows.

// Consistency is Trial 1's part of edge's stats ("consistency").
type Consistency struct {
	Phase      string   `json:"phase"`
	From       string   `json:"trial_from"` // the trial's first day, 2006-01-02
	To         string   `json:"trial_to"`   // its last
	Days       []RepDay `json:"days"`       // the last 28 days, oldest first
	Reps       int      `json:"reps"`       // days with a rep in the trial so far
	ShowUp     float64  `json:"show_up_14"` // days with a rep over the rolling 14, 0..1
	CueToStart *float64 `json:"median_cue_to_start_min"`
	Chain      int      `json:"chain"`
	Missed     int      `json:"missed"`
	SkipTokens int      `json:"skip_tokens"`
	Reverted   bool     `json:"reverted"` // the trial missed its line: the baseline rules are back
}

// RepDay is one day of Trial 1's record.
type RepDay struct {
	Date       string `json:"date"`
	Rep        bool   `json:"rep"`
	CueSet     bool   `json:"cue_set"`
	Cue        string `json:"cue"` // "19:30"
	CueToStart *int   `json:"cue_to_start_min"`
	Kind       string `json:"kind"`  // submit | focus | ac | round | giveup
	Token      bool   `json:"token"` // a skip token made the day neutral
}

// Readiness is Trial 2's model, in edge's stats ("readiness").
type Readiness struct {
	R        float64   `json:"r"` // overall readiness, 0..1 (0..100 is read too)
	Lo       float64   `json:"lo"`
	Hi       float64   `json:"hi"`
	Patterns []Pattern `json:"patterns"`
	Mistakes struct {
		All    map[string]int `json:"all"`
		Last30 map[string]int `json:"last_30d"`
		Top    string         `json:"top"`
	} `json:"mistakes"`
	Drills count    `json:"drills"`
	Brier  *float64 `json:"brier"`
}

// Pattern is one interview pattern in the model: P is the chance of solving
// an interview problem of it, R how much of it is still remembered; Lo..Hi
// is the 80% interval of P·R.
type Pattern struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Theta  float64 `json:"theta"`
	Sigma  float64 `json:"sigma"`
	P      float64 `json:"p"`
	R      float64 `json:"r"`
	Lo     float64 `json:"lo"`
	Hi     float64 `json:"hi"`
	Solved int     `json:"solved"`
	Last   string  `json:"last"`
}

// count reads a number, or a list as its length.
type count int

func (c *count) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var xs []json.RawMessage
		err := json.Unmarshal(b, &xs)
		*c = count(len(xs))
		return err
	}
	var n float64
	if string(b) == "null" {
		return nil
	}
	err := json.Unmarshal(b, &n)
	*c = count(n)
	return err
}

// frac is a share as 0..1, from 0..1 or 0..100.
func frac(x float64) float64 {
	if x > 1 {
		x /= 100
	}
	return math.Max(0, math.Min(1, x))
}

func pct(x float64) int { return int(math.Round(100 * frac(x))) }

// trialOn: edge runs a trial's rules now (phase trial1 or trial2, not reverted).
func trialOn(phase string, st *Stats) bool {
	if st != nil && st.Consistency != nil && st.Consistency.Reverted {
		return false
	}
	return phase == "trial1" || phase == "trial2"
}

// phaseOf is the phase edge last said: its status, else its stats.
func phaseOf(st *Stats, e *Status) string {
	if e != nil && e.Phase != "" {
		return e.Phase
	}
	if st != nil {
		return st.Phase
	}
	return ""
}

// repLeft is what's left of a rep's 45 minutes: it runs down only while
// edge counts work, and stands still (as of AsOfMs) when it doesn't. ok is
// false with no countdown, or once it's used up: then the task just goes on.
func repLeft(e *Status, now time.Time) (time.Duration, bool) {
	if e.DeadlineMs == 0 || !e.active() || !trialOn(e.Phase, nil) {
		return 0, false
	}
	end := time.UnixMilli(e.DeadlineMs)
	at := now
	if !e.ticking() && e.AsOfMs > 0 {
		at = time.UnixMilli(e.AsOfMs)
	}
	d := end.Sub(at)
	return d, d > 0
}

// clockLeft is time left as a clock: "31:12", "1h 05m".
func clockLeft(d time.Duration) string {
	d = (d + time.Second - 1).Truncate(time.Second)
	if d >= time.Hour {
		return mins(d)
	}
	return itoa(int(d.Minutes())) + ":" + pad2(int(d.Seconds())%60)
}

// minsLeft is time left in minutes, rounded up: "31m left".
func minsLeft(d time.Duration) string { return mins(d+time.Minute-1) + " left" }

// repName is the task as the pill names it in a trial: "Drill · LC 1234".
func repName(e *Status) string {
	if e.Drill {
		return "Drill · " + e.label()
	}
	return e.label()
}

func chainWord(n int) string { return "chain " + itoa(n) }

// cueAt is tonight's cue as a clock: "19:30".
func cueAt(e *Status) string { return time.UnixMilli(e.CueMs).Format("15:04") }

// repLine is the pill in a trial, before the rep and after it; nil leaves it
// to the usual line.
func repLine(e *Status, now time.Time, s *plugin.Status) bool {
	switch {
	case e.active():
		return false
	case e.RepDone:
		s.Text, s.Mood, s.Badge = "Rep done", plugin.Done, ""
		if e.Chain > 0 {
			s.Value = chainWord(e.Chain)
		}
		s.Salience = plugin.Float(0)
		return true
	case e.CueMs > 0:
		s.Text, s.Mood = "Rep · "+cueAt(e), plugin.Quiet
		s.Salience = plugin.Float(0.2) // quiet until its time
		if !now.Before(time.UnixMilli(e.CueMs)) {
			s.Mood, s.Salience = plugin.Nudge, plugin.Float(0.55)
		}
		if e.Task != "" {
			s.Value = e.label()
		}
		if e.Closed {
			s.Badge = "lock"
		}
		return true
	}
	return false
}

// Moment is Ember's word in the moment for edge's news in a trial: the
// rep's problem solved ("first try · chain 5"), or the rep counted.
func Moment(old, cur *Status) *plugin.Moment {
	if old == nil || cur == nil || !trialOn(cur.Phase, nil) {
		return nil
	}
	chain := ""
	if cur.Chain > 0 {
		chain = " · " + chainWord(cur.Chain)
	}
	if old.active() && old.TaskID != "" && cur.Explain == old.TaskID && old.Explain != old.TaskID { // solved: it waits for its explain-back
		text := "first try"
		if old.LastVerdict != "" {
			text = "solved"
		}
		if old.Drill {
			text = "drill solved"
		}
		return &plugin.Moment{Text: text + chain, Icon: "check", Mood: plugin.Done, TTLs: 6}
	}
	if !old.RepDone && cur.RepDone {
		return &plugin.Moment{Text: "Rep done" + chain, Icon: "check", Mood: plugin.Done, TTLs: 6}
	}
	return nil
}

// whyThis is the pick's reason in plain words.
func whyThis(st *Stats, cur *Task, e *Status) string {
	pat := e.Pattern
	p := patternOf(st, pat)
	switch {
	case e.Drill:
		return "picked blind · 45 minutes, like an interview"
	case e.Kind == "resolve" && p != nil && frac(p.R) < 0.8:
		return "cold re-solve · " + strings.ToLower(pat) + " is fading"
	case e.Kind == "resolve":
		return "cold re-solve · does it stick?"
	case e.Kind == "upsolve":
		return "owed from the round"
	case e.Kind == "reentry":
		return "a warm-up to get going"
	case p != nil:
		return fmt.Sprintf("%s · %d%% ready", pat, pct(frac(p.P)*frac(p.R)))
	case pat != "":
		return pat
	}
	if cur != nil && cur.Rating > 0 {
		if cf := st.Sources["cf"]; cf.BandHi > cf.BandLo && cur.Rating >= cf.BandLo && cur.Rating <= cf.BandHi {
			return fmt.Sprintf("in your band, %d–%d", cf.BandLo, cf.BandHi)
		}
	}
	return ""
}

func patternOf(st *Stats, name string) *Pattern {
	if st == nil || st.Readiness == nil || name == "" {
		return nil
	}
	for i := range st.Readiness.Patterns {
		if p := &st.Readiness.Patterns[i]; strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// mistakeName is a mistake kind in words.
func mistakeName(k string) string {
	if w, ok := map[string]string{"misread": "misreads", "edge": "edge cases", "edgecase": "edge cases", "edge_case": "edge cases",
		"overflow": "overflow", "approach": "the approach", "bug": "bugs"}[strings.ToLower(k)]; ok {
		return w
	}
	return strings.ToLower(k)
}

// topMistake is the commonest failure mode lately, and its share.
func topMistake(r *Readiness) (kind string, n, of int) {
	m := r.Mistakes.Last30
	if len(m) == 0 {
		m = r.Mistakes.All
	}
	for k, v := range m {
		of += v
		if v > n || (v == n && k < kind) {
			kind, n = k, v
		}
	}
	if r.Mistakes.Top != "" {
		kind, n = r.Mistakes.Top, m[r.Mistakes.Top]
	}
	return kind, n, of
}

// trialHero turns the day's hero to the rep: tonight's cue and pick (with
// why), the countdown while it runs, the chain once it's done.
func trialHero(h *plugin.Widget, st *Stats, cur *Task, e *Status, now time.Time) {
	h.Stat = nil
	if e.Chain > 0 { // never a 0: after a break the next rep starts at 1
		h.Stat = &plugin.HeroStat{Value: itoa(e.Chain), Caption: "day chain"}
	}
	switch {
	case e.RepDone || st.Today.Done:
		h.Title = "Tonight's rep is done"
		if e.Drill || (cur != nil && cur.Kind == "drill") {
			h.Title = "Drill done"
		}
		h.Sub, h.SubIcon, h.SubMood = "Claude is open", "", plugin.Quiet
		if e.Chain > 0 {
			h.Sub = "Chain " + itoa(e.Chain) + " · Claude is open"
		}
	case e.active():
		h.Title = "Rep"
		if e.Drill {
			h.Title = "Drill"
		}
		if e.Paused {
			h.Title += " · paused"
		}
		h.Sub, h.SubIcon, h.SubMood = "Counts at 25 focused minutes, or a submit after 15", "", plugin.Quiet
		if e.Missed > 0 {
			h.Sub = "Counts at 20 focused minutes, or a submit after 15"
		}
		if d, ok := repLeft(e, now); ok {
			h.Aside = minsLeft(d)
			if e.ticking() {
				end := time.UnixMilli(e.DeadlineMs)
				h.Ring = &plugin.HeroRing{Mode: "countdown", StartMs: end.Add(-45 * time.Minute).UnixMilli(), EndMs: end.UnixMilli(), Mood: plugin.Work}
			} else {
				h.Ring = &plugin.HeroRing{Value: math.Max(0, 45*60-d.Seconds()), Goal: 45 * 60, Mood: plugin.Quiet}
			}
		}
	default:
		h.Title, h.SubIcon, h.SubMood = "Tonight's rep", "", plugin.Quiet
		if e.Closed {
			h.SubIcon = "lock"
		}
		switch {
		case e.Missed > 0:
			h.Sub = "Yesterday slipped. Tonight keeps the chain · 20 minutes counts"
		case e.Chain > 0:
			h.Sub = "One rep keeps the chain"
		default:
			h.Sub = "One rep starts a chain"
		}
		if e.CueMs > 0 {
			h.Aside = cueAt(e)
			if !now.Before(time.UnixMilli(e.CueMs)) {
				h.Aside = "now"
			}
		}
		if e.Task != "" && !e.CanSkip && e.SkipTokens > 0 && (e.CueMs == 0 || now.Before(time.UnixMilli(e.CueMs))) && len(h.Buttons) == 1 {
			h.Buttons = append([]plugin.Button{{Key: "skiptonight", Label: "Skip tonight"}}, h.Buttons...)
		}
	}
	if h.Step != nil && e.Task != "" {
		if why := whyThis(st, cur, e); why != "" && !e.active() {
			if h.Step.Facts != "" {
				why = h.Step.Facts + " · " + why
			}
			h.Step.Facts = why
		}
		if e.Drill {
			h.Step.Tag, h.Step.TagMood = "Drill", plugin.Nudge
		}
	}
}

// chainWidget is Trial 1's record: the last 28 days, the chain, the skips.
func chainWidget(c *Consistency, e *Status) plugin.Widget {
	heat := plugin.Widget{Kind: "heatmap", Title: "Last 4 weeks", Weeks: 4}
	reps := 0
	for _, d := range c.Days {
		day := plugin.Day{Date: d.Date}
		switch {
		case d.Rep:
			day.Level = 1
			reps++
		case d.Token:
			day.Level, day.Mood = 1, plugin.Wait // a skip: neutral, never a gap
		}
		heat.Days = append(heat.Days, day)
	}
	chain, tokens := c.Chain, c.SkipTokens
	if e != nil && e.Chain > 0 {
		chain = e.Chain
	}
	if e != nil && (e.Phase != "" || e.SkipTokens > 0) {
		tokens = e.SkipTokens
	}
	ch := plugin.Item{Label: "Chain", Mood: plugin.Done, Value: "–", Caption: "starts with a rep"}
	if chain > 0 {
		ch.Value, ch.Caption = float64(chain), plural(chain, "day")
	}
	if c.Missed > 0 || (e != nil && e.Missed > 0) {
		ch.Mood, ch.Caption = plugin.Nudge, "one miss · a rep tonight keeps it"
	}
	sk := plugin.Item{Label: "Skips", Mood: plugin.Wait, Value: fmt.Sprint(tokens), Caption: "left this week"}
	show := plugin.Item{Label: "Show-up", Mood: plugin.Nudge, Value: "–", Caption: "last 14 days"}
	if c.ShowUp > 0 || len(c.Days) >= 14 {
		n := int(math.Round(frac(c.ShowUp) * 14))
		show.Value, show.Caption = fmt.Sprintf("%d of 14", n), "days with a rep"
		if n >= 10 {
			show.Mood = plugin.Done
		}
	}
	cue := plugin.Item{Label: "Cue → start", Mood: plugin.Quiet, Value: "–", Caption: "median"}
	if c.CueToStart != nil {
		cue.Value, cue.Unit = math.Max(0, *c.CueToStart)*60, "s"
	}
	stats := plugin.Widget{Kind: "stats", Cols: 2, Items: []plugin.Item{ch, sk, show, cue}}
	sum := "chain starts with a rep"
	if chain > 0 {
		sum = chainWord(chain)
	}
	return plugin.Widget{Kind: "columns", Title: "Chain", Split: 0.3, A: &heat, B: &stats,
		Summary: sum + " · " + plural(tokens, "skip") + " left · " + plural(reps, "rep") + " in 4 weeks"}
}

// readinessWidget is Trial 2's model: overall readiness with its interval,
// then each pattern against a 75% target, the weakest first.
func readinessWidget(r *Readiness) plugin.Widget {
	w := plugin.Widget{Kind: "bars", Layout: "rows", Title: fmt.Sprintf("Interview readiness · %d%%", pct(r.R)), Value: fmt.Sprintf("%d%%", pct(r.R))}
	if r.Hi > r.Lo {
		w.Caption = fmt.Sprintf("likely %d–%d%%", pct(r.Lo), pct(r.Hi))
	}
	ps := append([]Pattern(nil), r.Patterns...)
	val := func(p Pattern) float64 { return frac(p.P) * frac(p.R) }
	sort.SliceStable(ps, func(i, j int) bool { return val(ps[i]) < val(ps[j]) })
	if len(ps) > 8 {
		w.Summary = fmt.Sprintf("the 8 weakest of %d patterns", len(ps))
		ps = ps[:8]
	}
	for _, p := range ps {
		v := 100 * val(p)
		it := plugin.Item{Label: p.Name, Value: math.Round(v), Num: math.Round(v), Caption: fmt.Sprintf("%.0f%%", v), Target: plugin.Float(75), Mood: plugin.Nudge}
		if v >= 75 {
			it.Mood = plugin.Done
		}
		if p.Hi > p.Lo {
			it.Lo, it.Hi = plugin.Float(math.Round(100*frac(p.Lo))), plugin.Float(math.Round(100*frac(p.Hi)))
		}
		w.Items = append(w.Items, it)
	}
	if w.Summary == "" {
		w.Summary = w.Value
		if w.Caption != "" {
			w.Summary += " · " + w.Caption
		}
	}
	return w
}

// mistakeNote is the commonest failure mode, when there's one.
func mistakeNote(r *Readiness) (plugin.Widget, bool) {
	k, n, of := topMistake(r)
	if k == "" || n == 0 {
		return plugin.Widget{}, false
	}
	text := fmt.Sprintf("Most wrong answers lately: %s · %d of %d", mistakeName(k), n, of)
	if r.Drills > 0 && r.Brier != nil {
		text += fmt.Sprintf(" · drills %d, Brier %.2f", int(r.Drills), *r.Brier)
	}
	return plugin.Widget{Kind: "note", Icon: "search", Text: text, Mood: plugin.Quiet}, true
}

// phaseNote is a quiet line in the baseline weeks: when Trial 1 starts, or
// that it's off.
func phaseNote(st *Stats, now time.Time) (plugin.Widget, bool) {
	n := plugin.Widget{Kind: "note", Icon: "chart", Mood: plugin.Quiet}
	if c := st.Consistency; c != nil && c.Reverted {
		n.Text = "Trial 1 is off · back to the daily goal"
		return n, true
	}
	from := time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local)
	if c := st.Consistency; c != nil && c.From != "" {
		if t, err := time.ParseInLocation("2006-01-02", c.From, time.Local); err == nil {
			from = t
		}
	}
	if !now.Before(from) {
		return n, false
	}
	n.Text = "Baseline week · Trial 1 starts " + from.Format("Mon 2 Jan")
	return n, true
}
