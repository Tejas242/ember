# Changelog

Each release's section here becomes its notes on GitHub. Versions follow [semver](https://semver.org): the config
file, the commands and the plugin protocol are the public surface.

## Unreleased

**Ember, more natural**
- It dances on the beat: its knees bend into each beat (it used to land about 80 ms late), its weight steps from foot
  to foot, the first beat of a bar lands hardest, and its flame tip and headphones trail its head. It picks a sway,
  a groove, a head-bob or a bounce from how hard this part of the song goes, and blends between them.
- Its face moves like a face: a change of expression happens behind a blink, the eyes jump and hold, blinks land on
  beats and new lines, and each phrase of a song gets its own face (lost in it, grooving, beaming, now and then a
  wink).
- On the stage it sings the words. Ember listens for the singer in the middle of the stereo picture, finds where each
  syllable starts, and shapes its mouth for it: lips closing on m, b and p, teeth on f and v, the tongue on l and th,
  wide on "ah", round on "oh" and "oo", flowing from one to the next and holding a held note. It reads English,
  Hindi in Devanagari and in Latin letters, and has a go at Arabic and Japanese kana.
- When a song's words are early or late, Ember hears it after a few lines and moves them itself, and remembers it
  for the next play. A nudge you give the words (scroll over them) always wins.
- Its idle breath no longer jumps when its mood changes.

**Vibe**
- Click Ember in the card while a song plays (or the headphones in the music row, or `ember dash vibe`) and the card
  becomes a stage: Ember dances big on a floor of the cover's light, on its own beat, and sings along with the song's
  words, lit as they're sung. Claude's session stays in the top line; a request replaces the stage at once; when
  Claude finishes, the stage folds itself at the end of the song; paused two minutes, it closes.
- Every case has its look: a break in the words, a very long line, words without times, no words, an instrumental, a
  new song, a talk, an ad, paused, Claude done or stopped, the screen shared, calm motion, late at night.
- The words come from an `.lrc` beside the file, a cache in `~/.cache/ember/lyrics`, or LRCLIB, asked only while the
  stage is open; `[vibe] lyrics = false` keeps it offline. Scroll over the words to move them a quarter second.
- It's a plugin (`plugins/vibe`, built in as `ember plugin vibe`); `ember install` writes it and grants it `media`
  and `entry`.
- Words in scripts that join, reorder or run right to left (Hindi and other Indic scripts, Arabic, Hebrew) are shaped
  in their own script's font; right-to-left lines sit right and light up from the right, a mixed line in reading order.
- A player whose playhead never moves (some browsers) is noticed after four seconds: its words page by how long the
  song has played, and say so, until the playhead moves again.

**Plugin protocol** (backward compatible)
- Stages: the `stage`, `timed_text`, `media`, `sessions`, `audio` and `countdown` widgets.
- Pages may be `size: "card"`, `sticky`, and say when they `ends`; a `dash` close says why (`reason`).
- `media` events (the `media` grant): the song playing, only while the plugin's page is open.
- `[opens] ember = "music" | "idle"` in a manifest (the `entry` grant): a click on Ember in the card opens its page.
- Ember's hello says the plugin's grants; the Go SDK has `Plugin.Granted` and the harness `Media` and `DashClosed`.

## 1.0.2

- Ember's springs step in pieces no longer than a 60th of a second. At the card's 24 fps tick the old step was
  unstable: a pixel of error rang up until Ember flew off the island (or the island panicked), and the instability
  drove it into bursts of 60 fps frames.
- A missing glyph's fallback font tries fontconfig's candidates until one parses and has it, so Chinese, Japanese
  and Korean text no longer draws as boxes where the first candidate is a .ttc Ember can't read.

## 1.0.1

Built with Go 1.27.3: 1.0.0's binaries were built with Go 1.27.1, whose standard library has known vulnerabilities in
`net/http` (HTTP/2) and `crypto/x509` that Ember's album-art download could reach. Nothing else changes.
staticcheck moves to v0.8.2, which reads the newer toolchain.

## 1.0.0

The first release: everything Ember does today, in one static binary for Linux on amd64 and arm64.

**Claude Code**
- 15 hooks installed into `~/.claude/settings.json` (the file as it was kept once as `settings.json.pre-ember`;
  other tools' hooks left alone), recognised under any binary name so a reinstall replaces stale ones.
- The pill names the session at work and how long it has been at it; several sessions show the one you can't see,
  and the rest as `+N`. A session gone quiet turns amber; one silent for half an hour settles to idle.
- The card: the hero session's task, its facts and a live timeline hanging from Ember, with failed steps and
  subagents on a branch; every other session as one line; click one to make it the hero.
- Permission requests and questions answered on the island (Deny, Always, Allow, numbered options) or with
  `ember allow | deny | always | answer`. The whole command is shown, and what *Always* would allow. A request goes
  back to the terminal after `[claude] wait_s` (85 s), at once if Ember isn't running.
- Done and Stopped in words, quoting Claude's most informative sentence; outcomes you missed wait by the clock, and
  a line sums up what happened while you were away.

**Music and the desktop**
- Any MPRIS player: the cover in a ring in the pill, now playing on a haze of the cover's colours, controls, a
  scrubber you can drag, and an equaliser that listens to the output (8 kHz, nothing stored).
- Moments: volume, microphone and brightness, charging and battery low, Bluetooth headphones with their battery,
  screenshots with a preview, a dot while the screen is shared (niri).
- niri is first-class (focus, terminal titles, screencasts); Hyprland and Sway can focus a session's terminal.
  Noctalia's bar height and wallpaper palette are followed; `[look]` covers other bars and your own colours.

**Ember, the flame**
- A character with a pixel face, a physical body, acts for every moment, an energy and mood over the day, sleep,
  presence, habits, a dance that keeps the song's beat, and a gate on its flourishes.

**Plugins**
- Protocol v1 (JSON lines, `docs/PLUGINS.md` and its JSON Schema), a host with limits, coalescing, backoff and
  suspension, grants for the powers that reach past a plugin's own line, the Go SDK and test harness in
  `pkg/plugin`, and two examples: pomodoro (Go) and notify-build (shell).
- `ember note` and `ember status` for scripts; `ember plugins` and `ember dash`.

**Tools**
- `ember install` (a systemd user service, the hooks, a default config), `ember doctor` (the service, protocols,
  watchers, hooks, config lines, plugins, and whether the running island is this binary), `ember demo` (a Claude
  session played on the island), `ember uninstall [--purge]`.
- A config that applies within ten seconds and never stops Ember over a bad line; every key in `docs/CONFIG.md`.
- About 9 MB of memory and ≈1% of a core while Claude works, drawn in software with damage tracking; nothing polls.
