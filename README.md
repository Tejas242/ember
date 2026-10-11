# Ember

A live Claude Code island for your [Noctalia](https://noctalia.dev) bar. A small flame sits in the bar. When Claude
works, the bar **morphs** into what it is doing (`Editing repair.go +5 −2`). When Claude needs you, it morphs into the
question itself: **Deny · Always · Allow** right in the bar, or a panel of choices for `AskUserQuestion`.

## Plugin

| Field | Value |
| --- | --- |
| ID | `screenager/ember` |
| Entries | Bar widget: `island`; panel: `card`; service: `core` |

## What it does

- **Live activity.** Each tool call becomes a short label: reading, editing (with `+added −removed` lines), running a
  command, searching, delegating. With several sessions, the busiest one shows and the rest count as `+N`.
- **Approve from the bar.** A permission request turns the pill into the command and three buttons. Click the pill for
  the full request in a panel. **Always** applies the rule Claude suggests.
- **Answer questions.** `AskUserQuestion` opens a panel with the options (single or multi-select, several questions in a
  row). "Reply in terminal" hands the question back to Claude's own prompt.
- **Done.** When a session finishes and you aren't looking at its terminal, the pill says so for a few seconds. Click it
  to jump to the terminal (niri, Hyprland, Sway).
- **Ember.** A small flame drawn from your palette as SVG. It flickers while Claude works, looks up when it needs you,
  and smiles when it's done. It follows your wallpaper theme.
- **Never in the way.** Claude only waits on the bar for permissions and questions, at most 90 s. After that, or at once
  when Noctalia or the plugin isn't running, Claude asks in the terminal as usual. If Claude cancels, the request leaves
  the bar within a second.

## Requirements

- Noctalia 5.2 or newer (plugin API 31).
- Claude Code 2.1 or newer.
- `sh`, plus `python3` for the one-time hook installer only.
- Optional: `niri`, `hyprctl` or `swaymsg` to jump to a session's terminal.

## Install

1. Enable **Ember** in **Settings → Plugins**, then add the **island** widget to a bar. Bar center works best.
2. Connect Claude Code:

   ```sh
   sh ~/.local/share/noctalia/plugins/ember/hook install
   # community install path:
   sh ~/.local/state/noctalia/plugins/materialized/community/ember/hook install
   ```

   This backs up `~/.claude/settings.json` to `settings.json.pre-ember`, adds Ember's hooks, and leaves your other
   hooks alone. Running it again is safe. Restart running Claude sessions afterwards.

## Keyboard

Answer the oldest request without touching the mouse, for example in niri:

```kdl
binds {
    Mod+Shift+Y { spawn "noctalia" "msg" "plugin" "screenager/ember:core" "all" "allow"; }
    Mod+Shift+N { spawn "noctalia" "msg" "plugin" "screenager/ember:core" "all" "deny"; }
    Mod+Shift+C { spawn "noctalia" "msg" "panel-toggle" "screenager/ember:card"; }
}
```

IPC events on `screenager/ember:core`, all with target `all`:

| Event | Payload | Does |
| --- | --- | --- |
| `allow` / `deny` / `always` / `terminal` | none | Answers the oldest permission request |
| `answer` | `opt:N`, `submit`, `terminal`, `allow`, … | Answers the oldest request of either kind |
| `panel` | none | Toggles the panel |
| `edge` | JSON | Status and cards from [edge](#edge) |

## Mouse

| Gesture | Does |
| --- | --- |
| Click the pill | Opens the panel: the pending request, or sessions and their state |
| Click a session in the panel | Focuses its terminal |
| Right-click | Dismisses a "done" note |

## edge

[edge](https://github.com/Tejas242/edge) is a learning daemon that can lock Claude until you've practised. When it runs,
Ember shows its day in the bar: today's problem with a **Start** button and a timer, hints as they unlock, verdicts, and
a lock while practice is due. Without edge, none of this appears.

## How it works

Claude Code runs `hook <Event>`, a POSIX `sh` script. The script writes the event to `$XDG_RUNTIME_DIR/ember/` and pokes
the `core` service with `noctalia msg plugin … hook`. Reporting events don't wait. For a permission request or a
question, the hook polls for an `.answer` file that the service writes when you click, and `core` drops the request as
soon as the hook's process is gone. The bar widget renders `core`'s single published view, and skips frames that
haven't changed.

## License

MIT
