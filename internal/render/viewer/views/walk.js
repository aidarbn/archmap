/* views/walk.js — "Walk" mode: step through the code by hand. Start at a function; every outgoing arrow of the current
 * function (calls, queue hand-offs, external systems) is drawn on the map (level "functions of a module") and listed in
 * the right card; a click / → takes it, ← steps back. The path is an ordinary run (key "walk") of AM.state.run, so the
 * map camera, the sequence panel and the code card follow it. Works in a static HTML too; saving the path as a
 * scenario (add_flow + add_step ops through AM.spec) is offered only in the edit mode of `archmap serve`. */
AM.addI18n({
  ru: {
    wkBtn: 'Проход', wkBtnTip: 'Пошаговый проход по коду: стрелками выбираете, куда идти дальше', wkFrom: '▶ Пройти отсюда',
    wkStep: 'Шаг', wkNext: 'Куда дальше', wkNone: 'Исходящих вызовов нет — конец пути.', wkBack: '◀ Назад', wkExit: 'Выйти', wkKeys: '→ / Enter — идти · ↑ ↓ — выбрать · ← — назад · Esc — выйти',
    wkQueue: 'очередь →', wkSeen: 'уже был', wkGuide: 'по сценарию', wkMore: (n) => 'ещё ' + n + ' (хелперы)', wkLess: 'скрыть хелперы',
    wkTrace: 'След', wkCap: 'подпись шага', wkPlan: '+ запланированный шаг', wkPlanAsk: 'Что планируется на этом месте', wkAlt: '+ ветка', wkAltCond: 'условие', wkAltText: 'исход (текст)',
    wkSave: 'Сохранить как сценарий', wkSaveVar: 'Сохранить как вариант', wkId: 'id (латиницей)', wkName: 'название', wkActor: 'актор', wkExit: 'исход', wkOk: 'Сохранить', wkCancel: 'Отмена',
    wkCode: 'Код · вызов выбранной стрелки подсвечен', wkData: 'Данные', wkStatus: 'Статусы', wkNoStart: 'Не с чего начать: выберите функцию.', wkExtMark: 'внешняя система: отметить шаг',
    wkSaved: 'Сценарий сохранён', wkVarCond: 'условие варианта', wkAltN: (n) => 'веток: ' + n,
  },
  en: {
    wkBtn: 'Walk', wkBtnTip: 'Walk through the code step by step: pick the next arrow yourself', wkFrom: '▶ Walk from here',
    wkStep: 'Step', wkNext: 'Where next', wkNone: 'No outgoing calls — end of the path.', wkBack: '◀ Back', wkExit: 'Exit', wkKeys: '→ / Enter — go · ↑ ↓ — choose · ← — back · Esc — exit',
    wkQueue: 'queue →', wkSeen: 'been there', wkGuide: 'by scenario', wkMore: (n) => n + ' more (helpers)', wkLess: 'hide helpers',
    wkTrace: 'Trace', wkCap: 'step caption', wkPlan: '+ planned step', wkPlanAsk: 'What is planned here', wkAlt: '+ branch', wkAltCond: 'condition', wkAltText: 'outcome (text)',
    wkSave: 'Save as scenario', wkSaveVar: 'Save as variant', wkId: 'id (latin)', wkName: 'name', wkActor: 'actor', wkExit: 'exit', wkOk: 'Save', wkCancel: 'Cancel',
    wkCode: 'Code · the call of the chosen arrow is highlighted', wkData: 'Data', wkStatus: 'Statuses', wkNoStart: 'Nothing to start from: pick a function.', wkExtMark: 'external system: mark the step',
    wkSaved: 'Scenario saved', wkVarCond: 'variant condition', wkAltN: (n) => n + (n === 1 ? ' branch' : ' branches'),
  },
});

const T = (k, ...a) => AM.t(k, ...a);
const esc = AM.esc, D = AM.D, state = AM.state;
const W = { on: false, steps: [], kinds: [], exts: {}, caps: {}, plans: {}, alts: [], opts: [], sel: 0, helpers: false, guide: null, form: null, busy: false, seen: new Set() };

