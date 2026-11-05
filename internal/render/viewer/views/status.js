/* views/status.js — state machines: a diagram of statuses and the functions that switch them.
 * Data: AM.D.states = { entity: {states, transitions: [{from, to, fid, source, confirmed, label, on, drift}], doc, docs, final} }.
 * Everything comes from the code; annotations only caption or hide. planMachine() decides where things go:
 * the main path (the longest chain from the initial status to a successful final) in one line, side statuses
 * above it (failure-like ones under it), finals that are not on the main path in a band of their own at the
 * bottom, statuses the code never moves into on a shelf "no entry in the code". Parallel transitions between one
 * pair are one arrow "×N"; when ≥ GROUP statuses lead into one final, one bracket and one arrow replace the N arcs.
 * drawMachine() draws a plan — the Statuses view and the states frames of a page (views/pages.js) share it. */
const ruN = (n, a, b, c) => { const x = Math.abs(n) % 100, y = x % 10; return x > 10 && x < 20 ? c : y === 1 ? a : y >= 2 && y <= 4 ? b : c; };
AM.addI18n({
  ru: {
    stTitle: 'Статусы', stMachines: 'Машины состояний', stCounts: (s, t) => s + ' ст. · ' + t + ' пер.',
    stHeadCounts: (s, t, f) => s + ' ' + ruN(s, 'статус', 'статуса', 'статусов') + ' · ' + t + ' ' + ruN(t, 'переход', 'перехода', 'переходов') + ' · ' + f + ' ' + ruN(f, 'функция', 'функции', 'функций'),
    stShowEmpty: (n) => 'показать пустые (' + n + ')', stHideEmpty: 'скрыть пустые', stNone: 'В коде не найдено машин состояний.',
    stNoTrans: 'Для этой машины не найдено переходов.', stAny: 'любой', stAnyN: (n) => 'из любого' + (n > 1 ? ' ×' + n : ''), stFit: 'Вписать',
    stLeg: { main: 'главный путь', side: 'переход', group: 'из многих в один', any: 'из любого статуса', arch: 'только в .arch' },
    stMachine: 'машина состояний', stStatus: 'Статус', stInitial: 'начальный', stFinal: 'конечный',
    stKind: { ok: 'финал: успех', fail: 'финал: отказ', expired: 'финал: истёк', neutral: 'финал' },
    stRole: { main: 'главный путь', side: 'побочный', noentry: 'нет входа в коде' },
    stHowIn: 'Как попадают', stHowOut: 'Как выходят', stFns: 'Функции', stNoDoc: 'Описания у константы в коде нет.',
    stFinals: 'Финалы', stNoEntry: 'Нет входа в коде', stNoEntryTip: 'ни одна функция не переводит в этот статус',
    stCreate: 'создание', stFrom: 'из', stTo: 'в', stFromMany: (l, n) => 'из: ' + l + (n ? ' +' + n : ''),
    stSrc: { code: 'код', annotation: 'аннотация', spec: '.arch' }, stOk: 'подтверждено', stArchOnly: 'только в .arch, функции нет',
    stTrFrom: (n) => 'из ' + n + ' ' + ruN(n, 'статуса', 'статусов', 'статусов'),
    stDbl: 'клик по стрелке — функции перехода · двойной клик — функция на карте · Esc — закрыть',
    stEmptyMain: 'Начальный статус в коде не найден',
    stVia: 'передаёт статус в', stInsert: 'новая запись сразу в этом статусе',
  },
  en: {
    stTitle: 'Statuses', stMachines: 'State machines', stCounts: (s, t) => s + ' st. · ' + t + ' tr.',
    stHeadCounts: (s, t, f) => s + ' status' + (s === 1 ? '' : 'es') + ' · ' + t + ' transition' + (t === 1 ? '' : 's') + ' · ' + f + ' function' + (f === 1 ? '' : 's'),
    stShowEmpty: (n) => 'show empty (' + n + ')', stHideEmpty: 'hide empty', stNone: 'No state machines found in the code.',
    stNoTrans: 'No transitions found for this machine.', stAny: 'any', stAnyN: (n) => 'from any' + (n > 1 ? ' ×' + n : ''), stFit: 'Fit',
    stLeg: { main: 'main path', side: 'transition', group: 'many into one', any: 'from any status', arch: 'only in .arch' },
    stMachine: 'state machine', stStatus: 'Status', stInitial: 'initial', stFinal: 'final',
    stKind: { ok: 'final: success', fail: 'final: failure', expired: 'final: expired', neutral: 'final' },
    stRole: { main: 'main path', side: 'side', noentry: 'no entry in the code' },
    stHowIn: 'How it is entered', stHowOut: 'How it is left', stFns: 'Functions', stNoDoc: 'The constant has no comment in the code.',
    stFinals: 'Finals', stNoEntry: 'No entry in the code', stNoEntryTip: 'no function moves into this status',
    stCreate: 'creation', stFrom: 'from', stTo: 'to', stFromMany: (l, n) => 'from: ' + l + (n ? ' +' + n : ''),
    stSrc: { code: 'code', annotation: 'annotation', spec: '.arch' }, stOk: 'confirmed', stArchOnly: 'only in .arch, no function',
    stTrFrom: (n) => 'from ' + n + ' statuses',
    stDbl: 'click an arrow — the functions of the transition · double click — the function on the map · Esc — close',
    stEmptyMain: 'No initial status found in the code',
    stVia: 'hands the status to', stInsert: 'a new record born in this status',
  },
});

AM.registerView({
  id: 'status',
  order: 40,
  title: () => AM.t('stTitle'),
  mount(el, ctx) { mountStatus(el, ctx); },
  show(ctx) { if (V) { if (V.pendingEnt) { V.ent = V.pendingEnt; V.pendingEnt = null; } renderAll(); } },
  onStep(step) { onStepStatus(step); },
  onSelect() { paintSel(); },
  onLang() { if (V) renderAll(); },
  onData() { if (V) { if (!machines().some((m) => m.ent === V.ent)) V.ent = null; renderAll(); } },
});

let V = null; // {el, wrap, svg, cam, ent, showEmpty, hot, g:{nodes, edges, m, plan, bounds}}

