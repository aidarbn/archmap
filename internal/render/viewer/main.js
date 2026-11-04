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
  state.view = id;
  $$('#stage > .view').forEach((x) => x.classList.toggle('on', x.id === 'v-' + id));
  renderTabs();
  if (v.show) v.show(AM);
  bus.emit('view', id);
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
  $('#themeBtn').title = t('theme');
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
  renderHeader(); renderTabs(); rebuildMap(); renderPlayer(); renderPanel();
  bus.emit('lang', l); forViews('onLang');
}

/* ---------- data swap (archmap serve) ---------- */
// setData(d): replace facts/model, keep level, selection and the run when they still exist
function setData(d) {
  const key = state.run && state.run.key, fid = curStep() && curStep().fid;
  indexData(d);
  if (state.fn && !D.fns[state.fn]) state.fn = null;
  if (state.mod && !D.byMod[state.mod]) state.mod = null;
  renderHeader(); rebuildMap();
  const run = key && buildRun(key);
  if (run) { state.run = run; const i = fid ? runList().findIndex((x) => x.fid === fid) : -1; state.step = i; } else loadRun(defaultRunKey());
  renderPlayer(); renderPanel();
  forViews('onData');
}
function setupLive() {
  const box = $('#live'), label = box.querySelector('b'), info = $('#liveInfo');
  const status = (st, key, tip) => { box.hidden = false; box.className = 'live' + (st === 'ready' ? '' : ' ' + st); label.textContent = t(key); box.title = tip || ''; };
  let seq = D.live.seq || 0, want = seq, busy = false;
  const pull = async () => {
    if (busy) return; busy = true;
    try {
      while (seq !== want) {
        const r = await fetch(D.live.data, { cache: 'no-store' });
        if (!r.ok) throw new Error('HTTP ' + r.status);
        const d = await r.json();
        seq = d.seq; if (seq > want) want = seq;
        d.serve = Object.assign({}, D.live, { seq });
        setData(d);
        info.textContent = ' · ' + new Date().toLocaleTimeString();
      }
      status('ready', 'live');
    } catch (e) { status('error', 'error', String(e)); } finally { busy = false; }
  };
  status('ready', 'live');
  const es = new EventSource(D.live.events);
  es.onmessage = (ev) => {
    const u = JSON.parse(ev.data);
    if (u.state === 'building') { status('building', 'rebuilding'); return; }
    if (u.state === 'error') { status('error', 'error', u.error || ''); return; }
    if (u.seq !== seq) { want = u.seq; pull(); } else status('ready', 'live');
  };
  es.onerror = () => status('error', 'noConn');
}

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
  setupPicker();
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
  if (D.live) setupLive();
}

Object.assign(AM, { setView, setData, setTheme, setLang, viewEl, views: VIEWS });
init();
