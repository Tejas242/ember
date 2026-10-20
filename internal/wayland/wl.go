// Package wayland is the island's Wayland client: the wire protocol spoken
// directly (no toolkit, no libwayland), one layer-shell surface, shared-memory
// buffers, pointer input, blur, cursor shape and idle notifications.
//
// The Window is driven from one goroutine, the caller's loop: it selects on
// Messages and Errors, passes each message to Dispatch, and calls Flush once
// it has drawn.
package wayland

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

type handler func(op uint16, d *dec)

// conn is a minimal Wayland client connection. Only the main loop goroutine
// writes; a reader goroutine hands raw messages over a channel.
type conn struct {
	sock     *net.UnixConn
	handlers map[uint32]handler
	free     []uint32
	next     uint32
	out      []byte
	fds      []int
	msgs     chan Message
	readErr  chan error
}

// Message is one event from the compositor, for Window.Dispatch.
type Message struct {
	obj  uint32
	op   uint16
	data []byte
}

// ErrNoWayland: no compositor to draw on. The service doesn't restart on it
// (the island exits 78, the unit's RestartPreventExitStatus): without a
// graphical session there's nothing to retry.
var ErrNoWayland = errors.New("no Wayland socket")

func waylandSocket() string {
	name := os.Getenv("WAYLAND_DISPLAY")
	if name == "" {
		name = "wayland-0"
	}
	if !filepath.IsAbs(name) {
		name = filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), name)
	}
	return name
}

// WaitSocket returns once the compositor's socket exists, or ErrNoWayland
// after wait (a session still starting gets that long).
func WaitSocket(wait time.Duration) error {
	name := waylandSocket()
	for deadline := time.Now().Add(wait); ; time.Sleep(100 * time.Millisecond) {
		if _, err := os.Stat(name); err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w at %s: not starting without a graphical session", ErrNoWayland, name)
		}
	}
}

func dial() (*conn, error) {
	name := waylandSocket()
	s, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: name, Net: "unix"})
	if err != nil {
		return nil, err
	}
	c := &conn{sock: s, handlers: map[uint32]handler{}, next: 2, msgs: make(chan Message, 256), readErr: make(chan error, 1)}
	c.handlers[1] = c.onDisplay
	go c.read()
	return c, nil
}

func (c *conn) newID(h handler) uint32 {
	var id uint32
	if n := len(c.free); n > 0 {
		id, c.free = c.free[n-1], c.free[:n-1]
	} else {
		id = c.next
		c.next++
	}
	c.handlers[id] = h
	return id
}

func (c *conn) onDisplay(op uint16, d *dec) {
	switch op {
	case 0: // error
		obj, code, m := d.u32(), d.u32(), d.str()
		panic(fmt.Sprintf("wayland error on object %d code %d: %s", obj, code, m))
	case 1: // delete_id
		id := d.u32()
		delete(c.handlers, id)
		c.free = append(c.free, id)
	}
}

type fd int
type wlFixed float64

// send queues a request; flush writes the batch.
func (c *conn) send(obj uint32, op uint16, args ...any) {
	var b []byte
	b = binary.LittleEndian.AppendUint32(b, obj)
	b = binary.LittleEndian.AppendUint32(b, 0) // size|op, patched below
	for _, a := range args {
		switch v := a.(type) {
		case uint32:
			b = binary.LittleEndian.AppendUint32(b, v)
		case int32:
			b = binary.LittleEndian.AppendUint32(b, uint32(v))
		case int:
			b = binary.LittleEndian.AppendUint32(b, uint32(int32(v)))
		case wlFixed:
			b = binary.LittleEndian.AppendUint32(b, uint32(int32(math.Round(float64(v)*256))))
		case string:
			n := len(v) + 1
			b = binary.LittleEndian.AppendUint32(b, uint32(n))
			b = append(b, v...)
			b = append(b, make([]byte, 1+(4-n%4)%4)...) // NUL, then pad to 4
		case fd:
			c.fds = append(c.fds, int(v))
		default:
			panic(fmt.Sprintf("wayland: bad arg %T", a))
		}
	}
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b))<<16|uint32(op))
	c.out = append(c.out, b...)
}

func (c *conn) flush() error {
	if len(c.out) == 0 {
		return nil
	}
	var oob []byte
	if len(c.fds) > 0 {
		oob = unix.UnixRights(c.fds...)
	}
	_, _, err := c.sock.WriteMsgUnix(c.out, oob, nil)
	c.out, c.fds = c.out[:0], c.fds[:0]
	return err
}

