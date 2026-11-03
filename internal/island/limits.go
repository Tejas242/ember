package island

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Tejas242/ember/internal/xdg"
)

// Claude's rate limits. Claude Code hands its status line the numbers; the
// status line (or `ember limits`) writes them to $XDG_RUNTIME_DIR/ember/limits.json
// and Ember reads that file with the rest of its housekeeping. Nothing here
// runs a process or a goroutine. The numbers only change while some session
// is open, so a stale file is handled by its own reset times.

const (
	limitShow  = 80               // percent: below this the card says nothing
	limitStale = 15 * time.Minute // older than this, the line says "as of"
	limitH     = 24
	backFor    = 4 * time.Second  // "Claude's back"
	backLate   = 10 * time.Minute // not announced if Ember learns of it this late
)

// limitsFile is what the status line writes (times are unix seconds).
type limitsFile struct {
	H5   *float64 `json:"h5"`
	H5At int64    `json:"h5_at"`
	D7   *float64 `json:"d7"`
	D7At int64    `json:"d7_at"`
	T    int64    `json:"t"`
}

func limitsPath() string { return filepath.Join(xdg.RuntimeDir(), "ember", "limits.json") }

type limitsState struct {
	f      limitsFile
	at     time.Time // the file's mtime: unchanged means unread
	hit    time.Time // when a limit he hit resets: Claude is back then
	hitWin string
}

// readLimits loads the file if it changed, and announces a limit lifting.
func (a *app) readLimits() bool {
	u := a.ui
	changed := false
	if fi, err := os.Stat(limitsPath()); err == nil && !fi.ModTime().Equal(u.lim.at) {
		var f limitsFile
		if b, err := os.ReadFile(limitsPath()); err == nil && json.Unmarshal(b, &f) == nil {
			u.lim.f, u.lim.at, changed = f, fi.ModTime(), true
		}
	}
	return u.limitsTick(u.now()) || changed
}

// limitsTick remembers a limit that is full, and when its reset passes says Claude is back.
func (u *UI) limitsTick(now time.Time) bool {
	l := &u.lim
	for _, w := range []struct {
		name string
		pct  *float64
		at   int64
	}{{"5h", l.f.H5, l.f.H5At}, {"7d", l.f.D7, l.f.D7At}} {
		if w.pct == nil || w.at == 0 {
			continue
		}
		reset := time.Unix(w.at, 0)
		switch {
		case *w.pct >= 100 && now.Before(reset):
			l.hit, l.hitWin = reset, w.name
		case !l.hit.IsZero() && w.name == l.hitWin && !now.Before(l.hit):
			at := l.hit
			l.hit, l.hitWin = time.Time{}, ""
			if now.Sub(at) > backLate {
				return true
			}
			u.Moment(moment{key: "claude:back", act: "claude", mood: moodHappy, text: "Claude's back", until: now.Add(backFor)})
			return true
		}
	}
	return false
}

// limitText is the card's foot line, or empty when there is nothing to say.
type limitView struct {
	text string
	full bool // 100%: red, not amber
}

// limitLine: "Claude · 5h 84% · resets 4:10 PM", only at 80% and over, and only
// while the numbers can still be true (before their reset).
func (u *UI) limitLine() limitView {
	now, f := u.now(), u.lim.f
	var best limitView
	bestPct := -1.0
	for _, w := range []struct {
		name string
		pct  *float64
		at   int64
	}{{"5h", f.H5, f.H5At}, {"7d", f.D7, f.D7At}} {
		if w.pct == nil || *w.pct < limitShow || w.at == 0 {
			continue
		}
		reset := time.Unix(w.at, 0)
		if !now.Before(reset) { // it reset since: the numbers are history
			continue
		}
		if *w.pct <= bestPct {
			continue
		}
		bestPct = *w.pct
		when := reset.Format(u.TimeFmt)
		if reset.Sub(now) > 20*time.Hour {
			when = reset.Format("Mon ") + when
		}
		best = limitView{text: fmt.Sprintf("%s %.0f%% · resets %s", w.name, *w.pct, when), full: *w.pct >= 100}
	}
	if best.text != "" && f.T > 0 && now.Sub(time.Unix(f.T, 0)) > limitStale {
		best.text += " · as of " + time.Unix(f.T, 0).Format(u.TimeFmt)
	}
	return best
}

func (u *UI) drawLimit(l limitView, x, y float32) {
	p := u.pal
	col := p.yellow
	if l.full {
		col = p.red
	}
	tx := x + textX
	base := y + limitH/2 + 4
	tx += u.dc.text(fCaption(), "Claude", tx, base, p.faint)
	u.dc.text(fCaption(), " · "+l.text, tx, base, col)
}
