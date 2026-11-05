/* views/pages.js — the pages of arch/pages/*.arch (like pages in Figma): a canvas of frames, each frame one diagram
 * drawn from the code by the same code as the views — a journey / flow (the participants of the scenario in layers,
 * overview → steps, steps from AM.play.preview), a state machine (AM.status.draw, views/status.js), a part of the schema
 * (AM.dataER, views/data.js), endpoints (a compact list of facts.endpoints), a module with its functions, a note.
 * The page file only picks and orders the diagrams (model.spec.pages: frames with refs resolved by archmap, `layout`
 * the grid slots: row, col, span — internal/spec LayoutPage); it never changes what a diagram shows.
 *
 * The canvas pans and zooms like the map (Cam). A click on a frame shows it in the inspector ({t:'frame'}), a click on an
 * element inside — that element (status, table, endpoint, function); a double click opens the diagram on the whole canvas
 * (the scenario on the map, the Statuses / Data / API view, the module on the map) and Esc comes back to the page.
 * Stepping through the scenario of a journey / flow frame (the frame's buttons, ← / →, Home — the overview) lights on the
 * other frames what the step touches, from the code only: the tables of its data access (AM.dataER.stepTables), the status
 * transitions of arch:state / the code (AM.status.hot), the endpoint the scenario entered through, the step's function. */
const A = AM, D = A.D, esc = A.esc, sv = A.svg;

A.addI18n({
  ru: {
    pvTitle: 'Страница', pvNone: 'Страница не найдена', pvEmpty: 'На странице нет рамок',
    pvKind: { journey: 'Путь', flow: 'Сценарий', states: 'Статусы', tables: 'Данные', api: 'API', module: 'Модуль', note: 'Заметка' },
    pvMissing: 'нет в коде', pvOverview: 'Обзор', pvPlay: '▶ По шагам', pvStepOf: (k, n) => k + '/' + n, pvSteps: (n) => n + ' шаг.',
    pvActors: 'Пользователи', pvExt: 'Внешние', pvMore: (n) => 'ещё ' + n, pvNoSteps: 'нет шагов в коде', pvNoDesc: 'без описания',
    pvFrame: 'Рамка', pvFrames: 'Рамки', pvFrom: 'Файл', pvDbl: 'Двойной клик — открыть диаграмму на весь холст, Esc — назад к странице.',
    pvOpen: 'Открыть на весь холст', pvStepIt: 'Пройти по шагам', pvStep: 'Шаг', pvFlow: 'Сценарий', pvTouched: 'На этом шаге', pvTables: 'Таблицы', pvStatuses: 'Статусы', pvEntry: 'Вход',
    pvDrawn: { journey: 'путь из arch:journey / arch/*.arch, шаги — по графу вызовов', flow: 'сценарий из arch:flow / arch/*.arch, шаги — по графу вызовов', states: 'машины состояний из кода (arch:state, присваивания статусов)', tables: 'таблицы схемы и связи между ними', api: 'ручки из proto / роутеров с описаниями из кода', module: 'модуль и его функции', note: 'текст из файла страницы' },
    pvKeys: '← / → — шаги, Home — обзор', pvBroken: 'Ссылки на то, чего нет в коде', pvFit: 'Вписать', pvZIn: 'Приблизить', pvZOut: 'Отдалить',
    peServeOnly: 'Правка страниц доступна в archmap serve', peEmptyHint: 'Перетащите сюда путь, статусы, таблицы, ручки или модуль из навигации',
    peEmptyOpen: 'Показать навигацию «Система»', peEmptyStatic: 'Добавлять рамки можно в archmap serve',
    peAlready: 'Уже на странице', peConflict: 'Файл страницы изменён на диске — показана новая версия, повторите действие',
    peNoConn: 'Нет связи с archmap serve', peBroken: 'Нельзя добавить', peUndone: 'Отменено', peRedone: 'Повторено', peNoUndo: 'Нечего отменять', peNoRedo: 'Нечего повторять',
    peDelFrame: 'Убрать рамку со страницы', peNote: 'Заметка', peAddNote: '+ Заметка', peNotePh: 'Текст заметки', peNoteKeys: '⌘/Ctrl+Enter — сохранить, Esc — отмена; пустой текст убирает заметку',
    peEditNote: 'Изменить текст', peDropAdd: (n) => 'Добавить на страницу' + (n > 1 ? ' (' + n + ')' : ''), peDropNo: 'Нельзя',
    peNoPage: 'Откройте страницу, чтобы добавить на неё', peUndoHint: (/Mac|iPhone|iPad/.test(navigator.platform || '') ? '⌘Z' : 'Ctrl+Z') + ' — отменить', peRemoved: 'Рамка убрана', peRefRemoved: (r) => r + ' — убрано со страницы',
  },
  en: {
    pvTitle: 'Page', pvNone: 'No such page', pvEmpty: 'The page has no frames',
    pvKind: { journey: 'Journey', flow: 'Scenario', states: 'Statuses', tables: 'Data', api: 'API', module: 'Module', note: 'Note' },
    pvMissing: 'not in the code', pvOverview: 'Overview', pvPlay: '▶ Step through', pvStepOf: (k, n) => k + '/' + n, pvSteps: (n) => n + (n === 1 ? ' step' : ' steps'),
    pvActors: 'Users', pvExt: 'Outside', pvMore: (n) => n + ' more', pvNoSteps: 'no steps in the code', pvNoDesc: 'no description',
    pvFrame: 'Frame', pvFrames: 'Frames', pvFrom: 'File', pvDbl: 'Double click — open the diagram on the whole canvas, Esc — back to the page.',
    pvOpen: 'Open on the whole canvas', pvStepIt: 'Step through', pvStep: 'Step', pvFlow: 'Scenario', pvTouched: 'On this step', pvTables: 'Tables', pvStatuses: 'Statuses', pvEntry: 'Entry',
    pvDrawn: { journey: 'a journey of arch:journey / arch/*.arch, steps by the call graph', flow: 'a scenario of arch:flow / arch/*.arch, steps by the call graph', states: 'state machines of the code (arch:state, status assignments)', tables: 'schema tables and the references between them', api: 'endpoints of proto / routers with their descriptions from the code', module: 'the module and its functions', note: 'text of the page file' },
    pvKeys: '← / → — steps, Home — overview', pvBroken: 'Names something the code does not have', pvFit: 'Fit', pvZIn: 'Zoom in', pvZOut: 'Zoom out',
    peServeOnly: 'Pages are edited in archmap serve', peEmptyHint: 'Drag a path, statuses, tables, endpoints or a module here from the navigation',
    peEmptyOpen: 'Show the System navigation', peEmptyStatic: 'Frames are added in archmap serve',
    peAlready: 'Already on the page', peConflict: 'The page file changed on disk — showing the new version, try again',
    peNoConn: 'No connection to archmap serve', peBroken: 'Cannot add', peUndone: 'Undone', peRedone: 'Redone', peNoUndo: 'Nothing to undo', peNoRedo: 'Nothing to redo',
    peDelFrame: 'Take the frame off the page', peNote: 'Note', peAddNote: '+ Note', peNotePh: 'Text of the note', peNoteKeys: '⌘/Ctrl+Enter — save, Esc — cancel; empty text removes the note',
    peEditNote: 'Edit the text', peDropAdd: (n) => 'Add to the page' + (n > 1 ? ' (' + n + ')' : ''), peDropNo: 'Cannot',
    peNoPage: 'Open a page to add to it', peUndoHint: (/Mac|iPhone|iPad/.test(navigator.platform || '') ? '⌘Z' : 'Ctrl+Z') + ' — undo', peRemoved: 'Frame taken off', peRefRemoved: (r) => r + ' taken off the page',
  },
});
const tr = (k, ...a) => A.t(k, ...a);

/* ---------- state ---------- */
// cur: id of the page on the canvas; ret: the page a frame was opened from on the whole canvas (Esc comes back);
// play: the scenario of a frame being stepped {fi, steps, run, i} (i = -1 — the overview)
const P = { cur: null, ret: null, play: null, built: null, frames: [], stepFids: new Set(), extent: null, el: null, svg: null, cam: null, root: null };
const pagesOf = () => (D.M && D.M.spec && D.M.spec.pages) || [];
const pageOf = (id) => pagesOf().find((p) => p.id === id) || null;
const curPage = () => pageOf(P.cur);

/* ---------- geometry (world units) ---------- */
const COL = 560, GAP = 56, ROWGAP = 92, PAD = 20, HEAD = 38, FOOT = 34, MAXH = 980;
const frameW = (span) => span * COL + (span - 1) * GAP;
// slots of the page grid: the ones archmap computed (spec.LayoutPage), else the same rule here
function slotsOf(pg) {
  if (pg.layout && pg.layout.length === pg.frames.length) return pg.layout;
  const rank = (k) => (k === 'journey' || k === 'flow' ? 0 : k === 'states' ? 1 : 2), out = []; let row = 0, col = 0;
  [0, 1, 2].forEach((r) => pg.frames.forEach((f, i) => { if (rank(f.kind) !== r) return; const s = r < 2 ? 2 : 1; if (col > 0 && col + s > 4) { row++; col = 0; } out.push({ frame: i, row, col, span: s }); col += s; }));
  return out;
}

