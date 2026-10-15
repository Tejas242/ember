package island

import (
	"image"
	"log"
	"math"
	"time"
	"unsafe"
)

// layer-shell constants (wlr-layer-shell-unstable-v1)
const (
	layerTop     = 2
	anchorTop    = 1
	btnLeft      = 0x110
	shapeDefault = 1
	shapePointer = 4
)

type pointerEvent struct {
	kind   string // enter | motion | leave | press | release | scroll
	x, y   float64
	button int     // press: 1 left, 2 middle, 3 right
	steps  float64 // scroll: wheel notches, positive = down
}

// window is the island's one layer surface.
type window struct {
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
	onPointer                          func(pointerEvent)
	onFrame                            func()
	onIdle                             func(idle bool)
	layerKind                          uint32
	retire                             func() // the surface a raise replaced, to destroy once the new one shows
	marginTop                          int
	idleMgr, idleVer                   uint32
	idleNote                           uint32        // the idle notification, remade when away_after_s changes
	idleFor                            time.Duration // what it was asked for
	lastInput                          image.Rectangle
	lastShape                          uint32
}

func openWindow(w, h, marginTop int, layer uint32) (*window, error) {
	c, err := dial()
	if err != nil {
		return nil, err
	}
	win := &window{c: c, w: w, h: h}
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
	win.layerKind, win.marginTop = layer, marginTop
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

// watchIdle asks to hear when you've been away from the keyboard and mouse
// for d, and when you come back (ext-idle-notify-v1). Version 2's input
// idle ignores inhibitors, so a playing video doesn't count as you.
func (win *window) watchIdle(d time.Duration) {
	if win.idleMgr == 0 || win.seat == 0 || d == win.idleFor {
		return
	}
	if win.idleNote != 0 {
		win.c.send(win.idleNote, 0) // ext_idle_notification_v1.destroy
	}
	id := win.c.newID(func(op uint16, _ *dec) {
		if win.onIdle != nil {
			win.onIdle(op == 0) // 0 idled, 1 resumed
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
func (win *window) createSurface() {
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

// raise puts the island back on top of its layer. Layer-shell has no
// "raise": the newest surface of a layer is drawn above the older ones, so
// when the bar is re-created (the shell restarted, or a bar setting
// changed) it covers us. Making our surface again puts it above. The old
// one stays up until the new one has shown its first frame: no blink.
func (win *window) raise() {
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
		b.stale = image.Rect(0, 0, win.w, win.h)
	}
	win.createSurface()
	c.flush()
}

func (win *window) onSeat(op uint16, d *dec) {
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

func (win *window) onLayer(op uint16, d *dec) {
	switch op {
	case 0: // configure
		serial := d.u32()
		win.c.send(win.layer, 6, serial) // ack_configure
		win.configured = true
	case 1: // closed
		win.closed = true
	}
}

func (win *window) onPointerEv(op uint16, d *dec) {
	switch op {
	case 0: // enter
		win.enterSerial = d.u32()
		d.u32()
		win.px, win.py = d.fixed(), d.fixed()
		win.lastShape = 0
		win.emit(pointerEvent{kind: "enter", x: win.px, y: win.py})
	case 1: // leave
		win.emit(pointerEvent{kind: "leave"})
	case 2: // motion
		d.u32()
		win.px, win.py = d.fixed(), d.fixed()
		win.emit(pointerEvent{kind: "motion", x: win.px, y: win.py})
	case 3: // button
		d.u32()
		d.u32()
		btn, state := d.u32(), d.u32()
		b := map[uint32]int{0x110: 1, 0x112: 2, 0x111: 3}[btn]
		switch {
		case b == 0:
		case state == 1:
			win.emit(pointerEvent{kind: "press", x: win.px, y: win.py, button: b})
		default: // the compositor grabs the pointer for us between press and release
			win.emit(pointerEvent{kind: "release", x: win.px, y: win.py, button: b})
		}
	case 4: // axis: vertical wheel / touchpad, ~15 units per notch
		d.u32()
		if axis := d.u32(); axis == 0 {
			win.scroll += d.fixed()
		}
	case 5: // frame: deliver the accumulated scroll as whole notches
		if steps := math.Trunc(win.scroll / 15); steps != 0 {
			win.scroll -= steps * 15
			win.emit(pointerEvent{kind: "scroll", x: win.px, y: win.py, steps: steps})
		}
	}
}

func (win *window) emit(e pointerEvent) {
	if win.onPointer != nil {
		win.onPointer(e)
	}
}

// setCursor shows a hand over buttons, the arrow elsewhere.
func (win *window) setCursor(hand bool) {
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
func (win *window) setInput(r image.Rectangle) {
	reg := win.c.newID(nil)
	win.c.send(win.compositor, 1, reg)
	if !r.Empty() {
		win.c.send(reg, 1, int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
	}
	win.c.send(win.surface, 5, reg)
	win.c.send(reg, 0)
	win.lastInput = r
}

// present copies what changed (damage, plus whatever this buffer missed
// while the other one was on screen) into a free buffer as ARGB8888, tells
// the compositor only about that, and commits.
func (win *window) present(img *image.RGBA, input, damage image.Rectangle, wantFrame bool) bool {
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
	damage = damage.Intersect(img.Rect)
	region := damage.Union(b.stale).Intersect(img.Rect)
	for y := region.Min.Y; y < region.Max.Y; y++ {
		o := y*img.Stride + region.Min.X*4
		swizzle(b.data[o:o+region.Dx()*4], img.Pix[o:o+region.Dx()*4])
	}
	b.stale = image.Rectangle{}
	for _, o := range win.bufs { // the others missed this frame's pixels
		if o != b {
			o.stale = o.stale.Union(damage)
		}
	}
	if input != win.lastInput {
		win.setInput(input)
	}
	win.c.send(win.surface, 1, b.id, int32(0), int32(0)) // attach
	if !damage.Empty() {
		win.c.send(win.surface, 9, damage.Min.X, damage.Min.Y, damage.Dx(), damage.Dy())
	}
	if wantFrame && !win.framePending {
		win.framePending = true
		cb := win.c.newID(func(op uint16, d *dec) {
			win.framePending = false
			if win.onFrame != nil {
				win.onFrame()
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
		log.Println("island: flush:", err)
	}
	return true
}

// setBlur asks the compositor to blur what's behind these rectangles
// (ext-background-effect-v1). The pill's rounded corners are approximated
// by a few strips; the shape drawn on top hides the steps.
func (win *window) setBlur(rects []image.Rectangle) {
	if win.blurMgr == 0 {
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
