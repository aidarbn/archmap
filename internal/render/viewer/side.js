/* side.js — the sidebar, the base of the page: a docked column at the left (#side) with the full navigation of the
 * project as one tree, like the layers panel of Figma — names only, one line each with an ellipsis (the full name in
 * the tooltip), small grey counters only on sections and groups:
 *   System (the root) → Users (actors) → Modules by layer (Entry / Domain / Adapters / Infrastructure …; inside the
 *   modules, inside a module its functions, built only when unfolded) → Outside systems → Data (tables) → API
 *   (services → endpoints) → Statuses (entities) → Diagrams (through paths, unfolded → their scenarios; Scheduled —
 *   the scenarios of background jobs, entry task / cron; All scenarios, folded — by actor).
 * A click on a node selects it on the map (going to its level) or opens its view in the centre (Data / API /
 * Statuses), and the inspector shows it. The tree follows the selection made on the map or in a view: it unfolds
 * down to the selected node and scrolls to it. The filter on top narrows the whole tree (by name), folded groups too.
 * A scenario opens in this window (openScenario): the canvas turns into its diagram — the participants only, in
 * layers, in the order of the steps (map.js applyFocus) — the navigation marks it, the inspector shows the step; the
 * hash keeps it (#flow=<id> / #journey=<id> / #fn=<fid>). "‹ Map", Esc, another row of the tree or the same
 * scenario again bring the full map back.
 * The width is dragged by the right edge (200–420 px, localStorage archmap.side.w, double-click — 260); the toolbar
 * button or `[` hides the column (localStorage archmap.side). */

addI18n({
  ru: {
    nvSys: 'Система', nvActors: 'Пользователи', nvMods: 'Модули', nvExt: 'Внешние системы', nvData: 'Данные', nvApi: 'API', nvStatus: 'Статусы',
    nvScen: 'Диаграммы', nvPaths: 'Сквозные пути', nvSched: 'По расписанию', nvAll: 'Все сценарии', nvWin: 'Откроется диаграммой на холсте; повторный клик — назад к карте',
    sdFilter: 'Фильтр', sdNone: 'Ничего не найдено', sdHide: 'Скрыть навигацию', sdShow: 'Показать навигацию',
    sdLabel: 'Навигация', sdGrip: 'Потяните, чтобы изменить ширину; двойной клик — по умолчанию', sdFold: 'Свернуть', sdUnfold: 'Развернуть',
    chPanelHide: 'Скрыть инспектор', chPanelShow: 'Показать инспектор', chTopHide: 'Свернуть верхнюю строку', chTopShow: 'Показать верхнюю строку',
    chFocusOn: 'Только холст', chFocusOff: 'Вернуть панели',
    pgPages: 'Страницы', pgSystem: 'Система', pgFold: 'Свернуть список страниц', pgUnfold: 'Показать список страниц',
    pgBuiltin: 'Страница archmap: строится из кода', pgUser: (f) => 'Страница из ' + f, pgBad: 'Есть ссылки на то, чего нет в коде (archmap lint)',
    peNew: 'Новая страница', peNamePh: 'Имя страницы', peNameEmpty: 'Введите имя страницы', peNameDup: (n) => 'Страница «' + n + '» уже есть',
    peDelPage: 'Удалить страницу', peDelConfirm: (n, f) => 'Удалить страницу «' + n + '»?\n\nФайл ' + f + ' будет изменён или удалён на диске (⌘Z — вернуть).',
    peDeleted: (n) => 'Страница «' + n + '» удалена', peRenameTip: 'Двойной клик — переименовать',
    peTabPage: 'Страница', peTabSys: 'Система', peTabTip: 'Навигация «Система» — источник: перетащите элементы на холст страницы',
    peSrcHint: 'Перетащите на страницу · ⌘/Shift — несколько · двойной клик — добавить',
    peNoRefJ: 'этот путь собран автоматически — на страницу кладутся пути arch:journey', peNoRefF: 'у сценария нет id — на страницу кладутся сценарии arch:flow',
    peNoFn: 'функция — положите на страницу её модуль', peNoActor: 'пользователи и внешние системы рисуются внутри диаграмм', peNoSec: 'раздел целиком нельзя — выберите элементы внутри',
    peNoEp: 'у ручки нет однозначного имени', peOrderNo: 'большие диаграммы (пути, статусы) всегда стоят выше — порядок только внутри своей группы',
  },
  en: {
    nvSys: 'System', nvActors: 'Users', nvMods: 'Modules', nvExt: 'Outside systems', nvData: 'Data', nvApi: 'API', nvStatus: 'Statuses',
    nvScen: 'Diagrams', nvPaths: 'Through paths', nvSched: 'Scheduled', nvAll: 'All scenarios', nvWin: 'Opens as a diagram on the canvas; click again to go back to the map',
    sdFilter: 'Filter', sdNone: 'Nothing found', sdHide: 'Hide the navigation', sdShow: 'Show the navigation',
    sdLabel: 'Navigation', sdGrip: 'Drag to resize; double-click for the default', sdFold: 'Fold', sdUnfold: 'Unfold',
    chPanelHide: 'Hide the inspector', chPanelShow: 'Show the inspector', chTopHide: 'Fold the toolbar', chTopShow: 'Show the toolbar',
    chFocusOn: 'Canvas only', chFocusOff: 'Bring the panels back',
    pgPages: 'Pages', pgSystem: 'System', pgFold: 'Fold the page list', pgUnfold: 'Show the page list',
    pgBuiltin: 'An archmap page: built from the code', pgUser: (f) => 'A page from ' + f, pgBad: 'Names something the code does not have (archmap lint)',
    peNew: 'New page', peNamePh: 'Page name', peNameEmpty: 'Type a name for the page', peNameDup: (n) => 'There is a page “' + n + '” already',
    peDelPage: 'Delete the page', peDelConfirm: (n, f) => 'Delete the page “' + n + '”?\n\nThe file ' + f + ' is changed or removed on disk (⌘Z brings it back).',
    peDeleted: (n) => 'Page “' + n + '” deleted', peRenameTip: 'Double-click to rename',
    peTabPage: 'Page', peTabSys: 'System', peTabTip: 'The System navigation as a source: drag items onto the page canvas',
    peSrcHint: 'Drag onto the page · ⌘/Shift — several · double-click — add',
    peNoRefJ: 'this path is assembled automatically — pages take arch:journey paths', peNoRefF: 'the scenario has no id — pages take arch:flow scenarios',
    peNoFn: 'a function — put its module on the page', peNoActor: 'users and outside systems are drawn inside the diagrams', peNoSec: 'not a whole section — pick the items inside',
    peNoEp: 'the endpoint has no unambiguous name', peOrderNo: 'large diagrams (paths, statuses) always stay on top — order within their group only',
  },
});

