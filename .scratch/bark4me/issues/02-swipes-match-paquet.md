# 02: Swipes, Match et Paquet

**What to build:** Le Visiteur donne une Patte, une Truffe ou un Panneau à un Profil et voit le résultat. Truffe donne toujours un Match, Panneau jamais, Patte de façon déterministe à partir de l'identifiant du chien et du Visiteur. Chaque Profil n'est présenté qu'une fois. L'identifiant de Visiteur est un UUID généré dans le navigateur, et l'avancement survit au rechargement. Quand le Paquet est épuisé, l'écran affiche « Il n'y a plus de hotdogs dans votre région ».

**Blocked by:** 01 (Profils à l'écran)

**Status:** resolved

- [ ] `POST /api/swipes` (visiteur, chien, action) renvoie le Match
- [ ] Truffe donne toujours un Match, Panneau jamais, Patte est déterministe
- [ ] Un Profil déjà jugé n'est plus présenté, y compris après rechargement
- [ ] L'écran de fin « plus de hotdogs » s'affiche en fin de Paquet
- [ ] Boutons natifs Patte, Truffe et Panneau, sans compte ni authentification
- [ ] Tests Go `httptest` sur la règle de Match, les erreurs et les probes
- [ ] Un Dockerfile minimal construit l'image de `swipes`, utilisée par Docker Compose
