# TP 5 : premiers pas dans Kubernetes avec minikube

## Point de départ

La stack tourne avec Compose sur une machine. Si la machine tombe, tout tombe ; si un conteneur plante, il reste planté (ou redémarre au hasard avec une politique `restart`) ; pour passer à trois instances de l'API, il faut un répartiteur de charge à configurer soi-même. Le cours le résume : Compose sert au lab et aux petites installations, pas à la haute disponibilité.

Ce TP installe un cluster Kubernetes local et en fait le tour avec une application jouet, `whoami`, puis avec l'API du TP 3. La stack complète arrive au TP 6.

### Prérequis

- Docker fonctionne (minikube crée son nœud dans un conteneur Docker).
- 4 CPU et 6 Go de RAM libres pour le cluster.
- L'image `tp-api` du TP 3.
- Testé avec minikube 1.39.0, Kubernetes 1.37.0, kubectl 1.36.

---

## Cours

### Ce que Kubernetes ajoute à Docker

Kubernetes lance les mêmes images que Docker, sur un **ensemble de machines** (les nœuds), et se charge de ce que Compose laisse à l'humain :

- **placer** chaque conteneur sur un nœud qui a la place (*scheduling*) ;
- **relancer** ce qui meurt, ailleurs si le nœud a disparu (*self-healing*) ;
- **répartir** le trafic entre les instances et retirer celles qui ne sont pas prêtes ;
- **mettre à jour** sans coupure, et revenir en arrière ;
- **ajuster** le nombre d'instances à la charge.

### Déclarer un état, laisser le cluster converger

Avec Compose, on lance des commandes : « démarre ce service ». Avec Kubernetes, on **déclare un état désiré** : « il doit exister 3 Pods de cette image ». Des programmes appelés **contrôleurs** comparent en boucle l'état désiré (le champ `spec` des objets) à l'état réel (le champ `status`) et agissent pour réduire l'écart. Si un Pod disparaît, l'écart réapparaît, et le contrôleur en recrée un. Personne n'a besoin de le lui demander.

C'est pour ça que `kubectl apply -f` est **idempotent** : appliquer dix fois le même fichier donne le même résultat. Et c'est pour ça que les fichiers YAML dans Git peuvent servir de source de vérité.

### L'architecture d'un cluster

```text
                    ┌───────────────── control plane ─────────────────┐
 kubectl ──HTTPS──► │ kube-apiserver ◄──► etcd (base clé-valeur)      │
                    │      ▲    ▲                                     │
                    │      │    └── kube-scheduler (choisit un nœud)   │
                    │      └─────── kube-controller-manager (boucles)  │
                    └──────┬──────────────────────────────────────────┘
                           │ (les nœuds surveillent l'API)
        ┌──────────────────┴─────────┐   ┌────────────────────────────┐
        │ nœud 1                      │   │ nœud 2                      │
        │ kubelet ─► containerd ─► Pods│   │ kubelet ─► containerd ─► Pods│
        │ kube-proxy, CNI (Calico)    │   │ kube-proxy, CNI (Calico)    │
        └────────────────────────────┘   └────────────────────────────┘
```

- **kube-apiserver** : la seule porte d'entrée. `kubectl`, les contrôleurs, les nœuds : tout le monde passe par lui, en HTTPS, authentifié et autorisé (RBAC, TP 7).
- **etcd** : stocke tous les objets. Perdre etcd, c'est perdre le cluster ; on le sauvegarde.
- **kube-scheduler** : pour chaque Pod sans nœud, choisit un nœud selon les ressources demandées et les contraintes.
- **kube-controller-manager** : héberge les contrôleurs (ReplicaSet, Deployment, Node, Job...).
- **kubelet** (sur chaque nœud) : reçoit les Pods attribués à son nœud et les fait tourner via le *container runtime* (containerd sur minikube 1.39, pas Docker).
- **kube-proxy** : programme les règles réseau qui font marcher les Services.
- **CNI** (Container Network Interface) : le plugin qui donne une IP à chaque Pod et relie les nœuds. On installe Calico, qui applique aussi les NetworkPolicy (TP 7).
- **CoreDNS** : le DNS interne du cluster, lui-même un Deployment.

