package island

import (
	"context"
	"fmt"
	"image"
	"log"
	"math"
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
	f := &frames{}
	win.OnFrame = func() { f.due = true }
	log.Printf("ember up (bar %.0fpx)", a.ui.BarH)
	st := &counters{on: os.Getenv("EMBER_DEBUG") != "", since: time.Now()}
	draw := func() {
		var damage image.Rectangle
		f.animating, damage = a.ui.renderDamage()
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
	if !a.ui.mind.away {
		a.ui.bond.present(a.ui.now())
	}
	a.ui.habits.save()
	a.ui.bond.save()
	a.ui.day.save()
	if p := applyLook(a.ui.cfg, loadPalette()); p != a.ui.pal { // the wallpaper palette changed
		a.ui.pal = p
		a.ui.Invalidate()
	}
	if a.reloadConfig() {
		a.ui.Invalidate()
	}
	if a.ui.now().Sub(a.detectedAt) > redetectEvery { // a tool installed or removed meanwhile
		a.reconcile()
	}
}

// nextWake: animation runs on frame callbacks; otherwise sleep until the next
// blink, flicker tick, minute, hover delay or expiry.
func (a *app) nextWake(animating bool) time.Duration {
	u := a.ui
	now := u.now()
	if animating || u.dirty {
		return 16 * time.Millisecond // normally the frame callback comes first
	}
	next := u.nextBlink()
	if u.continuous() {
		next = now.Add(flickerEvery)
		if u.lastView != vCompact || u.eqShown || u.dancing || (u.body.phones.seated && u.danceable()) { // a dance needs every beat
			next = now.Add(time.Second / 24) // the card's pulses, a listening equaliser: only their squares repaint
		}
		if _, _, ok := u.stageNow(); ok { // a stage: as often as the song asks, less on battery
			next = now.Add(time.Second / time.Duration(u.stageFPS()))
		}
		if u.shimmering() { // a band of light crosses smoothly, then rests
			if t := now.Sub(u.start).Seconds(); shimmerPassing(t) {
				next = now.Add(time.Second / 40)
			} else {
				rest := time.Duration((shimmerEvery - math.Mod(t, shimmerEvery)) * float64(time.Second))
				next = minTime(next, now.Add(rest))
			}
		}
	} else if u.hiddenUnderFull() {
		next = now.Add(time.Minute) // folded away under a fullscreen window: a key, or the window, wakes it
	} else if u.mind.asleep {
		next = now.Add(time.Second / 4) // slow breaths, drifting z's
	} else if u.glancing() {
		next = now.Add(time.Second / 30) // eyes move smoothly; nothing else needs the rate
	} else {
		next = minTime(next, now.Add(idleEvery)) // Ember breathes even when nothing happens
	}

	next = minTime(next, now.Truncate(time.Minute).Add(time.Minute))
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

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
