/* nav.js — the header controls: the ⋯ menu (views API / Data / Statuses, language, theme, only changed), going
 * back. Also: the flash of what a live update changed (archmap serve). There is no search: the scenarios are in the
 * list at the left (side.js), the views open from the panels and from ⋯. */

addI18n({
  ru: {
    moreTip: 'Ещё: виды, язык, тема',
    mnView: (v) => v, mnLang: 'Язык: English', mnTheme: (d) => (d ? 'Тема: светлая' : 'Тема: тёмная'), mnOnly: (on) => (on ? 'Показать все, не только изменённые' : 'Только изменённые в ветке'),
    pkPath: 'сквозной путь', pkStepsN: (n, p) => n + ' ' + p(['шаг', 'шага', 'шагов']),
    ms: 'мс', changedN: 'изм.', noConnTip: 'Сервер archmap не отвечает; страница переподключится сама',
  },
  en: {
    moreTip: 'More: views, language, theme',
    mnView: (v) => v, mnLang: 'Язык: русский', mnTheme: (d) => (d ? 'Theme: light' : 'Theme: dark'), mnOnly: (on) => (on ? 'Show all, not only changed' : 'Only changed on the branch'),
    pkPath: 'through path', pkStepsN: (n) => n + (n === 1 ? ' step' : ' steps'),
    ms: 'ms', changedN: 'changed', noConnTip: 'The archmap server does not answer; the page reconnects by itself',
  },
});
const lc = (s) => String(s || '').toLowerCase();
// the actor of a flow: declared, else the actor of its first step
const flowActor = (fl) => { if (fl.actor) return fl.actor; const a = fl.steps && fl.steps.length ? actorOfFn(fl.steps[0]) : null; return a ? actorName(a) : ''; };

/* ---------- ⋯ menu ---------- */
function menuItems() {
  const dark = document.documentElement.getAttribute('data-theme') === 'dark';
  const out = VIEWS.filter((v) => v.id !== 'map' && v.menu !== false).map((v) => ({ nm: t('mnView', viewTitle(v)), cur: state.view === v.id, run: () => { if (state.view === v.id) setView('map'); else setView(v.id); } }));
  if (out.length) out.push(null);
  out.push({ nm: t('mnLang'), run: () => setLang(LANG === 'ru' ? 'en' : 'ru') });
  out.push({ nm: t('mnTheme', dark), run: () => setTheme(dark ? 'light' : 'dark') });
  if (D.changed.length || D.deleted.length) out.push({ nm: t('mnOnly', state.only), run: () => setOnly(!state.only) });
  return out;
}
let MENU = [];
function openMenu() {
  const m = $('#menu'); MENU = menuItems();
  m.innerHTML = MENU.map((it, i) => (it ? `<button role="menuitem" data-i="${i}">${esc(it.nm)}${it.cur ? '<span class="ck">✓</span>' : ''}</button>` : '<hr>')).join('');
  m.hidden = false; $('#moreBtn').setAttribute('aria-expanded', 'true');
  const f = m.querySelector('button'); if (f) f.focus();
}
function closeMenu() { const m = $('#menu'); if (!m || m.hidden) return; m.hidden = true; $('#moreBtn').setAttribute('aria-expanded', 'false'); }
function setupMenu() {
  const b = $('#moreBtn');
  b.title = t('moreTip'); b.setAttribute('aria-label', t('moreTip'));
  b.addEventListener('click', () => { if ($('#menu').hidden) openMenu(); else closeMenu(); });
  $('#menu').addEventListener('click', (e) => { const r = e.target.closest('button[data-i]'); if (!r) return; const it = MENU[+r.dataset.i]; closeMenu(); if (it) it.run(); });
  $('#menu').addEventListener('keydown', (e) => {
    const bs = $$('#menu button'), i = bs.indexOf(document.activeElement);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); e.stopPropagation(); const n = bs[(i + (e.key === 'ArrowDown' ? 1 : bs.length - 1)) % bs.length]; if (n) n.focus(); }
    else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeMenu(); b.focus(); }
  });
  document.addEventListener('pointerdown', (e) => { if (!e.target.closest('#more')) closeMenu(); }, true);
  window.addEventListener('blur', closeMenu);
  bus.on('lang', () => { b.title = t('moreTip'); b.setAttribute('aria-label', t('moreTip')); });
}

