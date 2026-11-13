# Application Bark4me

Cinq services et un Postgres, lancés en local avec Docker Compose.

| Service | Rôle |
|---|---|
| `web` | site statique (nginx), front issu de `../design/` |
| `profiles` | les 10 Profils du Paquet (`/api/profiles`) |
| `swipes` | Swipes, règle de Match, statistiques (`/api/swipes`) |
| `pedigree` | Pédigrée BARK-XXXX, vérification, certificat SVG (`/api/pedigree`) |
| `photos` | images des chiens depuis un volume partagé (`/photos`) ; `photos init` y copie les photos livrées |

## Lancer

```bash
docker compose up --build     # puis http://localhost:8080
docker compose down -v        # tout supprimer, volumes compris
```

La passerelle `gateway` reproduit le routage par chemin de l'Ingress du cluster.

## Tester

Chaque service Go a ses tests `httptest` (stockage en mémoire, pas de Postgres) :

```bash
for s in profiles swipes pedigree photos; do
  docker run --rm -v "$PWD/$s":/src -w /src golang:1.26-alpine go test ./...
done
```
