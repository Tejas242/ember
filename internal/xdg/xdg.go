// Package xdg writes Ember's files safely.
package xdg

import (
	"io/fs"
	"os"
	"path/filepath"
)

// AtomicWrite replaces path with data so that a reader (or a crash) sees the
// old file or the new one, never half of either: a temp file in the same
// directory, synced, then renamed over path. An existing file keeps its mode
// (perm is for a new one), and a symlink keeps pointing where it did: its
// target is what gets replaced.
func AtomicWrite(path string, data []byte, perm fs.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // gone after the rename; cleans up after a failure
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil { // the rename itself survives a crash
		d.Sync()
		d.Close()
	}
	return nil
}
