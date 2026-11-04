/* views/editor.js — the drawing editor ("Edit" mode), only under `archmap serve` where GET /spec answers.
 * No tab: a header toggle, tabs in the right panel (Card | Scenarios | Text | Drift), the spec client
 * (AM.spec) used by views/status.js (statuses on the diagram) and by the sequence panel / map (planned steps).
 * Contract with the server (docs/viewer.md, "Editor"):
 *   GET /spec           -> {files:[{path,text}], model, errors:[{path?,line,col?,message}], drift?:[…]}
 *   POST /spec/ops      body {ops:[{op, …}]} -> {model, files, errors, drift?}   (4xx/5xx: {error|errors})
 *   PUT /spec/file      body {path,text}      -> {model, files, errors, drift?} (422: {errors:[{line,col?,message}]})
 *   SSE /events         a message with `spec: true` after a write -> the editor re-reads GET /spec
 * Model: states {Machine: {states?:[name], transitions:[{from,to,on?,label?,source}], final:[name]}},
 *        flows [{id,name,actor?,exit?,steps:[{ref?|planned:true,label?,ext?,module?}],alts:[{cond,flow?|text?}],states:[{entity,from,to}]}],
 *        journeys [{id,name,flows:[id]}].
 * Ops: add_state/rename_state/remove_state/set_final/add_transition/remove_transition/update_transition (status machines),
 *      add_flow/remove_flow/add_step/remove_step/move_step/add_alt/remove_alt/set_actor/set_exit, add_journey/remove_journey. */
AM.addI18n({
  ru: {
    edBtn: 'Правка', edBtnTip: 'Режим правки: рисовать статусы и сценарии, сохраняется в текст arch/*.arch',
    edUndo: 'Отменить (⌘Z)', edRedo: 'Вернуть (⇧⌘Z)',
    edTabCard: 'Карточка', edTabFlows: 'Сценарии', edTabText: 'Текст', edTabDrift: 'Дрейф',
    edSaved: 'сохранено', edSaving: 'сохраняется…', edError: 'ошибка', edDirty: 'не сохранено',
    edAddFlow: '+ сценарий', edFlowName: 'Название сценария', edNoFlows: 'Сценариев в описании пока нет.',
    edAddStep: '+ шаг', edAddAlt: '+ ветка', edActor: 'актор', edExit: 'исход', edPlanned: 'запланированный шаг', edPlannedAsk: 'Текст запланированного шага',
    edFind: 'найти функцию…', edNoMatch: 'ничего не найдено', edCond: 'условие', edToFlow: '→ сценарий', edToText: '→ текст исхода', edAdd: 'Добавить',
    edDel: 'Удалить', edDragTip: 'перетащите, чтобы изменить порядок', edStepsN: (n) => n + ' шаг.',
    edNewState: 'Название статуса', edRename: 'Новое название', edEvent: 'Событие / подпись (необязательно)', edNewMachine: 'Название машины состояний',
    edAddMachine: '+ машина', edFinal: 'финальный', edRenameBtn: 'Переименовать', edDelState: 'Удалить статус', edDelTr: 'Удалить переход',
    edHint: 'двойной клик по пустому месту — статус · двойной клик по узлу — переименовать · тянуть от узла к узлу — переход · Del — удалить',
    edLgBoth: 'в описании и в коде', edLgCode: 'есть в коде, нет в описании', edLgSpec: 'описан, не реализован',
    edFiles: 'Файл', edSave: 'Сохранить (⌘S)', edErrors: 'Ошибки', edNoSpec: 'Файлов описания нет.', edTextHint: 'Сохранение — ⌘S', edChangedSrv: 'Файл изменился на сервере — ваши правки не затёрты',
    edDriftNone: 'Расхождений нет.', edDriftTitle: 'Расхождения описания с кодом', edOnMap: 'на карте', edInText: 'в тексте', edAccept: 'добавить в описание',
    edDCode: 'есть в коде, нет в описании', edDSpec: 'описан, но не найден в коде', edDRef: 'функции нет в коде', edDOther: 'расхождение',
    edRefMissing: 'нет в коде', edDrift_missing: 'нет в коде', edDrift_ambiguous: 'неоднозначно', edDrift_unreachable: 'недостижим', edPlanTag: 'план', edPlanned1: 'запланировано', edLoadFail: 'Не удалось прочитать /spec',
  },
  en: {
    edBtn: 'Edit', edBtnTip: 'Edit mode: draw statuses and scenarios, saved to the text arch/*.arch',
    edUndo: 'Undo (⌘Z)', edRedo: 'Redo (⇧⌘Z)',
    edTabCard: 'Card', edTabFlows: 'Scenarios', edTabText: 'Text', edTabDrift: 'Drift',
    edSaved: 'saved', edSaving: 'saving…', edError: 'error', edDirty: 'unsaved',
    edAddFlow: '+ scenario', edFlowName: 'Scenario name', edNoFlows: 'No scenarios in the description yet.',
    edAddStep: '+ step', edAddAlt: '+ branch', edActor: 'actor', edExit: 'exit', edPlanned: 'planned step', edPlannedAsk: 'Text of the planned step',
    edFind: 'find a function…', edNoMatch: 'nothing found', edCond: 'condition', edToFlow: '→ scenario', edToText: '→ outcome text', edAdd: 'Add',
    edDel: 'Delete', edDragTip: 'drag to reorder', edStepsN: (n) => n + (n === 1 ? ' step' : ' steps'),
    edNewState: 'Status name', edRename: 'New name', edEvent: 'Event / label (optional)', edNewMachine: 'State machine name',
    edAddMachine: '+ machine', edFinal: 'final', edRenameBtn: 'Rename', edDelState: 'Delete status', edDelTr: 'Delete transition',
    edHint: 'double click empty space — status · double click a node — rename · drag node to node — transition · Del — delete',
    edLgBoth: 'described and in code', edLgCode: 'in code, not described', edLgSpec: 'described, not implemented',
    edFiles: 'File', edSave: 'Save (⌘S)', edErrors: 'Errors', edNoSpec: 'No description files.', edTextHint: 'Save with ⌘S', edChangedSrv: 'The file changed on the server — your edits are kept',
    edDriftNone: 'No drift.', edDriftTitle: 'Where the description and the code differ', edOnMap: 'on the map', edInText: 'in the text', edAccept: 'add to the description',
    edDCode: 'in code, not described', edDSpec: 'described, not found in code', edDRef: 'function is not in the code', edDOther: 'drift',
    edRefMissing: 'not in code', edDrift_missing: 'not in code', edDrift_ambiguous: 'ambiguous', edDrift_unreachable: 'unreachable', edPlanTag: 'plan', edPlanned1: 'planned', edLoadFail: 'Could not read /spec',
  },
});

