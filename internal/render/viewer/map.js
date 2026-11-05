/* map.js — the map: one scene, four levels (0 system, 1 modules, 2 functions of a module, 3 code),
 * module groups for large repos, selection, drill-in/out with camera animation, crumbs, minimap. */

let sys = null, map = null, fv = null;
const INSET = 34; // px kept free at the top of every scene: the crumbs sit over its top-left corner
// the sequence dock (views/sequence.js) covers a strip at the bottom or on the right of the map: cameras fit into the rest
const DOCK = { b: 0, r: 0 };
const dockInsets = (cam) => { cam.insetBottom = DOCK.b; cam.insetRight = DOCK.r; };
// setDock(bottom, right, dur): the dock changed; re-frame the current step (or the whole level) in the free area
function setDock(b, r, dur = 220) {
  b = Math.max(0, b | 0); r = Math.max(0, r | 0);
  if (DOCK.b === b && DOCK.r === r) return;
  DOCK.b = b; DOCK.r = r;
  [sys, map, fv].forEach((sc) => { if (sc && sc.cam) dockInsets(sc.cam); });
  if (state.view !== 'map' || state.busy) return;
  const s = curStep();
  if (state.level === 0 && sys) sys.cam.animTo(sys.extent, 30, 0, RM ? 0 : dur);
  else if (state.level === 1 && map) {
    const a = s && s.prev ? unitFor(D.fns[s.prev].module) : s && s.actor && map.nodes[s.actor.id] ? s.actor.id : null, c = s ? unitFor(s.mod) : null;
    const ra = a && map.pos[a], rb = c && map.pos[c];
    if (rb) map.cam.animTo(bbox([ra, rb]), 160, 1100, RM ? 0 : dur); else map.cam.animTo(map.extent, 10, 0, RM ? 0 : dur);
  } else if (state.level === 2 && fv) fv.cam.animTo(fv.extent, 10, 0, RM ? 0 : dur);
}

/* ===================== module groups (more than GROUP_MIN modules) ===================== */
// A path trie of modules; the start cut greedily splits the largest branch while blocks <= GROUP_TARGET.
const GROUP_MIN = 60, GROUP_TARGET = 40;
const G = { on: false, TN: {}, modNode: {}, userOpen: new Set(), initOpen: new Set(), root: null, focus: null };
function buildTrie() {
  G.TN = {}; G.modNode = {}; G.initOpen = new Set();
  const mk = (path) => ({ id: 'g:' + path, path, parent: null, kids: new Map(), own: [], mods: [], n: 0, label: path });
  const root = mk('');
  D.allMods.forEach((m) => {
    let node = root;
    if (m !== '.' && m !== '') {
      let p = '';
      m.split('/').forEach((seg) => {
        p = p ? p + '/' + seg : seg;
        if (!node.kids.has(seg)) node.kids.set(seg, mk(p));
        node = node.kids.get(seg);
      });
    }
    node.own.push(m);
  });
  const compress = (n, top) => {
    n.kids = Array.from(n.kids.values()).map((k) => compress(k, false)).sort((a, b) => (a.path < b.path ? -1 : 1));
    return !top && !n.own.length && n.kids.length === 1 ? n.kids[0] : n;
  };
  const fin = (n, parent) => {
    n.parent = parent;
    n.label = parent && parent.path ? n.path.slice(parent.path.length + 1) : n.path;
    n.kids.forEach((k) => fin(k, n));
    n.mods = n.own.concat(...n.kids.map((k) => k.mods));
    n.n = n.own.reduce((s, m) => s + (D.byMod[m] || []).length, 0) + n.kids.reduce((s, k) => s + k.n, 0);
    G.TN[n.id] = n;
    n.own.forEach((m) => { G.modNode[m] = n; });
  };
  const r = compress(root, true);
  fin(r, null);
  G.initOpen.add(r.id);
  let cutList = r.kids.slice(), size = r.own.length + r.kids.length;
  const blocked = new Set();
  for (;;) {
    const cand = cutList.filter((n) => n.mods.length > 1 && !blocked.has(n.id)).sort((a, b) => b.mods.length - a.mods.length)[0];
    if (!cand) break;
    const delta = cand.own.length + cand.kids.length - 1;
    if (size + delta > GROUP_TARGET) { blocked.add(cand.id); continue; }
    cutList = cutList.filter((n) => n !== cand).concat(cand.kids);
    size += delta; G.initOpen.add(cand.id);
  }
  G.root = r;
}
const isOpen = (n) => G.initOpen.has(n.id) || G.userOpen.has(n.id);
function setupGroups() {
  G.on = D.allMods.length > GROUP_MIN;
  if (G.on) buildTrie(); else { G.root = null; G.TN = {}; G.modNode = {}; }
  [...G.userOpen].forEach((id) => { if (!G.TN[id]) G.userOpen.delete(id); });
  if (G.focus && !G.TN[G.focus.id]) G.focus = null; else if (G.focus) G.focus = G.TN[G.focus.id];
}
// units on the module map: modules, or collapsed groups of modules
let units = [], U = {}, unitOf = {};
function computeUnits() {
  units = []; U = {}; unitOf = {};
  const add = (u) => { u.chg = chgOf(u.mods); units.push(u); U[u.id] = u; u.mods.forEach((m) => { unitOf[m] = u.id; }); };
  const modUnit = (m) => add({ id: m, kind: 'm', mods: [m], n: (D.byMod[m] || []).length });
  if (FOCUS) { FOCUS.modList.forEach(modUnit); return; } // a scenario is open: its modules only, no groups
  if (!G.on) { D.allMods.forEach(modUnit); return; }
  const visit = (n) => {
    if (n.mods.length === 1 && n.parent) { modUnit(n.mods[0]); return; }
    if (n.parent && !isOpen(n)) { add({ id: n.id, kind: 'g', node: n, mods: n.mods, n: n.n, nm: n.mods.length }); return; }
    n.own.forEach(modUnit);
    n.kids.forEach(visit);
  };
  visit(G.root);
}
function modelFor(path) {
  for (let p = path; p; p = p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '') if (D.MM[p]) return D.MM[p];
  return null;
}
function unitLayer(u) {
  if (u.kind === 'm') return layerOf(u.id);
  const i = modelFor(u.node.path);
  if (i && LAYERS.includes(i.layer)) return i.layer;
  const c = {};
  u.mods.forEach((m) => { const l = layerOf(m); c[l] = (c[l] || 0) + (D.byMod[m] || []).length + 1; });
  return Object.keys(c).sort((a, b) => c[b] - c[a])[0] || 'other';
}
const unitTitle = (u) => (u.kind === 'g' ? (u.node.path || t('root')) + '/…' : modLabel(u.id));
// expand the groups on the path to module m; true when the map changed
function revealMod(m) {
  const n = !FOCUS && G.on && G.modNode[m];
  if (!n || unitOf[m] === m) return false;
  let changed = false, deepest = null;
  for (let a = n.mods.length > 1 ? n : n.parent; a && a.parent; a = a.parent) if (!isOpen(a)) { G.userOpen.add(a.id); changed = true; if (!deepest) deepest = a; }
  if (n.mods.length > 1 && n.parent && !isOpen(n)) { G.userOpen.add(n.id); changed = true; }
  if (changed) { G.focus = deepest || G.focus; buildMap(); }
  return changed;
}
const unitFor = (m) => (map && map.nodes[m] ? m : unitOf[m] || null);

/* ===================== scenario focus: the map of one scenario =====================
 * While a scenario (a path or a scenario of the list, or any other run) is open the map shows only what takes part in it: the actors that
 * start its flows, the modules and functions of its steps, the outside systems its steps call, the database when a
 * step reads or writes a table, the queue when a step is a hand-off. Everything else is not drawn at all. The same
 * levels, cards and arrows as the full map; on the modules level the participants keep their layer bands and go
 * left to right in the order the scenario reaches them. "Map" (or ✕) brings the full map back where it was. */
