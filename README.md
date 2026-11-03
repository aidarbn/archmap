# archmap

archmap builds an interactive, function-level architecture map of a repository:
modules, the call graph between functions, entry points (HTTP, gRPC, tasks,
`main`), what the current branch changes against its base, and replayable
flows. The result is one self-contained, offline HTML file (or a live page that
follows your edits with `archmap serve`). Written in Go; 20 languages are parsed
with [tree-sitter](https://tree-sitter.github.io/).

![archmap map of ripgrep](docs/img/map-ripgrep.png)
*`archmap map` on [ripgrep](https://github.com/BurntSushi/ripgrep): modules grouped by dependency level.*

![archmap serve on archmap itself](docs/img/serve-archmap.png)
*`archmap serve` on this repository: the live map with the function list.*

(The viewer UI is currently in Russian.)

## Install

**Homebrew** (macOS and Linux):

```sh
brew install aidarbn/tap/archmap
```

**Prebuilt binaries**: download the archive for your OS and CPU from
[Releases](https://github.com/aidarbn/archmap/releases) (macOS arm64/amd64,
Linux amd64/arm64, Windows amd64), check it against `checksums.txt`, unpack and
put `archmap` on your `PATH`.

**From source**:

```sh
go install github.com/aidarbn/archmap/cmd/archmap@latest
```

A C compiler is required for this (tree-sitter grammars are built with cgo).

## Quick start

```sh
archmap map --root . --open        # one HTML file in .archmap/arch.html
archmap serve --root . --open      # live map that follows the repo
archmap extract --out facts.json   # only the facts (JSON)
archmap render --facts facts.json --out arch.html
```

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

```json
{
  "title": "shop — architecture",
  "modules": {
    "internal/orders": {"role": "Order domain", "layer": "domain"}
  },
  "flows": [
    {
      "name": "Place an order",
      "steps": [
        "internal/orders/handler.go::Handler.Create",
        "internal/orders/service.go::Service.Place"
      ]
    }
  ]
}
```

## Languages

- **Hand-written parsers** (most precise): Python, Go, JS/TS (JSX/TSX), Swift,
  Kotlin. Source: `internal/lang/<name>`.
- **Through tree-sitter tags queries** (`internal/lang/generic`): Java, Rust,
  Ruby, C#, PHP, Scala, Dart, Elixir, Haskell, C, C++, Lua, Bash, Zig, OCaml (`.h` is parsed as C++; quoted
  `#include`s are imports, `<system>` ones are not). A language is one `Spec` (name, extensions, grammar, tags query,
  optional import query); the shared code turns `@definition.*` /
  `@reference.call` captures into functions, owners, line ranges, signatures,
  doc comments, calls with receivers and imports. Expect less precision than
  the hand-written parsers (no decorators/bases, no entry-point hints).

The registry (`internal/lang/all`) is the only source of file extensions.

### Contributing: adding a language

1. Add the grammar's Go binding module (`go get github.com/tree-sitter/tree-sitter-<lang>`).
2. Copy its `queries/tags.scm` to `internal/lang/generic/queries/<lang>/tags.scm`,
   adapt it (captures: `@definition.function|method|class|interface|module|impl`
   with `@name`, `@reference.call` with `@name`, optional `@doc`) and keep the
   source and licence in the file header. Optionally add `imports.scm` with
   `@import` / `@import.relative` / `@import.mod` (and `@wild`).
3. Append one `Spec` to `Specs()` in `internal/lang/generic/langs.go`.
4. Add a test next to `generic_test.go` with a mini source and the expected
   functions, owners, line ranges, calls and imports, and run
   `go test ./... && go vet ./...`.
5. Check it on a real repository: `archmap extract --root <repo> --out x.json`.

A language needing hand-written logic (decorators, bases, entry points) gets its
own package like the first five.

## Limitations

- Calls are resolved by name and receiver, which is an approximation: expect
  some extra or missing edges. Common method names (`get`, `Close`, ...) are not
  linked.
- The binary is about 60 MB because all 20 tree-sitter grammars are linked in.
- A language without a parser (see Languages) contributes nothing to the map.
- Change status needs git.

## License

MIT
