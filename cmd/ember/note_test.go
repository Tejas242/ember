package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/Tejas242/ember/internal/ipc"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// fakeIsland listens on the island's socket and returns what each
// one-liner sent.
func fakeIsland(t *testing.T) <-chan ipc.Msg {
	t.Helper()
	os.MkdirAll(strings.TrimSuffix(ipc.SockPath(), "/ember.sock"), 0o700)
	ln, err := net.Listen("unix", ipc.SockPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan ipc.Msg, 8)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			line, _ := bufio.NewReader(c).ReadBytes('\n')
			var m ipc.Msg
			json.Unmarshal(line, &m)
			got <- m
			c.Close()
		}
	}()
	return got
}

func runArgs(argv ...string) (int, string) {
	var out, errb bytes.Buffer
	c, args, flags, code := route(argv, &out, &errb)
	if c == nil {
		return code, errb.String()
	}
	return c.runf(args, flags), ""
}

func TestNoteAndStatus(t *testing.T) {
	if code, _ := runArgs("note", "Build passed"); code != 1 {
		t.Fatalf("no island: exit %d, want 1", code)
	}
	got := fakeIsland(t)

	if code, _ := runArgs("note", "Build passed", "--icon", "check", "--mood=done", "--sub", "42 tests", "--open", "https://ci.example.com/81", "--ttl", "8"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	m := <-got
	pm, err := proto.Decode(m.Payload)
	if err != nil || m.Type != "note" || pm.Moment.Text != "Build passed" || pm.Moment.Icon != "check" || pm.Moment.Mood != "done" ||
		pm.Moment.Sub != "42 tests" || pm.Moment.URL != "https://ci.example.com/81" || pm.Moment.TTLs != 8 {
		t.Fatalf("%v %+v %+v", err, m, pm.Moment)
	}

	if code, _ := runArgs("status", "deploy", "Deploying", "--progress", "40%", "--since", "now", "--ttl", "2h", "--urgency", "high"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	m = <-got
	pm, err = proto.Decode(m.Payload)
	s := pm.Status
	if err != nil || m.Type != "line" || m.ID != "deploy" || s.Text != "Deploying" || *s.Progress != 0.4 || s.SinceMs == 0 || s.TTLs != 7200 || s.Urgency != "high" || s.ID != "deploy" {
		t.Fatalf("%v %+v %+v", err, m, s)
	}
	if code, _ := runArgs("status", "deploy", "--clear"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if m = <-got; m.Type != "line" || m.Event != "clear" || m.ID != "deploy" {
		t.Fatalf("%+v", m)
	}

	for _, tc := range []struct {
		argv []string
		err  string
	}{
		{[]string{"note"}, "needs the note's text"},
		{[]string{"note", "x", "--colour", "red"}, `unexpected argument "--colour"`},
		{[]string{"note", "x", "--icon"}, "--icon needs a value"},
		{[]string{"note", "x", "--open", "not a url"}, "--open needs a URL"},
		{[]string{"note", "x", "--ttl", "soon"}, "--ttl is seconds"},
		{[]string{"status", "deploy"}, "needs the line's text, or --clear"},
		{[]string{"status", "Deploy!", "x"}, "the id is 1–32"},
		{[]string{"status", "deploy", "x", "--progress", "2"}, "--progress is 0..1"},
		{[]string{"status", "deploy", "x", "--ttl", "3d"}, "--ttl is a duration"},
		{[]string{"status", "deploy", "--clear", "--sub", "x"}, "--clear takes only the id"},
	} {
		// usage errors print to stderr: capture it
		r, w, _ := os.Pipe()
		old := os.Stderr
		os.Stderr = w
		code, routed := runArgs(tc.argv...)
		w.Close()
		os.Stderr = old
		var b bytes.Buffer
		b.ReadFrom(r)
		if msg := routed + b.String(); code != 2 || !strings.Contains(msg, tc.err) {
			t.Errorf("%q: exit %d %q, want %q", tc.argv, code, msg, tc.err)
		}
	}
}
