/* scene.js — camera over an SVG viewBox, moving dot, pulses, geometry, level transitions. */

/* ---------- wheel gestures (Figma / maps style) ---------- */
// Zoom: pinch (macOS sends wheel with ctrlKey), Cmd/Ctrl+wheel, Safari gesture events. Everything else
// (two-finger scroll, mouse wheel, Shift+wheel) moves the content and never changes the scale or the level.
const isZoomWheel = (e) => !!(e.ctrlKey || e.metaKey);
const isPinch = (e) => e.ctrlKey && !e.metaKey && e.deltaMode === 0;
const zoomRate = (e) => (isPinch(e) ? 0.01 : 0.003); // per pixel: pinch deltas are small, Cmd+wheel ones are steps
// wheel delta in pixels (deltaMode 1 — lines of a mouse wheel, 2 — pages)
function wheelPx(e) {
  const m = e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? 600 : 1;
  return { x: e.deltaX * m, y: e.deltaY * m };
}
// Safari: gesturestart/gesturechange carry a cumulative scale; hand fn a wheel-like pinch event per change
function onGesture(el, fn) {
  let last = 1;
  el.addEventListener('gesturestart', (e) => { e.preventDefault(); last = 1; });
  el.addEventListener('gesturechange', (e) => {
    e.preventDefault();
    const f = e.scale / last; last = e.scale;
    fn({ ctrlKey: true, metaKey: false, shiftKey: false, deltaMode: 0, deltaX: 0, deltaY: -Math.log(f) / 0.01, clientX: e.clientX, clientY: e.clientY, preventDefault() {} });
  });
}

