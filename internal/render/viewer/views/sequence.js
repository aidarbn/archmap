/* views/sequence.js — the "Scenario" tab: a sequence diagram of the current scenario.
 * Participants: the actor, modules by layer, the queue (async hand-offs), external systems.
 * Messages: the steps of the run (label or function name), replies back to the caller, async
 * hand-offs through the queue with a pause, alt blocks for the branches of the flow (a branch
 * that continues in another flow is folded and can be expanded; a text outcome is a pill),
 * effect and status badges, the exit. Follows the player: the current message is lit and
 * scrolled to, a click on a message plays up to that step. A module mini map sits on the right. */
AM.addI18n({
  ru: {
    seqTitle: 'Сценарий', seqEntry: 'Вход', seqReply: 'ответ', seqPause: 'пауза: задача ждёт в очереди', seqEnq: 'в очередь',
    seqExpand: 'развернуть', seqCollapse: 'свернуть', seqTake: '▶ пройти ветку', seqPick: '▶ выбрать исход',
    seqMapCap: 'Карта модулей', seqEmpty: 'Нет сценария для показа', seqExternal: 'внешняя', seqFlowTo: 'сценарий',
    seqSteps: (n, p) => n + ' ' + p(['шаг', 'шага', 'шагов']),
    seqLegend: 'вызов', seqLegendReply: 'ответ', seqLegendAsync: 'асинхронно через очередь', seqNoMap: 'карта модулей недоступна',
  },
  en: {
    seqTitle: 'Scenario', seqEntry: 'Entry', seqReply: 'reply', seqPause: 'pause: the job waits in the queue', seqEnq: 'enqueue',
    seqExpand: 'expand', seqCollapse: 'collapse', seqTake: '▶ take this branch', seqPick: '▶ pick this outcome',
    seqMapCap: 'Module map', seqEmpty: 'No scenario to show', seqExternal: 'external', seqFlowTo: 'scenario',
    seqSteps: (n, p) => n + ' ' + p(['step', 'steps']),
    seqLegend: 'call', seqLegendReply: 'reply', seqLegendAsync: 'async via the queue', seqNoMap: 'module map unavailable',
  },
});

const SEQ_LD = { entry: 0, ui: 1, domain: 2, other: 2, adapter: 3, infra: 4, test: 4 };
const SEQ_MAXREPLY = 150; // replies need the call stack of every step (quadratic): skipped in very long runs
const SEQ_TONE = { good: 'var(--good)', warning: 'var(--warning)', critical: 'var(--critical)', serious: 'var(--serious)', neutral: 'var(--muted)' };
const seqBad = /(fail|error|cancel|reject|denied|block|expire|timeout|ошиб|отмен|отклон|истек|истёк|отказ|не оплач)/i;

let sEl = null, sWrap = null, sSide = null, sSig = '', sDirty = true;
let sItems = [], sAct = null, sThumb = null, sPrevLast = -2;
const sOpen = new Set(); // expanded branch previews

const D_ = () => AM.D;
const txt = (s, n) => AM.cut(s, n);