/* ---------- text helpers ---------- */
const CH = 7.1; // average width of a 13 px character
function wrap(s, width, px, maxLines) {
  const per = Math.max(8, Math.floor(width / (px / 13 * CH))), out = [];
  let line = '';
  String(s || '').split(/\s+/).filter(Boolean).forEach((w) => {
    if ((line + ' ' + w).trim().length > per) { if (line) out.push(line); line = w; } else line = (line + ' ' + w).trim();
  });
  if (line) out.push(line);
  if (maxLines && out.length > maxLines) { out.length = maxLines; out[maxLines - 1] = A.cut(out[maxLines - 1], per - 1) + (out[maxLines - 1].endsWith('…') ? '' : '…'); }
  return out;
}
function textLines(g, x, y, lines, cls, lh) { lines.forEach((l, i) => sv('text', { x, y: y + i * lh, class: cls }, g, l)); return lines.length * lh; }
const firstLine = (s) => String(s || '').split('\n').map((x) => x.trim()).filter(Boolean)[0] || '';

/* ---------- frames ---------- */
// a frame: {i, kind, refs, title, label, w, h (content, world), draw(g) → painter, paint(step), missing}
function makeFrame(fr, i) {
  const f = { i, src: fr, kind: fr.kind, refs: fr.refs || [], missing: fr.missing || [], title: tr('pvKind')[fr.kind] || fr.kind };
  try {
    const mk = BUILD[fr.kind];
    if (mk) Object.assign(f, mk(fr, f));
  } catch (err) { console.error('archmap page frame', fr, err); f.err = String(err); }
  if (!f.draw) f.draw = () => ({});
  if (!f.w) { f.w = COL - PAD * 2; f.h = 60; }
  return f;
}
const BUILD = {
  journey: (fr) => scenarioFrame('journey:' + fr.refs[0], fr),
  flow: (fr) => { const fl = D.flowById[fr.refs[0]]; return scenarioFrame(fl ? 'flow:' + fl.i : null, fr); },
  states: statesFrame, tables: tablesFrame, api: apiFrame, module: moduleFrame, note: noteFrame,
};

/* --- a scenario: the participants in layers, the legs in the order of the steps --- */
const NW = 176, NH = 46, LG = 74, VG = 20, LANEH = 30;
const LANE_ORDER = ['actor', 'entry', 'ui', 'domain', 'adapter', 'infra', 'other', 'test', 'ext'];
function scenarioFrame(key, fr) {
  const pv = key ? A.play.preview(key) : null;
  const run = pv && pv.run, steps = pv ? pv.steps : [];
  const label = run ? run.name : fr.refs[0];
  const nodes = new Map(), legs = new Map(), stepNode = [], stepLegs = [];
  const node = (id, lane, name, sub) => { if (!nodes.has(id)) nodes.set(id, { id, lane, name, sub, first: nodes.size }); return id; };
  const leg = (a, b, i, kind) => { const k = a + '>' + b; if (!legs.has(k)) legs.set(k, { k, a, b, steps: [], kind }); const l = legs.get(k); if (!l.steps.includes(i)) l.steps.push(i); stepLegs[i].push(k); };
  steps.forEach((s, i) => {
    stepLegs[i] = [];
    const mid = s.mod ? node('m:' + s.mod, A.layerOf(s.mod), A.short(s.mod), A.t('layers')[A.layerOf(s.mod)] || '') : null;
    stepNode[i] = mid;
    if (s.actor) { const a = node('a:' + s.actor.id, 'actor', A.actorName(s.actor), tr('pvActors')); if (mid) leg(a, mid, i, 'entry'); }
    const pm = s.prev && D.fns[s.prev] ? 'm:' + D.fns[s.prev].module : null;
    if (mid && pm && pm !== mid && nodes.has(pm)) leg(pm, mid, i, s.kind === 'async' ? 'async' : 'call');
    if (mid) (s.exts || []).slice(0, 2).forEach((x) => { const xid = node('x:' + x.id, 'ext', x.name, tr('pvExt')); leg(mid, xid, i, 'ext'); });
  });
  const lanes = LANE_ORDER.filter((l) => Array.from(nodes.values()).some((n) => n.lane === l));
  const laneX = {}; lanes.forEach((l, k) => { laneX[l] = k * (NW + LG); });
  const rows = {}; Array.from(nodes.values()).sort((a, b) => a.first - b.first).forEach((n) => { const r = rows[n.lane] = (rows[n.lane] || 0) + 1; n.x = laneX[n.lane]; n.y = LANEH + (r - 1) * (NH + VG); });
  const maxRows = Math.max(1, ...Object.values(rows));
  const w = Math.max(NW, lanes.length * (NW + LG) - LG), h = LANEH + maxRows * (NH + VG) - VG;
  return {
    key, run, steps, label, w, h: steps.length ? h : 60, scenario: true, stepNode, stepLegs,
    draw(g) {
      const P_ = { nodes: {}, legs: {} };
      if (!steps.length) { sv('text', { x: 0, y: 24, class: 'pv-muted' }, g, tr('pvNoSteps')); return P_; }
      lanes.forEach((l) => sv('text', { x: laneX[l], y: 12, class: 'pv-lane' }, g, l === 'actor' ? tr('pvActors') : l === 'ext' ? tr('pvExt') : A.t('layers')[l] || l));
      const gl = sv('g', { class: 'pv-legs' }, g), gn = sv('g', { class: 'pv-nodes' }, g);
      legs.forEach((l) => {
        const a = nodes.get(l.a), b = nodes.get(l.b);
        let d, mx, my;
        if (b.x > a.x) { const x1 = a.x + NW, y1 = a.y + NH / 2, x2 = b.x - 2, y2 = b.y + NH / 2, k = Math.max(26, (x2 - x1) / 2); d = `M${x1},${y1} C${x1 + k},${y1} ${x2 - k},${y2} ${x2},${y2}`; mx = (x1 + x2) / 2; my = (y1 + y2) / 2; }
        else if (b.x < a.x) { const x1 = a.x, y1 = a.y + NH / 2 + 6, x2 = b.x + NW + 2, y2 = b.y + NH / 2 + 6, k = Math.max(26, (x1 - x2) / 2); d = `M${x1},${y1} C${x1 - k},${y1} ${x2 + k},${y2} ${x2},${y2}`; mx = (x1 + x2) / 2; my = (y1 + y2) / 2; }
        else { const x1 = a.x + NW, y1 = a.y + NH / 2, y2 = b.y + NH / 2, xr = x1 + 34; d = `M${x1},${y1} C${xr},${y1} ${xr},${y2} ${x1 + 2},${y2}`; mx = xr - 8; my = (y1 + y2) / 2; }
        const lg = sv('g', { class: 'pv-leg ' + l.kind }, gl);
        sv('path', { d, class: 'ln', 'marker-end': 'url(#arr)' }, lg);
        // several steps on one leg: "13…17", the list in the tooltip (what each step does, from the code)
        const n1 = l.steps.map((x) => x + 1), txt = n1.length > 2 ? n1[0] + '…' + n1[n1.length - 1] : n1.join(', '), tw = txt.length * 6.4 + 12;
        const pill = sv('rect', { x: mx - tw / 2, y: my - 9, width: tw, height: 18, rx: 9, class: 'pill' }, lg);
        const no = sv('text', { x: mx, y: my + 4, 'text-anchor': 'middle', class: 'no' }, lg, txt);
        sv('title', null, lg, l.steps.map((x) => tr('pvStep') + ' ' + (x + 1) + ': ' + (A.play.action(steps[x]).full.split('\n')[0] || steps[x].label) + '\n' + steps[x].label).join('\n\n'));
        lg.pv = { pill, no, txt, tw, mx }; // paintAll signs the current leg with what its step does
        P_.legs[l.k] = lg;
      });
      nodes.forEach((n) => {
        const ng = sv('g', { class: 'pv-node ' + (n.lane === 'actor' ? 'act' : n.lane === 'ext' ? 'ext' : 'mod') }, gn);
        sv('rect', { x: n.x, y: n.y, width: NW, height: NH, rx: 8, class: 'card' }, ng);
        sv('rect', { x: n.x, y: n.y + 9, width: 3, height: NH - 18, rx: 1.5, fill: A.LCOLOR[n.lane] || 'var(--edge)' }, ng);
        sv('text', { x: n.x + 14, y: n.y + 20, class: 'nm' }, ng, A.cut(n.name, 22));
        sv('text', { x: n.x + 14, y: n.y + 36, class: 'sub' }, ng, A.cut(n.sub, 26));
        sv('title', null, ng, n.id.slice(2));
        if (n.id.startsWith('m:')) ng.addEventListener('click', (e) => { if (P.cam.justDragged) return; e.stopPropagation(); A.select({ t: 'mod', id: n.id.slice(2) }); });
        P_.nodes[n.id] = ng;
      });
      return P_;
    },
  };
}

/* --- state machines (views/status.js draws them) --- */
function statesFrame(fr) {
  const ms = (fr.machines || []).map((k) => k && A.status ? A.status.machine(k) : null).filter(Boolean);
  const host = sv('g', {}), parts = [];
  let y = 0, w = 0;
  ms.forEach((m) => {
    const g = sv('g', {}, host);
    const r = A.status.draw(g, m, {
      compact: true, // a frame is small: statuses and the main path, the captions of the arrows are in the Statuses view
      nodeClick: (e, n) => { if (P.cam.justDragged) return; e.stopPropagation(); A.select({ t: 'status', entity: m.ent, id: n }); },
      trClick: (e, x, g, ed) => { if (P.cam.justDragged) return; e.stopPropagation(); A.select(A.status.sel(m, x, ed)); },
    });
    const b = r.bounds, bw = Math.max(0, b.x1 - b.x0), bh = Math.max(0, b.y1 - b.y0);
    const lab = sv('text', { x: 0, y: y + 16, class: 'pv-sub' }, host, m.ent);
    void lab;
    g.setAttribute('transform', `translate(${-b.x0},${y + 30 - b.y0})`);
    parts.push({ m, r });
    y += 30 + bh + 34; w = Math.max(w, bw);
  });
  return {
    label: (fr.refs || []).join(', '), w: Math.max(w, 300), h: Math.max(y - 34, 40), host, parts,
    draw(g) { g.appendChild(host); return { parts }; },
  };
}

/* --- tables (views/data.js draws them) --- */
function tablesFrame(fr) {
  if (!A.dataER) return { label: fr.refs.join(', ') };
  const LY = A.dataER.layout(fr.refs || []);
  return {
    label: (fr.refs || []).join(', '), w: Math.max(LY.w, 240), h: Math.max(LY.h, 40),
    draw(g) {
      const er = A.dataER.draw(g, LY, LY.names, { click: (n) => { if (P.cam.justDragged) return; A.select({ t: 'table', id: n }); } });
      Object.values(er.cards).forEach((c) => c.g.addEventListener('click', (e) => e.stopPropagation()));
      return { er };
    },
  };
}

/* --- endpoints: a compact card per endpoint, with its description from the code --- */
const AW = COL - PAD * 2;
function apiFrame(fr) {
  const idx = (fr.endpoints || []).filter((i) => i >= 0 && D.endpoints[i]);
  const items = idx.map((i) => {
    const e = D.endpoints[i], verb = (e.method || e.kind || '').toUpperCase(), path = e.path || e.name || '';
    const desc = wrap(e.desc || A.fnDesc(e.fid) || '', AW - 28, 12.5, 3);
    return { i, e, verb, path, desc, h: 54 + desc.length * 17 };
  });
  const h = items.reduce((s, x) => s + x.h + 10, 0) - 10;
  return {
    label: (fr.refs || []).join(', '), w: AW, h: Math.max(h, 40), items,
    draw(g) {
      const rows = {}; let y = 0;
      items.forEach((x) => {
        const rg = sv('g', { class: 'pv-ep' }, g);
        sv('rect', { x: 0, y, width: AW, height: x.h, rx: 8, class: 'row' }, rg);
        const vb = sv('g', {}, rg), vw = Math.max(34, x.verb.length * 7.6 + 12);
        sv('rect', { x: 12, y: y + 12, width: vw, height: 18, rx: 4, class: 'verb' }, vb);
        sv('text', { x: 12 + vw / 2, y: y + 25, 'text-anchor': 'middle', class: 'vt' }, vb, x.verb);
        sv('text', { x: 20 + vw, y: y + 25, class: 'path' }, rg, A.cut(x.path, 58));
        sv('text', { x: 14, y: y + 46, class: 'svc' }, rg, (x.e.service ? x.e.service + ' · ' : '') + (x.e.name || ''));
        textLines(rg, 14, y + 64, x.desc, 'desc', 17);
        sv('title', null, rg, [x.verb + ' ' + x.path, x.e.service, x.e.desc].filter(Boolean).join('\n'));
        rg.addEventListener('click', (e) => { if (P.cam.justDragged) return; e.stopPropagation(); A.select({ t: 'ep', id: x.i }); });
        rows[x.i] = rg; y += x.h + 10;
      });
      return { rows };
    },
  };
}

/* --- a module and its functions --- */
const FN_MAX = 10;
function moduleFrame(fr) {
  const mods = (fr.modules || []).filter((m) => m && (D.byMod[m] || D.mods[m]));
  const blocks = mods.map((m) => {
    // the steps of the scenarios on this page first, then the entry points, then the rest in file order
    const rank = (id) => (P.stepFids.has(id) ? 0 : D.fns[id].entry ? 1 : 2);
    const fids = (D.byMod[m] || []).slice().sort((a, b) => rank(a) - rank(b) || (D.fns[a].file < D.fns[b].file ? -1 : D.fns[a].file > D.fns[b].file ? 1 : D.fns[a].start - D.fns[b].start));
    const desc = wrap(A.modDesc(m), AW - 24, 13, 3);
    const shown = fids.slice(0, FN_MAX);
    return { m, desc, fids, shown, h: 34 + desc.length * 18 + 10 + shown.length * 34 + (fids.length > FN_MAX ? 26 : 0) };
  });
  const h = blocks.reduce((s, b) => s + b.h + 18, 0) - 18;
  return {
    label: (fr.refs || []).join(', '), w: AW, h: Math.max(h, 40), blocks,
    draw(g) {
      const rows = {}; let y = 0;
      blocks.forEach((b) => {
        const head = sv('g', { class: 'pv-mod' }, g);
        sv('rect', { x: 0, y: y + 6, width: 4, height: 20, rx: 2, fill: A.LCOLOR[A.layerOf(b.m)] }, head);
        sv('text', { x: 14, y: y + 22, class: 'mname' }, head, A.cut(A.modLabel(b.m), 60));
        head.addEventListener('click', (e) => { if (P.cam.justDragged) return; e.stopPropagation(); A.select({ t: 'mod', id: b.m }); });
        let yy = y + 34 + textLines(g, 14, y + 44, b.desc, 'desc', 18) + 10;
        b.shown.forEach((fid) => {
          const fn = D.fns[fid], fi = A.fnInfo(fid), rg = sv('g', { class: 'pv-fn' }, g);
          sv('rect', { x: 0, y: yy, width: AW, height: 30, rx: 6, class: 'row' }, rg);
          sv('text', { x: 14, y: yy + 19, class: 'fname' }, rg, A.cut(fi.name, 34) + (fn.entry ? '  ⚑ ' + fn.entry.kind : ''));
          sv('text', { x: AW - 12, y: yy + 19, 'text-anchor': 'end', class: 'fdoc' }, rg, A.cut(firstLine(A.fnDesc(fid)), 30));
          sv('title', null, rg, fid + (A.fnDesc(fid) ? '\n' + A.fnDesc(fid) : ''));
          rg.addEventListener('click', (e) => { if (P.cam.justDragged) return; e.stopPropagation(); A.select({ t: 'fn', id: fid }); });
          rows[fid] = rg; yy += 34;
        });
        if (b.fids.length > FN_MAX) sv('text', { x: 14, y: yy + 14, class: 'pv-muted' }, g, tr('pvMore', b.fids.length - FN_MAX));
        y += b.h + 18;
      });
      return { rows };
    },
  };
}

/* --- a note: text of the page file --- */
function noteFrame(fr) {
  // the line breaks typed in the note stay (an empty line too)
  const lines = String(fr.text || '').split('\n').flatMap((p) => (p.trim() ? wrap(p, AW, 15, 0) : ['']));
  return { label: '', w: AW, h: lines.length * 22 + 4, draw(g) { textLines(g, 0, 17, lines, 'pv-note', 22); return {}; } };
}

