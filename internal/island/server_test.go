package island

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
)

// An edge message is answered: ok when the edge plugin's process took it,
// not ok when no plugin runs or the message is over the limit. Other
// messages get no answer.
func TestEdgeAck(t *testing.T) {
	posts := make(chan func(*app), 4)
	srv, err := listen(posts)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	clock := time.Now()
	a := newTestApp(&clock)
	done := make(chan struct{})
	defer close(done)
	go func() { // the island's loop
		for {
			select {
			case p := <-posts:
				p(a)
			case <-done:
				return
			}
		}
	}()
	if ipc.Edge([]byte(`{"type":"status"}`)) {
		t.Fatal("ok with no plugin running")
	}
	h := &fakeHost{}
	posts <- func(a *app) { a.host = h }
	if !ipc.Edge([]byte(`{"type":"status"}`)) {
		t.Fatal("not ok with the plugin running")
	}
	big, _ := json.Marshal(map[string]string{"x": strings.Repeat("a", maxToolPayload)})
	if ipc.Edge(big) {
		t.Fatal("ok for a message over the limit")
	}

	c, err := net.Dial("unix", ipc.SockPath())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	b, _ := json.Marshal(ipc.Msg{Type: "plugin", Name: "edge", Payload: []byte(`{}`)})
	c.Write(append(b, '\n'))
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if line, err := bufio.NewReader(c).ReadBytes('\n'); err == nil {
		t.Fatalf("a plugin message was answered: %s", line)
	}
	synced := make(chan struct{})
	posts <- func(a *app) { close(synced) }
	<-synced
	if n := len(h.of("tool")); n != 2 {
		t.Fatalf("%d tool events", n)
	}
}
