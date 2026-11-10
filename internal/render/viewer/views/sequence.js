/* views/sequence.js — the sequence diagram dock of the map: a nested sequence diagram of the current scenario.
 * Not a tab: a panel over the map view that appears when a scenario, path or variant is chosen or plays, at the
 * bottom (default, 40%) or on the right, with a draggable separator, a collapse-to-a-strip button and a full-screen
 * mode that hides the map (Esc leaves it); the map's cameras fit into the free part (AM.map.setDock).
 * Four levels, like the map (and kept on the map's level while "link levels" is on):
 *   0 system    — the actor, the service as one participant, the queue, external systems; only the hand-offs
 *                 between them, the inner steps folded into one message labelled with its first step;
 *   1 modules   — modules by layer, the queue, externals; one message per change of module;
 *   2 functions — functions (of the whole scenario, or of one module with the rest folded to modules); one per call;
 *   3 code      — as functions, each call with the line of the caller that makes it.
 * Each message is built from the steps of the flow and the call graph between them: the caller of a step is the
 * previous step or a frame below it on the call stack that reaches it (callees / async hand-offs). "Enter" on a
 * message opens it in a frame one level deeper (at the function level: the chain of calls from the caller to the
 * step, up to SEQ_HOPS hops, with the call lines), frames nest; "Leave" folds it. Ctrl+wheel or a pinch over the
 * diagram scales it and, past the thresholds of the map's semantic zoom (zoom.js), changes the level keeping the
 * current step in view. Replies, async hand-offs with a pause, alt blocks of the branches, effect and status
 * badges and the exit are drawn on every level. Follows the player: the messages holding the current step are lit
 * and scrolled to, a click plays up to the message's first step. */
AM.addI18n({
  ru: {
    seqPage: 'Последовательность', seqHint: 'Выберите сценарий в списке слева — здесь будет его диаграмма последовательности',
    seqTitle: 'Сценарий', seqEntry: 'Вход', seqReply: 'ответ', seqPause: 'пауза: задача ждёт в очереди', seqEnq: 'в очередь',
    seqExpand: 'развернуть', seqCollapse: 'свернуть', seqTake: '▶ пройти ветку', seqPick: '▶ выбрать исход',
    seqMapCap: 'Карта модулей', seqEmpty: 'Нет сценария для показа', seqExternal: 'внешняя', seqFlowTo: 'сценарий',
    seqSteps: (n, p) => n + ' ' + p(['шаг', 'шага', 'шагов']),
    seqLegend: 'вызов', seqLegendReply: 'ответ', seqLegendAsync: 'асинхронно через очередь', seqNoMap: 'карта модулей недоступна',
    seqSvc: 'сервис', seqLink: 'с картой', seqLinkTip: 'Уровень сценария и уровень карты меняются вместе',
    seqEnter: '⤵ войти', seqLeave: '⤴ выйти', seqEnterTip: 'Раскрыть сообщение в рамку под-сценария (двойной клик, +)',
    seqFoldAll: 'свернуть рамки', seqScopeAll: 'функции: весь сценарий', seqScopeMod: (m) => 'функции модуля ' + m,
    seqScopeTip: 'Функции всего сценария или только одного модуля (остальные — модулями)',
    seqZoomHint: 'щипок / ⌘/Ctrl+колесо — масштаб и уровень', seqNoSrc: 'исходник недоступен', seqCallLine: 'строка вызова',
    dockBottom: 'Панель снизу', dockRight: 'Панель справа', dockMin: 'Свернуть в полоску', dockRestore: 'Развернуть диаграмму',
    dockFull: 'На весь экран (карта скрыта)', dockUnfull: 'Вернуть карту (Esc)', dockClose: 'Закрыть диаграмму',
    dockGrip: 'Потяните, чтобы изменить размер; двойной щелчок — по умолчанию', dockToggle: 'Сценарий: лист и последовательность',
  },
  en: {
    seqPage: 'Sequence', seqHint: 'Pick a scenario in the list at the left — its sequence diagram appears here',
    seqTitle: 'Scenario', seqEntry: 'Entry', seqReply: 'reply', seqPause: 'pause: the job waits in the queue', seqEnq: 'enqueue',
    seqExpand: 'expand', seqCollapse: 'collapse', seqTake: '▶ take this branch', seqPick: '▶ pick this outcome',
    seqMapCap: 'Module map', seqEmpty: 'No scenario to show', seqExternal: 'external', seqFlowTo: 'scenario',
    seqSteps: (n, p) => n + ' ' + p(['step', 'steps']),
    seqLegend: 'call', seqLegendReply: 'reply', seqLegendAsync: 'async via the queue', seqNoMap: 'module map unavailable',
    seqSvc: 'service', seqLink: 'with map', seqLinkTip: 'The scenario and the map change their level together',
    seqEnter: '⤵ enter', seqLeave: '⤴ leave', seqEnterTip: 'Open the message in a frame of its sub-scenario (double-click, +)',
    seqFoldAll: 'fold frames', seqScopeAll: 'functions: whole scenario', seqScopeMod: (m) => 'functions of ' + m,
    seqScopeTip: 'Functions of the whole scenario or of one module only (the rest as modules)',
    seqZoomHint: 'pinch / ⌘/Ctrl+wheel — zoom and level', seqNoSrc: 'source unavailable', seqCallLine: 'call line',
    dockBottom: 'Panel at the bottom', dockRight: 'Panel on the right', dockMin: 'Collapse to a strip', dockRestore: 'Expand the diagram',
    dockFull: 'Full screen (hides the map)', dockUnfull: 'Back to the map (Esc)', dockClose: 'Close the diagram',
    dockGrip: 'Drag to resize; double-click for the default', dockToggle: 'Scenario: sheet and sequence',
  },
});

const SEQ_LD = { entry: 0, ui: 1, domain: 2, other: 2, adapter: 3, infra: 4, test: 4 };
const SEQ_MAXREPLY = 150; // replies need the call stack of every step (quadratic): skipped in very long runs
const SEQ_HOPS = 8;       // the longest call chain shown inside a frame
const SEQ_TONE = { good: 'var(--good)', warning: 'var(--warning)', critical: 'var(--critical)', serious: 'var(--serious)', neutral: 'var(--muted)' };
const seqBad = /(fail|error|cancel|reject|denied|block|expire|timeout|ошиб|отмен|отклон|истек|истёк|отказ|не оплач)/i;

let sWrap = null, sBar = null, sSig = '', sDirty = true, sBox = '';
let sItems = [], sAct = null, sDim = null;
let sLevel = 1, sScope = null, sZ = 1, sLink = true;
const sOpen = new Set(); // expanded branch previews
const sSub = new Set();  // messages opened into frames (keys)
const sReq = new Set();  // function bodies already requested for the call lines
sLink = false; // the diagram is a page of its own: its level is not tied to the map's (the checkbox is hidden)
// the dock shows the scenario sheet (views/sheet.js, the default) or the sequence diagram; the choice is remembered
let sMode = 'sheet';
try { if (localStorage.getItem('archmap.seqmode') === 'seq') sMode = 'seq'; } catch (e) { /* file:// without storage */ }
const isSheet = () => sMode === 'sheet' && !!AM.sheet;
function seqMode(mode) {
  if (mode === sMode) return;
  sMode = mode === 'seq' ? 'seq' : 'sheet';
  try { localStorage.setItem('archmap.seqmode', sMode); } catch (e) { /* ignore */ }
  if (!isSheet() && AM.sheet) AM.sheet.clear();
  if (sShown()) { seqRender(); seqSync(false); } else sDirty = true;
}

const D_ = () => AM.D;
const txt = (s, n) => AM.cut(s, n);

