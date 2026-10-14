# TP 3 : construire, durcir et publier sa propre image

## Point de départ

Jusqu'ici, la stack n'utilise que des images officielles (`wordpress`, `mariadb`, `openbao`). Le cours demande de conteneuriser une application à soi : c'est l'objet de ce TP.

Le dossier `app/` contient une petite API écrite en Go (il n'est pas nécessaire de connaître Go, ni de l'installer : tout se compile dans Docker). Elle lit les articles publiés de WordPress directement dans MariaDB :

| Route | Rôle |
|---|---|
| `GET /api/posts` | les 10 derniers articles publiés |
| `GET /healthz` | répond `ok` si le processus tourne (servira de *liveness probe* au TP 7) |
| `GET /readyz` | répond 200 si la base répond, 503 sinon (*readiness probe*) |
| `GET /version` | version compilée et nom d'hôte du conteneur |
| `GET /burn?ms=200` | consomme du CPU pendant 200 ms (pour l'autoscaling du TP 7) |

Elle se configure par variables d'environnement : `DB_HOST`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`. Comme les images `mariadb` et `wordpress`, elle accepte la variante `_FILE` (`DB_PASSWORD_FILE=/chemin`), et relit le fichier à chaque requête : elle fonctionne donc avec les identifiants dynamiques d'OpenBao du TP 2.

Le dépôt contient déjà la solution (`app/Dockerfile`, `app.compose.yml`). Pour faire le TP, mettez-les de côté :

```bash
mv app/Dockerfile app/Dockerfile.solution
mv app/.dockerignore app/.dockerignore.solution
```

À la fin du TP :

- l'image de l'API pèse moins de 20 Mo au lieu de 1,45 Go ;
- elle tourne sans shell, sans root, avec un système de fichiers en lecture seule ;
- elle est scannée (Trivy), inventoriée (SBOM) et publiée sur GitHub Container Registry avec un tag sémantique ;
- elle fait partie de la stack Compose, avec un compte MariaDB dynamique en lecture seule.

### Prérequis

- La stack des TP 1 et 2 démarre (`docker compose up -d`).
- Un compte GitHub, pour publier sur GHCR (partie 6).
- Testé avec Docker 29.8, `golang:1.27-alpine` (Go 1.27.2), `gcr.io/distroless/static-debian13:nonroot`, Trivy 0.74.

---

## Cours

### Un conteneur est un processus

Un conteneur est un processus ordinaire du noyau Linux de l'hôte, lancé avec deux mécanismes du noyau :

- **Les namespaces** limitent ce que le processus *voit*. Chaque type isole une ressource :

  | Namespace | Ce que le conteneur voit |
  |---|---|
  | `pid` | ses propres processus ; le premier a le PID 1 |
  | `net` | ses propres interfaces réseau, routes, ports |
  | `mnt` | son propre arbre de fichiers (celui de l'image) |
  | `uts` | son propre nom d'hôte |
  | `ipc` | ses propres files de messages et mémoire partagée |
  | `user` | une correspondance d'UID propre (rarement activé par défaut avec Docker) |
  | `cgroup` | sa propre vue de la hiérarchie des cgroups |

- **Les cgroups** limitent ce que le processus *consomme* : CPU, mémoire, nombre de processus, débit disque. Le `deploy.resources.limits` de `docker-compose.yml` se traduit en cgroups.

Conséquence directe : tous les conteneurs partagent **le noyau de l'hôte**. Une VM embarque son propre noyau, d'où des minutes de démarrage et des gigaoctets ; un conteneur démarre en millisecondes parce qu'il n'y a rien à démarrer, juste un processus à lancer. Le revers : une faille du noyau touche tous les conteneurs à la fois, et un processus `root` dans un conteneur est l'UID 0 pour le noyau. Sans user namespace, si ce processus s'échappe de ses namespaces, il est root sur l'hôte.

Vérifiez vous-même que le conteneur est un processus de l'hôte :

```bash
docker run -d --name dort alpine:3.22 sleep 300
docker top dort                      # vu par Docker
ps -ef | grep "sleep 300"            # vu par l'hôte : c'est le même processus, avec un autre PID
docker exec dort ps                  # vu de l'intérieur : PID 1
docker rm -f dort
```

(Avec Docker Desktop, le noyau tourne dans une VM : le `ps` de l'hôte Windows ou macOS ne voit pas le processus. Sous Linux ou dans la distribution WSL qui héberge le moteur, si.)

### Une image est une pile de couches

Une image est un ensemble de **couches** (des archives tar de fichiers) plus un fichier de configuration (commande par défaut, variables, utilisateur, ports). Chaque instruction `RUN`, `COPY` ou `ADD` d'un Dockerfile produit une couche ; les autres (`ENV`, `USER`, `CMD`, `EXPOSE`...) ne modifient que la configuration.

Au démarrage, Docker empile les couches en lecture seule avec le pilote de stockage `overlay2` et ajoute par-dessus une couche inscriptible propre au conteneur. Quand le conteneur modifie un fichier de l'image, le fichier est d'abord copié dans la couche du dessus (*copy-on-write*). Supprimer le conteneur supprime cette couche : c'est pour ça que les données doivent aller dans des volumes.

Deux conséquences pratiques :

1. **Supprimer un fichier dans une couche ultérieure ne réduit pas la taille de l'image.** `RUN apt-get install ...` puis `RUN rm -rf /var/lib/apt/lists/*` laisse les listes dans la première couche. Il faut nettoyer dans le même `RUN`.
2. **Les couches sont partagées.** Dix images construites `FROM debian:13` ne stockent la couche Debian qu'une fois sur le disque et dans le registry.

Chaque couche et chaque image sont identifiées par un **digest**, l'empreinte SHA-256 de leur contenu (`sha256:42dd2c...`). Le digest est calculé à partir du contenu : il ne peut pas désigner autre chose. Un **tag** (`1.2.0`, `latest`) est une étiquette posée sur un digest, que n'importe qui ayant le droit d'écrire sur le dépôt peut déplacer. `latest` n'a rien de spécial : c'est le tag utilisé quand on n'en précise aucun, pas forcément le plus récent.

Le format est standardisé par l'OCI (Open Container Initiative) : Docker, Podman, containerd et Kubernetes lisent les mêmes images. Une même référence peut désigner un *index* qui pointe vers une image par architecture (amd64, arm64) : `docker pull` choisit celle de la machine.

### Le build et son cache

`docker build .` envoie le **contexte** (le dossier `.`) au moteur de build (BuildKit), qui exécute le Dockerfile instruction par instruction. `.dockerignore` exclut des fichiers du contexte : moins de données envoyées, et surtout pas de `.git`, de `.env` ou de clés copiés par erreur dans l'image avec un `COPY . .`.

Le cache fonctionne couche par couche. Pour chaque instruction, BuildKit réutilise la couche existante si l'instruction est identique **et** si les fichiers qu'elle copie n'ont pas changé. Dès qu'une couche est reconstruite, toutes les suivantes le sont aussi. D'où la règle d'ordre : ce qui change rarement en haut (image de base, dépendances), ce qui change à chaque commit en bas (le code).

### Multi-stage et choix de l'image de base

Compiler demande un compilateur, des en-têtes, des outils ; exécuter n'en demande aucun. Un Dockerfile **multi-stage** a plusieurs `FROM` : on compile dans une première étape, puis on copie seulement le résultat dans une image finale minimale. Les étapes intermédiaires ne sont pas dans l'image publiée.

| Base | Contenu | Taille indicative | À savoir |
|---|---|---|---|
| `debian:13`, `golang:1.27` | distribution complète | 100 Mo à 1 Go | pratique pour déboguer, beaucoup de paquets donc beaucoup de CVE |
| `*-slim` | Debian allégée | 30 à 80 Mo | bon compromis pour Python, Node, PHP |
| `alpine` | musl libc + busybox | 5 à 10 Mo | musl n'est pas glibc : certains binaires et extensions natives se comportent autrement |
| `gcr.io/distroless/*` | uniquement les bibliothèques d'exécution, certificats CA, fuseau horaire | 2 à 30 Mo | pas de shell ni de gestionnaire de paquets |
| `scratch` | rien | 0 | pour un binaire statique ; il faut alors apporter soi-même les certificats CA |

Moins il y a de fichiers dans l'image, moins il y a de CVE à corriger et moins un attaquant trouve d'outils une fois entré : pas de `sh`, pas de `curl`, pas de `apt`.

### Durcir l'exécution

La checklist du cours, et ce que fait chaque point :

- **Utilisateur non-root** (`USER` dans le Dockerfile). Le processus n'a plus l'UID 0 : il ne peut pas modifier les fichiers système de l'image, et une évasion le laisse simple utilisateur sur l'hôte.
- **Capabilities réduites** (`cap_drop: [ALL]`). Linux découpe les pouvoirs de root en une quarantaine de *capabilities* (`NET_BIND_SERVICE` pour écouter sous le port 1024, `CHOWN`, `SETUID`, `NET_RAW` pour forger des paquets...). Docker en accorde 14 par défaut ; une API qui écoute sur 8080 n'en utilise aucune.
- **`no-new-privileges`** : interdit à un processus de gagner des droits en exécutant un binaire setuid (`sudo`, `su`, `ping` sur certaines distributions).
- **Système de fichiers en lecture seule** (`read_only: true`). Un attaquant ne peut ni déposer un binaire ni modifier le code. Les dossiers où l'application doit écrire se montent en `tmpfs` ou en volume.
- **Pas de `--privileged`** : ce drapeau rend toutes les capabilities, tous les périphériques de l'hôte et désactive seccomp et AppArmor. Il n'a sa place que pour des outils système très particuliers.
- **seccomp** : Docker applique par défaut un filtre qui bloque une quarantaine d'appels système dangereux (`mount`, `reboot`, `kexec_load`...). Ne pas le désactiver.

### Vulnérabilités et chaîne d'approvisionnement

Un **scanner** comme Trivy lit la liste des paquets de l'image (base `dpkg` de Debian, base `apk` d'Alpine, métadonnées des binaires Go, `package-lock.json`, etc.) et la compare à des bases de CVE. Il ne teste pas l'application : il ne trouvera ni injection SQL ni mot de passe faible dans votre code. Une CVE est dite *fixed* quand une version corrigée du paquet existe, *unfixed* sinon ; on bloque en général la CI sur les HIGH et CRITICAL corrigeables, puisque ce sont les seules sur lesquelles on peut agir.

Un **SBOM** (*Software Bill of Materials*) est l'inventaire de tous les composants de l'image, dans un format standard (SPDX ou CycloneDX). Quand une nouvelle faille sort (comme log4shell en 2021), on cherche dans les SBOM au lieu de rescanner chaque image.

**Épingler** une image par digest (`FROM golang@sha256:...`) garantit qu'on reconstruit exactement la même chose. Mais une image épinglée ne reçoit plus les correctifs : sans outil qui propose les mises à jour (Dependabot, Renovate, voir TP 4), l'épinglage fige aussi les failles.

### Registries

Un registry stocke et distribue les images. Les plus courants : Docker Hub (le registry par défaut quand une référence n'a pas de domaine), GitHub Container Registry (`ghcr.io`), et ceux des clouds (Azure Container Registry au TP 9). Le nom complet d'une image est `registry/espace/nom:tag`, par exemple `ghcr.io/mateonicoud/tp-api:1.0.0`.

Docker Hub limite le nombre de téléchargements par heure, plus strictement pour les clients anonymes que pour les comptes connectés (chiffres actuels sur [la page Docker Hub usage](https://docs.docker.com/docker-hub/usage/)). Une salle de TP derrière une seule IP publique atteint vite la limite anonyme : connectez-vous (`docker login`).

---

## Partie 0 : deux conteneurs sans Compose

Compose fait beaucoup de choses sans le dire. Refaites-les à la main une fois.

**Consignes.**

1. Lancez deux conteneurs `alpine:3.22` (`sleep 600`) nommés `alpha` et `beta` **sans** préciser de réseau. Depuis `alpha`, essayez `ping -c1 beta`.
2. Créez un réseau `labnet`. Relancez `alpha` et `beta` dessus et refaites le ping.
3. Créez un volume `labdata`. Lancez `mariadb:13.0` nommé `db` sur `labnet`, avec ce volume sur `/var/lib/mysql`, un mot de passe root et l'option `--restart unless-stopped`. Ne publiez aucun port.
4. Créez une table et une ligne, supprimez le conteneur (`docker rm -f db`), recréez-le avec le même volume : la ligne est-elle toujours là ?
5. Avec `docker network inspect labnet` et `docker inspect db`, notez l'IP de `db`, son nom DNS et ses ports publiés.
6. Redémarrez Docker (Docker Desktop : *Restart* ; Linux : `sudo systemctl restart docker`). Quels conteneurs repartent seuls ?
7. Nettoyez tout (conteneurs, réseau, volume).

**Questions.**

1. Pourquoi le ping par nom échoue-t-il sur le réseau par défaut ?
2. Quelle est la différence entre `--restart always` et `--restart unless-stopped` ?
3. La stack Compose du dépôt n'a aucune politique `restart`. Si on en ajoutait une, la stack redémarrerait-elle correctement après un reboot ? Pensez à `bao-init`.

<details>
<summary>Solution</summary>

```bash
# 1. Réseau "bridge" par défaut : pas de DNS entre conteneurs
docker run -d --name alpha alpine:3.22 sleep 600
docker run -d --name beta alpine:3.22 sleep 600
docker exec alpha ping -c1 beta     # ping: bad address 'beta'
docker rm -f alpha beta

# 2. Réseau créé par l'utilisateur : Docker y fournit un DNS interne (127.0.0.11)
docker network create labnet
docker run -d --name alpha --network labnet alpine:3.22 sleep 600
docker run -d --name beta --network labnet alpine:3.22 sleep 600
docker exec alpha ping -c1 beta     # répond
docker rm -f alpha beta

# 3. Base avec volume nommé, sans port publié, redémarrée par le démon
docker volume create labdata
docker run -d --name db --network labnet --restart unless-stopped \
  -e MARIADB_ROOT_PASSWORD=lab -v labdata:/var/lib/mysql mariadb:13.0

# 4. Les données sont dans le volume, pas dans le conteneur
sleep 15
docker exec db mariadb -uroot -plab -e "CREATE DATABASE t; CREATE TABLE t.x (i INT); INSERT INTO t.x VALUES (42);"
docker rm -f db
docker run -d --name db --network labnet --restart unless-stopped \
  -e MARIADB_ROOT_PASSWORD=lab -v labdata:/var/lib/mysql mariadb:13.0
sleep 10
docker exec db mariadb -uroot -plab -e "SELECT * FROM t.x;"   # 42

# 5. Topologie
docker network inspect labnet --format '{{range .Containers}}{{.Name}} {{.IPv4Address}}{{"\n"}}{{end}}'
docker inspect db --format '{{json .NetworkSettings.Ports}}'  # {"3306/tcp":null} : exposé, pas publié

# 7. Nettoyage
docker rm -f db && docker network rm labnet && docker volume rm labdata
```

1. Le réseau `bridge` par défaut existe pour la compatibilité avec les anciennes versions de Docker, qui reliaient les conteneurs avec `--link`. Il n'a pas de DNS. Tout réseau créé par l'utilisateur (et tous ceux que crée Compose) en a un : les conteneurs se trouvent par leur nom et leurs alias.
2. Les deux redémarrent le conteneur s'il s'arrête et au démarrage du démon. `always` le redémarre même si vous l'aviez arrêté à la main avant le redémarrage du démon ; `unless-stopped` respecte cet arrêt manuel.
3. Non. Les politiques `restart` sont appliquées par le démon Docker, qui ne connaît pas `depends_on` : il relance les conteneurs dans n'importe quel ordre. OpenBao redémarrerait scellé, et `bao-init`, terminé avec le code 0, ne serait pas relancé. WordPress continuerait avec les identifiants déjà écrits tant que leur bail n'a pas expiré, puis tomberait. Après un reboot, la bonne commande reste `docker compose up -d`, qui rejoue l'ordre et les étapes d'initialisation. Sur un serveur, on la lance depuis une unité systemd.

</details>

---

## Partie 1 : un Dockerfile naïf

**Consignes.**

1. Dans `app/`, écrivez le Dockerfile le plus simple possible : partir de `golang:1.27`, copier tout le dossier, compiler avec `go build -o /api .`, lancer `/api`.
2. Construisez-le sous le nom `tp-api:naif` et lancez-le en publiant le port 8080 sur `127.0.0.1:18080`. Appelez `/healthz` et `/version`.
3. Notez la taille de l'image (`docker images tp-api`) et regardez ses couches (`docker history tp-api:naif`).
4. Ouvrez un shell dans le conteneur. Avec quel utilisateur tournez-vous ? Quels outils trouvez-vous (`go`, `git`, `curl`, `apt`) ?
5. Scannez l'image avec Trivy, sans l'installer :

   ```bash
   docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
     aquasec/trivy:0.74.0 image --severity HIGH,CRITICAL tp-api:naif
   ```

**Questions.**

1. Pourquoi l'image pèse-t-elle autant alors que le binaire fait quelques mégaoctets ?
2. Combien de vulnérabilités HIGH et CRITICAL ? Dans quoi : dans votre code ou dans l'image de base ?
3. `/version` répond `"version":"dev"`. Comment injecter le vrai numéro de version au build ?

<details>
<summary>Solution</summary>

```dockerfile
# Dockerfile naïf : fonctionne, mais tout ce qui a servi à compiler part en production.
FROM golang:1.27
WORKDIR /src
COPY . .
RUN go build -o /api .
CMD ["/api"]
```

```bash
cd app
docker build -t tp-api:naif .
docker run --rm -d --name naif -p 127.0.0.1:18080:8080 tp-api:naif
curl -s localhost:18080/healthz
curl -s localhost:18080/version
docker images tp-api:naif                     # environ 1,45 Go
docker history tp-api:naif
docker exec -it naif bash -c 'id; which go git curl apt'
docker rm -f naif
```

1. L'image contient Debian complète, la chaîne de compilation Go (plus de 200 Mo), git, gcc, les sources et le cache de compilation. Le binaire ne représente que quelques mégaoctets sur 1,45 Go.
2. Environ 160 au moment de l'écriture, presque toutes dans les paquets Debian de l'image de base (le binaire `api` lui-même : 0). Elles ne concernent pas votre code, mais elles sont dans votre image : c'est vous qui les livrez en production.
3. Avec `-ldflags "-X main.version=1.2.0"` à la compilation, la variable `version` de `main.go` est remplacée. On passe la valeur au build avec un `ARG` (partie 3).

</details>

---

## Partie 2 : l'ordre des couches et le cache

**Consignes.**

1. Reconstruisez `tp-api:naif` sans rien changer, en mesurant le temps (`time docker build ...`).
2. Ajoutez un commentaire dans `main.go` et reconstruisez. Regardez quelles étapes sont rejouées (`docker build --progress=plain`).
3. Réécrivez le Dockerfile pour que le téléchargement des dépendances (`go mod download`) ne se rejoue que si `go.mod` ou `go.sum` changent.
4. Écrivez un `.dockerignore` qui n'envoie au build que `go.mod`, `go.sum` et les fichiers `.go`, sans les tests.

**Questions.**

1. Pourquoi l'ajout d'un commentaire relance-t-il le téléchargement de toutes les dépendances dans la version naïve ?
2. Que se passerait-il avec un `COPY . .` sans `.dockerignore`, si le dossier contenait un fichier `.env` ?

<details>
<summary>Solution</summary>

```dockerfile
FROM golang:1.27
WORKDIR /src

# Ces deux fichiers changent rarement : la couche "go mod download" reste en cache
# tant qu'ils sont identiques, même si le code change.
COPY go.mod go.sum ./
RUN go mod download

# Le code change à chaque commit : il vient après.
COPY . .
RUN go build -o /api .
CMD ["/api"]
```

```text
# .dockerignore : on exclut tout, puis on ré-autorise le strict nécessaire.
*
!go.mod
!go.sum
!*.go
*_test.go
```

1. `COPY . .` copie `main.go`. Le fichier a changé, donc la couche est invalidée, et toutes les couches suivantes aussi, y compris la compilation qui télécharge les modules. En copiant d'abord `go.mod` et `go.sum` seuls, le téléchargement dépend uniquement de ces deux fichiers.
2. Le `.env` serait copié dans une couche de l'image. Même supprimé plus tard dans le Dockerfile, il resterait lisible dans cette couche (`docker save`, puis ouvrir les archives tar). Une liste d'exclusion « tout sauf » (`*` puis `!`) protège mieux qu'une liste « rien sauf » : un nouveau fichier sensible est exclu par défaut.

</details>

---

## Partie 3 : multi-stage et distroless

**Consignes.**

1. Transformez le Dockerfile en deux étapes :
   - une étape `build` basée sur `golang:1.27-alpine` qui produit un binaire **statique** (`CGO_ENABLED=0`) ;
   - une étape finale basée sur `gcr.io/distroless/static-debian13:nonroot` qui ne contient que ce binaire.
2. Ajoutez un `ARG VERSION` passé à `-ldflags "-X main.version=${VERSION}"`, et réduisez le binaire avec `-trimpath -ldflags "-s -w"`.
3. Construisez `tp-api:0.1.0` avec `--build-arg VERSION=0.1.0`. Comparez la taille et le nombre de CVE avec la version naïve.
4. Essayez d'ouvrir un shell dans le conteneur.

**Questions.**

1. Pourquoi `CGO_ENABLED=0` est-il nécessaire pour l'image `static` ?
2. L'image n'a ni `curl` ni shell. Comment écrire un `HEALTHCHECK` ?
3. Comment déboguer un conteneur qui n'a pas de shell ?

<details>
<summary>Solution</summary>

Le fichier complet est `app/Dockerfile.solution` (le `Dockerfile` du dépôt) :

```dockerfile
# syntax=docker/dockerfile:1

# --- Étape 1 : compilation. Image lourde (~380 Mo), jetée à la fin. ---
ARG GO_VERSION=1.27
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Dépendances d'abord : cette couche reste en cache tant que go.mod/go.sum ne bougent pas.
COPY go.mod go.sum ./
RUN go mod download

# Puis le code, qui change à chaque commit.
COPY . .
ARG VERSION=dev
# CGO_ENABLED=0 : binaire statique, pas besoin de libc dans l'image finale.
# -trimpath et -s -w : pas de chemins locaux ni de symboles de debug dans le binaire.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/api .

# --- Étape 2 : exécution. Ni shell, ni gestionnaire de paquets, utilisateur non-root. ---
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/api /api
# nonroot = UID 65532. Redit ici pour que ce soit visible à la lecture.
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/api", "healthcheck"]
ENTRYPOINT ["/api"]
```

```bash
docker build -t tp-api:0.1.0 --build-arg VERSION=0.1.0 app
docker images tp-api                       # 0.1.0 : environ 18 Mo ; naif : 1,45 Go
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  aquasec/trivy:0.74.0 image --severity HIGH,CRITICAL tp-api:0.1.0   # 0 au moment de l'écriture
docker run --rm -d --name api -p 127.0.0.1:18080:8080 tp-api:0.1.0
curl -s localhost:18080/version             # {"version":"0.1.0", ...}
docker exec -it api sh                      # exec: "sh": executable file not found
docker rm -f api
```

1. Avec cgo, le compilateur Go lie le binaire à la libc du système de build (musl sur Alpine). L'image `distroless/static` n'a aucune libc : le binaire ne démarrerait pas (« no such file or directory », message trompeur qui désigne en fait le chargeur de bibliothèques manquant). `CGO_ENABLED=0` produit un binaire qui n'a besoin de rien. Pour un programme qui a besoin de la libc, on prendrait `distroless/base`.
2. Le programme sait se tester lui-même : `/api healthcheck` appelle `http://127.0.0.1:8080/healthz` et sort avec le code 0 ou 1 (voir la fonction `healthcheck` de `main.go`). Le `HEALTHCHECK` utilise la forme tableau (`["/api", "healthcheck"]`), qui n'a pas besoin de shell.
3. Plusieurs options : `docker logs`, `docker inspect`, `docker cp` pour extraire des fichiers, ou lancer un conteneur outil qui partage les namespaces du conteneur cible : `docker run --rm -it --pid container:api --network container:api nicolaka/netshoot`. Kubernetes fait la même chose avec `kubectl debug` (TP 5).

</details>

---

## Partie 4 : durcir l'exécution

**Consignes.**

1. Vérifiez avec quel utilisateur tourne le conteneur : `docker inspect --format '{{.Config.User}}' tp-api:0.1.0`.
2. Lancez l'API avec, en plus : système de fichiers en lecture seule, toutes les capabilities retirées, `no-new-privileges`, 64 Mo de mémoire et un demi-CPU au maximum.
3. Vérifiez qu'elle fonctionne toujours. Puis prouvez que l'écriture est impossible en copiant un fichier dedans (`docker cp`).
4. Faites la même chose avec l'image `wordpress:7.1` : lancez-la avec `--read-only --cap-drop ALL`. Que se passe-t-il ?

**Questions.**

1. Qu'apporte `USER` dans le Dockerfile par rapport à `user:` dans Compose ?
2. Pourquoi WordPress ne supporte-t-il pas ce durcissement tel quel ? Que faudrait-il faire ?

<details>
<summary>Solution</summary>

```bash
docker inspect --format '{{.Config.User}}' tp-api:0.1.0     # 65532:65532

# --read-only                    : couche du conteneur en lecture seule
# --cap-drop ALL                 : aucune capability ; l'API écoute sur 8080, elle n'en a pas besoin
# --security-opt no-new-privileges : pas d'élévation via un binaire setuid
# --memory / --cpus              : limites appliquées par les cgroups
docker run --rm -d --name api -p 127.0.0.1:18080:8080 \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  --memory 64m --cpus 0.5 \
  tp-api:0.1.0
curl -s localhost:18080/healthz
echo test > /tmp/x && docker cp /tmp/x api:/x    # échec : read-only file system
docker rm -f api

docker run --rm --read-only --cap-drop ALL wordpress:7.1
# L'entrypoint échoue dès la copie de WordPress dans /var/www/html : sans la capability CHOWN,
# root ne peut pas donner les fichiers à www-data. Ensuite Apache aurait aussi besoin d'écrire
# dans /var/run/apache2, ce que --read-only interdit.
```

L'erreur de WordPress : `tar: .: Cannot change ownership to uid 33, gid 33: Operation not permitted`.

1. `USER` fait du non-root le comportement par défaut de l'image, partout où elle tourne : `docker run`, Compose, Kubernetes. `user:` dans Compose ne protège que les lancements faits avec ce fichier. Kubernetes s'appuie sur l'utilisateur de l'image pour vérifier `runAsNonRoot` (TP 7).
2. L'image officielle est faite pour démarrer en root : Apache ouvre le port 80 (il faut `NET_BIND_SERVICE`), puis bascule vers `www-data` (il faut `SETUID` et `SETGID`), et l'entrypoint copie et `chown` des fichiers. Pour la durcir, il faudrait une image dérivée : Apache sur le port 8080, `USER www-data`, WordPress déjà copié au build, et des `tmpfs` pour `/var/run/apache2` et `/tmp`. C'est faisable, mais c'est une image à maintenir. Au TP 7, on retire toutes les capabilities sauf celles dont Apache a besoin, et on documente l'écart.

</details>

---

## Partie 5 : scanner et inventorier

**Consignes.**

1. Scannez `tp-api:0.1.0` et `wordpress:7.1` avec Trivy, en ne gardant que les vulnérabilités corrigeables (`--ignore-unfixed`).
2. Générez un SBOM de `tp-api:0.1.0` au format SPDX JSON avec Syft (`anchore/syft`), puis faites scanner ce SBOM par Trivy (`trivy sbom`).
3. Faites échouer une commande de scan (code de sortie non nul) dès qu'une CVE CRITICAL corrigeable est trouvée. Testez sur `wordpress:7.1`.

**Questions.**

1. Votre image a 0 CVE aujourd'hui. Sera-ce encore vrai dans six mois, sans aucun changement de code ?
2. Pourquoi `--ignore-unfixed` dans une CI ? Quel risque prend-on ?

<details>
<summary>Solution</summary>

```bash
# Alias pour ne pas réécrire le montage du socket Docker à chaque fois.
trivy() { docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$PWD:/work" -w /work aquasec/trivy:0.74.0 "$@"; }
syft()  { docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$PWD:/work" -w /work anchore/syft:v1.54.1 "$@"; }

trivy image --ignore-unfixed --severity HIGH,CRITICAL tp-api:0.1.0
trivy image --ignore-unfixed --severity HIGH,CRITICAL wordpress:7.1

# SBOM : l'inventaire, écrit une fois...
syft tp-api:0.1.0 -o spdx-json=sbom.spdx.json
# ...puis rescanné à volonté, sans retélécharger l'image.
trivy sbom sbom.spdx.json

# Barrière : code de sortie 1 si une CRITICAL corrigeable existe.
trivy image --ignore-unfixed --severity CRITICAL --exit-code 1 wordpress:7.1; echo "code: $?"
```

1. Non. Les bases de CVE grossissent chaque jour : une image figée accumule des vulnérabilités connues sans qu'une ligne ne change. Il faut reconstruire régulièrement (nouvelle image de base, nouvelles dépendances) et rescanner les images déjà en production, pas seulement au moment du build.
2. Une CVE sans correctif ne peut pas être corrigée par une mise à jour : bloquer la CI dessus bloquerait toutes les livraisons sans moyen d'en sortir. Le risque est de livrer une faille connue. Il faut donc les suivre ailleurs (rapport complet archivé, décision écrite pour chacune : ignorer, contourner, changer de base), par exemple avec un fichier `.trivyignore` commenté et daté.

</details>

---

## Partie 6 : tags, digests et publication

**Consignes.**

1. Créez sur GitHub un *Personal Access Token (classic)* avec le droit `write:packages` uniquement, et connectez-vous : `docker login ghcr.io -u <votre-login>`.
2. Taguez l'image `ghcr.io/<login>/tp-api:0.1.0` (le nom doit être en minuscules) et poussez-la.
3. Relevez le digest affiché par `docker push` (`sha256:...`), puis supprimez l'image locale et retéléchargez-la **par digest**.
4. Modifiez une ligne de code, reconstruisez avec `VERSION=0.1.1`, poussez sous `0.1.1`. Ajoutez le tag `0.1` sur cette même image et poussez-le.
5. Supprimez les images locales devenues inutiles (`docker image prune`).

**Questions.**

1. Un collègue pousse par erreur une autre image sous le tag `0.1.0`. Qui est touché : ceux qui déploient par tag, ou par digest ?
2. Le cours conseille de « promouvoir une image de staging vers prod par un retag, sans reconstruire ». Pourquoi ne pas simplement reconstruire depuis le même commit ?
3. Le paquet est privé par défaut sur GHCR. Comment Kubernetes pourra-t-il le télécharger ?

<details>
<summary>Solution</summary>

```bash
docker login ghcr.io -u mateonicoud            # mot de passe : le token, pas le mot de passe GitHub

docker tag tp-api:0.1.0 ghcr.io/mateonicoud/tp-api:0.1.0
docker push ghcr.io/mateonicoud/tp-api:0.1.0   # dernière ligne : "digest: sha256:..."

docker rmi ghcr.io/mateonicoud/tp-api:0.1.0 tp-api:0.1.0
docker pull ghcr.io/mateonicoud/tp-api@sha256:<digest>

# Nouvelle version : un tag précis (0.1.1) et un tag "flottant" (0.1) qui suit la dernière 0.1.x
docker build -t ghcr.io/mateonicoud/tp-api:0.1.1 --build-arg VERSION=0.1.1 app
docker tag ghcr.io/mateonicoud/tp-api:0.1.1 ghcr.io/mateonicoud/tp-api:0.1
docker push ghcr.io/mateonicoud/tp-api:0.1.1
docker push ghcr.io/mateonicoud/tp-api:0.1

docker image prune          # images sans tag (« dangling ») ; ajoutez -a pour toutes les images inutilisées
```

1. Ceux qui déploient par tag reçoivent la mauvaise image au prochain téléchargement. Ceux qui déploient par digest ne voient rien : le digest désigne un contenu, il ne peut pas être réassigné. C'est pour ça que le TP 9 déploie par digest.
2. Deux builds du même commit ne donnent pas forcément la même image : l'image de base `golang:1.27-alpine` a pu changer entre-temps (nouveau correctif), comme les dépendances non épinglées. L'image testée en staging ne serait plus celle de la production. Le retag (ou l'import dans un autre registry, TP 9) garantit le même digest, donc exactement ce qui a été testé.
3. Deux possibilités : rendre le paquet public (Settings du paquet sur GitHub), ou créer dans Kubernetes un Secret de type `docker-registry` et le référencer dans `imagePullSecrets`. Au TP 9, AKS télécharge depuis Azure Container Registry avec son identité managée, sans mot de passe.

</details>

---

## Partie 7 : l'API dans la stack Compose

**Consignes.** Créez `app.compose.yml` et ajoutez-le à `COMPOSE_FILE` dans `.env`.

1. Service `api` :
   - construit depuis `./app` (`build:`), avec `VERSION` en argument de build, et nommé par `image:` ;
   - branché sur `internal` (pour MariaDB) et `public` ;
   - port publié sur `127.0.0.1:8081` ;
   - durci comme à la partie 4 (`user`, `read_only`, `cap_drop`, `security_opt`, limites) ;
   - démarré après MariaDB et l'agent OpenBao.
2. L'API n'a besoin que de lire : donnez-lui un compte MariaDB dynamique **en lecture seule**, distinct de celui de WordPress. Le rôle `wp-readonly` du TP 2 existe déjà ; il faut l'autoriser dans la policy de l'agent, ajouter les templates qui écrivent `db_user` et `db_password` dans un volume `bao_api`, et ne monter ce volume que dans l'API.
3. Ajoutez un service `adminer` (interface web pour la base) dans un **profil** `debug`, publié sur `127.0.0.1:8082`.
4. Lancez `docker compose up -d --build`, publiez un article dans WordPress et vérifiez qu'il apparaît dans `/api/posts`. Lancez Adminer avec `docker compose --profile debug up -d`.

**Questions.**

1. Que fait `docker compose up` quand `build:` et `image:` sont tous deux présents ?
2. Pourquoi un volume séparé `bao_api` plutôt que de réutiliser `bao_secrets` ?
3. Quel est l'intérêt d'un profil plutôt que d'un fichier Compose de plus ?

<details>
<summary>Solution</summary>

`app.compose.yml` (fichier du dépôt) :

```yaml
services:
  api:
    build:
      context: ./app
      args:
        VERSION: ${API_VERSION:-dev}       # ARG VERSION du Dockerfile
    image: ${API_IMAGE:-tp-api}:${API_VERSION:-dev}   # nom donné à l'image construite
    environment:
      DB_HOST: mariadb:3306
      DB_NAME: ${DB_NAME}
      DB_USER_FILE: /run/bao-api/db_user          # écrits par bao-agent
      DB_PASSWORD_FILE: /run/bao-api/db_password
    volumes:
      - bao_api:/run/bao-api:ro                   # lecture seule : l'API ne fait que lire
    ports:
      - "127.0.0.1:${API_PORT:-8081}:8080"
    networks:
      - internal                                  # pour joindre mariadb
      - public                                    # pour publier un port (internal: true ne le permet pas)
    user: "65532:65532"
    read_only: true
    cap_drop: [ALL]
    security_opt:
      - no-new-privileges:true
    deploy:
      resources:
        limits:
          cpus: "0.5"
          memory: 64M
    depends_on:
      bao-agent:
        condition: service_healthy                # les fichiers d'identifiants existent
      mariadb:
        condition: service_healthy

  adminer:
    image: adminer:${ADMINER_VERSION:-5.4.1}
    profiles: [debug]                             # ignoré par "docker compose up" sans --profile debug
    environment:
      ADMINER_DEFAULT_SERVER: mariadb
    ports:
      - "127.0.0.1:${ADMINER_PORT:-8082}:8080"
    networks:
      - internal
      - public
```

Dans `openbao/bootstrap.sh`, la policy de l'agent gagne un chemin :

```hcl
path "database/creds/wp-readonly" {
  capabilities = ["read"]
}
```

Dans `openbao/agent/agent.hcl`, deux templates de plus, vers un autre dossier :

```hcl
template {
  contents    = "{{ with secret \"database/creds/wp-readonly\" }}{{ .Data.username }}{{ end }}"
  destination = "/run/bao-api/db_user"
  perms       = "0444"
}
template {
  contents    = "{{ with secret \"database/creds/wp-readonly\" }}{{ .Data.password }}{{ end }}"
  destination = "/run/bao-api/db_password"
  perms       = "0444"
}
```

Et dans `openbao.compose.yml`, `bao-agent` monte `bao_api:/run/bao-api`, son healthcheck attend aussi `/run/bao-api/db_password`, et le volume `bao_api` est déclaré.

```bash
docker compose up -d --build
curl -s localhost:8081/api/posts
docker compose --profile debug up -d          # Adminer sur http://localhost:8082
```

1. Compose construit l'image si elle n'existe pas localement (ou toujours, avec `--build`) et lui donne le nom de `image:`. Sans `image:`, le nom serait généré (`<projet>-api`). Avec un nom explicite, on peut ensuite la pousser telle quelle (`docker compose push api`).
2. Un conteneur voit tout ce qui est dans les volumes qu'il monte. Avec un seul volume, l'API pourrait lire les identifiants de WordPress, qui ont tous les droits sur la base. Un volume par application, c'est le moindre privilège appliqué aux fichiers.
3. Le profil garde le service dans le même fichier, avec les mêmes réseaux et variables, mais ne le démarre que sur demande. Les outils de debug (Adminer, Mailhog) n'ouvrent ainsi aucun port tant qu'on n'en a pas besoin.

</details>

---

## Bilan

Le jalon Docker du cours demande une stack clonable, un Dockerfile durci, un scan commenté, et un README qui permet de tout refaire. Vérifiez sur un clone propre :

```bash
git clone <votre dépôt> /tmp/clone && cd /tmp/clone
cp .env.example .env
docker compose up -d --build
docker compose ps -a
```

Répondez en quelques lignes :

1. Citez trois éléments de la checklist sécurité du cours et l'endroit du dépôt où chacun est appliqué.
2. Votre image fait 18 Mo et n'a aucune CVE connue. Qu'est-ce qui pourrait quand même mal tourner en production ?
3. Où l'image de production doit-elle être construite : sur votre poste ou ailleurs ? Le TP 4 répond.
