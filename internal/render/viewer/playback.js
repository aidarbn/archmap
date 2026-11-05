/* playback.js — scenarios: a run of steps through the map levels, branches (continue in another flow or
 * exit), effects and status badges, the scenario bar. A scenario is opened from the list (side.js) or a card and goes only by
 * hand: → / ← or the bar's arrows. There is no auto play and nothing opens by itself.
 *
 * A run is {key, name, desc, actor, exit, segs: [{flow|null, steps: [fid]}]}; segs grow when a branch
 * continues in another flow. A step object (AM.play.current()) is:
 *   {i, fid, prev, next, seg, flow, kind: 'entry'|'call'|'async'|'next', label, effects: [string],
 *    data: [{table, op, fields}], states: [{entity, from, to, fid}], exts: [external], actor, mod}
 */

addI18n({
  ru: { planStep: 'запланированный шаг', planTag: 'план', planNoCode: 'Шаг запланирован в описании (arch/*.arch): кода ещё нет.', planIn: 'модуль', stepOf: (k, n) => 'Шаг ' + k + ' из ' + n, closeRun: 'Закрыть сценарий, вернуться к карте', backToMap: 'Карта' },
  en: { planStep: 'planned step', planTag: 'plan', planNoCode: 'The step is planned in the description (arch/*.arch): no code yet.', planIn: 'module', stepOf: (k, n) => 'Step ' + k + ' of ' + n, closeRun: 'Close the scenario, back to the map', backToMap: 'Map' },
});

/* ---------- runs ---------- */
const LAYER_DEPTH = { entry: 0, ui: 1, domain: 2, other: 2, adapter: 3, infra: 4, test: 4 };
// autoPath(start): a scenario without a model — follow callees, prefer another module and a deeper layer
function autoPath(start, max = 8) {
  const dep = (id) => LAYER_DEPTH[layerOf(D.fns[id].module)];
  const steps = [start], seen = new Set(steps);
  for (let cur = start; steps.length < max;) {
    let best = null, bs = -1e9;
    fnInfo(cur).out.forEach((c) => {
      if (seen.has(c)) return;
      const sc = (D.fns[c].module !== D.fns[cur].module ? 10 : 0) + (dep(c) - dep(cur)) * 2 - (/_test|\.test\.|\.spec\./.test(D.fns[c].file) ? 20 : 0);
      if (sc > bs) { bs = sc; best = c; }
    });
    if (!best) break;
    steps.push(best); seen.add(best); cur = best;
  }
  return steps;
}
function entryTour(kind) {
  const par = {}, dist = {}, q = [];
  D.entries.forEach((id) => { dist[id] = 0; par[id] = null; q.push(id); });
  for (let h = 0; h < q.length; h++) { const id = q[h]; (D.fns[id].callees || []).forEach((c) => { if (D.fns[c] && dist[c] == null) { dist[c] = dist[id] + 1; par[c] = id; q.push(c); } }); }
  const steps = [], push = (id) => { if (steps[steps.length - 1] !== id) steps.push(id); };
  if (kind === 'changes') {
    const ch = D.changed.slice().sort((a, b) => (dist[a] == null ? 1e9 : dist[a]) - (dist[b] == null ? 1e9 : dist[b]) || a.localeCompare(b));
    const seen = new Set();
    for (const id of ch) {
      if (seen.has(id) || steps.length > 400) continue;
      const path = []; for (let c = id; c != null; c = par[c]) path.push(c);
      path.reverse().forEach((p) => { push(p); seen.add(p); });
    }
    return steps;
  }
  const seen = new Set();
  const walk = (id, d) => { if (steps.length >= 60 || seen.has(id) || d > 4) return; seen.add(id); push(id); (D.fns[id].callees || []).filter((c) => D.fns[c]).slice(0, 3).forEach((c) => walk(c, d + 1)); };
  D.entries.slice(0, 6).forEach((e) => walk(e, 0));
  return steps;
}
function buildRun(key) {
  if (!key) return null;
  if (key.startsWith('flow:')) {
    const fl = D.flows.find((x) => x.i === +key.slice(5));
    if (!fl) return null;
    return { key, name: fl.name, desc: fl.desc || '', segs: [{ flow: fl, steps: flowSteps(fl) }] };
  }
  if (key.startsWith('journey:')) return jvBuildJourney(key);
  if (key.startsWith('variant:')) return jvBuildVariant(key);
  if (key.startsWith('fn:')) {
    const id = key.slice(3);
    if (!D.fns[id]) return null;
    return { key, name: t('fnFlowTitle') + fnInfo(id).name, desc: '', segs: [{ flow: null, steps: autoPath(id) }] };
  }
  if (key === 'changes') return { key, name: t('optChanges'), desc: '', segs: [{ flow: null, steps: entryTour('changes') }] };
  if (key === 'overview') return { key, name: t('optOverview'), desc: '', segs: [{ flow: null, steps: entryTour('overview') }] };
  return null;
}
// flat list of steps of the current run: [{fid, seg}]
function runList() {
  const out = [];
  if (state.run) state.run.segs.forEach((sg, si) => sg.steps.forEach((fid) => out.push({ fid, seg: si })));
  return out;
}
const runFids = () => runList().map((x) => x.fid);
const lastSeg = () => (state.run ? state.run.segs[state.run.segs.length - 1] : null);
// branches without repeats: the same condition into the same flow (or with the same outcome) is one branch
const uniqBr = (bs) => { const seen = new Set(); return (bs || []).filter((b) => { const k = (b.cond || '') + '\0' + (b.flow || '') + '\0' + (b.flow ? '' : b.text || ''); if (seen.has(k)) return false; seen.add(k); return true; }); };
// every branch of the last flow of a flow run (not of a path or a variant: those go as written)
const allBranchesHere = () => { const sg = lastSeg(); return state.run && !state.run.fixed && sg && sg.flow && (sg.flow.branches || []).length ? uniqBr(sg.flow.branches) : null; };
// the branches the run stops at: those that go on in another flow; the folded outcomes (branch.fold) only when the
// "+N branches" mark is unfolded. None — the run simply ends.
const branchesHere = () => { const all = allBranchesHere(); if (!all) return null; const v = all.filter((b) => !b.fold || PATH.open.has('end')); return v.length ? v : null; };
// the run waits for a branch choice: at its last step, the last flow has branches, none taken yet
const atBranch = () => { const n = runList().length; return n > 0 && state.step === n - 1 && !!branchesHere() && !state.exitText; };
function runActor() {
  const sg = state.run && state.run.segs[0];
  if (!sg || !sg.steps.length) return null;
  return actorOfFn(sg.steps[0]);
}
function statesOf(fid) {
  let st = D.transByFid[fid] || [];
  if (!st.length) (D.fns[fid] && D.fns[fid].callees || []).forEach((c) => { st = st.concat(D.transByFid[c] || []); });
  const seen = new Set();
  return st.filter((x) => { const k = x.entity + ':' + x.to; if (seen.has(k)) return false; seen.add(k); return true; }).slice(0, 2);
}
function stepAt(i) {
  const L_ = runList(); const it = L_[i]; if (!it) return null;
  const sg = state.run.segs[it.seg], fl = sg.flow;
  // the previous step with a function: planned steps (no code) in between are skipped
  let j = i - 1;
  while (j >= 0 && L_[j].seg === it.seg && isPlan(L_[j].fid)) j--;
  // the first step of a further flow of a path: a new entry (a queue hand-off keeps its caller)
  const first = j >= 0 && L_[j].seg !== it.seg, viaQ = first && sg.via && (sg.via.via === 'queue' || sg.via.queue);
  // links come from the code only: the next flow of a path keeps the previous step as its caller when the code calls
  // it (or hands it over through the queue); otherwise the client makes the next call — a 'client' step, no arrow
  // from the previous flow (the data may also say so: via 'client')
  const coded = first && !viaQ && !isPlan(L_[j].fid) && !isPlan(it.fid) && (isCall(L_[j].fid, it.fid) || isAsync(L_[j].fid, it.fid)) && !(sg.via && sg.via.via === 'client');
  const client = first && !viaQ && !coded;
  const prev = j >= 0 && (!first || viaQ || coded) && !isPlan(L_[j].fid) ? L_[j].fid : null, next = L_[i + 1] ? L_[i + 1].fid : null;
  const plan = isPlan(it.fid), f = D.fns[it.fid] || {};
  const extName = plan && fl && fl.ext && fl.ext[it.fid], planExt = extName ? D.externals.filter((x) => x.name === extName) : [];
  return {
    i, fid: it.fid, prev, next, seg: it.seg, flow: fl, planned: plan,
    kind: plan ? 'plan' : client ? 'client' : !prev ? 'entry' : viaQ || isAsync(prev, it.fid) ? 'async' : isCall(prev, it.fid) ? 'call' : 'next',
    label: (fl && fl.labels && fl.labels[it.fid]) || (plan ? t('planStep') : fnInfo(it.fid).name),
    effects: (fl && fl.effects && fl.effects[it.fid]) || [],
    data: f.data || [], states: plan ? [] : statesOf(it.fid), exts: plan ? planExt : extsOfFn(it.fid),
    actor: i === 0 ? runActor() : first && !prev ? actorOfFn(it.fid) || runActor() : null,
    mod: plan ? (fl && fl.planMod && fl.planMod[it.fid]) || null : f.module,
  };
}
const curStep = () => (state.step >= 0 ? stepAt(state.step) : null);