minikube met tout ça dans un seul conteneur Docker. C'est un lab : un seul nœud, un stockage local, aucune vraie panne de machine possible.

### Les objets

Tout objet Kubernetes a la même forme :

```yaml
apiVersion: apps/v1        # groupe et version de l'API
kind: Deployment           # type d'objet
metadata:                  # nom, namespace, labels, annotations
  name: whoami
spec: {}                   # ce que vous voulez
status: {}                 # ce qui est, rempli par le cluster (ne pas l'écrire)
```

`kubectl explain deployment.spec.strategy` affiche la documentation de n'importe quel champ.

Les **labels** sont des paires clé-valeur attachées aux objets (`app: whoami`). Les **sélecteurs** les utilisent pour relier les objets entre eux : un Service trouve ses Pods par label, un Deployment reconnaît les siens par label. Une faute de frappe dans un label, et le lien est rompu sans erreur.

Les **namespaces** cloisonnent les objets d'un cluster (par équipe, par environnement). Deux objets peuvent porter le même nom dans deux namespaces. Le cloisonnement est seulement logique : sans NetworkPolicy, un Pod peut joindre les Pods de tous les namespaces.

### Pod, ReplicaSet, Deployment

- Le **Pod** est la plus petite unité : un ou plusieurs conteneurs qui partagent une IP, des ports et des volumes, et tournent toujours sur le même nœud. Il est **jetable** : s'il meurt, il ne revient pas ; un nouveau Pod, avec un autre nom et une autre IP, le remplace.
- Le **ReplicaSet** maintient N Pods identiques. On ne le manipule presque jamais directement.
- Le **Deployment** gère des ReplicaSets. Pour une mise à jour, il crée un nouveau ReplicaSet et bascule progressivement les Pods de l'ancien vers le nouveau (*rolling update*). L'ancien ReplicaSet est gardé, vide, pour permettre le retour arrière.

On ne crée pratiquement jamais un Pod seul : rien ne le relancerait.

### Service : une adresse stable devant des Pods qui changent

Les Pods changent d'IP à chaque recréation. Un **Service** fournit un nom DNS et une IP virtuelle stables, et répartit les connexions entre les Pods qui correspondent à son sélecteur **et** qui sont prêts. La liste des IP de ces Pods est tenue à jour dans des objets **EndpointSlice** ; un Service sans Endpoints ne mène nulle part.

| Type | Accessible depuis | Usage |
|---|---|---|
| `ClusterIP` (défaut) | l'intérieur du cluster | communication entre applications, bases de données |
| `NodePort` | un port (30000-32767) de chaque nœud | tests, lab |
| `LoadBalancer` | une IP externe fournie par le cloud | exposition publique (sur minikube : `minikube tunnel`) |
| headless (`clusterIP: None`) | DNS direct vers les IP des Pods | StatefulSet, bases (TP 6) |

### Le DNS du cluster

Chaque Service reçoit un nom `<service>.<namespace>.svc.cluster.local`. Depuis un Pod du même namespace, le nom court (`whoami`) suffit, exactement comme le nom de service sur un réseau Compose. Depuis un autre namespace : `whoami.tp5`.

### Méthode de diagnostic

Toujours dans le même ordre, du symptôme vers la cause :