/* ---------- outgoing arrows ---------- */
// the same idea as autosem's isHelper: error constructors, mappers, loggers, getters
const HN = [
  /(error|err)s?$/i, /^(New|new|make|Make)\w*(Error|Err)$/, /^(Wrap|Wrapf|Errorf|Is|As|Unwrap)$/,
  /^(to|from|map|convert|parse|decode|encode|marshal|unmarshal|serialize|deserialize)([A-Z_0-9]|$)/,
  /(FromSqlc|ToSqlc|FromRow|ToRow|FromDB|ToDB|ToDomain|ToProto|ToPB|ToDTO|ToModel|ToResponse|ToEntity|FromProto|FromPB|FromDomain|FromModel|FromDTO)$/,
  /(logger|metric|telemetry|tracing)/i, /^(log|logf|debug|debugf|info|infof|warn|warnf|warning|trace|print|printf|println|observe|record|incr?|metric\w*|span\w*|track\w*)$/,
  /^(ptr|ptrTo|toPtr|deref|strPtr|stringPtr|boolPtr|atoi|itoa|sprintf)$/i, /Ptr$/, /(FromContext|FromCtx)$/, /^(format|fmt|stringify|normalize|sanitize)[A-Z_0-9]\w*$/, /^(t|tr|_|translate\w*|localize\w*|gettext)$/,
];
const HF = /(^|\/)(errors?|mapper\w*|\w+_mapper|\w*converters?|logger|log|metrics|i18n)(_test)?\.\w+$/i, HC = /(logger|logging|metrics?|tracer|span|counter|translator|i18n|localizer)/i, HM = /(^|\/)(log|logger|logging|metrics|telemetry|tracing|i18n|strings|conv|convert|ptr|errors?)(\/|$)/i;
function isHelper(fid) {
  const f = D.fns[fid]; if (!f) return false;
  return HF.test(f.file || '') || HN.some((re) => re.test(f.name || '')) || (f.cls && HC.test(f.cls)) || HM.test(f.module || '');
}
function optionsOf(fid) {
  const f = D.fns[fid]; if (!f) return [];
  const out = [], calls = (f.callees || []).filter((c) => D.fns[c]);
  calls.forEach((c) => out.push({ kind: 'call', fid: c, label: AM.fnInfo(c).name, helper: isHelper(c) }));
  (f.async_callees || []).filter((c) => D.fns[c] && !calls.includes(c)).forEach((c) => out.push({ kind: 'queue', fid: c, label: T('wkQueue') + ' ' + AM.fnInfo(c).name, helper: isHelper(c) }));
  AM.extsOfFn(fid).forEach((x) => out.push({ kind: 'ext', ext: x.name, label: '→ ' + x.name, helper: false }));
  return out;
}
const cur = () => W.steps[W.steps.length - 1];
const shown = () => W.opts.filter((o) => W.helpers || !o.helper);
// the scenario the walk started in: the next step of it is the "by scenario" arrow while the path follows it
function guided() {
  const g = W.guide; if (!g) return null;
  for (let k = 0; k < W.steps.length; k++) if (g.steps[g.start + k] !== W.steps[k]) return null;
  return g.steps[g.start + W.steps.length] || null;
}
function deviation() {
  const g = W.guide; if (!g) return -1;
  for (let k = 0; k < W.steps.length; k++) if (g.steps[g.start + k] !== W.steps[k]) return k;
  return -1;
}
function refreshOpts(keepSel) {
  W.opts = optionsOf(cur()).sort((a, b) => (a.kind === 'ext') - (b.kind === 'ext'));
  const gd = guided();
  W.opts.forEach((o) => { o.guided = !!gd && o.fid === gd; o.seen = !!o.fid && (W.seen.has(o.fid) || W.steps.includes(o.fid)); });
  const list = shown();
  let i = list.findIndex((o) => o.guided);
  if (i < 0) i = list.findIndex((o) => !o.seen && o.kind !== 'ext');
  if (i < 0) i = 0;
  W.sel = keepSel ? Math.min(W.sel, Math.max(0, list.length - 1)) : i;
}

