/* Bark4me : Découverte. JS vanilla, aucune dépendance.
   Évènements publiés sur document : "bark:card" (detail = Profil affiché),
   "bark:swiped" (detail = {profile, action, match}). */
(function () {
  'use strict';
  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)');
  var LABEL = { patte: 'Patte', truffe: 'Truffe', panneau: 'Panneau' };

  function $(s, r) { return (r || document).querySelector(s); }
  function esc(s) { return String(s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
  function emit(name, detail) { document.dispatchEvent(new CustomEvent(name, { detail: detail })); }
  function store(k, v) { try { if (v === undefined) return localStorage.getItem(k); localStorage.setItem(k, v); } catch (e) { return null; } }

  /* ---------- Visiteur : UUID aléatoire et avancement, côté navigateur ---------- */
  function uuid() {
    if (window.crypto && crypto.randomUUID) return crypto.randomUUID();
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
      var r = Math.random() * 16 | 0; return (c === 'x' ? r : (r & 3 | 8)).toString(16);
    });
  }
  var visitor = store('bark4me-visitor');
  if (!visitor) { visitor = uuid(); store('bark4me-visitor', visitor); }
  var seen = {};
  try { seen = JSON.parse(store('bark4me-seen') || '{}') || {}; } catch (e) { seen = {}; }
  function markSeen(id, action) { seen[id] = action; store('bark4me-seen', JSON.stringify(seen)); }

  /* ---------- API ---------- */
  function api(method, url, body) {
    return fetch(url, {
      method: method,
      headers: body ? { 'Content-Type': 'application/json' } : {},
      body: body ? JSON.stringify(body) : undefined
    }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (!r.ok) { var e = new Error(j.error || ('Erreur ' + r.status)); e.status = r.status; throw e; }
        return j;
      });
    });
  }

  /* ---------- Paquet ---------- */
  var slot = $('[data-slot]'), controls = $('[data-controls]'), hint = $('[data-hint]'), status = $('[data-status]');
  var deck = [], i = 0, busy = false;

  function say(msg) { status.textContent = ''; setTimeout(function () { status.textContent = msg; }, 30); }
  function cardHtml(p) {
    return '<article class="card" tabindex="-1" aria-label="Profil de ' + esc(p.name) + '"><img src="' + esc(p.photo) + '" alt="' + esc(p.name + ', ' + p.breed + ', ' + p.tagline) + '" width="800" height="1000">' +
      '<span class="card-badge" data-badge hidden></span>' +
      '<span class="card-stamp card-stamp--patte" aria-hidden="true">Patte</span><span class="card-stamp card-stamp--panneau" aria-hidden="true">Panneau</span><span class="card-stamp card-stamp--truffe" aria-hidden="true">Truffe</span>' +
      '<div class="card-body"><h2 class="card-name">' + esc(p.name) + '</h2><p class="card-breed">' + esc(p.breed) + '</p><p class="card-tagline">' + esc(p.tagline) + '</p></div></article>';
  }
  function setDisabled(v) { controls.querySelectorAll('.action').forEach(function (b) { b.disabled = v; }); }
  function showCard() {
    slot.innerHTML = cardHtml(deck[i]);
    controls.hidden = false; hint.hidden = false; setDisabled(false);
    emit('bark:card', deck[i]);
  }
  function showEnd() {
    slot.innerHTML = '<section class="end pop-in" aria-labelledby="end-t"><div class="end-art"><svg aria-hidden="true"><use href="#i-hotdog"/></svg></div><h2 id="end-t">Il n\'y a plus de hotdogs dans votre région</h2><p>Vous avez vu tout le Paquet.</p></section>';
    controls.hidden = true; hint.hidden = true;
    say('Il n\'y a plus de hotdogs dans votre région');
  }
  function showMatch(p) {
    slot.innerHTML = '<section class="match pop-in" aria-labelledby="mt"><p class="eyebrow" style="color:inherit">Vous et ' + esc(p.name) + '</p><h2 class="match-title" id="mt">C\'est un Match&nbsp;!</h2>' +
      '<div class="match-photos"><img src="' + esc(p.photo) + '" alt="' + esc(p.name) + '" width="800" height="1000"></div>' +
      '<p class="match-msg">' + esc(p.name) + ' vous a déjà écrit.</p>' +
      '<div class="match-actions"><button class="btn" type="button" data-read>Lire le message</button><button class="btn btn--ghost" type="button" data-next>Continuer à swiper</button></div></section>';
    controls.hidden = true; hint.hidden = true;
    say('C\'est un Match avec ' + p.name);
    $('[data-read]').focus();
  }
  function next() {
    i++;
    if (i >= deck.length) showEnd(); else showCard();
  }

  function swipe(action) {
    if (busy || controls.hidden) return;
    var card = $('.card', slot); if (!card) return;
    var p = deck[i];
    busy = true; setDisabled(true);
    var request = api('POST', '/api/swipes', { visitor: visitor, dog: p.id, action: action })
      .catch(function (e) { if (e.status === 409) return { match: false, duplicate: true }; throw e; });
    var animation = new Promise(function (resolve) {
      if (reduce.matches) { resolve(); return; }
      card.classList.add('fly-' + action);
      card.addEventListener('animationend', function () { resolve(); }, { once: true });
    });
    Promise.all([request, animation]).then(function (res) {
      busy = false;
      var r = res[0];
      markSeen(p.id, action);
      emit('bark:swiped', { profile: p, action: action, match: !!r.match, duplicate: !!r.duplicate });
      if (r.match) showMatch(p); else next();
    }).catch(function (e) {
      busy = false;
      card.classList.remove('fly-' + action);
      setDisabled(false);
      say('Le Swipe n\'a pas pu être enregistré : ' + e.message);
    });
  }
  controls.addEventListener('click', function (e) { var b = e.target.closest('[data-act]'); if (b) swipe(b.getAttribute('data-act')); });
  $('#panel-decouverte').addEventListener('keydown', function (e) {
    if (e.target.closest('input,textarea,button[role="tab"],a,dialog')) return;
    var k = { ArrowRight: 'patte', ArrowUp: 'truffe', ArrowLeft: 'panneau' }[e.key];
    if (k) { e.preventDefault(); swipe(k); }
  });
  slot.addEventListener('click', function (e) {
    if (e.target.closest('[data-read]')) { emit('bark:read', deck[i]); next(); }
    if (e.target.closest('[data-next]')) {
      next();
      var b = controls.hidden ? null : $('.action--patte', controls); if (b) b.focus();
    }
  });

  /* ---------- Démarrage ---------- */
  slot.innerHTML = '<p class="hint">Chargement du Paquet…</p>';
  api('GET', '/api/profiles').then(function (all) {
    emit('bark:profiles', all);
    deck = all.filter(function (p) { return !seen[p.id]; });
    if (!deck.length) showEnd(); else showCard();
  }).catch(function (e) {
    slot.innerHTML = '<p class="hint" role="alert">Impossible de charger les Profils : ' + esc(e.message) + '</p>';
  });
})();
