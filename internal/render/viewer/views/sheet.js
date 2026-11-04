/* views/sheet.js — the scenario sheet: the current run (scenario, path, variant, walk) on one sheet, a flowchart from
 * left to right. Not a tab: the sequence dock (views/sequence.js) shows it in place of the sequence diagram when its
 * mode is "Sheet" (the default) and calls AM.sheet.render / sync / wheel / select / clear.
 *
 * Columns are layers: actor (and the outcomes) → entry points → logic (domain modules) → adapters → external systems
 * (databases, the queue, outside APIs). On the "functions" level the functions of the steps sit in frames of their
 * modules; "modules" folds a frame into one node (arrows between modules list their step numbers "3–5"); "system" is the
 * actor, the service and the external systems. Each step is an arrow from its caller to its function, numbered in the
 * order of the run; a node visited again keeps one vertex and gets one more numbered arrow; a hand-off through the queue
 * is two dashed arrows ("enqueue" / "from the queue"); a planned step (`step plan`) is a dashed node in the frame of
 * its module; branches leave the fork node with "Na/Nb" and their condition towards an outcome or another scenario.
 *
 * Layout (no libraries): items (frames and free nodes) are stacked per column, ordered by the median of their
 * neighbours (first by step numbers, then by the y of the neighbours, several sweeps, the order with the fewest
 * crossings wins), then pulled toward their neighbours. Arrows are orthogonal with rounded corners: they leave and
 * enter nodes at ports spread over the sides, turn in the gaps between columns (tracks, so parallel arrows do not
 * overlap) and cross other columns only in free corridors between the frames. Number circles sit on the arrows in the
 * gaps, effect labels next to them where they cover nothing (else the least covered place).
 *
 * The sheet is fitted to the dock (never below a readable scale; then it scrolls and can be dragged); playing moves
 * nothing — the current arrow and node are lit, a dot runs along the arrow, past steps stay saturated, future ones are
 * dimmed. A level change or a step of a walk re-lays the sheet and animates every node from its old place (FLIP). */
AM.addI18n({
  ru: {
    shSheet: 'Лист', shSeq: 'Последовательность', shSheetTip: 'Сценарий на одном листе: слои слева направо, пронумерованные стрелки',
    shSeqTip: 'Диаграмма последовательности', shColAct: 'Актор и итог', shColIn: 'Входы', shColDom: 'Логика', shColAd: 'Адаптеры',
    shColExt: 'Внешние', shColSvc: 'Сервис', shEnq: 'поставить', shDeq: 'из очереди', shExit: 'Итог', shPlan: 'план',
    shMore: (n, m) => 'показаны первые ' + n + ' из ' + m + ' шагов', shWalk: 'клик по стрелке — шаг · ↑ ↓ — выбрать · ← — назад',
    shFlow: 'сценарий', shInner: 'шаги внутри', shPick: 'выбрать этот исход', shGoFork: 'к развилке', shEmpty: 'Нет сценария для показа',
    shNoCode: 'нет в коде',
  },
  en: {
    shSheet: 'Sheet', shSeq: 'Sequence', shSheetTip: 'The scenario on one sheet: layers from left to right, numbered arrows',
    shSeqTip: 'Sequence diagram', shColAct: 'Actor and outcome', shColIn: 'Entry', shColDom: 'Logic', shColAd: 'Adapters',
    shColExt: 'External', shColSvc: 'Service', shEnq: 'enqueue', shDeq: 'from the queue', shExit: 'Outcome', shPlan: 'plan',
    shMore: (n, m) => 'the first ' + n + ' of ' + m + ' steps', shWalk: 'click an arrow — step · ↑ ↓ — choose · ← — back',
    shFlow: 'scenario', shInner: 'steps inside', shPick: 'pick this outcome', shGoFork: 'to the fork', shEmpty: 'No scenario to show',
    shNoCode: 'not in code',
  },
});

const SH_MAX = 150;      // longer runs are cut (the sequence diagram shows them whole)
const SH_MINZ = 0.75;    // the smallest scale: 11.5px labels stay ≥ 8.6px; below it the sheet scrolls
const SH_MAXZ = 1.2;
const SH_COLS = { 0: ['act', 'svc', 'ext'], 1: ['act', 'in', 'dom', 'ad', 'ext'], 2: ['act', 'in', 'dom', 'ad', 'ext'] };
const SH_COLT = { act: 'shColAct', in: 'shColIn', dom: 'shColDom', ad: 'shColAd', ext: 'shColExt', svc: 'shColSvc' };
const SH_COL_OF = { entry: 'in', ui: 'in', domain: 'dom', other: 'dom', test: 'dom', adapter: 'ad', infra: 'ad' };
const SH_BAD = /(fail|error|cancel|reject|denied|block|expire|timeout|ошиб|отмен|отклон|истек|истёк|отказ|не оплач|неуспеш)/i;
const SH_W = { fn: 190, plan: 190, mod: 200, svc: 230, actor: 160, ext: 170, queue: 170, out: 184, flowref: 184 };
const SH_H = { fn: 30, plan: 30, mod: 44, svc: 64, actor: 36, ext: 40, queue: 40, out: 34, flowref: 34 };
const SH_GAP = { 0: 170, 1: 132, 2: 124 };
const FP = 8, FT = 22, NG = 8, IG = 20, Y0 = 58, X0 = 18, TS = 8, R = 6;

const SH = { svg: null, host: null, box: null, strip: null, m: null, lay: null, key: '', lv: -1, z: 1, uz: 1, els: null, dotRaf: 0, lastStep: -2, flipT: 0 };
const S = (tag, attrs, parent, text) => AM.svg(tag, attrs, parent, text);

/* ---------- text metrics ---------- */
let mctx = null, FONT = '', MONO = '';
function tw(s, px, mono, bold) {
  if (!mctx) {
    mctx = document.createElement('canvas').getContext('2d');
    const cs = getComputedStyle(document.documentElement);
    FONT = cs.getPropertyValue('--font') || 'sans-serif'; MONO = cs.getPropertyValue('--mono') || 'monospace';
  }
  mctx.font = (bold ? '600 ' : '') + px + 'px ' + (mono ? MONO : FONT);
  return mctx.measureText(String(s)).width;
}
function fit(s, maxW, px, mono, bold) {
  s = String(s || '');
  if (tw(s, px, mono, bold) <= maxW) return s;
  let lo = 0, hi = s.length;
  while (lo < hi) { const mid = (lo + hi + 1) >> 1; if (tw(s.slice(0, mid) + '…', px, mono, bold) <= maxW) lo = mid; else hi = mid - 1; }
  return s.slice(0, Math.max(1, lo)) + '…';
}
// step numbers as text: [2,3,4,7] → "3–5, 8"
function numText(list) {
  const a = Array.from(new Set(list)).sort((x, y) => x - y).map((i) => i + 1), out = [];
  for (let k = 0; k < a.length;) {
    let j = k; while (j + 1 < a.length && a[j + 1] === a[j] + 1) j++;
    out.push(j > k + 1 ? a[k] + '–' + a[j] : j === k + 1 ? a[k] + ', ' + a[j] : String(a[k]));
    k = j + 1;
  }
  let s = out[0];
  for (let k = 1; k < out.length; k++) { if ((s + ', ' + out[k]).length > 13) return s + ', …'; s += ', ' + out[k]; }
  return s;
}
// a module path that does not fit keeps its end: "…/petclinic/owner"
function fitTail(s, maxW, px, bold) {
  s = String(s || '');
  if (tw(s, px, false, bold) <= maxW) return s;
  const parts = s.split('/');
  for (let k = 1; k < parts.length; k++) { const t = '…/' + parts.slice(k).join('/'); if (tw(t, px, false, bold) <= maxW) return t; }
  return fit(parts[parts.length - 1], maxW, px, false, bold);
}
const median = (xs) => { if (!xs.length) return null; const s = xs.slice().sort((a, b) => a - b), m = s.length >> 1; return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2; };

