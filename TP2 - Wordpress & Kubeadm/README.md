# TP2 : cluster kubeadm HA et WordPress sur Azure

Le sujet prévoit 3 VM Proxmox sur un réseau L2. Ici, les 3 VM tournent sur un abonnement Azure for Students, créées par OpenTofu (`tofu/`) puis configurées par Ansible (`automatisation/`). Le site répond sur https://tp2-k8s-ynov.spaincentral.cloudapp.azure.com avec un certificat Let's Encrypt.

## Prérequis

- `az` connecté (`az login`) sur l'abonnement Azure for Students
- `tofu` (OpenTofu 1.10 ou plus)
- `uv` et `make`
- la clé SSH publique de chaque membre dans `tofu/terraform.tfvars`
- `tofu/azure.auto.tfvars` (gitignoré), copié depuis `azure.auto.tfvars.example`, avec le subscription ID et le tenant ID de l'abonnement (`az account show --query "{id:id, tenant:tenantId}"`)
- le mot de passe du vault Ansible (donné à l'oral)

## Lancement

```bash
cd "TP2 - Wordpress & Kubeadm/tofu"
cp azure.auto.tfvars.example azure.auto.tfvars
tofu init
tofu apply

cd ../automatisation
make install
make deploy
make check
export KUBECONFIG="$PWD/kubeconfig"
kubectl get nodes
```

`tofu apply` crée le resource group, le réseau, le Load Balancer et les 3 VM, puis écrit `automatisation/inventory.yml`. `make deploy` enchaîne les playbooks de `site.yml` et demande le mot de passe du vault. Pour un lancement sans invite, mettre ce mot de passe dans `automatisation/.vault_pass` (gitignoré) et lancer `ANSIBLE_VAULT_PASSWORD_FILE=.vault_pass make deploy`.

`make check` vérifie 3 nœuds Ready, 3 membres etcd started, aucun pod hors Running/Completed, les PVC Bound, le Certificate Ready et la réponse HTTPS du site avec un certificat valide.

Un second `make deploy` ne casse rien : il se termine sans échec.

| Playbook | Rôle |
|---|---|
| `01-nodes.yml` | hostname, swap, modules, sysctl, containerd, kubelet/kubeadm/kubectl figés, iSCSI, multipath, disque Longhorn, arrêt à 19 h |
| `02-apiserver-lb.yml` | HAProxy local `127.0.0.1:8443` vers les 3 apiservers |
| `03-kubeadm.yml` | init sur k8s-1, join control-plane de k8s-2 puis k8s-3, retrait du taint, kubeconfig du poste |
| `04-cni.yml` | Calico en VXLAN |
| `05-helm.yml` | Helm sur k8s-1 |
| `07-ingress.yml` | Traefik, cert-manager, ClusterIssuers, metrics-server |
| `08-storage.yml` | Longhorn |
| `09-wordpress.yml` | Secret depuis le vault, manifests kustomize de `files/k8s/wordpress/` |
| `11-etcd-snapshot.yml` | `make snapshot`, hors `site.yml` |

Les versions sont figées dans `automatisation/group_vars/all.yml`. Le module 06 (MetalLB) n'existe pas, le Load Balancer Azure le remplace.

## Architecture

```
                         Internet
                            |
          tp2-k8s-ynov.spaincentral.cloudapp.azure.com
                            |
        +-------------------+--------------------+
        |        Azure Load Balancer Standard     |
        |  80  -> 30080   443 -> 30443            |
        |  6443 -> 6443   2201/2202/2203 -> 22    |
        |  outbound SNAT pour apt et les images   |
        +-------------------+--------------------+
                            |   VNet 10.0.1.0/24
        +-------------------+-------------------+
        |                   |                   |
   k8s-1 10.0.1.11     k8s-2 10.0.1.12     k8s-3 10.0.1.13
   +-------------+     +-------------+     +-------------+
   | HAProxy     |     | HAProxy     |     | HAProxy     |
   | 127.0.0.1:  |     | 127.0.0.1:  |     | 127.0.0.1:  |
   | 8443 -> les |     | 8443 -> les |     | 8443 -> les |
   | 3 :6443     |     | 3 :6443     |     | 3 :6443     |
   | apiserver   |     | apiserver   |     | apiserver   |
   | etcd        |     | etcd        |     | etcd        |
   +-------------+     +-------------+     +-------------+
        Calico VXLAN entre les 3 nœuds, Longhorn sur /var/lib/longhorn

Traefik x2 (NodePort 30080/30443, TLS cert-manager)
   -> Service wordpress -> WordPress (Deployment, HPA 1 à 3)
                              -> PVC wordpress-data (Longhorn, 2 réplicas)
                              -> mariadb-0 (StatefulSet, Service headless)
                                    -> PVC data-mariadb-0 (Longhorn, 2 réplicas)
```

Chaque nœud est control-plane et worker. kubelet et les composants du plan de contrôle passent par le HAProxy local (`controlPlaneEndpoint: 127.0.0.1:8443`). `kubectl` sur le poste passe par la règle 6443 du Load Balancer : le certificat de l'apiserver couvre le FQDN, l'IP publique, `127.0.0.1` et les IP privées.

## Écarts avec le sujet Proxmox

| TP (Proxmox) | Azure | Raison |
|---|---|---|
| VIP kube-vip en ARP | HAProxy local sur chaque nœud et règle 6443 du Load Balancer | le VNet Azure ne relaie pas l'ARP gratuit, une VIP annoncée en ARP n'est jamais joignable |
| MetalLB L2 | Load Balancer Azure vers les NodePort fixes de Traefik | même raison |
| Calico IP-in-IP | Calico VXLAN, BGP désactivé | Azure bloque le protocole IP 4 entre VM |
| ClusterIssuer selfsigned et nip.io | Let's Encrypt HTTP-01 sur le label DNS Azure de l'IP publique | vrai certificat, accepté par les navigateurs |
| Snapshot Proxmox | `tofu destroy` puis `tofu apply` et `make deploy` | reconstruction complète scriptée |
| `environment_check.sh` de Longhorn | vérification de `iscsi_tcp` et `iscsid` par 01-nodes | le script n'existe plus dans Longhorn 1.12 |
| 3 vCPU par VM | `Standard_B2s_v2` (2 vCPU, 8 Go) | quota étudiant de 6 vCPU par région |

## Équipe, coût et destruction

Ajouter une clé SSH : une ligne dans `ssh_public_keys` de `tofu/terraform.tfvars`, puis `tofu apply`. La connexion se fait par l'IP publique et un port par nœud :

```bash
ssh -p 2201 azureuser@tp2-k8s-ynov.spaincentral.cloudapp.azure.com   # k8s-1
ssh -p 2202 azureuser@tp2-k8s-ynov.spaincentral.cloudapp.azure.com   # k8s-2
ssh -p 2203 azureuser@tp2-k8s-ynov.spaincentral.cloudapp.azure.com   # k8s-3
```

Le mot de passe SSH est désactivé, seules les clés passent.

Coût relevé sur l'API de prix Azure le 23/10/2026 (spaincentral, en dollars) :

| Ressource | Prix |
|---|---|
| 3 × `Standard_B2s_v2` | 3 × 0,0912 $/h, soit 0,27 $/h |
| IP publique Standard | 0,005 $/h, facturée même VM arrêtées |
| Load Balancer Standard (jusqu'à 5 règles) | environ 0,025 $/h, facturé même VM arrêtées |
| 3 disques OS 40 Go (E6) et 3 data disks 20 Go (E4) | 21,6 $/mois |

Cluster allumé : environ 0,33 $/h, soit 2,6 $ pour une séance de 8 h. Cluster arrêté mais pas détruit : environ 1,4 $ par jour (disques, IP, Load Balancer).

Chaque VM se désalloue elle-même à 19 h, heure de Paris. Après cette heure, rallumer avant de diagnostiquer quoi que ce soit :

```bash
az vm start -g rg-tp2-k8s -n k8s-1
az vm start -g rg-tp2-k8s -n k8s-2
az vm start -g rg-tp2-k8s -n k8s-3
```

En fin de séance, tout détruire :

```bash
cd "TP2 - Wordpress & Kubeadm/tofu"
tofu destroy
```

Le quota régional est de 6 vCPU et les 3 VM le prennent en entier. Aucune VM de plus n'est possible sans demande de quota.

## Sauvegardes

Snapshot etcd, copié dans `automatisation/backups/` (gitignoré) :

```bash
cd "TP2 - Wordpress & Kubeadm/automatisation"
make snapshot
```

Le playbook installe `etcdctl` et `etcdutl` à la version de l'etcd du cluster, prend le snapshot sur k8s-1, affiche `etcdutl snapshot status` et rapatrie le fichier.

Snapshot et retour arrière Longhorn sur le volume WordPress :

```bash
kubectl -n longhorn-system port-forward svc/longhorn-frontend 8080:80
```

1. Dans http://localhost:8080, volume de `wordpress-data`, Take Snapshot (`avant-test`).
2. Casser : `kubectl -n wordpress exec deploy/wordpress -- sh -c 'echo CASSE > /var/www/html/casse.txt'`
3. `kubectl -n wordpress scale deploy/wordpress --replicas=0`, attendre que le volume soit détaché.
4. Dans l'UI, Attach en mode maintenance sur un nœud, puis Revert vers `avant-test`, puis Detach.
5. `kubectl -n wordpress scale deploy/wordpress --replicas=1` : `casse.txt` a disparu.

## Pannes

Poste d'observation :

```bash
cd "TP2 - Wordpress & Kubeadm/automatisation"
./ha-probe.sh
watch -n2 'kubectl get nodes; echo; kubectl -n wordpress get pods -o wide'
```

`ha-probe.sh` interroge le FQDN chaque seconde, sans `-k` puisque le certificat est valide. `az vm stop --skip-shutdown` coupe la VM sans arrêt propre, comme un Stop Proxmox.

| Scénario | Arrêt | Retour |
|---|---|---|
| P1, un nœud applicatif | `az vm stop -g rg-tp2-k8s -n k8s-2 --skip-shutdown` | `az vm start -g rg-tp2-k8s -n k8s-2` |
| P3, le premier control-plane | `az vm stop -g rg-tp2-k8s -n k8s-1 --skip-shutdown` | `az vm start -g rg-tp2-k8s -n k8s-1` |
| P5, 2 control-planes sur 3 | `az vm stop -g rg-tp2-k8s -n k8s-2 --skip-shutdown` puis `-n k8s-3` | `az vm start -g rg-tp2-k8s -n k8s-2` |

Mesures du 23/10/2026 :

- P1 (k8s-2 portait MariaDB) : k8s-2 passe NotReady après 1 min 30. MariaDB est supprimé au bout de 6 min et recréé sur k8s-3 à 8 min 30. Le site renvoie 503 pendant ce temps, puis 200.
- P3 : `kubectl get nodes` répond en 0,2 s pendant toute la panne, via k8s-2 et k8s-3. Il n'y a pas de VIP à déplacer : la sonde 6443 du Load Balancer et HAProxy écartent k8s-1. Le site tombe en 503 parce que les pods WordPress étaient sur k8s-1.
- P5 : `kubectl` passe en timeout puis en `Forbidden`, l'apiserver ne lisant plus le RBAC dans etcd. Le site ne répond plus non plus, alors que Traefik, MariaDB, WordPress et les volumes tournaient sur k8s-1. Les deux CoreDNS étaient sur k8s-3, et WordPress ne résolvait plus `mariadb-0.mariadb`. Au redémarrage de k8s-2, l'API revient en 30 s.

## Démo et preuves

`automatisation/demo.sh` enchaîne la démonstration en 10 étapes, avec une pause avant chaque action. On peut n'en lancer qu'une partie : `./demo.sh 8 10` joue seulement les pannes.

| Étape | Contenu |
|---|---|
| 1 | nœuds Ready, taints, membres etcd, leader, `make check` |
| 2 | Traefik, ClusterIssuers, Certificate, certificat Let's Encrypt vu par `curl` |
| 3 | disque Longhorn monté, StorageClass, PVC, volumes Longhorn |
| 4 | suppression du pod WordPress, titre du site avant et après |
| 5 | probes, `id` de MariaDB, SecurityContext, token absent, RBAC, NetworkPolicy, montée du HPA sous charge |
| 6 | snapshot etcd rapatrié dans `backups/` |
| 7 | snapshot et revert Longhorn, guidés dans l'UI |
| 8 | P3, arrêt de k8s-1 |
| 9 | P1, arrêt du nœud qui porte MariaDB |
| 10 | P5, arrêt de k8s-2 et k8s-3 |

Pour garder une trace des étapes 1 à 6, enregistrer le terminal :

```bash
cd "TP2 - Wordpress & Kubeadm/automatisation"
mkdir -p ../preuves
asciinema rec ../preuves/01-06-cli.cast -c "./demo.sh 1 6"
```

Pour les étapes 7 à 10, filmer l'écran (OBS, ou Win+G sous Windows) avec trois terminaux visibles : `./demo.sh 8 10`, `./ha-probe.sh`, et `watch -n2 'kubectl get nodes; kubectl -n wordpress get pods -o wide'`.

Preuves à mettre dans `preuves/` :

| Fichier | Ce qu'on doit y voir |
|---|---|
| `01-06-cli.cast` | étapes 1 à 6 sans erreur, `failed=0` dans `make check` |
| `site.png` | le site ouvert dans le navigateur, sur le FQDN |
| `certificat.png` | le détail du certificat dans le navigateur : émis par Let's Encrypt, valide |
| `longhorn-volumes.png` | l'UI Longhorn, les 2 volumes `healthy` avec 2 réplicas |
| `longhorn-revert.mp4` | l'étape 7 : `casse.txt` présent, revert dans l'UI, `casse.txt` absent |
| `p3.mp4` | `kubectl` qui répond pendant l'arrêt de k8s-1 |
| `p1.mp4` | ha-probe en 503 puis de nouveau en 200, MariaDB recréé sur un autre nœud |
| `p5.mp4` | `kubectl` en erreur, état du site, retour de l'API au redémarrage de k8s-2 |
| `azure.png` | le portail Azure : resource group `rg-tp2-k8s` avec les 3 VM, le Load Balancer et l'IP publique |

Pour chaque panne, noter la prédiction avant l'arrêt et l'écart observé après : c'est ce que demande le module 12 du sujet.

## Limites

- SSH (2201 à 2203) et l'API (6443) sont ouverts à tout Internet (`admin_cidr = 0.0.0.0/0`) pour que le groupe se connecte sans gérer d'IP. L'accès SSH se fait par clé seulement, l'API par certificat client.
- Le vault Ansible a un mot de passe partagé dans le groupe. Qui l'a peut lire les mots de passe MariaDB.
- Le HPA scale un Deployment dont le volume est RWO : une podAffinity garde tous les replicas sur le nœud du volume. Si ce nœud tombe, tous les replicas tombent avec lui. Le RWX Longhorn est hors socle.
- WordPress ne tourne pas en non-root. L'image officielle démarre Apache en root puis passe en `www-data`. Toutes les capabilities sont retirées sauf `NET_BIND_SERVICE`, `SETUID`, `SETGID`, `CHOWN`, `DAC_OVERRIDE` et `FOWNER` : sans les deux dernières, la copie initiale des fichiers sur un volume neuf échoue. MariaDB tourne en `999`.
- L'arrêt automatique Azure (DevTestLab) n'existe pas en spaincentral. Chaque VM a donc une identité managée, avec le rôle Virtual Machine Contributor sur elle seule, et un cron à 19 h qui appelle l'API `deallocate`.
- Let's Encrypt refuse les adresses en `example.com`. Les ClusterIssuers n'ont pas d'e-mail de contact.
- Let's Encrypt limite à 5 certificats par semaine pour un même nom. Pendant les destroy/apply répétés, passer `cluster_issuer` à `letsencrypt-staging` dans `group_vars/all.yml`.
- Comme vu en P5, CoreDNS peut se retrouver entier sur un seul nœud.

## Questions du TP

**Q1, swap.** kubelet calcule les limites mémoire et les évictions sur la RAM réelle. Avec du swap, un pod peut dépasser sa limite sans être tué et le nœud devient lent sans que kubelet le voie, donc kubelet refuse de démarrer par défaut si le swap est actif.

**Q2, SystemdCgroup.** containerd crée alors les cgroups des conteneurs par systemd, comme kubelet configuré en `cgroupDriver: systemd`. Deux gestionnaires de cgroups sur la même machine donnent deux vues des ressources qui divergent sous pression.

**Q3, kubeconfig privilégié (adaptée).** Au premier `init`, kube-vip doit écrire un Lease avant que les droits RBAC d'`admin.conf` existent, d'où `super-admin.conf`. Ici HAProxy fait du TCP pur sans parler à l'API : aucun kubeconfig, et il tourne avant `kubeadm init`.

**Q4, static pod (adaptée).** Le kubelet du nœud gère les static pods, déclarés dans `/etc/kubernetes/manifests` : apiserver, etcd, controller-manager et scheduler. HAProxy, qui prend la place de kube-vip, est un service systemd hors Kubernetes.

**Q5, taint.** Les pods qui tolèrent `node-role.kubernetes.io/control-plane:NoSchedule` : les DaemonSets kube-proxy et calico-node, CoreDNS, les static pods. Ici le taint est retiré, tout se place sur les 3 nœuds.

**Q6, VIP (adaptée).** Oui, `kubectl` répond pendant l'arrêt de k8s-1 (0,2 s mesuré en P3). Personne ne déplace de VIP : la sonde TCP 6443 du Load Balancer retire k8s-1 du pool, et sur les nœuds HAProxy marque son backend DOWN.

**Q7, quorum.** 3 membres tolèrent 1 panne (quorum de 2). Avec 2 membres le quorum reste à 2, aucune panne n'est tolérée et il y a deux fois plus de machines qui peuvent tomber qu'avec 1.

**Q8, LoadBalancer Pending (adaptée).** Un Service `type: LoadBalancer` attend qu'un contrôleur lui donne une IP externe, et sans cloud-provider-azure ni MetalLB personne ne le fait. Ici Traefik est en NodePort fixe et le Load Balancer créé par tofu pointe sur ces ports. MetalLB L2 resterait muet, faute d'ARP gratuit sur le VNet.

**Q9, Service et Ingress.** Un Service donne une IP et un port stables vers des pods (niveau 4). Un Ingress décrit des règles HTTP par hôte, chemin et TLS, appliquées par un contrôleur (Traefik) qui envoie ensuite vers des Services.

**Q10, pod et PVC.** Supprimer le pod garde la donnée : le PVC et le volume Longhorn restent, le pod suivant la retrouve. Supprimer le PVC avec la `reclaimPolicy: Delete` de Longhorn supprime le volume et la donnée.

**Q11, StatefulSet et Deployment.** MariaDB a besoin d'un nom stable (`mariadb-0`, résolu par le Service headless) et d'un volume lié à ce nom (`volumeClaimTemplates`). Les pods WordPress sont interchangeables, tout leur état est dans le PVC et la base.

**Q12, exo 9 K3s.** Même couple WordPress et base avec Deployment, Service et PVC. Ici le stockage est répliqué (Longhorn au lieu de local-path), Traefik est installé à la main, le TLS vient de Let's Encrypt par cert-manager et les mots de passe sortent d'un vault.

**Q13, hors snapshot etcd.** Les données des volumes Longhorn, les images de conteneurs, la PKI de `/etc/kubernetes/pki`, les manifests des static pods, la configuration de containerd et de kubelet, et l'infrastructure Azure (VM, Load Balancer, disques).

**Q14, snapshot Longhorn.** Non. Le snapshot vit dans les réplicas du volume, à l'intérieur du cluster. Il faut un backup Longhorn vers une cible externe (S3, NFS, Azure Blob) pour survivre à la perte du cluster.

**Q15, éviction.** Le nœud passe NotReady après le délai de grâce du node controller, puis les pods gardent 300 s de tolérance `unreachable`, pour ne pas tout déplacer à la moindre coupure réseau. Pour un StatefulSet, Kubernetes ne recrée pas `mariadb-0` tant que l'ancien n'est pas confirmé mort ; c'est `nodeDownPodDeletionPolicy` de Longhorn qui force la suppression (8 min 30 au total en P1).

**Q16, apps sans API.** Les conteneurs tournent sous containerd et kubelet, et les règles kube-proxy et les routes Calico restent en place : une application déjà lancée n'a pas besoin de l'API. Ce qui s'arrête : le scheduling, la recréation des pods, la mise à jour des endpoints et le renouvellement des certificats. En P5, le site est tombé quand même, parce que CoreDNS tournait sur un nœud arrêté.
