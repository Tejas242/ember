package island

import (
	"strings"
	"time"
)

// Small words about a session, for the card.

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
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
