/* Bark4me : prototype Découverte + Messagerie. JS vanilla, aucune dépendance. */
(function () {
  'use strict';
  var root = document.documentElement;
  var photo = window.PHOTO || function (f) { return '../app/photos/images/' + f; };
  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)');

  /* Profils. fx/fy : position du visage (en %), z : zoom de l'avatar.
     hello : message envoyé au Match. replies : réponses aux messages du Visiteur. */
  var DECK = [
    { f: 'chien-01.jpg', name: 'Jean Baguette', breed: 'Bouledogue français', tag: 'Béret, baguette, aucune excuse.', ped: true, fx: 52, fy: 33, z: 1.7,
      hello: 'Bonjour. Je ne sors jamais sans ma baguette. Un café sur les quais, ça vous dit ? Ne parlez pas de la tour, elle me suit partout.',
      replies: ['Charmant. Je vous réserve la meilleure marche du quai.', 'Je vous préviens, je mange la moitié du pain avant d\'arriver.'] },
    { f: 'chien-02.jpg', name: 'Maurice', breed: 'Barzoï', tag: 'Je conduis. Vous marchez.', ped: false, fx: 22, fy: 28, z: 1.6,
      hello: 'Salut. Je passe vous prendre dans dix minutes. Je conduis, vous mettez la ceinture. Pas de musique.',
      replies: ['Je suis devant chez vous. Je ne klaxonne pas, je regarde fixement.', 'Fenêtre ouverte, oreilles au vent. Vous venez ?'] },
    { f: 'chien-03.jpg', name: 'Mage Gérard', breed: 'Carlin', tag: 'Niveau 40. Cherche quête secondaire.', ped: true, fx: 46, fy: 40, z: 1.8,
      hello: 'Salutations, voyageur. Je suis niveau 40 et il me manque un partenaire pour ma quête secondaire. Avez-vous des potions ?',
      replies: ['Excellent. Rendez-vous à la taverne, je viens avec mon bâton.', 'Mon sort préféré : « Siesta ». Il est très efficace.'] },
    { f: 'chien-04.jpg', name: 'Brutus', breed: 'Rottweiler', tag: 'Jamais sauté le jour des pattes.', ped: false, fx: 62, fy: 20, z: 1.8,
      hello: 'Salut. Aujourd\'hui c\'est jour des pattes, mais je me libère après ma série. Tu soulèves ?',
      replies: ['Quinze répétitions, puis boisson protéinée. Tu en es ?', 'Pas de pause. Sauf pour les croquettes.'] },
    { f: 'chien-05.jpg', name: 'Magalax', breed: 'Bouledogue anglais', tag: 'Canapé, chips, pas de regrets.', ped: false, fx: 55, fy: 40, z: 1.9,
      hello: 'Coucou. Je suis sur le canapé, il y a des chips, il y a de la place. Viens comme tu es.',
      replies: ['Apporte du Cola, c\'est la seule règle de la maison.', 'Le canapé a gardé ta place. Je n\'ai pas bougé depuis.'] },
    { f: 'chien-06.jpg', name: 'Kevin', breed: 'Chinois à crête', tag: 'En direct depuis 14 h. Merci pour les abonnements.', ped: false, fx: 70, fy: 32, z: 1.8,
      hello: 'Salut le chat, bienvenue en direct. Merci pour le follow. Je lis le chat entre deux parties, donc réponds vite.',
      replies: ['Ça compte comme un abonnement, merci. Je te cite à l\'antenne.', 'Je coupe le micro, on est en privé. Enfin presque.'] },
    { f: 'chien-07.jpg', name: 'Rex', breed: 'Chien (vérifié)', tag: 'Je suis un chien. Demandez à la niche.', ped: true, fx: 55, fy: 50, z: 2,
      hello: 'Bonjour. Je suis un chien. Ne regardez pas mes oreilles. Voulez-vous visiter la niche ?',
      replies: ['Mon Pédigrée est valide, je le montre à tout le monde.', 'Je n\'aime pas les souris. Rien à voir avec ma nature.'] },
    { f: 'chien-08.jpg', name: 'Minos', breed: 'Bouledogue français', tag: 'Je collectionne les cartouches. Pas les rendez-vous.', ped: false, fx: 55, fy: 45, z: 1.9,
      hello: 'Bonjour. Je cherche une cartouche rare et quelqu\'un pour souffler dessus avec moi. Vous êtes disponible ?',
      replies: ['Brocante samedi, 6 h. J\'apporte les lunettes.', 'Je garde la boîte d\'origine. C\'est non négociable.'] },
    { f: 'chien-09.jpg', name: 'Roger', breed: 'Bouledogue anglais', tag: 'Soirée cartes ce soir. Ne m\'appelez pas.', ped: false, fx: 52, fy: 33, z: 3,
      hello: 'Salut. Draft ce soir à 20 h, j\'ai réservé la table. Je réponds après la partie, ne m\'en veux pas.',
      replies: ['Tu as un deck ? Sinon j\'en prête un, mais il est fragile.', 'Pas de chips sur les cartes. Règle numéro un.'] },
    { f: 'chien-10.jpg', name: 'Dominique', breed: 'Chihuahua', tag: 'Mon meilleur profil, c\'est celui-ci.', ped: false, fx: 35, fy: 18, z: 2,
      hello: 'Coucou, je viens de prendre ma photo, tu as vu l\'angle ? Réponds vite, je suis dans la salle de bains.',
      replies: ['Je refais un cliché pour toi. Celui-là sera encore meilleur.', 'Attends, il y a un reflet. Je recommence.'] }
  ];

  /* ---------- helpers ---------- */
  function $(s, r) { return (r || document).querySelector(s); }
  function esc(s) { return String(s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
  function hhmm() { var d = new Date(); return ('0' + d.getHours()).slice(-2) + ':' + ('0' + d.getMinutes()).slice(-2); }
  function avatarStyle(d) {
    return 'background-image:url(' + photo(d.f) + ');background-size:' + (d.z * 100) + '% auto;background-position:' + d.fx + '% ' + d.fy + '%';
  }

  /* ---------- Mode daltochien ---------- */
  var KEY = 'bark4me-mode';
  function readMode() { try { return localStorage.getItem(KEY); } catch (e) { return null; } }
  function setMode(m) {
    if (m === 'daltochien') root.setAttribute('data-mode', 'daltochien'); else root.removeAttribute('data-mode');
    $('[data-switch]').setAttribute('aria-checked', m === 'daltochien' ? 'true' : 'false');
    try { localStorage.setItem(KEY, m); } catch (e) {}
  }
  setMode(readMode() === 'daltochien' ? 'daltochien' : 'normal');
  $('[data-switch]').addEventListener('click', function () { setMode($('[data-switch]').getAttribute('aria-checked') === 'true' ? 'normal' : 'daltochien'); });

  /* ---------- Bannière d'entrée ---------- */
  var intro = $('[data-intro]'), SEEN = 'bark4me-intro';
  function introSeen() { try { return sessionStorage.getItem(SEEN) === '1'; } catch (e) { return false; } }
  if (!introSeen() && intro) {
    if (intro.showModal) intro.showModal(); else intro.setAttribute('open', '');
    intro.addEventListener('cancel', function (e) { e.preventDefault(); });
  }
  $('[data-intro-ok]').addEventListener('click', function () {
    try { sessionStorage.setItem(SEEN, '1'); } catch (e) {}
    if (intro.close) intro.close(); else intro.removeAttribute('open');
  });

  /* ---------- Onglets ---------- */
  var tabs = Array.prototype.slice.call(document.querySelectorAll('[role="tab"]'));
  function showTab(id) {
    tabs.forEach(function (t) {
      var on = t.id === 'tab-' + id;
      t.setAttribute('aria-selected', on ? 'true' : 'false');
      t.tabIndex = on ? 0 : -1;
      document.getElementById(t.getAttribute('aria-controls')).hidden = !on;
    });
    if (id === 'messagerie') renderInbox();
  }
  tabs.forEach(function (t, i) {
    t.addEventListener('click', function () { showTab(t.id.replace('tab-', '')); });
    t.addEventListener('keydown', function (e) {
      var j = e.key === 'ArrowRight' ? (i + 1) % tabs.length : e.key === 'ArrowLeft' ? (i + tabs.length - 1) % tabs.length : -1;
      if (j >= 0) { e.preventDefault(); tabs[j].focus(); showTab(tabs[j].id.replace('tab-', '')); }
    });
  });

  /* ---------- Découverte ---------- */
  var slot = $('[data-slot]'), controls = $('[data-controls]'), hint = $('[data-hint]');
  var counts = { patte: 0, truffe: 0, panneau: 0 }, i = 0, busy = false;
  var LABEL = { patte: 'Patte', truffe: 'Truffe', panneau: 'Panneau' };
  var STATS = [['patte', 'Pattes'], ['truffe', 'Truffes'], ['panneau', 'Panneaux']];

  function cardHtml(d) {
    return '<article class="card" tabindex="-1" aria-label="Profil de ' + esc(d.name) + '"><img src="' + photo(d.f) + '" alt="' + esc(d.name + ', ' + d.breed + ', ' + d.tag) + '" width="800" height="1000">' +
      (d.ped ? '<span class="card-badge"><span class="badge"><svg aria-hidden="true"><use href="#i-seal"/></svg>Pédigrée validé</span></span>' : '') +
      '<span class="card-stamp card-stamp--patte" aria-hidden="true">Patte</span><span class="card-stamp card-stamp--panneau" aria-hidden="true">Panneau</span><span class="card-stamp card-stamp--truffe" aria-hidden="true">Truffe</span>' +
      '<div class="card-body"><h2 class="card-name">' + esc(d.name) + '</h2><p class="card-breed">' + esc(d.breed) + '</p><p class="card-tagline">' + esc(d.tag) + '</p></div></article>';
  }
  function renderStats() {
    var tot = counts.patte + counts.truffe + counts.panneau;
    $('[data-stats]').innerHTML = STATS.map(function (s) {
      var k = s[0], w = tot ? Math.round(counts[k] / tot * 100) : 0;
      return '<div class="stat stat--' + k + '" role="listitem" style="--a-fill:var(--c-' + k + ');--a-on:var(--c-on-' + k + ')"><span class="stat-icon"><svg aria-hidden="true"><use href="#i-' + k + '"/></svg></span><span class="stat-name">' + s[1] + '</span><span class="stat-value">' + counts[k] + '</span><span class="stat-bar" aria-hidden="true"><i style="--w:' + w + '%"></i></span></div>';
    }).join('');
  }
  function setDisabled(v) { controls.querySelectorAll('.action').forEach(function (b) { b.disabled = v; }); }
  function showCard() { slot.innerHTML = cardHtml(DECK[i]); controls.hidden = false; hint.hidden = false; setDisabled(false); }
  function showEnd() {
    slot.innerHTML = '<section class="end pop-in"><div class="end-art"><svg aria-hidden="true"><use href="#i-hotdog"/></svg></div><h2>Il n\'y a plus de hotdogs dans votre région</h2><p>Vous avez vu tout le Paquet. Vos Match vous attendent dans la Messagerie.</p><button class="btn" type="button" data-goto="messagerie">Ouvrir la Messagerie</button><button class="btn btn--ghost" type="button" data-restart>Recommencer</button></section>';
    controls.hidden = true; hint.hidden = true;
    $('[data-restart]').focus();
  }
  function isMatch(kind, idx) { return kind === 'truffe' || (kind === 'patte' && idx % 2 === 0); }
  function next() { i++; if (i >= DECK.length) showEnd(); else showCard(); }

  function showMatch(d) {
    slot.innerHTML = '<section class="match pop-in" aria-labelledby="mt"><p class="eyebrow" style="color:inherit">Vous et ' + esc(d.name) + '</p><h2 class="match-title" id="mt">C\'est un Match&nbsp;!</h2>' +
      '<div class="match-photos"><img src="' + photo(d.f) + '" alt="' + esc(d.name) + '" width="800" height="1000"></div>' +
      '<p class="match-msg">' + esc(d.name) + ' vous a déjà écrit.</p>' +
      '<div class="match-actions"><button class="btn" type="button" data-read>Lire le message</button><button class="btn btn--ghost" type="button" data-next>Continuer à swiper</button></div></section>';
    controls.hidden = true; hint.hidden = true;
    $('[data-read]').focus();
  }

  function swipe(kind) {
    if (busy || controls.hidden) return;
    var card = $('.card', slot); if (!card) return;
    busy = true; var idx = i, d = DECK[idx];
    var done = function () {
      busy = false; counts[kind]++; renderStats();
      if (isMatch(kind, idx)) { addMatch(d); showMatch(d); } else next();
    };
    if (reduce.matches) { done(); return; }
    card.classList.add('fly-' + kind);
    card.addEventListener('animationend', done, { once: true });
  }
  controls.addEventListener('click', function (e) { var b = e.target.closest('[data-act]'); if (b) swipe(b.getAttribute('data-act')); });
  $('[data-slot]').closest('.panel').addEventListener('keydown', function (e) {
    if (e.target.closest('input,textarea,button[role="tab"]')) return;
    var k = { ArrowRight: 'patte', ArrowUp: 'truffe', ArrowLeft: 'panneau' }[e.key];
    if (k) { e.preventDefault(); swipe(k); }
  });
  slot.addEventListener('click', function (e) {
    var t = e.target;
    if (t.closest('[data-next]')) { next(); var b = controls.hidden ? null : $('.action--patte', controls); if (b) b.focus(); }
    if (t.closest('[data-read]')) { openThread(DECK[i].f); showTab('messagerie'); $('#msg').focus(); i++; if (i >= DECK.length) showEnd(); else showCard(); }
    if (t.closest('[data-goto]')) showTab('messagerie');
    if (t.closest('[data-restart]')) { i = 0; counts = { patte: 0, truffe: 0, panneau: 0 }; renderStats(); showCard(); $('.action--patte', controls).focus(); }
  });

  /* ---------- Messagerie ---------- */
  var convs = [];       // plus récent en premier
  var current = null;   // clé (nom de fichier) de la conversation ouverte
  var inbox = $('#panel-messagerie'), badge = $('[data-unread]');
  function byKey(k) { return convs.filter(function (c) { return c.d.f === k; })[0]; }
  function unreadTotal() { return convs.reduce(function (n, c) { return n + c.unread; }, 0); }
  function updateBadge() {
    var n = unreadTotal();
    badge.hidden = n === 0; badge.textContent = n;
    badge.setAttribute('aria-label', n + ' message' + (n > 1 ? 's' : '') + ' non lu' + (n > 1 ? 's' : ''));
  }
  function addMatch(d) {
    if (byKey(d.f)) return;
    convs.unshift({ d: d, unread: 1, rep: 0, msgs: [{ from: 'sys', text: 'Match avec ' + d.name }, { from: 'dog', text: d.hello, t: hhmm() }] });
    updateBadge(); renderInbox();
  }
  function renderInbox() {
    var list = $('[data-threads]');
    $('[data-empty]').hidden = convs.length > 0;
    list.innerHTML = convs.map(function (c) {
      var last = c.msgs[c.msgs.length - 1], preview = last.from === 'me' ? 'Vous : ' + last.text : last.text;
      var t = c.msgs.filter(function (m) { return m.t; }).pop();
      return '<li><button type="button" class="thread-btn' + (c.unread ? ' unread' : '') + '" data-open="' + c.d.f + '"' + (c.d.f === current ? ' aria-current="true"' : '') + '><span class="avatar" role="img" aria-label="' + esc(c.d.name) + '" style="' + avatarStyle(c.d) + '"></span><span class="txt"><strong>' + esc(c.d.name) + (c.unread ? '<span class="sr-only"> (non lu)</span>' : '') + '</strong><span class="prev">' + esc(preview) + '</span></span><time>' + (t ? t.t : '') + '</time></button></li>';
    }).join('');
  }
  function renderThread() {
    var c = byKey(current), th = $('[data-thread]');
    if (!c) { th.hidden = true; return; }
    th.hidden = false;
    $('[data-th-avatar]').setAttribute('style', avatarStyle(c.d));
    $('[data-th-name]').textContent = c.d.name;
    $('[data-th-breed]').textContent = c.d.breed;
    var log = $('[data-log]');
    log.innerHTML = c.msgs.map(function (m) {
      if (m.from === 'sys') return '<p class="sys">' + esc(m.text) + '</p>';
      return '<div class="msg msg--' + m.from + '"><span class="sr-only">' + (m.from === 'me' ? 'Vous' : esc(c.d.name)) + ' : </span>' + esc(m.text) + '<time>' + m.t + '</time></div>';
    }).join('');
    log.scrollTop = log.scrollHeight;
  }
  function openThread(k) {
    current = k;
    var c = byKey(k); if (c) { c.unread = 0; }
    inbox.setAttribute('data-view', 'thread');
    updateBadge(); renderInbox(); renderThread();
  }
  inbox.addEventListener('click', function (e) {
    var o = e.target.closest('[data-open]');
    if (o) { openThread(o.getAttribute('data-open')); $('#msg').focus(); }
    if (e.target.closest('[data-back]')) { inbox.setAttribute('data-view', 'list'); current = null; renderInbox(); renderThread(); }
  });
  $('[data-form]').addEventListener('submit', function (e) {
    e.preventDefault();
    var input = $('#msg'), text = input.value.trim(), c = byKey(current);
    if (!text || !c) return;
    c.msgs.push({ from: 'me', text: text, t: hhmm() }); input.value = '';
    renderThread(); renderInbox();
    var key = c.d.f, reply = c.d.replies[c.rep % c.d.replies.length]; c.rep++;
    setTimeout(function () {
      var cc = byKey(key); if (!cc) return;
      cc.msgs.push({ from: 'dog', text: reply, t: hhmm() });
      if (current === key && !inbox.hidden) renderThread(); else cc.unread++;
      updateBadge(); renderInbox();
    }, reduce.matches ? 0 : 1100);
  });

  renderStats(); showCard(); updateBadge();
})();