/* ---------- model: rows and participants ---------- */
function seqModel() {
  const D = D_(), P = AM.play, run = AM.state.run;
  if (!run) return null;
  const n = P.list().length;
  if (!n) return null;
  const rows = [], parts = new Map();
  const addPart = (key, o) => { if (!parts.has(key)) parts.set(key, Object.assign({ key, n: parts.size }, o)); return key; };
  const seg0 = run.segs[0], fl0 = seg0 && seg0.flow;
  const s0 = P.stepAt(0);
  addPart('actor', { kind: 'actor', name: (fl0 && fl0.actor) || AM.t('seqEntry'), actor: s0 && s0.actor });
  const modPart = (mod) => addPart('m:' + mod, { kind: 'mod', mod, layer: AM.layerOf(mod), name: AM.short(mod) });
  const queueExt = D.externals.find((x) => x.kind === 'queue');
  const queuePart = () => addPart('queue', { kind: 'queue', name: queueExt ? queueExt.name : AM.t('queue'), ext: queueExt });
  const extPart = (x) => addPart('e:' + x.id, { kind: 'ext', ext: x, name: x.name });

  const depth = [];
  if (n <= SEQ_MAXREPLY) for (let i = 0; i < n; i++) depth.push(P.frames(i).length);
  let open = []; // calls waiting for their reply: {from, to, depth, i}
  const closeReplies = (d) => {
    while (open.length && open[open.length - 1].depth >= d) {
      const o = open.pop();
      rows.push({ k: 'msg', dir: 'reply', from: o.to, to: o.from, text: '', i: o.i, fid: null, main: false });
    }
  };
  const stepOf = (fid, prev, fl) => ({
    fid, prev, mod: D.fns[fid].module,
    kind: !prev ? 'entry' : AM.isAsync(prev, fid) ? 'async' : AM.isCall(prev, fid) ? 'call' : 'next',
    label: (fl && fl.labels && fl.labels[fid]) || AM.fnInfo(fid).name,
    effects: (fl && fl.effects && fl.effects[fid]) || [], data: D.fns[fid].data || [], states: (D.transByFid[fid] || []).slice(0, 2),
  });
  // emitStep(s, i): i is the index in the run, -1 for a folded-out preview of another flow
  const emitStep = (s, i) => {
    const real = i >= 0, toK = modPart(s.mod);
    const fromK = s.prev ? modPart(D.fns[s.prev].module) : 'actor';
    if (real && depth.length) closeReplies(depth[i]);
    const badges = { eff: AM.ui.effBadges(s), st: (s.states || []).slice(0, 2) };
    const base = { k: 'msg', i, pv: !real, fid: s.fid, main: true };
    if (s.kind === 'async') {
      const qk = queuePart();
      rows.push(Object.assign({}, base, { dir: 'async', from: fromK, to: qk, text: AM.t('seqEnq') + ': ' + AM.fnInfo(s.fid).short, num: false }));
      rows.push({ k: 'pause', i, pv: !real });
      rows.push(Object.assign({}, base, { dir: 'async', from: qk, to: toK, text: s.label, badges, fname: AM.fnInfo(s.fid).name }));
    } else {
      rows.push(Object.assign({}, base, { dir: fromK === toK ? 'self' : 'call', from: fromK, to: toK, text: s.label, badges, fname: AM.fnInfo(s.fid).name }));
    }
    (D.extByFn[s.fid] || []).filter((x) => x.kind !== 'queue').slice(0, 2).forEach((x) => {
      const ek = extPart(x);
      rows.push({ k: 'msg', dir: 'call', from: toK, to: ek, text: x.name, i, pv: !real, fid: s.fid, main: false });
      rows.push({ k: 'msg', dir: 'reply', from: ek, to: toK, text: '', i, pv: !real, fid: s.fid, main: false });
    });
    if (real && depth.length && s.kind !== 'async' && fromK !== toK) open.push({ from: fromK, to: toK, depth: depth[i], i });
  };
  const exitRow = (text, taken) => rows.push({ k: 'exit', text: AM.L(text), good: taken !== false && !seqBad.test(AM.L(text)) });
  const guard = (b) => b.cond || b.text || '?';
  const keyOf = (b) => (b.flow || '') + '|' + (b.cond || '') + '|' + (b.text || '');

  const emitSeg = (si, base) => {
    const sg = run.segs[si], fl = sg.flow, last = si === run.segs.length - 1;
    sg.steps.forEach((fid, k) => emitStep(P.stepAt(base + k), base + k));
    if (depth.length) closeReplies(-1);
    const nextBase = base + sg.steps.length, next = run.segs[si + 1];
    const brs = fl && (fl.branches || []).length ? fl.branches : null;
    if (!brs) { if (fl && fl.exit) exitRow(fl.exit); return; }
    if (!last && !next) return;
    rows.push({ k: 'alt', t: 'start', guard: guard(brs[0]) });
    brs.forEach((b, bi) => {
      if (bi) rows.push({ k: 'alt', t: 'else', guard: guard(b) });
      const target = b.flow && D.flowById[b.flow];
      if (next && next.via === b) { emitSeg(si + 1, nextBase); return; }
      if (target) {
        const key = keyOf(b), isOpen = sOpen.has(key);
        const tsteps = target.steps.filter((f) => D.fns[f]);
        rows.push({ k: 'branch', b, key, target, n: tsteps.length, open: isOpen });
        if (isOpen) {
          let prev = sg.steps[sg.steps.length - 1];
          tsteps.forEach((f) => { emitStep(stepOf(f, prev, target), -1); prev = f; });
          if (target.exit) exitRow(target.exit, null);
        }
        return;
      }
      const txtB = b.text || b.cond;
      rows.push({ k: 'out', b, text: txtB, taken: !next && !!AM.state.exitText && AM.state.exitText === txtB });
    });
    rows.push({ k: 'alt', t: 'end' });
  };
  emitSeg(0, 0);
  // replies at the end of the run, when a segment had no steps flushed them already
  const arr = Array.from(parts.values());
  const rank = (p) => (p.kind === 'actor' ? 0 : p.kind === 'mod' ? 1 : p.kind === 'queue' ? 2 : 3);
  arr.sort((a, b) => rank(a) - rank(b) || (a.kind === 'mod' ? (SEQ_LD[a.layer] || 0) - (SEQ_LD[b.layer] || 0) : 0) || a.n - b.n);
  return { rows, parts: arr };
}

