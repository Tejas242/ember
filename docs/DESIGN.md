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
| Quiet | Ember (breathing, blinking, a flame asleep against the pill's wall after a while) | time | — |
| Something finished unseen | | time, then `kinvault done` in green (or `stopped` in red) | — |
| Claude working (one) | Ember, orange, reading the label, a gulp of fuel per tool call | project, a band of light passing every 2.4 s | elapsed (from a minute), rolling; amber once it's been quiet 3 min |
| Claude working (several) | same | the session you can't see | `+2` chip, elapsed |
| Needs you | Ember, blue, hops and looks up at you | `kinvault needs you` | a blue light breathes around the pill, with a lit rim |
| Done | Ember, gold, celebrates as big as the work was long | **Done** kinvault, the reply's best sentence | a check that strokes itself in |
| Stopped on an error | Ember, red, flinches, a puff of smoke | **Stopped** kinvault, in words (`Rate limited`) | — |
| Music | Ember in headphones, dancing, and the cover in a ring of the song | title, then the artist in secondary | — (Ember is the equaliser now; paused, it takes its headphones off) |
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
| Done | Ember celebrates; 180 ms in, one band of light crosses the pill (ease-in-out, 700 ms); a check strokes itself in |
| Needs you | a blue light breathes around the pill (outside the bar), with a lit rim; it fades out when the wait is over |
| One beat | every pulse keeps a 1.6 s beat: the step ring, the waiting pulse, the halo; orbits turn once in two beats |
| Working | a band of light passes over the label every 2.4 s (1 s pass, eased); every shimmer passes together |
| A new step | the old steps make room, then a bead of light runs down the thread from Ember into the new step (420 ms); the stretch to the live step glows green |
| Ember reacts | see *Ember* below: every motion of Ember's is physics |
| The pill's song | the cover is a disc in a ring that fills as the song plays |
| System pop-ups | volume waves come and go with the level, the sun's rays lengthen with brightness, the meter stretches and springs back past 100, the battery fills to its level, a screenshot flashes like a shutter and its preview drops in |
| Asleep | the pill dims 16 % and the clock turns grey |
| The equaliser (the card's cover) | the song's own low, mid and high (25 Hz, rising fast, falling slowly) |
| Ember dancing | it counts the beat: kicks teach it the tempo, a phase-locked count keeps it on the beat between them (and through a bar without a kick); it dips on the beat, sways over two, nods, or bounces on an energetic song, a new move each eight beats, as big as the song's energy. A song with no kick to lock onto gets a slow sway, eyes closed |
| Queued requests | their edges peek out under the card, a stack |
| Request countdown | a 1 px hairline on the rim drains from both ends; in the last 15 s it eases to amber and thickens |

## Ember

Ember is a character, the same one everywhere: a small flame with a face. It's in every pill, never tucked away.
Beside it sits what it's showing you (the cover, a moment's icon, a screenshot). On a volume or brightness meter it
jumps onto the rail and rides the level as its knob. When a song plays it wears headphones and dances to it,
wherever it is (the pill, the card), unless something needs you or went wrong. It counts the beat rather than
jumping after each kick: kicks teach it the tempo and a phase-locked count keeps it on time (internal/island/groove.go).

**Its own fire.** Ember ignores the theme and the wallpaper. Its mood is the flame's temperature: a calm coral
ember, a working orange, the gold of a job done, the blue of a flame running hot because something needs you, a deep
red when it stopped, a fire running low when things went quiet. Each fire has three stops (the hot base and core,
the body, the cooler tips), drawn as a gradient. The face sits on the bright inner light, so dark eyes read on it.
Its headphones are one cool blue, against the warm fire.

**The face is pixels.** At the pill's 16 px, the eyes are 2 px wide, 2 px apart, snapped to whole pixels: an eye a
pixel and a half wide on a fractional position is a grey smudge. Eye height is attention: 1 a blink, 2 drowsy, 3
calm, 4 attentive, 5 startled. Lids and slants cut whole pixels. Faces an arc can't draw at that size are pixel
glyphs: `^ ^` happy, `> <` a flinch, a sleeping curve, a 2×2 "o", a yawn. A glance moves the eyes one pixel at most.

**A body with real physics** (internal/island/body.go). It's a small mass that jumps and lands under gravity (700 px/s²: a
2.6 px hop is 170 ms in the air). Landing squashes it by how hard it hit; leaving the ground stretches it. Its slot's
acceleration is felt as inertia: the body shears back when it's carried off and forward when it stops; the tip lags
the body. A trip across the pill (onto a meter, home again) is a ballistic hop that lands exactly on its slot. Sparks
and smoke are particles, launched, buoyant, dragged by the air; a spark is one bright pixel. Headphones fall onto its
head, bounce and ride it. It all integrates in fixed 1/240 s steps, so it's the same at 6 frames a second or 60,
and it rests (no frames) when nothing moves.