/* ---------- model: rows and participants ---------- */
function seqModel() {
  const D = D_(), P = AM.play, run = AM.state.run, lvl = sLevel;
  if (!run) return null;
  const n = P.list().length;
  if (!n) return null;
  const rows = [], parts = new Map(), modFirst = {};
  const addPart = (key, o) => {
    if (!parts.has(key)) {
      parts.set(key, Object.assign({ key, n: parts.size }, o));
      if (o.mod != null && modFirst[o.mod] == null) modFirst[o.mod] = parts.size;
    }
    return key;
  };
  const seg0 = run.segs[0], fl0 = seg0 && seg0.flow;
  const s0 = P.stepAt(0);
  addPart('actor', { kind: 'actor', name: (fl0 && fl0.actor) || AM.t('seqEntry'), actor: s0 && s0.actor });
  const PJ = D.F.project || {};
  const svcPart = () => addPart('svc', { kind: 'svc', name: PJ.name || D.M.title || D.F.repo || AM.t('seqSvc') });
  const modPart = (mod) => addPart('m:' + mod, { kind: 'mod', mod, layer: AM.layerOf(mod), name: AM.short(mod) });
  const fnPart = (fid) => { const m = D.fns[fid].module; return addPart('f:' + fid, { kind: 'fn', fid, mod: m, layer: AM.layerOf(m), name: AM.fnInfo(fid).name }); };
  const partAt = (l, fid) => (l === 0 ? svcPart() : l === 1 ? modPart(D.fns[fid].module) : sScope && D.fns[fid].module !== sScope ? modPart(D.fns[fid].module) : fnPart(fid));
  const queueExt = D.externals.find((x) => x.kind === 'queue');
  const queuePart = () => addPart('queue', { kind: 'queue', name: queueExt ? queueExt.name : AM.t('queue'), ext: queueExt });
  const extPart = (x) => addPart('e:' + x.id, { kind: 'ext', ext: x, name: x.name });

  // call stacks of the real steps (for replies and callers)
  const fr = [];
  if (n <= SEQ_MAXREPLY) for (let i = 0; i < n; i++) fr.push(P.frames(i));
  const depthOf = (i) => (fr[i] ? fr[i].length : 0);
  // caller(s): the function that makes the step's call — the previous step, or a frame below it on the stack
  const callers = new Map();
  const caller = (s) => {
    if (!s.prev) return null;
    if (s.planned) return s.prev;
    const ck = s.i >= 0 ? s.i : 'p' + s.prev + '>' + s.fid;
    if (callers.has(ck)) return callers.get(ck);
    let c = s.prev;
    if (s.kind !== 'async' && !AM.isCall(s.prev, s.fid)) {
      const stack = s.i > 0 && fr[s.i - 1] ? fr[s.i - 1].slice().reverse() : [];
      const cands = [s.prev].concat(stack.filter((f) => f !== s.prev && D.fns[f]));
      c = cands.find((f) => AM.isCall(f, s.fid) || AM.isAsync(f, s.fid)) || cands.find((f) => AM.callPath(f, s.fid, SEQ_HOPS)) || s.prev;
    }
    callers.set(ck, c);
    return c;
  };

  let open = []; // calls waiting for their reply: {from, to, depth, i}
  const closeReplies = (d) => {
    while (open.length && open[open.length - 1].depth >= d) {
      const o = open.pop();
      rows.push({ k: 'msg', dir: 'reply', from: o.to, to: o.from, text: '', steps: [o.i], i: o.i, fid: null, main: false });
    }
  };
  const stepOf = (fid, prev, fl) => ({
    i: -1, fid, prev, mod: D.fns[fid].module,
    kind: !prev ? 'entry' : AM.isAsync(prev, fid) ? 'async' : AM.isCall(prev, fid) ? 'call' : 'next',
    label: (fl && fl.labels && fl.labels[fid]) || AM.fnInfo(fid).name,
    effects: (fl && fl.effects && fl.effects[fid]) || [], data: D.fns[fid].data || [], states: (D.transByFid[fid] || []).slice(0, 2),
  });
  const extsOf = (s) => (D.extByFn[s.fid] || []).filter((x) => x.kind !== 'queue').slice(0, 2);
  const badgesOf = (ss) => {
    const eff = [], st = [], se = new Set(), ss2 = new Set();
    ss.forEach((s) => {
      AM.ui.effBadges(s).forEach((b) => { if (!se.has(b.text)) { se.add(b.text); eff.push(b); } });
      (s.states || []).forEach((x) => { const k = x.entity + ':' + x.to; if (!ss2.has(k)) { ss2.add(k); st.push(x); } });
    });
    return { eff: eff.slice(0, 2), st: st.slice(0, 2) };
  };
  const idxOf = (ss) => ss.map((s) => s.i).filter((i) => i >= 0);
  // calls of external systems: lit as the step's own message (on the system level it is the step's hand-off,
  // labelled with the step)
  const extRows = (s, from, l) => extsOf(s).forEach((x) => {
    const ek = extPart(x), st = idxOf([s]);
    rows.push({ k: 'msg', dir: 'call', from, to: ek, text: l === 0 && s.label !== AM.fnInfo(s.fid).name ? x.name + ': ' + s.label : x.name, steps: st, i: s.i, pv: s.i < 0, fid: s.fid, main: false, sub: true, ext: true, fname: AM.fnInfo(s.fid).name });
    rows.push({ k: 'msg', dir: 'reply', from: ek, to: from, text: '', steps: st, i: s.i, pv: s.i < 0, fid: s.fid, main: false });
  });
  const codeRow = (a, b, from, to, steps, pv) => rows.push({ k: 'code', fid: a, target: b, from, to, steps, pv });

  // groups(l, steps): messages of level l — a step starts a new message when its caller and itself are different
  // participants on this level (or it is an async hand-off); otherwise it folds into the message before it
  const groups = (l, steps) => {
    const out = []; let cur = null;
    steps.forEach((s) => {
      // a planned step (no code yet) is its own message to the module it is meant for (or the service)
      const src = caller(s), fromK = src ? partAt(l, src) : 'actor', toK = s.planned ? (l > 0 && s.mod ? modPart(s.mod) : svcPart()) : partAt(l, s.fid);
      if (!cur || s.kind === 'async' || s.planned || cur.plan || fromK !== toK) { cur = { l, from: fromK, to: toK, src, async: s.kind === 'async', plan: !!s.planned, steps: [s] }; out.push(cur); }
      else cur.steps.push(s);
    });
    return out;
  };
  // the chain of calls from the caller of s to s (for frames on the function and code levels)
  const hops = (s) => { const src = caller(s); if (!src) return []; return AM.callPath(src, s.fid, SEQ_HOPS) || [src, s.fid]; };
  const enterable = (g) => !g.plan && g.l < 2 || g.steps.some((s) => hops(s).length > (g.l === 3 ? 2 : 1));

  const emitGroups = (l, steps, pkey, top) => {
    groups(l, steps).forEach((g) => {
      const h = g.steps[0], real = h.i >= 0, st = idxOf(g.steps);
      if (top && real && fr.length) closeReplies(depthOf(h.i));
      const key = pkey + '/' + l + ':' + (real ? h.i : 'p' + h.fid) + ':' + g.from + '>' + g.to;
      const base = { k: 'msg', steps: st, i: real ? h.i : -1, pv: !real, fid: h.fid, main: true, key, l, more: g.steps.length - 1, titles: g.steps.map((s) => s.label), plan: g.plan };
      const can = enterable(g), isOpen = can && sSub.has(key);
      const badges = badgesOf(g.steps);
      if (g.async) {
        const qk = queuePart();
        rows.push(Object.assign({}, base, { dir: 'async', from: g.from, to: qk, text: AM.t('seqEnq') + ': ' + AM.fnInfo(h.fid).short, num: false, main: false, key: null }));
        rows.push({ k: 'pause', steps: st, i: base.i, pv: !real });
        rows.push(Object.assign({}, base, { dir: 'async', from: qk, to: g.to, text: h.label, badges, fname: AM.fnInfo(h.fid).name, can, open: isOpen }));
      } else {
        rows.push(Object.assign({}, base, { dir: g.from === g.to ? 'self' : 'call', from: g.from, to: g.to, text: h.label, badges, fname: AM.fnInfo(h.fid).name, can, open: isOpen }));
      }
      if (l === 3 && g.src && !isOpen && !g.plan) codeRow(g.src, h.fid, g.from, g.to, st, !real);
      if (isOpen) {
        rows.push({ k: 'alt', t: 'start', sub: true, key, l: Math.min(3, l + 1), guard: h.label });
        if (l < 2) emitGroups(l + 1, g.steps, key, false);
        else g.steps.forEach((s) => {
          const p = hops(s), sst = idxOf([s]);
          for (let k = 0; k + 1 < p.length; k++) {
            const a = p[k], b = p[k + 1], last = k + 2 === p.length;
            const fa = fnPart(a), fb = fnPart(b);
            rows.push({ k: 'msg', dir: AM.isAsync(a, b) ? 'async' : fa === fb ? 'self' : 'call', from: fa, to: fb, text: last ? s.label : AM.fnInfo(b).name, steps: sst, i: s.i, pv: s.i < 0, fid: b, main: false, sub: true, fname: AM.fnInfo(b).name });
            codeRow(a, b, fa, fb, sst, s.i < 0);
          }
          extRows(s, fnPart(s.fid), 3);
        });
        rows.push({ k: 'alt', t: 'end', sub: true });
      } else g.steps.forEach((s) => extRows(s, g.to, l));
      if (top && real && fr.length && !g.async && !g.plan && g.from !== g.to) open.push({ from: g.from, to: g.to, depth: depthOf(h.i), i: h.i });
    });
  };
  const exitRow = (text, taken) => rows.push({ k: 'exit', text: AM.L(text), good: taken !== false && !seqBad.test(AM.L(text)) });
  const guard = (b) => b.cond || b.text || '?';
  const keyOf = (b) => (b.flow || '') + '|' + (b.cond || '') + '|' + (b.text || '');

  // a through path or a variant: the flows one after another, a plate between them, the end of the run at the bottom
  const emitFixed = () => {
    let base = 0;
    run.segs.forEach((sg, si) => {
      if (si) rows.push({ k: 'jump', text: AM.jv.viaText(sg), kind: AM.jv.viaKind(sg) });
      emitGroups(lvl, sg.steps.map((fid, k) => P.stepAt(base + k)), 's' + si, true);
      if (fr.length) closeReplies(-1);
      base += sg.steps.length;
    });
    const e = AM.jv.end();
    if (!e) return;
    if (e.jump) rows.push({ k: 'jump', text: e.text, kind: 'on', go: true });
    else rows.push({ k: 'exit', text: '', raw: e.text, good: e.tone !== 'bad' });
  };
  const emitSeg = (si, base) => {
    if (run.fixed) return emitFixed();
    const sg = run.segs[si], fl = sg.flow, last = si === run.segs.length - 1;
    emitGroups(lvl, sg.steps.map((fid, k) => P.stepAt(base + k)), 's' + si, true);
    if (fr.length) closeReplies(-1);
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
          emitGroups(lvl, tsteps.map((f) => { const s = stepOf(f, prev, target); prev = f; return s; }), 'b' + key, false);
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
  const arr = Array.from(parts.values());
  const rank = (p) => ({ actor: 0, svc: 1, mod: 2, fn: 2, queue: 3, ext: 4 })[p.kind];
  arr.sort((a, b) => rank(a) - rank(b) ||
    (rank(a) === 2 ? (SEQ_LD[a.layer] || 0) - (SEQ_LD[b.layer] || 0) || modFirst[a.mod] - modFirst[b.mod] || (a.kind === 'mod' ? 0 : 1) - (b.kind === 'mod' ? 0 : 1) : 0) ||
    a.n - b.n);
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
const numText = (st) => {
  if (!st.length) return '';
  const a = Math.min(...st), b = Math.max(...st);
  return a === b ? String(a + 1) : (a + 1) + '–' + (b + 1);
};
// toggle a message frame; leaving also folds the frames nested in it
function seqToggle(key) {
  if (!key) return;
  if (sSub.has(key)) { Array.from(sSub).forEach((k) => { if (k === key || k.startsWith(key + '/')) sSub.delete(k); }); }
  else sSub.add(key);
  seqRender(); seqSync(true);
}

function seqRender() {
  sDirty = false; sItems = []; sAct = null;
  if (isSheet()) { seqBar(); AM.sheet.render(sWrap, { level: sLevel, setLevel: (l) => seqSetLevel(l) }); return; }
  const top = sWrap.scrollTop, left = sWrap.scrollLeft;
  sWrap.innerHTML = '';
  seqBar();
  const m = seqModel();
  if (!m || !m.rows.length) { AM.h('div', { class: 'seq-empty' }, sWrap, AM.esc(AM.t('seqEmpty'))); return; }
  const parts = m.parts, np = parts.length;
  // the right margin is wider than the left: a self-call loop with its label hangs to the right of the last lifeline
  const cw = (sWrap.clientWidth || 900) / sZ, x0 = 84, xr = 240, colMin = sLevel >= 2 ? 170 : 150;
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
    else if (r.k === 'out' || r.k === 'exit' || r.k === 'jump') h = 38;
    else if (r.k === 'code') h = 24;
    y += h; o.h = h; lay.push(o);
  });
  const Ht = y + 24;
  sDim = { W, Ht };
  const hsvg = AM.svg('svg', { class: 'seq seqhead', width: W * sZ, height: HEAD_H * sZ, viewBox: `0 0 ${W} ${HEAD_H}` }, sWrap);
  const svg = AM.svg('svg', { class: 'seq seqbody', width: W * sZ, height: Ht * sZ, viewBox: `0 0 ${W} ${Ht}`, style: 'display:block' }, sWrap);
  const gf = AM.svg('g', {}, svg), gl = AM.svg('g', {}, svg), gm = AM.svg('g', {}, svg), gh = AM.svg('g', {}, hsvg);

  // heads and lifelines
  parts.forEach((p) => {
    AM.svg('line', { x1: X[p.key], y1: 0, x2: X[p.key], y2: Ht - 10, class: 'lifeline' }, gl);
    const hd = AM.svg('g', { class: 'head ' + p.kind + (p.kind === 'ext' || p.kind === 'queue' ? ' ext' : ''), 'data-p': p.key, tabindex: 0, role: 'button' }, gh);
    const w = Math.min(colW ? colW - 10 : 150, p.kind === 'svc' ? 200 : 156);
    AM.svg('rect', { x: X[p.key] - w / 2, y: 12, width: w, height: 42, rx: p.kind === 'actor' ? 21 : p.kind === 'fn' ? 5 : 9, style: p.kind === 'ext' || p.kind === 'queue' ? 'stroke-dasharray:4 3' : '' }, hd);
    if (p.kind === 'mod' || p.kind === 'fn') AM.svg('rect', { x: X[p.key] - w / 2 + 8, y: 22, width: 3, height: 22, rx: 1.5, fill: AM.LCOLOR[p.layer], style: 'filter:none;stroke:none' }, hd);
    const kind = p.kind === 'actor' ? AM.t('actor') : p.kind === 'svc' ? AM.t('seqSvc') : p.kind === 'mod' ? AM.t('layers')[p.layer] : p.kind === 'fn' ? AM.short(p.mod) : p.kind === 'queue' ? AM.t('queue') : (AM.t('kinds')[p.ext.kind] || AM.t('seqExternal'));
    AM.svg('text', { x: X[p.key], y: 29, 'text-anchor': 'middle', class: 'pk' }, hd, txt(kind, Math.max(10, Math.floor(w / 7.5))));
    AM.svg('text', { x: X[p.key], y: 45, 'text-anchor': 'middle' }, hd, txt(p.name, Math.max(10, Math.floor(w / 7))));
    AM.svg('title', null, hd, p.kind === 'mod' ? p.mod : p.kind === 'fn' ? p.fid : p.name);
    const pick = () => {
      if (p.kind === 'mod') AM.select({ t: 'mod', id: AM.map.unitOf(p.mod) || p.mod });
      else if (p.kind === 'fn') AM.select({ t: 'fn', id: p.fid });
      else if (p.kind === 'svc') AM.select({ t: 'svc' });
      else if (p.kind === 'actor' && p.actor) AM.select({ t: 'actor', id: p.actor.id });
      else if (p.ext) AM.select({ t: 'ext', id: p.ext.id });
    };
    hd.addEventListener('click', pick);
    hd.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.stopPropagation(); pick(); } });
  });

  const button = (parent, x, y, label, fn, cls) => {
    const w = 18 + label.length * 6.3;
    const g = AM.svg('g', { class: 'sbtn' + (cls ? ' ' + cls : ''), tabindex: 0, role: 'button' }, parent);
    AM.svg('rect', { x: x - w, y: y - 11, width: w, height: 22, rx: 11 }, g);
    AM.svg('text', { x: x - w / 2, y: y + 4, 'text-anchor': 'middle' }, g, label);
    const run_ = (e) => { e.stopPropagation(); fn(); };
    g.addEventListener('click', run_);
    g.addEventListener('dblclick', (e) => e.stopPropagation());
    g.addEventListener('keydown', (e) => { if (e.key === 'Enter') run_(e); });
    return w;
  };

  // frames: alt blocks of the branches and the frames of opened messages
  const stack = [], frames = [];
  const xL = (d) => 12 + d * 10, xR = (d) => W - 12 - d * 10;
  lay.forEach((o) => {
    const r = o.r;
    if (r.k !== 'alt') return;
    if (r.t === 'start') stack.push({ y: o.y, d: stack.length, guard: r.guard, seps: [], sub: r.sub, key: r.key, l: r.l });
    else if (r.t === 'else') stack[stack.length - 1].seps.push({ y: o.y, guard: r.guard });
    else { const f = stack.pop(); f.y2 = o.y + 6; frames.push(f); }
  });
  frames.sort((a, b) => a.d - b.d).forEach((f) => {
    const g = AM.svg('g', { class: 'frame' + (f.sub ? ' sub' : '') }, gf), l = xL(f.d), w = xR(f.d) - l;
    const tag = f.sub ? String(AM.t('lv')[f.l]).toLowerCase() : 'alt', tw = Math.max(44, 14 + tag.length * 7.2);
    AM.svg('rect', { x: l, y: f.y, width: w, height: f.y2 - f.y, rx: 8, class: 'f' }, g);
    AM.svg('path', { d: `M${l},${f.y + 20} h${tw} l8,-8 v-12`, class: 'tag' }, g);
    AM.svg('text', { x: l + 10, y: f.y + 15, style: 'font-weight:700' }, g, tag);
    AM.svg('text', { x: l + tw + 20, y: f.y + 15 }, g, f.sub ? txt(f.guard, 90) : '[' + txt(f.guard, 90) + ']');
    if (f.sub) button(g, l + w - 8, f.y + 14, AM.t('seqLeave'), () => seqToggle(f.key), 'leave');
    f.seps.forEach((s) => {
      AM.svg('line', { x1: l, y1: s.y + 2, x2: l + w, y2: s.y + 2, stroke: 'var(--edge)', 'stroke-dasharray': '6 4' }, g);
      AM.svg('text', { x: l + tw + 20, y: s.y + 17 }, g, '[' + txt(s.guard, 90) + ']');
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
  const need = [];
  let depthNow = 0;
  lay.forEach((o) => {
    const r = o.r;
    if (r.k === 'alt') { if (r.t === 'start') depthNow++; else if (r.t === 'end') depthNow--; return; }
    const l = xL(depthNow), rr = xR(depthNow);
    if (r.k === 'pause') {
      const g = AM.svg('g', { class: 'pause' + (r.pv ? ' pv' : '') }, gm);
      AM.svg('rect', { x: l + 4, y: o.y + 6, width: rr - l - 8, height: 20, fill: 'var(--bg)' }, g);
      AM.svg('path', { d: `M${l + 12},${o.y + 8} l10,4 l-10,4 l10,4 l-10,4`, fill: 'none', stroke: 'var(--muted)' }, g);
      AM.svg('text', { x: W / 2, y: o.y + 20, 'text-anchor': 'middle' }, g, '⏸ ' + AM.t('seqPause'));
      sItems.push({ g, steps: r.pv ? [] : r.steps, main: false });
      return;
    }
    if (r.k === 'code') {
      const cs = AM.callSite(r.fid, r.target);
      if (!cs.has && cs.loadable && !sReq.has(r.fid)) need.push(r.fid);
      const g = AM.svg('g', { class: 'code' + (r.pv ? ' pv' : '') }, gm);
      const a = X[r.from], b = X[r.to], x = Math.max(l + 8, Math.min(Math.min(a, b) + 10, rr - 260));
      const loc = (cs.file ? cs.file.split('/').pop() : '') + (cs.no ? ':' + cs.no : '');
      const body = cs.text || (cs.has ? AM.fnInfo(r.target).name + '(…)' : AM.t('seqNoSrc'));
      const room = Math.max(30, Math.floor((rr - x - 30) / 6.6) - loc.length);
      const t = AM.svg('text', { x: x + 8, y: o.y + 13 }, g);
      AM.svg('tspan', { class: 'loc' }, t, loc + '  ');
      AM.svg('tspan', { class: cs.text ? 'src' : 'loc' }, t, txt(body, room));
      const bb = { w: Math.min(rr - x, 16 + (loc.length + 2 + Math.min(body.length, room)) * 6.6) };
      g.insertBefore(AM.svg('rect', { x, y: o.y, width: bb.w, height: 19, rx: 4 }), t);
      AM.svg('title', null, g, AM.t('seqCallLine') + ': ' + AM.fnInfo(r.fid).name + ' → ' + AM.fnInfo(r.target).name + (cs.text ? '\n' + cs.text : ''));
      g.addEventListener('click', () => AM.select({ t: 'fn', id: r.fid }));
      sItems.push({ g, steps: r.pv ? [] : r.steps, main: false, sub: true });
      return;
    }
    if (r.k === 'exit') {
      const g = AM.svg('g', { class: 'exitp' }, gm), cx = X.actor;
      const tx = (r.good ? '✓ ' : '✕ ') + (r.raw ? txt(r.raw, 90) : AM.t('exit') + ': ' + txt(r.text, 80)), w = Math.min(rr - cx + 40, 34 + tx.length * 6.6);
      const col = r.good ? 'var(--good)' : 'var(--critical)';
      AM.svg('rect', { x: cx - 60, y: o.y + 6, width: w, height: 24, rx: 12, fill: `color-mix(in oklab, ${col} 12%, var(--surface))`, stroke: col }, g);
      AM.svg('text', { x: cx - 46, y: o.y + 22, class: 'exit' }, g, tx);
      return;
    }
    if (r.k === 'jump') {
      const g = AM.svg('g', { class: 'jv-jump' }, gm), tx = txt(r.text, 100), w = Math.max(120, 30 + tx.length * 6.6);
      AM.svg('rect', { x: l + 14, y: o.y + 6, width: Math.min(rr - l - 28, w), height: 24, rx: 12 }, g);
      AM.svg('text', { x: l + 28, y: o.y + 22 }, g, tx);
      AM.svg('title', null, g, r.text);
      if (r.go) button(g, l + 14 + Math.min(rr - l - 28, w) + 14 + 18 + AM.t('jvGo').length * 6.3, o.y + 18, '↪ ' + AM.t('jvGo'), () => AM.jv.jump());
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
      bx += 8 + button(g, bx, o.y + 22, lab, () => { if (sOpen.has(r.key)) sOpen.delete(r.key); else sOpen.add(r.key); seqRender(); seqSync(true); });
      button(g, bx + 18 + AM.t('seqTake').length * 6.3, o.y + 22, AM.t('seqTake'), () => take(r.b));
      return;
    }
    // message
    const a = X[r.from], b = X[r.to], reply = r.dir === 'reply';
    const yy = o.y + (reply ? 16 : 28);
    const cls = 'msg ' + (reply ? 'reply' : r.dir === 'async' ? 'async' : '') + (r.plan ? ' plan' : '') + (r.pv ? ' pv' : '') + (r.sub ? ' sub' : '') + (r.open ? ' opened' : '');
    const g = AM.svg('g', { class: cls, tabindex: r.main ? 0 : null, role: r.main ? 'button' : null, 'data-i': r.steps && r.steps.length ? r.steps[0] : -1, 'data-fid': r.fid || '', 'data-key': r.key || '' }, gm);
    const mk = reply ? 'url(#arrM)' : r.dir === 'async' ? 'url(#arrO)' : 'url(#arrT)';
    let lx = (a + b) / 2, labEnd;
    const label = r.text ? r.text + (r.more ? '  +' + r.more : '') : '';
    if (r.dir === 'self') {
      const d = `M${a},${yy} h44 v16 h-38`;
      AM.svg('path', { d, 'marker-end': mk }, g); AM.svg('path', { d, class: 'hit' }, g);
      lx = a + 52;
      const tx = txt(label, 60);
      AM.svg('text', { x: lx, y: yy + 12, 'text-anchor': 'start' }, g, tx);
      labEnd = lx + tx.length * 6.4;
    } else {
      const dir = b > a ? 1 : -1;
      AM.svg('line', { x1: a + dir * 4, y1: yy, x2: b - dir * 6, y2: yy, 'marker-end': mk }, g);
      AM.svg('line', { x1: a, y1: yy, x2: b, y2: yy, class: 'hit' }, g);
      const room = Math.max(Math.abs(b - a), colW) * 1.5;
      const tx = txt(label, Math.max(12, Math.floor(room / 6.4)));
      if (tx) AM.svg('text', { x: lx, y: yy - 7, 'text-anchor': 'middle' }, g, tx);
      labEnd = Math.max(a, b);
      if (reply) AM.svg('title', null, g, AM.t('seqReply'));
    }
    if (r.main && r.num !== false && r.steps && r.steps.length) AM.svg('text', { x: Math.min(a, b) - 10, y: yy + 4, 'text-anchor': 'end', class: 'num' }, g, numText(r.steps));
    if (r.badges && (r.badges.eff.length || r.badges.st.length)) {
      const pills = seqBadgePills(r.badges), tw = pills.reduce((s, p) => s + p.w + 6, -6);
      const bg = AM.svg('g', { class: 'badges' }, g);
      seqPill(bg, r.dir === 'self' ? lx : Math.max(l + 6, Math.min(lx - tw / 2, rr - tw - 6)), yy + (r.dir === 'self' ? 28 : 20), pills);
    }
    if (r.fname) AM.svg('title', null, g, r.fname + (r.more ? '\n' + r.titles.map((s, k) => (k + 1) + '. ' + s).join('\n') : ''));
    if (r.can && r.key) {
      const lab = r.open ? AM.t('seqLeave') : AM.t('seqEnter'), bw = 18 + lab.length * 6.3;
      const bx = Math.min(rr - 4, labEnd + 14 + bw);
      button(g, bx, yy + (r.dir === 'self' ? 8 : 0), lab, () => seqToggle(r.key), 'enter' + (r.open ? ' on' : ''));
      AM.svg('title', null, g.lastChild, AM.t('seqEnterTip'));
    }
    const act = () => { if (r.steps && r.steps.length && !r.pv) go(r.steps[0]); else if (r.fid) AM.select({ t: 'fn', id: r.fid }); };
    g.addEventListener('click', act);
    if (r.can && r.key) g.addEventListener('dblclick', (e) => { e.preventDefault(); seqToggle(r.key); });
    g.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.stopPropagation(); act(); }
      else if (r.can && r.key && (e.key === '+' || e.key === '=' || (e.key === '-' && r.open))) { e.stopPropagation(); e.preventDefault(); seqToggle(r.key); }
    });
    sItems.push({ g, steps: r.pv ? [] : (r.steps || []), main: r.main, sub: r.sub, ext: r.ext, y: o.y + 20, x: b, x0: a, fid: r.fid, pv: r.pv, key: r.key });
  });
  sAct = AM.svg('rect', { class: 'act cur', width: 10, rx: 2, x: -100, y: -100, height: 0 }, gl);
  sWrap.scrollTop = top; sWrap.scrollLeft = left;
  if (need.length) {
    need.forEach((f) => sReq.add(f));
    Promise.all(need.map((f) => AM.loadSrc(f))).then((bs) => { if (bs.some((x) => x != null) && sShown()) { seqRender(); seqSync(false); } });
  }
}