/* ---------- start, step, back, exit ---------- */
async function start(fid, guide) {
  if (!fid || !D.fns[fid]) return;
  if (W.busy) return;
  Object.assign(W, { on: true, steps: [fid], kinds: ['entry'], exts: {}, caps: {}, plans: {}, alts: [], helpers: false, guide: guide || null, form: null, seen: new Set() });
  document.body.classList.add('walking');
  paintBtn();
  AM.play.load('fn:' + fid);
  const r = state.run; r.key = 'walk'; r.name = T('wkBtn') + ': ' + AM.fnInfo(fid).name; r.segs = [{ flow: null, steps: [fid] }];
  AM.bus.emit('flow', r);
  if (AM.setView && state.view !== 'map') AM.setView('map');
  if (state.level < 2) await AM.map.jumpToFn(fid, 2);
  W.busy = true;
  try { await AM.play.step(1); } finally { W.busy = false; }
  afterStep();
}
async function take(o) {
  if (!W.on || W.busy || !o) return;
  if (o.kind === 'ext') { const i = W.steps.length - 1; W.exts[i] = W.exts[i] === o.ext ? null : o.ext; renderCard(); drawOverlay(); return; }
  W.seen.add(cur());
  W.steps.push(o.fid); W.kinds.push(o.kind);
  state.run.segs[0].steps = W.steps.slice();
  W.busy = true;
  try { await AM.play.step(1); } finally { W.busy = false; }
  afterStep();
}
async function back() {
  if (!W.on || W.busy || W.steps.length < 2) return;
  const i = W.steps.length - 1;
  delete W.caps[i]; delete W.plans[i]; delete W.exts[i];
  W.busy = true;
  try { await AM.play.step(-1); } finally { W.busy = false; }
  W.steps.pop(); W.kinds.pop();
  state.run.segs[0].steps = W.steps.slice();
  afterStep();
}
function exit() {
  if (!W.on) return;
  W.on = false; document.body.classList.remove('walking'); paintBtn(); drawOverlay();
  AM.select({ t: 'step' });
}
function afterStep() {
  if (!W.on) return;
  refreshOpts(false);
  AM.select({ t: 'walk' });
  drawOverlay();
}

