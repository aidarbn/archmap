/* nav.js — ⌘K, the one control of the page: a field in the header and a palette under it that finds modules,
 * functions, files, scenarios (flows and through paths), API endpoints, tables and state machines, and runs the
 * commands (view, language, theme, details panel, back, close the scenario). Also: the flash of what a live
 * update changed (archmap serve). */

addI18n({
  ru: {
    search: 'Найти или выполнить…', searchNone: 'Ничего не найдено', searchFns: 'Функции', searchMods: 'Модули', searchFiles: 'Файлы',
    searchHint: '↑↓ выбрать · ↵ открыть · ⇧↵ функция в код · Esc закрыть',
    pkCmds: 'Команды', pkFlows: 'Сценарии', pkEps: 'API', pkTables: 'Таблицы', pkStates: 'Статусы',
    pkView: (v) => 'Вид: ' + v, pkLang: 'Язык: English', pkTheme: (d) => (d ? 'Тема: светлая' : 'Тема: тёмная'), pkPanel: (h) => (h ? 'Показать панель деталей' : 'Скрыть панель деталей'),
    pkBack: 'Назад', pkClose: 'Закрыть сценарий', pkOnly: (on) => (on ? 'Показать все, не только изменённые' : 'Только изменённые в ветке'),
    pkPath: 'сквозной путь', pkStepsN: (n, p) => n + ' ' + p(['шаг', 'шага', 'шагов']), pkStatesN: (n, p) => n + ' ' + p(['статус', 'статуса', 'статусов']),
    pkColsN: (n, p) => n + ' ' + p(['колонка', 'колонки', 'колонок']),
    grpAuto: 'Авто — без разметки', noActor: 'Без актора',
    ms: 'мс', changedN: 'изм.', noConnTip: 'Сервер archmap не отвечает; страница переподключится сама',
  },
  en: {
    search: 'Find or run…', searchNone: 'Nothing found', searchFns: 'Functions', searchMods: 'Modules', searchFiles: 'Files',
    searchHint: '↑↓ select · ↵ open · ⇧↵ a function to code · Esc close',
    pkCmds: 'Commands', pkFlows: 'Scenarios', pkEps: 'API', pkTables: 'Tables', pkStates: 'Statuses',
    pkView: (v) => 'View: ' + v, pkLang: 'Язык: русский', pkTheme: (d) => (d ? 'Theme: light' : 'Theme: dark'), pkPanel: (h) => (h ? 'Show the details panel' : 'Hide the details panel'),
    pkBack: 'Back', pkClose: 'Close the scenario', pkOnly: (on) => (on ? 'Show all, not only changed' : 'Only changed on the branch'),
    pkPath: 'through path', pkStepsN: (n) => n + (n === 1 ? ' step' : ' steps'), pkStatesN: (n) => n + (n === 1 ? ' status' : ' statuses'),
    pkColsN: (n) => n + (n === 1 ? ' column' : ' columns'),
    grpAuto: 'Auto — not annotated', noActor: 'No actor',
    ms: 'ms', changedN: 'changed', noConnTip: 'The archmap server does not answer; the page reconnects by itself',
  },
});

