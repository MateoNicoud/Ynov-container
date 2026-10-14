# TP 8 : OpenBao dans Kubernetes, identifiants dynamiques pour l'API

## Point de départ

Au TP 6, les mots de passe sont dans un Secret Kubernetes : encodé en base64, lisible par quiconque a `get secrets` ou peut créer un Pod, jamais changé. Le cours le dit : un Secret n'est pas un coffre-fort. Les TP 1 et 2 ont réglé ce problème pour Compose avec OpenBao et des comptes MariaDB temporaires. Ce TP fait la même chose dans le cluster, pour l'API.

À la fin du TP :

- OpenBao tourne dans le cluster (installé avec Helm) ;
- l'API prouve son identité avec son ServiceAccount, sans aucun mot de passe stocké ;
- chaque Pod de l'API reçoit son propre compte MariaDB en lecture seule, renouvelé automatiquement et supprimé quand il n'est plus utilisé.

### Prérequis

- Les TP 1 et 2 (vocabulaire OpenBao : policies, moteur `database`, leases, agent).
- La stack du TP 7 sur minikube, NetworkPolicy comprises.
- Testé avec le chart Helm `openbao/openbao` 0.30.3 (OpenBao 2.7.1), injecteur `hashicorp/vault-k8s` 1.7.6.

La solution est dans `k8s/openbao/` : `setup.sh` (configuration d'OpenBao, commentée), `networkpolicy.yaml`, `api-patch.yaml`, `kustomization.yaml`.

---

## Cours

### Quatre façons d'amener un secret externe dans un Pod

| Méthode | Principe | Le secret existe-t-il dans etcd ? | Rotation |
|---|---|---|---|
| **External Secrets Operator** | un opérateur lit le coffre et crée/actualise un Secret Kubernetes | oui | l'opérateur réécrit le Secret ; l'application doit relire |
| **Driver CSI Secrets Store** | un volume CSI monte les secrets du coffre en fichiers au démarrage du Pod | non (sauf option de synchronisation) | à intervalle, si activé |
| **Injecteur (sidecar)** | un webhook ajoute au Pod un agent qui s'authentifie et écrit les secrets en fichiers | non | l'agent renouvelle et réécrit |
| **Appel direct** | l'application parle elle-même à l'API du coffre | non | à la charge de l'application |

Pour des identifiants **dynamiques** (créés à la demande, avec un bail à renouveler), il faut un composant qui vit aussi longtemps que le Pod : c'est le rôle de l'agent injecté. C'est exactement le `bao-agent` des TP 1 et 2, placé cette fois dans chaque Pod. Le TP 9 utilise le driver CSI avec Azure Key Vault, pour des secrets statiques.

### Le webhook d'admission

Quand on crée un objet, l'API server peut le soumettre à des **webhooks d'admission** avant de l'enregistrer. Un webhook *mutating* peut le modifier. L'injecteur OpenBao en est un : pour chaque Pod qui porte l'annotation `vault.hashicorp.com/agent-inject: "true"`, il ajoute :

- un **conteneur d'init** `vault-agent-init`, qui s'authentifie et écrit les secrets avant le démarrage de l'application (l'API ne démarre jamais sans ses identifiants) ;
- un **conteneur sidecar** `vault-agent`, qui reste pendant toute la vie du Pod pour renouveler les baux et réécrire les fichiers ;
- un volume en mémoire monté sur `/vault/secrets` dans les trois conteneurs.

Les annotations commencent par `vault.hashicorp.com/` : le chart OpenBao réutilise l'injecteur de HashiCorp (`hashicorp/vault-k8s`), compatible avec l'API d'OpenBao.

### La méthode d'authentification Kubernetes