/* ---------- what a step does, from the code ----------
 * The pill of the current leg and the tooltips of the numbers say what happens and what is passed, read from the
 * code (not the step text of arch:step, that one is in the top bar and the inspector):
 *   a call — the called function with the key arguments → its result: requestQR(Payment, KaspiDevice) → QRResult
 *   a queue hand-off — → queue: CheckKaspiPaymentArgs; a client call — client → POST /orders/{id}/kaspi-qr
 *   data — UPDATE orders; an outside system — → Kaspi: CreateQR; a status — Order: validated → payment_requested */
addI18n({ ru: { actQueue: 'очередь', actClient: 'клиент' }, en: { actQueue: 'queue', actClient: 'client' } });
// split s by sep at the top level (outside (), [], {}, <>)
function splitTop(s, sep = ',') {
  const out = []; let d = 0, cur = '';
  for (const ch of s) {
    if ('([{<'.includes(ch)) d++; else if (')]}>'.includes(ch)) d--;
    if (ch === sep && d === 0) { out.push(cur.trim()); cur = ''; } else cur += ch;
  }
  if (cur.trim()) out.push(cur.trim());
  return out;
}
// the text inside the parenthesis group that starts at i (s[i] === '('): [inner, index after ')']
function parenAt(s, i) { let d = 0; for (let k = i; k < s.length; k++) { if (s[k] === '(') d++; else if (s[k] === ')' && --d === 0) return [s.slice(i + 1, k), k + 1]; } return [s.slice(i + 1), s.length]; }
// params of a parameter list: [{n, t}] — Go (`a, b int64`, unnamed types), TS / Python / Kotlin / Swift (`a: T`)
function paramList(inner, go) {
  const items = splitTop(inner).filter(Boolean), out = [];
  if (go) {
    const named = items.some((x) => /^[\w]+\s+\S/.test(x));
    let pend = [];
    items.forEach((x) => {
      if (!named) { out.push({ n: '', t: x }); return; }
      const m = x.match(/^([\w]+)\s+(.+)$/);
      if (!m) { pend.push(x); return; }
      pend.concat(m[1]).forEach((n) => out.push({ n, t: m[2] })); pend = [];
    });
    return out;
  }
  items.forEach((x) => {
    const m = x.replace(/=.*$/, '').match(/^(?:[@\w]+\s+)*?\*{0,2}([\w$]+)\??\s*:\s*(.+)$/);
    if (m) out.push({ n: m[1], t: m[2].trim() }); else if (!/^(self|cls|this)$/.test(x)) out.push({ n: x.replace(/[=:].*$/, '').trim(), t: '' });
  });
  return out;
}
// a signature → {params: [{n, t}], res: [t]}; null when it cannot be read
function sigParts(sig) {
  if (!sig) return null;
  const go = /^func\b/.test(sig);
  let i = 0;
  if (go) { i = 4; while (sig[i] === ' ') i++; if (sig[i] === '(') i = parenAt(sig, i)[1]; }
  const p0 = sig.indexOf('(', i); if (p0 < 0) return null;
  const [inner, after] = parenAt(sig, p0);
  let rest = sig.slice(after).replace(/\{\s*$/, '').trim(), res = [];
  if (go) { if (rest.startsWith('(')) res = paramList(parenAt(rest, 0)[0], true).map((x) => x.t); else if (rest) res = [rest]; }
  else { const m = rest.match(/^(?:->|:)\s*(.+?)\s*(?:\{|=>|:)?$/); if (m) res = [m[1]]; }
  return { params: paramList(inner, go), res };
}
// a type without packages and pointers: *tabletsv2.RequestKaspiQRForOrderRequest → RequestKaspiQRForOrderRequest
// a queue job river.Job[XArgs] → XArgs
const shortType = (tp) => String(tp).replace(/\*/g, '').replace(/\b[a-zA-Z_][\w]*\./g, '').replace(/^Job\[(.+)\]$/, '$1').trim();
// generic method names read better with their type: CheckKaspiPaymentWorker.Work
const GENERIC_FN = /^(Work|Handle|Run|Execute|Exec|Process|ServeHTTP|Call|Invoke|Do|Start|New|__call__|handle|run|execute|process|invoke)$/;
const SKIP_T = /^(context\.Context|\*?slog\.Logger|\*?zap\.(Sugared)?Logger|\*?logrus\.\w+|\.\.\..*|\w*\.?Option|\*?http\.Request|http\.ResponseWriter|\*?sql\.Tx|pgx\.Tx)$/;
const PRIM_T = /^(\*?u?int(8|16|32|64)?|string|bool|float(32|64)|byte|rune|any|interface\{\}|uuid\.UUID|time\.Time|time\.Duration|\[\]byte|number|str|int|float|boolean|String|Int|Long|Bool|Boolean|Double)$/;
const SKIP_R = /^(error|void|None|Unit|Promise<void>)$/;
// call text of a function: name(args) → result
function callText(fid) {
  const f = D.fns[fid]; if (!f) return fnInfo(fid).short || '';
  const sp = sigParts(f.sig), nm = (f.cls && GENERIC_FN.test(f.name) ? f.cls + '.' : '') + (f.name || fnInfo(fid).short);
  if (!sp) return nm + '()';
  const args = sp.params.filter((p) => !SKIP_T.test(p.t.trim())).map((p) => (!p.t || (PRIM_T.test(p.t.trim()) && p.n) ? p.n : shortType(p.t))).filter(Boolean);
  const res = sp.res.filter((r) => !SKIP_R.test(r.trim()) && !/^err\b/.test(r)).map(shortType);
  return nm + '(' + args.join(', ') + ')' + (res.length ? ' → ' + res.join(', ') : '');
}
// the type a queue job carries: river.Job[CheckKaspiPaymentArgs] (or an Args parameter) → CheckKaspiPaymentArgs
function jobType(fid) {
  const f = D.fns[fid], sp = f && sigParts(f.sig); if (!sp) return null;
  for (const p of sp.params) { const m = p.t.match(/Job\[(.+)\]/); if (m) return shortType(m[1]); if (/Args$/.test(shortType(p.t))) return shortType(p.t); }
  return null;
}
// an endpoint, short: POST …/orders/{order_id}/kaspi-qr (the whole path in the tooltip)
const epText = (fid, full) => {
  const ep = (D.epByFid[fid] || [])[0]; if (!ep) return null;
  let p = ep.path || (ep.service ? ep.service + '.' : '') + (ep.name || '');
  if (!full && ep.path && p.length > 26) p = '…/' + p.split('/').filter(Boolean).slice(-2).join('/');
  return ((ep.method || ep.kind || '') + ' ' + p).trim();
};
// stepAction(s) → {short, full}: what step s does and what it passes
function stepAction(s) {
  if (!s) return { short: '', full: '' };
  if (s.planned) return { short: s.label, full: s.label };
  const mainOf = (full) => (s.kind === 'async' ? '→ ' + t('actQueue') + ': ' + (jobType(s.fid) || callText(s.fid))
    : s.kind === 'client' ? t('actClient') + ' → ' + (epText(s.fid, full) || callText(s.fid))
      : s.kind === 'entry' && epText(s.fid) ? epText(s.fid, full) + (full ? ' → ' + callText(s.fid) : '')
        : callText(s.fid));
  const main = mainOf(false), mainFull = mainOf(true);
  const more = [];
  (s.data || []).forEach((a) => { const op = /^(read|select|get|find|list)/i.test(a.op || '') ? 'SELECT' : String(a.op || 'write').toUpperCase(); more.push(op + ' ' + a.table); });
  const f = D.fns[s.fid];
  (s.exts || []).forEach((x) => {
    if (x.kind === 'db' || x.kind === 'queue') return;
    const c = ((f && f.callees) || []).find((id) => D.fns[id] && (x.modules || []).includes(D.fns[id].module));
    more.push('→ ' + x.name + (c ? ': ' + D.fns[c].name : ''));
  });
  const st = (s.states || [])[0];
  if (st) more.push(st.entity.replace(/Status$/, '') + ': ' + (st.from ? st.from + ' → ' : '→ ') + st.to);
  const uniq = Array.from(new Set(more));
  let short = main;
  for (const m of uniq) { if ((short + ' · ' + m).length > 46) break; short += ' · ' + m; }
  return { short: cut(short, 46), full: [mainFull].concat(uniq).join('\n') };
}
const actionAt = (i) => stepAction(stepAt(i));
// call stack up to step i: a callee is pushed, a return pops to its caller, an async hand-off starts anew
function framesUpTo(i) {
  const L_ = runList(), out = [];
  for (let j = 0; j <= i && j < L_.length; j++) {
    const s = L_[j].fid, p = j ? L_[j - 1].fid : null;
    if (isPlan(s)) continue; // a planned step has no frame
    if (p && isAsync(p, s)) { out.length = 0; out.push(s); continue; }
    let k = -1;
    for (let q = out.length - 1; q >= 0 && k < 0; q--) if (isCall(out[q], s)) k = q;
    if (k >= 0) out.splice(k + 1);
    else {
      // reached through one intermediate call: keep the caller, add the bridge frame
      let b = null;
      for (let q = out.length - 1; q >= 0 && !b; q--) { const c = bridges(out[q], s)[0]; if (c) { out.splice(q + 1); b = c; } }
      if (b) out.push(b); else out.length = 0;
    }
    if (out[out.length - 1] !== s) out.push(s);
  }
  return out;
}

/* ---------- step on the map levels ---------- */
function hotOn(nodes, id) { if (nodes[id]) nodes[id].el.classList.add('hot'); }
function hotEdge(edges, a, b) { edges.forEach((e) => { if ((e.a === a && e.b === b) || (e.a === b && e.b === a)) e.path.classList.add('hot'); }); }
function startPlay(svg, fx) { clearSel(svg); clearPlay(svg); fx.innerHTML = ''; svg.classList.add('play'); }
// the dot of a hop a → b: along the leg the path overlay drew (the line is already there, the dot only moves);
// without a drawn leg along its own invisible curve
const legDot = (sc, a, b, d, opt) => runDot(sc.fx, legD(sc, a, b) || d, Object.assign({ ghost: true }, opt));
async function sysHop(s) {
  startPlay(sys.svg, sys.fx);
  const N = sys.nodes, svc = N.svc.r;
  hotOn(N, 'svc');
  let moved = false;
  if (s.actor && N[s.actor.id]) { hotOn(N, s.actor.id); hotEdge(sys.edges, s.actor.id, 'svc'); await legDot(sys, s.actor.id, 'svc', hop(N[s.actor.id].r, svc, .08)); moved = true; }
  const q = D.externals.find((x) => x.kind === 'queue');
  if (s.kind === 'async' && q && N[q.id]) {
    hotOn(N, q.id); hotEdge(sys.edges, 'svc', q.id);
    await legDot(sys, 'svc', q.id, hop(svc, N[q.id].r, .08));
    await runDot(sys.fx, hop(N[q.id].r, svc, .08), { async: true, ghost: true });
    moved = true;
  }
  for (const x of s.exts.slice(0, 2)) {
    if (!N[x.id] || (s.kind === 'async' && x === q)) continue;
    hotOn(N, x.id); hotEdge(sys.edges, 'svc', x.id);
    await legDot(sys, 'svc', x.id, hop(svc, N[x.id].r, .08)); moved = true;
  }
  if (!moved) pulse(sys.fx, svc);
}
async function mapHop(s) {
  startPlay(map.svg, map.fx);
  const a = s.prev ? unitFor(D.fns[s.prev].module) : s.actor && map.nodes[s.actor.id] ? s.actor.id : null;
  const b = unitFor(s.mod);
  const ra = a && map.pos[a], rb = b && map.pos[b];
  if (!rb) return;
  const ext = s.exts.map((x) => x.id).filter((id) => map.pos[id])[0];
  const box = bbox([ra, rb, ext && map.pos[ext]]);
  if (!map.cam.contains(box, 20)) await map.cam.animTo(box, 160, 1100, RM ? 0 : 480); // only when the step is off screen
  hotOn(map.nodes, b); if (a) { hotOn(map.nodes, a); hotEdge(map.edges, a, b); }
  if (!a || a === b) pulse(map.fx, rb);
  else await legDot(map, a, b, hop(ra, rb), { async: s.kind === 'async' });
  if (ext) { hotOn(map.nodes, ext); hotEdge(map.edges, b, ext); await legDot(map, b, ext, hop(rb, map.pos[ext], .06)); }
}
async function fnHop(s) {
  startPlay(fv.svg, fv.fx);
  const at = fv.nodes[s.fid]; if (!at) return;
  at.el.classList.add('hot');
  let src = null;
  if (s.prev && fv.nodes[s.prev]) src = fv.nodes[s.prev];
  else if (s.prev) src = fv.ports['in:' + D.fns[s.prev].module] || fv.ports['out:' + D.fns[s.prev].module];
  const box = bbox([at.r, src && src.r]);
  if (!fv.cam.contains(box, 10)) await fv.cam.animTo(fv.extent, 10, 0, RM ? 0 : 300);
  if (!src || src === at) { pulse(fv.fx, at.r); }
  else {
    src.el.classList.add('hot');
    fv.edges.forEach((e) => { if ((e.a === s.prev || e.a === 'port:in:' + D.fns[s.prev].module) && e.b === s.fid) e.path.classList.add('hot'); });
    const sid = fv.nodes[s.prev] ? s.prev : 'port:in:' + D.fns[s.prev].module;
    await legDot(fv, sid, s.fid, hop(src.r, at.r, .1), { async: s.kind === 'async' });
  }
  const xm = s.exts.map((x) => (x.modules || []).find((m) => fv.ports['out:' + m])).filter(Boolean)[0];
  if (xm) { const p = fv.ports['out:' + xm]; p.el.classList.add('hot'); await runDot(fv.fx, hop(at.r, p.r, .06)); }
}
// a planned step: no function to show — light the module it is meant for (or the service) where it is, the level and
// the camera stay
function planHop(s) {
  if (state.level === 0 && sys) { startPlay(sys.svg, sys.fx); hotOn(sys.nodes, 'svc'); if (sys.nodes.svc) pulse(sys.fx, sys.nodes.svc.r); }
  else if (state.level === 1 && map) {
    startPlay(map.svg, map.fx);
    const u = s.mod && unitFor(s.mod);
    if (u && map.pos[u]) { hotOn(map.nodes, u); pulse(map.fx, map.pos[u]); }
  } else if (state.level === 2 && fv) startPlay(fv.svg, fv.fx);
}
async function showStep() {
  const s = curStep(); if (!s) return;
  state.played = true;
  renderPlayer(); setSel({ t: 'step' });
  bus.emit('step', s);
  if (state.view !== 'map') return;
  if (s.planned) { planHop(s); return drawPath(); }
  // the path moves to the step at once (the dot follows along the drawn leg); a level that rebuilds for the step
  // (another module) draws it after
  if (state.level < 2 || (state.level === 2 && fv && fv.mod === (unitFor(s.mod) || s.mod) && fv.nodes[s.fid])) drawPath();
  try { await hopTo(s); } finally { if (state.step === s.i) drawPath(); }
}
async function hopTo(s) {
  if (state.level === 0) await sysHop(s);
  else if (state.level === 1) await mapHop(s);
  else if (state.level === 2) {
    // the step is in another module: its functions replace the current ones in place with a short fade — no trip
    // up to the modules and back down
    const m = s.mod, u = unitFor(m) || m;
    if (!fv || fv.mod !== u) {
      revealMod(m);
      if (state.mod !== u) state.fnAll = false;
      state.mod = u; buildFns(u); fitFns(); renderCrumbs();
      const sc = $('#sc2');
      if (sc.animate && !RM) await sc.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 260, easing: 'ease-out' }).finished.catch(() => {});
    }
    if (fv && !fv.nodes[s.fid]) { buildFns(fv.mod); fitFns(); }
    await fnHop(s);
  } else if (state.level === 3) {
    state.fn = s.fid; state.mod = s.mod;
    buildCode(s.fid, s.next, framesUpTo(state.step)); renderCrumbs();
  }
}
async function doStep(delta = 1) {
  if (state.busy || !state.run) return false;
  if (delta > 0 && atBranch()) { showChoice(true); return false; }
  const n = state.step + delta, total = runList().length;
  if (n === -1 && delta < 0) { state.step = -1; trimSegs(); showOverview(); return true; } // back before step 1: the overview
  if (n < 0 || n >= total) return false;
  state.step = n;
  if (delta < 0) trimSegs();
  state.busy = true;
  try { await showStep(); } finally { state.busy = false; }
  return true;
}
// stepping back before a taken branch forgets it
function trimSegs() {
  state.exitText = null;
  const r = state.run, n0 = r.segs.length; let start = 0;
  // a fixed run (path, variant) keeps its flows; only the flow added by a jump from a variant is dropped
  for (let si = 0; si < r.segs.length; si++) { if (si > 0 && state.step < start && (!r.fixed || r.segs[si].extra)) { r.segs.length = si; break; } start += r.segs[si].steps.length; }
  if (r.segs.length !== n0) applyFocus(); // the dropped flow's participants leave the map
}
async function replay() { if (state.busy) return; state.busy = true; try { await showStep(); } finally { state.busy = false; } }
async function stepIn() {
  if (state.view !== 'map') setView('map');
  if (state.step < 0) { await doStep(1); return; }
  const s = curStep(); if (!s) return;
  if (s.planned) return doStep(1); // nothing to enter: no code yet
  if (state.level === 0) { await drill(1, { from: sys.nodes.svc.el, focus: s.mod }); return replay(); }
  if (state.level === 1) { revealMod(s.mod); const u = unitFor(s.mod); if (!u) return; await drill(2, { from: map.nodes[u].el, mod: u }); return replay(); }
  if (state.level === 2) return drill(3, { from: fv.nodes[s.fid] ? fv.nodes[s.fid].el : null, fn: s.fid, target: s.next, frames: framesUpTo(state.step) });
  return doStep(1);
}
async function stepOut() { if (state.level > 0) { await up(); if (state.step >= 0) replay(); } }
function showChoice(on) {
  const c = $('#choice'); c.classList.toggle('on', on); c.innerHTML = '';
  if (!on) return;
  H('span', { style: 'color:var(--muted);width:100%' }, c, esc(t('chooseBranch')));
  (branchesHere() || []).forEach((b) => {
    const target = b.flow && D.flowById[b.flow];
    const btn = H('button', { class: 'pbtn', title: (b.text || '') + (target ? ' → ' + target.name : '') }, c, (target ? '↪ ' : '⏹ ') + esc(cut(b.cond || b.text || '?', 28)));
    btn.onclick = () => takeBranch(b);
  });
}
function takeBranch(b) {
  showChoice(false);
  const target = b.flow && D.flowById[b.flow];
  if (target) {
    state.run.segs.push({ flow: target, steps: flowSteps(target), via: b });
    applyFocus(); // the new flow brings its own participants onto the map
    doStep(1);
  } else {
    state.exitText = b.text || b.cond;
    renderPlayer(); bus.emit('step', curStep()); drawPath();
  }
}
// back to the overview of the scenario (Home)
async function resetRun() {
  if (!state.run || state.busy) return;
  state.run = buildRun(state.run.key); state.step = -1; state.exitText = null;
  applyFocus();
  showOverview();
}
// the overview (step 0, state.step = -1): no step is lit, the whole path is drawn evenly, the panel says what the
// run does (panel.js runOverviewHTML), the bar offers "→ Start"
function showOverview() {
  state.exitText = null;
  [sys, map, fv].forEach((x) => { if (x) { clearPlay(x.svg); x.fx.innerHTML = ''; } });
  showChoice(false); renderPlayer(); setSel({ t: 'run' });
  bus.emit('step', null);
  drawPath();
}