/* ---------- drawing ---------- */
const HEAD_H = 64;
function seqPill(parent, x, y, parts) {
  // one row of small pills starting at x (centre handled by the caller via total width); returns width
  let cx = x;
  parts.forEach((p) => {
    const g = AM.svg('g', { class: p.cls }, parent);
    const w = p.w;
    AM.svg('rect', { x: cx, y: y - 7, width: w, height: 16, rx: p.round ? 8 : 5 }, g);
    if (p.dot) AM.svg('circle', { cx: cx + 9, cy: y + 1, r: 4, fill: p.dot }, g);
    if (p.op) AM.svg('text', { x: cx + 6, y: y + 4, class: 'op' }, g, p.op);
    AM.svg('text', { x: cx + (p.dot ? 17 : p.op ? 14 + p.op.length * 7 : 6), y: y + 4 }, g, p.text);
    cx += w + 6;
  });
  return cx - x - 6;
}
function seqBadgePills(b) {
  const out = [];
  b.eff.forEach((e) => { const tx = txt(e.text, 34); out.push({ cls: 'pill', op: e.op, text: tx, w: 22 + e.op.length * 7 + tx.length * 6.7 }); });
  b.st.forEach((s) => { const tx = txt(s.to, 22); out.push({ cls: 'spill', dot: SEQ_TONE[AM.ui.stTone(s.to)], text: tx, round: true, w: 24 + tx.length * 6.3 }); });
  return out;
}

