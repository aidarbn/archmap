# Viewer: sources, build and views

The page that `archmap map`, `render` and `serve` produce is one offline HTML file. Its sources
live in `internal/render/viewer/` and are assembled once by `internal/render/render.go`
(`go:embed` + concatenation); nothing is fetched from the network.

## Files

| File | What it holds |
|------|---------------|
| `shell.html` | Markup of the page: header, stage with the map scenes, right panel, player. Slots `<!--@CSS-->`, `<!--@JS-->`, placeholders `<!--TITLE-->` and `/*ARCH_DATA*/null`. |
| `viewer.css` | Design tokens (light/dark), core styles, and the approved mockup styles for the views (sequence, lists, ER, matrix, status). |
| `core.js` | DOM helpers, i18n (`I18N`, `t`, `L`, `addI18n`), data index `D`, `state`, event `bus`, `registerView`, `registerPanel`, code highlighting and loading, the global `AM`. |
| `scene.js` | `Cam` (viewBox camera: pan, wheel zoom, `fit`, `animTo`), geometry (`clip`, `hop`, `bbox`), `runDot`, `pulse`, level transitions. |
| `layout.js` | `layered()` — Sugiyama-lite band layout. |
| `map.js` | The map: level 0 system, 1 modules (groups when > 60 modules), 2 functions of a module, 3 code; selection, drill in/out, crumbs, minimap, legend. |
| `playback.js` | Scenario runs: picker, steps through the levels, branches, effects/status badges, auto play. |
| `journeys.js` | Through paths and variants: the shared list rows (`AM.jv.journeyRows`, `variantRows`, `tog`), runs `journey:<id>` and `variant:<id>` (`fixed` runs: no branch stop), the plate between flows / at the end of a variant (`AM.jv.plate`, `end`, `jump`), progress and the label of the current path (`.jv-tag`). Event `jv` — a variants fold changed. |
| `panel.js` | Right card for the selection (`setSel`). The panel hides (`#panelBtn` in the header, the button in its corner, `]` or Cmd/Ctrl+.) and resizes by its left edge (arrows nudge, double-click resets); both kept in `localStorage` (`archmap.panel.hidden`, `archmap.panel.w`, the CSS variable `--pw` on `main`); a change fires a window `resize`, so the map, the views and the dock fit the new width. A selection never reopens a hidden panel: `#panelBtn` gets a dot ("there are details"). `AM.panel.setHidden(h)`, `setWidth(w, save)`, `hidden`, `width`. |
| `zoom.js` | Semantic zoom on the map (wheel/pinch dives into a card taking > 55% of the screen, leaves a level shrunk below 35%; 300 ms pause, one level per gesture; toggle `archmap.autozoom`), the scale zoom of content without a camera (`AM.zoom.scaleWheel`: code text, the sequence panel — past ×1.57 it dives, below ×0.65 it leaves, with the same gate), and the `?debug=1` audit `AM.audit()` that warns about foreign visible panels after a view or level change. |
| `nav.js` | Header search (functions, modules, files), the Lists drop-down (modules, functions, branch changes), the flash of what a live update changed. |
| `views/*.js`, `views/*.css` | Tabs. `views/map.js` registers the map; add one file per view. `views/sequence.js` is the exception: no tab, it builds the **sequence panel** (a dock over `#v-map`, see below) and exports `AM.dock`; `views/sheet.js` draws the **scenario sheet** inside that dock (`AM.sheet`, see below). |
| `main.js` | Start-up: header, tabs, theme, language, keys, `setData`, live mode. `fitHeader()` keeps the project name at least ~120px wide: when the header runs out of room the branch counters and "changed only" fold into the ⋯ menu, then the labels of the live badge and the Lists button go. |

Assembly order: `viewer.css`, then `views/*.css` by name; scripts `core.js`, `scene.js`,
`layout.js`, `map.js`, `playback.js`, `journeys.js`, `panel.js`, `nav.js`, `zoom.js` (one shared scope), then every `views/*.js` by
name, **each wrapped in its own `{ … }` block**, then `main.js`. The core list is in
`render.go` (`coreJS`); views are picked up automatically — adding a view needs no Go change.
Assembly fails (tests catch it) if a part contains `</script` / `</style`, or a placeholder.

