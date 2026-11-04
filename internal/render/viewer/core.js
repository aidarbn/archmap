/* core.js — DOM helpers, i18n, data index (D), state, event bus, view/panel registry.
 * Core files (core, scene, layout, map, playback, panel, main) share one script scope;
 * each views/*.js is wrapped in its own block by the Go assembler and talks to the core
 * only through the global AM object (see docs/viewer.md). */

/* ---------- DOM helpers ---------- */
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));
const NS = 'http://www.w3.org/2000/svg';
// S_(tag, attrs, parent, text): SVG element
function S_(tag, attrs, parent, text) {
  const e = document.createElementNS(NS, tag);
  if (attrs) for (const k in attrs) if (attrs[k] != null) e.setAttribute(k, attrs[k]);
  if (text != null) e.textContent = text;
  if (parent) parent.appendChild(e);
  return e;
}
// H(tag, attrs, parent, html): HTML element; html is trusted markup — escape data with esc()
function H(tag, attrs, parent, html) {
  const e = document.createElement(tag);
  if (attrs) for (const k in attrs) { if (k === 'class') e.className = attrs[k]; else if (attrs[k] != null) e.setAttribute(k, attrs[k]); }
  if (html != null) e.innerHTML = html;
  if (parent) parent.appendChild(e);
  return e;
}
const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const RM = matchMedia('(prefers-reduced-motion: reduce)').matches;
const qs = new URLSearchParams(location.search);
const DUR = RM ? 0 : 420 * (+qs.get('slow') || 1); // ?slow=8 slows transitions down for inspection
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const ease = (t) => (t < .5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2);
const cut = (s, n) => (s = String(s || ''), s.length > n ? s.slice(0, n - 1) + '…' : s);
const cutL = (s, n) => (s = String(s || ''), s.length > n ? '…' + s.slice(-(n - 1)) : s);
const uniq = (a) => Array.from(new Set(a));