**Acts** (internal/island/acts.go) answer what just happened: impulses to the body, a face over a timeline.

| Act | When |
| --- | --- |
| Ignite | a prompt is sent: it crouches, catches with a flash, pops up throwing two sparks |
| Gulp | each tool call of the session it's watching (one per 1.2 s at most): the flame swells and licks |
| Needs you | a request or a question: a crouch, a hop, wide eyes, a double blink, a smaller hop |
| Done | its size from how long the turn took (20 s small, 5 min the whole thing): a hop, sparks, a second hop; a flare if earned |
| Stopped | it flinches back with `> <`, the flame gutters, two puffs of smoke; it stays slumped |
| Wince, relief | a failed step; the first step that passes after three failures (a slow blink, an exhale) |
| Nod, shake | you allow; you deny (the head held by a muscle, the eyes leading, the tip trailing) |
| Sleep, wake | a yawn, then it banks low against the pill's wall, its light breathing; waking, it shoots up and looks around |
| Moments | charging is a gulp of power with sparks, a screenshot's flash makes it blink hard, headphones put on |

**Its character** (internal/island/bond.go). Four traits decide what it does on its own. *Watchful:* every look has a cause.
*Invested:* it feels how the work goes, honestly (smaller and worried while steps keep failing, squashed while Claude
compacts, awake keeping watch while Claude works and you're away, smaller on a low battery, a game face during a
contest round), and celebrates only what was earned (a turn three times your usual or ten minutes, or hard-won and
ended well). *Self-sufficient:* it never adds words, width or light to the pill for itself. *Wry:* rarely, about
something real: it rolls its eyes when Claude opens with "You're absolutely right".

A flourish (the morning stretch, a yawn near your bedtime, the eye-roll) goes through a gate: only when the island
is calm (nothing waits on you, nothing failed unseen, no card, no moment, nobody watching your screen), at most one
every twenty minutes and five a day, each on its own cooldown. It remembers in `~/.local/state/ember/bond.json`:
when your mornings start, your bedtime, a turn's usual length, the cooldowns.

Click Ember and it takes you where the pill would (a waiting session, a Done); otherwise it's a poke: it squashes
and looks at you, the third puffs it up, the sixth sends it into its coal for a moment. Rest the pointer on it in
the card and it closes its eyes, pleased.

## Its mind

Ember has a small mind. None of it is scripted animation. It is a handful of models running on real time:

- **Affect.** Ember has an energy and a mood that relax toward a baseline, set by the time of day (peaking
  mid-afternoon, lowest around 3 am), by what's running, and by whether you're at the machine. Events nudge them:
  a tool call a little, a finished task brightens the mood, an error darkens it for a while. Energy sets how hard the
  flame flickers, how fast it breathes and how open its eyes are. Mood tints its colour. It casts a soft light on the
  island, brighter when it's lively.
- **Life.** Blinks come at random, about fifteen a minute, sometimes doubled. Glances are random too, and the eyes are
  never perfectly still. Ember looks at things as they appear: a note, the satellite at the pill's end. When the desk has
  been quiet for a while it gets drowsy, then falls asleep (eyes shut, z's rising), and wakes when something happens
  or your pointer comes near.
- **Presence.** Through the compositor's idle notifications it knows when you've stepped away. Sessions that finish or
  ask for you meanwhile aren't shown to an empty room. When you come back, one line tells you what happened:
  *While you were away: kinvault done · ember asked you*.
- **Attention.** What the pill shows is scored, not hard-coded. A running session scores high. A new track scores
  high and then fades as you get used to it. If you're already looking at that session's terminal, the pill shows
  something else and Claude moves to the satellite. A choice holds until a rival clearly beats it, so the pill never
  flickers between close calls. A request waiting on you, and anything you just pressed, always come first.
- **Habits.** It learns which row you reach for in each situation (music while Claude works, say) and puts it
  higher. Older evidence fades, so a new habit takes over within a few uses.
- **Judgement.** "Done" quotes the most informative sentence of Claude's reply (outcomes and numbers win over
  preamble). Notes that arrive together merge ("Done · edge, ember"). A volume change hands a pending note back when
  it's finished. A step running long shows its minutes. A session that went silent for half an hour (a killed Claude
  never says Stop) settles to idle.

## Cost

All drawing is in software. Steady frames repaint only the live rectangles (Ember, shimmer, equaliser, halo,
countdown), at 12 fps in the pill, 24 in the card and 40 while a shimmer band is crossing. The halo and the sweep are live parts, not full repaints. Targets: ≤ 1.5% of a core while Claude works,
no full repaint while nothing changes, and memory under 16 MiB.
