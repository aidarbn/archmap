/* journeys.js — through paths (model.journeys) and flow variants (flow.variants) in the viewer.
 *
 * Lists: three levels — "Through paths" (journeys) → "Scenarios" (flows, as before) → the foldable "Variants" of a
 * scenario. The three lists (left column of the lists, the scenario column of the Scenario tab, the system panel)
 * share the row builders below and differ only in the CSS classes and the way a row carries its run key (cfg).
 *
 * Runs (see playback.js): key 'journey:<id>' plays the flows of a path one after another (a segment per flow,
 * seg.via = the link that leads into it); key 'variant:<id>' plays one variant of a flow — the forks are taken
 * automatically (steps end at the fork), the end of the variant is a plate: a flow to go on in, or the exit text.
 * Such runs are `fixed`: no "pick a branch" stop.
 */
addI18n({
  ru: {
    jvTitle: 'Сквозные пути', jvVariantsTip: 'Варианты сценария: развилки по условиям', jvMain: 'основной путь',
    jvAnyOf: (s) => 'одна из: ' + s, jvMore: (n) => '…ещё ' + n, jvFlowsN: (n, p) => n + ' ' + p(['сценарий', 'сценария', 'сценариев']),
    jvPath: 'Путь', jvVariant: 'Вариант', jvFlowOf: (a, b) => 'сценарий ' + a + '/' + b, jvStepOf: (a, b) => 'шаг ' + a + '/' + b,
    jvViaOn: (c) => (c ? '→ по условию «' + c + '»' : '→ по событию'), jvViaQueue: '→ через очередь',
    jvViaDecl: (n) => 'дальше клиент вызывает ' + n, jvClient: 'дальше вызывает клиент', jvFork: 'Развилка', jvGo: 'Перейти в сценарий', jvGoTip: 'Продолжить проигрывание в сценарии, в который ведёт вариант',
    jvIcOn: 'по условию (arch:on)', jvIcQueue: 'через очередь', jvIcDecl: 'порядок указан вручную', jvEnd: 'конец варианта',
  },
  en: {
    jvTitle: 'Through paths', jvVariantsTip: 'Scenario variants: forks by conditions', jvMain: 'main path',
    jvAnyOf: (s) => 'one of: ' + s, jvMore: (n) => '…' + n + ' more', jvFlowsN: (n, p) => n + ' ' + p(['scenario', 'scenarios']),
    jvPath: 'Path', jvVariant: 'Variant', jvFlowOf: (a, b) => 'flow ' + a + '/' + b, jvStepOf: (a, b) => 'step ' + a + '/' + b,
    jvViaOn: (c) => (c ? '→ on condition “' + c + '”' : '→ on an event'), jvViaQueue: '→ through the queue',
    jvViaDecl: (n) => 'next the client calls ' + n, jvClient: 'next the client calls', jvFork: 'Fork', jvGo: 'Go to the scenario', jvGoTip: 'Continue the run in the scenario the variant leads to',
    jvIcOn: 'on a condition (arch:on)', jvIcQueue: 'through the queue', jvIcDecl: 'order written by hand', jvEnd: 'end of the variant',
  },
});

const JV = { open: new Set() }; // flow ids whose variants are unfolded

