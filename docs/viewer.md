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
| `panel.js` | Right card for the selection (`setSel`). |
| `zoom.js` | Semantic zoom on the map (wheel/pinch dives into a card taking > 55% of the screen, leaves a level shrunk below 35%; 300 ms pause, one level per gesture; toggle `archmap.autozoom`), the scale zoom of content without a camera (`AM.zoom.scaleWheel`: code text, the Scenario diagram — past ×1.57 it dives, below ×0.65 it leaves, with the same gate), and the `?debug=1` audit `AM.audit()` that warns about foreign visible panels after a view or level change. |
| `nav.js` | Header search (functions, modules, files), the Lists drop-down (modules, functions, branch changes), the flash of what a live update changed. |
| `views/*.js`, `views/*.css` | Tabs. `views/map.js` registers the map; add one file per view. |
| `main.js` | Start-up: header, tabs, theme, language, keys, `setData`, live mode. `fitHeader()` keeps the project name at least ~120px wide: when the header runs out of room the branch counters and "changed only" fold into the ⋯ menu, then the labels of the live badge and the Lists button go. |

Assembly order: `viewer.css`, then `views/*.css` by name; scripts `core.js`, `scene.js`,
`layout.js`, `map.js`, `playback.js`, `panel.js`, `nav.js`, `zoom.js` (one shared scope), then every `views/*.js` by
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
  is called before `live`. `Cam` has an optional `insetTop` (px, default 0): `fit`/`fitTop`
  keep that strip free of content.
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
- **Zoom** `AM.zoom.scaleWheel(e, {z, set(z), in?(), out?()})` — scale content by the wheel and change the
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