/* ---------- i18n ---------- */
// Interface strings only; texts taken from code (docs, flow names) are shown as is.
// A value is a string or a function; plural helper p(forms) is passed as the last argument
// to one-number functions: ru forms [one, few, many], en [one, other].
const I18N = {
  ru: {
    viewMap: 'Карта',
    lv: ['Система', 'Модули', 'Функции', 'Код'], up: '↖ Назад', into: 'Внутрь ↘', expand: 'Раскрыть ↘',
    step: 'Шаг', stepIn: 'Войти', stepOut: 'Выйти', auto: '▶ Авто', pause: '❚❚ Пауза', back: 'Шаг назад', reset: 'Сначала',
    start: 'Нажмите «Шаг» или «Авто» — сценарий пройдёт от входа до выхода', chooseBranch: 'Ветвление:', continueMain: 'дальше по сценарию',
    actor: 'актёр', exit: 'выход', modules: 'модулей', fns: 'функций', entries: 'точек входа', tables: 'таблиц',
    layers: { entry: 'Входы', ui: 'Интерфейс', domain: 'Домен', adapter: 'Адаптеры', infra: 'Инфраструктура', other: 'Прочее', test: 'Тесты' },
    layerSub: { entry: 'API, обработчики, воркеры, main', ui: 'экраны и компоненты', domain: 'бизнес-логика и сущности', adapter: 'БД, очереди, клиенты внешних систем', infra: 'конфиг и общие библиотеки', other: 'без слоя', test: 'тесты и фикстуры' },
    ext: 'Внешние системы', users: 'Пользователи', callers: 'Кто вызывает', callees: 'Кого вызывает', code: 'Код', sig: 'Сигнатура',
    shown: 'показано', of: 'из', moreFns: (n) => '+ ещё ' + n + ' — показать', lessFns: 'показать меньше', inCalls: 'вызывают модуль', outCalls: 'модуль вызывает',
    clickHint: 'клик — выделить · двойной клик — внутрь · Esc — назад · колесо — масштаб',
    noBody: 'код не встроен в карту', noBodyHint: 'Соберите карту с телом функций или откройте через archmap serve.', loadingSrc: 'загружаю код…',
    path: 'Путь вызова', stepN: 'Шаг', status: 'статус', data: 'данные', where: 'где в коде', callSite: 'Где вызывается', callee: 'Вызываемая функция',
    overviewMap: 'Одна сцена, четыре уровня понимания: система → модули → функции → код. Клик выделяет элемент и его связи, двойной клик или «Внутрь» — погружение, Esc или крошки — назад.',
    rootsHidden: (m) => 'Связи ' + m + ' скрыты: точка сборки импортирует почти все модули.',
    implBy: 'Реализует модуль', inScen: 'В сценарии', role: 'Роль', layer: 'Слой', link: 'связь', imports: 'Импортирует', importedBy: 'Импортируют его',
    autoZoom: 'Автопогружение', autoZoomTip: 'Приближение колесом или щипком погружает в элемент, отдаление — выходит наверх',
    zoomIn: 'двойной клик — внутрь', zIn: 'Приблизить', zOut: 'Отдалить', zFit: 'Вписать', theme: 'Тема',
    kinds: { actor: 'Пользователи', payment: 'Оплата', pos: 'Кассы', auth: 'Авторизация', db: 'База данных', cache: 'Кэш', queue: 'Очередь', storage: 'Хранилище', messaging: 'Сообщения', http_api: 'Внешние API', other: 'Другие системы' },
    actHttp: 'HTTP-клиенты', actUser: 'Пользователь', actMain: 'Запуск', actTask: 'Планировщик', actPublic: 'Публичный доступ', actOthers: 'Прочие роли',
    epsN: (n, p) => n + ' ' + p(['ручка', 'ручки', 'ручек']), callsN: (n, p) => n + ' ' + p(['вызов', 'вызова', 'вызовов']),
    readsWrites: 'читает и пишет', calls: 'вызывает',
    onlyChanged: 'только изменённые', branch: 'ветка', uncommitted: 'не в коммите', changesTip: 'изменения ветки: добавлено / изменено / удалено / не в коммите',
    st_added: 'добавлена', st_modified: 'изменена', st_deleted: 'удалена',
    grpMods: (n, p) => n + ' ' + p(['модуль', 'модуля', 'модулей']), grpHint: 'группа модулей — двойной клик раскрывает', collapse: 'свернуть', group: 'группа', root: '(корень)',
    fnShort: 'ф.', modShort: 'мод.', lines: 'стр.', entryLbl: 'точка входа', fnsFiles: (f, fl) => 'функций: ' + f + ', файлов: ' + fl,
    flowSearch: 'Поиск сценария…', grpTop: 'Главные', grpGeneral: 'Общее', grpFlows: 'Сценарии', flowMore: (n) => '…ещё ' + n + ' — уточните поиск', flowNone: 'Сценарии не найдены',
    optChanges: 'Изменённые функции: путь от входов', optOverview: 'Обзор: вызовы от точек входа', fnFlowTitle: 'Сценарий от ', playFromFn: '▶ Сценарий от этой функции', play: '▶ Проиграть',
    noPlay: 'Нет сценариев для проигрывания', queue: 'очередь', viaQueue: 'через очередь', next: 'далее', end: 'конец',
    stepsN: (n, p) => n + ' ' + p(['шаг', 'шага', 'шагов']),
    live: 'живое', rebuilding: 'пересборка…', error: 'ошибка', noConn: 'нет связи',
    noData: 'Нет данных: файл нужно собирать командой archmap map.',
  },
  en: {
    viewMap: 'Map',
    lv: ['System', 'Modules', 'Functions', 'Code'], up: '↖ Back', into: 'Inside ↘', expand: 'Expand ↘',
    step: 'Step', stepIn: 'Step in', stepOut: 'Step out', auto: '▶ Auto', pause: '❚❚ Pause', back: 'Step back', reset: 'Restart',
    start: 'Press “Step” or “Auto” — the scenario runs from entry to exit', chooseBranch: 'Branch:', continueMain: 'continue the scenario',
    actor: 'actor', exit: 'exit', modules: 'modules', fns: 'functions', entries: 'entry points', tables: 'tables',
    layers: { entry: 'Entry', ui: 'Interface', domain: 'Domain', adapter: 'Adapters', infra: 'Infrastructure', other: 'Other', test: 'Tests' },
    layerSub: { entry: 'API, handlers, workers, main', ui: 'screens and components', domain: 'business logic and entities', adapter: 'DB, queues, external clients', infra: 'config and shared libs', other: 'no layer', test: 'tests and fixtures' },
    ext: 'External systems', users: 'Users', callers: 'Called by', callees: 'Calls', code: 'Code', sig: 'Signature',
    shown: 'showing', of: 'of', moreFns: (n) => '+ ' + n + ' more — show', lessFns: 'show fewer', inCalls: 'call this module', outCalls: 'this module calls',
    clickHint: 'click — select · double-click — inside · Esc — back · wheel — zoom',
    noBody: 'code is not embedded in the map', noBodyHint: 'Build the map with function bodies or open it via archmap serve.', loadingSrc: 'loading code…',
    path: 'Call path', stepN: 'Step', status: 'status', data: 'data', where: 'where in code', callSite: 'Call site', callee: 'Callee',
    overviewMap: 'One scene, four levels of understanding: system → modules → functions → code. Click selects an element and its links; double-click or “Inside” dives in; Esc or breadcrumbs go back.',
    rootsHidden: (m) => m + ' edges hidden: the composition root imports almost every module.',
    implBy: 'Implemented by', inScen: 'In scenario', role: 'Role', layer: 'Layer', link: 'link', imports: 'Imports', importedBy: 'Imported by',
    autoZoom: 'Auto-dive', autoZoomTip: 'Zooming in with the wheel or a pinch dives into the element, zooming out goes up',
    zoomIn: 'double-click — inside', zIn: 'Zoom in', zOut: 'Zoom out', zFit: 'Fit', theme: 'Theme',
    kinds: { actor: 'Users', payment: 'Payments', pos: 'POS', auth: 'Auth', db: 'Database', cache: 'Cache', queue: 'Queue', storage: 'Storage', messaging: 'Messaging', http_api: 'External APIs', other: 'Other systems' },
    actHttp: 'HTTP clients', actUser: 'User', actMain: 'Launch', actTask: 'Scheduler', actPublic: 'Public access', actOthers: 'Other roles',
    epsN: (n) => n + (n === 1 ? ' endpoint' : ' endpoints'), callsN: (n) => n + (n === 1 ? ' call' : ' calls'),
    readsWrites: 'reads & writes', calls: 'calls',
    onlyChanged: 'changed only', branch: 'branch', uncommitted: 'uncommitted', changesTip: 'branch changes: added / modified / deleted / uncommitted',
    st_added: 'added', st_modified: 'modified', st_deleted: 'deleted',
    grpMods: (n) => n + (n === 1 ? ' module' : ' modules'), grpHint: 'module group — double-click expands', collapse: 'collapse', group: 'group', root: '(root)',
    fnShort: 'fn', modShort: 'mod', lines: 'lines', entryLbl: 'entry point', fnsFiles: (f, fl) => 'functions: ' + f + ', files: ' + fl,
    flowSearch: 'Search scenarios…', grpTop: 'Main', grpGeneral: 'General', grpFlows: 'Scenarios', flowMore: (n) => '…' + n + ' more — refine the search', flowNone: 'No scenarios found',
    optChanges: 'Changed functions: path from entries', optOverview: 'Overview: calls from entry points', fnFlowTitle: 'Scenario from ', playFromFn: '▶ Scenario from this function', play: '▶ Play',
    noPlay: 'Nothing to play', queue: 'queue', viaQueue: 'via queue', next: 'next', end: 'end',
    stepsN: (n) => n + (n === 1 ? ' step' : ' steps'),
    live: 'live', rebuilding: 'rebuilding…', error: 'error', noConn: 'disconnected',
    noData: 'No data: build this file with archmap map.',
  },
};
const LANGS = ['ru', 'en'];
function detectLang() {
  let l = qs.get('lang');
  if (!LANGS.includes(l)) { try { l = localStorage.getItem('archmap.lang'); } catch (e) { l = null; } }
  if (!LANGS.includes(l)) l = /^ru/i.test(navigator.language || '') ? 'ru' : 'en';
  return l;
}
let LANG = detectLang();
const plural = (n) => (forms) => {
  if (LANG !== 'ru') return n === 1 ? forms[0] : forms[forms.length - 1];
  const a = Math.abs(n) % 100, b = a % 10;
  return forms[a > 10 && a < 20 ? 2 : b === 1 ? 0 : b >= 2 && b <= 4 ? 1 : 2];
};
// t(key, ...args): string, object or function result from the current language (fallback en)
function t(key, ...a) {
  let v = I18N[LANG][key]; if (v === undefined) v = I18N.en[key];
  if (v === undefined) return key;
  if (typeof v !== 'function') return v;
  return a.length === 1 && typeof a[0] === 'number' ? v(a[0], plural(a[0])) : v(...a);
}
// L(x): picks the current language from {ru, en}; other values as is
const L = (o) => (o && typeof o === 'object' && ('ru' in o || 'en' in o) ? (o[LANG] != null ? o[LANG] : o.en != null ? o.en : o.ru) : o);
// addI18n({ru:{…}, en:{…}}): views add their own strings (keys are global — prefix them with the view id)
function addI18n(dict) { for (const l in dict) I18N[l] = Object.assign(I18N[l] || {}, dict[l]); }

