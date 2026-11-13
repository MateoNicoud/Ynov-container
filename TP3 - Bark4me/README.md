# TP3 : Bark4me sur un cluster kubeadm HA, Azure

Bark4me est le faux site de rencontre pour chiens décrit dans `GLOSSARY.md`. Le TP3 le fait tourner sur le même type de cluster que le TP2 : 3 VM Azure créées par OpenTofu (`tofu/`), configurées par Ansible (`automatisation/`), avec l'application dans `app/` et sa livraison par GitHub Actions (`.github/workflows/tp3.yml` à la racine du dépôt).

Ce dépôt fournit la plateforme, l'application et la CI. Les manifestes Kubernetes de l'application (Deployments, Services, Ingress, Postgres, NetworkPolicy, PodDisruptionBudgets, Certificate...) ne sont pas fournis : on les écrit à la main.

Glossaire utilisé dans tout le document :

**Bark4me** : le faux site de rencontre pour chiens. Satire, sans authentification.

**Profil** : la fiche d'un chien affichée sur une carte à swiper (nom, race, phrase d'accroche, photo, pédigrée éventuel).

**Paquet** : la liste ordonnée des Profils présentés à un Visiteur, une seule fois chacun. Une dizaine de Profils au total.

**Visiteur** : une personne (ou un chien) qui swipe. Anonyme, identifié seulement par un identifiant tiré au hasard côté navigateur.

**Patte** : le « like ». Remplace le cœur.

**Truffe** : le « super like ». Remplace l'étoile. Provoque toujours un Match.

**Panneau** : le « non ». Un panneau « interdit aux chiens », ironique. Ne provoque jamais de Match.

**Swipe** : le choix d'un Visiteur sur un Profil : Patte, Truffe ou Panneau.

**Match** : le résultat positif d'un Swipe. Toujours pour une Truffe, déterministe pour une Patte.

**Hotdog** : l'ultime « profil » : quand le Paquet est épuisé, le site annonce « Il n'y a plus de hotdogs dans votre région ».

**Pédigrée** : le faux certificat « pédigrée validé » délivré à certains chiens, avec un numéro vérifiable. Remplace les certifications de sérieux d'un vrai site.

**Éleveur** : le public de la fausse page pour éleveurs, parodie de page professionnelle.

**Mode daltochien** : le thème qui restreint les couleurs au spectre visible par un chien (bleus et jaunes), activable par un interrupteur.

## Prérequis

- `az` connecté (`az login`) sur l'abonnement Azure for Students
- `tofu` (OpenTofu 1.10 ou plus), `uv`, `make`, `kubectl`
- `tofu/azure.auto.tfvars` (gitignoré), copié depuis `azure.auto.tfvars.example`, avec le subscription ID et le tenant ID (`az account show --query "{id:id, tenant:tenantId}"`)
- `tofu/terraform.tfvars`, copié depuis `terraform.tfvars.example`, avec le `dns_label` et la clé SSH publique de chaque membre
- Docker, pour lancer l'application en local (voir `app/README.md`)

## Déroulé

```bash
cd "TP3 - Bark4me/tofu"
cp azure.auto.tfvars.example azure.auto.tfvars
cp terraform.tfvars.example terraform.tfvars
make up                      # tofu init puis tofu apply -auto-approve

cd ../automatisation
make install                 # venv, dépendances Python, collections Ansible
make deploy                  # ansible-playbook site.yml
export KUBECONFIG="$PWD/kubeconfig"
kubectl get nodes
```