/* ---------- toolbar: levels, link with the map, scope, frames ---------- */
function seqBar() {
  if (!sBar) return;
  sBar.innerHTML = '';
  if (AM.sheet) {
    const md = AM.h('span', { class: 'seg sd-mode', role: 'group', 'aria-label': 'view' }, sBar);
    [['sheet', 'shSheet', 'shSheetTip'], ['seq', 'shSeq', 'shSeqTip']].forEach(([k, lab, tip]) => {
      const b = AM.h('button', { 'aria-pressed': sMode === k, 'data-mode': k, title: AM.t(tip) }, md, AM.esc(AM.t(lab)));
      b.onclick = () => seqMode(k);
    });
  }
  const sheet = isSheet();
  const seg = AM.h('span', { class: 'seg', role: 'group', 'aria-label': 'level' }, sBar);
  AM.t('lv').forEach((name, i) => {
    if (sheet && i === 3) return; // the sheet has no code level: "functions" stands for it
    const b = AM.h('button', { 'aria-pressed': i === (sheet ? Math.min(2, sLevel) : sLevel), 'data-l': i }, seg, AM.esc(name));
    b.onclick = () => (sheet && i === 2 && sLevel === 3 ? null : seqSetLevel(i));
  });
  const lk = AM.h('label', { class: 'lnk', title: AM.t('seqLinkTip') }, sBar);
  const cb = AM.h('input', { type: 'checkbox' }, lk); cb.checked = sLink;
  lk.appendChild(document.createTextNode(AM.t('seqLink')));
  cb.onchange = () => {
    sLink = cb.checked;
    try { localStorage.setItem('archmap.seqlink', sLink ? '1' : '0'); } catch (e) { /* ignore */ }
    if (sLink && AM.map.level !== sLevel) AM.map.setLevel(sLevel, seqFocusFid(seqFocusStep()));
  };
  if (sLevel >= 2 && !sheet) {
    const s = AM.play.current(), m = sScope || (AM.state.level === 2 && AM.state.mod) || (s && s.mod) || (AM.play.stepAt(0) || {}).mod;
    const b = AM.h('button', { class: 'chip-btn', 'aria-pressed': !!sScope, title: AM.t('seqScopeTip') }, sBar, AM.esc(sScope ? AM.t('seqScopeMod', AM.short(sScope)) : AM.t('seqScopeAll')));
    b.onclick = () => { sScope = sScope ? null : m || null; seqRender(); seqSync(true); };
  }
  if (sSub.size && !sheet) {
    const b = AM.h('button', { class: 'chip-btn' }, sBar, AM.esc(AM.t('seqFoldAll')));
    b.onclick = () => { sSub.clear(); seqRender(); seqSync(true); };
  }
  AM.h('span', { class: 'jv-tag', hidden: true }, sBar);
  AM.jv.paintTags();
  seg.title = AM.t('seqZoomHint');
}

