/* views/data.js — the "Data" tab: table schema (ER), table card, endpoints × tables matrix, playback marks.
 * Reads facts.data.tables (fields, source, refs) and Function.data (table, op, fields, line).
 * Events: listens to 'api:open-table' {table, op, ep, fid, via} (+ AM.apiTarget when mounted later)
 * and its alias 'data:focus' {table}; emits 'data:open-endpoint' {ep, fid, table} after switching to 'api'. */
const A = AM, D = A.D, h = A.h, svg = A.svg, esc = A.esc;

A.addI18n({
  ru: {
    dataTitle: 'Данные', dvEr: 'Схема', dvMx: 'Матрица', dvSearchT: 'Поиск таблицы…', dvSearchE: 'Поиск ручки…', dvSearchC: 'Колонки: таблица…',
    dvGroups: 'Группы', dvAllGroups: 'Все группы', dvFit: 'Вписать', dvNone: 'В фактах нет таблиц: данные размечаются из миграций, ORM-моделей и запросов.',
    dvOther: 'прочее', dvMore: (n) => '+ ещё ' + n, dvTables: (n, p) => n + ' ' + p(['таблица', 'таблицы', 'таблиц']),
    dvFields: 'Поля', dvRefsOut: 'Ссылается на', dvRefsIn: 'Ссылаются на неё', dvWho: 'Кто меняет', dvEps: 'Ручки, которые её трогают',
    dvNoWho: 'в коде не найдено обращений', dvNoEps: 'ни одна ручка не доходит до таблицы по вызовам', dvShowAll: (n) => 'показать все (' + n + ')',
    dvSource: { migration: 'миграции', orm: 'ORM', query: 'запросы', sql: 'SQL' }, dvVirtual: 'таблицы нет в схеме — найдена только по обращениям',
    dvLegend: 'R — читает, W — пишет, RW — оба. Клик по ячейке — функции, дающие доступ.', dvAllKinds: 'Все виды', dvMode: ['Все ручки', 'Только с данными', 'Есть запись', 'Только чтение'],
    dvSort: ['По имени', 'По числу таблиц', 'По записи'], dvColSort: ['Колонки: по имени', 'Колонки: по частоте'], dvRowsMore: (n) => 'ещё ' + n + ' ручек — показать',
    dvCell: 'Ручка × таблица', dvFns: 'Функции с доступом', dvOpenApi: 'Открыть ручку в API', dvOpenTable: 'Открыть таблицу', dvNoRows: 'Ни одна ручка не подходит под фильтры',
    dvRowTip: 'клик по ручке — открыть в API', dvAccess: 'Доступ', dvAcc: { R: 'чтение', W: 'запись', RW: 'чтение и запись' }, dvLine: 'стр.',
    dvEpsN: (n, p) => n + ' ' + p(['ручка', 'ручки', 'ручек']), dvCols: 'колонок', dvStepTables: 'на шаге',
  },
  en: {
    dataTitle: 'Data', dvEr: 'Schema', dvMx: 'Matrix', dvSearchT: 'Find a table…', dvSearchE: 'Find an endpoint…', dvSearchC: 'Columns: table…',
    dvGroups: 'Groups', dvAllGroups: 'All groups', dvFit: 'Fit', dvNone: 'No tables in the facts: data is read from migrations, ORM models and queries.',
    dvOther: 'other', dvMore: (n) => '+ ' + n + ' more', dvTables: (n) => n + (n === 1 ? ' table' : ' tables'),
    dvFields: 'Fields', dvRefsOut: 'References', dvRefsIn: 'Referenced by', dvWho: 'Who changes it', dvEps: 'Endpoints that touch it',
    dvNoWho: 'no access found in code', dvNoEps: 'no endpoint reaches this table through calls', dvShowAll: (n) => 'show all (' + n + ')',
    dvSource: { migration: 'migrations', orm: 'ORM', query: 'queries', sql: 'SQL' }, dvVirtual: 'not in the schema — found only from accesses',
    dvLegend: 'R — reads, W — writes, RW — both. Click a cell for the functions that give the access.', dvAllKinds: 'All kinds', dvMode: ['All endpoints', 'With data only', 'Writes', 'Read-only'],
    dvSort: ['By name', 'By tables count', 'By writes'], dvColSort: ['Columns: by name', 'Columns: by usage'], dvRowsMore: (n) => n + ' more endpoints — show',
    dvCell: 'Endpoint × table', dvFns: 'Functions with access', dvOpenApi: 'Open the endpoint in API', dvOpenTable: 'Open the table', dvNoRows: 'No endpoint matches the filters',
    dvRowTip: 'click an endpoint to open it in API', dvAccess: 'Access', dvAcc: { R: 'read', W: 'write', RW: 'read and write' }, dvLine: 'line',
    dvEpsN: (n) => n + (n === 1 ? ' endpoint' : ' endpoints'), dvCols: 'columns', dvStepTables: 'on this step',
  },
});
const tr = (k, ...a) => A.t(k, ...a);