/* ---------- back: one stack of levels ----------
 * Every step down — a module or a function on the map (drill), a scenario opened from anywhere (openRun), a frame
 * of a page opened on the whole canvas — first remembers where we were (navPush): the view, the page, the scenario
 * and its step, the map level, module, function, camera, selection and the place on screen the step down started
 * from. Esc, "‹ <where>" in the run bar and the browser's Back go to exactly that place (navRestore): a scenario
 * entered from a page returns to the page, a module entered from a scenario step returns to the scenario at that
 * step. Each push is a history entry, so Back and Forward of the browser walk the same stack.
 * A detail picked in the inspector on top (a function, a module…) is closed by the first Esc. */
addI18n({ ru: { navBack: (s) => 'Назад: ' + s + ' (Esc)' }, en: { navBack: (s) => 'Back: ' + s + ' (Esc)' } });
const BACK = { stack: [], restoring: false, hold: 0, hist: false, base: '', ptr: null };
const navSelKey = (s) => (s ? JSON.stringify(s) : '');
const navKey = (s) => [s.view, s.page, s.run, s.run ? s.step : '', s.view === 'map' ? s.level : '', s.view === 'map' && s.level >= 2 ? s.mod : '', s.view === 'map' && s.level >= 3 ? s.fn : ''].join('|');
const navCam = () => (state.view === 'map' ? curCam() : null);
// where we are now
function navSnap() {
  const cam = navCam();
  return {
    view: state.view, page: state.view === 'page' && AM.pages ? AM.pages.cur : null,
    run: state.run ? state.run.key : null, step: state.step, level: state.level, mod: state.mod, fn: state.fn,
    cam: cam ? { ...cam.vb } : null, sel: state.sel, rect: null,
    pcam: state.view === 'page' && AM.pages && AM.pages.camVb ? AM.pages.camVb() : null,
  };
}
// the place a step down started from: the element last pressed (a node, a frame, a row of the tree), when recent
function navFrom() {
  const p = BACK.ptr; if (!p || performance.now() - p.t > 1500) return null;
  const s = stageRect(); if (!s.width) return null;
  return { x: p.r.left - s.left, y: p.r.top - s.top, w: Math.max(24, p.r.width), h: Math.max(18, p.r.height) };
}
// a short name of a place for "‹ <where>"
function navLabel(s) {
  if (!s) return '';
  if (s.view === 'page') { const pg = AM.pageList ? AM.pageList().find((p) => p.id === s.page || p.id === 'p:' + s.page) : null; return pg ? pg.name : t('lv')[0]; }
  if (s.view !== 'map') { const v = VIEWS.find((x) => x.id === s.view); return v ? viewTitle(v) : s.view; }
  if (s.run) { const r = buildRun(s.run); return r ? cut(r.name, 22) + (s.step >= 0 ? ' · ' + t('stepOf', s.step + 1, r.segs.reduce((n, sg) => n + sg.steps.length, 0)) : '') : ''; }
  if (s.level >= 3 && s.fn) return fnInfo(s.fn).name;
  if (s.level >= 2 && s.mod) return short(s.mod);
  return t('lv')[s.level] || t('lv')[0];
}
// navPush(): remember where we are before a step down (inside navBatch only once; the same place is not repeated)
function navPush() {
  if (BACK.restoring || BACK.hold > 1) return;
  if (BACK.hold === 1) BACK.hold = 2; // the batch has pushed
  const s = navSnap(); s.rect = navFrom();
  const top = BACK.stack[BACK.stack.length - 1];
  if (top && navKey(top) === navKey(s)) { top.cam = s.cam; top.sel = s.sel; if (s.rect) top.rect = s.rect; return; }
  BACK.stack.push(s);
  if (BACK.hist) try { history.pushState({ amNav: BACK.stack.length }, '', location.href); } catch (e) { BACK.hist = false; }
  renderPlayer();
}
// navBatch(fn): a step down made of several (a module from the system level, a frame to its view) is one place back
async function navBatch(fn) {
  if (BACK.hold || BACK.restoring) return fn();
  BACK.hold = 1;
  try { return await fn(); } finally { BACK.hold = 0; }
}
const navDepth = () => BACK.stack.length;
// the place Esc goes back to: the deepest remembered one that is not where we are now
function navTarget() {
  const cur = navKey(navSnap());
  let n = BACK.stack.length - 1;
  while (n >= 0 && navKey(BACK.stack[n]) === cur) n--;
  return n;
}
const navBackLabel = () => { const n = navTarget(); return n >= 0 ? navLabel(BACK.stack[n]) : ''; };
// zoom a scene out of / into the rect r of the stage (the place a step down started from)
function navZoom(el, r, dir) {
  if (!el || !el.animate || RM || !DUR) return;
  const from = r ? (dir === 'in' ? tfFor(r) : tfZoomInto(r)) : dir === 'in' ? 'scale(.94)' : 'scale(1.06)';
  el.animate([{ transform: from, opacity: 0.35, transformOrigin: '0 0' }, { transform: 'none', opacity: 1, transformOrigin: '0 0' }], { duration: 360, easing: 'cubic-bezier(.3,0,.2,1)' });
}
const navScene = () => (state.view === 'map' ? SC()[state.level] : viewEl(state.view));
async function navRestore(s) {
  BACK.restoring = true;
  try {
    for (let i = 0; i < 40 && state.busy; i++) await sleep(50);
    const fromRun = state.run && !s.run;
    if (s.view !== 'map') {
      if (state.run) closeRun();
      if (s.view === 'page' && AM.pages) { AM.pages.open(s.page); if (s.pcam) AM.pages.setCam(s.pcam); } else setView(s.view);
      if (s.sel) setSel(s.sel);
      navZoom(navScene(), s.rect, 'out');
      return;
    }
    if (state.view !== 'map') setView('map');
    const rk = state.run ? state.run.key : null;
    if (rk !== s.run) { if (s.run) { loadRun(s.run); if (s.step < 0) showOverview(); } else closeRun(); }
    if (state.level > s.level) await up(s.level);
    else if (state.level < s.level && s.level >= 2 && s.mod) { if (s.level === 3 && s.fn) await jumpToFn(s.fn, 3); else await goModule(s.mod); }
    if (s.level === 2 && s.mod && fv && fv.mod !== s.mod && D.byMod[s.mod]) { state.mod = s.mod; buildFns(s.mod); fitFns(); afterLevel(); }
    if (s.run && state.run) {
      if (s.step >= 0 && state.step !== s.step) await AM.play.go(s.step);
      else if (s.step < 0 && state.step >= 0) { state.step = -1; trimSegs(); showOverview(); }
    }
    const cam = navCam(); if (cam && s.cam) cam.set(s.cam);
    if (s.sel && navSelKey(s.sel) !== navSelKey(state.sel)) setSel(s.sel);
    if (fromRun) navZoom(navScene(), s.rect, 'out');
  } finally {
    BACK.restoring = false; BACK.base = navSelKey(state.sel);
    renderPlayer();
  }
}
// go back to snapshot n (the stack keeps what is above it no more); through the browser history when it is ours
function navBackTo(n) {
  const L_ = BACK.stack.length;
  if (BACK.hist && history.state && history.state.amNav === L_) { history.go(n - L_); return; } // popstate restores
  const s = BACK.stack[n]; BACK.stack.length = n; navRestore(s);
}
function setupNav() {
  try { history.replaceState(history.state, '', location.href); BACK.hist = true; } catch (e) { BACK.hist = false; }
  // other code rewrites the URL of the current entry (the hash of the open scenario / page) with a null state: the
  // depth of the entry is kept
  const rs = history.replaceState.bind(history);
  history.replaceState = (st, title, url) => rs(st == null && history.state && history.state.amNav != null ? history.state : st, title, url);
  window.addEventListener('popstate', (e) => {
    const d = e.state && e.state.amNav != null ? e.state.amNav : 0;
    if (d >= BACK.stack.length) return; // forward: nothing remembered there
    const s = BACK.stack[d]; BACK.stack.length = d; navRestore(s);
  });
  document.addEventListener('pointerdown', (e) => {
    const el = e.target.closest && (e.target.closest('g.n, g.pf, .pv-node, .pleg, .slist .ti, .pgs .pg, li, tr, button') || e.target);
    if (el && el.getBoundingClientRect) BACK.ptr = { r: el.getBoundingClientRect(), t: performance.now() };
  }, true);
  bus.on('level', () => { if (!BACK.restoring) BACK.base = navSelKey(state.sel); });
  bus.on('flow', () => { if (!BACK.restoring) BACK.base = navSelKey(state.sel); });
  bus.on('view', () => { if (!BACK.restoring) BACK.base = navSelKey(state.sel); });
}
// the first Esc closes a detail picked on top (the inspector goes back to what the level shows)
function navDetail() {
  const s = state.sel;
  if (!s || ['page', 'step', 'run', 'pstep', 'frame'].includes(s.t) || navSelKey(s) === BACK.base) return false;
  [sys, map, fv].forEach((x) => { if (x) clearSel(x.svg); });
  const back = state.run ? { t: state.step >= 0 ? 'step' : 'run' } : state.view === 'page' && AM.pages && AM.pages.cur ? { t: 'page', id: AM.pages.cur } : null;
  setSel(back);
  if (state.run && state.view === 'map') restorePlay();
  BACK.base = navSelKey(state.sel);
  return true;
}
function goBack() {
  if (navDetail()) return;
  // a page stepping a frame: Esc stops the stepping first (views/pages.js)
  if (AM.pages && AM.pages.stopStepping && AM.pages.stopStepping()) return;
  const n = navTarget();
  if (n >= 0) return navBackTo(n);
  BACK.stack.length = 0;
  // nothing remembered (a link opened straight into a scenario): the scenario closes, another view returns to the
  // map, the map goes a level up
  if (AM.pages && AM.pages.back()) return;
  if (state.run) return closeRun();
  if (state.view !== 'map') return setView('map');
  return up();
}
function setOnly(on) {
  state.only = !!on;
  paintMap(); if (fv && state.level === 2) buildFns(fv.mod);
  bus.emit('filter', state.only);
}