1. `kubectl get pods` : l'état (`Pending`, `ContainerCreating`, `Running`, `CrashLoopBackOff`, `ImagePullBackOff`, `Error`) ;
2. `kubectl describe pod <nom>` : la section **Events** en bas, écrite par le scheduler et le kubelet, dit presque toujours pourquoi ;
3. `kubectl logs <pod>` et `kubectl logs <pod> --previous` (le conteneur d'avant le dernier redémarrage) ;
4. `kubectl get events --sort-by=.lastTimestamp` pour tout le namespace.

| État | Cause habituelle |
|---|---|
| `Pending` | aucun nœud n'a assez de CPU ou de mémoire pour les *requests*, ou un volume n'est pas disponible |
| `ImagePullBackOff` / `ErrImagePull` | image ou tag inexistant, registry privé sans identifiants |
| `CrashLoopBackOff` | le conteneur démarre puis s'arrête en boucle ; Kubernetes espace les redémarrages (10 s, 20 s, 40 s... jusqu'à 5 min) |
| `Running` mais `0/1` prêt | la readiness probe échoue (TP 7) |

---

## Partie 1 : installer minikube

**Consignes.**

1. Installez minikube et kubectl (voir solution).
2. Démarrez un cluster avec le driver Docker, le CNI Calico, 4 CPU et 6 Go de mémoire.
3. Vérifiez que le nœud est `Ready`, puis listez tous les Pods de tous les namespaces. Retrouvez chaque composant du cours.
4. Regardez le contexte courant de kubectl et le fichier qui le définit.

**Questions.**

1. Où tournent etcd et kube-apiserver dans minikube ?
2. Que se passe-t-il si vous tapez `kubectl get pods` alors que minikube est arrêté ?

<details>
<summary>Solution</summary>

```bash
# minikube (Linux / WSL, amd64). Sous Windows natif : winget install Kubernetes.minikube
curl -LO https://github.com/kubernetes/minikube/releases/download/v1.39.0/minikube-linux-amd64
sudo install minikube-linux-amd64 /usr/local/bin/minikube && rm minikube-linux-amd64

# kubectl : soit installé à part, soit celui embarqué par minikube
alias kubectl="minikube kubectl --"

# --cni=calico : un plugin réseau qui applique les NetworkPolicy (TP 7)
minikube start --driver=docker --cni=calico --cpus=4 --memory=6g

kubectl get nodes                  # minikube   Ready   control-plane
kubectl get pods -A                # etcd-minikube, kube-apiserver-minikube, kube-scheduler-minikube,
                                   # kube-controller-manager-minikube, kube-proxy-..., coredns-..., calico-...
kubectl config current-context     # minikube
kubectl config view --minify       # serveur API, certificat client ; fichier : ~/.kube/config
```

