/* nav.js — global search (functions, modules, files), the lists drop-down (modules, functions,
 * branch changes) and the flash of what a live update changed (archmap serve). */

addI18n({
  ru: {
    search: 'Поиск: функция, модуль или файл…', searchNone: 'Ничего не найдено', searchFns: 'Функции', searchMods: 'Модули', searchFiles: 'Файлы',
    searchHint: '↑↓ выбрать · ↵ на карту · ⇧↵ в код · Esc закрыть', lists: 'Списки', listsTip: 'Модули, функции и изменения ветки',
    tabMods: 'Модули', tabFns: 'Функции', tabChg: 'Изменения', fnFilter: 'Фильтр функций…', moreRows: (n) => '+ ещё ' + n + ' — уточните фильтр', deletedFns: 'Удалённые',
    tabFlows: 'Сценарии', flowsBtn: (n) => 'Сценарии (' + n + ')', flowsTip: 'Все сценарии: поиск, группы по актору', flowsFilter: 'Поиск сценария или актора…',
    grpAuto: 'Авто — без разметки', noActor: 'Без актора', flowsAll: (n) => 'Показать все (' + n + ')', flowsFewer: 'Свернуть', flowsBrowse: 'Открыть браузер сценариев', flowSteps: (n) => n + ' шаг.',
    ms: 'мс', changedN: 'изм.', noConnTip: 'Сервер archmap не отвечает; страница переподключится сама', closeLists: 'Закрыть',
  },
  en: {
    search: 'Search: function, module or file…', searchNone: 'Nothing found', searchFns: 'Functions', searchMods: 'Modules', searchFiles: 'Files',
    searchHint: '↑↓ select · ↵ on the map · ⇧↵ to code · Esc close', lists: 'Lists', listsTip: 'Modules, functions and branch changes',
    tabMods: 'Modules', tabFns: 'Functions', tabChg: 'Changes', fnFilter: 'Filter functions…', moreRows: (n) => '+ ' + n + ' more — refine the filter', deletedFns: 'Deleted',
    tabFlows: 'Scenarios', flowsBtn: (n) => 'Scenarios (' + n + ')', flowsTip: 'All scenarios: search, grouped by actor', flowsFilter: 'Search a scenario or actor…',
    grpAuto: 'Auto — not annotated', noActor: 'No actor', flowsAll: (n) => 'Show all (' + n + ')', flowsFewer: 'Collapse', flowsBrowse: 'Open the scenario browser', flowSteps: (n) => n + ' steps',
    ms: 'ms', changedN: 'changed', noConnTip: 'The archmap server does not answer; the page reconnects by itself', closeLists: 'Close',
  },
});