/* ---------- levels and zoom ---------- */
// the step the diagram keeps in view: the current one, else the one of the message nearest the middle
function seqFocusStep() {
  if (AM.state.step >= 0) return AM.state.step;
  const mid = sWrap.scrollTop / sZ + sWrap.clientHeight / sZ / 2;
  let best = -1, bd = Infinity;
  sItems.forEach((it) => { if (it.main && it.steps.length) { const d = Math.abs(it.y - mid); if (d < bd) { bd = d; best = it.steps[0]; } } });
  return best;
}
const seqFocusFid = (i) => { const s = i >= 0 ? AM.play.stepAt(i) : null; return s ? s.fid : null; };
function seqStepAt(e) {
  const el = document.elementFromPoint(e.clientX, e.clientY), g = el && el.closest && el.closest('.msg');
  const i = g ? +g.getAttribute('data-i') : -1;
  return i >= 0 ? i : seqFocusStep();
}
// scroll so the (deepest) message holding step i is in the middle
function seqScrollTo(i) {
  if (i < 0) return;
  let it = null; sItems.forEach((x) => { if ((x.main || x.sub) && x.steps.includes(i)) it = x; });
  if (!it) return;
  sWrap.scrollTop = Math.max(0, it.y * sZ - sWrap.clientHeight / 2);
  sWrap.scrollLeft = Math.max(0, it.x * sZ - sWrap.clientWidth / 2);
}
// seqSetLevel(l, focus, fromMap): the level of the whole diagram; keeps step `focus` in view; moves the map along
function seqSetLevel(l, focus, fromMap) {
  l = Math.max(0, Math.min(3, l));
  if (l === sLevel) return Promise.resolve();
  const f = focus != null ? focus : sShown() ? seqFocusStep() : -1;
  sLevel = l;
  if (sLink && !fromMap && AM.map.level !== l) AM.map.setLevel(l, seqFocusFid(f));
  if (!sShown()) { sDirty = true; return Promise.resolve(); }
  seqRender(); seqSync(false); seqScrollTo(AM.state.step >= 0 ? AM.state.step : f);
  return Promise.resolve();
}
function seqScale(z, e) {
  const old = sZ; sZ = z;
  if (!sDim || old === z) return;
  const r = sWrap.getBoundingClientRect(), px = e ? e.clientX - r.left : sWrap.clientWidth / 2, py = e ? e.clientY - r.top : sWrap.clientHeight / 2;
  const wx = (sWrap.scrollLeft + px) / old, wy = (sWrap.scrollTop + py) / old;
  sWrap.querySelectorAll('svg.seq').forEach((s) => {
    const vb = s.viewBox.baseVal; s.setAttribute('width', vb.width * z); s.setAttribute('height', vb.height * z);
  });
  sWrap.scrollLeft = wx * z - px; sWrap.scrollTop = wy * z - py;
}
function seqWheel(e) {
  if (!sShown()) return;
  if (isSheet()) { AM.sheet.wheel(e, sLevel, (l) => seqSetLevel(l)); return; }
  if (!AM.zoom.isZoomWheel(e)) return; // a plain wheel / two-finger scroll scrolls the diagram natively
  const focus = seqStepAt(e);
  AM.zoom.scaleWheel(e, {
    z: sZ, set: (z) => seqScale(z, e),
    in: sLevel < 3 ? () => seqSetLevel(sLevel + 1, focus) : null,
    out: sLevel > 0 ? () => seqSetLevel(sLevel - 1, focus) : null,
  });
}

