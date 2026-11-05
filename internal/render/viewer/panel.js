/* panel.js — the right card for the current selection (no selection — no panel).
 * Selection kinds of the core: null, {t:'svc'}, {t:'actor', id}, {t:'ext', id},
 * {t:'mod', id}, {t:'group', id}, {t:'fn', id, why?}, {t:'step'}. Views add kinds with
 * registerPanel(kind, render) and select them with AM.select({t: kind, …}). */

addI18n({
  ru: {
    dsWho: 'Кто это', dsWhere: 'Где определён', dsImpl: 'Реализация', dsNone: 'В коде нет описания — его можно дать разметкой',
    dsScen: 'Сценарии', dsThrough: 'Сценарии через этот узел', dsEps: 'Ручки', dsRunTip: 'Открыть сценарий', dsFnTip: 'Открыть функцию',
    dsEpOf: 'Ручка', dsSrc: { arch: 'из разметки arch:actor', proto: 'из комментария .proto', type: 'из комментария типа', package: 'из описания пакета' },
    dsToApi: 'API ↘', dsToData: 'Данные ↘', dsToStatus: 'Статусы ↘', dsTables: 'Таблицы', scenN: (n, p) => n + ' ' + p(['сценарий', 'сценария', 'сценариев']),
  },
  en: {
    dsWho: 'What it is', dsWhere: 'Defined in', dsImpl: 'Implemented by', dsNone: 'No description in the code — the markup can give one',
    dsScen: 'Scenarios', dsThrough: 'Scenarios through this node', dsEps: 'Endpoints', dsRunTip: 'Open the scenario', dsFnTip: 'Open the function',
    dsEpOf: 'Endpoint', dsSrc: { arch: 'from arch:actor markup', proto: 'from the .proto comment', type: 'from the type comment', package: 'from the package doc' },
    dsToApi: 'API ↘', dsToData: 'Data ↘', dsToStatus: 'Statuses ↘', dsTables: 'Tables', scenN: (n) => n + (n === 1 ? ' scenario' : ' scenarios'),
  },
});
// flows that start at a function / pass through it / through a module
// (technical scenarios — flow.hidden — are not listed: see autosem/trim.go)
const flowsFrom = (fid) => visibleFlows().filter((fl) => fl.steps && fl.steps[0] === fid);
const flowsThrough = (pred) => visibleFlows().filter((fl) => flowSteps(fl).some((s) => D.fns[s] && pred(s)));
// the views reached from a card (there is no other way to them but ⋯): API, Data (a table), Statuses (a machine)
const hasView = (id) => VIEWS.some((v) => v.id === id);
const viewBtn = (act, label, arg) => `<button class="pbtn" data-view="${act}"${arg != null ? ` data-arg="${esc(arg)}"` : ''}>${esc(label)}</button>`;
function tableChips(fids) {
  if (!hasView('data')) return '';
  const tb = []; fids.forEach((id) => ((D.fns[id] || {}).data || []).forEach((a) => { if (D.tables[a.table] && !tb.includes(a.table)) tb.push(a.table); }));
  return tb.length ? `<h3>${esc(t('dsTables'))}</h3><div class="tbl-pills">${tb.slice(0, 12).map((n) => `<span class="chip" data-view="table" data-arg="${esc(n)}" style="cursor:pointer" title="${esc(t('dsToData'))}">▦ ${esc(n)}</span>`).join('')}</div>` : '';
}
function statusBtns(fids) {
  if (!hasView('status')) return '';
  const ms = []; fids.forEach((id) => (D.transByFid[id] || []).forEach((tr) => { if (!ms.includes(tr.entity)) ms.push(tr.entity); }));
  return ms.slice(0, 3).map((m) => viewBtn('status', t('dsToStatus') + (ms.length > 1 ? ' · ' + m : ''), m)).join('');
}
const visibleOfActor = (a) => { const me = actorName(a).trim().toLowerCase(); return visibleFlows().filter((fl) => (fl.actor && fl.actor.trim().toLowerCase() === me) || (fl.steps && a.fids.includes(fl.steps[0]))); };
const flowLine = (fl) => fl.exit || fl.desc || t('pkStepsN', flowSteps(fl).length);
// a list of scenarios: name and one line (exit / description / steps); a click opens the run
function flowList(fls, title, max = 12) {
  if (!fls.length) return '';
  const top = fls.slice().sort((a, b) => (b.kind === 'declared') - (a.kind === 'declared') || (b.top ? 1 : 0) - (a.top ? 1 : 0)).slice(0, max);
  return `<h3>${esc(title)} (${fls.length})</h3><ul class="rel-list">` + top.map((fl) => `<li class="dsc" data-run="flow:${fl.i}" title="${esc(t('dsRunTip'))}"><span class="run">▶</span><span class="fn" style="font-family:inherit">${esc(fl.name || '')}</span><span class="ds">${esc(cut(flowLine(fl), 90))}</span></li>`).join('') + (fls.length > max ? `<li class="md">+${fls.length - max}</li>` : '') + '</ul>';
}
// the actor card: who it is, where it is defined, its scenarios and its endpoints by service with descriptions
function actorHTML(a) {
  const ad = actorDesc(a);
  let h = `<div class="kicker">${esc(t('kinds').actor)}</div><h2>${esc(actorName(a))}</h2>`;
  if (ad.desc) h += `<p class="lead">${esc(ad.desc)}</p>`;
  else h += `<p class="lead">${esc(actorSub(a))}</p><p class="nodesc">${esc(t('dsNone'))}: <code>arch:actor ${esc(actorName(a))} "…"</code></p>`;
  const si = ad.svcInfo, protos = Array.from(a.protos).join(', ');
  const who = [protos, ad.svcs.slice(0, 3).join(', ') + (ad.svcs.length > 3 ? ' +' + (ad.svcs.length - 3) : '')].filter(Boolean).join(' · ') || actorSub(a);
  h += `<dl class="kv"><dt>${esc(t('dsWho'))}</dt><dd>${esc(who)}${a.eps.length ? ' · ' + esc(t('epsN', a.eps.length)) : ''}</dd>`;
  if (ad.file) h += `<dt>${esc(t('dsWhere'))}</dt><dd class="mono" style="font-size:11.5px">${esc(ad.file)}${ad.line ? ':' + ad.line : ''}${ad.from ? ` <span class="md" style="color:var(--muted)">· ${esc(t('dsSrc')[ad.from] || '')}</span>` : ''}</dd>`;
  if (si && si.impl) h += `<dt>${esc(t('dsImpl'))}</dt><dd class="mono" style="font-size:11.5px">${esc(si.impl)}${si.implFile ? ' · ' + esc(si.implFile.split('/').pop()) + ':' + si.implLine : ''}</dd>`;
  h += '</dl>';
  // scenarios of the actor: its through paths (they are also in the list), then its visible scenarios (technical ones are
  // left out — flow.hidden); the rest of them are the ▶ of the endpoint rows below
  const me = actorName(a).trim().toLowerCase(), MAXF = 12;
  const own = visibleOfActor(a).map((fl, i) => [fl, i]).sort((p, q) => (q[0].top ? 1 : 0) - (p[0].top ? 1 : 0) || p[1] - q[1]).map((p) => p[0]);
  const paths = jvJourneys().filter((j) => (j.actor || '').trim().toLowerCase() === me || (j.fls[0] && j.fls[0].steps && a.fids.includes(j.fls[0].steps[0])));
  if (paths.length || own.length) {
    h += `<h3>${esc(t('dsScen'))} (${paths.length + own.length})</h3><ul class="rel-list">`;
    h += paths.map((j) => `<li class="dsc" data-run="journey:${esc(j.id)}" title="${esc(t('dsRunTip'))}"><span class="run">▶</span><span class="fn" style="font-family:inherit">${esc(j.name || j.id)}</span><span class="md">${esc(t('pkPath'))}</span><span class="ds">${esc(cut(j.exit || j.fls.map((f) => f.name).join(' → '), 90))}</span></li>`).join('');
    h += own.slice(0, MAXF).map((fl) => `<li class="dsc" data-run="flow:${fl.i}" title="${esc(t('dsRunTip'))}"><span class="run">▶</span><span class="fn" style="font-family:inherit">${esc(fl.name || '')}</span><span class="ds">${esc(cut(flowLine(fl), 90))}</span></li>`).join('');
    if (own.length > MAXF) h += `<li class="md">+${own.length - MAXF}</li>`;
    h += '</ul>';
  }
  if (a.eps.length) {
    const by = new Map();
    a.eps.forEach((e) => { const k = e.service || (e.kind === 'grpc' ? 'gRPC' : 'HTTP'); if (!by.has(k)) by.set(k, []); by.get(k).push(e); });
    Array.from(by).sort((p, q) => q[1].length - p[1].length).forEach(([svc, eps]) => {
      const s = (D.F.services || {})[svc], sd = s && s.desc ? plainDoc(s.desc) : '';
      h += `<h3>${esc(svc)} · ${eps.length}${sd ? `<span class="hs">${esc(sd)}</span>` : ''}</h3><ul class="rel-list">`;
      h += eps.slice(0, 200).map((e) => {
        const fl = e.fid ? flowsFrom(e.fid)[0] : null, at = fl ? ` data-run="flow:${fl.i}" title="${esc(t('dsRunTip'))}: ${esc(fl.name)}"` : e.fid && D.fns[e.fid] ? ` data-fn="${esc(e.fid)}" title="${esc(t('dsFnTip'))}"` : '';
        return `<li class="dsc"${at}>${fl ? '<span class="run">▶</span>' : ''}<span class="verb">${esc(e.method || e.kind)}</span><span class="fn">${esc(e.name || e.path)}</span>${e.desc ? `<span class="ds">${esc(e.desc)}</span>` : ''}</li>`;
      }).join('') + '</ul>';
    });
  } else if (a.fids.length) h += relList(a.fids, t('entries'), 'out', '');
  h += `<h3>${esc(t('modules'))}</h3><div class="tbl-pills">${Array.from(a.mods).map(chipMod).join('') || '—'}</div>`;
  const toApi = a.eps.length && hasView('api') ? viewBtn('api', t('dsToApi'), a.id) : '';
  if (a.mods.size || toApi) h += `<div class="btnrow">${a.mods.size ? `<button class="pbtn primary" data-act="intosys">${esc(t('into'))}</button>` : ''}${toApi}</div>`;
  return h;
}

