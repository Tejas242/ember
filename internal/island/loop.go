package island

import (
	"context"
	"fmt"
	"image"
	"log"
	"os"
	"time"

	"github.com/Tejas242/ember/internal/wayland"
)

// The island's loop: one goroutine owns the app and the UI. It wakes for
// the compositor (input, frame callbacks), for posts from every other
// goroutine (hooks, sources, plugins), for housekeeping, and for the next
// thing that changes on screen; it draws only when something did.

// frames is where drawing stands: a frame callback due, the last frame
// still animating, a frame sent and not yet answered, damage carried over.
type frames struct {
	due, animating, waiting, tick bool
	waitingSince                  time.Time
	lost                          image.Rectangle // damage of frames the compositor had no buffer for: the next frame carries it
}

// counters are EMBER_DEBUG's numbers, logged every 5 s.
type counters struct {
	on                            bool
	full, fast, fail, post, wakes int
	since                         time.Time
}

func (c *counters) report(a *app) {
	if !c.on || time.Since(c.since) <= 5*time.Second {
		return
	}
	log.Printf("5s: wakes %d, full %d, fast %d, failed %d, posts %d, view %v, cont %v", c.wakes, c.full, c.fast, c.fail, c.post, a.ui.lastView, a.ui.continuous())
	*c = counters{on: c.on, since: time.Now()}
}

// housekeepEvery: sessions swept, state saved, the palette and the config
// looked at again.
const housekeepEvery = 10 * time.Second

func (a *app) loop(ctx context.Context) error {
	win := a.win
	a.ui.bud.on = true
	f := &frames{}
	win.OnFrame = func() { f.due = true }
	log.Printf("ember up (bar %.0fpx)", a.ui.BarH)
	st := &counters{on: os.Getenv("EMBER_DEBUG") != "", since: time.Now()}
	draw := func() {
		var damage image.Rectangle
		f.animating, damage = a.ui.renderDamage()
		if a.ui.bud.skipped { // the flame moved less than half a pixel: nothing to send
			return
		}
		if a.ui.wantOverlay() { // a key's OSD over a fullscreen window: up above it, until it has folded away
			win.SetLayer(wayland.LayerOverlay)
		} else {
			win.SetLayer(wayland.LayerTop)
		}
		if a.ui.lastFull {
			st.full++
		} else {
			st.fast++
		}
		rects := a.ui.dmg
		if !f.lost.Empty() {
			rects = append(rects, f.lost)
		}
		damage = damage.Union(f.lost)
		if win.Present(a.ui.cv.img, a.ui.shape.bounds().Union(a.ui.bubble), rects, f.animating) {
			f.waiting, f.waitingSince = f.animating, time.Now()
			f.lost = image.Rectangle{}
		} else {
			st.fail++
			f.lost = damage   // what changed never reached the screen: keep it for the next frame
			a.ui.dirty = true // no free buffer: try again on the next wake
		}
		win.SetBlur(a.ui.shape.blurRects())
	}
	draw()
	housekeep := time.NewTicker(housekeepEvery)
	defer housekeep.Stop()
	timer := time.NewTimer(time.Hour)
	lastPos := time.Time{}
	saveDue := false
	for {
		select {
		case <-ctx.Done():
			log.Println("ember: stopping")
			return nil
		case m := <-win.Messages():
			win.Dispatch(m)
		case err := <-win.Errors():
			return err
		case p := <-a.posts:
			a.quiet = false
			p(a)
			if !a.quiet { // a post that changed nothing visible says so
				a.ui.Invalidate()
			}
			st.post++
			saveDue = true
		case <-housekeep.C:
			a.housekeep(&saveDue)
		case <-timer.C:
			f.tick = true
			if a.ui.expire() {
				a.ui.Invalidate()
			}
		}
		if win.Closed() {
			return fmt.Errorf("layer surface closed")
		}
		// the open card shows a progress bar: re-read the playhead now and then
		if (a.ui.lastView == vExpanded || a.ui.followsPlayhead()) && a.ui.track != nil && a.ui.track.Playing && time.Since(lastPos) > 5*time.Second {
			lastPos = time.Now()
			a.music.ReadPosition(a.ui.track.Player)
		}
		if f.waiting && time.Since(f.waitingSince) > frameGrace {
			f.waiting = false // niri skipped the callback (nothing on screen changed)
		}
		switch {
		case a.ui.gated(): // away, or under a full-screen window: nothing to see, nothing drawn; what happened is drawn when it ends
			f.due, f.waiting = false, false
		case f.due:
			f.due, f.waiting = false, false
			if f.animating || a.ui.dirty {
				draw()
			}
		case (a.ui.dirty || f.tick || f.animating) && !f.waiting:
			draw()
		}
		f.tick = false
		a.extDashChanged()
		a.extMedia()
		st.wakes++
		st.report(a)
		timer.Reset(a.nextWake(f.animating))
		win.Flush()
	}
}