/* ---------- sync with the player ---------- */
function seqSig() {
  const r = AM.state.run;
  if (!r) return 'none';
  return r.key + '|' + r.segs.map((s) => s.steps.length + ':' + (s.via ? 1 : 0)).join(',') + '|' + AM.state.exitText + '|' + AM.play.list().length;
}
// seqSync(keep): light the messages of the current step; keep = do not scroll
function seqSync(keep) {
  if (isSheet()) { AM.sheet.sync(keep); return; }
  const cur = AM.state.step;
  let last = null;
  sItems.forEach((it) => {
    if (!it.steps.length) return;
    const has = cur >= 0 && it.steps.includes(cur), lo = Math.min(...it.steps), hi = Math.max(...it.steps);
    it.g.classList.toggle('cur', !!(it.main || it.sub) && has);
    it.g.classList.toggle('past', cur >= 0 && !has && hi < cur);
    it.g.classList.toggle('future', cur >= 0 && !has && lo > cur);
    if ((it.main || it.sub) && has && it.y != null && !it.ext) last = it; // the deepest message holding the step
  });
  if (sAct) {
    if (last) { sAct.setAttribute('x', last.x - 5); sAct.setAttribute('y', last.y - 4); sAct.setAttribute('height', 26); }
    else { sAct.setAttribute('x', -100); sAct.setAttribute('height', 0); }
  }
  if (keep) return;
  if (last && sShown()) {
    const top = sWrap.scrollTop, hh = sWrap.clientHeight, ly = last.y * sZ, lx = last.x * sZ;
    if (ly < top + HEAD_H * sZ + 30 || ly > top + hh - 70) sWrap.scrollTo({ top: Math.max(0, ly - hh / 2), behavior: AM.RM ? 'auto' : 'smooth' });
    // horizontally: the arrow as a whole when it fits, else its head
    const lft = sWrap.scrollLeft, wd = sWrap.clientWidth, x1 = Math.min(last.x, last.x0) * sZ, x2 = Math.max(last.x, last.x0) * sZ;
    if (x1 < lft + 40 || x2 > lft + wd - 40) sWrap.scrollTo({ left: Math.max(0, (x2 - x1 < wd - 120 ? (x1 + x2) / 2 : lx) - wd / 2), behavior: AM.RM ? 'auto' : 'smooth' });
  } else if (!last && cur < 0 && sShown()) sWrap.scrollTop = 0;
}
// seqRefresh(force): the scenario or the size changed → redraw (when the dock is open, else on its next opening), then light the step
function seqRefresh(force) {
  const sg = seqSig();
  const changed = sg !== sSig;
  if (changed && sg.split('|')[0] !== sSig.split('|')[0]) { sSub.clear(); sScope = null; } // another scenario: its frames are gone
  sSig = sg;
  if (!sShown()) { if (changed || force) sDirty = true; return; }
  if (force || changed || sDirty) seqRender();
  seqSync();
}
function seqSelect(sel) {
  if (isSheet()) { AM.sheet.select(sel); return; }
  if (!sItems.length) return;
  sItems.forEach((it) => it.g.classList.toggle('sel', !!sel && sel.t === 'fn' && it.fid === sel.id && !!it.main));
}


