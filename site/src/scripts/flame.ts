// Ember's flame, ported from its Go renderer (island/draw.go, emberX): two
// tongues of cubic Béziers, a sway and a stretch that follow its mood, two
// eyes that blink at random and look at the pointer.

export type Mood = "idle" | "work" | "wait" | "happy" | "error"

const KAPPA = 0.5522847

export function tongue(s: number, scale: number, lift: number, sway: number, stretch: number): string {
  const r = s * 0.36 * scale
  const x = 0
  const y = s * 0.17 - lift
  const tX = x + r * 0.3 + sway * scale
  const tY = y - s * 0.74 * scale * stretch
  const k = r * KAPPA
  const lx = x - r * 0.9 + sway * 0.4
  const ly = y - r * 1.45 * stretch
  const p = (n: number) => n.toFixed(2)
  return [
    `M${p(tX)} ${p(tY)}`,
    `C${p(tX + r * 0.12)} ${p(tY + r * 0.8)} ${p(x + r * 1.08)} ${p(y - r * 0.95)} ${p(x + r)} ${p(y)}`,
    `C${p(x + r)} ${p(y + k)} ${p(x + k)} ${p(y + r)} ${p(x)} ${p(y + r)}`,
    `C${p(x - k)} ${p(y + r)} ${p(x - r)} ${p(y + k)} ${p(x - r)} ${p(y)}`,
    `C${p(x - r * 1.06)} ${p(y - r * 0.55)} ${p(x - r * 1.12)} ${p(y - r * 1.1)} ${p(lx)} ${p(ly)}`,
    `C${p(x - r * 0.72)} ${p(y - r * 1.2)} ${p(x - r * 0.48)} ${p(y - r * 1.08)} ${p(x - r * 0.28)} ${p(y - r * 1.12)}`,
    `C${p(x - r * 0.08)} ${p(y - r * 1.5)} ${p(tX - r * 0.32)} ${p(tY + r * 0.62)} ${p(tX)} ${p(tY)}Z`,
  ].join("")
}

export const COLOURS: Record<Mood, string> = {
  idle: "#9aa494",
  work: "#72dd71",
  wait: "#9ccaff",
  happy: "#72dd71",
  error: "#ffb4ab",
}

const S = 100 // the flame's size in its own units; the viewBox fits it

class EmberFlame extends HTMLElement {
  static observedAttributes = ["mood"]
  #outer!: SVGPathElement
  #inner!: SVGPathElement
  #eyes: SVGRectElement[] = []
  #look = 0
  #lookTo = 0
  #blinkAt = performance.now() + 1500
  #raf = 0
  #visible = true
  #energy = 0.6

  connectedCallback() {
    const ns = "http://www.w3.org/2000/svg"
    const svg = document.createElementNS(ns, "svg")
    svg.setAttribute("viewBox", "-50 -68 100 128")
    svg.setAttribute("aria-hidden", "true")
    this.#outer = document.createElementNS(ns, "path")
    this.#inner = document.createElementNS(ns, "path")
    svg.append(this.#outer, this.#inner)
    for (let i = 0; i < 2; i++) {
      const e = document.createElementNS(ns, "rect")
      e.setAttribute("fill", "#080b08")
      svg.append(e)
      this.#eyes.push(e)
    }
    this.append(svg)
    this.#energy = Number(this.dataset.energy ?? 0.6)
    this.#paint()
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) {
      this.#frame(0)
      return
    }
    new IntersectionObserver(([e]) => {
      this.#visible = e.isIntersecting
      if (this.#visible) this.#loop()
    }).observe(this)
    addEventListener("pointermove", (e) => {
      const r = this.getBoundingClientRect()
      const dx = e.clientX - (r.left + r.width / 2)
      this.#lookTo = Math.max(-1, Math.min(1, dx / Math.max(240, r.width * 3)))
    })
    this.#loop()
  }

  attributeChangedCallback() {
    this.#paint()
  }

  get mood(): Mood {
    return (this.getAttribute("mood") as Mood) || "idle"
  }

  #paint() {
    const c = COLOURS[this.mood] ?? COLOURS.idle
    this.style.setProperty("--flame", c)
    this.#outer?.setAttribute("fill", c)
    this.#inner?.setAttribute("fill", `color-mix(in oklab, ${c} 40%, white)`)
  }

  #loop = () => {
    cancelAnimationFrame(this.#raf)
    const tick = (now: number) => {
      if (!this.#visible || document.hidden) return
      this.#frame(now / 1000)
      this.#raf = requestAnimationFrame(tick)
    }
    this.#raf = requestAnimationFrame(tick)
  }

  #frame(t: number) {
    const en = this.#energy
    let sway = 0
    let stretch = 1
    switch (this.mood) {
      case "work": {
        const f = 0.75 + 0.5 * en
        sway = (Math.sin(t * 7 * f) * 0.06 + Math.sin(t * 11.3 * f) * 0.035) * (0.6 + 0.8 * en) * S
        stretch = 1 + Math.sin(t * 9.1 * f) * 0.05 * (0.6 + 0.8 * en)
        break
      }
      case "wait":
        stretch = 1 + Math.sin(t * 3 * (0.8 + 0.6 * en)) * 0.06
        break
      case "happy":
        stretch = 1 + Math.sin(t * 5) * 0.03
        break
      default:
        stretch = 1 + Math.sin(t * 1.6 * (0.55 + 0.9 * en)) * 0.05
        sway = Math.sin(t * 0.9 * (0.6 + 0.6 * en)) * 0.03 * S
    }
    this.#outer.setAttribute("d", tongue(S, 1, 0, sway, stretch))
    this.#inner.setAttribute("d", tongue(S, 0.5, -S * 0.1, sway, stretch))

    // blinks: a Poisson process, about fifteen a minute, sometimes doubled
    const now = performance.now()
    let blink = 0
    const since = now - this.#blinkAt
    if (since > 0 && since < 150) blink = Math.sin((since / 150) * Math.PI)
    else if (since >= 150) this.#blinkAt = now + 600 - Math.log(Math.random()) * 3400
    this.#look += (this.#lookTo - this.#look) * 0.08

    let eyeY = S * 0.19
    const gap = S * 0.125
    const ew = S * 0.1
    let eh = S * 0.17 * (1 - blink * 0.85)
    if (this.mood === "happy") {
      eh = S * 0.05
      eyeY -= S * 0.02
    }
    if (this.mood === "wait") eyeY -= S * 0.05
    const ex = sway * 0.3 + this.#look * S * 0.07
    this.#eyes.forEach((e, i) => {
      const cx = (i === 0 ? -gap : gap) + ex
      e.setAttribute("x", (cx - ew / 2).toFixed(2))
      e.setAttribute("y", (eyeY - eh / 2).toFixed(2))
      e.setAttribute("width", ew.toFixed(2))
      e.setAttribute("height", Math.max(eh, 1.5).toFixed(2))
      e.setAttribute("rx", (ew / 2).toFixed(2))
    })
  }
}

customElements.define("ember-flame", EmberFlame)
