/* side.js — the sidebar, the base of the page: a docked column at the left (#side) with the full navigation of the
 * project as one tree, like the layers panel of Figma — names only, one line each with an ellipsis (the full name in
 * the tooltip), small grey counters only on sections and groups:
 *   System (the root) → Users (actors) → Modules by layer (Entry / Domain / Adapters / Infrastructure …; inside the
 *   modules, inside a module its functions, built only when unfolded) → Outside systems → Data (tables) → API
 *   (services → endpoints) → Statuses (entities) → Scenarios (through paths → their scenarios; then by actor).
 * A click on a node selects it on the map (going to its level) or opens its view in the centre (Data / API /
 * Statuses), and the inspector shows it. The tree follows the selection made on the map or in a view: it unfolds
 * down to the selected node and scrolls to it. The filter on top narrows the whole tree (by name).
 * Scenarios open apart, in a window of their own (openScenario: one reused window 'archmap-scenario', the same page
 * with #flow=<id> / #journey=<id> / #fn=<fid> and &solo=1; a blocked pop-up falls back to a new tab). The main window
 * never plays a scenario. The scenario window (SOLO) has no sidebar: the toolbar with the run bar, the map of the
 * participants only, the inspector.
 * The width is dragged by the right edge (200–420 px, localStorage archmap.side.w, double-click — 240); the toolbar
 * button or `[` hides the column (localStorage archmap.side). */

addI18n({
  ru: {
    nvSys: 'Система', nvActors: 'Пользователи', nvMods: 'Модули', nvExt: 'Внешние системы', nvData: 'Данные', nvApi: 'API', nvStatus: 'Статусы',
    nvScen: 'Сценарии', nvPaths: 'Сквозные пути', nvWin: 'Откроется в окне сценария',
    sdFilter: 'Фильтр', sdNone: 'Ничего не найдено', sdHide: 'Скрыть боковую панель ([)', sdShow: 'Показать боковую панель ([)',
    sdLabel: 'Навигация', sdGrip: 'Потяните, чтобы изменить ширину; двойной клик — по умолчанию', sdFold: 'Свернуть', sdUnfold: 'Развернуть',
    soloClose: 'Закрыть окно сценария',
  },
  en: {
    nvSys: 'System', nvActors: 'Users', nvMods: 'Modules', nvExt: 'Outside systems', nvData: 'Data', nvApi: 'API', nvStatus: 'Statuses',
    nvScen: 'Scenarios', nvPaths: 'Through paths', nvWin: 'Opens in the scenario window',
    sdFilter: 'Filter', sdNone: 'Nothing found', sdHide: 'Hide the sidebar ([)', sdShow: 'Show the sidebar ([)',
    sdLabel: 'Navigation', sdGrip: 'Drag to resize; double-click for the default', sdFold: 'Fold', sdUnfold: 'Unfold',
    soloClose: 'Close the scenario window',
  },
});

