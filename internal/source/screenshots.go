package source

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// screenshotDirs is where screenshots land: Noctalia's setting, else the usual place.
func screenshotDirs() []string {
	h, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(h, "Pictures", "Screenshots")}
	if out, err := exec.Command("noctalia", "config", "export").Output(); err == nil {
		dirs = append(noctaliaShotDirs(string(out), h), dirs...)
	}
	return dirs
}

// noctaliaShotDirs reads the screenshot directory out of Noctalia's config.
func noctaliaShotDirs(config, home string) []string {
	var dirs []string
	inShot := false
	for _, ln := range strings.Split(config, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "[") {
			inShot = strings.HasSuffix(ln, "screenshot]")
			continue
		}
		if k, v, ok := strings.Cut(ln, "="); ok && inShot && strings.TrimSpace(k) == "directory" {
			d := strings.Trim(strings.TrimSpace(v), `"`)
			if strings.HasPrefix(d, "~/") {
				d = filepath.Join(home, d[2:])
			}
			dirs = append([]string{d}, dirs...)
		}
	}
	return dirs
}

// Screenshots says when an image lands in the screenshots folder. Without
// one it returns at once.
func Screenshots(emit Emit) error {
	paths := map[string]uint32{}
	for _, d := range screenshotDirs() {
		paths[d] = unix.IN_CLOSE_WRITE | unix.IN_MOVED_TO
	}
	err := inotify(paths, nil, func(dir, name string) {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".png", ".jpg", ".jpeg":
			emit(Screenshot{Path: filepath.Join(dir, name)})
		}
	})
	if err == errNothing {
		return errors.New("no screenshots folder")
	}
	return err
}