const T = (k, ...a) => AM.t(k, ...a);
const esc = AM.esc;
const el = (id) => document.getElementById(id);
const clone = (x) => JSON.parse(JSON.stringify(x));

/* ---------- the spec client ---------- */
const SP = (AM.spec = {
  avail: false, on: false, files: [], model: null, errors: [], findings: [],
  save: 'saved', saveMsg: '', undo: [], redo: [], tab: 'card', curFlow: null,
  mergeMachines: (base) => base, seqPlanned: () => 0,
});
let busy = Promise.resolve(), dirtyText = false, tabEls = null;

const mdl = () => SP.model || { states: [], flows: [], journeys: [] };
// read-only views (planned steps) also work in a static HTML: model.spec is embedded in the data
const viewModel = () => SP.model || (AM.D.M && AM.D.M.spec) || null;
const flowOf = (id) => ((viewModel() || {}).flows || []).find((f) => f.id === id);
const machineOf = (entity) => (mdl().states || []).find((s) => s.entity === entity);
const sameTr = (a, b) => (a.from || '') === (b.from || '') && a.to === b.to && (a.on || '') === (b.on || '');
const anyOut = (f) => (!f || f === '*' ? '' : f); // "*" (from any status) is shown as an empty origin
const anyIn = (f) => f || '*';

function setSave(st, msg) {
  SP.save = st; SP.saveMsg = msg || '';
  const s = el('saveInd'); if (!s) return;
  s.className = 'saveind ' + st; s.title = msg || '';
  s.textContent = T({ saved: 'edSaved', saving: 'edSaving', error: 'edError', dirty: 'edDirty' }[st] || 'edSaved');
}
function applyResp(r) {
  if (!r) return;
  if (r.files) SP.files = r.files;
  if (r.model) SP.model = r.model;
  if ('errors' in r) SP.errors = r.errors || [];
  if ('findings' in r) SP.findings = r.findings || [];
  AM.bus.emit('spec', SP);
  refreshAll();
}
async function readJSON(res) { try { return await res.json(); } catch (e) { return {}; } }
const errMsg = (e) => (e && (e.msg || e.message)) || (typeof e === 'string' ? e : '');
const errText = (j, res) => (j && (errMsg(j) || j.error || (j.errors && j.errors[0] && errMsg(j.errors[0])))) || ('HTTP ' + res.status);

async function load(quiet) {
  try {
    const res = await fetch('/spec', { cache: 'no-store' });
    if (!res.ok) { if (!SP.avail) return false; throw new Error('HTTP ' + res.status); }
    const j = await res.json();
    const first = !SP.avail; SP.avail = true;
    if (!quiet || first) setSave('saved');
    applyResp(j);
    return true;
  } catch (e) { if (SP.avail) setSave('error', String(e)); return false; }
}

// send(ops): one request at a time; the inverse of the batch is computed against the model the ops are applied
// to and kept for undo
function send(ops, opt = {}) {
  ops = Array.isArray(ops) ? ops : [ops];
  const run = async () => {
    const inv = [];
    ops.forEach((o) => inv.unshift(...inverse(o, mdl())));
    setSave('saving');
    try {
      const res = await fetch('/spec/ops', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ops }) });
      const j = await readJSON(res);
      if (!res.ok) { setSave('error', errText(j, res)); return false; }
      setSave('saved'); applyResp(j);
      if (opt.record !== false) { SP.undo.push({ ops: inv, redo: ops }); SP.redo = []; }
      paintTools();
      return true;
    } catch (e) { setSave('error', String(e)); return false; }
  };
  busy = busy.then(run, run);
  return busy;
}
async function undo() {
  const u = SP.undo.pop(); if (!u) return;
  const ok = await send(u.ops, { record: false });
  if (ok) SP.redo.push(u); else SP.undo.push(u);
  paintTools();
}
async function redo() {
  const u = SP.redo.pop(); if (!u) return;
  const ok = await send(u.redo, { record: false });
  if (ok) SP.undo.push(u); else SP.redo.push(u);
  paintTools();
}

// inverse(op, model) -> ops that take the op back (computed before the op is applied)
const stepOp = (flow, s, index) => Object.assign({ op: 'add_step', flow, index }, s.planned ? { planned: true } : { ref: s.ref }, s.text ? { text: s.text } : {}, s.ext ? { ext: s.ext } : {});
function inverse(o, m) {
  const st = (m.states || []).find((s) => s.entity === o.entity), tr = st ? st.transitions || [] : [];
  const fl = (m.flows || []).find((f) => f.id === o.flow);
  switch (o.op) {
    case 'add_state': return [{ op: 'remove_state', entity: o.entity, state: o.state }];
    case 'remove_state': {
      const out = [{ op: 'add_state', entity: o.entity, state: o.state }];
      tr.filter((x) => x.from === o.state || x.to === o.state).forEach((x) => out.push({ op: 'add_transition', entity: o.entity, from: x.from, to: x.to, on: x.on, label: x.label }));
      if (st && (st.final || []).includes(o.state)) out.push({ op: 'set_final', entity: o.entity, state: o.state, final: true });
      return out;
    }
    case 'rename_state': return [{ op: 'rename_state', entity: o.entity, from: o.to, to: o.from }];
    case 'set_final': return [{ op: 'set_final', entity: o.entity, state: o.state, final: !(st && (st.final || []).includes(o.state)) }];
    case 'add_transition': return [{ op: 'remove_transition', entity: o.entity, from: o.from, to: o.to, on: o.on }];
    case 'remove_transition': {
      const x = tr.find((y) => y.from === o.from && y.to === o.to && (!o.on || y.on === o.on)) || o;
      return [{ op: 'add_transition', entity: o.entity, from: x.from, to: x.to, on: x.on, label: x.label }];
    }
    case 'add_flow': return [{ op: 'remove_flow', flow: o.id }];
    case 'remove_flow': {
      if (!fl) return [];
      const out = [{ op: 'add_flow', id: fl.id, name: fl.name, actor: fl.actor }];
      if (fl.exit) out.push({ op: 'set_exit', flow: fl.id, exit: fl.exit });
      (fl.steps || []).forEach((s, i) => out.push(stepOp(fl.id, s, i)));
      (fl.alts || []).forEach((a) => out.push({ op: 'add_alt', flow: fl.id, cond: a.cond, target: a.flow, text: a.text }));
      return out;
    }
    case 'add_step': return [{ op: 'remove_step', flow: o.flow, index: o.index != null && o.index >= 0 ? o.index : fl ? fl.steps.length : 0 }];
    case 'remove_step': return fl && fl.steps[o.index] ? [stepOp(o.flow, fl.steps[o.index], o.index)] : [];
    case 'move_step': return [{ op: 'move_step', flow: o.flow, index: o.to_index, to_index: o.index }];
    case 'add_alt': return [{ op: 'remove_alt', flow: o.flow, index: fl ? (fl.alts || []).length : 0 }];
    case 'remove_alt': {
      const alts = fl ? fl.alts || [] : [], a = alts[o.index]; if (!a) return [];
      // alts are only appended: take the later ones off and put them back after this one
      const out = [];
      for (let k = alts.length - 1; k > o.index; k--) out.push({ op: 'remove_alt', flow: o.flow, index: k - 1 });
      out.push({ op: 'add_alt', flow: o.flow, cond: a.cond, target: a.flow, text: a.text });
      alts.slice(o.index + 1).forEach((b) => out.push({ op: 'add_alt', flow: o.flow, cond: b.cond, target: b.flow, text: b.text }));
      return out;
    }
    case 'set_actor': return [{ op: 'set_actor', flow: o.flow, actor: (fl && fl.actor) || '' }];
    case 'set_exit': return [{ op: 'set_exit', flow: o.flow, exit: (fl && fl.exit) || '' }];
    default: return [];
  }
}

