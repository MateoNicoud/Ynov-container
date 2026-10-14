# TP 7 : résilience, montée en charge et sécurité dans le cluster

## Point de départ

La stack du TP 6 tourne, mais le cluster ne sait rien d'elle : il ne sait pas si l'API est vraiment prête, combien de ressources elle consomme, ni qui a le droit de lui parler. Ce TP ajoute ce qui sépare un déploiement de démonstration d'un déploiement qu'on peut laisser tourner : sondes, ressources, autoscaling, stratégie de mise à jour, durcissement des Pods, droits d'accès et cloisonnement réseau.

Tous les ajouts sont déjà dans `k8s/base/` (solution). Pour faire le TP, repartez de votre version du TP 6.

### Prérequis

- La stack du TP 6 sur minikube, démarré avec `--cni=calico` (indispensable pour la partie 7).
- L'addon metrics-server : `minikube addons enable metrics-server`.
- Testé avec minikube 1.39.0 (Kubernetes 1.37.0), metrics-server 0.9.0, Calico.

---

## Cours

### Les sondes (probes)

Le kubelet interroge régulièrement chaque conteneur. Trois sondes, trois questions, trois réactions :

| Sonde | Question | Si elle échoue |
|---|---|---|
| **startup** | « as-tu fini de démarrer ? » | les deux autres sondes attendent ; après `failureThreshold` échecs, le conteneur est tué |
| **liveness** | « es-tu encore vivant ? » | le conteneur est **tué et redémarré** |
| **readiness** | « peux-tu recevoir du trafic ? » | le Pod est **retiré des Endpoints** du Service, sans redémarrage |

