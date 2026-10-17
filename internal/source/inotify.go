package source

import (
	"bytes"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// errNothing: a watcher with nothing to watch (no backlight, no folder).
var errNothing = errors.New("nothing to watch")

// inotify watches each path for its mask and calls got with the path and
// the changed file's name ("" for the path itself) until reading fails. It
// returns errNothing at once if it could watch nothing. ready runs once the
// watches are up, before the first event.
func inotify(paths map[string]uint32, ready func(), got func(path, name string)) error {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return fmt.Errorf("inotify: %w", err)
	}
	defer unix.Close(fd)
	wds := map[int32]string{}
	for p, mask := range paths {
		if wd, err := unix.InotifyAddWatch(fd, p, mask); err == nil {
			wds[int32(wd)] = p
		}
	}
	if len(wds) == 0 {
		return errNothing
	}
	if ready != nil {
		ready()
	}
	buf := make([]byte, 64<<10)
	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return fmt.Errorf("inotify: %w", err)
		}
		for off := 0; off+unix.SizeofInotifyEvent <= n; {
			ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
			name := ""
			if ev.Len > 0 {
				raw := buf[off+unix.SizeofInotifyEvent : off+unix.SizeofInotifyEvent+int(ev.Len)]
				name = string(bytes.TrimRight(raw, "\x00"))
			}
			off += unix.SizeofInotifyEvent + int(ev.Len)
			if p, ok := wds[ev.Wd]; ok {
				got(p, name)
			}
		}
	}
}
