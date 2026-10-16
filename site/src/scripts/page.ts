import { animate, inView, stagger } from "motion"
import type { EmberIsland, View } from "./island"
import "./flame"

// The page directs a day. Every section is a stretch of time (data-time to
// data-until, in minutes past midnight, past 24 for after midnight); where you
// are decides the hour, the island's clock, the sky, and what the island
// does. Noon is a session, pinned: one beat of the story per screen.

const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches
const ease = [0.22, 1, 0.36, 1] as const
const $ = <T extends Element = HTMLElement>(s: string, root: ParentNode = document) => root.querySelector<T>(s)
const $$ = <T extends Element = HTMLElement>(s: string, root: ParentNode = document) => [...root.querySelectorAll<T>(s)]
const clamp = (v: number, a = 0, b = 1) => Math.min(b, Math.max(a, v))
const mins = (s: string) => {
  const [h, m] = s.split(":").map(Number)
  return h * 60 + m
}
const clock = (m: number) => {
  const h = Math.floor(m / 60) % 24
  return `${h % 12 || 12}:${String(Math.floor(m % 60)).padStart(2, "0")} ${h < 12 ? "AM" : "PM"}`
}

type Flame = HTMLElement & { energy: number; wake(): void }
const island = $<EmberIsland>("ember-island")!

// ---- the hours ---------------------------------------------------------------

const scenes = $$("[data-time]").map((el) => ({
  el,
  name: el.dataset.scene ?? "",
  start: mins(el.dataset.time!),
  until: mins(el.dataset.until ?? el.dataset.time!),
  title: el.dataset.title ?? "",
}))

/** The scene under the middle of the window, and the time it makes. */
function now() {
  const mid = innerHeight * 0.5
  let pick = scenes[0]
  let p = 0
  for (const s of scenes) {
    const r = s.el.getBoundingClientRect()
    if (r.top <= mid) {
      pick = s
      p = clamp((mid - r.top) / r.height)
    }
  }
  return { scene: pick, t: pick.start + (pick.until - pick.start) * p }
}

// the sky: warm in the morning, green-grey at noon, amber, rose, then blue
type RGB = [number, number, number]
const skies: [number, RGB, number, RGB, number][] = [
  [9 * 60, [253, 214, 160], 0.1, [253, 200, 116], 0.07],
  [12 * 60, [170, 200, 160], 0.09, [114, 221, 113], 0.045],
  [15 * 60, [253, 200, 116], 0.08, [253, 170, 100], 0.065],
  [19 * 60, [255, 160, 120], 0.08, [230, 110, 90], 0.08],
  [22 * 60, [130, 150, 255], 0.075, [80, 100, 210], 0.07],
  [26 * 60, [100, 120, 230], 0.055, [60, 70, 170], 0.05],
]
function sky(t: number) {
  let i = skies.findIndex((k) => k[0] > t)
  if (i <= 0) i = i === 0 ? 1 : skies.length - 1
  const [a, b] = [skies[i - 1], skies[i]]
  const k = clamp((t - a[0]) / (b[0] - a[0]))
  const mix = (x: RGB, y: RGB) => x.map((v, j) => Math.round(v + (y[j] - v) * k)).join(" ")
  const root = document.documentElement.style
  root.setProperty("--sky-top", mix(a[1], b[1]))
  root.setProperty("--sky-top-a", String(a[2] + (b[2] - a[2]) * k))
  root.setProperty("--sky-low", mix(a[3], b[3]))
  root.setProperty("--sky-low-a", String(a[4] + (b[4] - a[4]) * k))
}

// the bar's window title says which window the day is in
const wintitle = $("[data-wintitle]")!
let titled = ""
function title(s: string) {
  if (s === titled) return
  titled = s
  if (reduced) {
    wintitle.textContent = s
    return
  }
  animate(wintitle, { opacity: [1, 0], y: [0, -4] }, { duration: 0.12 }).then(() => {
    wintitle.textContent = s
    animate(wintitle, { opacity: [0, 1], y: [4, 0] }, { duration: 0.22, ease })
  })
}

