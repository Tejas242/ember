package island

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// ---- Ember's character: what it remembers, and when it may play ----
//
// Four traits decide everything it does on its own. It's watchful: every
// look has a cause you could name. It's invested: it feels how the work is
// going, honestly, and celebrates only what was earned. It's self-sufficient:
// it never asks for attention, never adds words, width or light to the pill
// for itself. And it's wry, rarely, always about something real.
//
// A flourish (a stretch, a yawn, an eye-roll) is a treat because it's rare:
// the gate allows one only when the island is calm, at most one every
// twenty minutes and five a day, each on its own cooldown. Reactions to what
// you or Claude just did (a nod, a flinch, a Done) aren't flourishes: they
// always play.

type bond struct {
	Born    time.Time            `json:"born"`
	Wake    float64              `json:"wake,omitempty"`    // when you usually first sit down, minutes after midnight
	Bedtime float64              `json:"bedtime,omitempty"` // when you usually stop for the night (0: not learned)
	TurnLn  float64              `json:"turnLn,omitempty"`  // a turn's usual length, ln(seconds), smoothed
	Turns   int                  `json:"turns,omitempty"`
	Last    map[string]time.Time `json:"last"` // each flourish's last time, and "*" for any
	Day     string               `json:"day,omitempty"`
	Spent   int                  `json:"spent,omitempty"` // flourishes today
	Seen    string               `json:"seen,omitempty"`  // the last day you came back to the desk
	dirty   bool
}

const (
	flourishEvery = 20 * time.Minute
	flourishDaily = 5
)

func bondPath() string { return filepath.Join(filepath.Dir(habitsPath()), "bond.json") }

func loadBond(now time.Time) *bond {
	b := &bond{}
	if data, err := os.ReadFile(bondPath()); err == nil {
		json.Unmarshal(data, b)
	}
	if b.Last == nil {
		b.Last = map[string]time.Time{}
	}
	if b.Born.IsZero() {
		b.Born, b.dirty = now, true
	}
	return b
}

func (b *bond) save() {
	if b == nil || !b.dirty {
		return
	}
	b.dirty = false
	os.MkdirAll(filepath.Dir(bondPath()), 0o755)
	if data, err := json.Marshal(b); err == nil {
		os.WriteFile(bondPath(), data, 0o644)
	}
}

// cooled: name hasn't happened within every. It notes that it does now.
func (b *bond) cooled(name string, every time.Duration, now time.Time) bool {
	if t, ok := b.Last[name]; ok && now.Sub(t) < every && now.Sub(t) >= 0 {
		return false
	}
	b.Last[name], b.dirty = now, true
	return true
}

// calm: nothing needs you, nothing failed unseen, no card is open, nobody
// is watching your screen. Only then may Ember play on its own.
func (u *UI) calm() bool {
	if u.casting || u.lastView != vCompact || len(u.cards) > 0 || u.mind.valence < -0.2 || u.edgeLive() {
		return false
	}
	if m := u.moment; m != nil && m.mood == moodError {
		return false
	}
	for _, s := range u.sessions {
		if s.State == "wait" || (s.Recap != nil && s.Recap.Failed && !s.Unseen.IsZero()) {
			return false
		}
	}
	return true
}

// may: the gate for a flourish. Calm, within the day's budget, not within
// twenty minutes of the last flourish, and not within every of its own last.
func (u *UI) may(name string, every time.Duration) bool {
	b, now := u.bond, u.now()
	if b == nil || !u.calm() {
		return false
	}
	if d := now.Format("2006-01-02"); b.Day != d {
		b.Day, b.Spent, b.dirty = d, 0, true
	}
	if b.Spent >= flourishDaily {
		return false
	}
	if t, ok := b.Last["*"]; ok && now.Sub(t) < flourishEvery && now.Sub(t) >= 0 {
		return false
	}
	if !b.cooled(name, every, now) {
		return false
	}
	b.Last["*"], b.Spent = now, b.Spent+1
	return true
}

// ---- what it learns about you ----

// turn notes how long a turn took, and says whether this one was a real
// effort: three times your usual, or ten minutes at least.
func (b *bond) turn(took time.Duration) (long bool) {
	sec := took.Seconds()
	if sec < 1 {
		return false
	}
	if b.Turns > 5 {
		long = took >= 10*time.Minute && sec >= 3*math.Exp(b.TurnLn)
	}
	if b.Turns == 0 {
		b.TurnLn = math.Log(sec)
	} else {
		b.TurnLn += (math.Log(sec) - b.TurnLn) * 0.1
	}
	b.Turns++
	b.dirty = true
	return long
}

// clockMin is a time of day in minutes after midnight.
func clockMin(t time.Time) float64 { return float64(t.Hour()*60 + t.Minute()) }

// circEMA smooths a time of day on the clock's circle (23:50 and 00:10 average to midnight).
func circEMA(old, x, k float64) float64 {
	if old == 0 {
		return x
	}
	d := math.Mod(x-old+720+1440, 1440) - 720
	return math.Mod(old+d*k+1440, 1440)
}

// back: you returned after being away from since. A long evening absence
// teaches it your bedtime; the first return of the day, your morning.
func (b *bond) back(since, now time.Time) (firstToday bool) {
	if now.Sub(since) >= 4*time.Hour {
		if h := since.Hour(); h >= 20 || h < 5 {
			b.Bedtime, b.dirty = circEMA(b.Bedtime, clockMin(since), 0.2), true
		}
	}
	if d := now.Format("2006-01-02"); b.Seen != d {
		b.Seen = d
		b.Wake, b.dirty = circEMA(b.Wake, clockMin(now), 0.2), true
		return true
	}
	return false
}

// nearBedtime: within the hour after your usual bedtime (and ten minutes before).
func (b *bond) nearBedtime(now time.Time) bool {
	if b == nil || b.Bedtime == 0 {
		return false
	}
	d := math.Mod(clockMin(now)-b.Bedtime+720+1440, 1440) - 720
	return d > -10 && d < 60
}

// ---- the eye-roll ----

var absolutelyRight = regexp.MustCompile(`(?i)^\W*you('re| are) (absolutely|completely|totally|so) right`)

// ---- poking it ----

// poke: you clicked Ember. A waiting session is the one thing it wants you to
// see, so it takes you there. Otherwise it's a poke: it squashes and looks at
// you; the third in a row puffs it up, the sixth sends it into its coal for
// a moment, peeking out before it comes back.
func (u *UI) poke() {
	now := u.now()
	if ss := u.sessionList(); len(ss) > 0 && ss[0].State == "wait" {
		u.key("focus:" + ss[0].ID)
		u.body.shove(20)
		return
	}
	if now.Sub(u.pokeAt) > 4*time.Second {
		u.pokes = 0
	}
	u.pokeAt = now
	u.pokes++
	switch {
	case u.pokes >= 6:
		u.pokes = 0
		u.play("hide", 0)
	case u.pokes == 3:
		u.play("puff", 0)
	default:
		u.play("poke", 0)
	}
}
