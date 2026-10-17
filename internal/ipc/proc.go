package ipc

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Ancestors is the calling process's parent chain (shell, claude, terminal…),
// so the island can find the session's terminal window.
func Ancestors() []int {
	var out []int
	pid := os.Getppid()
	for i := 0; i < 8 && pid > 1; i++ {
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			break
		}
		// pid (comm) state ppid ...; comm may contain spaces, so cut after ')'
		s := string(b)
		if comm := s[strings.IndexByte(s, '(')+1 : strings.LastIndexByte(s, ')')]; comm == "systemd" || comm == "init" {
			break // the session manager outlives every session: it would keep this one forever
		}
		out = append(out, pid)
		f := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
		if len(f) < 2 {
			break
		}
		pid, _ = strconv.Atoi(f[1])
	}
	return out
}

// Alive reports whether pid is a running process.
func Alive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }
