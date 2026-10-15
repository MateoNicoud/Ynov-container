# TP 1 : sortir les secrets de la stack WordPress avec OpenBao

## Point de départ

Le dépôt contient une stack Compose à deux services :

- `wordpress`, exposé sur `${WORDPRESS_PORT}`, branché sur les réseaux `internal` et `public` ;
- `mariadb`, joignable uniquement par le réseau `internal` (`internal: true`), avec un healthcheck dont dépend WordPress (`condition: service_healthy`) et un volume `db_data`.

Toute la configuration passe par le fichier `.env`, mots de passe compris. C'est là que ce TP commence. Vérifiez par vous-même :

```bash
cp .env.example .env
docker compose up -d
docker compose exec wordpress env | grep PASSWORD
docker inspect "$(docker compose ps -q mariadb)" --format '{{json .Config.Env}}'
```

Le mot de passe de la base apparaît en clair dans l'environnement du conteneur et dans `docker inspect`. N'importe qui ayant accès au socket Docker le lit. Il traîne aussi dans `.env`, qu'un `git add .` distrait finit par pousser.

À la fin du TP :

- les mots de passe vivent dans OpenBao ;
- un conteneur d'initialisation les récupère avec une identité AppRole limitée en lecture ;
- MariaDB et WordPress lisent leurs mots de passe dans des fichiers, plus dans des variables d'environnement ;
- OpenBao tourne en mode serveur avec stockage persistant, et vous savez le desceller.

### OpenBao en deux phrases

OpenBao est un gestionnaire de secrets open source (licence MPL 2.0), hébergé par la Linux Foundation. C'est un fork de HashiCorp Vault créé après le passage de Vault sous licence BSL en 2023 : la CLI s'appelle `bao` au lieu de `vault`, et la plupart des commandes et chemins d'API sont identiques.

### Prérequis