/* ---------- the scenario combobox in the header of the dock ---------- */
// search; paths, then scenarios ("Main" on top, groups by actor with counts, the auto ones, the general runs); the current
// one lit; a click switches and closes; ↑/↓ in the field take the neighbour, Enter / Esc close
let sPop = null, sCur = null, sListQ = '', sListKeys = [], sPopOpen = false;
function sdPopMount() {
  sPop.innerHTML = '';
  const q = AM.h('input', { type: 'search', class: 'sl-q', placeholder: AM.t('flowSearch'), autocomplete: 'off', spellcheck: 'false', role: 'combobox', 'aria-controls': 'sdlist', 'aria-expanded': 'true' }, sPop);
  q.value = sListQ;
  let qt = null;
  q.oninput = () => { clearTimeout(qt); qt = setTimeout(() => { sListQ = q.value; seqListDraw(); }, 60); };
  q.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); seqListStep(e.key === 'ArrowDown' ? 1 : -1); }
    else if (e.key === 'Enter') { e.preventDefault(); sdPop(false); }
    else if (e.key === 'Escape') { e.preventDefault(); if (q.value) { q.value = ''; sListQ = ''; seqListDraw(); } else sdPop(false); }
  });
  AM.h('div', { class: 'sl-body', id: 'sdlist', role: 'listbox' }, sPop).addEventListener('click', (e) => {
    const vt = e.target.closest('[data-vt]'); if (vt) { AM.jv.toggle(vt.dataset.vt); return; }
    const r = e.target.closest('.sl-row'); if (r) seqListPick(r.dataset.k);
  });
  seqListDraw();
}
function sdPop(on) {
  if (!sPop || on === sPopOpen) return;
  sPopOpen = on; sPop.hidden = !on;
  sCur.setAttribute('aria-expanded', on);
  if (!on) return;
  // the list opens upwards from a bottom dock (over the map), else downwards; its height is what the stage leaves
  const hr = sHost.getBoundingClientRect(), br = sCur.getBoundingClientRect();
  const up = sDock.dataset.pos === 'bottom' && !sFull;
  sPop.style.maxHeight = Math.max(160, Math.min(460, (up ? br.top - hr.top : hr.bottom - br.bottom) - 14)) + 'px';
  seqListMark(true);
  const q = sPop.querySelector('.sl-q'); if (q) { q.focus(); q.select(); }
}
function seqListDraw() {
  const body = sPop && sPop.querySelector('.sl-body'); if (!body) return;
  const D = D_(), q = sListQ.trim().toLowerCase();
  const key = (fl) => 'flow:' + fl.i;
  const row = (k, name, sub, extra) => `<div class="sl-row" role="option" data-k="${AM.esc(k)}" title="${AM.esc(name)}"><span class="nm">${AM.esc(name)}</span>${sub ? `<span class="sb">${AM.esc(sub)}</span>` : ''}${extra || ''}</div>`;
  // journeys and variants: rows of the shared builders (journeys.js), the run key is in data-k
  const jcfg = { row: 'sl-row', head: (title, n) => `<div class="sl-gh">${AM.esc(title)} <span>${n}</span></div>`, attr: (k, id) => `data-k="${k}:${AM.esc(id)}"` };
  const fr = (fl) => row(key(fl), fl.name, String(fl.steps.filter((s) => D.fns[s]).length), AM.jv.tog(fl)) + AM.jv.variantRows(fl, sListQ, jcfg);
  const ok = (fl) => !q || fl.name.toLowerCase().includes(q) || String(AM.flowActor(fl)).toLowerCase().includes(q) || AM.jv.flowHit(fl, q);
  let h = AM.jv.journeyRows(sListQ, jcfg); sListKeys = [];
  const add = (title, items, toRow) => {
    if (!items.length) return;
    h += `<div class="sl-gh">${AM.esc(title)} <span>${items.length}</span></div>` + items.map(toRow).join('');
  };
  const tops = D.flows.filter((fl) => fl.top && ok(fl));
  add(AM.t('grpTop'), tops, fr);
  AM.flowGroups(sListQ).forEach((g) => add(g.name, g.items, fr));
  const gen = AM.play.options().filter((o) => o[2] === '\0g' && (!q || String(o[1]).toLowerCase().includes(q)));
  add(AM.t('grpGeneral'), gen, (o) => row(o[0], o[1], ''));
  body.innerHTML = h || `<div class="sl-none">${AM.esc(AM.t('flowNone'))}</div>`;
  body.querySelectorAll('.sl-row').forEach((r) => { if (!sListKeys.includes(r.dataset.k)) sListKeys.push(r.dataset.k); });
  seqListMark(false);
}
function seqListMark(scroll) {
  if (!sPop) return;
  const k = AM.state.run ? AM.state.run.key : '';
  let first = null;
  sPop.querySelectorAll('.sl-row').forEach((r) => { const on = r.dataset.k === k; r.classList.toggle('cur', on); r.setAttribute('aria-selected', on); if (on && !first) first = r; });
  if (scroll && first) { const b = first.parentElement, top = first.offsetTop - b.offsetTop; if (top < b.scrollTop + 30 || top > b.scrollTop + b.clientHeight - 30) b.scrollTop = top - b.clientHeight / 2; }
}
function seqListPick(k, fromKey) {
  if (k && (!AM.state.run || AM.state.run.key !== k)) AM.play.load(k);
  if (!fromKey) sdPop(false);
}
function seqListStep(d) {
  if (!sListKeys.length) return;
  const k = AM.state.run ? AM.state.run.key : '', i = sListKeys.indexOf(k);
  seqListPick(sListKeys[i < 0 ? 0 : Math.max(0, Math.min(sListKeys.length - 1, i + d))], true);
  seqListMark(true);
}