/* ---------- model: nodes, frames, numbered edges ---------- */
function shModel(lv) {
  const D = AM.D, P = AM.play, run = AM.state.run;
  if (!run) return null;
  const total = P.list().length;
  if (!total) return null;
  const n = Math.min(total, SH_MAX);
  const steps = []; for (let i = 0; i < n; i++) steps.push(P.stepAt(i));
  const nodes = new Map(), frames = new Map(), edges = new Map();
  const PJ = D.F.project || {};
  const colOf = (mod) => SH_COL_OF[AM.layerOf(mod)] || 'dom';
  const node = (id, o) => { let x = nodes.get(id); if (!x) { x = Object.assign({ id, steps: [], inner: [], tags: [], first: 1e9 }, o); nodes.set(id, x); } return x; };
  const frameOf = (mod) => {
    let f = frames.get('F:' + mod);
    if (!f) { f = { id: 'F:' + mod, mod, col: colOf(mod), label: AM.short(mod), layer: AM.layerOf(mod), nodes: [], plan: !D.mods[mod] }; frames.set(f.id, f); }
    return f;
  };
  const inFrame = (nd, mod) => { if (!nd.frame) { const f = frameOf(mod); nd.frame = f.id; f.nodes.push(nd.id); } return nd; };
  const svcNode = () => node('svc', { kind: 'svc', col: 'svc', label: PJ.name || D.M.title || D.F.repo || AM.t('seqSvc') });
  const modNode = (mod, ghost) => node('m:' + mod, { kind: 'mod', col: colOf(mod), mod, label: AM.short(mod), layer: AM.layerOf(mod), plan: !D.mods[mod], ghost: !!ghost });
  const fnNode = (fid, ghost) => {
    const mod = D.fns[fid].module;
    if (lv === 0) return svcNode();
    if (lv === 1) return modNode(mod, ghost);
    return inFrame(node('f:' + fid, { kind: 'fn', col: colOf(mod), fid, mod, label: AM.fnInfo(fid).name, layer: AM.layerOf(mod), ghost: !!ghost }), mod);
  };
  const planNode = (s) => {
    if (lv === 0) return svcNode();
    if (lv === 1 && s.mod) return modNode(s.mod);
    const nd = node('p:' + s.fid, { kind: 'plan', col: s.mod ? colOf(s.mod) : 'dom', mod: s.mod, label: s.label, fid: s.fid, layer: s.mod ? AM.layerOf(s.mod) : 'other' });
    return s.mod ? inFrame(nd, s.mod) : nd;
  };
  const actorNode = (name, actor) => node('a:' + name, { kind: 'actor', col: 'act', label: name, actor });
  const extNode = (x) => node('x:' + x.id, { kind: x.kind === 'queue' ? 'queue' : 'ext', col: 'ext', ext: x, label: x.name, sub: (AM.t('kinds') || {})[x.kind] || '' });
  const queueExt = D.externals.find((x) => x.kind === 'queue');
  const queueNode = () => (queueExt ? extNode(queueExt) : node('x:queue', { kind: 'queue', col: 'ext', label: AM.t('queue'), sub: '' }));
  const edge = (a, b, kind, i, o) => {
    const k = a.id + '>' + b.id + '>' + kind;
    let e = edges.get(k);
    if (!e) { e = Object.assign({ id: k, a: a.id, b: b.id, kind, steps: [], tags: [], num: true }, o); edges.set(k, e); }
    if (i != null && !e.steps.includes(i)) e.steps.push(i);
    return e;
  };
  const addTags = (o, tags) => tags.forEach((x) => { if (x && !o.tags.includes(x)) o.tags.push(x); });
  // the function that makes the step's call: the previous step, or a frame below it on the call stack that reaches it
  const callerOf = (s) => {
    if (!s.prev) return null;
    if (s.planned || s.kind === 'async' || AM.isCall(s.prev, s.fid)) return s.prev;
    const stack = s.i > 0 ? P.frames(s.i - 1).slice().reverse() : [];
    const c = [s.prev].concat(stack.filter((f) => f !== s.prev && D.fns[f]));
    return c.find((f) => AM.isCall(f, s.fid) || AM.isAsync(f, s.fid)) || c.find((f) => AM.callPath(f, s.fid, 8)) || s.prev;
  };
  const stTag = (x) => String(x.entity).replace(/(Status|State)$/, '') + ' → ' + x.to;
  const effTags = (s) => AM.ui.effBadges(s).filter((b) => b.op !== 'READ').map((b) => b.text);
  const segFirst = []; { let acc = 0; run.segs.forEach((sg) => { segFirst.push(acc); acc += sg.steps.length; }); }
  const actorFor = (s) => {
    const fl = run.segs[s.seg].flow;
    const nm = (s.seg === 0 && run.actor) || (fl && fl.actor) || (s.actor ? AM.actorName(s.actor) : '') || AM.t('seqEntry');
    return actorNode(String(nm).trim(), s.actor);
  };
  const extsOfStep = (s) => {
    if (s.planned) return s.exts || [];
    const fl = run.segs[s.seg].flow, named = fl && fl.ext && fl.ext[s.fid];
    const xs = (D.extByFn[s.fid] || []).filter((x) => x.kind !== 'queue').slice(0, 2);
    if (named) D.externals.filter((x) => x.name === named && !xs.includes(x)).forEach((x) => xs.push(x));
    return xs;
  };

  steps.forEach((s, i) => {
    const sg = run.segs[s.seg];
    const to = s.planned ? planNode(s) : fnNode(s.fid);
    to.steps.push(i); to.first = Math.min(to.first, i);
    const src = callerOf(s);
    let from, kind = 'call';
    // a planned step (no code yet) follows the step before it in its flow
    if (s.planned && i > segFirst[s.seg]) from = steps[i - 1].planned ? planNode(steps[i - 1]) : fnNode(steps[i - 1].fid);
    else if (src && D.fns[src]) from = fnNode(src);
    else if (i === segFirst[s.seg] && s.seg > 0 && sg.via && sg.via.at && D.fns[sg.via.at]) { from = fnNode(sg.via.at); kind = 'branch'; }
    else { from = actorFor(s); kind = 'entry'; }
    if (s.planned) kind = 'plan';
    const xs = extsOfStep(s), db = xs.some((x) => x.kind === 'db');
    const tags = (s.states || []).map(stTag).concat(db ? [] : effTags(s));
    let main = null;
    if (s.kind === 'async') {
      const q = queueNode();
      const e1 = edge(from, q, 'enq', i);
      addTags(e1, [AM.t('shEnq') + ' ' + AM.fnInfo(s.fid).short]);
      main = edge(q, to, 'deq', i, { num: false });
      addTags(main, [AM.t('shDeq')]);
      addTags(main, tags);
    } else if (from === to) {
      if (lv < 2) { to.inner.push(i); addTags(to, tags); } else { main = edge(from, to, 'self', i); addTags(main, tags); }
    } else {
      main = edge(from, to, kind, i);
      if (kind === 'branch') addTags(main, [sg.via.cond]);
      addTags(main, tags);
    }
    if (main && i === segFirst[s.seg] && run.segs.length > 1) main.seg = s.seg;
    xs.forEach((x) => {
      const xn = extNode(x);
      if (xn === to) return;
      const e = edge(to, xn, 'ext', i, { num: false });
      if (x.kind === 'db') addTags(e, effTags(s));
    });
  });

  // branches that the run did not take: from the fork node, "Na/Nb" with the condition, to an outcome or a scenario
  const walking = AM.walk && AM.walk.on;
  if (!run.fixed && !walking) {
    const can = AM.play.branches() || [];
    run.segs.forEach((sg, si) => {
      const fl = sg.flow; if (!fl || !(fl.branches || []).length) return;
      const next = run.segs[si + 1], byAt = new Map();
      fl.branches.forEach((b) => {
        const at = b.at && D.fns[b.at] && sg.steps.includes(b.at) ? b.at : sg.steps.filter((x) => D.fns[x]).pop();
        if (!at) return;
        if (!byAt.has(at)) byAt.set(at, []);
        byAt.get(at).push(b);
      });
      byAt.forEach((list, at) => {
        let k = -1;
        for (let i = segFirst[si]; i < Math.min(n, segFirst[si] + sg.steps.length); i++) if (steps[i].fid === at) k = i;
        if (k < 0) return;
        const fork = fnNode(at);
        list.forEach((b, bi) => {
          if (next && next.via === b) return; // taken: the next flow's entry arrow is this branch
          const no = (k + 1) + String.fromCharCode(97 + Math.min(bi, 25)), key = b.text || b.cond || String(bi);
          const tf = b.flow && D.flowById[b.flow];
          const tn = tf ? node('o:f:' + b.flow, { kind: 'flowref', col: 'act', label: tf.name, flow: tf, first: k + 0.5 + bi / 100 })
            : node('o:t:' + key, { kind: 'out', col: 'act', label: b.text || b.cond, tone: SH_BAD.test((b.cond || '') + ' ' + (b.text || '')) ? 'bad' : 'good', first: k + 0.5 + bi / 100 });
          tn.b = tn.b || b;
          const e = edge(fork, tn, 'branch', null, { numText: no, br: b, at: k, choice: can.includes(b) });
          addTags(e, [b.cond]);
          if (AM.state.exitText && AM.state.exitText === (b.text || b.cond)) { e.taken = true; tn.taken = true; }
        });
      });
    });
  }

  // a walk: the fan of the arrows that leave the current step (calls, the queue, external systems)
  if (walking && n) {
    const last = steps[n - 1], from = last.planned ? planNode(last) : fnNode(last.fid);
    AM.walk.options().forEach((o, k) => {
      let tn;
      if (o.fid && D.fns[o.fid]) tn = fnNode(o.fid, true);
      else { const x = D.externals.find((y) => y.name === o.ext); if (!x) return; tn = extNode(x); }
      if (!tn.steps.length && tn.first === 1e9) { tn.first = 1e6 + k; if (tn.kind !== 'ext' && tn.kind !== 'queue') tn.ghost = true; }
      if (tn === from) return; // a call inside the same module on a folded level
      const e = edge(from, tn, 'opt', null, { opt: o, optK: k, numText: String(k + 1), guided: o.guided, seen: o.seen });
      if (o.kind === 'queue') addTags(e, [AM.t('queue')]);
    });
  }

  // the order key of nodes that have no steps of their own (actors, externals): the first step that touches them
  const E = Array.from(edges.values());
  E.forEach((e) => {
    const lo = e.steps.length ? Math.min(...e.steps) : e.at != null ? e.at + 0.5 : null;
    if (lo == null) return;
    [nodes.get(e.a), nodes.get(e.b)].forEach((x) => { if (x && !x.steps.length && x.first > lo) x.first = lo; });
  });
  frames.forEach((f) => { f.ghost = f.nodes.every((id) => nodes.get(id).ghost); });
  const used = new Set(); nodes.forEach((x) => used.add(x.frame ? frames.get(x.frame).col : x.col));
  const cols = SH_COLS[lv].filter((c) => c === 'act' || used.has(c));
  return { lv, nodes, frames, edges: E, cols, steps, n, total, segFirst, run };
}