1. Ce sont des **Pods statiques** dans le namespace `kube-system` : le kubelet du nœud les lance directement depuis des fichiers de `/etc/kubernetes/manifests/`, sans passer par le scheduler (qui n'existe pas encore au démarrage). `minikube ssh -- ls /etc/kubernetes/manifests` les montre.
2. `The connection to the server 127.0.0.1:xxxxx was refused` : kubectl ne fait que parler à l'API server. Sans lui, il n'y a plus rien à interroger, même si des conteneurs tournaient encore.

</details>

---

## Partie 2 : un Pod tout seul

**Consignes.**

1. Créez un namespace `tp5` et faites-en le namespace par défaut de votre contexte.
2. Lancez un Pod `traefik/whoami:v1.11.0` avec `kubectl run`. Affichez son IP et son nœud (`-o wide`), puis son YAML complet (`-o yaml`). Repérez `spec` et `status`.
3. Accédez-y depuis votre machine avec `kubectl port-forward`.
4. Supprimez le Pod. Revient-il ?

<details>
<summary>Solution</summary>

```bash
kubectl create namespace tp5
kubectl config set-context --current --namespace=tp5   # évite de répéter -n tp5

kubectl run solo --image=traefik/whoami:v1.11.0
kubectl get pod solo -o wide          # IP du Pod (10.244.x.x), nœud minikube
kubectl get pod solo -o yaml | less   # spec : ce qui a été demandé ; status : phase, IP, conditions

kubectl port-forward pod/solo 8080:80 &   # tunnel temporaire machine -> Pod, via l'API server
curl -s localhost:8080 | head -3
kill %1

kubectl delete pod solo
kubectl get pods                      # plus rien : aucun contrôleur ne le recrée
```

</details>

---

## Partie 3 : un Deployment

**Consignes.**

1. Écrivez `whoami.yaml` : un Deployment `whoami` de 3 réplicas (`traefik/whoami:v1.11.0`, port nommé `http` sur 80, requests 10m CPU / 16Mi). Appliquez-le.
2. Affichez le Deployment, le ReplicaSet et les Pods, avec leurs labels (`--show-labels`).
3. Supprimez un Pod et regardez immédiatement la liste (`kubectl get pods -w`).
4. Passez à 5 réplicas avec `kubectl scale`, puis revenez à 3 **en modifiant le fichier** et en le réappliquant.
5. Retirez le label `app` d'un des Pods (`kubectl label pod <nom> app-`). Que fait le ReplicaSet ? Que devient le Pod ?

**Questions.**

1. Pourquoi revenir à 3 par le fichier plutôt que par `kubectl scale` ?
2. Expliquez le résultat de l'étape 5.

<details>
<summary>Solution</summary>

Le fichier est `k8s/tp5/whoami.yaml` (Deployment et Service, commentés).

```bash
kubectl apply -f k8s/tp5/whoami.yaml
kubectl get deploy,rs,pods --show-labels
# Le ReplicaSet s'appelle whoami-<hash> ; le hash vient du template des Pods.

kubectl delete pod <un-des-pods>; kubectl get pods -w     # un nouveau Pod apparaît aussitôt

kubectl scale deploy/whoami --replicas=5
# dans le fichier : replicas: 3, puis
kubectl apply -f k8s/tp5/whoami.yaml

kubectl label pod <un-pod> app-        # le "-" final retire le label
kubectl get pods --show-labels
```

1. Le fichier est la source de vérité. Un `kubectl scale` est un changement que Git ne connaît pas : le prochain `apply` du fichier le défera sans prévenir. On garde les commandes impératives pour les urgences et l'exploration.
2. Le ReplicaSet ne compte que les Pods qui correspondent à son sélecteur (`app: whoami`). Sans le label, le Pod n'en fait plus partie : le ReplicaSet n'en voit que 2 et en crée un nouveau. Le Pod sans label continue de tourner, orphelin : plus aucun contrôleur ne le gère, et le Service ne lui envoie plus de trafic. C'est la technique pour sortir un Pod malade du service et l'examiner. Supprimez-le à la main ensuite.

</details>

---

## Partie 4 : mise à jour et retour arrière

**Consignes.**

1. Passez l'image à `traefik/whoami:v1.10.4` avec `kubectl set image`, et suivez avec `kubectl rollout status`. Combien de ReplicaSets existent ensuite ?
2. Affichez l'historique (`kubectl rollout history`).
3. Passez l'image à `traefik/whoami:v9.9.9` (qui n'existe pas). Que deviennent les anciens Pods ? Le service est-il coupé ?
4. Revenez à la version précédente avec `kubectl rollout undo`.
5. Ajoutez une annotation `kubernetes.io/change-cause` pour que l'historique dise ce qui a changé.

**Questions.**

1. Pourquoi l'image inexistante ne coupe-t-elle pas le service ?
2. `kubectl rollout undo` affiche un avertissement sur `last-applied-configuration`. Que veut-il dire, et quelle est la bonne pratique ?

<details>
<summary>Solution</summary>

```bash
kubectl set image deploy/whoami whoami=traefik/whoami:v1.10.4
kubectl rollout status deploy/whoami
kubectl get rs                      # 2 ReplicaSets : l'ancien à 0, le nouveau à 3

kubectl rollout history deploy/whoami

kubectl set image deploy/whoami whoami=traefik/whoami:v9.9.9
kubectl get pods
# 1 Pod en ImagePullBackOff, les 3 anciens toujours Running.

kubectl rollout undo deploy/whoami
kubectl annotate deploy/whoami kubernetes.io/change-cause="retour v1.10.4 après image invalide"
kubectl rollout history deploy/whoami
```