// the operations the diagram and the panels use
const identRe = /^[A-Za-z0-9_.-]+$/;
// "event", or "event \"label\"", or a free label
function parseTr(text) {
  const s = (text || '').trim(); if (!s) return {};
  let m = /^([A-Za-z0-9_.-]+)?\s*"(.*)"$/.exec(s);
  if (m) return Object.assign(m[1] ? { on: m[1] } : {}, m[2] ? { label: m[2] } : {});
  m = identRe.test(s);
  return m ? { on: s } : { label: s };
}
SP.do = {
  addState: (entity, state) => send({ op: 'add_state', entity, state }),
  removeState: (entity, state) => send({ op: 'remove_state', entity, state }),
  rename: (entity, from, to) => send({ op: 'rename_state', entity, from, to }),
  setFinal: (entity, state, final) => send({ op: 'set_final', entity, state, final: !!final }),
  addTr: (entity, from, to, text) => send(Object.assign({ op: 'add_transition', entity, from: anyIn(from), to }, parseTr(text))),
  removeTr: (entity, x) => send({ op: 'remove_transition', entity, from: anyIn(x.from), to: x.to, ...(x.ref && x.ref.on ? { on: x.ref.on } : {}) }),
  editTr: (entity, x, text) => {
    const r = x.ref, from = anyIn(x.from);
    return send([{ op: 'remove_transition', entity, from, to: x.to, ...(r.on ? { on: r.on } : {}) }, Object.assign({ op: 'add_transition', entity, from, to: x.to }, parseTr(text))]);
  },
};

/* ---------- inline text field over a container (status names, labels) ---------- */
function ask(parent, cx, cy, init, placeholder, cb) {
  $$q('.ed-ask', parent).forEach((x) => x.remove());
  const r = parent.getBoundingClientRect();
  const inp = AM.h('input', { class: 'ed-ask', type: 'text', placeholder: placeholder || '', autocomplete: 'off', spellcheck: 'false', 'aria-label': placeholder || '' }, parent);
  inp.value = init || '';
  inp.style.left = Math.max(4, Math.min(cx - r.left - 70, r.width - 190)) + 'px';
  inp.style.top = Math.max(4, Math.min(cy - r.top - 14, r.height - 40)) + 'px';
  let done = false;
  const fin = (ok) => { if (done) return; done = true; const v = inp.value.trim(); inp.remove(); if (ok) cb(v); };
  inp.addEventListener('keydown', (e) => { e.stopPropagation(); if (e.key === 'Enter') fin(true); else if (e.key === 'Escape') fin(false); });
  inp.addEventListener('blur', () => fin(false));
  inp.addEventListener('pointerdown', (e) => e.stopPropagation());
  inp.addEventListener('dblclick', (e) => e.stopPropagation());
  setTimeout(() => { inp.focus(); inp.select(); }, 0);
  return inp;
}
function $$q(s, r) { return Array.from((r || document).querySelectorAll(s)); }
SP.ask = ask;

/* ---------- merging spec machines into the status diagram ---------- */
// base: [{ent, states, trs, empty}] from the code; returns copies in which every transition has `st`:
// 'both' (described and in code), 'code' (in code only: undeclared), 'spec' (described, not implemented; fid '').
// `ref` is the transition of the model when it is described (the thing the ops edit).
SP.mergeMachines = (base) => {
  if (!SP.avail || !SP.model) return base;
  const out = base.map((m) => Object.assign({}, m));
  const key = (x) => anyOut(x.from) + '\u0001' + x.to;
  (SP.model.states || []).forEach((sm) => {
    let m = out.find((x) => x.ent === sm.entity) || (sm.code && out.find((x) => x.ent === sm.code));
    if (!m) { m = { ent: sm.entity, states: [], trs: [], empty: false }; out.push(m); }
    m.ent = sm.entity;
    const code = m.trs, codeBy = new Map(code.map((x) => [key(x), x])), seen = new Set(), trs = [];
    (sm.transitions || []).forEach((t) => {
      const k = key(t), c = codeBy.get(k), described = t.source !== 'code' && t.drift !== 'undeclared';
      const st = t.drift === 'undeclared' ? 'code' : t.drift === 'not_implemented' ? 'spec' : c ? 'both' : 'spec';
      seen.add(k);
      trs.push(Object.assign({}, c || {}, { from: anyOut(t.from), to: t.to, fid: t.fid || (c && c.fid) || '', source: st === 'spec' ? 'spec' : (c && c.source) || t.source, st, ref: described ? t : undefined, line: t.line }));
    });
    code.forEach((x) => { if (!seen.has(key(x))) trs.push(Object.assign({}, x, { st: 'code' })); });
    const names = new Set(m.states);
    (sm.states || []).forEach((n) => names.add(n));
    trs.forEach((x) => { if (x.from) names.add(x.from); names.add(x.to); });
    (sm.final || []).forEach((n) => names.add(n));
    Object.assign(m, { trs, states: Array.from(names), final: new Set(sm.final || []), inSpec: true, empty: false, sm });
  });
  return out;
};