const S = () => AM.D.states || {};
function machines() {
  const base = Object.keys(S()).map((ent) => {
    const m = S()[ent] || {}, trs = m.transitions || [];
    const names = new Set(m.states || []);
    trs.forEach((x) => { if (x.from) names.add(x.from); if (x.to) names.add(x.to); });
    return { ent, states: Array.from(names), trs: trs.map((x) => (x.drift ? Object.assign({}, x, { st: x.drift === 'undeclared' ? 'code' : 'spec' }) : x)), empty: names.size < 2 || !trs.length, doc: m.doc || '', docs: m.docs || {}, codeFinal: m.final || [] };
  });
  // under `archmap serve` the described machines (views/editor.js, AM.spec) are merged in: each transition gets `st`
  return (AM.spec ? AM.spec.mergeMachines(base) : base)
    .sort((a, b) => b.trs.length - a.trs.length || b.states.length - a.states.length || a.ent.localeCompare(b.ent));
}
const editing = () => !!(AM.spec && AM.spec.on);
const trKey = (m, x) => m.ent + '\u0001' + (x.from || '') + '\u0001' + x.to + '\u0001' + (x.on || '');
const specLabel = (x) => (x.ref ? [x.ref.on, x.ref.label && '"' + x.ref.label + '"'].filter(Boolean).join(' ') : '');
const visibleMachines = () => {
  const all = machines(), good = all.filter((m) => !m.empty);
  return V.showEmpty || !good.length ? all : good;
};
const fnShort = (fid) => {
  const f = fid && AM.D.fns && AM.D.fns[fid] ? AM.fnInfo(fid) : null;
  const n = f ? f.name : String(fid || '').split(/[#:]/).pop();
  return String(n).split('.').pop();
};

/* ---------- human captions: from the code (arch:on / arch:step / scenario step captions), else the function ---------- */
// the clause a caption starts with: "Гость отсканировал QR: заказ переходит…" → "Гость отсканировал QR"
function clause(s) {
  s = String(s || '').replace(/\s+/g, ' ').trim();
  const m = s.match(/^(.{10,}?)(?::\s|;\s|\s—\s|\.\s)/);
  if (m) s = m[1];
  s = s.replace(/[.;:,]$/, '');
  if (s.length > 36) { const c = s.slice(0, 36), i = c.lastIndexOf(' '); s = (i > 18 ? c.slice(0, i) : c).replace(/[,;:]$/, '') + '…'; }
  return s;
}
const wordRe = (w) => new RegExp('(^|[^\\p{L}\\p{N}_])' + String(w).replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '($|[^\\p{L}\\p{N}_])', 'iu');
// trText(x): the human caption of one transition or '' — the .arch label, an arch:on of the function that names the
// target status (its condition: "касса приняла заказ"), the arch:step of the function, a scenario caption of it
function trText(x) {
  if (x.label) return x.label;
  if (x.ref && x.ref.label) return x.ref.label;
  const f = x.fid && AM.D.fns ? AM.D.fns[x.fid] : null, a = f && f.arch;
  if (a) {
    const re = wordRe(x.to);
    const on = (a.on || []).find((o) => re.test(o.cond || '') || re.test(o.text || ''));
    if (on) return clause(re.test(on.cond || '') ? on.text || on.cond : on.cond);
    const st = (a.steps || []).find((s) => s.text);
    if (st) return clause(st.text);
  }
  const M = AM.D.M || {};
  for (const fl of M.flows || []) if (fl.labels && x.fid && fl.labels[x.fid]) return clause(fl.labels[x.fid]);
  return '';
}
// the full caption (for the inspector): arch:step text as written
function trFull(x) {
  if (x.label) return x.label;
  const f = x.fid && AM.D.fns ? AM.D.fns[x.fid] : null, a = f && f.arch;
  const st = a && (a.steps || []).find((s) => s.text);
  return st ? st.text : trText(x);
}

/* plan:begin — pure: no DOM, no AM (internal/render/stplan_test.go runs it in node) */
const PSEUDO = /^(нет|none|nil|null|∅|-|start|начало|init)$/i;
const K_FAIL = /(cancel|fail|reject|error|invalid|denied|declin|abort|refus|block|broken|bounced|lost|отмен|ошиб|отказ)/i;
const K_EXP = /(expir|timeout|timed_?out|stale|overdue|истёк|истек|просроч)/i;
const K_OK = /(^|_)(done|success|succeeded|paid|completed?|created|processed|delivered|confirmed|accepted|approved|finished|linked|closed|resolved|fulfilled|settled|published|sent|shipped|ok)$/i;
const K_NOT = /(^|_)(not|no|non|un)_|^(not|non|un)[A-Z]/; // not_linked is no success
const K_FN_FAIL =/(fail|cancel|reject|error|abort|expire)/i;
// planMachine(m, o): where every status and transition of machine m goes.
// m: {states, trs: [{from, to, fid, source, st, …}], final?: Set|array, codeFinal?: array}
// o: {text(x) → caption or '', group: min sources of one bracket (3), short(fid) → name}
// → {pseudo, initial, main, side: [{n, col, below}], finals: [{n, col, kind}], loose, noEntry, kind, terminal,
//    edges: [{from, to, trs, fids, n, text, human, cls}], groups: [{to, from, edges, text}], any: {to: [trs]}}
function planMachine(m, o) {
  o = o || {};
  const G = o.group || 3, textOf = o.text || (() => ''), shortOf = o.short || ((f) => String(f || '').split(/[.:#/]/).pop());
  const all = Array.from(new Set((m.states || []).concat(...(m.trs || []).map((x) => [x.from, x.to])).filter(Boolean)));
  const hasIn = (n) => (m.trs || []).some((x) => x.to === n && x.from && x.from !== n);
  const pseudo = all.filter((n) => PSEUDO.test(n) && !hasIn(n) && (m.trs || []).some((x) => x.from === n));
  const isP = new Set(pseudo);
  // a transition "from any" that the same function also makes from a known status adds nothing
  const trs = (m.trs || []).filter((x) => x.to && (x.from || !(m.trs || []).some((y) => y.from && y.fid && y.fid === x.fid && y.to === x.to)));
  const names = all.filter((n) => !isP.has(n));
  const out = {}, inn = {};
  names.forEach((n) => { out[n] = new Set(); inn[n] = new Set(); });
  const anyIn = {}, create = {};
  trs.forEach((x) => {
    // a creation: from the pseudo status "нет" (arch:state), or a new row / object born in the status (insert)
    if ((!x.from && x.insert) || isP.has(x.from)) { if (inn[x.to]) (create[x.to] = create[x.to] || []).push(x); return; }
    if (!x.from) { if (inn[x.to]) (anyIn[x.to] = anyIn[x.to] || []).push(x); return; }
    if (x.from === x.to || !out[x.from] || !inn[x.to]) return;
    out[x.from].add(x.to); inn[x.to].add(x.from);
  });
  const finalSet = new Set([].concat(m.final ? Array.from(m.final) : [], m.codeFinal || []));
  const entered = (n) => inn[n].size > 0 || !!anyIn[n] || !!create[n];
  // initial: what the code creates, else what nothing leads into but leads somewhere
  // (a status nothing leads into but that leads on starts the machine too — unless the creation is declared
  // with "нет -> …": then such a status is one the code never enters)
  const declared = trs.some((x) => isP.has(x.from));
  const initial = names.filter((n) => create[n] || (!declared && !inn[n].size && !anyIn[n] && out[n].size));
  const isInit = new Set(initial);
  const terminal = new Set(names.filter((n) => entered(n) && (!out[n].size || finalSet.has(n))));
  const kind = {};
  names.forEach((n) => {
    let k = K_EXP.test(n) ? 'expired' : K_FAIL.test(n) ? 'fail' : K_OK.test(n) && !K_NOT.test(n) ? 'ok' : 'neutral';
    if (k === 'neutral' && terminal.has(n)) { // a final whose every function is a failure handler
      const fs = trs.filter((x) => x.to === n && x.fid).map((x) => shortOf(x.fid));
      if (fs.length && fs.every((f) => K_FN_FAIL.test(f))) k = fs.every((f) => /expire/i.test(f)) ? 'expired' : 'fail';
    }
    kind[n] = k;
  });
  const bad = (n) => kind[n] === 'fail' || kind[n] === 'expired';
  const tier = (n) => (bad(n) ? 0 : terminal.has(n) ? (kind[n] === 'ok' ? 4 : 3) : kind[n] === 'ok' ? 2 : 1);
  // the main path: the longest simple path from an initial status to the best end (tier, then length)
  const order = {}; names.forEach((n, i) => (order[n] = i));
  let best = null, budget = 60000;
  const better = (p) => {
    if (!best) return true;
    const a = tier(p[p.length - 1]), b = tier(best[best.length - 1]);
    return a !== b ? a > b : p.length !== best.length ? p.length > best.length : false;
  };
  const dfs = (p, seen, strict) => {
    if (--budget < 0) return;
    const last = p[p.length - 1];
    if (better(p)) best = p.slice();
    Array.from(out[last]).sort((a, b) => order[a] - order[b]).forEach((k) => {
      if (seen.has(k) || (strict && bad(k))) return; // strict: the main path does not pass through failures
      seen.add(k); p.push(k); dfs(p, seen, strict); p.pop(); seen.delete(k);
    });
  };
  initial.forEach((s) => dfs([s], new Set([s]), true));
  const strict = best && tier(best[best.length - 1]) > 0 ? best : null;
  // through failure-like statuses only when that reaches a better end (or the strict path is a lone status)
  best = null; budget = 60000;
  initial.forEach((s) => dfs([s], new Set([s]), false));
  const anyway = best;
  const tEnd = (p) => (p ? tier(p[p.length - 1]) : -1);
  const main = (strict && !(tEnd(anyway) > tEnd(strict) || (strict.length === 1 && anyway && anyway.length > 1 && tEnd(anyway) > 0)) ? strict : anyway) || [];
  const onMain = {}; main.forEach((n, i) => (onMain[n] = i));
  const loose = names.filter((n) => onMain[n] == null && !entered(n) && !out[n].size && !isInit.has(n));
  const isLoose = new Set(loose);
  const noEntry = names.filter((n) => !isInit.has(n) && !entered(n));
  const finalsN = names.filter((n) => onMain[n] == null && terminal.has(n));
  const isFin = new Set(finalsN);
  const sideN = names.filter((n) => onMain[n] == null && !isFin.has(n) && !isLoose.has(n));
  // columns (in units of main-path steps): side statuses by their main neighbours, finals by their sources
  const col = {}; main.forEach((n, i) => (col[n] = i));
  const nb = (n) => Array.from(inn[n]).concat(Array.from(out[n]));
  for (let it = 0; it < 3; it++) sideN.forEach((n) => {
    const c = nb(n).filter((k) => col[k] != null && k !== n).map((k) => col[k] + (out[k].has(n) && onMain[k] != null ? 0.5 : -0.5));
    if (c.length) col[n] = c.reduce((s, v) => s + v, 0) / c.length;
  });
  sideN.forEach((n, i) => { if (col[n] == null) col[n] = main.length + i; });
  finalsN.forEach((n) => {
    const c = Array.from(inn[n]).filter((k) => col[k] != null).map((k) => col[k]);
    col[n] = c.length ? c.reduce((s, v) => s + v, 0) / c.length : main.length;
  });
  const KO = { ok: 0, neutral: 1, expired: 2, fail: 3 };
  finalsN.sort((a, b) => col[a] - col[b] || KO[kind[a]] - KO[kind[b]] || order[a] - order[b]);
  // edges: one per pair, parallel transitions glued
  const byPair = new Map();
  trs.forEach((x) => {
    if (!x.from) return;
    const k = x.from + '\u0001' + x.to;
    if (!byPair.has(k)) byPair.set(k, { from: x.from, to: x.to, trs: [] });
    byPair.get(k).trs.push(x);
  });
  const edges = Array.from(byPair.values()).map((e) => {
    const fids = Array.from(new Set(e.trs.map((x) => x.fid).filter(Boolean)));
    const texts = e.trs.map(textOf).filter(Boolean);
    const human = texts.length > 0;
    const cls = isP.has(e.from) ? 'start' : e.from === e.to ? 'loop'
      : onMain[e.from] != null && onMain[e.to] != null ? (onMain[e.to] === onMain[e.from] + 1 ? 'main' : onMain[e.to] > onMain[e.from] ? 'skip' : 'back')
      : isFin.has(e.to) ? 'final' : 'side';
    return Object.assign(e, { fids, n: Math.max(fids.length, 1), text: human ? texts[0] : fids.length ? shortOf(fids[0]) : '', human, cls, archOnly: e.trs.every((x) => !x.fid) });
  });
  // many into one: ≥ G statuses lead into one final — one bracket instead of N arcs
  const groups = [];
  finalsN.forEach((f) => {
    const es = edges.filter((e) => e.cls === 'final' && e.to === f);
    if (new Set(es.map((e) => e.from)).size < G) return;
    es.sort((a, b) => col[a.from] - col[b.from]);
    es.forEach((e) => (e.grouped = true));
    const cnt = {}; es.forEach((e) => { if (e.human) cnt[e.text] = (cnt[e.text] || 0) + 1; });
    const text = Object.keys(cnt).sort((a, b) => cnt[b] - cnt[a])[0] || '';
    groups.push({ to: f, from: es.map((e) => e.from), edges: es, text });
  });
  const side = sideN.map((n) => ({ n, col: col[n], below: bad(n) }));
  const finals = finalsN.map((n) => ({ n, col: col[n], kind: kind[n] }));
  const term = Array.from(terminal);
  return { pseudo, initial, main, side, finals, loose, noEntry, kind, terminal: term, edges, groups, any: anyIn, create };
}
/* plan:end */

function mountStatus(el) {
  el.innerHTML = '';
  const split = AM.h('div', { class: 'sv-split' }, el);
  const listEl = AM.h('div', { class: 'sv-list' }, split);
  const wrap = AM.h('div', { class: 'sv-wrap' }, split);
  const svg = AM.svg('svg', { class: 'st sv-svg' }, wrap);
  const leg = AM.h('div', { class: 'sv-legend' }, wrap);
  const title = AM.h('div', { class: 'sv-title' }, wrap);
  const fit = AM.h('button', { class: 'iconbtn sv-fit', type: 'button' }, wrap);
  fit.onclick = () => fitView();
  V = { el, listEl, wrap, svg, leg, fit, title, cam: new AM.Cam(svg, () => farLabels()), ent: null, showEmpty: false, hot: null, pendingEnt: null, g: null };
  AM.bus.on('resize', () => { if (AM.state.view === 'status') fitView(); });
  // editing: new status on a double click on the empty place, Del, spec updates (views/editor.js)
  svg.addEventListener('dblclick', onBgDbl);
  window.addEventListener('keydown', onKey);
  // Esc closes the inspector of a status or a transition first (then the usual Esc leaves the view)
  window.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape' || AM.state.view !== 'status' || AM.state.run) return;
    const tg = e.target.tagName; if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT') return;
    const s = AM.state.sel;
    if ((s && (s.t === 'status' || s.t === 'sttr')) || V.selTr) { e.preventDefault(); e.stopImmediatePropagation(); V.selTr = null; AM.select(null); paintSel(); }
  }, true);
  svg.addEventListener('click', () => { if (V.selTr) { V.selTr = null; paintSel(); } });
  const redo = () => {
    if (!V) return;
    const s = AM.state.sel, m = s && (s.t === 'status' || s.t === 'sttr') && machines().find((x) => x.ent === s.entity);
    if (s && s.t === 'status' && (!m || !m.states.includes(s.id))) AM.select(null);
    if (AM.state.view === 'status') renderAll();
  };
  AM.bus.on('spec', redo); AM.bus.on('edit', redo);
  // from a card ({machine}) or another view ({machine, to}): show the machine, select the status when given
  AM.bus.on('status:focus', (f) => { V.ent = f.machine; V.hot = null; AM.select(f.to ? { t: 'status', entity: f.machine, id: f.to } : null); renderAll(); });
  renderAll();
}

function renderAll() {
  if (!V.ent || !machines().some((m) => m.ent === V.ent)) {
    const vm = visibleMachines();
    V.ent = vm.length ? vm[0].ent : null;
  }
  renderList(); renderLegend(); V.fit.textContent = '⤢ ' + AM.t('stFit');
  const cur = machines().find((m) => m.ent === V.ent);
  if (cur) {
    const p = planOf(cur), nSt = cur.states.length - p.pseudo.length, nFn = new Set(cur.trs.map((x) => x.fid).filter(Boolean)).size;
    V.title.innerHTML = `<b>${AM.esc(cur.ent)}</b><span class="ct">${AM.esc(AM.t('stHeadCounts', nSt, cur.trs.length, nFn))}</span>` + (cur.doc ? `<p>${AM.esc(cur.doc)}</p>` : '');
    V.title.style.display = '';
  } else { V.title.innerHTML = ''; V.title.style.display = 'none'; }
  renderDiagram();
}

function renderList() {
  const l = V.listEl; l.innerHTML = '';
  AM.h('div', { class: 'sv-head' }, l, `<b>${AM.esc(AM.t('stMachines'))}</b>`);
  const all = machines(), vm = visibleMachines(), nEmpty = all.filter((m) => m.empty).length;
  if (editing()) {
    const nb = AM.h('button', { class: 'sv-more', type: 'button' }, l, AM.esc(AM.t('edAddMachine')));
    nb.onclick = (e) => AM.spec.ask(V.listEl, e.clientX, e.clientY, '', AM.t('edNewMachine'), (v) => { if (v) { V.ent = v; AM.spec.do.addState(v, 'New'); } });
    AM.h('div', { class: 'sv-hint' }, l, AM.esc(AM.t('edHint')));
  }
  if (!all.length) { AM.h('div', { class: 'sv-none' }, l, AM.esc(AM.t('stNone'))); return; }
  vm.forEach((m) => {
    const r = AM.h('div', { class: 'sv-row' + (m.ent === V.ent ? ' sel' : '') + (m.empty ? ' dim' : ''), 'data-ent': m.ent, title: m.ent }, l,
      `<span class="nm">${AM.esc(m.ent)}</span><span class="ct">${AM.esc(AM.t('stCounts', m.states.length, m.trs.length))}</span>`);
    r.onclick = () => { V.ent = m.ent; V.hot = null; renderList(); renderDiagram(); AM.select(null); };
  });
  if (nEmpty) {
    const b = AM.h('button', { class: 'sv-more', type: 'button' }, l, AM.esc(V.showEmpty ? AM.t('stHideEmpty') : AM.t('stShowEmpty', nEmpty)));
    b.onclick = () => { V.showEmpty = !V.showEmpty; renderAll(); };
  }
}

function renderLegend() {
  const t = AM.t('stLeg'), sw = (st) => `<svg width="28" height="8"><path d="M1 4H27" style="${st}"/></svg>`;
  V.leg.innerHTML = `<span>${sw('stroke:var(--accent);stroke-width:2.4')}${AM.esc(t.main)}</span>` +
    `<span>${sw('stroke:var(--edge);stroke-width:1.5')}${AM.esc(t.side)}</span>` +
    `<span><svg width="28" height="10"><path d="M2 2V7H26V2M14 7V10" style="fill:none;stroke:var(--text-2);stroke-width:1.3"/></svg>${AM.esc(t.group)}</span>` +
    `<span>${sw('stroke:var(--muted);stroke-width:1.5;stroke-dasharray:4 3')}${AM.esc(t.any)}</span>`;
  // a transition with no function behind it (only described in .arch)
  if (machines().some((m) => m.trs.some((x) => !x.fid))) V.leg.innerHTML += `<span>${sw('stroke:var(--muted);stroke-width:1.5;stroke-dasharray:6 4')}${AM.esc(t.arch)}</span>`;
  if ((AM.spec && AM.spec.avail) || machines().some((m) => m.trs.some((x) => x.st))) {
    V.leg.innerHTML += `<span>${sw('stroke:var(--text-2);stroke-width:2')}${AM.esc(AM.t('edLgBoth'))}</span>` +
      `<span>${sw('stroke:var(--serious);stroke-width:2')}${AM.esc(AM.t('edLgCode'))}</span>` +
      `<span>${sw('stroke:var(--muted);stroke-width:2;stroke-dasharray:4 3')}${AM.esc(AM.t('edLgSpec'))}</span>`;
  }
}

/* ---------- plan + geometry ---------- */
function planOf(m) { return planMachine(m, { text: trText, short: fnShort, group: 3 }); }

const MH = 44, SH = 36; // node heights: main path, the rest
const CW = { main: 7.6, side: 7.0 }; // glyph width at the node font size (Inter 13 / 12.5)
const LW = 6.3; // caption glyph width (Inter 11.5)
// textW(s, cls): the real width of a text in the drawing's font (an off-screen SVG, cached); an estimate without a DOM
const TW = { svg: null, cache: new Map() };
function textW(s, cls, est) {
  const k = cls + '\u0001' + s;
  if (TW.cache.has(k)) return TW.cache.get(k);
  let w = 0;
  try {
    if (!TW.svg) { TW.svg = AM.svg('svg', { class: 'st', style: 'position:absolute;left:-9999px;top:0;width:10px;height:10px;visibility:hidden' }, document.body); TW.g = AM.svg('g', { class: 'stm' }, TW.svg); }
    const g = AM.svg('g', { class: cls }, TW.g), tx = AM.svg('text', { class: cls === 'n main' || cls === 'n side' ? 'nm' : 'elab hum' }, g, s);
    w = tx.getComputedTextLength(); g.remove();
  } catch (e) { w = 0; }
  if (!w) w = s.length * est;
  if (document.fonts && document.fonts.status !== 'loaded') return w; // measured with a fallback font: do not keep
  TW.cache.set(k, w);
  return w;
}
const nodeW = (n, main) => Math.max(main ? 92 : 80, Math.min(main ? 300 : 270, textW(AM.cut(n, 32), main ? 'n main' : 'n side', CW[main ? 'main' : 'side']) + (main ? 44 : 38)));
const labW = (s) => textW(s, 'lab', LW);
const capOf = (e) => AM.cut(e.text, 40) + (e.n > 1 ? ' ×' + e.n : '');
// wrap(s, n): up to two lines of ≤ n characters, split at spaces, the rest cut
function wrap(s, n) {
  const ws = String(s).split(' '), out = [''];
  ws.forEach((w) => {
    const cur = out[out.length - 1];
    if (!cur) out[out.length - 1] = w;
    else if ((cur + ' ' + w).length <= n) out[out.length - 1] = cur + ' ' + w;
    else out.push(w);
  });
  const two = out.length > 2 ? [out[0], out.slice(1).join(' ')] : out;
  return two.map((l) => AM.cut(l, n + 2));
}

// trSel: the selection of a drawn arrow (a glued pair, a bracket, a creation, "from any")
function trSel(m, x, ed) {
  if (ed && ed.group) return { t: 'sttr', entity: m.ent, to: ed.group.to, group: true };
  if (ed && ed.create) return { t: 'sttr', entity: m.ent, to: x.to, create: true };
  return { t: 'sttr', entity: m.ent, from: x.from || '', to: x.to };
}
const selKey = (o) => (o.group ? 'g' : o.create ? 'c' : 'p') + '\u0001' + (o.group || o.create ? '' : o.from || '') + '\u0001' + o.to;
// the hooks of the Statuses view: select, jump to the map, the editor (frames on a page pass their own)
function viewHooks(m) {
  return {
    trClick(e, x, g, ed) {
      e.stopPropagation(); if (V.cam.justDragged) return;
      if (editing()) { V.selTr = trKey(m, x); }
      AM.select(trSel(m, x, ed));
      paintSel();
    },
    trDbl(e, x) {
      e.stopPropagation();
      if (editing() && x.ref) { AM.spec.ask(V.wrap, e.clientX, e.clientY, specLabel(x), AM.t('edEvent'), (v) => AM.spec.do.editTr(m.ent, x, v)); return; }
      if (editing() && x.st === 'code') { AM.spec.do.addTr(m.ent, x.from, x.to, ''); return; }
      if (x.fid && AM.D.fns[x.fid]) AM.map.jumpToFn(x.fid, 2);
    },
    nodeClick(e, n) { e.stopPropagation(); if (V.cam.justDragged) return; V.selTr = null; AM.select({ t: 'status', entity: m.ent, id: n }); },
    nodeDbl(e, n) {
      if (!editing()) return;
      e.stopPropagation();
      AM.spec.ask(V.wrap, e.clientX, e.clientY, n, AM.t('edRename'), (v) => { if (v && v !== n) AM.spec.do.rename(m.ent, n, v); });
    },
    nodeDown(e, n, p) { if (editing() && e.button === 0) { e.stopPropagation(); startConnect(e, m, n, p); } },
  };
}

function renderDiagram() {
  const svg = V.svg; svg.innerHTML = ''; V.g = null;
  const m = machines().find((x) => x.ent === V.ent);
  if (!m) { V.cam.set({ x: 0, y: 0, w: 100, h: 60 }); return; }
  const r = drawMachine(svg, m, viewHooks(m));
  if (!m.trs.length) AM.svg('text', { x: r.bounds.x0, y: r.bounds.y0 - 16, class: 'sv-note' }, svg, AM.t('stNoTrans'));
  else if (!r.plan.initial.length) { AM.svg('text', { x: r.bounds.x0, y: r.bounds.y0 - 16, class: 'sv-note' }, svg, AM.t('stEmptyMain')); r.bounds.y0 -= 30; }
  svg.classList.toggle('editing', editing());
  V.g = { edges: r.edges, nodes: r.nodes, m, bounds: r.bounds, plan: r.plan };
  fitView();
  paintSel(); paintHot();
}

// drawMachine(svg, m, hooks): the diagram of one machine into svg (an <svg> or <g>); the same drawing serves the
// Statuses view and the frames of a page (views/pages.js through AM.status.draw). hooks: trClick / trDbl (e, x, g, edge),
// nodeClick / nodeDbl (e, n), nodeDown (e, n, p) — all optional; compact: true — no captions on the arrows (a small
// frame). Returns {edges: [{x, xs, g}], nodes: {name: g}, bounds, plan}.
function drawMachine(svg, m, hk) {
  hk = hk || {};
  const P = planOf(m), t = AM.t;
  const root = AM.svg('g', { class: 'stm' + (hk.compact ? ' compact' : '') }, svg);
  const bounds = { x0: 1e9, y0: 1e9, x1: -1e9, y1: -1e9 };
  const inc = (x, y) => { bounds.x0 = Math.min(bounds.x0, x); bounds.x1 = Math.max(bounds.x1, x); bounds.y0 = Math.min(bounds.y0, y); bounds.y1 = Math.max(bounds.y1, y); };
  const pos = {}; // name → {x, y (top), w, h, cx, cy}
  const put = (n, x, y, w, h) => { pos[n] = { x, y, w, h, cx: x + w / 2, cy: y + h / 2, n }; inc(x, y); inc(x + w, y + h); };
  const boxes = []; // obstacles for captions
  const hitBox = (b) => boxes.some((o) => b.x0 < o.x1 && b.x1 > o.x0 && b.y0 < o.y1 && b.y1 > o.y0);
  const edgeOf = (a, b) => P.edges.find((e) => e.from === a && e.to === b);

  // main path in one line
  let x = 0;
  P.main.forEach((n, i) => {
    const w = nodeW(n, true); put(n, x, -MH / 2, w, MH);
    const e = i < P.main.length - 1 ? edgeOf(n, P.main[i + 1]) : null;
    let lw = 0;
    // the caption of a main step sits above the line and may reach over the shoulders of its statuses
    if (e && !hk.compact && e.text) { e.lines = wrap(capOf(e), 18); lw = Math.max(...e.lines.map(labW)) - 36; }
    x += w + Math.max(hk.compact ? 48 : 60, Math.min(140, lw));
  });
  const mainEnd = x;
  const colX = (c) => { // a column (main steps, fractional) → x of a centre
    const L = P.main.length;
    if (!L) return c * 230;
    if (c <= 0) return pos[P.main[0]].cx + c * 230;
    if (c >= L - 1) return pos[P.main[L - 1]].cx + (c - (L - 1)) * 230;
    const i = Math.floor(c), f = c - i;
    return pos[P.main[i]].cx * (1 - f) + pos[P.main[i + 1]].cx * f;
  };
  P.main.forEach((n) => { const p = pos[n]; boxes.push({ x0: p.x, y0: p.y, x1: p.x + p.w, y1: p.y + p.h }); });

  // arcs above (skips forward along the main path, loops): levels by span
  const above = P.edges.filter((e) => e.cls === 'skip').sort((a, b) => (P.main.indexOf(a.to) - P.main.indexOf(a.from)) - (P.main.indexOf(b.to) - P.main.indexOf(b.from)));
  const levels = []; // [{a, b}] intervals per level
  const lvlOf = (a, b, L) => { let k = 0; for (; ; k++) { L[k] = L[k] || []; if (!L[k].some((s) => a < s.b + 30 && b > s.a - 30)) { L[k].push({ a, b }); return k; } } };
  above.forEach((e) => { e.lvl = lvlOf(pos[e.from].cx, pos[e.to].cx, levels); });
  const arcTop = -MH / 2 - (above.length ? 56 + (levels.length - 1) * 20 : 30) - (P.edges.some((e) => e.cls === 'loop') ? 44 : 0);
  // back edges below: levels too
  const backs = P.edges.filter((e) => e.cls === 'back'), blv = [];
  backs.forEach((e) => { e.lvl = lvlOf(Math.min(pos[e.from].cx, pos[e.to].cx), Math.max(pos[e.from].cx, pos[e.to].cx), blv); });
  let yBelow = MH / 2 + (backs.length ? 40 + (blv.length - 1) * 20 : 0) + 30;
  if (P.initial.some((n) => P.main.indexOf(n) > 0)) yBelow = Math.max(yBelow, MH / 2 + 64); // a creation from below

  // side statuses: above in rows (failure-like ones below), packed by their column
  const rowsUp = [], rowsDn = [];
  const placeRow = (rows, w, cx) => {
    for (let r = 0; ; r++) {
      rows[r] = rows[r] || [];
      let x0 = cx - w / 2;
      const clash = () => rows[r].find((s) => x0 < s.x1 + 28 && x0 + w > s.x0 - 28);
      let c, guard = 0;
      while ((c = clash()) && guard++ < 20) x0 = c.x1 + 28;
      if (Math.abs(x0 - (cx - w / 2)) < 160 || r > 4) { rows[r].push({ x0, x1: x0 + w }); return { r, x0 }; }
    }
  };
  const side = P.side.slice().sort((a, b) => a.col - b.col);
  side.forEach((s) => {
    const w = nodeW(s.n, false), cx = colX(s.col);
    const rows = s.below ? rowsDn : rowsUp, pl = placeRow(rows, w, cx);
    s.row = pl.r; s.x0 = pl.x0; s.w = w;
  });
  const upY = (r) => arcTop - 60 - r * 86 - SH / 2;
  side.filter((s) => !s.below).forEach((s) => put(s.n, s.x0, upY(s.row), s.w, SH));
  const dnBase = yBelow + (hk.compact ? 10 : 40); // room for the captions of the arrows going down
  side.filter((s) => s.below).forEach((s) => put(s.n, s.x0, dnBase + s.row * 80, s.w, SH));
  if (rowsDn.length) yBelow = dnBase + rowsDn.length * 80 + 10;
  side.forEach((s) => { const p = pos[s.n]; boxes.push({ x0: p.x, y0: p.y, x1: p.x + p.w, y1: p.y + p.h }); });

  // groups: one bracket per group under the main path, then the finals band
  const grpY = P.groups.map((g, i) => yBelow + 14 + i * 26);
  const bandTop = yBelow + (P.groups.length ? 14 + P.groups.length * 26 + 40 : 50);
  const capFrom = (g) => { const l = g.from.slice(0, 3).join(', '); return t('stFromMany', l, Math.max(0, g.from.length - 3)); };
  // finals: in the order of the plan, near the column of their sources, never overlapping
  let fx = -1e9;
  const finY = bandTop + 42;
  P.finals.forEach((f) => {
    const w = nodeW(f.n, false), g = P.groups.find((q) => q.to === f.n);
    const cap = g ? wrap(capFrom(g), 30) : null, slot = Math.max(w, cap ? Math.max(...cap.map((l) => l.length)) * 6 + 8 : 0);
    let cx = colX(f.col); if (cx - slot / 2 < fx + 26) cx = fx + 26 + slot / 2;
    put(f.n, cx - w / 2, finY, w, SH); f.cap = cap; f.slot = slot;
    fx = cx + slot / 2;
    boxes.push({ x0: cx - slot / 2, y0: finY, x1: cx + slot / 2, y1: finY + SH + (cap ? 6 + cap.length * 13 : 0) });
  });
  const capH = hk.compact ? 0 : Math.max(0, ...P.finals.map((f) => (f.cap ? f.cap.length * 13 + 6 : 0)));
  const bandBottom = P.finals.length ? finY + SH + capH + 30 : bandTop; // the title of the band sits at its bottom
  // statuses the code never enters: a shelf under everything
  const shelfTop = bandBottom + (P.finals.length ? 34 : 0);
  let sx = Math.min(0, ...Object.values(pos).map((p) => p.x)) + 14;
  P.loose.forEach((n) => { const w = nodeW(n, false); put(n, sx, shelfTop + 30, w, SH); sx += w + 22; });

  const gBand = AM.svg('g', { class: 'band' }, root), gE = AM.svg('g', {}, root), gN = AM.svg('g', {}, root), gL = AM.svg('g', { class: 'labs' }, root);
  const xs0 = Math.min(...Object.values(pos).map((p) => p.x), 0) - 28, xs1 = Math.max(...Object.values(pos).map((p) => p.x + p.w), mainEnd - 80) + 28;
  if (P.finals.length) {
    AM.svg('rect', { x: xs0, y: bandTop, width: xs1 - xs0, height: bandBottom - bandTop + 8, rx: 12, class: 'fband' }, gBand);
    // at the bottom left: the top belongs to the arrows coming in and the "from any" stubs
    AM.svg('text', { x: xs0 + 14, y: bandBottom - 2, class: 'bandt' }, gBand, t('stFinals'));
    inc(xs0, bandTop); inc(xs1, bandBottom + 8);
  }
  if (P.loose.length) {
    const x1 = Math.max(xs1, sx + 6);
    AM.svg('rect', { x: xs0, y: shelfTop, width: x1 - xs0, height: SH + 46, rx: 12, class: 'fband loose' }, gBand);
    const tt = AM.svg('text', { x: xs0 + 14, y: shelfTop + 20, class: 'bandt' }, gBand, t('stNoEntry'));
    AM.svg('title', null, tt, t('stNoEntryTip'));
    inc(x1, shelfTop + SH + 46);
  }

  const edges = [], nodes = {};
  const placeLab = (e, lx, ly, anchor, g) => {
    if (hk.compact || !e.text) return;
    // the lines sit above ly (the last one on it); moved up / down when they would cover something
    const lines = e.lines || wrap(capOf(e), 24), LH = 13.5, w = Math.max(...lines.map(labW)), hgt = lines.length * LH;
    const x0 = anchor === 'start' ? lx : anchor === 'end' ? lx - w : lx - w / 2;
    let dy = null;
    for (const d of [0, -LH, LH, -2 * LH, 2 * LH, -3 * LH]) { if (!hitBox({ x0, y0: ly + d - hgt + 2, x1: x0 + w, y1: ly + d + 3 })) { dy = d; break; } }
    if (dy == null) return;
    boxes.push({ x0, y0: ly + dy - hgt + 2, x1: x0 + w, y1: ly + dy + 3 });
    const tx = AM.svg('text', { x: lx, y: ly + dy - (lines.length - 1) * LH, 'text-anchor': anchor, class: 'elab' + (e.human ? ' hum' : ' fnl') }, gL);
    lines.forEach((l, i) => AM.svg('tspan', { x: lx, dy: i ? LH : 0 }, tx, l));
    AM.svg('title', null, tx, e.text + (e.n > 1 ? ' ×' + e.n : ''));
    tx.addEventListener('mouseenter', () => g.classList.add('hov')); tx.addEventListener('mouseleave', () => g.classList.remove('hov'));
    tx.addEventListener('click', (ev) => g.dispatchEvent(new MouseEvent('click', ev)));
    inc(x0, ly + dy - hgt); inc(x0 + w, ly + dy + 4);
    return tx;
  };
  const tip = (e) => (e.from ? e.from : t('stAny')) + ' → ' + e.to + '\n' + e.trs.map((x) => (x.fid ? fnShort(x.fid) : t('stArchOnly')) + ' · ' + (t('stSrc')[x.st === 'spec' ? 'spec' : x.source] || x.source || '') + (x.confirmed ? ' ✓' : '')).join('\n');
  const mkEdge = (e, d, cls, mark, lab, extra) => {
    const x0 = e.trs[0];
    const g = AM.svg('g', { class: 'tr ' + cls + (e.archOnly ? ' arch' : ''), 'data-fid': x0.fid || '' }, gE);
    AM.svg('path', { d, class: 'hit' }, g);
    const path = AM.svg('path', { d, class: 'ln', 'marker-end': mark }, g);
    void path;
    const st = e.trs.map((x) => x.st).find(Boolean); if (st) g.classList.add('st-' + st);
    AM.svg('title', null, g, tip(e));
    if (lab) placeLab(e, lab.x, lab.y, lab.anchor || 'middle', g);
    const ed = Object.assign({ x: x0, xs: e.trs, g, e }, extra || {});
    if (hk.trClick) g.addEventListener('click', (ev) => hk.trClick(ev, x0, g, ed));
    if (hk.trDbl) g.addEventListener('dblclick', (ev) => hk.trDbl(ev, x0, g, ed));
    edges.push(ed);
    return g;
  };
  const MK = (e) => (e.trs.some((x) => x.st === 'code') ? 'url(#arrW)' : e.archOnly || e.trs.every((x) => x.st === 'spec') ? 'url(#arrM)' : e.cls === 'main' ? 'url(#arrA)' : 'url(#arr)');

  P.edges.forEach((e) => {
    const A = pos[e.from], B = pos[e.to];
    if (e.grouped || e.cls === 'start' || !B) return;
    if (e.cls === 'loop') {
      const x0 = B.x + B.w * 0.35, x1 = B.x + B.w * 0.65, h = 38;
      mkEdge(e, `M${x0},${B.y} C${x0 - 14},${B.y - h} ${x1 + 14},${B.y - h} ${x1},${B.y - 2}`, 'loop', MK(e), { x: B.cx, y: B.y - h * 0.78 - 4 });
      inc(B.x, B.y - h - 16); return;
    }
    if (!A) return;
    if (e.cls === 'main') {
      const y = A.cy, gap = B.x - A.x - A.w, lw = e.lines ? Math.max(...e.lines.map(labW)) : 0;
      // a caption that fits the gap sits on the line, a wider one above the statuses
      mkEdge(e, `M${A.x + A.w},${y} H${B.x - 3}`, 'main', MK(e), { x: (A.x + A.w + B.x) / 2, y: lw <= gap - 8 && (e.lines || []).length < 2 ? y - 8 : -MH / 2 - 7 });
    } else if (e.cls === 'skip') {
      const h = 72 + e.lvl * 26, ax = A.cx + 10, bx = B.cx - 10, y0 = A.y, ya = -MH / 2 - h;
      mkEdge(e, `M${ax},${y0} C${ax},${ya} ${bx},${ya} ${bx},${B.y - 2}`, 'skip', MK(e), { x: (ax + bx) / 2, y: y0 - h * 0.75 - 4 });
      inc(ax, ya);
    } else if (e.cls === 'back') {
      const dd = 40 + e.lvl * 20, ax = A.cx - 10, bx = B.cx + 10, yb = MH / 2 + dd;
      mkEdge(e, `M${ax},${A.y + A.h} C${ax},${yb + 8} ${bx},${yb + 8} ${bx},${B.y + B.h + 2}`, 'back', MK(e), { x: (ax + bx) / 2, y: MH / 2 + dd * 0.75 + 14 });
      inc(ax, yb + 8);
    } else { // side / final: from the edge of A facing B to the edge of B facing A
      const down = B.cy > A.cy + 4, up = B.cy < A.cy - 4;
      let p0, p3;
      if (down) { p0 = { x: A.cx + (B.cx - A.cx) * 0.15, y: A.y + A.h }; p3 = { x: B.cx - (B.cx - A.cx) * 0.1, y: B.y - 2 }; }
      else if (up) { p0 = { x: A.cx + (B.cx - A.cx) * 0.15, y: A.y }; p3 = { x: B.cx - (B.cx - A.cx) * 0.1, y: B.y + B.h + 2 }; }
      else if (B.cx > A.cx) { p0 = { x: A.x + A.w, y: A.cy }; p3 = { x: B.x - 3, y: B.cy }; }
      else { p0 = { x: A.x, y: A.cy }; p3 = { x: B.x + B.w + 3, y: B.cy }; }
      if (edgeOf(e.to, e.from) && (down || up)) { const o = e.from < e.to ? 9 : -9; p0.x += o; p3.x += o; } // there and back: two lines
      const ky = (p3.y - p0.y) * 0.5, horiz = !down && !up;
      const d = horiz ? `M${p0.x},${p0.y} C${(p0.x + p3.x) / 2},${p0.y} ${(p0.x + p3.x) / 2},${p3.y} ${p3.x},${p3.y}` : `M${p0.x},${p0.y} C${p0.x},${p0.y + ky} ${p3.x},${p3.y - ky} ${p3.x},${p3.y}`;
      const mx = (p0.x + p3.x) / 2, my = (p0.y + p3.y) / 2;
      mkEdge(e, d, e.cls, MK(e), { x: mx + 6, y: my, anchor: 'start' });
    }
  });
  // brackets: ticks down from every source, one line, one arrow into the final
  P.groups.forEach((g, gi) => {
    const F = pos[g.to]; if (!F) return;
    const y = grpY[gi], off = (gi - (P.groups.length - 1) / 2) * 7;
    const xsrc = g.from.map((n) => pos[n] ? pos[n].cx + off : null).filter((v) => v != null);
    const xa = Math.min(...xsrc), xb = Math.max(...xsrc), ax = Math.max(xa, Math.min(xb, F.cx));
    let ticks = '';
    g.from.forEach((n) => { const p = pos[n]; if (p) ticks += `M${p.cx + off},${p.y + p.h} V${y} `; });
    const d = `M${xa},${y} H${xb} M${ax},${y} C${ax},${y + 30} ${F.cx},${F.y - 30} ${F.cx},${F.y - 2}`;
    const e = { from: '', to: g.to, trs: [].concat(...g.edges.map((q) => q.trs)), text: g.text, human: !!g.text, n: g.from.length, archOnly: g.edges.every((q) => q.archOnly) };
    const fns = Array.from(new Set(e.trs.map((x) => x.fid).filter(Boolean)));
    e.text = g.text || (fns.length === 1 ? fnShort(fns[0]) : t('stTrFrom', g.from.length));
    const ge = mkEdge(e, d, 'grp', MK(e), null, { group: g });
    AM.svg('path', { d: ticks, class: 'tick' }, ge, null);
    if (!hk.compact) { // the caption next to the arrow, where it leaves the bracket
      const s = AM.cut(e.text, 34) + (g.text || fns.length === 1 ? ' ×' + g.from.length : ''), w = labW(s), lx = ax + 8, ly = y + 16;
      const tx = AM.svg('text', { x: lx, y: ly, class: 'elab' + (g.text ? ' hum' : ' fnl') }, gL, s);
      tx.addEventListener('click', (ev) => ge.dispatchEvent(new MouseEvent('click', ev)));
      boxes.push({ x0: lx, y0: ly - 11, x1: lx + w, y1: ly + 3 }); inc(lx + w, ly);
    }
    inc(xa, y); inc(xb, y);
  });
  // from any status: one dashed stub per target
  Object.keys(P.any).forEach((to) => {
    const B = pos[to]; if (!B) return;
    // a short dashed stub into the left shoulder, its caption above the dot (nothing sticks out to the left)
    const xs = P.any[to], sx = B.x + 4, sy = B.y - 24;
    const e = { from: '', to, trs: xs, fids: Array.from(new Set(xs.map((x) => x.fid).filter(Boolean))), human: false, archOnly: xs.every((x) => !x.fid) };
    e.n = Math.max(1, e.fids.length); e.text = t('stAnyN', e.n);
    const g = mkEdge(e, `M${sx},${sy} C${sx},${sy + 10} ${B.x + 10},${B.y - 10} ${B.x + 14},${B.y - 1}`, 'any', 'url(#arrM)', null);
    AM.svg('circle', { cx: sx, cy: sy, r: 3.5, class: 'anydot' }, g);
    if (!hk.compact) { const tx = AM.svg('text', { x: sx - 4, y: sy - 7, 'text-anchor': 'start', class: 'elab anyl' }, gL, e.text); tx.addEventListener('click', (ev) => g.dispatchEvent(new MouseEvent('click', ev))); const w = labW(e.text); boxes.push({ x0: sx - 4, y0: sy - 18, x1: sx - 4 + w, y1: sy - 4 }); inc(sx - 4 + w, sy - 18); }
    inc(sx - 8, sy - 8);
  });
  // creation: a start dot before every initial status (what the code creates, or the status nothing leads into)
  P.initial.forEach((n) => {
    const p = pos[n]; if (!p) return;
    const xs = P.create[n] || [];
    const g = AM.svg('g', { class: 'tr start' }, gE);
    // the first status of the main path and side statuses: from the left; a later main status: from below
    // (its left is the arrow of the main path)
    // a final in the band: from above, at the right shoulder (the left one takes "from any")
    const fromBelow = P.main.indexOf(n) > 0, fromAbove = !fromBelow && P.finals.some((f) => f.n === n);
    const d0 = fromBelow ? { x: p.cx - 14, y: p.y + p.h + 30 } : fromAbove ? { x: p.x + p.w - 16, y: p.y - 28 } : { x: p.x - 34, y: p.cy };
    const dl = fromBelow ? `M${d0.x},${d0.y - 6} V${p.y + p.h + 3}` : fromAbove ? `M${d0.x},${d0.y + 6} V${p.y - 3}` : `M${p.x - 28},${p.cy} H${p.x - 3}`;
    AM.svg('circle', { cx: d0.x, cy: d0.y, r: 5.5, class: 'startdot' }, g);
    AM.svg('path', { d: dl, class: 'startln', 'marker-end': 'url(#arrT)' }, g);
    inc(d0.x - 8, d0.y + 8);
    if (xs.length) {
      const e = { from: xs[0].from, to: n, trs: xs, human: false, archOnly: xs.every((x) => !x.fid) };
      const tx = xs.map(trText).find(Boolean);
      AM.svg('path', { d: fromBelow || fromAbove ? dl : `M${p.x - 40},${p.cy} H${p.x - 3}`, class: 'hit' }, g);
      AM.svg('title', null, g, t('stCreate') + ' → ' + n + '\n' + xs.map((x) => x.fid ? fnShort(x.fid) : t('stArchOnly')).join('\n'));
      const ed = { x: xs[0], xs, g, e, create: true };
      if (hk.trClick) g.addEventListener('click', (ev) => hk.trClick(ev, xs[0], g, ed));
      if (hk.trDbl) g.addEventListener('dblclick', (ev) => hk.trDbl(ev, xs[0], g, ed));
      edges.push(ed);
      if (!hk.compact) {
        const s = AM.cut(tx || t('stCreate'), 30), w = labW(s);
        // above the start of the main path; under the dot for a side status (arrows come into it from above)
        const onMain = P.main[0] === n;
        const L = fromBelow ? { x: d0.x + 10, y: d0.y + 4, a: 'start' } : fromAbove ? { x: d0.x + 9, y: d0.y + 4, a: 'start' } : onMain ? { x: p.x - 40, y: p.cy - 12, a: 'start' } : { x: p.x - 44, y: p.cy + 22, a: 'end' };
        const lab = AM.svg('text', { x: L.x, y: L.y, 'text-anchor': L.a, class: 'elab startl' + (tx ? ' hum' : '') }, gL, s);
        lab.addEventListener('click', (ev) => g.dispatchEvent(new MouseEvent('click', ev)));
        const x0 = L.a === 'start' ? L.x : L.x - w;
        boxes.push({ x0, y0: L.y - 11, x1: x0 + w, y1: L.y + 3 }); inc(x0, L.y - 12); inc(x0 + w, L.y + 4);
      }
    }
  });

  // nodes
  const role = (n) => (P.main.includes(n) ? 'main' : P.finals.some((f) => f.n === n) ? 'fin' : P.loose.includes(n) ? 'loose' : 'side');
  const isTerm = new Set(P.terminal);
  const ICON = { ok: '✓', fail: '✕', expired: '–', neutral: '' }; // plain glyphs: an emoji does not fit the dot
  Object.keys(pos).forEach((n) => {
    const p = pos[n], r = role(n), kd = P.kind[n] || 'neutral';
    const fin = isTerm.has(n);
    const g = AM.svg('g', { class: 'n ' + r + (fin ? ' term k-' + kd : '') + (P.noEntry.includes(n) ? ' noentry' : ''), 'data-id': n }, gN);
    AM.svg('rect', { x: p.x, y: p.y, width: p.w, height: p.h, rx: p.h / 2, class: 'card' }, g);
    const ic = fin ? ICON[kd] : '';
    const dx = r === 'main' ? 17 : 15;
    AM.svg('circle', { cx: p.x + dx, cy: p.cy, r: r === 'main' ? 7.5 : 6.5, class: 'dot' }, g);
    if (ic) AM.svg('text', { x: p.x + dx, y: p.cy + 3.5, 'text-anchor': 'middle', class: 'ic' }, g, ic);
    AM.svg('text', { x: p.x + dx + 13, y: p.cy + (r === 'main' ? 4.5 : 4.5), class: 'nm' }, g, AM.cut(n, 32));
    const doc = (m.docs || {})[n];
    AM.svg('title', null, g, n + (P.initial.includes(n) ? ' · ' + t('stInitial') : '') + (fin ? ' · ' + t('stKind')[kd] : '') + (P.noEntry.includes(n) ? ' · ' + t('stNoEntry') : '') + (doc ? '\n' + doc : ''));
    const f = P.finals.find((q) => q.n === n);
    if (f && f.cap && !hk.compact) { const ct = AM.svg('text', { x: p.cx, y: p.y + p.h + 15, 'text-anchor': 'middle', class: 'fromcap' }, g); f.cap.forEach((l, i) => AM.svg('tspan', { x: p.cx, dy: i ? 13 : 0 }, ct, l)); }
    if (P.noEntry.includes(n) && r !== 'loose' && !hk.compact) AM.svg('text', { x: p.cx, y: p.y + p.h + 14, 'text-anchor': 'middle', class: 'nocap' }, g, t('stNoEntry'));
    if (hk.nodeClick) g.addEventListener('click', (e) => hk.nodeClick(e, n));
    if (hk.nodeDbl) g.addEventListener('dblclick', (e) => hk.nodeDbl(e, n));
    if (hk.nodeDown) g.addEventListener('pointerdown', (e) => hk.nodeDown(e, n, p));
    nodes[n] = g;
  });
  if (!Object.keys(pos).length) inc(0, 0);
  return { edges, nodes, bounds, plan: P };
}

function fitView() {
  if (!V || !V.g) return;
  const b = V.g.bounds, top = V.title.offsetHeight ? V.title.offsetHeight + 24 : 0;
  V.cam.insetTop = top; V.cam.insetBottom = 56;
  // a small machine is not blown up: at most 1.15 screen px per unit
  V.cam.fit({ x: b.x0, y: b.y0, w: b.x1 - b.x0, h: b.y1 - b.y0 }, 20, Math.max(560, (V.svg.clientWidth || 900) / 1.15));
}
// far away the captions of the arrows are unreadable: hide them, keep statuses and the main path
function farLabels() {
  if (!V) return;
  const k = (V.svg.clientWidth || 1000) / (V.cam.vb.w || 1);
  V.svg.classList.toggle('far', k < 0.42);
}

/* ---------- highlight ---------- */
function paintSel() {
  if (!V || !V.g) return;
  const s = AM.state.sel, ent = V.g.m.ent;
  Object.keys(V.g.nodes).forEach((n) => V.g.nodes[n].classList.toggle('sel', !!s && s.t === 'status' && s.entity === ent && s.id === n));
  V.g.edges.forEach((e) => {
    const on = (!!s && s.t === 'fn' && e.xs.some((x) => x.fid === s.id)) ||
      (!!s && s.t === 'sttr' && s.entity === ent && selKey(s) === selKey(trSel(V.g.m, e.x, e))) ||
      (!!V.selTr && e.xs.some((x) => V.selTr === trKey(V.g.m, x)));
    e.g.classList.toggle('sel', on);
  });
}

/* ---------- editing (AM.spec, views/editor.js) ---------- */
// drag from a status to another one: a rubber line, then an inline field for the event
function startConnect(e0, m, from, p) {
  const svg = V.svg, pt = (e) => { const q = svg.createSVGPoint(); q.x = e.clientX; q.y = e.clientY; return q.matrixTransform(svg.getScreenCTM().inverse()); };
  let line = null, moved = false;
  const move = (e) => {
    if (!moved && Math.hypot(e.clientX - e0.clientX, e.clientY - e0.clientY) < 6) return;
    moved = true;
    const q = pt(e);
    if (!line) line = AM.svg('path', { class: 'rubber', 'marker-end': 'url(#arrA)' }, svg);
    line.setAttribute('d', `M${p.cx},${p.cy} L${q.x},${q.y}`);
  };
  const up = (e) => {
    window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up);
    if (line) line.remove();
    if (!moved) return;
    const tg = document.elementFromPoint(e.clientX, e.clientY), nd = tg && tg.closest ? tg.closest('.n') : null;
    if (!nd || !svg.contains(nd)) return;
    const to = nd.getAttribute('data-id');
    AM.spec.ask(V.wrap, e.clientX, e.clientY, '', AM.t('edEvent'), (v) => AM.spec.do.addTr(m.ent, from, to, v));
  };
  window.addEventListener('pointermove', move); window.addEventListener('pointerup', up);
}
function onBgDbl(e) {
  if (!editing() || !V.g || e.target.closest('.n, .tr')) return;
  const m = V.g.m;
  AM.spec.ask(V.wrap, e.clientX, e.clientY, '', AM.t('edNewState'), (v) => { if (v && !m.states.includes(v)) AM.spec.do.addState(m.ent, v); });
}
function onKey(e) {
  if (!editing() || AM.state.view !== 'status' || !V.g || (e.key !== 'Delete' && e.key !== 'Backspace')) return;
  const tg = e.target.tagName; if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT') return;
  const m = V.g.m, s = AM.state.sel;
  e.preventDefault();
  if (V.selTr) {
    const ed = V.g.edges.find((x) => x.xs.some((y) => trKey(m, y) === V.selTr));
    const tr = ed && ed.xs.find((y) => trKey(m, y) === V.selTr);
    if (tr && tr.ref) { V.selTr = null; AM.spec.do.removeTr(m.ent, tr); }
  } else if (s && s.t === 'status' && s.entity === m.ent) AM.spec.do.removeState(m.ent, s.id);
}
function paintHot() {
  if (!V || !V.g) return;
  paintDrawn(V.g, V.hot && V.hot.ent === V.g.m.ent ? V.hot : null);
}
// paintDrawn(r, h): mark the statuses h.cur and transitions h.trs on a drawn machine (drawMachine's result)
function paintDrawn(r, h) {
  Object.keys(r.nodes).forEach((n) => r.nodes[n].classList.toggle('cur', !!h && h.cur.has(n)));
  r.edges.forEach((e) => e.g.classList.toggle('hot', !!h && e.xs.some((x) => h.trs.has(x))));
}
// the status changes a scenario step makes, from the code only: arch:state of the step (and its callees,
// step.states) and the transitions the step function itself makes (D.transByFid) → [{ent, from, to, fid?}]
function stepCands(step) {
  const cands = (step.states || []).map((s) => ({ ent: s.entity, from: s.from || '', to: s.to }));
  (AM.D.transByFid[step.fid] || []).forEach((x) => cands.push({ ent: x.entity, from: x.from || '', to: x.to, fid: step.fid }));
  return cands;
}
// stepHot(m, step): what of machine m the step touches → {ent, trs: Set(transition), cur: Set(status)} or null
function stepHot(m, step) {
  if (!m || !step) return null;
  const mine = stepCands(step).filter((c) => c.ent === m.ent);
  if (!mine.length) return null;
  const trs = new Set(), cur = new Set();
  m.trs.forEach((x) => { if (mine.some((c) => c.to === x.to && (c.from === (x.from || '') || (x.fid && c.fid === x.fid && c.from === (x.from || ''))))) { trs.add(x); cur.add(x.to); } });
  mine.forEach((c) => cur.add(c.to));
  return { ent: m.ent, trs, cur };
}
// the page frames (views/pages.js) draw machines with the same code
AM.status = { machine: (ent) => machines().find((x) => x.ent === ent) || null, draw: drawMachine, hot: stepHot, paint: paintDrawn, nodeH: MH, plan: planOf, sel: trSel };
function onStepStatus(step) {
  if (!V) return;
  if (!step) { V.hot = null; paintHot(); return; }
  const cands = stepCands(step);
  if (!cands.length) { V.hot = null; paintHot(); return; }
  const all = machines().map((m) => m.ent);
  let ent = V.ent;
  if (!cands.some((c) => c.ent === ent)) { const c = cands.find((x) => all.includes(x.ent)); if (!c) { V.hot = null; paintHot(); return; } ent = c.ent; }
  const m = machines().find((x) => x.ent === ent);
  V.hot = stepHot(m, step);
  if (ent !== V.ent) {
    if (AM.state.view === 'status') { V.ent = ent; renderList(); renderDiagram(); } else V.pendingEnt = ent;
  }
  paintHot();
}

/* ---------- right card ---------- */
const machineOfSel = (sel) => machines().find((x) => x.ent === sel.entity) || (V && machines().find((x) => x.ent === V.ent));
// one function of a transition: name, caption, file:line, where it comes from
function fnLi(x, ctx, shown) {
  const esc = ctx.esc, f = x.fid && ctx.D.fns[x.fid] ? ctx.fnInfo(x.fid) : null;
  const src = (ctx.t('stSrc')[x.st === 'spec' ? 'spec' : x.source] || x.source || '') + (x.confirmed ? ' ✓' : '');
  if (!x.fid) return `<li class="dsc"><span class="fn">${esc(ctx.t('stArchOnly'))}</span><span class="q">${esc(src)}</span>${x.on || x.label ? `<span class="ds">${esc([x.on, x.label].filter(Boolean).join(' · '))}</span>` : ''}</li>`;
  const full = trFull(x), cap = full && full !== shown ? full : ''; // said once: in the head of the pair
  // the status handed to a setter (via) / a new row (insert): said under the caption, the setter clickable
  const vf = x.via && ctx.D.fns[x.via] ? ctx.fnInfo(x.via) : null;
  const via = x.via ? `<span class="ds">${esc(ctx.t('stVia'))} <a data-fn="${esc(x.via)}">${esc(vf ? vf.name : fnShort(x.via))}</a>${vf && vf.file ? ' · ' + esc(vf.file.split('/').pop() + ':' + vf.s) : ''}</span>` : '';
  const ins = x.insert ? `<span class="ds">${esc(ctx.t('stInsert'))}</span>` : '';
  return `<li class="dsc" ${f ? `data-fn="${esc(x.fid)}"` : ''} title="${esc(x.fid)}"><span class="fn">${esc(f ? f.name : fnShort(x.fid))}</span><span class="q">${esc(src)}</span>` +
    `<span class="md">${esc(f && f.file ? f.file.split('/').pop() + ':' + f.s : '')}</span>${cap ? `<span class="ds">${esc(cap)}</span>` : ''}${via}${ins}</li>`;
}
// a glued transition in the status card: the other status, the caption, the functions
function pairBlock(e, dirIn, ctx) {
  const esc = ctx.esc, other = dirIn ? e.from : e.to;
  const full = e.human ? e.trs.map(trFull).find(Boolean) || e.text : '';
  const head = `<div class="st-pair"><span class="lbl">${esc(dirIn ? ctx.t('stFrom') : ctx.t('stTo'))}</span> <a data-st="${esc(other)}">${esc(other)}</a>` +
    (e.n > 1 ? ` <span class="q">×${e.n}</span>` : '') + (full ? `<div class="hum">${esc(full)}</div>` : '') + '</div>';
  return head + `<ul class="rel-list">${e.trs.map((x) => fnLi(x, ctx, full)).join('')}</ul>`;
}
function bindSt(m) {
  return (pane) => {
    pane.querySelectorAll('[data-st]').forEach((a) => (a.onclick = (ev) => { ev.preventDefault(); AM.select({ t: 'status', entity: m.ent, id: a.getAttribute('data-st') }); }));
    pane.querySelectorAll('.ds a[data-fn]').forEach((a) => a.addEventListener('click', (ev) => ev.stopPropagation())); // the setter, not the row
  };
}

AM.registerPanel('status', (sel, ctx) => {
  const m = machineOfSel(sel);
  if (!m) return '';
  const n = sel.id, esc = ctx.esc, P = planOf(m), t = ctx.t;
  // in the order of the diagram: the main path left to right, then the rest
  const rank = (s) => { const i = P.main.indexOf(s); return i >= 0 ? i : 1e3 + m.states.indexOf(s); };
  const ins = P.edges.filter((e) => e.to === n && e.from !== n && !P.pseudo.includes(e.from)).sort((a, b) => rank(a.from) - rank(b.from));
  const outs = P.edges.filter((e) => e.from === n && e.to !== n).sort((a, b) => rank(a.to) - rank(b.to));
  const loops = P.edges.filter((e) => e.from === n && e.to === n);
  const created = P.create[n] || [], any = P.any[n] || [];
  const term = P.terminal.includes(n), kd = P.kind[n] || 'neutral';
  const chips = [m.ent];
  if (P.initial.includes(n)) chips.push(t('stInitial'));
  if (P.main.includes(n)) chips.push(t('stRole').main);
  else if (!term && !P.loose.includes(n)) chips.push(t('stRole').side);
  if (term) chips.push(t('stKind')[kd]);
  let h = `<div class="kicker">${chips.map((c) => `<span class="chip">${esc(c)}</span>`).join('')}${P.noEntry.includes(n) ? `<span class="chip warn" title="${esc(t('stNoEntryTip'))}">${esc(t('stNoEntry'))}</span>` : ''}</div>`;
  const doc = (m.docs || {})[n];
  h += `<h2 class="mono">${esc(n)}</h2>` + (doc ? `<p class="lead what">${esc(doc)}</p>` : `<p class="loc">${esc(t('stNoDoc'))}</p>`);
  const nIn = ins.reduce((s, e) => s + e.trs.length, 0) + created.length + any.length;
  h += `<h3>${esc(t('stHowIn'))} (${nIn})</h3>`;
  if (!nIn) h += `<p class="loc">— ${esc(P.initial.includes(n) ? '' : t('stNoEntryTip'))}</p>`;
  if (created.length) h += `<div class="st-pair"><span class="lbl">${esc(t('stCreate'))}</span></div><ul class="rel-list">${created.map((x) => fnLi(x, ctx)).join('')}</ul>`;
  ins.forEach((e) => (h += pairBlock(e, true, ctx)));
  if (any.length) h += `<div class="st-pair"><span class="lbl">${esc(t('stLeg').any)}</span></div><ul class="rel-list">${any.map((x) => fnLi(x, ctx)).join('')}</ul>`;
  const nOut = outs.reduce((s, e) => s + e.trs.length, 0) + loops.reduce((s, e) => s + e.trs.length, 0);
  h += `<h3>${esc(t('stHowOut'))} (${nOut})</h3>` + (nOut ? '' : '<p class="loc">—</p>');
  outs.concat(loops).forEach((e) => (h += pairBlock(e, false, ctx)));
  h += `<p class="loc" style="margin-top:14px">${esc(t('stDbl'))}</p>`;
  if (!editing()) return { html: h, after: bindSt(m) };
  const fin = (m.final || new Set()).has(n);
  h += `<h3>${esc(t('edBtn'))}</h3><div class="btnrow"><label class="ed-chk"><input type="checkbox" data-act="final"${fin ? ' checked' : ''}> ${esc(t('edFinal'))}</label>` +
    `<button class="pbtn" type="button" data-act="rename">${esc(t('edRenameBtn'))}</button><button class="pbtn" type="button" data-act="del">${esc(t('edDelState'))}</button></div>` +
    `<p class="loc" style="margin-top:10px">${esc(t('edHint'))}</p>`;
  return {
    html: h,
    after(pane) {
      bindSt(m)(pane);
      const q = (a) => pane.querySelector(`[data-act="${a}"]`);
      q('final').onchange = (e) => AM.spec.do.setFinal(m.ent, n, e.target.checked);
      q('rename').onclick = (e) => AM.spec.ask(V.wrap, e.clientX, e.clientY, n, t('edRename'), (v) => { if (v && v !== n) AM.spec.do.rename(m.ent, n, v); });
      q('del').onclick = () => AM.spec.do.removeState(m.ent, n);
    },
  };
});

// a transition (a glued arrow, a bracket "many into one", "from any", a creation): its functions with file:line
AM.registerPanel('sttr', (sel, ctx) => {
  const m = machineOfSel(sel);
  if (!m) return '';
  const esc = ctx.esc, P = planOf(m), t = ctx.t;
  let h = `<div class="kicker"><span class="chip">${esc(m.ent)}</span></div>`;
  if (sel.group) {
    const g = P.groups.find((q) => q.to === sel.to);
    if (!g) return '';
    h += `<h2 class="mono">→ ${esc(g.to)}</h2><p class="lead">${esc(g.text ? g.text + ' · ' : '')}${esc(t('stTrFrom', g.from.length))}</p>`;
    g.edges.forEach((e) => (h += pairBlock(e, true, ctx)));
    return { html: h, after: bindSt(m) };
  }
  const from = sel.create ? '' : sel.from || '';
  const xs = sel.create ? (P.create[sel.to] || []) : from ? m.trs.filter((x) => (x.from || '') === from && x.to === sel.to) : (P.any[sel.to] || []);
  const name = sel.create || P.pseudo.includes(from) ? t('stCreate') : from || t('stLeg').any;
  const cap = xs.map(trText).find(Boolean);
  h += `<h2 class="mono">${from && !P.pseudo.includes(from) ? `<a data-st="${esc(from)}">${esc(name)}</a>` : esc(name)} → <a data-st="${esc(sel.to)}">${esc(sel.to)}</a></h2>`;
  h += cap ? `<p class="lead what">${esc(cap)}</p>` : '';
  h += `<h3>${esc(t('stFns'))} (${xs.length})</h3><ul class="rel-list">${xs.map((x) => fnLi(x, ctx)).join('')}</ul>`;
  h += `<p class="loc" style="margin-top:14px">${esc(t('stDbl'))}</p>`;
  return { html: h, after: bindSt(m) };
});