/* ---------- the whole path on the current map level ----------
 * While a scenario is open its path is drawn over the level the user is on: every hop between two nodes of the
 * level is a leg with the numbers of its steps. The current leg is bright (accent, thick, one soft glow when it
 * appears), the passed ones are a calm accent, the future ones pale and dashed; nodes off the path are dimmed. A
 * click on a leg (or its number) goes to its step, a click on the node of the next step steps forward, a pale
 * branch leg at a fork takes that branch. Nothing moves by itself: the overlay is redrawn by ← / → only. */
addI18n({ ru: { pathStep: (k, s) => 'Шаг ' + k + ': ' + s, pathBranch: (c) => 'Ветка: ' + c }, en: { pathStep: (k, s) => 'Step ' + k + ': ' + s, pathBranch: (c) => 'Branch: ' + c } });
addI18n({ ru: { brMore: (n, p) => '+' + n + ' ' + p(['ветка', 'ветки', 'веток']), brFold: '− свернуть ветки' }, en: { brMore: (n) => '+' + n + (n === 1 ? ' branch' : ' branches'), brFold: '− fold the branches' } });
const PATH = { legs: [], open: new Set() }; // open: the "+N branches" marks unfolded ('end' or '<seg>:<fid>')
// node ids of step i on the given level (sc: sys | map | fv); null when the step is not on it
function pathNode(level, i) {
  const s = stepAt(i); if (!s || s.planned) return null;
  if (level === 0) return 'svc';
  if (level === 1) { const u = unitFor(s.mod); return u && map.nodes[u] ? u : null; }
  if (level === 2) return fv.nodes[s.fid] ? s.fid : null;
  return null;
}
// legs of the run on a level: [{a, b, steps: [i], kind}] (a, b — node ids of the scene, ports as 'port:in|out:<m>')
function pathLegs(level) {
  const L_ = runList(), legs = new Map();
  // a client call (the next flow of a path the code does not call) is a leg of its own: dashed, signed
  const add = (a, b, i, kind) => { if (!a || !b || a === b) return; const k = a + '\0' + b + (kind === 'client' ? '\0c' : ''); if (!legs.has(k)) legs.set(k, { a, b, steps: [], kind }); legs.get(k).steps.push(i); };
  for (let i = 0; i < L_.length; i++) {
    const s = stepAt(i); if (!s || s.planned) continue;
    if (level === 0) {
      if (s.actor && sys.nodes[s.actor.id]) add(s.actor.id, 'svc', i, s.kind === 'client' ? 'client' : 'entry');
      const q = D.externals.find((x) => x.kind === 'queue');
      if (s.kind === 'async' && q && sys.nodes[q.id]) add('svc', q.id, i, 'async');
      s.exts.slice(0, 2).forEach((x) => { if (sys.nodes[x.id] && !(s.kind === 'async' && x === q)) add('svc', x.id, i, 'ext'); });
    } else if (level === 1) {
      const b = pathNode(1, i);
      const a = s.prev && D.fns[s.prev] ? unitFor(D.fns[s.prev].module) : s.actor && map.nodes[s.actor.id] ? s.actor.id : null;
      if (b && a && map.nodes[a]) add(a, b, i, s.kind);
      // the outside system of the current step only (as its hop shows it): every step touches Postgres somewhere
      const xx = b && i === state.step ? s.exts.find((x) => map.nodes[x.id]) : null;
      if (xx) add(b, xx.id, i, 'ext');
    } else if (level === 2) {
      const b = fv.nodes[s.fid] ? s.fid : null, pm = s.prev && D.fns[s.prev] ? D.fns[s.prev].module : null;
      const a = s.prev && fv.nodes[s.prev] ? s.prev : pm && fv.ports['in:' + pm] ? 'port:in:' + pm : null;
      if (b && a) add(a, b, i, s.kind);
      else if (!b && a && fv.nodes[a] && fv.ports['out:' + s.mod]) add(a, 'port:out:' + s.mod, i, s.kind);
    }
  }
  return Array.from(legs.values());
}
const pathRect = (sc, id) => { const p = sc.nodes[id] || (sc.ports && sc.ports[String(id).replace(/^port:/, '')]); return p ? (p.r || (sc.pos && sc.pos[id])) : (sc.pos && sc.pos[id]) || null; };
// The overlay of a scene is built once and then only updated: a step changes classes, attributes and text of the
// elements already there (legs stay, a leg that is not on the path any more is hidden, not removed), so nothing
// blinks. sc.pv keeps them: {legs: Map key → leg elements, inside, gl/gi/gb (layers), brSig, step, glow}.
function pathLayer(sc) {
  if (!sc.pg || !sc.pg.isConnected) { sc.pg = S_('g', { class: 'pathov' }); sc.g.insertBefore(sc.pg, sc.fx); sc.pv = null; }
  if (!sc.pv) {
    sc.pg.textContent = '';
    sc.pv = { legs: new Map(), inside: null, brSig: '', step: null, glow: null };
    sc.pv.gl = S_('g', {}, sc.pg); sc.pv.gi = S_('g', {}, sc.pg); sc.pv.gb = S_('g', {}, sc.pg);
  }
  return sc.pg;
}
// set an attribute / the text only when it differs (null removes the attribute)
const setA = (e, k, v) => { v = v == null ? null : String(v); if (e.getAttribute(k) === v) return; if (v == null) e.removeAttribute(k); else e.setAttribute(k, v); };
const setT = (e, s) => { s = String(s); const n = e.firstChild; if (n && n.nodeType === 3 && !n.nextSibling) { if (n.data !== s) n.data = s; } else e.textContent = s; };
function wipePath(sc) {
  if (!sc) return;
  if (sc.pv && sc.pv.glow) sc.pv.glow.cancel();
  if (sc.pg && sc.pg.firstChild) sc.pg.textContent = '';
  sc.pv = null;
  sc.svg.classList.remove('pathmode');
  sc.svg.querySelectorAll('.pn').forEach((e) => e.classList.remove('pn', 'pn-done', 'pn-cur', 'pn-next'));
}
function clearPath() { [sys, map, fv].forEach(wipePath); PATH.legs = []; }
// the path d of the drawn leg a → b on a scene (the dot of a hop runs along it), null when it is not drawn
function legD(sc, a, b) {
  const pv = sc && sc.pv; if (!pv) return null;
  const o = pv.legs.get(a + '\0' + b) || pv.legs.get(a + '\0' + b + '\0c');
  return o && o.grp.getAttribute('display') !== 'none' ? o.v.getAttribute('d') : null;
}
// restack(parent, want): put the children listed in want in that order with the fewest moves — the longest run
// already in order stays, only the others move (the hidden children in between do not matter)
function restack(parent, want) {
  const pos = new Map(Array.from(parent.children).map((e, i) => [e, i])), seq = want.map((e) => pos.get(e));
  const tails = [], prev = new Array(seq.length).fill(-1); // longest increasing run of seq (indices into want)
  seq.forEach((v, i) => {
    let lo = 0, hi = tails.length;
    while (lo < hi) { const m = (lo + hi) >> 1; if (seq[tails[m]] < v) lo = m + 1; else hi = m; }
    if (lo > 0) prev[i] = tails[lo - 1];
    tails[lo] = i;
  });
  const keep = new Set();
  for (let i = tails.length ? tails[tails.length - 1] : -1; i >= 0; i = prev[i]) keep.add(want[i]);
  let next = null;
  for (let k = want.length - 1; k >= 0; k--) {
    const el = want[k];
    if (!keep.has(el)) parent.insertBefore(el, next);
    next = el;
  }
}
// the elements of one leg, made once: the line, a wide hit line, the number pill
function legEl(g) {
  const grp = S_('g', {}, g), v = S_('path', { class: 'v' }, grp), hit = S_('path', { class: 'hit' }, grp), hitT = S_('title', {}, hit, ' ');
  const num = S_('g', { class: 'pnum' }, grp), rect = S_('rect', {}, num), text = S_('text', { 'text-anchor': 'middle' }, num, ' '), numT = S_('title', {}, num, ' ');
  const o = { grp, v, hit, hitT, num, rect, text, numT, go: 0 };
  grp.addEventListener('click', (e) => { e.stopPropagation(); AM.play.go(o.go); });
  return o;
}
// what a step pill must not cover on a level: the band and group titles (their boxes in scene coordinates)
function pathObstacles(sc) {
  const out = [];
  sc.g.querySelectorAll('.band text, text.grp').forEach((e) => {
    try { const b = e.getBBox(); if (b.width) out.push({ x: b.x - 8, y: b.y - 6, w: b.width + 16, h: b.height + 12 }); } catch (er) { /* not rendered */ }
  });
  return out;
}
// the area a rect r shares with the rects of a list
function overArea(r, list) {
  let s = 0;
  list.forEach((q) => {
    const w = Math.min(r.x + r.w, q.x + q.w) - Math.max(r.x, q.x), h = Math.min(r.y + r.h, q.y + q.h) - Math.max(r.y, q.y);
    if (w > 0 && h > 0) s += w * h;
  });
  return s;
}
function drawPath() {
  if (!state.run || state.view !== 'map' || state.level > 2) { clearPath(); return; }
  const lvl = state.level, sc = [sys, map, fv][lvl]; if (!sc) return;
  [sys, map, fv].forEach((x) => { if (x && x !== sc) wipePath(x); });
  PATH.legs = [];
  pathLayer(sc);
  const pv = sc.pv, cur = state.step, ov = cur < 0, legs = pathLegs(lvl); // ov: the overview — every leg calm, none current
  // the numbers keep a readable size on screen when the level is zoomed out (fixed when drawn)
  const sw = sc.svg.getBoundingClientRect().width || 1000, zk = Math.max(1, Math.min(3, (sc.cam ? sc.cam.vb.w : sw) / sw));
  const zoomAround = (p) => (zk === 1 ? null : `translate(${p.x},${p.y}) scale(${zk}) translate(${-p.x},${-p.y})`);
  sc.svg.classList.add('pathmode');
  // the marks of the nodes are gathered first and put on at the end, only where they change
  const want = new Map(); // node element → 'cur' | 'done' | 'next' | '' (on the path, idle)
  const nodeEl = (id) => { const n = sc.nodes[id] || (sc.ports && sc.ports[String(id).replace(/^port:/, '')]); return n && n.el; };
  const markNode = (id, st) => { const el = nodeEl(id); if (!el) return; if (st === 'cur' || want.get(el) !== 'cur') want.set(el, st); };
  const plainNode = (id) => { const el = nodeEl(id); if (el && !want.has(el)) want.set(el, ''); };
  const pairs = new Set(legs.map((l) => l.a + '\0' + l.b));
  // future legs first, the current one last: on top
  const stOf = (l) => (ov ? 'done' : l.steps.includes(cur) ? 'cur' : l.steps.some((i) => i < cur) ? 'done' : 'next');
  const order = { next: 0, done: 1, cur: 2 };
  // the number pills keep clear of each other, of the band and group titles and of the cards: each pill slides
  // along its leg to the spot that overlaps least (the current leg is placed first, so it keeps the middle)
  const taken = [], hard = pathObstacles(sc), soft = Object.keys(sc.nodes || {}).map((id) => pathRect(sc, id)).filter(Boolean);
  // ws: the weight of covering a card (a long signed pill keeps off the cards harder)
  const placePill = (v, w, h, fallback, ws = 1) => {
    const len = v.getTotalLength ? v.getTotalLength() : 0;
    if (!len) return fallback;
    let best = null, bestS = Infinity;
    // a long signed pill may also sit beside its leg (above, below, to either side) when the leg is short
    const offs = [[0, 0]];
    if (ws > 1) [1, 1.8].forEach((m) => { const dy = m * (h / 2 + 4); [0, w / 2 - 20, -(w / 2 - 20)].forEach((dx) => offs.push([dx, -dy], [dx, dy])); });
    // and inside the part of the scene on screen
    const vb = ws > 1 && sc.cam ? [sc.cam.vb] : null;
    [0.5, 0.42, 0.58, 0.34, 0.66, 0.26, 0.74, 0.18, 0.82, 0.12, 0.88].forEach((f, k) => {
      const q = v.getPointAtLength(len * f);
      offs.forEach(([dx, dy], j) => {
        const p = { x: q.x + dx, y: q.y + dy }, r = { x: p.x - w / 2, y: p.y - h / 2, w, h };
        const s = overArea(r, taken) * 10 + overArea(r, hard) * 8 + overArea(r, soft) * ws + k * 2 + j * 12 + (vb ? (w * h - overArea(r, vb)) * 8 : 0);
        if (s < bestS) { bestS = s; best = p; }
      });
    });
    taken.push({ x: best.x - w / 2, y: best.y - h / 2, w, h });
    return best;
  };
  const drawn = [];
  // the number of a step that stays inside its node sits on the node's corner: keep the pills off it too
  const here0 = ov ? null : pathNode(lvl, cur);
  // its pill says what the step does too: "3 · validateOrderItems(Order)"
  const inAct = here0 ? actionAt(cur).short : '', inTxt = String(cur + 1) + (inAct ? ' · ' + inAct : ''), inW = inAct ? inTxt.length * 6.9 + 18 : 26;
  if (here0 && !legs.some((l) => l.steps.includes(cur))) { const r = pathRect(sc, here0); if (r) taken.push({ x: r.x + r.w - 16 * zk, y: r.y - 16 * zk, w: (inW + 6) * zk, h: 32 * zk }); }
  const seen = new Set();
  legs.sort((p, q) => order[stOf(q)] - order[stOf(p)]).forEach((l) => {
    const ra = pathRect(sc, l.a), rb = pathRect(sc, l.b); if (!ra || !rb) return;
    const st = stOf(l), back = pairs.has(l.b + '\0' + l.a);
    const cl = l.kind === 'client';
    const key = l.a + '\0' + l.b + (cl ? '\0c' : '');
    let o = pv.legs.get(key); if (!o) { o = legEl(pv.gl); pv.legs.set(key, o); }
    seen.add(key);
    const d = hop(ra, rb, back ? 0.22 : cl ? 0.32 : 0.1); // a client leg bends away from the entry leg it may share ends with
    setA(o.grp, 'display', null);
    const shown = ov ? Math.min(...l.steps) : st === 'cur' ? cur : st === 'done' ? Math.max(...l.steps.filter((i) => i <= cur)) : Math.min(...l.steps);
    // the tooltip: every step of the leg — what it does and what it passes (the code), then its text
    const tip = l.steps.map((i) => stepTip(i)).join('\n\n');
    setA(o.v, 'd', d); setA(o.v, 'marker-end', st === 'next' ? 'url(#arrM)' : 'url(#arrA)');
    setA(o.hit, 'd', d); setT(o.hitT, tip);
    // several steps on one leg: "13…17" (the list is in the tooltip)
    const nums = l.steps.map((i) => i + 1), txt0 = nums.length > 2 ? nums[0] + '…' + nums[nums.length - 1] : nums.join(',');
    // the current leg says what happens: "3 · requestQR(Payment) → QRResult"; a client call of another leg is
    // signed short ("client")
    const big = st === 'cur', act = big ? actionAt(cur).short : '', lbl = !!act;
    const txt = big ? String(cur + 1) + (lbl ? ' · ' + act : cl ? ' · ' + t('jvClientShort') : '') : txt0 + (cl ? ' · ' + t('jvClientShort') : '');
    const w = Math.max(big ? 26 : 20, txt.length * (lbl ? 6.9 : cl ? (big ? 8.2 : 6.4) : big ? 9 : 7) + (lbl ? 18 : 10)), h = big ? 26 : 20;
    setA(o.grp, 'class', 'pleg ' + st + (l.kind === 'async' ? ' async' : '') + (cl ? ' client' : '') + (lbl ? ' lbl' : ''));
    const mid = placePill(o.v, w * zk, h * zk, ctr(ra), lbl ? 2 : 1);
    setA(o.num, 'transform', zoomAround(mid));
    setA(o.rect, 'x', mid.x - w / 2); setA(o.rect, 'y', mid.y - h / 2); setA(o.rect, 'width', w); setA(o.rect, 'height', h); setA(o.rect, 'rx', h / 2);
    setA(o.text, 'x', mid.x); setA(o.text, 'y', mid.y + (big && !lbl ? 5 : 4)); setT(o.text, txt);
    setT(o.numT, cl ? t('jvClient') + '\n' + tip : tip);
    o.go = st === 'cur' ? cur : shown;
    markNode(l.a, st === 'next' ? 'next' : 'done'); markNode(l.b, st);
    PATH.legs.push({ a: l.a, b: l.b, steps: l.steps, st });
    drawn.push({ o, st });
  });
  // a leg that is not on the path now is hidden, not removed: it comes back as it was
  pv.legs.forEach((o, k) => { if (!seen.has(k)) setA(o.grp, 'display', 'none'); });
  // the pills were placed current-first; the legs are stacked future-first so the current one is on top — only the
  // legs out of place move (one or two a step)
  restack(pv.gl, drawn.sort((p, q) => order[p.st] - order[q.st]).map((x) => x.o.grp));
  // the node of the current step even when no leg reaches it on this level (the first step, a step inside a module)
  // the overview keeps the outside systems of every step in view (a step shows only its own one — pathLegs)
  // on a step the outside systems of the other steps stay plain (pn only): idle, not faded
  if (lvl === 1) for (let i = 0; i < runList().length; i++) {
    const s = stepAt(i); if (!s || s.planned) continue;
    s.exts.forEach((x) => { if (ov) markNode(x.id, 'done'); else plainNode(x.id); });
  }
  const here = ov ? null : pathNode(lvl, cur); if (here) markNode(here, 'cur');
  // the step stays inside the node (a call within one module): its number sits on the node
  const ir = here && !legs.some((l) => l.steps.includes(cur)) ? pathRect(sc, here) : null;
  if (ir) {
    let o = pv.inside;
    if (!o) {
      const grp = S_('g', { class: 'pleg cur inside' }, pv.gi), num = S_('g', { class: 'pnum' }, grp);
      o = pv.inside = { grp, num, rect: S_('rect', { height: 26, rx: 13 }, num), text: S_('text', { 'text-anchor': 'middle' }, num, ' '), tt: S_('title', {}, grp, ' ') };
    }
    setA(o.grp, 'display', null); setA(o.grp, 'class', 'pleg cur inside' + (inAct ? ' lbl' : ''));
    setA(o.num, 'transform', zoomAround({ x: ir.x + ir.w - 3, y: ir.y }));
    setA(o.rect, 'x', ir.x + ir.w - 16); setA(o.rect, 'y', ir.y - 13); setA(o.rect, 'width', inW);
    setA(o.text, 'x', ir.x + ir.w - 16 + inW / 2); setA(o.text, 'y', ir.y + (inAct ? 4 : 5)); setT(o.text, inTxt);
    setT(o.tt, stepTip(cur));
  } else if (pv.inside) setA(pv.inside.grp, 'display', 'none');
  if (lvl === 0 && sc.nodes.svc) markNode('svc', ov ? 'done' : 'cur');
  drawBranches(sc, pv.gb, lvl, here, zoomAround, zk);
  // node marks: only the ones that change
  sc.svg.querySelectorAll('.pn').forEach((el) => { if (!want.has(el)) el.classList.remove('pn', 'pn-done', 'pn-cur', 'pn-next'); });
  want.forEach((st, el) => { el.classList.add('pn'); ['done', 'cur', 'next'].forEach((k) => el.classList.toggle('pn-' + k, st === k)); });
  // one soft glow of the current leg, only when the step really changed; a new step cancels the glow still running
  if (pv.step !== cur) {
    if (pv.glow) { pv.glow.cancel(); pv.glow = null; }
    const c = drawn.find((x) => x.st === 'cur');
    if (!ov && c && !RM && c.o.v.animate) pv.glow = c.o.v.animate([{ strokeWidth: '7px', opacity: 0.6 }, { strokeWidth: '3px', opacity: 1 }], { duration: 900, easing: 'ease-out' });
    pv.step = cur;
  }
}
// branches at the current step. At the fork that ends a flow run the branches that go on in another flow are pale
// legs / pills, a click takes one; the outcomes that end the story (branch.fold) are folded into one "+N branches"
// mark next to the node, a click unfolds them (and folds them back). On any other step (and in a path) the forks
// of the step are only that mark: unfolded, their pills say what happens, the path goes on as written.
// They are redrawn only when what they show changes (sc.pv.brSig): the same forks stay as they are.
function drawBranches(sc, g, lvl, here, zoomAround, zk) {
  const s = here ? curStep() : null, ra = s && pathRect(sc, here);
  const all = s && state.step === runList().length - 1 && !state.exitText ? allBranchesHere() : null;
  const key = !s ? '' : all ? 'end' : s.seg + ':' + s.fid;
  const shown = all ? all.filter((b) => !b.fold) : [];
  const folded = !s ? [] : all ? all.filter((b) => b.fold) : uniqBr(((s.flow && s.flow.branches) || []).filter((b) => b.at === s.fid));
  const open = PATH.open.has(key), list = open ? shown.concat(folded) : shown;
  const bk = (b) => (b.cond || '') + '\u0001' + (b.text || '') + '\u0001' + (b.flow || '');
  const sig = !ra || (!list.length && !folded.length) ? '' : JSON.stringify([key, open, !!all, here, lvl, zk, ra.x, ra.y, ra.w, ra.h, list.map(bk), folded.map(bk)]);
  if (sig === sc.pv.brSig) return;
  sc.pv.brSig = sig;
  if (g.firstChild) g.textContent = '';
  if (!sig) return;
  let row = 0;
  list.forEach((b, k) => {
    const live = !!all; // only the fork at the end of a flow run can be taken
    const tf = b.flow && D.flowById[b.flow], first = tf && flowSteps(tf).find((f) => D.fns[f]);
    let to = null;
    if (first && lvl === 1) to = unitFor(D.fns[first].module);
    else if (first && lvl === 2) to = fv.nodes[first] ? first : fv.ports['out:' + D.fns[first].module] ? 'port:out:' + D.fns[first].module : null;
    const rb = to && pathRect(sc, to), leg = rb && to !== here;
    const grp = S_('g', { class: 'pleg br' + (live ? '' : ' end') }, g), lab = cut(b.cond || b.text || '?', 22);
    let mid;
    if (leg) {
      const d = hop(ra, rb, 0.18 + k * 0.08);
      const v = S_('path', { d, class: 'v', 'marker-end': 'url(#arrM)' }, grp); S_('path', { d, class: 'hit' }, grp);
      mid = v.getPointAtLength(v.getTotalLength() / 2);
    } else mid = { x: ra.x + ra.w + 16, y: ra.y + 12 + row++ * 26 }; // the branch ends here: a pill next to the node
    const num = S_('g', { class: 'pnum', transform: zoomAround(mid) }, grp), w = lab.length * 6.6 + 18;
    S_('rect', { x: mid.x - (leg ? w / 2 : 0), y: mid.y - 10, width: w, height: 20, rx: 10 }, num);
    S_('text', { x: mid.x + (leg ? 0 : w / 2), y: mid.y + 4, 'text-anchor': 'middle' }, num, (tf ? '↪ ' : '⏹ ') + lab);
    S_('title', {}, grp, t('pathBranch', (b.cond || '') + (b.text ? ' — ' + b.text : '') + (tf ? ' → ' + tf.name : '')));
    grp.addEventListener('click', (e) => { e.stopPropagation(); if (live) takeBranch(b); });
  });
  if (!folded.length) return;
  const lab = open ? t('brFold') : t('brMore', folded.length), w = lab.length * 6.4 + 18;
  const at = { x: ra.x + ra.w + 16, y: ra.y + 12 + row * 26 };
  const grp = S_('g', { class: 'pleg br fold' }, g), num = S_('g', { class: 'pnum', transform: zoomAround(at) }, grp);
  S_('rect', { x: at.x, y: at.y - 10, width: w, height: 20, rx: 10 }, num);
  S_('text', { x: at.x + w / 2, y: at.y + 4, 'text-anchor': 'middle' }, num, lab);
  S_('title', {}, grp, folded.map((b) => (b.cond || '') + (b.text ? ' — ' + b.text : '')).join('\n'));
  grp.addEventListener('click', (e) => { e.stopPropagation(); if (PATH.open.has(key)) PATH.open.delete(key); else PATH.open.add(key); renderPlayer(); drawPath(); });
}
// a click on the map while a scenario is open: the node of the next step steps forward; true when it was taken
function pathClick(e) {
  if (!state.run || state.busy || state.level > 2) return false;
  const n = e.target.closest && e.target.closest('.n'); if (!n) return false;
  const id = n.getAttribute('data-id'), nx = state.step + 1;
  if (nx >= runList().length) return false;
  const want = pathNode(state.level, nx);
  if (!want || want !== id || want === pathNode(state.level, state.step)) return false;
  doStep(1);
  return true;
}