/* ---------- map overlay (level "functions of a module") ---------- */
const NSV = (tag, attrs, parent, text) => AM.svg(tag, attrs, parent, text);
function drawOverlay() {
  const fv = AM.map.functions;
  if (fv && fv.svg) Array.from(fv.svg.querySelectorAll('.walkfx')).forEach((x) => x.remove());
  if (!W.on || state.view !== 'map' || state.level !== 2 || !fv || !fv.nodes) return;
  const me = fv.nodes[cur()]; if (!me) return;
  const g = NSV('g', { class: 'walkfx' }, fv.g || fv.svg), R = (n) => n.r;
  const a = R(me), list = shown();
  // the trace: numbered badges and the path between the steps that are on this module
  W.steps.forEach((fid, i) => {
    const n = fv.nodes[fid]; if (!n) return;
    const r = R(n), prev = i && fv.nodes[W.steps[i - 1]];
    if (prev && prev !== n) {
      const p = R(prev);
      NSV('path', { d: curve(p, r), class: 'wk-trace' + (W.kinds[i] === 'queue' ? ' q' : ''), 'marker-end': 'url(#arrA)' }, g);
    }
    const dup = W.steps.slice(0, i).filter((f) => f === fid).length;
    const bx = r.x + 10 + dup * 22, by = r.y - 2;
    NSV('circle', { cx: bx, cy: by, r: 9, class: 'wk-badge' }, g);
    NSV('text', { x: bx, y: by + 3.5, 'text-anchor': 'middle', class: 'wk-badge-t' }, g, String(i + 1));
  });
  // outgoing arrows
  let stub = 0; const boxes = [a];
  list.forEach((o, k) => {
    const gg = NSV('g', { class: 'wk-opt ' + o.kind + (k === W.sel ? ' sel' : '') + (o.seen ? ' seen' : '') + (o.guided ? ' guided' : ''), tabindex: -1 }, g);
    const tn = o.fid && fv.nodes[o.fid];
    let from, to, lab, chip = null;
    if (tn && tn !== me) {
      const t = R(tn); from = a; to = t; boxes.push(t);
      NSV('path', { d: curve(a, t), class: 'ln hit' }, gg);
      NSV('path', { d: curve(a, t), class: 'ln', 'marker-end': k === W.sel ? 'url(#arrA)' : 'url(#arrM)' }, gg);
      lab = mid(a, t);
    } else {
      const w = Math.max(70, Math.min(230, o.label.length * 6.4 + 26)), x = a.x + a.w + 54, y = a.y - 8 + stub * 32;
      stub++;
      chip = { x, y, w, h: 24 }; boxes.push(chip);
      NSV('path', { d: `M${a.x + a.w},${a.y + a.h / 2} C${a.x + a.w + 30},${a.y + a.h / 2} ${x - 30},${y + 12} ${x},${y + 12}`, class: 'ln hit' }, gg);
      NSV('path', { d: `M${a.x + a.w},${a.y + a.h / 2} C${a.x + a.w + 30},${a.y + a.h / 2} ${x - 30},${y + 12} ${x},${y + 12}`, class: 'ln', 'marker-end': k === W.sel ? 'url(#arrA)' : 'url(#arrM)' }, gg);
      NSV('rect', { x, y, width: w, height: 24, rx: 12, class: 'chip' }, gg);
      NSV('text', { x: x + 12, y: y + 16, class: 'chip-t' }, gg, AM.cut(o.label, 34));
    }
    if (!chip) {
      const tx = AM.cut((k + 1) + '. ' + o.label, 30);
      NSV('rect', { x: lab.x - tx.length * 3.3 - 6, y: lab.y - 11, width: tx.length * 6.6 + 12, height: 17, rx: 8, class: 'lab-bg' }, gg);
      NSV('text', { x: lab.x, y: lab.y + 1, 'text-anchor': 'middle', class: 'lab' }, gg, tx);
    }
    NSV('title', null, gg, o.label + (o.guided ? ' · ' + T('wkGuide') : '') + (o.seen ? ' · ' + T('wkSeen') : ''));
    gg.addEventListener('click', (e) => { e.stopPropagation(); W.sel = k; take(o); });
    gg.addEventListener('mouseenter', () => { if (W.sel !== k && !W.busy) { W.sel = k; paintSel(); } });
  });
  if (AM.bbox && fv.cam && !fv.cam.contains(AM.bbox(boxes), 10)) fv.cam.animTo(AM.bbox(boxes), 40, 0, 280);
}
function curve(a, b) {
  const cx = a.x + a.w / 2, tx = b.x + b.w / 2;
  if (b.y > a.y + a.h) { const y0 = a.y + a.h, y1 = b.y, m = (y0 + y1) / 2; return `M${cx},${y0} C${cx},${m} ${tx},${m} ${tx},${y1}`; }
  if (b.y + b.h < a.y) { const y0 = a.y, y1 = b.y + b.h, m = (y0 + y1) / 2; return `M${cx},${y0} C${cx},${m} ${tx},${m} ${tx},${y1}`; }
  const right = tx >= cx, x0 = right ? a.x + a.w : a.x, x1 = right ? b.x : b.x + b.w, m = (x0 + x1) / 2, ya = a.y + a.h / 2, yb = b.y + b.h / 2;
  return `M${x0},${ya} C${m},${ya} ${m},${yb} ${x1},${yb}`;
}
function mid(a, b) {
  const cx = a.x + a.w / 2, tx = b.x + b.w / 2;
  if (b.y > a.y + a.h) return { x: (cx + tx) / 2, y: (a.y + a.h + b.y) / 2 };
  if (b.y + b.h < a.y) return { x: (cx + tx) / 2, y: (a.y + b.y + b.h) / 2 };
  const right = tx >= cx; return { x: ((right ? a.x + a.w : a.x) + (right ? b.x : b.x + b.w)) / 2, y: (a.y + b.y + a.h / 2 + b.h / 2) / 2 - 6 };
}
function paintSel() {
  const fv = AM.map.functions;
  if (fv && fv.svg) fv.svg.querySelectorAll('.walkfx .wk-opt').forEach((g, i) => g.classList.toggle('sel', i === W.sel));
  Array.from(document.querySelectorAll('#pane .wk-opts li')).forEach((li) => li.classList.toggle('sel', +li.dataset.k === W.sel));
  drawOverlay(); renderCard();
}

