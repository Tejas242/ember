package island

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// A save that fails says so once, not every ten seconds.
func TestSaveFailureLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	p := filepath.Join(t.TempDir(), "no", "such", "dir", "state.json")
	for i := 0; i < 3; i++ {
		save(p, []byte("{}"), 0o644)
	}
	if n := strings.Count(buf.String(), "can't save"); n != 1 {
		t.Fatalf("logged %d times: %q", n, buf.String())
	}
}

// A hook Ember can't read says so once per event, and is still taken.
func TestBadHookLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	clock := time.Now()
	a := newTestApp(&clock)
	for i := 0; i < 3; i++ {
		a.hook(ipc.Msg{Type: "hook", Event: "Stop", Payload: []byte(`{"session_id": 5}`)}, nil)
	}
	if n := strings.Count(buf.String(), "a Stop hook Ember can't read"); n != 1 {
		t.Fatalf("logged %d times: %q", n, buf.String())
	}
}
