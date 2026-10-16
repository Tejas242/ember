import { animate, stagger } from "motion"
import "./flame"

// The island's views and how it moves between them. Width leads without
// overshoot and the drop overshoots a little, the old view leaves in 90 ms
// drifting up, the new one's parts arrive one after another: the same
// choreography as Ember's renderer (docs/DESIGN.md, Motion).
//
// The page decides what it wants shown (state), what time it is (time), and
// where the session is (session). The island adds its own life on top: hover
// drops a card, it falls asleep when nobody moves, and it tells you what
// arrived while you were on another tab.

export type View =
  | "clock" | "missed" | "work" | "wait" | "done" | "away"
  | "music" | "volume" | "bright" | "phones" | "shot" | "charge"
  | "card" | "day" | "request"

const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches
const BAR = 30
const DROWSY = 40_000
const ASLEEP = 65_000

const steps: { kind: string; text: string; failed?: boolean; diff?: [number, number] }[] = [
  { kind: "read", text: "Read compact.go" },
  { kind: "search", text: "Searched for segment.Lock" },
  { kind: "edit", text: "Edited compact.go", diff: [6, 2] },
  { kind: "run", text: "Run the store tests", failed: true },
  { kind: "edit", text: "Edited segment.go", diff: [17, 4] },
  { kind: "run", text: "Run the whole suite under the race detector" },
]

const icons: Record<string, string> = {
  read: '<path d="M4.5 1.8h7v12.4h-7z"/><path d="M6.5 6h3M6.5 8.5h3"/>',
  search: '<circle cx="7" cy="7" r="4.2"/><path d="M10.2 10.2 13.6 13.6"/>',
  edit: '<path d="M3.5 12.5 11.5 4.5"/><path d="M2.6 13.4l.7-2.4 1.7 1.7z"/>',
  run: '<path d="M3 4.5 7 8l-4 3.5M8.5 12H13"/>',
}

// what hovering a pill opens
const opens: Partial<Record<View, View>> = { clock: "day", missed: "card", work: "card", done: "card", wait: "request" }
const live = new Set<View>(["clock", "missed", "work"])

export class EmberIsland extends HTMLElement {
  #isl!: HTMLElement
  #views = new Map<View, HTMLElement>()
  #state: View = "clock"
  #want: View = "clock"
  #hover = false
  #flash = 0
  #life = 0
  #step = 0
  #idleAt = performance.now()
  #sleep: "" | "drowsy" | "asleep" = ""
  #hiddenAt = 0
  #minutes = 9 * 60 + 4
  #night = false
  zoomed = false