/* ---------- camera ---------- */
// new Cam(svg, onChange): drag to pan, wheel to zoom; set/fit/animTo work in world coordinates.
class Cam {
  constructor(svg, onChange) {
    this.svg = svg; this.vb = { x: 0, y: 0, w: 100, h: 100 }; this.onChange = onChange; this.maxW = 1e5; this.minW = 120; this.insetTop = 0; this.insetBottom = 0; this.insetRight = 0; // px; the map scenes set them to clear their overlays
    let drag = null;
    svg.addEventListener('pointerdown', (e) => { if (e.button !== 0) return; drag = { x: e.clientX, y: e.clientY, vb: { ...this.vb }, moved: false }; });
    window.addEventListener('pointermove', (e) => {
      if (!drag) return;
      const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
      if (!drag.moved && Math.hypot(dx, dy) < 4) return;
      drag.moved = true; svg.style.cursor = 'grabbing';
      const k = this.vb.w / (svg.parentElement.offsetWidth || 1000);
      this.set({ ...drag.vb, x: drag.vb.x - dx * k, y: drag.vb.y - dy * k });
    });
    window.addEventListener('pointerup', () => { if (drag && drag.moved) { this.justDragged = true; setTimeout(() => (this.justDragged = false), 0); } drag = null; svg.style.cursor = ''; });
    const onWheel = (e) => {
      e.preventDefault();
      if (autoBusy()) return; // a level transition is running: the wheel does not move the camera under it
      const r = svg.getBoundingClientRect();
      if (isZoomWheel(e)) { // pinch / Cmd+wheel / Ctrl+wheel: zoom to the cursor, may dive
        this.zoomAt((e.clientX - r.left) / r.width, (e.clientY - r.top) / r.height, Math.exp(wheelPx(e).y * zoomRate(e)));
        semanticZoom(this, e); // zoom.js: dive into / out of a level when the zoom crosses a threshold
      } else { // two-finger scroll / wheel: pan only, never zooms or changes the level
        const d = wheelPx(e), k = this.vb.w / (r.width || 1);
        const sx = e.shiftKey && !d.x ? d.y : d.x, sy = e.shiftKey && !d.x ? 0 : d.y; // Shift+wheel — sideways
        this.set({ ...this.vb, x: this.vb.x + sx * k, y: this.vb.y + sy * k });
      }
    };
    svg.addEventListener('wheel', onWheel, { passive: false });
    onGesture(svg, onWheel); // Safari pinch
  }
  // view box in which rect r (world) takes `ratio` of the free area (by its larger side), centred
  rectForRatio(r, ratio) {
    const a = this.freeAspect(), w = Math.max(r.w, r.h * a) / ratio, h = w / a;
    return this.boxBelow(r.x + r.w / 2 - w / 2, r.y + r.h / 2 - h / 2, w, h);
  }
  // world rect of an element of this scene (via its screen box; the view box keeps the stage aspect)
  worldRect(el) {
    const r = el.getBoundingClientRect(), s = this.svg.getBoundingClientRect(), k = this.vb.w / (s.width || 1);
    return { x: this.vb.x + (r.left - s.left) * k, y: this.vb.y + (r.top - s.top) * k, w: r.width * k, h: r.height * k };
  }
  aspect() { const p = this.svg.parentElement; return (p.offsetWidth || 1000) / (p.offsetHeight || 700); }
  // the strips covered by overlays: insetTop (crumbs, level buttons), insetBottom / insetRight (the sequence dock) —
  // content is fitted into the free area that is left; each is capped so that the free area stays usable
  size() { const p = this.svg.parentElement; return { W: p.offsetWidth || 1000, H: p.offsetHeight || 700 }; }
  insetPx() { return Math.min(this.insetTop, this.size().H * 0.25); }
  insetBPx() { return Math.min(this.insetBottom, this.size().H * 0.6); }
  insetRPx() { return Math.min(this.insetRight, this.size().W * 0.6); }
  // aspect of the free area (inside the insets)
  freeAspect() { const { W, H } = this.size(); return (W - this.insetRPx()) / (H - this.insetPx() - this.insetBPx()); }
  // view box whose free area is w x h (world) and starts at world (x, y0); the free area is the top-left part minus the top strip
  boxBelow(x, y0, w, h) {
    const { W, H } = this.size(), iT = this.insetPx(), fw = w * W / (W - this.insetRPx()), fh = h * H / (H - iT - this.insetBPx());
    return { x, y: y0 - fh * iT / H, w: fw, h: fh };
  }
  set(vb) {
    this.vb = vb;
    this.svg.setAttribute('viewBox', `${vb.x} ${vb.y} ${vb.w} ${vb.h}`);
    if (this.onChange) this.onChange(vb);
  }
  zoomAt(px, py, f) {
    const v = this.vb, w = Math.min(Math.max(v.w * f, this.minW), this.maxW), h = w / this.aspect();
    this.set({ x: v.x + (v.w - w) * px, y: v.y + (v.h - h) * py, w, h });
  }
  rectFor(r, pad = 40, minW = 0) {
    const a = this.freeAspect();
    let w = Math.max(r.w + pad * 2, minW), h = r.h + pad * 2;
    if (w / h > a) h = w / a; else w = h * a;
    return this.boxBelow(r.x + r.w / 2 - w / 2, r.y + r.h / 2 - h / 2, w, h);
  }
  fit(r, pad, minW) { this.set(this.rectFor(r, pad, minW)); }
  // fitWidth: tall content — fit by width, top-aligned (scroll by panning)
  fitTop(r, pad = 20, maxScaleW = 0) {
    const a = this.freeAspect();
    let w = Math.max(r.w + pad * 2, maxScaleW), h = w / a;
    if (h >= r.h + pad * 2) return this.fit(r, pad, maxScaleW);
    this.set(this.boxBelow(r.x + r.w / 2 - w / 2, r.y - pad, w, h));
  }
  // contains(r): the rect is fully visible (below the inset)
  contains(r, m = 0) {
    const v = this.vb, k = v.w / this.size().W, top = v.y + this.insetPx() * k;
    return r.x >= v.x + m && r.y >= top + m && r.x + r.w <= v.x + v.w - this.insetRPx() * k - m && r.y + r.h <= v.y + v.h - this.insetBPx() * k - m;
  }
  insetWorld(w) { return this.insetPx() * w / this.size().W; }
  // the part of the scene on screen that no overlay covers (world)
  freeRect() {
    const v = this.vb, k = v.w / this.size().W, t = this.insetPx() * k;
    return { x: v.x, y: v.y + t, w: v.w - this.insetRPx() * k, h: v.h - t - this.insetBPx() * k };
  }
  animTo(r, pad = 40, minW = 0, dur = DUR) { return this.animToVB(this.rectFor(r, pad, minW), dur); }
  animToVB(to, dur = DUR) {
    const from = { ...this.vb };
    cancelAnimationFrame(this.raf);
    if (!dur || HURRY.on) { this.set(to); return Promise.resolve(); }
    return new Promise((res) => {
      const t0 = performance.now();
      const tick = (tm) => {
        const k = HURRY.on ? 1 : ease(Math.min(1, (tm - t0) / dur)); // a hurried step ends at once
        this.set({ x: from.x + (to.x - from.x) * k, y: from.y + (to.y - from.y) * k, w: from.w + (to.w - from.w) * k, h: from.h + (to.h - from.h) * k });
        if (k < 1) this.raf = requestAnimationFrame(tick); else res();
      };
      this.raf = requestAnimationFrame(tick);
    });
  }
}