// ---- the hero: the island, magnified, docks into the bar --------------------

const hero = $("[data-scene=hero]")!
const zoomMax = () => (reduced ? 1 : clamp((innerWidth * 0.86) / 340, 1.3, 2.6))
let reelAt = 0
let reelOn = false
const reelViews: View[] = ["clock", "work", "done", "music"]
const coneOf: Partial<Record<View, string>> = { work: "114 221 113", done: "114 221 113", music: "240 138 93", wait: "156 202 255", request: "156 202 255" }
island.addEventListener("view", (e) => {
  hero.style.setProperty("--cone", coneOf[(e as CustomEvent<View>).detail] ?? "154 164 148")
})
function reel(on: boolean) {
  if (on === reelOn) return
  reelOn = on
  clearInterval(reelAt)
  if (!on) return
  let i = 0
  island.state = reelViews[0]
  reelAt = window.setInterval(() => {
    i = (i + 1) % reelViews.length
    island.state = reelViews[i]
  }, 3000)
}

// ---- noon: the session -------------------------------------------------------

const session = $("[data-scene=session]")!
const desk = $("[data-desk]")!
const beats = $$("[data-beat]", session).filter((e) => e.tagName === "LI")
const ticks = $$<HTMLButtonElement>("[data-goto]")
const lines = $$("[data-at]", desk)
const tbody = $(".tbody", desk)!
const typed = $("[data-type]", desk)!
const cursor = $(".cursor", desk)!
const prompt = typed.dataset.type!
const n = beats.length
const beatViews: View[] = ["work", "work", "card", "request", "done", "missed"]
let beatNow = -1

const beatTimes = [...beats.map((li) => mins(li.querySelector("time")!.textContent!.replace(/ AM| PM/, ""))), mins("11:54")]
function sessionTime(b: number) {
  const i = Math.min(n - 1, Math.floor(b))
  return beatTimes[i] + (beatTimes[i + 1] - beatTimes[i]) * clamp(b - i) * 0.9
}

function sessionProgress() {
  const r = session.getBoundingClientRect()
  return clamp(-r.top / (r.height - innerHeight)) * n
}

function story(b: number, t: number) {
  const beat = Math.min(n - 1, Math.floor(b))
  if (beat !== beatNow) {
    beatNow = beat
    beats.forEach((li, i) => li.classList.toggle("on", i === beat))
    ticks.forEach((tk, i) => tk.classList.toggle("on", i === beat))
    desk.dataset.beat = String(beat)
    desk.dataset.focus = beat >= 1 && beat <= 3 ? "web" : "term"
  }
  ticks.forEach((tk, i) => tk.style.setProperty("--p", String(clamp(b - i))))
  // the prompt types itself as you arrive, then Claude's lines stream in
  const k = Math.round(clamp(b / 0.3) * prompt.length)
  if (typed.textContent!.length !== k) typed.textContent = prompt.slice(0, k)
  cursor.classList.toggle("gone", b > 0.4)
  for (const l of lines) {
    const on = b >= Number(l.dataset.at)
    l.classList.toggle("on", on)
    if (l.dataset.until) l.classList.toggle("gone", b >= Number(l.dataset.until))
  }
  // a full terminal keeps its latest lines in view
  tbody.scrollTop = tbody.scrollHeight
  island.session = b < 0.32 ? "none" : beat < 4 ? "live" : "done"
  island.state = b < 0.32 ? "clock" : beatViews[beat]
  const minutes = Math.max(1, Math.round(t - mins("11:40")))
  $$("[data-timer]", island).forEach((e) => (e.textContent = `${Math.min(minutes, 12)}m`))
}