function seqRender() {
  sDirty = false; sItems = []; sAct = null; sThumb = null;
  const top = sWrap.scrollTop;
  sWrap.innerHTML = '';
  const m = seqModel();
  if (!m || !m.rows.length) { AM.h('div', { class: 'seq-empty' }, sWrap, AM.esc(AM.t('seqEmpty'))); seqSide(); return; }
  const parts = m.parts, np = parts.length;
  // the right margin is wider than the left: a self-call loop with its label hangs to the right of the last lifeline
  const cw = sWrap.clientWidth || 900, x0 = 84, xr = 240, colMin = 150;
  const W = Math.max(cw - 1, x0 + xr + (np - 1) * colMin);
  const colW = np > 1 ? (W - x0 - xr) / (np - 1) : 0;
  const X = {}; parts.forEach((p, i) => { X[p.key] = np > 1 ? x0 + i * colW : W / 2; });

  // vertical layout
  let y = 16; const lay = [];
  m.rows.forEach((r) => {
    const o = { r, y };
    let h = 0;
    if (r.k === 'msg') {
      if (r.dir === 'reply') h = 28;
      else { h = 40 + (r.badges && (r.badges.eff.length || r.badges.st.length) ? 22 : 0) + (r.dir === 'self' ? 16 : 0); }
    } else if (r.k === 'pause') h = 32;
    else if (r.k === 'alt') h = r.t === 'end' ? 14 : 34;
    else if (r.k === 'branch') h = 46;
    else if (r.k === 'out' || r.k === 'exit') h = 38;
    y += h; o.h = h; lay.push(o);
  });
  const Ht = y + 24;
  const hsvg = AM.svg('svg', { class: 'seq seqhead', width: W, height: HEAD_H, viewBox: `0 0 ${W} ${HEAD_H}` }, sWrap);
  const svg = AM.svg('svg', { class: 'seq', width: W, height: Ht, viewBox: `0 0 ${W} ${Ht}`, style: 'display:block' }, sWrap);
  const gf = AM.svg('g', {}, svg), gl = AM.svg('g', {}, svg), gm = AM.svg('g', {}, svg), gh = AM.svg('g', {}, hsvg);

  // heads and lifelines
  parts.forEach((p) => {
    AM.svg('line', { x1: X[p.key], y1: 0, x2: X[p.key], y2: Ht - 10, class: 'lifeline' }, gl);
    const hd = AM.svg('g', { class: 'head' + (p.kind === 'ext' || p.kind === 'queue' ? ' ext' : ''), 'data-p': p.key, tabindex: 0, role: 'button' }, gh);
    const w = Math.min(colW ? colW - 10 : 150, 156);
    AM.svg('rect', { x: X[p.key] - w / 2, y: 12, width: w, height: 42, rx: p.kind === 'actor' ? 21 : 9, style: p.kind === 'ext' || p.kind === 'queue' ? 'stroke-dasharray:4 3' : '' }, hd);
    if (p.kind === 'mod') AM.svg('rect', { x: X[p.key] - w / 2 + 8, y: 22, width: 3, height: 22, rx: 1.5, fill: AM.LCOLOR[p.layer], style: 'filter:none;stroke:none' }, hd);
    const kind = p.kind === 'actor' ? AM.t('actor') : p.kind === 'mod' ? AM.t('layers')[p.layer] : p.kind === 'queue' ? AM.t('queue') : (AM.t('kinds')[p.ext.kind] || AM.t('seqExternal'));
    AM.svg('text', { x: X[p.key], y: 29, 'text-anchor': 'middle', class: 'k' }, hd, txt(kind, 20));
    AM.svg('text', { x: X[p.key], y: 45, 'text-anchor': 'middle' }, hd, txt(p.name, Math.max(10, Math.floor(w / 7))));
    const title = AM.svg('title', null, hd, p.kind === 'mod' ? p.mod : p.name);
    const pick = () => {
      if (p.kind === 'mod') AM.select({ t: 'mod', id: AM.map.unitOf(p.mod) || p.mod });
      else if (p.kind === 'actor' && p.actor) AM.select({ t: 'actor', id: p.actor.id });
      else if (p.ext) AM.select({ t: 'ext', id: p.ext.id });
    };
    hd.addEventListener('click', pick);
    hd.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.stopPropagation(); pick(); } });
    void title;
  });

  // frames of alt blocks
  const stack = [], frames = [];
  const xL = (d) => 12 + d * 10, xR = (d) => W - 12 - d * 10;
  lay.forEach((o) => {
    const r = o.r;
    if (r.k !== 'alt') return;
    if (r.t === 'start') stack.push({ y: o.y, d: stack.length, guard: r.guard, seps: [] });
    else if (r.t === 'else') stack[stack.length - 1].seps.push({ y: o.y, guard: r.guard });
    else { const f = stack.pop(); f.y2 = o.y + 6; frames.push(f); }
  });
  frames.sort((a, b) => a.d - b.d).forEach((f) => {
    const g = AM.svg('g', { class: 'frame' }, gf), l = xL(f.d), w = xR(f.d) - l;
    AM.svg('rect', { x: l, y: f.y, width: w, height: f.y2 - f.y, rx: 8, class: 'f' }, g);
    AM.svg('path', { d: `M${l},${f.y + 20} h44 l8,-8 v-12`, class: 'tag' }, g);
    AM.svg('text', { x: l + 10, y: f.y + 15, style: 'font-weight:700' }, g, 'alt');
    AM.svg('text', { x: l + 64, y: f.y + 15 }, g, '[' + txt(f.guard, 90) + ']');
    f.seps.forEach((s) => {
      AM.svg('line', { x1: l, y1: s.y + 2, x2: l + w, y2: s.y + 2, stroke: 'var(--edge)', 'stroke-dasharray': '6 4' }, g);
      AM.svg('text', { x: l + 64, y: s.y + 17 }, g, '[' + txt(s.guard, 90) + ']');
    });
  });

  // rows
  const go = (i) => AM.play.go(i);
  const take = async (b) => {
    if (AM.state.busy) return;
    const n = AM.play.list().length;
    if (AM.state.step !== n - 1) await AM.play.go(n - 1);
    AM.play.choose(b);
  };
  const button = (parent, x, y, label, fn) => {
    const w = 18 + label.length * 6.3;
    const g = AM.svg('g', { class: 'sbtn', tabindex: 0, role: 'button' }, parent);
    AM.svg('rect', { x: x - w, y: y - 11, width: w, height: 22, rx: 11 }, g);
    AM.svg('text', { x: x - w / 2, y: y + 4, 'text-anchor': 'middle' }, g, label);
    const run_ = (e) => { e.stopPropagation(); fn(); };
    g.addEventListener('click', run_);
    g.addEventListener('keydown', (e) => { if (e.key === 'Enter') run_(e); });
    return w;
  };
  let depthNow = 0;
  lay.forEach((o) => {
    const r = o.r;
    if (r.k === 'alt') { if (r.t === 'start') depthNow++; else if (r.t === 'end') depthNow--; return; }
    const l = xL(depthNow) , rr = xR(depthNow);
    if (r.k === 'pause') {
      const g = AM.svg('g', { class: 'pause' + (r.pv ? ' pv' : '') }, gm);
      AM.svg('rect', { x: l + 4, y: o.y + 6, width: rr - l - 8, height: 20, fill: 'var(--bg)' }, g);
      AM.svg('path', { d: `M${l + 12},${o.y + 8} l10,4 l-10,4 l10,4 l-10,4`, fill: 'none', stroke: 'var(--muted)' }, g);
      AM.svg('text', { x: W / 2, y: o.y + 20, 'text-anchor': 'middle' }, g, '⏸ ' + AM.t('seqPause'));
      sItems.push({ g, i: r.i, main: false });
      return;
    }
    if (r.k === 'exit') {
      const g = AM.svg('g', { class: 'exitp' }, gm), cx = X.actor;
      const tx = (r.good ? '✓ ' : '✕ ') + AM.t('exit') + ': ' + txt(r.text, 80), w = Math.min(rr - cx + 40, 34 + tx.length * 6.6);
      const col = r.good ? 'var(--good)' : 'var(--critical)';
      AM.svg('rect', { x: cx - 60, y: o.y + 6, width: w, height: 24, rx: 12, fill: `color-mix(in oklab, ${col} 12%, var(--surface))`, stroke: col }, g);
      AM.svg('text', { x: cx - 46, y: o.y + 22, class: 'exit' }, g, tx);
      return;
    }
    if (r.k === 'out') {
      const g = AM.svg('g', { class: 'outc' + (r.taken ? ' taken' : '') }, gm);
      const bad = seqBad.test(r.text || ''), col = bad ? 'var(--critical)' : 'var(--good)';
      const tx = (bad ? '✕ ' : '✓ ') + txt(r.text, 90), w = Math.min(rr - l - 150, 28 + tx.length * 6.6);
      AM.svg('rect', { x: l + 14, y: o.y + 6, width: Math.max(120, w), height: 24, rx: 12, fill: `color-mix(in oklab, ${col} ${r.taken ? 18 : 8}%, var(--surface))`, stroke: col, 'stroke-dasharray': r.taken ? '' : '4 3' }, g);
      AM.svg('text', { x: l + 28, y: o.y + 22, class: 'exit' }, g, tx);
      if (!r.taken) button(g, l + 14 + Math.max(120, w) + 14 + 18 + AM.t('seqPick').length * 6.3, o.y + 18, AM.t('seqPick'), () => take(r.b));
      return;
    }
    if (r.k === 'branch') {
      const g = AM.svg('g', { class: 'brc' }, gm);
      AM.svg('rect', { x: l + 14, y: o.y + 6, width: rr - l - 28, height: 32, rx: 8, class: 'brbox' }, g);
      AM.svg('text', { x: l + 28, y: o.y + 26, class: 'brt' }, g, '↪ ' + AM.t('seqFlowTo') + ': ' + txt(r.target.name, 70) + ' · ' + AM.t('seqSteps', r.n));
      const lab = r.open ? AM.t('seqCollapse') : AM.t('seqExpand');
      const tl = ('↪ ' + AM.t('seqFlowTo') + ': ' + txt(r.target.name, 70) + ' · ' + AM.t('seqSteps', r.n)).length * 6.7;
      let bx = l + 28 + tl + 20 + 18 + lab.length * 6.3;
      bx += 8 + button(g, bx, o.y + 22, lab, () => { if (sOpen.has(r.key)) sOpen.delete(r.key); else sOpen.add(r.key); seqRender(); seqSync(); });
      button(g, bx + 18 + AM.t('seqTake').length * 6.3, o.y + 22, AM.t('seqTake'), () => take(r.b));
      return;
    }
    // message
    const a = X[r.from], b = X[r.to], reply = r.dir === 'reply';
    const yy = o.y + (reply ? 16 : 28);
    const g = AM.svg('g', { class: 'msg ' + (reply ? 'reply' : r.dir === 'async' ? 'async' : '') + (r.pv ? ' pv' : ''), tabindex: r.main ? 0 : null, role: r.main ? 'button' : null, 'data-i': r.i, 'data-fid': r.fid || '' }, gm);
    const mk = reply ? 'url(#arrM)' : r.dir === 'async' ? 'url(#arrO)' : 'url(#arrT)';
    let lx = (a + b) / 2;
    if (r.dir === 'self') {
      const d = `M${a},${yy} h44 v16 h-38`;
      AM.svg('path', { d, 'marker-end': mk }, g); AM.svg('path', { d, class: 'hit' }, g);
      lx = a + 52;
      AM.svg('text', { x: lx, y: yy + 12, 'text-anchor': 'start' }, g, txt(r.text, 60));
    } else {
      const dir = b > a ? 1 : -1;
      AM.svg('line', { x1: a + dir * 4, y1: yy, x2: b - dir * 6, y2: yy, 'marker-end': mk }, g);
      AM.svg('line', { x1: a, y1: yy, x2: b, y2: yy, class: 'hit' }, g);
      const room = Math.max(Math.abs(b - a), colW) * 1.5;
      const label = reply ? (r.text || '') : r.text;
      if (label) AM.svg('text', { x: lx, y: yy - 7, 'text-anchor': 'middle' }, g, txt(label, Math.max(12, Math.floor(room / 6.4))));
      if (reply) AM.svg('title', null, g, AM.t('seqReply'));
    }
    if (r.main && r.num !== false && r.i >= 0) AM.svg('text', { x: Math.min(a, b) - 10, y: yy + 4, 'text-anchor': 'end', class: 'num' }, g, r.i + 1);
    if (r.badges && (r.badges.eff.length || r.badges.st.length)) {
      const pills = seqBadgePills(r.badges), tw = pills.reduce((s, p) => s + p.w + 6, -6);
      const bg = AM.svg('g', { class: 'badges' }, g);
      seqPill(bg, r.dir === 'self' ? lx : Math.max(l + 6, Math.min(lx - tw / 2, rr - tw - 6)), yy + (r.dir === 'self' ? 28 : 20), pills);
    }
    if (r.fname) AM.svg('title', null, g, r.fname);
    const act = () => { if (r.i >= 0 && !r.pv) go(r.i); else if (r.fid) AM.select({ t: 'fn', id: r.fid }); };
    g.addEventListener('click', act);
    g.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.stopPropagation(); act(); } });
    sItems.push({ g, i: r.pv ? -1 : r.i, main: r.main, y: o.y + 20, x: b, fid: r.fid, pv: r.pv });
  });
  sAct = AM.svg('rect', { class: 'act cur', width: 10, rx: 2, x: -100, y: -100, height: 0 }, gl);
  sWrap.scrollTop = top;
  seqSide();
}

