package wayland

import (
	"image"
	"log"
	"math"
	"slices"
	"time"
	"unsafe"
)

// Layer is a layer-shell layer (wlr-layer-shell-unstable-v1).
type Layer uint32

// LayerTop is above windows, with the bars.
const LayerTop Layer = 2

const (
	anchorTop    = 1
	shapeDefault = 1
	shapePointer = 4
)

// PointerEvent is the pointer over the surface.
type PointerEvent struct {
	Kind   string // enter | motion | leave | press | release | scroll
	X, Y   float64
	Button int     // press: 1 left, 2 middle, 3 right
	Steps  float64 // scroll: wheel notches, positive = down
}

// Caps are the optional protocols the compositor has.
type Caps struct {
	Blur        bool // ext-background-effect-v1
	CursorShape bool // wp-cursor-shape-v1
	Idle        bool // ext-idle-notify-v1
}

// Window is the island's one layer surface. Its callbacks run inside
// Dispatch, on the caller's goroutine.
type Window struct {
	OnPointer func(PointerEvent)
	OnFrame   func()          // the compositor wants the next frame
	OnIdle    func(idle bool) // you went away (true) or came back

	c                                  *conn
	compositor, shm, layerShell, seat  uint32
	cursorMgr                          uint32
	blurMgr, blurSurf                  uint32
	lastBlur                           []image.Rectangle
	surface, layer, pointer, cursorDev uint32
	w, h                               int
	bufs                               [3]*shmBuffer // three: one on screen, one queued, one to draw into
	configured, framePending, closed   bool
	enterSerial                        uint32
	px, py                             float64
	scroll                             float64
	layerKind                          uint32
	retire                             func() // the surface a raise replaced, to destroy once the new one shows
	marginTop                          int
	idleMgr, idleVer                   uint32
	idleNote                           uint32        // the idle notification, remade when away_after_s changes
	idleFor                            time.Duration // what it was asked for
	lastInput                          image.Rectangle
	lastShape                          uint32
	dmg, region                        []image.Rectangle // Present's scratch
}

// Open makes a w×h surface on layer, anchored to the top edge marginTop px down.
func Open(w, h, marginTop int, layer Layer) (*Window, error) {
	c, err := dial()
	if err != nil {
		return nil, err
	}
	win := &Window{c: c, w: w, h: h}
	reg := c.newID(nil)
	c.handlers[reg] = func(op uint16, d *dec) {
		if op != 0 { // global_remove
			return
		}
		name, iface, ver := d.u32(), d.str(), d.u32()
		bind := func(want uint32, h handler) uint32 {
			id := c.newID(h)
			c.send(reg, 0, name, iface, min(ver, want), id)
			return id
		}
		switch iface {
		case "wl_compositor":
			win.compositor = bind(4, nil)
		case "wl_shm":
			win.shm = bind(1, nil)
		case "zwlr_layer_shell_v1":
			win.layerShell = bind(4, nil)
		case "wp_cursor_shape_manager_v1":
			win.cursorMgr = bind(1, nil)
		case "ext_background_effect_manager_v1":
			win.blurMgr = bind(1, nil)
		case "ext_idle_notifier_v1":
			win.idleVer = min(ver, 2)
			win.idleMgr = bind(2, nil)
		case "wl_seat":
			if win.seat == 0 {
				win.seat = bind(5, win.onSeat)
			}
		}
	}
	c.send(1, 1, reg) // wl_display.get_registry
	if err := c.roundtrip(); err != nil {
		return nil, err
	}
	if win.layerShell == 0 || win.compositor == 0 || win.shm == 0 {
		return nil, errNoLayerShell
	}
	if err := c.roundtrip(); err != nil { // seat capabilities
		return nil, err
	}
	win.layerKind, win.marginTop = uint32(layer), marginTop
	win.createSurface()
	if err := c.roundtrip(); err != nil {
		return nil, err
	}
	for i := range win.bufs {
		if win.bufs[i], err = newShmBuffer(c, win.shm, w, h); err != nil {
			return nil, err
		}
	}
	return win, nil
}