/* ---------- index ---------- */
const OPL = { read: 'R', insert: 'I', update: 'U', delete: 'D', upsert: 'UP' };
const OPORD = { insert: 0, update: 1, upsert: 2, delete: 3, read: 4 };
const isW = (op) => op !== 'read' && op !== 'select';
let IDX = null;
function build() {
  const T = {}, acc = {};
  const mk = (n, tb, virtual) => {
    const fields = tb.fields || [], refs = (tb.refs || []);
    T[n] = { name: n, fields, refs, source: tb.source, virtual, fk: new Set(refs.map((r) => r.field)), in: [] };
  };
  Object.keys(D.tables).forEach((n) => mk(n, D.tables[n], false));
  D.fids.forEach((fid) => (D.fns[fid].data || []).forEach((a) => {
    if (!a || !a.table) return;
    if (!T[a.table]) mk(a.table, {}, true);
    (acc[a.table] = acc[a.table] || []).push({ fid, op: a.op, fields: a.fields || [], line: a.line });
  }));
  const names = Object.keys(T).sort();
  names.forEach((n) => { T[n].refs = T[n].refs.filter((r) => T[r.table]); T[n].refs.forEach((r) => T[r.table].in.push({ table: n, field: r.field, to: r.to })); });
  IDX = { T, names, acc, clo: new Map(), eps: null, tabEps: null };
}
const ix = () => { if (!IDX) build(); return IDX; };
// closure(i): tables reached by calls from endpoint i → {r, w, items:[{fid, op, fields, line}]}
function closure(i) {
  const X = ix();
  if (X.clo.has(i)) return X.clo.get(i);
  const e = D.endpoints[i], tabs = new Map();
  if (e && e.fid && D.fns[e.fid]) {
    const seen = new Set([e.fid]), st = [e.fid];
    while (st.length) {
      const fid = st.pop(), f = D.fns[fid];
      (f.data || []).forEach((a) => {
        if (!a || !a.table) return;
        let o = tabs.get(a.table); if (!o) tabs.set(a.table, (o = { r: false, w: false, items: [] }));
        if (isW(a.op)) o.w = true; else o.r = true;
        o.items.push({ fid, op: a.op, fields: a.fields || [], line: a.line });
      });
      (f.callees || []).concat(f.async_callees || []).forEach((c) => { if (D.fns[c] && !seen.has(c)) { seen.add(c); st.push(c); } });
    }
  }
  X.clo.set(i, tabs);
  return tabs;
}
function allEps() {
  const X = ix();
  if (X.eps) return X.eps;
  X.eps = []; X.tabEps = {};
  D.endpoints.forEach((e, i) => {
    if (!e.fid || !D.fns[e.fid]) return;
    const c = closure(i);
    X.eps.push(i);
    c.forEach((o, tb) => (X.tabEps[tb] = X.tabEps[tb] || []).push({ i, r: o.r, w: o.w }));
  });
  return X.eps;
}
const epName = (e) => (e.kind === 'task' && e.service ? e.service : e.name || e.path || e.fid);
const epLabel = (e) => (e.service && e.service !== epName(e) ? e.service + '.' : '') + epName(e);
const rwOf = (o) => (o.r && o.w ? 'RW' : o.w ? 'W' : 'R');
const opB = (op) => `<span class="op-b ${OPL[op] || 'R'}" title="${esc(op)}">${OPL[op] || esc(op)}</span>`;
const pkOf = (f) => f.name === 'id' || f.name === 'pk' || /^(big)?serial|AutoField|BigAutoField/.test(f.type || '');

/* ---------- view state ---------- */
const S = { tab: 'er', q: '', groups: null, grp: '', mq: '', cq: '', kind: '', mode: 1, sort: 0, csort: 0, rows: 250, mCell: null };
let V = null; // DOM refs and scenes
let HOT = null, SEEN = null; // playback marks: Map table → {ops:Set, fields:Set}; Set of earlier tables

