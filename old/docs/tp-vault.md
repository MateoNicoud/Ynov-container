# TP 2 : identifiants MariaDB dynamiques avec HashiCorp Vault

## Point de départ

Même stack que le TP 1 : `wordpress` et `mariadb` dans `docker-compose.yml`, configuration dans `.env`, MariaDB isolée sur le réseau `internal`.

Le TP 1 a déplacé un mot de passe statique dans OpenBao. Le mot de passe reste le même tant que personne ne le change, et un développeur qui doit consulter la base pour déboguer reçoit ce même mot de passe, avec tous ses droits.

Ce TP prend l'autre approche : Vault crée lui-même des comptes MariaDB à la demande, avec des droits limités et une date d'expiration. Quand le bail (*lease*) expire, Vault supprime le compte.

À la fin du TP, vous saurez :

- lancer Vault dans la stack et passer de `bao` à `vault` sans réapprendre ;
- configurer le moteur `database` sur MariaDB ;
- manipuler les leases : consulter, renouveler, révoquer ;
- activer un journal d'audit ;
- chiffrer des données sans stocker la clé, avec le moteur `transit`.

### Vault et OpenBao

Vault est édité par HashiCorp (racheté par IBM). Depuis août 2023, les nouvelles versions sont publiées sous Business Source License 1.1 : le code est consultable, mais l'usage dans un produit concurrent de HashiCorp est interdit. OpenBao est le fork communautaire de la dernière version sous licence MPL 2.0.

Pour ce TP, les deux sont interchangeables : remplacez `vault` par `bao` et `VAULT_` par `BAO_`. Vault a ses propres fonctionnalités (certaines réservées à la version Enterprise), et OpenBao en a ajouté d'autres de son côté. Les projets s'éloignent un peu plus à chaque version.

### Prérequis

