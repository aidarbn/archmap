# The `.arch` specification

Reference for the files in `<root>/arch/*.arch`, their JSON model, the edit
operations and the HTTP endpoints of `archmap serve`. It is written for the
editor in the viewer: the editor never writes text, it sends operations and
renders the model.

The directory is optional. All `*.arch` files directly in `<root>/arch` and in
`<root>/arch/pages` are read (no other subdirectories; names match
`[A-Za-z0-9_][A-Za-z0-9_.-]*.arch`). The language describes behaviour only: what
happens and in which order, not how. Pages (`page`, see "Pages") pick diagrams
to show together and change none of them.

## Grammar

Line oriented. A block starts at column 0 with `states`, `flow`, `journey` or `page`; its
body lines are indented (spaces or tabs). `#` starts a comment outside quotes.
Strings are in double quotes with the escapes `\"`, `\\`, `\n`. Identifiers
(`id`) are letters, digits, `_`, `-`, `.`, not starting with `-` or `.`; latin is
the convention, other letters are accepted (real code has states such as `нет`).

```
file     = { block } .
block    = states | flow | journey | page .

states   = "states" Entity NL { "  " statesline NL } .
statesline = "state" names                          # states without a transition
           | "final" names                          # states that end the machine
           | from "->" to [ "on" trigger ] [ string ] .   # from may be "*" (any state)
names    = id { [","] id } .

flow     = "flow" id [ string ] NL { "  " flowline NL } .
flowline = "actor" text
         | "step" ref [ "ext" System ] [ string ]   # ref: Type.Method | Func | path::fid
         | "step" "plan" string [ "in" module ]     # planned step: no code yet; module — where it will live
         | "state" Entity ":" from "->" to          # the flow moves an entity
         | "alt" cond [ "->" flowid ] [ string ]    # a branch or outcome
         | "exit" text .

journey  = "journey" id [ string ] ":" flowid { ">" flowid } NL .

page     = "page" ( string | id [ string ] ) NL { "  " frame NL } .   # id = slug of the name when omitted
frame    = "journey" journeyid | "flow" flowid      # one id per line
         | "states" refs | "tables" refs | "module" refs
         | "api" apirefs                            # Name | Service.Name | /path | "METHOD /path"
         | "note" text .
refs     = word { ( "," | " " ) word } .
text     = string | words to the end of the line .
```

Canonical output (what the server writes): blocks in the order states, flows,
journeys, pages (each in its order), two-space indent, the values of a page's
frames in one column (one space after the longest keyword), columns of `states` transitions
(arrow, `on`, label) and `step`s (ref, `ext`, caption) padded with three spaces,
`actor` bare unless it needs quotes, `exit` always quoted, one blank line between
blocks, a newline at the end. `Parse(Format(x)) == x`.

`actor` in a flow is the actor's name only. What the actor *is* comes from the code,
not from the spec: `arch:actor <Name> "description"` in a comment (it may stand alone,
without a flow), else the comment of the gRPC service most of its endpoints belong to.
The spec and the code meet by the name (case-insensitive), so `actor Tablet` here and
`// arch:actor Tablet "the tablet in the dining room"` in code are one actor with that
description. See the README, "Annotating flows" and "How it explains code".

`module` is a module path as archmap shows it (`services/kaspi`, `app.orders`): one
word without quotes, `#` or `->`. The viewer draws the planned step in that module
(on the map, in the sequence and on the scenario sheet); without `in` it sits next
to the service. Lint warns when no such module exists in the code. Example:

```
flow kaspi-pay "Оплата Kaspi QR"
  step Service.requestQR   ext Kaspi   "Создаём QR"
  step plan                            "Проверить лимит платежей"   in services/kaspi
```

Comments: a `#` line at column 0 stays attached to the block below it (field
`comment`), those after the last block are `tail`. Comments at the end of a line
and indented comment lines are dropped when archmap rewrites a file.

Step references are resolved against the code by the fid suffix or the name:
exact fid, then `file.go::Func` suffix, then `Type.Method` / `Func` equal to the
part after `::`, then a `.Method` suffix. The first tier with one match wins; with
several matches the step is *ambiguous* (an error that lists up to 5 candidates).

## Pages

A page is a set of diagrams shown together, like a page in Figma. The usual place
is `arch/pages/<name>.arch`; a `page` block is accepted in any `arch/*.arch`.

```
# Оплата заказа через Kaspi QR
page "Оплата Kaspi"
  journey guest-pay-kaspi
  states  Order, Payment
  tables  orders, payments, kaspi_payment_details
  api     RequestKaspiQRForOrder, CancelOrder
  module  services/kaspi
  note    "Гость платит Kaspi QR на планшете; затем заказ уходит в кассу."

page menu-sync "Синхронизация меню"
  journey menu-sync-pos
  states  SyncRunStatus
```