// open: nodes unfolded by hand (the root and Scenarios by default); last: the node clicked last (a view without a
// selection of its own — Statuses — keeps it marked)
// src: the System navigation shown as the source of a user page (archmap serve); picks: the rows picked in it
const SIDE = { hidden: false, q: '', open: new Set(['sys', 'sec:scen', 'scp', 'scs']), w: 0, last: null, lastView: null, src: false, picks: new Set() };
const SIDE_W = { def: 260, min: 200, max: 420 };
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
  clock: IC('<circle cx="7" cy="7" r="5"/><path d="M7 4.2V7l2 1.4"/>'),
  all: IC('<path d="M2.5 3.5h9M2.5 7h9M2.5 10.5h9"/>'),
  page: IC('<path d="M3.5 1.5h5l2.5 2.5v8.5h-7.5z"/><path d="M8.5 1.5V4H11"/>'),
  frame: IC('<path d="M4 1.5v11M10 1.5v11M1.5 4h11M1.5 10h11"/>'),
  note: IC('<rect x="2" y="2" width="10" height="10" rx="1.5"/><path d="M4.5 5h5M4.5 7.5h5M4.5 10h3"/>'),
  plus: IC('<path d="M7 2.5v9M2.5 7h9"/>'),
  trash: IC('<path d="M2.5 4h9M5.5 4V2.5h3V4M3.8 4l.6 7.5h5.2l.6-7.5"/>'),
};

/* ---------- pages ----------
 * Like pages in Figma: the archmap pages built from the code — System (the map and the whole tree), Data, API,
 * Statuses (the views of the same name) — then the pages of arch/pages/*.arch (views/pages.js draws them: frames,
 * each a diagram from the code). The list sits on top of the sidebar; the tree under it shows the current page. */
const PG = { fold: false };
const userPages = () => ((D.M && D.M.spec && D.M.spec.pages) || []);
function pageList() {
  const out = [{ id: 'system', name: t('pgSystem'), view: 'map', icon: 'sys' }];
  if (hasView('data') && Object.keys(D.tables || {}).length) out.push({ id: 'data', name: t('nvData'), view: 'data', icon: 'table', sec: 'sec:data' });
  if (hasView('api') && (D.endpoints || []).length) out.push({ id: 'api', name: t('nvApi'), view: 'api', icon: 'api', sec: 'sec:api' });
  if (hasView('status') && Object.keys(D.states || {}).length) out.push({ id: 'status', name: t('nvStatus'), view: 'status', icon: 'status', sec: 'sec:status' });
  if (hasView('page')) userPages().forEach((p) => out.push({ id: 'p:' + p.id, name: p.name, view: 'page', icon: 'page', page: p, user: true, bad: (p.frames || []).some((f) => (f.missing || []).length) }));
  return out;
}
// the page on the canvas: a user page while the page view is on, else the archmap page of the view (System for the map)
function curPage() {
  const all = pageList();
  if (state.view === 'page' && AM.pages && AM.pages.cur) return all.find((p) => p.id === 'p:' + AM.pages.cur) || all[0];
  return all.find((p) => p.view === state.view) || all[0];
}
function openPage(id) {
  const p = pageList().find((x) => x.id === id); if (!p) return;
  closeMenu();
  if (AM.pages) AM.pages.forget(); // a page picked by hand: Esc no longer returns to the page a frame came from
  if (state.run) closeRun();
  if (p.user) { AM.pages.open(p.page.id); return; }
  if (state.view !== p.view) setView(p.view);
  if (p.view === 'map' && state.level > 0) up(0);
  renderPages(); renderSide(); paintHName(); writeHash();
}
/* Editing the list (archmap serve only; views/pages.js sends the edits): "+" in the header — the name is typed right
 * in the list (Enter — create, Esc — cancel), a double click on a page of arch/pages — rename in place, the bin at
 * its right — delete (with a confirmation). In a static map "+" is greyed out and says where it works. */
