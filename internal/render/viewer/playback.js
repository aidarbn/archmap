/* playback.js — scenarios: picker (combobox with search and groups), run of steps through the map
 * levels, branches (continue in another flow or exit), effects and status badges, auto play.
 *
 * A run is {key, name, desc, actor, exit, segs: [{flow|null, steps: [fid]}]}; segs grow when a branch
 * continues in another flow. A step object (AM.play.current()) is:
 *   {i, fid, prev, next, seg, flow, kind: 'entry'|'call'|'async'|'next', label, effects: [string],
 *    data: [{table, op, fields}], states: [{entity, from, to, fid}], exts: [external], actor, mod}
 */

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
    return { key, name: fl.name, desc: fl.desc || '', segs: [{ flow: fl, steps: fl.steps.filter((s) => D.fns[s]) }] };
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
const branchesHere = () => { const sg = lastSeg(); return state.run && !state.run.fixed && sg && sg.flow && (sg.flow.branches || []).length ? sg.flow.branches : null; };
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
  // the first step of a further flow of a path: a new entry (a queue hand-off keeps its caller)
  const first = i > 0 && it.seg > 0 && L_[i - 1].seg !== it.seg, viaQ = first && sg.via && sg.via.via === 'queue';
  const prev = i > 0 && (!first || viaQ) ? L_[i - 1].fid : null, next = L_[i + 1] ? L_[i + 1].fid : null, f = D.fns[it.fid];
  return {
    i, fid: it.fid, prev, next, seg: it.seg, flow: fl,
    kind: !prev ? 'entry' : viaQ || isAsync(prev, it.fid) ? 'async' : isCall(prev, it.fid) ? 'call' : 'next',
    label: (fl && fl.labels && fl.labels[it.fid]) || fnInfo(it.fid).name,
    effects: (fl && fl.effects && fl.effects[it.fid]) || [],
    data: f.data || [], states: statesOf(it.fid), exts: extsOfFn(it.fid),
    actor: i === 0 ? runActor() : first && !prev ? actorOfFn(it.fid) : null, mod: f.module,
  };
}
const curStep = () => (state.step >= 0 ? stepAt(state.step) : null);
// call stack up to step i: a callee is pushed, a return pops to its caller, an async hand-off starts anew
function framesUpTo(i) {
  const L_ = runList(), out = [];
  for (let j = 0; j <= i && j < L_.length; j++) {
    const s = L_[j].fid, p = j ? L_[j - 1].fid : null;
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
  if (!map.cam.contains(box, 20) || map.cam.vb.w > 2600) await map.cam.animTo(box, 160, 1100, RM ? 0 : 380);
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
async function showStep() {
  const s = curStep(); if (!s) return;
  state.played = true;
  renderPlayer(); setSel({ t: 'step' });
  bus.emit('step', s);
  if (state.view !== 'map') return;
  if (state.level === 0) await sysHop(s);
  else if (state.level === 1) await mapHop(s);
  else if (state.level === 2) {
    const m = s.mod;
    if (!fv || fv.mod !== m || $('#sc2').classList.contains('hidden')) {
      await up(1, true);
      revealMod(m);
      await mapHop(s);
      const u = unitFor(m);
      if (u) await drill(2, { from: map.nodes[u].el, mod: u }, true);
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
  const r = state.run; let start = 0;
  // a fixed run (path, variant) keeps its flows; only the flow added by a jump from a variant is dropped
  for (let si = 0; si < r.segs.length; si++) { if (si > 0 && state.step < start && (!r.fixed || r.segs[si].extra)) { r.segs.length = si; break; } start += r.segs[si].steps.length; }
}
async function replay() { if (state.busy) return; state.busy = true; try { await showStep(); } finally { state.busy = false; } }
async function stepIn() {
  if (state.view !== 'map') setView('map');
  if (state.step < 0) { await doStep(1); return; }
  const s = curStep(); if (!s) return;
  if (state.level === 0) { await drill(1, { from: sys.nodes.svc.el, focus: s.mod }); return replay(); }
  if (state.level === 1) { revealMod(s.mod); const u = unitFor(s.mod); if (!u) return; await drill(2, { from: map.nodes[u].el, mod: u }); return replay(); }
  if (state.level === 2) return drill(3, { from: fv.nodes[s.fid] ? fv.nodes[s.fid].el : null, fn: s.fid, target: s.next, frames: framesUpTo(state.step) });
  return doStep(1);
}
async function stepOut() { if (state.level > 0) { await up(); if (state.step >= 0) replay(); } }
async function auto() {
  if (!state.run) return;
  state.playing = !state.playing; renderPlayer();
  if (state.playing && state.step >= runList().length - 1 && !atBranch()) { state.step = -1; state.run = buildRun(state.run.key); state.exitText = null; }
  while (state.playing) {
    const ok = await doStep(1);
    if (!ok) { state.playing = false; state.resume = atBranch(); break; }
    await sleep((RM ? 600 : 1100) / state.speed);
  }
  renderPlayer();
}
function pause() { state.playing = false; }
function showChoice(on) {
  const c = $('#choice'); c.classList.toggle('on', on); c.innerHTML = '';
  if (!on) return;
  H('span', { style: 'color:var(--muted);font-size:12px' }, c, esc(t('chooseBranch')));
  (branchesHere() || []).forEach((b) => {
    const target = b.flow && D.flowById[b.flow];
    const btn = H('button', { class: 'pbtn', style: 'height:26px', title: (b.text || '') + (target ? ' → ' + target.name : '') }, c, (target ? '↪ ' : '⏹ ') + esc(cut(b.cond || b.text || '?', 28)));
    btn.onclick = () => takeBranch(b);
  });
}
function takeBranch(b) {
  showChoice(false);
  const target = b.flow && D.flowById[b.flow];
  if (target) {
    state.run.segs.push({ flow: target, steps: target.steps.filter((s) => D.fns[s]), via: b });
    doStep(1).then(() => { if (state.resume) { state.resume = false; auto(); } });
  } else {
    state.exitText = b.text || b.cond; state.resume = false;
    renderPlayer(); bus.emit('step', curStep());
  }
}
function resetRun() {
  state.playing = false; state.step = -1; state.played = false; state.exitText = null;
  if (state.run) state.run = buildRun(state.run.key);
  [sys, map, fv].forEach((x) => { if (x) { clearPlay(x.svg); x.fx.innerHTML = ''; } });
  showChoice(false); renderPlayer(); renderCrumbs(); setSel(null);
  bus.emit('step', null);
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

/* ---------- player UI ---------- */
function renderPlayer() {
  const r = state.run, list = runList(), s = curStep();
  const fl = r && r.segs[0].flow;
  $('#scName').textContent = r ? r.name : t('noPlay');
  $('#pcur').title = r ? r.name + (r.desc ? '\n' + r.desc : '') : '';
  const act = (r && r.actor) || (fl && fl.actor) || (runActor() ? actorName(runActor()) : '');
  const br = branchesHere();
  const fe = r && r.fixed ? jvEnd() : null;
  const ex = r && r.fixed ? (fe && fe.exit ? fe.text.replace(/^[^:]+:\s*/, '') : '') : state.exitText || (lastSeg() && lastSeg().flow && lastSeg().flow.exit) || (br ? br.map((b) => b.cond).join(' / ') : '');
  $('#scActor').textContent = r ? [act && `${t('actor')}: ${act}`, ex && `${t('exit')}: ${ex}`].filter(Boolean).join(' · ') || t('stepsN', list.length) : '';
  $('#pStep').innerHTML = `${esc(t('step'))} <kbd>→</kbd>`; $('#pIn').innerHTML = `${esc(t('stepIn'))} <kbd>↵</kbd>`; $('#pOut').innerHTML = `${esc(t('stepOut'))} <kbd>Esc</kbd>`;
  $('#pAuto').textContent = state.playing ? t('pause') : t('auto');
  $('#pPrev').title = t('back') + ' (←)'; $('#pReset').title = t('reset');
  const tk = $('#ticks'); tk.innerHTML = '';
  const pendingBr = br && !state.exitText ? br.map((b) => D.flowById[b.flow]).filter(Boolean)[0] : null;
  const extra = pendingBr ? Math.min(20, pendingBr.steps.length) : 0;
  tk.classList.toggle('dense', list.length + extra > 60);
  const segStart = []; let acc = 0; (r ? r.segs : []).forEach((sg) => { segStart.push(acc); acc += sg.steps.length; });
  list.forEach((it, i) => {
    if (i && segStart.includes(i)) H('span', { class: 'gap' }, tk);
    const el = H('span', { class: i === state.step ? 'cur' : i < state.step ? 'done' : '', title: `${i + 1}. ${stepLabel(i)}` }, tk);
    el.onclick = () => { if (!state.busy) { state.step = i; trimSegs(); replay(); } };
  });
  if (extra) { H('span', { class: 'gap' }, tk); for (let i = 0; i < extra; i++) H('span', { class: 'br' }, tk); }
  $('#capNo').textContent = s ? (r.fixed && r.segs.length > 1 ? jvProgress() : `${state.step + 1}/${list.length}${extra ? '+' : ''}`) : '';
  $('#capTx').textContent = s ? s.label + (state.exitText && state.step === list.length - 1 ? ' → ' + t('exit') + ': ' + state.exitText : '') : r && list.length ? t('start') : t('noPlay');
  $('#capTx').title = s ? fnInfo(s.fid).name : '';
  $('#capMut').innerHTML = s ? effBadges(s).map(mutBadge).join('') : '';
  $('#capSt').innerHTML = s ? s.states.map(statusPill).join('') : '';
  $('#pPrev').disabled = state.step <= 0;
  ['pStep', 'pIn', 'pAuto'].forEach((id) => { $('#' + id).disabled = !list.length; });
  if (!atBranch()) showChoice(false);
  showPlate(); jvPaintTags();
}
// the plate between flows of a path / the end of a variant, in the caption of the player
function showPlate() {
  const p = jvPlate(); if (!p || atBranch()) return;
  const c = $('#choice'); c.classList.add('on');
  H('span', { class: 'jv-plate ' + p.tone, title: p.text }, c, (p.kind ? jvIcon(p.kind) : '') + esc(p.text));
  if (p.jump) { const b = H('button', { class: 'pbtn', style: 'height:26px', title: t('jvGoTip') }, c, '↪ ' + esc(cut(t('jvGo'), 30))); b.onclick = () => jvJump(); }
}
function stepLabel(i) { const it = runList()[i]; if (!it) return ''; const fl = state.run.segs[it.seg].flow; return (fl && fl.labels && fl.labels[it.fid]) || fnInfo(it.fid).name; }

/* ---------- scenario picker ---------- */
// options: [value, label, kind ('\0g' = general), top, name]
function tourOptions() {
  const opts = [];
  jvJourneys().forEach((j) => opts.push(['journey:' + j.id, j.name, '\0j', false, j.name]));
  D.flows.forEach((fl) => opts.push(['flow:' + fl.i, fl.name, fl.kind || '', !!fl.top, fl.name]));
  if (D.changed.length) opts.push(['changes', t('optChanges'), '\0g', false]);
  if (D.entries.length) opts.push(['overview', t('optOverview'), '\0g', false]);
  return opts;
}
function pickGroups(opts, q) {
  q = q.trim().toLowerCase();
  const ok = (o) => !q || (o[4] || o[1]).toLowerCase().includes(q) || (o[2] && o[2][0] !== '\0' && o[2].toLowerCase().includes(q));
  const flows = opts.filter((o) => o[2] !== '\0g' && o[2] !== '\0j'), gen = opts.filter((o) => o[2] === '\0g'), jr = opts.filter((o) => o[2] === '\0j');
  const hasKinds = flows.some((o) => o[2] || o[3]);
  const gs = [];
  const add = (label, arr) => { arr = arr.filter(ok); if (arr.length) gs.push({ label, items: arr }); };
  add(t('jvTitle'), jr);
  if (hasKinds) {
    add(t('grpTop'), flows.filter((o) => o[3]));
    add(t('grpGeneral'), gen);
    const by = {};
    flows.forEach((o) => { (by[o[2] || 'other'] = by[o[2] || 'other'] || []).push(o); });
    Object.keys(by).sort((a, b) => (a === 'declared' ? -1 : b === 'declared' ? 1 : by[b].length - by[a].length || a.localeCompare(b))).forEach((k) => {
      const arr = by[k].filter(ok);
      if (arr.length) gs.push({ label: k + ' (' + by[k].length + ')', items: arr });
    });
  } else {
    add(t('grpGeneral'), gen);
    add(t('grpFlows') + ' (' + flows.length + ')', flows);
  }
  return gs;
}
const PICK_MAX = 120;
const PK = { open: false, act: 0, flat: [] };
function drawPick() {
  const gs = pickGroups(tourOptions(), $('#pq').value);
  PK.flat = []; let h = '', shown = 0, total = 0;
  gs.forEach((g) => { total += g.items.length; });
  for (const g of gs) {
    if (shown >= PICK_MAX) break;
    h += `<div class="gh">${esc(g.label)}</div>`;
    for (const o of g.items) {
      if (shown >= PICK_MAX) break;
      h += `<div class="opt${state.run && o[0] === state.run.key ? ' cur' : ''}" role="option" data-i="${PK.flat.length}" title="${esc(o[1])}">${esc(o[4] || o[1])}</div>`;
      PK.flat.push(o[0]); shown++;
    }
  }
  if (!PK.flat.length) h = `<div class="none">${esc(t('flowNone'))}</div>`;
  else if (total > shown) h += `<div class="more">${esc(t('flowMore', total - shown))}</div>`;
  $('#plist').innerHTML = h;
  PK.act = Math.max(0, Math.min(PK.act, PK.flat.length - 1));
  markAct(false);
}
function markAct(scroll) {
  const l = $('#plist'), p = l.querySelector('.opt.act'); if (p) p.classList.remove('act');
  const n = l.querySelector(`.opt[data-i="${PK.act}"]`);
  if (n) { n.classList.add('act'); if (scroll) n.scrollIntoView({ block: 'nearest' }); }
}
function openPick() {
  PK.open = true; $('#ppop').hidden = false; $('#pcur').setAttribute('aria-expanded', 'true');
  $('#pq').value = ''; $('#pq').placeholder = t('flowSearch'); PK.act = 0; drawPick();
  const c = $('#plist').querySelector('.opt.cur'); if (c) { PK.act = +c.dataset.i; markAct(true); }
  $('#pq').focus();
}
function closePick(focus) {
  PK.open = false; $('#ppop').hidden = true; $('#pcur').setAttribute('aria-expanded', 'false');
  if (focus) $('#pcur').focus();
}
function choose(v) { closePick(true); if (v != null) loadRun(v); }
function setupPicker() {
  let pqT = null;
  $('#pcur').onclick = () => (PK.open ? closePick() : openPick());
  $('#pq').oninput = () => { clearTimeout(pqT); pqT = setTimeout(() => { PK.act = 0; drawPick(); }, 60); };
  $('#pq').addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'ArrowDown') { e.preventDefault(); PK.act = Math.min(PK.flat.length - 1, PK.act + 1); markAct(true); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); PK.act = Math.max(0, PK.act - 1); markAct(true); }
    else if (e.key === 'Enter') { e.preventDefault(); clearTimeout(pqT); drawPick(); choose(PK.flat[PK.act]); }
    else if (e.key === 'Escape') { e.preventDefault(); closePick(true); }
  });
  $('#plist').addEventListener('mousedown', (e) => { const o = e.target.closest('.opt'); if (o) { e.preventDefault(); choose(PK.flat[+o.dataset.i]); } });
  document.addEventListener('mousedown', (e) => { if (PK.open && !e.target.closest('#pcombo')) closePick(); });
}
function defaultRunKey() {
  const byActor = (x) => actorOfFn(x.steps[0]);
  const fl = D.flows.find((x) => x.kind === 'declared' && x.top && byActor(x)) || D.flows.find((x) => x.kind === 'declared' && x.top) ||
    D.flows.find((x) => x.top && byActor(x)) || D.flows.find((x) => x.top) || D.flows[0];
  if (fl) return 'flow:' + fl.i;
  return D.changed.length ? 'changes' : D.entries.length ? 'overview' : null;
}
// loadRun(key): 'flow:<i>' (index in model.flows), 'fn:<fid>', 'changes', 'overview'
function loadRun(key) {
  state.playing = false;
  state.run = buildRun(key); state.step = -1; state.played = false; state.exitText = null;
  [sys, map, fv].forEach((x) => { if (x) { clearPlay(x.svg); x.fx.innerHTML = ''; } });
  if (fv && state.level === 2) { buildFns(fv.mod); fitFns(); }
  showChoice(false); renderPlayer(); renderCrumbs();
  bus.emit('flow', state.run); bus.emit('step', null);
}
function playFrom(fid) {
  const fl = D.flows.find((x) => x.steps[0] === fid);
  loadRun(fl ? 'flow:' + fl.i : 'fn:' + fid);
  auto();
}

Object.assign(AM, {
  play: {
    load: loadRun, step: doStep, stepIn, stepOut, auto, pause, reset: resetRun, playFrom,
    go: (i) => { if (state.busy || !state.run) return; state.step = Math.max(0, Math.min(runList().length - 1, i)); trimSegs(); return replay(); },
    choose: (b) => takeBranch(b), branches: () => (atBranch() ? branchesHere() : null),
    current: curStep, stepAt, list: runList, frames: framesUpTo, options: tourOptions, defaultKey: defaultRunKey,
    get run() { return state.run; },
  },
  ui: { statusPill, mutBadge, effBadges, stTone },
});
