package island

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// The usage log's events, each behind one call so the places that make them
// stay one line long. What each line holds is in docs/DESIGN.md ("usage log").

// logSurface: Ember put something in front of you (a view grew, or a moment).
func (u *UI) logSurface(what, src string) { u.Log("surface", "what", what, "src", src) }

// logOpen: the open card closed. via: hover | click; dwell_ms: how long it
// was up; acted: you used a row in it.
func (u *UI) logOpen(via string, dwell time.Duration, acted bool) {
	u.Log("open", "via", via, "dwell_ms", dwell.Milliseconds(), "acted", acted)
}

// logRowAction: a press on a plugin's row, a music control or a focus row.
func (u *UI) logRowAction(key string) {
	u.Log("row_action", "key", strings.ReplaceAll(key, "\x00", ":"))
	u.openActs++
}

// momentSrc names who a moment is from, without its words ("x:edge", "claude").
func momentSrc(key string) string {
	parts := strings.SplitN(key, ":", 3)
	if parts[0] == "x" && len(parts) > 1 {
		return "x:" + parts[1]
	}
	return parts[0]
}

// logAsk: a session stopped asking. via: the hook that said so.
func (u *UI) logAsk(s *Session, before, via string, now time.Time) {
	switch {
	case before == "" && s.Asking != "":
		if u.askAt == nil {
			u.askAt = map[string]time.Time{}
		}
		u.askAt[s.ID] = now
	case before != "" && s.Asking == "":
		if at, ok := u.askAt[s.ID]; ok {
			u.Log("ask", "resolved_ms", now.Sub(at).Milliseconds(), "via", via)
			delete(u.askAt, s.ID)
		}
	}
}

// logFocus: you focused a Claude terminal that has no ask pending. Only the
// window's app id is kept (kitty, foot), never its title.
func (u *UI) logFocus(s *Session, appID string) {
	if s.Asking == "" {
		u.Log("focus", "app", appID)
	}
}

// ---- what Ember costs ----

const costEvery = 10 * time.Minute

// costMeter reads Ember's own CPU and memory from /proc.
type costMeter struct {
	at         time.Time
	cpu        float64 // seconds of CPU at `at`
	hz, pageSz float64
}

// logCost writes cpu% (of one core, since the last call) and rss (KiB) at
// most every costEvery. Called from housekeeping.
func (u *UI) logCost() {
	if u.usage == nil {
		return
	}
	now := u.now()
	m := &u.cost
	if m.hz == 0 {
		m.hz, m.pageSz = 100, float64(os.Getpagesize()) // clock ticks are 100/s on Linux
	}
	if !m.at.IsZero() && now.Sub(m.at) < costEvery {
		return
	}
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return
	}
	// fields after the ")" of the command name; utime and stime are the 12th and 13th of them
	s := string(b)
	f := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(f) < 13 {
		return
	}
	ut, _ := strconv.ParseFloat(f[11], 64)
	st, _ := strconv.ParseFloat(f[12], 64)
	cpu := (ut + st) / m.hz
	if !m.at.IsZero() {
		pct := (cpu - m.cpu) / now.Sub(m.at).Seconds() * 100
		var rss float64
		if b, err := os.ReadFile("/proc/self/statm"); err == nil {
			if g := strings.Fields(string(b)); len(g) > 1 {
				rss, _ = strconv.ParseFloat(g[1], 64)
			}
		}
		u.Log("cost", "cpu_pct", float64(int(pct*100+0.5))/100, "rss_kb", int64(rss*m.pageSz/1024))
	}
	m.at, m.cpu = now, cpu
}
