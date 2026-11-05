# archmap diff: what a change does to the diagrams

`archmap diff` compares the diagrams of the current tree with those of a base
revision and prints what changed, ready for a pull request comment:

- **scenarios** — declared ones (`arch:flow`, `.arch` flows) always; automatic
  ones only when the viewer shows them prominently (among the top scenarios, or
  the entry function has a doc comment), so that a heuristic walk does not make
  noise;
- **journeys** (`arch:journey`, `.arch` journeys; automatic journeys among the top);
- **state machines** (`arch:state`, `.arch` `states`, transitions found in the code).

The diagrams are built exactly like `archmap map` builds them (the same
extraction, spec merge and scenario model), once for the base and once for the
tree.

```sh
archmap diff [--root DIR] [--base REF | --base-root DIR]
             [--format text|markdown|json] [--lang en|ru] [--out FILE]
             [--fail-on broken] [--tests]
```

- `--base REF` (default `origin/main`) — the base revision. It is checked out
  with `git worktree add --detach` into a temporary directory, which is removed
  afterwards. `--root` may be a subdirectory of the repository: the same
  subdirectory of the base is compared.
- `--base-root DIR` — a directory that already holds the base tree (no git needed).
- `--format` — `text` (default), `markdown` (for a PR comment) or `json`.
- `--lang` — `en` (default) or `ru` for text and markdown.
- `--out FILE` — write the report to a file instead of stdout.
- `--fail-on broken` — exit with 1 when the change **breaks** a diagram (see
  below). Diagrams that were broken in the base already do not count; the
  summary shows how many there are.

Exit code: 0 — no changes, or only changes; 1 — `--fail-on broken` and the
change broke something, or an error; 2 — bad flags.

## What is reported

The key of a diagram is its id (scenario id, `auto:<fid>` for an automatic one,
journey id, entity of a state machine).

- **A diagram appeared or is gone**, with the reason: the entry-point function
  was deleted (its `file:line` in the base), `arch:flow` was taken off, the
  function is no longer an entry point, the flow was removed from `arch/*.arch`,
  every flow of a journey is gone.
- **Scenario steps**: a step is gone (caption and function, and why: the function
  was deleted, `arch:step` was taken off, it is still marked but the entry point
  no longer reaches it, or — automatic flows — the walk no longer goes there); a
  step was added; a caption changed; the order of the steps changed.
- **Journeys**: a scenario left the journey (and whether it exists at all) or joined it.
- **States**: a state added or removed; a transition appeared or is gone (with the
  function that makes it); the same transition is now made by other functions.

**Broken** diagrams (the same rules as `archmap lint`, see docs/spec.md,
"Annotations do not change the logic"):

- a journey names a scenario that does not exist;
- a branch (`arch:on -> flow`, `alt -> flow`) leads to a scenario that does not exist;
- a scenario has `arch:step` markers but no entry point (no `arch:flow`);
- a spec step names no function;
- a step is not reachable by calls from the entry point of its scenario.

## Markdown

```markdown
### Диаграммы archmap

Изменено: 2 · появилось: 0 · исчезло: 1 · битых: 1

**Битые диаграммы**

- путь `guest-cancel-order` ссылается на сценарий `cancel-order`, которого нет (internal/adapters/in/grpc/tablet_server.go:458)

<details><summary>Сценарий «Оплата заказа через Kaspi QR» (<code>kaspi-pay</code>) — изменён</summary>

- Шаг пропал: «QR сохраняется и сразу ставится фоновая проверка оплаты» — `Service.saveQRResult` (снята пометка arch:step)

</details>

Сценарий «Отмена заказа гостем» (`cancel-order`) — исчез: с точки входа `TabletsServer.CancelOrder` (internal/adapters/in/grpc/tablet_server.go:269 в базе) снята пометка arch:flow
```

No change at all prints the title and one line, "Diagrams did not change." /
«Диаграммы не изменились.».

## CI: diagrams diff in PRs

The job needs the base revision in the clone (`fetch-depth: 0`) and permission
to write PR comments. One comment per PR is kept up to date: the action below
finds its own comment by `header` and edits it on every push.

```yaml
name: archmap diff
on:
  pull_request:

permissions:
  contents: read
  pull-requests: write

jobs:
  diagrams:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5
        with:
          fetch-depth: 0 # the base branch must be in the clone
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6
        with:
          go-version: stable
      - name: Build archmap
        run: go install github.com/aidarbn/archmap/cmd/archmap@latest # better: pin a version
      - name: Diagrams diff
        run: >
          archmap diff --base origin/${{ github.base_ref }}
          --format markdown --lang ru --out diff.md
      - name: Comment on the PR
        uses: marocchino/sticky-pull-request-comment@5770ad5eb8f42dd2c4f34da00c94c5381e49af88 # v3.0.5
        with:
          header: archmap-diff
          path: diff.md
```

To fail the check when the change breaks a diagram, add `--fail-on broken` and
keep the comment step running anyway (`if: ${{ !cancelled() }}`), so the comment
explains the red check.

Without a third-party action, `gh` can do the same (it edits the last comment of
the token's user, so keep this the only comment the job writes):

```yaml
      - name: Comment on the PR
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh pr comment ${{ github.event.pull_request.number }} --body-file diff.md --edit-last \
            || gh pr comment ${{ github.event.pull_request.number }} --body-file diff.md
```