// drift: the transitions and steps the check marked, or found here by the same merge
function driftList() {
  const out = [];
  if (!SP.model) return out;
  const base = Object.keys(AM.D.states || {}).map((ent) => {
    const m = AM.D.states[ent] || {};
    return { ent, states: m.states || [], trs: m.transitions || [] };
  });
  SP.mergeMachines(base).forEach((m) => {
    if (!m.inSpec) return;
    const loc = (x) => (x.line ? { path: m.sm.file, line: x.line } : null);
    m.trs.forEach((x) => {
      if (x.st === 'code') out.push({ kind: 'undeclared', entity: m.ent, from: x.from, to: x.to, fid: x.fid, message: (x.from || '*') + ' → ' + x.to });
      else if (x.st === 'spec') out.push({ kind: 'not_implemented', entity: m.ent, from: x.from, to: x.to, message: (x.from || '*') + ' → ' + x.to, loc: loc(x) });
    });
  });
  (SP.model.flows || []).forEach((f) => (f.steps || []).forEach((s, i) => {
    if (s.drift) out.push({ kind: s.drift, flow: f.id, index: i, message: (f.name || f.id) + ': ' + (s.ref || s.text || '?'), loc: s.line ? { path: f.file, line: s.line } : null });
  }));
  (SP.model.journeys || []).forEach((j) => (j.missing || []).forEach((id) => out.push({ kind: 'journey_missing', journey: j.id, message: (j.name || j.id) + ': ' + id, loc: j.line ? { path: j.file, line: j.line } : null })));
  return out;
}

/* ---------- header: toggle, undo/redo, indicator ---------- */
function paintTools() {
  const u = el('undoBtn'), r = el('redoBtn'); if (!u) return;
  u.disabled = !SP.undo.length; r.disabled = !SP.redo.length;
  u.title = T('edUndo'); r.title = T('edRedo');
}
function paintHeader() {
  const b = el('editBtn'); if (!b) return;
  b.hidden = !SP.avail; b.setAttribute('aria-pressed', SP.on); b.title = T('edBtnTip');
  b.querySelector('span').textContent = T('edBtn');
  el('edTools').hidden = !SP.avail || !SP.on;
  setSave(SP.save, SP.saveMsg); paintTools();
}
function setEdit(on) {
  if (!SP.avail) on = false;
  SP.on = !!on;
  document.body.classList.toggle('editing', SP.on);
  if (!SP.on) { SP.tab = 'card'; document.body.classList.remove('ed-wide'); }
  paintHeader(); paintTabs(); AM.bus.emit('edit', SP.on); AM.bus.emit('resize');
  refreshAll();
}

/* ---------- right panel: tabs ---------- */
function mountTabs() {
  const aside = document.getElementById('panel');
  const tabs = AM.h('div', { class: 'etabs', id: 'etabs', role: 'tablist', hidden: '' }, aside);
  aside.insertBefore(tabs, aside.firstChild);
  const pane = AM.h('div', { class: 'epane', id: 'epane', hidden: '' }, aside);
  tabEls = { tabs, pane };
}
function paintTabs() {
  if (!tabEls) return;
  const { tabs, pane } = tabEls, nd = driftList().length;
  tabs.hidden = !SP.on;
  tabs.innerHTML = '';
  [['card', 'edTabCard'], ['flows', 'edTabFlows'], ['text', 'edTabText'], ['drift', 'edTabDrift']].forEach(([id, k]) => {
    const b = AM.h('button', { type: 'button', role: 'tab', 'aria-selected': SP.tab === id }, tabs, esc(T(k)) + (id === 'drift' && nd ? ` <i class="bdg">${nd}</i>` : ''));
    b.onclick = () => setTab(id);
  });
  const card = SP.on && SP.tab === 'card' || !SP.on;
  el('pane').hidden = !card; pane.hidden = card;
  document.body.classList.toggle('ed-wide', SP.on && SP.tab === 'text');
  if (!card) renderTab();
}
function setTab(id, focus) {
  if (!SP.on) setEdit(true);
  const was = SP.tab; SP.tab = id; paintTabs(); AM.bus.emit('resize');
  if (was !== id && id === 'text' && focus) revealLine(focus);
}
function renderTab() {
  const p = tabEls.pane, keep = p.scrollTop;
  if (SP.tab === 'flows') renderFlows(p);
  else if (SP.tab === 'text') renderText(p);
  else if (SP.tab === 'drift') renderDrift(p);
  p.scrollTop = keep;
}
function refreshAll() {
  paintTabs();
  drawGhosts();
  if (AM.dock && AM.dock.refresh) AM.dock.refresh();
}

