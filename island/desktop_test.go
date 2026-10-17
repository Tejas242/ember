package island

import (
	"testing"
)

func TestSessionTitle(t *testing.T) {
	for in, want := range map[string]string{
		"✳ Ember end-to-end product": "Ember end-to-end product",
		"◐ Fix the flaky test":       "Fix the flaky test",
		"⠂ Claude Code":              "",
		"user@host:~":                "",
		"~/src/ember":                "",
		"nvim main.go":               "",
		"":                           "",
	} {
		if got := sessionTitle(in); got != want {
			t.Errorf("sessionTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