/* ---------- layout ---------- */
function shLayout(m, stable) {
  const ci = {}; m.cols.forEach((c, i) => { ci[c] = i; });
  const nc = m.cols.length, last = nc - 1;
  const colOfN = (nd) => ci[nd.frame ? m.frames.get(nd.frame).col : nd.col];
  m.nodes.forEach((nd) => { nd.c = colOfN(nd); nd.w = SH_W[nd.kind]; nd.ports = { L: [], R: [] }; });
  // sides: forward arrows leave on the right and enter on the left, backward ones the other way round, arrows inside a
  // column loop through the gap on its right (on its left for the last column)
  m.edges.forEach((e) => {
    const A = m.nodes.get(e.a), B = m.nodes.get(e.b);
    if (e.a === e.b) { e.sa = e.sb = 'R'; } else if (A.c < B.c) { e.sa = 'R'; e.sb = 'L'; } else if (A.c > B.c) { e.sa = 'L'; e.sb = 'R'; } else { e.sa = e.sb = A.c < last ? 'R' : 'L'; }
    A.ports[e.sa].push({ e, end: 'a' }); B.ports[e.sb].push({ e, end: 'b' });
  });
  m.nodes.forEach((nd) => { const k = Math.max(nd.ports.L.length, nd.ports.R.length); nd.h = Math.max(SH_H[nd.kind], 10 + (nd.kind === 'fn' || nd.kind === 'plan' || nd.kind === 'mod' ? 11 : 6) * k); });
  m.frames.forEach((f) => { f.w = SH_W.fn + 2 * FP; });
  // items of the columns: frames and free nodes
  const items = m.cols.map(() => []);
  m.frames.forEach((f) => items[ci[f.col]].push({ fr: f, id: f.id }));
  m.nodes.forEach((nd) => { if (!nd.frame) items[nd.c].push({ nd, id: nd.id }); });
  const keyOfNode = (nd) => nd.first;
  const fNodes = (f) => f.nodes.map((id) => m.nodes.get(id));
  const itemKey = (it) => (it.fr ? Math.min(...fNodes(it.fr).map(keyOfNode)) : keyOfNode(it.nd));
  items.forEach((list) => list.sort((a, b) => itemKey(a) - itemKey(b) || (a.id < b.id ? -1 : 1)));
  m.frames.forEach((f) => f.nodes.sort((a, b) => m.nodes.get(a).first - m.nodes.get(b).first || (a < b ? -1 : 1)));
  const itemH = (it) => (it.fr ? FT + fNodes(it.fr).reduce((s, nd) => s + nd.h, 0) + NG * (it.fr.nodes.length - 1) + FP : it.nd.h);
  const stack = (c) => {
    let y = Y0;
    items[c].forEach((it) => { it.y = y; it.h = itemH(it); placeIn(it); y += it.h + IG; });
  };
  const placeIn = (it) => {
    if (it.nd) { it.nd.y = it.y; return; }
    let y = it.y + FT;
    fNodes(it.fr).forEach((nd) => { nd.y = y; y += nd.h + NG; });
    it.fr.y = it.y; it.fr.h = it.h;
  };
  const yc = (nd) => nd.y + nd.h / 2;
  items.forEach((_, c) => stack(c));
  const nbrs = new Map(); m.nodes.forEach((nd) => nbrs.set(nd.id, []));
  m.edges.forEach((e) => { if (e.a !== e.b) { nbrs.get(e.a).push(e.b); nbrs.get(e.b).push(e.a); } });
  const crossings = () => {
    const segs = [];
    m.edges.forEach((e) => {
      const A = m.nodes.get(e.a), B = m.nodes.get(e.b); if (A.c === B.c) return;
      segs.push(A.c < B.c ? [A.c, yc(A), B.c, yc(B)] : [B.c, yc(B), A.c, yc(A)]);
    });
    let x = 0;
    for (let i = 0; i < segs.length; i++) for (let j = i + 1; j < segs.length; j++) {
      const a = segs[i], b = segs[j];
      if (a[0] === b[0] && a[2] === b[2] && (a[1] - b[1]) * (a[3] - b[3]) < 0) x++;
      else if (a[0] < b[2] && b[0] < a[2] && a[0] !== b[0] && a[2] !== b[2]) {
        // spans overlap: compare where both are, at the inner boundary columns
        const ya = (c) => a[1] + (a[3] - a[1]) * (c - a[0]) / (a[2] - a[0]), yb = (c) => b[1] + (b[3] - b[1]) * (c - b[0]) / (b[2] - b[0]);
        const lo = Math.max(a[0], b[0]), hi = Math.min(a[2], b[2]);
        if ((ya(lo) - yb(lo)) * (ya(hi) - yb(hi)) < 0) x++;
      }
    }
    return x;
  };
  const snap = () => ({ items: items.map((l) => l.slice()), fr: Array.from(m.frames.values()).map((f) => [f, f.nodes.slice()]) });
  const restore = (s) => { s.items.forEach((l, c) => { items[c] = l; }); s.fr.forEach(([f, ns]) => { f.nodes = ns; }); items.forEach((_, c) => stack(c)); };
  if (!stable) {
    let best = snap(), bestX = crossings();
    for (let it = 0; it < 8 && bestX > 0; it++) {
      const order = it % 2 ? m.cols.map((_, c) => last - c) : m.cols.map((_, c) => c);
      order.forEach((c) => {
        const key = {};
        items[c].forEach((x) => (x.fr ? fNodes(x.fr) : [x.nd]).forEach((nd) => {
          const ys = nbrs.get(nd.id).map((id) => m.nodes.get(id)).filter((o) => o.c !== nd.c).map(yc);
          const md = median(ys);
          key[nd.id] = md == null ? yc(nd) : md * 0.8 + yc(nd) * 0.2;
        }));
        const ik = (x) => (x.fr ? fNodes(x.fr).reduce((s, nd) => s + key[nd.id], 0) / x.fr.nodes.length : key[x.nd.id]);
        const pos = new Map(items[c].map((x, i) => [x, i]));
        items[c].sort((a, b) => ik(a) - ik(b) || pos.get(a) - pos.get(b));
        items[c].forEach((x) => { if (x.fr) { const p = new Map(x.fr.nodes.map((id, i) => [id, i])); x.fr.nodes.sort((a, b) => key[a] - key[b] || p.get(a) - p.get(b)); } });
        stack(c);
      });
      const cx = crossings();
      if (cx < bestX) { bestX = cx; best = snap(); }
    }
    restore(best);
    m.crossings = bestX;
  }
  // pull the items toward their neighbours (straighter arrows), keeping the order and the gaps
  const pull = (c) => {
    let prev = Y0 - IG;
    items[c].forEach((it) => {
      const ns = it.fr ? fNodes(it.fr) : [it.nd];
      const d = median(ns.map((nd) => { const md = median(nbrs.get(nd.id).map((id) => m.nodes.get(id)).filter((o) => o.c !== nd.c).map(yc)); return md == null ? 0 : md - yc(nd); }).filter((v) => v !== 0));
      it.y = Math.max(prev + IG, it.y + (d || 0));
      placeIn(it); prev = it.y + it.h;
    });
  };
  for (let p = 0; p < 3; p++) (p % 2 ? m.cols.map((_, c) => last - c) : m.cols.map((_, c) => c)).forEach(pull);
  let minY = Infinity; items.forEach((l) => l.forEach((it) => { minY = Math.min(minY, it.y); }));
  const dy = Y0 - minY; items.forEach((l) => l.forEach((it) => { it.y += dy; placeIn(it); }));
  let maxY = 0; items.forEach((l) => l.forEach((it) => { maxY = Math.max(maxY, it.y + it.h); }));

  // ports: spread the arrow ends over the sides, ordered by the y of the other end
  m.nodes.forEach((nd) => ['L', 'R'].forEach((sd) => {
    const ps = nd.ports[sd];
    ps.forEach((p) => { const o = m.nodes.get(p.end === 'a' ? p.e.b : p.e.a); p.k = yc(o) + (o === nd ? (p.end === 'a' ? -1 : 1) : 0); });
    ps.sort((a, b) => a.k - b.k);
    ps.forEach((p, i) => { p.e['y' + p.end] = nd.y + (i + 1) * nd.h / (ps.length + 1); });
  }));

  // routing: vertical runs in the gaps (tracks), horizontal runs across other columns only in free corridors
  const occ = items.map((l) => l.map((it) => [it.y - 6, it.y + it.h + 6]));
  const freeIn = (c, y) => occ[c].every(([a, b]) => y < a || y > b);
  const hUsed = [];
  const hFree = (y, c1, c2) => hUsed.every((u) => Math.abs(u.y - y) >= 7 || u.c2 <= c1 || c2 <= u.c1);
  const useH = (y, c1, c2) => hUsed.push({ y, c1, c2 });
  const corridor = (mids, target, c1, c2) => {
    const cands = [target, Y0 - 16, maxY + 16];
    mids.forEach((c) => items[c].forEach((it) => { cands.push(it.y - IG / 2, it.y + it.h + IG / 2); }));
    const ok = (y) => mids.every((c) => freeIn(c, y)) && hFree(y, c1, c2);
    const list = cands.filter((y) => mids.every((c) => freeIn(c, y))).sort((a, b) => Math.abs(a - target) - Math.abs(b - target));
    for (const y of list) { if (hFree(y, c1, c2)) return y; for (let k = 1; k <= 3; k++) { if (ok(y + 7 * k)) return y + 7 * k; if (ok(y - 7 * k)) return y - 7 * k; } }
    return maxY + 16 + 7 * hUsed.length;
  };
  const vsegs = new Map(); // gap -> [vs]
  const VS = (gap, y1, y2) => { const v = { gap, y1: Math.min(y1, y2), y2: Math.max(y1, y2), x: 0 }; if (!vsegs.has(gap)) vsegs.set(gap, []); vsegs.get(gap).push(v); return v; };
  m.edges.forEach((e) => {
    const A = m.nodes.get(e.a), B = m.nodes.get(e.b), P = { y: e.ya }, Q = { y: e.yb };
    e.A = A; e.B = B;
    if (e.a === e.b) { e.pts = null; e.self = true; return; }
    const ca = A.c, cb = B.c;
    const straight = Math.abs(P.y - Q.y) < 0.5;
    if (ca === cb) { const v = VS(e.sa === 'R' ? ca : ca - 1, P.y, Q.y); e.pts = [P, { v, y: P.y }, { v, y: Q.y }, Q]; return; }
    const fwd = ca < cb, lo = Math.min(ca, cb), hi = Math.max(ca, cb);
    const mids = []; for (let c = lo + 1; c < hi; c++) mids.push(c);
    const g1 = fwd ? ca : ca - 1, g2 = fwd ? cb - 1 : cb;
    if (!mids.length) {
      if (straight) { e.pts = [P, Q]; return; }
      const v = VS(g1, P.y, Q.y); e.pts = [P, { v, y: P.y }, { v, y: Q.y }, Q]; return;
    }
    if (mids.every((c) => freeIn(c, P.y)) && hFree(P.y, lo, hi)) {
      useH(P.y, lo, hi);
      if (straight) { e.pts = [P, Q]; return; }
      const v = VS(g2, P.y, Q.y); e.pts = [P, { v, y: P.y }, { v, y: Q.y }, Q]; return;
    }
    if (mids.every((c) => freeIn(c, Q.y)) && hFree(Q.y, lo, hi)) {
      useH(Q.y, lo, hi);
      const v = VS(g1, P.y, Q.y); e.pts = [P, { v, y: P.y }, { v, y: Q.y }, Q]; return;
    }
    const yh = corridor(mids, (P.y + Q.y) / 2, lo, hi); useH(yh, lo, hi);
    const v1 = VS(g1, P.y, yh), v2 = VS(g2, yh, Q.y);
    e.pts = [P, { v: v1, y: P.y }, { v: v1, y: yh }, { v: v2, y: yh }, { v: v2, y: Q.y }, Q];
  });
  // tracks per gap: interval colouring, the gap as wide as its tracks need
  const tracks = {};
  vsegs.forEach((list, g) => {
    list.sort((a, b) => a.y1 - b.y1 || (b.y2 - b.y1) - (a.y2 - a.y1));
    const ends = [];
    list.forEach((v) => { let t = ends.findIndex((end) => end + 5 < v.y1); if (t < 0) { t = ends.length; ends.push(0); } ends[t] = v.y2; v.t = t; });
    tracks[g] = ends.length;
  });
  const gapW = (g) => Math.max(SH_GAP[m.lv], 34 + (tracks[g] || 0) * TS);
  const colW = m.cols.map((_, c) => Math.max(120, ...items[c].map((it) => (it.fr ? it.fr.w : it.nd.w))));
  const colX = []; let x = X0 + (tracks[-1] ? gapW(-1) : 0);
  m.cols.forEach((_, c) => { colX[c] = x; x += colW[c] + (c < last ? gapW(c) : 0); });
  const W = x + (tracks[last] ? gapW(last) : 0) + X0;
  const gapL = (g) => (g < 0 ? X0 : colX[g] + colW[g]), gapR = (g) => (g >= last ? W - X0 : colX[g + 1]);
  vsegs.forEach((list, g) => { const T = tracks[g], mid = (gapL(g) + gapR(g)) / 2; list.forEach((v) => { v.x = mid + (v.t - (T - 1) / 2) * TS; }); });
  items.forEach((list, c) => list.forEach((it) => {
    if (it.nd) { it.nd.x = colX[c] + (colW[c] - it.nd.w) / 2; return; }
    it.fr.x = colX[c] + (colW[c] - it.fr.w) / 2;
    fNodes(it.fr).forEach((nd) => { nd.x = it.fr.x + FP; });
  }));
  // the points of the arrows
  m.edges.forEach((e) => {
    const sx = (nd, sd) => (sd === 'R' ? nd.x + nd.w : nd.x);
    if (e.self) {
      const xr = e.A.x + e.A.w, y = e.A.y + e.A.h / 2;
      e.P = [{ x: xr, y: y - 6 }, { x: xr + 18, y: y - 6 }, { x: xr + 18, y: y + 6 }, { x: xr, y: y + 6 }];
      return;
    }
    e.P = e.pts.map((p, i) => ({ x: i === 0 ? sx(e.A, e.sa) : i === e.pts.length - 1 ? sx(e.B, e.sb) : p.v.x, y: p.y }));
  });
  return { W, H: maxY + 30, colX, colW, items, ci };
}

