package ipc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Ancestors never include the session manager, which would keep a dead
// session alive forever.
func TestAncestorsStopAtSystemd(t *testing.T) {
	for _, p := range Ancestors() {
		b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", p))
		if c := strings.TrimSpace(string(b)); c == "systemd" || p == 1 {
			t.Fatalf("pid %d (%s) in ancestors", p, c)
		}
	}
}