// open: nodes unfolded by hand (the root and Scenarios by default); last: the node clicked last (a view without a
// selection of its own — Statuses — keeps it marked)
const SIDE = { hidden: false, q: '', open: new Set(['sys', 'sec:scen', 'scp']), w: 0, last: null, lastView: null };
const SIDE_W = { def: 240, min: 200, max: 420 };
// the scenario window: the page opened with a scenario in the hash (#flow= / #journey= / #fn= / #run=)
const SOLO = /^#(journey|flow|fn|run)=/.test(location.hash || '');
const IC = (d) => `<svg class="ico" viewBox="0 0 14 14" aria-hidden="true">${d}</svg>`;
const SIDE_ICON = {
  chev: '<svg viewBox="0 0 8 8" aria-hidden="true"><path d="M2 .8L6.4 4 2 7.2z"/></svg>',
  sys: IC('<rect x="2" y="2" width="4" height="4" rx=".8"/><rect x="8" y="2" width="4" height="4" rx=".8"/><rect x="2" y="8" width="4" height="4" rx=".8"/><rect x="8" y="8" width="4" height="4" rx=".8"/>'),
  sec: IC('<path d="M4.5 1.5v11M9.5 1.5v11M1.5 4.5h11M1.5 9.5h11"/>'),
  layer: IC('<path d="M7 2l5 2.6L7 7.2 2 4.6z"/><path d="M2 7.2l5 2.6 5-2.6"/><path d="M2 9.6l5 2.6 5-2.6" opacity=".6"/>'),
  mod: IC('<rect x="2" y="3" width="10" height="8" rx="1.2"/><path d="M2 5.6h10"/>'),
  fn: IC('<path d="M5 3.5L2.5 7 5 10.5M9 3.5L11.5 7 9 10.5"/>'),
  role: IC('<circle cx="7" cy="4.6" r="2.2"/><path d="M2.8 12c.5-2.4 2.2-3.6 4.2-3.6s3.7 1.2 4.2 3.6"/>'),
  ext: IC('<path d="M5.5 3H3v8h8V8.5M8 2.5h3.5V6M11.5 2.5L6.5 7.5"/>'),
  table: IC('<rect x="2" y="2.5" width="10" height="9" rx="1"/><path d="M2 5.5h10M2 8.5h10M6 5.5v6"/>'),
  api: IC('<path d="M2 7h4M8 7h4"/><circle cx="7" cy="7" r="1.6"/><path d="M2 4v6M12 4v6"/>'),
  ep: IC('<path d="M3 7h8M8 4l3 3-3 3"/>'),
  status: IC('<circle cx="7" cy="7" r="4.5"/><circle cx="7" cy="7" r="1.6"/>'),
  path: IC('<circle cx="3" cy="3" r="1.5"/><circle cx="11" cy="11" r="1.5"/><path d="M4.5 3H9a2 2 0 010 4H5a2 2 0 000 4h4.5"/>'),
  flow: IC('<path d="M4.5 3.2v7.6L10.6 7z" stroke-linejoin="round"/>'),
};

/* ---------- texts ---------- */
// a description written for people, not the one the model builds from the name ("Actor: Name -> exit",
// "http: path through …"): those say nothing the name does not already say (the inspector shows the real one)
function realDesc(x) {
  const d = String((x && x.desc) || '').trim(), nm = String((x && x.name) || '').trim();
  if (!d || d === nm) return '';
  if (x.actor && d.startsWith(x.actor + ': ' + nm)) return '';
  if (/^[\w-]+: path through /.test(d)) return '';
  return d;
}
// an automatic path is named "<first flow> → … (N flows)": the tree shows the name without the tail
const pathName = (j) => String(j.name || j.id).replace(/\s*→\s*….*$/, '');

/* ---------- scenario groups ---------- */
// the visible scenarios by the actor that starts them, in the order of D.actors; the rest go to "other roles"
function sideGroups() {
  const by = new Map(), other = { id: 'actor:others', name: t('actOthers'), fls: [] };
  D.actors.forEach((a) => by.set(a.id, { id: a.id, name: actorName(a), fls: [] }));
  if (by.has('actor:others')) other.fls = by.get('actor:others').fls;
  const byName = new Map(); D.actors.forEach((a) => byName.set(actorName(a).trim().toLowerCase(), a.id));
  const fls = visibleFlows().slice().sort((a, b) => (b.kind === 'declared') - (a.kind === 'declared') || (b.top ? 1 : 0) - (a.top ? 1 : 0) || a.i - b.i);
  fls.forEach((fl) => {
    const a = (fl.actor && byName.get(fl.actor.trim().toLowerCase())) || (fl.steps && fl.steps[0] && (actorOfFn(fl.steps[0]) || {}).id);
    const g = (a && by.get(a)) || other;
    g.fls.push(fl);
  });
  const out = Array.from(by.values()).filter((g) => g.id !== 'actor:others' && g.fls.length);
  if (other.fls.length) out.push(other);
  return out;
}
// the scenarios of a path with the index of their first step
function pathSegs(key) {
  const r = state.run && state.run.key === key ? state.run : jvBuildJourney(key);
  if (!r) return [];
  let at = 0;
  return r.segs.map((sg, si) => { const s0 = at; at += sg.steps.length; return { si, at: s0, name: sg.flow ? sg.flow.name : r.name }; });
}

