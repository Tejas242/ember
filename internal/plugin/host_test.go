package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/testenv"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// The test binary is its own plugin: `-fake <mode>` makes it one.
func TestMain(m *testing.M) {
	for i, a := range os.Args {
		if a == "-fake" && i+1 < len(os.Args) {
			fakePlugin(os.Args[i+1])
			os.Exit(0)
		}
	}
	openTarget = func(t []string, dir string) { opened = append(opened, t...) } // never a real browser
	backoffMin, healthyRun, helloWithin, stopGrace = 20*time.Millisecond, time.Hour, 500*time.Millisecond, 500*time.Millisecond
	testenv.Main(m)
}

var opened []string

func say(v string) { fmt.Println(v) }

func fakePlugin(mode string) {
	in := bufio.NewScanner(os.Stdin)
	in.Scan() // Ember's hello
	var hello proto.HelloEmber
	json.Unmarshal(in.Bytes(), &hello)
	switch mode {
	case "nohello":
		time.Sleep(10 * time.Second)
		return
	case "wrongname":
		say(`{"type":"hello","v":1,"name":"someone-else"}`)
		time.Sleep(10 * time.Second)
		return
	}
	say(`{"type":"hello","v":1,"name":"fake"}`)
	switch mode {
	case "crash":
		os.Exit(3)
	case "good":
		say(fmt.Sprintf(`{"type":"log","text":"caps %s"}`, strings.Join(hello.Caps, ",")))
		say(`{"type":"status","text":"Building","mood":"work","progress":0.5,"future_field":1}`)
		say(`{"type":"row","title":"Build","sub":"step 3"}`)
		say(`{"type":"moment","text":"Started","ttl_s":60}`)
		say(`{"type":"teleport","to":"mars"}`) // a newer type: ignored
	case "burst":
		for i := 0; i < 50; i++ {
			say(fmt.Sprintf(`{"type":"status","text":"step %d"}`, i))
		}
		say(`{"type":"moment","text":"one"}`)
		say(`{"type":"moment","text":"two"}`) // within 10 s: dropped
	case "flood": // as fast as it can, for as long as it's let
		for i := 0; ; i++ {
			fmt.Printf("{\"type\":\"status\",\"text\":\"%d\"}\n", i)
		}
	case "garbage": // a stream of lines Ember refuses
		for {
			say(`{"type":"status"}`)
			time.Sleep(time.Millisecond)
		}
	case "beyond":
		say(`{"type":"row","title":"not allowed"}`) // no row capability
		say(`{"type":"status","text":""}`)          // no text: a violation
		say(`not json`)
		say(`{"type":"status","text":"fine"}`)
	}
	for in.Scan() { // events, until stop
		var ev proto.Event
		json.Unmarshal(in.Bytes(), &ev)
		if ev.Type == "stop" {
			return
		}
		say(fmt.Sprintf(`{"type":"log","text":"got %s %s%s"}`, ev.Type, ev.Target, ev.Key))
	}
}

type sink struct {
	mu   sync.Mutex
	msgs []proto.Message
	gone int
}

func (s *sink) Apply(name string, m proto.Message) {
	s.mu.Lock()
	s.msgs = append(s.msgs, m)
	s.mu.Unlock()
}
func (s *sink) Gone(name string) { s.mu.Lock(); s.gone++; s.mu.Unlock() }

func (s *sink) types() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, m := range s.msgs {
		out = append(out, m.Type)
	}
	return out
}

// fakeHost makes a plugins dir with one plugin "fake" in mode, and a host.
func fakeHost(t *testing.T, mode string, caps ...string) (*Host, *sink, context.CancelFunc) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "fake")
	os.Mkdir(dir, 0o755)
	if caps == nil {
		caps = []string{"pill", "row", "moment"}
	}
	q := func(ss []string) string { b, _ := json.Marshal(ss); return string(b) }
	toml := fmt.Sprintf("name = \"fake\"\ncommand = %q\nargs = %s\ncapabilities = %s\n[targets]\ndocs = \"https://example.com\"\n",
		os.Args[0], q([]string{"-fake", mode}), q(caps))
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &sink{}
	h := NewHost(root, "v9-test", s)
	ctx, cancel := context.WithCancel(context.Background())
	h.Start(ctx)
	t.Cleanup(func() { cancel(); h.Stop() })
	return h, s, cancel
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("never: " + what)
}

