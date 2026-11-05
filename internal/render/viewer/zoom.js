/* zoom.js — semantic zoom on the map ("like in maps": zooming into an element dives into it, zooming out
 * leaves the level) and the debug audit of foreign visible panels (?debug=1). */

const DEBUG = qs.get('debug') === '1';

/* ---------- semantic zoom ---------- */
const DIVE = 0.55;   // the target card takes more of the screen (by its larger side) than this: dive in
const LEAVE = 0.35;  // the whole level takes less than this: go up (hysteresis against DIVE)
const AFTER_UP = 0.4; // after leaving, the card we left takes this share of the parent's screen (< DIVE, no bounce back)
const PAUSE = 300;   // ms after a transition when no new one may start
const GESTURE_GAP = 140; // ms of silence that separates one wheel gesture from the next (trackpad inertia is one gesture)
const CODE_MAX = 2.5, CODE_LEAVE = 0.65;
// The dive is always on and has no switch: pinch / Cmd+wheel into a card past DIVE opens its level, zooming the
// level out past LEAVE goes a level up (the crumbs follow). A plain wheel / two-finger scroll only pans. Scenario
// steps never move by themselves: only zoom changes the level here.
const AZ = { on: true, armed: true, until: 0, last: 0, cz: 1 };

const autoBusy = () => state.busy;
function setupZoom() {
  const sc = $('#sc3');
  sc.addEventListener('wheel', codeWheel, { passive: false });
  onGesture(sc, codeWheel);
  bus.on('level', (l) => { if (l !== 3) setCodeZoom(1); });
}

// gesture gate: a transition starts only on the first event of a gesture after the pause
function gate(now) {
  const gap = now - AZ.last; AZ.last = now;
  if (!AZ.armed) { if (gap > GESTURE_GAP && now > AZ.until) AZ.armed = true; else return false; }
  return !state.busy;
}
function settle(p) {
  AZ.armed = false; AZ.until = performance.now() + PAUSE * 4; // re-armed by the first event after PAUSE of silence
  return Promise.resolve(p).catch((err) => console.error('archmap auto zoom', err)).then(() => { AZ.until = performance.now() + PAUSE; });
}

// the node a zoom-in aims at: under the pointer, else the selected one, else nearest to the screen centre
function zoomTarget(e) {
  const sc = [sys, map, fv][state.level]; if (!sc) return null;
  const nodes = sc.nodes, s = stageRect(), st = $('#v-map').getBoundingClientRect();
  const under = document.elementFromPoint(e.clientX, e.clientY), n = under && under.closest && under.closest('.n');
  let id = n && n.getAttribute('data-id'); if (id && nodes[id]) return id;
  const sel = state.sel;
  if (sel) {
    const cand = state.level === 0 ? (sel.t === 'svc' ? 'svc' : sel.id) : state.level === 1 ? (unitFor(sel.id) || sel.id) : sel.t === 'fn' ? sel.id : null;
    const el = cand && nodes[cand] && nodes[cand].el;
    if (el) { const r = el.getBoundingClientRect(); if (r.right > st.left && r.left < st.right && r.bottom > st.top && r.top < st.bottom) return cand; }
  }
  const cx = st.left + s.width / 2, cy = st.top + s.height / 2; let best = null, bd = Infinity;
  Object.keys(nodes).forEach((k) => {
    const el = nodes[k].el; if (!el || !el.getBoundingClientRect) return;
    const r = el.getBoundingClientRect(); if (!r.width) return;
    const d = Math.hypot(r.left + r.width / 2 - cx, r.top + r.height / 2 - cy);
    if (d < bd) { bd = d; best = k; }
  });
  return best;
}
const shareOf = (r, s) => Math.max(r.width / s.width, r.height / s.height);

function semanticZoom(cam, e) {
  if (!AZ.on || state.view !== 'map' || state.level > 2 || cam !== curCam()) return;
  const dir = e.deltaY < 0 ? 1 : e.deltaY > 0 ? -1 : 0; if (!dir) return;
  if (!gate(performance.now())) return;
  if (dir > 0) {
    const id = zoomTarget(e), sc = [sys, map, fv][state.level];
    if (!id || !sc.nodes[id]) return;
    if (shareOf(sc.nodes[id].el.getBoundingClientRect(), stageRect()) <= DIVE) return;
    if (state.level === 0) selectSys(id); else if (state.level === 1) selectMod(id); else selectFn(id);
    settle(intoSelected());
  } else if (state.level >= 1) {
    const ex = (state.level === 1 ? map : fv).extent, v = cam.vb;
    if (Math.max(ex.w / v.w, ex.h / v.h) >= LEAVE) return;
    settle(up(state.level - 1, false, AFTER_UP));
  }
}

