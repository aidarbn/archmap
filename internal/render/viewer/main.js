/* main.js — start-up: header, tabs from the view registry, theme, language, keyboard, data swap. */

window.__errs = window.__errs || [];
addEventListener('error', (e) => window.__errs.push(String(e.message)));
addEventListener('unhandledrejection', (e) => window.__errs.push('rej: ' + (e.reason && e.reason.stack || e.reason)));

/* ---------- views ---------- */
function viewEl(id) {
  let el = $('#v-' + id);
  if (!el) el = H('div', { class: 'view', id: 'v-' + id }, $('#stage'));
  return el;
}
function setView(id) {
  const v = VIEWS.find((x) => x.id === id); if (!v) return;
  const prev = VIEWS.find((x) => x.id === state.view);
  const el = viewEl(id);
  if (!v._mounted) { v._mounted = true; try { v.mount(el, AM); } catch (err) { console.error('archmap view', id, err); } }
  if (prev && prev !== v && prev._mounted && prev.hide) prev.hide(AM);
  const changed = state.view !== id;
  state.view = id;
  // one place decides what is shown: the active view only; everything else is inert and hidden
  $$('#stage > .view').forEach((x) => { const on = x.id === 'v-' + id; x.classList.toggle('on', on); x.toggleAttribute('inert', !on); x.setAttribute('aria-hidden', on ? 'false' : 'true'); });
  if (changed) { closeSearch(); if (LS.open) toggleLists(false); }
  renderTabs();
  if (v.show) v.show(AM);
  bus.emit('view', id);
  if (DEBUG) setTimeout(() => auditPanels('view:' + id), DUR + 120);
}
function renderTabs() {
  const tb = $('#tabs'); tb.innerHTML = '';
  if (VIEWS.length < 2) { tb.style.display = 'none'; return; }
  tb.style.display = '';
  VIEWS.forEach((v) => { const b = H('button', { role: 'tab', 'aria-selected': state.view === v.id }, tb, esc(viewTitle(v))); b.onclick = () => setView(v.id); });
}
const forViews = (hook, ...a) => VIEWS.forEach((v) => { if (v._mounted && v[hook]) { try { v[hook](...a, AM); } catch (err) { console.error('archmap view', v.id, hook, err); } } });
bus.on('step', (s) => forViews('onStep', s));
bus.on('select', (s) => forViews('onSelect', s));

/* ---------- header ---------- */
function renderHeader() {
  const F = D.F, PJ = F.project || {};
  const name = PJ.name || D.M.title || F.repo || 'archmap';
  $('#repoName').textContent = name;
  $('#repoName').title = PJ.description || D.M.summary || '';
  $('#repoMeta').textContent = [F.repo && F.repo !== name ? F.repo : '', F.branch ? t('branch') + ' ' + F.branch + (F.base ? ' vs ' + F.base : '') : '', F.head].filter(Boolean).join(' · ');
  document.title = name + (LANG === 'ru' ? ' — архитектура' : ' — architecture');
  const c = $('#chg'); c.innerHTML = '';
  if (D.changed.length || D.deleted.length) {
    c.title = t('changesTip');
    c.innerHTML = `<span class="c a">+${D.nAdded}</span><span class="c m">~${D.nModified}</span><span class="c d">−${D.deleted.length}</span>` +
      (D.nLocal ? `<span class="c l" title="${esc(t('uncommitted'))}">●${D.nLocal}</span>` : '') +
      `<button class="chgbtn" id="onlyBtn" aria-pressed="${state.only}">${esc(t('onlyChanged'))}</button>`;
    $('#onlyBtn').onclick = () => { state.only = !state.only; $('#onlyBtn').setAttribute('aria-pressed', state.only); paintMap(); if (fv && state.level === 2) { buildFns(fv.mod); } bus.emit('filter', state.only); };
  }
  $('#themeBtn').title = t('theme'); paintAutoZoom(); paintFlowsBtn();
  $('#zIn').title = t('zIn'); $('#zOut').title = t('zOut'); $('#zFit').title = t('zFit');
  $$('#langSeg button').forEach((b) => b.setAttribute('aria-pressed', b.dataset.l === LANG));
  document.documentElement.lang = LANG;
}