/* ---------- search ---------- */
const SR = { open: false, act: 0, items: [] };
const lc = (s) => String(s || '').toLowerCase();
// score of a name against a query token: higher is better, 0 — no match
function tokScore(name, tok) {
  if (name === tok) return 100;
  if (name.startsWith(tok)) return 80;
  const i = name.indexOf(tok);
  if (i < 0) return 0;
  return /[^a-z0-9]/.test(name[i - 1] || ' ') || /[A-Z]/.test(name[i] || '') ? 60 : 40;
}
function searchAll(q) {
  const toks = lc(q).split(/\s+/).filter(Boolean);
  if (!toks.length) return [];
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
  D.allMods.forEach((m) => {
    const nm = lc(modLabel(m));
    let sc = 0, ok = true;
    for (const tk of toks) { const a = tokScore(nm, tk) || tokScore(lc(m.split('/').pop()), tk); if (!a) { ok = false; break; } sc += a; }
    if (ok) mods.push({ k: 'mod', id: m, sc: sc + 5 });
  });
  const fl = [];
  files.forEach((o) => {
    const path = lc(o.id); let sc = 0, ok = true;
    for (const tk of toks) { const a = tokScore(path.split('/').pop(), tk) || tokScore(path, tk) / 2; if (!a) { ok = false; break; } sc += a; }
    if (ok) { o.sc = sc; fl.push(o); }
  });
  const by = (a, b) => b.sc - a.sc || (a.id < b.id ? -1 : 1);
  return fns.sort(by).slice(0, 9).concat(mods.sort(by).slice(0, 4), fl.sort(by).slice(0, 4));
}
function srRow(o, i) {
  let ico, nm, sub, st = '';
  if (o.k === 'fn') {
    const f = fnInfo(o.id); ico = 'ƒ'; nm = f.name; sub = `${short(f.mod)} · ${(f.file || '').split('/').pop()}:${f.s}`;
    if (f.status) st = `<span class="st-${f.status === 'added' ? 'a' : 'm'}">${f.status === 'added' ? '+' : '~'}</span>`;
  } else if (o.k === 'mod') { ico = '▢'; nm = short(o.id); sub = (D.byMod[o.id] || []).length + ' ' + t('fnShort'); }
  else { ico = '▤'; nm = o.id.split('/').pop(); sub = cutL(o.id, 46); }
  return `<div class="qi${i === SR.act ? ' act' : ''}" role="option" data-i="${i}" title="${esc(o.k === 'fn' ? o.id : o.k === 'file' ? o.id : modLabel(o.id))}"><b class="ico">${ico}</b><span class="nm">${esc(nm)}</span>${st}<span class="sb">${esc(sub)}</span></div>`;
}
function drawSearch() {
  const q = $('#q').value.trim(), pop = $('#qpop');
  if (!q) { closeSearch(); return; }
  SR.items = searchAll(q);
  SR.act = Math.max(0, Math.min(SR.act, SR.items.length - 1));
  let h = '', last = '';
  const hd = { fn: 'searchFns', mod: 'searchMods', file: 'searchFiles' };
  SR.items.forEach((o, i) => { if (o.k !== last) { last = o.k; h += `<div class="gh">${esc(t(hd[o.k]))}</div>`; } h += srRow(o, i); });
  if (!SR.items.length) h = `<div class="none">${esc(t('searchNone'))}</div>`;
  else h += `<div class="hint">${esc(t('searchHint'))}</div>`;
  pop.innerHTML = h; pop.hidden = false; SR.open = true;
}
function closeSearch() { SR.open = false; $('#qpop').hidden = true; }
function markSearch(scroll) {
  const pop = $('#qpop'), p = pop.querySelector('.qi.act'); if (p) p.classList.remove('act');
  const n = pop.querySelector(`.qi[data-i="${SR.act}"]`);
  if (n) { n.classList.add('act'); if (scroll) n.scrollIntoView({ block: 'nearest' }); }
}
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
function pickSearch(i, code) {
  const o = SR.items[i]; if (!o) return;
  closeSearch(); $('#q').blur();
  navTo(o, code);
}
function setupSearch() {
  const q = $('#q'); let tm = null;
  q.placeholder = t('search');
  q.addEventListener('input', () => { clearTimeout(tm); tm = setTimeout(() => { SR.act = 0; drawSearch(); }, 70); });
  q.addEventListener('focus', () => { if (q.value.trim()) drawSearch(); });
  q.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'ArrowDown') { e.preventDefault(); if (!SR.open) drawSearch(); SR.act = Math.min(SR.items.length - 1, SR.act + 1); markSearch(true); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); SR.act = Math.max(0, SR.act - 1); markSearch(true); }
    else if (e.key === 'Enter') { e.preventDefault(); clearTimeout(tm); if (!SR.open) drawSearch(); pickSearch(SR.act, e.shiftKey || e.metaKey || e.ctrlKey); }
    else if (e.key === 'Escape') { e.preventDefault(); if (SR.open) closeSearch(); else { q.value = ''; q.blur(); } }
  });
  $('#qpop').addEventListener('mousedown', (e) => { const r = e.target.closest('.qi'); if (r) { e.preventDefault(); pickSearch(+r.dataset.i, e.shiftKey); } });
  // close on a click anywhere outside (capture: scenes stop the bubbling of their own events), on blur
  // (late enough for a click on a row) and when the window loses focus
  document.addEventListener('pointerdown', (e) => { if (SR.open && !e.target.closest('#search')) closeSearch(); }, true);
  document.addEventListener('mousedown', (e) => { if (SR.open && !e.target.closest('#search')) closeSearch(); }, true);
  q.addEventListener('blur', () => setTimeout(() => { if (document.activeElement !== q) closeSearch(); }, 160));
  window.addEventListener('blur', () => closeSearch());
  window.addEventListener('keydown', (e) => {
    const tg = e.target.tagName, typing = tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT' || e.target.isContentEditable;
    if ((e.key === '/' && !typing && !e.metaKey && !e.ctrlKey && !e.altKey) || ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k')) {
      e.preventDefault(); q.focus(); q.select();
    }
  });
}

