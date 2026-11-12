/* Bark4me : script de la page de présentation. JS vanilla, aucune dépendance. */
(function () {
  'use strict';
  var root = document.documentElement;
  var KEY = 'bark4me-mode';

  /* Mode daltochien : interrupteur, mémorisé côté navigateur */
  function readMode() { try { return localStorage.getItem(KEY); } catch (e) { return null; } }
  function setMode(mode) {
    if (mode === 'daltochien') root.setAttribute('data-mode', 'daltochien'); else root.removeAttribute('data-mode');
    document.querySelectorAll('[data-switch]').forEach(function (b) { b.setAttribute('aria-checked', mode === 'daltochien' ? 'true' : 'false'); });
    try { localStorage.setItem(KEY, mode); } catch (e) {}
  }
  setMode(readMode() === 'daltochien' ? 'daltochien' : 'normal');
  document.addEventListener('click', function (e) {
    var sw = e.target.closest('[data-switch]');
    if (sw) setMode(sw.getAttribute('aria-checked') === 'true' ? 'normal' : 'daltochien');
  });

  /* Nuanciers et contrastes, lus dans les variables CSS de chaque mode */
  var COLORS = ['bg', 'surface', 'surface-2', 'ink', 'ink-soft', 'line', 'brand', 'on-brand', 'patte', 'on-patte', 'patte-line', 'truffe', 'on-truffe', 'truffe-line', 'panneau', 'on-panneau', 'panneau-line', 'focus', 'badge-bg', 'badge-ink', 'badge-line', 'match-a', 'match-b'];
  function cv(el, name) { return getComputedStyle(el).getPropertyValue('--c-' + name).trim(); }
  function lum(hex) {
    var h = hex.replace('#', ''), c = [0, 2, 4].map(function (i) { var v = parseInt(h.substr(i, 2), 16) / 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); });
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
  }
  function ratio(a, b) { var x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); }

  var palettes = document.querySelectorAll('[data-palette]');
  palettes.forEach(function (p) {
    var box = p.querySelector('.swatches');
    COLORS.forEach(function (n) {
      var d = document.createElement('div'); d.className = 'sw';
      d.innerHTML = '<i style="background:var(--c-' + n + ')"></i><div><code>--c-' + n + '</code><span>' + cv(p, n) + '</span></div>';
      box.appendChild(d);
    });
  });

  var PAIRS = [
    ['ink', 'bg', 4.5, 'Texte sur fond'], ['ink', 'surface', 4.5, 'Texte sur carte'], ['ink', 'surface-2', 4.5, 'Texte sur fond alterné'],
    ['ink-soft', 'bg', 4.5, 'Texte secondaire sur fond'], ['ink-soft', 'surface', 4.5, 'Texte secondaire sur carte'], ['ink-soft', 'surface-2', 4.5, 'Texte secondaire sur fond alterné'],
    ['on-brand', 'brand', 4.5, 'Texte sur couleur de marque'], ['brand', 'bg', 4.5, 'Lien de marque sur fond'],
    ['on-patte', 'patte', 4.5, 'Icône Patte, survolée'], ['on-truffe', 'truffe', 4.5, 'Icône Truffe, survolée'], ['on-panneau', 'panneau', 4.5, 'Icône Panneau, survolée'],
    ['patte-line', 'surface', 3, 'Icône Patte, normale'], ['truffe-line', 'surface', 3, 'Icône Truffe, normale'], ['panneau-line', 'surface', 3, 'Icône Panneau, normale'],
    ['badge-ink', 'badge-bg', 4.5, 'Texte du badge Pédigrée'], ['badge-line', 'badge-bg', 3, 'Bordure du badge'],
    ['on-match', 'match-a', 4.5, 'Texte du Match (début du dégradé)'], ['on-match', 'match-b', 4.5, 'Texte du Match (fin du dégradé)'],
    ['line', 'bg', 3, 'Bordure de composant'], ['focus', 'bg', 3, 'Anneau de focus sur fond'], ['focus', 'surface', 3, 'Anneau de focus sur carte']
  ];
  var tb = document.querySelector('[data-contrast] tbody');
  if (tb && palettes.length === 2) {
    PAIRS.forEach(function (p) {
      var tr = document.createElement('tr'), cells = '<th scope="row">' + p[3] + ' <code>' + p[0] + ' / ' + p[1] + '</code></th><td>' + p[2] + ':1</td>';
      palettes.forEach(function (pal) {
        var r = ratio(cv(pal, p[0]), cv(pal, p[1]));
        cells += '<td class="' + (r >= p[2] ? 'ok' : 'ko') + '">' + r.toFixed(2).replace('.', ',') + ':1</td>';
      });
      tr.innerHTML = cells; tb.appendChild(tr);
    });
  }

  /* Démo du Paquet */
  var DECK = [
    ['chien-01.jpg', 'Jean-Baptiste', 'Bouledogue français', 'Béret, baguette, aucune excuse.', true],
    ['chien-02.jpg', 'Maurice', 'Barzoï', 'Je conduis. Vous marchez.', false],
    ['chien-03.jpg', 'Mage Gérard', 'Carlin', 'Niveau 40. Cherche quête secondaire.', true],
    ['chien-04.jpg', 'Brutus', 'Rottweiler', 'Jamais sauté le jour des pattes.', false],
    ['chien-05.jpg', 'Monique', 'Bouledogue anglais', 'Canapé, chips, pas de regrets.', false]
  ];
  var demo = document.querySelector('[data-demo]');
  if (!demo) return;
  var slot = demo.querySelector('[data-slot]'), controls = demo.querySelector('[data-controls]'), live = demo.querySelector('[data-live]');
  var counts = { patte: 0, truffe: 0, panneau: 0 }, i = 0, busy = false;
  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)');
  var LABEL = { patte: 'Patte', truffe: 'Truffe', panneau: 'Panneau' };
  var tpl = {
    badge: '<span class="card-badge"><span class="badge"><svg aria-hidden="true"><use href="#i-seal"/></svg>Pédigrée validé</span></span>'
  };
  function esc(s) { return s.replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]; }); }
  function cardHtml(d) {
    return '<article class="card" tabindex="-1" aria-label="Profil de ' + esc(d[1]) + '"><img src="../app/photos/images/' + d[0] + '" alt="' + esc(d[1] + ', ' + d[2] + ', ' + d[3]) + '" width="800" height="1000">' +
      (d[4] ? tpl.badge : '') +
      '<span class="card-stamp card-stamp--patte" aria-hidden="true">Patte</span><span class="card-stamp card-stamp--panneau" aria-hidden="true">Panneau</span><span class="card-stamp card-stamp--truffe" aria-hidden="true">Truffe</span>' +
      '<div class="card-body"><h3 class="card-name">' + esc(d[1]) + '</h3><p class="card-breed">' + esc(d[2]) + '</p><p class="card-tagline">' + esc(d[3]) + '</p></div></article>';
  }
  function show() { slot.innerHTML = cardHtml(DECK[i]); }
  function updateStats() {
    var tot = counts.patte + counts.truffe + counts.panneau || 1;
    demo.querySelectorAll('.stat').forEach(function (s, k) {
      var key = ['patte', 'truffe', 'panneau'][k];
      s.querySelector('.stat-value').textContent = counts[key];
      s.querySelector('.stat-bar i').style.setProperty('--w', Math.round(counts[key] / tot * 100) + '%');
    });
  }
  function isMatch(kind, idx) { return kind === 'truffe' || (kind === 'patte' && idx % 2 === 0); }
  function setDisabled(v) { controls.querySelectorAll('.action').forEach(function (b) { b.disabled = v; }); }
  function finish(kind, idx) {
    var d = DECK[idx];
    counts[kind]++; updateStats(); i++;
    if (i >= DECK.length) {
      slot.innerHTML = '<section class="end pop-in"><div class="end-art"><svg aria-hidden="true"><use href="#i-hotdog"/></svg></div><h3 style="font-family:var(--font-display);font-size:var(--fs-xl)">Il n\'y a plus de hotdogs dans votre région</h3><p>Vous avez vu tout le Paquet.</p><button class="btn" type="button" data-restart>Recommencer</button></section>';
      controls.hidden = true; live.textContent = 'Fin du Paquet : il n\'y a plus de hotdogs dans votre région.';
      slot.querySelector('[data-restart]').focus();
      return;
    }
    if (isMatch(kind, idx)) {
      slot.innerHTML = '<section class="match pop-in"><h3 class="match-title">C\'est un Match&nbsp;!</h3><div class="match-photos"><img src="../app/photos/images/' + d[0] + '" alt="' + esc(d[1]) + '" width="800" height="1000"></div><p>' + esc(d[1]) + ' vous a reniflé en retour.</p><button class="btn" type="button" data-next>Continuer à swiper</button></section>';
      setDisabled(true); live.textContent = 'Match avec ' + d[1] + '.';
      slot.querySelector('[data-next]').focus();
    } else { show(); setDisabled(false); live.textContent = LABEL[kind] + ' pour ' + d[1] + '.'; }
  }
  function swipe(kind) {
    if (busy || controls.hidden) return;
    var card = slot.querySelector('.card'); if (!card) return;
    busy = true; var idx = i;
    var done = function () { busy = false; finish(kind, idx); };
    if (reduce.matches) { done(); return; }
    card.classList.add('fly-' + kind);
    card.addEventListener('animationend', done, { once: true });
  }
  controls.addEventListener('click', function (e) { var b = e.target.closest('[data-act]'); if (b) swipe(b.getAttribute('data-act')); });
  demo.addEventListener('keydown', function (e) {
    var k = { ArrowRight: 'patte', ArrowUp: 'truffe', ArrowLeft: 'panneau' }[e.key];
    if (k && !e.target.closest('input,textarea')) { e.preventDefault(); swipe(k); }
  });
  slot.addEventListener('click', function (e) {
    if (e.target.closest('[data-next]')) { show(); setDisabled(false); controls.querySelector('.action--patte').focus(); }
    if (e.target.closest('[data-restart]')) { i = 0; counts = { patte: 0, truffe: 0, panneau: 0 }; updateStats(); controls.hidden = false; setDisabled(false); show(); live.textContent = ''; controls.querySelector('.action--patte').focus(); }
  });
  show();
})();
