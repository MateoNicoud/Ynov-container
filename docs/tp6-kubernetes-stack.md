# TP 6 : la stack WordPress dans Kubernetes

## Point de départ

Le TP 5 a montré les objets de base avec une application sans état. Une vraie stack a de la configuration, des mots de passe, une base de données qui doit garder ses données, et un point d'entrée HTTP. Ce TP porte la stack Compose (MariaDB, WordPress, API) dans minikube.

À la fin du TP :

- tout est décrit dans `k8s/`, appliqué avec une seule commande (`kubectl apply -k`) ;
- les mots de passe sont dans un Secret qui n'est pas dans Git ;
- MariaDB garde ses données quand son Pod est supprimé, et vous savez la sauvegarder et la restaurer ;
- WordPress et l'API sont joignables par un Ingress, puis par une Gateway en HTTPS.

### Prérequis

- Le cluster minikube du TP 5 (`--cni=calico`).
- L'image `tp-api` chargée dans minikube : `minikube image load tp-api:dev`.
- Testé avec minikube 1.39.0 (Kubernetes 1.37.0), Envoy Gateway 1.9.2, cert-manager 1.21.2, Helm 4.3.0.

Les fichiers du dépôt (`k8s/base/`) contiennent déjà les ajouts du TP 7 : sondes, `resources`, `securityContext`, NetworkPolicy, RBAC, HPA. Pour ce TP, ignorez ces blocs ; les solutions ci-dessous montrent la version de départ.

---

## Cours

### De Compose à Kubernetes

| Compose | Kubernetes | Remarque |
|---|---|---|
| un `service` | un **Deployment** (sans état) ou un **StatefulSet** (avec état) + un **Service** | le Service est l'adresse, le Deployment fait tourner les Pods |
| `image`, `command`, `environment` | les mêmes champs dans le template de Pod (`image`, `args`, `env`) | |
| `environment` non secret | **ConfigMap** | |
| mots de passe, `secrets:` | **Secret** | voir plus bas : ce n'est pas un coffre-fort |
| `volumes` nommés | **PersistentVolumeClaim** | le stockage vient d'une StorageClass |
| `ports` | **Service** (`NodePort`, `LoadBalancer`) ou **Ingress / Gateway** | on n'expose presque jamais un Pod directement |
| réseau, nom de service | Service + DNS `<svc>.<ns>.svc.cluster.local` | tous les Pods se voient par défaut |
| `networks: internal: true` | **NetworkPolicy** | TP 7 |
| `depends_on` + `healthcheck` | rien d'équivalent : **readiness probes** + applications qui réessaient | TP 7 |
| `deploy.resources.limits` | `resources.requests` et `resources.limits` | TP 7 |
| `restart: unless-stopped` | inutile : les contrôleurs relancent toujours | |
| `.env` | **Kustomize** (overlays par environnement) ou Helm | |

