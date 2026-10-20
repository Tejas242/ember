# Ember plugins: protocol v1

A plugin is any program, in any language, that tells Ember **what** to show. Ember decides **how** it looks (in
its own style, with its own fonts and icons) and **when** it shows (its attention model: a request from Claude
always comes first, then what you pressed, then whatever matters most right now). A plugin never draws pixels,
never holds the pill against Ember's judgement, never sees or answers Claude's requests, and can't make Ember
spam: everything it sends is validated, limited and coalesced.

There are two ways in:

- **One-liners**, for scripts: `ember note` shows a moment and `ember status` holds a pill line. No plugin needed.
- **Plugins**: a long-running program Ember starts, that speaks JSON lines on its stdin and stdout.

## One-liners

```sh
ember note "Build passed" --icon check --mood done --sub "42 tests" --open https://ci.example.com/run/81
ember status deploy "Deploying" --sub "eu-west" --progress 0.4 --mood work --ttl 10m
ember status deploy --clear
```

`ember note TEXT` shows a moment for a few seconds. Flags: `--sub`, `--icon`, `--mood`, `--ttl` (seconds, up to
10) and `--open URL` (a click opens it).

`ember status ID TEXT` holds a pill line named `ID` until its `--ttl` runs out (a Go duration, default `10m`, at
most `24h`) or `ember status ID --clear`. Flags: `--sub`, `--icon`, `--mood`, `--progress 0..1`, `--since now`
(an elapsed timer from now), `--urgency low|normal|high`. Saying `ember status ID …` again replaces the line.

Both go over the island's socket and return at once; if the island isn't running they fail with exit 1, and a
bad flag fails with exit 2. `note` and `status` are the plugin protocol's `moment` and `status` messages (with,
for the one-liners only, a line's `id` and `ttl_s`, and a note's `url`), checked the same way. A new note
replaces the one showing, at most one a second (more are dropped); each status line takes ten updates a second.

## A plugin

A plugin lives in its own directory, `~/.config/ember/plugins/<name>/`, with a manifest, `plugin.toml`:

```toml
name         = "pomodoro"            # [a-z0-9-], 1–32 characters; must match the directory
description  = "A focus timer"
command      = "./pomodoro"          # relative to the plugin's directory, or a program on PATH
args         = ["--minutes", "25"]
icon         = "timer"               # its icon in `ember plugins` and on its row
capabilities = ["pill", "moment", "row", "card", "dashboard", "cheer"]   # what it may send

[targets]                            # what a click may do: only these, and only what you wrote here
docs  = "https://example.com/pomodoro"    # a string is a URL, opened with xdg-open
again = ["pomodoro-cli", "restart"]       # an array is a command, run without a shell
```

Ember starts every plugin it finds when it starts (and one that appears, within a minute), in the plugin's directory, with `EMBER_VERSION` and
`EMBER_PLUGIN_DIR` in its environment, and stops it when the island stops. `[plugins.<name>] enabled = false`
in Ember's config turns one off (see the README's Config). `ember plugins` lists them, and `ember doctor` says
how each is doing.

A plugin that exits is started again after 1 s, then 2, 4, 8… up to 5 minutes; a plugin that ran for a minute
or more starts the count over. One that exits 5 times within 10 minutes, or floods Ember (below), is
**suspended**: Ember stops it and leaves it stopped until its manifest changes or Ember restarts, and doctor says
why. Whatever a plugin showed goes away when it stops.

Its stderr is its log: the last lines are kept for `ember plugins` and doctor, and it is not shown anywhere else.

## The wire

One JSON object per line, UTF-8, on the plugin's stdin (from Ember) and stdout (to Ember), each line at most
**64 KiB**. Every object has a `type`. **Unknown types and unknown fields are ignored**, so a plugin written for
a later version of the protocol still works with this one, and the other way round. Times are Unix milliseconds.

A line that isn't a JSON object, has no `type`, has a field of the wrong JSON type, uses a capability the
manifest didn't declare, or is longer than 64 KiB is a **violation**: it is dropped and counted (see Limits).
Values that are merely too long or too many are not violations: text is cut to its limit (with an ellipsis),
lists to their count, and numbers are clamped to their range. An unknown enum value (a mood, an icon, a widget
kind) reads as its default, so new values can be added without breaking old Embers.

### The handshake

Ember speaks first, as soon as the plugin starts:

```json
{"type":"hello","v":1,"ember":"v1.4.0","caps":["pill","moment","row"]}
```