/* ---------- the dock: it appears with a scenario, sits at the bottom or on the right of the map ---------- */
// Lives inside the map view (#v-map), over the map. Place (bottom / right), size (a share of the stage) and the
// collapsed strip are kept in localStorage; the full-screen mode (the map is covered) is not. The map's cameras fit
// into what the dock leaves free (Cam.insetBottom / insetRight, AM.map.setDock); the CSS variables --dock-b / --dock-r
// move the map's own buttons. Esc leaves the full screen (AM.dock.esc, called by the key handler of main.js).
const SD_KEY = 'archmap.seqdock', SD_STRIP = 38;
const SD = { pos: 'bottom', bottom: 0.4, right: 0.45, min: false };
try {
  const o = JSON.parse(localStorage.getItem(SD_KEY) || '{}');
  if (o.pos === 'right') SD.pos = 'right';
  if (o.bottom >= 0.15 && o.bottom <= 0.75) SD.bottom = o.bottom;
  if (o.right >= 0.2 && o.right <= 0.75) SD.right = o.right;
  SD.min = o.min === true;
} catch (e) { /* file:// without storage or a broken value */ }
const sdSave = () => { try { localStorage.setItem(SD_KEY, JSON.stringify(SD)); } catch (e) { /* ignore */ } };
const clampN = (v, a, b) => Math.max(a, Math.min(b, v));
let sInitKey = null; // the run the page started with: it does not open the dock by itself
let sHint = null, sDock = null, sHost = null, sGrip = null, sStrip = null, sFull = false, sUser = null, sRaf = 0;
const sCtl = {};
const ICON = {
  bottom: '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="2" y="2.5" width="12" height="11" rx="2"/><path d="M2 9h12" /><rect x="3" y="9.4" width="10" height="3.4" fill="currentColor" stroke="none"/></svg>',
  right: '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="2" y="2.5" width="12" height="11" rx="2"/><path d="M9 2.5v11"/><rect x="9.4" y="3.5" width="3.6" height="9" fill="currentColor" stroke="none"/></svg>',
  min: '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M3 6l5 5 5-5"/></svg>',
  full: '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M6 2.5H2.5V6M10 2.5h3.5V6M6 13.5H2.5V10M10 13.5h3.5V10"/></svg>',
  unfull: '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 6H6V2.5M13.5 6H10V2.5M2.5 10H6v3.5M13.5 10H10v3.5"/></svg>',
  close: '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M4 4l8 8M12 4l-8 8"/></svg>',
  expand: '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M3 10l5-5 5 5"/></svg>',
};