// WatchIdle asks to hear when you've been away from the keyboard and mouse
// for d, and when you come back (ext-idle-notify-v1). Version 2's input
// idle ignores inhibitors, so a playing video doesn't count as you.
func (win *Window) WatchIdle(d time.Duration) {
	if win.idleMgr == 0 || win.seat == 0 || d == win.idleFor {
		return
	}
	if win.idleNote != 0 {
		win.c.send(win.idleNote, 0) // ext_idle_notification_v1.destroy
	}
	id := win.c.newID(func(op uint16, _ *dec) {
		if win.OnIdle != nil {
			win.OnIdle(op == 0) // 0 idled, 1 resumed
		}
	})
	op := uint16(1) // get_idle_notification
	if win.idleVer >= 2 {
		op = 2 // get_input_idle_notification
	}
	win.c.send(win.idleMgr, op, id, uint32(d.Milliseconds()), win.seat)
	win.idleNote, win.idleFor = id, d
}

// createSurface makes the island's surface and its layer role; the
// compositor answers with a configure before the first buffer may go up.
func (win *Window) createSurface() {
	c := win.c
	win.surface = c.newID(nil)
	c.send(win.compositor, 0, win.surface)
	win.layer = c.newID(win.onLayer)
	c.send(win.layerShell, 0, win.layer, win.surface, uint32(0), win.layerKind, "ember")
	c.send(win.layer, 0, uint32(win.w), uint32(win.h))  // set_size
	c.send(win.layer, 1, uint32(anchorTop))             // set_anchor
	c.send(win.layer, 2, int32(-1))                     // set_exclusive_zone -1: sit on top of the bar, reserve nothing
	c.send(win.layer, 3, int32(win.marginTop), 0, 0, 0) // set_margin
	c.send(win.layer, 4, uint32(0))                     // keyboard: none, never steal focus
	win.setInput(image.Rectangle{})
	c.send(win.surface, 6) // commit
}

// Raise puts the island back on top of its layer. Layer-shell has no
// "raise": the newest surface of a layer is drawn above the older ones, so
// when the bar is re-created (the shell restarted, or a bar setting
// changed) it covers us. Making our surface again puts it above. The old
// one stays up until the new one has shown its first frame: no blink.
func (win *Window) Raise() {
	c := win.c
	oldLayer, oldSurf, oldBlur := win.layer, win.surface, win.blurSurf
	win.retire = func() {
		if oldBlur != 0 {
			c.send(oldBlur, 0) // ext_background_effect_surface_v1.destroy
		}
		c.send(oldLayer, 7) // zwlr_layer_surface_v1.destroy
		c.send(oldSurf, 0)  // wl_surface.destroy
	}
	win.blurSurf, win.lastBlur = 0, nil
	win.configured, win.framePending, win.lastShape = false, false, 0
	win.lastInput = image.Rect(-1, -1, -1, -1) // the new surface needs its input region
	for _, b := range win.bufs {
		b.stale = append(b.stale[:0], image.Rect(0, 0, win.w, win.h))
	}
	win.createSurface()
	c.flush()
}

func (win *Window) onSeat(op uint16, d *dec) {
	if op != 0 { // capabilities
		return
	}
	caps := d.u32()
	if caps&1 != 0 && win.pointer == 0 {
		win.pointer = win.c.newID(win.onPointerEv)
		win.c.send(win.seat, 0, win.pointer)
		if win.cursorMgr != 0 {
			win.cursorDev = win.c.newID(nil)
			win.c.send(win.cursorMgr, 1, win.cursorDev, win.pointer)
		}
	}
}

func (win *Window) onLayer(op uint16, d *dec) {
	switch op {
	case 0: // configure
		serial := d.u32()
		win.c.send(win.layer, 6, serial) // ack_configure
		win.configured = true
	case 1: // closed
		win.closed = true
	}
}