/* ---------- the canvas ---------- */
function mountPages(el) {
  el.innerHTML = '';
  const box = A.h('div', { class: 'pv-box' }, el);
  P.svg = sv('svg', { class: 'pv-svg' }, box);
  P.cam = new A.Cam(P.svg);
  P.cam.minW = 200; P.cam.maxW = 4e4;
  P.root = sv('g', {}, P.svg);
  const z = A.h('div', { class: 'zoomctl' }, el);
  const zb = (txt, tip, fn) => { const b = A.h('button', { type: 'button', title: tip, 'aria-label': tip }, z, txt); b.onclick = fn; return b; };
  P.zOut = zb('−', '', () => P.cam.zoomAt(0.5, 0.5, 1.25));
  P.zFit = zb('<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2.5 6V2.5H6M10 2.5h3.5V6M13.5 10v3.5H10M6 13.5H2.5V10"/></svg>', '', () => fit(true));
  P.zIn = zb('+', '', () => P.cam.zoomAt(0.5, 0.5, 0.8));
  P.svg.addEventListener('click', (e) => { if (P.cam.justDragged || e.target.closest('.pf')) return; A.select({ t: 'page', id: P.cur }); });
  // editing (archmap serve): "+ Note" in the corner, the hint of an empty page in the middle
  P.tools = A.h('div', { class: 'pe-tools' }, el);
  P.addNote = A.h('button', { type: 'button', class: 'pbtn' }, P.tools);
  P.addNote.onclick = () => noteEditor(null);
  P.empty = A.h('div', { class: 'pe-empty', hidden: '' }, el);
  P.el = el;
  A.bus.on('resize', () => { if (A.state.view === 'page') fit(false); });
}
function paintZoomTips() { if (!P.zIn) return; [[P.zIn, 'pvZIn'], [P.zOut, 'pvZOut'], [P.zFit, 'pvFit']].forEach(([b, k]) => { b.title = tr(k); b.setAttribute('aria-label', tr(k)); }); }
// the whole page, its top at the top of the canvas (a wide page does not float in the middle)
function fit(anim) {
  if (!P.extent || !P.svg.parentElement.offsetWidth) return;
  // 52 px on top stay free for the crumbs in the corner of the canvas
  const e = P.extent, pad = 40, top = 52, { W, H } = P.cam.size(), a = W / H;
  const w = Math.max(e.w + pad * 2, (e.h + pad * 2) * W / Math.max(1, H - top)), h = w / a;
  const vb = { x: e.x + e.w / 2 - w / 2, y: e.y - pad - top * w / W, w, h };
  if (anim) P.cam.animToVB(vb, 300); else P.cam.set(vb);
}

// the frames of a page as the file says them: a re-render with the same ones keeps the camera
const pageSig = (pg) => (pg ? pg.id + '\0' + JSON.stringify(pg.frames.map((f) => [f.kind, f.refs || [], f.text || ''])) : '');
function render() {
  if (!P.svg) return;
  P.root.innerHTML = ''; P.frames = []; P.extent = null;
  const pg = curPage();
  paintZoomTips(); paintTools(pg);
  // the camera: the same page with the same frames (a live update of the code) stays where it is; a frame added or
  // taken off — the page fits again, animated; another page — at once
  const sig = pageSig(pg), samePage = !!pg && P.sigId === pg.id, keep = samePage && sig === P.sig;
  P.sig = sig; P.sigId = pg ? pg.id : null;
  if (samePage && !keep) P.play = null; // the frame indexes moved
  const place = () => { if (!keep) fit(samePage); };
  if (!pg) { sv('text', { x: 0, y: 0, class: 'pv-muted' }, P.root, tr('pvNone')); P.extent = { x: -50, y: -30, w: 400, h: 80 }; fit(false); return; }
  if (!pg.frames.length) { P.extent = { x: -50, y: -30, w: 400, h: 80 }; P.built = P.cur; fit(false); return; }
  const slots = slotsOf(pg);
  // the scenarios first: their steps go first in the module frames
  const frames = [];
  P.stepFids = new Set();
  pg.frames.forEach((fr, i) => { if (fr.kind === 'journey' || fr.kind === 'flow') { frames[i] = makeFrame(fr, i); (frames[i].steps || []).forEach((s) => P.stepFids.add(s.fid)); } });
  pg.frames.forEach((fr, i) => { if (!frames[i]) frames[i] = makeFrame(fr, i); });
  // sizes: the content scaled into the width of its slot (never enlarged), capped in height
  slots.forEach((s) => {
    const f = frames[s.frame]; f.slot = s;
    f.fw = frameW(s.span);
    const iw = f.fw - PAD * 2;
    f.k = Math.min(1, iw / Math.max(1, f.w), MAXH / Math.max(1, f.h));
    f.ih = f.h * f.k;
    const miss = f.missing.length ? 22 : 0;
    f.fh = (f.scenario ? HEAD : 0) + PAD * 2 + f.ih + (f.scenario ? FOOT : 0) + miss;
  });
  const rowH = {}, nRows = Math.max(...slots.map((s) => s.row)) + 1;
  slots.forEach((s) => { rowH[s.row] = Math.max(rowH[s.row] || 0, frames[s.frame].fh); });
  const rowY = []; let y = 0; for (let r = 0; r < nRows; r++) { rowY[r] = y; y += (rowH[r] || 0) + ROWGAP; }
  let maxX = 0;
  slots.forEach((s) => {
    const f = frames[s.frame];
    f.x = s.col * (COL + GAP); f.y = rowY[s.row];
    maxX = Math.max(maxX, f.x + f.fw);
    drawFrame(f);
  });
  P.frames = frames;
  P.extent = { x: -20, y: -40, w: maxX + 40, h: y - ROWGAP + 60 };
  P.built = P.cur;
  if (P.play) { const f = frames[P.play.fi]; if (f && f.scenario) P.play.steps = f.steps; else P.play = null; }
  paintAll();
  place();
}
function drawFrame(f) {
  const g = sv('g', { class: 'pf pf-' + f.kind, 'data-fi': f.i, transform: `translate(${f.x},${f.y})` }, P.root);
  // the frame name above it, small, at the left (as in Figma)
  const name = f.title + (f.label ? ' · ' + f.label : '');
  sv('text', { x: 2, y: -10, class: 'pf-name' }, g, A.cut(name, Math.floor(f.fw / 7.2)));
  sv('rect', { x: 0, y: 0, width: f.fw, height: f.fh, rx: 12, class: 'pf-bg' }, g);
  let top = PAD;
  if (f.scenario) { drawPlayBar(f, g); top += HEAD; }
  const inner = sv('svg', { x: PAD, y: top, width: f.fw - PAD * 2, height: f.ih, viewBox: `0 0 ${f.w} ${f.h}`, preserveAspectRatio: 'xMinYMin meet', class: 'pf-in ' + ({ states: 'st', tables: 'er dv-er' }[f.kind] || ''), overflow: 'visible' }, g);
  f.painter = f.draw(inner) || {};
  if (f.missing.length) sv('text', { x: PAD, y: f.fh - PAD + 4 - (f.scenario ? FOOT : 0), class: 'pf-miss' }, g, '⚠ ' + tr('pvMissing') + ': ' + f.missing.join(', '));
  if (f.scenario) f.cap = sv('text', { x: PAD, y: f.fh - 16, class: 'pf-cap' }, g, '');
  g.addEventListener('click', (e) => { if (P.cam.justDragged) return; e.stopPropagation(); A.select({ t: 'frame', page: P.cur, i: f.i }); });
  g.addEventListener('dblclick', (e) => { e.stopPropagation(); if (f.kind === 'note' && canEdit()) noteEditor(f.i); else openFrame(f.i); });
  if (canEdit()) {
    // × above the frame at the right, on hover and on the selected frame (archmap serve)
    const del = sv('g', { class: 'pf-del', role: 'button', 'aria-label': tr('peDelFrame') }, g), x = f.fw - 24;
    sv('rect', { x, y: -26, width: 24, height: 22, rx: 6 }, del);
    sv('path', { d: `M${x + 8},-19 l8,8 M${x + 16},-19 l-8,8`, class: 'x' }, del);
    sv('title', null, del, tr('peDelFrame') + ' — Delete');
    del.addEventListener('click', (e) => { e.stopPropagation(); if (!P.cam.justDragged) removeFrame(f.i); });
    del.addEventListener('dblclick', (e) => e.stopPropagation());
  }
  f.g = g;
}
// the buttons of a scenario frame: Overview · ‹ k/N › (or "▶ Step through" on the overview)
function drawPlayBar(f, g) {
  const bar = sv('g', { class: 'pf-bar' }, g), y = PAD - 4;
  // the title: the scenario; while stepping — the chapter and the text of the current step
  const tg = sv('g', {}, bar), title = sv('text', { x: PAD, y: y + 19, class: 'pf-title' }, tg, A.cut(f.run ? f.run.name : f.refs[0], Math.floor((f.fw - 300) / 7.6)));
  const titleT = sv('title', null, tg, f.run ? f.run.name : f.refs[0]);
  const btn = (x, w, label, fn, cls) => {
    const b = sv('g', { class: 'pbtn-s' + (cls ? ' ' + cls : ''), role: 'button' }, bar);
    sv('rect', { x, y, width: w, height: 26, rx: 7 }, b);
    const tx = sv('text', { x: x + w / 2, y: y + 17.5, 'text-anchor': 'middle' }, b, label);
    b.addEventListener('click', (e) => { e.stopPropagation(); if (!P.cam.justDragged) fn(); });
    b.addEventListener('dblclick', (e) => e.stopPropagation());
    return { b, tx };
  };
  const R = f.fw - PAD;
  f.ui = {
    title, titleT,
    next: btn(R - 30, 30, '›', () => stepFrame(f.i, 1)),
    no: btn(R - 100, 66, '', () => {}, 'ro'),
    prev: btn(R - 134, 30, '‹', () => stepFrame(f.i, -1)),
    ov: btn(R - 228, 90, tr('pvOverview'), () => overview(f.i)),
    play: btn(R - 134, 134, tr('pvPlay'), () => stepFrame(f.i, 1), 'primary'),
  };
}

