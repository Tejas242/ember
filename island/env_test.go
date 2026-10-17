package island

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tejas242/ember/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

// No test sees this machine's home: the palette is the built-in one and every
// path Ember reads or writes is under the sandbox.
func TestHermetic(t *testing.T) {
	h, _ := os.UserHomeDir()
	if !strings.HasPrefix(h, os.TempDir()) {
		t.Fatalf("HOME is %s", h)
	}
	for _, p := range []string{themePath(), ConfigPath(), habitsPath(), SockPath(), sessionsPath()} {
		if rel, err := filepath.Rel(h, p); err != nil || strings.HasPrefix(rel, "..") {
			t.Errorf("%s is outside the sandbox", p)
		}
	}
	if _, err := os.Stat(themePath()); err == nil {
		t.Fatal("a theme in the sandbox")
	}
}
