/* side.js — the scenario list: a narrow column at the left of the map (#side), hidden by its chevron or `[`
 * (kept in localStorage archmap.side). On top "Through paths" (journeys, in the model order), under them the
 * visible scenarios (no flow.hidden) grouped by the actor that starts them (folded by default, a counter in the
 * header). A small filter narrows this list only (by name and description) when it is long. A click opens the
 * path / scenario on the map as before (only its participants, steps by hand); a click on the selected row, ✕
 * of the bar or Esc brings the whole map back. While a path is open its scenarios unfold under it with the
 * current one marked; a click goes to the first step of that scenario. The selection is kept in the hash:
 * #journey=<id> or #flow=<id>. */

addI18n({
  ru: {
    sdPaths: 'Сквозные пути', sdFilter: 'Фильтр', sdNone: 'Ничего не найдено', sdHide: 'Скрыть список ([)', sdShow: 'Показать сценарии ([)',
    sdLabel: 'Сценарии', sdOpenTip: 'Открыть на карте', sdCloseTip: 'Закрыть: вся карта',
  },
  en: {
    sdPaths: 'Through paths', sdFilter: 'Filter', sdNone: 'Nothing found', sdHide: 'Hide the list ([)', sdShow: 'Show the scenarios ([)',
    sdLabel: 'Scenarios', sdOpenTip: 'Open on the map', sdCloseTip: 'Close: the whole map',
  },
});

const SIDE = { hidden: false, q: '', open: new Set() }; // open: actor groups unfolded by hand
const SIDE_FILTER_MIN = 15; // the filter shows once the list has more scenarios than this

/* ---------- texts ---------- */
// a description written for people, not the one the model builds from the name ("Actor: Name -> exit",
// "http: path through …"): those say nothing the row does not already say
function realDesc(x) {
  const d = String((x && x.desc) || '').trim(), nm = String((x && x.name) || '').trim();
  if (!d || d === nm) return '';
  if (x.actor && d.startsWith(x.actor + ': ' + nm)) return '';
  if (/^[\w-]+: path through /.test(d)) return '';
  return d;
}
// an automatic path is named "<first flow> → … (N flows)": the tail is the counter under the name
const pathName = (j) => String(j.name || j.id).replace(/\s*→\s*….*$/, '');
const flowLine2 = (fl) => realDesc(fl) || (fl.exit ? '→ ' + fl.exit : '');
const pathLine = (j) => realDesc(j) || (j.exit ? '→ ' + j.exit : '');
const pathSteps = (j) => j.fls.reduce((n, f) => n + flowSteps(f).length, 0);

/* ---------- groups ---------- */
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

/* ---------- keys and the hash ---------- */
const flowKey = (fl) => 'flow:' + fl.i;
const flowHashId = (fl) => fl.id || String(fl.i);
function runHash() {
  const k = state.run && state.run.key; if (!k) return '';
  if (k.startsWith('journey:')) return '#journey=' + encodeURIComponent(k.slice(8));
  if (k.startsWith('flow:')) { const fl = D.flows.find((x) => x.i === +k.slice(5)); return fl ? '#flow=' + encodeURIComponent(flowHashId(fl)) : ''; }
  return '';
}
function writeHash() {
  const want = runHash();
  if ((location.hash || '') === want) return;
  try { history.replaceState(null, '', want || location.pathname + location.search); } catch (e) { /* file:// in some browsers */ }
}
// the run the hash names: 'journey:<id>' or 'flow:<i>'; null when it names nothing known
function hashRun() {
  const m = /^#(journey|flow)=(.+)$/.exec(location.hash || '');
  if (!m) return null;
  const id = decodeURIComponent(m[2]);
  if (m[1] === 'journey') return jvJourney(id) ? 'journey:' + id : null;
  const fl = D.flows.find((x) => x.id === id) || (/^\d+$/.test(id) ? D.flows.find((x) => x.i === +id) : null);
  return fl ? flowKey(fl) : null;
}