- La stack du dépôt démarre sans erreur.
- Docker Compose 2.24 ou plus récent (`docker compose version`), pour la balise `!reset` utilisée à la partie 6.
- Testé avec `openbao/openbao:2.7.1`. Les tags disponibles sont listés sur [Docker Hub](https://hub.docker.com/r/openbao/openbao/tags).

Les solutions sont repliées sous chaque partie. Cherchez d'abord.

---

## Partie 1 : démarrer OpenBao en mode dev

On garde `docker-compose.yml` intact et on ajoute un second fichier, `openbao.compose.yml`. Compose fusionne les fichiers listés dans la variable `COMPOSE_FILE`. Ajoutez au `.env` :

```dotenv
COMPOSE_FILE=docker-compose.yml:openbao.compose.yml

OPENBAO_VERSION=2.7.1
BAO_PORT=8200
BAO_ROOT_TOKEN=root
```

(Sous Windows, le séparateur de `COMPOSE_FILE` est `;`.)

**Consignes.** Dans `openbao.compose.yml`, déclarez un service `openbao` qui :

1. utilise l'image `openbao/openbao:${OPENBAO_VERSION}` ;
2. démarre avec la commande `server -dev` ;
3. fixe le token root via la variable `BAO_DEV_ROOT_TOKEN_ID` ;
4. définit `BAO_ADDR=http://127.0.0.1:8200` pour que la CLI dans le conteneur sache où parler ;
5. publie le port 8200 uniquement sur 127.0.0.1 ;
6. est branché sur un nouveau réseau `secrets` ;
7. a un healthcheck basé sur `bao status`.

Puis :

```bash
docker compose up -d openbao
docker compose ps
docker compose exec openbao bao status
```

**Questions.**

1. Pourquoi ne pas brancher OpenBao sur le réseau `internal` existant ?
2. Que se passerait-il avec `ports: - "8200:8200"` sur une machine avec une IP publique ?
3. `bao status` renvoie un code de sortie. Lequel quand OpenBao est scellé ? Pourquoi est-ce pratique pour un healthcheck ?
4. Le mode dev affiche « Storage Type: inmem ». Qu'est-ce que ça implique ?

<details>
<summary>Solution</summary>

```yaml
services:
  openbao:
    image: openbao/openbao:${OPENBAO_VERSION}
    command: server -dev
    cap_add: [IPC_LOCK]
    environment:
      BAO_DEV_ROOT_TOKEN_ID: ${BAO_ROOT_TOKEN}
      BAO_ADDR: http://127.0.0.1:8200
    ports:
      - "127.0.0.1:${BAO_PORT}:8200"
    networks:
      - secrets
    healthcheck:
      test: ["CMD", "bao", "status"]
      interval: 5s
      timeout: 3s
      retries: 10

networks:
  secrets:
```

`cap_add: [IPC_LOCK]` autorise OpenBao à verrouiller sa mémoire (`mlock`) pour que les secrets ne partent pas dans le swap.

1. OpenBao n'a rien à faire avec MariaDB. Le réseau `secrets` isole le trafic lié aux secrets : seuls les services qui en ont besoin y seront branchés.
2. L'interface d'OpenBao serait exposée sur Internet, avec un token root qui vaut `root`.
3. `0` si descellé, `2` si scellé, `1` en cas d'erreur. Un OpenBao scellé ne sert aucun secret, le healthcheck le marque donc `unhealthy`, ce qui est exact.
4. Tout est en mémoire. Au prochain redémarrage du conteneur, les secrets, policies et méthodes d'authentification ont disparu. La partie 7 règle ça.

</details>

---

## Partie 2 : premiers secrets avec le moteur KV v2

En mode dev, un moteur KV version 2 est déjà monté sur `secret/`.

```bash
docker compose exec -e BAO_TOKEN=root openbao bao secrets list
```

**Consignes.** Toutes les commandes se lancent avec `docker compose exec -e BAO_TOKEN=root openbao bao ...`. Pour alléger, définissez un alias dans votre shell :

```bash
alias bao='docker compose exec -e BAO_TOKEN=root openbao bao'
```

1. Stockez dans `secret/wordpress/db` trois clés : `user`, `password` et `root_password`. Reprenez les valeurs de votre `.env`.
2. Relisez le secret entier, puis uniquement le champ `password`.
3. Changez `password`, puis relisez la version 1.
4. Affichez les métadonnées du secret.
5. Supprimez la dernière version, constatez, puis annulez la suppression.
6. Remettez le mot de passe d'origine (sinon la partie 6 cassera la connexion à la base).

**Questions.**

1. La sortie de `kv get` mentionne `secret/data/wordpress/db`. D'où vient ce `data/` ?
2. Quelle différence entre `kv delete` et `kv destroy` ?
3. En quoi l'historique des versions vous aide-t-il le jour où quelqu'un écrase un mot de passe par erreur ?

<details>
<summary>Solution</summary>

```bash
bao kv put secret/wordpress/db user=wordpress password=change-me root_password=change-me-root
bao kv get secret/wordpress/db
bao kv get -field=password secret/wordpress/db

bao kv put secret/wordpress/db user=wordpress password=autre root_password=change-me-root
bao kv get -version=1 -field=password secret/wordpress/db

bao kv metadata get secret/wordpress/db

bao kv delete secret/wordpress/db
bao kv get secret/wordpress/db          # renvoie les métadonnées, deletion_time renseigné
bao kv undelete -versions=2 secret/wordpress/db

bao kv put secret/wordpress/db user=wordpress password=change-me root_password=change-me-root
```

1. KV v2 sépare les données (`secret/data/...`) des métadonnées (`secret/metadata/...`). La commande `bao kv` ajoute le préfixe pour vous. Les policies, elles, doivent viser le vrai chemin d'API : vous allez écrire `secret/data/...` à la partie 4.
2. `delete` est une suppression logique, réversible avec `undelete`. `destroy` efface les données d'une version de façon définitive.
3. On relit la version précédente avec `-version=N` et on la réécrit, ou on utilise `bao kv rollback -version=N`.

</details>

---

## Partie 3 : l'interface web

Ouvrez <http://127.0.0.1:8200/ui> et connectez-vous avec la méthode *Token* et la valeur `root`.

**Consignes.**

1. Retrouvez `secret/wordpress/db` et son historique de versions.
2. Créez un secret `secret/wordpress/smtp` depuis l'interface, puis lisez-le en CLI.
3. Ouvrez la console intégrée (icône terminal en haut) et lancez `read sys/health`.

L'interface et la CLI appellent la même API HTTP. Pour le vérifier depuis votre machine :

```bash
curl -s -H "X-Vault-Token: root" http://127.0.0.1:8200/v1/secret/data/wordpress/db
```

L'en-tête s'appelle encore `X-Vault-Token` : OpenBao l'a gardé pour rester compatible avec les clients Vault existants.

---

## Partie 4 : policies et tokens

Le token root peut tout faire. Le conteneur qui récupère les mots de passe de WordPress n'a besoin que de lire `secret/wordpress/*`.

**Consignes.**

1. Écrivez une policy `wordpress-read` qui donne le droit `read` sur les secrets de `secret/wordpress/`, et rien d'autre.
2. Chargez-la dans OpenBao.
3. Créez un token qui porte cette policy, avec une durée de vie d'une heure.
4. Avec ce token : lisez `secret/wordpress/db`, puis tentez d'y écrire, puis tentez de lister `sys/policies/acl`.

Astuce : `bao policy write <nom> -` lit la policy sur l'entrée standard. Avec `docker compose exec`, ajoutez `-T` pour que le pipe fonctionne.

**Questions.**

1. Votre policy vise-t-elle `secret/wordpress/*` ou `secret/data/wordpress/*` ? Testez les deux.
2. Que renvoie `bao token lookup` pour votre nouveau token ? Repérez `ttl` et `policies`.
3. Que devient le token au bout d'une heure ?

<details>
<summary>Solution</summary>

```bash
cat > wordpress-read.hcl <<'EOF'
path "secret/data/wordpress/*" {
  capabilities = ["read"]
}
EOF

docker compose exec -T -e BAO_TOKEN=root openbao bao policy write wordpress-read - < wordpress-read.hcl

TOKEN=$(docker compose exec -T -e BAO_TOKEN=root openbao \
  bao token create -policy=wordpress-read -ttl=1h -field=token)

docker compose exec -e BAO_TOKEN="$TOKEN" openbao bao kv get secret/wordpress/db      # OK
docker compose exec -e BAO_TOKEN="$TOKEN" openbao bao kv put secret/wordpress/db x=y  # permission denied
docker compose exec -e BAO_TOKEN="$TOKEN" openbao bao token lookup
```

1. `secret/data/wordpress/*`. Avec `secret/wordpress/*`, la lecture échoue : la policy porte sur le chemin d'API réel, et la CLI ajoute `data/` sans le montrer.
2. `policies` contient `default` et `wordpress-read`. `ttl` décompte depuis 1h.
3. Il expire et OpenBao le révoque. Toute requête faite avec renvoie `permission denied`.

</details>

---

## Partie 5 : une identité machine avec AppRole

Créer un token à la main et le coller dans `.env` ne tient pas : il expire, et il faut quelqu'un pour le renouveler. AppRole donne à une application deux identifiants, un `role_id` (l'équivalent d'un nom d'utilisateur, peu sensible) et un `secret_id` (l'équivalent d'un mot de passe). Avec les deux, l'application obtient elle-même un token de courte durée.