const PGE = { add: false, ren: null, err: '', val: '', focus: false, busy: false, last: { id: null, at: 0 } };
const canEditPages = () => !!(AM.pages && AM.pages.edit && AM.pages.edit.can);
const peToast = (m, k) => { if (AM.pages && AM.pages.edit) AM.pages.edit.toast(m, k); };
function nameRow(v, mode) {
  return `<div class="pg pgin${PGE.err ? ' err' : ''}">${SIDE_ICON.page}<input class="pgname" data-mode="${mode}" value="${esc(v)}" placeholder="${esc(t('peNamePh'))}" aria-label="${esc(t('peNamePh'))}" maxlength="120" spellcheck="false" autocomplete="off"></div>` +
    `<div class="pgerr" role="alert"${PGE.err ? '' : ' hidden'}>${esc(PGE.err)}</div>`;
}
function renderPages() {
  const box = $('#pages'); if (!box || !D.F) return;
  const ae = document.activeElement;
  if (ae && ae.tagName === 'INPUT' && box.contains(ae)) return; // a name is being typed: the list waits
  const all = pageList(), cur = curPage(), live = canEditPages();
  const chev = `<span class="chev${PG.fold ? '' : ' on'}">${SIDE_ICON.chev}</span>`;
  let h = '<div class="pghr">' + `<button type="button" class="pgh" data-pgfold="1" title="${esc(t(PG.fold ? 'pgUnfold' : 'pgFold'))}" aria-expanded="${!PG.fold}">${chev}<span class="nm">${esc(t('pgPages'))}</span>` +
    (PG.fold ? `<span class="cur">${esc(cur.name)}</span>` : `<span class="ct">${all.length}</span>`) + '</button>' +
    (hasView('page') ? `<button type="button" class="pgadd${live ? '' : ' off'}" data-pgadd="1" title="${esc(live ? t('peNew') : t('peServeOnly'))}" aria-label="${esc(t('peNew'))}"${live ? '' : ' aria-disabled="true"'}>${SIDE_ICON.plus}</button>` : '') + '</div>';
  if (!PG.fold) {
    h += '<div class="pgl" role="listbox" aria-label="' + esc(t('pgPages')) + '">';
    all.forEach((p, i) => {
      if (p.user && (i === 0 || !all[i - 1].user)) h += '<div class="pgsep" role="separator"></div>';
      if (p.user && PGE.ren === p.page.id) { h += nameRow(PGE.val, 'ren'); return; }
      const tip = p.name + '\n' + (p.user ? t('pgUser', p.page.file || 'arch/pages') + (live ? '\n' + t('peRenameTip') : '') : t('pgBuiltin')) + (p.bad ? '\n' + t('pgBad') : '');
      h += `<div class="pgr"><button type="button" role="option" class="pg${p.id === cur.id ? ' cur' : ''}${p.bad ? ' bad' : ''}" data-pg="${esc(p.id)}" title="${esc(tip)}" aria-selected="${p.id === cur.id}">${SIDE_ICON[p.icon]}<span class="nm">${esc(p.name)}</span>${p.user ? `<span class="ct">${(p.page.frames || []).length}</span>` : ''}</button>` +
        (p.user && live ? `<button type="button" class="pgdel" data-pgdel="${esc(p.page.id)}" title="${esc(t('peDelPage'))}" aria-label="${esc(t('peDelPage') + ' ' + p.name)}">${SIDE_ICON.trash}</button>` : '') + '</div>';
    });
    if (PGE.add) { if (!all.some((p) => p.user)) h += '<div class="pgsep" role="separator"></div>'; h += nameRow(PGE.val, 'add'); }
    h += '</div>';
  }
  const st = box.querySelector('.pgl') ? box.querySelector('.pgl').scrollTop : 0;
  box.innerHTML = h;
  // the list keeps its scroll; a page opened (a new one at the end) is scrolled into it
  const l = box.querySelector('.pgl'), c = box.querySelector('.pg.cur, .pgin');
  if (l) {
    l.scrollTop = st;
    if (c && (PGE.seen !== cur.id || c.classList.contains('pgin'))) { const lb = l.getBoundingClientRect(), cb = c.getBoundingClientRect(); if (cb.bottom > lb.bottom) l.scrollTop += cb.bottom - lb.bottom + 4; else if (cb.top < lb.top) l.scrollTop -= lb.top - cb.top + 4; }
  }
  PGE.seen = cur.id;
  const inp = box.querySelector('input.pgname');
  if (inp && PGE.focus) { PGE.focus = false; inp.focus(); if (PGE.ren) inp.select(); else inp.setSelectionRange(inp.value.length, inp.value.length); }
}
function nameErr(msg) {
  PGE.err = msg || '';
  const box = $('#pages'), row = box.querySelector('.pgin'), er = box.querySelector('.pgerr');
  if (row) row.classList.toggle('err', !!msg);
  if (er) { er.textContent = PGE.err; er.hidden = !msg; }
}
function startName(mode, id) {
  if (!canEditPages()) { peToast(t('peServeOnly')); return; }
  PGE.add = mode === 'add'; PGE.ren = mode === 'ren' ? id : null; PGE.err = ''; PGE.focus = true;
  PGE.val = mode === 'ren' ? (userPages().find((p) => p.id === id) || {}).name || '' : '';
  if (PG.fold) PG.fold = false;
  const ae = document.activeElement; if (ae && ae.blur) ae.blur();
  renderPages();
}
function endName() {
  PGE.add = false; PGE.ren = null; PGE.err = ''; PGE.val = '';
  const ae = document.activeElement; if (ae && ae.tagName === 'INPUT' && $('#pages').contains(ae)) { PGE.busy = true; ae.blur(); PGE.busy = false; }
  renderPages();
}
async function commitName(mode, v) {
  if (PGE.busy) return;
  const name = String(v || '').trim();
  PGE.val = v;
  const self = mode === 'ren' ? userPages().find((p) => p.id === PGE.ren) : null;
  if (mode === 'ren' && (!self || name === self.name)) { endName(); return; }
  if (!name) { if (mode === 'add') endName(); else nameErr(t('peNameEmpty')); return; }
  const dup = userPages().find((p) => String(p.name).trim().toLowerCase() === name.toLowerCase() && (!self || p.id !== self.id));
  if (dup) { nameErr(t('peNameDup', dup.name)); return; }
  PGE.busy = true;
  const j = await AM.pages.edit.op(mode === 'add' ? { op: 'create', name } : { op: 'rename', page: self.id, name }, { quiet: true });
  PGE.busy = false;
  if (!j) return;
  if (j.error) { nameErr(j.error); return; }
  endName();
  if (mode === 'add' && j.page && AM.pages.has(j.page)) { SIDE.src = true; SIDE.picks.clear(); AM.pages.open(j.page); }
}
async function deletePage(id) {
  const p = userPages().find((x) => x.id === id); if (!p) return;
  if (!window.confirm(t('peDelConfirm', p.name, p.file || 'arch/pages'))) return;
  const j = await AM.pages.edit.op({ op: 'delete', page: id });
  if (j && !j.error) peToast(t('peDeleted', p.name) + ' · ' + t('peUndoHint'));
}
function setupPages() {
  try { PG.fold = localStorage.getItem('archmap.pages.fold') === '1'; } catch (e) { /* file:// without storage */ }
  const box = $('#pages');
  box.addEventListener('click', (e) => {
    if (e.target.closest('[data-pgfold]')) {
      PG.fold = !PG.fold;
      try { localStorage.setItem('archmap.pages.fold', PG.fold ? '1' : '0'); } catch (er) { /* ignore */ }
      renderPages(); return;
    }
    if (e.target.closest('[data-pgadd]')) { startName('add'); return; }
    const d = e.target.closest('[data-pgdel]'); if (d) { deletePage(d.getAttribute('data-pgdel')); return; }
    const b = e.target.closest('[data-pg]'); if (!b) return;
    const id = b.getAttribute('data-pg'), now = Date.now(), p = pageList().find((x) => x.id === id);
    // a double click: the list is redrawn by the first click, so it is told by time and page, not by the element
    if (p && p.user && PGE.last.id === id && now - PGE.last.at < 450) { PGE.last = { id: null, at: 0 }; if (canEditPages()) startName('ren', p.page.id); else peToast(t('peServeOnly')); return; }
    PGE.last = { id, at: now };
    openPage(id);
  });
  box.addEventListener('keydown', (e) => {
    const inp = e.target.closest('input.pgname');
    if (inp) {
      e.stopPropagation();
      if (e.key === 'Enter') { e.preventDefault(); commitName(inp.getAttribute('data-mode'), inp.value); }
      else if (e.key === 'Escape') { e.preventDefault(); endName(); }
      return;
    }
    const b = e.target.closest('[data-pg]');
    if (b && e.key === 'F2') { const p = pageList().find((x) => x.id === b.getAttribute('data-pg')); if (p && p.user) { e.preventDefault(); startName('ren', p.page.id); } }
  });
  box.addEventListener('input', (e) => { if (e.target.closest('input.pgname')) { PGE.val = e.target.value; if (PGE.err) nameErr(''); } });
  box.addEventListener('focusout', (e) => {
    const inp = e.target.closest && e.target.closest('input.pgname');
    if (inp && !PGE.busy) commitName(inp.getAttribute('data-mode'), inp.value);
  });
  renderPages();
}