/* ---------- matching ---------- */
const SR = { open: false, act: 0, items: [], openG: new Set() };
const lc = (s) => String(s || '').toLowerCase();
// score of a name against a query token: higher is better, 0 — no match
function tokScore(name, tok) {
  if (name === tok) return 100;
  if (name.startsWith(tok)) return 80;
  const i = name.indexOf(tok);
  if (i < 0) return 0;
  return /[^a-z0-9]/.test(name[i - 1] || ' ') || /[A-Z]/.test(name[i] || '') ? 60 : 40;
}
// every token must hit one of the texts (the first ones count more); 0 — no match
function hits(texts, toks) {
  let sc = 0;
  for (const tk of toks) {
    let best = 0;
    texts.forEach((x, i) => { if (x) best = Math.max(best, tokScore(lc(x), tk) / (i ? 2 : 1)); });
    if (!best) return 0;
    sc += best;
  }
  return sc;
}
function searchCode(toks) {
  const fns = [], mods = [], files = new Map();
  D.fids.forEach((id) => {
    const f = D.fns[id], nm = lc(fname(f)), path = lc(f.file);
    let sc = 0, ok = true;
    for (const tk of toks) {
      const a = tokScore(nm, tk) || tokScore(lc(f.name), tk), b = a ? 0 : tokScore(path, tk) / 4 || tokScore(lc(f.module), tk) / 4;
      if (!a && !b) { ok = false; break; }
      sc += a || b;
    }
    if (ok) fns.push({ k: 'fn', id, sc: sc + (f.status ? 6 : 0) + (f.entry ? 3 : 0) });
    if (f.file && !files.has(f.file)) files.set(f.file, { k: 'file', id: f.file, mod: f.module, fid: id, sc: 0 });
  });
  D.allMods.forEach((m) => { const sc = hits([modLabel(m), m.split('/').pop(), modDesc(m)], toks); if (sc) mods.push({ k: 'mod', id: m, sc: sc + 5 }); });
  const fl = [];
  files.forEach((o) => { const sc = hits([o.id.split('/').pop(), o.id], toks); if (sc) { o.sc = sc; fl.push(o); } });
  return { fns, mods, files: fl };
}
/* scenarios: the actor of a flow; groups by actor (declared) then the auto ones */
const flowActor = (fl) => { if (fl.actor) return fl.actor; const a = fl.steps && fl.steps.length ? actorOfFn(fl.steps[0]) : null; return a ? actorName(a) : ''; };
function flowGroups(q, subset) {
  q = lc(q).trim();
  const ok = (fl) => !q || lc(fl.name).includes(q) || lc(flowActor(fl)).includes(q) || AM.jv.flowHit(fl, q);
  const by = new Map(), auto = [];
  (subset || D.flows).filter(ok).forEach((fl) => {
    if (fl.kind === 'declared') { const k = flowActor(fl) || t('noActor'); if (!by.has(k)) by.set(k, []); by.get(k).push(fl); } else auto.push(fl);
  });
  const topFirst = (a) => a.map((x, i) => [x, i]).sort((p, q2) => (q2[0].top ? 1 : 0) - (p[0].top ? 1 : 0) || p[1] - q2[1]).map((p) => p[0]);
  const gs = Array.from(by, ([name, items]) => ({ name, items: topFirst(items) })).sort((a, b) => b.items.length - a.items.length || a.name.localeCompare(b.name));
  if (auto.length) gs.push({ name: by.size ? t('grpAuto') : t('pkFlows'), items: topFirst(auto), auto: true });
  return gs;
}
function searchFlows(toks) {
  const out = [];
  jvJourneys().forEach((j) => { const sc = toks.length ? hits([j.name, j.actor, j.desc], toks) : 50; if (sc) out.push({ k: 'run', id: 'journey:' + j.id, nm: j.name, sb: t('pkPath'), sc: sc + 4 }); });
  D.flows.forEach((fl) => {
    const sc = toks.length ? hits([fl.name, flowActor(fl), fl.desc], toks) : (fl.top ? 40 : fl.kind === 'declared' ? 30 : 0);
    if (sc) out.push({ k: 'run', id: 'flow:' + fl.i, nm: fl.name, sb: [flowActor(fl), t('pkStepsN', fl.steps.length)].filter(Boolean).join(' · '), sc: sc + (fl.kind === 'declared' ? 3 : 0) + (fl.top ? 2 : 0) });
  });
  return out;
}
function searchData(toks) {
  const eps = [], tbls = [], sts = [];
  (D.endpoints || []).forEach((e, i) => { const sc = hits([e.path, e.name, e.service, e.method, e.desc], toks); if (sc) eps.push({ k: 'ep', id: i, sc }); });
  Object.keys(D.tables).forEach((n) => { const tb = D.tables[n], sc = hits([n, tb.desc], toks); if (sc) tbls.push({ k: 'table', id: n, sc }); });
  Object.keys(D.states).forEach((n) => { const sc = hits([n], toks); if (sc) sts.push({ k: 'status', id: n, sc }); });
  return { eps, tbls, sts };
}
/* commands */
function commands() {
  const dark = document.documentElement.getAttribute('data-theme') === 'dark';
  const out = VIEWS.map((v) => ({ k: 'cmd', id: 'view:' + v.id, nm: t('pkView', viewTitle(v)), alt: [v.id, viewTitleIn(v, 'ru'), viewTitleIn(v, 'en'), 'view вид'], cur: state.view === v.id, run: () => setView(v.id) }));
  out.push({ k: 'cmd', id: 'lang', nm: t('pkLang'), alt: ['lang language язык ru en русский english'], run: () => setLang(LANG === 'ru' ? 'en' : 'ru') });
  out.push({ k: 'cmd', id: 'theme', nm: t('pkTheme', dark), alt: ['theme тема dark light тёмная светлая'], run: () => setTheme(dark ? 'light' : 'dark') });
  if (state.sel) { const h = AM.panel.hidden; out.push({ k: 'cmd', id: 'panel', nm: t('pkPanel', h), alt: ['panel панель details детали'], kbd: ']', run: () => AM.panel.setHidden(!h) }); }
  if (state.run) out.push({ k: 'cmd', id: 'close', nm: t('pkClose'), alt: ['close закрыть scenario сценарий'], kbd: 'Esc', run: () => closeRun() });
  if (state.view !== 'map' || state.level > 0) out.push({ k: 'cmd', id: 'back', nm: t('pkBack'), alt: ['back назад up вверх'], kbd: 'Esc', run: () => goBack() });
  if (D.changed.length || D.deleted.length) out.push({ k: 'cmd', id: 'only', nm: t('pkOnly', state.only), alt: ['changed изменённые only только diff'], run: () => setOnly(!state.only) });
  return out;
}
const viewTitleIn = (v, l) => { const keep = LANG; LANG = l; try { return viewTitle(v); } finally { LANG = keep; } };

