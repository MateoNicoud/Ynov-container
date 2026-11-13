# 04: Pédigrée

**What to build:** Certains Profils portent un badge Pédigrée validé. Le Visiteur peut vérifier un numéro de Pédigrée et voir le certificat en SVG. Le service `pedigree` délivre un numéro `BARK-XXXX` à un chien, le vérifie, et renvoie le certificat en image SVG.

**Blocked by:** 01 (Profils à l'écran)

**Status:** resolved

- [ ] Un Pédigrée est délivré avec un numéro `BARK-XXXX`
- [ ] La vérification d'un numéro connu réussit, celle d'un numéro inconnu renvoie une erreur cohérente
- [ ] Le certificat est servi en SVG avec le numéro
- [ ] Le badge apparaît sur les Profils concernés
- [ ] Tests Go `httptest` y compris le numéro inconnu
- [ ] Un Dockerfile minimal construit l'image de `pedigree`, utilisée par Docker Compose
