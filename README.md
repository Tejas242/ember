# Ember

A dynamic island for the top of a Wayland desktop. It sits over the centre of your bar as a small pill with a flame
in it, and morphs to show what is going on: what Claude Code is doing and what it needs from you, the song that's
playing, volume and brightness as you change them, the charger, your headphones, a screenshot you just took.

Hover it and a card drops down with the details and the controls.

![The pill's states](docs/pills.png)

![The card: music, system, a permission request, a question](docs/cards.png)

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

**System page.** Volume and brightness sliders (click or scroll), mute, and the battery with time to empty or full.

**The clock**, when nothing else is going on. And [edge](https://github.com/Tejas242/edge)'s practice status, if
you run it.

Ember the flame has moods. It flickers while Claude works, looks up when it needs you, smiles and bounces when work is
done, and breathes when idle. Its eyes follow your pointer. The colours follow Noctalia's wallpaper palette.

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
| Click a moment | Screenshot: opens it. Volume, brightness, battery: opens the System page |
| Click the bubble | Brings what the bubble shows (music beside Claude, say) to the pill |
| Scroll on the pill | Switches what the pill shows |
| Scroll on the card | Switches pages. On a slider or the music controls, changes the level |
| Middle-click | Play / pause |
| Right-click | Dismisses a moment or the card |

Commands, for keybindings or scripts:

```
ember open [claude|music|edge|system]   open the card, on a page
ember close | toggle
ember next | prev                       switch what the pill shows
ember play                              play / pause
ember allow | deny | always             answer the oldest permission request
ember terminal                          hand it back to Claude's own prompt
ember answer opt:N | submit             answer the oldest question
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

`~/.config/ember/config.toml`. Changes apply within a few seconds, with no restart.

```toml
clock          = "12h"   # 12h | 24h
hover_delay_ms = 240

[show]
claude      = true  # sessions, permission requests, questions
done        = true  # "Done · project" when a session finishes and you're elsewhere
music       = true
edge        = true
osd         = true  # volume, microphone, brightness
battery     = true  # charging, fully charged, battery low
devices     = true  # Bluetooth audio
screenshots = true
privacy     = true  # the screen-sharing dot
```

## How it works

- **Claude Code** runs `ember hook <Event>`. Reporting events are sent to the island over
  `$XDG_RUNTIME_DIR/ember.sock` and return at once. A permission request or a question holds the connection open
  until you answer, or 90 s pass. If the hook process goes away because Claude cancelled, the island drops the card.
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