let FOCUS = null; // {key, mods: Set, modList: [module by first step], fids: Set, acts: Set, exts: Set, actTo: {actor: Set(module)}, extFrom: {ext: Set(module)}}
let PRE = null;   // the full map before the focus: level, module, function and cameras
const shownActors = () => (FOCUS ? D.actors.filter((a) => FOCUS.acts.has(a.id)) : D.actors);
const shownExts = () => (FOCUS ? D.externals.filter((x) => FOCUS.exts.has(x.id)) : D.externals);
function computeFocus() {
  const L_ = state.run ? runList() : [];
  if (!L_.length) return null;
  const mods = new Set(), modList = [], fids = new Set(), acts = new Set(), exts = new Set(), actTo = {}, extFrom = {};
  const q = D.externals.find((x) => x.kind === 'queue'), dbs = D.externals.filter((x) => x.kind === 'db');
  const link = (o, k, m) => { if (m && mods.has(m)) (o[k] = o[k] || new Set()).add(m); };
  for (let i = 0; i < L_.length; i++) {
    const s = stepAt(i); if (!s) continue;
    const m = s.mod && D.allMods.includes(s.mod) ? s.mod : null;
    if (m && !mods.has(m)) { mods.add(m); modList.push(m); }
    if (D.fns[s.fid]) fids.add(s.fid);
    if (s.actor) { acts.add(s.actor.id); link(actTo, s.actor.id, m); }
    s.exts.forEach((x) => { exts.add(x.id); link(extFrom, x.id, m); });
    if (s.kind === 'async' && q) { exts.add(q.id); link(extFrom, q.id, s.prev && D.fns[s.prev] ? D.fns[s.prev].module : m); }
    const writes = (s.data || []).length || (s.effects || []).some((e) => /^(INSERT|UPDATE|DELETE|UPSERT|SELECT|READ)\b/i.test(e));
    if (writes && dbs.length) {
      const own = dbs.filter((x) => (x.functions || []).includes(s.fid) || (x.modules || []).includes(s.mod));
      (own.length ? own : dbs.length === 1 ? dbs : []).forEach((x) => { exts.add(x.id); link(extFrom, x.id, m); });
    }
  }
  // the actor of a through path, by name, when its first step is not one of the actor's entry points
  const an = state.run.actor && String(state.run.actor).trim().toLowerCase();
  const byName = an && D.actors.find((a) => actorName(a).trim().toLowerCase() === an);
  if (byName && !acts.size) { acts.add(byName.id); link(actTo, byName.id, modList[0]); }
  if (!mods.size) return null;
  const key = [modList.join(','), Array.from(fids).sort().join(','), Array.from(acts).sort().join(','), Array.from(exts).sort().join(',')].join('|');
  return { key, mods, modList, fids, acts, exts, actTo, extFrom };
}
// applyFocus(): the open run changed (opened, a branch taken, closed) — rebuild the levels for its participants.
// Opening goes to the modules level; closing returns the full map to the level and cameras it had before.
// toModules: a new scenario was opened — start on the modules level even when another one was open.
function applyFocus(toModules) {
  const nf = computeFocus();
  if ((nf ? nf.key : '') === (FOCUS ? FOCUS.key : '') && !(toModules && nf && state.level !== 1)) { FOCUS = nf; return false; }
  const entering = !FOCUS && nf, leaving = FOCUS && !nf;
  if (entering) PRE = { level: state.level, mod: state.mod, fn: state.fn, sys: sys && { ...sys.cam.vb }, map: map && { ...map.cam.vb }, fv: fv && { mod: fv.mod, vb: { ...fv.cam.vb } } };
  FOCUS = nf;
  buildSystem(); buildMap(); fitSys(); fitMap();
  let lvl = state.level;
  if (entering || (toModules && nf)) lvl = 1;
  else if (leaving && PRE) {
    lvl = PRE.level; state.mod = PRE.mod; state.fn = PRE.fn;
    if (PRE.sys) sys.cam.set(PRE.sys);
    if (PRE.map) map.cam.set(PRE.map);
  } else if (lvl >= 2 && !(state.mod && nf.mods.has(state.mod))) lvl = 1;
  if (lvl >= 2 && !(state.mod && D.byMod[state.mod])) lvl = 1;
  if (lvl >= 2) { buildFns(state.mod); fitFns(); if (leaving && PRE && PRE.fv && PRE.fv.mod === state.mod) fv.cam.set(PRE.fv.vb); }
  if (lvl === 3) { if (state.fn && D.fns[state.fn]) buildCode(state.fn, null, defaultFrames(state.fn)); else lvl = 2; }
  if (leaving) PRE = null;
  state.level = lvl;
  SC().forEach((e, i) => e.classList.toggle('hidden', i !== lvl));
  const sc = SC()[lvl];
  if (sc && sc.animate && !RM) sc.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 320, easing: 'ease-out' });
  if (state.sel && state.sel.t !== 'step') { state.sel = null; renderPanel(); }
  afterLevel();
  return true;
}

/* ===================== selection helpers ===================== */
function clearSel(svg) {
  svg.classList.remove('has-sel');
  svg.querySelectorAll('.sel,.rel').forEach((e) => { e.classList.remove('sel', 'rel'); if (e.tagName === 'path') e.style.stroke = ''; });
}
function clearPlay(svg) { svg.classList.remove('play'); svg.querySelectorAll('.hot').forEach((e) => e.classList.remove('hot')); }
function wireSelect(svg, cam, onSel, onDrill) {
  svg.addEventListener('click', (e) => {
    if (cam && cam.justDragged) return;
    if (pathClick(e)) return; // a scenario is open: the node of the next step steps forward
    clearPlay(svg);
    const n = e.target.closest('.n');
    onSel(n ? n.getAttribute('data-id') : null);
  });
  svg.addEventListener('dblclick', (e) => { const n = e.target.closest('.n'); if (n) onDrill(n.getAttribute('data-id')); });
}
function sysIcon(g, kind, x, y) {
  const st = { fill: 'none', stroke: 'var(--text-2)', 'stroke-width': 1.6, 'stroke-linecap': 'round', 'stroke-linejoin': 'round' };
  const P = (d) => S_('path', { d, ...st }, g);
  if (kind === 'payment') { S_('rect', { x, y: y + 2, width: 22, height: 16, rx: 3, ...st }, g); P(`M${x},${y + 8}h22`); }
  else if (kind === 'pos') { S_('rect', { x: x + 2, y, width: 18, height: 12, rx: 2, ...st }, g); P(`M${x},${y + 20}h22M${x + 5},${y + 16}h12`); }
  else if (kind === 'auth') { S_('circle', { cx: x + 7, cy: y + 10, r: 5, ...st }, g); P(`M${x + 12},${y + 10}h10v5M${x + 18},${y + 10}v4`); }
  else if (kind === 'db' || kind === 'storage' || kind === 'cache') { S_('ellipse', { cx: x + 11, cy: y + 3, rx: 10, ry: 3.5, ...st }, g); P(`M${x + 1},${y + 3}v14c0,4.5 20,4.5 20,0v-14M${x + 1},${y + 10}c0,4.5 20,4.5 20,0`); }
  else if (kind === 'queue') { for (let i = 0; i < 3; i++) S_('rect', { x: x + i * 8, y: y + 4, width: 6, height: 14, rx: 1.5, ...st }, g); }
  else if (kind === 'messaging') { S_('rect', { x, y: y + 3, width: 22, height: 15, rx: 2, ...st }, g); P(`M${x},${y + 4}l11,8l11,-8`); }
  else { S_('circle', { cx: x + 11, cy: y + 10, r: 9, ...st }, g); P(`M${x + 2},${y + 10}h18M${x + 11},${y + 1}c-5,5 -5,13 0,18c5,-5 5,-13 0,-18`); }
}
function actorIcon(n, x, y) {
  S_('circle', { cx: x + 11, cy: y + 6, r: 6, fill: 'none', stroke: 'var(--text-2)', 'stroke-width': 1.6 }, n);
  S_('path', { d: `M${x},${y + 26} q11,-14 22,0`, fill: 'none', stroke: 'var(--text-2)', 'stroke-width': 1.6 }, n);
}

/* ===================== edge labels for the system level ===================== */
function extLabel(x) {
  if (x.kind === 'db' && Object.keys(D.tables).length) {
    const cnt = {};
    D.fids.forEach((id) => { const f = D.fns[id]; if (!f.data) return; if (!(x.modules || []).includes(f.module) && !(x.functions || []).includes(id)) return; f.data.forEach((a) => { cnt[a.table] = (cnt[a.table] || 0) + 1; }); });
    const tb = Object.keys(cnt).sort((a, b) => cnt[b] - cnt[a]);
    if (tb.length) return t('readsWrites') + ': ' + tb.slice(0, 3).join(', ') + (tb.length > 3 ? ' +' + (tb.length - 3) : '');
  }
  const mods = new Set(x.modules || []), names = [];
  (x.functions || []).forEach((id) => { const f = D.fns[id]; if (!f) return; (f.callees || []).concat(f.async_callees || []).forEach((c) => { const g = D.fns[c]; if (g && mods.has(g.module) && !names.includes(g.name) && !/^(New|new|init|Init|With|Base|base)[A-Z_]|^(New|init|BaseURL|String|Error)$/.test(g.name)) names.push(g.name); }); });
  if (!names.length) return '';
  return names.slice(0, 2).join(', ') + (names.length > 2 ? ' +' + (names.length - 2) : '');
}
// the one line under the name of an actor or an external system: its description when the code has one
// ("61 · Админский сервис…"), else the count of handles / the kind
function sysSub(p, n) {
  if (p.kind === 'actor' || p.actor) {
    // the count is of the scenarios the lists show (technical ones are left out — flow.hidden)
    const a = p.a, d = actorDesc(a).desc, k = visibleOfActor(a).length;
    return cut((k ? t('scenN', k) + ' · ' : '') + (d || actorSub(a)), n);
  }
  return cut(extDesc(p.ext) || t('kinds')[p.ext.kind] || p.ext.kind, n);
}
function sysTip(p) {
  if (p.kind === 'actor' || p.actor) { const d = actorDesc(p.a).desc; return actorName(p.a) + ' — ' + actorSub(p.a) + (d ? '\n' + d : ''); }
  const d = extDesc(p.ext); return p.ext.name + ' · ' + (t('kinds')[p.ext.kind] || p.ext.kind) + (d ? '\n' + d : '');
}
function actorLabel(a) {
  const sv = Array.from(a.services).map((s) => s.replace(/(Service|Server|Handler|Controller)$/, '')).slice(0, 2);
  const pr = Array.from(a.protos).join(', ');
  if (a.eps.length) return (sv.length ? sv.join(', ') + (a.services.size > 2 ? '…' : '') + ' — ' : '') + (pr || t('epsN', a.eps.length));
  return '';
}