/* ---------- badges ---------- */
const OPW = { read: 'READ', select: 'READ' };
function effBadges(s) {
  const out = [];
  (s.effects || []).forEach((e) => out.push({ op: /^\s*(select|read)/i.test(e) ? 'READ' : 'WRITE', text: e }));
  if (!out.length) (s.data || []).forEach((a) => out.push({ op: OPW[a.op] || 'WRITE', text: (a.op || '').toUpperCase() + ' ' + a.table + (a.fields && a.fields.length ? ' (' + a.fields.slice(0, 3).join(', ') + (a.fields.length > 3 ? ', …' : '') + ')' : '') }));
  const seen = new Set();
  return out.filter((b) => (seen.has(b.text) ? false : seen.add(b.text))).slice(0, 2);
}
const mutBadge = (b) => `<span class="mut" title="${esc(b.text)}"><b>${b.op}</b>${esc(b.text)}</span>`;
function stTone(s) {
  if (/cancel|fail|reject|error|denied|block/i.test(s)) return 'critical';
  if (/expire|timeout|stale/i.test(s)) return 'serious';
  if (/pending|request|wait|scan|progress|process(ing)?$|created_?qr|queued/i.test(s)) return 'warning';
  if (/done|success|paid|accept|active|complete|created|processed|ok$|delivered|confirmed/i.test(s)) return 'good';
  return 'neutral';
}
function statusPill(st) {
  const ic = { good: ['✓', 'var(--good)'], warning: ['⏳', 'var(--warning)'], critical: ['✕', 'var(--critical)'], serious: ['⌛', 'var(--serious)'], neutral: ['•', 'var(--muted)'] }[stTone(st.to)];
  return `<span class="stat" title="${esc(st.entity + ': ' + (st.from || '?') + ' → ' + st.to)}"><span class="ic" style="background:${ic[1]}">${ic[0]}</span>${esc(st.to)}<small>${esc(st.entity.replace(/Status$/, ''))}</small></span>`;
}