/* ---------- ER layout ---------- */
const CW = 232, HH = 28, RH = 17, CAP = 10, GX = 64, GY = 16, GP = 16, GT = 26;
function shown(tb) {
  const fs = tb.fields;
  if (fs.length <= CAP + 1) return { list: fs, more: [] };
  const pri = (f) => (pkOf(f) ? 0 : tb.fk.has(f.name) ? 1 : 2);
  const keep = fs.map((f, i) => [f, i]).sort((a, b) => pri(a[0]) - pri(b[0]) || a[1] - b[1]).slice(0, CAP).sort((a, b) => a[1] - b[1]).map((x) => x[0]);
  const set = new Set(keep);
  return { list: keep, more: fs.filter((f) => !set.has(f)) };
}
const cardH = (tb) => { const s = shown(tb); return HH + (s.list.length + (s.more.length ? 1 : 0)) * RH + 10; };
function layoutCluster(names, maxH) {
  const X = ix(), set = new Set(names), out = {}, inn = {};
  names.forEach((n) => { out[n] = []; inn[n] = []; });
  names.forEach((n) => X.T[n].refs.forEach((r) => { if (set.has(r.table) && r.table !== n) { out[n].push(r.table); inn[r.table].push(n); } }));
  const linked = names.filter((n) => out[n].length || inn[n].length), lone = names.filter((n) => !out[n].length && !inn[n].length);
  const depth = {}, vis = {};
  const dep = (n) => {
    if (depth[n] != null) return depth[n];
    if (vis[n]) return 0;
    vis[n] = 1; let d = 0; out[n].forEach((m) => { d = Math.max(d, dep(m) + 1); }); vis[n] = 0;
    return (depth[n] = d);
  };
  const layers = [];
  linked.forEach((n) => { const d = dep(n); (layers[d] = layers[d] || []).push(n); });
  for (let i = 0; i < layers.length; i++) layers[i] = layers[i] || [];
  const ord = {};
  const reord = () => layers.forEach((a) => a.forEach((n, i) => { ord[n] = (i + 0.5) / a.length; }));
  reord();
  for (let sw = 0; sw < 4; sw++) {
    const seq = sw % 2 ? layers.slice().reverse() : layers;
    seq.forEach((a) => {
      const key = {};
      a.forEach((n) => { const nb = out[n].concat(inn[n]); key[n] = nb.length ? nb.reduce((s, m) => s + ord[m], 0) / nb.length : ord[n]; });
      a.sort((p, q) => key[p] - key[q] || (p < q ? -1 : 1));
      a.forEach((n, i) => { ord[n] = (i + 0.5) / a.length; });
    });
  }
  const pos = {}; let cx = 0, W = 0, H = 0;
  layers.forEach((a) => {
    let y = 0, x = cx, used = false;
    a.forEach((n) => {
      const ch = cardH(X.T[n]);
      if (used && y + ch > maxH) { x += CW + GX; y = 0; used = false; }
      pos[n] = { x, y, w: CW, h: ch }; y += ch + GY; used = true; H = Math.max(H, y - GY); W = Math.max(W, x + CW);
    });
    cx = x + CW + GX;
  });
  if (lone.length) {
    const colsLinked = Math.max(1, Math.round((W + GX) / (CW + GX)));
    const cols = Math.min(8, linked.length ? Math.max(2, colsLinked) : Math.max(1, Math.ceil(Math.sqrt(lone.length * 1.4))));
    let y0 = linked.length ? H + GY * 3 : 0, i = 0;
    while (i < lone.length) {
      const row = lone.slice(i, i + cols), rh = Math.max(...row.map((n) => cardH(X.T[n])));
      row.forEach((n, j) => { pos[n] = { x: j * (CW + GX), y: y0, w: CW, h: cardH(X.T[n]) }; });
      W = Math.max(W, row.length * (CW + GX) - GX); y0 += rh + GY; i += cols;
    }
    H = Math.max(H, y0 - GY);
  }
  return { pos, w: W, h: H };
}
function groupsOf(names) {
  const m = {};
  names.forEach((n) => { const k = n.split('_')[0]; (m[k] = m[k] || []).push(n); });
  const rest = [], gs = [];
  Object.keys(m).forEach((k) => (m[k].length > 1 ? gs.push({ key: k, names: m[k] }) : rest.push(m[k][0])));
  gs.sort((a, b) => b.names.length - a.names.length || (a.key < b.key ? -1 : 1));
  if (rest.length) gs.push({ key: '', names: rest, other: true });
  return gs;
}
function layoutAll() {
  const X = ix();
  if (S.groups == null) S.groups = X.names.length > 60;
  const pos = {}, boxes = [];
  if (!S.groups) {
    const area = X.names.reduce((s, n) => s + (cardH(X.T[n]) + GY) * (CW + GX), 0);
    const c = layoutCluster(X.names, Math.max(650, Math.min(1700, Math.sqrt(area / 1.8))));
    Object.assign(pos, c.pos);
    return { pos, boxes, w: c.w, h: c.h };
  }
  const cl = groupsOf(X.names).map((g) => Object.assign(g, layoutCluster(g.names, 900)));
  let area = 0, maxW = 0;
  cl.forEach((g) => { g.bw = g.w + GP * 2; g.bh = g.h + GP * 2 + GT; area += g.bw * g.bh; maxW = Math.max(maxW, g.bw); });
  const rowW = Math.max(maxW, Math.sqrt(area * 2.4));
  let x = 0, y = 0, rh = 0, W = 0;
  cl.forEach((g) => {
    if (x > 0 && x + g.bw > rowW) { x = 0; y += rh + 36; rh = 0; }
    boxes.push({ key: g.key, other: g.other, x, y, w: g.bw, h: g.bh, n: g.names.length });
    Object.keys(g.pos).forEach((n) => { const p = g.pos[n]; pos[n] = { x: x + GP + p.x, y: y + GP + GT + p.y, w: p.w, h: p.h, g: g.key }; });
    x += g.bw + 36; rh = Math.max(rh, g.bh); W = Math.max(W, x - 36);
  });
  return { pos, boxes, w: W, h: y + rh };
}
const tshort = (s, n) => A.cut(s, n);
function renderER() {
  const X = ix(), host = V.erBox;
  host.innerHTML = '';
  if (!X.names.length) { h('div', { class: 'empty' }, host, esc(tr('dvNone'))); V.er = null; return; }
  const sv = svg('svg', { class: 'er dv-er' }, host);
  const LY = layoutAll();
  const gb = svg('g', {}, sv), ge = svg('g', {}, sv), gc = svg('g', {}, sv);
  const er = V.er = { sv, cam: new A.Cam(sv, (vb) => sv.classList.toggle('lod-lo', (sv.parentElement.offsetWidth || 1000) / vb.w < 0.4)), cards: {}, edges: [], by: {}, extent: { x: 0, y: 0, w: LY.w, h: LY.h }, boxes: LY.boxes, pos: LY.pos };
  er.cam.maxW = Math.max(2e5, LY.w * 4);
  LY.boxes.forEach((b) => {
    svg('rect', { x: b.x, y: b.y, width: b.w, height: b.h, rx: 14, class: 'gbox' }, gb);
    svg('text', { x: b.x + GP, y: b.y + 19, class: 'gname' }, gb, (b.other ? tr('dvOther') : b.key) + ' · ' + b.n);
  });
  // cards
  X.names.forEach((n) => {
    const tb = X.T[n], p = LY.pos[n], s = shown(tb);
    const g = svg('g', { class: 'tb n' + (tb.virtual ? ' ext' : ''), 'data-id': n }, gc);
    svg('rect', { x: p.x, y: p.y, width: p.w, height: p.h, rx: 10, class: 'card' }, g);
    svg('path', { d: `M${p.x},${p.y + HH} v-${HH - 10} a10,10 0 0 1 10,-10 h${p.w - 20} a10,10 0 0 1 10,10 v${HH - 10} z`, class: 'hd' }, g);
    svg('text', { x: p.x + 12, y: p.y + 19, class: 'nm' }, g, tshort(n, 27));
    const opb = svg('text', { x: p.x + p.w - 10, y: p.y + 19, 'text-anchor': 'end', class: 'opb' }, g, '');
    const rows = {};
    s.list.forEach((f, i) => {
      const y = p.y + HH + i * RH + 2;
      const fg = svg('g', { class: 'fld' }, g);
      rows[f.name] = svg('rect', { x: p.x + 4, y, width: p.w - 8, height: RH, rx: 4, class: 'row' }, fg);
      const tx = svg('text', { x: p.x + 12, y: y + 12.5 }, fg);
      svg('tspan', { class: 'fn' }, tx, tshort(f.name, 20));
      svg('tspan', { dx: 6, class: 'ft' }, tx, tshort(f.type || '', 11));
      const mark = pkOf(f) ? 'PK' : tb.fk.has(f.name) ? 'FK' : '';
      if (mark) svg('text', { x: p.x + p.w - 12, y: y + 12.5, 'text-anchor': 'end', class: 'fm' }, fg, mark);
    });
    let more = null;
    if (s.more.length) {
      const y = p.y + HH + s.list.length * RH + 2;
      const fg = svg('g', { class: 'fld' }, g);
      more = svg('rect', { x: p.x + 4, y, width: p.w - 8, height: RH, rx: 4, class: 'row' }, fg);
      svg('text', { x: p.x + 12, y: y + 12.5, class: 'ft' }, fg, tr('dvMore', s.more.length));
    }
    g.addEventListener('click', () => { if (er.cam.justDragged) return; A.select({ t: 'table', id: n }); });
    g.addEventListener('mouseenter', () => (er.by[n] || []).forEach((e) => e.el.classList.add('hov')));
    g.addEventListener('mouseleave', () => (er.by[n] || []).forEach((e) => e.el.classList.remove('hov')));
    er.cards[n] = { g, rows, more, moreSet: new Set(s.more.map((f) => f.name)), opb, p };
  });
  // relations
  X.names.forEach((a) => X.T[a].refs.forEach((r) => {
    const b = r.table; if (a === b) return;
    const A_ = er.cards[a].p, B_ = er.cards[b].p, sa = shown(X.T[a]).list.findIndex((f) => f.name === r.field);
    const y1 = A_.y + (sa >= 0 ? HH + sa * RH + 2 + RH / 2 : HH / 2);
    const tl = shown(X.T[b]).list, ti = tl.findIndex((f) => f.name === r.to || pkOf(f));
    const y2 = B_.y + (ti >= 0 ? HH + ti * RH + 2 + RH / 2 : HH / 2);
    let d;
    if (A_.x + A_.w + 8 <= B_.x) { const x1 = A_.x + A_.w, x2 = B_.x, k = Math.max(30, (x2 - x1) / 2); d = `M${x1},${y1} C${x1 + k},${y1} ${x2 - k},${y2} ${x2},${y2}`; }
    else if (B_.x + B_.w + 8 <= A_.x) { const x1 = A_.x, x2 = B_.x + B_.w, k = Math.max(30, (x1 - x2) / 2); d = `M${x1},${y1} C${x1 - k},${y1} ${x2 + k},${y2} ${x2},${y2}`; }
    else { const x1 = A_.x, x2 = B_.x, xm = Math.min(x1, x2) - 34; d = `M${x1},${y1} C${xm},${y1} ${xm},${y2} ${x2},${y2}`; }
    const el = svg('path', { d, class: 'rel' + (A_.g !== B_.g ? ' x' : ''), 'marker-end': 'url(#arr)' }, ge);
    const e = { a, b, el };
    er.edges.push(e); (er.by[a] = er.by[a] || []).push(e); (er.by[b] = er.by[b] || []).push(e);
  }));
  paintER();
  fitER();
}
function fitER() { if (V.er && V.erBox.offsetWidth) V.er.cam.fit(V.er.extent, 30); }
function focusTable(n) {
  const er = V && V.er; if (!er || !er.cards[n]) return;
  const p = er.cards[n].p;
  const r = { x: p.x - 160, y: p.y - 60, w: p.w + 320, h: p.h + 120 };
  er.cam.animTo(r, 20, 900);
}
function paintER() {
  const er = V && V.er; if (!er) return;
  const sel = A.state.sel, st = sel && sel.t === 'table' ? sel.id : null, q = S.q.trim().toLowerCase();
  const nb = new Set();
  if (st && er.cards[st]) (er.by[st] || []).forEach((e) => { nb.add(e.a); nb.add(e.b); });
  er.sv.classList.toggle('has-sel', !!st && !!er.cards[st]);
  er.sv.classList.toggle('has-hot', !!HOT);
  Object.keys(er.cards).forEach((n) => {
    const c = er.cards[n], hot = HOT && HOT.get(n);
    c.g.classList.toggle('sel', n === st);
    c.g.classList.toggle('nb', n !== st && nb.has(n));
    c.g.classList.toggle('hot', !!hot);
    c.g.classList.toggle('seen', !hot && !!(SEEN && SEEN.has(n)));
    c.g.classList.toggle('dim', !!q && !n.toLowerCase().includes(q));
    c.g.classList.toggle('hit', !!q && n.toLowerCase().includes(q));
    Object.keys(c.rows).forEach((f) => c.rows[f].classList.toggle('hot', !!(hot && hot.fields.has(f))));
    if (c.more) c.more.classList.toggle('hot', !!(hot && Array.from(hot.fields).some((f) => c.moreSet.has(f))));
    c.opb.textContent = hot ? Array.from(hot.ops).map((o) => OPL[o] || o).join(' ') : '';
  });
  er.edges.forEach((e) => { e.el.classList.toggle('on', !!st && (e.a === st || e.b === st)); });
}