/* ---------- theme and language ---------- */
function setTheme(th) {
  document.documentElement.setAttribute('data-theme', th);
  try { localStorage.setItem('archmap.theme', th); } catch (e) { /* file:// without storage */ }
  bus.emit('theme', th);
}
function setLang(l) {
  if (!LANGS.includes(l) || l === LANG) return;
  LANG = l;
  try { localStorage.setItem('archmap.lang', l); } catch (e) { /* ignore */ }
  if (state.run && !state.run.key.startsWith('flow:')) state.run.name = (buildRun(state.run.key) || state.run).name;
  D.actors = computeActors();
  renderHeader(); renderTabs(); rebuildMap(); renderPlayer(); renderPanel(); relangNav(); relangLive();
  bus.emit('lang', l); forViews('onLang');
}

/* ---------- data swap (archmap serve) ---------- */
// the flow in the new data that corresponds to an old one: by id, else by name
const sameFlow = (a) => D.flows.find((x) => (a.id && x.id === a.id) || x.name === a.name);
// setData(d, flashIds): replace facts/model; keep the view, level, selection, camera, opened groups,
// the filter, the scenario and the playback position; then flash what changed.
function setData(d, flashIds) {
  const ids = flashIds || [];
  const old = state.run, oldFid = curStep() && curStep().fid, oldStep = state.step, oldFlows = old ? old.segs.map((sg) => sg.flow) : [];
  const exitText = state.exitText, pane = $('#pane'), paneTop = pane.scrollTop;
  ids.forEach((id) => { delete SRC[id]; });
  Object.keys(SRC_MISS).forEach((k) => delete SRC_MISS[k]);
  indexData(d);
  if (state.fn && !D.fns[state.fn]) state.fn = null;
  if (state.mod && !D.byMod[state.mod]) state.mod = null;
  const s = state.sel;
  if (s && ((s.t === 'fn' && !D.fns[s.id]) || (s.t === 'mod' && !D.mods[s.id]))) state.sel = null;
  if (state.only && !D.changed.length && !D.deleted.length) state.only = false;
  // the scenario: same flow (ids and indexes may shift), the step of the same function
  let run = null;
  if (old) {
    const f0 = oldFlows[0], nf = f0 && sameFlow(f0);
    run = buildRun(f0 ? (nf ? 'flow:' + nf.i : null) : old.key);
    if (run && f0) oldFlows.slice(1).forEach((fl, i) => {
      const nx = fl && sameFlow(fl); if (!nx) return;
      run.segs.push({ flow: nx, steps: nx.steps.filter((x) => D.fns[x]), via: old.segs[i + 1].via });
    });
  }
  state.run = run; state.playing = false;
  if (run) {
    const list = runList();
    let i = oldStep >= 0 && list[oldStep] && list[oldStep].fid === oldFid ? oldStep : oldFid ? list.findIndex((x) => x.fid === oldFid) : -1;
    if (oldStep >= 0 && i < 0) i = Math.min(oldStep, list.length - 1);
    state.step = i; state.exitText = i >= 0 ? exitText : null; state.played = state.played && i >= 0;
  }
  renderHeader();
  rebuildMap(true);
  if (!run) loadRun(defaultRunKey());
  renderPlayer(); renderPanel(); pane.scrollTop = paneTop;
  if (LS.open) drawLists();
  forViews('onData');
  startFlash(ids);
  bus.emit('live', { flash: ids });
}
function setupLive() {
  const box = $('#live'), label = box.querySelector('b'), info = $('#liveInfo');
  let cur = null, extra = '';
  const paint = () => {
    if (!cur) return;
    const [st, key, tip] = cur;
    box.hidden = false; box.className = 'live' + (st === 'ready' ? '' : ' ' + st);
    label.textContent = t(key); box.title = tip || ''; info.textContent = st === 'ready' ? extra : '';
  };
  const status = (st, key, tip) => { cur = [st, key, tip]; paint(); };
  relangLive = paint;
  let seq = D.live.seq || 0, want = seq, busy = false, ms = 0;
  const pend = new Set();
  const pull = async () => {
    if (busy) return; busy = true;
    let tip = '';
    try {
      while (seq !== want) {
        const r = await fetch(D.live.data, { cache: 'no-store' });
        if (!r.ok) throw new Error('HTTP ' + r.status);
        const d = await r.json();
        seq = d.seq; if (seq > want) want = seq;
        const ids = Array.from(pend); pend.clear();
        for (let i = 0; i < 40 && state.busy; i++) await sleep(100); // not in the middle of a transition
        d.serve = Object.assign({}, D.live, { seq });
        setData(d, ids);
        extra = ' · ' + new Date().toLocaleTimeString() + (ms ? ' · ' + ms + ' ' + t('ms') : '') + (ids.length ? ' · ' + t('changedN') + ' ' + ids.length : '');
        tip = ids.slice(0, 12).map((id) => fnInfo(id).name).join('\n');
      }
      status('ready', 'live', tip);
    } catch (e) { status('error', 'error', String(e)); } finally { busy = false; }
  };
  status('ready', 'live');
  const es = new EventSource(D.live.events);
  es.onmessage = (ev) => {
    let u; try { u = JSON.parse(ev.data); } catch (e) { return; }
    if (u.state === 'building') { status('building', 'rebuilding'); return; }
    if (u.state === 'error') { status('error', 'error', u.error || ''); info.textContent = ' · ' + (u.error || '').slice(0, 80); return; }
    (u.flash || []).forEach((id) => pend.add(id));
    ms = u.ms || 0;
    if (u.seq !== seq) { want = u.seq; pull(); } else status('ready', 'live');
  };
  es.onerror = () => { status('error', 'noConn', t('noConnTip')); };
}
let relangLive = () => {};