The data contract is unchanged: `const DATA = {"facts":…,"model":…,"serve"?:…};` with every `<`
escaped as `<`.

## Adding a view

Create `internal/render/viewer/views/<id>.js` (and optionally `views/<id>.css`). The file runs
in its own block; use only the global `AM`:

```js
AM.addI18n({ ru: { apiTitle: 'API' }, en: { apiTitle: 'API' } }); // prefix keys with the view id
AM.registerView({
  id: 'api',                 // container: <div class="view" id="v-api"> inside #stage
  order: 20,                 // tab position; map is 0
  title: () => AM.t('apiTitle'),
  mount(el, ctx) { /* once, the first time the tab opens; build DOM in el */ },
  show(ctx) {}, hide(ctx) {},            // every switch to / from the tab
  onStep(step, ctx) {},      // playback moved (step object below, or null on reset)
  onSelect(sel, ctx) {},     // selection changed
  onLang(ctx) {},            // language switched: re-render texts
  onData(ctx) {},            // data replaced by archmap serve: re-render
});
AM.registerPanel('ep', (sel, ctx) => `<h2>${ctx.esc(sel.id)}</h2>`); // right card for {t:'ep', id}
```

Hooks other than `mount` are called only after the view was mounted; errors in a hook are
logged and do not break the page.

## What a view gets (`AM`)

- **Data** `AM.D`: `F` (facts), `M` (model), `fns`, `fids`, `mods`, `MM` (model modules),
  `byMod`, `callers`, `allMods`, `changed`, `deleted`, `modChg`, `entries`, `externals`
  (`{id:'ext:Name', name, kind, modules, functions}`), `extByMod`, `extByFn`, `endpoints`,
  `epByFid`, `states`, `transByFid` (fid → `[{entity, from, to}]`), `tables`
  (`facts.data.tables`), `flows` (model flows with known steps, `i` = index in `model.flows`),
  `flowById`, `actors` (`{id:'actor:…', eps, fids, mods}`), `live` (serve endpoints or null).
- **State** `AM.state`: `view`, `level`, `mod`, `fn`, `sel`, `only` (changed-only filter),
  `run`, `step`, `playing`, `speed`.
- **Bus** `AM.bus.on(event, fn)` → unsubscribe; events: `data`, `lang`, `theme`, `view`,
  `level`, `select`, `step`, `flow`, `filter`, `resize`, `live` (after a serve update; `{flash: [fid]}` — functions that
  changed, to highlight in your own view; the core flashes its own scenes and lists). `onData`
  is called before `live`. `Cam` has optional `insetTop`, `insetBottom` and `insetRight` (px,
  default 0; capped at 25% / 60% / 60% of the scene): `fit`/`fitTop`/`animTo`/`contains` keep
  those strips free of content (the free area is the top-left part of the scene minus the top strip).
  `AM.map.setDock(bottom, right, dur)` sets the bottom/right insets of all map cameras and re-frames the
  current step (or the level) in what is left.
- **Sequence panel** `AM.dock` (`views/sequence.js`): `toggle()`, `open(full)`, `esc()` (closes the
  scenario list, then leaves the full screen; returns true when it took the key — `main.js` calls it
  first on Esc), `shown`. The panel is `section.seqdock` inside `#v-map`: bottom (40% of the height by
  default) or right (45% of the width), a separator (`.sd-grip`: drag, arrows, double-click for the default),
  a strip when folded, full screen over the whole map. It appears when the run is a scenario, path, variant
  or function run other than the one the page started with, or as soon as a step was taken; the diagram
  button `#btnSeq` and the × of the panel override that per run. Header: the scenario combobox with search
  (`AM.jv` rows, the same list as "Scenarios"), levels, "with map" (keeps the diagram's level equal to the map's),
  function scope, fold frames, place buttons, fold, full screen, close. `localStorage` `archmap.seqdock`
  holds `{pos, bottom, right, min}` (shares of the stage); `archmap.seqlink` the "with map" switch. CSS
  variables `--dock-b` / `--dock-r` on `#v-map` (and `data-dock="b|r"`) move the map's own buttons; the code
  scene `#sc3` is shrunk by them. `?view=sequence` opens the map with the panel full screen.
  The header switch "Sheet | Sequence" (`AM.dock.mode`, `AM.dock.setMode('sheet'|'seq')`, `localStorage`
  `archmap.seqmode`, the sheet by default) picks what the panel draws; in the sheet mode the level switch has no
  "Code" (functions stand for it) and the scope / frame chips are hidden.