/* ---------- geometry ---------- */
const ctr = (r) => ({ x: r.x + r.w / 2, y: r.y + r.h / 2 });
// curve(a, b, bend): quadratic path from a to b bent sideways
const curve = (a, b, bend = 0.18) => {
  const mx = (a.x + b.x) / 2, my = (a.y + b.y) / 2, dx = b.x - a.x, dy = b.y - a.y;
  return `M${a.x},${a.y} Q${mx - dy * bend},${my + dx * bend} ${b.x},${b.y}`;
};
// clip(r, p): point on the border of rect r toward p
function clip(r, p) {
  const cx = r.x + r.w / 2, cy = r.y + r.h / 2, dx = p.x - cx, dy = p.y - cy;
  if (!dx && !dy) return { x: cx, y: cy };
  const s = Math.min(Math.abs((r.w / 2) / (dx || 1e-9)), Math.abs((r.h / 2) / (dy || 1e-9)));
  return { x: cx + dx * s, y: cy + dy * s };
}
const hop = (ra, rb, bend = 0.12) => curve(clip(ra, ctr(rb)), clip(rb, ctr(ra)), bend);
const bbox = (rs) => {
  rs = rs.filter(Boolean);
  if (!rs.length) return { x: 0, y: 0, w: 100, h: 100 };
  const x = Math.min(...rs.map((r) => r.x)), y = Math.min(...rs.map((r) => r.y));
  return { x, y, w: Math.max(...rs.map((r) => r.x + r.w)) - x, h: Math.max(...rs.map((r) => r.y + r.h)) - y };
};

