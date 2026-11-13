# 05: Photos et carrousel

**What to build:** Les photos des chiens sont servies par `photos` depuis un volume partagé (en Compose, un volume nommé). Un conteneur d'initialisation copie les photos livrées dans l'image vers le volume au premier démarrage. Le Visiteur voit les photos défiler en carrousel.

**Blocked by:** 01 (Profils à l'écran)

**Status:** resolved

- [ ] `/photos` sert les 10 images depuis le volume partagé
- [ ] L'initialisation copie les photos au premier démarrage seulement
- [ ] Plusieurs instances de `photos` peuvent partager le même volume
- [ ] Le carrousel défile dans l'interface
- [ ] Test Go `httptest` sur le service des images et les probes
- [ ] Un Dockerfile minimal construit l'image de `photos` avec les photos livrées, utilisée par Docker Compose