  connectedCallback() {
    this.#isl = this.querySelector(".isl")!
    for (const v of this.querySelectorAll<HTMLElement>(".view")) this.#views.set(v.dataset.view as View, v)
    this.#show("clock", true)

    // hovering the pill drops its card, as on the desktop
    this.#isl.addEventListener("pointerenter", (e) => {
      if (e.pointerType === "touch") return
      this.#hover = true
      this.#open()
    })
    this.#isl.addEventListener("pointerleave", (e) => {
      if (e.pointerType === "touch") return
      this.#hover = false
      this.#settle()
    })
    // a tap opens what the pill stands for, and a tap outside folds it back
    this.#isl.addEventListener("click", (e) => {
      if ((e.target as HTMLElement).closest("button")) return
      this.#open()
    })
    document.addEventListener("click", (e) => {
      if (!this.contains(e.target as Node)) this.#settle()
    })
    this.querySelectorAll<HTMLButtonElement>("[data-act]").forEach((b) =>
      b.addEventListener("click", () => this.#answer(b.dataset.act!)),
    )

    // it grows drowsy, then sleeps, when nobody moves; anything wakes it
    const stir = () => {
      this.#idleAt = performance.now()
      if (this.#sleep) this.#wake()
    }
    for (const ev of ["pointermove", "pointerdown", "keydown", "scroll", "wheel", "touchstart"]) addEventListener(ev, stir, { passive: true })
    setInterval(() => this.#doze(), 1000)

    // presence: what arrived while you were on another tab, once you're back
    document.addEventListener("visibilitychange", () => {
      if (document.hidden) this.#hiddenAt = performance.now()
      else if (this.#hiddenAt && performance.now() - this.#hiddenAt > 3500) {
        this.flash("away", 4200)
        this.dispatchEvent(new CustomEvent("presence"))
      }
    })
  }

  /** What the page wants the island to show now. */
  set state(v: View) {
    if (v === this.#want) return
    this.#want = v
    if (!this.#hover && !this.#flash) this.#show(v)
  }
  get state() {
    return this.#want
  }

  /** The time of day, in minutes since midnight. */
  set time(m: number) {
    m = Math.round(m)
    if (m === this.#minutes) return
    this.#minutes = m
    const h = Math.floor(m / 60) % 24
    const label = `${h % 12 || 12}:${String(m % 60).padStart(2, "0")} ${h < 12 ? "AM" : "PM"}`
    this.querySelectorAll("[data-clock]").forEach((e) => (e.textContent = label))
    const greet = this.querySelector("[data-greeting]")
    if (greet) greet.textContent = h >= 5 && h < 12 ? "Good morning" : h >= 12 && h < 17 ? "Good afternoon" : h >= 17 && h < 22 ? "Good evening" : "Good night"
    const done = m >= 11 * 60 + 52 || m < 5 * 60
    const line = this.querySelector("[data-dayline]")
    if (line) line.textContent = done ? "1 task done · 12m with Claude · mostly kinvault" : "Nothing running yet · 3 tasks yesterday"
    this.querySelectorAll<HTMLElement>(".hours i").forEach((i) => {
      const hh = Number(i.dataset.h)
      i.classList.toggle("past", (m >= 24 * 60 ? 24 : h) > hh || m >= 24 * 60)
      i.classList.toggle("now", hh === h)
    })
    // a pill showing the clock follows its width
    if (this.#state === "clock" || this.#state === "missed") this.#reshape()
  }

  /** Where the story's session is: none yet, running, or finished. */
  set session(s: "none" | "live" | "done") {
    this.dataset.session = s
  }

  /** Night: it sleeps whatever you do, until the page says it's morning. */
  set night(v: boolean) {
    if (v === this.#night) return
    this.#night = v
    if (v) this.#asleep()
    else this.#wake()
  }

  /** Show a view for a while, then go back to what the page wants. */
  flash(v: View, ms: number) {
    clearTimeout(this.#flash)
    this.#show(v)
    this.#flash = window.setTimeout(() => {
      this.#flash = 0
      this.#settle()
    }, ms)
  }

  /** How long nobody has moved, and how long until it dozes off. */
  get idle() {
    const t = performance.now() - this.#idleAt
    return { for: t, drowsyIn: Math.max(0, DROWSY - t), asleepIn: Math.max(0, ASLEEP - t), sleep: this.#sleep }
  }

  #open() {
    if (this.zoomed) return
    const to = opens[this.#state]
    if (to) this.#show(to)
  }

  #settle() {
    if (this.#hover || this.#flash) return
    if (this.#state !== this.#want) this.#show(this.#want)
  }

  #doze() {
    const t = performance.now() - this.#idleAt
    if (document.hidden || !live.has(this.#state)) return
    if (t > ASLEEP && this.#sleep !== "asleep") this.#asleep()
    else if (t > DROWSY && !this.#sleep) {
      this.#sleep = "drowsy"
      this.dataset.sleep = "drowsy"
      this.querySelectorAll<HTMLElement>(".lead").forEach((f) => (f.dataset.drowsy = ""))
    }
  }

  #asleep() {
    this.#sleep = "asleep"
    this.dataset.sleep = "asleep"
    this.querySelectorAll<HTMLElement>("[data-view=clock] ember-flame, [data-view=missed] ember-flame").forEach((f) => f.setAttribute("mood", "sleep"))
    this.#say("Ember fell asleep")
  }

  #wake() {
    if (this.#night) return
    const was = this.#sleep
    this.#sleep = ""
    delete this.dataset.sleep
    this.querySelectorAll<HTMLElement>(".lead").forEach((f) => delete f.dataset.drowsy)
    this.querySelectorAll<HTMLElement & { wake(): void }>("[data-view=clock] ember-flame, [data-view=missed] ember-flame").forEach((f) => {
      f.setAttribute("mood", "idle")
      if (was === "asleep") f.wake()
    })
  }

  #say(s: string) {
    const el = this.querySelector("[data-say]")
    if (el) el.textContent = s
  }

  #size(v: HTMLElement): [number, number] {
    const pill = v.classList.contains("pill")
    const k = this.#scale() // the page may be showing it magnified
    const max = innerWidth / k - 24
    let w: number
    if (v.dataset.w) w = Number(v.dataset.w)
    else {
      v.style.width = "max-content" // a pill is as wide as what it says
      w = Math.ceil(v.getBoundingClientRect().width / k)
    }
    w = Math.min(w, Math.floor(max))
    v.style.width = `${w}px`
    return [w, pill ? BAR : v.scrollHeight]
  }

  #scale() {
    const t = getComputedStyle(this).transform
    if (!t || t === "none") return 1
    return new DOMMatrix(t).a || 1
  }

  #shape(w: number, h: number, kind: "open" | "fold" | "ask" | "instant") {
    const radius = Math.min(h / 2, h > BAR + 4 ? 22 : 15)
    const box = { borderBottomLeftRadius: `${radius}px`, borderBottomRightRadius: `${radius}px` }
    if (kind === "instant" || reduced) {
      Object.assign(this.#isl.style, { width: `${w}px`, height: `${h}px`, ...box })
      return
    }
    const open = kind !== "fold"
    animate(this.#isl, { width: `${w}px` }, { type: "spring", stiffness: 420, damping: open ? 39 : 34 })
    animate(
      this.#isl,
      { height: `${h}px`, ...box },
      open ? { type: "spring", stiffness: kind === "ask" ? 300 : 320, damping: kind === "ask" ? 23 : 27 } : { type: "spring", stiffness: 600, damping: 49 },
    )
  }

  /** Follow the view's own size when what it says changes in place. */
  #reshape() {
    const v = this.#views.get(this.#state)!
    const [w, h] = this.#size(v)
    const dh = h - this.#isl.offsetHeight
    if (Math.abs(this.#isl.offsetWidth - w) > 1 || Math.abs(dh) > 1) this.#shape(w, h, dh < -1 ? "fold" : "open")
  }

  #show(name: View, instant = false) {
    if (name === this.#state && !instant) return
    const from = this.#views.get(this.#state)!
    const to = this.#views.get(name)!
    this.#state = name
    this.dataset.state = name
    if (this.#sleep && !live.has(name) && !this.#night) this.#wake()
    this.#enter(name) // first, so the card's steps are there to be measured
    const [w, h] = this.#size(to)
    const grow = h > this.#isl.offsetHeight + 1
    this.#shape(w, h, instant ? "instant" : !grow && h < this.#isl.offsetHeight - 1 ? "fold" : name === "request" ? "ask" : "open")
    if (from !== to) {
      from.classList.remove("on")
      if (!reduced && !instant) animate(from, { opacity: 0, y: -3 }, { duration: 0.09 })
      else from.style.opacity = "0"
    }
    to.classList.add("on")
    const parts = to.querySelectorAll(":scope > *:not(.sweep, .flash, .countdown, .zz), .steps > li:not(.thread)")
    if (reduced || instant) {
      to.style.opacity = "1"
      to.style.transform = "none"
    } else {
      // the words wait for the shape to be mostly open, then arrive in turn
      const wait = grow ? 0.09 : 0.03
      animate(to, { opacity: 1, y: 0 }, { duration: 0.2, delay: wait })
      animate(parts, { opacity: [0, 1], y: [4, 0] }, { duration: 0.2, delay: stagger(0.024, { startDelay: wait + 0.02 }) })
    }
    this.dispatchEvent(new CustomEvent("view", { detail: name }))
  }

  /** Each view's own life once it's on screen. */
  #enter(name: View) {
    clearInterval(this.#life)
    if (name === "card") {
      const list = this.querySelector<HTMLElement>("[data-steps]")!
      if (list.children.length < 2) for (let i = 0; i < 3; i++) this.#addStep(list, false)
      if (this.dataset.session !== "done") this.#life = window.setInterval(() => this.#addStep(list, true), 1900)
    }
    if (name === "request") {
      const c = this.querySelector<HTMLElement>(".countdown")!
      c.getAnimations().forEach((a) => a.cancel())
      c.animate(
        [{ transform: "scaleX(1)", background: "rgb(222 229 216 / .16)" }, { transform: "scaleX(.18)", background: "rgb(222 229 216 / .16)", offset: 0.82 }, { transform: "scaleX(0)", background: "#fdc874", height: "2px" }],
        { duration: 85_000, fill: "forwards" },
      )
    }
    if (name === "done" && !reduced) {
      const ch = this.querySelector<SVGPathElement>("[data-view=done] .check path")!
      ch.animate([{ strokeDashoffset: 18 }, { strokeDashoffset: 0 }], { duration: 320, delay: 175, easing: "cubic-bezier(.22,1,.36,1)", fill: "backwards" })
      const sw = this.querySelector<HTMLElement>("[data-view=done] .sweep")!
      sw.animate([{ transform: "translateX(-60px)", opacity: 0 }, { opacity: 1, offset: 0.4 }, { transform: "translateX(380px)", opacity: 0 }], { duration: 700, delay: 180, easing: "cubic-bezier(.65,0,.35,1)", fill: "backwards" })
      this.querySelector("[data-view=done] ember-flame")?.animate([{ transform: "translateY(0)" }, { transform: "translateY(-3px)" }, { transform: "translateY(0)" }, { transform: "translateY(-1.5px)" }, { transform: "translateY(0)" }], { duration: 650 })
    }
  }

  #addStep(list: HTMLElement, live: boolean) {
    const s = steps[this.#step++ % steps.length]
    const li = document.createElement("li")
    li.dataset.kind = s.kind
    const right = s.failed ? '<span class="fail">failed</span>' : '<span class="when">now</span>'
    const diff = s.diff ? `<span class="diff"><b class="g">+${s.diff[0]}</b> <b class="r">−${s.diff[1]}</b></span>` : "<span></span>"
    li.innerHTML = `<svg viewBox="0 0 16 16" aria-hidden="true">${icons[s.kind]}</svg><span class="txt">${s.text}</span>${diff}${right}`
    if (s.failed) li.classList.add("failed")
    // the new step is the live one; the ones before it are done
    list.querySelector("li.now")?.classList.remove("now")
    list.querySelectorAll<HTMLElement>("li .when").forEach((w, i) => (w.textContent = `${i + 1}m`))
    li.classList.add("now")
    list.insertBefore(li, list.children[1] ?? null)
    while (list.children.length > 5) list.lastElementChild!.remove()
    if (!live || reduced) return
    const bead = list.querySelector<HTMLElement>(".bead")!
    animate(li, { opacity: [0, 1], y: [-6, 0] }, { duration: 0.24, delay: 0.11 })
    animate(bead, { y: [-18, 12], opacity: [1, 0] }, { duration: 0.42, ease: [0.65, 0, 0.35, 1] })
    if (this.#state === "card") this.#reshape()
    // a step that fails makes Ember flinch
    if (s.failed) this.querySelector("[data-view=card] ember-flame")?.animate([{ transform: "scale(1)" }, { transform: "scale(.9, .94) translateY(1px)" }, { transform: "scale(1)" }], { duration: 360, delay: 120 })
  }

  #answer(act: string) {
    const flame = this.querySelector<HTMLElement>("[data-view=request] ember-flame")!
    const then = (said: string) => {
      this.dispatchEvent(new CustomEvent("answer", { detail: act }))
      this.#say(said)
      // Claude carries on: the pill goes back to the work for a while
      setTimeout(() => this.flash("work", 2600), 440)
    }
    if (act === "deny") {
      if (!reduced) animate(flame, { x: [0, 2.4, -2.4, 1.6, -1, 0] }, { duration: 0.46 })
      return then("Denied. Claude tries another way.")
    }
    if (act === "terminal") {
      this.dispatchEvent(new CustomEvent("answer", { detail: act }))
      return this.flash("wait", 2600)
    }
    if (!reduced) animate(flame, { y: [0, 2.2, 0, 1, 0] }, { duration: 0.46 })
    then(act === "always" ? "Always allowed go test in kinvault." : "Allowed. Claude carries on.")
  }
}

customElements.define("ember-island", EmberIsland)
