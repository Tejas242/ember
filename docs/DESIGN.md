# Ember's design

Ember is a dynamic island for a 1366×768 desktop, hanging from the centre of a 22 px glass bar. The bar already shows
workspaces, the window title, CPU, RAM, the tray, network, volume and battery percentage. It shows no clock. So Ember
never repeats any of that, and it does own the time.

## What it is for

You glance at the top of the screen for under two seconds. Ember answers, in this order:

1. **Does anything need me?** A request, a question, a session waiting in its terminal.
2. **Is the work going well?** What's running, for how long, whether anything failed.
3. **What happened?** Something finished, a song changed, a key you pressed.

Everything else is noise and stays out: no settings, no system controls, no battery level (the bar has it), no
subagent counts and no tool-call-by-tool-call chatter in the pill.

## Principles

- **One surface, three sizes.** A pill the height of the bar; the card, when you hover; a request, when Claude asks.
  The shape morphs between them on one spring and never pops.
- **Stable at a glance.** The pill's width and words change only when the *situation* changes (working → done), never
  per tool call. Live detail (steps, timers) lives in the card.
- **No boxes in boxes.** The card is one dark surface. Sections are separated by space and a hairline, never by
  nested panels. A soft highlight appears only under the pointer.
- **Colour is meaning.** Green works or succeeded, blue needs you, red failed, amber is a subagent, a nudge or a
  session gone quiet. Everything else is three greys: text (fg), secondary (fg 64%, 7:1), tertiary (fg 50%, 4.4:1),
  all readable at 1×.
- **Nothing slips by.** What finished or stopped while you weren't looking waits by the clock ("kinvault done") until
  you've seen it. A failed turn never shows up as a success.
- **Ember is the thread.** The flame is the identity. In the card, the session's timeline hangs from it on a thin
  rail, so the mascot and the work read as one thing.

## Type

Geist, at 1× (72 dpi px). Four sizes, three weights.

| Role | Face | Size | Colour |
| --- | --- | --- | --- |
| Headline | SemiBold | 14 | fg |
| Body, names, the live step | Medium | 12.5 | fg |
| Secondary, steps | Regular | 12 | secondary |
| Caption, facts, times | Regular | 11 | tertiary |
| Eyebrow | SemiBold caps, +0.6 tracking | 9.5 | tertiary |
| Code | Geist Mono | 11.5 | syntax tint |

## Space

4 px grid. The card is 380 wide with a 16 px margin. The rail sits on Ember's centre line (x = 26), and text starts
at 48. Rows: the headline block 52, a step 22, a session line 30, music 52 (a 24 px cover inside the margin), edge 34.
Radii: the card 22 (continuous superellipse), hovers 10, buttons fully round (26 tall), code 10.

## The pill

`[lead 16] label  detail ………… trailing [satellite]`, 10 px ends, height = bar.

| Situation | Lead | Label | Trailing |
| --- | --- | --- | --- |
| Quiet | Ember (breathing, blinking, asleep after a while) | time | — |
| Something finished unseen | | time, then `kinvault done` in green (or `stopped` in red) | — |
| Claude working (one) | Ember flickering green | project, a band of light passing every 2.4 s | elapsed (from a minute), rolling; amber once it's been quiet 3 min |
| Claude working (several) | same | the session you can't see | `+2` chip, elapsed |
| Needs you | Ember, blue, looking up | `kinvault needs you` | a blue light breathes around the pill, with a lit rim |
| Done | Ember, happy, bounces | **Done** kinvault, the reply's best sentence | a check that strokes itself in |
| Stopped on an error | Ember, red | **Stopped** kinvault, in words (`Rate limited`) | — |
| Music | the cover | title, then the artist in secondary | the equaliser in the cover's colour; a hairline of the song along the rim |
| Volume, brightness | icon | — | a 4 px meter, the value rolling |
| Charging, headphones, screenshot | icon / thumbnail | what happened (`Charging`, `full in 40m`) | battery glyph; a value only where the bar has none (headphones, battery low) |
| A second activity | | | a satellite after a hairline: the spinning cover, a tiny flame. Click swaps it in |

## The card

```
 [flame] Fix the race in the store's compaction path         5:24 PM
    │    kinvault · 4m · 5 steps · +19 −5
    ◌    Running the whole suite under the race detector         12s
    │    Edited store.go                               +4 −2    now
    ✕    Run the store tests                                  failed
    ╰ ✦  Map the store's locking   Searching segment.Lock         1m
 ───────────────────────────────────────────────────────────────────
    ●    edge     Searching segment.Lock                          2m
    ✓    ember    Done · The island is redesigned             3m ago
 ───────────────────────────────────────────────────────────────────
 [art]   Veridis Quo                                   ▮▮▮   −2:58
         Daft Punk
         ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━──────────────────
 ───────────────────────────────────────────────────────────────────
    ◎    Practice   1/1 today · 4-day streak                     ▥
```