// shown: the dock is on screen and open (not the strip) on the map tab — only then the diagram is drawn
const sShown = () => !!sDock && !sDock.hidden && AM.state.view === 'seq';
// wanted: a scenario, path or variant is chosen or plays (the start-up run waits for a step); the user's choice for this run wins
function sdWanted() {
  const r = AM.state.run;
  return AM.state.view === 'seq' && !!r && AM.play.list().length > 0;
}
function sdProgress() {
  const r = AM.state.run; if (!r) return '';
  if (r.fixed) return AM.jv.progress();
  return AM.t('jvStepOf', Math.max(0, AM.state.step + 1), AM.play.list().length);
}
// the labels that follow the player: the combobox button, the collapsed strip, the toggle on the map
function sdNames() {
  if (!sDock) return;
  const r = AM.state.run, name = r ? r.name : '';
  sCur.innerHTML = `<span class="nm">${AM.esc(name)}</span><span class="car" aria-hidden="true">▾</span>`;
  sCur.title = name;
  const pr = sdProgress();
  sStrip.innerHTML = `${ICON.expand}<span class="nm">${AM.esc(name)}</span><span class="pr">${AM.esc(pr)}</span>`;
  sStrip.title = AM.t('dockRestore');
  const tg = document.getElementById('btnSeq');
  if (tg) { tg.setAttribute('aria-pressed', !sDock.hidden); tg.disabled = !r || !AM.play.list().length; }
}
function sdTitles() {
  sCtl.bottom.title = AM.t('dockBottom'); sCtl.right.title = AM.t('dockRight');
  sCtl.min.title = AM.t('dockMin'); sCtl.close.title = AM.t('dockClose');
  sCtl.full.title = AM.t(sFull ? 'dockUnfull' : 'dockFull'); sCtl.full.innerHTML = sFull ? ICON.unfull : ICON.full;
  sCtl.bottom.setAttribute('aria-pressed', SD.pos === 'bottom'); sCtl.right.setAttribute('aria-pressed', SD.pos === 'right');
  sGrip.title = AM.t('dockGrip');
  sGrip.setAttribute('aria-orientation', SD.pos === 'bottom' ? 'horizontal' : 'vertical');
  const tg = document.getElementById('btnSeq'); if (tg) tg.title = AM.t('dockToggle');
}
// place and size; tells the map how much it lost. dur — the camera's animation (0 while dragging)
function sdLayout(dur) {
  if (!sDock) return;
  // the sequence diagram is a page of its own: it takes the whole canvas of its view (the map is not shared with it)
  const vis = sdWanted();
  sFull = vis;
  sDock.hidden = !vis;
  if (sHint) sHint.hidden = vis;
  const W = sHost.clientWidth || 1000, H = sHost.clientHeight || 600, st = sDock.style;
  sDock.dataset.pos = 'bottom';
  sDock.classList.remove('min');
  sDock.classList.add('full');
  st.top = st.left = st.right = st.bottom = '0'; st.width = st.height = '';
  const box = [vis, W, H].join();
  if (box !== sBox) { sBox = box; sDirty = true; } // another size: the diagram is laid out for the width it has
  sdTitles();
}
// one place that brings everything in line: visibility, layout, labels, the diagram
function sdSync(force, dur) {
  if (!sdBuild()) return;
  const was = sShown();
  sdLayout(dur);
  if (sDock.hidden && sPopOpen) sdPop(false);
  sdNames();
  seqRefresh(force || was !== sShown());
}
function sdRender() {
  if (sRaf) return;
  sRaf = requestAnimationFrame(() => { sRaf = 0; if (sShown()) { seqRender(); seqSync(true); } });
}
function sdUser(on) { const r = AM.state.run; if (!r) return; sUser = { key: r.key, on }; sdSync(true); }
function sdFull(on) {
  if (!sDock || on === sFull) return;
  sFull = on; sdPop(false);
  sdSync(true, 0);
  if (on && sCtl.full) sCtl.full.focus({ preventScroll: true });
}
function sdSet(patch) { Object.assign(SD, patch); sdSave(); sdPop(false); sdSync(true); }
function sdBuild() {
  if (sDock) return true;
  sHost = document.getElementById('v-seq'); if (!sHost) return false;
  sInitKey = AM.state.run ? AM.state.run.key : null;
  sDock = AM.h('section', { class: 'seqdock', hidden: true, 'aria-label': AM.t('seqTitle') }, sHost);
  sGrip = AM.h('div', { class: 'sd-grip', role: 'separator', tabindex: 0 }, sDock);
  const hd = AM.h('div', { class: 'sd-hd' }, sDock);
  sStrip = AM.h('button', { class: 'sd-strip' }, hd);
  const cbx = AM.h('div', { class: 'sd-cbx' }, hd);
  sCur = AM.h('button', { class: 'sd-cur', 'aria-haspopup': 'listbox', 'aria-expanded': 'false' }, cbx);
  sPop = AM.h('div', { class: 'sd-pop', hidden: true }, cbx);
  sBar = AM.h('div', { class: 'seq-bar' }, hd);
  const ctl = AM.h('div', { class: 'sd-ctl' }, hd);
  const pos = AM.h('span', { class: 'seg', role: 'group' }, ctl);
  sCtl.bottom = AM.h('button', null, pos, ICON.bottom); sCtl.right = AM.h('button', null, pos, ICON.right);
  sCtl.min = AM.h('button', { class: 'iconbtn sd-minb' }, ctl, ICON.min);
  sCtl.full = AM.h('button', { class: 'iconbtn' }, ctl, ICON.full);
  sCtl.close = AM.h('button', { class: 'iconbtn' }, ctl, ICON.close);
  sWrap = AM.h('div', { class: 'seq-wrap' }, AM.h('div', { class: 'sd-body' }, sDock));
  sLevel = sLink ? AM.map.level : 1;

  sCur.onclick = () => sdPop(!sPopOpen);
  sStrip.onclick = () => sdSet({ min: false });
  sCtl.bottom.onclick = () => sdSet({ pos: 'bottom' });
  sCtl.right.onclick = () => sdSet({ pos: 'right' });
  sCtl.min.onclick = () => sdSet({ min: true });
  sCtl.full.onclick = () => sdFull(!sFull);
  sCtl.close.onclick = () => sdUser(false);
  document.addEventListener('pointerdown', (e) => { if (sPopOpen && !e.target.closest('.sd-cbx')) sdPop(false); });
  sWrap.addEventListener('wheel', seqWheel, { passive: false });
  AM.zoom.onGesture(sWrap, seqWheel);

  // the separator: drag to resize, arrows to nudge, double-click for the default
  sGrip.addEventListener('pointerdown', (e) => {
    if (e.button !== 0 || SD.min || sFull) return;
    e.preventDefault(); try { sGrip.setPointerCapture(e.pointerId); } catch (err) { /* synthetic pointer */ } sDock.classList.add('drag');
    const move = (ev) => {
      const r = sHost.getBoundingClientRect();
      if (SD.pos === 'bottom') SD.bottom = clampN((r.bottom - ev.clientY) / r.height, 0.15, 0.7);
      else SD.right = clampN((r.right - ev.clientX) / r.width, 0.2, 0.7);
      sdLayout(0); sdRender();
    };
    const end = () => { sGrip.removeEventListener('pointermove', move); sGrip.removeEventListener('pointerup', end); sGrip.removeEventListener('pointercancel', end); sDock.classList.remove('drag'); sdSave(); };
    sGrip.addEventListener('pointermove', move); sGrip.addEventListener('pointerup', end); sGrip.addEventListener('pointercancel', end);
  });
  sGrip.addEventListener('dblclick', () => { SD.bottom = 0.4; SD.right = 0.45; sdSave(); sdLayout(); sdRender(); });
  sGrip.addEventListener('keydown', (e) => {
    const k = e.key, bot = SD.pos === 'bottom';
    const d = bot ? (k === 'ArrowUp' ? 1 : k === 'ArrowDown' ? -1 : 0) : (k === 'ArrowLeft' ? 1 : k === 'ArrowRight' ? -1 : 0);
    if (!d) return;
    e.preventDefault(); e.stopPropagation();
    if (bot) SD.bottom = clampN(SD.bottom + d * 0.03, 0.15, 0.7); else SD.right = clampN(SD.right + d * 0.03, 0.2, 0.7);
    sdSave(); sdLayout(0); sdRender();
  });
  const tg = document.getElementById('btnSeq'); if (tg) tg.onclick = () => AM.dock.toggle();
  sdPopMount();
  return true;
}

// Esc: close the open list, then leave the full screen; true when it took the key
function sdEsc() {
  if (sPopOpen) { sdPop(false); return true; }
  return false;
}
// the page "Sequence" of the sidebar list (like Data, API, Statuses): the sequence diagram of the chosen scenario
AM.registerView({
  id: 'seq',
  order: 45,
  menu: false,
  title: () => AM.t('seqPage'),
  mount(el) {
    sHint = AM.h('div', { class: 'seq-hint' }, el);
    sHint.textContent = AM.t('seqHint');
    sdSync(true, 0);
  },
  show() { if (sHint) sHint.textContent = AM.t('seqHint'); sdSync(true, 0); },
  onLang() { if (sHint) sHint.textContent = AM.t('seqHint'); },
});
AM.dock = {
  toggle() { const r = AM.state.run; if (!r) return; if (sdBuild()) sdUser(!sdWanted()); },
  // open(full): show the dock for the current run (full screen when asked)
  open(full) { const r = AM.state.run; if (!r || !sdBuild()) return; sUser = { key: r.key, on: true }; if (full) sFull = true; sdSync(true, 0); },
  esc: sdEsc,
  refresh() { if (sDock) seqRefresh(true); },
  // mode: 'sheet' (the scenario sheet) or 'seq' (the sequence diagram)
  get mode() { return isSheet() ? 'sheet' : 'seq'; }, setMode: (m) => seqMode(m),
  get shown() { return sShown(); },
};

AM.bus.on('flow', () => { sdSync(false); seqListMark(true); });
AM.bus.on('step', () => sdSync(false));
AM.bus.on('jv', () => seqListDraw());
AM.bus.on('select', (sel) => seqSelect(sel));
AM.bus.on('level', (l) => { if (sDock && sLink && l !== sLevel) seqSetLevel(l, -1, true); });
AM.bus.on('view', () => { if (sDock) sdSync(false, 0); });
AM.bus.on('lang', () => { if (!sDock) return; sdPopMount(); sdTitles(); sSig = seqSig(); sdNames(); if (sShown()) { seqRender(); seqSync(); } else sDirty = true; });
AM.bus.on('live', () => { if (!sDock) return; seqListDraw(); sdSync(true); });
AM.bus.on('resize', () => { if (sDock) sdSync(true, 0); });
// a walk step / its arrows changed: the sheet grows (the sequence follows the run by itself)
AM.bus.on('walk', (w) => {
  if (!sDock || !isSheet()) return;
  if (!sShown()) { sDirty = true; return; }
  if (w && w.kind === 'sel') AM.sheet.sync(true); else { seqRender(); seqSync(true); }
});