/* ---------- a user page: its tree, or the System navigation as the source of what goes on it ---------- */
// the tabs "Page | System" over the tree: only on a page of arch/pages under archmap serve
const srcTabs = () => state.view === 'page' && !state.run && curPage().user && canEditPages();
const isSrc = () => SIDE.src && srcTabs();
function paintTabs() {
  const el = $('#sideTabs'); if (!el) return;
  const on = srcTabs();
  el.hidden = !on;
  if (!on) return;
  el.innerHTML = `<button type="button" role="tab" data-stab="page" aria-selected="${!SIDE.src}">${esc(t('peTabPage'))}</button>` +
    `<button type="button" role="tab" data-stab="sys" aria-selected="${SIDE.src}" title="${esc(t('peTabTip'))}">${SIDE_ICON.plus}${esc(t('peTabSys'))}</button>`;
}
function setSource(on) {
  SIDE.src = !!on; SIDE.picks.clear();
  if (on && SIDE.hidden) setSideHidden(false);
  paintTabs(); renderSide();
}
// what a row of the System navigation puts on a page: {kind, refs, keys, label} or {why} — why it cannot
const refFlows = () => { const s = new Set(((D.M.spec || {}).flows || []).map((f) => f.id)); Object.values(D.fns).forEach((fn) => ((fn.arch && fn.arch.flows) || []).forEach((d) => s.add(d.id))); return s; };
const refJourneys = () => { const s = new Set(((D.M.spec || {}).journeys || []).map((j) => j.id)); Object.values(D.fns).forEach((fn) => ((fn.arch && fn.arch.journeys) || []).forEach((d) => s.add(d.id))); return s; };
function epRef(i) {
  const eps = D.endpoints || [], e = eps[i]; if (!e) return null;
  const hit = (x, r) => (x.name && x.name === r) || (x.service && x.name && x.service + '.' + x.name === r) || (x.path && x.path === r) || (x.method && x.path && (x.method + ' ' + x.path).toLowerCase() === r.toLowerCase());
  const cands = [e.name, e.service && e.name ? e.service + '.' + e.name : '', e.path, e.method && e.path ? e.method + ' ' + e.path : ''].filter(Boolean);
  return cands.find((c) => eps.filter((x) => hit(x, c)).length === 1) || null;
}
function itemOf(key) {
  const n = NAV.get(key), label = n ? n.name : key, no = (why) => ({ why, label, icon: n && n.icon });
  let m;
  if ((m = /^r:journey:(.+?)(?:@\d+)?$/.exec(key))) return refJourneys().has(m[1]) ? { kind: 'journey', refs: [m[1]], keys: [m[1]], label, icon: 'path' } : no(t('peNoRefJ'));
  if ((m = /^rs?:flow:(\d+)$/.exec(key))) { const fl = D.flows.find((x) => x.i === +m[1]); return fl && fl.id && refFlows().has(fl.id) ? { kind: 'flow', refs: [fl.id], keys: [fl.id], label, icon: 'flow' } : no(t('peNoRefF')); }
  if (key.startsWith('s:')) return { kind: 'states', refs: [key.slice(2)], keys: [key.slice(2)], label, icon: 'status' };
  if (key.startsWith('t:')) return { kind: 'tables', refs: [key.slice(2)], keys: [key.slice(2)], label, icon: 'table' };
  if (key.startsWith('m:')) return { kind: 'module', refs: [key.slice(2)], keys: [key.slice(2)], label, icon: 'mod' };
  if (key.startsWith('e:')) { const i = +key.slice(2), r = epRef(i); return r ? { kind: 'api', refs: [r], keys: [i], label, icon: 'ep' } : no(t('peNoEp')); }
  if (key.startsWith('svc:')) {
    const svc = key.slice(4), is = (D.endpoints || []).map((e, i) => ((e.service || (e.kind === 'grpc' ? 'gRPC' : 'HTTP')) === svc ? i : -1)).filter((i) => i >= 0);
    const ok = is.filter((i) => epRef(i));
    return ok.length ? { kind: 'api', refs: ok.map(epRef), keys: ok, label, icon: 'api' } : no(t('peNoEp'));
  }
  if (key.startsWith('f:')) return no(t('peNoFn'));
  if (key.startsWith('a:') || key.startsWith('x:')) return no(t('peNoActor'));
  return no(t('peNoSec'));
}
// the items a drag (or a double click) of row key takes: the picked rows when key is one of them
const dragItems = (key) => (SIDE.picks.has(key) && SIDE.picks.size > 1 ? Array.from(SIDE.picks) : [key]).map(itemOf);
function paintPicks() { $('#sideList').querySelectorAll('.ti[data-k]').forEach((el) => el.classList.toggle('pick', SIDE.picks.has(el.getAttribute('data-k')))); }
function addToPage(keys) {
  const items = keys.map(itemOf), v = AM.pages.edit.accepts(items);
  if (!v.ok) { peToast((v.dup ? '' : t('peDropNo') + ': ') + v.why, v.dup ? '' : 'err'); return; }
  AM.pages.edit.add(items);
}

/* dragging with the pointer (works with a mouse, a pen and WebDriver alike): System rows onto the page — the canvas,
 * the "Page" tab (held over it, the tab opens and the page tree takes the drop), the page in the list; the frames of
 * the page tree up and down to reorder. The ghost says what a drop does here: add, already there, or why not. */