/* ---------- run bar ---------- */
// in the centre of the toolbar, only while a scenario is open: "<name>  Overview · ← Step k/N →  ✕"; the step itself
// is in the inspector; forks, the plate between flows and the outcome go to the strip on top of the inspector (#choice)
function renderPlayer() {
  const r = state.run, bar = $('#player');
  bar.hidden = !r;
  if (typeof paintHName === 'function') paintHName();
  if (!r) { showChoice(false); jvPaintTags(); return; }
  const list = runList(), s = curStep(), br = atBranch(), fe = r.fixed ? jvEnd() : null;
  $('#scName').textContent = r.name; $('#scName').title = r.name + (r.desc ? '\n' + r.desc : '');
  const ov = state.step < 0; // the overview: "Overview · N steps  Start →"
  $('#pOv').textContent = t('ovOverview'); $('#pOv').setAttribute('aria-pressed', String(ov)); $('#pOv').title = t('ovOverview') + ' (Home)';
  // "Step 8 of 25 · <the step text>": a path is one story numbered through; its flow (the chapter) small in front
  const cap = $('#capNo'), ch = s && r.fixed ? jvChapter() : '';
  if (ov || !s) cap.textContent = t('pkStepsN', list.length);
  else cap.innerHTML = (ch ? `<small class="ch">${esc(cut(ch, 30))} ·</small> ` : '') + esc(t('stepOf', state.step + 1, list.length)) + (s.label ? ` · <span class="sl">${esc(cut(s.label, 40))}</span>` : '');
  cap.title = s ? (ch ? ch + '\n' : '') + s.label + '\n' + actionAt(state.step).full : list.length ? '' : t('noPlay');
  $('#pPrev').disabled = ov;
  $('#pStep').textContent = ov && list.length ? t('ovStart') + ' →' : '→';
  $('#pStep').classList.toggle('primary', ov && !!list.length);
  $('#pStep').disabled = !list.length || (state.step >= list.length - 1 && !br && !(fe && fe.jump));
  $('#pPrev').title = t('back') + ' (←)'; $('#pStep').title = t('step') + ' (→)'; // "‹ <where Esc goes>": the page, the scenario step, the module the scenario was opened from; the map by default
  const bk = typeof navBackLabel === 'function' ? navBackLabel() : '';
  $('#pClose').textContent = '‹ ' + cut(bk || t('backToMap'), 40); $('#pClose').title = bk ? t('navBack', bk) : t('closeRun') + ' (Esc)';
  showChoice(br);
  if (!br) showPlate();
  if (!br && state.exitText && state.step === list.length - 1) {
    const c = $('#choice'); c.classList.add('on');
    H('span', { class: 'jv-plate' }, c, esc('→ ' + t('exit') + ': ' + cut(state.exitText, 60)));
  }
  jvPaintTags();
}
// the plate between flows of a path / the end of a variant, on top of the inspector
function showPlate() {
  const p = jvPlate(); if (!p) return;
  const c = $('#choice'); c.classList.add('on');
  H('span', { class: 'jv-plate ' + p.tone, title: p.text }, c, (p.kind ? jvIcon(p.kind) : '') + esc(cut(p.text, 60)));
  if (p.jump) { const b = H('button', { class: 'pbtn', title: t('jvGoTip') }, c, '↪ ' + esc(cut(t('jvGo'), 30))); b.onclick = () => jvJump(); }
}
// the tooltip of a step number: "Step k: <what it does>", the rest of what it does, then the step text
function stepTip(i) {
  const lines = actionAt(i).full.split('\n').filter(Boolean), lab = stepLabel(i);
  return t('pathStep', i + 1, lines[0] || lab) + (lines.length > 1 ? '\n' + lines.slice(1).join('\n') : '') + (lab && lines.length && lab !== lines[0] ? '\n' + lab : '');
}
function stepLabel(i) { const it = runList()[i]; if (!it) return ''; const fl = state.run.segs[it.seg].flow; return (fl && fl.labels && fl.labels[it.fid]) || fnInfo(it.fid).name; }