// the area a rect r shares with the rects of a list
function overArea(r, list) {
  let s = 0;
  list.forEach((q) => {
    const w = Math.min(r.x + r.w, q.x + q.w) - Math.max(r.x, q.x), h = Math.min(r.y + r.h, q.y + q.h) - Math.max(r.y, q.y);
    if (w > 0 && h > 0) s += w * h;
  });
  return s;
}
// placeLabel(o): the spot of a label w x h (the pill of a step) that belongs to a line. Candidates: on the line
// (o.along — points on it, in the order they are preferred) and around it (o.anchors — points on the line; above,
// below, to the left and to the right of each at growing distances). Each is scored by what it covers — o.avoid:
// [{rects, k}], k = the cost of covering a rect whole — and by how much of it is outside o.view; a spot off
// the line costs more (it needs a leader line to its anchor), the farther the more (o.offPen: extra for any spot
// off the line). → {x, y (centre), a (the
// anchor on the line), off}
function placeLabel(o) {
  const { w, h } = o, g0 = o.gap || 8, A0 = w * h || 1, cands = [];
  (o.along || []).forEach((p, k) => cands.push({ x: p.x, y: p.y, a: p, pen: k * 2, off: false }));
  (o.anchors || []).forEach((a, k) => {
    [0.5, 1, 2, 3.5, 5, 7, 10, 14].forEach((m, mi) => {
      const g = g0 * m, base = 30 + (o.offPen || 0) + k * 10 + mi * 14;
      [-1, 1].forEach((sy) => [0, 0.45, -0.45, 0.25, -0.25].forEach((fx, n) => cands.push({ x: a.x + fx * w, y: a.y + sy * (h / 2 + g), a, pen: base + n * 4, off: true })));
      [-1, 1].forEach((sx) => [0, -1, 1, -2, 2].forEach((fy, n) => cands.push({ x: a.x + sx * (w / 2 + g), y: a.y + fy * (h / 2 + g0), a, pen: base + 8 + n * 4, off: true })));
    });
  });
  let best = null, bs = Infinity;
  const score = (c) => {
    const r = { x: c.x - w / 2, y: c.y - h / 2, w, h };
    let s = c.pen;
    // each covered rect counts by the share of the smaller of the two that is covered: a small number under a long
    // pill costs as much as a long pill over a card
    // (plus a fixed cost per touched rect: a far clean spot beats a near one that clips a corner)
    (o.avoid || []).forEach((av) => av.rects.forEach((q) => { const a = overArea(r, [q]); if (a > 0) s += (a / Math.min(A0, q.w * q.h || A0)) * 100 * av.k + 15 * av.k; }));
    // off screen (even a sliver) is worse than anything else on screen
    if (o.view) { const out = A0 - overArea(r, [o.view]); if (out > 0.5) s += 300 + (out / A0) * 100 * (o.viewK || 8); }
    return s;
  };
  const scored = cands.map((c) => ({ c, s: score(c) })).sort((p, q) => p.s - q.s);
  if (scored.length) { best = scored[0].c; bs = scored[0].s; }
  // the best few that still cover something: slide each just clear of what it covers (above / below / beside the
  // rect), twice — a narrow free strip between rows of cards is found this way
  const hard = (o.avoid || []).filter((av) => av.k >= 1).flatMap((av) => av.rects);
  const covers = (c) => { const r = { x: c.x - w / 2, y: c.y - h / 2, w, h }; return hard.some((q) => overArea(r, [q]) > 0); };
  let seeds = best && covers(best) ? scored.slice(0, 12).map((x) => x.c) : [];
  for (let round = 0; round < 2 && seeds.length; round++) {
    const next = [];
    seeds.forEach((c) => {
      const r = { x: c.x - w / 2, y: c.y - h / 2, w, h };
      hard.forEach((q) => {
        if (overArea(r, [q]) <= 0) return;
        [{ x: c.x, y: q.y - h / 2 - 1 }, { x: c.x, y: q.y + q.h + h / 2 + 1 }, { x: q.x - w / 2 - 1, y: c.y }, { x: q.x + q.w + w / 2 + 1, y: c.y }].forEach((p) => {
          const d = Math.hypot(p.x - c.a.x, p.y - c.a.y), v = { x: p.x, y: p.y, a: c.a, pen: 30 + (o.offPen || 0) + d / Math.max(1, g0), off: true }, s = score(v);
          next.push(v);
          if (s < bs) { bs = s; best = v; }
        });
      });
    });
    seeds = next.sort((p, q) => score(p) - score(q)).slice(0, 12);
  }
  // a spot beside the line may still clip an edge: nudge it a little where that covers less
  if (best && best.off) {
    const b0 = best;
    [-1, 1].forEach((sy) => [0.15, 0.3, 0.5].forEach((f) => [0, -0.06, 0.06].forEach((fx) => {
      const c = { x: b0.x + fx * w, y: b0.y + sy * f * h, a: b0.a, pen: b0.pen + 1, off: true }, s = score(c);
      if (s < bs) { bs = s; best = c; }
    })));
  }
  return best;
}
// the leader of a label put off its line: from the anchor a to the nearest point of the label rect (centre p, w x h);
// null when the anchor is (almost) under the label
function leader(a, p, w, h, min = 3) {
  const x = Math.max(p.x - w / 2, Math.min(a.x, p.x + w / 2)), y = Math.max(p.y - h / 2, Math.min(a.y, p.y + h / 2));
  return Math.hypot(x - a.x, y - a.y) < min ? null : `M${a.x},${a.y} L${x},${y}`;
}