const DRAG = { st: null, quiet: 0, tabT: 0 };
function dropTarget(x, y) {
  const el = document.elementFromPoint(x, y); if (!el) return null;
  const pv = AM.pages && AM.pages.edit.el;
  if (pv && pv.contains(el)) return { kind: 'canvas', el: pv };
  const tab = el.closest('[data-stab="page"]'); if (tab) return { kind: 'tab', el: tab };
  const pg = el.closest('.pg.cur'); if (pg && curPage().user) return { kind: 'list', el: pg };
  if (!isSrc() && $('#sideList').contains(el)) return { kind: 'tree', el: $('#sideList') };
  return null;
}
function clearDropMarks() {
  document.querySelectorAll('.pe-drop-ok, .pe-drop-no').forEach((e) => e.classList.remove('pe-drop-ok', 'pe-drop-no'));
  const l = $('#sideList .dropline'); if (l) l.remove();
}
function dragMove(e) {
  const s = DRAG.st; if (!s) return;
  if (!s.on) {
    if (Math.hypot(e.clientX - s.x, e.clientY - s.y) < 5) return;
    s.on = true;
    s.items = s.mode === 'add' ? dragItems(s.key) : null;
    const n = NAV.get(s.key), lab = s.items ? (s.items[0].label + (s.items.length > 1 ? '  +' + (s.items.length - 1) : '')) : n ? n.name : '';
    const ic = s.items ? s.items[0].icon : n && n.icon;
    s.ghost = H('div', { class: 'pe-ghost' }, document.body, (ic && SIDE_ICON[ic] ? SIDE_ICON[ic] : '') + `<span class="nm">${esc(lab)}</span><span class="vd"></span>`);
    document.body.classList.add('pe-dragging');
  }
  s.ghost.style.transform = `translate(${e.clientX + 14}px, ${e.clientY + 12}px)`;
  clearDropMarks();
  const vd = s.ghost.querySelector('.vd');
  let cls = '', msg = '';
  if (s.mode === 'add') {
    const tg = dropTarget(e.clientX, e.clientY);
    if (tg && tg.kind === 'tab') { if (!DRAG.tabT) DRAG.tabT = setTimeout(() => { DRAG.tabT = 0; if (DRAG.st && SIDE.src) { SIDE.src = false; paintTabs(); renderSide(); } }, 450); }
    else if (DRAG.tabT) { clearTimeout(DRAG.tabT); DRAG.tabT = 0; }
    s.target = tg;
    if (tg) {
      const v = AM.pages.edit.accepts(s.items);
      s.ok = v.ok; cls = v.ok ? 'ok' : v.dup ? 'dup' : 'no'; msg = v.ok ? v.why : (v.dup ? '' : t('peDropNo') + ': ') + v.why;
      if (!v.dup) tg.el.classList.add(v.ok ? 'pe-drop-ok' : 'pe-drop-no'); // already there: neither yes nor red
    }
  } else {
    // reorder: the frame row under the pointer, before or after it, within the same group
    const box = $('#sideList'), rows = Array.from(box.querySelectorAll('.ti[data-k]')).filter((r) => /^pf:\d+$/.test(r.getAttribute('data-k')));
    s.target = null;
    if (box.contains(document.elementFromPoint(e.clientX, e.clientY)) && rows.length) {
      let best = null;
      rows.forEach((r) => { const b = r.getBoundingClientRect(), d = e.clientY < b.top ? b.top - e.clientY : e.clientY > b.bottom ? e.clientY - b.bottom : 0; if (!best || d < best.d) best = { r, b, d }; });
      const j = +best.r.getAttribute('data-k').slice(3), after = e.clientY > best.b.top + best.b.height / 2, i = +s.key.slice(3);
      const pg = curPage().page, rank = AM.pages.edit.rank, ok = j !== i && rank(pg.frames[i].kind) === rank(pg.frames[j].kind);
      s.target = { j, after }; s.ok = ok;
      const bb = box.getBoundingClientRect(), line = H('div', { class: 'dropline' + (ok ? '' : ' no') }, box);
      line.style.top = (after ? best.b.bottom : best.b.top) - bb.top + box.scrollTop - 1 + 'px';
      cls = ok ? 'ok' : 'no'; msg = ok ? '' : t('peOrderNo');
      if (j === i) { cls = ''; msg = ''; line.remove(); }
    }
  }
  s.ghost.className = 'pe-ghost' + (cls ? ' ' + cls : '');
  vd.textContent = msg;
}
function dragEnd(e, cancel) {
  const s = DRAG.st; DRAG.st = null;
  window.removeEventListener('pointermove', dragMove); window.removeEventListener('pointerup', dragUp); window.removeEventListener('pointercancel', dragCancel); window.removeEventListener('keydown', dragKey, true);
  if (DRAG.tabT) { clearTimeout(DRAG.tabT); DRAG.tabT = 0; }
  if (!s || !s.on) return;
  DRAG.quiet = Date.now(); // the click that ends a drag does nothing
  if (s.ghost) s.ghost.remove();
  document.body.classList.remove('pe-dragging');
  clearDropMarks();
  if (cancel || !s.target) return;
  if (s.mode === 'add') {
    if (s.ok) AM.pages.edit.add(s.items);
    else { const v = AM.pages.edit.accepts(s.items); peToast((v.dup ? '' : t('peDropNo') + ': ') + v.why, v.dup ? '' : 'err'); }
  } else if (s.ok) AM.pages.edit.move(+s.key.slice(3), s.target.j, s.target.after);
}
const dragUp = (e) => dragEnd(e, false), dragCancel = (e) => dragEnd(e, true);
const dragKey = (e) => { if (e.key === 'Escape' && DRAG.st && DRAG.st.on) { e.preventDefault(); e.stopPropagation(); dragEnd(e, true); } };
function setupDrag() {
  const box = $('#sideList');
  box.addEventListener('pointerdown', (e) => {
    if (e.button !== 0 || DRAG.st) return;
    const r = e.target.closest('.ti[data-k]'); if (!r || e.target.closest('[data-tog]')) return;
    const key = r.getAttribute('data-k');
    const mode = isSrc() ? 'add' : srcTabs() && /^pf:\d+$/.test(key) ? 'move' : null;
    if (!mode) return;
    DRAG.st = { mode, key, x: e.clientX, y: e.clientY, on: false };
    window.addEventListener('pointermove', dragMove); window.addEventListener('pointerup', dragUp); window.addEventListener('pointercancel', dragCancel); window.addEventListener('keydown', dragKey, true);
  });
  box.addEventListener('dblclick', (e) => {
    if (!isSrc()) return;
    const r = e.target.closest('.ti[data-k]'); if (!r || e.target.closest('[data-tog]')) return;
    const key = r.getAttribute('data-k');
    addToPage(SIDE.picks.has(key) && SIDE.picks.size > 1 ? Array.from(SIDE.picks) : [key]);
  });
  box.addEventListener('keydown', (e) => {
    if (!isSrc() || e.key !== 'Enter') return;
    const r = e.target.closest('.ti[data-k]'); if (!r) return;
    e.preventDefault(); e.stopPropagation();
    const key = r.getAttribute('data-k');
    addToPage(SIDE.picks.has(key) && SIDE.picks.size > 1 ? Array.from(SIDE.picks) : [key]);
  });
  const tabs = H('div', { class: 'stabs', id: 'sideTabs', role: 'tablist', hidden: '' });
  box.parentNode.insertBefore(tabs, box);
  tabs.addEventListener('click', (e) => { const b = e.target.closest('[data-stab]'); if (b) setSource(b.getAttribute('data-stab') === 'sys'); });
}

