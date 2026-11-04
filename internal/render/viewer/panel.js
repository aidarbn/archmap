/* panel.js — the right card for the current selection.
 * Selection kinds of the core: null (overview), {t:'svc'}, {t:'actor', id}, {t:'ext', id},
 * {t:'mod', id}, {t:'group', id}, {t:'fn', id, why?}, {t:'step'}. Views add kinds with
 * registerPanel(kind, render) and select them with AM.select({t: kind, …}). */

let QUIET = false; // reapplying marks after a live update: no panel redraw, no select event
function setSel(sel) {
  state.sel = sel || null;
  if (QUIET) return;
  renderPanel();
  if (state.view === 'map') renderLevelBar();
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
  if (f.doc) h += `<p class="lead">${esc(f.doc)}</p>`;
  if (f.sig) h += `<h3>${esc(t('sig'))}</h3><div class="sig">${hlLine(f.sig, f.lang)}</div>`;
  if (f.file) h += `<div class="loc">${esc(f.file)}:${f.s}–${f.e}</div>`;
  const eps = D.epByFid[id] || [];
  if (eps.length) h += `<div class="btnrow">${eps.slice(0, 3).map((e) => `<span class="chip"><span class="verb">${esc(e.method || e.kind)}</span>${esc(cut(e.path || e.name || '', 40))}</span>`).join('')}</div>`;
  if (!opt.noRel) h += relList(f.in, t('callers'), 'in', id) + relList(f.out, t('callees'), 'out', id);
  const c = codeHTML(id, target);
  h += `<h3>${esc(t('code'))}</h3><div class="code"${opt.codeId ? ` id="${opt.codeId}"` : ''}>${c.html}</div>`;
  return { h, hlIdx: c.hlIdx, loading: c.loading };
}
function overviewHTML() {
  const PJ = D.F.project || {};
  let h = `<div class="kicker">${esc(t('lv')[state.level])}</div><h2>${esc(PJ.name || D.M.title || D.F.repo || '')}</h2>`;
  h += `<p class="lead">${esc(PJ.description || D.M.summary || '')}</p><p class="lead" style="margin-top:-6px">${esc(t('overviewMap'))}</p>`;
  h += `<dl class="kv"><dt>repo</dt><dd class="mono">${esc(D.F.repo || '')} @ ${esc(D.F.head || '')}</dd>` +
    `<dt>${esc(t('branch'))}</dt><dd class="mono">${esc(D.F.branch || '')}${D.F.base ? ' vs ' + esc(D.F.base) : ''}</dd>` +
    `<dt>${esc(t('modules'))}</dt><dd>${D.allMods.length}</dd><dt>${esc(t('fns'))}</dt><dd>${D.fids.length}</dd><dt>${esc(t('entries'))}</dt><dd>${D.entries.length}</dd></dl>`;
  if (state.level === 1 && D.roots.size) h += `<p class="loc" style="margin-top:14px">${esc(t('rootsHidden', Array.from(D.roots).join(', ')))}</p>`;
  if (state.run) h += `<h3>${esc(t('grpFlows'))}</h3><p style="margin:0">${esc(state.run.name)}</p>`;
  h += `<p class="loc">${esc(t('clickHint'))}</p>`;
  return h;
}
function stepHTML() {
  const s = curStep(); if (!s) return { h: overviewHTML() };
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
    const inRun = runList().map((x, i) => [x, i]).filter(([x]) => D.fns[x.fid].module === m);
    h = `<div class="kicker"><span class="chip"><i style="background:${LCOLOR[layerOf(m)]}"></i>${esc(t('layers')[layerOf(m)])}</span></div><h2 class="mono">${esc(modLabel(m))}</h2>`;
    const d = modDesc(m); h += `<p class="lead">${esc(d || modRole(m))}</p>`;
    h += `<dl class="kv"><dt>${esc(t('fns'))}</dt><dd>${(D.byMod[m] || []).length}</dd><dt>files</dt><dd>${((D.mods[m] || {}).files || []).length}</dd><dt>${esc(t('imports'))}</dt><dd>${imps.length}</dd><dt>${esc(t('importedBy'))}</dt><dd>${by.length}</dd>${hasChg(c) ? `<dt>${esc(t('st_modified'))}</dt><dd>${esc(chgText(c))}</dd>` : ''}</dl>`;
    if (D.extByMod[m]) h += `<div class="btnrow">${D.extByMod[m].map((x) => `<span class="chip">↗ ${esc(x.name)}</span>`).join('')}</div>`;
    h += `<h3>${esc(t('imports'))}</h3><div class="tbl-pills">${imps.slice(0, 30).map(chipMod).join('') || '—'}</div>`;
    if (by.length) h += `<h3>${esc(t('importedBy'))}</h3><div class="tbl-pills">${by.slice(0, 30).map(chipMod).join('')}</div>`;
    if (inRun.length) h += `<h3>${esc(t('inScen'))}</h3><ul class="rel-list">${inRun.slice(0, 12).map(([x, i]) => `<li data-step="${i}"><span class="fn">${i + 1}.</span><span>${esc(stepLabel(i))}</span></li>`).join('')}</ul>`;
    h += `<div class="btnrow"><button class="pbtn primary" data-act="intomod">${esc(t('into'))}</button></div>`;
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
    h = `<div class="kicker">${esc(t('kinds').actor)}</div><h2>${esc(actorName(a))}</h2><p class="lead">${esc(actorSub(a))}</p>`;
    const e = sys && sys.edges.find((q) => q.a === a.id);
    if (e && e.text) h += `<dl class="kv"><dt>${esc(t('link'))}</dt><dd>${esc(e.text)}</dd></dl>`;
    if (a.eps.length) h += `<h3>${esc(t('epsN', a.eps.length))}</h3><ul class="rel-list">${a.eps.slice(0, 14).map((ep) => `<li${ep.fid ? ` data-fn="${esc(ep.fid)}"` : ''}><span class="verb">${esc(ep.method || ep.kind)}</span><span class="fn">${esc(ep.name || ep.path)}</span><span class="md">${esc(ep.path || ep.service || '')}</span></li>`).join('')}${a.eps.length > 14 ? `<li class="md">+${a.eps.length - 14}</li>` : ''}</ul>`;
    else if (a.fids.length) h += `<h3>${esc(t('entries'))}</h3>` + relList(a.fids, t('entries'), 'out', '').replace(/^<h3>.*?<\/h3>/, '');
    h += `<h3>${esc(t('modules'))}</h3><div class="tbl-pills">${Array.from(a.mods).map(chipMod).join('') || '—'}</div>`;
    if (a.mods.size) h += `<div class="btnrow"><button class="pbtn primary" data-act="intosys">${esc(t('into'))}</button></div>`;
  } else if (s.t === 'ext') {
    const x = D.externals.find((q) => q.id === s.id); if (!x) { state.sel = null; return renderPanel(); }
    h = `<div class="kicker">${esc(t('kinds')[x.kind] || x.kind)}</div><h2>${esc(x.name)}</h2>`;
    const e = sys && sys.edges.find((q) => q.b === x.id);
    h += `<dl class="kv">${e && e.text ? `<dt>${esc(t('link'))}</dt><dd>${esc(e.text)}</dd>` : ''}<dt>${esc(t('implBy'))}</dt><dd>${(x.modules || []).map(chipMod).join(' ') || '—'}</dd></dl>`;
    h += relList((x.functions || []).filter((f) => D.fns[f]), t('callers'), 'in', '');
    const used = runList().map((it, i) => [it, i]).filter(([it]) => extsOfFn(it.fid).includes(x));
    if (used.length) h += `<h3>${esc(t('inScen'))}</h3><ul class="rel-list">${used.slice(0, 10).map(([, i]) => `<li data-step="${i}"><span class="fn">${i + 1}.</span><span>${esc(stepLabel(i))}</span></li>`).join('')}</ul>`;
    if ((x.modules || []).length) h += `<div class="btnrow"><button class="pbtn primary" data-act="intosys">${esc(t('into'))}</button></div>`;
  }
  p.innerHTML = h; p.classList.remove('fadein'); void p.offsetWidth; p.classList.add('fadein');
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
  const act = (a, f) => { const b = p.querySelector(`[data-act="${a}"]`); if (b) b.onclick = f; };
  act('code', () => jumpToFn(s.id, 3));
  act('playfn', () => playFrom(s.id));
  act('intomod', () => goModule(s.id));
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

Object.assign(AM, { select: setSel, openFn, openMod, fnSection, renderPanel });
