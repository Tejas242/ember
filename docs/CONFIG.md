# Configuring Ember

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

[moments]
osd_ms = 1600  # how long a volume or brightness level stays up

[claude]
settle_min      = 30    # a session silent this long is taken as stopped (a killed Claude never says so)
agent_quiet_min = 20    # the same for a subagent

[look]
bar_height = 0  # your bar's thickness in px, so the pill sits on its line; 0 asks Noctalia, else 24 (16-64)

[ember]
motion = "full"  # full | calm: calm keeps the colours but drops hops, sparks, bands of light, the breathing halo, the little things it does on its own and the dance (reduced motion, and less CPU)
dance  = true    # dance to your music, in headphones

[show]
claude      = true  # Claude Code sessions: who is working, who asks, who is done
done        = true  # "Done · project" when a session finishes and you're elsewhere
music       = true  # the playing track (MPRIS)
edge        = true  # edge's practice status, if edge runs
osd         = true  # volume, microphone and brightness changes
battery     = true  # charging, fully charged, battery low
devices     = true  # Bluetooth audio connecting and disconnecting
screenshots = true  # a preview when a screenshot is saved

[privacy]
dot      = true  # a dot while your screen is being shared or recorded (once [show] privacy, still read)

[vibe]
lyrics = true  # click Ember in the card while a song plays: its words, from an .lrc beside the file, a cache, or LRCLIB; false: no network

# Plugins run when their tool is installed (edge) or when they're in
# ~/.config/ember/plugins (see docs/PLUGINS.md); ember plugins lists them.
# [plugins]
# enabled = "all"   # all | none | ["pomodoro", "edge"]: only these
# [plugins.edge]
# enabled = "auto"  # auto | true | false — edge's practice status, cards and dashboard
[plugins.vibe]
grants = ["media", "entry"]  # vibe hears the song only while its stage is open; a click on Ember opens it
```

A value Ember can't use, an unknown key or a stray line never stops it: that line keeps its default, and
`ember doctor` says which line and why (`config: line 12: show.music must be true or false`). Doctor also
lists settings newer than your file, so you can copy them from here. Keys Ember 1.3 retired (`[claude] answer` and
`wait_s`, `[card] steps` and `others`, `[moments] done_s`, `[look]` opacity and colours, `[ember] flourishes` and
`celebrate`, `[privacy] discreet`) do nothing now; doctor says so and the file still loads. `hover_delay_ms` at the top of the file, where
it lived before `[card]`, still works. `EMBER_AWAY_AFTER` (a Go duration, like `20s`) overrides `away_after_s` for
testing.

`[ember]` is about the flame. `motion = "calm"` is reduced motion: Ember keeps its colours and its face but no
longer hops or throws sparks, bands of light don't cross the pill, a request waiting on you doesn't breathe a halo,
what it does on its own (the morning stretch, a yawn near your bedtime, the eye-roll, hello) stays quiet, and it
doesn't dance. It also costs less CPU, since fewer frames are drawn. `dance` turns just the dance off.

`[privacy]` is for when someone can see your screen (niri tells Ember when it's being cast or recorded). `dot`
shows a dot while it is; it used to be `[show] privacy`, which still works. While the screen is shared Ember is
always discreet: a Done says only "Done kinvault", a screenshot shows the camera instead of its preview, and the card
opens when you click, not when the pointer passes.

`[vibe]` is the stage you get by clicking Ember in the card while a song plays (the vibe plugin). Its words come
from an `.lrc` file beside the song, then a cache in `~/.cache/ember/lyrics`, then LRCLIB (lrclib.net), asked only
while the stage is open and only for the song playing. `lyrics = false` keeps it to the first two: nothing leaves
your machine. The stage still dances, and shows the song's title over its ribbon.

`[plugins] enabled` says which plugins may run at all: `"all"` (the default), `"none"`, or a list of names.
`[plugins.<name>]` tables belong to plugins. `enabled = "auto"` runs a plugin when its tool is installed (a plugin in
`~/.config/ember/plugins` always is); `true` or `false` decides for it. `[show] edge = false` still turns edge off.
Anyone can write a plugin, in any language: [PLUGINS.md](PLUGINS.md). `ember plugins` says how each is
doing.

## Fitting it to your desktop

**Your bar.** Ember hangs from the top edge, over the middle of whatever bar you use. Leave the bar's centre empty
and tell Ember its height with `[look] bar_height` (Waybar, say, at 30 px: `bar_height = 30`), so the pill sits on
its line.

**Noctalia.** Ember reads the bar's height from Noctalia itself and follows its wallpaper palette. To keep Noctalia from
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

## Claude as a counter, and its limits

The pill shows Claude as a counter: Ember, the session you aren't watching, one bead per session (light and pulsing
while it works, blue when it asks, green when it finished and you haven't looked, red when it stopped, a hollow ring
when idle; four at most, then `+n`), the agents as a number, and the time. A new ask grows the island to two lines
for five seconds, with the question (cut at 52 characters); then it settles to *jobwatch asks*. A finish stays as
*kinvault done* until you focus that terminal, or for 20 minutes, and then lives in the card as *done 14m ago*.
Clicking a pill about an ask or a finish takes you to that terminal, and so does a session's row in the card.

`ember go` does the same from a key: the oldest ask, else the newest finish you haven't seen, else the newest
session working; Ember shakes its head if there is none. In niri: `Mod+G { spawn "ember" "go"; }`.

**Limits.** At 80% of the 5 hour or 7 day limit the card gets a last line, *Claude · 5h 84% · resets 4:10 PM*
(amber, red at 100%). It reads `$XDG_RUNTIME_DIR/ember/limits.json` every ten seconds, which your Claude Code status
line writes. Ember never edits your status line. Add this one line at its end (it uses the `$p5 $r5 $p7 $r7`
variables the status line already has; `ember doctor` prints it too):

```bash
[ "$p5" != "-" ] && { d="$XDG_RUNTIME_DIR/ember"; mkdir -p "$d"; printf '{"h5":%s,"h5_at":%s,"d7":%s,"d7_at":%s,"t":%s}\n' "$p5" "$r5" "${p7/-/null}" "${r7/-/null}" "$(date +%s)" >"$d/limits.tmp" && mv "$d/limits.tmp" "$d/limits.json"; }
```

Or, if you'd rather not touch the script, pipe the status line's JSON to `ember limits` (a Go process every few
seconds per session, which is why the shell line is better). The numbers only move while a session is open, so Ember
hides the line once its reset time has passed, and adds *as of 2:02 PM* when the file is over 15 minutes old. When a
limit you hit (100%) resets, the pill says *Claude's back* for four seconds.