func (win *Window) onPointerEv(op uint16, d *dec) {
	switch op {
	case 0: // enter
		win.enterSerial = d.u32()
		d.u32()
		win.px, win.py = d.fixed(), d.fixed()
		win.lastShape = 0
		win.emit(PointerEvent{Kind: "enter", X: win.px, Y: win.py})
	case 1: // leave
		win.emit(PointerEvent{Kind: "leave"})
	case 2: // motion
		d.u32()
		win.px, win.py = d.fixed(), d.fixed()
		win.emit(PointerEvent{Kind: "motion", X: win.px, Y: win.py})
	case 3: // button
		d.u32()
		d.u32()
		btn, state := d.u32(), d.u32()
		b := map[uint32]int{0x110: 1, 0x112: 2, 0x111: 3}[btn]
		switch {
		case b == 0:
		case state == 1:
			win.emit(PointerEvent{Kind: "press", X: win.px, Y: win.py, Button: b})
		default: // the compositor grabs the pointer for us between press and release
			win.emit(PointerEvent{Kind: "release", X: win.px, Y: win.py, Button: b})
		}
	case 4: // axis: vertical wheel / touchpad, ~15 units per notch
		d.u32()
		if axis := d.u32(); axis == 0 {
			win.scroll += d.fixed()
		}
	case 5: // frame: deliver the accumulated scroll as whole notches
		if steps := math.Trunc(win.scroll / 15); steps != 0 {
			win.scroll -= steps * 15
			win.emit(PointerEvent{Kind: "scroll", X: win.px, Y: win.py, Steps: steps})
		}
	}
}

func (win *Window) emit(e PointerEvent) {
	if win.OnPointer != nil {
		win.OnPointer(e)
	}
}

// SetCursor shows a hand over buttons, the arrow elsewhere.
func (win *Window) SetCursor(hand bool) {
	if win.cursorDev == 0 {
		return
	}
	shape := uint32(shapeDefault)
	if hand {
		shape = shapePointer
	}
	if shape == win.lastShape {
		return
	}
	win.lastShape = shape
	win.c.send(win.cursorDev, 1, win.enterSerial, shape)
}

