# Contributing to Ember

Thanks for looking. Ember is one static Go binary that draws everything itself, so most changes are plain Go with a
picture to check. Bug reports with `ember doctor`'s output, and reports from compositors other than niri, help as
much as code.

## Build and test

You need Go 1.27+ (see `go.mod`) and a Linux machine; nothing else to build.

```sh
make build     # ./ember, static (CGO_ENABLED=0), stamped with git describe
make test      # go vet and go test
make check     # everything CI checks: gofmt, vet, staticcheck (with and without the render tag), tests, the race detector
```

Run `make check` before every commit. The packages that touch the machine (`cmd/ember`, `internal/island`,
`internal/source`, `internal/plugin`) run their tests with `HOME` and the XDG directories in a temp sandbox
(`internal/testenv`), so no test reads or writes your real config, Claude's settings or the running island. Keep it
that way: a new package whose tests touch files, sockets or the session bus uses `testenv.Main` as its `TestMain`.

`make install` installs into `~/.local/bin` and sets up the service and Claude's hooks on your machine: use it when
you mean to run your build, not to test it.

## Pictures: the render tag

The renderer is checked by eye as well as by tests. The tools that draw it are tests behind the `render` build tag,
so `go test ./...` skips them:

```sh
make gallery   # every state of the island over a made-up desktop, as stills, in ./preview
make preview   # every state on its own, and filmstrips
make docs      # the README's stills and animations (needs ffmpeg with libwebp) and the banner, drawn with plugins off
```

A change that shouldn't alter the look should leave `make gallery`'s PNGs byte-identical: render before and after
into two directories and `cmp` them. A change that does alter it should say so in its pull request, with a before
and after.

## The performance budget

Ember sits on screen all day, so it has a budget: steady frames in the tens to low hundreds of µs, at most 1.5% of a
core while Claude works, no full repaint while nothing changes, and under 16 MiB of memory (see the Cost section of
[docs/DESIGN.md](docs/DESIGN.md)). For anything on the drawing path, run the benchmarks before and after:

```sh
go test -run xxx -bench 'PillSteady|BriefSteady|FlickerFrame|PillFull' -benchtime 2s -count 5 ./internal/island
```

and put both sets of numbers in the pull request. A regression over 5% needs a reason.

## Where things live

`cmd/ember` is the command; `internal/island` the island (state, drawing, Ember); `internal/wayland` its Wayland
client; `internal/source` the watchers; `internal/plugin` the plugin host; `pkg/plugin` the plugin protocol and Go
SDK; `plugins/` the example plugins. [docs/DESIGN.md](docs/DESIGN.md) is the design every visual change follows.

## Plugins

A plugin is a separate program that speaks JSON lines; the protocol is [docs/PLUGINS.md](docs/PLUGINS.md), with
its JSON Schema in `docs/plugin-protocol.schema.json`. Write a plugin against that, in any language; you don't need
to change Ember. A change to the protocol itself updates the doc, the schema and `pkg/plugin` together, and stays
backward compatible within v1.

## Commits

One change per commit, and the message is one line: `area: what changed`, with a lowercase area (`island`,
`plugins`, `docs`, `install`, …), no body.

```
island: a request's countdown runs on the rim instead of the footer
```