// loadRun(key): 'flow:<i>' (index in model.flows), 'journey:<id>', 'variant:<id>', 'fn:<fid>', 'changes', 'overview';
// null closes the scenario. Nothing plays by itself: the steps go by → / ← only.
function loadRun(key) {
  state.run = buildRun(key); state.step = -1; state.played = false; state.exitText = null;
  PATH.open.clear();
  [sys, map, fv].forEach((x) => { if (x) { clearPlay(x.svg); x.fx.innerHTML = ''; } });
  clearPath();
  // the map of the scenario: only its participants (map.js applyFocus); closing brings the full map back
  if (!applyFocus(!!key) && fv && state.level === 2) { buildFns(fv.mod); fitFns(); }
  showChoice(false); renderPlayer(); renderCrumbs();
  bus.emit('flow', state.run); bus.emit('step', null);
}
// openRun(key): open a scenario on its overview (what it does, the whole path on the map); the steps by hand
// Opening is a step down (nav.js): Esc comes back to where it was opened from; the diagram zooms in from that place.
async function openRun(key) {
  const down = !BACK.restoring && (!state.run || state.run.key !== key);
  if (down) navPush();
  const from = down ? navFrom() : null;
  if (state.view !== 'map') setView('map');
  for (let i = 0; i < 40 && state.busy; i++) await sleep(100);
  state.navZoom = down;
  try { loadRun(key); } finally { state.navZoom = false; }
  if (state.run) showOverview();
  if (down && state.run) navZoom(SC()[state.level], from, 'in');
}
function closeRun() {
  if (!state.run) return;
  loadRun(null);
  setSel(null);
}
function playFrom(fid) {
  const fl = D.flows.find((x) => x.steps[0] === fid);
  return openScenario(fl ? 'flow:' + fl.i : 'fn:' + fid);
}

// previewRun(key): a run and every step object of it without opening it (state.run is left as it is) — for the
// diagrams drawn elsewhere: the journey / flow frames of a page (views/pages.js). → {run, steps} or null
function previewRun(key) {
  const r = buildRun(key); if (!r) return null;
  const keep = [state.run, state.step];
  state.run = r;
  try {
    const n = runList().length, steps = [];
    for (let i = 0; i < n; i++) steps.push(stepAt(i));
    return { run: r, steps };
  } finally { state.run = keep[0]; state.step = keep[1]; }
}

Object.assign(AM, {
  play: {
    preview: previewRun, action: stepAction,
    load: loadRun, open: openRun, close: closeRun, step: doStep, stepIn, stepOut, reset: resetRun, playFrom,
    go: (i) => { if (state.busy || !state.run) return; state.step = Math.max(0, Math.min(runList().length - 1, i)); trimSegs(); return replay(); },
    choose: (b) => takeBranch(b), branches: () => (atBranch() ? branchesHere() : null),
    current: curStep, stepAt, list: runList, frames: framesUpTo,
    get run() { return state.run; },
  },
  ui: { statusPill, mutBadge, effBadges, stTone },
});