/* ---------- the scenario window and the hash ---------- */
const flowKey = (fl) => 'flow:' + fl.i;
const flowHashId = (fl) => fl.id || String(fl.i);
function keyHash(key) {
  if (!key) return '';
  if (key.startsWith('journey:')) return '#journey=' + encodeURIComponent(key.slice(8));
  if (key.startsWith('flow:')) { const fl = D.flows.find((x) => x.i === +key.slice(5)); return fl ? '#flow=' + encodeURIComponent(flowHashId(fl)) : ''; }
  if (key.startsWith('fn:')) return '#fn=' + encodeURIComponent(key.slice(3));
  return '#run=' + encodeURIComponent(key);
}
// the run the hash names and the step to start at: {key, step}; null when it names nothing known
function hashRun() {
  const h = location.hash || '', m = /^#(journey|flow|fn|run)=([^&]+)/.exec(h);
  if (!m) return null;
  const id = decodeURIComponent(m[2]), st = /&step=(\d+)/.exec(h), step = st ? +st[1] : -1;
  let key = null;
  if (m[1] === 'journey') key = jvJourney(id) ? 'journey:' + id : null;
  else if (m[1] === 'flow') { const fl = D.flows.find((x) => x.id === id) || (/^\d+$/.test(id) ? D.flows.find((x) => x.i === +id) : null); key = fl ? flowKey(fl) : null; }
  else if (m[1] === 'fn') key = D.fns[id] ? 'fn:' + id : null;
  else key = buildRun(id) ? id : null;
  return key ? { key, step } : null;
}
// the scenario window keeps its run in the hash (a reload shows the same); the main window writes nothing
function writeHash() {
  if (!SOLO || !state.run) return;
  const want = keyHash(state.run.key) + '&solo=1';
  if ((location.hash || '') === want) return;
  try { history.replaceState(null, '', want); } catch (e) { /* file:// in some browsers */ }
}
// openScenario(key, step): the scenario in its own window — one reused window; inside it, in place
function openScenario(key, step) {
  closeMenu();
  if (SOLO) { openRunAt(key, step); return; }
  const h = keyHash(key); if (!h) return;
  const url = location.href.split('#')[0] + h + '&solo=1' + (step >= 0 ? '&step=' + step : '');
  let w = null;
  try { w = window.open(url, 'archmap-scenario'); } catch (e) { w = null; }
  if (w) { try { w.focus(); } catch (e) { /* ignore */ } return; }
  // the pop-up is blocked: a plain link to a new tab
  const a = H('a', { href: url, target: '_blank', rel: 'opener' }, document.body); a.click(); a.remove();
}
async function openRunAt(key, step) {
  if (!state.run || state.run.key !== key) await openRun(key);
  if (step >= 0) { for (let i = 0; i < 40 && state.busy; i++) await sleep(100); AM.play.go(step); }
}
// the scenario window: no sidebar, the toolbar title is the run, ✕ closes the window
function setupSolo() {
  document.body.classList.add('solo');
  SIDE.hidden = true; $('main').classList.add('noside');
  $('#pClose').onclick = () => { window.close(); setTimeout(() => { if (!window.closed) location.href = location.href.split('#')[0]; }, 200); };
  bus.on('flow', () => { if (state.run) document.title = state.run.name + ' — archmap'; writeHash(); });
  bus.on('lang', () => { $('#pClose').title = t('soloClose'); });
  window.addEventListener('hashchange', () => { const r = hashRun(); if (r) openRunAt(r.key, r.step); });
}

