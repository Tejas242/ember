<p align="center">
  <img src="docs/banner.png" alt="Ember, a dynamic island for Wayland: the name and tagline beside the real card, hanging from a bar, showing a Claude Code session at work with its timeline and a subagent" width="840">
</p>

<p align="center">
  <a href="https://github.com/Tejas242/ember/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/Tejas242/ember/ci.yml?branch=main&style=flat-square&label=ci"></a>
  <a href="https://go.dev"><img alt="Go" src="https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go&logoColor=white"></a>
  <a href="https://wayland.freedesktop.org"><img alt="Wayland" src="https://img.shields.io/badge/Wayland-layer--shell-FFBC00?style=flat-square&logo=wayland&logoColor=black"></a>
  <a href="https://docs.anthropic.com/en/docs/claude-code"><img alt="Claude Code" src="https://img.shields.io/badge/Claude_Code-hooks-D97757?style=flat-square&logo=claude&logoColor=white"></a>
  <img alt="No GPU" src="https://img.shields.io/badge/drawn_in-software-6E56CF?style=flat-square">
  <a href="LICENSE"><img alt="MIT" src="https://img.shields.io/badge/license-MIT-3DA639?style=flat-square"></a>
</p>

<p align="center">
  <b>A dynamic island for Wayland, made for Claude Code.</b><br>
  Watch your sessions, answer their requests, and see your music and system moments from the top of your screen.<br>
  Useful without Claude too. One native Go binary: no toolkit, no GPU, about 9 MB of memory.
</p>

<p align="center">
  <img src="docs/demo/claude.webp" alt="Claude works: the pill names the session, the card shows its live timeline, a request is allowed from the island, and it's done" width="760">
</p>

<p align="center">
  <a href="https://tejas242.github.io/ember/"><b>Website</b></a> ·
  <a href="#trust">Trust</a> ·
  <a href="#install">Install</a> ·
  <a href="#what-it-shows">What it shows</a> ·
  <a href="#using-it">Using it</a> ·
  <a href="#config">Config</a> ·
  <a href="#plugins">Plugins</a> ·
  <a href="docs/DESIGN.md">Design</a>
</p>

You run a few Claude Code sessions in terminals you can't all see. Ember answers, from the top of the screen and
without switching windows: *does anything need me, is it going well, what just happened.* Claude is still the one
that asks for permission; Ember lets you answer from up there instead of hunting for the terminal. When nothing's
going on it's a clock with a small flame that breathes, blinks and falls asleep at night. The name is two things:
Ember, the app, and Ember, the flame who lives in it.

## Trust

Ember sits next to Claude's permission prompts, so here is exactly what it does and doesn't do.

- **It approves nothing on its own.** A request is allowed only when you click Allow or Always on the island, or run
  `ember allow` / `ember always`. *Always* applies the rule Claude itself suggested, and the card says what it would
  allow. A request goes back to the terminal after `[claude] wait_s` (85 s by default), and at once if Ember isn't
  running. `[claude] answer = false` leaves every answer to the terminal.
- **The socket** is `$XDG_RUNTIME_DIR/ember.sock`, mode 0600: only your user can talk to it (that's how
  `ember allow` works, so anything running as you can too).
- **What it reads.** Claude Code's hook events (the session, its folder, each tool call and its input, your prompt,
  Claude's last message) and the process tree above a hook, to find its terminal. Your player over MPRIS. While a
  song plays, the output's audio levels (`parec`, 8 kHz mono), turned into three numbers 25 times a second: nothing
  is recorded or stored, and it stops when the music does. Volume, brightness, battery, Bluetooth, the screenshots
  folder, and niri's window and screencast events.