1. **`make up`** crée le resource group `rg-tp3-bark4me`, le réseau, le Load Balancer, les 3 VM, l'arrêt du soir, et écrit `automatisation/inventory.yml`. Les versions de la plateforme viennent de `platform_versions` dans `tofu/variables.tf`. `make validate` vérifie la syntaxe et le format sans rien créer.
2. **`make deploy`** enchaîne les playbooks de `site.yml`. Il est rejouable sans casse.
3. **La CI** construit, scanne et publie les images, puis met à jour les Deployments (voir plus bas).
4. **Les manifestes** de l'application s'appliquent à la main, depuis le poste, avec `KUBECONFIG` pointant sur `automatisation/kubeconfig` : `kubectl apply -k <dossier-des-manifestes>`. Le namespace, les Deployments, le StatefulSet Postgres et le ClusterIssuer sont à écrire. Les Deployments doivent s'appeler `web`, `profiles`, `swipes`, `pedigree` et `photos` (le nom du service, sans préfixe), et leur conteneur porte le même nom. Seules les images gardent le préfixe : `ghcr.io/<owner>/bark4me-<service>`. Les scénarios et `group_vars/all.yml` (`app_services`) suivent cette convention ; les variables `K8S_NAMESPACE`, `DEPLOY_PREFIX` et `CONTAINER_PREFIX` en tête du workflow s'adaptent à d'autres noms (alors `APP_DEPLOYMENTS` côté scénarios).
5. **`make check`** vérifie la plateforme puis l'application.

| Playbook | Rôle |
|---|---|
| `01-nodes.yml` | système des nœuds, containerd, kubelet/kubeadm, disque Longhorn, arrêt du soir |
| `02-apiserver-lb.yml` | HAProxy local `127.0.0.1:8443` vers les 3 apiservers |
| `03-kubeadm.yml` | init et join des 3 control-planes, kubeconfig du poste |
| `04-cni.yml` | Calico |
| `05-helm.yml` | Helm sur k8s-1 |
| `07-ingress.yml` | Traefik, cert-manager (contrôleur et CRD seulement), metrics-server |
| `08-storage.yml` | Longhorn |

Les contrôles, dans `automatisation/` :

| Cible | Contenu |
|---|---|
| `make check-platform` | 3 nœuds Ready, 3 membres etcd started, pods Running, workloads des addons, NodePort 30080/30443 de Traefik, metrics-server, StorageClass `longhorn-postgres` à 3 réplicas, `longhorn` à 2 |
| `make check-app` | PVC Bound, Certificate Ready, réponse HTTPS 200 avec certificat valide sur `/`, `/api/profiles`, `/api/swipes/stats`, `/api/pedigree` et `/photos/chien-01.jpg` |
| `make check` | les deux. `check-platform` marche sans l'application déployée |
| `make syntax` | `--syntax-check` de `site.yml` et `check.yml` |

## Frontière automatisé et manuel

| Élément | Qui | Comment |
|---|---|---|
| VM, réseau, Load Balancer, IP publique, inventaire | automatisé | `make up` |
| Arrêt du soir des VM | automatisé | `make up` (planification Azure) et `make deploy` (cron dans la VM) |
| Cluster kubeadm HA, CNI, Helm | automatisé | `make deploy` |
| Traefik, cert-manager, metrics-server, Longhorn | automatisé | `make deploy` |
| Build, scan Trivy et push des images | automatisé | CI, à chaque push sur `main` touchant `app/` |
| Mise à jour des images des Deployments | automatisé | CI, `kubectl set image` puis `rollout status` |
| ClusterIssuer cert-manager | manuel | écrit à la main, le playbook 07 n'en crée aucun |
| Manifestes de l'application et de Postgres | manuel | `kubectl apply -k` |
| Secret GitHub, visibilité publique des packages GHCR | manuel | interface GitHub |
| Scénarios de panne | manuel | `make scenario-*` |
| Destruction | manuel | `make down` |

La CI ne fait jamais de `kubectl apply` : elle ne touche qu'à l'image d'un Deployment qui existe déjà. Le premier push sur `main` échoue donc tant que les manifestes ne sont pas appliqués.

## Secrets GitHub

Un seul secret est à créer, dans Settings, Secrets and variables, Actions :

| Secret | Valeur |
|---|---|
| `KUBECONFIG_B64` | `base64 -w0 automatisation/kubeconfig` |