/* ---------- event bus ---------- */
// Events: 'data' (after reindex), 'lang', 'theme', 'view' (id), 'level' (n), 'select' (sel),
// 'step' (step|null), 'flow' (run), 'resize'.
const bus = {
  h: {},
  on(e, f) { (this.h[e] = this.h[e] || []).push(f); return () => { this.h[e] = this.h[e].filter((x) => x !== f); }; },
  emit(e, ...a) { (this.h[e] || []).slice().forEach((f) => { try { f(...a); } catch (err) { console.error('archmap:', e, err); } }); },
};

/* ---------- data index ---------- */
const LAYERS = ['entry', 'ui', 'domain', 'adapter', 'infra', 'other', 'test'];
const LCOLOR = { entry: 'var(--in)', ui: 'var(--ui)', domain: 'var(--dom)', adapter: 'var(--ad)', infra: 'var(--infra)', other: 'var(--oth)', test: 'var(--tst)', ext: 'var(--ext)', actor: 'var(--text-2)' };
const D = {};
// indexData(data): (re)builds every index from {facts, model, serve}. Views read D.* only.
function indexData(data) {
  const F = (data && data.facts) || {}, M = (data && data.model) || {};
  D.raw = data; D.F = F; D.M = M; D.live = (data && data.serve) || null;
  D.fns = F.functions || {}; D.fids = Object.keys(D.fns);
  D.mods = F.modules || {}; D.MM = M.modules || {};
  D.callers = {}; D.byMod = {};
  D.fids.forEach((id) => { D.callers[id] = []; });
  D.fids.forEach((id) => {
    const f = D.fns[id];
    (D.byMod[f.module] = D.byMod[f.module] || []).push(id);
    (f.callees || []).forEach((c) => { if (D.callers[c]) D.callers[c].push(id); });
    (f.async_callees || []).forEach((c) => { if (D.callers[c] && !D.callers[c].includes(id)) D.callers[c].push(id); });
  });
  D.changed = D.fids.filter((id) => D.fns[id].status);
  D.deleted = F.deleted || [];
  D.entries = D.fids.filter((id) => D.fns[id].entry);
  D.modChg = {};
  const bump = (m, k) => { const o = D.modChg[m] = D.modChg[m] || { added: 0, modified: 0, deleted: 0, local: 0 }; o[k]++; };
  D.changed.forEach((id) => { const f = D.fns[id]; bump(f.module, f.status); if (f.uncommitted) bump(f.module, 'local'); });
  D.deleted.forEach((d) => bump(d.module, 'deleted'));
  D.nAdded = D.changed.filter((id) => D.fns[id].status === 'added').length;
  D.nModified = D.changed.filter((id) => D.fns[id].status === 'modified').length;
  D.nLocal = D.changed.filter((id) => D.fns[id].uncommitted).length;
  D.importedBy = {};
  Object.keys(D.mods).forEach((a) => (D.mods[a].imports || []).forEach((b) => { if (a !== b) (D.importedBy[b] = D.importedBy[b] || []).push(a); }));
  // modules shown on the map: with functions, files or changes
  D.allMods = Object.keys(D.mods).filter((m) => (D.byMod[m] || []).length || ((D.mods[m].files || []).length) || D.modChg[m]).sort();
  // composition roots: an entry module importing most of the others; its edges are hidden
  const nm = D.allMods.length;
  D.roots = new Set(D.allMods.filter((m) => layerOf(m) === 'entry' && (D.mods[m].imports || []).length >= Math.max(8, nm * 0.35)));
  // externals
  D.externals = (F.externals || []).map((x, i) => Object.assign({ id: 'ext:' + x.name, i }, x));
  D.extByMod = {}; D.extByFn = {};
  D.externals.forEach((x) => {
    (x.modules || []).forEach((m) => (D.extByMod[m] = D.extByMod[m] || []).push(x));
    (x.functions || []).forEach((f) => (D.extByFn[f] = D.extByFn[f] || []).push(x));
  });
  // endpoints, states, data
  D.endpoints = F.endpoints || [];
  D.epByFid = {};
  D.endpoints.forEach((e) => { if (e.fid) (D.epByFid[e.fid] = D.epByFid[e.fid] || []).push(e); });
  D.states = F.states || {};
  D.transByFid = {};
  Object.keys(D.states).forEach((ent) => (D.states[ent].transitions || []).forEach((tr) => (D.transByFid[tr.fid] = D.transByFid[tr.fid] || []).push(Object.assign({ entity: ent }, tr))));
  D.tables = (F.data && F.data.tables) || {};
  // flows: only those with at least one known step
  D.flows = (M.flows || []).map((fl, i) => Object.assign({ i }, fl)).filter((fl) => (fl.steps || []).some((s) => D.fns[s]));
  D.flowById = {}; D.flows.forEach((fl) => { if (fl.id) D.flowById[fl.id] = fl; });
  D.actors = computeActors();
  bus.emit('data', D);
}