/* ---------- the right card ---------- */
function renderCard() { if (W.on && state.sel && state.sel.t === 'walk') AM.renderPanel(); }
AM.registerPanel('walk', (sel, ctx) => {
  if (!W.on) return '';
  const fid = cur(), f = ctx.fnInfo(fid), list = shown(), n = W.opts.length - list.length;
  const o = list[W.sel];
  let h = `<div class="kicker"><span class="chip">${esc(T('wkStep'))} ${W.steps.length}</span>${f.mod ? `<span class="chip" data-mod="${esc(f.mod)}" style="cursor:pointer"><i style="background:${ctx.LCOLOR[ctx.layerOf(f.mod)]}"></i>${esc(ctx.cutL(ctx.short(f.mod), 30))}</span>` : ''}</div>`;
  h += `<h2 class="mono">${esc(f.name)}</h2>`;
  h += `<div class="btnrow"><button class="pbtn" data-w="back"${W.steps.length < 2 ? ' disabled' : ''}>${esc(T('wkBack'))}</button><button class="pbtn" data-w="exit">${esc(T('wkExit'))}</button>` +
    (AM.spec && AM.spec.on ? `<button class="pbtn primary" data-w="save">${esc(T('wkSave'))}</button>${deviation() >= 0 ? `<button class="pbtn" data-w="savevar">${esc(T('wkSaveVar'))}</button>` : ''}` : '') + `</div>`;
  if (W.form) h += formHTML();
  h += `<h3>${esc(T('wkNext'))} (${list.length})</h3>`;
  if (!W.opts.length) h += `<p class="loc">${esc(T('wkNone'))}</p>`;
  h += `<ul class="rel-list wk-opts">` + list.map((x, k) => {
    const mk = x.kind === 'queue' ? `<span class="q">${esc(T('wkQueue').replace(' →', ''))}</span>` : x.kind === 'ext' ? `<span class="q">${W.exts[W.steps.length - 1] === x.ext ? '✓ ' : ''}ext</span>` : '';
    return `<li data-k="${k}" class="${k === W.sel ? 'sel ' : ''}${x.seen ? 'seen ' : ''}${x.guided ? 'guided' : ''}" title="${esc(x.fid || x.ext)}"><span class="no">${k + 1}</span><span class="fn">${esc(x.label)}</span>${mk}` +
      (x.guided ? `<span class="q g">★ ${esc(T('wkGuide'))}</span>` : '') + (x.seen ? `<span class="q">${esc(T('wkSeen'))}</span>` : '') + `</li>`;
  }).join('') + `</ul>`;
  if (n > 0 || W.helpers) h += `<button class="pbtn" data-w="helpers" type="button">${esc(W.helpers ? T('wkLess') : T('wkMore', n))}</button>`;
  h += `<p class="loc" style="margin-top:8px">${esc(T('wkKeys'))}</p>`;
  // code with the call of the chosen arrow
  const tgt = o && o.fid ? o.fid : null, c = ctx.codeHTML(fid, tgt);
  h += `<h3>${esc(T('wkCode'))}</h3><div class="code" id="pcode">${c.html}</div>`;
  const st = AM.play.current();
  if (st && st.data && st.data.length) h += `<h3>${esc(T('wkData'))}</h3><ul class="rel-list">` + st.data.map((d) => `<li><span class="fn">${esc(d.table)}</span><span class="q">${esc(d.op)}</span><span class="md">${esc((d.fields || []).slice(0, 6).join(', '))}</span></li>`).join('') + `</ul>`;
  const eff = (st && st.effects) || [], sts = (st && st.states) || [];
  if (sts.length || eff.length) h += `<h3>${esc(T('wkStatus'))}</h3><div class="btnrow">${sts.map((s) => ctx.ui.statusPill(s)).join('')}${eff.map((e) => `<span class="chip">${esc(e)}</span>`).join('')}</div>`;
  // the trace
  h += `<h3>${esc(T('wkTrace'))}</h3><ol class="wk-trace-list">` + W.steps.map((id, i) => {
    const last = i === W.steps.length - 1, kind = W.kinds[i];
    let r = `<li data-i="${i}"><span class="no">${i + 1}</span><span class="fn" data-fn="${esc(id)}">${kind === 'queue' ? esc(T('wkQueue')) + ' ' : ''}${esc(ctx.fnInfo(id).name)}</span>${W.exts[i] ? `<span class="q">ext ${esc(W.exts[i])}</span>` : ''}`;
    if (AM.spec && AM.spec.on) r += `<input class="cap" data-cap="${i}" type="text" placeholder="${esc(T('wkCap'))}" value="${esc(W.caps[i] || '')}" aria-label="${esc(T('wkCap'))}">`;
    (W.plans[i] || []).forEach((p) => { r += `<div class="plan">⌁ ${esc(p)}</div>`; });
    if (last && AM.spec && AM.spec.on) r += `<div class="btnrow"><button class="pbtn" data-w="plan">${esc(T('wkPlan'))}</button><button class="pbtn" data-w="alt">${esc(T('wkAlt'))}</button>${W.alts.length ? `<span class="q">${esc(T('wkAltN', W.alts.length))}</span>` : ''}</div>`;
    return r + '</li>';
  }).join('') + `</ol>`;
  return {
    html: h,
    after(p) {
      const q = (a) => p.querySelector(`[data-w="${a}"]`);
      q('back').onclick = back; q('exit').onclick = exit;
      if (q('helpers')) q('helpers').onclick = () => { W.helpers = !W.helpers; refreshOpts(false); drawOverlay(); renderCard(); };
      p.querySelectorAll('.wk-opts li').forEach((li) => { li.onclick = () => { W.sel = +li.dataset.k; take(shown()[W.sel]); }; });
      p.querySelectorAll('[data-fn]').forEach((e) => e.addEventListener('click', () => AM.openFn(e.getAttribute('data-fn'))));
      p.querySelectorAll('input[data-cap]').forEach((i) => { i.onkeydown = (e) => e.stopPropagation(); i.oninput = () => { W.caps[+i.dataset.cap] = i.value; }; });
      if (q('plan')) q('plan').onclick = (e) => AM.spec.ask(p, e.clientX, e.clientY, '', T('wkPlanAsk'), (v) => { if (v) { const i = W.steps.length - 1; (W.plans[i] = W.plans[i] || []).push(v); renderCard(); } });
      if (q('alt')) q('alt').onclick = () => { W.form = { kind: 'alt' }; renderCard(); };
      if (q('save')) q('save').onclick = () => { W.form = { kind: 'save' }; renderCard(); };
      if (q('savevar')) q('savevar').onclick = () => { W.form = { kind: 'var' }; renderCard(); };
      wireForm(p);
      const hl = p.querySelector('#pcode .hl'); if (hl) hl.scrollIntoView({ block: 'nearest' });
      if (c.loading || (!c.has && AM.srcOf(fid) == null)) AM.loadSrc(fid).then(() => { if (W.on && cur() === fid && state.sel && state.sel.t === 'walk') AM.renderPanel(); });
    },
  };
});