const GROUPS = [['cmd', 'pkCmds', 6], ['mod', 'searchMods', 5], ['fn', 'searchFns', 8], ['run', 'pkFlows', 8], ['ep', 'pkEps', 6], ['table', 'pkTables', 6], ['status', 'pkStates', 4], ['file', 'searchFiles', 4]];
function paletteItems(q) {
  const toks = lc(q).split(/\s+/).filter(Boolean);
  const by = (a, b) => b.sc - a.sc || (String(a.id) < String(b.id) ? -1 : 1);
  const cmds = commands().map((c) => Object.assign(c, { sc: toks.length ? hits([c.nm].concat(c.alt || []), toks) : 1 })).filter((c) => c.sc);
  if (!toks.length) return cmds.concat(scenarioTree()); // an empty query: the commands and every scenario by actor
  const g = { cmd: cmds, run: searchFlows(toks) };
  const code = searchCode(toks), data = searchData(toks);
  Object.assign(g, { mod: code.mods, fn: code.fns, file: code.files, ep: data.eps, table: data.tbls, status: data.sts });
  const out = [];
  GROUPS.forEach(([k, , max]) => { if (g[k]) (k === 'cmd' ? g[k] : g[k].sort(by)).slice(0, max).forEach((o) => out.push(o)); });
  return out;
}
// every scenario grouped by actor (through paths in the group of their actor), folded groups: a header row per
// actor toggles its rows (SR.openG keeps what is unfolded while the page lives)
function scenarioTree() {
  const groups = flowGroups('').map((gr) => ({ name: gr.name, auto: gr.auto, items: gr.items.map((fl) => ({ k: 'run', id: 'flow:' + fl.i, nm: fl.name, sb: fl.exit ? cut(fl.exit, 60) : t('pkStepsN', fl.steps.length), sc: 0, sec: 'run', ind: true })) }));
  jvJourneys().forEach((j) => {
    const nm = (j.actor || '').trim(), gr = groups.find((x) => !x.auto && x.name.toLowerCase() === nm.toLowerCase()) || (() => { const x = { name: nm || t('pkPath'), items: [] }; groups.unshift(x); return x; })();
    gr.items.unshift({ k: 'run', id: 'journey:' + j.id, nm: j.name, sb: t('pkPath'), sc: 0, sec: 'run', ind: true });
  });
  const out = [];
  groups.forEach((gr) => {
    const open = SR.openG.has(gr.name);
    out.push({ k: 'grp', id: gr.name, nm: gr.name, n: gr.items.length, open, sec: 'run' });
    if (open) gr.items.forEach((o) => out.push(o));
  });
  return out;
}