/* ---------- Scenarios tab ---------- */
const slug = (s) => String(s).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'flow';
const fidOf = (s) => (s.fid && AM.D.fns[s.fid] ? s.fid : s.ref && AM.D.fns[s.ref] ? s.ref : null);
const stepName = (s) => (s.planned ? s.text || T('edPlanned') : fidOf(s) ? AM.fnInfo(fidOf(s)).name : s.ref || s.text || '?');
let FA = { addStep: null, addAlt: null }; // which flow has the open "+ step" / "+ branch" form
function renderFlows(p) {
  p.innerHTML = '';
  const head = AM.h('div', { class: 'ed-head' }, p, `<b>${esc(T('edTabFlows'))}</b>`);
  const add = AM.h('button', { class: 'pbtn', type: 'button' }, head, esc(T('edAddFlow')));
  add.onclick = (e) => ask(p, e.clientX, e.clientY, '', T('edFlowName'), (v) => {
    if (!v) return;
    let id = slug(v); const ids = new Set((mdl().flows || []).map((f) => f.id)); let n = 2; const b = id;
    while (ids.has(id)) id = b + '-' + n++;
    SP.curFlow = id; send({ op: 'add_flow', id, name: v });
  });
  const flows = mdl().flows || [];
  if (!flows.length) AM.h('p', { class: 'loc' }, p, esc(T('edNoFlows')));
  flows.forEach((f) => flowCard(p, f));
}
function flowCard(p, f) {
  const c = AM.h('div', { class: 'ed-flow' + (SP.curFlow === f.id ? ' cur' : ''), 'data-flow': f.id }, p);
  const h = AM.h('div', { class: 'ed-fh' }, c, `<b title="${esc(f.id)}">${esc(f.name || f.id)}</b><span class="ct">${esc(T('edStepsN', (f.steps || []).length))}</span>`);
  h.onclick = () => { SP.curFlow = SP.curFlow === f.id ? null : f.id; drawGhosts(); $$q('.ed-flow', p).forEach((x) => x.classList.toggle('cur', x.dataset.flow === SP.curFlow)); };
  const del = AM.h('button', { class: 'xbtn', type: 'button', title: T('edDel'), 'aria-label': T('edDel') }, h, '×');
  del.onclick = (e) => { e.stopPropagation(); send({ op: 'remove_flow', flow: f.id }); };
  const kv = AM.h('div', { class: 'ed-kv' }, c);
  [['actor', 'edActor', 'set_actor'], ['exit', 'edExit', 'set_exit']].forEach(([field, lk, op]) => {
    const lab = AM.h('label', null, kv, `<span>${esc(T(lk))}</span>`);
    const inp = AM.h('input', { type: 'text', autocomplete: 'off', spellcheck: 'false', 'aria-label': T(lk) }, lab);
    inp.value = f[field] || '';
    inp.addEventListener('keydown', (e) => { e.stopPropagation(); if (e.key === 'Enter') inp.blur(); });
    inp.addEventListener('change', () => { if (inp.value !== (f[field] || '')) send({ op, flow: f.id, [field]: inp.value.trim() }); });
  });
  const ol = AM.h('ol', { class: 'ed-steps' }, c);
  (f.steps || []).forEach((s, i) => {
    const bad = s.drift || (s.ref && !fidOf(s));
    const li = AM.h('li', { class: 'ed-step' + (s.planned ? ' planned' : '') + (bad ? ' bad' : ''), draggable: 'true', 'data-i': i, title: T('edDragTip') }, ol,
      `<span class="grip" aria-hidden="true">⋮⋮</span><span class="nm">${esc(stepName(s))}</span>` +
      (s.planned ? `<span class="q">${esc(T('edPlanTag'))}</span>` : '') + (s.ext ? `<span class="q">${esc(s.ext)}</span>` : '') + (bad ? `<span class="q w">${esc(T('edRefMissing'))}</span>` : ''));
    const x = AM.h('button', { class: 'xbtn', type: 'button', title: T('edDel'), 'aria-label': T('edDel') }, li, '×');
    x.onclick = (e) => { e.stopPropagation(); send({ op: 'remove_step', flow: f.id, index: i }); };
    if (fidOf(s)) li.querySelector('.nm').onclick = () => AM.select({ t: 'fn', id: fidOf(s) });
    li.addEventListener('dragstart', (e) => { e.dataTransfer.setData('text/plain', f.id + ':' + i); e.dataTransfer.effectAllowed = 'move'; li.classList.add('drag'); });
    li.addEventListener('dragend', () => li.classList.remove('drag'));
    li.addEventListener('dragover', (e) => { e.preventDefault(); li.classList.add('over'); });
    li.addEventListener('dragleave', () => li.classList.remove('over'));
    li.addEventListener('drop', (e) => {
      e.preventDefault(); li.classList.remove('over');
      const [fid, from] = (e.dataTransfer.getData('text/plain') || '').split(/:(?=\d+$)/);
      if (fid === f.id && +from !== i) send({ op: 'move_step', flow: f.id, index: +from, to_index: i });
    });
  });
  (f.alts || []).forEach((a, i) => {
    const tgt = [a.flow ? (flowOf(a.flow) || {}).name || a.flow : '', a.text || ''].filter(Boolean).join(' ');
    const li = AM.h('div', { class: 'ed-alt' }, c, `<span class="q">${esc(T('edAddAlt').replace('+ ', ''))}</span><span class="nm">${esc(a.cond || '')} → ${esc(tgt)}</span>`);
    const x = AM.h('button', { class: 'xbtn', type: 'button', title: T('edDel'), 'aria-label': T('edDel') }, li, '×');
    x.onclick = () => send({ op: 'remove_alt', flow: f.id, index: i });
  });
  const bar = AM.h('div', { class: 'ed-bar' }, c);
  const bs = AM.h('button', { class: 'pbtn', type: 'button' }, bar, esc(T('edAddStep')));
  const ba = AM.h('button', { class: 'pbtn', type: 'button' }, bar, esc(T('edAddAlt')));
  bs.onclick = () => { FA.addStep = FA.addStep === f.id ? null : f.id; FA.addAlt = null; renderTab(); };
  ba.onclick = () => { FA.addAlt = FA.addAlt === f.id ? null : f.id; FA.addStep = null; renderTab(); };
  if (FA.addStep === f.id) stepForm(c, f);
  if (FA.addAlt === f.id) altForm(c, f);
}
function stepForm(c, f) {
  const box = AM.h('div', { class: 'ed-form' }, c);
  const inp = AM.h('input', { type: 'search', placeholder: T('edFind'), autocomplete: 'off', spellcheck: 'false', 'aria-label': T('edFind') }, box);
  const list = AM.h('div', { class: 'ed-pick', role: 'listbox' }, box);
  const draw = () => {
    const q = inp.value.trim().toLowerCase(); list.innerHTML = '';
    const pl = AM.h('div', { class: 'ed-opt planned', role: 'option' }, list, `<span class="nm">⌁ ${esc(T('edPlanned'))}${q ? ': ' + esc(inp.value.trim()) : '…'}</span>`);
    pl.onclick = (e) => {
      const go = (v) => { if (v) { FA.addStep = null; send({ op: 'add_step', flow: f.id, planned: true, text: v }); } };
      if (q) go(inp.value.trim()); else ask(box, e.clientX, e.clientY, '', T('edPlannedAsk'), go);
    };
    if (!q) return;
    const hits = (AM.D.fids || Object.keys(AM.D.fns)).map((id) => AM.fnInfo(id)).filter((x) => (x.name + ' ' + (x.mod || '')).toLowerCase().includes(q)).slice(0, 8);
    hits.forEach((x) => {
      const o = AM.h('div', { class: 'ed-opt', role: 'option', title: x.id }, list, `<span class="nm">${esc(x.name)}</span><span class="md">${esc(AM.cutL(AM.short(x.mod || ''), 28))}</span>`);
      o.onclick = () => { FA.addStep = null; send({ op: 'add_step', flow: f.id, ref: x.id }); };
    });
    if (!hits.length) AM.h('div', { class: 'ed-none' }, list, esc(T('edNoMatch')));
  };
  inp.addEventListener('input', draw);
  inp.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'Escape') { FA.addStep = null; renderTab(); }
    else if (e.key === 'Enter') { const o = list.querySelector('.ed-opt:not(.planned)') || list.querySelector('.ed-opt'); if (o) o.click(); }
  });
  draw(); setTimeout(() => inp.focus(), 0);
}
function altForm(c, f) {
  const box = AM.h('div', { class: 'ed-form' }, c);
  const cond = AM.h('input', { type: 'text', placeholder: T('edCond'), autocomplete: 'off', 'aria-label': T('edCond') }, box);
  const sel = AM.h('select', { 'aria-label': T('edToFlow') }, box);
  AM.h('option', { value: '' }, sel, esc(T('edToText')));
  (mdl().flows || []).filter((x) => x.id !== f.id).forEach((x) => AM.h('option', { value: x.id }, sel, esc(T('edToFlow') + ' ' + (x.name || x.id))));
  const txt = AM.h('input', { type: 'text', placeholder: T('edToText'), autocomplete: 'off', 'aria-label': T('edToText') }, box);
  sel.onchange = () => { txt.hidden = !!sel.value; };
  const ok = AM.h('button', { class: 'pbtn primary', type: 'button' }, box, esc(T('edAdd')));
  const go = () => {
    if (!cond.value.trim()) { cond.focus(); return; }
    const op = { op: 'add_alt', flow: f.id, cond: cond.value.trim().replace(/\s+/g, '_') };
    if (sel.value) op.target = sel.value; else op.text = txt.value.trim();
    FA.addAlt = null; send(op);
  };
  ok.onclick = go;
  [cond, txt].forEach((i) => i.addEventListener('keydown', (e) => { e.stopPropagation(); if (e.key === 'Enter') go(); else if (e.key === 'Escape') { FA.addAlt = null; renderTab(); } }));
  setTimeout(() => cond.focus(), 0);
}