/* ---------- the tree ---------- */
// a node: {key, name, title?, icon, count?, kids?: [] | () => [], act?: () => …, cls?}
function navTree() {
  const PJ = D.F.project || {}, kids = [];
  const N = (key, name, icon, o) => Object.assign({ key, name, icon }, o || {});
  if (D.actors.length) kids.push(N('sec:actors', t('nvActors'), 'sec', { count: D.actors.length, kids: D.actors.map((a) => N('a:' + a.id, actorName(a), 'role', { act: () => navSys(a.id) })) }));
  const present = LAYERS.filter((l) => D.allMods.some((m) => layerOf(m) === l));
  kids.push(N('sec:mods', t('nvMods'), 'sec', { count: D.allMods.length, kids: present.map((l) => {
    const ms = D.allMods.filter((m) => layerOf(m) === l).sort((a, b) => (short(a) < short(b) ? -1 : 1));
    return N('L:' + l, t('layers')[l], 'layer', { count: ms.length, kids: ms.map((m) => N('m:' + m, short(m), 'mod', { title: modLabel(m), act: () => navMod(m),
      kids: (D.byMod[m] || []).length ? () => (D.byMod[m] || []).map((id) => N('f:' + id, fnInfo(id).name, 'fn', { title: id, cls: 'mono', act: () => jumpToFn(id, 2) })) : null })) });
  }) }));
  if (D.externals.length) kids.push(N('sec:ext', t('nvExt'), 'sec', { count: D.externals.length, kids: D.externals.map((x) => N('x:' + x.id, x.name, 'ext', { act: () => navSys(x.id) })) }));
  const tbs = Object.keys(D.tables || {}).sort();
  if (tbs.length && hasView('data')) kids.push(N('sec:data', t('nvData'), 'sec', { count: tbs.length, act: () => setView('data'), kids: tbs.map((n) => N('t:' + n, n, 'table', { cls: 'mono', act: () => { setView('data'); bus.emit('data:focus', { table: n }); } })) }));
  const eps = D.endpoints || [];
  if (eps.length && hasView('api')) {
    const by = new Map();
    eps.forEach((e, i) => { const k = e.service || (e.kind === 'grpc' ? 'gRPC' : 'HTTP'); if (!by.has(k)) by.set(k, []); by.get(k).push(i); });
    kids.push(N('sec:api', t('nvApi'), 'sec', { count: eps.length, act: () => setView('api'), kids: Array.from(by).map(([svc, is]) => N('svc:' + svc, svc, 'api', { count: is.length,
      kids: () => is.map((i) => { const e = eps[i]; return N('e:' + i, ((e.method || e.kind || '') + ' ' + (e.path || e.name || '')).trim(), 'ep', { cls: 'mono', act: () => { setView('api'); setSel({ t: 'ep', id: i }); } }); }) })) }));
  }
  const ents = Object.keys(D.states || {});
  if (ents.length && hasView('status')) kids.push(N('sec:status', t('nvStatus'), 'sec', { count: ents.length, act: () => setView('status'), kids: ents.map((en) => N('s:' + en, en, 'status', { act: () => { setView('status'); bus.emit('status:focus', { machine: en }); } })) }));
  // scenarios: through paths (a scenario of a path opens the path at its first step), then by actor
  const sk = [], js = jvJourneys();
  if (js.length) sk.push(N('scp', t('nvPaths'), 'sec', { count: js.length, kids: js.map((j) => { const key = 'journey:' + j.id; return N('r:' + key, pathName(j), 'path', { title: j.name, win: true, act: () => openScenario(key),
    kids: () => pathSegs(key).map((sg) => N('r:' + key + '@' + sg.at, sg.name, null, { no: sg.si + 1, win: true, act: () => openScenario(key, sg.at) })) }); }) }));
  sideGroups().forEach((g) => sk.push(N('scg:' + g.id, g.name, 'role', { count: g.fls.length, kids: g.fls.map((fl) => N('r:' + flowKey(fl), fl.name, 'flow', { win: true, act: () => openScenario(flowKey(fl)) })) })));
  if (sk.length) kids.push(N('sec:scen', t('nvScen'), 'sec', { count: visibleFlows().length, kids: sk }));
  return N('sys', PJ.name || D.M.title || D.F.repo || t('nvSys'), 'sys', { title: t('nvSys'), act: () => navSys(null), kids });
}
const kidsOf = (n) => (typeof n.kids === 'function' ? n.kids() : n.kids || []);

