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
  ru: { planStep: 'запланированный шаг', planTag: 'план', planNoCode: 'Шаг запланирован в описании (arch/*.arch): кода ещё нет.', planIn: 'модуль', stepOf: (k, n) => 'Шаг ' + k + '/' + n, closeRun: 'Закрыть сценарий, вернуться к карте', backToMap: 'Карта' },
  en: { planStep: 'planned step', planTag: 'plan', planNoCode: 'The step is planned in the description (arch/*.arch): no code yet.', planIn: 'module', stepOf: (k, n) => 'Step ' + k + '/' + n, closeRun: 'Close the scenario, back to the map', backToMap: 'Map' },
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
async function sysHop(s) {
  startPlay(sys.svg, sys.fx);
  const N = sys.nodes, svc = N.svc.r;
  hotOn(N, 'svc');
  let moved = false;
  if (s.actor && N[s.actor.id]) { hotOn(N, s.actor.id); hotEdge(sys.edges, s.actor.id, 'svc'); await runDot(sys.fx, hop(N[s.actor.id].r, svc, .08)); moved = true; }
  const q = D.externals.find((x) => x.kind === 'queue');
  if (s.kind === 'async' && q && N[q.id]) {
    hotOn(N, q.id); hotEdge(sys.edges, 'svc', q.id);
    await runDot(sys.fx, hop(svc, N[q.id].r, .08));
    await runDot(sys.fx, hop(N[q.id].r, svc, .08), { async: true });
    moved = true;
  }
  for (const x of s.exts.slice(0, 2)) {
    if (!N[x.id] || (s.kind === 'async' && x === q)) continue;
    hotOn(N, x.id); hotEdge(sys.edges, 'svc', x.id);
    await runDot(sys.fx, hop(svc, N[x.id].r, .08)); moved = true;
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
  else await runDot(map.fx, hop(ra, rb), { async: s.kind === 'async' });
  if (ext) { hotOn(map.nodes, ext); hotEdge(map.edges, b, ext); await runDot(map.fx, hop(rb, map.pos[ext], .06)); }
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
    await runDot(fv.fx, hop(src.r, at.r, .1), { async: s.kind === 'async' });
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
function pathLayer(sc) {
  if (!sc.pg || !sc.pg.isConnected) { sc.pg = S_('g', { class: 'pathov' }); sc.g.insertBefore(sc.pg, sc.fx); }
  return sc.pg;
}
function clearPath() {
  [sys, map, fv].forEach((sc) => { if (!sc) return; if (sc.pg) sc.pg.innerHTML = ''; sc.svg.classList.remove('pathmode'); sc.svg.querySelectorAll('.pn').forEach((e) => e.classList.remove('pn', 'pn-done', 'pn-cur', 'pn-next')); });
  PATH.legs = [];
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
  clearPath();
  if (!state.run || state.view !== 'map' || state.level > 2) return;
  const lvl = state.level, sc = [sys, map, fv][lvl]; if (!sc) return;
  const g = pathLayer(sc), cur = state.step, ov = cur < 0, legs = pathLegs(lvl); // ov: the overview — every leg calm, none current
  // the numbers keep a readable size on screen when the level is zoomed out (fixed when drawn)
  const sw = sc.svg.getBoundingClientRect().width || 1000, zk = Math.max(1, Math.min(3, (sc.cam ? sc.cam.vb.w : sw) / sw));
  const zoomAround = (p) => (zk === 1 ? null : `translate(${p.x},${p.y}) scale(${zk}) translate(${-p.x},${-p.y})`);
  sc.svg.classList.add('pathmode');
  const markNode = (id, st) => { const n = sc.nodes[id] || (sc.ports && sc.ports[String(id).replace(/^port:/, '')]); if (!n || !n.el) return; n.el.classList.add('pn'); if (st === 'cur' || !n.el.classList.contains('pn-cur')) { n.el.classList.remove('pn-done', 'pn-next'); n.el.classList.add('pn-' + st); } };
  const pairs = new Set(legs.map((l) => l.a + '\0' + l.b));
  // future legs first, the current one last: on top
  const stOf = (l) => (ov ? 'done' : l.steps.includes(cur) ? 'cur' : l.steps.some((i) => i < cur) ? 'done' : 'next');
  const order = { next: 0, done: 1, cur: 2 };
  // the number pills keep clear of each other, of the band and group titles and of the cards: each pill slides
  // along its leg to the spot that overlaps least (the current leg is placed first, so it keeps the middle)
  const taken = [], hard = pathObstacles(sc), soft = Object.keys(sc.nodes || {}).map((id) => pathRect(sc, id)).filter(Boolean);
  const placePill = (v, w, h, fallback) => {
    const len = v.getTotalLength ? v.getTotalLength() : 0;
    if (!len) return fallback;
    let best = null, bestS = Infinity;
    [0.5, 0.42, 0.58, 0.34, 0.66, 0.26, 0.74, 0.18, 0.82, 0.12, 0.88].forEach((f, k) => {
      const p = v.getPointAtLength(len * f), r = { x: p.x - w / 2, y: p.y - h / 2, w, h };
      const s = overArea(r, taken) * 10 + overArea(r, hard) * 8 + overArea(r, soft) + k * 2;
      if (s < bestS) { bestS = s; best = p; }
    });
    taken.push({ x: best.x - w / 2, y: best.y - h / 2, w, h });
    return best;
  };
  const drawn = [];
  // the number of a step that stays inside its node sits on the node's corner: keep the pills off it too
  const here0 = ov ? null : pathNode(lvl, cur);
  if (here0 && !legs.some((l) => l.steps.includes(cur))) { const r = pathRect(sc, here0); if (r) taken.push({ x: r.x + r.w - 3 - 16 * zk, y: r.y - 16 * zk, w: 32 * zk, h: 32 * zk }); }
  legs.sort((p, q) => order[stOf(q)] - order[stOf(p)]).forEach((l) => {
    const ra = pathRect(sc, l.a), rb = pathRect(sc, l.b); if (!ra || !rb) return;
    const st = stOf(l), back = pairs.has(l.b + '\0' + l.a);
    const cl = l.kind === 'client';
    const d = hop(ra, rb, back ? 0.22 : cl ? 0.32 : 0.1); // a client leg bends away from the entry leg it may share ends with
    const grp = S_('g', { class: 'pleg ' + st + (l.kind === 'async' ? ' async' : '') + (cl ? ' client' : '') }, g);
    const shown = ov ? Math.min(...l.steps) : st === 'cur' ? cur : st === 'done' ? Math.max(...l.steps.filter((i) => i <= cur)) : Math.min(...l.steps);
    const tip = l.steps.map((i) => t('pathStep', i + 1, stepLabel(i))).join('\n');
    const v = S_('path', { d, class: 'v', 'marker-end': st === 'next' ? 'url(#arrM)' : 'url(#arrA)' }, grp);
    const hit = S_('path', { d, class: 'hit' }, grp);
    S_('title', {}, hit, tip);
    const nums = l.steps.map((i) => i + 1), txt0 = nums.length > 3 ? nums[0] + '…' + nums[nums.length - 1] : nums.join(',');
    // a client call is signed short ("client"); the full "next the client calls" is in the tooltip
    const big = st === 'cur', txt = (big ? String(cur + 1) : txt0) + (cl ? ' · ' + t('jvClientShort') : '');
    const w = Math.max(big ? 26 : 20, txt.length * (cl ? (big ? 8.2 : 6.4) : big ? 9 : 7) + 10), h = big ? 26 : 20;
    const mid = placePill(v, w * zk, h * zk, ctr(ra));
    const num = S_('g', { class: 'pnum', transform: zoomAround(mid) }, grp);
    S_('rect', { x: mid.x - w / 2, y: mid.y - h / 2, width: w, height: h, rx: h / 2 }, num);
    S_('text', { x: mid.x, y: mid.y + (big ? 5 : 4), 'text-anchor': 'middle' }, num, txt);
    S_('title', {}, num, cl ? t('jvClient') + '\n' + tip : tip);
    grp.addEventListener('click', (e) => { e.stopPropagation(); AM.play.go(st === 'cur' ? cur : shown); });
    markNode(l.a, st === 'next' ? 'next' : 'done'); markNode(l.b, st);
    PATH.legs.push({ a: l.a, b: l.b, steps: l.steps, st });
    drawn.push({ grp, st });
  });
  // the pills were placed current-first; the legs are stacked future-first so the current one is on top
  drawn.sort((p, q) => order[p.st] - order[q.st]).forEach((x) => g.appendChild(x.grp));
  // the node of the current step even when no leg reaches it on this level (the first step, a step inside a module)
  // the overview keeps the outside systems of every step in view (a step shows only its own one — pathLegs)
  // on a step the outside systems of the other steps stay plain (pn only): idle, not faded
  if (lvl === 1) for (let i = 0; i < runList().length; i++) {
    const s = stepAt(i); if (!s || s.planned) continue;
    s.exts.forEach((x) => { if (ov) markNode(x.id, 'done'); else { const n = sc.nodes[x.id]; if (n && n.el) n.el.classList.add('pn'); } });
  }
  const here = ov ? null : pathNode(lvl, cur); if (here) markNode(here, 'cur');
  // the step stays inside the node (a call within one module): its number sits on the node
  if (here && !legs.some((l) => l.steps.includes(cur))) {
    const r = pathRect(sc, here);
    if (r) {
      const grp = S_('g', { class: 'pleg cur inside' }, g), num = S_('g', { class: 'pnum', transform: zoomAround({ x: r.x + r.w - 3, y: r.y }) }, grp);
      S_('rect', { x: r.x + r.w - 16, y: r.y - 13, width: 26, height: 26, rx: 13 }, num);
      S_('text', { x: r.x + r.w - 3, y: r.y + 5, 'text-anchor': 'middle' }, num, String(cur + 1));
      S_('title', {}, grp, t('pathStep', cur + 1, stepLabel(cur)));
    }
  }
  if (lvl === 0 && sc.nodes.svc) markNode('svc', ov ? 'done' : 'cur');
  if (here) drawBranches(sc, g, lvl, here, zoomAround);
  // the glow of the current leg plays once, when it appears
  g.querySelectorAll('.pleg.cur').forEach((x) => x.classList.add('flash'));
}
// branches at the current step. At the fork that ends a flow run the branches that go on in another flow are pale
// legs / pills, a click takes one; the outcomes that end the story (branch.fold) are folded into one "+N branches"
// mark next to the node, a click unfolds them (and folds them back). On any other step (and in a path) the forks
// of the step are only that mark: unfolded, their pills say what happens, the path goes on as written.
function drawBranches(sc, g, lvl, here, zoomAround) {
  const s = curStep(); if (!s) return;
  const all = state.step === runList().length - 1 && !state.exitText ? allBranchesHere() : null;
  const key = all ? 'end' : s.seg + ':' + s.fid;
  const shown = all ? all.filter((b) => !b.fold) : [];
  const folded = all ? all.filter((b) => b.fold) : uniqBr(((s.flow && s.flow.branches) || []).filter((b) => b.at === s.fid));
  const open = PATH.open.has(key), list = open ? shown.concat(folded) : shown;
  const ra = pathRect(sc, here); if (!ra) return;
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
  grp.addEventListener('click', (e) => { e.stopPropagation(); if (open) PATH.open.delete(key); else PATH.open.add(key); renderPlayer(); drawPath(); });
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
  $('#capNo').textContent = ov ? t('pkStepsN', list.length) : r.fixed && r.segs.length > 1 && s ? jvProgress() : t('stepOf', state.step + 1, list.length);
  $('#capNo').title = s ? s.label + (D.fns[s.fid] ? '\n' + fnInfo(s.fid).name : '') : list.length ? '' : t('noPlay');
  $('#pPrev').disabled = ov;
  $('#pStep').textContent = ov && list.length ? t('ovStart') + ' →' : '→';
  $('#pStep').classList.toggle('primary', ov && !!list.length);
  $('#pStep').disabled = !list.length || (state.step >= list.length - 1 && !br && !(fe && fe.jump));
  $('#pPrev').title = t('back') + ' (←)'; $('#pStep').title = t('step') + ' (→)'; $('#pClose').textContent = '‹ ' + t('backToMap'); $('#pClose').title = t('closeRun') + ' (Esc)';
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
async function openRun(key) {
  if (state.view !== 'map') setView('map');
  for (let i = 0; i < 40 && state.busy; i++) await sleep(100);
  loadRun(key);
  if (state.run) showOverview();
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

Object.assign(AM, {
  play: {
    load: loadRun, open: openRun, close: closeRun, step: doStep, stepIn, stepOut, reset: resetRun, playFrom,
    go: (i) => { if (state.busy || !state.run) return; state.step = Math.max(0, Math.min(runList().length - 1, i)); trimSegs(); return replay(); },
    choose: (b) => takeBranch(b), branches: () => (atBranch() ? branchesHere() : null),
    current: curStep, stepAt, list: runList, frames: framesUpTo,
    get run() { return state.run; },
  },
  ui: { statusPill, mutBadge, effBadges, stTone },
});