**Consignes.**

1. Activez la méthode d'authentification `approle`.
2. Créez un rôle `wordpress` qui délivre des tokens avec la policy `wordpress-read` et un TTL de 10 minutes.
3. Récupérez son `role_id` et générez un `secret_id`.
4. Connectez-vous avec ces deux valeurs (`bao write auth/approle/login ...`) et lisez `secret/wordpress/db` avec le token obtenu.
5. Ajoutez au `.env` :

   ```dotenv
   BAO_ROLE_ID=<votre role_id>
   BAO_SECRET_ID=<votre secret_id>
   ```

**Questions.**

1. Le `.env` contient encore un identifiant. Qu'est-ce qui a changé par rapport au mot de passe MariaDB en clair ?
2. Comment limiter la durée de vie ou le nombre d'utilisations d'un `secret_id` ? Cherchez dans la documentation AppRole.

<details>
<summary>Solution</summary>

```bash
bao auth enable approle
bao write auth/approle/role/wordpress token_policies=wordpress-read token_ttl=10m secret_id_ttl=24h
bao read -field=role_id auth/approle/role/wordpress/role-id
bao write -f -field=secret_id auth/approle/role/wordpress/secret-id

docker compose exec openbao bao write auth/approle/login role_id=<role_id> secret_id=<secret_id>
```

