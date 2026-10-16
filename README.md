# TP final : conteneurisation et orchestration

Une API Go et une base MariaDB, lancées avec Docker Compose puis sur un cluster k3d.

API : `/healthz`, `/readyz`, `/version`, `/api/posts` (lit la table `wp_posts`), `/burn?ms=500` (consomme du CPU).

## Prérequis

Docker avec Compose v2, k3d, kubectl.

## Compose

```sh
cp .env.example .env
docker compose up -d --build --wait
docker compose ps
curl http://localhost:8081/api/posts
```

Changer les mots de passe dans `.env` avant un usage réel. `.env` est gitignoré.

Persistance :

```sh
docker compose exec mariadb mariadb -uapp -pchange-me app -e "INSERT INTO wp_posts (post_title) VALUES ('Persiste')"
docker compose down
docker compose up -d --wait
curl http://localhost:8081/api/posts
```

La ligne `Persiste` est toujours là. `docker compose down -v` supprime le volume.

```
            hôte 127.0.0.1:8081
                    |
  réseau public     |
  +-----------------v-----+
  | api (tp-finale-api)   |
  +-----------+-----------+
              |  réseau internal (internal: true, sans sortie)
  +-----------v-----------+
  | mariadb 11.8          |---- volume db_data
  +-----------------------+
```

## k3d

```sh
docker build -t tp-finale-api:1.0.0 --build-arg VERSION=1.0.0 app
k3d cluster create tp-finale -p "8080:80@loadbalancer"
k3d image import tp-finale-api:1.0.0 -c tp-finale
kubectl apply -f k8s/
kubectl create secret generic db -n tp-finale \
  --from-literal=DB_PASSWORD=change-me \
  --from-literal=DB_ROOT_PASSWORD=change-me-root
kubectl rollout status deploy/mariadb deploy/api -n tp-finale
curl http://api.localhost:8080/healthz
curl http://api.localhost:8080/api/posts
```

Les pods attendent le Secret, puis démarrent. Juste après la création du cluster, Traefik peut répondre 404 ou 503 pendant quelques secondes : relancer le `curl`.

Suppression : `k3d cluster delete tp-finale`.

```
  hôte :8080 -> load balancer k3d :80
                    |
  +-----------------v---------------------------------+
  | namespace tp-finale                               |
  |                                                   |
  |  Ingress api.localhost (Traefik)                  |
  |        |                                          |
  |  Service api :80                                  |
  |        |                                          |
  |  Deployment api (1 à 3 pods, HPA CPU 50 %)        |
  |   ServiceAccount api, ConfigMap app-config,       |
  |   Secret db                                       |
  |        |                                          |
  |  Service mariadb :3306  (NetworkPolicy : api)     |
  |        |                                          |
  |  Deployment mariadb -- PVC mariadb-data           |
  |                        (local-path, 1Gi)          |
  +---------------------------------------------------+
```

## Compose vers Kubernetes

| Compose | Kubernetes |
|---|---|
| service `api` | Deployment `api` + Service `api` |
| service `mariadb` | Deployment `mariadb` + Service `mariadb` |
| `ports: 127.0.0.1:8081:8080` | Ingress `api.localhost` via Traefik |
| variables non sensibles de `.env` | ConfigMap `app-config` |
| mots de passe de `.env` | Secret `db` créé à la main |
| volume `db_data` | PVC `mariadb-data` |
| montage `db/init.sql` | ConfigMap `mariadb-init` |
| réseau `internal` | NetworkPolicy `mariadb` |
| `healthcheck` | readiness et liveness probes |
| `depends_on: service_healthy` | readiness de l'API sur `/readyz`, qui teste la base |
| `read_only`, `cap_drop`, `no-new-privileges` | `securityContext` |
| (rien) | HPA `api`, ServiceAccount, Role, RoleBinding |

## Sécurité

Docker :

- Dockerfile multi-stage : build en `golang:alpine`, image finale `distroless/static-debian13:nonroot` avec le seul binaire, sans shell ni gestionnaire de paquets.
- Utilisateur `65532`, non-root.
- `.dockerignore` : seul le code de l'API part au build.
- Image taguée `1.0.0`, version injectée dans le binaire.
- Compose : `read_only`, `cap_drop: [ALL]`, `no-new-privileges`, port publié sur `127.0.0.1` seulement.
- MariaDB sur un réseau `internal`, sans port publié.
- Mots de passe dans `.env` gitignoré, `.env.example` commité.

Kubernetes :

- `securityContext` de l'API : `runAsNonRoot`, `runAsUser: 65532`, `seccompProfile: RuntimeDefault`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem`, `capabilities.drop: [ALL]`.
- ServiceAccount `api` avec un Role qui ne permet que `get` sur la ConfigMap `app-config`.
- NetworkPolicy : seuls les pods `app: api` joignent MariaDB sur 3306. k3s applique les NetworkPolicy.
- Secret `db` créé par `kubectl create secret`, absent du dépôt.
- Requests et limits CPU et mémoire sur les deux Deployments.

Scan Trivy de `tp-finale-api:1.0.0` (HIGH et CRITICAL, `ignore-unfixed`) : 0 HIGH, 0 CRITICAL. L'image ne contient que le binaire Go statique et la base distroless (certificats, tzdata, `/etc/passwd`), il n'y a presque aucun paquet à scanner. Les failles sans correctif sont ignorées : elles ne se corrigent pas de notre côté, il faut attendre la nouvelle image de base. MariaDB n'est pas scannée, c'est l'image officielle.

## CI

`.github/workflows/ci.yml` : à chaque push sur `main`, build de l'image puis scan Trivy. Le job échoue sur une faille HIGH ou CRITICAL qui a un correctif. Actions épinglées par SHA, `permissions: contents: read`.

Run vert : https://github.com/MateoNicoud/tp-finale-docker/actions/runs/38880398246

## Démo

HPA, charge sur `/burn` :

```sh
kubectl run load -n tp-finale --image=busybox:1.36 --restart=Never --command -- \
  sh -c 'while true; do wget -q -O /dev/null http://api/burn?ms=500; done'
kubectl get hpa api -n tp-finale -w
kubectl delete pod load -n tp-finale
```

L'API passe de 1 à 2 puis 3 réplicas en une à deux minutes.

RBAC :

```sh
kubectl auth can-i get configmap/app-config --as=system:serviceaccount:tp-finale:api -n tp-finale
kubectl auth can-i get secrets --as=system:serviceaccount:tp-finale:api -n tp-finale
```

Réponses : `yes` puis `no`.

NetworkPolicy, un pod autre que l'API ne joint pas MariaDB :

```sh
kubectl run nettest -n tp-finale --image=busybox:1.36 --restart=Never --rm -i --command -- nc -zv -w 3 mariadb 3306
```

Le `nc` échoue après 3 s.

## Limites lab face à la production

- Un seul nœud k3d : aucune tolérance à la panne.
- MariaDB en Deployment à une réplique sur `local-path` : pas de réplication, pas de sauvegarde, données liées au nœud.
- Secret créé à la main et stocké en base64 dans etcd : en production, un gestionnaire de secrets (Vault, Sealed Secrets).
- Pas de TLS sur l'Ingress.
- Pas de registry : image importée avec `k3d image import`, sans signature.
- Mots de passe d'exemple dans `.env.example` et dans les commandes ci-dessus.
- Ni monitoring ni logs centralisés.
- La CI ne déploie pas, elle build et scanne.