/* ---------- matrix ---------- */
function rowsFor() {
  const X = ix(); allEps();
  const q = S.mq.trim().toLowerCase();
  let rows = X.eps.map((i) => {
    const e = D.endpoints[i], c = closure(i);
    let nw = 0; c.forEach((o) => { if (o.w) nw++; });
    return { i, e, c, n: c.size, nw };
  });
  if (S.kind) rows = rows.filter((r) => r.e.kind === S.kind);
  if (q) rows = rows.filter((r) => (epLabel(r.e) + ' ' + (r.e.path || '') + ' ' + (r.e.method || '')).toLowerCase().includes(q));
  if (S.mode === 1) rows = rows.filter((r) => r.n);
  else if (S.mode === 2) rows = rows.filter((r) => r.nw);
  else if (S.mode === 3) rows = rows.filter((r) => r.n && !r.nw);
  const nm = (r) => epLabel(r.e).toLowerCase();
  rows.sort(S.sort === 1 ? (a, b) => b.n - a.n || (nm(a) < nm(b) ? -1 : 1) : S.sort === 2 ? (a, b) => b.nw - a.nw || b.n - a.n || (nm(a) < nm(b) ? -1 : 1) : (a, b) => (nm(a) < nm(b) ? -1 : nm(a) > nm(b) ? 1 : 0));
  return rows;
}
function renderMatrix() {
  const X = ix(), box = V.mxBox, rows = rowsFor();
  const vis = rows.slice(0, S.rows);
  const cq = S.cq.trim().toLowerCase();
  const used = {};
  rows.forEach((r) => r.c.forEach((_, tb) => { used[tb] = (used[tb] || 0) + 1; }));
  let cols = X.names.filter((n) => (S.mode === 0 ? true : used[n]) && (!cq || n.toLowerCase().includes(cq)));
  if (S.mode === 0 && !cq && cols.length > 60) cols = cols.filter((n) => used[n]);
  if (S.csort === 1) cols.sort((a, b) => (used[b] || 0) - (used[a] || 0) || (a < b ? -1 : 1));
  V.mxCols = cols;
  if (!X.names.length || !rows.length || !cols.length) { box.innerHTML = `<div class="empty">${esc(tr(X.names.length ? 'dvNoRows' : 'dvNone'))}</div>`; V.mxInfo.textContent = ''; return; }
  V.mxInfo.textContent = tr('dvEpsN', rows.length) + ' × ' + cols.length + ' ' + tr('dvCols');
  let s = '<table class="matrix"><thead><tr><th class="corner"></th>' + cols.map((c) => `<th data-t="${esc(c)}" title="${esc(c)}"><div>${esc(tshort(c, 28))}</div></th>`).join('') + '</tr></thead><tbody>';
  vis.forEach((r) => {
    s += `<tr data-i="${r.i}"><th class="rh" title="${esc(tr('dvRowTip') + ': ' + (r.e.path || ''))}">${esc(epName(r.e))} <small>${esc((r.e.service || r.e.kind || '') + (r.e.method ? ' · ' + r.e.method : ''))}</small></th>`;
    cols.forEach((c) => { const o = r.c.get(c); s += o ? `<td class="${rwOf(o)}" data-t="${esc(c)}">${rwOf(o)}</td>` : '<td></td>'; });
    s += '</tr>';
  });
  s += '</tbody></table>';
  if (rows.length > vis.length) s += `<button class="pbtn dv-more" data-more="1">${esc(tr('dvRowsMore', rows.length - vis.length))}</button>`;
  box.innerHTML = s;
  paintMatrix();
}
function paintMatrix() {
  const box = V && V.mxBox; if (!box) return;
  const sel = A.state.sel, c = S.mCell, st = sel && sel.t === 'table' ? sel.id : null;
  box.querySelectorAll('.hotc, .selc, .stepc').forEach((x) => x.classList.remove('hotc', 'selc', 'stepc'));
  box.querySelectorAll('tr.hot').forEach((x) => x.classList.remove('hot'));
  box.querySelectorAll('thead th[data-t]').forEach((th) => {
    const n = th.dataset.t;
    th.classList.toggle('selc', n === st || (sel && sel.t === 'cell' && sel.table === n));
    th.classList.toggle('stepc', !!(HOT && HOT.has(n)));
  });
  let entry = null;
  if (HOT) { try { const l = A.play.list(); entry = l && l[0] && l[0].fid; } catch (e) { entry = null; } }
  box.querySelectorAll('tbody tr').forEach((tr_) => {
    const e = D.endpoints[+tr_.dataset.i], isEntry = !!(entry && e && e.fid === entry);
    tr_.classList.toggle('hot', isEntry);
    if (isEntry) tr_.querySelectorAll('td[data-t]').forEach((td) => { if (HOT.has(td.dataset.t)) td.classList.add('stepc'); });
  });
  if (sel && sel.t === 'cell') {
    const td = box.querySelector(`tr[data-i="${sel.ep}"] td[data-t="${CSS.escape(sel.table)}"]`);
    if (td) td.classList.add('selc');
  }
  if (c) { /* reserved */ }
}

