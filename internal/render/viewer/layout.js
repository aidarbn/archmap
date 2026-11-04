/* layout.js — layered layout (Sugiyama-lite), no external libraries.
 *
 * layered(nodes, edges, opt) places equal-size cards in horizontal bands:
 *   nodes  [{id, layer: number, order?: number}] — the band is given (model layer or call depth);
 *   edges  [[a, b]] — links used to order nodes inside the bands (direction does not matter);
 *   opt    {w, h, gx, gy, cols, x0, y0, head, pad, gap, sweeps, anchors: {id: x}}
 *          cols — cards per row; a wider band wraps into rows filled column by column, so nodes
 *          that want the same x share a column; anchors — fixed x of outside nodes (actors,
 *          externals) that pull their neighbours.
 * Ordering: median of neighbour x, alternating down/up sweeps; the order with the fewest edge
 * crossings between band pairs is kept. Returns {pos: {id: {x, y, w, h}}, bands: [{layer, y, h,
 * rows, n}], width, height, crossings}.
 */
function layered(nodes, edges, opt) {
  const o = Object.assign({ w: 196, h: 68, gx: 14, gy: 16, cols: 7, x0: 0, y0: 0, head: 40, pad: 6, gap: 22, sweeps: 8, anchors: {} }, opt);
  const layers = uniq(nodes.map((n) => n.layer)).sort((a, b) => a - b);
  const byL = new Map(layers.map((l) => [l, []]));
  const lay = {}, key0 = {};
  nodes.forEach((n, i) => { byL.get(n.layer).push(n.id); lay[n.id] = n.layer; key0[n.id] = n.order != null ? n.order : i; });
  byL.forEach((list) => list.sort((a, b) => key0[a] - key0[b] || (a < b ? -1 : 1)));
  const nb = {}; nodes.forEach((n) => { nb[n.id] = []; });
  const cross = []; // edges between different bands, for the crossing count
  const seen = new Set();
  edges.forEach(([a, b]) => {
    if (a === b) return;
    const k = a < b ? a + '\0' + b : b + '\0' + a;
    if (seen.has(k)) return; seen.add(k);
    if (nb[a] && nb[b]) {
      if (lay[a] !== lay[b]) { nb[a].push(b); nb[b].push(a); cross.push(lay[a] < lay[b] ? [a, b] : [b, a]); }
    } else if (nb[a] && o.anchors[b] != null) nb[a].push(b);
    else if (nb[b] && o.anchors[a] != null) nb[b].push(a);
  });
  const W = o.cols * (o.w + o.gx) - o.gx;
  const x = {}, rowOf = {}, rowsOf = {};
  const place = (l) => {
    const list = byL.get(l), n = list.length, rows = Math.max(1, Math.ceil(n / o.cols)), ncols = Math.ceil(n / rows);
    const off = (W - (ncols * (o.w + o.gx) - o.gx)) / 2;
    rowsOf[l] = rows;
    list.forEach((id, i) => { const col = Math.floor(i / rows); rowOf[id] = i % rows; x[id] = o.x0 + off + col * (o.w + o.gx) + o.w / 2; });
  };
  layers.forEach(place);
  const xOf = (v) => (x[v] != null ? x[v] : o.anchors[v]);
  const median = (id) => {
    const xs = nb[id].map(xOf).filter((v) => v != null).sort((a, b) => a - b);
    if (!xs.length) return null;
    const m = xs.length >> 1;
    return xs.length % 2 ? xs[m] : (xs[m - 1] + xs[m]) / 2;
  };
  const countCross = () => {
    const buckets = new Map();
    cross.forEach(([a, b]) => { const k = lay[a] + ':' + lay[b]; if (!buckets.has(k)) buckets.set(k, []); buckets.get(k).push([x[a], x[b]]); });
    let c = 0;
    buckets.forEach((es) => {
      for (let i = 0; i < es.length; i++) for (let j = i + 1; j < es.length; j++) {
        if ((es[i][0] - es[j][0]) * (es[i][1] - es[j][1]) < 0) c++;
      }
    });
    return c;
  };
  const snap = () => { const s = new Map(); byL.forEach((list, l) => s.set(l, list.slice())); return s; };
  let best = snap(), bestC = countCross();
  const sweep = (order) => order.forEach((l) => {
    const list = byL.get(l), key = {};
    list.forEach((id) => { const m = median(id); key[id] = m == null ? x[id] : m; });
    list.sort((a, b) => key[a] - key[b] || x[a] - x[b]);
    place(l);
  });
  for (let it = 0; it < o.sweeps && bestC > 0; it++) {
    sweep(it % 2 ? layers.slice().reverse() : layers);
    const c = countCross();
    if (c < bestC) { bestC = c; best = snap(); }
  }
  best.forEach((list, l) => { byL.set(l, list); place(l); });
  // y: bands one under another
  const pos = {}, bands = [];
  let y = o.y0;
  layers.forEach((l) => {
    const rows = rowsOf[l], h = o.head + rows * (o.h + o.gy) - o.gy + o.pad + 10;
    bands.push({ layer: l, y, h, rows, n: byL.get(l).length });
    byL.get(l).forEach((id) => { pos[id] = { x: x[id] - o.w / 2, y: y + o.head + rowOf[id] * (o.h + o.gy), w: o.w, h: o.h }; });
    y += h + o.gap;
  });
  return { pos, bands, width: W, height: y - o.gap - o.y0, crossings: bestC, order: byL };
}

AM.layered = layered;