/* ---------- render ---------- */
const sideHay = (...a) => lc(a.filter(Boolean).join(' '));
function sideRow(key, name, line, meta, cls, title) {
  const cur = state.run && state.run.key === key;
  return `<button type="button" class="si ${cls}${cur ? ' cur' : ''}" data-run="${esc(key)}" aria-pressed="${cur}" title="${esc(title || name)}">` +
    `<span class="nm">${esc(name)}</span>${line ? `<span class="ds">${esc(line)}</span>` : ''}${meta ? `<span class="mt">${esc(meta)}</span>` : ''}</button>`;
}
// the scenarios of the open path, under it: the current one marked, a click goes to its first step
function pathKids() {
  const r = state.run; if (!r || !r.key.startsWith('journey:')) return '';
  const L_ = runList(), it = L_[Math.max(0, state.step)], cur = state.step >= 0 && it ? it.seg : -1;
  let start = 0;
  return '<div class="sk">' + r.segs.map((sg, si) => {
    const at = start; start += sg.steps.length;
    const nm = sg.flow ? sg.flow.name : r.name;
    return `<button type="button" class="sk-i${si === cur ? ' cur' : ''}" data-go="${at}" title="${esc(nm + (sg.flow && realDesc(sg.flow) ? '\n' + realDesc(sg.flow) : ''))}"><i>${si + 1}</i><span>${esc(nm)}</span></button>`;
  }).join('') + '</div>';
}
function renderSide() {
  const box = $('#sideList'); if (!box || !D.flows) return;
  const top = box.scrollTop, qq = lc(SIDE.q).trim();
  const curKey = state.run && state.run.key;
  let h = '';
  const js = jvJourneys().filter((j) => !qq || sideHay(j.name, realDesc(j), j.exit, j.fls.map((f) => f.name).join(' ')).includes(qq));
  if (js.length) {
    h += `<div class="sh">${esc(t('sdPaths'))}</div>`;
    js.forEach((j) => {
      const nm = pathName(j);
      h += sideRow('journey:' + j.id, nm, pathLine(j), t('jvFlowsN', j.fls.length) + ' · ' + t('pkStepsN', pathSteps(j)), 'j', j.name + (realDesc(j) ? '\n' + realDesc(j) : ''));
      if (curKey === 'journey:' + j.id) h += pathKids();
    });
  }
  const gs = sideGroups();
  gs.forEach((g) => {
    const fls = qq ? g.fls.filter((fl) => sideHay(fl.name, realDesc(fl), fl.exit).includes(qq)) : g.fls;
    if (!fls.length) return;
    const hasCur = fls.some((fl) => flowKey(fl) === curKey);
    const open = !!qq || SIDE.open.has(g.id) || hasCur;
    h += `<button type="button" class="sg${open ? ' on' : ''}" data-grp="${esc(g.id)}" aria-expanded="${open}"><span class="ar">${open ? '▾' : '▸'}</span><span class="nm">${esc(g.name)}</span><span class="n">${fls.length}</span></button>`;
    if (open) h += fls.map((fl) => sideRow(flowKey(fl), fl.name, flowLine2(fl), '', 'f', fl.name + (realDesc(fl) ? '\n' + realDesc(fl) : '') + (fl.exit ? '\n→ ' + fl.exit : ''))).join('');
  });
  if (!h) h = `<div class="sn">${esc(t('sdNone'))}</div>`;
  box.innerHTML = h;
  box.scrollTop = top;
  const c = box.querySelector('.si.cur');
  if (c && SIDE.reveal) { SIDE.reveal = false; c.scrollIntoView({ block: 'nearest' }); }
}
function paintSide() {
  const m = $('main'); m.classList.toggle('noside', SIDE.hidden);
  const b = $('#sideTog'), tip = t(SIDE.hidden ? 'sdShow' : 'sdHide');
  b.title = tip; b.setAttribute('aria-label', tip); b.setAttribute('aria-expanded', String(!SIDE.hidden));
  b.textContent = SIDE.hidden ? '›' : '‹';
  $('#side').toggleAttribute('inert', SIDE.hidden);
  const q = $('#sideQ'); q.placeholder = t('sdFilter'); q.setAttribute('aria-label', t('sdFilter'));
  $('#side').setAttribute('aria-label', t('sdLabel'));
  q.parentElement.hidden = visibleFlows().length + jvJourneys().length <= SIDE_FILTER_MIN;
}
function setSideHidden(h) {
  SIDE.hidden = !!h;
  try { localStorage.setItem('archmap.side', h ? '0' : '1'); } catch (e) { /* file:// without storage */ }
  paintSide();
  // the stage changed its width: the scenes take the new size
  rebuildMap(); bus.emit('resize');
}
function pickRun(key) {
  closeMenu();
  if (state.run && state.run.key === key && state.view === 'map') { closeRun(); return; }
  SIDE.reveal = true;
  openRun(key);
}
function setupSide() {
  let v = null; try { v = localStorage.getItem('archmap.side'); } catch (e) { /* ignore */ }
  SIDE.hidden = v === '0';
  const box = $('#sideList');
  box.addEventListener('click', (e) => {
    const g = e.target.closest('[data-grp]');
    if (g) { const id = g.getAttribute('data-grp'); if (SIDE.open.has(id)) SIDE.open.delete(id); else SIDE.open.add(id); renderSide(); return; }
    const k = e.target.closest('[data-go]');
    if (k) { AM.play.go(+k.getAttribute('data-go')); return; }
    const r = e.target.closest('[data-run]');
    if (r) pickRun(r.getAttribute('data-run'));
  });
  $('#sideQ').addEventListener('input', (e) => { SIDE.q = e.target.value; renderSide(); });
  $('#sideQ').addEventListener('keydown', (e) => { if (e.key === 'Escape') { e.stopPropagation(); if (e.target.value) { e.target.value = ''; SIDE.q = ''; renderSide(); } else e.target.blur(); } });
  $('#sideTog').onclick = () => setSideHidden(!SIDE.hidden);
  window.addEventListener('keydown', (e) => {
    const tg = e.target.tagName;
    if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT' || e.target.isContentEditable || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === '[') { e.preventDefault(); setSideHidden(!SIDE.hidden); }
  });
  bus.on('flow', () => { renderSide(); writeHash(); });
  bus.on('step', renderSide);
  bus.on('lang', () => { paintSide(); renderSide(); });
  bus.on('data', () => { paintSide(); renderSide(); });
  window.addEventListener('hashchange', () => {
    const k = hashRun();
    if (k) { if (!state.run || state.run.key !== k) openRun(k); } else if (state.run && !location.hash) closeRun();
  });
  paintSide(); renderSide();
}

Object.assign(AM, { side: { render: renderSide, setHidden: setSideHidden, get hidden() { return SIDE.hidden; }, groups: sideGroups, realDesc } });
