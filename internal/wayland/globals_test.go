package wayland

import (
	"encoding/binary"
	"net"
	"path/filepath"
	"slices"
	"testing"
)

// msg is one event on the wire: object, opcode, then u32 and string args.
func msg(obj uint32, op uint16, args ...any) []byte {
	var body []byte
	for _, a := range args {
		switch v := a.(type) {
		case uint32:
			body = binary.LittleEndian.AppendUint32(body, v)
		case string:
			n := len(v) + 1
			body = binary.LittleEndian.AppendUint32(body, uint32(n))
			body = append(body, v...)
			body = append(body, make([]byte, 1+(4-n%4)%4)...)
		}
	}
	b := binary.LittleEndian.AppendUint32(nil, obj)
	b = binary.LittleEndian.AppendUint32(b, uint32(8+len(body))<<16|uint32(op))
	return append(b, body...)
}

// Globals lists what a (fake) compositor announces.
func TestGlobals(t *testing.T) {
	d := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", d)
	t.Setenv("WAYLAND_DISPLAY", "wayland-fake")
	ln, err := net.Listen("unix", filepath.Join(d, "wayland-fake"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		n := 0
		for n < 24 { // get_registry (12 bytes) and sync (12 bytes)
			m, err := c.Read(buf[n:])
			if err != nil {
				return
			}
			n += m
		}
		// the registry is id 2, the sync callback id 3
		c.Write(append(append(msg(2, 0, uint32(1), "wl_compositor", uint32(6)),
			msg(2, 0, uint32(2), "zwlr_layer_shell_v1", uint32(4))...), msg(3, 0, uint32(0))...))
		c.Read(buf) // a compositor stays until the client goes
	}()
	got, err := Globals()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"wl_compositor", "zwlr_layer_shell_v1"}) {
		t.Fatalf("got %v", got)
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-none")
	if _, err := Globals(); err == nil {
		t.Fatal("no compositor, no error")
	}
}