/* ---------- stepping a scenario frame ---------- */
function stepFrame(fi, d) {
  const f = P.frames[fi]; if (!f || !f.scenario || !f.steps.length) return;
  if (!P.play || P.play.fi !== fi) P.play = { fi, steps: f.steps, i: -1 };
  P.play.i = Math.max(0, Math.min(f.steps.length - 1, P.play.i + d));
  paintAll();
  A.select({ t: 'pstep', page: P.cur, fi, i: P.play.i });
}
function goStep(fi, i) { const f = P.frames[fi]; if (!f || !f.scenario || !f.steps.length) return; P.play = { fi, steps: f.steps, i: -1 }; stepFrame(fi, i + 1); }
function overview(fi) { P.play = null; paintAll(); A.select({ t: 'frame', page: P.cur, i: fi }); }
const curStep = () => (P.play && P.play.i >= 0 ? P.play.steps[P.play.i] : null);

// light on every frame what the current step touches (code only); the frame being stepped shows the step
function paintAll() {
  const step = curStep(), fi = P.play ? P.play.fi : -1;
  // the endpoint the scenario entered through: the endpoint of the first step of the current flow of the run
  let entryFid = null;
  if (step) { const s0 = P.play.steps.find((s) => s.seg === step.seg); entryFid = s0 ? s0.fid : null; }
  P.frames.forEach((f) => {
    const pt = f.painter || {}; let lit = false;
    if (f.scenario) {
      const on = f.i === fi && step, i = on ? P.play.i : -1;
      f.g.classList.toggle('playing', !!on);
      Object.keys(pt.nodes || {}).forEach((id) => {
        const cur = on && f.stepNode[i] === id, done = on && f.stepNode.slice(0, i).includes(id);
        const actNow = on && (f.stepLegs[i] || []).some((k) => k.endsWith('>' + id) || k.startsWith(id + '>'));
        pt.nodes[id].classList.toggle('cur', !!cur); pt.nodes[id].classList.toggle('done', !cur && !!done); pt.nodes[id].classList.toggle('touch', !cur && !!actNow);
      });
      Object.keys(pt.legs || {}).forEach((k) => {
        const cur = on && (f.stepLegs[i] || []).includes(k), done = on && f.stepLegs.slice(0, i).some((ls) => ls.includes(k));
        pt.legs[k].classList.toggle('cur', !!cur); pt.legs[k].classList.toggle('done', !cur && !!done);
        const ln = pt.legs[k].querySelector('.ln'); if (ln) ln.setAttribute('marker-end', cur ? 'url(#arrA)' : 'url(#arr)');
        signLeg(pt.legs[k], cur ? i : -1, cur ? step : null);
      });
      if (f.ui && f.ui.title) {
        const ch = on && f.run && f.run.segs.length > 1 && f.run.segs[step.seg] && f.run.segs[step.seg].flow ? f.run.segs[step.seg].flow.name + ' · ' : '';
        const tt = on ? ch + step.label : f.run ? f.run.name : f.refs[0];
        const tx = A.cut(tt, Math.floor((f.fw - 300) / 7.6)), tip = on ? ch + step.label + '\n' + A.play.action(step).full : tt;
        [[f.ui.title, tx], [f.ui.titleT, tip]].forEach(([e, s]) => { const n = e.firstChild; if (n && n.nodeType === 3 && !n.nextSibling) { if (n.data !== s) n.data = s; } else e.textContent = s; });
      }
      const svgEl = f.g.querySelector('.pf-in'); if (svgEl) svgEl.classList.toggle('stepping', !!on);
      paintBar(f, on ? i : -1);
    } else if (f.kind === 'states') {
      (pt.parts || []).forEach((p) => { const h = step ? A.status.hot(p.m, step) : null; A.status.paint(p.r, h); if (h) lit = true; });
    } else if (f.kind === 'tables' && pt.er) {
      const hot = step ? A.dataER.stepTables(step) : null; A.dataER.paint(pt.er, hot); lit = !!(hot && Object.keys(pt.er.cards).some((n) => hot.has(n)));
    } else if (f.kind === 'api') {
      Object.keys(pt.rows || {}).forEach((i) => { const e = D.endpoints[i], on = !!step && !!e.fid && (e.fid === entryFid || e.fid === step.fid); pt.rows[i].classList.toggle('hot', on); if (on) lit = true; });
    } else if (f.kind === 'module') {
      Object.keys(pt.rows || {}).forEach((fid) => { const on = !!step && fid === step.fid; pt.rows[fid].classList.toggle('hot', on); if (on) lit = true; });
      if (step && (f.src.modules || []).includes(step.mod)) lit = true;
    }
    f.g.classList.toggle('lit', lit);
  });
  paintSel();
  if (A.renderPages) A.renderPages();
  const n = document.getElementById('hName'); if (n && A.state.view === 'page' && !A.state.run) n.textContent = label(null) || n.textContent;
}
// signLeg(lg, i, step): the current leg of a scenario frame says what its step does — "8 · requestQR(Payment)" —
// when the pill is readable at the scale the frame is shown at; else only the number (the step text is in the frame
// title). i < 0 — back to the plain step numbers.
function signLeg(lg, i, step) {
  const p = lg.pv; if (!p) return;
  let txt = p.txt, tw = p.tw;
  if (i >= 0 && step) {
    const k = (p.pill.getBoundingClientRect().height || 0) / 18, act = A.play.action(step).short;
    txt = String(i + 1) + (k >= 0.7 && act ? ' · ' + A.cut(act, 40) : '');
    tw = txt.length * (txt.length > 4 ? 6.6 : 6.4) + 14;
  }
  const tn = p.no.firstChild; // the text node is kept: only its data changes
  if (tn && tn.nodeType === 3) { if (tn.data !== txt) tn.data = txt; } else p.no.textContent = txt;
  const x = String(p.mx - tw / 2), w = String(tw);
  if (p.pill.getAttribute('x') !== x) p.pill.setAttribute('x', x);
  if (p.pill.getAttribute('width') !== w) p.pill.setAttribute('width', w);
}
function paintBar(f, i) {
  const u = f.ui; if (!u) return;
  const ov = i < 0, n = f.steps.length;
  u.play.b.style.display = ov && n ? '' : 'none';
  [u.prev, u.no, u.next, u.ov].forEach((x) => { x.b.style.display = ov ? 'none' : ''; });
  u.no.tx.textContent = ov ? '' : tr('pvStepOf', i + 1, n);
  u.prev.b.classList.toggle('off', i <= 0); u.next.b.classList.toggle('off', i >= n - 1);
  const s = ov ? null : f.steps[i];
  if (f.cap) f.cap.textContent = s ? A.cut(tr('pvStep') + ' ' + (i + 1) + ' · ' + s.label + (s.flow && f.run && f.run.segs.length > 1 ? ' — ' + s.flow.name : ''), Math.floor((f.fw - PAD * 2) / 7.4)) : n ? tr('pvSteps', n) + (f.run && f.run.segs.length > 1 ? ' · ' + f.run.segs.length + ' ' + tr('pvFlow').toLowerCase() : '') + ' · ' + tr('pvKeys') : tr('pvNoSteps');
}
function paintSel() {
  const s = A.state.sel;
  P.frames.forEach((f) => { if (f.g) f.g.classList.toggle('sel', !!s && (s.t === 'frame' || s.t === 'pstep') && s.page === P.cur && (s.i === f.i || s.fi === f.i)); });
}

/* ---------- opening a frame on the whole canvas ---------- */
function openFrame(fi) {
  const f = P.frames[fi]; if (!f) return;
  const back = P.cur;
  const go = () => { P.ret = back; };
  switch (f.kind) {
    case 'journey': case 'flow': {
      if (!f.key || !f.steps.length) return;
      go(); const at = P.play && P.play.fi === fi ? P.play.i : -1;
      A.side.openScenario(f.key, at); break;
    }
    case 'states': { const k = (f.src.machines || []).find(Boolean); if (!k) return; go(); A.setView('status'); A.bus.emit('status:focus', { machine: k }); break; }
    case 'tables': { const n = (f.refs || []).find((x) => A.dataER && A.dataER.has(x)); if (!n) return; go(); A.setView('data'); A.bus.emit('data:focus', { table: n }); break; }
    case 'api': { const i = (f.src.endpoints || []).find((x) => x >= 0); if (i == null) return; go(); A.setView('api'); A.select({ t: 'ep', id: i }); break; }
    case 'module': { const m = (f.src.modules || []).find(Boolean); if (!m) return; go(); A.setView('map'); A.map.goModule(m); break; }
    default:
  }
}
// Esc: stepping stops first; a frame open on the whole canvas comes back to its page
function back() {
  if (A.state.view === 'page' && P.play && !A.state.run) { overview(P.play.fi); return true; }
  if (!P.ret) return false;
  const id = P.ret; P.ret = null;
  if (A.state.run) A.play.close();
  open(id);
  return true;
}
function open(id) {
  if (!pageOf(id)) return;
  if (P.cur !== id) P.play = null;
  P.cur = id;
  A.setView('page'); // (again) — the sidebar, the page list and the hash follow the 'view' event
  A.select({ t: 'page', id });
}

