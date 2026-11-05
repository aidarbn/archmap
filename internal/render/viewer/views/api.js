/* views/api.js — the API tab: every endpoint of facts.endpoints, grouped by service, with filters,
 * search, and the endpoint card in the right panel (selection kind 'ep').
 *
 * CONTRACT with the Data view (views/data.js):
 *   The card lists the tables the endpoint touches (closure of calls, depth 6, Function.data).
 *   A click on a table first switches the tab (AM.setView('data') — a not yet mounted view is mounted
 *   synchronously there, so its subscriptions exist) and then emits on the bus:
 *       AM.bus.emit('api:open-table', { table, op, ep, fid, via })
 *     table — table name, a key of AM.D.tables / Function.data[].table
 *     op    — 'R' | 'W' | 'RW' (read / any write: insert, update, delete, upsert)
 *     ep    — the endpoint object from AM.D.endpoints
 *     fid   — the endpoint function id (may be '')
 *     via   — fid of the first function in the closure that touches the table
 *   The Data view should subscribe with AM.bus.on('api:open-table', ({table}) => …) and select the table.
 *   The same payload is kept in AM.apiTarget (last click) for a subscriber that mounts later.
 *
 * Selection: AM.select({t:'ep', id:<index in AM.D.endpoints>}). */
{
  const D = AM.D, S = AM.state, esc = AM.esc, t = AM.t;
  const DEPTH = 6;
  const ROW_H = 36, GRP_H = 38;

  AM.addI18n({
    ru: {
      apiTitle: 'API', apiSearch: 'Поиск по пути, имени, сервису…', apiAllKinds: 'Все', apiAccess: 'Доступ', apiAllAccess: 'любой доступ',
      apiPublic: 'публичная', apiAuthed: 'любой вход', apiRole: (r) => 'роль ' + r, apiChanged: 'только изменённые',
      apiTotal: (n) => n + ' ручек', apiShown: (a, b) => a + ' из ' + b, apiNone: 'Ручки не найдены', apiNoEps: 'В коде не найдено ручек (HTTP, gRPC, задач).',
      apiOwnChg: 'функция ручки изменена на ветке', apiClosureChg: 'в замыкании вызовов есть изменённые функции',
      apiReqResp: 'Запрос → ответ', apiFn: 'Функция', apiScenario: 'Сценарий', apiPlay: '▶ Проиграть', apiPlayFn: '▶ Сценарий от функции',
      apiNoScenario: 'Сценарий не размечен', apiTables: 'Какие данные трогают', apiTablesHint: (n, d) => n + ' функций в замыкании вызовов, глубина ' + d,
      apiNoTables: 'Таблицы не найдены в замыкании вызовов', apiChgInClosure: (n) => 'изменено в замыкании: ' + n, apiToData: 'открыть в виде «Данные»',
      apiUnknownFn: 'функция не найдена в фактах', apiKind: { http: 'HTTP', grpc: 'gRPC', ws: 'WebSocket', task: 'Задача', cli: 'CLI' },
      apiSource: 'источник', apiCollapse: 'свернуть группу', apiExpand: 'раскрыть группу', apiNoSvc: 'без сервиса',
    },
    en: {
      apiTitle: 'API', apiSearch: 'Search path, name, service…', apiAllKinds: 'All', apiAccess: 'Access', apiAllAccess: 'any access',
      apiPublic: 'public', apiAuthed: 'any login', apiRole: (r) => 'role ' + r, apiChanged: 'changed only',
      apiTotal: (n) => n + (n === 1 ? ' endpoint' : ' endpoints'), apiShown: (a, b) => a + ' of ' + b, apiNone: 'No endpoints found', apiNoEps: 'No endpoints (HTTP, gRPC, tasks) found in the code.',
      apiOwnChg: 'the endpoint function is changed on the branch', apiClosureChg: 'the call closure contains changed functions',
      apiReqResp: 'Request → response', apiFn: 'Function', apiScenario: 'Scenario', apiPlay: '▶ Play', apiPlayFn: '▶ Scenario from the function',
      apiNoScenario: 'No annotated scenario', apiTables: 'Data touched', apiTablesHint: (n, d) => n + ' functions in the call closure, depth ' + d,
      apiNoTables: 'No tables found in the call closure', apiChgInClosure: (n) => 'changed in the closure: ' + n, apiToData: 'open in the Data view',
      apiUnknownFn: 'function not found in facts', apiKind: { http: 'HTTP', grpc: 'gRPC', ws: 'WebSocket', task: 'Task', cli: 'CLI' },
      apiSource: 'source', apiCollapse: 'collapse group', apiExpand: 'expand group', apiNoSvc: 'no service',
    },
  });

  /* ---------- derived data (reset on every data swap) ---------- */
  let near = null;      // fid -> distance to the nearest changed function reachable from it (<= DEPTH)
  let clos = new Map(); // fid -> closure info
  const fnOf = (fid) => (fid ? D.fns[fid] : null);

  // Reverse BFS from every changed function over callers: one pass instead of a closure per endpoint.
  function nearChanged() {
    if (near) return near;
    near = new Map();
    let cur = D.changed.slice();
    cur.forEach((id) => near.set(id, 0));
    for (let d = 1; d <= DEPTH && cur.length; d++) {
      const nx = [];
      for (const id of cur) for (const c of (D.callers[id] || [])) if (!near.has(c)) { near.set(c, d); nx.push(c); }
      cur = nx;
    }
    return near;
  }
  // Forward closure of fid (callees + async_callees), depth DEPTH: count, changed count, tables with R/W.
  function closure(fid) {
    let r = clos.get(fid);
    if (r) return r;
    const seen = new Set([fid]), tbl = new Map();
    let cur = [fid], chg = 0;
    for (let d = 0; d <= DEPTH && cur.length; d++) {
      const nx = [];
      for (const id of cur) {
        const f = D.fns[id]; if (!f) continue;
        if (f.status) chg++;
        for (const a of (f.data || [])) {
          let o = tbl.get(a.table);
          if (!o) tbl.set(a.table, o = { table: a.table, r: false, w: false, via: id });
          if (a.op === 'read') o.r = true; else o.w = true;
        }
        if (d === DEPTH) continue;
        for (const c of (f.callees || [])) if (!seen.has(c) && D.fns[c]) { seen.add(c); nx.push(c); }
        for (const c of (f.async_callees || [])) if (!seen.has(c) && D.fns[c]) { seen.add(c); nx.push(c); }
      }
      cur = nx;
    }
    const tables = Array.from(tbl.values()).sort((a, b) => (b.w - a.w) || a.table.localeCompare(b.table));
    clos.set(fid, r = { n: seen.size, chg, tables });
    return r;
  }
  const opOf = (o) => (o.r && o.w ? 'RW' : o.w ? 'W' : 'R');

  /* ---------- endpoint helpers ---------- */
  const EPS = () => D.endpoints;
  const verbOf = (e) => e.method || (e.kind || '').toUpperCase();
  const accKind = (e) => { const a = (e.auth || '').trim(); return !a || a === 'public' || a === 'none' ? 'public' : a === 'authenticated' ? 'authed' : 'role'; };
  const accLabel = (e) => { const k = accKind(e); return k === 'public' ? t('apiPublic') : k === 'authed' ? t('apiAuthed') : t('apiRole', AM.epRole(e)); };
  const epName = (e) => e.name || e.path || '?';
  const svcDesc = (s) => { const x = s && (D.F.services || {})[s]; return x && x.desc ? AM.plainDoc(x.desc) : ''; };
  // 'own' — the endpoint function itself changed; 'deep' — something in its call closure; '' — nothing
  function chgOf(e) {
    const f = fnOf(e.fid); if (!f) return '';
    if (f.status) return 'own';
    return nearChanged().has(e.fid) ? 'deep' : '';
  }
  const verbBadge = (e) => `<span class="api-verb v-${esc((e.method || e.kind || '').toLowerCase())}">${esc(verbOf(e))}</span>`;
  const accBadge = (e) => `<span class="api-acc a-${accKind(e)}" title="${esc(e.auth || '')}">${esc(accLabel(e))}</span>`;
  const chgMark = (c) => (c === 'own' ? `<span class="api-chg own" title="${esc(t('apiOwnChg'))}">~</span>` : c === 'deep' ? `<span class="api-chg deep" title="${esc(t('apiClosureChg'))}">≈</span>` : '');
  const groupOf = (e) => e.service || t('apiKind')[e.kind] || e.kind || t('apiNoSvc');

  /* ---------- view state ---------- */
  const V = { el: null, q: '', kind: '', access: '', only: false, collapsed: new Set(), items: [], total: 0, shown: 0, sel: -1, rows: [], timer: 0 };
  let ui = {};

  function filtered() {
    const q = V.q.trim().toLowerCase(), out = [];
    EPS().forEach((e, i) => {
      if (V.kind && e.kind !== V.kind) return;
      if (V.access) { const k = accKind(e); if (V.access === 'public' || V.access === 'authed') { if (k !== V.access) return; } else if (k !== 'role' || AM.epRole(e) !== V.access.slice(5)) return; }
      if (V.only && !chgOf(e)) return;
      if (q && !((e.path || '') + ' ' + (e.name || '') + ' ' + (e.service || '') + ' ' + (e.method || '') + ' ' + (e.desc || '')).toLowerCase().includes(q)) return;
      out.push(i);
    });
    return out;
  }
  // flat list of rows: group headers and endpoints; collapsed groups keep only the header
  function build() {
    const idx = filtered(), groups = new Map();
    idx.forEach((i) => { const g = groupOf(EPS()[i]); if (!groups.has(g)) groups.set(g, []); groups.get(g).push(i); });
    const names = Array.from(groups.keys()).sort((a, b) => a.localeCompare(b));
    const rows = []; let y = 0;
    for (const g of names) {
      const list = groups.get(g), open = !V.collapsed.has(g);
      const chg = list.filter((i) => chgOf(EPS()[i])).length;
      rows.push({ g, n: list.length, chg, open, y, h: GRP_H }); y += GRP_H;
      if (open) for (const i of list) { rows.push({ i, y, h: ROW_H }); y += ROW_H; }
    }
    V.rows = rows; V.height = y; V.shown = idx.length;
  }
  function rowHTML(r) {
    if (r.g != null) {
      return `<div class="api-grp" style="top:${r.y}px;height:${r.h}px" data-g="${esc(r.g)}" role="button" tabindex="-1" aria-expanded="${r.open}" title="${esc(t(r.open ? 'apiCollapse' : 'apiExpand'))}">` +
        `<span class="api-caret">${r.open ? '▾' : '▸'}</span><b>${esc(r.g)}</b>${svcDesc(r.g) ? `<span class="api-gd" title="${esc(svcDesc(r.g))}">${esc(svcDesc(r.g))}</span>` : ''}${r.chg ? `<span class="api-chg own" title="${esc(t('apiChanged'))}">~${r.chg}</span>` : ''}<span class="api-cnt">${r.n}</span></div>`;
    }
    const e = EPS()[r.i];
    const hasPath = e.path && e.path !== e.name;
    // the description, when the code has one, takes the place of the path (the path stays in the tooltip)
    const pth = e.desc ? `<span class="api-pth ds">${esc(e.desc)}</span>` : hasPath ? `<span class="api-pth">${esc(e.path)}</span>` : '<span class="api-pth"></span>';
    return `<div class="api-row${r.i === V.sel ? ' sel' : ''}" style="top:${r.y}px;height:${r.h}px" data-i="${r.i}" role="option" aria-selected="${r.i === V.sel}" title="${esc([e.path, e.desc].filter(Boolean).join('\n'))}">` +
      `${verbBadge(e)}<span class="api-nm">${esc(epName(e))}</span>${pth}` +
      `${chgMark(chgOf(e))}${accBadge(e)}</div>`;
  }
  // Virtualisation: only rows inside the viewport (+ buffer) exist in the DOM.
  function paint() {
    const box = ui.list; if (!box) return;
    const top = box.scrollTop, h = box.clientHeight || 600, rows = V.rows;
    let lo = 0, hi = rows.length;
    while (lo < hi) { const m = (lo + hi) >> 1; if (rows[m].y + rows[m].h < top - 200) lo = m + 1; else hi = m; }
    let html = '';
    for (let k = lo; k < rows.length && rows[k].y < top + h + 200; k++) html += rowHTML(rows[k]);
    ui.spacer.innerHTML = html;
  }
  function refresh(keepScroll) {
    build();
    ui.spacer.style.height = V.height + 'px';
    if (!keepScroll) ui.list.scrollTop = 0;
    paint();
    ui.count.textContent = V.shown === V.total ? t('apiTotal', V.total) : t('apiShown', V.shown, V.total);
    ui.empty.hidden = V.shown > 0;
    ui.empty.textContent = V.total ? t('apiNone') : t('apiNoEps');
  }

  /* ---------- toolbar ---------- */
  // the minimal view: no filters of its own — an endpoint is found with ⌘K, "only changed" is a ⌘K command too
  // (the global filter event below); the bar keeps the count only
  function toolbar() {
    const bar = ui.bar; bar.innerHTML = '';
    ui.count = AM.h('span', { class: 'api-total' }, bar);
  }

  /* ---------- selection ---------- */
  function pick(i) {
    V.sel = i;
    AM.select({ t: 'ep', id: i });
  }
  function scrollTo(i) {
    const r = V.rows.find((x) => x.i === i); if (!r) return;
    const b = ui.list;
    if (r.y < b.scrollTop || r.y + r.h > b.scrollTop + b.clientHeight) b.scrollTop = Math.max(0, r.y - b.clientHeight / 3);
  }
  function move(d) {
    const eps = V.rows.filter((r) => r.i != null); if (!eps.length) return;
    let k = eps.findIndex((r) => r.i === V.sel); k = Math.max(0, Math.min(eps.length - 1, k < 0 ? (d > 0 ? 0 : eps.length - 1) : k + d));
    pick(eps[k].i); scrollTo(eps[k].i); paint();
  }

  /* ---------- the card (right panel) ---------- */
  const flowOf = (e) => (e.fid ? D.flows.find((fl) => fl.steps && fl.steps[0] === e.fid) : null);
  function card(sel) {
    const e = EPS()[sel.id];
    if (!e) return `<p class="lead">${esc(t('apiNone'))}</p>`;
    const f = fnOf(e.fid), c = chgOf(e);
    let h = `<div class="kicker"><span class="chip">${esc(e.service || t('apiKind')[e.kind] || e.kind)}</span>${accBadge(e)}` +
      `${c ? `<span class="chip ${c === 'own' ? 'm' : ''}">${esc(t(c === 'own' ? 'apiOwnChg' : 'apiClosureChg'))}</span>` : ''}</div>`;
    h += `<h2 class="mono">${esc(epName(e))}</h2>`;
    if (e.desc) h += `<p class="lead">${esc(e.desc)}</p>`;
    h += `<div class="api-pathline">${verbBadge(e)}<span class="mono">${esc(e.path || '')}</span></div>`;
    if (svcDesc(e.service)) h += `<p class="loc" style="font-family:inherit">${esc(e.service)}: ${esc(svcDesc(e.service))}</p>`;
    if (e.request || e.response) {
      h += `<h3>${esc(t('apiReqResp'))}</h3><div class="api-io"><div class="sig"><span class="tk-ty">${esc(e.request || '—')}</span></div><div class="sig"><span class="tk-ty">${esc(e.response || '—')}</span></div></div>`;
    }
    h += `<h3>${esc(t('apiFn'))}</h3>`;
    if (f) {
      const info = AM.fnInfo(e.fid);
      h += `<button class="api-fn" data-api-fn="1" title="${esc(e.fid)}"><span class="mono">${esc(info.name)}</span><small>${esc(AM.short(info.mod))}</small></button>`;
      if (info.file) h += `<div class="loc">${esc(info.file)}:${info.s}–${info.e}</div>`;
      if (e.source) h += `<div class="loc">${esc(t('apiSource'))}: ${esc(e.source)}</div>`;
    } else h += `<p class="loc">${esc(t('apiUnknownFn'))}</p>`;
    // scenario from this entry
    const fl = flowOf(e);
    if (f) {
      h += `<h3>${esc(t('apiScenario'))}</h3>`;
      if (fl) h += `<p style="margin:0 0 4px">${esc(fl.name || fl.id || '')}</p><div class="loc" style="margin-top:0">${esc(t('stepsN', (fl.steps || []).length))}</div>`;
      else h += `<p class="loc" style="margin-top:0">${esc(t('apiNoScenario'))}</p>`;
      if (fl || AM.fnInfo(e.fid).out.length) h += `<div class="btnrow"><button class="pbtn primary" data-api-play="1">${esc(t(fl ? 'apiPlay' : 'apiPlayFn'))}</button></div>`;
    }
    // tables of the call closure
    if (f) {
      const cl = closure(e.fid);
      h += `<h3>${esc(t('apiTables'))}</h3>`;
      h += `<div class="loc" style="margin:-4px 0 8px">${esc(t('apiTablesHint', cl.n, DEPTH))}${cl.chg ? ' · ' + esc(t('apiChgInClosure', cl.chg)) : ''}</div>`;
      if (cl.tables.length) {
        h += `<ul class="api-tbls">` + cl.tables.map((o, k) => `<li data-api-tbl="${k}" tabindex="0" role="button" title="${esc(t('apiToData'))}"><span class="api-op o-${opOf(o)}">${opOf(o)}</span><span class="mono">${esc(o.table)}</span></li>`).join('') + `</ul>`;
      } else h += `<p class="loc" style="margin-top:0">${esc(t('apiNoTables'))}</p>`;
    }
    return {
      html: h,
      after(p) {
        const fb = p.querySelector('[data-api-fn]'); if (fb) fb.onclick = () => AM.map.jumpToFn(e.fid, 2);
        const pb = p.querySelector('[data-api-play]');
        if (pb) pb.onclick = () => { AM.setView('map'); AM.play.playFrom(e.fid); };
        const cl = f ? closure(e.fid) : null;
        p.querySelectorAll('[data-api-tbl]').forEach((li) => {
          const go = () => {
            const o = cl.tables[+li.getAttribute('data-api-tbl')], payload = { table: o.table, op: opOf(o), ep: e, fid: e.fid || '', via: o.via };
            AM.apiTarget = payload;
            AM.setView('data');
            AM.bus.emit('api:open-table', payload);
          };
          li.onclick = go;
          li.onkeydown = (ev) => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); go(); } };
        });
      },
    };
  }
  AM.registerPanel('ep', card);

  /* ---------- the view ---------- */
  function resetDerived() { near = null; clos = new Map(); V.total = EPS().length; if (V.sel >= V.total) V.sel = -1; }
  function render() {
    resetDerived();
    toolbar();
    refresh(true);
  }

  AM.registerView({
    id: 'api',
    order: 20,
    title: () => t('apiTitle'),
    mount(el) {
      V.el = el;
      el.classList.add('api-view');
      ui.bar = AM.h('div', { class: 'api-bar' }, el);
      ui.list = AM.h('div', { class: 'api-list', tabindex: '0', role: 'listbox', 'aria-label': t('apiTitle') }, el);
      ui.spacer = AM.h('div', { class: 'api-spacer' }, ui.list);
      ui.empty = AM.h('div', { class: 'api-empty', hidden: '' }, el);
      let raf = 0;
      ui.list.addEventListener('scroll', () => { if (!raf) raf = requestAnimationFrame(() => { raf = 0; paint(); }); });
      ui.list.addEventListener('click', (ev) => {
        const g = ev.target.closest('[data-g]');
        if (g) { const n = g.getAttribute('data-g'); if (V.collapsed.has(n)) V.collapsed.delete(n); else V.collapsed.add(n); refresh(true); return; }
        const r = ev.target.closest('[data-i]');
        if (r) { pick(+r.getAttribute('data-i')); paint(); }
      });
      ui.list.addEventListener('keydown', (ev) => {
        if (ev.key === 'ArrowDown') { ev.preventDefault(); move(1); } else if (ev.key === 'ArrowUp') { ev.preventDefault(); move(-1); }
      });
      new ResizeObserver(() => { if (V.el && V.el.classList.contains('on')) paint(); }).observe(ui.list);
      V.only = !!S.only;
      render();
    },
    show() {
      paint();
      if (S.sel && S.sel.t === 'ep') { V.sel = S.sel.id; paint(); }
      else if (!S.sel || S.sel.t !== 'ep') AM.renderPanel();
    },
    onSelect(sel) {
      const i = sel && sel.t === 'ep' ? sel.id : -1;
      if (i !== V.sel) { V.sel = i; if (S.view === 'api') { if (i >= 0) scrollTo(i); paint(); } }
    },
    onLang() { render(); },
    onData() { render(); },
  });
  // Data -> API: 'data:open-endpoint' {ep, fid, table} — find the endpoint, scroll to it, select it, open the card.
  function openEndpoint(p) {
    if (!p || !V.el) return;
    const eps = EPS(), ep = p.ep || null;
    let i = ep ? eps.indexOf(ep) : -1;
    if (i < 0 && ep) i = eps.findIndex((x) => x.fid === ep.fid && x.path === ep.path && x.method === ep.method);
    if (i < 0 && ep) i = eps.findIndex((x) => x.fid === ep.fid && x.path === ep.path);
    if (i < 0) { const fid = p.fid || (ep && ep.fid); if (fid) i = eps.findIndex((x) => x.fid === fid); }
    if (i < 0) return;
    // a filter that hides the endpoint is dropped, a collapsed group is expanded
    if (!filtered().includes(i)) { V.q = ''; V.kind = ''; V.access = ''; V.only = false; toolbar(); }
    V.collapsed.delete(groupOf(eps[i]));
    refresh(true);
    pick(i); scrollTo(i); paint();
  }
  AM.bus.on('data:open-endpoint', openEndpoint);
  // the global "changed only" button of the header drives the list too
  AM.bus.on('filter', (on) => { if (!V.el) return; V.only = !!on; if (ui.onlyBtn) ui.onlyBtn.setAttribute('aria-pressed', V.only); refresh(true); });
  // data replaced (serve): derived caches are stale even before the view is mounted
  AM.bus.on('data', () => { near = null; clos = new Map(); });
}