/* ===================== Level 0: system context ===================== */
const SVC = { x: -230, y: -135, w: 460, h: 270 };
const SIDES = { top: ['auth'], right: ['payment', 'pos', 'http_api', 'messaging', 'other'], bottom: ['db', 'queue', 'cache', 'storage'] };
function sysLayout() {
  const P = {}, labels = [];
  // actors: left column
  const acts = shownActors(), AH = 64, AG = 46;
  const ah = acts.length * (AH + AG) - AG;
  acts.forEach((a, i) => { P[a.id] = { x: -730, y: -ah / 2 + i * (AH + AG), w: 200, h: AH, kind: 'actor', a }; });
  if (acts.length) labels.push({ x: -630, y: -ah / 2 - 22, text: t('kinds').actor });
  // externals: kinds to sides
  const byKind = {};
  shownExts().forEach((x) => { const k = Object.values(SIDES).some((s) => s.includes(x.kind)) ? x.kind : 'other'; (byKind[k] = byKind[k] || []).push(x); });
  const side = { top: [], right: [], bottom: [] };
  for (const s in SIDES) SIDES[s].forEach((k) => { if (byKind[k]) side[s].push(k); });
  const colH = (ks) => ks.reduce((h, k) => h + 30 + byKind[k].length * 66, 0);
  while (colH(side.right) > 760 && side.right.length > 1) { const k = side.right.pop(); (side.top.length <= side.bottom.length ? side.top : side.bottom).push(k); }
  // right column
  let y = -colH(side.right) / 2;
  side.right.forEach((k) => {
    labels.push({ x: 640, y: y + 14, text: t('kinds')[k] || k });
    y += 30;
    byKind[k].forEach((x) => { P[x.id] = { x: 545, y, w: 190, h: 56, kind: x.kind, ext: x }; y += 66; });
  });
  // top and bottom rows
  const row = (ks, yy, below) => {
    const n = ks.reduce((s, k) => s + byKind[k].length, 0) + (ks.length - 1) * 0.4;
    let xx = -(n * 210 - 20) / 2;
    ks.forEach((k) => {
      const list = byKind[k], w = list.length * 210 - 20;
      labels.push({ x: xx + w / 2, y: below ? yy + 84 : yy - 16, text: t('kinds')[k] || k });
      list.forEach((x) => { P[x.id] = { x: xx, y: yy, w: 190, h: 56, kind: x.kind, ext: x }; xx += 210; });
      xx += 84;
    });
  };
  row(side.top, -340, false);
  row(side.bottom, 300, true);
  return { P, labels };
}
function buildSystem() {
  const root = $('#sc0'); root.innerHTML = '';
  const svg = S_('svg', {}, root);
  const g = S_('g', {}, svg), gl = S_('g', {}, g), ge = S_('g', {}, g), gn = S_('g', {}, g), fx = S_('g', {}, g);
  sys = { svg, g, fx, nodes: {}, edges: [] };
  sys.cam = new Cam(svg); sys.cam.insetTop = INSET; dockInsets(sys.cam);
  const { P, labels } = sysLayout();
  labels.forEach((l) => S_('text', { x: l.x, y: l.y, 'text-anchor': 'middle', class: 'grp' }, gl, l.text));
  // service card
  const PJ = D.F.project || {};
  const sv = S_('g', { class: 'n', 'data-id': 'svc' }, gn);
  S_('rect', { x: SVC.x, y: SVC.y, width: SVC.w, height: SVC.h, rx: 18, class: 'card', style: 'stroke-width:1.5' }, sv);
  const title = PJ.name || D.M.title || D.F.repo || 'repo';
  const tt = S_('text', { x: SVC.x + 24, y: SVC.y + 40, class: 't1', style: 'font-size:22px;letter-spacing:-.01em' }, sv, cut(title, 26));
  const langs = Object.entries(D.F.langs || {}).sort((a, b) => b[1] - a[1]).slice(0, 2).map((x) => x[0]).join(', ');
  const meta = S_('text', { x: SVC.x + 24, y: SVC.y + 40, class: 't3 mono' }, sv, cut((D.F.repo && D.F.repo !== title ? D.F.repo + ' · ' : '') + langs, 34));
  requestAnimationFrame(() => { try { meta.setAttribute('x', SVC.x + 34 + tt.getComputedTextLength()); } catch (e) { /* hidden */ } });
  const desc = PJ.description || D.M.summary || '';
  const dl = wrapText(desc, 54, 2);
  dl.forEach((ln, i) => S_('text', { x: SVC.x + 24, y: SVC.y + 66 + i * 17, class: 't2', style: 'font-size:13px' }, sv, ln));
  const y0 = SVC.y + 70 + Math.max(1, dl.length) * 17;
  S_('text', { x: SVC.x + 24, y: y0 + 4, class: 't3' }, sv, `${D.allMods.length} ${t('modules')} · ${D.fids.length} ${t('fns')} · ${D.entries.length} ${t('entries')}`);
  const lays = LAYERS.filter((l) => D.allMods.some((m) => layerOf(m) === l));
  const rowH = Math.min(24, (SVC.y + SVC.h - 34 - (y0 + 18)) / Math.max(1, lays.length));
  lays.forEach((l, i) => {
    const ms = D.allMods.filter((m) => layerOf(m) === l), yy = y0 + 18 + i * rowH;
    S_('text', { x: SVC.x + 24, y: yy + 11, class: 't3', style: 'font-size:10.5px' }, sv, t('layers')[l]);
    const nShow = Math.min(ms.length, 26), step = Math.min(11, 280 / Math.max(1, nShow));
    ms.slice(0, nShow).forEach((m, j) => S_('rect', { x: SVC.x + 132 + j * step, y: yy + 2, width: Math.max(3, step - 3), height: Math.min(12, rowH - 4), rx: 2, fill: LCOLOR[l], opacity: l === 'infra' || l === 'other' ? .55 : .8 }, sv));
    if (ms.length > nShow) S_('text', { x: SVC.x + 136 + nShow * step, y: yy + 11, class: 't3', style: 'font-size:10px' }, sv, '+' + (ms.length - nShow));
  });
  S_('text', { x: SVC.x + SVC.w - 20, y: SVC.y + SVC.h - 14, 'text-anchor': 'end', class: 't3', style: 'font-size:10.5px' }, sv, t('zoomIn') + ' ↘');
  sys.nodes.svc = { el: sv, r: SVC };
  // actors and externals
  for (const id in P) {
    const p = P[id], r = { x: p.x, y: p.y, w: p.w, h: p.h };
    const n = S_('g', { class: 'n', 'data-id': id }, gn);
    const actor = p.kind === 'actor';
    S_('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: actor ? r.h / 2 : 12, class: 'card' }, n);
    if (actor) actorIcon(n, r.x + 18, r.y + 18); else sysIcon(n, p.kind, r.x + 16, r.y + 18);
    const name = actor ? actorName(p.a) : p.ext.name, sub = sysSub(p, 26);
    S_('text', { x: r.x + 52, y: r.y + (actor ? 29 : 26), class: 't1' }, n, cut(name, 20));
    S_('text', { x: r.x + 52, y: r.y + (actor ? 46 : 43), class: 't3' }, n, sub);
    S_('title', {}, n, sysTip(p));
    sys.nodes[id] = { el: n, r, p };
  }
  // edges with labels
  const seenLab = new Set();
  const edge = (a, b, lab, mod, mods) => {
    const ra = sys.nodes[a].r, rb = sys.nodes[b].r;
    const p1 = clip(ra, ctr(rb)), p2 = clip(rb, ctr(ra));
    const path = S_('path', { d: `M${p1.x},${p1.y} L${p2.x},${p2.y}`, class: 'e', 'marker-end': 'url(#arr)' }, ge);
    let lb = null;
    if (lab && !seenLab.has(lab)) {
      seenLab.add(lab);
      const vert = Math.abs(p2.y - p1.y) > Math.abs(p2.x - p1.x), dxs = Math.sign(p2.x - p1.x) || 1;
      const mx = (p1.x + p2.x) / 2 + (vert ? dxs * 10 : 0), my = (p1.y + p2.y) / 2 + (vert ? 4 : -8);
      lb = S_('text', { x: mx, y: my, 'text-anchor': vert ? (dxs < 0 ? 'end' : 'start') : 'middle', class: 'elabel' }, ge, cut(lab, 44));
    }
    sys.edges.push({ a, b, path, lab: lb, text: lab, mod, mods });
  };
  shownActors().forEach((a) => edge(a.id, 'svc', actorLabel(a), null, Array.from(a.mods)));
  shownExts().forEach((x) => edge('svc', x.id, extLabel(x), (x.modules || [])[0], x.modules || []));
  sys.extent = bbox(Object.values(sys.nodes).map((n) => n.r).concat(labels.map((l) => ({ x: l.x - 60, y: l.y - 14, w: 120, h: 20 }))));
  wireSelect(svg, sys.cam, selectSys, drillFromSys);
}
function wrapText(s, n, maxLines) {
  const words = String(s || '').replace(/\s+/g, ' ').trim().split(' '), out = [];
  let cur = '';
  for (const w of words) {
    if ((cur + ' ' + w).trim().length > n) { if (cur) out.push(cur); cur = w; if (out.length === maxLines) break; } else cur = (cur + ' ' + w).trim();
  }
  if (out.length < maxLines && cur) out.push(cur);
  if (out.length === maxLines && words.join(' ').length > out.join(' ').length) out[maxLines - 1] = cut(out[maxLines - 1], n - 1) + '…';
  return out.filter(Boolean);
}
function selectSys(id) {
  const svg = sys.svg; clearSel(svg);
  if (!id) return setSel(null);
  svg.classList.add('has-sel');
  sys.nodes[id].el.classList.add('sel');
  for (const e of sys.edges) if (e.a === id || e.b === id) {
    e.path.classList.add('rel'); if (e.lab) e.lab.classList.add('rel');
    sys.nodes[e.a].el.classList.add('rel'); sys.nodes[e.b].el.classList.add('rel');
  }
  setSel(id === 'svc' ? { t: 'svc' } : id.startsWith('actor:') ? { t: 'actor', id } : { t: 'ext', id });
}
function drillFromSys(id) {
  if (!id || id === 'svc') return drill(1, { from: sys.nodes.svc.el });
  return drill(1, { from: sys.nodes[id].el, focus: id });
}

/* ===================== Level 1: module map ===================== */
const CW = 196, CH = 68, GX = 14, GY = 16, LBL = 160;
function buildMap() {
  computeUnits();
  const root = $('#sc1'); root.innerHTML = '';
  const svg = S_('svg', {}, root);
  const g = S_('g', {}, svg), gb = S_('g', {}, g), ge = S_('g', {}, g), gn = S_('g', {}, g), fx = S_('g', {}, g);
  // unit edges (imports), composition roots skipped
  const w = new Map();
  units.forEach((u) => u.mods.forEach((a) => {
    if (D.roots.has(a)) return;
    ((D.mods[a] || {}).imports || []).forEach((b) => {
      if (D.roots.has(b)) return;
      const ub = unitOf[b];
      if (ub && ub !== u.id) { const k = u.id + '\0' + ub; w.set(k, (w.get(k) || 0) + 1); }
    });
  }));
  const E = Array.from(w, ([k, c]) => { const [a, b] = k.split('\0'); return { a, b, w: c, dash: false }; });
  const present = LAYERS.filter((l) => units.some((u) => unitLayer(u) === l));
  // a scenario: one row per band, in the order the scenario reaches the modules (no reordering sweeps)
  const perBand = present.map((l) => units.filter((u) => unitLayer(u) === l).length);
  const cols = FOCUS ? Math.max(1, Math.min(6, ...perBand)) : Math.max(5, Math.min(8, Math.ceil(Math.sqrt(units.length))));
  const lay = layered(units.map((u, i) => ({ id: u.id, layer: present.indexOf(unitLayer(u)), order: i })), E.map((e) => [e.a, e.b]),
    { w: CW, h: CH, gx: FOCUS ? 44 : GX, gy: GY, cols, x0: LBL, y0: 120, head: 40, gap: 22, sweeps: FOCUS ? 0 : 8 });
  const pos = lay.pos, W = LBL + lay.width + 20;
  const bands = {};
  lay.bands.forEach((b) => { bands[present[b.layer]] = { y: b.y, h: b.h }; });
  map = { svg, g, fx, ge, pos, bands, nodes: {}, edges: [], W, crossings: lay.crossings };
  for (const l of present) {
    const b = bands[l], bg = S_('g', { class: 'band' }, gb);
    S_('rect', { x: 0, y: b.y, width: W, height: b.h, rx: 14 }, bg);
    S_('rect', { x: 0, y: b.y + 14, width: 3, height: b.h - 28, rx: 1.5, fill: LCOLOR[l], style: 'stroke:none' }, bg);
    S_('text', { x: 18, y: b.y + 26 }, bg, t('layers')[l]);
    wrapText(t('layerSub')[l], 20, 3).forEach((ln, i) => S_('text', { x: 18, y: b.y + 44 + i * 16, class: 'bsub' }, bg, ln));
  }
  const ux = (ids) => { const xs = ids.map((id) => pos[id] && pos[id].x + CW / 2).filter((v) => v != null).sort((a, b) => a - b); return xs.length ? xs[xs.length >> 1] : null; };
  const uy = (ids) => { const ys = ids.map((id) => pos[id] && pos[id].y + CH / 2).filter((v) => v != null).sort((a, b) => a - b); return ys.length ? ys[ys.length >> 1] : null; };
  // actors (top row), placed over their entry modules
  const acts = shownActors().map((a) => ({ a, tg: uniq(Array.from(FOCUS ? FOCUS.actTo[a.id] || [] : a.mods).map((m) => unitOf[m]).filter(Boolean)) }));
  acts.forEach((o) => { o.x = ux(o.tg); if (o.x == null) o.x = LBL + lay.width / 2; });
  acts.sort((p, q) => p.x - q.x);
  let lastX = -1e9;
  acts.forEach((o) => { o.x = Math.max(o.x - 100, lastX + 214); lastX = o.x; pos[o.a.id] = { x: o.x, y: 30, w: 200, h: 50, actor: true, a: o.a }; });
  // externals (right column), placed level with their adapter modules
  const EX = W + 90;
  const exts = shownExts().map((x) => ({ x, tg: uniq((FOCUS ? Array.from(FOCUS.extFrom[x.id] || []).concat((x.modules || []).filter((m) => FOCUS.mods.has(m))) : x.modules || []).map((m) => unitOf[m]).filter(Boolean)) }));
  exts.forEach((o) => { o.y = uy(o.tg); if (o.y == null) o.y = 200; });
  exts.sort((p, q) => p.y - q.y);
  let lastY = -1e9;
  exts.forEach((o) => { const yy = Math.max(o.y - 28, lastY + 70); lastY = yy; pos[o.x.id] = { x: EX, y: yy, w: 190, h: 56, ext: o.x }; });
  if (exts.length) S_('text', { x: EX, y: pos[exts[0].x.id].y - 16, class: 'grp' }, gb, t('ext'));
  const lastBand = bands[present[present.length - 1]] || { y: 120, h: 100 };
  const maxY = Math.max(lastBand.y + lastBand.h, ...exts.map((o) => pos[o.x.id].y + 56));
  const right = Math.max(exts.length ? EX + 190 : W, ...acts.map((o) => o.x + 200));
  map.extent = { x: -20, y: acts.length ? 10 : 90, w: right + 40, h: maxY + 20 - (acts.length ? 10 : 90) };
  // cards
  units.forEach((u) => (u.kind === 'g' ? groupCard : mapCard)(gn, u, pos[u.id]));
  acts.forEach((o) => extCard(gn, o.a.id, pos[o.a.id]));
  exts.forEach((o) => extCard(gn, o.x.id, pos[o.x.id]));
  // dashed links: actor → entry units, adapter units → external
  acts.forEach((o) => o.tg.forEach((u) => E.push({ a: o.a.id, b: u, w: 1, dash: true })));
  exts.forEach((o) => o.tg.forEach((u) => E.push({ a: u, b: o.x.id, w: 1, dash: true })));
  drawPortEdges(E, pos, ge);
  map.cam = new Cam(svg, (vb) => { svg.classList.toggle('lod-lo', (svg.parentElement.offsetWidth || 1000) / vb.w < 0.75); });
  map.cam.insetTop = INSET; dockInsets(map.cam); map.cam.maxW = Math.max(map.extent.w, map.extent.h * map.cam.aspect()) * 4; // far enough out for the level to drop below LEAVE (zoom.js)
  wireSelect(svg, map.cam, selectMod, (id) => {
    if (U[id] && U[id].kind === 'g') return expandGroup(U[id].node);
    if (U[id]) return drill(2, { from: map.nodes[id].el, mod: id });
    const m = (id.startsWith('ext:') ? (D.externals.find((x) => x.id === id) || {}).modules || [] : Array.from((D.actors.find((a) => a.id === id) || { mods: new Set() }).mods))[0];
    if (m) goModule(m);
  });
  paintMap();
}
// edges distributed over card ports (top/bottom/sides) and pulled toward a common trunk (bundles)
function drawPortEdges(E, pos, ge) {
  const side = (e) => {
    const A = pos[e.a], B = pos[e.b];
    if (B.ext) return ['r', 'l'];
    if (A.actor) return ['b', 't'];
    if (Math.abs(A.y - B.y) < 2) return ['b', 'b'];
    return B.y > A.y ? ['b', 't'] : ['t', 'b'];
  };
  const ports = {};
  E.forEach((e) => {
    if (!pos[e.a] || !pos[e.b]) return;
    const [sa, sb] = side(e); e.sa = sa; e.sb = sb;
    (ports[e.a + '\0' + sa] = ports[e.a + '\0' + sa] || []).push({ e, end: 'a' });
    (ports[e.b + '\0' + sb] = ports[e.b + '\0' + sb] || []).push({ e, end: 'b' });
  });
  for (const k in ports) {
    const list = ports[k];
    const other = (p) => { const o = pos[p.end === 'a' ? p.e.b : p.e.a]; return p.e.sa === 'r' || p.e.sb === 'l' ? o.y : o.x; };
    list.sort((p, q) => other(p) - other(q));
    list.forEach((p, i) => {
      const id = p.end === 'a' ? p.e.a : p.e.b, r = pos[id], s = p.end === 'a' ? p.e.sa : p.e.sb;
      const tt = (i + 1) / (list.length + 1);
      let pt;
      if (s === 'b') pt = { x: r.x + 18 + (r.w - 36) * tt, y: r.y + r.h };
      else if (s === 't') pt = { x: r.x + 18 + (r.w - 36) * tt, y: r.y };
      else if (s === 'r') pt = { x: r.x + r.w, y: r.y + 10 + (r.h - 20) * tt };
      else pt = { x: r.x, y: r.y + 10 + (r.h - 20) * tt };
      p.e[p.end === 'a' ? 'pa' : 'pb'] = pt;
    });
  }
  for (const e of E) {
    if (!e.pa || !e.pb) continue;
    const a = e.pa, b = e.pb;
    let d;
    if (e.sa === 'r') { const k = Math.max(60, (b.x - a.x) * 0.5); d = `M${a.x},${a.y} C${a.x + k},${a.y} ${b.x - k},${b.y} ${b.x},${b.y}`; }
    else if (e.sa === 'b' && e.sb === 'b') { const k = 46; d = `M${a.x},${a.y} C${a.x},${a.y + k} ${b.x},${b.y + k} ${b.x},${b.y}`; }
    else {
      const dy = b.y - a.y, k = Math.max(36, Math.abs(dy) * 0.5) * Math.sign(dy || 1);
      const mx = (a.x + b.x) / 2, bund = 0.28;
      d = `M${a.x},${a.y} C${a.x + (mx - a.x) * bund},${a.y + k} ${b.x + (mx - b.x) * bund},${b.y - k} ${b.x},${b.y}`;
    }
    const sw = e.w > 1 ? (1.2 + Math.min(2.4, Math.log2(e.w) * 0.6)).toFixed(2) : null;
    e.path = S_('path', { d, class: 'e' + (e.dash ? ' dash' : ''), 'marker-end': 'url(#arr)', style: sw ? 'stroke-width:' + sw : null }, ge);
    S_('title', {}, e.path, nodeTitle(e.a) + ' → ' + nodeTitle(e.b) + (e.w > 1 ? ' · ' + e.w : ''));
    map.edges.push(e);
  }
}
function nodeTitle(id) {
  if (U[id]) return unitTitle(U[id]);
  if (id.startsWith('actor:')) { const a = D.actors.find((x) => x.id === id); return a ? actorName(a) : id; }
  if (id.startsWith('ext:')) return id.slice(4);
  return id;
}
function mapCard(gn, u, r) {
  const m = u.id, l = layerOf(m);
  const n = S_('g', { class: 'n', 'data-id': m }, gn);
  S_('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: 10, class: 'card' }, n);
  S_('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: 10, fill: LCOLOR[l], opacity: l === 'infra' || l === 'other' ? .05 : .07, style: 'pointer-events:none' }, n);
  S_('rect', { x: r.x + 10, y: r.y + 12, width: 3, height: r.h - 24, rx: 1.5, fill: LCOLOR[l], class: 'acc' }, n);
  const nm = short(m), parts = nm.split('/'), lastp = parts.pop();
  S_('text', { x: r.x + 20, y: r.y + 24, class: 't3 lod-o', style: 'font-size:13px' }, n, cutL(parts.join('/'), 22));
  S_('text', { x: r.x + 20, y: r.y + 50, class: 't1 lod-o', style: 'font-size:19px' }, n, cut(lastp, 16));
  S_('text', { x: r.x + 22, y: r.y + 21, class: 't3 lod-d mono', style: 'font-size:10.5px' }, n, cutL(parts.join('/') + (parts.length ? '/' : ''), 24));
  S_('text', { x: r.x + 22, y: r.y + 38, class: 't1 lod-d' }, n, cut(lastp, 22));
  const d = modDesc(m), role = D.MM[m] && D.MM[m].role && D.MM[m].role.toLowerCase() !== lastp.toLowerCase() ? D.MM[m].role : '';
  S_('text', { x: r.x + 22, y: r.y + 55, class: 't3 lod-d' }, n, cut(d || role, 28));
  S_('text', { x: r.x + r.w - 10, y: r.y + 21, 'text-anchor': 'end', class: 't3 lod-d', style: 'font-variant-numeric:tabular-nums' }, n, u.n + ' ' + t('fnShort'));
  chgBadge(n, u.chg, r);
  S_('title', {}, n, modLabel(m) + (d ? '\n' + d : ''));
  map.nodes[m] = { el: n, r };
}
function chgBadge(n, c, r) {
  if (!hasChg(c)) return;
  S_('text', { x: r.x + r.w - 10, y: r.y + r.h - 9, 'text-anchor': 'end', class: 'badge', fill: c.modified ? 'var(--mod)' : c.added ? 'var(--add)' : 'var(--del)' }, n, chgText(c));
  if (c.local) S_('circle', { cx: r.x + r.w - 8, cy: r.y + 8, r: 3.5, fill: 'var(--loc)' }, n);
}
function groupCard(gn, u, r) {
  const l = unitLayer(u), n = S_('g', { class: 'n grp-n', 'data-id': u.id }, gn);
  S_('rect', { x: r.x + 5, y: r.y + 5, width: r.w, height: r.h, rx: 10, class: 'stack' }, n);
  S_('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: 10, class: 'card' }, n);
  S_('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: 10, fill: LCOLOR[l], opacity: .07, style: 'pointer-events:none' }, n);
  S_('rect', { x: r.x + 10, y: r.y + 12, width: 3, height: r.h - 24, rx: 1.5, fill: LCOLOR[l], class: 'acc' }, n);
  const p = u.node.path || t('root'), parts = p.split('/'), lastp = parts.pop();
  S_('text', { x: r.x + 20, y: r.y + 50, class: 't1 lod-o', style: 'font-size:19px' }, n, cut(lastp + '/…', 16));
  S_('text', { x: r.x + 22, y: r.y + 21, class: 't3 lod-d mono', style: 'font-size:10.5px' }, n, cutL(parts.join('/') + (parts.length ? '/' : ''), 24));
  S_('text', { x: r.x + 22, y: r.y + 38, class: 't1 lod-d' }, n, cut(lastp + '/…', 22));
  S_('text', { x: r.x + 22, y: r.y + 55, class: 't3 lod-d' }, n, t('grpMods', u.nm) + ' · ' + u.n + ' ' + t('fnShort'));
  chgBadge(n, u.chg, r);
  S_('title', {}, n, p + '\n' + t('grpHint'));
  map.nodes[u.id] = { el: n, r };
}
function extCard(gn, id, r) {
  const n = S_('g', { class: 'n x', 'data-id': id }, gn);
  S_('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: r.actor ? 25 : 10, class: 'card', style: 'stroke-dasharray:4 3' }, n);
  if (r.actor) actorIcon(n, r.x + 14, r.y + 12); else sysIcon(n, r.ext.kind, r.x + 14, r.y + 17);
  const name = r.actor ? actorName(r.a) : r.ext.name;
  S_('text', { x: r.x + 48, y: r.y + (r.actor ? 22 : 24), class: 't1' }, n, cut(name, 18));
  S_('text', { x: r.x + 48, y: r.y + (r.actor ? 39 : 41), class: 't3' }, n, sysSub(r, 24));
  S_('title', {}, n, sysTip(r));
  map.nodes[id] = { el: n, r };
}
function paintMap() {
  if (!map) return;
  for (const id in map.nodes) {
    const u = U[id], el = map.nodes[id].el;
    if (!u) continue;
    ['chg-a', 'chg-m', 'chg-d'].forEach((c) => el.classList.remove(c));
    const c = chgClass(u.chg); if (c && hasChg(u.chg)) el.classList.add(c);
    el.classList.toggle('dim', state.only && !hasChg(u.chg));
  }
  map.edges.forEach((e) => e.path.classList.toggle('dim', state.only && !(U[e.a] && hasChg(U[e.a].chg)) && !(U[e.b] && hasChg(U[e.b].chg))));
}
function selectMod(id) {
  const svg = map.svg; clearSel(svg);
  if (!id) return setSel(null);
  svg.classList.add('has-sel');
  if (map.nodes[id]) map.nodes[id].el.classList.add('sel');
  for (const e of map.edges) if (e.a === id || e.b === id) {
    e.path.classList.add('rel');
    if (map.nodes[e.a]) map.nodes[e.a].el.classList.add('rel'); if (map.nodes[e.b]) map.nodes[e.b].el.classList.add('rel');
    const l = U[e.a] ? unitLayer(U[e.a]) : 'ext'; e.path.style.stroke = LCOLOR[l];
  }
  if (U[id]) setSel(U[id].kind === 'g' ? { t: 'group', id } : { t: 'mod', id });
  else setSel(id.startsWith('actor:') ? { t: 'actor', id } : { t: 'ext', id });
}
function expandGroup(n) {
  if (!n) return;
  G.userOpen.add(n.id); G.focus = n;
  buildMap();
  const set = new Set(n.mods), rs = units.filter((u) => set.has(u.mods[0])).map((u) => map.pos[u.id]);
  map.cam.fit(map.extent, 10);
  if (rs.length) map.cam.animTo(bbox(rs), 120, 900, 0);
  setSel({ t: 'group', id: n.id, open: true });
  renderCrumbs();
}
// crumb of an open group: close what was opened below it, keep it open
function focusGroup(n) {
  if (!n || n === G.focus) return;
  [...G.userOpen].forEach((id) => { const x = G.TN[id]; if (x && x !== n && (!n.path || x.path.startsWith(n.path + '/'))) G.userOpen.delete(id); });
  G.focus = n;
  buildMap();
  const set = new Set(n.mods), rs = units.filter((u) => set.has(u.mods[0])).map((u) => map.pos[u.id]);
  map.cam.fit(map.extent, 10);
  if (rs.length) map.cam.animTo(bbox(rs), 120, 900, 0);
  setSel({ t: 'group', id: n.id });
  renderCrumbs();
}
function collapseGroup(n) {
  if (!n) return;
  [...G.userOpen].forEach((id) => { const x = G.TN[id]; if (x && (x === n || !n.path || x.path.startsWith(n.path + '/'))) G.userOpen.delete(id); });
  G.focus = n.parent && n.parent.parent ? n.parent : null;
  buildMap(); fitMap();
  if (map.nodes[n.id]) selectMod(n.id); else setSel(null);
  renderCrumbs();
}

/* ===================== Level 2: functions of a module ===================== */
const FW = 300, FH = 66;
function fnScore(id, flowSet) {
  const f = D.fns[id];
  return (flowSet.has(id) ? 1e6 : 0) + (f.status ? 1e5 : 0) + (f.entry ? 1e4 : 0) + Math.min(999, ((f.callees || []).length + (D.callers[id] || []).length) * 10) + Math.min(9, (f.end - f.start) / 20);
}
function buildFns(mod) {
  const root = $('#sc2'); root.innerHTML = '';
  const svg = S_('svg', {}, root);
  const g = S_('g', {}, svg), ge = S_('g', {}, g), gn = S_('g', {}, g), fx = S_('g', {}, g);
  fv = { svg, g, fx, mod, nodes: {}, edges: [], ports: {} };
  const flowSet = new Set(runFids());
  // a scenario is open: only the functions of this module that are its steps (all of them when it has none here)
  const own = FOCUS ? (D.byMod[mod] || []).filter((k) => FOCUS.fids.has(k)) : [];
  const all = (own.length ? own : D.byMod[mod] || []).slice().sort((a, b) => fnScore(b, flowSet) - fnScore(a, flowSet) || (a < b ? -1 : 1));
  const limit = state.fnAll ? 80 : 16;
  let shown = all.slice(0, limit);
  const must = [state.fn, curStep() && curStep().fid].filter((x) => x && D.fns[x] && D.fns[x].module === mod && !shown.includes(x));
  shown = shown.concat(must);
  const set = new Set(shown);
  // depth by intra-module calls: longest path over the DAG (back edges dropped), capped
  const out = {}; shown.forEach((k) => { out[k] = fnInfo(k).out.filter((o) => set.has(o) && o !== k); });
  const st = {}, order = [], back = new Set();
  const visit = (a) => { st[a] = 1; out[a].forEach((b) => { if (st[b] === 1) back.add(a + '\0' + b); else if (!st[b]) visit(b); }); st[a] = 2; order.push(a); };
  shown.forEach((k) => { if (!st[k]) visit(k); });
  const depth = {}; shown.forEach((k) => { depth[k] = 0; });
  for (let i = order.length - 1; i >= 0; i--) { const a = order[i]; out[a].forEach((b) => { if (!back.has(a + '\0' + b) && depth[b] < depth[a] + 1) depth[b] = Math.min(4, depth[a] + 1); }); }
  const E = []; shown.forEach((k) => out[k].forEach((o) => E.push([k, o])));
  const lay = layered(shown.map((k, i) => ({ id: k, layer: depth[k], order: i })), E, { w: FW, h: FH, gx: 28, gy: 26, cols: 4, x0: 0, y0: 120, head: 0, pad: 0, gap: 30, sweeps: 6 });
  const pos = lay.pos;
  const fr = bbox(Object.values(pos));
  const frame = { x: fr.x - 30, y: 40, w: Math.max(fr.w + 60, 700), h: fr.y + fr.h - 40 + 50 };
  if (frame.w > fr.w + 60) frame.x = fr.x + fr.w / 2 - frame.w / 2;
  const l = layerOf(mod);
  S_('rect', { x: frame.x, y: frame.y, width: frame.w, height: frame.h, rx: 18, fill: 'var(--surface)', stroke: 'var(--line-2)', filter: 'url(#sh)' }, ge);
  S_('rect', { x: frame.x, y: frame.y, width: frame.w, height: frame.h, rx: 18, fill: LCOLOR[l], opacity: .045 }, ge);
  S_('rect', { x: frame.x + 18, y: frame.y + 18, width: 4, height: 30, rx: 2, fill: LCOLOR[l] }, ge);
  S_('text', { x: frame.x + 32, y: frame.y + 32, style: 'font-size:20px;font-weight:650;fill:var(--text)' }, ge, cutL(short(mod), 60));
  S_('text', { x: frame.x + 32, y: frame.y + 56, style: 'font-size:14px;fill:var(--text-2)' }, ge, `${cut(modDesc(mod) || modRole(mod), 70)} · ${t('shown')} ${shown.length} ${t('of')} ${all.length}`);
  // ports: inbound (top) and outbound (bottom)
  const outP = {}, inP = {};
  shown.forEach((k) => {
    const f = fnInfo(k);
    const part = (om) => !own.length || FOCUS.mods.has(om); // in a scenario: ports to its modules only
    f.out.forEach((o) => { const om = D.fns[o].module; if (om !== mod && part(om)) (outP[om] = outP[om] || []).push([k, o]); });
    f.in.forEach((o) => { const om = D.fns[o].module; if (om !== mod && part(om)) (inP[om] = inP[om] || []).push([o, k]); });
  });
  const cs = curStep();
  const keepPort = cs && cs.prev && D.fns[cs.prev] && D.fns[cs.prev].module !== mod && D.fns[cs.fid] && D.fns[cs.fid].module === mod ? D.fns[cs.prev].module : null;
  if (keepPort && !inP[keepPort]) inP[keepPort] = [[cs.prev, cs.fid]];
  const PL = 'font-size:11.5px;font-weight:600;letter-spacing:.06em;text-transform:uppercase;fill:var(--muted)';
  const portCol = (obj, y, dir) => {
    let ks = Object.keys(obj).sort((a, b) => obj[b].length - obj[a].length);
    if (keepPort && dir === 'in' && ks.includes(keepPort)) ks = [keepPort].concat(ks.filter((k) => k !== keepPort));
    ks = ks.slice(0, 6);
    const left = frame.x + frame.w / 2 - (ks.length * 222 - 22) / 2;
    ks.forEach((m, i) => {
      const r = { x: left + i * 222, y, w: 200, h: 40 };
      const n = S_('g', { class: 'n port', 'data-id': 'port:' + dir + ':' + m }, gn);
      S_('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: 20, class: 'card', style: 'stroke-dasharray:3 3' }, n);
      S_('circle', { cx: r.x + 18, cy: r.y + 20, r: 5, fill: LCOLOR[layerOf(m)] }, n);
      S_('text', { x: r.x + 32, y: r.y + 25, class: 't2 mono', style: 'font-size:14px' }, n, cutL(short(m), 21));
      S_('title', {}, n, modLabel(m));
      fv.ports[dir + ':' + m] = { el: n, r, m };
      const seen = new Set();
      obj[m].forEach(([a, b]) => {
        const key = dir === 'out' ? a : b; if (seen.has(key)) return; seen.add(key);
        const src = dir === 'out' ? pos[a] : r, dst = dir === 'out' ? r : pos[b];
        if (!src || !dst) return;
        const p1 = { x: src.x + src.w / 2, y: src.y + src.h }, p2 = { x: dst.x + dst.w / 2, y: dst.y };
        const k = Math.max(40, (p2.y - p1.y) / 2);
        const path = S_('path', { d: `M${p1.x},${p1.y} C${p1.x},${p1.y + k} ${p2.x},${p2.y - k} ${p2.x},${p2.y}`, class: 'e dash', 'marker-end': 'url(#arr)' }, ge);
        fv.edges.push({ a: dir === 'out' ? a : 'port:in:' + m, b: dir === 'out' ? 'port:out:' + m : b, path });
      });
    });
    return ks.length;
  };
  const nIn = portCol(inP, frame.y - 70, 'in');
  const nOut = portCol(outP, frame.y + frame.h + 34, 'out');
  if (nIn) S_('text', { x: frame.x, y: frame.y - 80, style: PL }, gn, t('inCalls'));
  if (nOut) S_('text', { x: frame.x, y: frame.y + frame.h + 24, style: PL }, gn, t('outCalls'));
  // intra-module edges
  E.forEach(([k, o]) => {
    const A = pos[k], B = pos[o]; let d;
    if (B.y > A.y + 2) { const p1 = { x: A.x + A.w / 2, y: A.y + A.h }, p2 = { x: B.x + B.w / 2, y: B.y }, ky = (p2.y - p1.y) / 2; d = `M${p1.x},${p1.y} C${p1.x},${p1.y + ky} ${p2.x},${p2.y - ky} ${p2.x},${p2.y}`; }
    else if (Math.abs(B.y - A.y) <= 2) { const p1 = { x: A.x + A.w / 2 + 30, y: A.y + A.h }, p2 = { x: B.x + B.w / 2 - 30, y: B.y + B.h }; d = `M${p1.x},${p1.y} C${p1.x},${p1.y + 40} ${p2.x},${p2.y + 40} ${p2.x},${p2.y}`; }
    else { const p1 = { x: A.x + (B.x > A.x ? A.w : 0), y: A.y + A.h / 2 }, p2 = { x: B.x + (B.x > A.x ? 0 : B.w), y: B.y + B.h / 2 }, kx = (p2.x - p1.x) / 2; d = `M${p1.x},${p1.y} C${p1.x + kx},${p1.y - 30} ${p2.x - kx},${p2.y - 30} ${p2.x},${p2.y}`; }
    fv.edges.push({ a: k, b: o, path: S_('path', { d, class: 'e' + (isAsync(k, o) ? ' dash' : ''), 'marker-end': 'url(#arr)' }, ge) });
  });
  // function cards
  shown.forEach((k) => {
    const r = pos[k], f = fnInfo(k), n = S_('g', { class: 'n ' + fnChgClass(k), 'data-id': k }, gn);
    S_('rect', { x: r.x, y: r.y, width: r.w, height: r.h, rx: 9, class: 'card' }, n);
    if (flowSet.has(k)) S_('rect', { x: r.x + 8, y: r.y + 10, width: 3, height: r.h - 20, rx: 1.5, fill: 'var(--accent)' }, n);
    S_('text', { x: r.x + 20, y: r.y + 26, class: 't1 mono', style: 'font-size:15px' }, n, (f.entry ? '⚑ ' : '') + cut(f.name, f.entry ? 25 : 27));
    // the second line: the description when the code has one, else where the function is
    const loc = `${(f.file || '').split('/').pop()}:${f.s}–${f.e} · ${f.e - f.s + 1} ${t('lines')}`, fd = fnDesc(k);
    S_('text', { x: r.x + 20, y: r.y + 50, class: 't3', style: 'font-size:12.5px' }, n, fd ? cut(fd, 38) : loc);
    if (srcOf(k)) S_('text', { x: r.x + r.w - 12, y: r.y + 24, 'text-anchor': 'end', class: 't3', style: 'font-size:10px' }, n, '{ }');
    if (f.status) S_('text', { x: r.x + r.w - 12, y: r.y + 50, 'text-anchor': 'end', class: 'badge', fill: f.status === 'added' ? 'var(--add)' : 'var(--mod)' }, n, f.status === 'added' ? '+' : '~');
    if (state.only && !f.status) n.classList.add('dim');
    S_('title', {}, n, k + '\n' + loc + (fd ? '\n' + fd : ''));
    fv.nodes[k] = { el: n, r };
  });
  if (all.length > 16) {
    const more = S_('text', { x: frame.x + frame.w - 20, y: frame.y + frame.h - 16, 'text-anchor': 'end', class: 't3', style: 'font-size:12px;fill:var(--accent);cursor:pointer' }, ge, state.fnAll ? t('lessFns') : t('moreFns', all.length - shown.length));
    more.addEventListener('click', (e) => { e.stopPropagation(); state.fnAll = !state.fnAll; buildFns(mod); fitFns(); });
  }
  const pr = Object.values(fv.ports).map((p) => p.r);
  const x1 = Math.min(frame.x, ...pr.map((r) => r.x)) - 10, x2 = Math.max(frame.x + frame.w, ...pr.map((r) => r.x + r.w)) + 10;
  fv.extent = { x: x1, y: frame.y - (nIn ? 100 : 10), w: x2 - x1, h: frame.h + (nIn ? 100 : 10) + (nOut ? 90 : 10) };
  fv.cam = new Cam(svg);
  fv.cam.insetTop = INSET; dockInsets(fv.cam); fv.cam.maxW = Math.max(fv.extent.w, fv.extent.h * fv.cam.aspect()) * 4;
  wireSelect(svg, fv.cam, selectFn, (id) => {
    if (id.startsWith('port:')) { goModule(id.split(':').slice(2).join(':')); return; }
    drill(3, { from: fv.nodes[id].el, fn: id });
  });
}
function selectFn(id) {
  const svg = fv.svg; clearSel(svg);
  if (!id) return setSel(null);
  svg.classList.add('has-sel');
  const nd = fv.nodes[id] || fv.ports[id.replace(/^port:/, '')];
  if (nd) nd.el.classList.add('sel');
  for (const e of fv.edges) if (e.a === id || e.b === id) {
    e.path.classList.add('rel');
    [e.a, e.b].forEach((x) => { const q = fv.nodes[x] || fv.ports[x.replace(/^port:/, '')]; if (q) q.el.classList.add('rel'); });
  }
  if (id.startsWith('port:')) setSel({ t: 'mod', id: id.split(':').slice(2).join(':') });
  else { state.fn = id; setSel({ t: 'fn', id }); }
}

/* ===================== Level 3: code (debugger) ===================== */
function buildCode(fid, target, frames) {
  const root = $('#sc3'); root.innerHTML = '';
  const f = fnInfo(fid);
  const wrap = H('div', { class: 'dbg' }, root);
  const st = H('div', { class: 'stack' }, wrap);
  H('div', { class: 'hd' }, st, `<span>${esc(t('path'))}</span>`);
  const ul = H('ul', { class: 'frames' }, st);
  (frames && frames.length ? frames : [fid]).forEach((k, i, a) => {
    const li = H('li', { class: i === a.length - 1 ? 'top' : '', title: k }, ul, `${esc(fnInfo(k).name)}<small>${esc(short(fnInfo(k).mod))}</small>`);
    li.onclick = () => { state.fn = k; buildCode(k, a[i + 1] || null, a.slice(0, i + 1)); setSel({ t: 'fn', id: k }); renderCrumbs(); };
  });
  const src = H('div', { class: 'src' }, wrap);
  H('div', { class: 'hd' }, src, `<b>${esc(f.name)}</b><span class="loc">${esc(f.file || '')}${f.s ? ':' + f.s + '–' + f.e : ''}</span>`);
  const c = codeHTML(fid, target);
  const code = H('div', { class: 'code fadein' }, src, c.html + (c.has ? '' : `<div class="nosrc">${esc(c.loading ? '' : t('noBodyHint'))}</div>`));
  if (c.hlIdx >= 0) requestAnimationFrame(() => { const ln = code.children[c.hlIdx]; if (ln) code.scrollTop = ln.offsetTop - code.clientHeight / 2 + 20; });
  if (c.loading) loadSrc(fid).then(() => { if (state.level === 3 && state.fn === fid) buildCode(fid, target, frames); });
}

/* ===================== drill in / out ===================== */
async function drill(lvl, ctx = {}, inner = false) {
  if (state.busy && !inner) return; const wasBusy = state.busy; state.busy = true;
  try {
    const from = state.level, rect = ctx.from ? relRect(ctx.from) : midRect();
    if (lvl === 1) { $('#sc1').classList.remove('hidden'); fitMap(ctx.focus); }
    if (lvl === 2) {
      if (U[ctx.mod] && U[ctx.mod].kind === 'g') { state.busy = wasBusy; return expandGroup(U[ctx.mod].node); }
      if (state.mod !== ctx.mod) state.fnAll = false;
      state.mod = ctx.mod; buildFns(ctx.mod); $('#sc2').classList.remove('hidden'); fitFns();
    }
    if (lvl === 3) { state.fn = ctx.fn; state.mod = D.fns[ctx.fn] ? D.fns[ctx.fn].module : state.mod; buildCode(ctx.fn, ctx.target, ctx.frames || defaultFrames(ctx.fn)); }
    state.level = lvl;
    await zoomSwap(from, lvl, rect, 'in');
    if (lvl === 1 && ctx.focus) { const u = focusNode(ctx.focus); if (u) selectMod(u); else setSel(null); }
    else if (lvl === 2) setSel({ t: 'mod', id: ctx.mod });
    else if (lvl === 3) setSel({ t: 'fn', id: ctx.fn });
    else setSel(null);
    afterLevel();
  } finally { state.busy = wasBusy; }
}
// focusRatio (auto zoom-out): the parent's camera is set so the card we leave takes this share of the screen
async function up(toLvl, inner = false, focusRatio = 0) {
  if (state.level === 0 || (state.busy && !inner)) return; const wasBusy = state.busy;
  const target = toLvl == null ? state.level - 1 : toLvl;
  state.busy = true;
  try {
    while (state.level > target) {
      const from = state.level, to = from - 1;
      SC()[to].classList.remove('hidden');
      let el = null;
      if (to === 0) el = sys.nodes.svc.el;
      if (to === 1) { const u = state.mod && unitFor(state.mod); el = u && map.nodes[u] && map.nodes[u].el; }
      if (to === 2) {
        const m = D.fns[state.fn] ? D.fns[state.fn].module : state.mod;
        if (!fv || fv.mod !== m) { state.mod = m; buildFns(m); fitFns(); }
        el = fv.nodes[state.fn] && fv.nodes[state.fn].el;
      }
      if (focusRatio && el && to <= 2) { const cam = [sys.cam, map.cam, fv.cam][to]; cam.set(cam.rectForRatio(cam.worldRect(el), focusRatio)); }
      const rect = el ? relRect(el) : midRect();
      state.level = to;
      await zoomSwap(from, to, rect, 'out');
    }
    if (state.level === 1 && state.mod && unitFor(state.mod)) selectMod(unitFor(state.mod));
    else if (state.level === 2 && state.fn && fv.nodes[state.fn]) selectFn(state.fn);
    else setSel(null);
    afterLevel();
  } finally { state.busy = wasBusy; }
}
// node id on the module map for a focus: module (revealed from its group), actor or external
function focusNode(f) {
  if (!f || !map) return null;
  if (map.nodes[f]) return f;
  if (D.allMods.includes(f) || D.mods[f]) { revealMod(f); return unitFor(f); }
  return null;
}
function fitMap(focus) {
  const id = focusNode(focus);
  if (FOCUS) { // the whole scenario at once, not blown up when it is small, clear of the step card on the right
    const keep = map.cam.insetRight; map.cam.insetRight = Math.max(keep, (PNL.w || 380) + 20);
    map.cam.fit({ ...map.extent }, 40, 1150); map.cam.insetRight = keep;
  }
  else if (id && map.pos[id]) { const r = map.pos[id]; map.cam.fit({ x: r.x - 380, y: r.y - 240, w: r.w + 760, h: r.h + 480 }, 20); }
  else map.cam.fitTop({ ...map.extent }, 40);
}
function fitFns() { fv.cam.fit(fv.extent, 10); }
function fitSys() { sys.cam.fit(sys.extent, 30); }
function defaultFrames(fn) {
  const fr = [fn]; let cur = fn;
  for (let i = 0; i < 3; i++) { const c = (D.callers[cur] || [])[0]; if (!c || fr.includes(c)) break; fr.unshift(c); cur = c; }
  return fr;
}
// from any level to the functions of module m via the map
async function goModule(m) {
  if (state.level > 1) await up(1);
  else if (state.level === 0) await drill(1, { from: sys.nodes.svc.el });
  revealMod(m);
  const u = unitFor(m); if (!u) return;
  selectMod(u);
  await map.cam.animTo(map.pos[u], 300, 0, 300);
  await drill(2, { from: map.nodes[u].el, mod: u });
}
// jumpToFn(fid, level 2|3): open the function on the map
async function jumpToFn(id, lvl = 2) {
  const f = D.fns[id]; if (!f) return;
  if (state.view !== 'map') setView('map');
  const m = f.module;
  if (state.level === 0) await drill(1, { from: sys.nodes.svc.el, focus: m });
  if (state.level >= 2 && state.mod !== m) await up(1);
  if (state.level === 3) await up(2);
  if (state.level === 1) { revealMod(m); const u = unitFor(m); if (u) await drill(2, { from: map.nodes[u].el, mod: u }); }
  if (!fv || !fv.nodes[id]) { state.fn = id; buildFns(m); fitFns(); }
  if (lvl === 3) await drill(3, { from: fv.nodes[id] ? fv.nodes[id].el : null, fn: id });
  else selectFn(id);
}
// setLevel(lvl, fid): put the map on level lvl at function fid (the current step's by default) at once, without the
// zoom transition — the Scenario tab uses it to keep the map on its level while the map is off screen
let LVL_QUIET = false; // the level was set off screen: the map puts back its play and selection marks when shown
function setLevel(lvl, fid) {
  if (state.busy || lvl < 0 || lvl > 3) return;
  LVL_QUIET = true;
  const s = curStep();
  // a planned step has no function: its caller stands for it
  const sf = s ? (D.fns[s.fid] ? s.fid : D.fns[s.prev] ? s.prev : null) : null;
  if (!fid || !D.fns[fid]) fid = sf || (state.fn && D.fns[state.fn] ? state.fn : null);
  if (!fid && lvl >= 2) { const m = state.mod && D.byMod[state.mod] ? state.mod : null; fid = m ? D.byMod[m][0] : D.entries[0] || D.fids[0] || null; }
  const m = fid ? D.fns[fid].module : null;
  if (lvl >= 2 && !m) lvl = 1;
  if (lvl === state.level && (lvl < 2 || (lvl === 2 ? fv && fv.mod === m && fv.nodes[fid] : state.fn === fid))) return;
  if (lvl >= 1) { if (m) revealMod(m); fitMap(m); }
  if (lvl >= 2) {
    const u = unitFor(m) || m;
    if (state.mod !== u) state.fnAll = false;
    state.mod = u; state.fn = fid;
    buildFns(u); fitFns();
  }
  if (lvl === 3) {
    const own = s && s.fid === fid;
    state.mod = m; buildCode(fid, own ? s.next : null, own ? framesUpTo(state.step) : defaultFrames(fid));
  }
  state.level = lvl;
  SC().forEach((e, i) => e.classList.toggle('hidden', i !== lvl));
  // marks on the scene only: the selection (and the panel) stay as the Scenario tab left them
  const keep = state.sel; QUIET = true;
  try {
    if (lvl === 1 && m && unitFor(m)) selectMod(unitFor(m));
    else if (lvl === 2 && fv && fv.nodes[fid]) selectFn(fid);
  } finally { QUIET = false; state.sel = keep; }
  afterLevel();
}
function afterLevel() {
  $('.zoomctl').style.display = state.level === 3 ? 'none' : '';
  renderCrumbs();
  drawPath(); // an open scenario: its path on the new level
  bus.emit('level', state.level);
}
// the crumbs in the header: where I am — system › modules › module › function (a click goes back up);
// on another view (API, Data, Statuses) its title, a click on the system returns to the map
function renderCrumbs() {
  const c = $('#crumbs'); if (!c) return;
  c.innerHTML = '';
  if (state.view !== 'map') {
    const v = VIEWS.find((x) => x.id === state.view);
    const b0 = H('button', { title: t('lv')[0] }, c, esc(t('lv')[0])); b0.onclick = () => setView('map');
    H('span', { class: 'sep' }, c, '›');
    H('button', { class: 'cur' }, c, esc(v ? viewTitle(v) : state.view));
    return;
  }
  const items = [[t('lv')[0], 0]];
  if (state.level >= 1) {
    items.push([t('lv')[1], 1]);
    if (G.on && state.level === 1) { const ch = []; for (let n = G.focus; n && n.parent; n = n.parent) ch.unshift(n); ch.forEach((n) => items.push([n.label + '/', 'g:' + n.id])); }
  }
  if (state.level >= 2 && state.mod) items.push([short(state.mod), 2]);
  if (state.level >= 3 && state.fn) items.push([fnInfo(state.fn).name, 3]);
  items.forEach(([tx, l], i) => {
    if (i) H('span', { class: 'sep' }, c, '›');
    const b = H('button', { class: i === items.length - 1 ? 'cur' : '', title: tx }, c, esc(tx));
    b.onclick = () => {
      if (typeof l === 'string') { focusGroup(G.TN[l.slice(2)]); return; }
      if (l === 1 && state.level === 1 && G.focus) { G.userOpen.clear(); G.focus = null; buildMap(); fitMap(); setSel(null); renderCrumbs(); return; }
      if (l < state.level) up(l);
    };
  });
}
function intoSelected() {
  const s = state.sel;
  if (state.level === 0) return drillFromSys(s && (s.t === 'actor' || s.t === 'ext') ? s.id : 'svc');
  if (state.level === 1) {
    if (s && s.t === 'group') return expandGroup(G.TN[s.id]);
    if (s && (s.t === 'actor' || s.t === 'ext')) { const m = (s.t === 'ext' ? (D.externals.find((x) => x.id === s.id) || {}).modules || [] : Array.from((D.actors.find((a) => a.id === s.id) || { mods: new Set() }).mods))[0]; if (m) return goModule(m); return; }
    const m = s && s.t === 'mod' ? s.id : (state.mod && unitFor(state.mod)) || units.filter((u) => u.kind === 'm').sort((a, b) => b.n - a.n)[0].id;
    return drill(2, { from: map.nodes[unitFor(m) || m] && map.nodes[unitFor(m) || m].el, mod: m });
  }
  if (state.level === 2) { const f = s && s.t === 'fn' && fv.nodes[s.id] ? s.id : Object.keys(fv.nodes)[0]; if (f) return drill(3, { from: fv.nodes[f].el, fn: f }); }
}
const curCam = () => [sys && sys.cam, map && map.cam, fv && fv.cam][state.level];
function fitCur() { if (state.level === 0) sys.cam.animTo(sys.extent, 30, 0, 300); if (state.level === 1) map.cam.animTo(map.extent, 10, 0, 300); if (state.level === 2) fv.cam.animTo(fv.extent, 10, 0, 300); }
// rebuildMap(keep): rebuild every level in place (language, data, resize) keeping level and selection.
// keep = true also keeps the cameras (live updates) and the selection marks on the scene.
function rebuildMap(keep) {
  const lvl = state.level, mod = state.mod, fn = state.fn;
  const cams = keep ? { sys: sys && { ...sys.cam.vb }, map: map && { ...map.cam.vb }, fv: fv && { mod: fv.mod, vb: { ...fv.cam.vb } } } : null;
  FOCUS = computeFocus(); if (!FOCUS) PRE = null;
  setupGroups();
  buildSystem(); buildMap();
  fitSys(); fitMap();
  if (lvl >= 2 && mod && D.byMod[mod]) { buildFns(mod); fitFns(); } else if (lvl >= 2) { state.level = Math.min(lvl, 1); }
  if (state.level >= 3 && fn && D.fns[fn]) { const s = curStep(); buildCode(fn, s && s.fid === fn ? s.next : null, s ? framesUpTo(state.step) : defaultFrames(fn)); } else if (state.level >= 3) state.level = 2;
  SC().forEach((e, i) => e.classList.toggle('hidden', i !== state.level));
  if (cams) {
    if (cams.sys) sys.cam.set(cams.sys);
    if (cams.map) map.cam.set(cams.map);
    if (cams.fv && fv && fv.mod === cams.fv.mod) fv.cam.set(cams.fv.vb);
    reapplySel(); restorePlay();
  }
  afterLevel();
}
// the selection marks live in the scene DOM: put them back after a rebuild without touching the panel
function reapplySel() {
  const s = state.sel; if (!s) return;
  QUIET = true;
  try {
    if (state.level === 0 && sys) { const id = s.t === 'svc' ? 'svc' : s.t === 'actor' || s.t === 'ext' ? s.id : null; if (id && sys.nodes[id]) selectSys(id); }
    else if (state.level === 1 && map) { const id = s.t === 'mod' || s.t === 'group' ? (unitFor(s.id) || s.id) : s.t === 'actor' || s.t === 'ext' ? s.id : null; if (id && map.nodes[id]) selectMod(id); }
    else if (state.level === 2 && fv && s.t === 'fn' && fv.nodes[s.id]) selectFn(s.id);
  } finally { QUIET = false; state.sel = s; }
}
// play marks of the current step on the visible level (no camera move, no dot)
function restorePlay() {
  const s = curStep(); if (!s || state.view !== 'map') return;
  const sc = [sys, map, fv][state.level]; if (!sc) return;
  clearSel(sc.svg); sc.svg.classList.add('play');
  if (state.level === 0) hotOn(sys.nodes, 'svc');
  else if (state.level === 1) { const b = unitFor(s.mod), a = s.prev && D.fns[s.prev] ? unitFor(D.fns[s.prev].module) : null; hotOn(map.nodes, b); if (a) { hotOn(map.nodes, a); hotEdge(map.edges, a, b); } }
  else if (state.level === 2) hotOn(fv.nodes, s.fid);
  drawPath();
}

Object.assign(AM, {
  map: {
    drill, up, goModule, jumpToFn, setLevel, setDock, marks: () => { if (LVL_QUIET) { LVL_QUIET = false; restorePlay(); reapplySel(); } }, revealMod, intoSelected, rebuild: rebuildMap, fit: fitCur,
    get level() { return state.level; }, get units() { return units; }, unitOf: (m) => unitFor(m), unitLayer: (id) => (U[id] ? unitLayer(U[id]) : null),
    get modules() { return map; }, get system() { return sys; }, get functions() { return fv; }, groups: G,
  },
});