/* ---------- tabs and toolbar ---------- */
function ctl(tag, attrs, parent, html) { return h(tag, attrs, parent, html); }
function buildToolbar() {
  const tb = V.bar; tb.innerHTML = '';
  const st = ctl('div', { class: 'subtabs' }, tb);
  [['er', tr('dvEr')], ['mx', tr('dvMx')]].forEach(([k, name]) => {
    const b = ctl('button', { 'aria-selected': S.tab === k }, st, esc(name));
    b.onclick = () => { S.tab = k; syncTab(); };
  });
  const er = V.erCtl = ctl('div', { class: 'dv-ctl' }, tb);
  const inp = ctl('input', { type: 'search', class: 'dv-in', placeholder: tr('dvSearchT'), value: S.q }, er);
  inp.oninput = () => { S.q = inp.value; paintER(); };
  inp.onkeydown = (ev) => {
    if (ev.key !== 'Enter' || !V.er) return;
    const q = S.q.trim().toLowerCase(), n = Object.keys(V.er.cards).find((x) => q && x.toLowerCase().includes(q));
    if (n) { A.select({ t: 'table', id: n }); focusTable(n); }
  };
  const gb = ctl('button', { class: 'pbtn', 'aria-pressed': !!S.groups }, er, esc(tr('dvGroups')));
  gb.onclick = () => { S.groups = !S.groups; S.grp = ''; renderER(); buildToolbar(); };
  if (S.groups && V.er) {
    const sel = ctl('select', { class: 'dv-in' }, er, `<option value="">${esc(tr('dvAllGroups'))}</option>` + V.er.boxes.map((b) => `<option value="${esc(b.key)}|${b.other ? 1 : 0}">${esc(b.other ? tr('dvOther') : b.key)} (${b.n})</option>`).join(''));
    sel.value = S.grp;
    sel.onchange = () => {
      S.grp = sel.value;
      if (!sel.value) return fitER();
      const b = V.er.boxes.find((x) => x.key + '|' + (x.other ? 1 : 0) === sel.value);
      if (b) V.er.cam.animTo(b, 30, 600);
    };
  }
  [['+', 0.7], ['−', 1.4]].forEach(([l, f]) => { const b = ctl('button', { class: 'iconbtn' }, er, l); b.onclick = () => V.er && V.er.cam.zoomAt(.5, .5, f); });
  const fb = ctl('button', { class: 'iconbtn', title: tr('dvFit') }, er, '⤢'); fb.onclick = fitER;
  const mx = V.mxCtl = ctl('div', { class: 'dv-ctl' }, tb);
  const re = () => { S.rows = 250; renderMatrix(); };
  const i1 = ctl('input', { type: 'search', class: 'dv-in', placeholder: tr('dvSearchE'), value: S.mq }, mx); i1.oninput = () => { S.mq = i1.value; re(); };
  const i2 = ctl('input', { type: 'search', class: 'dv-in', placeholder: tr('dvSearchC'), value: S.cq }, mx); i2.oninput = () => { S.cq = i2.value; re(); };
  const kinds = Array.from(new Set(D.endpoints.map((e) => e.kind).filter(Boolean)));
  const sel = (opts, cur, fn) => { const s = ctl('select', { class: 'dv-in' }, mx, opts.map((o, i) => `<option value="${i}">${esc(o)}</option>`).join('')); s.value = cur; s.onchange = () => { fn(+s.value); re(); }; };
  if (kinds.length > 1) { const s = ctl('select', { class: 'dv-in' }, mx, `<option value="">${esc(tr('dvAllKinds'))}</option>` + kinds.map((k) => `<option>${esc(k)}</option>`).join('')); s.value = S.kind; s.onchange = () => { S.kind = s.value; re(); }; }
  sel(tr('dvMode'), S.mode, (v) => (S.mode = v));
  sel(tr('dvSort'), S.sort, (v) => (S.sort = v));
  sel(tr('dvColSort'), S.csort, (v) => (S.csort = v));
  V.mxInfo = ctl('span', { class: 'dv-info' }, mx);
  V.badge = ctl('span', { class: 'dv-badge' }, tb);
  syncTab(true);
  paintBadge(A.play.current ? safeCur() : null);
}
function syncTab(skipBar) {
  if (!V) return;
  V.erBox.style.display = S.tab === 'er' ? '' : 'none';
  V.mxWrap.style.display = S.tab === 'mx' ? '' : 'none';
  V.erCtl.style.display = S.tab === 'er' ? '' : 'none';
  V.mxCtl.style.display = S.tab === 'mx' ? '' : 'none';
  if (!skipBar) V.bar.querySelectorAll('.subtabs button').forEach((b, i) => b.setAttribute('aria-selected', (i === 0) === (S.tab === 'er')));
  if (S.tab === 'mx' && !V.mxDone) { V.mxDone = true; renderMatrix(); }
  if (S.tab === 'er') requestAnimationFrame(() => { if (V.er && !V.fitted && V.erBox.offsetWidth) { V.fitted = true; fitER(); } });
}
function rebuildAll() {
  if (!V) return;
  V.mxDone = false; V.fitted = false;
  buildToolbar();
  renderER();
  if (S.tab === 'mx') { V.mxDone = true; renderMatrix(); }
}
const safeCur = () => { try { return A.play.current(); } catch (e) { return null; } };