/* ---------- flash of what a live update changed ---------- */
// FL.fns / FL.mods: the flash in progress (cleared after it fades); SES.fns / SES.mods: everything changed
// since the page was opened, kept until a reload and shown as a dot (class lmark) next to the name.
const FL = { fns: new Set(), mods: new Set(), timer: null };
const SES = { fns: new Set(), mods: new Set(), reach: new Map() };
const FLASH_MS = 3200;
function startFlash(ids) {
  const live = (ids || []).filter((id) => D.fns[id]);
  clearTimeout(FL.timer);
  $$('.lflash').forEach((e) => e.classList.remove('lflash'));
  FL.fns = new Set(live); FL.mods = new Set(live.map((id) => D.fns[id].module));
  live.forEach((id) => { SES.fns.add(id); SES.mods.add(D.fns[id].module); });
  SES.reach.clear();
  if (!live.length) { applyFlash(); return; }
  applyFlash();
  FL.timer = setTimeout(() => { FL.fns = new Set(); FL.mods = new Set(); $$('.lflash').forEach((e) => e.classList.remove('lflash')); }, FLASH_MS);
}
// functions that reach a changed one through calls (an endpoint "contains" the edit)
function reachesSes(fid) {
  if (SES.reach.has(fid)) return SES.reach.get(fid);
  const seen = new Set([fid]); let cur = [fid], hit = SES.fns.has(fid);
  for (let d = 0; d < 8 && cur.length && !hit; d++) {
    const nx = [];
    cur.forEach((x) => { const f = D.fns[x]; if (f) (f.callees || []).concat(f.async_callees || []).forEach((c) => { if (!seen.has(c) && D.fns[c]) { seen.add(c); nx.push(c); if (SES.fns.has(c)) hit = true; } }); });
    cur = nx;
  }
  SES.reach.set(fid, hit); return hit;
}
// the session dot: a circle in SVG nodes, a ::before dot (css) in HTML rows
function mark(el, fresh) {
  if (!el) return;
  if (!el.classList.contains('lmark')) {
    el.classList.add('lmark');
    if (el instanceof SVGElement) {
      const c = el.querySelector('.card') || el.querySelector('rect');
      if (c && !el.querySelector('.ldot')) {
        const x = +c.getAttribute('x') + +c.getAttribute('width') - 10, y = +c.getAttribute('y') + 10;
        const d = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
        d.setAttribute('class', 'ldot'); d.setAttribute('cx', x); d.setAttribute('cy', y); d.setAttribute('r', 5);
        el.appendChild(d);
      }
    }
  }
  if (fresh && !el.classList.contains('lflash')) { void el.getBoundingClientRect(); el.classList.add('lflash'); }
}
// put flash and marks on what is on screen now; also called when a level or a view draws during the session
function applyFlash() {
  if (!SES.fns.size) return;
  const isF = (id) => FL.fns.has(id), isM = (m) => FL.mods.has(m);
  const f = (id) => SES.fns.has(id), m = (x) => SES.mods.has(x);
  if (sys && sys.nodes.svc) mark(sys.nodes.svc.el, FL.fns.size > 0);
  if (map) SES.mods.forEach((x) => { const u = unitFor(x); if (u && map.nodes[u]) mark(map.nodes[u].el, FL.mods.has(x)); });
  if (fv) SES.fns.forEach((id) => { if (fv.nodes[id]) mark(fv.nodes[id].el, isF(id)); });
  const s = state.sel;
  if (s && ((s.t === 'fn' && f(s.id)) || (s.t === 'mod' && m(s.id)) || (s.t === 'step' && curStep() && f(curStep().fid)))) {
    const fresh = (s.t === 'fn' && isF(s.id)) || (s.t === 'mod' && isM(s.id)) || (s.t === 'step' && curStep() && isF(curStep().fid));
    mark($('#pane'), fresh);
  }
  if (state.level === 3 && state.fn && f(state.fn)) mark($('#sc3 .src'), isF(state.fn));
  // views
  const eps = D.endpoints || [];
  $$('.api-row[data-i]').forEach((e) => { const ep = eps[+e.dataset.i]; if (ep && ep.fid && reachesSes(ep.fid)) mark(e, reachesFresh(ep.fid)); });
  $$('.matrix tr[data-i]').forEach((e) => { const ep = eps[+e.dataset.i]; if (ep && ep.fid && reachesSes(ep.fid)) mark(e, reachesFresh(ep.fid)); });
  const tbl = new Map(); // table -> fresh?
  SES.fns.forEach((id) => D.fns[id] && (D.fns[id].data || []).forEach((a) => { tbl.set(a.table, tbl.get(a.table) || isF(id)); }));
  $$('.matrix thead th[data-t]').forEach((e) => { if (tbl.has(e.dataset.t)) mark(e, tbl.get(e.dataset.t)); });
  $$('g.tb[data-id]').forEach((e) => { if (tbl.has(e.dataset.id)) mark(e, tbl.get(e.dataset.id)); });
  $$('g.tr[data-fid]').forEach((e) => { const id = e.dataset.fid; if (id && f(id)) mark(e, isF(id)); });
  $$('[data-jf]').forEach((e) => { const id = e.dataset.jf; if (id && f(id)) mark(e, isF(id)); });
  $$('li[data-fn]').forEach((e) => { const id = e.dataset.fn; if (f(id)) mark(e, isF(id)); });
}
function reachesFresh(fid) {
  const seen = new Set([fid]); let cur = [fid];
  if (FL.fns.has(fid)) return true;
  for (let d = 0; d < 8 && cur.length; d++) {
    const nx = [];
    for (const x of cur) { const f = D.fns[x]; if (f) for (const c of (f.callees || []).concat(f.async_callees || [])) { if (!seen.has(c) && D.fns[c]) { if (FL.fns.has(c)) return true; seen.add(c); nx.push(c); } } }
    cur = nx;
  }
  return false;
}
// views redraw their rows on scroll/select: re-apply the marks after any DOM change inside the stage
let _mkq = 0;
function watchMarks() {
  const root = document.getElementById('stage') || document.body;
  new MutationObserver((ms) => {
    if (!SES.fns.size || _mkq) return;
    if (!ms.some((m) => m.type === 'childList' && (m.addedNodes.length))) return;
    _mkq = requestAnimationFrame(() => { _mkq = 0; applyFlash(); });
  }).observe(root, { childList: true, subtree: true });
}
bus.on('level', () => { if (SES.fns.size) requestAnimationFrame(applyFlash); });
bus.on('view', () => { if (SES.fns.size) requestAnimationFrame(applyFlash); });
Object.assign(AM, { flowActor, menu: { open: openMenu, close: closeMenu, items: menuItems }, navPush, navBatch, nav: { get stack() { return BACK.stack; }, back: goBack, label: navBackLabel } });