/* ---------- rows ---------- */
function srRow(o, i) {
  let ico, nm, sub, st = '', code = false;
  if (o.k === 'fn') {
    const f = fnInfo(o.id); ico = 'ƒ'; nm = f.name; sub = `${short(f.mod)} · ${(f.file || '').split('/').pop()}:${f.s}`; code = true;
    if (f.status) st = `<span class="st-${f.status === 'added' ? 'a' : 'm'}">${f.status === 'added' ? '+' : '~'}</span>`;
  } else if (o.k === 'mod') { ico = '▢'; nm = short(o.id); sub = cut(modDesc(o.id), 60) || (D.byMod[o.id] || []).length + ' ' + t('fnShort'); code = true; }
  else if (o.k === 'file') { ico = '▤'; nm = o.id.split('/').pop(); sub = cutL(o.id, 46); code = true; }
  else if (o.k === 'run') { ico = '▶'; nm = o.nm; sub = o.sb; }
  else if (o.k === 'grp') { ico = o.open ? '▾' : '▸'; nm = o.nm; sub = String(o.n); }
  else if (o.k === 'ep') { const e = D.endpoints[o.id]; ico = '⇄'; nm = (e.method ? e.method + ' ' : '') + (e.path || e.name || ''); sub = e.desc ? cut(e.desc, 70) : e.service || e.kind || ''; code = true; }
  else if (o.k === 'table') { const tb = D.tables[o.id]; ico = '▦'; nm = o.id; sub = cut(tb.desc || '', 60) || t('pkColsN', (tb.fields || []).length); code = true; }
  else if (o.k === 'status') { const m = D.states[o.id] || {}; ico = '◎'; nm = o.id; sub = t('pkStatesN', (m.states || []).length); code = true; }
  else { ico = '›'; nm = o.nm; sub = o.kbd ? `<kbd>${esc(o.kbd)}</kbd>` : o.cur ? '✓' : ''; }
  const subHTML = o.k === 'cmd' ? sub : esc(sub);
  return `<div class="qi${code ? ' code' : ''}${o.k === 'grp' ? ' qgrp' : ''}${o.ind ? ' ind' : ''}${i === SR.act ? ' act' : ''}" role="option"${o.k === 'grp' ? ` aria-expanded="${o.open}"` : ''} data-i="${i}" title="${esc(o.k === 'fn' || o.k === 'file' ? o.id : nm)}"><b class="ico">${ico}</b><span class="nm">${esc(nm)}</span>${st}<span class="sb">${subHTML}</span></div>`;
}
function drawSearch() {
  const q = $('#q').value, pop = $('#qpop');
  SR.items = paletteItems(q);
  SR.act = Math.max(0, Math.min(SR.act, SR.items.length - 1));
  let h = '', last = '';
  const hd = Object.fromEntries(GROUPS.map(([k, label]) => [k, label]));
  SR.items.forEach((o, i) => { const sec = o.sec || o.k; if (sec !== last) { last = sec; h += `<div class="gh">${esc(t(hd[sec]))}</div>`; } h += srRow(o, i); });
  if (!SR.items.length) h = `<div class="none">${esc(t('searchNone'))}</div>`;
  h += `<div class="hint">${esc(t('searchHint'))}</div>`;
  pop.innerHTML = h; pop.hidden = false; SR.open = true; $('#q').setAttribute('aria-expanded', 'true');
}
function closeSearch() { SR.open = false; $('#qpop').hidden = true; $('#q').setAttribute('aria-expanded', 'false'); }
function markSearch(scroll) {
  const pop = $('#qpop'), p = pop.querySelector('.qi.act'); if (p) p.classList.remove('act');
  const n = pop.querySelector(`.qi[data-i="${SR.act}"]`);
  if (n) { n.classList.add('act'); if (scroll) n.scrollIntoView({ block: 'nearest' }); }
}

