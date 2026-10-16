import { animate, stagger } from "motion"
import "./flame"

// The island's views and how it moves between them. Width leads without
// overshoot and the drop overshoots a little, the old view leaves in 90 ms
// drifting up, the new one's parts arrive one after another: the same
// choreography as Ember's renderer (docs/DESIGN.md, Motion).

type View = "clock" | "work" | "wait" | "card" | "request" | "done" | "music" | "volume"

// on a phone the island keeps to pills while you read; a tap opens the card
const narrow = matchMedia("(width < 700px)")
const compact: Partial<Record<View, View>> = { card: "work", request: "wait" }

const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches
const BAR = 30

const steps: { kind: string; text: string; meta?: string; diff?: [number, number] }[] = [
  { kind: "read", text: "Read compact.go" },
  { kind: "search", text: "Searched for segment.Lock" },
  { kind: "edit", text: "Edited compact.go", diff: [6, 2] },
  { kind: "run", text: "Run the store tests", meta: "failed" },
  { kind: "edit", text: "Edited segment.go", diff: [17, 4] },
  { kind: "run", text: "Run the whole suite under the race detector" },
  { kind: "edit", text: "Edited quota.go", diff: [3, 1] },
  { kind: "search", text: "Searched for rotate(" },
]

const icons: Record<string, string> = {
  read: '<path d="M4.5 1.8h7v12.4h-7z"/><path d="M6.5 6h3M6.5 8.5h3"/>',
  search: '<circle cx="7" cy="7" r="4.2"/><path d="M10.2 10.2 13.6 13.6"/>',
  edit: '<path d="M3.5 12.5 11.5 4.5"/><path d="M2.6 13.4l.7-2.4 1.7 1.7z"/>',
  run: '<path d="M3 4.5 7 8l-4 3.5M8.5 12H13"/>',
}

class EmberIsland extends HTMLElement {
  #isl!: HTMLElement
  #views = new Map<View, HTMLElement>()
  #state: View = "clock"
  #want: View = "clock"
  #hover = false
  #stepTimer = 0
  #step = 0