/* ---------- labels: number circles and effect tags without overlaps ---------- */
function shLabels(m, lay) {
  const obst = [];
  m.nodes.forEach((nd) => obst.push({ x: nd.x - 2, y: nd.y - 2, w: nd.w + 4, h: nd.h + 4 }));
  m.frames.forEach((f) => obst.push({ x: f.x, y: f.y, w: f.w, h: FT - 2 }));
  const over = (a) => obst.reduce((s, b) => s + Math.max(0, Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x)) * Math.max(0, Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y)), 0);
  const order = m.edges.slice().sort((a, b) => (a.steps.length ? Math.min(...a.steps) : 1e6) - (b.steps.length ? Math.min(...b.steps) : 1e6));
  const segsOf = (e) => { const out = []; for (let i = 0; i + 1 < e.P.length; i++) out.push([e.P[i], e.P[i + 1]]); return out; };
  // the number: on the first vertical run long enough, else on the longest horizontal run, sliding off other circles
  order.forEach((e) => {
    const txt = e.numText || (e.num && e.steps.length ? numText(e.steps) : '');
    e.nt = txt;
    if (!txt) return;
    const w = Math.max(18, tw(txt, 10.5, false, true) + 10), h = 18;
    const ss = segsOf(e);
    let base = ss.find(([a, b]) => a.x === b.x && Math.abs(a.y - b.y) >= 26);
    let along = 'y';
    if (!base) { base = ss.slice().sort((p, q) => Math.abs(q[1].x - q[0].x) - Math.abs(p[1].x - p[0].x))[0]; along = 'x'; }
    const [a, b] = base, mid = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
    let best = null, bs = Infinity;
    for (const d of [0, 14, -14, 28, -28, 42, -42]) {
      const c = along === 'y' ? { x: mid.x, y: Math.min(Math.max(a.y, b.y) - 9, Math.max(Math.min(a.y, b.y) + 9, mid.y + d)) } : { x: Math.min(Math.max(a.x, b.x) - w / 2 - 4, Math.max(Math.min(a.x, b.x) + w / 2 + 4, mid.x + d)), y: mid.y };
      const box = { x: c.x - w / 2, y: c.y - h / 2, w, h }, sc = over(box) + Math.abs(d) * 0.5;
      if (sc < bs) { bs = sc; best = box; }
      if (sc === 0) break;
    }
    e.nb = best; obst.push(best);
  });
  // the tags (effects, conditions): next to the number (or the middle of the arrow), the place that covers least
  order.forEach((e) => {
    const lines = (e.tags || []).slice(0, 2).map((s) => fit(s, 200, 11, true));
    if ((e.tags || []).length > 2) lines[1] = fit(lines[1] + ' …', 200, 11, true);
    e.tl = lines;
    if (!lines.length) return;
    const w = Math.max(...lines.map((s) => tw(s, 11, true))) + 10, h = lines.length * 14 + 4;
    let c;
    if (e.nb) c = { x: e.nb.x + e.nb.w / 2, y: e.nb.y + e.nb.h / 2, r: e.nb.w / 2 };
    else { const ss = segsOf(e).sort((p, q) => Math.hypot(q[1].x - q[0].x, q[1].y - q[0].y) - Math.hypot(p[1].x - p[0].x, p[1].y - p[0].y))[0]; c = { x: (ss[0].x + ss[1].x) / 2, y: (ss[0].y + ss[1].y) / 2, r: 2 }; }
    const cand = [
      [c.x + c.r + 4, c.y - h / 2], [c.x - c.r - 4 - w, c.y - h / 2], [c.x - w / 2, c.y - c.r - 4 - h], [c.x - w / 2, c.y + c.r + 4],
      [c.x + 4, c.y - c.r - 2 - h], [c.x - w - 4, c.y - c.r - 2 - h], [c.x + 4, c.y + c.r + 2], [c.x - w - 4, c.y + c.r + 2],
      [c.x + c.r + 26, c.y - h - 12], [c.x - c.r - 26 - w, c.y - h - 12], [c.x + c.r + 26, c.y + 12], [c.x - c.r - 26 - w, c.y + 12],
    ];
    // farther away (with a leader line) when everything near is taken
    [44, 70, 100].forEach((d) => { cand.push([c.x - w / 2, c.y - d - h], [c.x - w / 2, c.y + d], [c.x + 8, c.y - d - h], [c.x - w - 8, c.y + d]); });
    let best = null, bs = Infinity;
    cand.forEach(([x, y], k) => {
      const box = { x, y, w, h }, sc = over(box) * 4 + k * 3 + (x < 4 || x + w > lay.W - 4 ? 1e5 : 0);
      if (sc < bs) { bs = sc; best = box; }
    });
    e.tb = best; e.tc = c; e.lead = Math.hypot(best.x + w / 2 - c.x, best.y + h / 2 - c.y) > c.r + Math.max(w, h) / 2 + 14;
    obst.push(best);
  });
}