1. Par défaut, un rolling update crée au plus 25 % de Pods en plus (`maxSurge`) et n'en retire qu'au plus 25 % (`maxUnavailable`, arrondi à l'inférieur : 0 sur 3 Pods). Un ancien Pod n'est supprimé que quand un nouveau est prêt. Le nouveau n'est jamais prêt, donc les anciens restent. Le déploiement est bloqué, pas le service ; au bout de `progressDeadlineSeconds` (10 min), le Deployment passe en condition `Progressing=False`.
2. `kubectl apply` garde dans une annotation la dernière configuration appliquée, pour calculer les différences. `rollout undo` modifie le Deployment sans la mettre à jour : le prochain `apply` pourrait produire un résultat inattendu. Bonne pratique : `undo` pour éteindre l'incendie, puis corriger le fichier dans Git et le réappliquer.

</details>

---

## Partie 5 : Service et Endpoints

**Consignes.**

1. Créez le Service `whoami` (ClusterIP, port 80 vers le port nommé `http`). Affichez ses Endpoints (`kubectl get endpointslices -l kubernetes.io/service-name=whoami`).
2. Depuis un Pod de debug dans le cluster, appelez le Service 6 fois de suite. Combien de noms de Pods différents voyez-vous ?
3. Modifiez le sélecteur du Service en `app: whoam` (faute de frappe), réappliquez, et regardez les Endpoints. Que renvoie l'appel ?
4. Corrigez. Puis passez le Service en `NodePort` et appelez-le avec `minikube service whoami --url`.

<details>
<summary>Solution</summary>

```bash
kubectl apply -f k8s/tp5/whoami.yaml           # le Service est dans le même fichier
kubectl get svc whoami                         # CLUSTER-IP 10.x.x.x
kubectl get endpointslices -l kubernetes.io/service-name=whoami   # les 3 IP de Pods

# Pod jetable avec des outils réseau (supprimé à la sortie grâce à --rm)
kubectl run dbg --rm -it --restart=Never --image=busybox:1.37 -- \
  sh -c 'for i in 1 2 3 4 5 6; do wget -qO- http://whoami | grep Hostname; done'
# Plusieurs noms différents : kube-proxy répartit les connexions entre les Pods prêts.

# Sélecteur faux : plus d'Endpoints, l'appel échoue (connexion refusée ou délai dépassé)
kubectl get endpointslices -l kubernetes.io/service-name=whoami

# NodePort : un port de 30000 à 32767 ouvert sur le nœud
kubectl patch svc whoami -p '{"spec":{"type":"NodePort"}}'
minikube service whoami -n tp5 --url
```

Des Endpoints vides sont le premier réflexe de diagnostic quand « le Service ne répond pas » : soit le sélecteur ne correspond à aucun label, soit les Pods ne sont pas prêts.

</details>

---

## Partie 6 : DNS et outils de debug

**Consignes.**

1. Depuis un Pod de debug, résolvez `whoami`, `whoami.tp5`, `whoami.tp5.svc.cluster.local`, et `kube-dns.kube-system`. Lisez `/etc/resolv.conf` du Pod.
2. Ouvrez un shell dans un Pod `whoami` : y en a-t-il un ? Utilisez `kubectl debug` pour attacher un conteneur outil à ce Pod.
3. Faites un `port-forward` vers le **Service** (et non plus un Pod).

**Questions.**

1. Pourquoi le nom court `whoami` fonctionne-t-il depuis le namespace `tp5` mais pas depuis `default` ?
2. Quel lien avec le DNS des réseaux Compose (TP 3, partie 0) ?

<details>
<summary>Solution</summary>

```bash
kubectl run dbg --rm -it --restart=Never --image=busybox:1.37 -- sh
  nslookup whoami
  nslookup whoami.tp5.svc.cluster.local
  nslookup kube-dns.kube-system
  cat /etc/resolv.conf
  # search tp5.svc.cluster.local svc.cluster.local cluster.local
  # nameserver 10.96.0.10   (IP du Service kube-dns, servi par CoreDNS)
  exit

kubectl exec -it deploy/whoami -- sh           # échec : l'image n'a pas de shell
# Conteneur éphémère ajouté au Pod, qui partage l'espace de processus de la cible
POD=$(kubectl get pod -l app=whoami -o name | head -1)
kubectl debug -it "$POD" --image=nicolaka/netshoot:v0.15 --target=whoami -- bash
  ps aux        # on voit le processus whoami
  ss -lntp      # port 80 en écoute

kubectl port-forward svc/whoami 8080:80
```

1. La ligne `search` de `/etc/resolv.conf` fait essayer `whoami.tp5.svc.cluster.local` en premier, parce que le Pod est dans `tp5`. Depuis `default`, l'essai devient `whoami.default.svc.cluster.local`, qui n'existe pas. Il faut alors écrire `whoami.tp5`.
2. Même principe : un DNS interne qui résout le nom d'un service vers ses instances. Compose le fait par réseau, Kubernetes par namespace avec un nom complet qui marche partout dans le cluster.

</details>

---

## Partie 7 : l'API du TP 3 et trois pannes classiques

**Consignes.**

1. Chargez l'image locale dans minikube (`minikube image load tp-api:0.1.0`) : le nœud a son propre containerd, il ne voit pas les images de votre Docker.
2. Déployez l'API avec un Deployment de 2 réplicas (`imagePullPolicy: IfNotPresent`). Appelez `/version` et `/readyz` par `port-forward`. Pourquoi `/readyz` répond-il 503 ?
3. Provoquez et diagnostiquez chaque panne avec la méthode du cours :
   - `Pending` : demandez 64 CPU en *requests* ;
   - `ImagePullBackOff` : un tag qui n'existe pas ;
   - `CrashLoopBackOff` : un conteneur `busybox` dont la commande est `sh -c 'exit 1'`.
4. Supprimez le namespace `tp5`.

<details>
<summary>Solution</summary>

```bash
minikube image load tp-api:0.1.0
minikube image ls | grep tp-api

kubectl create deployment api --image=tp-api:0.1.0 --replicas=2 --port=8080
kubectl port-forward deploy/api 8080:8080 &
curl -s localhost:8080/version
curl -s localhost:8080/readyz      # 503 : "lookup mariadb ... no such host", il n'y a pas de base ici
kill %1

# Pending
kubectl run gros --image=busybox:1.37 \
  --overrides='{"spec":{"containers":[{"name":"gros","image":"busybox:1.37","command":["sleep","600"],"resources":{"requests":{"cpu":"64"}}}]}}'
kubectl describe pod gros | tail -3
# FailedScheduling  0/1 nodes are available: 1 Insufficient cpu.

# ImagePullBackOff
kubectl run fantome --image=tp-api:n-existe-pas
kubectl describe pod fantome | tail -5      # Failed to pull image ... not found

# CrashLoopBackOff
kubectl run crash --image=busybox:1.37 -- sh -c 'exit 1'
kubectl get pod crash -w                    # Error, puis CrashLoopBackOff, RESTARTS qui augmente
kubectl logs crash --previous

kubectl delete namespace tp5                # supprime tout ce qu'il contient
kubectl config set-context --current --namespace=default
```

`kubectl create deployment` met `imagePullPolicy: IfNotPresent` pour un tag autre que `latest`. Avec `latest`, la politique par défaut est `Always` : le nœud chercherait l'image sur Docker Hub et échouerait.

</details>

---

## Bilan

1. Quel est le rôle de l'API server, et pourquoi tout passe-t-il par lui ?
2. Pod ou Deployment : lequel écrire, et pourquoi ?
3. Un Service ne répond pas. Donnez les trois premières commandes que vous lancez.
4. Dressez le tableau de correspondance entre ce que vous savez faire avec Compose et ce que vous venez de faire avec Kubernetes (service, `ports`, réseau, nom DNS, `restart`, `--scale`). Le TP 6 le complète.