// setInput limits clicks to the visible shape; everything else passes through.
func (win *Window) setInput(r image.Rectangle) {
	reg := win.c.newID(nil)
	win.c.send(win.compositor, 1, reg)
	if !r.Empty() {
		win.c.send(reg, 1, int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
	}
	win.c.send(win.surface, 5, reg)
	win.c.send(reg, 0)
	win.lastInput = r
}

// Present copies img: what changed (the damage rectangles, plus whatever
// this buffer missed while the others were on screen) into a free buffer as
// ARGB8888, tells the compositor only about that, and commits. Separate
// rectangles stay separate (Ember at one end, a meter at the other), so the
// gap between them is neither copied nor uploaded again.
func (win *Window) Present(img *image.RGBA, input image.Rectangle, damage []image.Rectangle, wantFrame bool) bool {
	if !win.configured {
		return false
	}
	var b *shmBuffer
	for _, cand := range win.bufs {
		if !cand.busy {
			b = cand
			break
		}
	}
	if b == nil {
		return false
	}
	win.dmg = clipRects(append(win.dmg[:0], damage...), img.Rect)
	win.region = MergeRects(append(append(win.region[:0], win.dmg...), b.stale...))
	for _, r := range win.region {
		r = r.Intersect(img.Rect)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			o := y*img.Stride + r.Min.X*4
			swizzle(b.data[o:o+r.Dx()*4], img.Pix[o:o+r.Dx()*4])
		}
	}
	b.stale = b.stale[:0]
	for _, o := range win.bufs { // the others missed this frame's pixels
		if o != b && len(win.dmg) > 0 {
			o.stale = MergeRects(append(o.stale, win.dmg...))
		}
	}
	if input != win.lastInput {
		win.setInput(input)
	}
	win.c.send(win.surface, 1, b.id, int32(0), int32(0)) // attach
	for _, r := range MergeRects(win.dmg) {
		win.c.send(win.surface, 9, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
	}
	if wantFrame && !win.framePending {
		win.framePending = true
		cb := win.c.newID(func(op uint16, d *dec) {
			win.framePending = false
			if win.OnFrame != nil {
				win.OnFrame()
			}
		})
		win.c.send(win.surface, 3, cb)
	}
	win.c.send(win.surface, 6) // commit
	b.busy = true
	if win.retire != nil { // a raise: the new surface is up, the old one can go
		win.retire()
		win.retire = nil
	}
	if err := win.c.flush(); err != nil {
		log.Println("wayland: flush:", err)
	}
	return true
}

// SetBlur asks the compositor to blur what's behind these rectangles
// (ext-background-effect-v1); the same rectangles again cost nothing. The
// pill's rounded corners are approximated by a few strips; the shape drawn
// on top hides the steps.
func (win *Window) SetBlur(rects []image.Rectangle) {
	if win.blurMgr == 0 || slices.Equal(rects, win.lastBlur) {
		return
	}
	if win.blurSurf == 0 {
		win.blurSurf = win.c.newID(nil)
		win.c.send(win.blurMgr, 1, win.blurSurf, win.surface)
	}
	reg := win.c.newID(nil)
	win.c.send(win.compositor, 1, reg)
	for _, r := range rects {
		win.c.send(reg, 1, int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
	}
	win.c.send(win.blurSurf, 1, reg)
	win.c.send(reg, 0)
	win.lastBlur = rects
}

// Caps says which optional protocols the compositor has (none on a nil
// Window).
func (win *Window) Caps() Caps {
	if win == nil {
		return Caps{}
	}
	return Caps{Blur: win.blurMgr != 0, CursorShape: win.cursorMgr != 0, Idle: win.idleMgr != 0}
}

// Closed: the compositor took the surface away.
func (win *Window) Closed() bool { return win.closed }

// Messages are the compositor's events, for Dispatch.
func (win *Window) Messages() <-chan Message { return win.c.msgs }

// Errors says the connection broke.
func (win *Window) Errors() <-chan error { return win.c.readErr }

// Dispatch handles one message (and runs the callbacks it causes).
func (win *Window) Dispatch(m Message) { win.c.dispatch(m) }

// Flush sends what the window has queued.
func (win *Window) Flush() error { return win.c.flush() }

// swizzle copies RGBA pixels to ARGB8888 (B, G, R, A in memory), a whole
// pixel at a time: swap the first and third byte of each 32-bit word.
func swizzle(dst, src []byte) {
	n := len(src) / 4
	if n == 0 {
		return
	}
	s := unsafe.Slice((*uint32)(unsafe.Pointer(&src[0])), n)
	d := unsafe.Slice((*uint32)(unsafe.Pointer(&dst[0])), n)
	for i, v := range s {
		d[i] = v&0xFF00FF00 | v>>16&0xFF | v&0xFF<<16
	}
}

// clipRects keeps the parts of rs inside bounds, dropping the empty ones.
func clipRects(rs []image.Rectangle, bounds image.Rectangle) []image.Rectangle {
	out := rs[:0]
	for _, r := range rs {
		if r = r.Intersect(bounds); !r.Empty() {
			out = append(out, r)
		}
	}
	return out
}

// MergeRects folds rectangles that overlap, or whose union costs little more
// than the two apart, into one; past maxRects it keeps their union. It works
// in place: the result reuses rs.
func MergeRects(rs []image.Rectangle) []image.Rectangle {
	const maxRects = 8
	area := func(r image.Rectangle) int { return r.Dx() * r.Dy() }
	rs = clipRects(rs, image.Rect(-1<<20, -1<<20, 1<<20, 1<<20))
	for merged := true; merged && len(rs) > 1; {
		merged = false
		for i := 0; i < len(rs) && !merged; i++ {
			for j := i + 1; j < len(rs); j++ {
				u := rs[i].Union(rs[j])
				if area(u) <= area(rs[i])+area(rs[j])+1024 || rs[i].Overlaps(rs[j]) {
					rs[i] = u
					rs = append(rs[:j], rs[j+1:]...)
					merged = true
					break
				}
			}
		}
	}
	if len(rs) > maxRects {
		u := rs[0]
		for _, r := range rs[1:] {
			u = u.Union(r)
		}
		rs = append(rs[:0], u)
	}
	return rs
}
