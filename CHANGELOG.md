# Changelog

Each release's section here becomes its notes on GitHub. Versions follow [semver](https://semver.org): the config
file, the commands and the plugin protocol are the public surface.

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