ticks.forEach((tk, i) =>
  tk.addEventListener("click", () => {
    const r = session.getBoundingClientRect()
    const top = scrollY + r.top + ((i + 0.45) / n) * (r.height - innerHeight)
    scrollTo({ top, behavior: reduced ? "auto" : "smooth" })
  }),
)

// answering from the island: a word under the caption, as Claude carries on
const ack = $("[data-ack]")!
let ackAt = 0
island.addEventListener("answer", (e) => {
  const act = (e as CustomEvent<string>).detail
  ack.textContent =
    act === "deny" ? "Denied. Claude tries another way." : act === "always" ? "Always allowed: go test, in kinvault, from now on." : act === "terminal" ? "Fine: it's waiting in the terminal, the way it always did." : "Allowed from the top of the screen. Claude carries on."
  ack.className = `ack on ${act === "deny" ? "r" : "b"}`
  clearTimeout(ackAt)
  ackAt = window.setTimeout(() => ack.classList.remove("on"), 3200)
})

// ---- the frame loop: one pass per scroll -------------------------------------

let queued = false
function frame() {
  queued = false
  const here = now()
  const scene = here.scene
  const b = scene.name === "session" ? sessionProgress() : 0
  const t = scene.name === "session" ? sessionTime(b) : here.t
  island.time = t
  sky(t)
  title(scene.title)

  // the hero's island: magnified at the top, docked once you scroll
  const k = clamp(scrollY / (innerHeight * 0.5))
  const z = 1 + (zoomMax() - 1) * (1 - k) ** 2
  island.zoomed = z > 1.08
  // while the session is pinned, the island is the star: a little larger on a big screen
  const r = session.getBoundingClientRect()
  const pinned = clamp(-r.top / (innerHeight * 0.4)) * clamp((r.bottom - innerHeight) / (innerHeight * 0.4))
  const big = !reduced && innerWidth >= 1200 && innerHeight >= 740 ? 0.3 : 0
  const zs = Math.max(z, 1 + big * pinned)
  island.style.transform = zs > 1.001 ? `scale(${zs})` : ""

  reel(scene.name === "hero" && k < 0.6)
  if (scene.name === "session") story(b, t)
  else {
    const want: Partial<Record<string, View>> = { moments: "music" }
    if (!reelOn) island.state = want[scene.name] ?? "clock"
    island.session = t >= mins("11:52") ? "done" : "none"
  }
  island.night = scene.name === "night"
  mind.follow(t)
  lit()
}
const queue = () => {
  if (!queued) requestAnimationFrame(frame)
  queued = true
}
addEventListener("scroll", queue, { passive: true })
addEventListener("resize", queue)

// ---- what it's for: the words light as you read ------------------------------

function words(el: HTMLElement): HTMLElement[] {
  const out: HTMLElement[] = []
  const walk = (node: Node) => {
    for (const child of [...node.childNodes]) {
      if (child.nodeType === Node.TEXT_NODE) {
        const frag = document.createDocumentFragment()
        for (const part of child.textContent!.split(/(\s+)/)) {
          if (!part.trim()) {
            frag.append(part)
            continue
          }
          const w = document.createElement("span")
          w.className = "word"
          w.textContent = part
          out.push(w)
          frag.append(w)
        }
        child.replaceWith(frag)
      } else walk(child)
    }
  }
  walk(el)
  return out
}
const litEl = $("[data-lit]")!
const litWords = words(litEl)
function lit() {
  const r = litEl.getBoundingClientRect()
  const p = reduced ? 1 : clamp((innerHeight * 0.82 - r.top) / (r.height + innerHeight * 0.3))
  const on = Math.round(p * litWords.length)
  litWords.forEach((w, i) => w.classList.toggle("on", i < on))
}

// ---- arrivals ----------------------------------------------------------------