`caps` are the capabilities this plugin may use: the ones its manifest declared that this Ember knows. The
plugin answers within 5 seconds:

```json
{"type":"hello","v":1,"name":"pomodoro"}
```

`name` must be the manifest's. `v` is the highest protocol version the plugin speaks; Ember speaks v1 with any
plugin that says 1 or more, and stops one that says less, or doesn't answer, or answers with another name.
Nothing a plugin sends before its hello counts.

### Plugin → Ember

**`status`**: the plugin's line in the pill (capability `pill`). Ember shows it when it is the most salient
thing going on, beside Claude, music and the clock; it never covers a request from Claude.

```json
{"type":"status","text":"Focus","sub":"18 min left","icon":"timer","mood":"work",
 "until_ms":1792234567000,"urgency":"normal"}
```

| field | | |
| --- | --- | --- |
| `text` | string, ≤ 40 | required: the line |
| `sub` | string, ≤ 40 | secondary words after it |
| `icon` | icon | at the left |
| `mood` | mood | default `work` |
| `since_ms` | int | a timer counting up from then ("12m") |
| `until_ms` | int | a timer counting down to then ("17:59"); wins over `since_ms` |
| `progress` | number 0..1 | a meter |
| `urgency` | `low` \| `normal` \| `high` \| `critical` | how much it asks for the pill; default `normal`. `critical` needs the `critical` grant (else it reads as `high`) |
| `salience` | number 0..1 | instead of `urgency`, exactly how much: capped at 0.75, or 0.95 with the `critical` grant |
| `timer` | `auto` \| `none` \| `h:mm` | how the timer shows: `auto` ("12m" up, "17:59" down), `none` (the times only drive `warn_ms`), `h:mm` ("1:12" down) |
| `warn_ms` | int | in the last `warn_ms` before `until_ms`, the timer and `sub` turn amber |
| `value` | string, ≤ 20 | a quiet value at the right end |
| `chips` | ≤ 9 × `{label ≤ 4, state}` | letters in chips at the right end; states as the `chips` widget's |
| `badge` | icon | one small amber icon at the right end; `lock` is a padlock |
| `badge_clock` | bool | the badge also sits on the clock, when the clock shows (the `badge` grant; one plugin's at a time) |
| `satellite` | bool | when another line holds the pill, its timer shows in the pill's satellite |
| `focus` | bool | while this line holds the pill, Ember wears its game face: eyes front, no flourishes (the `focus` grant) |

A new `status` replaces the last. `{"type":"remove","what":"status"}` takes it back.

**`moment`**: a short note in the pill, the way Ember says "Screenshot saved" (capability `moment`).

```json
{"type":"moment","text":"Break time","sub":"5 minutes","icon":"check","mood":"done","ttl_s":6,"target":"docs"}
```

| field | | |
| --- | --- | --- |
| `text` | string, ≤ 40 | required |
| `sub` | string, ≤ 40 | |
| `icon` | icon | |
| `mood` | mood | default `quiet` |
| `ttl_s` | number 1..10 | how long it stays, default 5 |
| `target` | string | a `[targets]` name: a click on the moment does it, and Ember sends `click` |

**`row`**: the plugin's row in the open card, under music and edge (capability `row`).

```json
{"type":"row","title":"Focus","sub":"18 min left · 3 today","icon":"timer","mood":"work","progress":0.28}
```

| field | | |
| --- | --- | --- |
| `title` | string, ≤ 40 | required |
| `sub` | string, ≤ 60 | |
| `icon` | icon | on the card's rail |
| `mood` | mood | default `quiet` |
| `progress` | number 0..1 | a thin bar under the row |
| `target` | string | a `[targets]` name for a click on the row |
| `ring` | `{value 0..1, mood}` or `{start_ms, end_ms, mood}` | a ring on the card's rail instead of the icon: filled to `value`, or emptying from `start_ms` to `end_ms` |
| `buttons` | ≤ 2 × `{key ≤ 32, label ≤ 24, primary bool}` | small buttons at the row's end; a press sends `button` with `"card":"row"` |
| `chips` | ≤ 9 × `{label ≤ 4, state}` | letters in chips after the title |
| `badge` | icon | a small amber icon after the title (`lock`) |
| `flame` | string, ≤ 40 | the end of `sub` that a small live flame goes before ("4-day streak") |

A click on the row opens the plugin's dashboard if it has one; otherwise it does `target` (if any) and Ember
sends `click` with `"target":"row"` or the target's name. `{"type":"remove","what":"row"}` takes it back.

**`card`**: a question or an offer, with buttons (capability `card`). It drops down like Claude's requests, but
after them, and at most 3 of a plugin's are open at once (a fourth is dropped, a violation).

```json
{"type":"card","id":"break","eyebrow":"Pomodoro","title":"Take a break?","body":"25 minutes of focus done.",
 "mood":"done","ttl_s":120,"buttons":[{"key":"skip","label":"Keep going"},{"key":"break","label":"Break","primary":true}]}
```

| field | | |
| --- | --- | --- |
| `id` | string, ≤ 64 | required: a card with the same id replaces it |
| `eyebrow` | string, ≤ 32 | the small capitals above; default the plugin's name |
| `title` | string, ≤ 60 | required |
| `body` | string, ≤ 280 | |
| `mood` | mood | default `work` |
| `buttons` | ≤ 3 × `{key ≤ 32, label ≤ 24, primary bool}` | a pick sends `button` and closes the card |
| `ttl_s` | number 5..600 | default 180 with buttons, 7 without |
| `urgent` | bool | before Claude's own requests (the `critical` grant) |

`{"type":"remove","what":"card","id":"break"}` takes it back. You can always close one yourself (right-click).

**`dashboard`**: the plugin's page, opened from its row or with `ember dash <name>` (capability `dashboard`).
The plugin declares widgets; Ember lays them out and draws them in its own language: sections under small
capitals, divided by hairlines.

```json
{"type":"dashboard","title":"Pomodoro","widgets":[
  {"kind":"stats","items":[{"label":"Today","value":"3","caption":"of 8"},{"label":"Streak","value":"5 days"}]},
  {"kind":"sparkline","title":"This week","values":[2,4,3,6,5,0,3],"caption":"focus sessions a day"},
  {"kind":"bars","title":"By project","items":[{"label":"ember","value":5},{"label":"notes","value":2}]},
  {"kind":"chips","title":"Today","items":[{"label":"1","state":"done"},{"label":"2","state":"done"},{"label":"3","state":"todo"}]},
  {"kind":"list","title":"Recent","items":[{"title":"Plugin host","sub":"25 min · 10:40","icon":"check","mood":"done"}]}
]}
```

| widget | fields |
| --- | --- |
| `stats` | `title`, `items`: ≤ 4 × `{label ≤ 20, value ≤ 12, caption ≤ 24}`: big numbers in a row |
| `sparkline` | `title`, `values`: ≤ 120 numbers, `caption ≤ 40`: a line, oldest first |
| `bars` | `title`, `items`: ≤ 12 × `{label ≤ 20, value number ≥ 0, caption ≤ 12}`: horizontal bars, longest first as given |
| `chips` | `title`, `items`: ≤ 12 × `{label ≤ 4, state}`: letters in chips; `state` is `done` (filled green), `tried` (ringed red), `owed` (ringed amber), `fixed` (filled blue), `gone` (struck), or `todo` (quiet, the default) |
| `list` | `title`, `items`: ≤ 8 × `{title ≤ 40, sub ≤ 60, icon, mood}` |

`title` is ≤ 32 for every widget and the dashboard (default the plugin's name); at most 12 widgets. A new
`dashboard` replaces the last; Ember sends `dash` when it opens or closes, so a plugin can send updates only
while it's open. `{"type":"remove","what":"dashboard"}` takes it back.

**`cheer`**: asks Ember to react (capability `cheer`): `{"type":"cheer","kind":"hop"}`, `nod` or `celebrate`.
`hop` and `nod` are reactions to something that just happened (a test passed, a letter solved): at most one
every 5 seconds per plugin, and never while something waits on you. `celebrate` is for something earned (a
milestone): at most once an hour per plugin. Neither is a flourish, and Ember may still let one go.

**`log`**: `{"type":"log","level":"info","text":"synced 3 sessions"}`, `level` `info` | `warn` | `error`, text
≤ 500. Kept with the plugin's stderr for `ember plugins` and doctor.

**`remove`**: `{"type":"remove","what":"status"}`: `what` is `status`, `row`, `dashboard`, or `card` with its
`id`. Removing what isn't there is fine.

### Ember → plugin

| message | when |
| --- | --- |
| `{"type":"hello","v":1,"ember":"v1.4.0","caps":[…]}` | first, always |
| `{"type":"click","target":"docs"}` | a click on its moment or row: the target's name, or `"row"` for a row without one |
| `{"type":"button","card":"break","key":"break"}` | a button on its card; the card has closed |
| `{"type":"dash","state":"open"}` | its dashboard opened (`"close"`: it closed) |
| `{"type":"tool","payload":{…}}` | a message from the plugin's own tool (below) |
| `{"type":"stop"}` | the island is stopping: exit within 2 seconds (then SIGTERM, then SIGKILL) |

A plugin must keep reading its stdin: a plugin that stops reading may miss events, but never blocks Ember.

**A tool's messages.** A plugin often fronts a tool that has its own daemon. The tool can reach its plugin
through Ember's socket, `$XDG_RUNTIME_DIR/ember.sock`, with one line, `{"type":"plugin","name":"<plugin>",
"payload":{…}}`; the plugin gets the payload as a `tool` event. (edge's older `{"type":"edge","payload":…}` is
the same, for the plugin named edge.) A payload is at most 60 KiB.

### Grants

Three things a plugin can do reach past its own line, so the user grants them, in Ember's config:

```toml
[plugins.edge]
grants = ["critical", "badge", "focus"]
```

`critical` lets its line outrank Claude at work (never a request waiting on you) and its cards come first;
`badge` lets its badge sit on the clock; `focus` lets its line set Ember's game face. Without the grant the
field is ignored (`critical` reads as `high`). `ember plugins` and doctor show each plugin's grants. A
manifest may ask with `grants = [...]`; that is a request shown in `ember plugins`, never a grant.

### Vocabulary

**Moods**: `work` (at it: Ember's working colour), `wait` (needs you: blue), `nudge` (worth a look: amber),
`done` (green), `fail` (red), `quiet` (muted).

**Icons**: `check`, `warning`, `bolt`, `timer`, `chart`, `note`, `read`, `edit`, `run`, `search`, `web`,
`agent`, `camera`, `screen`, `cast`, `headphones`, `bluetooth`, `battery`, `unplug`, `sun`, `volume`, `mic`,
`lock`, `other`. An unknown name shows no icon.

### Limits

| what | limit | over it |
| --- | --- | --- |
| a line | 64 KiB | violation |
| `status`, `row` | 10 a second | coalesced: the latest wins |
| `dashboard` | 4 a second | coalesced: the latest wins |
| `moment` | 1 every 10 seconds | dropped |
| `card` | 3 open, 1 new a second | dropped, a violation |
| `cheer` | `hop`/`nod` 1 every 5 seconds, `celebrate` 1 an hour | dropped |
| `log` | 5 a second | dropped |
| everything | 100 messages a second for 3 seconds | suspended |
| violations | 50 within a minute | suspended |
| exits | 5 within 10 minutes | suspended |

A plugin over a rate is throttled, quietly: what was dropped is counted in `ember plugins`. Ember stays fast
however a plugin behaves: reading, validating and coalescing happen off the island's loop, which applies at most
one update per plugin per frame.

## The Go SDK

`github.com/Tejas242/ember/pkg/plugin` is the protocol in Go: the messages as types (with the limits Ember
applies), `Run` (the handshake, then your function, with Ember's events on `p.Events()` until it says stop),
helpers to build widgets, and `Harness`, a fake Ember for tests: it checks every line the way Ember does and
sends events.

```go
func main() {
	err := plugin.Run("pomodoro", func(p *plugin.Plugin) error {
		end := time.Now().Add(25 * time.Minute)
		p.Status(plugin.Status{Text: "Focus", Icon: "timer", Mood: plugin.Work, UntilMs: end.UnixMilli()})
		for ev := range p.Events() { // closed when Ember says stop
			p.Logf("%s %s", ev.Type, ev.Target+ev.Key)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err) // stderr is the plugin's log
	}
}
```

```go
h := plugin.NewHarness("pomodoro", plugin.Caps, run)
st, err := h.Wait("status", time.Second)
h.Button("break", "break")
err = h.Stop()
```

## Examples

- `plugins/pomodoro` (Go): focus sessions and breaks. A countdown in the pill, a row with its progress (a click
  starts a session), a card offering the break, a moment at each end, today's sessions on its dashboard, and a
  celebration when the day's goal is met. Install it with
  `go build -o ~/.config/ember/plugins/pomodoro/pomodoro ./plugins/pomodoro` and
  `cp plugins/pomodoro/plugin.toml ~/.config/ember/plugins/pomodoro/`.
- `plugins/notify-build` (shell, no plugin needed): `notify-build make test` holds a pill line while the build
  runs and leaves a note saying how it went. It is `ember status` and `ember note`, and nothing else.

## The schema

`docs/plugin-protocol.schema.json` is a JSON Schema (2020-12) of every message both ways, with the limits.