/* ---------- save as scenario / variant / branch (edit mode) ---------- */
const slug = (s) => String(s).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
function formHTML() {
  const fm = W.form, row = (k, lk, v) => `<label><span>${esc(T(lk))}</span><input type="text" data-f="${k}" value="${esc(v || '')}" autocomplete="off" spellcheck="false"></label>`;
  let body = '';
  const first = AM.fnInfo(W.steps[0]).short || AM.fnInfo(W.steps[0]).name;
  if (fm.kind === 'save') {
    const actor = (AM.D.actors || []).length && AM.actorName ? '' : '';
    body = row('id', 'wkId', slug(first)) + row('name', 'wkName', first) + row('actor', 'wkActor', actor) + row('exit', 'wkExit', '');
  } else if (fm.kind === 'var') body = row('cond', 'wkVarCond', '') + row('id', 'wkId', slug((W.guide && W.guide.id || first) + '-alt'));
  else body = row('cond', 'wkAltCond', '') + row('text', 'wkAltText', '');
  return `<div class="wk-form" data-kind="${fm.kind}">${body}<div class="btnrow"><button class="pbtn primary" data-w="ok">${esc(T('wkOk'))}</button><button class="pbtn" data-w="cancel">${esc(T('wkCancel'))}</button></div></div>`;
}
function wireForm(p) {
  const f = p.querySelector('.wk-form'); if (!f) return;
  const val = (k) => ((f.querySelector(`[data-f="${k}"]`) || {}).value || '').trim();
  f.querySelectorAll('input').forEach((i) => { i.onkeydown = (e) => { e.stopPropagation(); if (e.key === 'Enter') f.querySelector('[data-w="ok"]').click(); else if (e.key === 'Escape') { W.form = null; renderCard(); } }; });
  f.querySelector('[data-w="cancel"]').onclick = () => { W.form = null; renderCard(); };
  const first = f.querySelector('input'); if (first) setTimeout(() => first.focus(), 0);
  f.querySelector('[data-w="ok"]').onclick = async () => {
    const kind = f.dataset.kind;
    if (kind === 'alt') { if (!val('cond')) return; W.alts.push({ cond: val('cond').replace(/\s+/g, '_'), text: val('text') }); W.form = null; renderCard(); return; }
    const id = val('id'); if (!id) return;
    const ok = kind === 'save' ? await saveScenario(id, val('name'), val('actor'), val('exit')) : await saveVariant(id, val('cond'));
    if (ok) { W.form = null; AM.spec.curFlow = id; AM.spec.setTab('flows'); }
  };
}
function stepOps(flow, from, to) {
  const ops = [];
  for (let i = from; i < to; i++) {
    const s = { op: 'add_step', flow, ref: W.steps[i] };
    if (W.caps[i]) s.text = W.caps[i].trim();
    if (W.exts[i]) s.ext = W.exts[i];
    ops.push(s);
    (W.plans[i] || []).forEach((p) => ops.push({ op: 'add_step', flow, planned: true, text: p }));
  }
  return ops;
}
async function saveScenario(id, name, actor, exit) {
  const ops = [Object.assign({ op: 'add_flow', id, name: name || id }, actor ? { actor } : {})];
  if (exit) ops.push({ op: 'set_exit', flow: id, exit });
  ops.push(...stepOps(id, 0, W.steps.length));
  W.alts.forEach((a) => ops.push(Object.assign({ op: 'add_alt', flow: id, cond: a.cond }, a.text ? { text: a.text } : {})));
  return AM.spec.send(ops);
}
// a variant: the steps from the one before the deviation, as a new scenario, hooked to the guide scenario by a branch
async function saveVariant(id, cond) {
  const d = Math.max(0, deviation() - 1), g = W.guide;
  const ops = [{ op: 'add_flow', id, name: (g && g.name ? g.name + ': ' : '') + (cond || id) }];
  ops.push(...stepOps(id, d, W.steps.length));
  const specFlow = g && AM.spec.model && (AM.spec.model.flows || []).some((f) => f.id === g.id);
  if (specFlow && cond) ops.push({ op: 'add_alt', flow: g.id, cond: cond.replace(/\s+/g, '_'), target: id });
  return AM.spec.send(ops);
}