const h1 = $("[data-words]")!
const h1Words = words(h1)
if (!reduced) {
  animate(h1Words, { opacity: [0, 1], y: ["0.4em", "0em"], filter: ["blur(8px)", "blur(0px)"] }, { duration: 0.9, delay: stagger(0.055, { startDelay: 0.2 }), ease })
  inView("[data-rise]", (el) => {
    animate(el, { opacity: [0, 1], y: [24, 0] }, { duration: 0.8, ease, delay: el.closest(".hero") ? 0.55 : 0 })
  }, { amount: 0.2 })
  inView("[data-group]", (el) => {
    animate([...el.children], { opacity: [0, 1], y: [22, 0] }, { duration: 0.7, delay: stagger(0.07), ease })
  }, { amount: 0.15 })
}

// numbers count up once
inView("[data-count]", (el) => {
  const to = Number((el as HTMLElement).dataset.count)
  const prefix = el.textContent!.startsWith("≈") ? "≈" : ""
  if (reduced) return
  animate(0, to, { duration: 1.2, ease, onUpdate: (v) => (el.textContent = `${prefix}${Math.round(v)}`) })
}, { amount: 0.6 })

// ---- afternoon: hover a moment, and the island shows it -----------------------

for (const spec of $$("[data-moment]")) {
  const go = () => island.flash(spec.dataset.moment as View, 3200)
  spec.addEventListener("pointerenter", go)
  spec.addEventListener("focus", go)
}

// ---- evening: the small mind -------------------------------------------------

/** Ember's energy over the day: the hour's baseline, a lift, a sulk. */
function energy(h: number) {
  const base = 0.5 + 0.36 * Math.cos(((h - 16.3) / 24) * 2 * Math.PI)
  const lift = 0.16 * Math.exp(-(((h - 11.87) / 0.45) ** 2))
  const sulk = -0.2 * Math.exp(-(((h - 15.08) / 0.4) ** 2))
  return clamp(base + lift + sulk, 0.04, 0.98)
}
function moodAt(h: number): [string, string, string] {
  if (h >= 0.5 && h < 7) return ["sleep", "asleep", "111 120 107"]
  if (Math.abs(h - 11.87) < 0.4) return ["happy", "pleased with itself", "114 221 113"]
  if (Math.abs(h - 15.08) < 0.35) return ["error", "sulking", "255 180 171"]
  if (h >= 22 || h < 0.5) return ["idle", "winding down", "154 164 148"]
  return ["idle", energy(h) > 0.7 ? "bright" : "steady", "154 164 148"]
}

const mind = (() => {
  const plot = $(".plot")!
  const line = $("[data-line]")!
  const area = $("[data-area]")!
  const cur = $("[data-cursor]")!
  const flame = $<Flame>("[data-dayflame]")!
  const read = $("[data-dayread]")!
  const holder = flame.parentElement!
  const Y = (e: number) => 206 - e * 180
  const pts: string[] = []
  for (let i = 0; i <= 192; i++) {
    const h = (i / 192) * 24
    pts.push(`${((h / 24) * 960).toFixed(1)} ${Y(energy(h)).toFixed(1)}`)
  }
  line.setAttribute("d", `M${pts.join(" L")}`)
  area.setAttribute("d", `M${pts.join(" L")} L960 220 L0 220 Z`)
  let hovering = false
  const show = (h: number) => {
    h = ((h % 24) + 24) % 24
    const e = energy(h)
    cur.style.setProperty("--cx", `${(h / 24) * 100}%`)
    cur.style.setProperty("--cy", `${18 + Y(e)}px`)
    const [mood, word, rgb] = moodAt(h)
    if (flame.getAttribute("mood") !== mood) flame.setAttribute("mood", mood)
    flame.energy = e
    holder.style.setProperty("--flame-rgb", rgb)
    read.textContent = `${clock(h * 60)} · ${word}`
  }
  const at = (e: PointerEvent) => {
    const r = plot.getBoundingClientRect()
    show(clamp((e.clientX - r.left) / r.width) * 24)
  }
  plot.addEventListener("pointerenter", (e) => {
    hovering = true
    at(e)
  })
  plot.addEventListener("pointermove", at)
  plot.addEventListener("pointerleave", () => {
    hovering = false
    queue()
  })
  return {
    follow(t: number) {
      if (!hovering) show(t / 60)
    },
  }
})()

