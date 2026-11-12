/* Pédigrée : badge « validé » sur les Profils concernés, certificat SVG, vérification d'un numéro. */
(function () {
  'use strict';
  var byDog = {};
  var dlg = document.querySelector('[data-cert]');
  var form = document.querySelector('[data-verify]');
  var result = document.querySelector('[data-verify-result]');
  function esc(s) { return String(s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
  function svgUrl(n) { return '/api/pedigree/' + encodeURIComponent(n) + '/certificate.svg'; }

  function placeBadge(p) {
    var c = byDog[p.id], slot = document.querySelector('[data-badge]');
    if (!c || !slot) return;
    slot.hidden = false;
    slot.innerHTML = '<button type="button" class="badge badge--btn" data-number="' + esc(c.number) + '"><svg aria-hidden="true"><use href="#i-seal"/></svg>Pédigrée validé<span class="sr-only"> ' + esc(c.number) + ', voir le certificat</span></button>';
  }
  function showCert(c) {
    dlg.querySelector('[data-cert-img]').src = svgUrl(c.number);
    dlg.querySelector('[data-cert-img]').alt = 'Certificat de Pédigrée ' + c.number + ' pour ' + c.name + ', ' + c.breed;
    dlg.querySelector('[data-cert-num]').textContent = c.number;
    if (dlg.showModal) dlg.showModal(); else dlg.setAttribute('open', '');
  }

  document.addEventListener('bark:card', function (e) { placeBadge(e.detail); });
  document.addEventListener('click', function (e) {
    var b = e.target.closest('[data-number]');
    if (b) {
      fetch('/api/pedigree/' + encodeURIComponent(b.getAttribute('data-number'))).then(function (r) { return r.json(); })
        .then(function (j) { if (j.certificate) showCert(j.certificate); });
    }
    if (e.target.closest('[data-cert-close]')) { if (dlg.close) dlg.close(); else dlg.removeAttribute('open'); }
  });

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    var n = form.elements.number.value.trim().toUpperCase();
    if (!n) return;
    fetch('/api/pedigree/' + encodeURIComponent(n)).then(function (r) {
      return r.json().then(function (j) { return { ok: r.ok, j: j }; });
    }).then(function (res) {
      if (!res.ok) { result.className = 'verify-result verify-result--ko'; result.textContent = res.j.error || 'Vérification impossible.'; return; }
      var c = res.j.certificate;
      result.className = 'verify-result verify-result--ok';
      result.innerHTML = '';
      var t = document.createElement('span');
      t.textContent = 'Pédigrée valide : ' + c.number + ' pour ' + c.name + ' (' + c.breed + ').';
      var btn = document.createElement('button');
      btn.type = 'button'; btn.className = 'btn btn--ghost'; btn.textContent = 'Voir le certificat';
      btn.addEventListener('click', function () { showCert(c); });
      result.appendChild(t); result.appendChild(btn);
    }).catch(function () { result.className = 'verify-result verify-result--ko'; result.textContent = 'Service Pédigrée indisponible.'; });
  });

  fetch('/api/pedigree').then(function (r) { return r.json(); }).then(function (list) {
    list.forEach(function (c) { byDog[c.dog] = c; });
    var cur = document.querySelector('.card');
    if (cur) { /* une carte est déjà affichée avant l'arrivée des Pédigrées */
      var name = cur.getAttribute('aria-label') || '';
      list.forEach(function (c) { if (name === 'Profil de ' + c.name) placeBadge({ id: c.dog }); });
    }
  }).catch(function () {});
})();