/* ---------- start points, keys, header ---------- */
function startDefault() {
  const r = state.run, fl = r && r.segs && r.segs[0] && r.segs[0].flow;
  if (fl && fl.steps && fl.steps.length) {
    // from the current step of a scenario (else from its first step): the scenario guides the walk
    const steps = fl.steps.filter((x) => D.fns[x]), at = state.step > 0 && r.segs[0].steps[state.step] ? steps.indexOf(r.segs[0].steps[state.step]) : 0;
    return start(steps[Math.max(0, at)], { id: fl.id, name: fl.name, steps, start: Math.max(0, at) });
  }
  const s = state.sel, fid = s && s.t === 'fn' ? s.id : state.fn || (D.entries && D.entries[0]);
  if (fid) return start(fid);
}
function paintBtn() {
  const b = document.getElementById('walkBtn'); if (!b) return;
  b.setAttribute('aria-pressed', W.on); b.title = T('wkBtnTip'); b.querySelector('span').textContent = T('wkBtn');
}
function onKey(e) {
  if (!W.on || e.metaKey || e.ctrlKey || e.altKey) return;
  const tg = e.target.tagName; if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT') return;
  const list = shown(), k = e.key;
  const stop = () => { e.preventDefault(); e.stopImmediatePropagation(); };
  if (k === 'ArrowRight' || (k === 'Enter' && tg !== 'BUTTON')) { stop(); take(list[W.sel]); }
  else if (k === 'ArrowDown' || k === 'ArrowUp') { stop(); if (list.length) { W.sel = (W.sel + (k === 'ArrowDown' ? 1 : list.length - 1)) % list.length; paintSel(); } }
  else if (k === 'ArrowLeft' || k === 'Backspace') { stop(); back(); }
  else if (k === 'Escape') { if (AM.dock && AM.dock.shown && document.querySelector('.seqdock.full')) return; stop(); exit(); }
}
window.addEventListener('keydown', onKey, true);
{
  const b = document.getElementById('walkBtn');
  if (b) b.onclick = () => (W.on ? exit() : startDefault());
}
AM.bus.on('level', () => { if (W.on) drawOverlay(); });
AM.bus.on('lang', () => { paintBtn(); renderCard(); });
AM.bus.on('live', () => { if (W.on) { W.steps = W.steps.filter((x) => D.fns[x]); if (!W.steps.length) exit(); else { refreshOpts(true); drawOverlay(); renderCard(); } } });
AM.bus.on('spec', () => { if (W.on && W.form) return; renderCard(); });
AM.bus.on('edit', () => renderCard());
AM.walk = { start, exit, startDefault, get on() { return W.on; }, get state() { return W; } };
paintBtn();