// the tree of the current page: System — the whole navigation; Data / API / Statuses — their section of it; a page of
// arch/pages — its frames and what is in them (views/pages.js), or the System navigation while it is the source
function pageTree() {
  const p = curPage();
  if (p.user && isSrc()) return navTree();
  if (p.user && AM.pages) return AM.pages.tree(p.page);
  const full = navTree();
  if (!p.sec) return full;
  return kidsOf(full).find((n) => n.key === p.sec) || full;
}
// a scenario started by the scheduler: its entry point is a task / cron (background jobs)
const isScheduled = (fl) => { const f = fl.steps && D.fns[fl.steps[0]], k = f && f.entry ? f.entry.kind : ''; return k === 'task' || k === 'cron'; };

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

/* ---------- the open scenario and the hash ---------- */
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
// the open scenario is kept in the hash (a reload or a shared link shows the same); closing it clears the hash.
// Old links of the former scenario window (with the solo flag) open the scenario here too and lose the flag.
const BASE_TITLE = document.title;
function writeHash() {
  // no scenario open: a page of arch/pages keeps its own hash (#page=<id>)
  const pg = !state.run && state.view === 'page' && AM.pages && AM.pages.cur ? AM.pages.cur : null;
  const want = state.run ? keyHash(state.run.key) : pg ? '#page=' + encodeURIComponent(pg) : '';
  document.title = state.run ? state.run.name + ' — ' + BASE_TITLE : pg ? curPage().name + ' — ' + BASE_TITLE : BASE_TITLE;
  if ((location.hash || '') === want) return;
  try { history.replaceState(null, '', want || location.pathname + location.search); } catch (e) { /* file:// in some browsers */ }
}
// openScenario(key, step): the scenario in this window — the canvas turns into its diagram (the participants only,
// map.js applyFocus); the navigation and the inspector stay. The same scenario again (at the same place) closes it.
function openScenario(key, step) {
  closeMenu();
  if (state.run && state.run.key === key && (step == null || step < 0 || step === state.step)) { closeRun(); return; }
  openRunAt(key, step);
}
async function openRunAt(key, step) {
  if (!state.run || state.run.key !== key) await openRun(key);
  if (step >= 0) { for (let i = 0; i < 40 && state.busy; i++) await sleep(100); AM.play.go(step); }
}
// the page of arch/pages the hash names (#page=<id>), when it exists
function hashPage() {
  const m = /^#page=([^&]+)/.exec(location.hash || '');
  const id = m ? decodeURIComponent(m[1]) : null;
  return id && AM.pages && AM.pages.has(id) ? id : null;
}
function setupHash() {
  bus.on('flow', writeHash);
  window.addEventListener('hashchange', () => {
    const r = hashRun(), pg = hashPage();
    if (r) openRunAt(r.key, r.step);
    else if (pg) { if (state.run) closeRun(); if (state.view !== 'page' || AM.pages.cur !== pg) AM.pages.open(pg); }
    else if (state.run && !location.hash) closeRun();
  });
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
  // diagrams: through paths (unfolded; a scenario of a path opens the path at its first step), the scheduled
  // scenarios (entry point task / cron), then every visible scenario by actor — folded, the filter still finds them
  const sk = [], js = jvJourneys(), fls = visibleFlows();
  const flowNode = (pre, fl) => N(pre + flowKey(fl), fl.name, 'flow', { rk: flowKey(fl), win: true, act: () => openScenario(flowKey(fl)) });
  if (js.length) sk.push(N('scp', t('nvPaths'), 'sec', { count: js.length, kids: js.map((j) => { const key = 'journey:' + j.id; return N('r:' + key, pathName(j), 'path', { title: j.name, rk: key, win: true, act: () => openScenario(key),
    kids: () => pathSegs(key).map((sg) => N('r:' + key + '@' + sg.at, sg.name, null, { no: sg.si + 1, win: true, act: () => openScenario(key, sg.at) })) }); }) }));
  const sched = fls.filter(isScheduled);
  if (sched.length) sk.push(N('scs', t('nvSched'), 'clock', { count: sched.length, kids: sched.map((fl) => flowNode('rs:', fl)) }));
  const groups = sideGroups();
  if (groups.length) sk.push(N('sca', t('nvAll'), 'all', { count: fls.length, kids: groups.map((g) => N('scg:' + g.id, g.name, 'role', { count: g.fls.length, kids: g.fls.map((fl) => flowNode('r:', fl)) })) }));
  if (sk.length) kids.push(N('sec:scen', t('nvScen'), 'sec', { count: fls.length + js.length, kids: sk }));
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
  if (state.run) return null; // the open scenario is marked by its own row (renderSide, n.rk)
  if (state.view === 'page' && AM.pages) return AM.pages.selKey();
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
// the folders above the row of an open scenario: a path, a scheduled one, else its actor in "All scenarios"
function runAncestors(key) {
  const a = ['sys', 'sec:scen'];
  if (key.startsWith('journey:')) return a.concat('scp');
  const fl = key.startsWith('flow:') ? D.flows.find((x) => x.i === +key.slice(5)) : null;
  if (!fl) return a;
  if (isScheduled(fl) && !fl.hidden) return a.concat('scs');
  const g = sideGroups().find((x) => x.fls.includes(fl));
  return g ? a.concat('sca', 'scg:' + g.id) : a;
}
// unfold the tree down to the selected node and scroll to it
function revealSel() {
  if (state.run) { runAncestors(state.run.key).forEach((x) => SIDE.open.add(x)); SIDE.reveal = true; return; }
  const k = selKey(); if (!k) return;
  (state.view === 'page' && AM.pages ? AM.pages.ancestors(k) : ancestors(k)).forEach((x) => SIDE.open.add(x));
  SIDE.reveal = true;
}

/* ---------- render ---------- */
function treeRow(n, d, open, hasKids, cur) {
  const chev = hasKids ? `<span class="chev${open ? ' on' : ''}" data-tog="${esc(n.key)}" title="${esc(t(open ? 'sdFold' : 'sdUnfold'))}">${SIDE_ICON.chev}</span>` : '<span class="chev"></span>';
  const tip = (n.title || n.name) + (n.win ? '\n' + t('nvWin') : '');
  const src = isSrc(), drag = src || (srcTabs() && /^pf:\d+$/.test(n.key));
  return `<button type="button" class="ti${n.cls ? ' ' + n.cls : ''}${n.key.startsWith('sec:') || n.key === 'scp' || n.key === 'scs' || n.key === 'sca' || n.key === 'sys' ? ' sec' : ''}${cur && !src ? ' cur' : ''}${src && SIDE.picks.has(n.key) ? ' pick' : ''}${drag ? ' dz' : ''}" style="--d:${d}" data-k="${esc(n.key)}" title="${esc(tip)}"${hasKids ? ` aria-expanded="${open}"` : ''}${cur ? ' aria-current="true"' : ''}>` +
    chev + (n.icon ? SIDE_ICON[n.icon] : '') + (n.no != null ? `<span class="no">${n.no}</span>` : '') + `<span class="nm">${esc(n.name)}</span>` + (n.count != null ? `<span class="ct">${n.count}</span>` : '') + '</button>';
}
let NAV = new Map(); // key -> node, for the clicks
function renderSide() {
  const box = $('#sideList'); if (!box || !D.F) return;
  const top = box.scrollTop, qq = lc(SIDE.q).trim(), cur = selKey(), out = [];
  NAV = new Map();
  // with a filter: a node shows when its name or a name under it matches; the matching branches are unfolded
  const hit = (n) => lc(n.name).includes(qq);
  const rk = state.run ? state.run.key : null, isCur = (n) => n.key === cur || (!!rk && n.rk === rk);
  const walk = (n, d) => {
    NAV.set(n.key, n);
    const ks = kidsOf(n), hasKids = ks.length > 0;
    let sub = [];
    if (qq) {
      const mine = hit(n), rows = [];
      ks.forEach((c) => { const r = walk(c, d + 1); if (r) rows.push(r); });
      if (!mine && !rows.length) return '';
      return treeRow(n, d, rows.length > 0, hasKids, isCur(n)) + rows.join('');
    }
    // the root of the page's tree is always unfolded
    const open = hasKids && (d === 0 || SIDE.open.has(n.key));
    if (open) sub = ks.map((c) => walk(c, d + 1));
    return treeRow(n, d, open, d > 0 && hasKids, isCur(n)) + sub.join('');
  };
  paintTabs();
  const h = walk(pageTree(), 0);
  box.innerHTML = (isSrc() ? `<div class="shint">${esc(t('peSrcHint'))}</div>` : '') + (h || `<div class="sn">${esc(t('sdNone'))}</div>`);
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
  paintHName(); paintChrome();
}
// the name of the current page in the centre of the toolbar (the project name with the sidebar hidden and on the
// System page); the run bar takes the place while a scenario is open
function paintHName() {
  const n = $('#hName'); if (!n) return;
  n.hidden = !!state.run;
  if (!D.F) return;
  const p = curPage(), proj = (D.F.project || {}).name || D.M.title || D.F.repo || 'archmap';
  n.textContent = p.id === 'system' && SIDE.hidden ? proj : (AM.pages && AM.pages.label ? AM.pages.label(p) : '') || p.name;
}
// the canvas changed its width: the scenes take the new size
const stageResized = () => { rebuildMap(); bus.emit('resize'); };
function setSideHidden(h) {
  if (SIDE.hidden === !!h) return;
  SIDE.hidden = !!h;
  try { localStorage.setItem('archmap.side', h ? '0' : '1'); } catch (e) { /* file:// without storage */ }
  paintSide();
  chromeResized();
}

/* ---------- the panels: hiding ----------
 * Every panel hides: the navigation (`[`), the inspector (`]`), the toolbar (`\`, its button or a double click on its
 * empty space; a small floating button in the corner of the canvas brings it back and the step navigation of an open
 * scenario moves to a floating plate at the bottom of the canvas). Cmd/Ctrl+. — the canvas only: everything at once,
 * and back to what was shown before. A column slides shut (CSS); once it has, the scenes take the new size and the
 * camera fits again, animated. All kept in localStorage: archmap.side, archmap.panel, archmap.top, archmap.focus. */
const CHR = { top: false, focus: null, rt: 0 };
const MAC = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent || '');
const chStore = (k, v) => { try { if (v == null) localStorage.removeItem(k); else localStorage.setItem(k, v); } catch (e) { /* file:// without storage */ } };
const chLoad = (k) => { try { return localStorage.getItem(k); } catch (e) { return null; } };
function chromeResized() {
  clearTimeout(CHR.rt);
  const ms = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--chrome-t')) || 0;
  CHR.rt = setTimeout(() => {
    rebuildMap(true);
    if (state.view === 'map') fitCur();
    bus.emit('resize');
  }, ms + 30);
}
function paintChrome() {
  const tip = (b, s, k) => { const el = $(b); if (!el) return; const v = s + (k ? '  ' + k : ''); el.title = v; el.setAttribute('aria-label', s); };
  const fk = (MAC ? '⌘' : 'Ctrl+') + '.';
  tip('#sideTog', t(SIDE.hidden ? 'sdShow' : 'sdHide'), '[');
  tip('#panelTog', t(PNL.closed ? 'chPanelShow' : 'chPanelHide'), ']');
  $('#panelTog').setAttribute('aria-expanded', String(!PNL.closed));
  tip('#topTog', t('chTopHide'), '\\');
  tip('#focusTog', t(CHR.focus ? 'chFocusOff' : 'chFocusOn'), fk);
  $('#focusTog').setAttribute('aria-pressed', String(!!CHR.focus));
  tip('#topFab', t(CHR.focus ? 'chFocusOff' : 'chTopShow'), CHR.focus ? fk : '\\');
}
function setTopHidden(h) {
  if (CHR.top === !!h) return;
  CHR.top = !!h;
  chStore('archmap.top', CHR.top ? '0' : null);
  paintTop();
  chromeResized();
}
function paintTop() {
  $('.app').classList.toggle('notop', CHR.top);
  $('header').toggleAttribute('inert', CHR.top);
  // the run bar: in the toolbar, or on the floating plate at the bottom of the canvas while the toolbar is folded
  const p = $('#player'), home = CHR.top ? $('#runDock') : $('header .hc');
  if (p.parentNode !== home) home.appendChild(p);
  paintChrome();
}
// the canvas only: hide everything, remember what was shown; again — bring it back
function setFocus(on) {
  if (!!CHR.focus === !!on) return;
  if (on) {
    CHR.focus = { side: SIDE.hidden, panel: PNL.closed, top: CHR.top };
    setSideHidden(true); setPanelHidden(true); setTopHidden(true);
    chStore('archmap.focus', JSON.stringify(CHR.focus));
  } else {
    const f = CHR.focus; CHR.focus = null; chStore('archmap.focus', null);
    setSideHidden(f.side); setPanelHidden(f.panel); setTopHidden(f.top);
  }
  paintChrome();
}
// a single panel toggled by hand ends the canvas-only mode without restoring the rest
function dropFocus() { if (CHR.focus) { CHR.focus = null; chStore('archmap.focus', null); paintChrome(); } }
function setupChrome() {
  CHR.top = chLoad('archmap.top') === '0';
  try { CHR.focus = JSON.parse(chLoad('archmap.focus') || 'null'); } catch (e) { CHR.focus = null; }
  // the first paint takes the stored layout without the slide
  document.documentElement.style.setProperty('--chrome-t', '0ms');
  paintTop();
  requestAnimationFrame(() => requestAnimationFrame(() => document.documentElement.style.removeProperty('--chrome-t')));
  $('#topTog').onclick = () => { dropFocus(); setTopHidden(true); };
  $('#focusTog').onclick = () => setFocus(!CHR.focus);
  $('#topFab').onclick = () => { if (CHR.focus) setFocus(false); else setTopHidden(false); };
  $('header').addEventListener('dblclick', (e) => { if (e.target.closest('button, input, .menu, .runbar, .hname')) return; dropFocus(); setTopHidden(true); });
  window.addEventListener('keydown', (e) => {
    const tg = e.target.tagName;
    if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT' || e.target.isContentEditable) return;
    if (e.key === '.' && (e.metaKey || e.ctrlKey) && !e.altKey) { e.preventDefault(); setFocus(!CHR.focus); }
    else if (e.key === '\\' && !e.metaKey && !e.ctrlKey && !e.altKey) { e.preventDefault(); dropFocus(); setTopHidden(!CHR.top); }
  });
  bus.on('lang', paintChrome);
  paintChrome();
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
  setupHash();
  let v = null, w = 0; try { v = localStorage.getItem('archmap.side'); w = +localStorage.getItem('archmap.side.w'); } catch (e) { /* ignore */ }
  SIDE.hidden = v === '0';
  if (w) setSideWidth(w);
  const box = $('#sideList');
  box.addEventListener('click', (e) => {
    const tg = e.target.closest('[data-tog]');
    if (tg) { const k = tg.getAttribute('data-tog'); if (SIDE.open.has(k)) SIDE.open.delete(k); else SIDE.open.add(k); renderSide(); return; }
    const r = e.target.closest('[data-k]'); if (!r) return;
    const n = NAV.get(r.getAttribute('data-k')); if (!n) return;
    if (Date.now() - DRAG.quiet < 350) return; // the end of a drag
    // the System navigation as the source of a page: a click picks (⌘/Ctrl/Shift — several), a folder folds
    if (isSrc()) {
      const it = itemOf(n.key);
      if (!it.refs && kidsOf(n).length && !(e.metaKey || e.ctrlKey || e.shiftKey)) { if (SIDE.open.has(n.key)) SIDE.open.delete(n.key); else SIDE.open.add(n.key); renderSide(); return; }
      if (e.metaKey || e.ctrlKey || e.shiftKey) { if (SIDE.picks.has(n.key)) SIDE.picks.delete(n.key); else SIDE.picks.add(n.key); } else { SIDE.picks.clear(); SIDE.picks.add(n.key); }
      paintPicks(); return;
    }
    closeMenu();
    if (n.act) {
      // another row of the tree leaves the open scenario: back to the full map first
      if (!n.win && state.run) closeRun();
      if (!n.win) { SIDE.last = n.key; SIDE.lastView = null; }
      Promise.resolve(n.act()).then(() => { if (!n.win) { SIDE.lastView = state.view; renderSide(); } });
      if (!n.win) renderSide();
    } else if (kidsOf(n).length) { if (SIDE.open.has(n.key)) SIDE.open.delete(n.key); else SIDE.open.add(n.key); renderSide(); }
  });
  $('#sideQ').addEventListener('input', (e) => { SIDE.q = e.target.value; renderSide(); });
  $('#sideQ').addEventListener('keydown', (e) => { if (e.key === 'Escape') { e.stopPropagation(); if (e.target.value) { e.target.value = ''; SIDE.q = ''; renderSide(); } else e.target.blur(); } });
  $('#sideTog').onclick = () => { dropFocus(); setSideHidden(!SIDE.hidden); };
  setupSideGrip();
  window.addEventListener('keydown', (e) => {
    const tg = e.target.tagName;
    if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT' || e.target.isContentEditable || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === '[') { e.preventDefault(); dropFocus(); setSideHidden(!SIDE.hidden); }
  });
  // the tree follows the selection on the map and in the views, and the open scenario
  const follow = () => { revealSel(); renderSide(); };
  ['select', 'level', 'view', 'flow'].forEach((ev) => bus.on(ev, follow));
  bus.on('view', () => { renderPages(); paintHName(); writeHash(); });
  bus.on('lang', () => { paintSide(); renderSide(); renderPages(); });
  bus.on('data', () => { paintSide(); renderSide(); renderPages(); });
  bus.on('live', () => { renderPages(); renderSide(); paintHName(); });
  setupPages(); setupDrag();
  bus.on('data', () => SIDE.picks.clear());
  paintSide(); renderSide();
}

Object.assign(AM, { pageList, curPage, openPage, renderPages, side: { render: renderSide, setSource, get source() { return isSrc(); }, setHidden: setSideHidden, setWidth: setSideWidth, get hidden() { return SIDE.hidden; }, get width() { return SIDE.w || SIDE_W.def; }, groups: sideGroups, realDesc, openScenario, hashRun },
  chrome: { setTop: setTopHidden, setFocus, get top() { return CHR.top; }, get focus() { return !!CHR.focus; } } });