/* ---------- playback marks ---------- */
function stepTables(step) {
  const X = ix(), hot = new Map();
  const add = (tb, op, fields) => {
    if (!X.T[tb]) return;
    const o = hot.get(tb) || { ops: new Set(), fields: new Set() };
    if (op) o.ops.add(op); (fields || []).forEach((f) => o.fields.add(f)); hot.set(tb, o);
  };
  (step.data || []).forEach((a) => add(a.table, a.op, a.fields));
  if (!hot.size) (step.effects || []).forEach((e) => String(e).split(/[^A-Za-z0-9_]+/).forEach((w) => { if (w.length > 2 && X.T[w]) add(w, /^\s*(select|read)/i.test(e) ? 'read' : 'update', []); }));
  return hot;
}
function paintBadge(step) {
  if (!V || !V.badge) return;
  if (!step || !HOT || !HOT.size) { V.badge.innerHTML = ''; return; }
  V.badge.innerHTML = A.ui.effBadges(step).map(A.ui.mutBadge).join('');
}
function onStep(step) {
  ix();
  HOT = null; SEEN = null;
  if (step) {
    HOT = stepTables(step);
    SEEN = new Set();
    try { A.play.list().slice(0, step.i).forEach((it) => ((D.fns[it.fid] || {}).data || []).forEach((a) => SEEN.add(a.table))); } catch (e) { /* no run */ }
    if (!HOT.size) HOT = null;
  }
  paintBadge(step); paintER(); paintMatrix();
  if (HOT && A.state.view === 'data' && S.tab === 'er' && V.er) {
    const n = Array.from(HOT.keys()).find((x) => V.er.cards[x]);
    if (n) focusTable(n);
  }
}