/* ---------- data ---------- */
function jvJourneys() {
  return ((D.M && D.M.journeys) || []).map((j) => Object.assign({}, j, { fls: (j.flows || []).map((id) => D.flowById[id]).filter(Boolean) })).filter((j) => j.fls.length > 0);
}
function jvJourney(id) { return jvJourneys().find((j) => j.id === id) || null; }
// the order a path is played in: a walk over the links from the first flow (children in the order of j.flows)
function jvOrder(j) {
  const pos = {}; (j.flows || []).forEach((id, i) => { pos[id] = i; });
  const kids = {}; (j.links || []).forEach((l) => { (kids[l.from] = kids[l.from] || []).push(l); });
  const seen = new Set(), out = [];
  const visit = (id, via) => {
    if (seen.has(id) || !D.flowById[id]) return;
    seen.add(id); out.push({ flow: D.flowById[id], via });
    (kids[id] || []).slice().sort((a, b) => (pos[a.to] || 0) - (pos[b.to] || 0)).forEach((l) => visit(l.to, l));
  };
  const ids = (j.flows || []).filter((id) => D.flowById[id]);
  if (ids.length) visit(ids[0], null);
  ids.forEach((id) => visit(id, (j.links || []).find((l) => l.to === id) || null));
  return out;
}
function jvVariant(id) {
  const fid = String(id).replace(/#\d+$/, ''), fl = D.flowById[fid];
  const v = fl && (fl.variants || []).find((x) => x.id === id);
  return v ? { fl, v } : null;
}
const jvVLabel = (v) => (v.main ? t('jvMain') : v.any ? t('jvAnyOf', (v.choices || []).map((c) => c.cond).filter(Boolean).join(' | ') || v.name) : v.name);

/* ---------- list rows ---------- */
const jvIcons = {
  on: '<svg class="jv-ic" viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><path d="M6 1.2 10.8 6 6 10.8 1.2 6z" fill="none" stroke="currentColor" stroke-width="1.4"/></svg>',
  queue: '<svg class="jv-ic" viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><path d="M1 3h6M1 6h6M1 9h6M8.5 4l2.3 2-2.3 2" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>',
  declared: '<svg class="jv-ic" viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><path d="M1 6h7M8 3.5 10.8 6 8 8.5" fill="none" stroke="currentColor" stroke-width="1.3" stroke-dasharray="2 1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>',
};
const jvIcon = (via) => `<span class="jv-lk" title="${esc({ on: t('jvIcOn'), queue: t('jvIcQueue') }[via] || t('jvIcDecl'))}">${jvIcons[via] || jvIcons.declared}</span>`;
function jvChain(j) {
  const ord = jvOrder(j), MAX = 5;
  let h = '';
  ord.slice(0, MAX).forEach((o, i) => { if (i) h += jvIcon(o.via ? o.via.via : 'declared'); h += `<span class="jv-f" title="${esc(o.flow.name)}">${esc(cut(o.flow.name, 26))}</span>`; });
  if (ord.length > MAX) h += `<span class="jv-f more">+${ord.length - MAX}</span>`;
  return h;
}
const jvHay = (j) => lc([j.name, j.desc, j.actor, j.exit].concat(j.fls.map((f) => f.name)).join(' '));
// cfg: {row: css class of a row, head(title, n): header html, attr(kind, id): data attributes of the row}
function jvJourneyRows(q, cfg, max) {
  const qq = lc(q).trim(), all = jvJourneys().filter((j) => !qq || jvHay(j).includes(qq));
  if (!all.length) return '';
  const js = max ? all.slice(0, max) : all, cur = state.run && state.run.key;
  const tail = all.length > js.length ? `<div class="loc" style="padding:2px 8px">+ ${all.length - js.length}</div>` : '';
  return cfg.head(t('jvTitle'), all.length) + js.map((j) => {
    const sub = (j.actor ? esc(j.actor) + ' · ' : '') + esc(t('jvFlowsN', j.fls.length));
    return `<div class="${cfg.row} jv-j${cur === 'journey:' + j.id ? ' cur' : ''}" role="option" ${cfg.attr('journey', j.id)} title="${esc(j.name + (j.desc ? '\n' + j.desc : ''))}"><span class="nm">${esc(j.name)}</span><span class="sb">${sub}</span><span class="jv-chain">${jvChain(j)}</span></div>`;
  }).join('') + tail;
}
const jvVarHits = (fl, qq) => (qq ? (fl.variants || []).filter((v) => lc(jvVLabel(v) + ' ' + v.name).includes(qq)) : []);
const jvFlowHit = (fl, q) => { const qq = lc(q).trim(); return !!qq && jvVarHits(fl, qq).length > 0; };
function jvTog(fl) {
  const vs = fl.variants || []; if (!vs.length) return '';
  const on = JV.open.has(fl.id);
  return `<button type="button" class="jv-tog${on ? ' on' : ''}" data-vt="${esc(fl.id)}" aria-expanded="${on}" title="${esc(t('jvVariantsTip'))}">${on ? '▾' : '▸'} ${vs.length}${fl.variantsMore ? '+' : ''}</button>`;
}
function jvVariantRows(fl, q, cfg) {
  const vs = fl.variants || []; if (!vs.length) return '';
  const hits = jvVarHits(fl, lc(q).trim());
  if (!JV.open.has(fl.id) && !hits.length) return '';
  const list = hits.length ? hits : vs, cur = state.run && state.run.key;
  let h = list.map((v) => `<div class="${cfg.row} jv-v${cur === 'variant:' + v.id ? ' cur' : ''}" role="option" ${cfg.attr('variant', v.id)} title="${esc(jvVLabel(v))}"><span class="nm">${esc(jvVLabel(v))}</span><span class="sb">${v.steps.length}</span></div>`).join('');
  if (list === vs && fl.variantsMore) h += `<div class="jv-more">${esc(t('jvMore', fl.variantsMore))}</div>`;
  return h;
}
function jvToggle(id) { if (JV.open.has(id)) JV.open.delete(id); else JV.open.add(id); bus.emit('jv'); }

/* ---------- runs ---------- */
function jvBuildJourney(key) {
  const j = jvJourney(key.slice(8)); if (!j) return null;
  // a step already played by an earlier flow of the path is not played again (a queue handler that the caller's
  // flow already walks into, a shared service call): the path reads once, start to end
  const segs = [], seen = new Set();
  jvOrder(j).forEach((o) => {
    const steps = flowSteps(o.flow).filter((s) => isPlan(s) || !seen.has(s));
    steps.forEach((s) => seen.add(s));
    if (steps.length) segs.push({ flow: o.flow, steps, via: o.via, from: o.via && D.flowById[o.via.from] });
  });
  if (!segs.length) return null;
  return { key, name: j.name, desc: j.desc || '', kind: 'journey', fixed: true, journey: j, actor: j.actor, segs };
}
function jvBuildVariant(key) {
  const r = jvVariant(key.slice(8)); if (!r) return null;
  const { fl, v } = r;
  const keep = new Set(flowSteps(fl)), steps = (v.main ? fl.steps : v.steps).filter((s) => keep.has(s));
  if (!steps.length) return null;
  return { key, name: fl.name + ' · ' + jvVLabel(v), desc: fl.desc || '', kind: 'variant', fixed: true, variant: v, baseFlow: fl, segs: [{ flow: fl, steps, variant: v }] };
}
// text of the plate between two flows (seg = the flow we have just entered)
function jvViaText(sg) {
  const via = sg.via; if (!via) return '';
  if (via.via === 'variant') return t('jvViaOn', via.cond) + ' · ' + sg.flow.name;
  if (via.via === 'queue') return t('jvViaQueue') + ' · ' + sg.flow.name;
  if (via.via === 'on') return t('jvViaOn', via.cond) + ' · ' + sg.flow.name;
  return t('jvViaDecl', '«' + sg.flow.name + '»');
}
const jvViaKind = (sg) => (sg.via ? (sg.via.via === 'variant' ? 'on' : sg.via.via) : 'declared');
// how the run ends: {text, tone: 'good'|'bad'|'link', jump?: flow}; null while there is nothing to say
function jvEnd() {
  const r = state.run; if (!r || !r.fixed) return null;
  const sg = r.segs[r.segs.length - 1], v = sg.variant;
  const bad = (s) => /(fail|error|cancel|reject|denied|block|expire|timeout|ошиб|отмен|отклон|истек|истёк|отказ|не оплач)/i.test(s);
  if (v && !v.main) {
    const tg = v.end && v.end.kind === 'flow' && D.flowById[v.end.flow];
    if (tg && (tg.steps || []).some((s) => D.fns[s])) return { text: t('jvFork') + ': ' + v.name, tone: 'link', jump: tg };
    const tx = L((v.end && v.end.text) || v.name);
    return { text: t('jvFork') + ': ' + v.name, tone: bad(String(tx)) ? 'bad' : 'good', exit: true };
  }
  const ex = L(r.kind === 'journey' ? (r.journey.exit || (sg.flow && sg.flow.exit)) : (sg.flow && sg.flow.exit));
  return ex ? { text: t('exit') + ': ' + ex, tone: bad(String(ex)) ? 'bad' : 'good', exit: true } : null;
}
// the plate to show at the current step: entering a flow of a path, or the end of the run
function jvPlate() {
  const r = state.run; if (!r || !r.fixed || state.step < 0) return null;
  const L_ = runList(), it = L_[state.step]; if (!it) return null;
  const first = it.seg > 0 && L_[state.step - 1].seg !== it.seg;
  if (first) return { text: jvViaText(r.segs[it.seg]), tone: 'link', kind: jvViaKind(r.segs[it.seg]) };
  if (state.step === L_.length - 1) return jvEnd();
  return null;
}
// go on in the flow a variant leads to: a new segment, flagged `extra` so that stepping back drops it
async function jvJump() {
  const r = state.run; if (!r || !r.fixed || state.busy) return;
  const e = jvEnd(); if (!e || !e.jump) return;
  const sg = r.segs[r.segs.length - 1], last = runList().length - 1;
  r.segs.push({ flow: e.jump, steps: flowSteps(e.jump), via: { via: 'variant', cond: (sg.variant && sg.variant.choices && sg.variant.choices[0] && sg.variant.choices[0].cond) || '' }, extra: true });
  state.step = last;
  applyFocus();
  await doStep(1);
}

/* ---------- progress and label ---------- */
function jvProgress() {
  const r = state.run; if (!r || !r.fixed) return '';
  const L_ = runList(), it = L_[Math.max(0, state.step)]; if (!it) return '';
  const sg = r.segs[it.seg];
  let k = 0; for (let i = 0; i < L_.length; i++) if (L_[i].seg === it.seg && i <= Math.max(0, state.step)) k++;
  const step = state.step < 0 ? 0 : k;
  return (r.segs.length > 1 ? t('jvFlowOf', it.seg + 1, r.segs.length) + ' · ' : '') + t('jvStepOf', step, sg.steps.length);
}
function jvTagHTML() {
  const r = state.run; if (!r || !r.fixed) return '';
  const lab = r.kind === 'journey' ? t('jvPath') : t('jvVariant');
  return `<b>${esc(lab)}</b><span class="nm">${esc(r.kind === 'journey' ? r.name : jvVLabel(r.variant))}</span>` +
    (r.kind === 'variant' ? `<span class="fl">${esc(r.baseFlow.name)}</span>` : '') + `<span class="pr">${esc(jvProgress())}</span>`;
}
function jvPaintTags() {
  const h = jvTagHTML();
  $$('.jv-tag').forEach((el) => { el.hidden = !h; el.innerHTML = h; el.title = state.run ? state.run.name : ''; });
}

Object.assign(AM, { jv: { open: JV.open, toggle: jvToggle, journeyRows: jvJourneyRows, variantRows: jvVariantRows, tog: jvTog, flowHit: jvFlowHit, plate: jvPlate, end: jvEnd, jump: jvJump, viaText: jvViaText, viaKind: jvViaKind, progress: jvProgress, tag: jvTagHTML, paintTags: jvPaintTags, journeys: jvJourneys, icon: jvIcon } });