/* ---------- start ---------- */
function init() {
  let th = null; try { th = localStorage.getItem('archmap.theme'); } catch (e) { /* ignore */ }
  if (qs.get('theme')) th = qs.get('theme');
  setTheme(th === 'dark' || th === 'light' ? th : matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  if (!DATA || !DATA.facts) { $('#pane').innerHTML = `<p class="lead">${esc(t('noData'))}</p>`; return; }
  indexData(DATA);
  renderHeader();
  $('#themeBtn').onclick = () => setTheme(document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark');
  $$('#langSeg button').forEach((b) => { b.onclick = () => setLang(b.dataset.l); });
  $('#btnUp').onclick = () => up(); $('#btnIn').onclick = () => intoSelected();
  $('#zIn').onclick = () => curCam() && curCam().zoomAt(.5, .5, 0.8);
  $('#zOut').onclick = () => curCam() && curCam().zoomAt(.5, .5, 1.25);
  $('#zFit').onclick = fitCur;
  $('#pStep').onclick = () => doStep(1);
  $('#pPrev').onclick = () => doStep(-1);
  $('#pIn').onclick = () => stepIn();
  $('#pOut').onclick = () => stepOut();
  $('#pAuto').onclick = () => auto();
  $('#pReset').onclick = () => resetRun();
  $('#pSpeed').onchange = (e) => { state.speed = +e.target.value; };
  setupPicker(); setupSearch(); setupLists(); setupAutoZoom();
  window.addEventListener('keydown', (e) => {
    const tg = e.target.tagName;
    if (tg === 'SELECT' || tg === 'INPUT' || tg === 'TEXTAREA' || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'Escape') { if (PK.open) return closePick(true); e.preventDefault(); if (state.view === 'map') up(); }
    else if (e.key === 'Enter') { if (tg === 'BUTTON') return; e.preventDefault(); if (state.view === 'map') (state.step >= 0 ? stepIn() : intoSelected()); }
    else if (e.key === 'ArrowRight' || (e.key === ' ' && tg !== 'BUTTON')) { e.preventDefault(); doStep(1); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); doStep(-1); }
    else if (e.key === 'Home') { e.preventDefault(); resetRun(); }
  });
  let rt; window.addEventListener('resize', () => { clearTimeout(rt); rt = setTimeout(() => { rebuildMap(); bus.emit('resize'); }, 150); });
  rebuildMap();
  loadRun(defaultRunKey());
  setView('map');
  renderPanel();
  if (D.live) { setupLive(); watchMarks(); }
}

Object.assign(AM, { setView, setData, setTheme, setLang, viewEl, views: VIEWS });
init();
