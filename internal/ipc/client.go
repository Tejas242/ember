package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"time"
)

func dialIsland() (net.Conn, error) {
	return net.DialTimeout("unix", SockPath(), 150*time.Millisecond)
}

// DemoSession starts the session id of every session ember demo plays.
const DemoSession = "ember-demo-"

// Send tells the island about a hook event without waiting. Silent if the
// island isn't running.
func Send(event string, payload []byte) { SendHook(event, payload, Ancestors()) }

// SendHook is Send for a hook whose process tree is pids (nil: none, so no
// terminal is its own).
func SendHook(event string, payload []byte, pids []int) {
	c, err := dialIsland()
	if err != nil {
		return
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(300 * time.Millisecond))
	b, _ := json.Marshal(Msg{Type: "hook", Event: event, Pids: pids, Payload: payload})
	c.Write(append(b, '\n'))
}

// Edge sends one message from edge (status, card, remove) to the island and
// reports whether the edge plugin got it: the island answers {"ok":…}; an
// island too old to answer within 300 ms counts as delivered.
func Edge(raw []byte) bool {
	c, err := dialIsland()
	if err != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(500 * time.Millisecond))
	b, _ := json.Marshal(Msg{Type: "edge", Payload: raw})
	if _, err := c.Write(append(b, '\n')); err != nil {
		return false
	}
	c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	line, err := bufio.NewReader(c).ReadBytes('\n')
	var ack Ack
	if err != nil || json.Unmarshal(line, &ack) != nil {
		return true
	}
	return ack.OK
}

// Note shows a moment: payload is a protocol moment line.
func Note(payload []byte) bool { return oneShot(Msg{Type: "note", Payload: payload}) }

// SetStatus holds the pill line id: payload is a protocol status.
func SetStatus(id string, payload []byte) bool {
	return oneShot(Msg{Type: "line", ID: id, Payload: payload})
}

// ClearStatus takes the pill line id away.
func ClearStatus(id string) bool { return oneShot(Msg{Type: "line", ID: id, Event: "clear"}) }

// Cmd sends a keyboard command (open, close, toggle, next, prev, play, dash <name>).
func Cmd(cmd, arg string) bool { return oneShot(Msg{Type: "cmd", Event: cmd, ID: arg}) }

func oneShot(m Msg) bool {
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

// QueryStatus asks the running island what it's connected to; ok is false if
// it isn't running.
func QueryStatus() (st Status, ok bool) {
	c, err := dialIsland()
	if err != nil {
		return st, false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	b, _ := json.Marshal(Msg{Type: "status"})
	if _, err := c.Write(append(b, '\n')); err != nil {
		return st, false
	}
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return st, false
	}
	return st, json.Unmarshal(line, &st) == nil
}