let QUIET = false; // reapplying marks after a live update: no panel redraw, no select event
function setSel(sel) {
  state.sel = sel || null;
  if (QUIET) return;
  PNL.closed = false; // a new selection shows its details again
  renderPanel();
  bus.emit('select', state.sel);
}
const chipMod = (m) => `<span class="chip" data-mod="${esc(m)}" style="cursor:pointer" title="${esc(modLabel(m))}"><i style="background:${LCOLOR[layerOf(m)]}"></i>${esc(cutL(short(m), 34))}</span>`;
function relList(ids, title, dir, of) {
  if (!ids.length) return '';
  return `<h3>${esc(title)} (${ids.length})</h3><ul class="rel-list">` + ids.slice(0, 14).map((k) => {
    const f = fnInfo(k), q = dir === 'in' ? isAsync(k, of) : isAsync(of, k);
    const st = f.status ? `<span class="st-${f.status === 'added' ? 'a' : 'm'}">${f.status === 'added' ? '+' : '~'}</span>` : '';
    return `<li data-fn="${esc(k)}" title="${esc(k)}">${st}<span class="fn">${esc(f.name)}</span>${q ? `<span class="q">${esc(t('queue'))}</span>` : ''}<span class="md">${esc(short(f.mod))}</span></li>`;
  }).join('') + (ids.length > 14 ? `<li class="md">+${ids.length - 14}</li>` : '') + '</ul>';
}
function fnSection(id, target, opt = {}) {
  const f = fnInfo(id);
  let h = `<div class="kicker">${f.mod ? chipMod(f.mod) : ''}${f.status ? `<span class="chip ${f.status === 'added' ? 'a' : 'm'}">${esc(t('st_' + f.status))}${f.uncommitted ? ' · ' + esc(t('uncommitted')) : ''}</span>` : ''}${f.entry ? `<span class="chip" title="${esc((f.entry.labels || []).join(', '))}">⚑ ${esc(f.entry.kind)}</span>` : ''}</div>`;
  h += `<h2 class="mono">${esc(f.name)}</h2>`;
  const fd = f.full ? fnDesc(id) : '';
  if (fd) h += `<p class="lead">${esc(fd)}</p>`;
  if (f.sig) h += `<h3>${esc(t('sig'))}</h3><div class="sig">${hlLine(f.sig, f.lang)}</div>`;
  if (f.file) h += `<div class="loc">${esc(f.file)}:${f.s}–${f.e}</div>`;
  const eps = D.epByFid[id] || [];
  if (eps.length) h += `<div class="btnrow">${eps.slice(0, 3).map((e) => `<span class="chip" title="${esc(e.desc || '')}"><span class="verb">${esc(e.method || e.kind)}</span>${esc(cut(e.path || e.name || '', 40))}</span>`).join('')}</div>`;
  if (eps.some((e) => e.desc && e.desc !== fd)) h += `<p class="loc" style="font-family:inherit">${eps.filter((e) => e.desc && e.desc !== fd).slice(0, 3).map((e) => esc(t('dsEpOf')) + ': ' + esc(e.desc)).join('<br>')}</p>`;
  if (!opt.noRel && D.fns[id]) h += flowList(flowsThrough((s) => s === id), t('dsThrough'), 8);
  if (!opt.noRel && D.fns[id]) { h += tableChips([id]); const sb = statusBtns([id]); if (sb) h += `<div class="btnrow">${sb}</div>`; }
  if (!opt.noRel) h += relList(f.in, t('callers'), 'in', id) + relList(f.out, t('callees'), 'out', id);
  const c = codeHTML(id, target);
  h += `<h3>${esc(t('code'))}</h3><div class="code"${opt.codeId ? ` id="${opt.codeId}"` : ''}>${c.html}</div>`;
  return { h, hlIdx: c.hlIdx, loading: c.loading };
}
// no selection: no panel (the overview is never shown on its own)
function overviewHTML() { return ''; }
function stepHTML() {
  const s = curStep(); if (!s) return { h: overviewHTML() };
  if (s.planned) {
    const from = s.prev ? fnInfo(s.prev).name : '—';
    let h = `<div class="kicker">${esc(t('stepN'))} ${s.i + 1} · ${esc(cut(state.run.name, 40))} <span class="chip">⌁ ${esc(t('planTag'))}</span></div><h2>${esc(s.label)}</h2>`;
    h += `<p class="lead">${esc(from)} → ⌁ ${esc(t('planStep'))}${s.mod ? ' · ' + esc(t('planIn')) + ' ' + esc(modLabel(s.mod)) : ''}</p><p class="loc">${esc(t('planNoCode'))}</p>`;
    if (s.mod && D.mods[s.mod]) h += `<div class="btnrow">${chipMod(s.mod)}</div>`;
    if (s.exts.length) h += `<div class="btnrow" style="margin:6px 0">${s.exts.map((x) => `<span class="chip">↗ ${esc(x.name)}</span>`).join('')}</div>`;
    return { h, hlIdx: -1, loading: false, fids: s.prev ? [s.prev] : [] };
  }
  const f = fnInfo(s.fid), from = s.prev ? fnInfo(s.prev).name : s.actor ? actorName(s.actor) : '—';
  let h = `<div class="kicker">${esc(t('stepN'))} ${s.i + 1} · ${esc(cut(state.run.name, 40))}</div><h2>${esc(s.label)}</h2>
    <p class="lead">${esc(from)} → ${esc(f.name)} · <span class="mono" style="font-size:11.5px">${esc(s.kind === 'async' ? t('viaQueue') : s.kind)}</span></p>`;
  const bd = effBadges(s).map(mutBadge).join('') + s.states.map(statusPill).join('');
  if (bd) h += `<div class="btnrow" style="margin:0 0 6px">${bd}</div>`;
  if (s.exts.length) h += `<div class="btnrow" style="margin:6px 0">${s.exts.map((x) => `<span class="chip">↗ ${esc(x.name)}</span>`).join('')}</div>`;
  let hlIdx = -1, loading = false;
  if (s.prev && isCall(s.prev, s.fid)) {
    const c = codeHTML(s.prev, s.fid);
    h += `<h3>${esc(t('callSite'))} · ${esc(fnInfo(s.prev).name)}</h3><div class="code" id="pcode">${c.html}</div>`;
    hlIdx = c.hlIdx; loading = c.loading;
    h += `<h3 style="margin-top:22px">${esc(t('callee'))}</h3>` + fnSection(s.fid, s.next).h;
  } else { const sec = fnSection(s.fid, s.next, { codeId: 'pcode' }); h += sec.h; hlIdx = sec.hlIdx; loading = sec.loading; }
  return { h, hlIdx, loading, fids: [s.prev, s.fid] };
}
function renderPanel() {
  const p = $('#pane'), s = state.sel;
  let h = '', hlIdx = -1, loading = false, after = null, fids = [];
  if (s && PANELS[s.t]) {
    const r = PANELS[s.t](s, AM);
    if (r && typeof r === 'object') { h = r.html || ''; after = r.after; } else h = r || '';
  } else if (!s) h = overviewHTML();
  else if (s.t === 'step') { const r = stepHTML(); h = r.h; hlIdx = r.hlIdx; loading = r.loading; fids = r.fids || []; }
  else if (s.t === 'fn') {
    const sec = fnSection(s.id, null, { codeId: 'pcode' }); loading = sec.loading; fids = [s.id];
    h = (s.why ? `<div class="kicker">${esc(t('where'))}: ${esc(s.why)}</div>` : '') + sec.h;
    h += `<div class="btnrow"><button class="pbtn" data-act="code">${esc(t('into'))} · ${esc(t('lv')[3])}</button>${D.fns[s.id] && fnInfo(s.id).out.length ? `<button class="pbtn" data-act="playfn">${esc(t('playFromFn'))}</button>` : ''}</div>`;
  } else if (s.t === 'mod') {
    const m = s.id, c = D.modChg[m], imps = ((D.mods[m] || {}).imports || []).filter((x) => D.mods[x]), by = D.importedBy[m] || [];
    const inRun = runList().map((x, i) => [x, i]).filter(([x]) => (D.fns[x.fid] || {}).module === m);
    h = `<div class="kicker"><span class="chip"><i style="background:${LCOLOR[layerOf(m)]}"></i>${esc(t('layers')[layerOf(m)])}</span></div><h2 class="mono">${esc(modLabel(m))}</h2>`;
    const d = modDesc(m); h += `<p class="lead">${esc(d || modRole(m))}</p>`;
    h += `<dl class="kv"><dt>${esc(t('fns'))}</dt><dd>${(D.byMod[m] || []).length}</dd><dt>files</dt><dd>${((D.mods[m] || {}).files || []).length}</dd><dt>${esc(t('imports'))}</dt><dd>${imps.length}</dd><dt>${esc(t('importedBy'))}</dt><dd>${by.length}</dd>${hasChg(c) ? `<dt>${esc(t('st_modified'))}</dt><dd>${esc(chgText(c))}</dd>` : ''}</dl>`;
    if (D.extByMod[m]) h += `<div class="btnrow">${D.extByMod[m].map((x) => `<span class="chip">↗ ${esc(x.name)}</span>`).join('')}</div>`;
    h += `<h3>${esc(t('imports'))}</h3><div class="tbl-pills">${imps.slice(0, 30).map(chipMod).join('') || '—'}</div>`;
    if (by.length) h += `<h3>${esc(t('importedBy'))}</h3><div class="tbl-pills">${by.slice(0, 30).map(chipMod).join('')}</div>`;
    if (inRun.length) h += `<h3>${esc(t('inScen'))}</h3><ul class="rel-list">${inRun.slice(0, 12).map(([x, i]) => `<li data-step="${i}"><span class="fn">${i + 1}.</span><span>${esc(stepLabel(i))}</span></li>`).join('')}</ul>`;
    h += flowList(flowsThrough((s) => D.fns[s].module === m), t('dsThrough'));
    h += tableChips(D.byMod[m] || []);
    h += `<div class="btnrow"><button class="pbtn primary" data-act="intomod">${esc(t('into'))}</button>${statusBtns(D.byMod[m] || [])}</div>`;
  } else if (s.t === 'group') {
    const n = G.TN[s.id]; if (!n) { state.sel = null; return renderPanel(); }
    const c = chgOf(n.mods), info = modelFor(n.path);
    h = `<div class="kicker">${esc(t('group'))}${info ? ` · ${esc(t('layers')[info.layer] || '')}` : ''}</div><h2 class="mono">${esc((n.path || t('root')) + '/')}</h2>`;
    if (info && info.desc) h += `<p class="lead">${esc(info.desc)}</p>`;
    h += `<dl class="kv"><dt>${esc(t('modules'))}</dt><dd>${n.mods.length}</dd><dt>${esc(t('fns'))}</dt><dd>${n.n}</dd>${hasChg(c) ? `<dt>${esc(t('st_modified'))}</dt><dd>${esc(chgText(c))}</dd>` : ''}</dl>`;
    h += `<h3>${esc(t('modules'))}</h3><div class="tbl-pills">${n.mods.slice(0, 40).map(chipMod).join('')}${n.mods.length > 40 ? ` +${n.mods.length - 40}` : ''}</div>`;
    h += `<div class="btnrow">${isOpen(n) ? '' : `<button class="pbtn primary" data-act="expand">${esc(t('expand'))}</button>`}${G.userOpen.has(n.id) ? `<button class="pbtn" data-act="collapse">${esc(t('collapse'))}</button>` : ''}</div>`;
  } else if (s.t === 'svc') {
    const PJ = D.F.project || {};
    h = `<div class="kicker">${esc(t('lv')[0])}</div><h2>${esc(PJ.name || D.F.repo || '')}</h2><p class="lead">${esc(PJ.description || D.M.summary || '')}</p>`;
    h += `<dl class="kv"><dt>${esc(t('modules'))}</dt><dd>${D.allMods.length}</dd><dt>${esc(t('fns'))}</dt><dd>${D.fids.length}</dd><dt>${esc(t('entries'))}</dt><dd>${D.entries.length}</dd>${Object.keys(D.tables).length ? `<dt>${esc(t('tables'))}</dt><dd>${Object.keys(D.tables).length}</dd>` : ''}</dl>`;
    h += `<h3>${esc(t('layer'))}</h3><div class="tbl-pills">${LAYERS.filter((l) => D.allMods.some((m) => layerOf(m) === l)).map((l) => `<span class="chip"><i style="background:${LCOLOR[l]}"></i>${esc(t('layers')[l])} · ${D.allMods.filter((m) => layerOf(m) === l).length}</span>`).join('')}</div>`;
    h += `<div class="btnrow"><button class="pbtn primary" data-act="intosvc">${esc(t('into'))}</button></div>`;
  } else if (s.t === 'actor') {
    const a = D.actors.find((x) => x.id === s.id); if (!a) { state.sel = null; return renderPanel(); }
    h = actorHTML(a);
  } else if (s.t === 'ext') {
    const x = D.externals.find((q) => q.id === s.id); if (!x) { state.sel = null; return renderPanel(); }
    h = `<div class="kicker">${esc(t('kinds')[x.kind] || x.kind)}</div><h2>${esc(x.name)}</h2>`;
    if (extDesc(x)) h += `<p class="lead">${esc(extDesc(x))}</p>`;
    const e = sys && sys.edges.find((q) => q.b === x.id);
    h += `<dl class="kv">${e && e.text ? `<dt>${esc(t('link'))}</dt><dd>${esc(e.text)}</dd>` : ''}<dt>${esc(t('implBy'))}</dt><dd>${(x.modules || []).map(chipMod).join(' ') || '—'}</dd></dl>`;
    h += relList((x.functions || []).filter((f) => D.fns[f]), t('callers'), 'in', '');
    const used = runList().map((it, i) => [it, i]).filter(([it]) => extsOfFn(it.fid).includes(x));
    if (used.length) h += `<h3>${esc(t('inScen'))}</h3><ul class="rel-list">${used.slice(0, 10).map(([, i]) => `<li data-step="${i}"><span class="fn">${i + 1}.</span><span>${esc(stepLabel(i))}</span></li>`).join('')}</ul>`;
    const toData = x.kind === 'db' && Object.keys(D.tables).length && VIEWS.some((v) => v.id === 'data');
    if ((x.modules || []).length || toData) h += `<div class="btnrow">${(x.modules || []).length ? `<button class="pbtn primary" data-act="intosys">${esc(t('into'))}</button>` : ''}${toData ? `<button class="pbtn" data-act="todata">${esc(t('dsToData'))}</button>` : ''}</div>`;
  }
  p.innerHTML = h; p.classList.remove('fadein'); void p.offsetWidth; p.classList.add('fadein');
  paintPanelShown();
  wirePanel(p, s);
  const pc = $('#pcode');
  if (pc && hlIdx >= 0) requestAnimationFrame(() => { const ln = pc.children[hlIdx]; if (ln) pc.scrollTop = ln.offsetTop - pc.clientHeight / 2 + 10; });
  if (loading) {
    const sel = state.sel;
    Promise.all(fids.filter(Boolean).map(loadSrc)).then(() => { if (state.sel === sel) renderPanel(); });
  }
  if (after) after(p, AM);
}
function wirePanel(p, s) {
  p.querySelectorAll('[data-fn]').forEach((e) => e.addEventListener('click', () => openFn(e.getAttribute('data-fn'))));
  p.querySelectorAll('[data-mod]').forEach((e) => e.addEventListener('click', () => openMod(e.getAttribute('data-mod'))));
  p.querySelectorAll('[data-step]').forEach((e) => e.addEventListener('click', () => AM.play.go(+e.getAttribute('data-step'))));
  p.querySelectorAll('[data-run]').forEach((e) => e.addEventListener('click', () => { if (state.view !== 'map') setView('map'); openRun(e.getAttribute('data-run')); }));
  const act = (a, f) => { const b = p.querySelector(`[data-act="${a}"]`); if (b) b.onclick = f; };
  act('code', () => jumpToFn(s.id, 3));
  act('playfn', () => playFrom(s.id));
  act('intomod', () => goModule(s.id));
  act('todata', () => setView('data'));
  // a card leads to a view: API (an actor), Data (a table), Statuses (a machine)
  p.querySelectorAll('[data-view]').forEach((e) => e.addEventListener('click', () => {
    const v = e.getAttribute('data-view'), arg = e.getAttribute('data-arg');
    if (v === 'api') setView('api');
    else if (v === 'table') { setView('data'); bus.emit('data:focus', { table: arg }); }
    else if (v === 'status') { setView('status'); bus.emit('status:focus', { machine: arg }); }
  }));
  act('intosvc', () => { setView('map'); drill(1, { from: sys.nodes.svc.el }); });
  act('intosys', () => { setView('map'); if (state.level === 0) drillFromSys(s.id); else intoSelected(); });
  act('expand', () => expandGroup(G.TN[s.id]));
  act('collapse', () => collapseGroup(G.TN[s.id]));
}
function openFn(id) {
  if (!D.fns[id]) return;
  if (state.view === 'map' && state.level === 2 && fv && fv.nodes[id]) return selectFn(id);
  setSel({ t: 'fn', id });
}
function openMod(m) {
  if (state.view === 'map' && state.level === 1) { revealMod(m); const u = unitFor(m); if (u) { selectMod(u); map.cam.animTo(map.pos[u], 300, 0, 300); return; } }
  setSel({ t: 'mod', id: m });
}