/* ---------- the tree of the page (side.js shows it under the page list) ---------- */
function tree(pg) {
  const N = (key, name, icon, o) => Object.assign({ key, name, icon }, o || {});
  const frames = slotsOf(pg).map((s) => {
    const fr = pg.frames[s.frame], i = s.frame, k = 'pf:' + i, kind = tr('pvKind')[fr.kind] || fr.kind, focus = () => focusFrame(i);
    const name = fr.kind === 'note' ? kind + ' · ' + A.cut(fr.text || '', 40) : kind + ' · ' + (fr.refs || []).join(', ');
    const icon = { journey: 'path', flow: 'flow', states: 'status', tables: 'table', api: 'api', module: 'mod', note: 'note' }[fr.kind];
    let kids = [];
    const miss = new Set(fr.missing || []);
    if (fr.kind === 'journey' || fr.kind === 'flow') {
      kids = () => { const f = P.frames[i]; if (!f || !f.run) return []; let at = 0; return f.run.segs.map((sg, si) => { const s0 = at; at += sg.steps.length; return N(k + ':f:' + si, sg.flow ? sg.flow.name : f.run.name, null, { no: si + 1, act: () => { focusFrame(i, true); goStep(i, s0); } }); }); };
    } else if (fr.kind === 'states') kids = (fr.refs || []).map((r, j) => N(k + ':s:' + ((fr.machines || [])[j] || r), r, 'status', { cls: miss.has(r) ? 'bad' : '', act: focus }));
    else if (fr.kind === 'tables') kids = (fr.refs || []).map((r) => N(k + ':t:' + r, r, 'table', { cls: 'mono' + (miss.has(r) ? ' bad' : ''), act: () => { focusFrame(i, true); if (!miss.has(r)) A.select({ t: 'table', id: r }); } }));
    else if (fr.kind === 'api') kids = (fr.refs || []).map((r, j) => { const ei = (fr.endpoints || [])[j]; return N(k + ':e:' + (ei >= 0 ? ei : r), r, 'ep', { cls: 'mono' + (miss.has(r) ? ' bad' : ''), act: () => { focusFrame(i, true); if (ei >= 0) A.select({ t: 'ep', id: ei }); } }); });
    else if (fr.kind === 'module') kids = (fr.refs || []).map((r, j) => { const m = (fr.modules || [])[j]; return N(k + ':m:' + (m || r), r, 'mod', { cls: miss.has(r) ? 'bad' : '', act: () => { focusFrame(i, true); if (m) A.select({ t: 'mod', id: m }); } }); });
    return N(k, name, icon, { title: name, cls: (fr.missing || []).length ? 'bad' : '', act: focus, kids });
  });
  return N('pg:' + pg.id, pg.name, 'page', { title: pg.name + (pg.file ? '\n' + pg.file + ':' + pg.line : ''), act: () => { if (A.state.view !== 'page' || P.cur !== pg.id) open(pg.id); else { fit(true); A.select({ t: 'page', id: pg.id }); } }, kids: frames });
}
function focusFrame(i, keepSel) {
  if (A.state.view !== 'page') open(P.cur);
  const f = P.frames[i]; if (!f) return;
  P.cam.animTo({ x: f.x, y: f.y - 30, w: f.fw, h: f.fh + 30 }, 40, 0, 320);
  if (!keepSel) A.select({ t: 'frame', page: P.cur, i });
}
function selKey() {
  const s = A.state.sel, pg = curPage(); if (!pg) return null;
  const inFrame = (kind, test) => { const i = pg.frames.findIndex((f, j) => f.kind === kind && test(f, j)); return i; };
  if (!s || s.t === 'page') return 'pg:' + pg.id;
  if (s.t === 'frame') return 'pf:' + s.i;
  if (s.t === 'pstep') { const f = P.frames[s.fi], st = f && f.steps[s.i]; return 'pf:' + s.fi + (st ? ':f:' + st.seg : ''); }
  if (s.t === 'table') { const i = inFrame('tables', (f) => (f.refs || []).includes(s.id)); if (i >= 0) return 'pf:' + i + ':t:' + s.id; }
  if (s.t === 'ep') { const i = inFrame('api', (f) => (f.endpoints || []).includes(s.id)); if (i >= 0) return 'pf:' + i + ':e:' + s.id; }
  if (s.t === 'mod') { const i = inFrame('module', (f) => (f.modules || []).includes(s.id)); if (i >= 0) return 'pf:' + i + ':m:' + s.id; }
  if (s.t === 'status') { const i = inFrame('states', (f) => (f.machines || []).includes(s.entity)); if (i >= 0) return 'pf:' + i + ':s:' + s.entity; }
  return null;
}
function ancestors(key) {
  const a = ['pg:' + P.cur], m = /^pf:(\d+)/.exec(key || '');
  if (m && key !== m[0]) a.push(m[0]);
  return a;
}
// the toolbar name of a page: with the step while a frame is stepped
function label(p) {
  if (p && (!p.user || p.page.id !== P.cur)) return '';
  const pg = curPage(); if (!pg) return '';
  const st = curStep();
  return st ? pg.name + '  ·  ' + tr('pvStep') + ' ' + (P.play.i + 1) + '/' + P.play.steps.length : pg.name;
}

/* ---------- the inspector ---------- */
const kindChip = (k) => `<span class="chip">${esc(tr('pvKind')[k] || k)}</span>`;
function frameLine(fr) { return fr.kind === 'note' ? esc(A.cut(fr.text || '', 80)) : `<span class="mono">${esc((fr.refs || []).join(', '))}</span>`; }
A.registerPanel('page', (sel) => {
  const pg = pageOf(sel.id); if (!pg) return `<p class="lead">${esc(tr('pvNone'))}</p>`;
  const bad = pg.frames.filter((f) => (f.missing || []).length);
  let h = `<div class="kicker"><span class="chip">${esc(tr('pvTitle'))}</span></div><h2>${esc(pg.name)}</h2>`;
  h += `<dl class="kv"><dt>${esc(tr('pvFrom'))}</dt><dd class="mono">${esc((pg.file || '') + (pg.line ? ':' + pg.line : ''))}</dd><dt>${esc(tr('pvFrames'))}</dt><dd>${pg.frames.length}</dd></dl>`;
  h += `<h3>${esc(tr('pvFrames'))}</h3><ul class="rel-list">${slotsOf(pg).map((s) => { const fr = pg.frames[s.frame]; return `<li data-pf="${s.frame}">${kindChip(fr.kind)}<span>${frameLine(fr)}</span></li>`; }).join('')}</ul>`;
  if (bad.length) h += `<h3>${esc(tr('pvBroken'))}</h3><ul class="rel-list">${bad.map((f) => `<li data-pf="${pg.frames.indexOf(f)}"><span class="mono" style="color:var(--crit-text)">${esc(f.kind + ' ' + f.missing.join(', '))}</span><span class="md">${esc(pg.file + ':' + f.line)}</span></li>`).join('')}</ul>`;
  h += `<p class="loc" style="margin-top:14px">${esc(tr('pvDbl'))}</p>`;
  return { html: h, after: wirePanel };
});
A.registerPanel('frame', (sel) => {
  const pg = pageOf(sel.page), fr = pg && pg.frames[sel.i]; if (!fr) return '';
  const f = P.frames[sel.i];
  let h = `<div class="kicker">${kindChip(fr.kind)}<span class="chip">${esc(pg.name)}</span></div>`;
  h += `<h2>${fr.kind === 'note' ? esc(tr('pvKind').note) : esc(f && f.run ? f.run.name : (fr.refs || []).join(', '))}</h2>`;
  h += `<p class="lead">${esc(tr('pvDrawn')[fr.kind] || '')}</p>`;
  if (fr.kind === 'note') h += `<p>${esc(fr.text || '')}</p>`;
  if (f && f.scenario && f.run) {
    let at = 0;
    h += `<h3>${esc(tr('pvFlow'))} (${f.run.segs.length}) · ${esc(tr('pvSteps', f.steps.length))}</h3><ul class="rel-list">${f.run.segs.map((sg, si) => { const s0 = at; at += sg.steps.length; return `<li data-go="${s0}"><span class="fn">${si + 1}.</span><span>${esc(sg.flow ? sg.flow.name : f.run.name)}</span><span class="md">${sg.steps.length}</span></li>`; }).join('')}</ul>`;
  }
  if (fr.kind === 'tables' || fr.kind === 'states' || fr.kind === 'api' || fr.kind === 'module') h += `<h3>${esc(tr('pvKind')[fr.kind])}</h3><div class="tbl-pills">${(fr.refs || []).map((r) => `<span class="chip mono"${(fr.missing || []).includes(r) ? ' style="color:var(--crit-text)"' : ''}>${esc(r)}</span>`).join('')}</div>`;
  if ((fr.missing || []).length) h += `<p class="loc" style="color:var(--crit-text)">⚠ ${esc(tr('pvMissing'))}: ${esc(fr.missing.join(', '))}</p>`;
  h += `<div class="loc">${esc((pg.file || '') + ':' + fr.line)}</div>`;
  h += '<div class="btnrow">' + (fr.kind !== 'note' ? `<button class="pbtn primary" data-act="pvopen">${esc(tr('pvOpen'))}</button>` : '') + (f && f.scenario && f.steps.length ? `<button class="pbtn" data-act="pvstep">${esc(tr('pvStepIt'))}</button>` : '') +
    (canEdit() && fr.kind === 'note' ? `<button class="pbtn primary" data-act="penote">${esc(tr('peEditNote'))}</button>` : '') +
    (canEdit() ? `<button class="pbtn" data-act="pedel" title="Delete">${esc(tr('peDelFrame'))}</button>` : '') + '</div>';
  h += `<p class="loc" style="margin-top:14px">${esc(tr('pvDbl'))}</p>`;
  return { html: h, after: (p) => {
    wirePanel(p);
    const on = (act, fn) => { const b = p.querySelector(`[data-act="${act}"]`); if (b) b.onclick = fn; };
    on('pvopen', () => openFrame(sel.i)); on('pvstep', () => goStep(sel.i, 0)); on('penote', () => noteEditor(sel.i)); on('pedel', () => removeFrame(sel.i));
    p.querySelectorAll('[data-go]').forEach((li) => { li.onclick = () => goStep(sel.i, +li.getAttribute('data-go')); });
  } };
});
A.registerPanel('pstep', (sel) => {
  const f = P.frames[sel.fi], s = f && f.steps[sel.i]; if (!s) return '';
  let h = `<div class="kicker">${kindChip(f.kind)}<span class="chip">${esc(tr('pvStep'))} ${sel.i + 1}/${f.steps.length}</span>${s.flow ? `<span class="chip">${esc(A.cut(s.flow.name, 40))}</span>` : ''}</div>`;
  h += `<h2>${esc(s.label)}</h2>`;
  const tb = A.dataER ? Array.from(A.dataER.stepTables(s).entries()) : [];
  const seen = new Set();
  const sts = (s.states || []).concat((D.transByFid[s.fid] || []).map((x) => ({ entity: x.entity, from: x.from, to: x.to })))
    .filter((x) => { const k = x.entity + '>' + x.to; if (seen.has(k)) return false; seen.add(k); return true; });
  if (tb.length || sts.length) {
    h += `<h3>${esc(tr('pvTouched'))}</h3>`;
    if (tb.length) h += `<div class="tbl-pills">${tb.map(([n, o]) => `<span class="chip mono">${esc(n)} · ${esc(Array.from(o.ops).join('/'))}</span>`).join('')}</div>`;
    if (sts.length) h += `<div class="btnrow">${sts.slice(0, 4).map((x) => A.ui.statusPill(x)).join('')}</div>`;
  }
  if (D.fns[s.fid]) h += A.fnSection(s.fid, s.next, { codeId: 'pvcode', noSum: false }).h;
  return h;
});
function wirePanel(p) {
  p.querySelectorAll('[data-pf]').forEach((li) => { li.onclick = () => focusFrame(+li.getAttribute('data-pf')); });
}