func (c *conn) read() {
	buf := make([]byte, 0, 1<<16)
	tmp := make([]byte, 1<<16)
	oob := make([]byte, unix.CmsgSpace(4*28))
	var fds []int
	for {
		n, oobn, _, _, err := c.sock.ReadMsgUnix(tmp, oob)
		if err != nil {
			c.readErr <- err
			return
		}
		if oobn > 0 {
			if cms, err := unix.ParseSocketControlMessage(oob[:oobn]); err == nil {
				for _, cm := range cms {
					if got, err := unix.ParseUnixRights(&cm); err == nil {
						fds = append(fds, got...)
					}
				}
			}
		}
		buf = append(buf, tmp[:n]...)
		for len(buf) >= 8 {
			size := int(binary.LittleEndian.Uint32(buf[4:]) >> 16)
			if size < 8 || len(buf) < size {
				break
			}
			m := Message{obj: binary.LittleEndian.Uint32(buf), op: uint16(binary.LittleEndian.Uint32(buf[4:])), data: append([]byte(nil), buf[8:size]...)}
			// The only fd-carrying events we bind (keymap) are not used; close any we get.
			for _, f := range fds {
				unix.Close(f)
			}
			fds = fds[:0]
			c.msgs <- m
			buf = buf[size:]
		}
		buf = append(buf[:0:0], buf...)
	}
}

func (c *conn) dispatch(m Message) {
	if h := c.handlers[m.obj]; h != nil {
		h(m.op, &dec{b: m.data})
	}
}

// roundtrip blocks until the server has processed everything sent so far.
func (c *conn) roundtrip() error {
	done := false
	cb := c.newID(func(op uint16, d *dec) { done = true })
	c.send(1, 0, cb) // wl_display.sync
	if err := c.flush(); err != nil {
		return err
	}
	for !done {
		select {
		case m := <-c.msgs:
			c.dispatch(m)
		case err := <-c.readErr:
			return err
		}
	}
	return nil
}

type dec struct {
	b []byte
}

func (d *dec) u32() uint32 {
	if len(d.b) < 4 {
		return 0
	}
	v := binary.LittleEndian.Uint32(d.b)
	d.b = d.b[4:]
	return v
}

func (d *dec) i32() int32     { return int32(d.u32()) }
func (d *dec) fixed() float64 { return float64(d.i32()) / 256 }

func (d *dec) str() string {
	n := int(d.u32())
	if n == 0 || len(d.b) < n {
		return ""
	}
	s := string(d.b[:n-1])
	pad := (n + 3) &^ 3
	if pad > len(d.b) {
		pad = len(d.b)
	}
	d.b = d.b[pad:]
	return s
}

// shmBuffer is one ARGB8888 buffer in its own memfd.
type shmBuffer struct {
	id    uint32
	data  []byte
	busy  bool
	stale image.Rectangle // pixels this buffer hasn't received yet
}

func newShmBuffer(c *conn, shm uint32, w, h int) (*shmBuffer, error) {
	size := w * h * 4
	f, err := unix.MemfdCreate("ember", unix.MFD_CLOEXEC)
	if err != nil {
		return nil, err
	}
	if err := unix.Ftruncate(f, int64(size)); err != nil {
		unix.Close(f)
		return nil, err
	}
	data, err := unix.Mmap(f, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(f)
		return nil, err
	}
	b := &shmBuffer{data: data, stale: image.Rect(0, 0, w, h)}
	pool := c.newID(nil)
	c.send(shm, 0, pool, fd(f), int32(size)) // wl_shm.create_pool
	b.id = c.newID(func(op uint16, d *dec) {
		if op == 0 { // release
			b.busy = false
		}
	})
	c.send(pool, 0, b.id, int32(0), int32(w), int32(h), int32(w*4), uint32(0)) // create_buffer, ARGB8888
	c.send(pool, 1)                                                            // pool.destroy (buffer keeps the memory)
	if err := c.flush(); err != nil {
		return nil, err
	}
	unix.Close(f)
	return b, nil
}

var errNoLayerShell = errors.New("compositor has no zwlr_layer_shell_v1")

// Globals lists the interfaces the compositor offers (wl_compositor,
// zwlr_layer_shell_v1, …), for doctor. It gives up after two seconds.
func Globals() ([]string, error) {
	c, err := dial()
	if err != nil {
		return nil, err
	}
	defer c.sock.Close()
	c.sock.SetDeadline(time.Now().Add(2 * time.Second))
	var out []string
	reg := c.newID(func(op uint16, d *dec) {
		if op == 0 { // global: name, interface, version
			d.u32()
			out = append(out, d.str())
		}
	})
	c.send(1, 1, reg) // wl_display.get_registry
	if err := c.roundtrip(); err != nil {
		return nil, err
	}
	return out, nil
}

// Socket is the compositor's socket this process would connect to.
func Socket() string { return waylandSocket() }