/* ---------- showing, hiding and resizing the details panel ----------
 * The panel floats over the right of the stage and is there only while something is selected: no selection — no
 * panel. ✕ (or `]`, Cmd/Ctrl+.) hides it until the next selection. The width is
 * dragged by the left edge (arrows nudge it, double-click resets it) and kept in localStorage (archmap.panel.w). */
addI18n({
  ru: { pnHide: 'Закрыть (] или ⌘.)', pnGrip: 'Потяните, чтобы изменить ширину; двойной клик — по умолчанию' },
  en: { pnHide: 'Close (] or ⌘.)', pnGrip: 'Drag to resize; double-click for the default' },
});
const PNL = { closed: false, w: 0 };
const pnStore = (k, v) => { try { if (v == null) localStorage.removeItem(k); else localStorage.setItem(k, v); } catch (e) { /* private mode */ } };
const pnLoad = (k) => { try { return localStorage.getItem(k); } catch (e) { return null; } };
const pnMaxW = () => Math.max(260, Math.min(720, Math.round(window.innerWidth * 0.6)));
const pnHasContent = () => !!$('#pane').childElementCount;
// the panel is shown when there are details and the user did not close it
function paintPanelShown() { $('main').classList.toggle('nopanel', PNL.closed || !pnHasContent()); }
function paintPanelBtn() {
  $('#panelHide').title = t('pnHide'); $('#panelHide').setAttribute('aria-label', t('pnHide'));
  $('#panelGrip').title = t('pnGrip'); $('#panelGrip').setAttribute('aria-label', t('pnGrip'));
}
function setPanelHidden(h) {
  PNL.closed = !!h;
  paintPanelShown();
}
function setPanelWidth(w, save) {
  PNL.w = w ? Math.max(260, Math.min(pnMaxW(), Math.round(w))) : 0;
  if (PNL.w) $('main').style.setProperty('--pw', PNL.w + 'px'); else $('main').style.removeProperty('--pw');
  if (save) pnStore('archmap.panel.w', PNL.w ? String(PNL.w) : null);
}
function setupSidePanel() {
  const w = +pnLoad('archmap.panel.w');
  if (w) setPanelWidth(w);
  $('#panelHide').onclick = () => setPanelHidden(true);
  window.addEventListener('keydown', (e) => {
    const tg = e.target.tagName;
    if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT' || e.target.isContentEditable) return;
    if ((e.key === ']' && !e.metaKey && !e.ctrlKey && !e.altKey) || (e.key === '.' && (e.metaKey || e.ctrlKey))) { e.preventDefault(); setPanelHidden(!PNL.closed); }
  });
  const grip = $('#panelGrip');
  grip.addEventListener('pointerdown', (e) => {
    if (e.button !== 0) return;
    e.preventDefault();
    const x0 = e.clientX, w0 = $('#panel').getBoundingClientRect().width;
    grip.classList.add('drag'); $('main').classList.add('presize');
    try { grip.setPointerCapture(e.pointerId); } catch (er) { /* synthetic */ }
    const mv = (ev) => setPanelWidth(w0 + (x0 - ev.clientX));
    const up = () => { grip.removeEventListener('pointermove', mv); grip.removeEventListener('pointerup', up); grip.removeEventListener('pointercancel', up); grip.classList.remove('drag'); $('main').classList.remove('presize'); setPanelWidth(PNL.w, true); };
    grip.addEventListener('pointermove', mv); grip.addEventListener('pointerup', up); grip.addEventListener('pointercancel', up);
  });
  grip.addEventListener('dblclick', () => setPanelWidth(0, true));
  grip.addEventListener('keydown', (e) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    e.preventDefault(); e.stopPropagation();
    setPanelWidth($('#panel').getBoundingClientRect().width + (e.key === 'ArrowLeft' ? 24 : -24), true);
  });
  paintPanelBtn(); paintPanelShown();
  bus.on('lang', paintPanelBtn);
}

Object.assign(AM, { select: setSel, openFn, openMod, fnSection, renderPanel, panel: { setHidden: setPanelHidden, setWidth: setPanelWidth, get hidden() { return $('main').classList.contains('nopanel'); }, get width() { return PNL.w; } } });