/* ---------- editing (archmap serve) ----------
 * Every edit is one POST /spec/page (internal/serve): the server changes only the lines of the page block in its file,
 * checks that what is added exists in the code, writes atomically and answers with the checked model — the page is
 * redrawn from it at once, the live rebuild (SSE) follows. Each edit carries the page as shown here (`expect`): a page
 * changed on disk meanwhile is not overwritten (409 — the new version is shown). The answer has the texts before and
 * after: ⌘Z / Ctrl+Z puts the last edit back (⌘⇧Z / Ctrl+Y — again), only while the file is still what was written.
 * In a static map (no D.live) nothing here is offered. */
const canEdit = () => !!D.live;
const ED = { undo: [], redo: [], q: Promise.resolve(), tt: 0 };
function toast(msg, kind) {
  let el = document.getElementById('peToast');
  if (!el) el = A.h('div', { id: 'peToast', class: 'pe-toast', role: 'status', 'aria-live': 'polite' }, document.body);
  el.textContent = msg; el.className = 'pe-toast on' + (kind ? ' ' + kind : '');
  clearTimeout(ED.tt); ED.tt = setTimeout(() => { el.className = 'pe-toast'; }, kind === 'err' ? 6500 : 2600);
}
const viewOfPage = (pg) => ({ name: pg.name, frames: pg.frames.map((f) => ({ kind: f.kind, refs: f.refs || [], text: f.text || '' })) });
const errOf = (j, res) => (j && (j.error || (j.errors && j.errors[0] && j.errors[0].msg))) || 'HTTP ' + res.status;
// the checked model of the answer: its pages replace the ones on screen
function applyModel(m) {
  if (!m || !D.M) return;
  D.M.spec = D.M.spec ? Object.assign({}, D.M.spec, { pages: m.pages || [] }) : m;
  const gone = P.cur && !pageOf(P.cur);
  if (gone) { P.cur = null; P.play = null; if (A.state.view === 'page') { A.openPage('system'); } }
  P.built = null;
  if (A.state.view === 'page' && P.cur) render();
  const s = A.state.sel;
  if (s && (s.t === 'frame' || s.t === 'pstep') && s.page === P.cur && !(curPage() && curPage().frames[s.i != null ? s.i : s.fi])) A.select({ t: 'page', id: P.cur });
  else if (A.renderPanel) A.renderPanel();
  if (A.renderPages) A.renderPages();
  if (A.side) A.side.render();
}
// pageOp(op, {record, quiet}) → the answer, {error} (quiet: the caller shows it) or null (conflict / no connection)
function pageOp(op, o = {}) {
  const run = async () => {
    if (!canEdit()) { toast(tr('peServeOnly')); return null; }
    if (op.page && op.op !== 'create' && op.op !== 'restore' && !op.expect) { const pg = pageOf(op.page); if (pg) op = Object.assign({ expect: viewOfPage(pg) }, op); }
    let res, j;
    try {
      res = await fetch('/spec/page', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(op) });
      j = await res.json().catch(() => ({}));
    } catch (e) { toast(tr('peNoConn'), 'err'); return null; }
    if (res.status === 409) { applyModel(j.model); toast(tr('peConflict'), 'err'); return null; }
    if (!res.ok) { const msg = (res.status === 422 ? tr('peBroken') + ': ' : '') + errOf(j, res); if (!o.quiet) toast(msg, 'err'); return { error: msg }; }
    if (j.notice === 'already') toast(tr('peAlready'));
    if ((j.edits || []).length && o.record !== false) { ED.undo.push(j.edits); if (ED.undo.length > 100) ED.undo.shift(); ED.redo = []; }
    applyModel(j.model);
    return j;
  };
  ED.q = ED.q.then(run, run);
  return ED.q;
}
async function undo(again) {
  const from = again ? ED.redo : ED.undo, to = again ? ED.undo : ED.redo;
  const edits = from.pop();
  if (!edits) { toast(tr(again ? 'peNoRedo' : 'peNoUndo')); return; }
  const j = await pageOp({ op: 'restore', files: edits.map((e) => ({ path: e.path, text: e.before, expect: e.after })) }, { record: false });
  if (!j) return; // changed on disk: this step is gone
  if (j.error) { from.push(edits); return; }
  to.push(j.edits);
  toast(tr(again ? 'peRedone' : 'peUndone'));
  // a page that came back (a delete undone): open it
  if (A.state.view !== 'page' || !P.cur) { const pg = pagesOf().find((p) => edits.some((e) => e.path === p.file && e.before != null)); if (pg) open(pg.id); }
}
async function removeFrame(i) {
  if (!P.cur) return;
  const j = await pageOp({ op: 'remove_frame', page: P.cur, index: i });
  if (j && !j.error) toast(tr('peRemoved') + ' · ' + tr('peUndoHint'));
}
// Delete: the selected frame, or the selected table / endpoint / status / module out of its frame
function deleteSelected() {
  const s = A.state.sel, pg = curPage(); if (!s || !pg || A.state.run) return false;
  if (s.t === 'frame' && s.page === P.cur) { removeFrame(s.i); return true; }
  const find = (kind, list, key) => { for (let i = 0; i < pg.frames.length; i++) { const f = pg.frames[i]; if (f.kind !== kind) continue; const j = (list(f) || []).indexOf(key); if (j >= 0) return { i, ref: f.refs[j] }; } return null; };
  const hit = s.t === 'table' ? find('tables', (f) => f.refs, s.id) : s.t === 'ep' ? find('api', (f) => f.endpoints, s.id) : s.t === 'mod' ? find('module', (f) => f.modules, s.id) : s.t === 'status' && !s.id ? find('states', (f) => f.machines, s.entity) : null;
  if (!hit) return false;
  pageOp({ op: 'remove_ref', page: P.cur, index: hit.i, refs: [hit.ref] }).then((j) => { if (j && !j.error) toast(tr('peRefRemoved', hit.ref) + ' · ' + tr('peUndoHint')); });
  return true;
}