1. Le `secret_id` donne seulement le droit de lire `secret/wordpress/*`, pendant 10 minutes par connexion. Il se révoque et se régénère sans toucher à la base de données. Une fuite du mot de passe MariaDB, elle, oblige à le changer partout.
2. Paramètres `secret_id_ttl` et `secret_id_num_uses` du rôle. On peut aussi restreindre les adresses IP autorisées avec `secret_id_bound_cidrs`.

</details>

---

## Partie 6 : brancher la stack sur OpenBao

Les images officielles `mariadb` et `wordpress` acceptent une variante `_FILE` de leurs variables : `MARIADB_PASSWORD_FILE`, `MARIADB_ROOT_PASSWORD_FILE`, `WORDPRESS_DB_PASSWORD_FILE`. Le conteneur lit alors le mot de passe dans le fichier indiqué.

Le plan :

```text
openbao ──(AppRole)──> secrets-init ──écrit──> volume bao_secrets
                                                  │
                                ┌─────────────────┴─────────────┐
                              mariadb (lecture seule)   wordpress (lecture seule)
```

**Consignes.** Dans `openbao.compose.yml` :

1. Ajoutez un service `secrets-init` basé sur l'image OpenBao, qui :
   - attend qu'`openbao` soit `healthy` ;
   - se connecte en AppRole avec `BAO_ROLE_ID` et `BAO_SECRET_ID` ;
   - écrit `password` et `root_password` dans `/run/bao/db_password` et `/run/bao/db_root_password`, sur un volume nommé `bao_secrets` ;
   - se termine.
2. Surchargez `mariadb` et `wordpress` pour :
   - monter `bao_secrets` en lecture seule ;
   - supprimer les variables `*_PASSWORD` héritées de `docker-compose.yml` (balise `!reset`) ;
   - définir les variables `*_PASSWORD_FILE` ;
   - faire démarrer `mariadb` seulement après la fin réussie de `secrets-init`.
3. Retirez `DB_PASSWORD` et `DB_ROOT_PASSWORD` du `.env`.

Pièges que vous allez probablement rencontrer :

- L'image OpenBao s'exécute avec l'utilisateur `openbao`, qui n'a pas le droit d'écrire sur un volume neuf.
- MariaDB lit le fichier en tant qu'utilisateur `mysql`, WordPress en tant que `www-data`. Vérifiez les permissions.
- Dans un `command` Compose, `$` est interprété par Compose. Pour qu'il arrive jusqu'au shell, écrivez `$$`.
- Le volume `db_data` a été initialisé avec l'ancien mot de passe. MariaDB ne lit `MARIADB_PASSWORD` qu'au premier démarrage. Si le mot de passe dans OpenBao diffère, repartez de zéro avec `docker compose down -v` (vous perdez la base WordPress).

Validation :

```bash
docker compose up -d
docker compose ps -a                         # secrets-init : Exited (0)
docker compose logs secrets-init
docker compose exec wordpress env | grep PASSWORD   # seulement WORDPRESS_DB_PASSWORD_FILE
curl -I http://localhost:${WORDPRESS_PORT}
```

