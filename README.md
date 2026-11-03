# archmap

Interactive, function-level architecture map of a repository: modules, the call
graph between functions, entry points (HTTP, gRPC, tasks, `main`), what the
current branch changes against its base, and replayable flows. The result is one
self-contained, offline HTML file. Written in Go; syntax is parsed with
[tree-sitter](https://tree-sitter.github.io/).

![screenshot](docs/screenshot.png)
<!-- TODO: add a screenshot -->

## Install

```sh
go install github.com/aidarbn/archmap/cmd/archmap@latest
```

A C compiler is required (tree-sitter grammars are built with cgo).

## Commands

```sh
archmap extract [--root DIR] [--base REF] [--tests] --out facts.json
archmap render  --facts facts.json [--model arch.model.json] --out arch.html
archmap map     [--root DIR] [--base REF] [--tests] [--model FILE] [--out FILE] [--open]
archmap serve   [--root DIR] [--base REF] [--tests] [--model FILE] [--addr 127.0.0.1:0] [--open]
archmap version
```

- `extract` walks the repo (skipping `vendor`, `node_modules`, generated code and,
  unless `--tests`, tests), parses functions and calls, and writes `facts.json`.
  A short summary goes to stdout.
- `--base REF` is the ref to compare against; the default is the merge-base with
  the default branch. Uncommitted changes are included and marked.
- `render` merges facts and the optional model into one HTML file.
- `map` does both; the default output is `<root>/.archmap/arch.html`, and
  `<root>/arch.model.json` is picked up when present.
- `serve` is described below.

## serve: live map while you code

```sh
archmap serve [--root DIR] [--base REF] [--tests] [--model FILE] [--addr 127.0.0.1:0] [--open]
```

Prints a `http://127.0.0.1:PORT/` URL (`--open` opens it). The page is the same
viewer as `map`, but it follows the repo:

- The server polls file mtimes and sizes (every 400 ms) for source files, the
  model file and `.git` HEAD/index/branch ref, waits until the tree has been
  quiet for 300 ms, then re-extracts and pushes an update over SSE (`/events`);
  the page reloads `/data.json`. Polling was chosen over fsnotify: no
  dependency and no inotify/kqueue watch limits on big trees.
- A rebuild re-parses only files whose content changed; resolving calls and
  imports is redone in full. A failed rebuild keeps the last good map and shows
  the error.
- The browser keeps your selected function, opened groups, map position, search,
  filters and playback; functions that are new or whose body (or change status)
  changed flash on the map and in the lists. The header shows
  live / rebuilding / error.
- Only loopback addresses are accepted for `--addr`; requests with a foreign
  `Host` header and non-GET methods are rejected.
- `/` renders the same page as `map` plus a `serve` key in the data; a static
  `arch.html` is unchanged and works offline as before.

Limits: the watcher ignores `.gitignore` (an ignored source file that changes
causes a harmless rebuild); a change in a file's callers only shows as a flash
on the edited function, not on its callers.

## arch.model.json

The optional semantic layer on top of the facts: titles, module roles and
layers (`entry|domain|adapter|infra|ui|test|other`) and named flows, where each
step is an existing function id (`path/file.ext::Class.method`). See
[examples/arch.model.json](examples/arch.model.json).

## Limitations

- Calls are resolved by name and receiver, which is an approximation: expect
  some extra or missing edges. Common method names (`get`, `Close`, ...) are not
  linked.
- Language support is in progress: Python, Go, JS/TS, Swift and Kotlin parsers
  plug into `internal/lang`; a language without an implemented parser
  contributes nothing to the map yet.
- Change status needs git.

## License

MIT