// housekeep: what the loop does every housekeepEvery.
func (a *app) housekeep(saveDue *bool) {
	if a.sweep() {
		a.ui.Invalidate()
	}
	if *saveDue {
		a.saveSessions()
		*saveDue = false
	}
	if gap := a.ui.now().Sub(a.lastHouse); !a.lastHouse.IsZero() && gap > sleptGap { // it slept (a lid closed): you're back
		a.ui.returned(a.lastHouse, a.ui.now())
	}
	a.lastHouse = a.ui.now()
	a.syncAudio() // the battery may have fallen under a fifth
	if !a.ui.mind.away {
		a.ui.bond.present(a.ui.now())
	}
	a.ui.logCost()
	a.ui.habits.save()
	a.ui.bond.save()
	if p := applyLook(a.ui.cfg, loadPalette()); p != a.ui.pal { // the wallpaper palette changed
		a.ui.pal = p
		a.ui.Invalidate()
	}
	if a.reloadConfig() {
		a.ui.Invalidate()
	}
	if a.readLimits() {
		a.ui.Invalidate()
	}
	if a.ui.now().Sub(a.detectedAt) > redetectEvery { // a tool installed or removed meanwhile
		a.reconcile()
	}
}

// nextWake: animation runs on frame callbacks; otherwise sleep until the next
// blink, glance, tick of the state's budget (budget.go), minute, hover delay
// or expiry.
func (a *app) nextWake(animating bool) time.Duration {
	u := a.ui
	now := u.now()
	var next time.Time
	switch {
	case u.gated():
		next = now.Add(time.Minute) // a post, a key or the end of the gate wakes the loop sooner
	case animating || u.dirty:
		return 16 * time.Millisecond // normally the frame callback comes first
	default:
		next = u.nextTick(now)
		if u.bud.burst && u.baseTick() > time.Second/12 { // at 12 a second or faster, blinks ride the base tick
			next = minTime(next, now.Add(u.burstEvery())) // a blink, a glance, a swing
		}
		next = minTime(next, now.Truncate(time.Minute).Add(time.Minute))
	}
	for _, c := range u.cards {
		if !c.Expires.IsZero() {
			next = minTime(next, c.Expires)
		}
	}
	if u.hover && now.Sub(u.hoverAt) < u.cfg.HoverDelay {
		next = minTime(next, u.hoverAt.Add(u.cfg.HoverDelay))
	}
	if !u.hover && now.Sub(u.leftAt) < leaveGrace+100*time.Millisecond { // wake just after the grace, to fold the card
		next = minTime(next, u.leftAt.Add(leaveGrace+10*time.Millisecond))
	}
	return max(next.Sub(now), 5*time.Millisecond)
}

// nextTick: when this state next needs a frame if nothing else asks.
func (u *UI) nextTick(now time.Time) time.Time {
	m := u.mind
	next := now.Add(time.Hour)
	if u.nextBlink().After(now) { // (one under way is the burst's)
		next = u.nextBlink()
	}
	if m.glanceAt.After(now) {
		next = minTime(next, m.glanceAt) // it starts looking away on time
	}
	switch {
	case u.continuous():
		next = now.Add(u.baseTick())
		if _, _, ok := u.stageNow(); ok { // a stage: as often as the song asks, less on battery
			next = now.Add(time.Second / time.Duration(u.stageFPS()))
		}
	case m.asleep:
		next = minTime(next, now.Add(u.tick(2.5, 1.7, 0.7))) // slow breaths, drifting z's
	default:
		next = minTime(next, now.Add(u.tick(4, 3, 1.5))) // Ember breathes even when nothing happens
	}
	return next
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
