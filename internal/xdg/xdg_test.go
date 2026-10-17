package xdg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWrite(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "settings.json")

	if err := AtomicWrite(p, []byte("one"), 0o640); err != nil { // a new file gets perm
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "one" {
		t.Fatalf("got %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o640 {
		t.Fatalf("new file mode %v", fi.Mode().Perm())
	}

	os.Chmod(p, 0o600) // an existing file keeps its mode
	if err := AtomicWrite(p, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode changed to %v", fi.Mode().Perm())
	}
	if b, _ := os.ReadFile(p); string(b) != "two" {
		t.Fatalf("got %q", b)
	}

	link := filepath.Join(d, "link.json") // a symlink stays a symlink
	os.Symlink(p, link)
	if err := AtomicWrite(link, []byte("three"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced by a file")
	}
	if b, _ := os.ReadFile(p); string(b) != "three" {
		t.Fatalf("target got %q", b)
	}

	if es, _ := os.ReadDir(d); len(es) != 2 { // no temp files left behind
		t.Fatalf("left %d entries", len(es))
	}
	if err := AtomicWrite(filepath.Join(d, "missing", "x"), nil, 0o644); err == nil {
		t.Fatal("wrote into a missing directory")
	}
}

func TestDirs(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	if ConfigHome() != "/h/.config" || StateHome() != "/h/.local/state" || RuntimeDir() != os.TempDir() {
		t.Fatal(ConfigHome(), StateHome(), RuntimeDir())
	}
	t.Setenv("XDG_CONFIG_HOME", "/c")
	t.Setenv("XDG_STATE_HOME", "/s")
	t.Setenv("XDG_RUNTIME_DIR", "/r")
	if ConfigHome() != "/c" || StateHome() != "/s" || RuntimeDir() != "/r" {
		t.Fatal(ConfigHome(), StateHome(), RuntimeDir())
	}
}