/* ---------- actions ---------- */
async function navSys(id) {
  if (state.view !== 'map') setView('map');
  if (state.level > 0) await up(0);
  if (id && sys.nodes[id]) selectSys(id); else setSel(null);
}
async function navMod(m) {
  if (state.view !== 'map') setView('map');
  if (state.level === 0) await drill(1, { from: sys.nodes.svc.el, focus: m });
  else if (state.level > 1) await up(1);
  openMod(m);
}

/* ---------- the selection in the tree ---------- */
// the node of the current selection (map or view) and the nodes above it
function selKey() {
  const s = state.sel;
  if (s) {
    if (s.t === 'svc') return 'sys';
    if (s.t === 'actor') return 'a:' + s.id;
    if (s.t === 'ext') return 'x:' + s.id;
    if (s.t === 'mod') return 'm:' + s.id;
    if (s.t === 'fn') return 'f:' + s.id;
    if (s.t === 'table') return 't:' + s.id;
    if (s.t === 'ep') return 'e:' + s.id;
    if (s.t === 'status') return 's:' + s.entity;
  }
  if (state.view === 'map' && !s) {
    if (state.level === 2 && state.mod) return 'm:' + state.mod;
    if (state.level === 3 && state.fn) return 'f:' + state.fn;
    if (state.level === 0) return 'sys';
  }
  if (SIDE.last && SIDE.lastView === state.view) return SIDE.last;
  if (state.view !== 'map') return 'sec:' + state.view;
  return null;
}
function ancestors(key) {
  const a = ['sys'];
  if (!key) return a;
  const k = key.slice(0, key.indexOf(':') + 1), id = key.slice(k.length);
  if (k === 'a:') a.push('sec:actors');
  else if (k === 'x:') a.push('sec:ext');
  else if (k === 'm:') a.push('sec:mods', 'L:' + layerOf(id));
  else if (k === 'f:' && D.fns[id]) a.push('sec:mods', 'L:' + layerOf(D.fns[id].module), 'm:' + D.fns[id].module);
  else if (k === 't:') a.push('sec:data');
  else if (k === 's:') a.push('sec:status');
  else if (k === 'e:') { const e = (D.endpoints || [])[+id]; if (e) a.push('sec:api', 'svc:' + (e.service || (e.kind === 'grpc' ? 'gRPC' : 'HTTP'))); }
  return a;
}
// unfold the tree down to the selected node and scroll to it
function revealSel() {
  const k = selKey(); if (!k) return;
  ancestors(k).forEach((x) => SIDE.open.add(x));
  SIDE.reveal = true;
}