- **What it writes.** `ember install` writes Ember's hooks into `~/.claude/settings.json` (your other hooks stay,
  and the file as it was is kept once as `settings.json.pre-ember`), `~/.config/ember/config.toml` if there isn't
  one, and `~/.config/systemd/user/ember.service`. While it runs it keeps what it has learned (habits, your usual
  hours, today's tally) in `~/.local/state/ember`, and the sessions on show in
  `$XDG_RUNTIME_DIR/ember-sessions.json` (0600) so a restart doesn't lose them. `ember uninstall` takes out the
  hooks and the service; your config and state stay.
- **Network.** Only album art, when your player gives the cover as an `http(s)` URL (8 s timeout, 4 MiB at most,
  kept in memory). No telemetry, no updates, nothing else.
- **Plugins** are programs you put in `~/.config/ember/plugins`, and they run as your user, like anything else you
  install. They tell Ember what to show and never see or answer Claude's requests; their buttons can only do what
  their manifest declares. The three powers that reach past their own line (outranking Claude at work, a badge on
  the clock, Ember's game face) need a grant in your config.

**Display limits.** Ember draws at 1×, so on a HiDPI output with scaling it's upscaled by the compositor and looks
soft. It appears on whichever output the compositor gives it. The screen-sharing dot needs niri.

## Install

```sh
git clone https://github.com/Tejas242/ember && cd ember
make install    # the binary in ~/.local/bin, a user service, Claude Code's hooks, a config
ember doctor    # says what's missing, and how to get it
```

Without Go, take a static binary (amd64 or arm64) from [Releases](https://github.com/Tejas242/ember/releases):

```sh
curl -LO https://github.com/Tejas242/ember/releases/latest/download/ember-linux-amd64
install -Dm755 ember-linux-amd64 ~/.local/bin/ember && ~/.local/bin/ember install
```

Each release lists its `sha256sums.txt`. You need a Wayland compositor with `zwlr_layer_shell_v1` and systemd, and
Go 1.27+ only to build. Ember is made on **niri** with the **Noctalia** shell, where it gets the most (focus,
terminal titles, screen sharing, the bar's height and palette); Hyprland, Sway and river should work but are less
tested, and reports are welcome. Music needs an MPRIS player; volume and the listening equaliser PipeWire (`wpctl`,
`pactl`, `parec`); the battery UPower; headphones BlueZ. Restart open Claude sessions after installing. With another
bar, tell Ember its height (`[look] bar_height`); [docs/CONFIG.md](docs/CONFIG.md#fitting-it-to-your-desktop) has
that, Noctalia's popups and niri's blur. `ember uninstall` takes out the hooks and the service and leaves your
config.

## What it shows

**Claude Code.** The pill names the session at work and how long it has been at it; with several, the one you can't
see is named and the rest count as `+N`. Hover it and the card drops: the task in words, one line of facts, and a live
timeline hanging from Ember, with subagents branching off it. Requests arrive as Deny, Always or Allow, with Claude's
own words as the title and the whole command (it wraps; it's never cut where an `&& rm -rf` could hide), and a line
saying what *Always* would allow. Questions are a numbered list. When a session finishes where you can't see it,
the pill quotes Claude's most informative sentence, and the outcome waits by the clock until you've looked.

<p align="center">
  <img src="docs/cards.png" alt="The card: working with a subagent, three sessions, a session waiting, just finished, a quiet day, subagents opened" width="840">
</p>

<p align="center">
  <img src="docs/requests.png" alt="Requests: a command, an edit with two more waiting, a question, several answers" width="840">
</p>

<img align="right" src="docs/demo/music.webp" alt="Music: the cover in its ring, the card's now playing, pause and play, the volume" width="440">

**Music.** Any MPRIS player: the cover as a disc in the pill, and in the card on a haze of its own colours, with
controls and a scrubber you can drag. While a song plays the equaliser listens to the output, and Ember puts on
headphones and dances on the beat.

<br clear="right">

<img align="right" src="docs/demo/moments.webp" alt="Moments: charging, headphones, a screenshot, brightness, a session waiting" width="400">

**Moments.** Volume, microphone and brightness as you press the keys; charging, unplugged and battery low;
Bluetooth headphones with their battery; a screenshot with its preview (click to open it); a dot while your screen
is shared. They appear for a moment, then step aside.

<br clear="right">

<p align="center">
  <img src="docs/pills.png" alt="Every state of the pill" width="840">
</p>

## Using it

| Gesture | Does |
| --- | --- |
| Hover the pill | Drops the card. Leave and it folds back |
| Click the pill | Opens the card. On a request or a "Done", jumps to that terminal |
| Click a moment | Screenshot: opens it. Others open the card |
| Click the satellite | Swaps the second activity (the cover, the little flame) into the pill |
| Click another session in the card | Makes it the hero: its timeline opens |
| Scroll on the pill | Switches what the pill shows |
| Drag the song's progress line | Follows your pointer, even past the island's edge. The seek happens on release |
| Scroll on the music row | Changes the player's volume |
| Middle-click | Play / pause |
| Right-click | Dismisses a moment, the card or a dashboard |
| Click a plugin's row in the card | Opens its dashboard, if it has one. Leave it, click it or right-click to close |

Commands, for keybindings or scripts:

```
ember open                              open the card
ember close | toggle
ember next | prev                       switch what the pill shows
ember play                              play / pause
ember allow | deny | always             answer the oldest permission request
ember terminal                          hand it back to Claude's own prompt
ember answer opt:N | submit             answer the oldest question
ember dash [name]                       open or close a plugin's dashboard
```

For example, in niri:

```kdl
binds {
    Mod+Shift+Y { spawn "ember" "allow"; }
    Mod+Shift+N { spawn "ember" "deny"; }
    Mod+Shift+C { spawn "ember" "toggle"; }
}
```

## Config

`~/.config/ember/config.toml`, written by `ember install`. Every key is optional, changes apply within ten seconds,
and `ember doctor` says which line it couldn't use and why. The ones people change most:

| Key | Default | Does |
| --- | --- | --- |
| `clock` | `"12h"` | `12h` or `24h` |
| `away_after_s` | `120` | idle this long and you're away: news waits and is summed up when you're back |
| `[pill] idle` | `"clock"` | what the pill shows when nothing's going on: `clock`, `date` or `both` |
| `[card] hover_delay_ms` | `150` | how long the pointer rests on the pill before the card drops |
| `[claude] answer` | `true` | answer requests on the island; `false` leaves them to the terminal |
| `[claude] wait_s` | `85` | how long a request waits on the island before the terminal asks (5-85) |
| `[look] bar_height` | `0` | your bar's thickness in px; `0` asks Noctalia, else 24 |
| `[look] opacity` | `0.87` | the island's body: 1 is solid, lower lets the blur through |

`[show]` turns any part off (music, the volume popups, battery, …), and `[look]` takes your own colours. The whole file, with
every default, is in [docs/CONFIG.md](docs/CONFIG.md).

## Ember, the character

Ember is a small flame with a face, in every pill. None of it is scripted animation: it has an energy and a mood
that follow the time of day and how the work is going, a body with real physics that hops, squashes and lands, and a
handful of acts for what just happened (it catches when you send a prompt, nods when you allow, flinches when a
session stops). It blinks and glances at random, falls asleep when the desk goes quiet, knows when you've stepped
away and tells you what you missed, learns your hours, and celebrates only what was earned. The island's colours
follow Noctalia's wallpaper palette, or your own. Every token, timing and act is in [docs/DESIGN.md](docs/DESIGN.md).

## Plugins

Anything can show on the island through a plugin: any program, in any language, that tells Ember what to show in
plain JSON lines on its stdin and stdout. Ember draws it in its own style and decides when it shows; a plugin can't
draw pixels, hold the pill against Ember's judgement, or touch Claude's requests. Scripts need no plugin at all:
`ember note "Build passed" --icon check` shows a moment and `ember status deploy "Deploying" --progress 0.4` holds a
pill line. The protocol, the Go SDK and the examples (a pomodoro timer, a build notifier) are in
[docs/PLUGINS.md](docs/PLUGINS.md), with **edge**, a practice coach with its own row, nudges and dashboard, as the
worked example of a bigger one.

## How it works

- **Claude Code** runs `ember hook <Event>`. Reporting events are sent to the island over
  `$XDG_RUNTIME_DIR/ember.sock` and return at once. A permission request or a question holds the connection open
  until you answer, or for `[claude] wait_s` (85 s); then Claude asks in the terminal. If the hook process goes
  away because Claude cancelled, the island drops the card.
- **The island** is one `zwlr_layer_shell_v1` surface on the top layer. It reserves no space, so it sits over the
  bar, and takes input only inside its shape. It draws into shared memory with damage tracking. While nothing
  moves, only Ember's few pixels and the live parts (equaliser, progress) are repainted.
- **Everything else is event-driven.** MPRIS and UPower signals over D-Bus, BlueZ for devices, `pactl subscribe` for
  volume, inotify for the backlight and the screenshots folder, and niri's IPC socket for windows and casts. Nothing
  polls.

The code: `cmd/ember` is the command (install, doctor, the hook); `internal/island` is the island itself (state,
drawing, Ember); `internal/wayland` is its Wayland client; `internal/source` holds the watchers (players, battery,
volume, niri, …), each supervised and restarted on its own; `internal/plugin` hosts plugins, whose protocol and Go
SDK are `pkg/plugin`, with examples in `plugins/`; `internal/ipc`, `internal/config` and `internal/xdg` are the
socket protocol, the config file and the directories.

## More

- [Website](https://tejas242.github.io/ember/), with Ember living a day
- [docs/DESIGN.md](docs/DESIGN.md): the design, every token and timing, and Ember's mind
- [docs/CONFIG.md](docs/CONFIG.md): the whole config file
- [docs/PLUGINS.md](docs/PLUGINS.md): the plugin protocol, the Go SDK and the examples
- [CHANGELOG.md](CHANGELOG.md): what each release brought

## Credits

- [Geist and Geist Mono](https://github.com/vercel/geist-font) by Vercel, under the SIL Open Font License 1.1.
- Built for, and alongside, [Claude Code](https://docs.anthropic.com/en/docs/claude-code), whose hooks make the
  whole thing possible.

## License

[MIT](LICENSE)