- **The hero** is the session that matters most: one waiting on you, else one working that you aren't looking at, else
  the one that just finished. Its headline is the task in words, its facts line is one row (where, how long, what it
  changed, failures in red), and its timeline hangs from Ember. Waiting says what it's asking (*Wants to run go test
  …· waiting 1m*) and adds a blue *Answer in terminal* button. Done replaces the timeline with Claude's most
  informative sentence beside a bar: green, amber if it admits a failure, red with the error if it stopped. With no
  session, the hero is the day: a greeting, the day in one line, and 24 hour bars.
- **Other sessions** are one line each: *Done ·*, *Stopped ·*, *Needs you ·* say the outcome in its colour. Under the
  pointer a chevron replaces the time. Click one and it becomes the hero: the card cross-fades and the lines that
  stay glide to their places.
- **Music** has no box. The cover lights the section softly in its own colour, fading out before any edge. Controls
  replace the equaliser under the pointer. The progress line thickens and can be dragged. A song paused for 15 minutes
  leaves the card.
- **Practice** (edge) is one line with a chevron: the way to its dashboard. While a contest round matters (two hours
  before one edge holds you to, while it runs, half an hour after) the line is the round: a ring on the rail that
  empties as the clock runs, the letters, and its one action (Open, Can't make it, Upsolve).
- **Round letters** are chips, one per problem: solved filled green, tried ringed red, owed ringed amber, upsolved
  filled blue, lapsed struck through. The pill, the row and the dashboard draw them the same way.

## Requests

```
 [flame]  KINVAULT WANTS TO RUN                               2 more
          Run the store tests with the race detector
   ┌─────────────────────────────────────────────────────────────┐
   │ go test -race ./store/... -run TestRepair -count=1          │
   └─────────────────────────────────────────────────────────────┘
   Always allows go test:* in this project
   Answer in terminal                   Deny    Always   [ Allow ]
 ──────────── a hairline drains while it waits ────────────
```

An eyebrow says who wants what (*wants to run*, *wants to edit*, *asks*). The title is Claude's own words. The command
is never cut where it matters: it wraps, and a very long one keeps its start and its end. A quiet line says what
*Always* would allow before you press it. A question's options form one grouped list with hairlines and numbered
keys, not separate boxes.

## Motion

Every motion is physical (a spring) or a short ease. Nothing is linear except a progress bar.

| What | How |
| --- | --- |
| Opening | the width leads (k 420, ζ 0.95, no overshoot); the drop overshoots a few px (k 300, ζ 0.81). A request arrives with a little more (ζ 0.75) |
| Closing | the card folds up first, critically damped (k 600), then the width follows. The pill's words wait until the card has nearly folded |
| Content in | the card's words wait until the shape is ¾ open, so no line starts clipped. Rows then fade in and rise 4 px, 24 ms apart (the seventh and later together), 200 ms each. Rows land on whole pixels, so text and its icon move as one |
| Content out | the old view fades in 90 ms from however visible it was, drifting 3 px up |
| Live parts (pulses, the equaliser, shimmer) | arrive faded with their row, never pop in after it |
| The pill changing width | content rides the shape's live edges; Ember stays in its slot |
| Anticipation | the pill swells 8 × 2 px under the pointer on its own quick spring; its content doesn't move |
| A label changing in place | the old words slide up and out, the new ones rise in (260 ms) |
| Numbers | minutes roll, digit by digit, inside their own line; seconds just tick |
| Hover | in over ~95 ms, out over ~250 ms; controls fade in under the pointer |
| Press | a control goes in at once under the finger (50 ms) and acts on release over it, so you can slide off one you regret |
| Another session becomes the hero | the card cross-fades, the new hero's rows arrive staggered from the click, the lines that stay glide |
| A step finishing | its green and white ease back to grey |
| Done | Ember bounces; 180 ms in, one band of light crosses the pill (ease-in-out, 700 ms); a check strokes itself in |
| Needs you | a blue light breathes around the pill (outside the bar), with a lit rim; it fades out when the wait is over |
| One beat | every pulse keeps a 1.6 s beat: the step ring, the waiting pulse, the halo; orbits turn once in two beats |
| Working | a band of light passes over the label every 2.4 s (1 s pass, eased); every shimmer passes together |
| A new step | the old steps make room, then a bead of light runs down the thread from Ember into the new step (420 ms); the stretch to the live step glows green |
| Ember reacts | a nod (two small dips) when you allow, a head shake when you deny, a flinch when a step fails |
| The pill's song | the cover is a disc in a ring that fills as the song plays |
| System pop-ups | volume waves come and go with the level, the sun's rays lengthen with brightness, the meter stretches and springs back past 100, the battery fills to its level, a screenshot flashes like a shutter and its preview drops in |
| Asleep | the pill dims 16 % and the clock turns grey |
| The equaliser | the song's own low, mid and high (25 Hz, rising fast, falling slowly) |
| Queued requests | their edges peek out under the card, a stack |
| Request countdown | a 1 px hairline on the rim drains from both ends; in the last 15 s it eases to amber and thickens |

## Cost

All drawing is in software. Steady frames repaint only the live rectangles (Ember, shimmer, equaliser, halo,
countdown), at 12 fps in the pill, 24 in the card and 40 while a shimmer band is crossing. The halo and the sweep are live parts, not full repaints. Targets: ≤ 1.5% of a core while Claude works,
no full repaint while nothing changes, and memory under 16 MiB.
