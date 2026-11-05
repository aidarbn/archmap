/* nav.js — the header controls: the tabs ("Map" and one tab per through path, in the order of model.journeys),
 * the ⋯ menu (views API / Data / Statuses, language, theme, only changed), the #journey=<id> hash, going back.
 * Also: the flash of what a live update changed (archmap serve). There is no search: other scenarios open from the
 * panels ("Scenarios through this node"), the views from the panels and from ⋯. */

addI18n({
  ru: {
    tabMap: 'Карта', tabMapTip: 'Вся карта (выйти из пути)', moreTip: 'Ещё: виды, язык, тема',
    mnView: (v) => v, mnLang: 'Язык: English', mnTheme: (d) => (d ? 'Тема: светлая' : 'Тема: тёмная'), mnOnly: (on) => (on ? 'Показать все, не только изменённые' : 'Только изменённые в ветке'),
    pkPath: 'сквозной путь', pkStepsN: (n, p) => n + ' ' + p(['шаг', 'шага', 'шагов']),
    ms: 'мс', changedN: 'изм.', noConnTip: 'Сервер archmap не отвечает; страница переподключится сама',
  },
  en: {
    tabMap: 'Map', tabMapTip: 'The whole map (leave the path)', moreTip: 'More: views, language, theme',
    mnView: (v) => v, mnLang: 'Язык: русский', mnTheme: (d) => (d ? 'Theme: light' : 'Theme: dark'), mnOnly: (on) => (on ? 'Show all, not only changed' : 'Only changed on the branch'),
    pkPath: 'through path', pkStepsN: (n) => n + (n === 1 ? ' step' : ' steps'),
    ms: 'ms', changedN: 'changed', noConnTip: 'The archmap server does not answer; the page reconnects by itself',
  },
});
const lc = (s) => String(s || '').toLowerCase();
// the actor of a flow: declared, else the actor of its first step
const flowActor = (fl) => { if (fl.actor) return fl.actor; const a = fl.steps && fl.steps.length ? actorOfFn(fl.steps[0]) : null; return a ? actorName(a) : ''; };

/* ---------- tabs ---------- */
const TAB_LEN = 30; // characters of a tab title; the whole name is in its tooltip
function renderTabs() {
  const box = $('#tabs'); if (!box) return;
  const cur = state.run && state.run.key && state.run.key.startsWith('journey:') ? state.run.key.slice(8) : null;
  const onMap = !cur && state.view === 'map';
  let h = `<button class="tab" role="tab" data-tab="" aria-selected="${onMap}" title="${esc(t('tabMapTip'))}">${esc(t('tabMap'))}</button>`;
  jvJourneys().forEach((j) => {
    const nm = j.name || j.id, sh = nm.replace(/\s*→\s*….*$/, ''); // an automatic path: "<first flow> → … (N flows)"
    h += `<button class="tab" role="tab" data-tab="${esc(j.id)}" aria-selected="${cur === j.id}" title="${esc(nm + (j.desc && j.desc !== nm ? '\n' + j.desc : ''))}">${esc(cut(sh, TAB_LEN))}</button>`;
  });
  box.innerHTML = h;
  const act = box.querySelector('[aria-selected="true"]');
  if (act && act.scrollIntoView) act.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}
function pickTab(id) {
  closeMenu();
  if (!id) { if (state.run) closeRun(); if (state.view !== 'map') setView('map'); renderTabs(); return; }
  if (state.run && state.run.key === 'journey:' + id && state.view === 'map') return;
  openRun('journey:' + id);
}
// the hash keeps the open path over a reload: #journey=<id>
function writeHash() {
  const k = state.run && state.run.key, want = k && k.startsWith('journey:') ? '#journey=' + encodeURIComponent(k.slice(8)) : '';
  if ((location.hash || '') === want) return;
  try { history.replaceState(null, '', want || location.pathname + location.search); } catch (e) { /* file:// in some browsers */ }
}
function hashJourney() {
  const m = /^#journey=(.+)$/.exec(location.hash || '');
  if (!m) return null;
  const id = decodeURIComponent(m[1]);
  return jvJourney(id) ? id : null;
}
function setupTabs() {
  const box = $('#tabs');
  box.addEventListener('click', (e) => { const b = e.target.closest('.tab'); if (b) pickTab(b.getAttribute('data-tab')); });
  // a vertical wheel scrolls the tabs sideways
  box.addEventListener('wheel', (e) => { if (Math.abs(e.deltaY) > Math.abs(e.deltaX) && box.scrollWidth > box.clientWidth) { box.scrollLeft += e.deltaY; e.preventDefault(); } }, { passive: false });
  bus.on('flow', () => { renderTabs(); writeHash(); });
  bus.on('view', renderTabs);
  bus.on('lang', renderTabs);
  bus.on('data', renderTabs);
  window.addEventListener('hashchange', () => { const id = hashJourney(); if (id) pickTab(id); else if (state.run && state.run.key.startsWith('journey:')) pickTab(''); });
  renderTabs();
}

/* ---------- ⋯ menu ---------- */
function menuItems() {
  const dark = document.documentElement.getAttribute('data-theme') === 'dark';
  const out = VIEWS.filter((v) => v.id !== 'map').map((v) => ({ nm: t('mnView', viewTitle(v)), cur: state.view === v.id, run: () => { if (state.view === v.id) setView('map'); else setView(v.id); } }));
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

/* ---------- back ---------- */
// back: the scenario closes first, then another view returns to the map, then the map goes a level up
function goBack() {
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
Object.assign(AM, { flowActor, tabs: { render: renderTabs, pick: pickTab }, menu: { open: openMenu, close: closeMenu, items: menuItems } });