**Questions.**

1. Pourquoi `service_completed_successfully` plutôt que `service_started` ?
2. Le mot de passe est-il encore visible dans `docker inspect` ? Où peut-on encore le lire ?
3. Quelle différence avec les [secrets Compose](https://docs.docker.com/compose/how-tos/use-secrets/) natifs (`secrets:` au niveau racine) ?

<details>
<summary>Solution</summary>

```yaml
services:
  openbao:
    # ... inchangé depuis la partie 1

  secrets-init:
    image: openbao/openbao:${OPENBAO_VERSION}
    entrypoint: ["/bin/sh", "-c"]
    user: root
    command:
      - |
        set -e
        export BAO_TOKEN=$$(bao write -field=token auth/approle/login role_id="$$ROLE_ID" secret_id="$$SECRET_ID")
        bao kv get -field=password secret/wordpress/db > /run/bao/db_password
        bao kv get -field=root_password secret/wordpress/db > /run/bao/db_root_password
        chmod 0444 /run/bao/*
        echo "secrets écrits"
    environment:
      BAO_ADDR: http://openbao:8200
      ROLE_ID: ${BAO_ROLE_ID}
      SECRET_ID: ${BAO_SECRET_ID}
    volumes:
      - bao_secrets:/run/bao
    networks:
      - secrets
    depends_on:
      openbao:
        condition: service_healthy

  mariadb:
    environment:
      MARIADB_PASSWORD: !reset null
      MARIADB_ROOT_PASSWORD: !reset null
      MARIADB_PASSWORD_FILE: /run/bao/db_password
      MARIADB_ROOT_PASSWORD_FILE: /run/bao/db_root_password
    volumes:
      - bao_secrets:/run/bao:ro
    depends_on:
      secrets-init:
        condition: service_completed_successfully

  wordpress:
    environment:
      WORDPRESS_DB_PASSWORD: !reset null
      WORDPRESS_DB_PASSWORD_FILE: /run/bao/db_password
    volumes:
      - bao_secrets:/run/bao:ro

networks:
  secrets:

volumes:
  bao_secrets:
```

Compose fusionne les listes `volumes` de `mariadb` : `db_data` reste monté.

1. `service_started` laisserait MariaDB démarrer pendant que `secrets-init` tourne encore, avant que les fichiers existent. Et si `secrets-init` échoue (OpenBao injoignable, `secret_id` expiré), la stack s'arrête au lieu de démarrer avec un fichier vide.
2. Non, `docker inspect` ne montre plus que le chemin du fichier. Le mot de passe reste lisible dans le volume `bao_secrets` (pour qui a accès à Docker ou à `/var/lib/docker/volumes`) et dans `wp-config.php` à l'intérieur du conteneur WordPress.
3. Les secrets Compose montent un fichier local (ou une variable d'environnement de l'hôte) dans `/run/secrets/`. Le secret existe toujours en clair sur l'hôte. Ici, la source est OpenBao, avec contrôle d'accès, historique et révocation.

</details>

---

## Partie 7 : quitter le mode dev

Faites l'expérience :

```bash
docker compose restart openbao
docker compose up -d --force-recreate secrets-init
docker compose logs secrets-init
```

`secrets-init` échoue : OpenBao a redémarré à vide. Le mode dev ne garde rien d'un démarrage à l'autre.

**Consignes.**

1. Créez `openbao/config.hcl` avec :
   - un stockage `raft` dans `/openbao/file` ;
   - un listener TCP sur `0.0.0.0:8200`, sans TLS (on reste en local, voir question 3) ;
   - l'interface web activée ;
   - `api_addr` et `cluster_addr`.
2. Modifiez le service `openbao` : commande `server` (sans `-dev`), montage de la config dans `/openbao/config/`, volume nommé `bao_data` sur `/openbao/file`. Retirez `BAO_DEV_ROOT_TOKEN_ID`.
3. Démarrez, puis lancez `bao status`. Le healthcheck reste `unhealthy` : c'est normal.
4. Initialisez OpenBao avec 3 parts de clé et un seuil de 2 : `bao operator init -key-shares=3 -key-threshold=2`. **Notez les clés et le token root initial**, ils ne seront plus jamais affichés.
5. Descellez avec deux des trois clés (`bao operator unseal`, deux fois).
6. Avec le token root initial, réactivez KV v2 sur `secret/`, rejouez les parties 2, 4 et 5, mettez à jour `BAO_ROLE_ID` et `BAO_SECRET_ID` dans `.env`.
7. Redémarrez `openbao` : les données sont toujours là, mais il faut desceller à nouveau.

**Questions.**

1. À quoi sert le partage de la clé en plusieurs parts (Shamir) ?
2. Desceller à la main à chaque redémarrage ne convient pas en production. Quelles solutions propose OpenBao ? Regardez la page *Seal/Unseal* et la configuration `seal`.
3. Qu'est-ce qui manque encore à cette configuration pour une vraie production ?

<details>
<summary>Solution</summary>

`openbao/config.hcl` :

```hcl
ui            = true
disable_mlock = true
api_addr      = "http://openbao:8200"
cluster_addr  = "http://openbao:8201"

storage "raft" {
  path    = "/openbao/file"
  node_id = "bao-1"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = true
}
```

Service modifié :

```yaml
  openbao:
    image: openbao/openbao:${OPENBAO_VERSION}
    command: server
    environment:
      BAO_ADDR: http://127.0.0.1:8200
    volumes:
      - ./openbao/config.hcl:/openbao/config/config.hcl:ro
      - bao_data:/openbao/file
    ports:
      - "127.0.0.1:${BAO_PORT}:8200"
    networks:
      - secrets
    healthcheck:
      test: ["CMD", "bao", "status"]
      interval: 5s
      timeout: 3s
      retries: 10
```

Sans oublier `bao_data:` dans la section `volumes` racine.

```bash
docker compose up -d openbao
docker compose exec openbao bao operator init -key-shares=3 -key-threshold=2
docker compose exec openbao bao operator unseal   # clé 1
docker compose exec openbao bao operator unseal   # clé 2
export ROOT=<token root initial>
docker compose exec -e BAO_TOKEN=$ROOT openbao bao secrets enable -path=secret -version=2 kv
```

La sortie de `operator init` parle encore de « Vault » : héritage du fork.

1. Aucune personne seule ne peut desceller. Il faut réunir un nombre minimal de détenteurs de clés (le seuil). Perdre une part ne bloque pas tout tant qu'il en reste assez.
2. L'auto-unseal : la clé racine est chiffrée par un service externe (KMS d'un cloud, HSM via PKCS#11, ou le moteur transit d'un autre OpenBao). Au démarrage, OpenBao demande à ce service de la déchiffrer, sans intervention humaine.
3. TLS sur le listener, plusieurs nœuds Raft pour la haute disponibilité, des sauvegardes (`bao operator raft snapshot save`), un device d'audit, et révoquer le token root initial une fois l'administration configurée.

</details>

---

## Pour aller plus loin

- Activez un device d'audit (`bao audit enable file file_path=/openbao/logs/audit.log`) et lisez ce qu'OpenBao journalise quand `secrets-init` tourne. Les valeurs sensibles y sont hachées.
- Faites générer à `secrets-init` un `secret_id` à usage unique (`secret_id_num_uses=1`) et observez ce qui se passe au deuxième `docker compose up`.
- Ajoutez une policy `wordpress-admin` qui peut écrire `secret/data/wordpress/*` sans pouvoir lire les autres chemins, et testez-la.
- Le TP 2 ([Vault](./tp-vault.md)) remplace les mots de passe statiques par des identifiants MariaDB générés à la demande. Tout ce qu'il fait marche aussi avec OpenBao.

Les liens de documentation sont regroupés dans [references.md](./references.md).