Le push vers GHCR utilise le `GITHUB_TOKEN` fourni par GitHub. Les images sont publiques pour éviter tout `imagePullSecret` : un package créé par le workflow est privé au départ, il faut passer sa visibilité sur Public une fois (Packages, `bark4me-<service>`, Package settings, Change visibility). Le job `deploy` joint l'apiserver sur `lb_fqdn:6443` depuis les runners GitHub, dont les adresses ne sont pas connues d'avance : un `admin_cidr` restreint à votre IP bloque la livraison (la règle NSG du port 6443 ne laisse alors passer que cette IP). Le job `deploy` écrit le kubeconfig dans un fichier temporaire en `umask 077`, ne l'affiche pas et le supprime à la fin.

## Scénarios de démo

Les 4 scénarios se lancent depuis `automatisation/`, avec `KUBECONFIG` sur `automatisation/kubeconfig` (c'est la valeur par défaut). Chacun sonde le site pendant la panne (deux sondes : `/` et `/photos/...`, sans Postgres, avec la tolérance stricte `PROBE_TOLERANCE` ; `/api/profiles`, `/api/swipes/stats` et `/api/pedigree`, qui dépendent de Postgres, avec `PROBE_PG_TOLERANCE`), attend le retour à la normale, puis lance `make check` (sauf avec `SKIP_MAKE_CHECK=1`). `make scenarios` les enchaîne.

| Cible | Panne | Attendu |
|---|---|---|
| `make scenario-drain` | drain du nœud qui porte Postgres (`NODE=k8s-2` pour en choisir un autre), puis uncordon | les PodDisruptionBudgets évitent toute coupure du site |
| `make scenario-node-stop` | arrêt brutal de la VM de ce nœud par `az vm stop --skip-shutdown`, puis redémarrage | Postgres est recréé sur un autre nœud, le quorum etcd tient (2 control-planes sur 3). Prend plusieurs minutes |
| `make scenario-longhorn` | suppression d'un réplica du volume de Postgres | Longhorn en reconstruit un autre, le site ne s'interrompt pas |
| `make scenario-postgres` | suppression du pod `postgres-0` | le StatefulSet le recrée sur le même volume, les statistiques de Swipes reviennent |

Les réglages utiles sont des variables d'environnement : `APP_NAMESPACE` (`bark4me`), `APP_FQDN`, `PROBE_TOLERANCE` (15 sondes ratées consécutives tolérées, le temps que le Load Balancer retire un nœud arrêté), `PROBE_PG_TOLERANCE` (240, pour la bascule de Postgres qui prend plusieurs minutes ; le scénario drain la ramène à `PROBE_TOLERANCE`, puisque les PodDisruptionBudgets interdisent toute coupure, Postgres compris), `RECOVERY_TIMEOUT` (900 s), `AZ_RESOURCE_GROUP` (`rg-tp3-bark4me`). La liste complète est en tête de `automatisation/scenarios/lib.sh`.

Si un scénario est interrompu, il remet le nœud en service (uncordon ou `az vm start`) avant de sortir.

## Repli B2s_v2

La taille cible est `Standard_B4s_v2`. Si le crédit Azure ou le quota de vCPU manque, décommenter dans `tofu/terraform.tfvars` :

```hcl
vm_size = "Standard_B2s_v2"
```

puis `make up`. Le cluster tient avec 2 vCPU et 8 Go par nœud, avec moins de marge pendant les scénarios de panne.

## Arrêt du soir

Les VM s'arrêtent seules chaque soir :

- Azure les désalloue à 20 h, heure de Paris (`shutdown_time = "2000"` et `shutdown_timezone` dans `tofu/variables.tf`).
- `01-nodes.yml` pose en plus un cron à 20 h 05 qui appelle l'API `deallocate` avec l'identité managée de la VM, au cas où l'arrêt Azure ne désallouerait pas (heures dans `deallocate_hour` et `deallocate_minute` de `group_vars/all.yml`).

Le cron de 20 h 05 coupe aussi une VM rallumée avant cette heure (par exemple pour une démo qui déborde) : prévoir de la rallumer ou d'ajuster `deallocate_hour` et `deallocate_minute`.

Après cette heure, rallumer avant de diagnostiquer quoi que ce soit :

```bash
for n in k8s-1 k8s-2 k8s-3; do az vm start -g rg-tp3-bark4me -n "$n"; done
```

En fin de projet, tout détruire :

```bash
cd "TP3 - Bark4me/tofu"
make down
```

Les disques, l'IP publique et le Load Balancer sont facturés même VM désallouées : seul `make down` arrête la facture.

## Exercice optionnel : CA interne cert-manager

Cet exercice est facultatif et distinct du Pédigrée. Le Pédigrée est le faux certificat du domaine, un numéro vérifiable servi par le service `pedigree`. Ici il s'agit de vrais certificats TLS entre deux composants du cluster, et rien dans le déroulé ci-dessus n'en dépend.

**Objectif.** Chiffrer la connexion de `swipes` vers Postgres avec une CA interne émise par cert-manager, déjà installé par `07-ingress.yml`.

**Ce que fait le code aujourd'hui.** `app/swipes/main.go` ouvre la base avec le driver `pgx` et lit l'URL dans la variable d'environnement `DATABASE_URL`. La valeur par défaut est `postgres://bark4me:bark4me@postgres:5432/bark4me`, sans paramètre `sslmode`. Le code n'a rien à changer : tout passe par l'URL donnée au Deployment, où `sslmode`, `sslrootcert`, `sslcert` et `sslkey` se règlent en paramètres. Sans `sslmode`, pgx essaie TLS puis retombe en clair si le serveur ne le propose pas, ce qui ne protège de rien.

**Pistes.**

1. Un `ClusterIssuer` de type `selfSigned`, qui sert uniquement à signer la CA.
2. Un `Certificate` avec `isCA: true` dans le namespace de l'application, signé par ce premier émetteur. Son Secret contient la clé et le certificat de la CA.
3. Un second `ClusterIssuer`, de type `ca`, qui référence ce Secret. C'est lui qui émet les certificats utiles.
4. Un `Certificate` serveur pour Postgres, dont les noms DNS couvrent le Service (`postgres`, `postgres.<namespace>.svc`). Monter son Secret dans le pod Postgres et activer `ssl=on` avec `ssl_cert_file` et `ssl_key_file`. Postgres refuse une clé lisible par le groupe ou par tous, et l'utilisateur du conteneur doit pouvoir la lire.
5. Un `Certificate` client pour `swipes`, si vous voulez de l'authentification par certificat. C'est facultatif : un serveur TLS seul suffit pour chiffrer.
6. Dans le Deployment de `swipes`, monter le Secret de la CA (`ca.crt`, et le couple client s'il y en a un) et ajouter à `DATABASE_URL` les paramètres `sslmode` et `sslrootcert`. `require` chiffre sans vérifier le serveur, `verify-ca` vérifie la chaîne, `verify-full` vérifie aussi le nom. Visez `verify-full`.
7. Pour imposer TLS côté serveur, un `pg_hba.conf` avec des lignes `hostssl` et sans ligne `host` pour l'application.

**Critères de réussite.**

- `kubectl get certificate` montre la CA, le certificat serveur (et le client s'il existe) en `Ready`.
- `swipes` démarre avec `sslmode=verify-full` et `/api/swipes/stats` répond 200.
- Une connexion en clair depuis un autre pod (`sslmode=disable`) est refusée.
- Une connexion en `verify-full` sans la CA échoue sur la vérification du certificat.
- `SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid();` renvoie `t` depuis une session ouverte par `swipes`.
- `make scenario-postgres` passe toujours : le pod recréé repart avec ses certificats.
- Le renouvellement est prévu : Postgres ne relit ses certificats qu'au rechargement de sa configuration, il faut décider comment ce rechargement se fait.

Ce chantier reste hors de la CI : l'image de `swipes` et le workflow n'ont pas à changer.
