package adapter

import (
	"testing"
	"time"
)

// A "stats" message without a body is edge saying the stats are too big to
// push: the adapter reads the file instead.
func TestStatsPingRereadsTheFile(t *testing.T) {
	read := 0
	a := &adapter{o: Options{Now: time.Now}, reread: func() { read++ }}
	a.message(Message{Type: "stats"})
	if read != 1 {
		t.Fatalf("a stats ping read the file %d times, want 1", read)
	}
}