/* ---------- drawing ---------- */
const rounded = (P) => {
  if (P.length < 3) return 'M' + P.map((p) => p.x + ',' + p.y).join(' L');
  let d = 'M' + P[0].x + ',' + P[0].y;
  for (let i = 1; i < P.length - 1; i++) {
    const a = P[i - 1], b = P[i], c = P[i + 1];
    const r = Math.min(R, Math.hypot(b.x - a.x, b.y - a.y) / 2, Math.hypot(c.x - b.x, c.y - b.y) / 2);
    const ux = Math.sign(b.x - a.x), uy = Math.sign(b.y - a.y), vx = Math.sign(c.x - b.x), vy = Math.sign(c.y - b.y);
    d += ' L' + (b.x - ux * r) + ',' + (b.y - uy * r) + ' Q' + b.x + ',' + b.y + ' ' + (b.x + vx * r) + ',' + (b.y + vy * r);
  }
  return d + ' L' + P[P.length - 1].x + ',' + P[P.length - 1].y;
};
const LC = (layer) => AM.LCOLOR[layer] || 'var(--oth)';

function shDraw(m, lay, svg) {
  const gCols = S('g', { class: 'cols' }, svg), gF = S('g', { class: 'frs' }, svg), gE = S('g', { class: 'es' }, svg);
  const gN = S('g', { class: 'ns' }, svg), gL = S('g', { class: 'labs' }, svg), gX = S('g', { class: 'fx' }, svg);
  m.cols.forEach((c, i) => {
    S('rect', { class: 'band', x: lay.colX[i] - 10, y: 30, width: lay.colW[i] + 20, height: lay.H - 36, rx: 12 }, gCols);
    S('text', { class: 'colh', x: lay.colX[i] + lay.colW[i] / 2, y: 22, 'text-anchor': 'middle' }, gCols, AM.t(SH_COLT[c]).toUpperCase());
  });
  const els = { nodes: new Map(), edges: [], frames: new Map() };
  m.frames.forEach((f) => {
    const g = S('g', { class: 'fr' + (f.plan ? ' plan' : '') + (f.ghost ? ' ghost' : ''), 'data-k': f.id, style: '--lc:' + LC(f.layer), tabindex: 0, role: 'button' }, gF);
    S('rect', { x: f.x, y: f.y, width: f.w, height: f.h, rx: 10 }, g);
    S('text', { x: f.x + 10, y: f.y + 15 }, g, fitTail(f.label, f.w - 18 - (f.plan ? tw(' · ' + AM.t('shNoCode'), 11, false, true) : 0), 11, true) + (f.plan ? ' · ' + AM.t('shNoCode') : ''));
    S('title', null, g, f.mod);
    g.addEventListener('click', () => AM.select({ t: 'mod', id: AM.map.unitOf(f.mod) || f.mod }));
    els.frames.set(f.id, g);
  });
  m.edges.forEach((e) => {
    const cls = 'e k-' + e.kind + (e.taken ? ' taken' : '') + (e.choice ? ' choice' : '') + (e.guided ? ' guided' : '') + (e.seen ? ' seen' : '') + (e.B.ghost ? ' ghost' : '');
    const g = S('g', { class: cls, 'data-k': 'e:' + e.id }, gE);
    const d = rounded(e.P);
    const line = S('path', { class: 'l', d }, g);
    S('path', { class: 'hit', d }, g);
    const title = [e.nt, (e.tags || []).join(' · ')].filter(Boolean).join(' — ') || (e.A.label + ' → ' + e.B.label);
    S('title', null, g, title);
    const it = { e, g, line, num: null, tag: null };
    if (e.tb) {
      const tg = S('g', { class: 'tag' + (e.kind === 'branch' ? ' cond' : ''), 'data-k': 't:' + e.id }, gL);
      if (e.lead) S('path', { class: 'lead', d: `M${e.tc.x},${e.tc.y} L${e.tb.x + e.tb.w / 2},${e.tb.y + e.tb.h / 2}` }, tg);
      S('rect', { x: e.tb.x, y: e.tb.y, width: e.tb.w, height: e.tb.h, rx: 4 }, tg);
      e.tl.forEach((s, k) => S('text', { x: e.tb.x + 5, y: e.tb.y + 13 + k * 14 }, tg, s));
      S('title', null, tg, (e.tags || []).join('\n'));
      it.tag = tg;
    }
    if (e.nb) {
      const ng = S('g', { class: 'num', 'data-k': 'n:' + e.id, tabindex: 0, role: 'button' }, gL);
      S('rect', { x: e.nb.x, y: e.nb.y, width: e.nb.w, height: e.nb.h, rx: 9 }, ng);
      S('text', { x: e.nb.x + e.nb.w / 2, y: e.nb.y + 12.5, 'text-anchor': 'middle' }, ng, e.nt);
      S('title', null, ng, title);
      it.num = ng;
    }
    const act = (ev) => { if (ev) ev.stopPropagation(); shEdgeAct(e); };
    [g, it.num, it.tag].forEach((x) => { if (x) x.addEventListener('click', act); });
    if (it.num) it.num.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') act(ev); });
    if (e.kind === 'opt') [g, it.num].forEach((x) => { if (x) x.addEventListener('mouseenter', () => AM.walk.select(e.optK)); });
    els.edges.push(it);
  });
  m.nodes.forEach((nd) => {
    const cls = 'n k-' + nd.kind + (nd.ghost ? ' ghost' : '') + (nd.plan ? ' plan' : '') + (nd.tone ? ' ' + nd.tone : '') + (nd.taken ? ' taken' : '');
    const g = S('g', { class: cls, 'data-k': nd.id, style: nd.layer ? '--lc:' + LC(nd.layer) : null, tabindex: 0, role: 'button' }, gN);
    const pill = nd.kind === 'actor' || nd.kind === 'out' || nd.kind === 'flowref';
    S('rect', { class: 'b', x: nd.x, y: nd.y, width: nd.w, height: nd.h, rx: pill ? Math.min(17, nd.h / 2) : nd.kind === 'svc' ? 12 : 7 }, g);
    if (nd.kind === 'fn' || nd.kind === 'mod' || nd.kind === 'plan') S('rect', { class: 'strip', x: nd.x + 5, y: nd.y + 6, width: 3, height: Math.max(8, nd.h - 12), rx: 1.5 }, g);
    const cy = nd.y + nd.h / 2;
    const pre = nd.kind === 'plan' ? '⌁ ' : nd.kind === 'out' ? (nd.tone === 'bad' ? '✕ ' : '✓ ') : nd.kind === 'flowref' ? '↪ ' : '';
    const mono = nd.kind === 'fn';
    const tx = nd.kind === 'fn' || nd.kind === 'mod' || nd.kind === 'plan' ? nd.x + 14 : nd.x + 12;
    const room = nd.x + nd.w - tx - (nd.inner.length ? 34 : 8);
    if (nd.sub || nd.kind === 'mod' || nd.kind === 'svc') {
      const sub = nd.kind === 'mod' ? (AM.t('layers') || {})[nd.layer] || '' : nd.kind === 'svc' ? AM.t('seqSvc') : nd.sub;
      S('text', { class: 'sub', x: tx, y: cy - 4 }, g, fit(sub, room, 10, false));
      S('text', { class: 'lb', x: tx, y: cy + 10 }, g, nd.kind === 'mod' ? fitTail(nd.label, room, 12, true) : fit(pre + nd.label, room, 12, mono, true));
    } else S('text', { class: 'lb' + (mono ? ' mono' : ''), x: tx, y: cy + 4 }, g, fit(pre + nd.label, room, mono ? 11.5 : 12, mono, nd.kind === 'actor'));
    if (nd.inner.length) {
      const t = numText(nd.inner), w = tw(t, 10, false, true) + 10;
      const bg = S('g', { class: 'inner' }, g);
      S('rect', { x: nd.x + nd.w - w - 5, y: nd.y + 5, width: w, height: 15, rx: 7.5 }, bg);
      S('text', { x: nd.x + nd.w - w / 2 - 5, y: nd.y + 15.5, 'text-anchor': 'middle' }, bg, t);
    }
    const tip = [nd.kind === 'fn' ? nd.fid : nd.kind === 'mod' ? nd.mod : nd.label];
    if (nd.kind === 'fn' || nd.kind === 'plan') {
      const caps = new Set(); nd.steps.forEach((i) => { const s = m.steps[i]; if (s && s.label && s.label !== AM.fnInfo(s.fid).name) caps.add((i + 1) + '. ' + s.label); });
      caps.forEach((c) => tip.push(c));
    }
    if (nd.inner.length) tip.push(AM.t('shInner') + ': ' + numText(nd.inner));
    if (nd.tags.length) tip.push(nd.tags.join(' · '));
    S('title', null, g, tip.join('\n'));
    const act = () => shNodeAct(nd);
    g.addEventListener('click', (ev) => { ev.stopPropagation(); act(); });
    g.addEventListener('dblclick', (ev) => { ev.stopPropagation(); if (nd.steps.length) AM.play.go(nd.steps[0]); });
    g.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') { ev.stopPropagation(); act(); } });
    els.nodes.set(nd.id, { nd, g });
  });
  els.fx = gX;
  return els;
}
function shEdgeAct(e) {
  if (e.kind === 'opt') { AM.walk.take(e.opt); return; }
  if (e.kind === 'branch') {
    const can = AM.play.branches() || [];
    if (can.includes(e.br)) { AM.play.choose(e.br); return; }
    if (e.at != null) AM.play.go(e.at);
    return;
  }
  if (e.steps.length) {
    const cur = AM.state.step, s = e.steps.slice().sort((a, b) => a - b);
    // a second click on an arrow with several numbers goes to its next one
    const nx = s.find((i) => i > cur && s.includes(cur)) ?? s[0];
    AM.play.go(nx);
  }
}
function shNodeAct(nd) {
  if (nd.kind === 'fn') AM.select({ t: 'fn', id: nd.fid });
  else if (nd.kind === 'mod') AM.select({ t: 'mod', id: AM.map.unitOf(nd.mod) || nd.mod });
  else if (nd.kind === 'svc') AM.select({ t: 'svc' });
  else if ((nd.kind === 'ext' || nd.kind === 'queue') && nd.ext) AM.select({ t: 'ext', id: nd.ext.id });
  else if (nd.kind === 'actor' && nd.actor) AM.select({ t: 'actor', id: nd.actor.id });
  else if (nd.kind === 'plan' && nd.steps.length) AM.play.go(nd.steps[0]);
  else if ((nd.kind === 'out' || nd.kind === 'flowref') && nd.b) {
    const can = AM.play.branches() || [];
    if (can.includes(nd.b)) AM.play.choose(nd.b);
    else if (nd.kind === 'flowref') AM.play.load('flow:' + nd.flow.i);
  }
}

