/* Onglets Découverte / Éleveurs, Mode daltochien et bannière d'entrée. */
(function () {
  'use strict';
  var root = document.documentElement;
  function $(s) { return document.querySelector(s); }
  function get(s, k) { try { return s.getItem(k); } catch (e) { return null; } }
  function set(s, k, v) { try { s.setItem(k, v); } catch (e) {} }

  /* Mode daltochien : data-mode sur <html>, choix mémorisé (voir design/ACCESSIBILITE.md) */
  var KEY = 'bark4me-mode', sw = $('[data-switch]');
  function setMode(m) {
    if (m === 'daltochien') root.setAttribute('data-mode', 'daltochien'); else root.removeAttribute('data-mode');
    sw.setAttribute('aria-checked', m === 'daltochien' ? 'true' : 'false');
    set(localStorage, KEY, m);
  }
  setMode(get(localStorage, KEY) === 'daltochien' ? 'daltochien' : 'normal');
  sw.addEventListener('click', function () { setMode(sw.getAttribute('aria-checked') === 'true' ? 'normal' : 'daltochien'); });

  /* Bannière d'entrée : la parodie est annoncée avant tout */
  var intro = $('[data-intro]'), SEEN = 'bark4me-intro';
  if (intro && get(sessionStorage, SEEN) !== '1') {
    if (intro.showModal) intro.showModal(); else intro.setAttribute('open', '');
    intro.addEventListener('cancel', function (e) { e.preventDefault(); });
  }
  $('[data-intro-ok]').addEventListener('click', function () {
    set(sessionStorage, SEEN, '1');
    if (intro.close) intro.close(); else intro.removeAttribute('open');
  });

  /* Onglets */
  var tabs = Array.prototype.slice.call(document.querySelectorAll('[role="tab"]'));
  function showTab(id) {
    tabs.forEach(function (t) {
      var on = t.id === 'tab-' + id;
      t.setAttribute('aria-selected', on ? 'true' : 'false');
      t.tabIndex = on ? 0 : -1;
      document.getElementById(t.getAttribute('aria-controls')).hidden = !on;
    });
  }
  tabs.forEach(function (t, i) {
    t.addEventListener('click', function () { showTab(t.id.replace('tab-', '')); });
    t.addEventListener('keydown', function (e) {
      var j = e.key === 'ArrowRight' ? (i + 1) % tabs.length : e.key === 'ArrowLeft' ? (i + tabs.length - 1) % tabs.length : -1;
      if (j >= 0) { e.preventDefault(); tabs[j].focus(); showTab(tabs[j].id.replace('tab-', '')); }
    });
  });
})();