- **Scenario sheet** `AM.sheet` (`views/sheet.js`): `render(host, {level, setLevel})` (the dock's scroll box),
  `sync(keep)` (lights the current step; `keep` = no running dot), `select(sel)`, `wheel(e, level, setLevel)`
  (pinch / Cmd+wheel: `AM.zoom.scaleWheel`, past the thresholds a level), `clear()`; `model`, `layout`, `scale`;
  `crossings()` (the real crossings of the routed arrows now; `window.shCrossings()` with `?debug=1`) and
  `toggle(key | "5b")` (unfold / fold a branch).
  The model: nodes (`f:<fid>` functions in frames `F:<module>`, `m:<module>`, `svc`, `a:<actor>`, `x:<external id>`,
  `p:<plan id>` planned steps, `o:t:<key>` outcomes, `o:f:<flow>` scenarios a branch leads to, `o:exit` the outcome
  of the run), edges keyed `from>to>kind` with `kind` `entry call self plan enq deq ext branch exit opt` and the run
  indexes in `steps` (one edge per pair and kind: numbers `3–5`). The steps of the run are the **main path**
  (`main` on edges and nodes: bold, ending in `o:exit`); calls of external systems are secondary (thin, muted), two
  or more of them into one system from the left share a **bus** (`bus`: one port, one trunk in the gap, the DB
  effects on the branches). **Branches** are folded by default: `node.stubs` (`{key, no, cond, text, flow, tone,
  open, taken}`) drawn under the fork as pills "5b · QR истёк ✕" / "5a · оплачен → … ↪", three at most and then
  "ещё N" (`SH.more`); a click unfolds one (`SH.open`, kept while the run key is the same): its outcome joins the
  outcome column with a dashed `branch` edge. The branch a variant ends in (or the outcome picked at the end of a
  run) is unfolded and `taken`; a fork past the end of the run is not drawn. Columns `act in dom ad ext out`
  (`act svc ext out` on the system level) by `layerOf` (entry/ui → in, domain/other/test → dom, adapter/infra →
  ad). Layout: items stacked per column, ordered by the median y of their neighbours (8 sweeps), nodes inside a
  frame in the order of the run; then a local search on the real routes (swap neighbouring items, nodes of a frame
  off the main path, the side of a loop inside a column; two starts — the sweeps and the order of the run) scored by
  crossings ×10 + jogs of the main path ×3, time-boxed; items pulled toward their neighbours (main-path ones by
  their main-path neighbours, a bus system up to its topmost branch); ports spread over the node sides (a loop
  inside the column nearest its target, a corridor route by the corridor's y), a main arrow a few pixels off
  straight is straightened; orthogonal routes with the vertical runs on tracks in the gaps (the order of the tracks
  swapped while it crosses less) and horizontal runs across other columns only in free corridors; number pills
  and tags (two short lines, the whole text in the tooltip) placed by the least overlap, then pushed apart and,
  if still on another label, moved to the nearest free place (a leader line when far). Fit: `min(width, height)` of the box,
  clamped to 0.75…1.2 (then it scrolls; a drag on the background pans). A level change or a walk step re-renders
  and animates every node and frame from its old screen box (or from the frame / module / service it folded into)
  with the Web Animations API; a walk never zooms in. Bus event `walk` (`{kind: 'step'|'sel'|'exit'}`, from
  `views/walk.js`) redraws the fan of the walk; `AM.walk.options()`, `take(o)`, `select(k)`, `sel` feed it.
- **Cross-view events** (API ↔ Data), both emitted after `AM.setView(…)` of the target tab:
  - `api:open-table` `{table, op, ep, fid, via}` — from the API view (a table in the endpoint card);
    `op` is `R`/`W`/`RW`, `ep` the endpoint object, `via` the first function of the call closure that
    touches the table. The same payload is kept in `AM.apiTarget` for a Data view that mounts later.
  - `data:open-endpoint` `{ep, fid, table}` — from the Data view (an endpoint in the table card or the
    matrix); `ep` is the endpoint object from `AM.D.endpoints`. The API view finds it (by object, then
    `fid`+`path`, then `fid`), drops filters that hide it, expands its group, scrolls, selects it
    and opens the card.
- **Text** `AM.t(key, …)`, `AM.L({ru, en})`, `AM.lang`, `AM.addI18n`, `AM.plural`.
- **DOM** `AM.h(tag, attrs, parent, html)`, `AM.svg(tag, attrs, parent, text)`, `AM.esc`,
  `AM.cut`, `AM.cutL`; `AM.Cam`, `AM.layered`, `AM.hop`, `AM.clip`, `AM.bbox`, `AM.runDot`,
  `AM.pulse`; shared SVG defs: filters `#sh`, `#sh2`, markers `#arr`, `#arrT`, `#arrM`, `#arrA`,
  `#arrO`.
- **Facts helpers** `AM.fnInfo(fid)`, `AM.fname`, `AM.short(module)`, `AM.modLabel`,
  `AM.layerOf`, `AM.modRole`, `AM.modDesc`, `AM.LAYERS`, `AM.LCOLOR`, `AM.isCall`, `AM.isAsync`,
  `AM.extsOfFn`, `AM.actorName`, `AM.epRole`.
- **Call graph** `AM.callPath(a, b, max = 8)` — the shortest chain of calls (callees and async hand-offs)
  `[a, …, b]` or null; `AM.callSite(fid, target)` → `{no, text, file, has, loadable?}` — the line of
  `fid` that calls `target` (or a callee leading to it); `has` is false while the body is unknown.
- **Scenarios** `AM.flowGroups(query)` → `[{name, items: [flow], auto?}]` (by actor, then the automatic
  ones), `AM.flowActor(flow)`.
- **Zoom** `AM.zoom.scaleWheel(e, {z, set(z), in?(), out?()})` — scale content by pinch / Cmd+wheel / Ctrl+wheel (a plain wheel is left to native scrolling) and change the
  level past the thresholds (once per gesture); `AM.zoom.on` — auto-dive is on.
- **Code** `AM.hlLine(line, lang)`, `AM.codeHTML(fid, targetFid)` → `{html, hlIdx, has}`,
  `AM.srcOf(fid)`, `AM.loadSrc(fid)` (fetches `/src` only under `archmap serve`),
  `AM.fnSection(fid)` (the function card used by the panel).
- **Navigation** `AM.setView(id)`, `AM.select(sel)`, `AM.openFn(fid)`, `AM.openMod(module)`,
  `AM.map.jumpToFn(fid, 2|3)`, `AM.map.goModule(module)`, `AM.map.drill`, `AM.map.up`,
  `AM.map.unitOf(module)`, `AM.map.setLevel(level, fid?)` (switch the map's level at once, without the
  transition — for a view that keeps the map on its level while it is off screen; `AM.map.marks()` puts
  the play and selection marks back when the map is shown); the scenes are `AM.map.system`, `AM.map.modules` (`pos`, `nodes`,
  `edges`, `extent`), `AM.map.functions`.
- **Playback** `AM.play.load(key)` (`'flow:<i>'`, `'fn:<fid>'`, `'changes'`, `'overview'`),
  `step(±1)`, `go(i)`, `stepIn`, `stepOut`, `auto`, `pause`, `reset`, `playFrom(fid)`,
  `current()`, `stepAt(i)`, `list()`, `frames(i)`, `branches()`, `choose(branch)`, `run`.
  Badges: `AM.ui.statusPill`, `AM.ui.mutBadge`, `AM.ui.effBadges`.

A **step** is `{i, fid, prev, next, seg, flow, kind: 'entry'|'call'|'async'|'next', label,
effects: [string], data: [{table, op, fields}], states: [{entity, from, to}], exts, actor, mod}`.
A run is `{key, name, desc, segs: [{flow, steps}]}`; a branch with `flow` appends a segment.

Selection kinds of the core: `null`, `{t:'svc'}`, `{t:'actor', id}`, `{t:'ext', id}`,
`{t:'mod', id}`, `{t:'group', id}`, `{t:'fn', id}`, `{t:'step'}`. Views add their own kinds
(`ep`, `table`, `status`, …) with `registerPanel`.

## Model data: flow ids, journeys, variants

`DATA.model.flows[]` (see `autosem.Flow`). New or changed fields:

- `id` — **always present and unique**: the arch id for `kind:"declared"`, `"auto:" + <fid of the
  entry point>` for automatic flows. (It used to exist for declared flows only; use `kind` to
  tell them apart.)
- `variants` — only declared flows that have `arch:on`; absent otherwise (no forks = no variants,
  the flow is its own only path). `variantsMore` — how many were cut by the limit (12).
  Variant: `{id: "<flow id>#<n>", name, main?, any?, choices: [{at: fid, cond}], steps: [fid],
  end: {kind: "flow"|"exit"|"text", flow?, text?}}`. `#0` is the main variant (`main:true`,
  `choices: []`, all steps, `end.kind:"exit"`, `end.text` = the flow's `exit`; its `name` is the
  constant `"main path"`, localize it). Every other variant is one `arch:on` outcome: `steps` is
  the flow's steps up to and including the step with the fork (`choices[0].at`); the variant ends
  there — in another flow (`end.kind:"flow"`, `end.flow` = flow id, `end.text` = the caption if
  any), or with a text outcome (`end.kind:"text"`). `name` is `"<cond> → <caption or target flow
  name>"`. When a flow has more than 12 outcomes, variants with the same `end` are folded into
  one with `any:true` (`choices` are then alternatives, `name` = conds joined with `" | "`),
  then the list is cut to 12.

`DATA.model.journeys[]` — paths over several flows, declared ones first, `[]` when none:

```
{id, name, desc, actor, exit, declared: bool, top: bool,
 flows: [flow id],            // unique; flows[0] is where it starts
 links: [{from: flow id, to: flow id, via: "on"|"queue"|"declared", cond?}]}
```

`links` is a graph (a tree with joins) over `flows`, never a cycle. `via:"on"` — `arch:on` with a
condition (`cond`); `"queue"` — a step puts a job on a queue whose handler is the entry point of
`to`; `"declared"` — the order was written by hand and the code shows no link (draw it dashed).
A declared journey lists its flows in the written order and has one link per neighbouring pair
(several when both an `on` and a queue connect them); automatic ones (`declared:false`, id
`"j:" + <first flow id>`) are branching walks from every flow that a person starts (actor not
cron/queue/system; without an actor: entry kind http/ui), at least 2 flows, at most 16, journeys
that are only a part of a longer one are dropped. `top` is true for the first 8.
Markup syntax: README, "Journeys".

## Layout (`layered`)

`layered(nodes, edges, opt)` puts equal cards into horizontal bands (`node.layer`), wraps a wide
band into rows filled column by column, and orders nodes by the median of their neighbours' x in
alternating down/up sweeps, keeping the order with the fewest crossings between band pairs.
`opt.anchors` pins outside nodes (actors, externals) that pull their neighbours. The module map
then spreads edges over card ports and pulls their control points toward a common trunk
(bundles). No libraries; Taply (60 modules) lays out in a few ms.

## Checking

`go test ./internal/render/` covers the assembly and the data contract. To try a change, render a
repo and open the file: `go run ./cmd/archmap map --root <repo> --out /tmp/x.html`.
`window.__errs` collects JS errors; `?slow=8` slows transitions, `?lang=en`, `?theme=dark`.

## Editor and Walk (`views/editor.js`, `views/walk.js`)

**Edit mode** is a header toggle (`#editBtn`), present only under `archmap serve` when `GET /spec` answers
(`?edit=1` opens it on load); the static HTML never shows it. It has no tab: the right panel gets tabs
Card | Scenarios | Text | Drift. `AM.spec` is the client: `avail`, `on`, `model`, `files`, `errors`,
`findings`, `undo`/`redo` stacks, `send(ops)`, `doUndo()`, `doRedo()`, `do.*` (state and transition
helpers used by `views/status.js`), `ask()` (inline field), `mergeMachines()`. Bus events `spec` (data
read or written) and `edit` (mode toggled).

Contract (see `docs/spec.md`): `GET /spec` → `{seq, files:[{path,text}], model, errors:[{file,line,col,msg}], findings}`;
`POST /spec/ops` takes `{ops:[…]}`; `PUT /spec/file` `{path,text}` (422 with `errors`). The editor re-reads
`/spec` after every `live` update (SSE `ready`). Model fields used: `states[].{entity,states,transitions[{from,to,on,label,line,source,drift,fid}],final,file,line,code}`,
`flows[].{id,name,actor,exit,steps[{ref,planned,ext,text,line,fid,drift}],alts[{cond,flow,text}],file,line}`,
`journeys[].{missing}`. Operations sent: `add_state remove_state rename_state set_final add_transition
remove_transition add_flow remove_flow add_step remove_step move_step add_alt remove_alt set_actor set_exit`,
fields as in docs/spec.md. Undo/redo (Cmd+Z, Shift+Cmd+Z) replays the inverse operations (computed against the model before
the op); a text save clears the stacks. Hand-written `.arch` text: `PUT /spec/file`, errors by line.

Statuses (`views/status.js`): transitions get `st` = `both` (described and in code), `code` (`drift: undeclared`, orange)
or `spec` (`drift: not_implemented`, dashed grey); in edit mode: double click empty space adds a status, double click a node
renames, drag node to node draws a transition (inline `event` or `event "label"`), Del removes a status or the selected
transition, double click an orange edge adds it to the spec. Planned steps (`planned: true`) are dashed chips on the map
(system / module level, in their module when `module` is set; they read `model.spec` from the data, so they also show
in a static HTML). They are steps of the run too: `flowSteps(fl)` (core) keeps the `plan:<flow>:<n>` ids of
`fl.planned`, `AM.isPlan(id)`; `stepAt` gives them `planned: true`, `kind: 'plan'`, `mod` from `fl.planMod`, no
data / statuses, `prev` = the last step with a function (later steps skip them too). The map lights the module (or
the service) without moving, the sequence draws a dashed message to the module, the sheet a dashed node, the card
says "no code yet". A flow with planned steps only is playable. In edit mode the scenario card sets the module of a
planned step (`set_step` with `module`); a walk saves its planned steps with the module of the step they follow.
The Drift tab lists each drift once.

**Walk** (`#walkBtn`, "▶ Walk from here" in a function card, `AM.walk.start(fid, guide?)`): a run with key `walk` whose steps
are the chosen path. On the level "functions of a module" every outgoing arrow of the current function is drawn
(`callees`, `async_callees` dashed "queue →", external systems "→ Name"; helpers folded by the same name/file/class/module
rules as `autosem.isHelper`; arrows to visited functions muted); click or →/Enter takes the arrow, ↑/↓ chooses, ←/Backspace
steps back, Esc exits. The right card (`registerPanel('walk')`) shows the arrows, the code with the call of the chosen
arrow highlighted, data, statuses and the trace. Started inside a scenario, its next step is marked "by scenario".
In edit mode the card can save the path: `add_flow` + `add_step` (`ref` = fid, `text` = caption, `ext`, planned steps
as `planned:true,text`) + `add_alt`; "save as variant" saves the steps from the deviation as a new flow and hooks it with
`add_alt {target}` on the guide scenario.
