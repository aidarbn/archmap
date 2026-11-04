/* views/status.js — state machines: a diagram of statuses and the functions that switch them.
 * Data: AM.D.states = { entity: {states: [..], transitions: [{from, to, fid, source, confirmed}]} }.
 * Layout: starts (no incoming edges) on the left, finals (no outgoing) on the right, the rest in BFS
 * layers; back edges and loops are arcs; a transition without `from` is a dashed stub from "any". */
AM.addI18n({
  ru: {
    stTitle: 'Статусы', stMachines: 'Машины состояний', stCounts: (s, t) => s + ' ст. · ' + t + ' пер.',
    stShowEmpty: (n) => 'показать пустые (' + n + ')', stHideEmpty: 'скрыть пустые', stNone: 'В коде не найдено машин состояний.',
    stNoTrans: 'Для этой машины не найдено переходов.', stAny: 'любой', stFit: 'Вписать',
    stLeg: { code: 'в коде', annotation: 'аннотация', confirmed: 'подтверждено', any: 'из любого статуса' },
    stMachine: 'машина состояний', stStatus: 'Статус', stIn: 'Кто переводит в него', stOut: 'Во что переводит', stInitial: 'начальный', stFinal: 'конечный',
    stSrc: { code: 'код', annotation: 'аннотация' }, stOk: 'подтверждено', stFrom: 'из', stTo: 'в', stTrN: 'переходов', stStN: 'статусов',
    stDbl: 'клик — функция · двойной клик — на карту',
  },
  en: {
    stTitle: 'Statuses', stMachines: 'State machines', stCounts: (s, t) => s + ' st. · ' + t + ' tr.',
    stShowEmpty: (n) => 'show empty (' + n + ')', stHideEmpty: 'hide empty', stNone: 'No state machines found in the code.',
    stNoTrans: 'No transitions found for this machine.', stAny: 'any', stFit: 'Fit',
    stLeg: { code: 'in code', annotation: 'annotation', confirmed: 'confirmed', any: 'from any status' },
    stMachine: 'state machine', stStatus: 'Status', stIn: 'Moved into it by', stOut: 'Moves it to', stInitial: 'initial', stFinal: 'final',
    stSrc: { code: 'code', annotation: 'annotation' }, stOk: 'confirmed', stFrom: 'from', stTo: 'to', stTrN: 'transitions', stStN: 'statuses',
    stDbl: 'click — function · double click — open on the map',
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

let V = null; // {el, listEl, wrap, svg, cam, ent, showEmpty, hot, g:{nodes, edges}}

const S = () => AM.D.states || {};
function machines() {
  const base = Object.keys(S()).map((ent) => {
    const m = S()[ent] || {}, trs = m.transitions || [];
    const names = new Set(m.states || []);
    trs.forEach((x) => { if (x.from) names.add(x.from); if (x.to) names.add(x.to); });
    return { ent, states: Array.from(names), trs: trs.map((x) => (x.drift ? Object.assign({}, x, { st: x.drift === 'undeclared' ? 'code' : 'spec' }) : x)), empty: names.size < 2 || !trs.length };
  });
  // under `archmap serve` the described machines (views/editor.js, AM.spec) are merged in: each transition gets `st`
  return (AM.spec ? AM.spec.mergeMachines(base) : base)
    .sort((a, b) => b.trs.length - a.trs.length || b.states.length - a.states.length || a.ent.localeCompare(b.ent));
}
const editing = () => !!(AM.spec && AM.spec.on);
const trKey = (m, x) => m.ent + '\u0001' + (x.from || '') + '\u0001' + x.to + '\u0001' + (x.on || '');
const specLabel = (x) => (x.ref ? [x.ref.on, x.ref.label && '"' + x.ref.label + '"'].filter(Boolean).join(' ') : '');
const edgeText = (x) => (x.st === 'spec' ? specLabel(x) : [x.fid ? fnShort(x.fid) : '', specLabel(x)].filter(Boolean).join(' · '));
const MARK = { spec: 'url(#arrM)', code: 'url(#arrW)' };
const markOf = (x) => MARK[x.st] || 'url(#arr)';
const visibleMachines = () => {
  const all = machines(), good = all.filter((m) => !m.empty);
  return V.showEmpty || !good.length ? all : good;
};
const fnShort = (fid) => {
  const f = fid && AM.D.fns && AM.D.fns[fid] ? AM.fnInfo(fid) : null;
  const n = f ? f.name : String(fid || '').split(/[#:]/).pop();
  return String(n).split('.').pop();
};

function mountStatus(el) {
  el.innerHTML = '';
  const split = AM.h('div', { class: 'sv-split' }, el);
  const listEl = AM.h('div', { class: 'sv-list' }, split);
  const wrap = AM.h('div', { class: 'sv-wrap' }, split);
  const svg = AM.svg('svg', { class: 'st sv-svg' }, wrap);
  const leg = AM.h('div', { class: 'sv-legend' }, wrap);
  const fit = AM.h('button', { class: 'iconbtn sv-fit', type: 'button' }, wrap);
  fit.onclick = () => fitView();
  V = { el, listEl, wrap, svg, leg, fit, cam: new AM.Cam(svg), ent: null, showEmpty: false, hot: null, pendingEnt: null, g: null };
  AM.bus.on('resize', () => { if (AM.state.view === 'status') fitView(); });
  // editing: new status on a double click on the empty place, Del, spec updates (views/editor.js)
  svg.addEventListener('dblclick', onBgDbl);
  window.addEventListener('keydown', onKey);
  svg.addEventListener('click', () => { if (V.selTr) { V.selTr = null; paintSel(); } });
  const redo = () => {
    if (!V) return;
    const s = AM.state.sel, m = s && s.t === 'status' && machines().find((x) => x.ent === s.entity);
    if (s && s.t === 'status' && (!m || !m.states.includes(s.id))) AM.select(null);
    if (AM.state.view === 'status') renderAll();
  };
  AM.bus.on('spec', redo); AM.bus.on('edit', redo);
  AM.bus.on('status:focus', (f) => { V.ent = f.machine; V.hot = null; AM.select({ t: 'status', entity: f.machine, id: f.to }); renderAll(); });
  renderAll();
}

function renderAll() {
  if (!V.ent || !machines().some((m) => m.ent === V.ent)) {
    const vm = visibleMachines();
    V.ent = vm.length ? vm[0].ent : null;
  }
  renderList(); renderLegend(); V.fit.textContent = '⤢ ' + AM.t('stFit');
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
  const t = AM.t('stLeg');
  V.leg.innerHTML = `<span><svg width="30" height="8"><path d="M1 4H29" stroke="var(--edge)" stroke-width="1.5"/></svg>${AM.esc(t.code)}</span>` +
    `<span><b class="gl">@</b>${AM.esc(t.annotation)}</span><span><b class="gl ok">✓</b>${AM.esc(t.confirmed)}</span>` +
    `<span><svg width="30" height="8"><path d="M1 4H29" stroke="var(--edge)" stroke-width="1.5" stroke-dasharray="4 3"/></svg>${AM.esc(t.any)}</span>`;
  if ((AM.spec && AM.spec.avail) || machines().some((m) => m.trs.some((x) => x.st))) {
    V.leg.innerHTML += `<span><svg width="30" height="8"><path d="M1 4H29" stroke="var(--text-2)" stroke-width="2"/></svg>${AM.esc(AM.t('edLgBoth'))}</span>` +
      `<span><svg width="30" height="8"><path d="M1 4H29" stroke="var(--serious)" stroke-width="2"/></svg>${AM.esc(AM.t('edLgCode'))}</span>` +
      `<span><svg width="30" height="8"><path d="M1 4H29" stroke="var(--muted)" stroke-width="2" stroke-dasharray="4 3"/></svg>${AM.esc(AM.t('edLgSpec'))}</span>`;
  }
}

/* ---------- layout ---------- */
function layout(m) {
  const names = m.states, idx = {}; names.forEach((n, i) => (idx[n] = i));
  const out = {}, inn = {}; names.forEach((n) => { out[n] = []; inn[n] = []; });
  m.trs.forEach((x) => { if (x.from && x.from !== x.to && idx[x.from] != null && idx[x.to] != null) { out[x.from].push(x.to); inn[x.to].push(x.from); } });
  const anyTo = new Set(m.trs.filter((x) => !x.from).map((x) => x.to));
  const isFinal = (n) => (m.final && m.final.has(n)) || (!out[n].length && (inn[n].length > 0 || anyTo.has(n)));
  let starts = names.filter((n) => !inn[n].length && !isFinal(n));
  if (!starts.length && names.length) starts = [names[0]];
  const layer = {}; let q = starts.slice(); starts.forEach((n) => (layer[n] = 0));
  while (q.length) { const n = q.shift(); out[n].forEach((k) => { if (layer[k] == null) { layer[k] = layer[n] + 1; q.push(k); } }); }
  names.forEach((n) => { if (layer[n] == null) layer[n] = 0; });
  // finals to the right-most column
  let maxL = 0; names.forEach((n) => { if (!isFinal(n)) maxL = Math.max(maxL, layer[n]); });
  const hasFinal = names.some(isFinal);
  if (hasFinal) { maxL += 1; names.forEach((n) => { if (isFinal(n)) layer[n] = maxL; }); }
  else names.forEach((n) => (maxL = Math.max(maxL, layer[n])));
  const cols = []; for (let i = 0; i <= maxL; i++) cols.push([]);
  names.forEach((n) => cols[layer[n]].push(n));
  // order inside layers by barycentre of neighbours
  const ord = {}; cols.forEach((c) => c.forEach((n, i) => (ord[n] = i)));
  const bary = (n, adj) => { const a = adj.filter((k) => layer[k] !== layer[n]); return a.length ? a.reduce((s, k) => s + ord[k], 0) / a.length : ord[n]; };
  for (let it = 0; it < 4; it++) {
    (it % 2 ? cols.slice().reverse() : cols).forEach((c) => {
      c.sort((a, b) => bary(a, inn[a].concat(out[a])) - bary(b, inn[b].concat(out[b])) || ord[a] - ord[b]);
      c.forEach((n, i) => (ord[n] = i));
    });
  }
  return { cols, layer, starts: new Set(names.filter((n) => !inn[n].length && !isFinal(n))), finals: new Set(names.filter(isFinal)) };
}

const NH = 40, ROWGAP = 90, COLGAP = 200;

function renderDiagram() {
  const svg = V.svg; svg.innerHTML = ''; V.g = null;
  const m = machines().find((x) => x.ent === V.ent);
  if (!m) { V.cam.set({ x: 0, y: 0, w: 100, h: 60 }); return; }
  const L = layout(m), bounds = { x0: 1e9, y0: 1e9, x1: -1e9, y1: -1e9 };
  const inc = (x, y) => { bounds.x0 = Math.min(bounds.x0, x); bounds.x1 = Math.max(bounds.x1, x); bounds.y0 = Math.min(bounds.y0, y); bounds.y1 = Math.max(bounds.y1, y); };
  const widthOf = (n) => Math.max(110, Math.min(250, AM.cut(n, 28).length * 8 + 56));
  // columns
  const pos = {}; let cx = 0;
  const maxRows = Math.max(...L.cols.map((c) => c.length), 1);
  L.cols.forEach((c) => {
    const cw = Math.max(110, ...c.map(widthOf));
    c.forEach((n, i) => {
      const w = widthOf(n), y = (i - (c.length - 1) / 2) * ROWGAP + (maxRows * ROWGAP) / 2;
      pos[n] = { x: cx + (cw - w) / 2, y, w, h: NH, n };
      inc(pos[n].x, y); inc(pos[n].x + w, y + NH);
    });
    cx += cw + COLGAP;
  });
  const gE = AM.svg('g', {}, svg), gN = AM.svg('g', {}, svg);
  const edges = [], nodes = {};
  // group parallel transitions
  const pairKey = (x) => (x.from || '') + '\u0001' + x.to;
  const pairs = {}; m.trs.forEach((x) => { (pairs[pairKey(x)] = pairs[pairKey(x)] || []).push(x); });
  const anyCount = {}, loopCount = {}, backCount = {};
  const bez = (p0, p1, p2, p3, t) => { const u = 1 - t; return { x: u * u * u * p0.x + 3 * u * u * t * p1.x + 3 * u * t * t * p2.x + t * t * t * p3.x, y: u * u * u * p0.y + 3 * u * u * t * p1.y + 3 * u * t * t * p2.y + t * t * t * p3.y }; };
  m.trs.forEach((x, ti) => {
    const B = pos[x.to]; if (!B) return;
    const grp = pairs[pairKey(x)], k = grp.indexOf(x), off = (k - (grp.length - 1) / 2) * 12;
    const g = AM.svg('g', { class: 'tr' + (x.from ? '' : ' any'), 'data-fid': x.fid || '' }, gE);
    let d, lab, anchor = 'middle';
    if (!x.from) {
      const c = (anyCount[x.to] = (anyCount[x.to] || 0) + 1) - 1;
      const sx = B.x - 26, sy = B.y - 30 - c * 16;
      d = `M${sx},${sy} C${sx + 8},${sy + 14} ${B.x + 10},${B.y - 10} ${B.x + 22},${B.y - 1}`;
      lab = { x: sx - 9, y: sy + 3, anchor: 'end' }; inc(sx - 140, sy - 12);
      AM.svg('circle', { cx: sx, cy: sy, r: 4, class: 'anydot' }, g);
    } else if (x.from === x.to) {
      const c = (loopCount[x.to] = (loopCount[x.to] || 0) + 1) - 1, h = 34 + c * 20;
      const x0 = B.x + B.w * 0.3, x1 = B.x + B.w * 0.7;
      d = `M${x0},${B.y} C${x0 - 12},${B.y - h} ${x1 + 12},${B.y - h} ${x1},${B.y - 2}`;
      lab = { x: B.x + B.w / 2, y: B.y - h * 0.75 - 4, anchor: 'middle' }; inc(B.x, B.y - h - 18);
    } else {
      const A = pos[x.from];
      if (!A) return;
      const la = L.layer[x.from], lb = L.layer[x.to];
      if (lb > la) {
        const p0 = { x: A.x + A.w, y: A.y + NH / 2 + off }, p3 = { x: B.x - 2, y: B.y + NH / 2 + off }, kx = Math.max(40, (p3.x - p0.x) * 0.5);
        const p1 = { x: p0.x + kx, y: p0.y }, p2 = { x: p3.x - kx, y: p3.y };
        d = `M${p0.x},${p0.y} C${p1.x},${p1.y} ${p2.x},${p2.y} ${p3.x},${p3.y}`;
        const mid = bez(p0, p1, p2, p3, 0.5); lab = { x: mid.x, y: mid.y - 7, anchor: 'middle' };
      } else if (lb === la) { // same column: arc around the right side
        const c = (backCount['r' + la] = (backCount['r' + la] || 0) + 1) - 1, dd = 50 + c * 18 + Math.abs(A.y - B.y) * 0.1;
        const cw = Math.max(A.w, B.w), xr = Math.max(A.x + A.w, B.x + B.w);
        const p0 = { x: A.x + A.w, y: A.y + NH / 2 + off }, p3 = { x: B.x + B.w + 2, y: B.y + NH / 2 + off };
        d = `M${p0.x},${p0.y} C${xr + dd},${p0.y} ${xr + dd},${p3.y} ${p3.x},${p3.y}`;
        lab = { x: xr + dd * 0.75 + 4, y: (p0.y + p3.y) / 2, anchor: 'start' }; inc(xr + dd + 120, p0.y); void cw;
      } else { // back edge: arc below
        const c = (backCount['b'] = (backCount['b'] || 0) + 1) - 1;
        const ax = A.x + A.w / 2 + off, bx = B.x + B.w / 2 + off, dd = 44 + c * 14 + Math.min(Math.abs(ax - bx), 900) * 0.1;
        const y0 = A.y + NH, y1 = B.y + NH + 2, yb = Math.max(y0, y1) + dd;
        d = `M${ax},${y0} C${ax},${yb} ${bx},${yb} ${bx},${y1}`;
        lab = { x: (ax + bx) / 2, y: yb - dd * 0.22 + 14, anchor: 'middle' }; inc(Math.min(ax, bx), yb + 18);
      }
    }
    const hit = AM.svg('path', { d, class: 'hit' }, g);
    const path = AM.svg('path', { d, class: 'ln', 'marker-end': markOf(x) }, g);
    if (!x.from) path.setAttribute('stroke-dasharray', '5 4');
    const glyph = (x.source === 'annotation' ? '@ ' : '') + (x.confirmed ? '✓ ' : '');
    const etxt = glyph + AM.cut(edgeText(x), 32);
    const txt = AM.svg('text', { x: lab.x, y: lab.y, 'text-anchor': lab.anchor, class: 'fnl' }, g, etxt);
    AM.svg('title', null, txt, `${x.fid || ''}\n${x.from || AM.t('stAny')} → ${x.to}\n${AM.t('stSrc')[x.source] || x.source || ''}${x.confirmed ? ' · ' + AM.t('stOk') : ''}${x.st ? '\n' + AM.t({ both: 'edLgBoth', code: 'edLgCode', spec: 'edLgSpec' }[x.st]) : ''}`);
    void hit; void ti;
    if (x.st) g.classList.add('st-' + x.st);
    g.addEventListener('click', (e) => {
      e.stopPropagation(); if (V.cam.justDragged) return;
      if (editing()) { V.selTr = trKey(m, x); paintSel(); }
      if (x.fid && AM.D.fns[x.fid]) AM.select({ t: 'fn', id: x.fid, why: `${m.ent}: ${x.from || AM.t('stAny')} → ${x.to}` });
      else if (editing()) AM.select(null);
    });
    g.addEventListener('dblclick', (e) => {
      e.stopPropagation();
      if (editing() && x.ref) { AM.spec.ask(V.wrap, e.clientX, e.clientY, specLabel(x), AM.t('edEvent'), (v) => AM.spec.do.editTr(m.ent, x, v)); return; }
      if (editing() && x.st === 'code') { AM.spec.do.addTr(m.ent, x.from, x.to, ''); return; }
      if (x.fid && AM.D.fns[x.fid]) AM.map.jumpToFn(x.fid, 2);
    });
    const tw = etxt.length * 7.4;
    inc(lab.x - (lab.anchor === 'end' ? tw : lab.anchor === 'middle' ? tw / 2 : 0), lab.y - 14);
    inc(lab.x + (lab.anchor === 'start' ? tw : lab.anchor === 'middle' ? tw / 2 : 0), lab.y + 4);
    edges.push({ x, g });
  });
  // nodes
  const tone = AM.ui ? AM.ui.stTone : () => 'neutral';
  const icon = { good: ['✓', 'var(--good)'], warning: ['⏳', 'var(--warning)'], critical: ['✕', 'var(--critical)'], serious: ['⌛', 'var(--serious)'], neutral: ['', 'var(--muted)'] };
  m.states.forEach((n) => {
    const p = pos[n]; if (!p) return;
    const g = AM.svg('g', { class: 'n', 'data-id': n }, gN);
    AM.svg('rect', { x: p.x, y: p.y, width: p.w, height: NH, rx: NH / 2, class: 'card' }, g);
    const [ic, col] = icon[tone(n)] || icon.neutral;
    AM.svg('circle', { cx: p.x + 22, cy: p.y + NH / 2, r: 9, fill: col }, g);
    if (ic) AM.svg('text', { x: p.x + 22, y: p.y + NH / 2 + 4, 'text-anchor': 'middle', style: 'font-size:10px;fill:#fff;font-weight:700' }, g, ic);
    AM.svg('text', { x: p.x + 38, y: p.y + NH / 2 + 5, class: 'nm' }, g, AM.cut(n, 28));
    AM.svg('title', null, g, n + (L.starts.has(n) ? ' · ' + AM.t('stInitial') : '') + (L.finals.has(n) ? ' · ' + AM.t('stFinal') : ''));
    if (L.finals.has(n)) AM.svg('rect', { x: p.x - 4, y: p.y - 4, width: p.w + 8, height: NH + 8, rx: NH / 2 + 4, class: 'finalring' }, g);
    g.addEventListener('click', (e) => { e.stopPropagation(); if (V.cam.justDragged) return; V.selTr = null; AM.select({ t: 'status', entity: m.ent, id: n }); });
    g.addEventListener('dblclick', (e) => {
      if (!editing()) return;
      e.stopPropagation();
      AM.spec.ask(V.wrap, e.clientX, e.clientY, n, AM.t('edRename'), (v) => { if (v && v !== n) AM.spec.do.rename(m.ent, n, v); });
    });
    g.addEventListener('pointerdown', (e) => { if (editing() && e.button === 0) { e.stopPropagation(); startConnect(e, m, n, p); } });
    if (L.starts.has(n) && !m.trs.some((x) => !x.from && x.to === n)) {
      AM.svg('circle', { cx: p.x - 30, cy: p.y + NH / 2, r: 5, class: 'startdot' }, gE);
      AM.svg('path', { d: `M${p.x - 24},${p.y + NH / 2} H${p.x - 3}`, class: 'startln', 'marker-end': 'url(#arr)' }, gE);
      inc(p.x - 36, p.y);
    }
    nodes[n] = g;
  });
  if (!m.trs.length) AM.svg('text', { x: 0, y: -16, class: 'sv-note' }, svg, AM.t('stNoTrans'));
  svg.classList.toggle('editing', editing());
  V.g = { edges, nodes, m, bounds };
  fitView();
  paintSel(); paintHot();
}

function fitView() {
  if (!V || !V.g) return;
  const b = V.g.bounds;
  V.cam.fit({ x: b.x0, y: b.y0, w: b.x1 - b.x0, h: b.y1 - b.y0 }, 36, 560);
}

/* ---------- highlight ---------- */
function paintSel() {
  if (!V || !V.g) return;
  const s = AM.state.sel, ent = V.g.m.ent;
  Object.keys(V.g.nodes).forEach((n) => V.g.nodes[n].classList.toggle('sel', !!s && s.t === 'status' && s.entity === ent && s.id === n));
  V.g.edges.forEach((e) => e.g.classList.toggle('sel', (!!s && s.t === 'fn' && e.x.fid === s.id) || (!!V.selTr && V.selTr === trKey(V.g.m, e.x))));
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
    line.setAttribute('d', `M${p.x + p.w / 2},${p.y + NH / 2} L${q.x},${q.y}`);
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
    const ed = V.g.edges.find((x) => trKey(m, x.x) === V.selTr);
    if (ed && ed.x.ref) { V.selTr = null; AM.spec.do.removeTr(m.ent, ed.x); }
  } else if (s && s.t === 'status' && s.entity === m.ent) AM.spec.do.removeState(m.ent, s.id);
}
function paintHot() {
  if (!V || !V.g) return;
  const h = V.hot && V.hot.ent === V.g.m.ent ? V.hot : null;
  Object.keys(V.g.nodes).forEach((n) => V.g.nodes[n].classList.toggle('cur', !!h && h.cur.has(n)));
  V.g.edges.forEach((e) => {
    const on = !!h && h.trs.has(e.x);
    e.g.classList.toggle('hot', on);
    const p = e.g.querySelector('.ln'); if (p) p.setAttribute('marker-end', on ? 'url(#arrA)' : markOf(e.x));
  });
}
function onStepStatus(step) {
  if (!V) return;
  if (!step) { V.hot = null; paintHot(); return; }
  const cands = (step.states || []).map((s) => ({ ent: s.entity, from: s.from || '', to: s.to }));
  (AM.D.transByFid[step.fid] || []).forEach((x) => cands.push({ ent: x.entity, from: x.from || '', to: x.to, fid: step.fid }));
  if (!cands.length) { V.hot = null; paintHot(); return; }
  const all = machines().map((m) => m.ent);
  let ent = V.ent;
  if (!cands.some((c) => c.ent === ent)) { const c = cands.find((x) => all.includes(x.ent)); if (!c) { V.hot = null; paintHot(); return; } ent = c.ent; }
  const m = machines().find((x) => x.ent === ent);
  const mine = cands.filter((c) => c.ent === ent), trs = new Set(), cur = new Set();
  m.trs.forEach((x) => { if (mine.some((c) => c.to === x.to && (c.from === (x.from || '') || (x.fid && c.fid === x.fid && c.from === (x.from || ''))))) { trs.add(x); cur.add(x.to); } });
  mine.forEach((c) => cur.add(c.to));
  V.hot = { ent, trs, cur };
  if (ent !== V.ent) {
    if (AM.state.view === 'status') { V.ent = ent; renderList(); renderDiagram(); } else V.pendingEnt = ent;
  }
  paintHot();
}

/* ---------- right card ---------- */
AM.registerPanel('status', (sel, ctx) => {
  const m = machines().find((x) => x.ent === sel.entity) || machines().find((x) => x.ent === V.ent);
  if (!m) return '';
  const n = sel.id, esc = ctx.esc;
  const inn = m.trs.filter((x) => x.to === n), outs = m.trs.filter((x) => x.from === n);
  const hasIn = m.trs.some((x) => x.to === n && x.from && x.from !== n), hasOut = m.trs.some((x) => x.from === n && x.to !== n);
  const tone = ctx.ui && ctx.ui.stTone ? ctx.ui.stTone(n) : 'neutral';
  const li = (x, dirIn) => {
    const f = x.fid && ctx.D.fns[x.fid] ? ctx.fnInfo(x.fid) : null;
    const other = dirIn ? (x.from || ctx.t('stAny')) : x.to;
    const badge = `<span class="q">${esc((ctx.t('stSrc')[x.source] || x.source || '') + (x.confirmed ? ' ✓' : ''))}</span>`;
    return `<li ${f ? `data-fn="${esc(x.fid)}"` : ''} title="${esc(x.fid || '')}"><span class="fn">${esc(f ? f.name : fnShort(x.fid))}</span>${badge}<span class="md">${esc((dirIn ? ctx.t('stFrom') : ctx.t('stTo')) + ' ' + other)}</span></li>`;
  };
  let h = `<div class="kicker"><span class="chip">${esc(m.ent)}</span>${!hasIn ? `<span class="chip">${esc(ctx.t('stInitial'))}</span>` : ''}${!hasOut ? `<span class="chip">${esc(ctx.t('stFinal'))}</span>` : ''}</div>`;
  h += `<h2 class="mono">${esc(n)}</h2><p class="lead" style="color:var(--${tone === 'neutral' ? 'muted' : tone})">${esc(ctx.t('stMachine'))} · ${esc(ctx.t('stCounts', m.states.length, m.trs.length))}</p>`;
  h += `<h3>${esc(ctx.t('stIn'))} (${inn.length})</h3>` + (inn.length ? `<ul class="rel-list">${inn.map((x) => li(x, true)).join('')}</ul>` : '<p class="loc">—</p>');
  h += `<h3>${esc(ctx.t('stOut'))} (${outs.length})</h3>` + (outs.length ? `<ul class="rel-list">${outs.map((x) => li(x, false)).join('')}</ul>` : '<p class="loc">—</p>');
  h += `<p class="loc" style="margin-top:14px">${esc(ctx.t('stDbl'))}</p>`;
  if (!editing()) return h;
  const fin = (m.final || new Set()).has(n);
  h += `<h3>${esc(ctx.t('edBtn'))}</h3><div class="btnrow"><label class="ed-chk"><input type="checkbox" data-act="final"${fin ? ' checked' : ''}> ${esc(ctx.t('edFinal'))}</label>` +
    `<button class="pbtn" type="button" data-act="rename">${esc(ctx.t('edRenameBtn'))}</button><button class="pbtn" type="button" data-act="del">${esc(ctx.t('edDelState'))}</button></div>` +
    `<p class="loc" style="margin-top:10px">${esc(ctx.t('edHint'))}</p>`;
  return {
    html: h,
    after(pane) {
      const q = (a) => pane.querySelector(`[data-act="${a}"]`);
      q('final').onchange = (e) => AM.spec.do.setFinal(m.ent, n, e.target.checked);
      q('rename').onclick = (e) => AM.spec.ask(V.wrap, e.clientX, e.clientY, n, ctx.t('edRename'), (v) => { if (v && v !== n) AM.spec.do.rename(m.ent, n, v); });
      q('del').onclick = () => AM.spec.do.removeState(m.ent, n);
    },
  };
});
