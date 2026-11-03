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