- **Id** — `page <id> "Name"`, or `page "Name"` with the id made from the name
  (lower case, letters and digits of any script kept, the rest folded into `-`:
  `оплата-kaspi`), or `page <id>` (the name is the id). Ids are unique over all
  files (a second one is a `duplicate` error).
- **One line is one frame**, in the order of the lines; each frame is a separate
  diagram drawn by the viewer **from the code**, the page only says which:

  | Line | The diagram | The names must be |
  |------|-------------|-------------------|
  | `journey <id>` | the journey: participants in layers, overview → steps | a journey of `arch:journey` or `arch/*.arch` |
  | `flow <id>` | the scenario, the same way | a flow of `arch:flow` or `arch/*.arch` |
  | `states A, B` | the state machine of each entity | an entity with a state machine in the code (`Order` finds `OrderStatus`, as `states` does) or a `states` block |
  | `tables a, b` | these tables and the references between them | tables of the schema or of a data access |
  | `api X, Y` | a compact list of the endpoints with their descriptions | an endpoint: `Name`, `Service.Name`, the URL path or `"METHOD /path"`; a name of several endpoints is an error that lists them |
  | `module m` | the module and its functions | a module path, exact or a unique suffix (`services/kaspi` for `internal/services/kaspi`) |
  | `note "…"` | the text | — |

- **Layout** (enrich `layout`, `internal/spec.LayoutPage`): a grid of 4 columns;
  journeys and flows first, then state machines, each 2 columns wide; then the
  rest, 1 column, in the order of the lines; left to right, a frame that does not
  fit the row starts the next one. The viewer sizes columns and rows by the
  diagrams in them.
- In the JSON model `pages[]` is `{id, name, frames, comment?, file, line, layout}`;
  a frame is `{kind, refs?, text?, line, machines?, endpoints?, modules?,
  missing?}`: `machines` — the `facts.states` key per `states` ref, `endpoints` —
  the index in `facts.endpoints` per `api` ref (`-1` when none), `modules` — the
  module per `module` ref, `missing` — the refs that point nowhere. `layout` is
  `[{frame, row, col, span}]`.
- `archmap diff` reports pages that appeared, are gone or changed (a frame line
  added or removed, the name), and a reference that points nowhere as a broken
  diagram (`page-ref`).

## JSON model

`GET /spec` and the writing endpoints return the model of all files. Fields marked
*enrich* are computed from the code and never written to files. Absent optional
fields are omitted; lists are never `null`.

```jsonc
{
  "states": [{
    "entity": "OrderStatus",
    "states": ["draft"],                  // only `state` lines; others are implied by transitions/final
    "transitions": [{
      "from": "validated", "to": "payment_requested",   // from may be "*"
      "on": "kaspi-pay", "label": "QR requested", "line": 3,
      "source": "spec | code | annotation",   // enrich; spec ones are "spec"; code ones appear only as drift
      "drift": "not_implemented | undeclared", // enrich
      "fid": "path/file.go::Type.Method"      // enrich: function that makes the transition in the code
    }],
    "final": ["created"],
    "comment": ["lines above the block"],
    "file": "arch/orders.arch", "line": 1,
    "code": "OrderStatus"                  // enrich: key in facts.states of the matching machine ("" = none)
  }],
  "flows": [{
    "id": "kaspi-pay", "name": "…", "actor": "…", "exit": "…",
    "steps": [{
      "ref": "Service.requestQR",          // empty for a planned step
      "planned": true,                     // `step plan "…"`
      "module": "services/kaspi",          // planned steps only: `in <module>`
      "ext": "Kaspi", "text": "caption", "line": 9,
      "fid": "path::fid",                  // enrich: resolved function
      "drift": "missing | ambiguous | unreachable"   // enrich
    }],
    "states": [{"entity": "Order", "from": "validated", "to": "payment_requested", "line": 12}],
    "alts": [{"cond": "processed", "flow": "confirm-order", "text": "…", "line": 13}],
    "comment": [], "file": "arch/orders.arch", "line": 5,
    "code": true,                          // enrich: the code declares a flow with this id (arch:flow)
    "planned": 1                           // enrich: number of planned steps
  }],
  "journeys": [{
    "id": "guest-kaspi", "name": "…", "flows": ["create-order", "kaspi-pay"],
    "comment": [], "file": "…", "line": 20,
    "code": false,                         // enrich: the code declares this journey
    "missing": ["ghost"]                   // enrich: flow ids that exist nowhere
  }]
}
```

Transitions with `drift: "undeclared"` have no `line` and are not in any file:
they are in the response so the map can draw them; they come from the code
(`source` `code` or `annotation`, `fid` set). Operations never touch them.

A declaration repeated in several files keeps the first; lint reports the rest.

## Operations