/* ---------- facts helpers ---------- */
const fname = (f) => (f.cls ? f.cls + '.' : '') + f.name;
function fnInfo(id) {
  const f = D.fns[id];
  if (f) {
    const out = (f.callees || []).filter((c) => D.fns[c]).concat((f.async_callees || []).filter((c) => D.fns[c] && !(f.callees || []).includes(c)));
    return { id, name: fname(f), short: f.name, mod: f.module, file: f.file, s: f.start, e: f.end, sig: f.sig, doc: cleanDoc(f.doc), rawDoc: f.doc, lang: f.lang, out, in: D.callers[id] || [], status: f.status, uncommitted: !!f.uncommitted, entry: f.entry, data: f.data || [], full: true };
  }
  const nm = String(id).split('::').pop();
  return { id, name: nm, short: nm.split('.').pop(), mod: '', out: [], in: [], data: [], full: false };
}
const isAsync = (from, to) => !!D.fns[from] && (D.fns[from].async_callees || []).includes(to) && !(D.fns[from].callees || []).includes(to);
const isCall = (from, to) => !!D.fns[from] && (D.fns[from].callees || []).includes(to);
const modLabel = (m) => (m === '.' || m === '' ? t('root') : m);
function short(m) {
  if (m === '.' || m === '') return t('root');
  const s = m.replace(/^internal\//, '').replace(/^src\//, '').replace(/^adapters\//, '').replace(/^app\//, '');
  return s || m;
}
function layerOf(m) {
  const i = D.MM && D.MM[m];
  return i && LAYERS.includes(i.layer) ? i.layer : 'other';
}
function modRole(m) {
  const i = D.MM[m];
  if (i && i.role) return i.role;
  return short(m).split('/').pop();
}
const modDesc = (m) => { const i = D.MM[m]; return (i && i.desc) || ((D.mods[m] || {}).doc) || ''; };
const chgOf = (mods) => {
  const o = { added: 0, modified: 0, deleted: 0, local: 0 };
  mods.forEach((m) => { const c = D.modChg[m]; if (c) for (const k in o) o[k] += c[k]; });
  return o;
};
const hasChg = (c) => !!(c && (c.added || c.modified || c.deleted));
const chgClass = (c) => (!c ? '' : c.modified ? 'chg-m' : c.added ? 'chg-a' : c.deleted ? 'chg-d' : '');
const chgText = (c) => { const p = []; if (c.added) p.push('+' + c.added); if (c.modified) p.push('~' + c.modified); if (c.deleted) p.push('−' + c.deleted); return p.join(' '); };
const fnChgClass = (id) => { const f = D.fns[id]; return f && f.status === 'added' ? 'chg-a' : f && f.status === 'modified' ? 'chg-m' : ''; };

// role of an endpoint by its auth: 'role:a,b' → 'a'; 'public'/'' → 'public'
function epRole(e) {
  const a = (e.auth || '').trim();
  if (!a || a === 'public' || a === 'none') return 'public';
  return a.replace(/^role:/, '').split(/[,|\s]+/)[0] || 'public';
}
const humanize = (s) => String(s).replace(/[_-]+/g, ' ').replace(/^\w/, (c) => c.toUpperCase());

// Actors of the system: callers grouped by endpoint role, or by entry kind when there are no endpoints.
// [{id:'actor:…', name, sub, eps:[endpoint], fids:[entry fids], mods:Set}]
function computeActors() {
  const acts = new Map();
  const get = (key, name) => {
    if (!acts.has(key)) acts.set(key, { id: 'actor:' + key, key, name, eps: [], fids: [], mods: new Set(), protos: new Set(), services: new Set() });
    return acts.get(key);
  };
  const hasQueue = D.externals.some((x) => x.kind === 'queue');
  D.endpoints.forEach((e) => {
    if (e.kind === 'task' || e.kind === 'cli') return;
    const r = epRole(e);
    const a = get(r, r === 'public' ? null : humanize(r));
    a.eps.push(e); if (e.fid) { a.fids.push(e.fid); if (D.fns[e.fid]) a.mods.add(D.fns[e.fid].module); }
    a.protos.add(e.kind === 'grpc' ? 'gRPC' : e.kind === 'ws' ? 'WebSocket' : 'HTTP');
    if (e.service) a.services.add(e.service);
  });
  if (!acts.size) {
    D.entries.forEach((id) => {
      const k = D.fns[id].entry.kind;
      if (k === 'task' && hasQueue) return;
      const key = { http: 'http', ui: 'user', lifecycle: 'user', main: 'main', task: 'task' }[k] || k;
      const a = get(key, null);
      a.fids.push(id); a.mods.add(D.fns[id].module);
    });
  } else if (!hasQueue) {
    D.endpoints.filter((e) => e.kind === 'task').forEach((e) => { const a = get('task', null); a.eps.push(e); if (e.fid && D.fns[e.fid]) { a.fids.push(e.fid); a.mods.add(D.fns[e.fid].module); } });
  }
  // a declared flow names the actor of its first step
  D.flows.forEach((fl) => {
    if (!fl.actor || !fl.steps || !fl.steps.length) return;
    for (const a of acts.values()) if (a.fids.includes(fl.steps[0]) && !a.named) { a.name = fl.actor; a.named = true; }
  });
  let list = Array.from(acts.values()).sort((a, b) => (b.eps.length + b.fids.length) - (a.eps.length + a.fids.length));
  if (list.length > 6) {
    const rest = list.slice(5), o = { id: 'actor:others', key: 'others', name: null, eps: [], fids: [], mods: new Set(), protos: new Set(), services: new Set() };
    rest.forEach((a) => { o.eps.push(...a.eps); o.fids.push(...a.fids); a.mods.forEach((m) => o.mods.add(m)); a.protos.forEach((p) => o.protos.add(p)); });
    list = list.slice(0, 5).concat([o]);
  }
  return list;
}
function actorName(a) {
  if (a.name) return a.name;
  return t({ public: 'actPublic', http: 'actHttp', user: 'actUser', main: 'actMain', task: 'actTask', others: 'actOthers' }[a.key] || 'actUser');
}
function actorSub(a) {
  if (a.eps.length) return t('epsN', a.eps.length) + ' · ' + Array.from(a.protos).join(', ');
  return t('fns') + ': ' + a.fids.length;
}
const actorOfFn = (fid) => D.actors.find((a) => a.fids.includes(fid)) || null;
// external systems a function talks to: declared callers of the external, or code inside its modules
function extsOfFn(fid) {
  const f = D.fns[fid]; if (!f) return [];
  const out = (D.extByFn[fid] || []).slice();
  (D.extByMod[f.module] || []).forEach((x) => { if (!out.includes(x)) out.push(x); });
  return out;
}

/* ---------- source code: highlighting and loading ---------- */
const KW = {
  go: 'func return if else for range switch case default var const type struct interface map chan go defer select break continue fallthrough package import nil true false',
  python: 'def return if elif else for while in not and or is None True False class import from as with try except finally raise yield lambda pass break continue async await global nonlocal del assert self',
  js: 'function return if else for while do switch case default var let const class extends new this super import export from as async await try catch finally throw typeof instanceof in of null undefined true false yield delete void interface type enum implements public private protected readonly static',
  rust: 'fn let mut pub impl struct enum trait match if else for while loop return use mod crate self Self super as where move ref async await dyn const static true false None Some Ok Err',
  c: 'class struct interface enum public private protected internal static final abstract override virtual void return if else for foreach while do switch case default new this super base import using package namespace try catch finally throw throws null true false var val let fun def func let async await suspend object companion when is in as extends implements record sealed data inline const readonly',
};
const KWS = {}; for (const k in KW) KWS[k] = new Set(KW[k].split(' '));
const TY = new Set('string int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 float32 float64 error bool byte rune any context uuid time str float dict list tuple set bytes number boolean object void char long double short Integer String Boolean'.split(' '));
const kwFor = (lang) => KWS[{ go: 'go', python: 'python', typescript: 'js', javascript: 'js', tsx: 'js', jsx: 'js', rust: 'rust' }[lang] || (lang === 'ruby' || lang === 'elixir' ? 'python' : 'c')];
const hashCom = (lang) => /^(python|ruby|elixir|bash|sh|shell|perl|r|yaml|toml|make|nim|crystal)$/.test(lang || '');
const dashCom = (lang) => /^(haskell|sql|lua|elm|ada)$/.test(lang || '');
// hlLine(line, lang): HTML of one source line with token spans
function hlLine(line, lang) {
  const kw = kwFor(lang);
  const com = hashCom(lang) ? '#.*$' : dashCom(lang) ? '--.*$' : '\\/\\/.*$|\\/\\*.*?(?:\\*\\/|$)';
  const re = new RegExp('(' + com + ')|("(?:[^"\\\\]|\\\\.)*"?|\'(?:[^\'\\\\]|\\\\.)*\'?|`[^`]*`?)|(\\b\\d+(?:\\.\\d+)?\\b)|([A-Za-z_$][\\w$]*)(\\s*\\()?', 'g');
  let out = '', last = 0, m;
  while ((m = re.exec(line))) {
    if (m[0] === '') { re.lastIndex++; continue; }
    out += esc(line.slice(last, m.index));
    if (m[1]) out += `<span class="tk-com">${esc(m[1])}</span>`;
    else if (m[2]) out += `<span class="tk-str">${esc(m[2])}</span>`;
    else if (m[3]) out += `<span class="tk-num">${m[3]}</span>`;
    else {
      const w = m[4];
      if (kw.has(w)) out += `<span class="tk-kw">${w}</span>`;
      else if (TY.has(w)) out += `<span class="tk-ty">${w}</span>`;
      else if (m[5]) out += `<span class="tk-fn">${esc(w)}</span>`;
      else out += esc(w);
      if (m[5]) out += esc(m[5]);
    }
    last = re.lastIndex;
  }
  return out + esc(line.slice(last));
}
const SRC = {}, SRC_MISS = {}; // fid -> body loaded from the serve /src endpoint / fids it could not give
function srcOf(fid) { const f = D.fns[fid]; return (f && f.src) || SRC[fid] || null; }
const canLoadSrc = (fid) => !!(D.live && D.live.src && D.fns[fid] && !SRC_MISS[fid]);
// loadSrc(fid): Promise<string|null>; fetches only in serve mode (D.live.src), never for a static file
function loadSrc(fid) {
  const s = srcOf(fid);
  if (s != null) return Promise.resolve(s);
  if (!canLoadSrc(fid)) return Promise.resolve(null);
  return fetch(D.live.src + '?fid=' + encodeURIComponent(fid), { cache: 'no-store' })
    .then((r) => (r.ok ? r.text() : null)).catch(() => null)
    .then((b) => { if (b != null) SRC[fid] = b; else SRC_MISS[fid] = true; return b; });
}
// line index to highlight in fid's body: the call of `target` (a fid) — first line after the header
function callLine(lines, target) {
  if (!target) return -1;
  const nm = fnInfo(target).short;
  if (!nm) return -1;
  const re = new RegExp('(^|[^\\w$])' + nm.replace(/[$]/g, '\\$') + '\\s*[(<\\[{]');
  for (let i = 1; i < lines.length; i++) if (re.test(lines[i])) return i;
  const re2 = new RegExp('(^|[^\\w$])' + nm.replace(/[$]/g, '\\$') + '([^\\w$]|$)');
  for (let i = 1; i < lines.length; i++) if (re2.test(lines[i])) return i;
  return -1;
}
// bridges(a, b): callees of a that call (or enqueue) b
const bridges = (a, b) => fnInfo(a).out.filter((c) => c !== b && fnInfo(c).out.includes(b));
// doc text without arch: annotations (they are shown structurally)
const cleanDoc = (s) => { const d = String(s || ''), i = d.search(/\s*\barch:[a-z]/); return (i > 0 ? d.slice(0, i) : i === 0 ? '' : d).trim(); };
// codeHTML(fid, target): {html, hlIdx, has} — body lines with numbers; highlighted call of target
function codeHTML(fid, target) {
  const f = fnInfo(fid), body = srcOf(fid);
  if (body == null) {
    const loading = canLoadSrc(fid);
    return { html: (f.sig ? `<div class="ln"><span class="no">${f.s || ''}</span><span>${hlLine(f.sig, f.lang)}</span></div>` : '') + `<div class="nosrc">${esc(loading ? t('loadingSrc') : t('noBody'))}</div>`, hlIdx: -1, has: false, loading: !!loading };
  }
  const lines = body.replace(/\t/g, '    ').split('\n');
  let hlIdx = callLine(lines, target);
  // the target is reached through one of fid's callees (e.g. an enqueue call): highlight that call
  if (hlIdx < 0 && target) for (const c of bridges(fid, target)) { hlIdx = callLine(lines, c); if (hlIdx >= 0) break; }
  const start = f.s || 1;
  return { html: lines.map((l, i) => `<div class="ln${i === hlIdx ? ' hl' : ''}"><span class="no">${start + i}</span><span>${hlLine(l, f.lang) || ' '}</span></div>`).join(''), hlIdx, has: true };
}

/* ---------- state ---------- */
const state = {
  view: 'map', level: 0, mod: null, fn: null, sel: null, speed: 1, only: false,
  busy: false, fnAll: false,
  // playback (playback.js): run = {key, name, segs:[{flow, steps}]}, step index, branch choice
  run: null, step: -1, playing: false, played: false, exitText: null,
};

/* ---------- view and panel registry ---------- */
// registerView({id, title, order?, mount(el, ctx), show?(ctx), hide?(ctx), onStep?(step, ctx), onSelect?(sel, ctx), onLang?(ctx), onData?(ctx)})
//   id     — unique id, also the element id suffix: <div class="view" id="v-<id>">
//   title  — string, {ru, en} or () => string; shown as the header tab
//   order  — tab position (map = 0; views 10, 20, …)
//   mount  — called once, lazily, the first time the tab opens; el is the empty view container
//   show/hide — called on every tab switch to/from the view
//   onStep — playback moved (step object from AM.play.current(), or null on reset); called for every view, visible or not, after mount
//   onSelect — selection changed (sel object, see panel.js), after mount
//   onLang / onData — language switched / data replaced (serve); re-render here
const VIEWS = [];
function registerView(v) {
  if (!v || !v.id || typeof v.mount !== 'function') throw new Error('registerView: id and mount(el, ctx) are required');
  if (VIEWS.some((x) => x.id === v.id)) throw new Error('registerView: duplicate id ' + v.id);
  VIEWS.push(v);
  VIEWS.sort((a, b) => (a.order == null ? 100 : a.order) - (b.order == null ? 100 : b.order));
}
const viewTitle = (v) => (typeof v.title === 'function' ? v.title() : L(v.title) || v.id);
// registerPanel(kind, render(sel, ctx) => html string | {html, after(paneEl)}): right card for a selection kind
const PANELS = {};
function registerPanel(kind, fn) { PANELS[kind] = fn; }

/* ---------- public API for views ---------- */
// Filled further by scene.js, layout.js, map.js, playback.js, panel.js, main.js.
const AM = window.AM = {
  version: 3, D, state, bus, t, L, addI18n, plural, registerView, registerPanel,
  h: H, svg: S_, esc, cut, cutL, fnInfo, fname, short, modLabel, layerOf, modRole, modDesc, LAYERS, LCOLOR,
  isAsync, isCall, extsOfFn, actorName, epRole, hlLine, codeHTML, loadSrc, srcOf,
  get lang() { return LANG; }, get RM() { return RM; }, get DUR() { return DUR; },
};