// level 3 (code): zoom scales the text; far enough out leaves for the functions. A plain wheel on code that
// scrolls keeps scrolling it; pinch (ctrl) always zooms.
function setCodeZoom(z) { AZ.cz = z; $('#sc3').style.setProperty('--cz', z); }
function codeWheel(e) {
  if (state.level !== 3 || state.view !== 'map') return;
  const code = $('#sc3 .code'); if (!code) return;
  if (!isZoomWheel(e)) return; // a plain wheel / two-finger scroll scrolls the code natively
  scaleWheel(e, { z: AZ.cz, set: setCodeZoom, out: () => up(2, false, AFTER_UP) });
}
// Content without a camera (code text, the scenario diagram) zooms by a scale z; crossing a threshold changes the
// level once per gesture, with the same gate and pause as the map: z above SCALE_DIVE dives (o.in), below
// CODE_LEAVE leaves (o.out); without a level to go to the scale stops at CODE_LEAVE / CODE_MAX.
// o: {z, set(z), in?() → Promise, out?() → Promise}. The caller decides whether a plain wheel scrolls instead.
const SCALE_DIVE = DIVE / LEAVE; // ≈ 1.57: as far in as the map's dive/leave hysteresis
function scaleWheel(e, o) {
  if (!isZoomWheel(e)) return; // only pinch / Cmd+wheel / Ctrl+wheel zoom; the rest scrolls the container natively
  e.preventDefault();
  const dy = wheelPx(e).y;
  if (autoBusy() || !dy) return;
  const free = AZ.on ? gate(performance.now()) : true; // the tail of the gesture that changed the level does not zoom on
  if (AZ.on && !free && !AZ.armed) return;
  const z = o.z * Math.exp(-dy * zoomRate(e));
  if (AZ.on && o.out && z < CODE_LEAVE) { if (free) { o.set(1); settle(o.out()); } return; }
  if (AZ.on && o.in && z > SCALE_DIVE) { if (free) { o.set(1); settle(o.in()); } return; }
  o.set(Math.min(CODE_MAX, Math.max(CODE_LEAVE, z)));
}
AM.zoom = { scaleWheel, onGesture, isZoomWheel, get on() { return AZ.on; }, DIVE: SCALE_DIVE, LEAVE: CODE_LEAVE, MAX: CODE_MAX };

/* ---------- debug audit: no foreign panels after a view or level change ---------- */
// auditPanels(label): visible position:absolute/fixed elements (and anything inside an inactive view) that do not
// belong to the active view, the header, the footer, the side panel or an open popup. Warns and returns them.
function auditPanels(label) {
  const act = $('#stage > .view.on');
  const seen = (el) => (el.checkVisibility ? el.checkVisibility({ opacityProperty: true, visibilityProperty: true }) : getComputedStyle(el).visibility !== 'hidden');
  const open = [act, $('header'), $('footer'), $('aside'), $('#crumbs'), $('svg[aria-hidden]')].filter(Boolean);
  const bad = [], name = (el) => (el.id ? '#' + el.id : '') + (el.getAttribute('class') ? '.' + String(el.getAttribute('class')).split(/\s+/).join('.') : '') || el.tagName;
  const add = (el, why) => { const r = el.getBoundingClientRect(); if (r.width > 1 && r.height > 1 && seen(el)) bad.push(why + ': ' + el.tagName.toLowerCase() + name(el)); };
  $$('body *').forEach((el) => {
    if (open.some((o) => o.contains(el))) return;
    const pos = getComputedStyle(el).position;
    if (el.closest('.view')) add(el, 'inactive view'); else if (pos === 'absolute' || pos === 'fixed') add(el, pos);
  });
  if (state.view === 'map' && !state.busy) $$('#v-map .scene').forEach((el, i) => { if (i !== state.level && !el.classList.contains('hidden')) bad.push('scene sc' + i + ' shown on level ' + state.level); });
  if (bad.length) { const u = Array.from(new Set(bad)); console.warn('[archmap debug] foreign elements after ' + label + ':\n' + u.slice(0, 12).join('\n')); (window.__auditWarns = window.__auditWarns || []).push({ label, bad: u }); return u; }
  return [];
}
if (DEBUG) {
  AM.audit = auditPanels;
  bus.on('level', () => setTimeout(() => auditPanels('level:' + state.level), DUR + 150));
}
