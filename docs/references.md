# Références

Liens vérifiés le 14 octobre 2026. Les TP 1 à 8 ont été testés avec `openbao/openbao:2.7.1`, `hashicorp/vault:2.1.2`, Docker 29.8, Docker Compose 5.5, Go 1.27.2, Trivy 0.74, minikube 1.39.0 (Kubernetes 1.37.0), Helm 4.3.0, Envoy Gateway 1.9.2 et cert-manager 1.21.2. Le TP 9 (Azure) a été validé hors d'Azure : rendu Kustomize et validation par l'API server, sans déploiement réel.

## Docker Compose

- [Référence des services](https://docs.docker.com/reference/compose-file/services/) : `depends_on` et ses conditions (`service_healthy`, `service_completed_successfully`), `healthcheck`, `cap_add`, `user`, `entrypoint`.
- [Référence des réseaux](https://docs.docker.com/reference/compose-file/networks/) : l'option `internal`.
- [Fusion de plusieurs fichiers Compose](https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/) : règles de fusion et balises `!reset` / `!override`.
- [Variables d'environnement prédéfinies](https://docs.docker.com/compose/how-tos/environment-variables/envvars/) : `COMPOSE_FILE`, `COMPOSE_PATH_SEPARATOR`, `COMPOSE_PROJECT_NAME`.
- [Secrets dans Compose](https://docs.docker.com/compose/how-tos/use-secrets/) : l'alternative native, à comparer avec la partie 6 du TP 1.

## Images utilisées

- [mariadb sur Docker Hub](https://hub.docker.com/_/mariadb) : variables `MARIADB_*`, variantes `_FILE`, script `healthcheck.sh`.
- [wordpress sur Docker Hub](https://hub.docker.com/_/wordpress) : variables `WORDPRESS_DB_*` et `_FILE`.
- [openbao/openbao sur Docker Hub](https://hub.docker.com/r/openbao/openbao) et son [script d'entrée](https://github.com/openbao/openbao/blob/main/.release/docker/docker-entrypoint.sh) (variables `BAO_DEV_*`, répertoires `/openbao/config`, `/openbao/file`, `/openbao/logs`).
- [hashicorp/vault sur Docker Hub](https://hub.docker.com/r/hashicorp/vault) : variables `VAULT_DEV_*`, répertoires `/vault/config`, `/vault/file`, `/vault/logs`.

## OpenBao

- [Documentation](https://openbao.org/docs/)
- [Dépôt GitHub](https://github.com/openbao/openbao) : code, notes de version, discussions.
- [Mode dev](https://openbao.org/docs/concepts/dev-server/)
- [Scellement et descellement](https://openbao.org/docs/concepts/seal/) puis [configuration `seal`](https://openbao.org/docs/configuration/seal/) pour l'auto-unseal.
- [Stockage Raft](https://openbao.org/docs/configuration/storage/raft/)
- [Policies](https://openbao.org/docs/concepts/policies/)
- [Moteur KV v2](https://openbao.org/docs/secrets/kv/kv-v2/)
- [Authentification AppRole](https://openbao.org/docs/auth/approle/)
- [Moteur database pour MySQL/MariaDB](https://openbao.org/docs/secrets/databases/mysql-maria/)
- [Référence de la CLI `bao`](https://openbao.org/docs/commands/)

## HashiCorp Vault

- [Documentation](https://developer.hashicorp.com/vault/docs)
- [Tutoriels de prise en main](https://developer.hashicorp.com/vault/tutorials/get-started) : parcours guidé officiel, utile en complément du TP 2.
- [Mode dev](https://developer.hashicorp.com/vault/docs/concepts/dev-server)
- [Moteur KV v2](https://developer.hashicorp.com/vault/docs/secrets/kv/kv-v2)
- [Moteur database pour MySQL/MariaDB](https://developer.hashicorp.com/vault/docs/secrets/databases/mysql-maria) : paramètres de connexion, choix du plugin, `rotate-root`.
- [Leases, renouvellement et révocation](https://developer.hashicorp.com/vault/docs/concepts/lease)
- [Device d'audit `file`](https://developer.hashicorp.com/vault/docs/audit/file)
- [Moteur transit](https://developer.hashicorp.com/vault/docs/secrets/transit)
- [Vault Agent et ses templates](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent/template) : piste pour la question 2 du bilan.

## Licence et histoire du fork

- [HashiCorp adopte la Business Source License](https://www.hashicorp.com/blog/hashicorp-adopts-business-source-license) (annonce d'août 2023).
- [FAQ de la licence BSL de HashiCorp](https://www.hashicorp.com/license-faq)
- [Licence MPL 2.0 d'OpenBao](https://github.com/openbao/openbao/blob/main/LICENSE)

## Image et chaîne d'approvisionnement (TP 3 et 4)

- [Bonnes pratiques Dockerfile](https://docs.docker.com/build/building/best-practices/) : cache, multi-stage, `.dockerignore`.
- [Images distroless](https://github.com/GoogleContainerTools/distroless) : variantes `static`, `base`, `nonroot`.
- [Capabilities Linux](https://man7.org/linux/man-pages/man7/capabilities.7.html) et [options de sécurité de Docker](https://docs.docker.com/engine/security/).
- [Trivy](https://trivy.dev/) et [Syft](https://github.com/anchore/syft).
- [Limites de téléchargement Docker Hub](https://docs.docker.com/docker-hub/usage/).
- [GitHub Container Registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).
- [Durcissement de la sécurité de GitHub Actions](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions) : épinglage par SHA, permissions du jeton.
- [CVE-2026-33634](https://nvd.nist.gov/vuln/detail/CVE-2026-33634) : détournement des tags de `trivy-action` en mars 2026.
- [Dependabot](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference).

## Kubernetes (TP 5 à 8)

- [minikube : démarrage](https://minikube.sigs.k8s.io/docs/start/), [NetworkPolicy avec Calico](https://minikube.sigs.k8s.io/docs/handbook/network_policy/), [accès aux services](https://minikube.sigs.k8s.io/docs/handbook/accessing/).
- [Concepts Kubernetes](https://kubernetes.io/docs/concepts/) : Pods, Deployments, Services, stockage, configuration.
- [Sondes liveness, readiness, startup](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/).
- [Ressources et QoS](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/), [HPA](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/).
- [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/), [RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/), [NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/).
- [Kustomize](https://kubectl.docs.kubernetes.io/references/kustomize/).
- [Fin d'ingress-nginx](https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/) et [Gateway API](https://gateway-api.sigs.k8s.io/).
- [Envoy Gateway](https://gateway.envoyproxy.io/docs/), [cert-manager avec Gateway API](https://cert-manager.io/docs/usage/gateway/).
- [Chart Helm OpenBao](https://github.com/openbao/openbao-helm), [authentification Kubernetes d'OpenBao](https://openbao.org/docs/auth/kubernetes/).

## Azure (TP 9)

- [Azure for Students](https://learn.microsoft.com/en-us/azure/education-hub/about-azure-for-students) et [abonnement désactivé](https://learn.microsoft.com/en-us/azure/cost-management-billing/manage/azurestudents-subscription-disabled).
- [AKS : tiers Free et Standard](https://learn.microsoft.com/en-us/azure/aks/free-standard-pricing-tiers), [quotas et tailles de VM](https://learn.microsoft.com/en-us/azure/aks/quotas-skus-regions), [arrêt et démarrage](https://learn.microsoft.com/en-us/azure/aks/start-stop-cluster).
- [ACR et AKS](https://learn.microsoft.com/en-us/azure/aks/cluster-container-registry-integration).
- [Workload identity](https://learn.microsoft.com/en-us/azure/aks/workload-identity-deploy-cluster), [driver CSI Key Vault](https://learn.microsoft.com/en-us/azure/aks/csi-secrets-store-driver).
- [Azure Database for MySQL Flexible Server](https://learn.microsoft.com/en-us/azure/mysql/flexible-server/quickstart-create-server-cli).
- [Application routing (NGINX, fin de support)](https://learn.microsoft.com/en-us/azure/aks/app-routing) et [sa variante Gateway API](https://learn.microsoft.com/en-us/azure/aks/app-routing-gateway-api).
- [GitHub Actions vers Azure en OIDC](https://learn.microsoft.com/en-us/azure/developer/github/connect-from-azure-openid-connect).
