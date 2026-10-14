# Le dépôt face au cours « Conteneurisation » (63 h)

Comparaison entre le support de cours (F. Guillemard, 64 diapositives) et ce que contient le dépôt. Les TP 1 et 2 vont plus loin que le cours sur les secrets ; tout le reste du cours (image maison, CI, Kubernetes) manquait. Les TP 3 à 9 le couvrent.

## Partie Docker (diapositives 3 à 33)

| Thème du cours | État avant les nouveaux TP | Où c'est traité |
|---|---|---|
| Conteneur vs VM, namespaces, cgroups | Pas abordé | TP 3, cours |
| `docker run`, `logs`, `exec`, `inspect`, `cp` | Utilisés au passage | TP 3, partie 0 |
| Écrire un Dockerfile, `.dockerignore` | **Absent** : la stack n'utilise que des images officielles | TP 3, parties 1 à 3 |
| Couches, cache, multi-stage, images slim | **Absent** | TP 3, parties 2 et 3 |
| Tags sémantiques, digest, retag, `image prune` | **Absent** | TP 3, partie 6 ; TP 4 ; TP 9 |
| Registry (Docker Hub, GHCR), rate limits | **Absent** | TP 3, partie 6 |
| Réseau bridge par défaut vs user-defined | Implicite (Compose crée les réseaux) | TP 3, partie 0 |
| Ports, `127.0.0.1` vs `0.0.0.0`, BDD non publiée | Fait : MariaDB sans port publié, OpenBao sur `127.0.0.1` | TP 1 |
| Volumes nommés, sauvegarde | Volume `db_data` ; aucune sauvegarde | TP 3, partie 0 ; TP 6, partie 4 |
| Bind mounts | Utilisés pour la config OpenBao | TP 1 |
| Critère « reboot de l'hôte, données présentes » | **Non tenu** : aucune politique `restart` | TP 3, partie 0 |
| Compose : `build:`, `profiles:` | **Absent** | TP 3, partie 7 |
| Utilisateur non-root, `read_only`, capabilities, pas de `--privileged` | **Absent** pour les conteneurs de la stack | TP 3, partie 4 |
| Secrets hors image | **Au-delà du cours** : OpenBao, identifiants dynamiques | TP 1, TP 2 |
| Scan Trivy, SBOM, épinglage par digest, Dependabot/Renovate | **Absent** | TP 3, partie 5 ; TP 4 |
| CI GitHub Actions (build, test, scan, push) | **Absent** | TP 4 |
| Jalon Docker (Compose clonable, Dockerfile durci, CI verte, README) | Partiel | TP 3 + TP 4 |

## Partie Kubernetes (diapositives 34 à 63)

Rien n'existait. Le cours laisse le choix entre kind, k3d et minikube ; on prend **minikube**.

| Thème du cours | Où c'est traité |
|---|---|
| Pourquoi Kubernetes, architecture (control plane, nœuds), namespaces | TP 5, cours |
| Cluster local, `kubectl get nodes` | TP 5, partie 1 |
| `kubectl` : get, describe, logs, exec, apply, explain | TP 5 |
| Pod, ReplicaSet, Deployment, labels, rollback | TP 5, parties 2 à 4 |
| Service (ClusterIP, NodePort, LoadBalancer), Endpoints | TP 5, partie 5 |
| DNS du cluster, debug réseau (`port-forward`, `exec`, `nslookup`) | TP 5, partie 6 |
| Mapping Compose → Kubernetes | TP 6, cours |
| ConfigMap, Secret (et pourquoi ce n'est pas un coffre-fort) | TP 6, partie 2 ; TP 8 |
| PVC, StorageClass, persistance après suppression du Pod, dump/restore | TP 6, parties 3 et 4 |
| Ingress, contrôleur, TLS de lab | TP 6, parties 5 et 6 |
| Probes liveness / readiness / startup | TP 7, partie 1 |
| Troubleshooting (Events, CrashLoopBackOff, ImagePullBackOff, Pending) | TP 5, partie 7 ; TP 7, partie 1 |
| Requests, limits, metrics-server | TP 7, partie 2 |
| HPA sous charge | TP 7, partie 3 |
| RollingUpdate, Recreate, blue-green, canary | TP 7, partie 4 |
| SecurityContext, ServiceAccount, RBAC, NetworkPolicy | TP 7, parties 5 à 7 |
| CI vers le cluster | TP 9, partie 7 |
| Démo de soutenance (Compose + cluster + sécu + CI) | TP 9, bilan |

## Ce qui a changé depuis l'écriture du cours

Deux points du support ne sont plus exacts en octobre 2026 :

- **ingress-nginx**, cité en exemple de contrôleur Ingress, n'est plus maintenu depuis mars 2026 ([annonce Kubernetes](https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/)). L'addon `ingress` de minikube l'utilise encore, ce qui suffit pour apprendre l'objet Ingress. Le TP 6 montre ensuite Gateway API, son successeur, avec Envoy Gateway ; on réutilise ces mêmes manifests en production au TP 9.
- **Les tags d'actions GitHub ne sont pas fiables** : en mars 2026, les tags de `aquasecurity/trivy-action` ont été redirigés vers du code malveillant ([CVE-2026-33634](https://nvd.nist.gov/vuln/detail/CVE-2026-33634)). Le TP 4 épingle chaque action par SHA de commit.

## Production

Le cours s'arrête au cluster de lab et mentionne les « écarts cloud ». Le TP 9 déploie la même stack sur Azure (compte Azure for Students) : AKS, Azure Container Registry, Azure Database for MySQL, Key Vault, Gateway API avec certificat Let's Encrypt, déploiement depuis GitHub Actions sans mot de passe.