// life: every blink of this flame, as a tick on a minute-long strip
{
  const blinker = $("[data-blinker]")!
  const canvas = $<HTMLCanvasElement>("[data-raster]")!
  const g = canvas.getContext("2d")!
  const blinks: number[] = []
  blinker.addEventListener("blink", () => blinks.push(performance.now()))
  let on = false
  const draw = () => {
    if (!on) return
    const { width: W, height: H } = canvas
    const t = performance.now()
    g.clearRect(0, 0, W, H)
    g.fillStyle = "rgba(222,229,216,0.12)"
    g.fillRect(0, H - 2, W, 2)
    for (let s = 0; s <= 60; s += 10) g.fillRect(W - (s / 60) * W - 1, H - 10, 2, 8)
    while (blinks.length && t - blinks[0] > 60_000) blinks.shift()
    for (const b of blinks) {
      const x = W - ((t - b) / 60_000) * W
      const age = (t - b) / 60_000
      g.fillStyle = `rgba(222,229,216,${0.85 - age * 0.55})`
      g.beginPath()
      g.roundRect(x - 2, 10, 4, H - 26, 2)
      g.fill()
    }
    requestAnimationFrame(draw)
  }
  inView(canvas, () => {
    on = true
    draw()
    return () => (on = false)
  })
}

// attention: three rivals, scored; the pill changes only when one clearly wins
{
  const box = $("[data-attention]")!
  const rows = $$("p", box)
  const out = $("[data-switches]", box)!
  const s = [0.7, 0.55, 0.35]
  const goal = [0.7, 0.6, 0.3]
  let lead = 0
  let naive = 0
  let switches = 0
  let flips = 0
  let timer = 0
  const tick = () => {
    if (Math.random() < 0.04) {
      const i = Math.floor(Math.random() * 3)
      goal[i] = 0.25 + Math.random() * 0.65
    }
    for (let i = 0; i < 3; i++) s[i] = clamp(s[i] + (goal[i] - s[i]) * 0.06 + (Math.random() - 0.5) * 0.03, 0.05, 1)
    const best = s.indexOf(Math.max(...s))
    if (best !== naive) {
      naive = best
      flips++
    }
    if (best !== lead && s[best] > s[lead] + 0.12) {
      lead = best
      switches++
    }
    rows.forEach((r, i) => {
      r.classList.toggle("lead", i === lead)
      r.querySelector("b")!.style.setProperty("--s", s[i].toFixed(3))
    })
    out.textContent = `the pill switched ${switches}× · without hysteresis ${flips}×`
  }
  inView(box, () => {
    timer = window.setInterval(tick, 120)
    return () => clearInterval(timer)
  })
}

// presence: come back from another tab, and it tells you what happened
{
  const box = $(".presence")!
  const say = $("[data-presence]")!
  let left = 0
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) left = performance.now()
  })
  island.addEventListener("presence", () => {
    const s = Math.round((performance.now() - left) / 1000)
    box.classList.add("back")
    say.textContent = `welcome back · you were away ${s}s · look up`
    setTimeout(() => {
      box.classList.remove("back")
      say.textContent = "switch tabs for a few seconds, then come back"
    }, 6000)
  })
}