Le point qui surprend le plus : **il n'y a pas d'ordre de démarrage**. Kubernetes démarre tout en même temps. Si WordPress démarre avant MariaDB, il échoue, redémarre, et finit par réussir quand la base est prête. Les applications doivent donc savoir réessayer. Le cours le dit déjà pour Docker (« L'application doit gérer les tentatives de reconnexion ») ; ici, ce n'est plus optionnel.

### ConfigMap et Secret

Une **ConfigMap** stocke des paires clé-valeur non sensibles. Un **Secret** a la même forme, avec trois différences : les valeurs sont encodées en base64, l'accès peut être restreint séparément par RBAC, et le kubelet les monte en mémoire (`tmpfs`) plutôt que sur disque.

Le base64 n'est **pas** un chiffrement : `echo aHllbDNM | base64 -d` le décode. Par défaut, etcd stocke les Secrets en clair ; le chiffrement au repos (`EncryptionConfiguration`) doit être activé par l'administrateur du cluster, et les clouds le font pour vous (AKS chiffre etcd ; on peut ajouter une clé Key Vault). Quiconque peut lire les Secrets d'un namespace (`get secrets`), ou créer un Pod qui les monte, a tous les mots de passe du namespace.

Deux façons de les passer à un conteneur :

- en **variable d'environnement** (`env.valueFrom.secretKeyRef`) : simple, mais visible par tous les processus du conteneur, parfois écrit dans les logs ou les rapports d'erreur, et figé jusqu'au redémarrage du Pod ;
- en **fichier** (volume `secret`) : invisible dans `kubectl describe pod`, et mis à jour automatiquement par le kubelet quand le Secret change (avec un délai d'environ une minute).

Le dépôt utilise les fichiers chaque fois que l'application le permet (`WORDPRESS_DB_PASSWORD_FILE`, `DB_PASSWORD_FILE`). MariaDB lit `MARIADB_ROOT_PASSWORD` une seule fois, à l'initialisation : la variable d'environnement suffit.

Et surtout : un Secret dans un fichier YAML commité n'est plus un secret. On le crée avec `kubectl create secret`, ou on utilise un outil qui garde la source chiffrée ou ailleurs : Sealed Secrets, External Secrets Operator, le driver CSI de Key Vault (TP 9), ou OpenBao (TP 8).

### Le stockage

Trois objets, trois rôles :

- **PersistentVolume (PV)** : un morceau de stockage réel (un disque Azure, un dossier sur le nœud minikube, un partage NFS) ;
- **PersistentVolumeClaim (PVC)** : une demande de stockage faite par une application (« 1 Gi, en lecture-écriture par un seul nœud ») ;
- **StorageClass** : la recette pour créer des PV à la demande. Quand un PVC apparaît, le *provisioner* de la classe crée le PV correspondant (*provisionnement dynamique*).

Sur minikube, la classe `standard` crée un dossier sur le nœud (`k8s.io/minikube-hostpath`). Sur AKS, `managed-csi` crée un disque managé Azure. Les manifests ne changent pas : seul le nom de la classe par défaut diffère.

Les **modes d'accès** décrivent qui peut monter le volume :

| Mode | Signification | Exemple |
|---|---|---|
| `ReadWriteOnce` (RWO) | lecture-écriture, un seul nœud à la fois | disque Azure, disque local |
| `ReadOnlyMany` (ROX) | lecture seule, plusieurs nœuds | |
| `ReadWriteMany` (RWX) | lecture-écriture, plusieurs nœuds | Azure Files, NFS |
| `ReadWriteOncePod` (RWOP) | un seul Pod | |

La **politique de récupération** (`reclaimPolicy`) dit ce que devient le PV quand on supprime le PVC : `Delete` (le disque est détruit, défaut des classes dynamiques) ou `Retain` (il reste, à nettoyer à la main). Supprimer un PVC par erreur avec `Delete`, c'est perdre la base.

### StatefulSet

Un Deployment considère ses Pods comme interchangeables : noms aléatoires, ordre quelconque, un même volume partagé par tous. Une base de données a besoin du contraire :

- un **nom stable** : `mariadb-0` reste `mariadb-0` après recréation ;
- un **volume par réplica**, attaché à ce nom : `volumeClaimTemplates` crée `data-mariadb-0`, et le Pod recréé retrouve le même PVC ;
- un **ordre** de création et de suppression (0, puis 1, puis 2) ;
- un **Service headless** (`clusterIP: None`) qui donne à chaque Pod un nom DNS : `mariadb-0.mariadb.wordpress.svc.cluster.local`.

Supprimer un StatefulSet ne supprime pas ses PVC : c'est voulu. Un StatefulSet ne transforme pas MariaDB en cluster répliqué ; pour ça, il faut configurer la réplication MariaDB elle-même (ou utiliser un opérateur). En production, on préfère souvent une base managée (TP 9).

### Exposer en HTTP : Ingress, puis Gateway API

Un Service `LoadBalancer` par application coûte une IP publique chacun et ne sait rien de HTTP. Un **Ingress** décrit des règles HTTP (nom d'hôte, chemin) vers des Services, derrière une seule adresse. L'objet Ingress n'est qu'une configuration : il faut un **contrôleur d'Ingress** (un reverse proxy qui lit ces objets) installé dans le cluster. Sans contrôleur, un Ingress ne fait rien.

L'objet Ingress a deux limites que la pratique a montrées : il ne décrit que l'essentiel (hôte, chemin, TLS), et tout le reste (redirections, réécritures, délais, poids) passe par des **annotations** propres à chaque contrôleur, non portables. Le contrôleur le plus répandu, **ingress-nginx**, n'est plus maintenu depuis mars 2026 ([annonce](https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/)).

Son successeur standard est **Gateway API**, qui sépare les rôles en trois objets :

| Objet | Qui le gère | Ce qu'il décrit |
|---|---|---|
| `GatewayClass` | l'équipe plateforme, une fois par cluster | quel logiciel implémente les Gateway (Envoy Gateway, Istio, Traefik, Azure...) |
| `Gateway` | l'équipe plateforme ou ops | une entrée : adresses, ports, protocoles, certificats TLS, quels namespaces peuvent s'y attacher |
| `HTTPRoute` | l'équipe applicative | les règles : hôte, chemin, en-têtes, redirections, répartition pondérée (canary) |

Les fonctions qui demandaient des annotations sont dans la spécification (redirection, réécriture, poids), donc portables d'une implémentation à l'autre. Ce TP utilise d'abord l'Ingress, parce que le cours le demande et que vous en croiserez partout, puis Gateway API avec **Envoy Gateway**. Les mêmes Gateway et HTTPRoute serviront en production sur Azure.

### TLS dans le cluster

**cert-manager** surveille les Gateway (ou Ingress) annotés, demande un certificat à un émetteur (`Issuer` ou `ClusterIssuer`), le range dans un Secret de type `kubernetes.io/tls`, et le renouvelle avant expiration. En lab, l'émetteur est auto-signé (le navigateur affiche un avertissement) ; en production, Let's Encrypt (TP 9).

---

## Partie 1 : namespace et Secret

**Consignes.**

1. Créez le namespace `wordpress`.
2. Créez un Secret `db` avec trois clés générées aléatoirement : `root-password`, `wordpress-password`, `api-password`. Il ne doit apparaître dans aucun fichier.
3. Relisez la valeur de `api-password` à partir du Secret. Combien de commandes faut-il ?
4. Ajoutez `*.secret.yaml` et `secret*.env` à `.gitignore`, au cas où.

**Questions.**

1. Qui peut lire ce Secret dans votre cluster minikube ?
2. Pourquoi ne pas simplement mettre le Secret dans un fichier `k8s/base/secret.yaml` ?

<details>
<summary>Solution</summary>

```bash
kubectl apply -f k8s/base/namespace.yaml

# Valeurs générées à la volée : elles n'existent que dans le cluster.
kubectl -n wordpress create secret generic db \
  --from-literal=root-password="$(openssl rand -base64 24)" \
  --from-literal=wordpress-password="$(openssl rand -base64 24)" \
  --from-literal=api-password="$(openssl rand -base64 24)"

# Une commande suffit : le base64 n'est pas un chiffrement.
kubectl -n wordpress get secret db -o jsonpath='{.data.api-password}' | base64 -d
```

1. Vous, avec le contexte `minikube` qui est administrateur du cluster. Et tout compte ou ServiceAccount qui a `get secrets` sur le namespace, ou qui peut créer des Pods dans ce namespace (un Pod peut monter n'importe quel Secret de son namespace). Le TP 7 crée un compte `dev` qui n'a pas ce droit.
2. Le dépôt est cloné sur des postes, dans la CI, parfois rendu public : le mot de passe le suivrait partout et resterait dans l'historique Git même après suppression du fichier. Le Secret créé à la main n'existe que dans etcd.

</details>

---

## Partie 2 : configuration commune

**Consignes.**

1. Créez une ConfigMap `wordpress-config` avec `DB_HOST`, `DB_NAME`, `DB_USER`.
2. Écrivez un Pod jetable qui affiche la ConfigMap en variables d'environnement (`envFrom`), et le Secret monté en fichiers sous `/run/secrets/db`.
3. Modifiez une valeur du Secret (`kubectl edit secret db` ou `kubectl create secret ... --dry-run=client -o yaml | kubectl apply -f -`). Le fichier monté change-t-il ? Et une variable d'environnement venue du même Secret ?

<details>
<summary>Solution</summary>

```yaml
# k8s/base/config.yaml : configuration non secrète, partagée par les trois applications
apiVersion: v1
kind: ConfigMap
metadata:
  name: wordpress-config
data:
  DB_HOST: mariadb:3306        # le Service "mariadb", dans le même namespace
  DB_NAME: wordpress
  DB_USER: wordpress
```

```yaml
# Pod de test : à appliquer dans le namespace wordpress, puis à supprimer
apiVersion: v1
kind: Pod
metadata:
  name: lecteur
spec:
  restartPolicy: Never
  containers:
    - name: lecteur
      image: busybox:1.37
      command: [sh, -c, 'env | grep DB_; ls -l /run/secrets/db; sleep 3600']
      envFrom:
        - configMapRef:
            name: wordpress-config     # chaque clé devient une variable
      volumeMounts:
        - name: db
          mountPath: /run/secrets/db
          readOnly: true
  volumes:
    - name: db
      secret:
        secretName: db                 # chaque clé devient un fichier
```

```bash
kubectl -n wordpress apply -f k8s/base/config.yaml -f lecteur.yaml
kubectl -n wordpress logs lecteur
kubectl -n wordpress exec lecteur -- mount | grep secrets   # tmpfs : en mémoire
```

Le fichier monté est mis à jour par le kubelet au bout d'une minute environ (les fichiers sont des liens symboliques vers un dossier remplacé d'un coup). Une variable d'environnement est fixée au démarrage du conteneur : il faut recréer le Pod. Exception : un Secret monté avec `subPath` n'est jamais mis à jour.

</details>

---

## Partie 3 : MariaDB avec un StatefulSet

**Consignes.**

1. Écrivez un Service headless `mariadb` et un StatefulSet `mariadb` d'un réplica (`mariadb:13.0`) :
   - variables `MARIADB_DATABASE` et `MARIADB_USER` depuis la ConfigMap, `MARIADB_ROOT_PASSWORD` et `MARIADB_PASSWORD` depuis le Secret ;
   - un `volumeClaimTemplates` de 1 Gi monté sur `/var/lib/mysql`.
2. L'API doit avoir un compte en lecture seule. Ajoutez une ConfigMap `mariadb-init` contenant un script `01-api-user.sh` monté dans `/docker-entrypoint-initdb.d`, qui crée l'utilisateur `api` avec `SELECT` sur la base, et le mot de passe de la clé `api-password` (passée en variable `API_PASSWORD`).
3. Appliquez, attendez `mariadb-0` prêt, et listez les PVC et PV.
4. Créez une table de test, supprimez le Pod `mariadb-0`, attendez son retour et vérifiez la table.
5. Supprimez le StatefulSet (pas le PVC), recréez-le : les données sont-elles là ?

**Questions.**

1. Le script d'init s'exécute-t-il à chaque redémarrage du Pod ?
2. Pourquoi un Service headless, et pas un ClusterIP normal ?
3. Que se passerait-il avec `replicas: 2` ?

<details>
<summary>Solution</summary>

Version de départ (le fichier complet `k8s/base/mariadb.yaml` ajoute sondes, ressources et durcissement au TP 7) :

```yaml
apiVersion: v1
kind: Service
metadata:
  name: mariadb
spec:
  clusterIP: None          # headless : le nom DNS renvoie directement l'IP du Pod
  selector:
    app: mariadb
  ports:
    - name: mysql
      port: 3306
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: mariadb-init
data:
  # Sourcé par l'entrypoint MariaDB au premier démarrage, avec les fonctions docker_process_sql
  # et docker_sql_escape_string_literal déjà définies.
  01-api-user.sh: |
    docker_process_sql --database=mysql <<-EOSQL
    	CREATE USER 'api'@'%' IDENTIFIED BY '$(docker_sql_escape_string_literal "${API_PASSWORD}")';
    	GRANT SELECT ON \`${MARIADB_DATABASE}\`.* TO 'api'@'%';
    EOSQL
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mariadb
spec:
  serviceName: mariadb               # le Service headless qui donne leur DNS aux Pods
  replicas: 1
  selector:
    matchLabels:
      app: mariadb
  template:
    metadata:
      labels:
        app: mariadb
    spec:
      containers:
        - name: mariadb
          image: mariadb:13.0
          ports:
            - name: mysql
              containerPort: 3306
          env:
            - name: MARIADB_DATABASE
              valueFrom:
                configMapKeyRef: { name: wordpress-config, key: DB_NAME }
            - name: MARIADB_USER
              valueFrom:
                configMapKeyRef: { name: wordpress-config, key: DB_USER }
            - name: MARIADB_ROOT_PASSWORD
              valueFrom:
                secretKeyRef: { name: db, key: root-password }
            - name: MARIADB_PASSWORD
              valueFrom:
                secretKeyRef: { name: db, key: wordpress-password }
            - name: API_PASSWORD
              valueFrom:
                secretKeyRef: { name: db, key: api-password }
          volumeMounts:
            - name: data
              mountPath: /var/lib/mysql
            - name: init
              mountPath: /docker-entrypoint-initdb.d
              readOnly: true
      volumes:
        - name: init
          configMap:
            name: mariadb-init
  volumeClaimTemplates:              # un PVC par réplica : data-mariadb-0, data-mariadb-1...
    - metadata:
        name: data
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 1Gi             # pas de storageClassName : la classe par défaut ("standard")
```

```bash
kubectl -n wordpress apply -f k8s/base/config.yaml -f k8s/base/mariadb.yaml
kubectl -n wordpress rollout status statefulset/mariadb
kubectl -n wordpress get pvc,pv          # data-mariadb-0 Bound, et le PV créé automatiquement

# Raccourci : client mariadb dans le Pod, mot de passe root lu dans son propre environnement
sql() { kubectl -n wordpress exec -i mariadb-0 -- sh -c 'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD"' ; }
echo "CREATE TABLE wordpress.preuve (i INT); INSERT INTO wordpress.preuve VALUES (1);" | sql

kubectl -n wordpress delete pod mariadb-0
kubectl -n wordpress wait --for=condition=Ready pod/mariadb-0 --timeout=3m
echo "SELECT * FROM wordpress.preuve;" | sql        # 1

kubectl -n wordpress delete statefulset mariadb     # le PVC reste
kubectl -n wordpress get pvc
kubectl -n wordpress apply -f k8s/base/mariadb.yaml
kubectl -n wordpress wait --for=condition=Ready pod/mariadb-0 --timeout=3m
echo "SELECT * FROM wordpress.preuve; DROP TABLE wordpress.preuve;" | sql
```

1. Non. L'entrypoint ne lance les scripts de `/docker-entrypoint-initdb.d` que si le dossier de données est vide. Changer le script plus tard n'a aucun effet sur une base existante : il faut appliquer le changement en SQL à la main (ou par une migration).
2. Un ClusterIP répartit les connexions entre tous les Pods prêts. Pour une base avec un primaire et des réplicas, il faut viser un Pod précis (`mariadb-0.mariadb` pour les écritures). Le Service headless donne ces noms. Avec un seul réplica, les deux marcheraient ; on prend l'habitude correcte.
3. Kubernetes créerait `mariadb-1` avec son propre PVC vide : une deuxième base indépendante, initialisée de zéro. WordPress serait réparti entre deux bases différentes. Sans réplication configurée dans MariaDB, `replicas: 2` casse la cohérence au lieu de la renforcer.

</details>

---

## Partie 4 : sauvegarder et restaurer

Le cours le rappelle : un volume persistant n'est pas une sauvegarde. Il protège contre la perte du Pod, pas contre un `DROP TABLE`, une corruption ou la suppression du PVC.

**Consignes.**

1. Exportez la base `wordpress` dans un fichier local avec `mariadb-dump`, exécuté dans le Pod.
2. Supprimez une table, restaurez le fichier, vérifiez.
3. Écrivez la procédure dans le README (commande exacte, où va le fichier, comment restaurer).

**Questions.**

1. Pourquoi `--single-transaction` ?
2. Ce fichier contient-il des données sensibles ? Où le ranger ?
3. Quelle autre méthode de sauvegarde le cours mentionne-t-il, et quelle est sa limite pour une base de données ?

<details>
<summary>Solution</summary>

```bash
# Dump logique : des instructions SQL, lisibles et portables d'une version à l'autre.
kubectl -n wordpress exec mariadb-0 -- sh -c \
  'mariadb-dump -uroot -p"$MARIADB_ROOT_PASSWORD" --single-transaction wordpress' \
  > wordpress-$(date +%F).sql

# Incident simulé
kubectl -n wordpress exec mariadb-0 -- sh -c \
  'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "DROP TABLE wordpress.wp_comments"'

# Restauration : le fichier est envoyé sur l'entrée standard du client (d'où -i)
kubectl -n wordpress exec -i mariadb-0 -- sh -c \
  'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" wordpress' < wordpress-$(date +%F).sql
```

1. Le dump se fait dans une seule transaction : il voit la base dans l'état d'un instant précis, même si WordPress écrit pendant ce temps, sans verrouiller les tables InnoDB.
2. Oui : comptes WordPress (hash des mots de passe, adresses e-mail), contenus non publiés. Il se range hors du dépôt, chiffré, et idéalement hors de la machine (un stockage objet avec durée de rétention). Une sauvegarde qui reste sur le même disque que la base disparaît avec lui.
3. L'instantané (*snapshot*) du volume. Pris pendant que MariaDB écrit, il peut contenir des fichiers dans un état incohérent (comme une coupure de courant) : il faut arrêter la base, la verrouiller, ou s'appuyer sur la récupération après crash d'InnoDB. Dans tous les cas, une sauvegarde n'existe vraiment que si on a déjà testé sa restauration.

</details>

---

## Partie 5 : WordPress et l'API

**Consignes.**

1. WordPress : un Service, un PVC `wp-content` (1 Gi, RWO), un Deployment d'un réplica qui monte ce PVC sur `/var/www/html/wp-content` et lit son mot de passe dans un fichier (`WORDPRESS_DB_PASSWORD_FILE`). Stratégie de mise à jour `Recreate`.
2. L'API : un Service (port 80 vers 8080), un ServiceAccount `api` sans token monté, un Deployment de 2 réplicas, image `tp-api:dev`, utilisateur `api`, mot de passe lu dans `/run/secrets/db/api-password`.
3. Regroupez tous les fichiers avec un `kustomization.yaml` qui fixe le namespace, et appliquez avec `kubectl apply -k`.
4. Accédez à WordPress avec `port-forward`, installez-le, publiez un article, et retrouvez-le dans `/api/posts`.

**Questions.**

1. Pourquoi WordPress reste-t-il à un seul réplica ici ? Que faudrait-il pour en avoir trois ?
2. Pourquoi `Recreate` pour WordPress et `RollingUpdate` pour l'API ?
3. Si l'API démarre avant MariaDB, que se passe-t-il ?

<details>
<summary>Solution</summary>

Extraits de `k8s/base/wordpress.yaml` (sans les ajouts du TP 7) :

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: wp-content           # médias envoyés, plugins, thèmes : doivent survivre aux Pods
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: wordpress
spec:
  replicas: 1
  strategy:
    type: Recreate           # l'ancien Pod libère le volume avant que le nouveau le monte
  selector:
    matchLabels:
      app: wordpress
  template:
    metadata:
      labels:
        app: wordpress
    spec:
      containers:
        - name: wordpress
          image: wordpress:7.1
          ports:
            - name: http
              containerPort: 80
          env:
            - name: WORDPRESS_DB_HOST
              valueFrom:
                configMapKeyRef: { name: wordpress-config, key: DB_HOST }
            - name: WORDPRESS_DB_NAME
              valueFrom:
                configMapKeyRef: { name: wordpress-config, key: DB_NAME }
            - name: WORDPRESS_DB_USER
              valueFrom:
                configMapKeyRef: { name: wordpress-config, key: DB_USER }
            - name: WORDPRESS_DB_PASSWORD_FILE         # le fichier plutôt que la variable
              value: /run/secrets/db/wordpress-password
          volumeMounts:
            - name: wp-content
              mountPath: /var/www/html/wp-content
            - name: db-secret
              mountPath: /run/secrets/db
              readOnly: true
      volumes:
        - name: wp-content
          persistentVolumeClaim:
            claimName: wp-content
        - name: db-secret
          secret:
            secretName: db
            items:                                     # ne monter que la clé utile
              - key: wordpress-password
                path: wordpress-password
```

Le Service et le Deployment de l'API suivent le même modèle (`k8s/base/api.yaml`), avec `DB_USER: api` et la clé `api-password`.

```yaml
# k8s/base/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: wordpress          # appliqué à tous les objets listés
resources:
  - namespace.yaml
  - config.yaml
  - mariadb.yaml
  - wordpress.yaml
  - api.yaml
```

```bash
kubectl apply -k k8s/base
kubectl -n wordpress get pods -w
kubectl -n wordpress port-forward svc/wordpress 8080:80     # http://localhost:8080, installer, publier
kubectl -n wordpress port-forward svc/api 8081:80 &
curl -s localhost:8081/api/posts
```

1. Le PVC est en `ReadWriteOnce` : un seul nœud peut le monter. Sur minikube (un nœud) trois réplicas marcheraient par hasard ; sur un vrai cluster, les Pods placés sur d'autres nœuds resteraient bloqués en `ContainerCreating`. Pour plusieurs réplicas, il faut un stockage `ReadWriteMany` (Azure Files, NFS) pour `wp-content`, ou sortir les médias vers un stockage objet avec un plugin, et des sessions qui ne dépendent pas du Pod.
2. Avec `RollingUpdate`, le nouveau Pod WordPress démarre pendant que l'ancien tient encore le volume RWO : sur un autre nœud, il ne pourrait pas le monter et la mise à jour bloquerait. `Recreate` arrête d'abord l'ancien, au prix d'une courte coupure. L'API n'a pas de volume partagé : elle peut faire un rolling update sans coupure.
3. Ses appels à `/readyz` et `/api/posts` échouent jusqu'à ce que MariaDB réponde ; elle ne plante pas, elle réessaie à chaque requête. Au TP 7, la readiness probe la retire du Service pendant ce temps.

</details>

---

## Partie 6 : un Ingress

**Consignes.**

1. Activez l'addon `ingress` de minikube et regardez ce qu'il installe.
2. Écrivez un Ingress `web` (classe `nginx`) : `wordpress.localhost` vers le Service `wordpress`, `api.localhost` vers le Service `api`. Ajoutez-le dans un **overlay** Kustomize `k8s/minikube/` qui reprend `k8s/base/`.
3. Accédez au contrôleur par `port-forward` sur le port 8080 et testez les deux noms.
4. Que se passe-t-il pour un nom d'hôte qui ne correspond à aucune règle ?

**Questions.**

1. Pourquoi `*.localhost` dispense-t-il de modifier `/etc/hosts` ?
2. Pourquoi l'Ingress est-il dans un overlay et pas dans la base ?

<details>
<summary>Solution</summary>

```bash
minikube addons enable ingress
kubectl -n ingress-nginx get pods,svc      # le contrôleur (un nginx qui lit les objets Ingress)
```

```yaml
# k8s/minikube/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
spec:
  ingressClassName: nginx                 # quel contrôleur doit traiter cet Ingress
  rules:
    - host: wordpress.localhost           # routage sur l'en-tête Host de la requête
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service: { name: wordpress, port: { name: http } }
    - host: api.localhost
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service: { name: api, port: { name: http } }
```

```yaml
# k8s/minikube/kustomization.yaml : le lab = la base + ce qui est propre à minikube
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: wordpress
resources:
  - ../base
  - ingress.yaml
images:
  - name: tp-api            # image locale, chargée avec "minikube image load"
    newTag: dev
```

```bash
kubectl apply -k k8s/minikube
kubectl -n ingress-nginx port-forward svc/ingress-nginx-controller 8080:80 &
curl -sI http://wordpress.localhost:8080/
curl -s  http://api.localhost:8080/version
curl -sI -H "Host: inconnu.localhost" http://localhost:8080/    # 404 du contrôleur
```

1. Les navigateurs et curl résolvent tout nom en `.localhost` vers 127.0.0.1 sans interroger le DNS (le nom est réservé pour ça). Le `port-forward` écoute sur 127.0.0.1:8080 et l'en-tête `Host` garde le nom complet, que le contrôleur utilise pour choisir la règle.
2. La base décrit l'application, identique partout. L'entrée dépend de l'environnement : Ingress nginx et noms `localhost` sur minikube, Gateway et vrai nom de domaine sur Azure. Chaque environnement a son overlay ; c'est la façon Kustomize de « documenter les différences entre lab et prod » demandée par le cours.

Sur Docker Desktop, l'IP du nœud minikube (`minikube ip`) n'est pas joignable depuis l'hôte : d'où le `port-forward`. Sous Linux natif, `curl --resolve wordpress.localhost:80:$(minikube ip) http://wordpress.localhost/` marche aussi.

</details>

---

## Partie 7 : Gateway API avec Envoy Gateway, en HTTPS

**Consignes.**

1. Installez Envoy Gateway avec Helm (il installe aussi les CRD de Gateway API) :

   ```bash
   helm install eg oci://docker.io/envoyproxy/gateway-helm --version v1.9.2 \
     -n envoy-gateway-system --create-namespace --wait
   ```

2. Créez une `GatewayClass` `eg`, une `Gateway` `web` (listener HTTP sur 80) et deux `HTTPRoute` équivalentes à l'Ingress.
3. Regardez l'état de la Gateway (`kubectl describe gateway web`). Pourquoi `Programmed` est-il `False` ? Accédez-y par `port-forward` sur le Service créé par Envoy Gateway.
4. Installez cert-manager avec le support de Gateway API, créez un `ClusterIssuer` auto-signé, et ajoutez à la Gateway un listener HTTPS pour `api.localhost` dont le certificat est demandé par annotation. Testez avec `curl -k`.

**Questions.**

1. Qui crée le Deployment Envoy et son Service ?
2. Comparez la configuration d'une redirection HTTP vers HTTPS avec ingress-nginx et avec Gateway API.

<details>
<summary>Solution</summary>

Les fichiers sont dans `k8s/gateway/`.

```yaml
# GatewayClass : "quel logiciel implémente les Gateway". Un par cluster.
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: eg
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: web
  annotations:
    # cert-manager crée un Certificate pour chaque listener HTTPS et le range dans certificateRefs.
    cert-manager.io/cluster-issuer: selfsigned
spec:
  gatewayClassName: eg
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: Same             # seules les routes du namespace wordpress peuvent s'y attacher
    - name: https-api
      protocol: HTTPS
      port: 443
      hostname: api.localhost    # cert-manager a besoin d'un nom par listener
      tls:
        mode: Terminate          # Envoy déchiffre ; HTTP simple vers les Pods
        certificateRefs:
          - name: api-tls        # Secret créé par cert-manager
      allowedRoutes:
        namespaces:
          from: Same
---
# HTTPRoute : la partie gérée par l'équipe applicative.
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api
spec:
  parentRefs:
    - name: web                  # s'attache à tous les listeners compatibles de la Gateway
  hostnames: [api.localhost]
  rules:
    - backendRefs:
        - name: api
          port: 80
```

(Route `wordpress` identique avec `wordpress.localhost`.)

```bash
kubectl apply -k k8s/gateway        # sans le listener HTTPS au début (étape 2)
kubectl -n wordpress describe gateway web
# Programmed False, AddressNotAssigned : le Service LoadBalancer attend une IP externe.

SVC=$(kubectl -n envoy-gateway-system get svc \
  -l gateway.envoyproxy.io/owning-gateway-name=web -o name)
kubectl -n envoy-gateway-system port-forward "$SVC" 8090:80 8443:443 &
curl -s http://api.localhost:8090/version

# cert-manager, avec la prise en charge des Gateway activée dans sa configuration
cat > cm-values.yaml <<'EOF'
crds:
  enabled: true
config:
  apiVersion: controller.config.cert-manager.io/v1alpha1
  kind: ControllerConfiguration
  gatewayAPI:
    enabled: true
EOF
helm install cert-manager oci://quay.io/jetstack/charts/cert-manager --version v1.21.2 \
  -n cert-manager --create-namespace -f cm-values.yaml --wait

kubectl apply -f k8s/gateway/tls-selfsigned.yaml    # ClusterIssuer "selfsigned"
kubectl apply -k k8s/gateway                         # Gateway avec le listener HTTPS
kubectl -n wordpress get certificate                 # api-tls  READY True
curl -sk https://api.localhost:8443/version
```

1. Envoy Gateway (le contrôleur installé par Helm). Il surveille les objets `Gateway` de sa classe et crée pour chacun un Deployment Envoy et un Service `LoadBalancer`. Sur minikube, personne ne fournit d'IP externe à ce Service (sauf avec `minikube tunnel`), d'où `Programmed: False`. Sur AKS, Azure attribue une IP publique et la Gateway devient `Programmed`.
2. Avec ingress-nginx : une annotation propre à nginx (`nginx.ingress.kubernetes.io/ssl-redirect: "true"`), qui ne marche avec aucun autre contrôleur. Avec Gateway API : une HTTPRoute attachée au listener HTTP avec un filtre `RequestRedirect` (`scheme: https`), qui fait partie du standard. Le TP 9 l'utilise (`k8s/azure/gateway.yaml`).

Si la commande `helm install cert-manager oci://...` échoue, l'ancienne méthode marche aussi : `helm repo add jetstack https://charts.jetstack.io` puis `helm install cert-manager jetstack/cert-manager ...`.

</details>

---

## Bilan

1. Complétez le tableau Compose → Kubernetes du cours avec, pour chaque ligne, le fichier du dépôt qui l'illustre.
2. Secret Kubernetes : citez deux raisons pour lesquelles ce n'est pas un coffre-fort, et ce qu'on utilise à la place en production.
3. Supprimer le Pod MariaDB, le StatefulSet, le PVC, le namespace : lesquelles de ces opérations perdent les données ?
4. Ingress ou Gateway API pour un nouveau projet en 2026 ? Justifiez.
