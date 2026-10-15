# Ember's design

Ember is a dynamic island for a 1366×768 desktop, hanging from the centre of a 22 px glass bar. The bar already shows
workspaces, the window title, CPU, RAM, the tray, network, volume and battery percentage. It shows no clock. So Ember
never repeats any of that, and it does own the time.

## What it is for

You glance at the top of the screen for under two seconds. Ember answers, in this order:

1. **Does anything need me?** A request, a question, a session waiting in its terminal.
2. **Is the work going well?** What's running, for how long, whether anything failed.
3. **What happened?** Something finished, a song changed, a key you pressed.

Everything else is noise and stays out: no settings, no system controls, no battery level (the bar has it), and no
tool-call-by-tool-call chatter in the pill.

## Principles

- **One surface, three sizes.** A pill the height of the bar; the card, when you hover; a request, when Claude asks.
  The shape morphs between them on one spring and never pops.
- **Stable at a glance.** The pill's width and words change only when the *situation* changes (working → done), never
  per tool call. Live detail (steps, timers) lives in the card.
- **No boxes in boxes.** The card is one dark surface. Sections are separated by space and a hairline, never by
  nested panels. A soft highlight appears only under the pointer.
- **Colour is meaning.** Green works or succeeded, blue needs you, red failed, amber is a subagent or a nudge.
  Everything else is three greys: text, secondary, tertiary.
- **Ember is the thread.** The flame is the identity. In the card, the session's timeline hangs from it on a thin
  rail, so the mascot and the work read as one thing.

## Type

Geist, at 1× (72 dpi px). Four sizes, three weights.

| Role | Face | Size | Colour |
| --- | --- | --- | --- |
| Headline | SemiBold | 14 | fg |
| Body, names, the live step | Medium | 12.5 | fg |
| Secondary, steps | Regular | 12 | 70% fg |
| Caption, facts, times | Regular | 11 | tertiary |
| Eyebrow | SemiBold caps, +0.6 tracking | 9.5 | tertiary |
| Code | Geist Mono | 11.5 | syntax tint |

## Space

4 px grid. The card is 380 wide with a 16 px margin. The rail sits on Ember's centre line (x = 25), and text starts
at 42. Rows: the headline block 52, a step 22, a session line 30, music 52, edge 32. Radii: the card 22 (continuous
superellipse), hovers 10, buttons fully round, code 10.

## The pill

`[lead 16] label  detail ………… trailing [satellite]`, 10 px ends, height = bar.

| Situation | Lead | Label | Trailing |
| --- | --- | --- | --- |
| Quiet | Ember (breathing, blinking, asleep after a while) | time | — |
| Claude working (one) | Ember flickering green | project, lit by a slow shimmer | elapsed, rolling digits |
| Claude working (several) | same | first project | `+2` chip, elapsed |
| Subagents running | | | amber spark and count |
| Needs you | Ember, blue, looking up | `kinvault needs you` | a blue halo breathes around the pill |
| Done | Ember, happy; a check draws itself | **Done** kinvault | the first sentence of the reply, secondary |
| Stopped on an error | Ember, red | **Stopped** kinvault | the error |
| Music | the cover | title, then the artist in secondary | the equaliser in the cover's colour; a hairline of the song along the rim |
| Volume, brightness | icon | — | a 4 px meter, the value rolling |
| Charging, headphones, screenshot | icon / thumbnail | what happened | value, battery glyph |
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
    ✓    ember    Redesign the card around sessions               3m
 ───────────────────────────────────────────────────────────────────
 [art]   Veridis Quo                                   ▮▮▮   −2:58
         Daft Punk
         ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━──────────────────
 ───────────────────────────────────────────────────────────────────
    ◎    Practice   1/1 today · 4-day streak                     ▥
```

- **The hero** is the session that matters most: one waiting on you, else one working that you aren't looking at, else
  the one that just finished. Its headline is the task in words, its facts line is one row, and its timeline
  hangs from Ember. Waiting adds a blue *Go to terminal* button. Done replaces the timeline with Claude's most
  informative sentence, under a green bar. With no session, the hero is the day: a greeting, the day in one line, and
  24 hour bars.
- **Other sessions** are one line each. Click one and it becomes the hero: the rows glide to their new places and
  the hero's height springs.
- **Music** has no box. The cover lights the section softly in its own colour. Controls replace the equaliser
  under the pointer. The progress line thickens and can be dragged.
- **edge** is one line, and a way to its dashboard.

## Requests

```
 [flame]  KINVAULT WANTS TO RUN                               2 more
          Run the store tests with the race detector
   ┌─────────────────────────────────────────────────────────────┐
   │ go test -race ./store/... -run TestRepair -count=1          │
   └─────────────────────────────────────────────────────────────┘
   In terminal                          Deny    Always   [ Allow ]
 ──────────── a hairline drains while it waits ────────────
```

An eyebrow says who wants what (*wants to run*, *wants to edit*, *asks*). The title is Claude's own words. A question's
options form one grouped list with hairlines and numbered keys, not separate boxes.

## Motion

Every motion is physical (a spring) or a short ease-out. Nothing is linear except a progress bar.

| What | How |
| --- | --- |
| Shape (pill ↔ card ↔ request) | width and height on a spring (k 320, d 31): quick, with a whisper of overshoot. The corner radius follows the height |
| Content in | rows fade in and rise 6 px, staggered 28 ms apart, each over 220 ms (ease-out cubic) |
| Content out | the old view fades in the first 45% of the new one's entrance, drifting 3 px up |
| Pill label change in place | the old words slide up and out, the new ones rise in (260 ms) |
| Numbers | each changed digit rolls (260 ms) |
| Rows reordering, the hero changing | each row's y on its own critically damped spring: rows glide, never jump |
| A new step in the timeline | it slides down from the top, and the others make room |
| Hover | the highlight eases in over ~120 ms; controls fade in under the pointer |
| Press | the control dips (scale 0.96, brighten) for 220 ms |
| Anticipation | the pill swells 8×2 px under the pointer before the card drops |
| Done | Ember bounces, a check strokes itself in over 320 ms, and one band of light sweeps the pill |
| Needs you | a blue halo breathes around the pill (1.6 s period) |
| Working | a slow band of light passes over the label every ~2.5 s |
| Request countdown | a hairline drains from both ends toward the centre, and turns amber in the last 15 s |

## Cost

All drawing is in software. Steady frames repaint only the live rectangles (Ember, shimmer, equaliser, halo,
countdown). The halo and the sweep are live parts, not full repaints. Targets: ≤ 1.5% of a core while Claude works,
no full repaint while nothing changes, and memory under 16 MiB.