func info(h *Host) Info {
	for _, i := range h.Info() {
		if i.Name == "fake" {
			return i
		}
	}
	return Info{}
}

func TestHandshakeAndMessages(t *testing.T) {
	h, s, cancel := fakeHost(t, "good")
	eventually(t, "status, row and moment applied", func() bool { return len(s.types()) == 3 })
	if got := strings.Join(s.types(), ","); got != "status,row,moment" {
		t.Fatal(got)
	}
	s.mu.Lock()
	st, mo := s.msgs[0].Status, s.msgs[2].Moment
	s.mu.Unlock()
	if st.Text != "Building" || *st.Progress != 0.5 || mo.TTLs != 10 {
		t.Fatalf("status %+v moment %+v (ttl is capped at 10)", st, mo)
	}
	if i := info(h); i.State != Running {
		t.Fatalf("state %s", i.State)
	}
	h.Click("fake", "docs")
	h.Send("fake", proto.Event{Type: "button", Card: "c", Key: "go"})
	defer func() { t.Logf("info: %+v", info(h)) }()
	eventually(t, "events reach the plugin", func() bool {
		l := strings.Join(info(h).Log, "|")
		return strings.Contains(l, "caps pill,row,moment") && strings.Contains(l, "got click docs") && strings.Contains(l, "got button go")
	})
	if fmt.Sprint(opened) != "[https://example.com]" {
		t.Fatalf("opened %v", opened)
	}
	cancel()
	h.Stop()
	if i := info(h); i.State != Stopped {
		t.Fatalf("after stop: %s", i.State)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gone != 1 {
		t.Fatalf("gone %d", s.gone)
	}
}

func TestCoalescingAndDrops(t *testing.T) {
	h, s, _ := fakeHost(t, "burst")
	eventually(t, "the last status", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, m := range s.msgs {
			if m.Type == "status" && m.Status.Text == "step 49" {
				return true
			}
		}
		return false
	})
	n := strings.Count(strings.Join(s.types(), ","), "status")
	if n > 5 {
		t.Fatalf("50 statuses in a burst applied %d times: not coalesced", n)
	}
	if m := strings.Count(strings.Join(s.types(), ","), "moment"); m != 1 {
		t.Fatalf("%d moments: one every 10 s", m)
	}
	if i := info(h); i.Dropped < 40 || i.Violations != 0 {
		t.Fatalf("dropped %d violations %d", i.Dropped, i.Violations)
	}
}

func TestBeyondItsCapabilities(t *testing.T) {
	h, s, _ := fakeHost(t, "beyond", "pill")
	eventually(t, "the good status", func() bool { return len(s.types()) == 1 })
	if s.types()[0] != "status" {
		t.Fatal(s.types())
	}
	eventually(t, "three violations", func() bool { return info(h).Violations == 3 })
}

func TestNoHello(t *testing.T) {
	h, _, _ := fakeHost(t, "nohello")
	eventually(t, "no hello", func() bool { return strings.Contains(info(h).LastError, "no hello") })
}

func TestWrongName(t *testing.T) {
	h, _, _ := fakeHost(t, "wrongname")
	eventually(t, "wrong name", func() bool { return strings.Contains(info(h).LastError, "someone-else") })
}

func TestCrashLoopSuspends(t *testing.T) {
	h, s, _ := fakeHost(t, "crash")
	eventually(t, "suspended", func() bool { return info(h).State == Suspended })
	i := info(h)
	if !strings.Contains(i.LastError, "exited 5 times") || i.Restarts != 4 {
		t.Fatalf("%+v", i)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gone != 5 {
		t.Fatalf("gone %d", s.gone)
	}
}

func TestManifestChangeLiftsSuspension(t *testing.T) {
	h, _, _ := fakeHost(t, "crash")
	eventually(t, "suspended", func() bool { return info(h).State == Suspended })
	p := filepath.Join(info(h).Dir, "plugin.toml")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(b), `"crash"`, `"good"`, 1)), 0o644)
	later := time.Now().Add(time.Second)
	os.Chtimes(p, later, later)
	h.Scan()
	eventually(t, "running again", func() bool { return info(h).State == Running })
}

