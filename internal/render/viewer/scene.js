/* scene.js — camera over an SVG viewBox, moving dot, pulses, geometry, level transitions. */

/* ---------- camera ---------- */
// new Cam(svg, onChange): drag to pan, wheel to zoom; set/fit/animTo work in world coordinates.
class Cam {
  constructor(svg, onChange) {
    this.svg = svg; this.vb = { x: 0, y: 0, w: 100, h: 100 }; this.onChange = onChange; this.maxW = 1e5; this.minW = 120;
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
    svg.addEventListener('wheel', (e) => {
      e.preventDefault();
      const r = svg.getBoundingClientRect();
      this.zoomAt((e.clientX - r.left) / r.width, (e.clientY - r.top) / r.height, Math.exp(e.deltaY * (e.ctrlKey ? 0.01 : 0.0015)));
    }, { passive: false });
  }
  aspect() { const p = this.svg.parentElement; return (p.offsetWidth || 1000) / (p.offsetHeight || 700); }
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
    const a = this.aspect();
    let w = Math.max(r.w + pad * 2, minW), h = r.h + pad * 2;
    if (w / h > a) h = w / a; else w = h * a;
    return { x: r.x + r.w / 2 - w / 2, y: r.y + r.h / 2 - h / 2, w, h };
  }
  fit(r, pad, minW) { this.set(this.rectFor(r, pad, minW)); }
  // fitWidth: tall content — fit by width, top-aligned (scroll by panning)
  fitTop(r, pad = 20, maxScaleW = 0) {
    const a = this.aspect();
    let w = Math.max(r.w + pad * 2, maxScaleW), h = w / a;
    if (h >= r.h + pad * 2) return this.fit(r, pad, maxScaleW);
    this.set({ x: r.x + r.w / 2 - w / 2, y: r.y - pad, w, h });
  }
  // contains(r): the rect is fully visible
  contains(r, m = 0) { const v = this.vb; return r.x >= v.x + m && r.y >= v.y + m && r.x + r.w <= v.x + v.w - m && r.y + r.h <= v.y + v.h - m; }
  animTo(r, pad = 40, minW = 0, dur = DUR) { return this.animToVB(this.rectFor(r, pad, minW), dur); }
  animToVB(to, dur = DUR) {
    const from = { ...this.vb };
    cancelAnimationFrame(this.raf);
    if (!dur) { this.set(to); return Promise.resolve(); }
    return new Promise((res) => {
      const t0 = performance.now();
      const tick = (tm) => {
        const k = ease(Math.min(1, (tm - t0) / dur));
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

/* ---------- playback marks ---------- */
// runDot(layer, d, {async, label}): animates the accent dot along path d; async — dotted line
// with a pause mid-way (the job waits in the queue). Resolves when the dot arrives.
function runDot(layer, d, opt = {}) {
  const p = S_('path', { d, class: 'e hot' + (opt.async ? ' dash' : ''), 'marker-end': 'url(#arrA)' }, layer);
  const c = S_('circle', { r: 6, class: 'dot' }, layer);
  const len = p.getTotalLength();
  const D_ = RM ? 0 : (opt.dur || 650) / state.speed;
  const put = (k) => { const q = p.getPointAtLength(len * k); c.setAttribute('cx', q.x); c.setAttribute('cy', q.y); return q; };
  if (opt.async) { const q = p.getPointAtLength(len / 2); S_('text', { x: q.x, y: q.y - 12, 'text-anchor': 'middle', class: 'qtag' }, layer, '⏸ ' + t('queue')); }
  if (!D_) { put(1); return Promise.resolve(); }
  const leg = (k0, k1, dur) => new Promise((res) => {
    const t0 = performance.now();
    const tick = (tm) => { const k = ease(Math.min(1, (tm - t0) / dur)); put(k0 + (k1 - k0) * k); if (k < 1) requestAnimationFrame(tick); else res(); };
    requestAnimationFrame(tick);
  });
  if (!opt.async) return leg(0, 1, D_);
  return leg(0, .5, D_ / 2).then(() => sleep(450 / state.speed)).then(() => leg(.5, 1, D_ / 2));
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

Object.assign(AM, { Cam, curve, clip, ctr, hop, bbox, runDot, pulse });