Chaque sonde peut être une requête HTTP (`httpGet`, succès entre 200 et 399), une connexion TCP (`tcpSocket`), une commande (`exec`, succès si code 0) ou un appel gRPC. Les paramètres : `periodSeconds` (intervalle), `timeoutSeconds`, `failureThreshold` (échecs consécutifs avant d'agir), `initialDelaySeconds`.

Le piège classique, cité dans le cours : **une liveness trop agressive ou qui dépend d'autre chose que du processus**. Si la liveness de l'API testait la base de données, une panne de MariaDB ferait redémarrer toutes les API en boucle : des redémarrages qui ne réparent rien et ajoutent de la charge au moment du problème. La règle :

- liveness : le processus est-il bloqué ? Ne regarder que lui (`/healthz` de l'API ne touche pas la base) ;
- readiness : peut-il servir des requêtes utiles ? On peut y tester les dépendances (`/readyz` fait un `ping` MariaDB) ;
- startup : pour les applications lentes au premier démarrage (MariaDB initialise ses tables système), plutôt qu'un `initialDelaySeconds` long qui retarde aussi tous les redémarrages.

### Requests, limits et classes de QoS

- **requests** : ce que le conteneur réserve. Le scheduler ne place un Pod que sur un nœud qui a assez de ressources *non réservées*. C'est aussi la base de calcul du HPA.
- **limits** : le plafond.
  - CPU au-delà de la limite : le conteneur est **ralenti** (*throttling*), pas tué.
  - Mémoire au-delà de la limite : le conteneur est **tué** (`OOMKilled`) et redémarré.

L'unité CPU est le cœur : `100m` (millicores) vaut un dixième de cœur. La mémoire s'écrit `64Mi` (mébioctets).

Selon ce qui est déclaré, Kubernetes classe chaque Pod :

| Classe QoS | Condition | En cas de manque de mémoire sur le nœud |
|---|---|---|
| `Guaranteed` | requests = limits, CPU et mémoire, pour tous les conteneurs | évincé en dernier |
| `Burstable` | au moins une request, mais pas Guaranteed | entre les deux |
| `BestEffort` | rien de déclaré | évincé en premier |

Une pratique courante : requests réalistes (mesurées avec `kubectl top`), limite mémoire, et pas de limite CPU, ou une limite large, pour ne pas brider inutilement. Surdimensionner les requests ne sécurise rien : le cluster semble plein alors que les nœuds sont vides.

### Autoscaling horizontal (HPA)

Le **HorizontalPodAutoscaler** ajuste `replicas` d'un Deployment selon une métrique. Pour le CPU :

```text
réplicas voulus = ceil( réplicas actuels × utilisation moyenne / cible )
```

où l'utilisation est la consommation CPU rapportée aux **requests** (sans request CPU, pas de HPA CPU). Les mesures viennent de **metrics-server**, qui interroge le kubelet de chaque nœud toutes les 15 secondes environ. Le HPA monte vite et descend lentement : il attend une fenêtre de stabilisation (5 minutes par défaut) avant de réduire, pour ne pas osciller.

En lab, un seul nœud partage son CPU entre tous les Pods : passer de 2 à 6 réplicas ne donne pas 3 fois plus de puissance. Le TP montre le mécanisme, pas un gain de performance. En production, on combine le HPA avec l'autoscaler de nœuds (*cluster autoscaler*), et on scale souvent sur des métriques applicatives (requêtes par seconde, longueur de file) plutôt que sur le CPU.

### Stratégies de mise à jour

- **RollingUpdate** (défaut) : remplace les Pods progressivement. `maxSurge` : combien de Pods en plus pendant l'opération ; `maxUnavailable` : combien de Pods en moins. Avec `maxSurge: 1, maxUnavailable: 0`, la capacité ne baisse jamais, au prix d'un Pod supplémentaire temporaire. Pendant un moment, ancienne et nouvelle versions servent en même temps : elles doivent être compatibles (schéma de base, format d'API).
- **Recreate** : arrête tout, puis démarre la nouvelle version. Coupure de service, mais jamais deux versions en même temps (nécessaire quand un volume RWO ou une migration l'impose).
- **Blue-green** : deux déploiements complets ; on bascule le Service (ou la route) de l'un à l'autre d'un coup, et on rebascule en cas de problème.
- **Canary** : une petite part du trafic va vers la nouvelle version, et on augmente si les indicateurs restent bons. Gateway API le permet nativement avec des `backendRefs` pondérés.

Les deux dernières demandent deux Deployments et un routage ; Kubernetes ne les fournit pas seul (des outils comme Argo Rollouts ou Flagger les automatisent).

### Arrêt propre

Pour arrêter un Pod (mise à jour, scale down), Kubernetes le retire des Endpoints, envoie `SIGTERM` au processus, attend `terminationGracePeriodSeconds` (30 s par défaut), puis envoie `SIGKILL`. Une application qui ignore `SIGTERM` coupe les requêtes en cours. L'API (`main.go`) intercepte `SIGTERM` et termine les requêtes en cours avant de s'arrêter.

### SecurityContext et Pod Security Admission

Le `securityContext` traduit la checklist sécurité du TP 3 en champs Kubernetes :

| Champ | Effet | Équivalent Docker |
|---|---|---|
| `runAsNonRoot: true` | refuse de démarrer si l'utilisateur effectif est root | `USER` |
| `runAsUser`, `runAsGroup` | UID et GID du processus | `--user` |
| `fsGroup` | groupe propriétaire des volumes montés | |
| `readOnlyRootFilesystem: true` | système de fichiers de l'image en lecture seule | `--read-only` |
| `allowPrivilegeEscalation: false` | pas de gain de droits par setuid | `no-new-privileges` |
| `capabilities.drop: [ALL]` | retire toutes les capabilities | `--cap-drop ALL` |
| `seccompProfile.type: RuntimeDefault` | filtre d'appels système du runtime | profil seccomp par défaut de Docker |
| `privileged: true` | à proscrire | `--privileged` |

Kubernetes, contrairement à Docker, **n'applique pas seccomp par défaut** : il faut le demander.

Ces champs sont facultatifs. Pour les imposer à tout un namespace, **Pod Security Admission** (intégré à Kubernetes) vérifie chaque Pod contre un profil, choisi par label sur le namespace :

- `privileged` : aucune restriction ;
- `baseline` : interdit les privilèges manifestes (`privileged`, `hostNetwork`, `hostPath`, capabilities ajoutées hors d'une liste courte...) ;
- `restricted` : impose en plus `runAsNonRoot`, `drop: [ALL]`, seccomp, `allowPrivilegeEscalation: false`.

Chaque profil s'applique en mode `enforce` (refus), `warn` (avertissement au `kubectl apply`) ou `audit` (journal).

### Identités et RBAC

Deux sortes d'identités parlent à l'API server : les **utilisateurs** (humains, gérés hors de Kubernetes : certificats, fournisseur OIDC, Entra ID) et les **ServiceAccounts** (applications, objets Kubernetes par namespace). Chaque Pod tourne avec un ServiceAccount (`default` si rien n'est précisé), dont le token est monté par défaut dans `/var/run/secrets/kubernetes.io/serviceaccount/`. Une application qui n'appelle jamais l'API Kubernetes n'a pas besoin de ce token : `automountServiceAccountToken: false`. Un token monté inutilement, c'est un identifiant de plus à voler.

**RBAC** (*Role-Based Access Control*) répond à « qui peut faire quoi sur quoi » :

- un **Role** liste des permissions (verbes `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` sur des ressources) dans un namespace ; un **ClusterRole**, dans tout le cluster ;
- un **RoleBinding** donne un Role à des sujets (utilisateurs, groupes, ServiceAccounts) ; un **ClusterRoleBinding**, partout.

Tout est refusé par défaut ; on ajoute des droits, jamais on n'en retire. `kubectl auth can-i <verbe> <ressource> --as=<sujet>` vérifie un droit.

### NetworkPolicy

Par défaut, tout Pod peut joindre tout Pod, dans tous les namespaces. Une **NetworkPolicy** sélectionne des Pods (par labels) et liste le trafic autorisé en entrée (`ingress`) et/ou en sortie (`egress`). Dès qu'un Pod est sélectionné par une policy d'un type, tout le trafic de ce type non autorisé explicitement est refusé.

Le schéma habituel :

1. une policy « tout refuser en entrée » pour le namespace (`podSelector: {}`) ;
2. une policy par flux légitime (« MariaDB accepte WordPress et l'API sur 3306 »).

Les NetworkPolicy ne font rien si le **plugin réseau** ne les applique pas. Le CNI par défaut de minikube les ignore silencieusement ; Calico (installé avec `--cni=calico`) les applique. Sur AKS, il faut choisir un moteur de policies à la création du cluster (TP 9). C'est l'équivalent du réseau `internal: true` de Compose, en plus fin.

---

## Partie 1 : les sondes

**Consignes.**

1. API : liveness HTTP sur `/healthz`, readiness HTTP sur `/readyz`.
2. MariaDB : startup et readiness avec `healthcheck.sh --connect --innodb_initialized` (script fourni par l'image), liveness avec `healthcheck.sh --connect`.
3. WordPress : readiness HTTP sur `/wp-login.php`, liveness TCP sur le port 80.
4. Arrêtez MariaDB (`kubectl scale statefulset mariadb --replicas=0`). Observez les Pods de l'API et les Endpoints du Service `api`. Les API redémarrent-elles ? Relancez MariaDB.
5. Faites l'expérience inverse : mettez temporairement `/readyz` en liveness de l'API, arrêtez MariaDB, et comptez les redémarrages.

**Questions.**

1. Expliquez la différence observée entre les étapes 4 et 5.
2. Pourquoi une startupProbe pour MariaDB plutôt qu'un `initialDelaySeconds: 120` sur la liveness ?

<details>
<summary>Solution</summary>

```yaml
# k8s/base/api.yaml, conteneur "api"
livenessProbe:                       # le processus répond : ne dépend pas de la base, exprès
  httpGet: { path: /healthz, port: http }
  periodSeconds: 10
  failureThreshold: 3                # 30 s sans réponse avant de redémarrer
readinessProbe:                      # la base répond : sinon, retiré du Service
  httpGet: { path: /readyz, port: http }
  periodSeconds: 5
  failureThreshold: 2
```

```yaml
# k8s/base/mariadb.yaml, conteneur "mariadb"
startupProbe:                        # jusqu'à 24 × 5 s = 2 min pour la première initialisation
  exec:
    command: [healthcheck.sh, --connect, --innodb_initialized]
  periodSeconds: 5
  failureThreshold: 24
readinessProbe:
  exec:
    command: [healthcheck.sh, --connect, --innodb_initialized]
  periodSeconds: 10
livenessProbe:
  exec:
    command: [healthcheck.sh, --connect]
  periodSeconds: 20
  failureThreshold: 3
```

```yaml
# k8s/base/wordpress.yaml, conteneur "wordpress"
readinessProbe:
  httpGet: { path: /wp-login.php, port: http }   # page servie par PHP, qui interroge la base
  periodSeconds: 10
  timeoutSeconds: 5
livenessProbe:
  tcpSocket: { port: http }                      # Apache accepte des connexions
  initialDelaySeconds: 30
  periodSeconds: 20
```

```bash
kubectl -n wordpress scale statefulset mariadb --replicas=0
kubectl -n wordpress get pods -l app=api -w        # READY 0/1, RESTARTS 0
kubectl -n wordpress get endpointslices -l kubernetes.io/service-name=api -o yaml | grep -A2 conditions
# ready: false pour les deux Pods
kubectl -n wordpress scale statefulset mariadb --replicas=1
```

1. Étape 4 : la readiness échoue, les Pods sortent du Service mais tournent toujours ; dès que MariaDB revient, ils redeviennent prêts sans redémarrer. Étape 5 : la liveness échoue, le kubelet tue les conteneurs, qui redémarrent, échouent encore, et finissent en `CrashLoopBackOff` avec des délais croissants. Quand MariaDB revient, il faut attendre la fin du délai en cours avant que l'API reparte : la panne dure plus longtemps que celle de la base.
2. `initialDelaySeconds` s'applique à chaque démarrage du conteneur, y compris les redémarrages après un vrai blocage : MariaDB resterait 2 minutes sans surveillance à chaque fois. La startupProbe ne dure que jusqu'au premier succès ; ensuite, la liveness prend le relais avec ses paramètres courts.

</details>

---

## Partie 2 : ressources

**Consignes.**

1. Mesurez la consommation réelle avec `kubectl top pods -n wordpress`.
2. Donnez à chaque conteneur des requests proches de la mesure et une limite mémoire. API : 50m / 32Mi en requests, 500m / 64Mi en limits.
3. Affichez la classe QoS de chaque Pod.
4. Lancez un Pod `busybox` limité à 32 Mi qui exécute `tail /dev/zero` (qui remplit la mémoire). Quel état obtenez-vous ?
5. Regardez la capacité réservée du nœud : `kubectl describe node minikube`, section *Allocated resources*.

<details>
<summary>Solution</summary>

```yaml
# API : ce que le scheduler réserve, et le plafond
resources:
  requests: { cpu: 50m, memory: 32Mi }     # base du calcul du HPA (partie 3)
  limits: { cpu: 500m, memory: 64Mi }      # au-delà de 64Mi : OOMKilled
```

```bash
kubectl top pods -n wordpress
kubectl -n wordpress get pods -o custom-columns=NOM:.metadata.name,QOS:.status.qosClass
# Burstable partout : requests < limits

kubectl -n wordpress run oom --image=busybox:1.37 --restart=Never \
  --overrides='{"spec":{"containers":[{"name":"oom","image":"busybox:1.37","command":["sh","-c","tail /dev/zero"],"resources":{"limits":{"memory":"32Mi"}}}]}}'
kubectl -n wordpress get pod oom -o jsonpath='{.status.containerStatuses[0].state.terminated.reason}'
# OOMKilled
kubectl -n wordpress delete pod oom

kubectl describe node minikube | grep -A8 "Allocated resources"
```

`tail /dev/zero` lit un flux sans fin de zéros sans jamais trouver de fin de ligne, et garde tout en mémoire. À 32 Mi, le noyau tue le processus (cgroup mémoire), et Kubernetes affiche `OOMKilled` (code de sortie 137). La même chose arriverait à l'API avec une fuite mémoire.

</details>

---

## Partie 3 : autoscaling

**Consignes.**

1. Créez un HPA pour l'API : 2 à 6 réplicas, cible 60 % d'utilisation CPU. Réduisez la fenêtre de stabilisation à la baisse à 60 s.
2. Générez de la charge **depuis un autre namespace**, en passant par le contrôleur d'Ingress (vous comprendrez pourquoi à la partie 7) :

   ```bash
   kubectl create namespace charge
   kubectl -n charge run charge --image=busybox:1.37 --restart=Never -- sh -c \
     'while true; do wget -q -O /dev/null --header "Host: api.localhost" \
        "http://ingress-nginx-controller.ingress-nginx/burn?ms=200"; done'
   ```

3. Suivez le HPA (`kubectl -n wordpress get hpa api -w`) et les Pods. Arrêtez la charge, suivez la descente.

**Questions.**

1. Avec 2 réplicas à 540 % de leur request, combien de réplicas le HPA demande-t-il ? Combien en obtient-il ?
2. Pourquoi la première ligne affiche-t-elle `<unknown>/60%` ?

<details>
<summary>Solution</summary>

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api
spec:
  scaleTargetRef:                 # ce que le HPA pilote
    apiVersion: apps/v1
    kind: Deployment
    name: api
  minReplicas: 2
  maxReplicas: 6
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 60  # 60 % de la request (50m), soit 30m par Pod en moyenne
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 60   # 300 par défaut ; raccourci pour le TP
```

Relevé obtenu en testant ce TP :

```text
cpu: <unknown>/60%   2 réplicas
cpu: 31%/60%         2
cpu: 541%/60%        6
cpu: 229%/60%        6
... charge arrêtée ...
cpu: 6%/60%          6, puis 2 après la fenêtre de 60 s
```

1. ceil(2 × 541 / 60) = 19 réplicas demandés, plafonnés à `maxReplicas` : 6. À 6 réplicas, l'utilisation moyenne reste au-dessus de la cible (229 %) parce que le nœud unique n'a pas plus de CPU à donner : c'est la limite du lab citée par le cours.
2. metrics-server n'a pas encore de mesure pour ces Pods (première collecte, ou Pods trop récents). Sans mesure, le HPA ne fait rien. Si ça dure, vérifiez que l'addon tourne (`kubectl -n kube-system get pods | grep metrics`) et que les conteneurs ont une request CPU.

</details>

---

## Partie 4 : mise à jour sans coupure

**Consignes.**

1. Réglez l'API sur `RollingUpdate` avec `maxSurge: 1`, `maxUnavailable: 0`.
2. Construisez une version `2.0.0` de l'image (`--build-arg VERSION=2.0.0`), chargez-la dans minikube et déployez-la. Pendant le déploiement, appelez `/version` en boucle depuis une autre fenêtre : y a-t-il des erreurs ? Voyez-vous les deux versions ?
3. Revenez en arrière.
4. Pourquoi WordPress est-il en `Recreate` ? (Rappel du TP 6.)

<details>
<summary>Solution</summary>

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 1          # au plus 1 Pod en plus pendant la mise à jour
    maxUnavailable: 0    # jamais moins de Pods prêts que demandé
```

```bash
docker build -t tp-api:2.0.0 --build-arg VERSION=2.0.0 app
minikube image load tp-api:2.0.0

# Fenêtre 1
while true; do curl -s -o /dev/null -w "%{http_code} " http://api.localhost:8080/version; \
  curl -s http://api.localhost:8080/version | grep -o '"version":"[^"]*"'; sleep 0.2; done

# Fenêtre 2
kubectl -n wordpress set image deploy/api api=tp-api:2.0.0
kubectl -n wordpress rollout status deploy/api
kubectl -n wordpress rollout undo deploy/api
```

Que des 200, avec un moment où `dev` et `2.0.0` alternent : pendant un rolling update, les deux versions servent en même temps. Le nouveau Pod ne reçoit du trafic qu'une fois sa readiness réussie, et l'ancien n'est arrêté qu'ensuite (puis termine ses requêtes grâce à la gestion de `SIGTERM`).

</details>

---

## Partie 5 : durcir les Pods

**Consignes.**

1. Ajoutez au namespace `wordpress` les labels Pod Security Admission : `enforce: baseline`, `warn: restricted`. Réappliquez la stack et lisez les avertissements.
2. API : `runAsNonRoot`, UID/GID 65532, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, `drop: [ALL]`, seccomp `RuntimeDefault`. Elle doit respecter `restricted`.
3. MariaDB : même chose avec l'UID 999 (`mysql`), `fsGroup: 999` pour le volume. Avec un système de fichiers en lecture seule, quels dossiers faut-il rendre inscriptibles ?
4. WordPress : `drop: [ALL]`, puis ajoutez une à une les capabilities dont Apache a besoin jusqu'à ce que le Pod démarre. Pourquoi ne peut-il pas respecter `restricted` ?
5. Essayez de créer dans le namespace un Pod `privileged: true`.

<details>
<summary>Solution</summary>

```yaml
# k8s/base/namespace.yaml
metadata:
  name: wordpress
  labels:
    pod-security.kubernetes.io/enforce: baseline   # refus de ce qui viole baseline
    pod-security.kubernetes.io/warn: restricted    # avertissement pour ce qui viole restricted
```

```yaml
# API : profil restricted complet
spec:
  serviceAccountName: api
  securityContext:                       # niveau Pod
    runAsNonRoot: true
    runAsUser: 65532
    runAsGroup: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: api
      securityContext:                   # niveau conteneur
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities:
          drop: [ALL]
```

```yaml
# MariaDB : écrit dans son volume, dans /run/mysqld (socket) et /tmp
securityContext:
  runAsUser: 999
  runAsGroup: 999
  runAsNonRoot: true
  fsGroup: 999                 # le volume appartient au groupe 999 : MariaDB peut y écrire
  seccompProfile:
    type: RuntimeDefault
# ...
volumeMounts:
  - { name: data, mountPath: /var/lib/mysql }
  - { name: run,  mountPath: /run/mysqld }     # emptyDir
  - { name: tmp,  mountPath: /tmp }            # emptyDir
```

```yaml
# WordPress : le minimum dont Apache et l'entrypoint ont besoin
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
    add: [CHOWN, SETUID, SETGID, NET_BIND_SERVICE, DAC_OVERRIDE, FOWNER, KILL]
```

```bash
kubectl apply -k k8s/minikube
# Warning: would violate PodSecurity "restricted:latest": unrestricted capabilities
#   (container "wordpress" must not include "CHOWN", ...), runAsNonRoot != true ...

kubectl -n wordpress run root --image=busybox:1.37 \
  --overrides='{"spec":{"containers":[{"name":"root","image":"busybox:1.37","command":["sleep","60"],"securityContext":{"privileged":true}}]}}'
# Error from server (Forbidden): ... violates PodSecurity "baseline:latest": privileged
```

WordPress démarre en root (port 80, puis bascule vers `www-data`), et son entrypoint fait des `chown`. Le profil `restricted` interdit root et toute capability ajoutée sauf `NET_BIND_SERVICE`. Pour y arriver, il faut une image dérivée qui tourne en `www-data` sur le port 8080 (TP 3, partie 4). L'écart est documenté dans le manifest : c'est ce que le cours appelle une « limite assumée ».

`KILL` sert au processus Apache principal pour arrêter ses processus enfants, qui tournent sous un autre utilisateur.

</details>

---

## Partie 6 : RBAC

**Consignes.**

1. L'API n'appelle pas l'API Kubernetes : donnez-lui un ServiceAccount `api` avec `automountServiceAccountToken: false`. Vérifiez qu'aucun token n'est monté dans ses Pods.
2. Créez un ServiceAccount `dev` et un Role `lecture-debug` qui permet de lire Pods, logs, Services, Endpoints, ConfigMaps, Events, Deployments, StatefulSets et ReplicaSets, sans les Secrets, sans rien modifier. Liez-les.
3. Vérifiez avec `kubectl auth can-i ... --as=system:serviceaccount:wordpress:dev` : lire les Pods, les logs, les Secrets ; supprimer un Pod ; créer un Deployment ; faire un `exec`.
4. Générez un token pour `dev` (`kubectl create token dev`) et utilisez-le pour un vrai appel.

**Questions.**

1. Pourquoi ne pas donner `create pods` au compte de debug, même sans `get secrets` ?
2. Quand utiliser un ClusterRole plutôt qu'un Role ?

<details>
<summary>Solution</summary>

```yaml
# k8s/base/rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dev
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role                                   # limité au namespace wordpress
metadata:
  name: lecture-debug
rules:
  - apiGroups: [""]                          # groupe "core" : pods, services...
    resources: [pods, pods/log, services, endpoints, configmaps, events]
    verbs: [get, list, watch]
  - apiGroups: [apps]
    resources: [deployments, statefulsets, replicasets]
    verbs: [get, list, watch]
  # Ni secrets, ni pods/exec, ni verbes d'écriture.
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: dev-lecture-debug
subjects:
  - kind: ServiceAccount
    name: dev
    namespace: wordpress
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: lecture-debug
```

```bash
kubectl -n wordpress exec deploy/api -- ls /var/run/secrets/kubernetes.io/serviceaccount
# échec : le dossier n'existe pas (et l'image n'a pas de ls, de toute façon)
kubectl -n wordpress get pod -l app=api -o jsonpath='{.items[0].spec.automountServiceAccountToken}'

for v in "get pods" "get pods/log" "get secrets" "delete pods" "create deployments" "create pods/exec"; do
  printf "%-20s %s\n" "$v" "$(kubectl auth can-i $v -n wordpress --as=system:serviceaccount:wordpress:dev)"
done
# get pods yes, get pods/log yes, get secrets no, delete pods no, create deployments no, create pods/exec no

# Token valable 10 minutes pour le ServiceAccount dev
TOKEN=$(kubectl -n wordpress create token dev --duration=10m)
SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CA=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.certificate-authority}')

# KUBECONFIG=/dev/null : sans ça, kubectl présenterait aussi le certificat administrateur
# du kubeconfig minikube, et l'API server vous reconnaîtrait comme admin, pas comme dev.
dev() { KUBECONFIG=/dev/null kubectl --server="$SERVER" --certificate-authority="$CA" --token="$TOKEN" "$@"; }
dev -n wordpress get pods       # OK
dev -n wordpress get secrets    # Forbidden: User "system:serviceaccount:wordpress:dev" cannot list ...
```

1. Créer un Pod, c'est pouvoir monter n'importe quel Secret du namespace dans ce Pod et le lire, ou lancer un Pod avec un autre ServiceAccount plus puissant. `create pods` revient presque à `get secrets`. Même chose pour `pods/exec` : un shell dans le Pod WordPress donne son mot de passe.
2. Pour des ressources qui n'appartiennent à aucun namespace (nœuds, PersistentVolumes, StorageClasses, namespaces eux-mêmes), ou pour définir un rôle réutilisable dans plusieurs namespaces (un ClusterRole peut être lié par un RoleBinding, il n'a alors effet que dans ce namespace).

</details>

---

## Partie 7 : NetworkPolicy

**Consignes.**

1. Avant toute policy : depuis un Pod `busybox` étiqueté `app=intrus`, testez une connexion à `mariadb:3306` (`nc -zv -w 3 mariadb 3306`).
2. Ajoutez une policy qui refuse tout trafic entrant dans le namespace.
3. Autorisez :
   - MariaDB depuis les Pods `app=wordpress` et `app=api`, port 3306 ;
   - WordPress et l'API depuis les namespaces `ingress-nginx` et `envoy-gateway-system`, ports 80 et 8080.
4. Refaites le test de l'étape 1, puis le même avec un Pod étiqueté `app=api`. Vérifiez que le site marche toujours par l'Ingress.
5. Relancez la charge de la partie 3 en appelant directement `http://api.wordpress/burn` depuis le namespace `charge`. Que se passe-t-il ?

**Questions.**

1. Le test avec `app=api` réussit. Est-ce une faille ? Comment la limiter ?
2. Ces policies ne filtrent que l'entrée. Que permettrait une policy de sortie (`egress`) sur l'API ?

<details>
<summary>Solution</summary>

```yaml
# k8s/base/network-policies.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-ingress
spec:
  podSelector: {}               # tous les Pods du namespace
  policyTypes: [Ingress]        # aucune règle "ingress" : tout est refusé en entrée
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: mariadb-from-apps
spec:
  podSelector:
    matchLabels:
      app: mariadb
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector:          # Pods du même namespace avec ces labels
            matchExpressions:
              - { key: app, operator: In, values: [wordpress, api] }
      ports:
        - port: 3306
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: web-from-edge
spec:
  podSelector:
    matchExpressions:
      - { key: app, operator: In, values: [wordpress, api] }
  policyTypes: [Ingress]
  ingress:
    - from:
        - namespaceSelector:    # tous les Pods de ces namespaces (contrôleurs d'entrée)
            matchExpressions:
              - key: kubernetes.io/metadata.name     # label posé automatiquement sur chaque namespace
                operator: In
                values: [ingress-nginx, envoy-gateway-system]
      ports:
        - port: 80
        - port: 8080
```

```bash
kubectl -n wordpress run test --rm -i --restart=Never --image=busybox:1.37 --labels=app=intrus -- \
  nc -zv -w 3 mariadb 3306          # avant : open ; après : Connection timed out
kubectl apply -k k8s/minikube
kubectl -n wordpress run test --rm -i --restart=Never --image=busybox:1.37 --labels=app=api -- \
  nc -zv -w 3 mariadb 3306          # open
curl -s http://api.localhost:8080/readyz      # toujours OK par l'Ingress
```

Étape 5 : les requêtes directes depuis `charge` vers l'API sont bloquées (délai dépassé) ; seul le chemin par le contrôleur d'Ingress est autorisé. C'est pour ça que la partie 3 passait par lui.

1. Les policies se fondent sur les labels, et quiconque peut créer un Pod dans le namespace choisit ses labels. La vraie barrière est donc RBAC : ne pas donner `create pods` dans `wordpress` (partie 6). Pour aller plus loin, certains CNI (Cilium) filtrent sur l'identité du ServiceAccount, et un maillage de services (*service mesh*) authentifie chaque connexion avec des certificats.
2. Empêcher l'API de joindre autre chose que MariaDB et le DNS. Si elle était compromise, l'attaquant ne pourrait ni télécharger d'outils, ni exfiltrer des données vers Internet, ni scanner le reste du cluster. Attention : une policy egress doit autoriser explicitement le DNS (UDP et TCP 53 vers `kube-system`), sinon plus rien ne se résout.

</details>

---

## Bilan

1. Citez trois éléments de la checklist sécurité Kubernetes du cours et le fichier du dépôt qui applique chacun.
2. Une API qui ne répond plus que lentement (10 s par requête) : quelle sonde réagit, et qu'est-ce qui se passe ? Est-ce le bon comportement ?
3. Pour la soutenance : préparez la démonstration en trois commandes du HPA, d'un rolling update sans coupure, et d'une NetworkPolicy qui bloque un intrus.