- La stack du dépôt démarre sans erreur.
- Testé avec `hashicorp/vault:2.1.2`. Les tags sont listés sur [Docker Hub](https://hub.docker.com/r/hashicorp/vault/tags). L'ancienne image `vault` (sans préfixe `hashicorp/`) n'est plus mise à jour.
- Si le TP 1 tourne encore, arrêtez-le ou changez le port publié : les deux utilisent 8200 par défaut.

Les solutions sont repliées sous chaque partie.

---

## Partie 1 : démarrer Vault

Ajoutez au `.env` :

```dotenv
COMPOSE_FILE=docker-compose.yml:vault.compose.yml

VAULT_VERSION=2.1.2
VAULT_PORT=8200
VAULT_ROOT_TOKEN=root
```

**Consignes.** Dans `vault.compose.yml`, déclarez un service `vault` :

1. image `hashicorp/vault:${VAULT_VERSION}`, commande `server -dev` ;
2. token root fixé par `VAULT_DEV_ROOT_TOKEN_ID`, `VAULT_ADDR=http://127.0.0.1:8200` ;
3. capacité `IPC_LOCK` ;
4. port publié sur `127.0.0.1` seulement ;
5. healthcheck `vault status` ;
6. un volume nommé `vault_logs` monté sur `/vault/logs` (pour la partie 5).

Réfléchissez aux réseaux avant d'écrire la section `networks` du service : cette fois, Vault va se connecter à MariaDB.

```bash
docker compose up -d
docker compose exec vault vault status
alias vault='docker compose exec -e VAULT_TOKEN=root vault vault'
vault secrets list
```

**Questions.**

1. Sur quels réseaux brancher `vault` ? Pourquoi `internal` est-il nécessaire cette fois ?
2. MariaDB est sur un réseau `internal: true`. Si Vault n'était branché que sur ce réseau, pourriez-vous accéder à l'interface web depuis votre navigateur ?
3. En cinq minutes, refaites la partie 2 du TP 1 (KV v2) avec `vault`. Qu'est-ce qui change ?

<details>
<summary>Solution</summary>

```yaml
services:
  vault:
    image: hashicorp/vault:${VAULT_VERSION}
    command: server -dev
    cap_add: [IPC_LOCK]
    environment:
      VAULT_DEV_ROOT_TOKEN_ID: ${VAULT_ROOT_TOKEN}
      VAULT_ADDR: http://127.0.0.1:8200
    ports:
      - "127.0.0.1:${VAULT_PORT}:8200"
    volumes:
      - vault_logs:/vault/logs
    networks:
      - secrets
      - internal
    healthcheck:
      test: ["CMD", "vault", "status"]
      interval: 5s
      timeout: 3s
      retries: 10

networks:
  secrets:

volumes:
  vault_logs:
```

1. `secrets` pour être joignable depuis l'hôte, `internal` pour atteindre `mariadb:3306`. Vault parle à la base pour créer et supprimer des comptes.
2. Non. Docker ne publie pas de port pour un conteneur branché uniquement sur un réseau `internal`. Il faut un second réseau non interne.
3. Le nom de la commande et le préfixe des variables. Les sous-commandes, les chemins (`secret/data/...`) et les sorties sont les mêmes.

</details>

---

## Partie 2 : brancher Vault sur MariaDB

Le moteur `database` a besoin d'un compte MariaDB capable de créer des utilisateurs. On utilise `root` pour ce TP.

**Consignes.**

1. Activez le moteur `database`.
2. Déclarez une connexion `wordpress-db` :
   - plugin `mysql-database-plugin` (il gère aussi MariaDB) ;
   - URL de connexion `{{username}}:{{password}}@tcp(mariadb:3306)/` ;
   - identifiants root tirés de votre `.env` ;
   - rôle autorisé : `wp-readonly`.
3. Créez le rôle `wp-readonly` :
   - il crée un utilisateur qui n'a que `SELECT` sur la base `wordpress` ;
   - durée de vie par défaut 5 minutes, maximum 1 heure.
4. Demandez des identifiants : `vault read database/creds/wp-readonly`.
5. Connectez-vous à MariaDB avec, lancez `SHOW TABLES;` puis tentez `CREATE TABLE pirate (i INT);`.
6. Listez les utilisateurs MariaDB qui commencent par `v-`.

Pour se connecter avec les identifiants générés (le mot de passe peut commencer par `-`, d'où la forme longue) :

```bash
docker compose exec mariadb mariadb --user='<username>' --password='<password>' wordpress
```

**Questions.**

1. Que contiennent `{{name}}` et `{{password}}` dans l'instruction de création ?
2. Lisez le nom d'utilisateur généré. Quelles informations contient-il ?
3. Pourquoi les accolades doubles `{{username}}` dans l'URL de connexion plutôt que le mot de passe directement ?

<details>
<summary>Solution</summary>

```bash
vault secrets enable database

vault write database/config/wordpress-db \
  plugin_name=mysql-database-plugin \
  connection_url='{{username}}:{{password}}@tcp(mariadb:3306)/' \
  allowed_roles=wp-readonly \
  username=root \
  password=change-me-root

vault write database/roles/wp-readonly \
  db_name=wordpress-db \
  creation_statements="CREATE USER '{{name}}'@'%' IDENTIFIED BY '{{password}}'; GRANT SELECT ON wordpress.* TO '{{name}}'@'%';" \
  default_ttl=5m \
  max_ttl=1h

vault read database/creds/wp-readonly
```

Le `CREATE TABLE` échoue :

```text
ERROR 1142 (42000) at line 1: CREATE command denied to user 'v-token-wp-readonl-...'@'localhost' for table `wordpress`.`pirate`
```

```bash
docker compose exec mariadb mariadb -uroot -p'change-me-root' \
  -e "SELECT user, host FROM mysql.user WHERE user LIKE 'v-%';"
```

1. Vault les remplace au moment de la création : `{{name}}` par un nom unique, `{{password}}` par un mot de passe aléatoire.
2. Le préfixe `v-`, la méthode d'authentification du demandeur (`token`), le rôle tronqué (`wp-readonl`) et une partie aléatoire. Dans un journal MariaDB, on sait d'où vient le compte.
3. Vault garde le mot de passe à part et l'injecte au moment de la connexion. Il ne réapparaît pas en lisant la configuration (`vault read database/config/wordpress-db`), et il peut être changé par Vault (partie 4).

</details>

---

## Partie 3 : vivre avec les leases

Chaque identifiant dynamique est lié à un lease. La sortie de `vault read database/creds/wp-readonly` affiche `lease_id`, `lease_duration` et `lease_renewable`.

**Consignes.**

1. Générez des identifiants, notez le `lease_id`.
2. Consultez le lease : `vault lease lookup <lease_id>`.
3. Renouvelez-le : `vault lease renew <lease_id>`. Réessayez avec `-increment=2h`. Que se passe-t-il ?
4. Révoquez-le. Tentez de vous reconnecter à MariaDB avec ces identifiants.
5. Générez trois paires d'identifiants, puis révoquez-les toutes d'une commande.
6. Générez une paire, n'y touchez plus, et vérifiez au bout de 5 minutes que l'utilisateur a disparu de `mysql.user`.

**Questions.**

1. Pourquoi `-increment=2h` ne donne-t-il pas 2 heures ?
2. Un développeur demande un accès à la base de production pour déboguer. Comparez : lui donner le mot de passe de `wordpress`, ou lui donner une policy qui l'autorise à lire `database/creds/wp-readonly`.
3. Que se passe-t-il pour les leases si Vault redémarre en mode dev ? Et pour les comptes MariaDB ?

<details>
<summary>Solution</summary>

```bash
vault read database/creds/wp-readonly
vault lease lookup database/creds/wp-readonly/<id>
vault lease renew database/creds/wp-readonly/<id>
vault lease renew -increment=2h database/creds/wp-readonly/<id>
vault lease revoke database/creds/wp-readonly/<id>

vault list sys/leases/lookup/database/creds/wp-readonly
vault lease revoke -prefix database/creds/wp-readonly
```

Après révocation :

```text
ERROR 1045 (28000): Access denied for user 'v-token-wp-readonl-...'@'localhost' (using password: YES)
```

1. Le renouvellement est plafonné par `max_ttl` (1 heure), compté depuis la création des identifiants. Vault affiche un avertissement et accorde ce qui reste.
2. Avec le mot de passe partagé : droits complets, pas de date de fin, impossible de savoir qui a fait quoi, et il faut changer le mot de passe de WordPress quand le développeur quitte l'équipe. Avec le rôle : accès en lecture seule, limité dans le temps, tracé dans l'audit, révocable sans toucher à WordPress.
3. En mode dev, Vault perd ses leases au redémarrage. Il ne pourra donc plus supprimer les comptes déjà créés : ils restent dans MariaDB jusqu'à ce que quelqu'un les efface à la main. Encore une raison de ne pas utiliser le mode dev au-delà de l'apprentissage.

</details>

---

## Partie 4 (bonus) : Vault prend le mot de passe root

Le mot de passe root de MariaDB est encore dans `.env`. Vault peut le changer lui-même, et alors personne ne le connaît plus.

```bash
vault write -force database/rotate-root/wordpress-db
```

**Attention :** en mode dev, Vault oublie le nouveau mot de passe au prochain redémarrage. Pour revenir en arrière : `docker compose down -v` (la base WordPress est perdue).

Testez ensuite les deux connexions suivantes avec l'ancien mot de passe :

```bash
# depuis le conteneur MariaDB lui-même
docker compose exec mariadb mariadb -uroot -p'change-me-root' \
  -e "SELECT user, host FROM mysql.user WHERE user = 'root';"

# depuis un autre conteneur du réseau internal
docker run --rm --network ynov-container_internal mariadb:${MARIADB_VERSION} \
  mariadb -h mariadb -uroot -p'change-me-root' -e 'SELECT CURRENT_USER();'
```

Compose préfixe les réseaux par le nom du projet, c'est-à-dire le nom du dossier en minuscules : vérifiez le vôtre avec `docker network ls`. Remplacez `${MARIADB_VERSION}` par la valeur de votre `.env` si la variable n'est pas exportée dans votre shell.

**Questions.**

1. Vérifiez que `vault read database/creds/wp-readonly` fonctionne toujours. Pourquoi ?
2. La première connexion réussit, la seconde échoue. Expliquez avec la sortie de la requête sur `mysql.user`.
3. Quel compte devriez-vous créer à la place de root pour que Vault n'ait que les droits nécessaires ?

<details>
<summary>Solution</summary>

1. Vault a mis à jour sa propre configuration avec le nouveau mot de passe.
2. MariaDB a deux comptes root distincts : `root@localhost` (connexions locales, par socket) et `root@'%'` (connexions réseau). Vault se connecte par le réseau, il a donc changé le mot de passe de `root@'%'` seulement. `root@localhost` garde l'ancien mot de passe, qui est toujours dans `.env`. La rotation est incomplète : il faudrait supprimer `root@'%'` et donner à Vault son propre compte (question 3).
3. Un compte dédié, par exemple `vault@'%'`, avec le privilège global `CREATE USER` et `SELECT ON wordpress.* WITH GRANT OPTION` (on ne peut transmettre que des droits qu'on possède). Une fuite de la configuration Vault ne donne alors pas accès à toutes les bases du serveur.

</details>

---

## Partie 5 : journal d'audit

**Consignes.**

1. Activez un device d'audit de type `file` qui écrit dans `/vault/logs/audit.log`.
2. Générez des identifiants, lisez un secret KV.
3. Lisez la dernière ligne du journal : `docker compose exec vault tail -n1 /vault/logs/audit.log`.
4. Cherchez le mot de passe généré dans le journal.

**Questions.**

1. Le mot de passe apparaît-il en clair ? Qu'est-ce qui apparaît à la place ?
2. Comment savoir si une valeur précise est passée dans le journal, sans qu'elle y soit stockée en clair ?
3. Que fait Vault si le seul device d'audit activé ne peut plus écrire (disque plein, par exemple) ?

<details>
<summary>Solution</summary>

```bash
vault audit enable file file_path=/vault/logs/audit.log
vault read database/creds/wp-readonly
docker compose exec vault tail -n1 /vault/logs/audit.log
```

1. Non. Les tokens, mots de passe et autres valeurs sensibles sont remplacés par `hmac-sha256:...`.
2. `vault write sys/audit-hash/file input=<valeur>` renvoie le HMAC de la valeur, à comparer avec le journal.
3. Il refuse toutes les requêtes, par conception : chaque requête servie doit laisser une trace dans l'audit. En production, on active au moins deux devices d'audit.

</details>

---

## Partie 6 : chiffrer sans garder la clé (transit)

Le moteur `transit` chiffre et déchiffre des données à la demande. L'application envoie un texte, reçoit un texte chiffré, et le stocke où elle veut (dans MariaDB par exemple). La clé ne quitte jamais Vault.

**Consignes.**

1. Activez `transit` et créez une clé `wordpress`.
2. Chiffrez la chaîne `bonjour` (le texte doit être encodé en base64).
3. Déchiffrez le résultat.
4. Faites tourner la clé (`rotate`), chiffrez de nouveau `bonjour` et comparez le préfixe des deux textes chiffrés.
5. Déchiffrez l'ancien texte chiffré. Puis faites-le « rechiffrer » avec la nouvelle version de la clé (`rewrap`).

**Questions.**

1. Que signifie le préfixe `vault:v1:` ?
2. Après une rotation, pourquoi l'ancien texte chiffré se déchiffre-t-il encore ? Comment l'interdire ?
3. Quel avantage par rapport à un chiffrement fait par l'application avec une clé dans sa configuration ?

<details>
<summary>Solution</summary>

```bash
vault secrets enable transit
vault write -f transit/keys/wordpress

vault write -field=ciphertext transit/encrypt/wordpress plaintext=$(printf 'bonjour' | base64)
# vault:v1:kx8vToZu...

vault write -field=plaintext transit/decrypt/wordpress ciphertext='vault:v1:...' | base64 -d

vault write -f transit/keys/wordpress/rotate
vault write -field=ciphertext transit/encrypt/wordpress plaintext=$(printf 'bonjour' | base64)
# vault:v2:...

vault write -field=ciphertext transit/rewrap/wordpress ciphertext='vault:v1:...'
```

1. Le texte a été chiffré par Vault avec la version 1 de la clé.
2. Vault garde les anciennes versions pour le déchiffrement. On fixe `min_decryption_version` dans la configuration de la clé (`vault write transit/keys/wordpress/config min_decryption_version=2`) après avoir rechiffré les données existantes avec `rewrap`.
3. Une fuite de la base ou du code ne donne pas la clé. La rotation se fait sans redéployer l'application, et chaque déchiffrement passe par Vault, donc par son audit.

</details>

---

## Bilan

Répondez en quelques lignes, à partir de ce que vous avez fait dans les deux TP :

1. Pour la stack WordPress, quels secrets restent statiques et lesquels pourraient devenir dynamiques ?
2. WordPress lit son mot de passe une seule fois, dans `wp-config.php`. Qu'est-ce qui empêche de lui donner des identifiants dynamiques de 5 minutes ? Regardez ce que fait Vault Agent (ou OpenBao Agent) avec ses templates.
3. Pour un projet que vous démarrez demain, choisiriez-vous Vault ou OpenBao ? Appuyez-vous sur la licence, les fonctionnalités dont vous avez besoin et l'endroit où le projet sera hébergé.

Les liens de documentation sont regroupés dans [references.md](./references.md).
