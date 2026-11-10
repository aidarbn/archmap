/* rail.js — the steps rail: while a scenario is open, ALL its steps stand as a static list at the left of the canvas
 * (number, what happens, what it touches). Reading needs no animation; stepping only lights the current row
 * (`.cur`), the passed ones are calm (`.done`), the future ones pale. A row click goes to its step.
 * The cameras keep the rail's strip free (`setDock(…, left)` → `Cam.insetLeft`). */

addI18n({
  ru: { rlTitle: 'Шаги', rlHide: 'Свернуть шаги', rlShow: 'Шаги', rlClient: 'вызов клиента', rlCalls: 'Обращается к', rlFromQueue: 'Выполняется из очереди фоновых задач', rlStatus: 'Статус', rlInCode: 'в коде:', rlPlan: 'запланировано', rlStart: 'Начать →', rlN: (n) => n + ' шагов' },
  en: { rlTitle: 'Steps', rlHide: 'Hide the steps', rlShow: 'Steps', rlClient: 'client call', rlCalls: 'Calls', rlFromQueue: 'Runs from the background job queue', rlStatus: 'Status', rlInCode: 'in the code:', rlPlan: 'planned', rlStart: 'Start →', rlN: (n) => n + ' steps' },
});
const RAIL = { w: 312, closed: false, mounted: false };
try { RAIL.closed = localStorage.getItem('archmap.rail') === '0'; } catch (e) { /* private mode */ }

// The text of a step in human words, from the annotations in the code (`arch:step <flow> "…"` — the caption), else the
// doc comment of the function (its first sentence), else the function name. A call of a function is not shown as the
// text: it is not obvious to a reader; it appears under the current step only, as "in the code".
function railTitle(s) {
  const lab = s.flow && s.flow.labels && s.flow.labels[s.fid];
  if (lab) return { text: lab, src: 'arch' };
  const d = s.planned ? '' : fnDesc(s.fid);
  if (d) return { text: d.split(/(?<=[.!?])\s/)[0], src: 'doc' };
  return { text: s.label || '', src: 'name' };
}
const RAIL_OPS = {
  ru: { insert: 'Создаёт запись', update: 'Меняет запись', upsert: 'Создаёт или меняет запись', delete: 'Удаляет запись', read: 'Читает' },
  en: { insert: 'Creates a row', update: 'Changes a row', upsert: 'Creates or changes a row', delete: 'Deletes a row', read: 'Reads' },
};
// what the step does to data, outside systems and statuses — a phrase each, nothing about functions
function railFacts(s) {
  if (!s || s.planned) return [];
  const out = [], ops = RAIL_OPS[LANG === 'en' ? 'en' : 'ru'], seen = new Set();
  (s.data || []).forEach((a) => {
    const raw = String(a.op || '').toLowerCase(), op = isReadOp(raw) ? 'read' : ops[raw] ? raw : 'update', k = op + ' ' + a.table;
    if (seen.has(k)) return; seen.add(k);
    out.push({ k: 'db', t: ops[op] + ' · ' + a.table + (op === 'update' && a.fields && a.fields.length ? ' (' + a.fields.slice(0, 3).join(', ') + ')' : '') });
  });
  (s.exts || []).forEach((x) => { if (x.kind !== 'db' && x.kind !== 'queue') out.push({ k: 'ext', t: t('rlCalls') + ' ' + x.name }); });
  if (s.kind === 'async') out.push({ k: 'q', t: t('rlFromQueue') });
  (s.states || []).forEach((st) => out.push({ k: 'st', t: t('rlStatus') + ' ' + st.entity.replace(/Status$/, '') + ': ' + (st.from ? st.from + ' → ' : '→ ') + st.to }));
  return out;
}
const RAIL_MARK = { db: '▦', ext: '↗', st: '◉', q: '⇢' };

function railRow(i, s, cur) {
  const tt = railTitle(s), mod = s.mod ? short(s.mod) : '', md = s.mod ? cut(modDesc(s.mod), 120) : '';
  const where = s.actor && i === 0 ? actorName(s.actor) + ' → ' + mod : mod;
  const tag = s.kind === 'client' ? t('rlClient') : s.planned ? t('rlPlan') : '';
  const facts = railFacts(s).map((f) => `<li class="rl-f ${f.k}"><b>${RAIL_MARK[f.k]}</b><span>${esc(cut(f.t, 90))}</span></li>`).join('');
  const code = s.planned ? '' : stepAction(s).short;
  const cls = 'rl-s' + (i === cur ? ' cur' : cur >= 0 && i < cur ? ' done' : '');
  return `<li class="${cls}" data-i="${i}" role="button" tabindex="0"><span class="rl-n">${i + 1}</span><div class="rl-b"><div class="rl-t${tt.src === 'name' ? ' raw' : ''}">${esc(cut(tt.text, 130))}</div>${where || tag ? `<div class="rl-w" title="${esc(md)}">${esc(where)}${tag ? ` · <i>${esc(tag)}</i>` : ''}</div>` : ''}${facts ? `<ul class="rl-fs">${facts}</ul>` : ''}${code ? `<div class="rl-code"><small>${esc(t('rlInCode'))}</small> ${esc(code)}</div>` : ''}</div></li>`;
}