  connectedCallback() {
    this.#isl = this.querySelector(".isl")!
    for (const v of this.querySelectorAll<HTMLElement>(".view")) {
      this.#views.set(v.dataset.view as View, v)
    }
    this.#tickClock()
    setInterval(() => this.#tickClock(), 15_000)
    this.#show("clock", true)

    // hovering the pill drops the card, as on the desktop
    this.#isl.addEventListener("pointerenter", (e) => {
      if (e.pointerType === "touch") return
      this.#hover = true
      if (this.#state === "clock" || this.#state === "work") this.#show("card")
    })
    this.#isl.addEventListener("pointerleave", (e) => {
      if (e.pointerType === "touch") return
      this.#hover = false
      if (this.#state !== this.#fit(this.#want)) this.#show(this.#fit(this.#want))
    })
    // a tap opens what the pill stands for, and a tap outside folds it back
    this.#isl.addEventListener("click", (e) => {
      if ((e.target as HTMLElement).closest("button")) return
      const open: Partial<Record<View, View>> = { clock: "card", work: "card", wait: "request" }
      if (open[this.#state]) this.#show(open[this.#state]!)
    })
    document.addEventListener("click", (e) => {
      if (!this.contains(e.target as Node) && this.#state !== this.#fit(this.#want)) this.#show(this.#fit(this.#want))
    })
    this.querySelectorAll<HTMLButtonElement>("[data-act]").forEach((b) =>
      b.addEventListener("click", () => this.#answer(b.dataset.act!)),
    )
  }

  /** What the page wants the island to show now. */
  set state(v: View) {
    this.#want = v
    if (!this.#hover) this.#show(this.#fit(v))
  }

  #fit(v: View): View {
    return narrow.matches ? (compact[v] ?? v) : v
  }

  #tickClock() {
    const t = new Date().toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })
    this.querySelectorAll("[data-clock]").forEach((e) => (e.textContent = t))
  }

  #size(v: HTMLElement): [number, number] {
    const pill = v.classList.contains("pill")
    if (pill) v.style.width = "max-content" // a pill is as wide as what it says
    const w = Math.min(pill ? Math.ceil(v.getBoundingClientRect().width) : Number(v.dataset.w), innerWidth - 24)
    v.style.width = `${w}px`
    return [w, pill ? BAR : v.scrollHeight]
  }

  #show(name: View, instant = false) {
    if (name === this.#state && !instant) return
    const from = this.#views.get(this.#state)!
    const to = this.#views.get(name)!
    this.#state = name
    this.dataset.state = name
    const [w, h] = this.#size(to)
    const grow = h > this.#isl.offsetHeight + 1
    const radius = Math.min(h / 2, h > BAR + 4 ? 22 : 15)
    if (instant || reduced) {
      Object.assign(this.#isl.style, { width: `${w}px`, height: `${h}px`, borderBottomLeftRadius: `${radius}px`, borderBottomRightRadius: `${radius}px` })
    } else {
      animate(this.#isl, { width: `${w}px` }, { type: "spring", stiffness: 420, damping: grow ? 39 : 34 })
      animate(
        this.#isl,
        { height: `${h}px`, borderBottomLeftRadius: `${radius}px`, borderBottomRightRadius: `${radius}px` },
        grow ? { type: "spring", stiffness: name === "request" ? 300 : 320, damping: name === "request" ? 24 : 27 } : { type: "spring", stiffness: 600, damping: 49 },
      )
    }
    if (from !== to) {
      from.classList.remove("on")
      if (!reduced) animate(from, { opacity: 0, y: -3 }, { duration: 0.09 })
      else from.style.opacity = "0"
    }
    to.classList.add("on")
    const parts = to.querySelectorAll(":scope > *:not(.ear), .steps > li:not(.thread)")
    if (reduced) {
      to.style.opacity = "1"
    } else {
      animate(to, { opacity: 1, y: 0 }, { duration: 0.2, delay: grow ? 0.06 : 0.03 })
      animate(parts, { opacity: [0, 1], y: [4, 0] }, { duration: 0.2, delay: stagger(0.024, { startDelay: grow ? 0.08 : 0.03 }) })
    }
    this.#enter(name)
  }

  /** Each view's own life once it's on screen. */
  #enter(name: View) {
    clearInterval(this.#stepTimer)
    if (name === "card") {
      const list = this.querySelector<HTMLElement>("[data-steps]")!
      if (list.children.length < 2) for (let i = 0; i < 3; i++) this.#addStep(list, false)
      this.#stepTimer = window.setInterval(() => this.#addStep(list, true), 1900)
    }
    if (name === "request") {
      const c = this.querySelector<HTMLElement>(".countdown")!
      c.getAnimations().forEach((a) => a.cancel())
      c.animate([{ transform: "scaleX(1)" }, { transform: "scaleX(0)" }], { duration: 30_000, fill: "forwards" })
    }
    if (name === "done" && !reduced) {
      const ch = this.querySelector<SVGPathElement>(".check path")!
      ch.animate([{ strokeDashoffset: 18 }, { strokeDashoffset: 0 }], { duration: 320, delay: 175, easing: "cubic-bezier(.22,1,.36,1)", fill: "backwards" })
      const sw = this.querySelector<HTMLElement>(".sweep")!
      sw.animate([{ transform: "translateX(-60px)", opacity: 0 }, { opacity: 1, offset: 0.4 }, { transform: "translateX(380px)", opacity: 0 }], { duration: 700, delay: 180, easing: "cubic-bezier(.65,0,.35,1)", fill: "backwards" })
      this.querySelector("[mood=happy]")?.animate([{ transform: "translateY(0)" }, { transform: "translateY(-3px)" }, { transform: "translateY(0)" }, { transform: "translateY(-1.5px)" }, { transform: "translateY(0)" }], { duration: 650 })
    }
    if (name === "volume") {
      const lvl = this.querySelector<HTMLElement>("[data-level]")!
      const val = this.querySelector<HTMLElement>("[data-value]")!
      let n = 38
      const up = () => {
        n = Math.min(100, n + 9)
        animate(lvl, { scaleX: n / 100 }, { type: "spring", stiffness: 420, damping: 40 })
        val.textContent = String(n)
        this.dataset.level = n > 45 ? "2" : n > 0 ? "1" : "0"
        if (n === 100) {
          animate(lvl.parentElement!, { scaleX: [1, 1.06, 1] }, { duration: 0.32 })
        }
      }
      lvl.style.transform = `scaleX(${n / 100})`
      this.#stepTimer = window.setInterval(() => (n >= 100 ? (n = 38) : up()), 420)
    }
  }

  #addStep(list: HTMLElement, live: boolean) {
    const s = steps[this.#step++ % steps.length]
    const li = document.createElement("li")
    li.dataset.kind = s.kind
    const right = s.meta === "failed" ? '<span class="fail">failed</span>' : '<span class="when">0s</span>'
    const diff = s.diff ? `<span class="diff"><b class="g">+${s.diff[0]}</b> <b class="r">−${s.diff[1]}</b></span>` : ""
    li.innerHTML = `<svg viewBox="0 0 16 16" aria-hidden="true">${icons[s.kind]}</svg><span class="txt">${s.text}</span>${diff}${right}`
    if (s.meta === "failed") li.classList.add("failed")
    // the new step is the live one; the one before it is done
    list.querySelector("li.now")?.classList.remove("now")
    list.querySelectorAll<HTMLElement>("li .when").forEach((w, i) => (w.textContent = i === 0 ? "now" : `${i}m`))
    li.classList.add("now")
    list.insertBefore(li, list.children[1] ?? null)
    while (list.children.length > 5) list.lastElementChild!.remove()
    if (!live || reduced) return
    const bead = list.querySelector<HTMLElement>(".bead")!
    animate(li, { opacity: [0, 1], y: [-6, 0] }, { duration: 0.24, delay: 0.11 })
    animate(bead, { y: [-18, 12], opacity: [1, 0] }, { duration: 0.42, ease: [0.65, 0, 0.35, 1] })
    const [, h] = this.#size(this.#views.get("card")!)
    if (this.#state === "card") animate(this.#isl, { height: `${h}px` }, { type: "spring", stiffness: 520, damping: 46 })
  }

  #answer(act: string) {
    const flame = this.querySelector<HTMLElement>("[data-view=request] ember-flame")!
    if (act === "deny") {
      if (!reduced) animate(flame, { x: [0, 2.4, -2.4, 1.6, -1, 0] }, { duration: 0.46 })
      setTimeout(() => this.#show("work"), 480)
      return
    }
    if (act === "terminal") return this.#show(this.#fit("work"))
    if (!reduced) animate(flame, { y: [0, 2.2, 0, 1, 0] }, { duration: 0.46 })
    setTimeout(() => this.#show("done"), 420)
  }
}

customElements.define("ember-island", EmberIsland)