/* ---------- lists drop-down ---------- */
const LS = { open: false, tab: 'mods', q: '' };
const LIST_MAX = 200;
function rowFn(id) {
  const f = fnInfo(id), st = f.status ? `<span class="st-${f.status === 'added' ? 'a' : 'm'}">${f.status === 'added' ? '+' : '~'}</span>` : '';
  return `<div class="lrow${state.fn === id ? ' cur' : ''}" data-k="fn" data-id="${esc(id)}" title="${esc(id)}">${st}<span class="nm mono">${esc(f.name)}</span><span class="sb">${esc((f.file || '').split('/').pop())}:${f.s}</span></div>`;
}
function rowMod(m) {
  const c = D.modChg[m], l = layerOf(m);
  return `<div class="lrow${state.mod === m ? ' cur' : ''}" data-k="mod" data-id="${esc(m)}" title="${esc(modLabel(m))}"><i class="dot" style="background:${LCOLOR[l]}"></i><span class="nm">${esc(short(m))}</span>${hasChg(c) ? `<span class="chgt">${esc(chgText(c))}</span>` : ''}<span class="sb">${(D.byMod[m] || []).length}</span></div>`;
}
/* scenarios: the actor of a flow, groups by actor (declared) then the auto ones, play on click */
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
  if (auto.length) gs.push({ name: by.size ? t('grpAuto') : t('grpFlows'), items: topFirst(auto), auto: true });
  return gs;
}
// rows of the journeys and variants share the builders in journeys.js; cfg tells them the classes of this list
const jvCfg = (cls) => ({ row: cls, head: (title, n) => (cls === 'sc' ? `<div class="scen-grp">${esc(title)}<small>${n}</small></div>` : `<div class="gh">${esc(title)} · ${n}</div>`), attr: (k, id) => `data-k="${k}" data-id="${esc(id)}"` });
const flowRow = (fl, cls = 'lrow', q = '') => `<div class="${cls}${state.run && state.run.key === 'flow:' + fl.i ? ' cur' : ''}" data-k="flow" data-id="${fl.i}" title="${esc(fl.name)}"><span class="nm">${esc(fl.name)}</span><span class="sb">${esc(t('flowSteps', fl.steps.length))}</span>${AM.jv.tog(fl)}</div>` + AM.jv.variantRows(fl, q, jvCfg(cls));
function playRunKey(key) {
  if (state.view !== 'map') setView('map');
  loadRun(key); auto();
}
// click on a row of a journey / variant / the fold button of a flow; true when it was one of them
function jvClick(e) {
  const vt = e.target.closest('[data-vt]'); if (vt) { e.stopPropagation(); AM.jv.toggle(vt.dataset.vt); return 'vt'; }
  const r = e.target.closest('[data-k="journey"],[data-k="variant"]'); if (!r) return false;
  playRunKey(r.dataset.k + ':' + r.dataset.id); return r.dataset.k;
}
function playFlowIdx(i) {
  if (state.view !== 'map') setView('map');
  loadRun('flow:' + i); auto();
}
function openFlows() {
  LS.tab = 'flows'; LS.q = ''; toggleLists(true);
  const i = $('#lists .lq'); if (i) i.focus();
}
function paintFlowsBtn() {
  const b = $('#flowsBtn'); if (!b) return;
  b.hidden = !D.flows.length; b.textContent = t('flowsBtn', D.flows.length); b.title = t('flowsTip');
}
function listBody() {
  const q = lc(LS.q).trim(); let h = '';
  if (LS.tab === 'flows') {
    const gs = flowGroups(LS.q);
    h = AM.jv.journeyRows(LS.q, jvCfg('lrow')) + gs.map((g) => `<div class="gh">${esc(g.name)} · ${g.items.length}</div>` + g.items.map((fl) => flowRow(fl, 'lrow', LS.q)).join('')).join('');
    if (!h) h = `<div class="none">${esc(t('flowNone'))}</div>`;
  } else if (LS.tab === 'mods') {
    LAYERS.forEach((l) => {
      const ms = D.allMods.filter((m) => layerOf(m) === l && (!state.only || hasChg(D.modChg[m])));
      if (!ms.length) return;
      h += `<div class="gh"><i class="dot" style="background:${LCOLOR[l]}"></i>${esc(t('layers')[l])} · ${ms.length}</div>` + ms.map(rowMod).join('');
    });
  } else if (LS.tab === 'fns') {
    const toks = q.split(/\s+/).filter(Boolean);
    const all = D.fids.filter((id) => { const f = D.fns[id]; if (state.only && !f.status) return false; const hay = lc(fname(f) + ' ' + f.file); return toks.every((tk) => hay.includes(tk)); });
    all.sort((a, b) => (D.fns[b].status ? 1 : 0) - (D.fns[a].status ? 1 : 0) || (D.fns[a].file < D.fns[b].file ? -1 : D.fns[a].file > D.fns[b].file ? 1 : D.fns[a].start - D.fns[b].start));
    h = all.slice(0, LIST_MAX).map(rowFn).join('') + (all.length > LIST_MAX ? `<div class="more">${esc(t('moreRows', all.length - LIST_MAX))}</div>` : '') + (all.length ? '' : `<div class="none">${esc(t('searchNone'))}</div>`);
  } else {
    const by = {};
    D.changed.forEach((id) => { (by[D.fns[id].module] = by[D.fns[id].module] || []).push(id); });
    Object.keys(by).sort().forEach((m) => { h += `<div class="gh lnk" data-k="mod" data-id="${esc(m)}">${esc(short(m))} · ${by[m].length}</div>` + by[m].map(rowFn).join(''); });
    if (D.deleted.length) h += `<div class="gh">${esc(t('deletedFns'))} · ${D.deleted.length}</div>` + D.deleted.slice(0, 100).map((d) => `<div class="lrow del"><span class="st-d">−</span><span class="nm mono">${esc(d.name || d.fid || '')}</span><span class="sb">${esc(d.file ? d.file.split('/').pop() : short(d.module || ''))}</span></div>`).join('');
    if (!h) h = `<div class="none">${esc(t('searchNone'))}</div>`;
  }
  return h;
}
function drawLists() {
  const box = $('#lists'); if (!box) return;
  const hasChgs = D.changed.length || D.deleted.length;
  if (LS.tab === 'chg' && !hasChgs) LS.tab = 'mods';
  if (LS.tab === 'flows' && !D.flows.length) LS.tab = 'mods';
  const tabs = [['mods', 'tabMods', D.allMods.length], ['fns', 'tabFns', D.fids.length]].concat(D.flows.length ? [['flows', 'tabFlows', D.flows.length]] : []).concat(hasChgs ? [['chg', 'tabChg', D.changed.length + D.deleted.length]] : []);
  const top = box.querySelector('.lbody') ? box.querySelector('.lbody').scrollTop : 0;
  box.innerHTML = `<div class="lhead"><div class="ltabs" role="tablist">${tabs.map(([k, lab, n]) => `<button role="tab" data-tab="${k}" aria-selected="${LS.tab === k}">${esc(t(lab))}<small>${n}</small></button>`).join('')}</div><button class="x" data-close title="${esc(t('closeLists'))}">×</button></div>` +
    (LS.tab === 'fns' || LS.tab === 'flows' ? `<input type="search" class="lq" placeholder="${esc(t(LS.tab === 'flows' ? 'flowsFilter' : 'fnFilter'))}" value="${esc(LS.q)}" autocomplete="off">` : '') +
    `<div class="lbody">${listBody()}</div>`;
  box.querySelector('.lbody').scrollTop = top;
  const inp = box.querySelector('.lq');
  if (inp) {
    let tm = null;
    inp.addEventListener('input', () => { clearTimeout(tm); tm = setTimeout(() => { LS.q = inp.value; const b = box.querySelector('.lbody'); b.innerHTML = listBody(); b.scrollTop = 0; }, 80); });
    inp.addEventListener('keydown', (e) => { e.stopPropagation(); if (e.key === 'Escape') { e.preventDefault(); toggleLists(false); } });
  }
}
function toggleLists(on) {
  LS.open = on == null ? !LS.open : on;
  $('#lists').hidden = !LS.open; $('#listBtn').setAttribute('aria-expanded', LS.open);
  if (LS.open) { drawLists(); const i = $('#lists .lq'); if (i && (LS.tab === 'fns' || LS.tab === 'flows')) i.focus(); }
}
function setupLists() {
  const box = $('#lists'), btn = $('#listBtn');
  btn.title = t('listsTip'); btn.querySelector('span').textContent = t('lists');
  btn.onclick = () => toggleLists();
  const fb = $('#flowsBtn'); if (fb) fb.onclick = () => (LS.open && LS.tab === 'flows' ? toggleLists(false) : openFlows());
  paintFlowsBtn();
  box.addEventListener('click', (e) => {
    if (e.target.closest('[data-close]')) return toggleLists(false);
    const tb = e.target.closest('[data-tab]'); if (tb) { if (LS.tab !== tb.dataset.tab) LS.q = ''; LS.tab = tb.dataset.tab; drawLists(); return; }
    const jk = jvClick(e); if (jk === 'vt') return; if (jk) { toggleLists(false); return; }
    const r = e.target.closest('[data-k]'); if (!r) return;
    const k = r.dataset.k, id = r.dataset.id;
    if (k === 'flow') { toggleLists(false); playFlowIdx(+id); return; }
    navTo(k === 'fn' ? { k, id } : { k: 'mod', id }, e.shiftKey);
  });
  window.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && LS.open && !e.target.closest('#search') && !PK.open) { e.stopImmediatePropagation(); toggleLists(false); }
  });
  bus.on('select', () => { if (LS.open) markCur(); });
  bus.on('jv', () => { if (LS.open && LS.tab === 'flows') drawLists(); });
}
// highlight the current function / module in the open list without redrawing it
function markCur() {
  $$('#lists .lrow.cur').forEach((e) => e.classList.remove('cur'));
  const s = state.sel, id = s && s.t === 'fn' ? s.id : null, m = s && s.t === 'mod' ? s.id : state.mod;
  $$('#lists .lrow').forEach((e) => { if ((e.dataset.k === 'fn' && e.dataset.id === id) || (e.dataset.k === 'mod' && e.dataset.id === m)) e.classList.add('cur'); });
}
function relangNav() { paintFlowsBtn(); $('#q').placeholder = t('search'); const b = $('#listBtn'); b.title = t('listsTip'); b.querySelector('span').textContent = t('lists'); if (SR.open) drawSearch(); if (LS.open) drawLists(); }

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
  $$('#lists .lrow').forEach((e) => { const fn = e.dataset.k === 'fn' && e.dataset.id, mo = e.dataset.k === 'mod' && e.dataset.id; if ((fn && f(fn)) || (mo && m(mo))) mark(e, (fn && isF(fn)) || (mo && isM(mo))); });
  // views
  $$('.seqdock .msg:not(.reply)[data-fid]').forEach((g) => { const id = g.dataset.fid; if (id && f(id)) mark(g, isF(id)); });
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
bus.on('filter', () => { if (LS.open) drawLists(); });
bus.on('level', () => { if (SES.fns.size) requestAnimationFrame(applyFlash); });
bus.on('view', () => { if (SES.fns.size) requestAnimationFrame(applyFlash); });
Object.assign(AM, { flowGroups, flowActor });
