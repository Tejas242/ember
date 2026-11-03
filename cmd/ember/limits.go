package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Tejas242/ember/internal/xdg"
)

// `ember limits`: Claude Code's status line gives each session its rate
// limits as JSON on stdin. This writes the ones Ember shows to
// $XDG_RUNTIME_DIR/ember/limits.json, which Ember reads with its housekeeping.
//
// The status line runs every few seconds per session, and this is a Go
// binary, so the cheaper way is one line of shell in the status line script
// (ember doctor prints it). This command is for those who'd rather not edit it.

// limitsShellLine is that line. $p5 $r5 $p7 $r7 are the status line's own
// variables (percent and reset time of the 5 hour and 7 day windows, "-" when absent).
const limitsShellLine = `[ "$p5" != "-" ] && { d="$XDG_RUNTIME_DIR/ember"; mkdir -p "$d"; printf '{"h5":%s,"h5_at":%s,"d7":%s,"d7_at":%s,"t":%s}\n' "$p5" "$r5" "${p7/-/null}" "${r7/-/null}" "$(date +%s)" >"$d/limits.tmp" && mv "$d/limits.tmp" "$d/limits.json"; }`

type limitsOut struct {
	H5   float64  `json:"h5"`
	H5At int64    `json:"h5_at"`
	D7   *float64 `json:"d7"`
	D7At *int64   `json:"d7_at"`
	T    int64    `json:"t"`
}

// limitsJSON turns the status line's JSON into limits.json's. ok is false
// when it holds no 5 hour limit (an API key, or no session data yet).
func limitsJSON(in []byte, now time.Time) (out []byte, ok bool) {
	var s struct {
		Limits struct {
			Five  window `json:"five_hour"`
			Seven window `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if json.Unmarshal(in, &s) != nil || s.Limits.Five.Used == nil {
		return nil, false
	}
	o := limitsOut{H5: *s.Limits.Five.Used, H5At: s.Limits.Five.At, T: now.Unix()}
	if s.Limits.Seven.Used != nil {
		o.D7, o.D7At = s.Limits.Seven.Used, &s.Limits.Seven.At
	}
	b, err := json.Marshal(o)
	return b, err == nil
}

type window struct {
	Used *float64 `json:"used_percentage"`
	At   int64    `json:"resets_at"`
}

func runLimits([]string) int {
	in, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if b, ok := limitsJSON(in, time.Now()); ok { // a status line must never fail on our account
		dir := filepath.Join(xdg.RuntimeDir(), "ember")
		if os.MkdirAll(dir, 0o700) == nil {
			xdg.AtomicWrite(filepath.Join(dir, "limits.json"), append(b, '\n'), 0o600)
		}
	}
	return 0
}