/* ---------- the strip above the sheet: flows of a path, the outcome, the walk hint ---------- */
function shStrip(m) {
  const st = SH.strip; st.innerHTML = '';
  const run = m.run, parts = [];
  if (run.segs.length > 1) {
    run.segs.forEach((sg, si) => {
      const a = m.segFirst[si], b = a + sg.steps.length - 1;
      if (si) parts.push(`<span class="sep" aria-hidden="true">▸</span>`);
      parts.push(`<button class="seg" data-si="${si}" data-i="${a}" title="${AM.esc(sg.flow ? sg.flow.name : '')}"><b>${a + 1}${b > a ? '–' + (b + 1) : ''}</b> ${AM.esc(AM.cut(sg.flow ? sg.flow.name : run.name, 46))}</button>`);
    });
  }
  const ex = run.fixed ? (AM.jv.end() || {}).text : AM.state.exitText || (run.segs[run.segs.length - 1].flow || {}).exit;
  if (ex) parts.push(`<span class="ex" title="${AM.esc(ex)}"><i>${AM.esc(AM.t('shExit'))}:</i> ${AM.esc(AM.cut(String(ex).replace(/^[^:]{0,20}:\s*/, (s) => (run.fixed ? '' : s)), 110))}</span>`);
  if (AM.walk && AM.walk.on) parts.push(`<span class="hint">${AM.esc(AM.t('shWalk'))}</span>`);
  if (m.total > m.n) parts.push(`<span class="hint">${AM.esc(AM.t('shMore', m.n, m.total))}</span>`);
  st.hidden = !parts.length;
  st.innerHTML = parts.join('');
  st.querySelectorAll('button.seg').forEach((b) => { b.onclick = () => AM.play.go(+b.dataset.i); });
}

