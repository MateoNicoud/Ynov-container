# 01: Profils à l'écran

**What to build:** Avec Docker Compose, `web`, `profiles` et Postgres démarrent ensemble et le Visiteur voit un Profil : photo, nom, race et phrase d'accroche. Les 10 Profils initiaux sont chargés au démarrage. Chaque service Go expose `/healthz` et `/readyz` (qui vérifie Postgres), avec des erreurs JSON `{"error": "..."}`. C'est le tracer bullet : il fixe la forme d'un service (Dockerfile minimal, tests `httptest` avec stockage en mémoire).

**Blocked by:** None (can start immediately)

**Status:** resolved

- [ ] `docker compose up` démarre `web`, `profiles` et Postgres
- [ ] Le site affiche un Profil avec photo, nom, race et accroche
- [ ] Les 10 Profils sont disponibles via `/api/profiles`
- [ ] `/healthz` et `/readyz` répondent, `/readyz` échoue si Postgres est coupé
- [ ] Tests Go `httptest` sur le contenu des Profils et les probes, sans Postgres réel
- [ ] Un Dockerfile minimal construit l'image de `profiles` et une autre celle de `web` (nginx), utilisés par Docker Compose
