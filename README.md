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
- `serve` is reserved and not implemented yet.

## arch.model.json

The optional semantic layer on top of the facts: titles, module roles and
layers (`entry|domain|adapter|infra|ui|test|other`) and named flows, where each
step is an existing function id (`path/file.ext::Class.method`). See
[examples/arch.model.json](examples/arch.model.json).

## Languages

- **Hand-written parsers** (most precise): Python, Go, JS/TS (JSX/TSX), Swift,
  Kotlin. Source: `internal/lang/<name>`.
- **Through tree-sitter tags queries** (`internal/lang/generic`): Java, Rust,
  Ruby. A language is one `Spec` (name, extensions, grammar, tags query,
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
- A language without a parser (see Languages) contributes nothing to the map.
- Change status needs git.

## License

MIT