/* ---------- render: model, layout, fit, FLIP from the previous sheet ---------- */
function shRender(host, ctx) {
  const lv = Math.min(2, ctx.level);
  const run = AM.state.run;
  if (AM.walk && AM.walk.on && AM.walk.state.busy) return; // the walk redraws after its step (bus "walk")
  const key = (run ? run.key : '') + '|' + (run ? run.segs.map((s) => s.flow ? s.flow.id : '').join(',') : '');
  const sameRun = key === SH.key && SH.svg && SH.host === host && host.contains(SH.svg);
  const walk = !!(AM.walk && AM.walk.on);
  const anim = sameRun && !AM.RM && (lv !== SH.lv || walk);
  // the old picture: screen boxes of its nodes and frames for the morph
  const old = new Map();
  let oldSvg = null;
  if (anim) {
    SH.svg.querySelectorAll('[data-k]').forEach((el) => { const k = el.getAttribute('data-k'); if (!/^[etn]:/.test(k)) old.set(k, el.getBoundingClientRect()); });
    if (lv !== SH.lv) oldSvg = SH.svg;
  }
  const sl = host.scrollLeft, stp = host.scrollTop;
  shStop();
  if (oldSvg) {
    const hr = host.getBoundingClientRect(), r = oldSvg.getBoundingClientRect();
    oldSvg.classList.add('sh-old');
    oldSvg.style.left = (r.left - hr.left + host.scrollLeft) + 'px'; oldSvg.style.top = (r.top - hr.top + host.scrollTop) + 'px';
    host.appendChild(oldSvg);
  }
  Array.from(host.children).forEach((c) => { if (c !== oldSvg) c.remove(); });
  host.classList.add('sh-host');
  SH.host = host; SH.key = key; SH.lv = lv; SH.ctx = ctx;
  SH.strip = AM.h('div', { class: 'sh-strip', hidden: true }, host);
  SH.box = AM.h('div', { class: 'sh-box' }, host);
  const m = shModel(lv);
  SH.m = m;
  if (!m) { SH.svg = null; AM.h('div', { class: 'seq-empty' }, SH.box, AM.esc(AM.t('shEmpty'))); if (oldSvg) oldSvg.remove(); return; }
  const lay = shLayout(m, walk);
  shLabels(m, lay);
  SH.lay = lay;
  shStrip(m);
  const cw = Math.max(200, host.clientWidth - 2), ch = Math.max(120, host.clientHeight - (SH.strip.hidden ? 0 : SH.strip.offsetHeight) - 2);
  let z = Math.max(SH_MINZ, Math.min(SH_MAXZ, cw / lay.W, ch / lay.H));
  if (walk && sameRun && SH.z0) z = Math.min(z, SH.z0); // a growing walk never zooms in: no jumps
  SH.z0 = z;
  if (!sameRun) SH.uz = 1;
  SH.z = z * SH.uz;
  const svg = S('svg', { class: 'sh', viewBox: `0 0 ${lay.W} ${lay.H}`, width: lay.W * SH.z, height: lay.H * SH.z, role: 'img', 'aria-label': run.name }, SH.box);
  SH.svg = svg;
  SH.els = shDraw(m, lay, svg);
  SH.lastStep = -2;
  if (sameRun) { host.scrollLeft = sl; host.scrollTop = stp; } else { host.scrollLeft = 0; host.scrollTop = 0; }
  if (anim) shFlip(old, svg, lv);
  if (oldSvg) {
    const a = oldSvg.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 260 * (AM.DUR / 420 || 1), easing: 'ease-out' });
    a.onfinish = () => oldSvg.remove(); setTimeout(() => oldSvg.remove(), 2000);
  }
  shSync(true);
  if (walk) shReveal(true);
}
// FLIP: every node and frame starts where it (or what it folded into / unfolded from) was on the old sheet
function shFlip(old, svg, lv) {
  const D = AM.D, sr = svg.getBoundingClientRect(), z = SH.z;
  const toUser = (r) => ({ x: (r.left - sr.left) / z, y: (r.top - sr.top) / z, w: r.width / z, h: r.height / z });
  const alias = (k) => {
    const f = k.startsWith('f:') ? D.fns[k.slice(2)] : null;
    if (f) return ['F:' + f.module, 'm:' + f.module, 'svc'];
    if (k.startsWith('F:')) return ['m:' + k.slice(2), 'svc'];
    if (k.startsWith('m:')) return ['F:' + k.slice(2), 'svc'];
    if (k === 'svc') return ['*mods'];
    return [];
  };
  const union = (pred) => { let a = null; old.forEach((r, k) => { if (!pred(k)) return; a = a ? { left: Math.min(a.left, r.left), top: Math.min(a.top, r.top), right: Math.max(a.right, r.right), bottom: Math.max(a.bottom, r.bottom) } : { left: r.left, top: r.top, right: r.right, bottom: r.bottom }; }); return a && { left: a.left, top: a.top, width: a.right - a.left, height: a.bottom - a.top }; };
  const dur = Math.max(1, AM.DUR) * 1.05, ease = 'cubic-bezier(.2,.7,.2,1)';
  svg.querySelectorAll('[data-k]').forEach((el) => {
    const k = el.getAttribute('data-k');
    if (/^[etn]:/.test(k)) { el.animate([{ opacity: 0 }, { opacity: 0, offset: 0.45 }, { opacity: 1 }], { duration: dur, easing: 'ease-out' }); return; }
    let r = old.get(k);
    if (!r) for (const a of alias(k)) { r = a === '*mods' ? union((x) => /^[mF]:/.test(x)) : old.get(a); if (r) break; }
    if (!r) { el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: dur, easing: 'ease-out' }); return; }
    const N = el.getBBox(), O = toUser(r);
    if (!N.width || !N.height) return;
    const sx = O.w / N.width, sy = O.h / N.height, tx = O.x - sx * N.x, ty = O.y - sy * N.y;
    if (Math.abs(tx) < 0.5 && Math.abs(ty) < 0.5 && Math.abs(sx - 1) < 0.01 && Math.abs(sy - 1) < 0.01) return;
    el.style.transformOrigin = '0 0';
    el.animate([{ transform: `translate(${tx}px, ${ty}px) scale(${sx}, ${sy})` }, { transform: 'none' }], { duration: dur, easing: ease });
  });
}
function shClear() { shStop(); SH.svg = null; SH.m = null; SH.key = ''; SH.lv = -1; if (SH.host) SH.host.classList.remove('sh-host'); }