Le problème du « secret zéro » (TP 1 : le `secret_id` d'AppRole dans `.env`) disparaît dans Kubernetes, parce que chaque Pod a déjà une identité signée par le cluster : le **token de son ServiceAccount**, un JWT monté dans le Pod.

```text
Pod (SA "api")                         OpenBao                         API server
  │  login(role=api, jwt=<token SA>)  ──►│                                 │
  │                                      │── TokenReview(jwt) ───────────►│
  │                                      │◄── valide : wordpress/api ──────│
  │                                      │ vérifie : SA "api" dans "wordpress"
  │◄── token OpenBao (policy "api") ─────│   autorisé par le rôle ?        │
```

OpenBao ne croit pas le Pod sur parole : il demande à l'API server si le token est authentique (*TokenReview*). Pour ça, OpenBao doit lui-même avoir le droit de faire des TokenReview : le chart Helm le lui donne (ClusterRoleBinding vers `system:auth-delegator`). Le rôle OpenBao lie ensuite une identité Kubernetes précise (`bound_service_account_names`, `bound_service_account_namespaces`) à des policies.

Conséquence : un Pod qui tourne avec un autre ServiceAccount, même dans le même namespace, n'obtient rien. Et la sécurité repose sur RBAC : qui peut créer des Pods avec le ServiceAccount `api` peut obtenir ses identifiants (TP 7, partie 6).

### Mode dev et production

Ce TP installe OpenBao en **mode dev** : en mémoire, descellé, token root `root`. Après un redémarrage du Pod `openbao-0`, toute la configuration est perdue (comme au TP 1, partie 7). En production on utiliserait :

- le stockage `raft` sur un PVC, avec 3 ou 5 réplicas pour la haute disponibilité ;
- l'**auto-unseal** par un KMS : OpenBao supporte notamment `seal "azurekeyvault"`, qui confie la clé de descellement à Azure Key Vault ;
- TLS sur le listener, et un device d'audit.

---

## Partie 1 : installer OpenBao avec Helm

**Consignes.**

1. Ajoutez le dépôt Helm d'OpenBao et regardez les valeurs du chart (`helm show values`) : repérez `server.dev`, `injector`, `server.authDelegator`.
2. Installez OpenBao dans le namespace `openbao`, en mode dev avec le token root `root`, injecteur activé.
3. Listez ce qui a été créé : Pods, Services, `MutatingWebhookConfiguration`, `ClusterRoleBinding`.

**Questions.**

1. Que fait Helm de plus que `kubectl apply` ?
2. Pourquoi le mode dev est-il acceptable ici, et pas pour de vrai ?

<details>
<summary>Solution</summary>

```bash
helm repo add openbao https://openbao.github.io/openbao-helm
helm repo update
helm show values openbao/openbao --version 0.30.3 | less

# server.dev.enabled : OpenBao en mémoire, descellé, token root fixé
# (l'injecteur est activé par défaut)
helm install openbao openbao/openbao --version 0.30.3 \
  -n openbao --create-namespace \
  --set server.dev.enabled=true --set server.dev.devRootToken=root \
  --wait

kubectl -n openbao get pods,svc
# openbao-0 (serveur), openbao-agent-injector-... (webhook)
kubectl get mutatingwebhookconfigurations | grep openbao
kubectl get clusterrolebinding | grep openbao    # openbao-openbao-server-binding -> system:auth-delegator
```

1. Helm assemble les manifests à partir de modèles et de valeurs (`--set`, fichier `-f values.yaml`), les applique, et garde une trace de chaque installation (*release*) : `helm list`, `helm upgrade`, `helm rollback`, `helm uninstall`. Pour un logiciel tiers, c'est la façon standard de l'installer et de le mettre à jour sans écrire soi-même des centaines de lignes de YAML.
2. Le TP porte sur l'authentification Kubernetes et l'injection, pas sur l'exploitation d'OpenBao (déjà vue au TP 1). En mode dev, tout disparaît au redémarrage du Pod, le token root est connu, et rien n'est chiffré en transit.

</details>

---

## Partie 2 : le compte MariaDB d'OpenBao

**Consignes.**

1. Créez dans MariaDB un compte `bao` qui peut créer des utilisateurs et donner `SELECT` sur la base `wordpress`. Le mot de passe initial peut être jetable.
2. Configurez le moteur `database` d'OpenBao vers `mariadb.wordpress.svc.cluster.local:3306`. La commande échoue-t-elle ? Pourquoi ?
3. Corrigez avec une NetworkPolicy, puis faites tourner le mot de passe du compte `bao` (`rotate-root`).

**Questions.**

1. Pourquoi le nom complet `mariadb.wordpress.svc.cluster.local` ?
2. Pourquoi `GRANT SELECT ... WITH GRANT OPTION` pour le compte `bao` ?

<details>
<summary>Solution</summary>

```bash
# Raccourci : la CLI bao du Pod openbao-0 avec le token root du mode dev
bao() { kubectl -n openbao exec -i openbao-0 -- env BAO_TOKEN=root bao "$@"; }

ROOT_PW=$(kubectl -n wordpress get secret db -o jsonpath='{.data.root-password}' | base64 -d)
BAO_PW=$(openssl rand -hex 16)
kubectl -n wordpress exec -i mariadb-0 -- mariadb -uroot -p"$ROOT_PW" <<SQL
CREATE USER 'bao'@'%' IDENTIFIED BY '$BAO_PW';
GRANT CREATE USER ON *.* TO 'bao'@'%';                       -- créer et supprimer des comptes
GRANT SELECT ON wordpress.* TO 'bao'@'%' WITH GRANT OPTION;  -- pouvoir transmettre SELECT
SQL

bao secrets enable database
bao write database/config/wordpress \
  plugin_name=mysql-database-plugin \
  connection_url='{{username}}:{{password}}@tcp(mariadb.wordpress.svc.cluster.local:3306)/' \
  allowed_roles=api \
  username=bao password="$BAO_PW"
# Échec : délai de connexion dépassé. La policy default-deny-ingress du TP 7 bloque OpenBao.
```

```yaml
# k8s/openbao/networkpolicy.yaml : MariaDB accepte aussi le namespace openbao
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: mariadb-from-openbao
spec:
  podSelector:
    matchLabels:
      app: mariadb
  policyTypes: [Ingress]
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: openbao
      ports:
        - port: 3306
```

```bash
kubectl -n wordpress apply -f k8s/openbao/networkpolicy.yaml
# relancer "bao write database/config/wordpress ...", puis :
bao write -f database/rotate-root/wordpress     # plus personne ne connaît le mot de passe de bao
```

1. OpenBao tourne dans le namespace `openbao` : le nom court `mariadb` y serait cherché comme `mariadb.openbao.svc.cluster.local`, qui n'existe pas (TP 5, partie 6).
2. On ne peut donner un droit qu'on n'a pas, et il faut le droit de le transmettre. `bao` ne lit pas la base lui-même : il donne `SELECT` aux comptes qu'il crée. `CREATE USER` lui permet aussi de les supprimer (`DROP USER`).

</details>

---

## Partie 3 : rôle, policy et authentification Kubernetes

**Consignes.**

1. Créez le rôle database `api` : utilisateur avec `SELECT` sur `wordpress`, bail d'1 h, 24 h maximum. Précisez l'instruction de révocation (`DROP USER IF EXISTS`), comme au TP 2.
2. Écrivez une policy `api` qui autorise seulement la lecture de `database/creds/api`.
3. Activez la méthode `kubernetes` et configurez-la avec l'adresse de l'API server vue depuis le cluster.
4. Créez le rôle Kubernetes `api` : ServiceAccount `api`, namespace `wordpress`, policy `api`.
5. Testez à la main : depuis un Pod qui tourne avec le ServiceAccount `api`, lisez son token et appelez `auth/kubernetes/login`.

**Questions.**

1. Où OpenBao trouve-t-il les identifiants pour appeler l'API TokenReview ?
2. L'API a `automountServiceAccountToken: false` (TP 7). Qu'est-ce que ça change ici ?

<details>
<summary>Solution</summary>

Tout est dans `k8s/openbao/setup.sh`, qui rejoue les parties 2 et 3 :

```bash
bao write database/roles/api \
  db_name=wordpress \
  creation_statements="CREATE USER '{{name}}'@'%' IDENTIFIED BY '{{password}}'; GRANT SELECT ON wordpress.* TO '{{name}}'@'%';" \
  revocation_statements="DROP USER IF EXISTS '{{name}}'@'%';" \
  default_ttl=1h max_ttl=24h

bao policy write api - <<'HCL'
path "database/creds/api" {
  capabilities = ["read"]
}
HCL

bao auth enable kubernetes
# Adresse de l'API server depuis un Pod. Sans autre paramètre, OpenBao utilise le token
# et le certificat CA de son propre ServiceAccount pour appeler TokenReview.
bao write auth/kubernetes/config kubernetes_host=https://kubernetes.default.svc:443

bao write auth/kubernetes/role/api \
  bound_service_account_names=api \
  bound_service_account_namespaces=wordpress \
  token_policies=api \
  token_ttl=1h
```

```bash
# Test manuel : un Pod avec le ServiceAccount api, token monté
kubectl -n wordpress run testsa --rm -i --restart=Never --image=busybox:1.37 \
  --overrides='{"spec":{"serviceAccountName":"api","automountServiceAccountToken":true}}' -- \
  sh -c 'JWT=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token);
         wget -qO- --post-data "{\"role\":\"api\",\"jwt\":\"$JWT\"}" \
           http://openbao.openbao.svc:8200/v1/auth/kubernetes/login'
# Réponse JSON avec auth.client_token et "policies":["api","default"]
```

1. Dans le Pod `openbao-0` : le token de son ServiceAccount (`openbao`) et le CA du cluster, montés par Kubernetes. Le ClusterRoleBinding créé par le chart l'autorise à faire des TokenReview.
2. Sans token monté, le Pod n'a rien à présenter : l'agent ne peut pas s'authentifier. Il faut réactiver `automountServiceAccountToken` pour l'API. On rend un identifiant au Pod, mais un identifiant de courte durée, propre à ce ServiceAccount et vérifié à chaque usage, à la place d'un mot de passe permanent partagé.

</details>

---

## Partie 4 : injecter l'agent dans l'API

**Consignes.**

1. Créez un overlay Kustomize `k8s/openbao/` qui reprend `k8s/minikube/` et modifie le Deployment `api` :
   - annotations d'injection : rôle `api`, deux fichiers `db_user` et `db_password` rendus depuis `database/creds/api` par des templates ;
   - `DB_USER_FILE=/vault/secrets/db_user` et `DB_PASSWORD_FILE=/vault/secrets/db_password` ;
   - suppression de la variable `DB_USER` et du volume `db-secret` (le Secret Kubernetes n'est plus utilisé) ;
   - `automountServiceAccountToken: true`.
2. Appliquez. Combien de conteneurs ont les Pods de l'API ? Lisez les logs du conteneur `vault-agent`.
3. Vérifiez `/api/posts`, puis listez les utilisateurs MariaDB `v-%`.
4. Lancez un Pod avec les mêmes annotations mais le ServiceAccount `default`. Que se passe-t-il ?
5. Relancez la charge du TP 7. Le HPA réagit-il comme avant ?

**Questions.**

1. Combien de comptes MariaDB existent quand l'API a 2 réplicas ? Et après un rolling update ?
2. Pourquoi réduire les ressources demandées par l'agent ?
3. Le Secret `db` contient toujours `api-password`. Que reste-t-il à faire ?

<details>
<summary>Solution</summary>

```yaml
# k8s/openbao/api-patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    metadata:
      annotations:
        vault.hashicorp.com/agent-inject: "true"          # le webhook ajoute l'agent
        vault.hashicorp.com/role: api                     # rôle auth/kubernetes/role/api
        # Fichier /vault/secrets/db_user, rendu par ce template :
        vault.hashicorp.com/agent-inject-secret-db_user: database/creds/api
        vault.hashicorp.com/agent-inject-template-db_user: |
          {{ with secret "database/creds/api" }}{{ .Data.username }}{{ end }}
        # Même secret, même bail : l'agent ne fait qu'une demande pour les deux fichiers.
        vault.hashicorp.com/agent-inject-secret-db_password: database/creds/api
        vault.hashicorp.com/agent-inject-template-db_password: |
          {{ with secret "database/creds/api" }}{{ .Data.password }}{{ end }}
        # Par défaut l'agent demande 250m de CPU. Le HPA calcule sur la somme des requests du Pod :
        # 250m d'agent contre 50m d'API, et la charge de l'API deviendrait invisible.
        vault.hashicorp.com/agent-requests-cpu: 10m
        vault.hashicorp.com/agent-limits-cpu: 100m
        vault.hashicorp.com/agent-requests-mem: 16Mi
        vault.hashicorp.com/agent-limits-mem: 64Mi
    spec:
      automountServiceAccountToken: true     # l'agent présente ce token à OpenBao
      containers:
        - name: api
          env:
            - name: DB_USER
              $patch: delete                 # le nom d'utilisateur vient maintenant d'OpenBao
            - name: DB_PASSWORD_FILE
              value: /vault/secrets/db_password
            - name: DB_USER_FILE
              value: /vault/secrets/db_user
          volumeMounts:
            - name: db-secret
              $patch: delete
      volumes:
        - name: db-secret
          $patch: delete                     # plus aucun Secret Kubernetes monté
```

```yaml
# k8s/openbao/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: wordpress
resources:
  - ../minikube
  - networkpolicy.yaml
patches:
  - path: api-patch.yaml
```

```bash
./k8s/openbao/setup.sh            # parties 2 et 3 d'un coup, si vous repartez de zéro
kubectl apply -k k8s/openbao
kubectl -n wordpress get pods -l app=api
# READY 2/2 : api + vault-agent (et un conteneur d'init vault-agent-init, terminé)
kubectl -n wordpress logs deploy/api -c vault-agent --tail=5
# rendered "(dynamic)" => "/vault/secrets/db_user" ...

curl -s http://api.localhost:8080/api/posts
ROOT_PW=$(kubectl -n wordpress get secret db -o jsonpath='{.data.root-password}' | base64 -d)
kubectl -n wordpress exec mariadb-0 -- mariadb -uroot -p"$ROOT_PW" \
  -e "SELECT user FROM mysql.user WHERE user LIKE 'v-%'"
# v-kubernetes-api-6fOFtPIuXaiu2Ln ...

# Même annotations, ServiceAccount "default"
kubectl -n wordpress run intrus --image=busybox:1.37 --restart=Never \
  --overrides='{"metadata":{"annotations":{"vault.hashicorp.com/agent-inject":"true","vault.hashicorp.com/role":"api","vault.hashicorp.com/agent-inject-secret-x":"database/creds/api"}},"spec":{"containers":[{"name":"c","image":"busybox:1.37","command":["sleep","300"]}]}}'
kubectl -n wordpress get pod intrus                     # Init:0/1, bloqué
kubectl -n wordpress logs intrus -c vault-agent-init    # Code: 403 ... service account name not authorized
kubectl -n wordpress delete pod intrus
```

1. Un par Pod : chaque agent s'authentifie séparément et demande ses propres identifiants. Après un rolling update, les anciens comptes existent encore jusqu'à la fin de leur bail (1 h au plus) ou jusqu'à la révocation du token de l'agent, puis OpenBao les supprime. On voit donc temporairement plus de comptes que de Pods. Le nom (`v-kubernetes-api-...`) indique la méthode d'authentification et le rôle : dans les journaux MariaDB, on sait d'où vient chaque connexion.
2. Le HPA fait la moyenne de l'utilisation CPU rapportée à la **somme des requests de tous les conteneurs du Pod**. Avec 250m pour l'agent et 50m pour l'API, l'API pourrait saturer sa part sans que le Pod dépasse 60 %. Avec 10m, le calcul reflète l'API. (Le HPA `autoscaling/v2` permet aussi de cibler un seul conteneur avec une métrique `ContainerResource`.)
3. Supprimer la clé `api-password` du Secret et l'utilisateur statique `api` de MariaDB, qui ne servent plus. Puis faire de même pour WordPress (rôle database avec tous les droits sur la base) et, pour finir, faire gérer le mot de passe root par OpenBao. Le Secret `db` pourra alors disparaître.

</details>

---

## Bilan

1. Comparez les quatre méthodes du cours pour cette stack : laquelle pour WordPress, laquelle pour l'API, laquelle sur Azure ?
2. Qu'est-ce qui remplace le `secret_id` d'AppRole du TP 1 dans ce TP ? Pourquoi est-ce plus sûr ?
3. OpenBao en mode dev redémarre : que se passe-t-il pour l'API ? Pour les comptes MariaDB déjà créés ?