func TestScanStates(t *testing.T) {
	root := t.TempDir()
	for name, toml := range map[string]string{
		"broken": "name = \"broken\"\n",                    // no command
		"misfit": "name = \"other\"\ncommand = \"true\"\n", // not its directory's name
		"edge":   "name = \"edge\"\ncommand = \"true\"\n",  // a built-in's name
		"quiet":  "name = \"quiet\"\ncommand = \"true\"\n", // turned off
		"none":   "",                                       // no manifest at all: not a plugin
	} {
		os.Mkdir(filepath.Join(root, name), 0o755)
		if toml != "" {
			os.WriteFile(filepath.Join(root, name, "plugin.toml"), []byte(toml), 0o644)
		}
	}
	h := NewHost(root, "v", &sink{})
	h.Reserved = func(n string) bool { return n == "edge" }
	h.Enabled = func(n string) bool { return n != "quiet" }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.Start(ctx)
	got := map[string]string{}
	for _, i := range h.Info() {
		got[i.Name] = i.State
	}
	want := map[string]string{"broken": Broken, "misfit": Broken, "edge": Broken, "quiet": Off}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// The pomodoro example, built and run by the real host from its own manifest.
func TestPomodoroUnderTheHost(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil || testing.Short() {
		t.Skip("no go to build the example, or -short")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "pomodoro")
	os.Mkdir(dir, 0o755)
	build := exec.Command(gobin, "build", "-o", filepath.Join(dir, "pomodoro"), "../../plugins/pomodoro")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	toml, _ := os.ReadFile("../../plugins/pomodoro/plugin.toml")
	os.WriteFile(filepath.Join(dir, "plugin.toml"), toml, 0o644)
	s := &sink{}
	h := NewHost(root, "v", s)
	ctx, cancel := context.WithCancel(context.Background())
	h.Start(ctx)
	eventually(t, "its row", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, m := range s.msgs {
			if m.Type == "row" && m.Row.Title == "Pomodoro" {
				return true
			}
		}
		return false
	})
	h.Click("pomodoro", "")
	eventually(t, "a focus session", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, m := range s.msgs {
			if m.Type == "status" && m.Status.Text == "Focus" && m.Status.UntilMs > 0 {
				return true
			}
		}
		return false
	})
	cancel()
	h.Stop()
	for _, i := range h.Info() {
		if i.State != Stopped || i.Violations != 0 {
			t.Fatalf("%+v", i)
		}
	}
}

// A plugin that floods is coalesced while it does, then suspended; one that
// sends nothing but bad lines is suspended for its violations.
func TestFloodSuspends(t *testing.T) {
	h, s, _ := fakeHost(t, "flood")
	start := time.Now()
	eventually(t, "suspended", func() bool { return info(h).State == Suspended })
	took := time.Since(start)
	i := info(h)
	if !strings.Contains(i.LastError, "flooding") || took > 5*time.Second {
		t.Fatalf("after %v: %+v", took, i)
	}
	n := strings.Count(strings.Join(s.types(), ","), "status")
	if limit := int(took.Seconds()*10) + 3; n > limit {
		t.Fatalf("%d statuses reached the island in %v: more than 10 a second", n, took)
	}
	if i.Dropped < 1000 {
		t.Fatalf("dropped %d", i.Dropped)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gone != 1 {
		t.Fatal("a suspended plugin's things go")
	}
}

func TestViolationsSuspend(t *testing.T) {
	h, s, _ := fakeHost(t, "garbage")
	eventually(t, "suspended", func() bool { return info(h).State == Suspended })
	if i := info(h); !strings.Contains(i.LastError, "bad messages within a minute") || len(s.types()) != 0 {
		t.Fatalf("%+v %v", i, s.types())
	}
}