function railEl() {
  let el = $('#rail');
  if (el) return el;
  el = H('aside', { class: 'rail', id: 'rail', 'aria-label': t('rlTitle') }, $('#stage'), '');
  el.addEventListener('click', (e) => {
    if (e.target.closest('[data-rail-hide]')) { railSetClosed(true); return; }
    const row = e.target.closest('[data-i]'); if (row) AM.play.go(+row.getAttribute('data-i'));
  });
  el.addEventListener('keydown', (e) => { if (e.key !== 'Enter') return; const row = e.target.closest('[data-i]'); if (row) AM.play.go(+row.getAttribute('data-i')); });
  return el;
}
function railTab() {
  let b = $('#railTab');
  if (!b) { b = H('button', { class: 'rail-tab', id: 'railTab' }, $('#stage'), ''); b.addEventListener('click', () => railSetClosed(false)); }
  return b;
}
function railSetClosed(c) {
  RAIL.closed = c;
  try { localStorage.setItem('archmap.rail', c ? '0' : '1'); } catch (e) { /* private mode */ }
  renderRail(true);
}
// the strip the cameras keep free: the rail's width plus its margin; 0 when it is hidden
const railInset = () => (state.run && state.view === 'map' && !RAIL.closed ? RAIL.w + 16 : 0);

function renderRail(reframe) {
  const el = railEl(), tab = railTab(), r = state.run, on = !!r && state.view === 'map';
  el.hidden = !on || RAIL.closed; tab.hidden = !on || !RAIL.closed;
  tab.textContent = t('rlShow') + ' ›';
  el.style.width = RAIL.w + 'px';
  if (on && !RAIL.closed) {
    const list = runList(), cur = state.step;
    let h = `<header><b>${esc(t('rlTitle'))}</b><span>${esc(t('rlN', list.length))}</span><button data-rail-hide title="${esc(t('rlHide'))}" aria-label="${esc(t('rlHide'))}">‹</button></header><ol>`, at = 0;
    r.segs.forEach((sg) => {
      if (r.segs.length > 1 && sg.flow) h += `<li class="rl-ch">${esc(cut(sg.flow.name || '', 60))}</li>`;
      sg.steps.forEach(() => { const s = stepAt(at); if (s) h += railRow(at, s, cur); at++; });
    });
    el.innerHTML = h + '</ol>';
    const c = el.querySelector('.rl-s.cur');
    if (c) { const ol = el.querySelector('ol'), top = c.offsetTop - ol.offsetTop, vis = ol.clientHeight; if (top < ol.scrollTop + 8 || top + c.offsetHeight > ol.scrollTop + vis - 8) ol.scrollTo({ top: Math.max(0, top - vis / 3), behavior: RM ? 'auto' : 'smooth' }); }
  }
  const w = railInset();
  document.documentElement.style.setProperty('--rail-w', w ? w + 'px' : '0px');
  if (w !== DOCK.l) setDock(DOCK.b, DOCK.r, reframe ? 220 : 0, w);
}
// The rail already says where the scenario is, so the navigation column gives its width to the diagram while a run
// is open (about 260 px of a canvas that would otherwise be ~550 px wide); it comes back when the run closes.
// Only an auto-hide is undone, and it is not remembered: a reload keeps what the user chose.
function railSide() {
  if (!AM.side || typeof SIDE === 'undefined') return;
  if (state.run && !SIDE.hidden && !RAIL.sideAuto) {
    RAIL.sideAuto = true; AM.side.setHidden(true);
    try { localStorage.setItem('archmap.side', '1'); } catch (e) { /* private mode */ }
  } else if (!state.run && RAIL.sideAuto) {
    RAIL.sideAuto = false; AM.side.setHidden(false);
  }
}
bus.on('flow', () => { railSide(); renderRail(); });
bus.on('step', () => renderRail());
bus.on('view', () => renderRail());
bus.on('lang', () => renderRail());
bus.on('data', () => renderRail());
bus.on('resize', () => renderRail());
