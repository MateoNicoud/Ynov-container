/* Statistiques globales : Pattes, Truffes et Panneaux donnés sur le site. */
(function () {
  'use strict';
  var root = document.querySelector('[data-stats]');
  if (!root) return;
  var ROWS = [['patte', 'Pattes'], ['truffe', 'Truffes'], ['panneau', 'Panneaux']];
  function render(c) {
    var tot = c.patte + c.truffe + c.panneau;
    root.innerHTML = ROWS.map(function (r) {
      var k = r[0], w = tot ? Math.round(c[k] / tot * 100) : 0;
      return '<div class="stat stat--' + k + '" role="listitem" style="--a-fill:var(--c-' + k + ');--a-on:var(--c-on-' + k + ')">' +
        '<span class="stat-icon"><svg aria-hidden="true"><use href="#i-' + k + '"/></svg></span><span class="stat-name">' + r[1] + '</span>' +
        '<span class="stat-value">' + c[k] + '</span><span class="stat-bar" aria-hidden="true"><i style="--w:' + w + '%"></i></span></div>';
    }).join('');
  }
  function load() {
    fetch('/api/swipes/stats').then(function (r) { return r.json(); }).then(render).catch(function () {
      root.innerHTML = '<p class="hint">Statistiques indisponibles pour le moment.</p>';
    });
  }
  document.addEventListener('bark:swiped', load);
  load();
})();