/* ---------- Text tab: .arch files with highlighting and per-line errors ---------- */
const KWS = new Set(['states', 'state', 'final', 'flow', 'journey', 'actor', 'exit', 'step', 'alt', 'on', 'plan', 'ext']);
function hlArch(line) {
  let out = '', i = 0;
  const cm = line.search(/(^|\s)(#|\/\/)/);
  const body = cm >= 0 ? line.slice(0, cm + (/\s/.test(line[cm]) ? 1 : 0)) : line, tail = cm >= 0 ? line.slice(body.length) : '';
  const re = /("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')|(->|→|=>|<-)|([A-Za-z_][\w.:-]*)|(\d+)|([{}()[\],:;=|])/g;
  let m;
  while ((m = re.exec(body))) {
    out += esc(body.slice(i, m.index)); i = m.index + m[0].length;
    const s = esc(m[0]);
    if (m[1]) out += `<i class="s">${s}</i>`;
    else if (m[2]) out += `<i class="a">${s}</i>`;
    else if (m[3]) out += KWS.has(m[3].toLowerCase()) ? `<i class="k">${s}</i>` : s;
    else if (m[4]) out += `<i class="n">${s}</i>`;
    else out += `<i class="p">${s}</i>`;
  }
  out += esc(body.slice(i));
  if (tail) out += `<i class="c">${esc(tail)}</i>`;
  return out;
}
const TX = { path: null, text: '', saved: '', errs: [], ta: null, pre: null, gut: null, errBox: null, focusLine: 0 };
const fileOf = (path) => SP.files.find((f) => f.path === path);
function renderText(p) {
  if (!SP.files.length) { p.innerHTML = `<p class="loc" style="padding:16px">${esc(T('edNoSpec'))}</p>`; return; }
  if (!TX.path || !fileOf(TX.path)) { TX.path = SP.files[0].path; dirtyText = false; }
  const f = fileOf(TX.path);
  if (!dirtyText) { TX.text = f.text; TX.saved = f.text; }
  else if (f.text !== TX.saved) { TX.saved = f.text; TX.note = true; }
  p.innerHTML = '';
  const head = AM.h('div', { class: 'ed-head' }, p);
  const sel = AM.h('select', { 'aria-label': T('edFiles') }, head);
  SP.files.forEach((x) => { const o = AM.h('option', { value: x.path }, sel, esc(x.path)); if (x.path === TX.path) o.selected = true; });
  sel.onchange = () => { TX.path = sel.value; dirtyText = false; renderTab(); };
  const sv = AM.h('button', { class: 'pbtn primary', type: 'button' }, head, esc(T('edSave')));
  sv.onclick = saveText;
  const box = AM.h('div', { class: 'ed-code' }, p);
  const gut = AM.h('div', { class: 'gut', 'aria-hidden': 'true' }, box);
  const sc = AM.h('div', { class: 'ed-sc' }, box);
  const pre = AM.h('pre', { class: 'hl', 'aria-hidden': 'true' }, sc);
  const ta = AM.h('textarea', { spellcheck: 'false', autocomplete: 'off', wrap: 'off', 'aria-label': TX.path }, sc);
  ta.value = TX.text;
  Object.assign(TX, { ta, pre, gut });
  const errBox = AM.h('div', { class: 'ed-errs' }, p);
  TX.errBox = errBox;
  paintText();
  ta.addEventListener('input', () => { TX.text = ta.value; dirtyText = ta.value !== TX.saved; setSave(dirtyText ? 'dirty' : 'saved'); paintText(); });
  ta.addEventListener('scroll', () => { pre.style.transform = `translate(${-ta.scrollLeft}px,${-ta.scrollTop}px)`; gut.style.transform = `translateY(${-ta.scrollTop}px)`; });
  ta.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') { e.preventDefault(); saveText(); }
    else if (e.key === 'Tab') { e.preventDefault(); document.execCommand('insertText', false, '  '); }
  });
  if (TX.note) { TX.note = false; AM.h('div', { class: 'ed-note' }, p, esc(T('edChangedSrv'))); }
  if (TX.focusLine) { const l = TX.focusLine; TX.focusLine = 0; setTimeout(() => scrollToLine(l), 0); }
}
function errsFor(path) {
  return (SP.errors || []).concat(TX.errs || []).filter((e) => e && e.line && (!(e.file || e.path) || (e.file || e.path) === path));
}
function paintText() {
  if (!TX.pre) return;
  const lines = TX.text.split('\n'), errs = errsFor(TX.path), bad = new Set(errs.map((e) => e.line));
  TX.pre.innerHTML = lines.map((l, i) => `<div class="ln${bad.has(i + 1) ? ' err' : ''}">${hlArch(l) || ' '}</div>`).join('');
  TX.gut.innerHTML = lines.map((l, i) => `<div class="${bad.has(i + 1) ? 'err' : ''}">${i + 1}</div>`).join('');
  TX.errBox.innerHTML = errs.length ? `<h3>${esc(T('edErrors'))} (${errs.length})</h3>` + errs.map((e) => `<div class="er" data-l="${e.line}"><b>${e.line}${e.col ? ':' + e.col : ''}</b> ${esc(errMsg(e))}</div>`).join('') : '';
  $$q('.er', TX.errBox).forEach((d) => { d.onclick = () => scrollToLine(+d.dataset.l); });
}
function scrollToLine(n) {
  if (!TX.ta) return;
  const lh = parseFloat(getComputedStyle(TX.ta).lineHeight) || 18;
  TX.ta.scrollTop = Math.max(0, (n - 3) * lh);
  const pos = TX.text.split('\n').slice(0, n - 1).join('\n').length + (n > 1 ? 1 : 0);
  TX.ta.focus({ preventScroll: true }); TX.ta.setSelectionRange(pos, pos);
}
function revealLine(loc) {
  if (!loc) return;
  if (loc.path && fileOf(loc.path) && loc.path !== TX.path) { TX.path = loc.path; dirtyText = false; }
  TX.focusLine = loc.line || 1; renderTab();
}
async function saveText() {
  if (!TX.path) return;
  setSave('saving');
  try {
    const res = await fetch('/spec/file', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: TX.path, text: TX.text }) });
    const j = await readJSON(res);
    if (res.status === 422 || (res.ok && j.errors && j.errors.length && !j.model)) {
      TX.errs = (j.errors || (j.line ? [j] : [])).map((e) => Object.assign({ file: TX.path }, e));
      setSave('error', errText(j, res)); paintText();
      if (TX.errs[0]) scrollToLine(TX.errs[0].line);
      return;
    }
    if (!res.ok) { setSave('error', errText(j, res)); return; }
    TX.errs = []; dirtyText = false; TX.saved = TX.text;
    setSave('saved');
    if (j.model || j.files) {
      if (j.files) SP.files = j.files.map((f) => (f.path === TX.path ? Object.assign({}, f, { text: TX.text }) : f));
      applyResp(Object.assign({}, j, { files: SP.files }));
    } else await load(true);
    SP.undo = []; SP.redo = []; paintTools(); // a hand edit of the text cannot be undone operation by operation
  } catch (e) { setSave('error', String(e)); }
}