/* the text of a note, typed in place: over the frame (or, for a new note, in the middle of the canvas) */
function noteEditor(fi) {
  if (!canEdit()) { toast(tr('peServeOnly')); return; }
  const pg = curPage(); if (!pg || !P.el) return;
  closeNote();
  const r0 = P.el.getBoundingClientRect();
  let x = Math.max(16, r0.width / 2 - 230), y = 76, w = 460, h = 170;
  const f = fi != null ? P.frames[fi] : null;
  if (f && f.g) { const r = f.g.querySelector('.pf-bg').getBoundingClientRect(); x = Math.max(8, r.left - r0.left); y = Math.max(8, r.top - r0.top); w = Math.max(320, r.width); h = Math.max(140, Math.min(r.height, r0.height - y - 60)); }
  const box = A.h('div', { class: 'pe-note', style: `left:${x}px;top:${y}px;width:${w}px` }, P.el);
  const ta = A.h('textarea', { class: 'pe-note-ta', placeholder: tr('peNotePh'), 'aria-label': tr('peNote'), style: `height:${h}px` }, box);
  A.h('div', { class: 'pe-note-hint' }, box, esc(tr('peNoteKeys')));
  const old = f ? pg.frames[fi].text || '' : '';
  ta.value = old;
  let done = false;
  const finish = (save) => {
    if (done) return; done = true;
    const v = ta.value.replace(/\r/g, '');
    box.remove(); P.note = null;
    if (!save) return;
    if (fi == null) { if (v.trim()) pageOp({ op: 'add_note', page: pg.id, text: v }); return; }
    if (!v.trim()) pageOp({ op: 'remove_frame', page: pg.id, index: fi });
    else if (v !== old) pageOp({ op: 'set_note', page: pg.id, index: fi, text: v });
  };
  ta.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'Escape') { e.preventDefault(); finish(false); }
    else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); finish(true); }
  });
  ta.addEventListener('blur', () => finish(true));
  ['pointerdown', 'wheel', 'dblclick', 'click'].forEach((ev) => box.addEventListener(ev, (e) => e.stopPropagation()));
  P.note = { box, finish };
  ta.focus(); ta.setSelectionRange(ta.value.length, ta.value.length);
}
function closeNote() { if (P.note) P.note.finish(true); }

// the corner tools and the hint of an empty page
function paintTools(pg) {
  if (!P.tools) return;
  const edit = canEdit() && !!pg;
  P.tools.hidden = !edit;
  P.addNote.textContent = tr('peAddNote'); P.addNote.title = tr('peNote');
  const empty = !!pg && !pg.frames.length;
  P.empty.hidden = !empty;
  if (empty) {
    P.empty.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="3" stroke-dasharray="3 3"/><path d="M12 8v8M8 12h8"/></svg>` +
      `<p>${esc(canEdit() ? tr('peEmptyHint') : tr('pvEmpty'))}</p>` +
      (canEdit() ? `<button type="button" class="pbtn" data-act="srcnav">${esc(tr('peEmptyOpen'))}</button>` : `<p class="sub">${esc(tr('peEmptyStatic'))}</p>`);
    const b = P.empty.querySelector('[data-act="srcnav"]'); if (b) b.onclick = () => { if (A.side.setSource) A.side.setSource(true); };
  }
}

/* dropping things of the System navigation on the page (side.js drags them): items [{kind, refs, label}] */
const RANK = (k) => (k === 'journey' || k === 'flow' ? 0 : k === 'states' ? 1 : 2);
function accepts(items) {
  if (!canEdit()) return { ok: false, why: tr('peServeOnly') };
  const pg = curPage(); if (!pg || A.state.view !== 'page') return { ok: false, why: tr('peNoPage') };
  const bad = items.find((x) => x.why); if (bad) return { ok: false, why: bad.why };
  const n = newRefs(pg, items).reduce((s, x) => s + x.refs.length, 0);
  if (!n) return { ok: false, dup: true, why: tr('peAlready') };
  return { ok: true, n, why: tr('peDropAdd', n) };
}
// what is not on the page yet, by kind: an endpoint / state machine / module counts as there under any name that
// resolves to it (CancelOrder and TabletsService.CancelOrder are one endpoint)
function newRefs(pg, items) {
  const have = (kind, ref, key) => pg.frames.some((f) => f.kind === kind && ((f.refs || []).includes(ref) ||
    (kind === 'api' && (f.endpoints || []).includes(key)) || (kind === 'states' && (f.machines || []).includes(key)) || (kind === 'module' && (f.modules || []).includes(key))));
  const by = new Map();
  items.forEach((x) => x.refs.forEach((r, j) => {
    const key = x.keys ? x.keys[j] : r;
    if (have(x.kind, r, key)) return;
    if (!by.has(x.kind)) by.set(x.kind, []);
    if (!by.get(x.kind).includes(r)) by.get(x.kind).push(r);
  }));
  return Array.from(by, ([kind, refs]) => ({ kind, refs }));
}
async function addItems(items) {
  const pg = curPage(); if (!pg) return;
  const add = newRefs(pg, items);
  if (!add.length) { toast(tr('peAlready')); return; }
  for (const x of add) await pageOp({ op: 'add', page: pg.id, kind: x.kind, refs: x.refs });
}
// move frame i before / after frame j (the order of the lines; the layout keeps the large diagrams on top)
function moveFrame(i, j, after) {
  const pg = curPage(); if (!pg || i === j) return;
  const order = pg.frames.map((_, k) => k).filter((k) => k !== i);
  const to = order.indexOf(j) + (after ? 1 : 0);
  if (to === i) return;
  pageOp({ op: 'move_frame', page: pg.id, index: i, to });
}
window.addEventListener('keydown', (e) => {
  const tg = e.target.tagName;
  if (tg === 'INPUT' || tg === 'TEXTAREA' || tg === 'SELECT' || e.target.isContentEditable || !canEdit()) return;
  const mod = (e.metaKey || e.ctrlKey) && !e.altKey;
  if (mod && e.code === 'KeyZ' && (ED.undo.length || ED.redo.length)) { e.preventDefault(); undo(e.shiftKey); }
  else if (mod && e.code === 'KeyY' && ED.redo.length) { e.preventDefault(); undo(true); }
});

/* ---------- keys ---------- */
function onKey(e) {
  if (A.state.view !== 'page' || !P.frames.length) return false;
  const k = e.key;
  if ((k === 'Delete' || k === 'Backspace') && canEdit()) return deleteSelected();
  if (k !== 'ArrowRight' && k !== 'ArrowLeft' && k !== 'Home' && !(k === ' ' && e.target.tagName !== 'BUTTON')) return false;
  let fi = P.play ? P.play.fi : -1;
  if (fi < 0) { const s = A.state.sel; fi = s && s.t === 'frame' && P.frames[s.i] && P.frames[s.i].scenario ? s.i : P.frames.findIndex((f) => f.scenario && f.steps.length); }
  if (fi < 0) return false;
  if (k === 'Home') { overview(fi); return true; }
  if (!P.play && k === 'ArrowLeft') return false;
  stepFrame(fi, k === 'ArrowLeft' ? -1 : 1);
  return true;
}

A.registerView({
  id: 'page',
  order: 90,
  menu: false, // pages open from the page list on top of the sidebar, not from ⋯
  title: () => { const pg = curPage(); return pg ? pg.name : tr('pvTitle'); },
  mount(el) { mountPages(el); },
  show() { if (P.built !== P.cur || !P.frames.length) render(); else { paintAll(); fit(false); } },
  onSelect() { paintSel(); },
  onLang() { if (A.state.view === 'page') render(); else P.built = null; },
  onData() { if (P.cur && !pageOf(P.cur)) P.cur = null; P.built = null; if (A.state.view === 'page') render(); },
  onKey,
});

A.pages = {
  get cur() { return P.cur; }, has: (id) => !!pageOf(id), open, back, forget: () => { P.ret = null; },
  tree, selKey, ancestors, label, render,
  // editing (archmap serve; side.js: the page list, dragging from the System navigation, the order in the tree)
  edit: { get can() { return canEdit(); }, op: pageOp, undo, toast, accepts, add: addItems, move: moveFrame, rank: RANK, removeFrame, note: noteEditor, get el() { return P.el; }, get undoN() { return ED.undo.length; } },
  // for tests and the ?debug=1 audit: the frames on the canvas
  get frames() { return P.frames.map((f) => ({ i: f.i, kind: f.kind, x: f.x, y: f.y, w: f.fw, h: f.fh, k: f.k, lit: f.g ? f.g.classList.contains('lit') : false })); },
  step: (fi, i) => goStep(fi, i),
};
