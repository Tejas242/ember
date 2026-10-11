package island

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Ancestors is the calling process's parent chain (shell, claude, terminal…),
// so the island can find the session's terminal window.
func Ancestors() []int {
	var out []int
	pid := os.Getppid()
	for i := 0; i < 8 && pid > 1; i++ {
		out = append(out, pid)
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			break
		}
		// pid (comm) state ppid ...; comm may contain spaces, so cut after ')'
		s := string(b)
		f := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
		if len(f) < 2 {
			break
		}
		pid, _ = strconv.Atoi(f[1])
	}
	return out
}

func dialIsland() (net.Conn, error) {
	return net.DialTimeout("unix", SockPath(), 150*time.Millisecond)
}

// Send tells the island about a hook event without waiting. Silent if the
// island isn't running.
func Send(event string, payload []byte) {
	c, err := dialIsland()
	if err != nil {
		return
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(300 * time.Millisecond))
	b, _ := json.Marshal(inMsg{Type: "hook", Event: event, Pids: Ancestors(), Payload: payload})
	c.Write(append(b, '\n'))
}

// Ask shows a hook on the island and waits for the user's answer: the hook
// output to print, or nil to fall back to Claude's own prompt. It returns nil
// at once if the island isn't running, and after AskTimeout at the latest.
func Ask(event string, payload []byte) []byte {
	c, err := dialIsland()
	if err != nil {
		return nil
	}
	defer c.Close()
	b, _ := json.Marshal(inMsg{Type: "hook", Event: event, Wait: true, Pids: Ancestors(), Payload: payload})
	c.SetDeadline(time.Now().Add(AskTimeout + 5*time.Second))
	if _, err := c.Write(append(b, '\n')); err != nil {
		return nil
	}
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return nil
	}
	var r reply
	if json.Unmarshal(line, &r) != nil || len(r.Output) == 0 || string(r.Output) == "null" {
		return nil
	}
	return r.Output
}

// Edge sends one message from edge (status, card, remove) to the island.
func Edge(raw []byte) bool { return oneShot(inMsg{Type: "edge", Payload: raw}) }

// Cmd sends a keyboard command (allow, deny, always, terminal, answer <key>).
func Cmd(cmd, arg string) bool { return oneShot(inMsg{Type: "cmd", Event: cmd, ID: arg}) }

func oneShot(m inMsg) bool {
	c, err := dialIsland()
	if err != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(500 * time.Millisecond))
	b, _ := json.Marshal(m)
	_, err = c.Write(append(b, '\n'))
	return err == nil
}