// sleep: the same idle clock the island keeps
{
  const pill = $("[data-sleepy]")!
  const flame = $<Flame>("ember-flame", pill)!
  const say = $("[data-idle]")!
  const label = $("[data-sleepclock]")!
  let was = ""
  setInterval(() => {
    const { sleep, drowsyIn, asleepIn } = island.idle
    label.textContent = $("[data-clock]", island)!.textContent
    pill.classList.toggle("drowsy", sleep === "drowsy")
    pill.classList.toggle("asleep", sleep === "asleep")
    if (sleep !== was) {
      if (sleep === "asleep") flame.setAttribute("mood", "sleep")
      else {
        flame.setAttribute("mood", "idle")
        if (was === "asleep") flame.wake()
      }
      if (sleep === "drowsy") flame.dataset.drowsy = ""
      else delete flame.dataset.drowsy
      was = sleep
    }
    say.textContent =
      sleep === "asleep" ? "asleep · move to wake it" : sleep === "drowsy" ? `drowsy · asleep in ${Math.ceil(asleepIn / 1000)}s` : `awake · drowsy in ${Math.ceil(drowsyIn / 1000)}s if you stop`
  }, 500)
}

// ---- night: events in, pixels out ---------------------------------------------

{
  const flow = $("[data-flow]")!
  const svg = $<SVGSVGElement>("[data-wires]")!
  const srcs = $$("[data-src]", flow)
  const core = $("[data-core]", flow)!
  const screen = $("[data-screen]", flow)!
  const coreFlame = $<Flame>("ember-flame", core)!
  const events = $("[data-events]")!
  const frames = $("[data-frames]")!
  const NS = "http://www.w3.org/2000/svg"
  const tone: Record<string, string> = { g: "#72dd71", a: "#f08a5d", b: "#9ccaff", "": "#dee5d8" }
  let paths: SVGPathElement[] = []
  let out!: SVGPathElement
  let count = 0
  let drawn = 0
  const lay = () => {
    const f = flow.getBoundingClientRect()
    const box = (el: Element) => {
      const r = el.getBoundingClientRect()
      return { l: r.left - f.left, r: r.right - f.left, t: r.top - f.top, b: r.bottom - f.top, cx: (r.left + r.right) / 2 - f.left, cy: (r.top + r.bottom) / 2 - f.top }
    }
    const c = box(core)
    const sc = box(screen)
    const stacked = c.l < box(srcs[0]).r // the narrow layout: sources above, the screen below
    svg.replaceChildren()
    const wire = (d: string) => {
      const p = document.createElementNS(NS, "path")
      p.setAttribute("d", d)
      svg.append(p)
      return p
    }
    paths = srcs.map((s) => {
      const b = box(s)
      if (stacked) return wire(`M${b.cx} ${b.b} C${b.cx} ${(b.b + c.t) / 2} ${c.cx} ${(b.b + c.t) / 2} ${c.cx} ${c.t + 10}`)
      const mx = (b.r + c.l) / 2
      return wire(`M${b.r} ${b.cy} C${mx} ${b.cy} ${mx} ${c.cy} ${c.l + 10} ${c.cy}`)
    })
    out = stacked ? wire(`M${c.cx} ${c.b - 10} L${sc.cx} ${sc.t}`) : wire(`M${c.r - 10} ${c.cy} L${sc.l} ${sc.cy}`)
  }
  type Pulse = { path: SVGPathElement; dot: SVGCircleElement; t0: number; dur: number; done: () => void }
  const pulses: Pulse[] = []
  const send = (path: SVGPathElement, colour: string, dur: number, done: () => void) => {
    const dot = document.createElementNS(NS, "circle")
    dot.setAttribute("r", "3")
    dot.style.fill = colour
    dot.style.filter = `drop-shadow(0 0 4px ${colour})`
    svg.append(dot)
    pulses.push({ path, dot, t0: performance.now(), dur, done })
  }
  let on = false
  let mood = 0
  const loop = (t: number) => {
    if (!on) return
    for (let i = pulses.length - 1; i >= 0; i--) {
      const p = pulses[i]
      const k = clamp((t - p.t0) / p.dur)
      const e = k < 0.5 ? 2 * k * k : 1 - (-2 * k + 2) ** 2 / 2
      const pt = p.path.getPointAtLength(p.path.getTotalLength() * e)
      p.dot.setAttribute("cx", String(pt.x))
      p.dot.setAttribute("cy", String(pt.y))
      if (k >= 1) {
        p.dot.remove()
        pulses.splice(i, 1)
        p.done()
      }
    }
    requestAnimationFrame(loop)
  }
  // each source fires as a Poisson process at its own rate
  const timers: number[] = []
  const arm = (i: number) => {
    const rate = Number(srcs[i].dataset.rate)
    timers[i] = window.setTimeout(() => {
      if (!on) return
      const li = srcs[i]
      li.classList.add("ping")
      setTimeout(() => li.classList.remove("ping"), 160)
      const colour = tone[[...li.classList].find((c) => c in tone && c) ?? ""]
      send(paths[i], colour, 900, () => {
        events.textContent = String(++count)
        coneFlick()
        send(out, colour, 500, () => {
          frames.textContent = String(++drawn)
          screen.classList.add("ping")
          setTimeout(() => screen.classList.remove("ping"), 420)
        })
      })
      arm(i)
    }, -Math.log(Math.random()) * (1000 / rate))
  }
  const coneFlick = () => {
    coreFlame.setAttribute("mood", "work")
    clearTimeout(mood)
    mood = window.setTimeout(() => coreFlame.setAttribute("mood", "idle"), 1400)
  }
  new ResizeObserver(lay).observe(flow)
  inView(flow, () => {
    if (reduced) return
    on = true
    lay()
    srcs.forEach((_, i) => arm(i))
    requestAnimationFrame(loop)
    return () => {
      on = false
      timers.forEach(clearTimeout)
      pulses.splice(0).forEach((p) => p.dot.remove())
    }
  })
}

