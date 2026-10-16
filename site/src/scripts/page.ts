import { animate, inView, scroll, stagger } from "motion"
import "./flame"

const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches
const ease = [0.22, 1, 0.36, 1] as const

// The island acts out the section you're reading: the one crossing a line
// just above the middle of the window decides what it shows.
const island = document.querySelector("ember-island") as (HTMLElement & { state: string }) | null
if (island) {
  const io = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (e.isIntersecting) island.state = (e.target as HTMLElement).dataset.island!
      }
    },
    { rootMargin: "-40% 0px -55% 0px" },
  )
  document.querySelectorAll("[data-island]").forEach((el) => io.observe(el))
}

// the headline's words rise in, one after another
const h1 = document.querySelector("h1")
if (h1 && !reduced) {
  const words: HTMLElement[] = []
  const split = (node: Node) => {
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
          words.push(w)
          frag.append(w)
        }
        child.replaceWith(frag)
      } else split(child)
    }
  }
  split(h1)
  animate(words, { opacity: [0, 1], y: ["0.35em", "0em"], filter: ["blur(6px)", "blur(0px)"] }, { duration: 0.8, delay: stagger(0.06, { startDelay: 0.15 }), ease })
}

// everything else arrives as it comes into view
if (!reduced) {
  inView(
    "[data-reveal]",
    (el) => {
      animate(el, { opacity: [0, 1], y: [22, 0] }, { duration: 0.7, ease })
    },
    { amount: 0.15 },
  )
  inView(
    "[data-reveal-group]",
    (el) => {
      animate(el, { opacity: 1 }, { duration: 0.01 })
      animate(el.children, { opacity: [0, 1], y: [18, 0] }, { duration: 0.6, delay: stagger(0.08), ease })
    },
    { amount: 0.2 },
  )
  // the thread down the side fills as you read
  const thread = document.querySelector<HTMLElement>("[data-thread]")
  if (thread) scroll(animate(thread, { scaleY: [0, 1] }, { ease: "linear" }))
}

// the numbers count up once
inView(
  ".numbers dd",
  (el) => {
    const node = el.firstChild
    if (!node || node.nodeType !== Node.TEXT_NODE) return
    const final = node.textContent!.trim()
    const n = Number(final.replace(/[^\d.]/g, ""))
    const prefix = final.startsWith("≈") ? "≈" : ""
    if (!n || reduced) return
    animate(0, n, { duration: 1.1, ease, onUpdate: (v) => (node.textContent = `${prefix}${Math.round(v)}`) })
  },
  { amount: 0.6 },
)

// the energy curve draws itself
inView(".energy", (el) => {
  const path = el.querySelector("path")!
  const len = path.getTotalLength()
  path.style.strokeDasharray = `${len}`
  if (!reduced) animate(path, { strokeDashoffset: [len, 0] }, { duration: 1.4, ease })
})

// copy the install lines, without the comments
document.querySelectorAll<HTMLButtonElement>("[data-copy]").forEach((b) =>
  b.addEventListener("click", async () => {
    const pre = b.closest(".terminal")?.querySelector("code")
    if (!pre) return
    const text = [...pre.innerText.split("\n")]
      .map((l) => l.replace(/\s+#.*$/, "").trim())
      .filter((l) => l && !l.startsWith("#"))
      .join("\n")
    try {
      await navigator.clipboard.writeText(text)
      b.textContent = "Copied"
    } catch {
      b.textContent = "Select and copy"
    }
    setTimeout(() => (b.textContent = "Copy"), 1600)
  }),
)
