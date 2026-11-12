/* Messagerie : conversations ouvertes par les Match (réponses simulées, côté navigateur). */
(function () {
  'use strict';
  function $(s, r) { return (r || document).querySelector(s); }
  function esc(s) { return String(s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
  function hhmm() { var d = new Date(); return ('0' + d.getHours()).slice(-2) + ':' + ('0' + d.getMinutes()).slice(-2); }
  function avatarStyle(d) { return 'background-image:url(' + d.photo + ');background-size:' + (d.z * 100) + '% auto;background-position:' + d.fx + '% ' + d.fy + '%'; }
  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)');
  var EXTRA = {"chien-01": {"fx": 52, "fy": 33, "z": 1.7, "hello": "Bonjour. Je ne sors jamais sans ma baguette. Un café sur les quais, ça vous dit ? Ne parlez pas de la tour, elle me suit partout.", "replies": ["Charmant. Je vous réserve la meilleure marche du quai.", "Je vous préviens, je mange la moitié du pain avant d'arriver."]}, "chien-02": {"fx": 22, "fy": 28, "z": 1.6, "hello": "Salut. Je passe vous prendre dans dix minutes. Je conduis, vous mettez la ceinture. Pas de musique.", "replies": ["Je suis devant chez vous. Je ne klaxonne pas, je regarde fixement.", "Fenêtre ouverte, oreilles au vent. Vous venez ?"]}, "chien-03": {"fx": 46, "fy": 40, "z": 1.8, "hello": "Salutations, voyageur. Je suis niveau 40 et il me manque un partenaire pour ma quête secondaire. Avez-vous des potions ?", "replies": ["Excellent. Rendez-vous à la taverne, je viens avec mon bâton.", "Mon sort préféré : « Siesta ». Il est très efficace."]}, "chien-04": {"fx": 62, "fy": 20, "z": 1.8, "hello": "Salut. Aujourd'hui c'est jour des pattes, mais je me libère après ma série. Tu soulèves ?", "replies": ["Quinze répétitions, puis boisson protéinée. Tu en es ?", "Pas de pause. Sauf pour les croquettes."]}, "chien-05": {"fx": 55, "fy": 40, "z": 1.9, "hello": "Coucou. Je suis sur le canapé, il y a des chips, il y a de la place. Viens comme tu es.", "replies": ["Apporte du Cola, c'est la seule règle de la maison.", "Le canapé a gardé ta place. Je n'ai pas bougé depuis."]}, "chien-06": {"fx": 70, "fy": 32, "z": 1.8, "hello": "Salut le chat, bienvenue en direct. Merci pour le follow. Je lis le chat entre deux parties, donc réponds vite.", "replies": ["Ça compte comme un abonnement, merci. Je te cite à l'antenne.", "Je coupe le micro, on est en privé. Enfin presque."]}, "chien-07": {"fx": 55, "fy": 50, "z": 2.0, "hello": "Bonjour. Je suis un chien. Ne regardez pas mes oreilles. Voulez-vous visiter la niche ?", "replies": ["Mon Pédigrée est valide, je le montre à tout le monde.", "Je n'aime pas les souris. Rien à voir avec ma nature."]}, "chien-08": {"fx": 55, "fy": 45, "z": 1.9, "hello": "Bonjour. Je cherche une cartouche rare et quelqu'un pour souffler dessus avec moi. Vous êtes disponible ?", "replies": ["Brocante samedi, 6 h. J'apporte les lunettes.", "Je garde la boîte d'origine. C'est non négociable."]}, "chien-09": {"fx": 52, "fy": 33, "z": 3.0, "hello": "Salut. Draft ce soir à 20 h, j'ai réservé la table. Je réponds après la partie, ne m'en veux pas.", "replies": ["Tu as un deck ? Sinon j'en prête un, mais il est fragile.", "Pas de chips sur les cartes. Règle numéro un."]}, "chien-10": {"fx": 35, "fy": 18, "z": 2.0, "hello": "Coucou, je viens de prendre ma photo, tu as vu l'angle ? Réponds vite, je suis dans la salle de bains.", "replies": ["Je refais un cliché pour toi. Celui-là sera encore meilleur.", "Attends, il y a un reflet. Je recommence."]}};
  function dog(p) { var x = EXTRA[p.id] || { fx: 50, fy: 40, z: 1.8, hello: 'Bonjour !', replies: ['Wouaf.'] }; return Object.assign({}, p, x); }
  /* ---------- Messagerie ---------- */
  var convs = [];       // plus récent en premier
  var current = null;   // clé (nom de fichier) de la conversation ouverte
  var inbox = $('#panel-messagerie'), badge = $('[data-unread]');
  function byKey(k) { return convs.filter(function (c) { return c.d.id === k; })[0]; }
  function unreadTotal() { return convs.reduce(function (n, c) { return n + c.unread; }, 0); }
  function updateBadge() {
    var n = unreadTotal();
    badge.hidden = n === 0; badge.textContent = n;
    badge.setAttribute('aria-label', n + ' message' + (n > 1 ? 's' : '') + ' non lu' + (n > 1 ? 's' : ''));
  }
  function addMatch(d) {
    if (byKey(d.id)) return;
    convs.unshift({ d: d, unread: 1, rep: 0, msgs: [{ from: 'sys', text: 'Match avec ' + d.name }, { from: 'dog', text: d.hello, t: hhmm() }] });
    updateBadge(); renderInbox();
  }
  function renderInbox() {
    var list = $('[data-threads]');
    $('[data-empty]').hidden = convs.length > 0;
    list.innerHTML = convs.map(function (c) {
      var last = c.msgs[c.msgs.length - 1], preview = last.from === 'me' ? 'Vous : ' + last.text : last.text;
      var t = c.msgs.filter(function (m) { return m.t; }).pop();
      return '<li><button type="button" class="thread-btn' + (c.unread ? ' unread' : '') + '" data-open="' + c.d.id + '"' + (c.d.id === current ? ' aria-current="true"' : '') + '><span class="avatar" role="img" aria-label="' + esc(c.d.name) + '" style="' + avatarStyle(c.d) + '"></span><span class="txt"><strong>' + esc(c.d.name) + (c.unread ? '<span class="sr-only"> (non lu)</span>' : '') + '</strong><span class="prev">' + esc(preview) + '</span></span><time>' + (t ? t.t : '') + '</time></button></li>';
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
    var key = c.d.id, reply = c.d.replies[c.rep % c.d.replies.length]; c.rep++;
    setTimeout(function () {
      var cc = byKey(key); if (!cc) return;
      cc.msgs.push({ from: 'dog', text: reply, t: hhmm() });
      if (current === key && !inbox.hidden) renderThread(); else cc.unread++;
      updateBadge(); renderInbox();
    }, reduce.matches ? 0 : 1100);
  });


  document.addEventListener('bark:swiped', function (e) { if (e.detail.match && !e.detail.duplicate) addMatch(dog(e.detail.profile)); });
  document.addEventListener('bark:read', function (e) { openThread(e.detail.id); document.dispatchEvent(new CustomEvent('bark:goto', { detail: 'messagerie' })); $('#msg').focus(); });
  document.addEventListener('bark:tab', function (e) { if (e.detail === 'messagerie') renderInbox(); });
  updateBadge();
})();
