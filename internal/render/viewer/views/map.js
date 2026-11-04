/* views/map.js — the map tab. The scene itself lives in the core (map.js) because playback and
 * the panel drive it; this file only registers it, and doubles as the smallest example of a view. */
AM.registerView({
  id: 'map',
  order: 0,
  title: () => AM.t('viewMap'),
  mount() { /* the map markup is part of shell.html (#v-map) and is built at start-up */ },
  show(ctx) { ctx.renderPanel(); },
});