`POST /spec/ops` takes one operation `{"op": …}`, an array of them, or
`{"ops": […]}`. They are applied in order to the model; if one fails nothing is
written. Operations edit entities in the file that holds them; a new entity goes
to `arch/<prefix>.arch` (the lowercase part of the id before the first `-`, `.`
or `_`) or `arch/main.arch`, or to `file` (`arch/<name>.arch`) when given. Every
field is a JSON string unless noted; `index` and `to_index` are 0-based numbers.

- `add_state` `entity`, `state` [, `file`]. Creates the entity if needed.
- `remove_state` `entity`, `state`. Also removes its transitions and final mark.
- `rename_state` `entity`, `from`, `to`. Also in `final` and in the `state` lines
  of flows.
- `set_final` `entity`, `state`, `final` (bool, default true).
- `add_transition` `entity`, `from` (or `*`), `to` [, `on`, `label`, `file`].
  Creates the entity if needed; a duplicate (same from, to, on) is an error.
- `remove_transition` `entity`, `from`, `to` [, `on`]. Without `on` removes
  every transition from→to.
- `remove_states` `entity`. The whole machine.
- `add_flow` `id` [, `name`, `actor`, `file`].
- `remove_flow` `flow`.
- `rename_flow` `flow`, `to`. Also in `alt … -> flow` and in journeys.
- `set_name` `flow` or `journey`, `name`.
- `set_actor` `flow`, `actor`; `set_exit` `flow`, `exit`. An empty string clears.
- `add_step` `flow`, and either `ref` [, `ext`] or `planned: true` with `text`
  [, `module`]; [`index` (default: the end)]. `module` with a `ref` is an error.
- `set_step` `flow`, `index`, and the same fields as `add_step`; replaces the step.
- `remove_step` `flow`, `index`.
- `move_step` `flow`, `index`, `to_index`. The step lands at `to_index` in the
  resulting list.
- `add_alt` `flow`, `cond` [, `target` (a flow id), `text`]. `remove_alt` `flow`, `index`.
- `add_journey` `id`, `flows` (array) [, `name`, `file`]; `set_journey_flows`
  `journey`, `flows`; `remove_journey` `journey`.

`name`, `actor` and `exit` are sent as strings (so `""` is distinguishable from
absent). `state`, `entity`, `id`, `from`, `to`, `on`, `flow`, `target` must be
identifiers; `ref` a function reference without spaces; `cond` words without `"`,
`#` or `->`. A planned step needs `text`.

## Endpoints (`archmap serve`)

All answers are JSON (`application/json; charset=utf-8`). The server accepts
loopback addresses only and checks the `Host` header. The two writing endpoints
need `Content-Type: application/json` and refuse a foreign `Origin`; bodies are
capped at 1 MiB; paths are `arch/<name>.arch`, nothing else is ever written.

- `GET /spec` → `{"seq", "files": [{"path", "text"}], "model", "errors": [{"file", "line", "col", "msg"}], "findings": [{"file", "line", "severity", "code", "msg"}]}`.
  `files` holds the text as it is on disk, `errors` the syntax errors, `findings`
  what `lint` would report for the spec (severity `error | warning | info`; codes
  `syntax duplicate no-function ambiguous-function unreachable-step planned
  no-flow undeclared-transition not-implemented unknown-state unknown-module`; `undeclared-transition`
  points into the code file). A file with a syntax error contributes the part
  that parsed. `seq` is the number of the last rebuild (the SSE `seq`).
- `POST /spec/ops` → same shape plus `"changed": ["arch/orders.arch"]` (the files
  written). Errors: `400 {"error": "…"}` for a bad request or an operation that
  does not apply (nothing is written), `409` when the spec has syntax errors (fix
  the text first; `errors` and `at` give the position).
- `PUT /spec/file` `{"path": "arch/orders.arch", "text": "…"}` → same shape as
  `POST /spec/ops`. The text is stored as typed (not reformatted). A syntax error
  answers `422 {"error": "arch/orders.arch:2:3: …", "at": {"file","line","col","msg"}, "errors": […]}`
  and the file is not touched. `400` for a bad path.

After a write the usual rebuild runs: SSE `/events` publishes `building` then
`ready` with a new `seq`, and `/data.json` carries the merged model. Files edited
outside archmap are picked up the same way. The static `map` and `render` only
read the files.

## What the viewer data gets

`/data.json` and `archmap map` embed the spec merged into the usual data.