/* ---------- panels ---------- */
const chipT = (n) => `<span class="chip dv-link" data-tbl="${esc(n)}">${esc(n)}</span>`;
function goTable(n) { if (A.state.view !== 'data') A.setView('data'); S.tab = 'er'; syncTab(); A.select({ t: 'table', id: n }); focusTable(n); }
function goApi(i) {
  const e = D.endpoints[i]; if (!e) return;
  if (A.views.some((v) => v.id === 'api')) A.setView('api');
  A.bus.emit('data:open-endpoint', { ep: e, fid: e.fid, table: A.state.sel && A.state.sel.table || (A.state.sel && A.state.sel.id) || null });
}
function wirePanel(p) {
  p.querySelectorAll('[data-tbl]').forEach((e) => e.addEventListener('click', () => goTable(e.dataset.tbl)));
  p.querySelectorAll('[data-jf]').forEach((e) => e.addEventListener('click', () => A.map.jumpToFn(e.dataset.jf, 2)));
  p.querySelectorAll('[data-api]').forEach((e) => e.addEventListener('click', () => goApi(+e.dataset.api)));
  p.querySelectorAll('[data-all]').forEach((e) => e.addEventListener('click', () => { A.state.sel.all = true; A.renderPanel(); }));
}
function fnRows(items, sortW) {
  const by = new Map();
  items.forEach((it) => { let o = by.get(it.fid); if (!o) by.set(it.fid, (o = { fid: it.fid, ops: new Set(), fields: new Set(), line: it.line })); o.ops.add(it.op); (it.fields || []).forEach((f) => o.fields.add(f)); });
  const list = Array.from(by.values());
  const w = (o) => Math.min(...Array.from(o.ops).map((x) => OPORD[x] == null ? 5 : OPORD[x]));
  if (sortW) list.sort((a, b) => w(a) - w(b));
  return list;
}
function fnRowHTML(o) {
  const f = A.fnInfo(o.fid), ops = Array.from(o.ops).sort((a, b) => OPORD[a] - OPORD[b]).map(opB).join('');
  const fl = Array.from(o.fields);
  return `<tr><td class="op">${ops}</td><td><div class="fn" data-jf="${esc(o.fid)}" title="${esc(f.file || '')}${o.line ? ':' + o.line : ''}">${esc(f.name)}</div>${fl.length ? `<div class="fl">${esc(A.cut(fl.join(', '), 90))}</div>` : ''}</td></tr>`;
}
A.registerPanel('table', (sel) => {
  const X = ix(), tb = X.T[sel.id];
  if (!tb) return `<h2>${esc(sel.id)}</h2>`;
  const accs = X.acc[tb.name] || [];
  let hh = `<div class="kicker"><span class="chip">${esc(tr('dvSource')[tb.source] || tb.source || 'table')}</span>${tb.virtual ? `<span class="chip">${esc(tr('dvVirtual'))}</span>` : ''}</div><h2 class="mono">${esc(tb.name)}</h2>`;
  if (tb.fields.length) {
    hh += `<h3>${esc(tr('dvFields'))} (${tb.fields.length})</h3><table class="touch dv-fields">` + tb.fields.map((f) => {
      const r = tb.refs.find((x) => x.field === f.name);
      return `<tr><td class="fl"><b>${esc(f.name)}</b></td><td class="fl">${esc(f.type || '')}</td><td>${pkOf(f) ? '<span class="op-b R">PK</span>' : ''}${r ? `<span class="chip dv-link" data-tbl="${esc(r.table)}">→ ${esc(r.table)}</span>` : ''}</td></tr>`;
    }).join('') + '</table>';
  }
  if (tb.refs.length) hh += `<h3>${esc(tr('dvRefsOut'))}</h3><div class="tbl-pills">` + Array.from(new Set(tb.refs.map((r) => r.table))).map(chipT).join('') + '</div>';
  if (tb.in.length) hh += `<h3>${esc(tr('dvRefsIn'))}</h3><div class="tbl-pills">` + Array.from(new Set(tb.in.map((r) => r.table))).map(chipT).join('') + '</div>';
  // who changes it: functions grouped by module
  const rows = fnRows(accs, true);
  hh += `<h3>${esc(tr('dvWho'))} (${rows.length})</h3>`;
  if (!rows.length) hh += `<p class="lead">${esc(tr('dvNoWho'))}</p>`;
  else {
    const byMod = new Map();
    rows.forEach((o) => { const m = (D.fns[o.fid] || {}).module || ''; (byMod.get(m) || byMod.set(m, []).get(m)).push(o); });
    const mods = Array.from(byMod.entries()).sort((a, b) => {
      const w = (l) => l.filter((o) => Array.from(o.ops).some(isW)).length;
      return w(b[1]) - w(a[1]) || b[1].length - a[1].length;
    });
    let left = sel.all ? 1e9 : 40;
    mods.forEach(([m, list]) => {
      if (left <= 0) return;
      const part = list.slice(0, left); left -= part.length;
      hh += `<div class="dv-mod"><span class="chip" data-mod="${esc(m)}" style="cursor:pointer">${esc(A.cut(A.short(m), 40))}</span></div><table class="touch">${part.map(fnRowHTML).join('')}</table>`;
    });
    if (!sel.all && rows.length > 40) hh += `<div class="btnrow"><button class="pbtn" data-all="1">${esc(tr('dvShowAll', rows.length))}</button></div>`;
  }
  // endpoints
  allEps();
  const eps = (X.tabEps[tb.name] || []).slice().sort((a, b) => (b.w - a.w) || (epLabel(D.endpoints[a.i]) < epLabel(D.endpoints[b.i]) ? -1 : 1));
  hh += `<h3>${esc(tr('dvEps'))} (${eps.length})</h3>`;
  if (!eps.length) hh += `<p class="lead">${esc(tr('dvNoEps'))}</p>`;
  else {
    hh += '<ul class="rel-list">' + eps.slice(0, sel.all ? 1e9 : 30).map((o) => {
      const e = D.endpoints[o.i];
      return `<li data-api="${o.i}" title="${esc(e.path || '')}"><span class="op-b ${o.w ? 'U' : 'R'}">${o.r && o.w ? 'RW' : o.w ? 'W' : 'R'}</span><span class="fn">${esc(epName(e))}</span><span class="md">${esc(e.service || e.kind || '')}</span></li>`;
    }).join('') + '</ul>';
    if (!sel.all && eps.length > 30) hh += `<div class="btnrow"><button class="pbtn" data-all="1">${esc(tr('dvShowAll', eps.length))}</button></div>`;
  }
  return { html: hh, after: (p) => wirePanel(p) };
});
A.registerPanel('cell', (sel) => {
  const X = ix(), e = D.endpoints[sel.ep], o = e && closure(sel.ep).get(sel.table);
  if (!e || !o) return `<h2>${esc(sel.table || '')}</h2>`;
  const rw = rwOf(o);
  let hh = `<div class="kicker">${esc(tr('dvCell'))}</div><h2 class="mono">${esc(epName(e))}</h2><p class="lead mono">${esc((e.method ? e.method + ' ' : '') + (e.path || e.fid || ''))}</p>`;
  hh += `<dl class="kv"><dt>${esc(tr('dvAccess'))}</dt><dd><span class="op-b ${o.w ? 'U' : 'R'}">${rw}</span> ${esc(tr('dvAcc')[rw])}</dd><dt>${esc(tr('tables'))}</dt><dd><span class="chip dv-link" data-tbl="${esc(sel.table)}">${esc(sel.table)}</span></dd></dl>`;
  const rows = fnRows(o.items, true);
  hh += `<h3>${esc(tr('dvFns'))} (${rows.length})</h3><table class="touch">${rows.map(fnRowHTML).join('')}</table>`;
  hh += `<div class="btnrow"><button class="pbtn" data-api="${sel.ep}">${esc(tr('dvOpenApi'))}</button><button class="pbtn" data-tbl="${esc(sel.table)}">${esc(tr('dvOpenTable'))}</button></div>`;
  return { html: hh, after: (p) => wirePanel(p) };
});