/* ---------- playback: lit arrow and node, the running dot, past saturated, future dimmed ---------- */
function shSync(keep) {
  if (!SH.m || !SH.els || !SH.svg) return;
  const cur = AM.state.step, m = SH.m, on = cur >= 0;
  const can = AM.play.branches() || [];
  const curE = [];
  SH.els.edges.forEach((it) => {
    const e = it.e, st = e.steps;
    const has = on && st.includes(cur);
    const past = on && !has && st.some((i) => i < cur);
    const fut = on && !has && !past && (st.length > 0 || e.kind === 'branch');
    [it.g, it.num, it.tag].forEach((x) => { if (!x) return; x.classList.toggle('cur', has); x.classList.toggle('past', past); x.classList.toggle('future', fut); });
    const ch = e.kind === 'branch' && can.includes(e.br);
    [it.g, it.num].forEach((x) => { if (x) x.classList.toggle('choice', ch); });
    if (e.kind === 'opt') [it.g, it.num, it.tag].forEach((x) => { if (x) x.classList.toggle('sel', e.optK === AM.walk.sel); });
    if (has) curE.push(it);
  });
  const touch = new Map();
  m.edges.forEach((e) => { if (e.kind === 'opt' || e.kind === 'branch') return; [e.a, e.b].forEach((id) => { const nd = m.nodes.get(id); if (nd.kind === 'fn' || nd.kind === 'plan' || nd.kind === 'mod' || nd.kind === 'svc') return; if (!touch.has(id)) touch.set(id, []); touch.get(id).push(...e.steps); }); });
  SH.els.nodes.forEach(({ nd, g }) => {
    // a function / module / service is current when the step lands in it; actors and external systems when an arrow
    // of the step touches them
    const own = nd.steps.concat(nd.inner).concat(touch.get(nd.id) || []);
    const isCur = on && own.includes(cur);
    const past = on && !isCur && own.some((i) => i < cur);
    g.classList.toggle('cur', isCur);
    g.classList.toggle('past', past);
    g.classList.toggle('future', on && !isCur && !past && nd.kind !== 'actor');
    g.classList.toggle('choice', !!nd.b && can.includes(nd.b));
  });
  if (AM.state.sel) shSelect(AM.state.sel);
  if (SH.strip) SH.strip.querySelectorAll('.seg').forEach((b) => b.classList.toggle('cur', on && !!m.steps[cur] && m.steps[cur].seg === +b.dataset.si));
  if (cur !== SH.lastStep) {
    SH.lastStep = cur;
    if (!keep && on) shRun(curE);
    if (on) shReveal(false);
  }
}
function shStop() { if (SH.dotRaf) cancelAnimationFrame(SH.dotRaf); SH.dotRaf = 0; if (SH.els && SH.els.fx) SH.els.fx.innerHTML = ''; }
// the dot runs along the arrows of the step: the call (or enqueue, then dequeue), then the calls of external systems
function shRun(list) {
  shStop();
  if (AM.RM || !list.length) return;
  const order = { entry: 0, call: 0, branch: 0, plan: 0, self: 0, enq: 0, deq: 1, ext: 2 };
  const seq = list.slice().sort((a, b) => (order[a.e.kind] || 0) - (order[b.e.kind] || 0)).map((it) => it.line);
  const dot = S('circle', { class: 'dot', r: 5 }, SH.els.fx);
  const per = 520 / (AM.state.speed || 1);
  let k = 0, t0 = performance.now();
  const tick = (now) => {
    const p = seq[k]; if (!p || !p.isConnected) { dot.remove(); SH.dotRaf = 0; return; }
    const L = p.getTotalLength(), f = Math.min(1, (now - t0) / per), pt = p.getPointAtLength(L * (1 - Math.pow(1 - f, 2)));
    dot.setAttribute('cx', pt.x); dot.setAttribute('cy', pt.y);
    if (f >= 1) { k++; t0 = now; if (k >= seq.length) { setTimeout(() => dot.remove(), 160); SH.dotRaf = 0; return; } }
    SH.dotRaf = requestAnimationFrame(tick);
  };
  SH.dotRaf = requestAnimationFrame(tick);
}
// keep the current arrow (or the walk's fan) in view: scroll only when it is out of sight, by the least amount
function shReveal(fan) {
  const host = SH.host; if (!host || !SH.svg) return;
  const sel = fan ? '.e.k-opt, .n.cur' : '.e.cur, .n.cur';
  const els = Array.from(SH.svg.querySelectorAll(sel)); if (!els.length) return;
  const hr = host.getBoundingClientRect(), top = hr.top + (SH.strip && !SH.strip.hidden ? SH.strip.offsetHeight : 0);
  let l = Infinity, t = Infinity, r = -Infinity, b = -Infinity;
  els.forEach((e) => { const x = e.getBoundingClientRect(); l = Math.min(l, x.left); t = Math.min(t, x.top); r = Math.max(r, x.right); b = Math.max(b, x.bottom); });
  let dx = 0, dy = 0;
  if (r - l < hr.width - 40) { if (l < hr.left + 20) dx = l - hr.left - 20; else if (r > hr.right - 20) dx = r - hr.right + 20; } else if (l < hr.left || l > hr.right) dx = l - hr.left - 20;
  if (b - t < hr.bottom - top - 40) { if (t < top + 20) dy = t - top - 20; else if (b > hr.bottom - 20) dy = b - hr.bottom + 20; } else if (t < top || t > hr.bottom) dy = t - top - 20;
  if (dx || dy) host.scrollBy({ left: dx, top: dy, behavior: AM.RM ? 'auto' : 'smooth' });
}
function shSelect(sel) {
  if (!SH.els) return;
  SH.els.nodes.forEach(({ nd, g }) => g.classList.toggle('sel', !!sel && ((sel.t === 'fn' && nd.fid === sel.id) || (sel.t === 'mod' && nd.mod && (nd.mod === sel.id || AM.map.unitOf(nd.mod) === sel.id)) || (sel.t === 'ext' && nd.ext && nd.ext.id === sel.id))));
}

/* ---------- gestures: pinch / Ctrl+wheel scales and changes the level, drag pans ---------- */
function shScale(uz, e) {
  if (!SH.svg || !SH.lay) return;
  const host = SH.host, old = SH.z;
  SH.uz = uz; SH.z = SH.z0 * uz;
  const r = host.getBoundingClientRect(), px = e ? e.clientX - r.left : host.clientWidth / 2, py = e ? e.clientY - r.top : host.clientHeight / 2;
  const wx = (host.scrollLeft + px) / old, wy = (host.scrollTop + py) / old;
  SH.svg.setAttribute('width', SH.lay.W * SH.z); SH.svg.setAttribute('height', SH.lay.H * SH.z);
  host.scrollLeft = wx * SH.z - px; host.scrollTop = wy * SH.z - py;
}
function shWheel(e, level, setLevel) {
  if (!AM.zoom.isZoomWheel(e)) return; // a plain wheel / two-finger scroll scrolls the sheet natively
  const lv = Math.min(2, level);
  AM.zoom.scaleWheel(e, {
    z: SH.uz, set: (z) => shScale(z, e),
    in: lv < 2 ? () => setLevel(lv + 1) : null,
    out: lv > 0 ? () => setLevel(lv - 1) : null,
  });
}
function shPan(host) {
  if (host.__shPan) return;
  host.__shPan = true;
  host.addEventListener('pointerdown', (e) => {
    if (!host.classList.contains('sh-host') || e.button !== 0 || e.target.closest('.n, .e, .num, .tag, .fr, .sh-strip, button')) return;
    const x0 = e.clientX, y0 = e.clientY, sl = host.scrollLeft, st = host.scrollTop;
    let moved = false;
    const mv = (ev) => {
      if (!moved && Math.hypot(ev.clientX - x0, ev.clientY - y0) < 4) return;
      if (!moved) { moved = true; host.classList.add('panning'); try { host.setPointerCapture(e.pointerId); } catch (er) { /* synthetic */ } }
      host.scrollLeft = sl - (ev.clientX - x0); host.scrollTop = st - (ev.clientY - y0);
    };
    const up = () => { host.removeEventListener('pointermove', mv); host.removeEventListener('pointerup', up); host.removeEventListener('pointercancel', up); host.classList.remove('panning'); };
    host.addEventListener('pointermove', mv); host.addEventListener('pointerup', up); host.addEventListener('pointercancel', up);
  });
}

AM.sheet = {
  // render(host, {level, setLevel}): draw the current run into host (the dock's scroll box)
  render(host, ctx) { shPan(host); shRender(host, ctx); },
  sync: shSync, select: shSelect, wheel: shWheel, clear: shClear,
  get model() { return SH.m; }, get layout() { return SH.lay; }, get scale() { return SH.z; },
};