`model.spec` is the model above. `model.flows[]` gets, for flows the spec
declares: `spec: true`, `unspecified` (step id → true), `ext` (step id →
system) and `plan` — the separate plan layer: `[{id, text, module?, after?}]`,
one per `step plan`, `id` = `plan:<flow id>:<position in the spec flow>`,
`after` = the code step (fid) it follows in the spec. Planned steps are never in
`steps`: a diagram shows what the code does (see "Annotations do not change the
logic" below).

- Flow with the same id as an `arch:flow` of the code: the spec wins on `name`,
  `actor`, `exit` and captions; its steps join the `arch:step` ones and all of
  them go in the order of the call graph from the entry point; code steps the
  spec flow does not list are in `unspecified`. `alt` lines become `branches`
  (`at` is the last step). `variants` are dropped for such a flow.
- A spec-only flow: `kind: "declared"`, `top: true`, `spec: true`, placed after
  the other declared flows. Its entry point is the step no other step of it
  reaches by calls; the steps follow the call graph from there.
- Journeys: a spec journey with the id of a code journey replaces its flow list; a
  new one is added, `declared: true`. Flow ids that exist nowhere are left out.
  Each link is the one the code shows (`on`, `queue`), else `via: "client"`.

## Annotations do not change the logic

archmap reads what calls what from the code. A diagram is a simplified view of
that graph; `arch:` markup and `.arch` files only **name and describe** (titles,
captions, actors, outcomes), **add knowledge from outside the code** (client
links, the plan layer) or **hide**. They never invent a step, a link or an order:

1. **A step must be reachable.** `arch:step <flow>` on a function that the walk
   from the flow's entry point does not reach is a lint error. The walk uses the
   edges autosem builds scenarios from: calls in the body (direct, through fields
   and interfaces as the extractor resolves them) and queue hand-offs (a worker
   of the same flow is reachable). A step after an `arch:on -> other` belongs to
   `other` and is checked against its entry point. The same holds for a spec
   `step`: error `unreachable-step` when the entry point (the `arch:flow`
   function, else the root step) does not reach it. A call the extractor does not
   see (through a function value, reflection, generated code) makes a false
   finding: the fix belongs in the extractor, not in a weaker rule.
2. **The order of steps is the order of the call graph**: depth-first from the
   entry point, calls in body order, then queue hand-offs. Neither the file
   order, nor the order of the markup, nor the order of a spec flow changes it.
3. **A state transition must be in the code.** `arch:state E: a -> b` with no
   code transition to `b` of the machine `E` made by the function or anything it
   reaches is reported by lint: as a warning for now (an error with `--strict`),
   because the extractor does not yet see a status passed as an argument
   (`repo.SetStatus(ctx, id, StatusPaid)`) or written by SQL text, so most such
   findings on real projects are extractor gaps.
4. **A journey link with no link in the code is a client link** (`via:
   "client"`: the client calls the next flow itself). Not a fault: lint lists it
   as information, the viewer draws it dashed.
5. **Planned steps (`step plan`) are a separate plan layer** (`flows[].plan`),
   never part of `steps` and never a lint problem (information only).

`archmap diff` reports broken diagrams with the same rules.

`facts.states[entity]` (the state machines): for each `states` entity of the spec,
transitions the code lacks are added with `source: "spec"`, `drift:
"not_implemented"` (a `*` source is expanded to the known non-final states); code
transitions the spec lacks get `drift: "undeclared"`; matched code transitions get
the spec's `on` and `label`. States of the spec are added to `states`. The entity
is matched to a machine by name (any case, or with a `Status`/`State` suffix).

## Lint rules

`archmap lint` runs the `arch:` checks and these, with `file:line` of the spec
(or of the code function for `undeclared-transition`). `--strict` fails on
warnings too. Text output prints `path:line: message`, warnings as `warning: …`
(without the prefix under `--strict`); `--format json` has `problems`, `warnings`,
`info` (each `{file, line, msg}`), and `spec`, `spec_flows`, `planned`.

- error: syntax error in a `.arch` file; a flow, states or journey declared twice.
- error: a code transition of an entity that the spec declares (`states`) that the
  spec does not declare. A code transition with an unknown source state matches by
  target state; `*` matches any source.
- warning: a spec transition the code does not make ("not implemented").
- error: a `step` that names no function; a `step` name that matches several.
- error: a step not reachable by calls from the entry point of the flow (the
  `arch:flow` function, else the root step). The order of the steps in the file
  does not matter: the call graph orders them.
- error: an `alt … -> flow` or a journey that names a flow declared neither in the
  spec nor with `arch:flow`.
- warning: `state Entity: a -> b` in a flow where `a` or `b` is not in that
  entity's `states`.
- info: every planned step (counted in `planned`).
- warning: a planned step `in <module>` that names no module of the code.
- error (`unknown-page-ref`): a page frame names a journey, flow, entity, table,
  endpoint or module the code (and the spec) does not have, or an endpoint /
  module name that matches several; `file:line` of the frame. A page declared
  twice is a `duplicate`. The JSON output counts pages in `pages`.

`arch:step`, `arch:on` and `arch:journey` in the code may name flows that only the
spec declares.