/* ---------- Drift tab ---------- */
function renderDrift(p) {
  p.innerHTML = '';
  AM.h('div', { class: 'ed-head' }, p, `<b>${esc(T('edDriftTitle'))}</b>`);
  const list = driftList();
  if (!list.length && !SP.findings.length) { AM.h('p', { class: 'loc', style: 'padding:0 16px' }, p, esc(T('edDriftNone'))); return; }
  list.forEach((d) => {
    const lab = { undeclared: 'edDCode', not_implemented: 'edDSpec', missing: 'edDRef', ambiguous: 'edDrift_ambiguous', unreachable: 'edDrift_unreachable', journey_missing: 'edDRef' }[d.kind] || 'edDOther';
    const row = AM.h('div', { class: 'ed-drift ' + d.kind }, p,
      `<span class="q">${esc(T(lab))}</span><span class="nm">${esc((d.entity ? d.entity + ': ' : '') + (d.message || ((d.from || '*') + ' → ' + d.to)))}</span>`);
    const bar = AM.h('div', { class: 'ed-bar' }, row);
    if (d.entity) AM.h('button', { class: 'pbtn', type: 'button' }, bar, esc(T('edOnMap'))).onclick = () => { AM.setView('status'); AM.bus.emit('status:focus', { machine: d.entity, from: d.from, to: d.to }); };
    else if (d.fid && AM.D.fns[d.fid]) AM.h('button', { class: 'pbtn', type: 'button' }, bar, esc(T('edOnMap'))).onclick = () => AM.map.jumpToFn(d.fid, 2);
    if (d.flow) AM.h('button', { class: 'pbtn', type: 'button' }, bar, esc(T('edTabFlows'))).onclick = () => { SP.curFlow = d.flow; setTab('flows'); };
    if (d.loc) AM.h('button', { class: 'pbtn', type: 'button' }, bar, esc(T('edInText'))).onclick = () => setTab('text', d.loc);
    else if (SP.files.length) AM.h('button', { class: 'pbtn', type: 'button' }, bar, esc(T('edInText'))).onclick = () => setTab('text', { line: findLine(d) });
    if (d.kind === 'undeclared' && d.entity) AM.h('button', { class: 'pbtn primary', type: 'button' }, bar, esc(T('edAccept'))).onclick = () => SP.do.addTr(d.entity, d.from, d.to, '');
  });
  // lint findings of the spec (GET /spec "findings"), except the ones drawn above
  const rest = SP.findings.filter((x) => x.code !== 'undeclared-transition' && x.code !== 'not-implemented');
  if (rest.length) {
    AM.h('h3', { style: 'padding:0 16px' }, p, 'lint');
    rest.forEach((x) => {
      const row = AM.h('div', { class: 'ed-drift ' + (x.severity === 'error' ? 'bad_ref' : '') }, p, `<span class="q">${esc(x.severity + ' · ' + x.code)}</span><span class="nm">${esc(x.msg)}</span>`);
      if (x.file && /^arch\//.test(x.file) && x.line) AM.h('button', { class: 'pbtn', type: 'button' }, AM.h('div', { class: 'ed-bar' }, row)).onclick = () => setTab('text', { path: x.file, line: x.line });
    });
    p.querySelectorAll('.ed-bar > button:empty').forEach((b) => { b.textContent = T('edInText'); });
  }
}
// the first line of the spec text that mentions the drift item (a guess when the server gave no position)
function findLine(d) {
  const needles = [d.to, d.from, d.entity, d.flow, d.journey].filter(Boolean);
  for (const f of SP.files) {
    const ls = f.text.split('\n');
    for (const n of needles) { const i = ls.findIndex((l) => l.includes(n)); if (i >= 0) { TX.path = f.path; return i + 1; } }
  }
  return 1;
}

/* ---------- planned steps on the map (ghost chips) and in the sequence ---------- */
function plannedSteps() {
  const fl = (viewModel() || {}).flows || [], run = AM.state.run, rid = run && run.segs && run.segs[0] && run.segs[0].flow && run.segs[0].flow.id;
  const only = SP.curFlow || rid;
  const out = [];
  fl.forEach((f) => {
    if (only && fl.some((x) => x.id === only) && f.id !== only) return;
    (f.steps || []).forEach((s) => { if (s.planned) out.push({ flow: f, step: s }); });
  });
  return out;
}
function drawGhosts() {
  [AM.map.system, AM.map.modules].forEach((sc) => { if (sc && sc.g) Array.from(sc.g.querySelectorAll(':scope > .ghosts')).forEach((x) => x.remove()); });
  if (!viewModel()) return;
  const list = plannedSteps(); if (!list.length) return;
  const place = (sc, anchorOf, fallback) => {
    if (!sc || !sc.g) return;
    const g = AM.svg('g', { class: 'ghosts' }, sc.g), used = new Map();
    list.forEach(({ flow, step }) => {
      const a = anchorOf(step), bb = a || fallback(); if (!bb) return;
      const k = Math.round(bb.x) + ':' + Math.round(bb.y), n = used.get(k) || 0; used.set(k, n + 1);
      const text = AM.cut('⌁ ' + (step.text || T('edPlanned1')) + (step.ext ? ' · ' + step.ext : ''), 34), w = Math.max(90, text.length * 6.6 + 18);
      const x = bb.x + (a ? 6 : n * (w + 10)), y = bb.y + bb.h + 8 + (a ? n * 28 : 0);
      const c = AM.svg('g', { class: 'ghost', tabindex: 0, role: 'button' }, g);
      AM.svg('rect', { x, y, width: w, height: 22, rx: 11 }, c);
      AM.svg('text', { x: x + 10, y: y + 15 }, c, text);
      AM.svg('title', null, c, `${flow.name || flow.id}\n${T('edPlanned1')}: ${step.text || ''}${step.ext ? ' · ' + step.ext : ''}`);
      c.addEventListener('click', (e) => { e.stopPropagation(); SP.curFlow = flow.id; setTab('flows'); });
    });
  };
  const bbox = (n) => { const e = n && (n.el || n); try { const b = e && e.getBBox ? e.getBBox() : null; return b && { x: b.x, y: b.y, w: b.width, h: b.height }; } catch (er) { return null; } };
  const ext = (sc) => (sc && sc.extent ? { x: sc.extent.x, y: sc.extent.y + sc.extent.h - 52, w: sc.extent.w, h: 22 } : null);
  place(AM.map.system, () => (AM.map.system && AM.map.system.nodes.svc ? bbox(AM.map.system.nodes.svc) : null), () => ext(AM.map.system));
  place(AM.map.modules, (s) => {
    if (!s.module) return null;
    const u = AM.map.unitOf(s.module); const nd = AM.map.modules && AM.map.modules.nodes && AM.map.modules.nodes[u];
    return bbox(nd);
  }, () => ext(AM.map.modules));
}
// called by views/sequence.js after the diagram body was drawn; returns the added height
SP.seqPlanned = (c) => {
  if (!viewModel()) return 0;
  const run = AM.state.run, rid = run && run.segs && run.segs[0] && run.segs[0].flow && run.segs[0].flow.id;
  const f = rid && flowOf(rid); if (!f) return 0;
  const pl = (f.steps || []).filter((s) => s.planned); if (!pl.length) return 0;
  const parts = c.parts, a = parts[0], base = parts.find((p) => p.kind === 'svc') || parts[1] || a;
  const g = AM.svg('g', { class: 'seq-planned' }, c.svg);
  let y = c.Ht;
  pl.forEach((s) => {
    const tgt = (s.module && parts.find((p) => p.key === s.module || p.key === 'mod:' + s.module || String(p.key).endsWith(s.module))) || base;
    const x1 = c.X[a.key], x2 = c.X[tgt.key] === x1 ? x1 + 60 : c.X[tgt.key];
    AM.svg('line', { x1, y1: y + 20, x2, y2: y + 20, class: 'pl-line', 'marker-end': 'url(#arrM)' }, g);
    const lab = AM.svg('text', { x: (x1 + x2) / 2, y: y + 14, 'text-anchor': 'middle', class: 'pl-lab' }, g, AM.cut('⌁ ' + (s.text || T('edPlanned1')) + (s.ext ? ' · ' + s.ext : ''), 44));
    AM.svg('title', null, lab, f.name || f.id);
    y += 34;
  });
  const H2 = y + 14;
  c.svg.setAttribute('viewBox', `0 0 ${c.W} ${H2}`); c.svg.setAttribute('height', H2 * c.sZ);
  return H2 - c.Ht;
};

/* ---------- start ---------- */
function mount() {
  const b = el('editBtn'); if (!b) return;
  b.onclick = () => setEdit(!SP.on);
  el('undoBtn').onclick = undo; el('redoBtn').onclick = redo;
  mountTabs();
  window.addEventListener('keydown', (e) => {
    if (!SP.on) return;
    const tg = e.target.tagName;
    if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT') return;
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'z') { e.preventDefault(); e.stopImmediatePropagation(); if (e.shiftKey) redo(); else undo(); }
    else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') { e.preventDefault(); if (SP.tab === 'text') saveText(); }
  }, true);
  if (AM.D.live) {
    load().then((ok) => {
      if (!ok) return;
      paintHeader();
      if (AM.D.live.events) {
        const es = new EventSource(AM.D.live.events);
        let t0 = 0;
        es.onmessage = (ev) => { let u; try { u = JSON.parse(ev.data); } catch (er) { return; } if (u.spec && SP.save !== 'saving') { clearTimeout(t0); t0 = setTimeout(() => load(true), 150); } };
      }
      if (new URLSearchParams(location.search).get('edit') === '1') setEdit(true);
    });
  }
}
AM.bus.on('lang', () => { paintHeader(); paintTabs(); });
AM.bus.on('live', () => { if (SP.avail) load(true); });
AM.bus.on('level', () => drawGhosts());
AM.bus.on('flow', () => drawGhosts());
AM.bus.on('live', () => { drawGhosts(); if (AM.dock && AM.dock.refresh) AM.dock.refresh(); });
AM.bus.on('theme', () => {});
Object.assign(SP, { setEdit, send, doUndo: undo, doRedo: redo, load, setTab, driftList });
setTimeout(mount, 0);