/* ---------- side: module mini map ---------- */
function seqSide() {
  sSide.innerHTML = '';
  const box = AM.h('div', { class: 'thumb' }, sSide);
  const cap = AM.h('div', { class: 'cap' }, box, AM.esc(AM.t('seqMapCap')));
  const mp = AM.map.modules;
  sThumb = null;
  if (mp && mp.pos && mp.extent && Object.keys(mp.pos).length) {
    const ex = mp.extent;
    const svg = AM.svg('svg', { viewBox: `${ex.x} ${ex.y} ${ex.w} ${ex.h}`, width: '100%', height: '100%', preserveAspectRatio: 'xMidYMid meet' }, box);
    const nodes = {};
    for (const id in mp.pos) {
      const r = mp.pos[id], ly = AM.map.unitLayer(id);
      nodes[id] = AM.svg('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: 10, fill: ly ? AM.LCOLOR[ly] : 'var(--muted)', opacity: .28 }, svg);
    }
    sThumb = { svg, nodes, fx: AM.svg('g', {}, svg), cap, pos: mp.pos, sw: ex.w / 110 };
  } else AM.h('div', { class: 'loc', style: 'padding:36px 12px 0' }, box, AM.esc(AM.t('seqNoMap')));
  const lg = AM.h('div', { class: 'seqlegend' }, sSide);
  lg.innerHTML = `<div><svg width="34" height="10"><line x1="0" y1="5" x2="30" y2="5" stroke="var(--text-2)" stroke-width="1.4" marker-end="url(#arrT)"/></svg>${AM.esc(AM.t('seqLegend'))}</div>` +
    `<div><svg width="34" height="10"><line x1="0" y1="5" x2="30" y2="5" stroke="var(--muted)" stroke-width="1.3" stroke-dasharray="5 4" marker-end="url(#arrM)"/></svg>${AM.esc(AM.t('seqLegendReply'))}</div>` +
    `<div><svg width="34" height="10"><line x1="0" y1="5" x2="30" y2="5" stroke="var(--text-2)" stroke-width="1.4" stroke-dasharray="2 3" marker-end="url(#arrO)"/></svg>${AM.esc(AM.t('seqLegendAsync'))}</div>`;
  seqThumbSync();
}
function seqThumbSync() {
  const th = sThumb; if (!th) return;
  const s = AM.play.current();
  Object.values(th.nodes).forEach((n) => { n.setAttribute('opacity', .28); n.removeAttribute('stroke'); });
  th.fx.innerHTML = '';
  th.cap.textContent = AM.t('seqMapCap');
  if (!s) return;
  const pk = (f) => AM.map.unitOf(D_().fns[f].module);
  const a = s.prev ? pk(s.prev) : s.actor && th.pos[s.actor.id] ? s.actor.id : null, b = pk(s.fid);
  [a, b].forEach((id) => { const n = id && th.nodes[id]; if (n) { n.setAttribute('opacity', 1); n.setAttribute('stroke', 'var(--accent)'); n.setAttribute('stroke-width', th.sw); } });
  const ra = a && th.pos[a], rb = b && th.pos[b];
  if (ra && rb && a !== b) AM.svg('path', { d: AM.curve(AM.clip(ra, AM.ctr(rb)), AM.clip(rb, AM.ctr(ra)), 0.12), fill: 'none', stroke: 'var(--accent)', 'stroke-width': th.sw * 1.1, 'marker-end': 'url(#arrA)' }, th.fx);
  const nm = (id) => (id ? AM.short(id).replace('actor:', '') : '');
  if (b) th.cap.textContent = AM.t('seqMapCap') + ': ' + [a, b].filter(Boolean).map(nm).join(' → ');
}

/* ---------- sync with the player ---------- */
function seqSig() {
  const r = AM.state.run;
  if (!r) return 'none';
  return r.key + '|' + r.segs.map((s) => s.steps.length + ':' + (s.via ? 1 : 0)).join(',') + '|' + AM.state.exitText + '|' + AM.play.list().length;
}
function seqSync() {
  const cur = AM.state.step;
  let last = null;
  sItems.forEach((it) => {
    if (it.i < 0) return;
    it.g.classList.toggle('cur', it.main && it.i === cur && cur >= 0);
    it.g.classList.toggle('past', cur >= 0 && it.i < cur);
    it.g.classList.toggle('future', cur >= 0 && it.i > cur);
    if (it.main && it.i === cur && cur >= 0) last = it;
  });
  if (sAct) {
    if (last) { sAct.setAttribute('x', last.x - 5); sAct.setAttribute('y', last.y - 4); sAct.setAttribute('height', 26); }
    else { sAct.setAttribute('x', -100); sAct.setAttribute('height', 0); }
  }
  seqThumbSync();
  if (last && AM.state.view === 'sequence') {
    const top = sWrap.scrollTop, hh = sWrap.clientHeight;
    if (last.y < top + HEAD_H + 30 || last.y > top + hh - 70) sWrap.scrollTo({ top: Math.max(0, last.y - hh / 2), behavior: AM.RM ? 'auto' : 'smooth' });
    const lft = sWrap.scrollLeft, wd = sWrap.clientWidth;
    if (last.x < lft + 40 || last.x > lft + wd - 40) sWrap.scrollTo({ left: Math.max(0, last.x - wd / 2), behavior: AM.RM ? 'auto' : 'smooth' });
  } else if (!last && cur < 0 && AM.state.view === 'sequence') sWrap.scrollTop = 0;
}
function seqRefresh(force) {
  const sg = seqSig();
  if (force || sg !== sSig) {
    sSig = sg;
    if (AM.state.view === 'sequence') { seqRender(); } else { sDirty = true; return; }
  }
  seqSync();
}
function seqSelect(sel) {
  if (!sItems.length) return;
  sItems.forEach((it) => it.g.classList.toggle('sel', !!sel && sel.t === 'fn' && it.fid === sel.id && it.main));
}

AM.registerView({
  id: 'sequence',
  order: 10,
  title: () => AM.t('seqTitle'),
  mount(el) {
    sEl = AM.h('div', { class: 'seqv' }, el);
    sWrap = AM.h('div', { class: 'seq-wrap' }, sEl);
    sSide = AM.h('aside', { class: 'seq-side' }, sEl);
    AM.bus.on('flow', () => seqRefresh(false));
  },
  show() {
    if (sDirty || sSig !== seqSig()) { sSig = seqSig(); seqRender(); }
    seqSync();
  },
  onStep() { seqRefresh(false); },
  onSelect(sel) { seqSelect(sel); },
  onLang() { sSig = seqSig(); if (AM.state.view === 'sequence') { seqRender(); seqSync(); } else sDirty = true; },
  onData() { sSig = seqSig(); if (AM.state.view === 'sequence') { seqRender(); seqSync(); } else sDirty = true; },
});
AM.bus.on('resize', () => { if (sWrap && AM.state.view === 'sequence') { seqRender(); seqSync(); } else sDirty = true; });