/* ---------- actions ---------- */
// open(o, code): function — its level on the map (2) or its code (3); module — its functions; file — its first function
async function navTo(o, code) {
  if (!o) return;
  if (state.view !== 'map') setView('map');
  if (state.busy) await waitIdle();
  if (o.k === 'fn') return jumpToFn(o.id, code ? 3 : 2);
  if (o.k === 'mod') return goModule(o.id);
  if (o.k === 'file') return o.fid && D.fns[o.fid] ? jumpToFn(o.fid, code ? 3 : 2) : goModule(o.mod);
}
async function waitIdle() { for (let i = 0; i < 40 && state.busy; i++) await sleep(100); }
function runItem(o, code) {
  if (o.k === 'cmd') return o.run();
  if (o.k === 'run') return openRun(o.id);
  if (o.k === 'ep') { setView('api'); bus.emit('data:open-endpoint', { ep: D.endpoints[o.id] }); return; }
  if (o.k === 'table') { setView('data'); bus.emit('data:focus', { table: o.id }); return; }
  if (o.k === 'status') { setView('status'); bus.emit('status:focus', { machine: o.id }); return; }
  return navTo(o, code);
}
function pickSearch(i, code) {
  const o = SR.items[i]; if (!o) return;
  if (o.k === 'grp') { // fold / unfold an actor's scenarios; the palette stays open on the same row
    if (SR.openG.has(o.id)) SR.openG.delete(o.id); else SR.openG.add(o.id);
    SR.act = i; drawSearch(); markSearch(true); return;
  }
  closeSearch(); $('#q').value = ''; $('#q').blur();
  runItem(o, code);
}
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
function setupSearch() {
  const q = $('#q'); let tm = null;
  q.placeholder = t('search');
  q.addEventListener('input', () => { clearTimeout(tm); tm = setTimeout(() => { SR.act = 0; drawSearch(); }, 60); });
  q.addEventListener('focus', () => { SR.act = 0; drawSearch(); });
  q.addEventListener('click', () => { if (!SR.open) drawSearch(); });
  q.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'ArrowDown') { e.preventDefault(); if (!SR.open) drawSearch(); SR.act = Math.min(SR.items.length - 1, SR.act + 1); markSearch(true); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); SR.act = Math.max(0, SR.act - 1); markSearch(true); }
    else if (e.key === 'Enter') { e.preventDefault(); clearTimeout(tm); drawSearch(); pickSearch(SR.act, e.shiftKey || e.metaKey || e.ctrlKey); }
    else if (e.key === 'Escape') { e.preventDefault(); closeSearch(); q.value = ''; q.blur(); }
  });
  $('#qpop').addEventListener('mousedown', (e) => { const r = e.target.closest('.qi'); if (r) { e.preventDefault(); pickSearch(+r.dataset.i, e.shiftKey); } });
  // close on a click anywhere outside (capture: scenes stop the bubbling of their own events), on blur
  // (late enough for a click on a row) and when the window loses focus
  document.addEventListener('pointerdown', (e) => { if (SR.open && !e.target.closest('#search')) closeSearch(); }, true);
  q.addEventListener('blur', () => setTimeout(() => { if (document.activeElement !== q) closeSearch(); }, 160));
  window.addEventListener('blur', () => closeSearch());
  window.addEventListener('keydown', (e) => {
    const tg = e.target.tagName, typing = tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT' || e.target.isContentEditable;
    if ((e.key === '/' && !typing && !e.metaKey && !e.ctrlKey && !e.altKey) || ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k')) {
      e.preventDefault(); q.focus(); q.select(); if (!SR.open) drawSearch();
    }
  });
}
function relangNav() { $('#q').placeholder = t('search'); if (SR.open) drawSearch(); }

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
Object.assign(AM, { flowGroups, flowActor, palette: { open: () => { $('#q').focus(); }, items: paletteItems, close: closeSearch } });
