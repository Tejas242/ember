# Ember

A dynamic island for the top of a Wayland desktop. It sits over the centre of your bar as a small pill with a flame
in it, and morphs to show what is going on: what Claude Code is doing and what it needs from you, the song that's
playing, volume and brightness as you change them, the charger, your headphones, a screenshot you just took.

It is always one pill. When two things are going on, the second rides along as a small satellite at the pill's end,
behind a hairline: a tiny flickering flame while Claude works, the spinning cover while a song plays. Click the
satellite to swap it in. Hover the pill and a card drops down. It's a brief on what your agents are doing, not a settings window.

![The pill's states](docs/pills.png)

![The card: working, just finished, a quiet day, a request](docs/cards.png)

It is one native Go binary. It talks the Wayland protocol directly and draws in software, with no toolkit and no
GPU. It wakes only when something changes. Measured on a 2018 laptop, it uses about 9 MB of private memory and about 1%
of one core while Claude works and the flame is animating, and a fraction of that when idle.

## What it shows

**Claude Code**
- **Live activity.** Each tool call becomes a short label, such as `Editing quota.go +12 −3`, `Running the store tests`
  or `Searching the web`. It also shows running subagents and context compaction. With several sessions, the busiest
  one shows and the rest count as `+N`.
- **Permission requests in the pill.** Deny, Always (applies the rule Claude suggests) or Allow, with the command or
  file shown. Several requests queue up behind each other, and the card says how many more are waiting.
- **Questions.** `AskUserQuestion` options show as rows, single or multi-select, one question after another.
- **Done.** When a session finishes while you're looking elsewhere, the pill says so with the first line of Claude's
  reply. Click it to jump to that terminal. If it stops on an error (a rate limit, say), you hear about that too.
- **Never in the way.** Claude waits at most 90 s on the island, then asks in the terminal as usual. It asks in the
  terminal at once if Ember isn't running. If Claude cancels, the request leaves the island within a second.
- **Sessions by name.** The card lists sessions with the topic Claude gave each one (read from the terminal's
  title), the current step, the diff so far and how long it has been running.

**Music.** Any MPRIS player: title, artist and cover, a progress hairline under the pill, and a card with seek,
previous, play/pause and next. Scroll on it to change the volume. The card takes its colour from the album cover.

**System moments.** They appear for a moment, then step aside:
- Volume, microphone mute and brightness, with a level bar. They replace Noctalia's OSD for those.
- Charging, unplugged, fully charged, and battery low at 20, 10 and 5 %.
- Bluetooth headphones connecting, with their battery, and disconnecting.
- Screenshots, with a preview. Click it to open the file.
- A dot while your screen is being shared or recorded.

**The card is a brief, not a window.** You glance at the island for two seconds and ask one of three things: does
anything need me, what is it doing and is it going well, what happened while I wasn't looking. The card answers
whichever is true right now, and nothing else. It has no tabs and no settings: volume and brightness speak up in the
pill when they change, and that's enough.
- **Headline.** The task in words: what you asked, if your prompt said something ("yes, do it" doesn't), else the
  topic Claude gave the session. Under it, the facts: *kinvault · 4m in · 9 steps · 3 files +58 −12 · 1 failed*.
  Click it to jump to that terminal.
- **Working.** A live timeline of the turn, newest first, on a thin rail. The current step pulses and counts its
  seconds. Earlier steps say what they changed (+15 −3) and when. A step that failed says so in red, because Claude
  Code reports tool failures to Ember.
- **Just finished.** A recap: Claude's most informative sentence, then what the turn amounted to (steps, files
  changed, the diff, failures).
- **Waiting.** Who's asking, about what, and a button that takes you there.
- **Quiet.** A greeting that knows the time, your day in one line (*7 tasks done · 2h 14m with Claude · mostly
  ember*), and the day hour by hour.
- **Then one line each** for other sessions (what they're doing, or what they just did), the song with its controls
  under your pointer, and edge when it has something for you.

**Requests** use Claude's own words as the title ("Run the store tests with the race detector") and show the command
once, syntax-tinted, with a line along the bottom that drains while the request waits. Questions have numbered
option rows, with "In terminal" tucked into the corner.

Hovers fade in on springs, numbers roll when they change, and corners are continuous (superellipse) rather than
circular arcs. Text in any script draws: styled Unicode letters fold to plain ones, and missing characters come from
a system font found through fontconfig.

## Alive

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

The colours follow Noctalia's wallpaper palette.

## Install

Requirements:
- A wlroots-style compositor with `zwlr_layer_shell_v1`. niri is first-class: focus, terminal titles and screen
  sharing come from its event stream. Hyprland and Sway can focus a session's terminal.
- Go 1.27+ to build.
- Optional: `wpctl` and `pactl` (PipeWire) for volume, UPower for the battery, BlueZ for headphones, Claude Code 2.1+.

```sh
git clone https://github.com/Tejas242/ember && cd ember
make install
```

`make install` puts the binary in `~/.local/bin` and runs `ember install`. That command:
- writes and starts `ember.service` (a systemd user service that starts with your session);
- adds Ember's hooks to `~/.claude/settings.json`, keeping a backup at `settings.json.pre-ember` and leaving your
  other hooks alone;
- writes a default config.

Running it again is safe. Restart open Claude sessions afterwards.

Then check everything:

```sh
ember doctor
```

It checks the service, the compositor protocols (blur, cursor shape), the niri stream, music, volume, brightness,
battery, Claude's hooks and the config, and says what to run for anything missing.

**Noctalia.** Leave the bar's centre empty and Ember sits there, matching the bar's height. To keep Noctalia from
showing a second volume and brightness popup, add this to `~/.config/noctalia/services.toml`:

```toml
[osd.kinds]
volume_output = false
volume_input  = false
brightness    = false
media         = false
```

**Blur.** On niri, blur what's behind the island:

```kdl
layer-rule {
    match namespace="^ember$"
    background-effect { blur true; }
}
```

## Using it

| Gesture | Does |
| --- | --- |
| Hover the pill | Drops the card. Leave and it folds back |
| Click the pill | Opens the card. On a request or a "Done", jumps to that terminal |
| Click a moment | Screenshot: opens it. Others open the card |
| Click the satellite | Swaps the second activity (the cover, the little flame) into the pill |
| Scroll on the pill | Switches what the pill shows |
| Drag the song's progress line | Follows your pointer, even past the island's edge. The seek happens on release |
| Scroll on the music row | Changes the player's volume |
| Middle-click | Play / pause |
| Right-click | Dismisses a moment, the card or a dashboard |
| Click edge's row in the card | Opens edge's dashboard. Leave it, click it or right-click to close |

Commands, for keybindings or scripts:

```
ember open                              open the card
ember close | toggle
ember next | prev                       switch what the pill shows
ember play                              play / pause
ember allow | deny | always             answer the oldest permission request
ember terminal                          hand it back to Claude's own prompt
ember answer opt:N | submit             answer the oldest question
ember dash [name]                       open or close a plugin's dashboard (edge)
```

For example, in niri:

```kdl
binds {
    Mod+Shift+Y { spawn "ember" "allow"; }
    Mod+Shift+N { spawn "ember" "deny"; }
    Mod+Shift+C { spawn "ember" "toggle"; }
}
```

## Plugins

Some of what Ember shows belongs to tools not everyone has. Those parts are plugins: compiled in, but asleep
(no watchers, no rows, no pages) until their tool is installed, and they wake within a minute of it appearing.
`[plugins.<name>] enabled = "auto" | true | false` in the config decides otherwise, and `ember doctor` says which are on.

**edge** ([edge](https://github.com/Tejas242/edge), a practice coach). Its day sits in the pill and the card, its
notes and buttons on the island, and it has a dashboard (`ember dash edge`, or click its row in the card):

- **the day**: a ring for today's tasks, whether Claude is locked, the streak's flame
- **the problem in progress**: how long you've worked on it, when the next hint unlocks
- **the week** as bars against each day's need, and your **Codeforces rating** over the band edge picks from
- **the year** as a heatmap. Hover a day for what happened
- **solo rate, retention, focus and your weakest focus topic**

It reads edge's live stats file (`$XDG_RUNTIME_DIR/edge/stats.json`) and edge's pushes over the socket. It opens with
a short choreography: the ring sweeps, the bars grow in turn, the rating line draws itself, the year ripples in and
the numbers count up. After that only the flame and the pulses move.

A tool reaches its plugin over the same socket with `{"type":"plugin","name":"<name>","payload":…}`. A new plugin
implements `Plugin` in `island/plugin.go` (detect, start, stop, messages, an optional `Dashboard`) and is listed in
`plugins()`.

## Config

`~/.config/ember/config.toml`, written by `ember install` the first time (never overwritten after). Every key is
optional and changes apply within ten seconds, with no restart. This is the whole file, with the defaults:

```toml
# Ember: changes apply within ten seconds, no restart needed.
# A value Ember can't use keeps its default; `ember doctor` says which.

clock        = "12h"  # 12h | 24h
away_after_s = 120    # idle this long and you're away: news waits and is summed up when you're back

[pill]
idle = "clock"  # what the pill shows when nothing's going on: clock | date | both

[card]
hover_delay_ms = 150  # how long the pointer rests on the pill before the card drops
steps          = 4    # timeline steps, at most (1-4; fewer when other rows need the room)
others         = 3    # other sessions listed under the story (0-3)

[moments]
done_s = 6     # how long "Done · project" stays up
osd_ms = 1600  # how long a volume or brightness level stays up

[claude]
answer          = true  # answer permission requests and questions on the island; false: only the terminal asks
wait_s          = 85    # how long a request waits on the island before the terminal asks (5-85)
settle_min      = 30    # a session silent this long is taken as stopped (a killed Claude never says so)
agent_quiet_min = 20    # the same for a subagent

[look]
opacity = 0.87  # the island's body: 1 is solid, lower lets the blur through (0.3-1)
# colours are hex, like "#72dd71"; "" follows Noctalia's theme
green   = ""    # working, done
blue    = ""    # waiting on you
red     = ""    # errors
amber   = ""    # nudges, low battery

[show]
claude      = true  # Claude Code sessions, permission requests, questions
done        = true  # "Done · project" when a session finishes and you're elsewhere
music       = true  # the playing track (MPRIS)
edge        = true  # edge's practice status, if edge runs
osd         = true  # volume, microphone and brightness changes
battery     = true  # charging, fully charged, battery low
devices     = true  # Bluetooth audio connecting and disconnecting
screenshots = true  # a preview when a screenshot is saved
privacy     = true  # a dot while your screen is being shared or recorded

# Plugins run when their tool is installed; set enabled to turn one on or off.
# [plugins.edge]
# enabled = "auto"  # auto | true | false — edge's practice status, cards and dashboard
```

A value Ember can't use, an unknown key or a stray line never stops it: that line keeps its default, and
`ember doctor` says which line and why (`config: line 12: card.steps must be a whole number from 1 to 4`). Doctor also
lists settings newer than your file, so you can copy them from here. `hover_delay_ms` at the top of the file, where
it lived before `[card]`, still works. `EMBER_AWAY_AFTER` (a Go duration, like `20s`) overrides `away_after_s` for
testing.

`[plugins.<name>]` tables belong to plugins. `enabled = "auto"` runs a plugin when its tool is installed; `true` or
`false` decides for it. `[show] edge = false` still turns edge off.

## How it works

- **Claude Code** runs `ember hook <Event>`. Reporting events are sent to the island over
  `$XDG_RUNTIME_DIR/ember.sock` and return at once. A permission request or a question holds the connection open
  until you answer, or for `[claude] wait_s` (85 s); then Claude asks in the terminal. If the hook process goes away because Claude cancelled, the island drops the card.
- **The island** is one `zwlr_layer_shell_v1` surface on the top layer. It reserves no space, so it sits over the
  bar, and takes input only inside its shape. It draws into shared memory with damage tracking. While nothing
  moves, only Ember's few pixels and the live parts (equaliser, progress) are repainted.
- **Everything else is event-driven.** MPRIS and UPower signals over D-Bus, BlueZ for devices, `pactl subscribe` for
  volume, inotify for the backlight and the screenshots folder, and niri's IPC socket for windows and casts. Nothing
  polls.

## Uninstall

```sh
make uninstall    # or: ember uninstall
```

This removes Ember's hooks from Claude's settings and the service. Your config stays.

## License

MIT