/* ---------- registration ---------- */
A.registerView({
  id: 'data',
  order: 40,
  title: () => tr('dataTitle'),
  mount(el) {
    ix();
    el.classList.add('dv');
    const bar = h('div', { class: 'dv-bar' }, el);
    const erBox = h('div', { class: 'dv-er-box' }, el);
    const mxWrap = h('div', { class: 'dv-mx-wrap' }, el);
    const mxBox = h('div', { class: 'dv-mx' }, mxWrap);
    h('div', { class: 'loc dv-legend' }, mxWrap, esc(tr('dvLegend')));
    V = { el, bar, erBox, mxWrap, mxBox, er: null, mxDone: false, fitted: false };
    mxBox.addEventListener('click', (ev) => {
      const more = ev.target.closest('[data-more]');
      if (more) { S.rows += 250; renderMatrix(); return; }
      const td = ev.target.closest('td[data-t]');
      if (td) { const i = +td.parentElement.dataset.i; S.mCell = { ep: i, table: td.dataset.t }; A.select({ t: 'cell', ep: i, table: td.dataset.t }); return; }
      const rh = ev.target.closest('th.rh');
      if (rh) { goApi(+rh.parentElement.dataset.i); return; }
      const ch = ev.target.closest('thead th[data-t]');
      if (ch) A.select({ t: 'table', id: ch.dataset.t });
    });
    mxBox.addEventListener('mouseover', (ev) => {
      const td = ev.target.closest('td'); const hov = mxBox.querySelector('thead th.hovc'); if (hov) hov.classList.remove('hovc');
      if (td && td.dataset.t) { const th = mxBox.querySelector(`thead th[data-t="${CSS.escape(td.dataset.t)}"]`); if (th) th.classList.add('hovc'); }
    });
    buildToolbar();
    renderER();
    if (HOT === null) { const c = safeCur(); if (c) onStep(c); }
    if (A.apiTarget) { const p = A.apiTarget; A.apiTarget = null; setTimeout(() => openTarget(p), 0); }
  },
  show() { if (V) { syncTab(); if (S.tab === 'er') requestAnimationFrame(() => { if (V.er && !V.fitted) { V.fitted = true; fitER(); } }); A.renderPanel(); } },
  onStep,
  onSelect() { paintER(); paintMatrix(); },
  onLang() { rebuildAll(); },
  onData() { IDX = null; rebuildAll(); },
});
A.bus.on('data', () => { IDX = null; });
// contract with the API view: AM.bus.emit('data:focus', {table, field?}) opens the table in this tab
function openTarget(p) {
  if (!p || !p.table) return;
  if (A.state.view !== 'data') A.setView('data');
  if (!V || !ix().T[p.table]) return;
  goTable(p.table);
}
A.bus.on('data:focus', openTarget); // own alias
// contract with the API view: 'api:open-table' {table, op, ep, fid, via}; AM.apiTarget holds the same payload
A.bus.on('api:open-table', openTarget);
A.bus.on('view', (id) => { if (id === 'data' && A.apiTarget) { const p = A.apiTarget; A.apiTarget = null; openTarget(p); } });
