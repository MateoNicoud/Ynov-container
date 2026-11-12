/* Carrousel des photos : défile seul, avec boutons précédent, pause et suivant. */
(function () {
  'use strict';
  var root = document.querySelector('[data-carousel]');
  if (!root) return;
  var img = root.querySelector('[data-car-img]'), cap = root.querySelector('[data-car-cap]'), pauseBtn = root.querySelector('[data-car-pause]');
  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)');
  var items = [], idx = 0, timer = null, paused = reduce.matches;

  function show(i) {
    if (!items.length) return;
    idx = (i + items.length) % items.length;
    var p = items[idx];
    img.src = p.photo; img.alt = p.name + ', ' + p.breed;
    cap.textContent = p.name + ', ' + p.breed;
  }
  function start() { stop(); if (!paused && items.length > 1) timer = setInterval(function () { show(idx + 1); }, 4000); }
  function stop() { if (timer) { clearInterval(timer); timer = null; } }
  function syncPause() {
    pauseBtn.setAttribute('aria-pressed', paused ? 'true' : 'false');
    pauseBtn.textContent = paused ? 'Reprendre' : 'Pause';
    root.querySelector('[data-car-live]').setAttribute('aria-live', paused ? 'polite' : 'off');
  }
  root.addEventListener('click', function (e) {
    if (e.target.closest('[data-car-prev]')) { show(idx - 1); start(); }
    if (e.target.closest('[data-car-next]')) { show(idx + 1); start(); }
    if (e.target.closest('[data-car-pause]')) { paused = !paused; syncPause(); start(); }
  });
  root.addEventListener('focusin', stop);
  root.addEventListener('focusout', start);
  document.addEventListener('bark:profiles', function (e) {
    items = e.detail; root.hidden = !items.length; syncPause(); show(0); start();
  });
})();