// ---- late: the reel drags like a strip of film ---------------------------------

{
  const r = $("[data-reel]")!
  let x0 = 0
  let s0 = 0
  let down = false
  r.addEventListener("pointerdown", (e) => {
    if (e.pointerType !== "mouse") return
    down = true
    x0 = e.clientX
    s0 = r.scrollLeft
    r.classList.add("dragging")
    r.setPointerCapture(e.pointerId)
  })
  r.addEventListener("pointermove", (e) => {
    if (down) r.scrollLeft = s0 - (e.clientX - x0)
  })
  const up = () => {
    down = false
    r.classList.remove("dragging")
  }
  r.addEventListener("pointerup", up)
  r.addEventListener("pointercancel", up)
}

// ---- install: copy the lines, without the comments ----------------------------

for (const b of $$<HTMLButtonElement>("[data-copy]")) {
  b.addEventListener("click", async () => {
    const code = b.closest(".terminal")?.querySelector("code")
    if (!code) return
    const text = code.innerText
      .split("\n")
      .map((l) => l.replace(/^\$\s*/, "").replace(/\s+#.*$/, "").trim())
      .filter((l) => l && !l.startsWith("#"))
      .join("\n")
    try {
      await navigator.clipboard.writeText(text)
      b.textContent = "Copied"
      b.classList.add("done")
    } catch {
      b.textContent = "Select and copy"
    }
    setTimeout(() => {
      b.textContent = "Copy"
      b.classList.remove("done")
    }, 1600)
  })
}

// ---- 2 AM: hover the sleeper and it stirs, then settles back ------------------

{
  const s = $("[data-sleeper]")!
  const f = $<Flame>("ember-flame", s)!
  let back = 0
  s.addEventListener("pointerenter", () => {
    clearTimeout(back)
    s.classList.add("awake")
    f.setAttribute("mood", "idle")
    f.wake()
  })
  s.addEventListener("pointerleave", () => {
    back = window.setTimeout(() => {
      s.classList.remove("awake")
      f.setAttribute("mood", "sleep")
    }, 1600)
  })
}

frame()