/* ---------- render ---------- */
function treeRow(n, d, open, hasKids, cur) {
  const chev = hasKids ? `<span class="chev${open ? ' on' : ''}" data-tog="${esc(n.key)}" title="${esc(t(open ? 'sdFold' : 'sdUnfold'))}">${SIDE_ICON.chev}</span>` : '<span class="chev"></span>';
  const tip = (n.title || n.name) + (n.win ? '\n' + t('nvWin') : '');
  return `<button type="button" class="ti${n.cls ? ' ' + n.cls : ''}${n.key.startsWith('sec:') || n.key === 'scp' || n.key === 'sys' ? ' sec' : ''}${cur ? ' cur' : ''}" style="--d:${d}" data-k="${esc(n.key)}" title="${esc(tip)}"${hasKids ? ` aria-expanded="${open}"` : ''}${cur ? ' aria-current="true"' : ''}>` +
    chev + (n.icon ? SIDE_ICON[n.icon] : '') + (n.no != null ? `<span class="no">${n.no}</span>` : '') + `<span class="nm">${esc(n.name)}</span>` + (n.count != null ? `<span class="ct">${n.count}</span>` : '') + '</button>';
}
let NAV = new Map(); // key -> node, for the clicks
function renderSide() {
  const box = $('#sideList'); if (!box || !D.F || SOLO) return;
  const top = box.scrollTop, qq = lc(SIDE.q).trim(), cur = selKey(), out = [];
  NAV = new Map();
  // with a filter: a node shows when its name or a name under it matches; the matching branches are unfolded
  const hit = (n) => lc(n.name).includes(qq);
  const walk = (n, d) => {
    NAV.set(n.key, n);
    const ks = kidsOf(n), hasKids = ks.length > 0;
    let sub = [];
    if (qq) {
      const mine = hit(n), rows = [];
      ks.forEach((c) => { const r = walk(c, d + 1); if (r) rows.push(r); });
      if (!mine && !rows.length) return '';
      return treeRow(n, d, rows.length > 0, hasKids, n.key === cur) + rows.join('');
    }
    const open = hasKids && SIDE.open.has(n.key);
    if (open) sub = ks.map((c) => walk(c, d + 1));
    return treeRow(n, d, open, hasKids, n.key === cur) + sub.join('');
  };
  const h = walk(navTree(), 0);
  box.innerHTML = h || `<div class="sn">${esc(t('sdNone'))}</div>`;
  box.scrollTop = top;
  const c = box.querySelector('.ti.cur');
  if (c && SIDE.reveal) { SIDE.reveal = false; c.scrollIntoView({ block: 'nearest' }); }
}
function paintSide() {
  const m = $('main'); m.classList.toggle('noside', SIDE.hidden);
  const b = $('#sideTog'), tip = t(SIDE.hidden ? 'sdShow' : 'sdHide');
  b.title = tip; b.setAttribute('aria-label', tip); b.setAttribute('aria-expanded', String(!SIDE.hidden));
  $('#side').toggleAttribute('inert', SIDE.hidden);
  const q = $('#sideQ'); q.placeholder = t('sdFilter'); q.setAttribute('aria-label', t('sdFilter'));
  $('#side').setAttribute('aria-label', t('sdLabel'));
  const g = $('#sideGrip'); g.title = t('sdGrip'); g.setAttribute('aria-label', t('sdGrip'));
  paintHName();
}
// the project name in the centre of the toolbar: only while the sidebar (which has it on top) is hidden and no run is open
function paintHName() { const n = $('#hName'); if (n) n.hidden = SOLO || !SIDE.hidden || !!state.run; }
// the canvas changed its width: the scenes take the new size
const stageResized = () => { rebuildMap(); bus.emit('resize'); };
function setSideHidden(h) {
  if (SOLO) return;
  SIDE.hidden = !!h;
  try { localStorage.setItem('archmap.side', h ? '0' : '1'); } catch (e) { /* file:// without storage */ }
  paintSide();
  stageResized();
}
function setSideWidth(w, save) {
  SIDE.w = w ? Math.max(SIDE_W.min, Math.min(SIDE_W.max, Math.round(w))) : 0;
  if (SIDE.w && SIDE.w !== SIDE_W.def) $('main').style.setProperty('--sw', SIDE.w + 'px'); else $('main').style.removeProperty('--sw');
  if (save) { try { if (SIDE.w && SIDE.w !== SIDE_W.def) localStorage.setItem('archmap.side.w', String(SIDE.w)); else localStorage.removeItem('archmap.side.w'); } catch (e) { /* ignore */ } }
}
function setupSideGrip() {
  const grip = $('#sideGrip');
  grip.addEventListener('pointerdown', (e) => {
    if (e.button !== 0) return;
    e.preventDefault();
    const x0 = e.clientX, w0 = $('#side').getBoundingClientRect().width;
    grip.classList.add('drag'); $('main').classList.add('presize');
    try { grip.setPointerCapture(e.pointerId); } catch (er) { /* synthetic */ }
    const mv = (ev) => setSideWidth(w0 + (ev.clientX - x0));
    const up = () => { grip.removeEventListener('pointermove', mv); grip.removeEventListener('pointerup', up); grip.removeEventListener('pointercancel', up); grip.classList.remove('drag'); $('main').classList.remove('presize'); setSideWidth(SIDE.w, true); stageResized(); };
    grip.addEventListener('pointermove', mv); grip.addEventListener('pointerup', up); grip.addEventListener('pointercancel', up);
  });
  grip.addEventListener('dblclick', () => { setSideWidth(0, true); stageResized(); });
  grip.addEventListener('keydown', (e) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    e.preventDefault(); e.stopPropagation();
    setSideWidth($('#side').getBoundingClientRect().width + (e.key === 'ArrowRight' ? 16 : -16), true); stageResized();
  });
}
function setupSide() {
  if (SOLO) { setupSolo(); paintSide(); return; }
  let v = null, w = 0; try { v = localStorage.getItem('archmap.side'); w = +localStorage.getItem('archmap.side.w'); } catch (e) { /* ignore */ }
  SIDE.hidden = v === '0';
  if (w) setSideWidth(w);
  const box = $('#sideList');
  box.addEventListener('click', (e) => {
    const tg = e.target.closest('[data-tog]');
    if (tg) { const k = tg.getAttribute('data-tog'); if (SIDE.open.has(k)) SIDE.open.delete(k); else SIDE.open.add(k); renderSide(); return; }
    const r = e.target.closest('[data-k]'); if (!r) return;
    const n = NAV.get(r.getAttribute('data-k')); if (!n) return;
    closeMenu();
    if (n.act) {
      if (!n.win) { SIDE.last = n.key; SIDE.lastView = null; }
      Promise.resolve(n.act()).then(() => { if (!n.win) { SIDE.lastView = state.view; renderSide(); } });
      if (!n.win) renderSide();
    } else if (kidsOf(n).length) { if (SIDE.open.has(n.key)) SIDE.open.delete(n.key); else SIDE.open.add(n.key); renderSide(); }
  });
  $('#sideQ').addEventListener('input', (e) => { SIDE.q = e.target.value; renderSide(); });
  $('#sideQ').addEventListener('keydown', (e) => { if (e.key === 'Escape') { e.stopPropagation(); if (e.target.value) { e.target.value = ''; SIDE.q = ''; renderSide(); } else e.target.blur(); } });
  $('#sideTog').onclick = () => setSideHidden(!SIDE.hidden);
  setupSideGrip();
  window.addEventListener('keydown', (e) => {
    const tg = e.target.tagName;
    if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT' || e.target.isContentEditable || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === '[') { e.preventDefault(); setSideHidden(!SIDE.hidden); }
  });
  // the tree follows the selection on the map and in the views
  const follow = () => { revealSel(); renderSide(); };
  ['select', 'level', 'view'].forEach((ev) => bus.on(ev, follow));
  bus.on('lang', () => { paintSide(); renderSide(); });
  bus.on('data', () => { paintSide(); renderSide(); });
  paintSide(); renderSide();
}

Object.assign(AM, { side: { render: renderSide, setHidden: setSideHidden, setWidth: setSideWidth, get hidden() { return SIDE.hidden; }, get width() { return SIDE.w || SIDE_W.def; }, groups: sideGroups, realDesc, openScenario, solo: SOLO } });