/* ---------- playback marks ---------- */
// HURRY.on: a step was pressed while the previous one still animates — every animation of that step ends at once
// (the dot jumps to its end, the camera to its target); HURRY.anims: the running Web Animations of the step
const HURRY = { on: false, anims: new Set() };
function hurry() { HURRY.on = true; HURRY.anims.forEach((a) => { try { a.finish(); } catch (e) { /* no end: infinite */ } }); HURRY.anims.clear(); }
// a Web Animation the next press may cut short; resolves when it ends
function hurryable(anim) {
  if (!anim) return Promise.resolve();
  if (HURRY.on) { try { anim.finish(); } catch (e) { /* ignore */ } return Promise.resolve(); }
  HURRY.anims.add(anim);
  return anim.finished.catch(() => {}).finally(() => HURRY.anims.delete(anim));
}
// runDot(layer, d, {async, label}): animates the accent dot along path d; async — dotted line
// with a pause mid-way (the job waits in the queue); ghost — the line is not drawn (the path overlay already shows
// the leg), only the dot runs. Resolves when the dot arrives (at once when the step is hurried).
function runDot(layer, d, opt = {}) {
  const p = opt.ghost ? S_('path', { d, class: 'e ghost' }, layer) : S_('path', { d, class: 'e hot' + (opt.async ? ' dash' : ''), 'marker-end': 'url(#arrA)' }, layer);
  const c = S_('circle', { r: 6, class: 'dot' }, layer);
  const len = p.getTotalLength();
  const D_ = RM ? 0 : (opt.dur || 650) / state.speed;
  const put = (k) => { const q = p.getPointAtLength(len * k); c.setAttribute('cx', q.x); c.setAttribute('cy', q.y); return q; };
  if (opt.async) { const q = p.getPointAtLength(len / 2); S_('text', { x: q.x, y: q.y - 12, 'text-anchor': 'middle', class: 'qtag' }, layer, '⏸ ' + t('queue')); }
  if (!D_ || HURRY.on) { put(1); return Promise.resolve(); }
  const leg = (k0, k1, dur) => new Promise((res) => {
    const t0 = performance.now();
    const tick = (tm) => { const k = HURRY.on ? 1 : ease(Math.min(1, (tm - t0) / dur)); put(k0 + (k1 - k0) * k); if (k < 1) requestAnimationFrame(tick); else res(); };
    requestAnimationFrame(tick);
  });
  if (!opt.async) return leg(0, 1, D_);
  // the pause in the queue is a leg that stays: a hurried step skips it too
  return leg(0, .5, D_ / 2).then(() => leg(.5, .5, 450 / state.speed)).then(() => leg(.5, 1, D_ / 2));
}
function pulse(layer, r) {
  const e = S_('rect', { x: r.x - 6, y: r.y - 6, width: r.w + 12, height: r.h + 12, rx: 14, class: 'pulse flash' }, layer);
  setTimeout(() => e.remove(), 950);
}

/* ---------- level transitions (semantic zoom) ---------- */
const SC = () => [$('#sc0'), $('#sc1'), $('#sc2'), $('#sc3')];
const stageRect = () => $('#v-map').getBoundingClientRect();
function relRect(el) {
  const s = stageRect(), r = el.getBoundingClientRect();
  return { x: r.left - s.left, y: r.top - s.top, w: r.width, h: r.height };
}
const midRect = () => { const s = stageRect(); return { x: s.width / 2 - 60, y: s.height / 2 - 40, w: 120, h: 80 }; };
function tfFor(r) { // maps the full stage into rect r
  const s = stageRect(), k = Math.min(r.w / s.width, r.h / s.height);
  return `translate(${r.x + r.w / 2 - (s.width * k) / 2}px,${r.y + r.h / 2 - (s.height * k) / 2}px) scale(${k})`;
}
function tfZoomInto(r) { // zooms the current scene so rect r fills the stage
  const s = stageRect(), k = Math.min(s.width / r.w, s.height / r.h) * 0.9;
  return `translate(${s.width / 2 - (r.x + r.w / 2) * k}px,${s.height / 2 - (r.y + r.h / 2) * k}px) scale(${k})`;
}
async function zoomSwap(fromIdx, toIdx, rect, dir) {
  const sc = SC(), A = sc[fromIdx], B = sc[toIdx];
  B.classList.remove('hidden');
  if (A === B) return;
  if (!DUR || !A.animate) { A.classList.add('hidden'); return; }
  const opt = { duration: DUR + 60, easing: 'cubic-bezier(.45,0,.2,1)', fill: 'none' };
  let a1, a2;
  if (dir === 'in') {
    a1 = A.animate([{ transform: 'none', opacity: 1 }, { transform: tfZoomInto(rect), opacity: 0 }], opt);
    a2 = B.animate([{ transform: tfFor(rect), opacity: 0 }, { transform: 'none', opacity: 1 }], opt);
  } else {
    a1 = A.animate([{ transform: 'none', opacity: 1 }, { transform: tfFor(rect), opacity: 0 }], opt);
    a2 = B.animate([{ transform: tfZoomInto(rect), opacity: 0 }, { transform: 'none', opacity: 1 }], opt);
  }
  await Promise.all([a1.finished, a2.finished]);
  A.classList.add('hidden');
}

Object.assign(AM, { Cam, curve, clip, ctr, hop, bbox, runDot, pulse, overArea, placeLabel, leader });
