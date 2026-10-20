// Package notifybuild tests the notify-build script (a shell example of
// Ember's one-liners) with a fake ember on the PATH.
package notifybuild

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (calls []string, code int) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	fake := "#!/bin/sh\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "ember"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("./notify-build", args...)
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin")
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	return strings.Split(strings.TrimSpace(string(b)), "\n"), code
}

func TestPassed(t *testing.T) {
	calls, code := run(t, "--open", "https://ci.example.com/81", "true")
	if code != 0 || len(calls) != 3 {
		t.Fatalf("exit %d, calls %q", code, calls)
	}
	if !strings.HasPrefix(calls[0], "status build-") || !strings.Contains(calls[0], " true --sub true --icon run --mood work --since now --ttl 2h") {
		t.Errorf("start: %q", calls[0])
	}
	if !strings.HasSuffix(calls[1], "--clear") {
		t.Errorf("clear: %q", calls[1])
	}
	if calls[2] != "note true passed --sub in 0s --icon check --mood done --open https://ci.example.com/81" {
		t.Errorf("note: %q", calls[2])
	}
}

func TestFailed(t *testing.T) {
	calls, code := run(t, "sh", "-c", "exit 3")
	if code != 3 || len(calls) != 3 || calls[2] != "note sh failed --sub exit 3 after 0s --icon warning --mood fail" {
		t.Fatalf("exit %d, calls %q", code, calls)
	}
}

func TestUsage(t *testing.T) {
	if _, code := run(t); code != 2 {
		t.Fatal("no command: exit 2")
	}
}
