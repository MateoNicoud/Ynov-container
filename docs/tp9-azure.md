# TP 9 : mise en production sur Azure

## Point de départ

Tout tourne sur minikube : un nœud, un disque local, des certificats auto-signés, des noms en `.localhost`. Le cours demande de « noter ce qui change hors lab ». Ce TP déploie la même stack sur Azure, avec le compte Azure for Students, et fait la liste de ces changements en les mettant en œuvre.

À la fin du TP :

- l'image de l'API est promue de GHCR vers Azure Container Registry sans être reconstruite ;
- la stack tourne sur AKS, avec une base Azure Database for MySQL managée ;
- les mots de passe sont dans Azure Key Vault, lus par les Pods avec une identité managée, sans aucun mot de passe Azure stocké ;
- le site répond en HTTPS sur un nom public, avec un certificat Let's Encrypt renouvelé automatiquement ;
- un workflow GitHub Actions déploie une version choisie, sans secret Azure dans GitHub ;
- vous savez ce que coûte chaque ressource et comment tout arrêter.

**À lire avant de commencer.** Les manifests de ce TP (`k8s/azure/`) ont été validés hors d'Azure : rendu Kustomize et validation par un API server Kubernetes 1.37. Le déploiement sur un vrai abonnement n'a pas été rejoué au moment de l'écriture, pour ne pas consommer de crédits. Les commandes `az` suivent la documentation Microsoft d'octobre 2026. Si une commande a changé, la documentation liée dans chaque partie fait foi.

### Prérequis

- Les TP 3 à 7. Le workflow `ci` du TP 4 a publié au moins une image sur GHCR.
- Un abonnement Azure for Students actif, et Azure CLI 2.86 ou plus récent (`az version`).
- Helm et kubectl.

---

## Cours

### Le compte Azure for Students

- 100 $ de crédit, valables 12 mois, sans carte bancaire, renouvelables chaque année tant que vous êtes étudiant ([page Microsoft](https://learn.microsoft.com/en-us/azure/education-hub/about-azure-for-students)).
- Quand le crédit est épuisé ou expiré, **l'abonnement est désactivé** et les ressources s'arrêtent ; sans passage à un abonnement payant, elles sont supprimées. Aucune facture surprise, mais aussi aucun moyen de récupérer la base.
- Les abonnements étudiants ont des restrictions : au plus 3 clusters AKS par région, pas d'augmentation de quota possible, et souvent une liste de **régions autorisées** imposée par une Azure Policy (la liste varie d'un compte à l'autre). La partie 0 vérifie tout ça.

### Ce que coûte la stack

Prix catalogue en dollars, Linux, France Central, octobre 2026 ([calculatrice](https://azure.microsoft.com/en-us/pricing/calculator/)). Un mois compte 730 heures.

| Ressource | Choix du TP | Prix | Par mois si allumé en continu |
|---|---|---|---|
| Plan de contrôle AKS | tier **Free** | 0 | 0 (le tier Standard coûte 0,10 $/h, soit 73 $/mois) |
| Nœud AKS | 1 × `Standard_D2as_v5` (2 vCPU, 8 Go) | 0,101 $/h | ~74 $ |
| Disque système du nœud | disque managé | | ~10 à 20 $ |
| IP publiques | sortie du cluster + entrée de la Gateway | 0,005 $/h chacune | ~7 $ |
| Azure Database for MySQL | Burstable `B1ms`, 20 Go | 0,019 $/h + stockage | ~17 $ |
| Azure Container Registry | Basic | 0,167 $/jour | ~5 $ |
| Key Vault | standard | à l'opération | quelques centimes |
| Disque `wp-content` | 1 Gi (taille minimale facturée plus grande) | | ~2 $ |

Allumée en continu, la stack consomme **plus de 100 $ par mois** : le crédit de l'année part en quatre semaines. D'où la règle du TP : **on supprime le cluster à la fin de chaque séance** (partie 8). Une séance de 3 h coûte environ 0,50 $.

Les VM de la série B, moins chères, ne conviennent pas : la documentation AKS indique qu'elles **ne sont pas prises en charge pour le pool système** ([quotas et tailles AKS](https://learn.microsoft.com/en-us/azure/aks/quotas-skus-regions)), le pool qui héberge CoreDNS et les composants du cluster. On prend donc la plus petite VM généraliste à 2 vCPU.

### Les écarts entre le lab et la production

| Sujet | minikube (TP 5 à 8) | Azure (ce TP) | Où |
|---|---|---|---|
| Nœuds | 1 conteneur Docker | 1 VM (on peut en ajouter, autoscaler de nœuds) | partie 2 |
| Registry | `minikube image load` | Azure Container Registry, lu par l'identité du cluster | partie 1 |
| Base de données | StatefulSet MariaDB + PVC local | Azure Database for MySQL : sauvegardes, correctifs, TLS gérés par Azure | partie 3 |
| Mots de passe | Secret Kubernetes créé à la main | Azure Key Vault + driver CSI + workload identity | partie 4 |
| Stockage | `standard` (dossier sur le nœud) | `managed-csi` (disque Azure) | automatique |
| Entrée | port-forward, `.localhost` | IP publique, nom `*.cloudapp.azure.com` | partie 5 |
| TLS | auto-signé | Let's Encrypt | partie 5 |
| NetworkPolicy | Calico | Azure CNI propulsé par Cilium | partie 2 |
| Déploiement | `kubectl apply` depuis le poste | GitHub Actions avec identité fédérée | partie 7 |

Dans le dépôt, ces écarts sont visibles en un coup d'œil : `k8s/base/` est commun, `k8s/minikube/` et `k8s/azure/` ne contiennent que les différences (overlays Kustomize).

### Base managée ou StatefulSet

Faire tourner MariaDB dans le cluster marche (TP 6), mais tout l'opérationnel est pour vous : sauvegardes, tests de restauration, mises à jour de version, réplication, supervision du disque. Un service managé fait ce travail, contre un prix et moins de contrôle. Pour une équipe de développement, la base managée est presque toujours le bon choix en production.

Azure ne propose plus de MariaDB managé : Azure Database for MariaDB a été retiré le 19 septembre 2025, et Microsoft oriente vers **Azure Database for MySQL Flexible Server**. WordPress fonctionne avec MySQL 8. Le serveur impose TLS par défaut (`require_secure_transport`) : WordPress et l'API doivent se connecter en TLS.

### Identités managées et workload identity

Dans Azure, une **identité managée** est un compte de service géré par Azure, sans mot de passe à stocker. Une **identité managée affectée par l'utilisateur** (*user-assigned*) est une ressource à part, à laquelle on donne des rôles (« lire les secrets de ce Key Vault »).

**Workload identity** relie un ServiceAccount Kubernetes à une identité managée, avec le même mécanisme qu'au TP 8 dans l'autre sens :

1. AKS publie un **émetteur OIDC** (*OIDC issuer*) : une URL qui expose les clés publiques avec lesquelles le cluster signe les tokens de ServiceAccount ;
2. sur l'identité managée, un **federated credential** déclare : « je fais confiance aux tokens signés par cet émetteur, pour le sujet `system:serviceaccount:wordpress:api` » ;
3. le Pod présente son token de ServiceAccount à Microsoft Entra ID, qui le vérifie avec les clés publiques de l'émetteur et renvoie un token Azure pour l'identité managée.

Aucun secret n'est échangé ni stocké. GitHub Actions utilise exactement le même mécanisme (partie 7) : GitHub est un émetteur OIDC, et le sujet est `repo:<owner>/<repo>:environment:production`.

### Key Vault et le driver CSI

Le **driver CSI Secrets Store** (add-on AKS `azure-keyvault-secrets-provider`) monte les secrets de Key Vault comme un volume de fichiers. Une **SecretProviderClass** dit quels secrets lire, dans quel coffre, avec quelle identité. Les applications lisent toujours leurs fichiers (`/run/secrets/db/wordpress-password`) : seule la source du volume change, du Secret Kubernetes au driver CSI. Les manifests de la base n'ont presque pas bougé.

### Gateway API sur AKS

AKS propose son propre contrôleur (« application routing »). Sa variante NGINX n'est maintenue que jusqu'en novembre 2026 ; sa variante Gateway API est fondée sur Istio et lance plusieurs réplicas, lourds pour un nœud de 2 vCPU. On installe donc **Envoy Gateway** comme sur minikube : les objets `Gateway` et `HTTPRoute` du TP 6 fonctionnent tels quels. La différence : sur AKS, le Service `LoadBalancer` créé par Envoy Gateway reçoit une vraie IP publique, et l'annotation `service.beta.kubernetes.io/azure-dns-label-name` lui donne un nom `<label>.francecentral.cloudapp.azure.com`.

### Let's Encrypt

Let's Encrypt délivre gratuitement des certificats reconnus par les navigateurs, valables 90 jours, après une preuve que vous contrôlez le nom de domaine. Avec le challenge **HTTP-01**, Let's Encrypt demande un fichier précis sur `http://<nom>/.well-known/acme-challenge/<jeton>` ; cert-manager crée temporairement un Pod et une HTTPRoute qui le servent, puis range le certificat dans un Secret et le renouvelle avant expiration. Le serveur de production limite le nombre de certificats par nom et par semaine : faites vos essais avec le serveur de **staging** (certificat non reconnu, mais limites larges).

---

## Partie 0 : préparer l'abonnement

**Consignes.**

1. Connectez-vous (`az login`), vérifiez l'abonnement actif.
2. Trouvez la liste des régions autorisées pour votre abonnement.
3. Vérifiez le quota de vCPU de la famille `DASv5` dans la région choisie.
4. Dans le portail, *Cost Management > Budgets*, créez un budget de 20 $ par mois avec une alerte e-mail à 50 % et 90 %.
5. Fixez les variables utilisées dans tout le TP.

<details>
<summary>Solution</summary>

```bash
az login
az account show --query "{nom:name, id:id, offre:user.type}" -o table

# Régions autorisées : cherchez une assignation de policy "Allowed locations" ou "Allowed resource deployment regions"
az policy assignment list --query "[].{nom:displayName, params:parameters}" -o json | less

# Quotas : "Standard DASv5 Family vCPUs" et "Total Regional vCPUs" doivent laisser au moins 2 vCPU libres
az vm list-usage --location francecentral -o table | grep -E "DASv5|Total Regional"

# Variables (à recharger dans chaque nouveau terminal)
export LOC=francecentral                 # une région de votre liste autorisée
export RG=rg-tp-wordpress
export SUFFIX=mn42                       # vos initiales + 2 chiffres : les noms doivent être uniques dans Azure
export ACR=acrtp$SUFFIX                  # alphanumérique uniquement
export AKS=aks-tp
export MYSQL=mysql-tp-$SUFFIX
export KV=kv-tp-$SUFFIX
export UAMI=id-tp-workload
export DNS_LABEL=wp-tp-$SUFFIX
```

Le budget ne coupe rien : il prévient. Il complète la règle de suppression à la fin de chaque séance.

</details>

---

## Partie 1 : groupe de ressources et registry

Documentation : [ACR et AKS](https://learn.microsoft.com/en-us/azure/aks/cluster-container-registry-integration).

**Consignes.**

1. Créez le groupe de ressources et un registry ACR Basic.
2. Importez une image publiée par la CI du TP 4 (`ghcr.io/<owner>/tp-api:<tag>`) dans ACR avec `az acr import`, sans la retélécharger sur votre poste. Comparez les digests.

**Questions.**

1. Pourquoi importer plutôt que reconstruire l'image pour Azure ?
2. Pourquoi un groupe de ressources dédié ?

<details>
<summary>Solution</summary>

```bash
az group create -n $RG -l $LOC
az acr create -g $RG -n $ACR --sku Basic

# Si le paquet GHCR est privé, ajoutez --username <login GitHub> --password <token read:packages>
az acr import -n $ACR --source ghcr.io/mateonicoud/tp-api:sha-1a2b3c4 --image tp-api:sha-1a2b3c4

az acr repository show -n $ACR --image tp-api:sha-1a2b3c4 --query digest -o tsv
docker buildx imagetools inspect ghcr.io/mateonicoud/tp-api:sha-1a2b3c4 | grep Digest
# Même digest : c'est la même image, octet pour octet.
```

1. L'image importée est celle qui a été testée et scannée par la CI, avec le même digest. Une reconstruction pourrait embarquer une image de base différente (TP 3, partie 6). C'est la « promotion par retag » du cours, entre deux registries.
2. Tout ce qui est créé dans ce TP est dans `rg-tp-wordpress` (sauf ce qu'AKS crée lui-même, dans un groupe `MC_...` qu'il supprime avec le cluster). `az group delete` efface tout d'un coup : rien n'est oublié, rien ne continue de coûter.

</details>

---

## Partie 2 : le cluster AKS

Documentation : [créer un cluster](https://learn.microsoft.com/en-us/azure/aks/learn/quick-kubernetes-deploy-cli), [workload identity](https://learn.microsoft.com/en-us/azure/aks/workload-identity-deploy-cluster), [driver CSI Key Vault](https://learn.microsoft.com/en-us/azure/aks/csi-secrets-store-driver), [arrêt et démarrage](https://learn.microsoft.com/en-us/azure/aks/start-stop-cluster).

**Consignes.** Créez un cluster qui :

1. est dans le tier Free, avec un seul nœud `Standard_D2as_v5` ;
2. utilise Azure CNI en mode overlay avec le dataplane Cilium (qui applique les NetworkPolicy) ;
3. publie un émetteur OIDC et active workload identity ;
4. active l'add-on Key Vault (driver CSI) ;
5. peut télécharger les images de votre ACR.

Récupérez ensuite les identifiants kubectl, listez les nœuds, les StorageClasses et les Pods de `kube-system`.

**Questions.**

1. Que fait `--attach-acr` exactement ?
2. Qu'est-ce qui est à vous, qu'est-ce qui est à Azure dans un cluster AKS ?

<details>
<summary>Solution</summary>

```bash
# --tier free                       : plan de contrôle gratuit, sans SLA (suffisant pour moins de 10 nœuds)
# --network-dataplane cilium        : Cilium applique les NetworkPolicy du TP 7
# --enable-oidc-issuer / --enable-workload-identity : partie 4
# --enable-addons azure-keyvault-secrets-provider   : driver CSI pour Key Vault
# --attach-acr                      : droit AcrPull sur le registry pour l'identité des nœuds
az aks create -g $RG -n $AKS -l $LOC \
  --tier free \
  --node-count 1 --node-vm-size Standard_D2as_v5 \
  --network-plugin azure --network-plugin-mode overlay --network-dataplane cilium \
  --enable-oidc-issuer --enable-workload-identity \
  --enable-addons azure-keyvault-secrets-provider \
  --attach-acr $ACR \
  --generate-ssh-keys

az aks get-credentials -g $RG -n $AKS       # ajoute un contexte au ~/.kube/config
kubectl config current-context              # aks-tp
kubectl get nodes -o wide
kubectl get storageclass                    # managed-csi (default), azurefile-csi...
kubectl -n kube-system get pods             # coredns, cilium, csi-azuredisk, secrets-store...
```

1. Elle donne le rôle `AcrPull` sur le registry à l'identité managée des nœuds (*kubelet identity*). Les nœuds téléchargent les images privées sans `imagePullSecrets` ni mot de passe.
2. Azure gère le plan de contrôle (API server, etcd, scheduler) : vous ne le voyez pas, vous ne le payez pas en tier Free, vous ne pouvez pas y entrer. Les nœuds sont des VM dans votre abonnement (groupe `MC_rg-tp-wordpress_aks-tp_francecentral`) : vous les payez, et vous êtes responsable de ce qui tourne dessus.

</details>

---

## Partie 3 : la base Azure Database for MySQL

Documentation : [créer un serveur flexible](https://learn.microsoft.com/en-us/azure/mysql/flexible-server/quickstart-create-server-cli).

**Consignes.**

1. Générez un mot de passe administrateur et créez un serveur flexible Burstable `B1ms`, MySQL 8.4, 20 Go, sans aucune IP autorisée au départ. Créez la base `wordpress`.
2. Trouvez l'IP publique de sortie du cluster AKS, et autorisez-la (et elle seule) dans le pare-feu du serveur.
3. Depuis un Pod client `mysql:8.4` dans le cluster, en TLS, créez les utilisateurs `wordpress` (tous les droits sur `wordpress`) et `api` (`SELECT` seulement), avec des mots de passe générés.
4. Gardez les trois mots de passe dans des variables du terminal : ils iront dans Key Vault à la partie 4.

**Questions.**

1. Pourquoi l'IP de sortie du cluster, et pas celle de votre poste ?
2. Quelle amélioration apporterait l'accès privé (intégration à un réseau virtuel) ?

<details>
<summary>Solution</summary>

```bash
export MYSQL_ADMIN_PW=$(openssl rand -base64 24)
export WP_PW=$(openssl rand -base64 24)
export API_PW=$(openssl rand -base64 24)

# --public-access None : accès public activé mais aucune IP autorisée pour l'instant
az mysql flexible-server create -g $RG -n $MYSQL -l $LOC \
  --tier Burstable --sku-name Standard_B1ms --storage-size 20 --version 8.4 \
  --admin-user tpadmin --admin-password "$MYSQL_ADMIN_PW" \
  --public-access None --yes
az mysql flexible-server db create -g $RG -s $MYSQL -d wordpress

# IP par laquelle les Pods sortent vers Internet (load balancer sortant d'AKS)
OUT_IP_ID=$(az aks show -g $RG -n $AKS \
  --query "networkProfile.loadBalancerProfile.effectiveOutboundIPs[0].id" -o tsv)
OUT_IP=$(az network public-ip show --ids "$OUT_IP_ID" --query ipAddress -o tsv)
az mysql flexible-server firewall-rule create -g $RG -n $MYSQL \
  --rule-name aks-sortie --start-ip-address $OUT_IP --end-ip-address $OUT_IP

# Client MySQL jetable dans le cluster, connexion TLS obligatoire
kubectl run mysql-client --rm -i --restart=Never --image=mysql:8.4 -- \
  mysql -h $MYSQL.mysql.database.azure.com -u tpadmin -p"$MYSQL_ADMIN_PW" --ssl-mode=REQUIRED <<SQL
CREATE USER 'wordpress'@'%' IDENTIFIED BY '$WP_PW';
GRANT ALL PRIVILEGES ON wordpress.* TO 'wordpress'@'%';
CREATE USER 'api'@'%' IDENTIFIED BY '$API_PW';
GRANT SELECT ON wordpress.* TO 'api'@'%';
SQL
```

1. Ce sont les Pods qui se connectent à la base, et ils sortent par l'IP du load balancer d'AKS. Votre poste n'a rien à faire sur la base ; si vous en avez besoin, passez par un Pod client comme ici.
2. Le serveur n'aurait plus d'adresse publique du tout : il serait joignable uniquement depuis le réseau virtuel du cluster. Le pare-feu par IP protège bien, mais l'IP de sortie d'AKS est partagée par tous les Pods du cluster, et le serveur reste exposé sur Internet. L'accès privé demande un sous-réseau délégué et une zone DNS privée : plus de travail, à faire pour un vrai service.

</details>

---

## Partie 4 : Key Vault et workload identity

Documentation : [CSI + workload identity](https://learn.microsoft.com/en-us/azure/aks/csi-secrets-store-identity-access), [federated credentials](https://learn.microsoft.com/en-us/azure/aks/workload-identity-deploy-cluster).

**Consignes.**

1. Créez un Key Vault en mode RBAC. Donnez-vous le rôle *Key Vault Secrets Officer* et rangez-y `wordpress-password`, `api-password` et `mysql-admin-password`.
2. Créez une identité managée `id-tp-workload`, donnez-lui *Key Vault Secrets User* sur le coffre.
3. Créez deux federated credentials sur cette identité : un pour le ServiceAccount `wordpress`, un pour `api`, dans le namespace `wordpress`.
4. Remplissez `k8s/azure/params.env` (nom du coffre, tenant, client ID de l'identité, hôte MySQL, label DNS, nom public) et lisez le rendu : `kubectl kustomize k8s/azure`.

**Questions.**

1. L'identité peut lire `mysql-admin-password` alors que personne dans le cluster n'en a besoin. Comment l'éviter ?
2. Le fichier `params.env` est commité. Est-ce un problème ?

<details>
<summary>Solution</summary>

```bash
az keyvault create -g $RG -n $KV -l $LOC --enable-rbac-authorization true
KV_ID=$(az keyvault show -n $KV --query id -o tsv)

# Vous : écrire des secrets
az role assignment create --assignee "$(az ad signed-in-user show --query id -o tsv)" \
  --role "Key Vault Secrets Officer" --scope $KV_ID
sleep 30    # les attributions de rôle mettent un moment à se propager

az keyvault secret set --vault-name $KV -n wordpress-password --value "$WP_PW" -o none
az keyvault secret set --vault-name $KV -n api-password --value "$API_PW" -o none
az keyvault secret set --vault-name $KV -n mysql-admin-password --value "$MYSQL_ADMIN_PW" -o none

# L'identité des Pods : lire les secrets, rien d'autre
az identity create -g $RG -n $UAMI
CLIENT_ID=$(az identity show -g $RG -n $UAMI --query clientId -o tsv)
PRINCIPAL_ID=$(az identity show -g $RG -n $UAMI --query principalId -o tsv)
az role assignment create --assignee-object-id $PRINCIPAL_ID --assignee-principal-type ServicePrincipal \
  --role "Key Vault Secrets User" --scope $KV_ID

# Confiance : les tokens des ServiceAccounts wordpress/wordpress et wordpress/api, signés par ce cluster
ISSUER=$(az aks show -g $RG -n $AKS --query oidcIssuerProfile.issuerUrl -o tsv)
for sa in wordpress api; do
  az identity federated-credential create -g $RG --identity-name $UAMI --name fc-$sa \
    --issuer "$ISSUER" --subject system:serviceaccount:wordpress:$sa \
    --audience api://AzureADTokenExchange
done

# Valeurs pour k8s/azure/params.env
echo "MYSQL_HOST=$MYSQL.mysql.database.azure.com:3306"
echo "KEYVAULT_NAME=$KV"
echo "TENANT_ID=$(az account show --query tenantId -o tsv)"
echo "IDENTITY_CLIENT_ID=$CLIENT_ID"
echo "DNS_LABEL=$DNS_LABEL"
echo "PUBLIC_HOST=$DNS_LABEL.$LOC.cloudapp.azure.com"

kubectl kustomize k8s/azure | less
```

Ce que fait l'overlay `k8s/azure/` (fichiers commentés) :

- `remove-mariadb.yaml` retire le StatefulSet, son Service, son script d'init et sa NetworkPolicy ;
- `secretproviderclass.yaml` décrit deux sources Key Vault, une par application ;
- `wordpress-patch.yaml` et `api-patch.yaml` remplacent le volume `db-secret` (Secret Kubernetes) par un volume CSI, et activent TLS vers MySQL ;
- `identities.yaml` et `api-sa-patch.yaml` annotent les ServiceAccounts avec le client ID de l'identité ;
- les `replacements` de `kustomization.yaml` recopient les valeurs de `params.env` aux bons endroits.

1. Une identité par application, chacune avec un rôle limité au secret dont elle a besoin (le rôle peut être attribué sur un secret précis : `--scope $KV_ID/secrets/api-password`), et le mot de passe administrateur dans un autre coffre ou hors d'Azure. Le TP partage une identité pour garder les commandes courtes.
2. Non : il ne contient que des identifiants publics (noms, ID de tenant, client ID). Un client ID ne donne aucun accès sans un token signé par le cluster, et seul un ServiceAccount listé dans un federated credential peut l'obtenir. C'est la différence avec un mot de passe.

</details>

---

## Partie 5 : entrée publique et TLS

Documentation : [Envoy Gateway](https://gateway.envoyproxy.io/docs/), [cert-manager et Gateway API](https://cert-manager.io/docs/usage/gateway/), [label DNS sur IP publique](https://learn.microsoft.com/en-us/azure/aks/static-ip).

**Consignes.**

1. Installez Envoy Gateway et cert-manager avec Helm, exactement comme au TP 6, partie 7.
2. Créez la GatewayClass `eg` et le ClusterIssuer `letsencrypt` (`k8s/azure/cluster/`), avec votre e-mail et **le serveur de staging** pour commencer.
3. Lisez `k8s/azure/gateway.yaml` : un listener HTTP, un listener HTTPS, une route de redirection, une route qui envoie `/api/` à l'API et le reste à WordPress.
4. Pourquoi faut-il `k8s/azure/networkpolicy-acme.yaml` ?

<details>
<summary>Solution</summary>

```bash
helm install eg oci://docker.io/envoyproxy/gateway-helm --version v1.9.2 \
  -n envoy-gateway-system --create-namespace --wait
helm install cert-manager oci://quay.io/jetstack/charts/cert-manager --version v1.21.2 \
  -n cert-manager --create-namespace -f cm-values.yaml --wait     # cm-values.yaml du TP 6

# Modifier l'e-mail, et l'URL du serveur ACME (staging pour les essais) dans clusterissuer.yaml
kubectl apply -f k8s/azure/cluster/
```

Extrait commenté de `k8s/azure/gateway.yaml` :

```yaml
spec:
  gatewayClassName: eg
  infrastructure:
    annotations:
      # Recopiée par Envoy Gateway sur le Service LoadBalancer : Azure nomme l'IP publique
      # <DNS_LABEL>.<région>.cloudapp.azure.com
      service.beta.kubernetes.io/azure-dns-label-name: DNS_LABEL
  listeners:
    - name: http          # challenge HTTP-01 de Let's Encrypt, et redirection vers HTTPS
      protocol: HTTP
      port: 80
    - name: https
      protocol: HTTPS
      port: 443
      hostname: PUBLIC_HOST
      tls:
        certificateRefs:
          - name: web-tls  # créé et renouvelé par cert-manager (annotation cluster-issuer)
---
# Route principale : /api/ vers l'API, tout le reste vers WordPress.
# /healthz, /readyz et /burn de l'API ne sont pas exposés publiquement.
rules:
  - matches:
      - path: { type: PathPrefix, value: /api/ }
    backendRefs:
      - name: api
        port: 80
  - backendRefs:
      - name: wordpress
        port: 80
```

La route de redirection est attachée au listener `http` (`sectionName: http`) avec un filtre `RequestRedirect` vers `https`. Elle n'empêche pas le challenge ACME : la route que cert-manager crée pour `/.well-known/acme-challenge/<jeton>` est plus précise, et Gateway API donne la priorité à la correspondance de chemin la plus précise.

`networkpolicy-acme.yaml` : pendant le challenge, cert-manager lance dans le namespace `wordpress` un Pod qui sert le jeton sur le port 8089. La policy `default-deny-ingress` du TP 7 bloquerait Envoy, Let's Encrypt n'obtiendrait jamais de réponse, et le certificat resterait en attente.

Si le nom DNS n'apparaît pas sur l'IP publique (`nslookup $DNS_LABEL.$LOC.cloudapp.azure.com`), c'est que l'annotation n'a pas été recopiée sur le Service : vérifiez avec `kubectl -n envoy-gateway-system get svc -o yaml | grep dns-label`. La ressource `EnvoyProxy` d'Envoy Gateway permet aussi de fixer les annotations du Service.

</details>

---

## Partie 6 : déployer

**Consignes.**

1. Déployez l'overlay en remplaçant l'image de l'API par celle d'ACR, **par digest**.
2. Suivez le déploiement : Pods, montage des volumes CSI, certificat, adresse de la Gateway.
3. Ouvrez `https://<PUBLIC_HOST>/`, installez WordPress, publiez un article, appelez `https://<PUBLIC_HOST>/api/posts`.
4. Passez le ClusterIssuer sur le serveur de production de Let's Encrypt, supprimez le Secret `web-tls`, et vérifiez le nouveau certificat.
5. Testez la résilience : supprimez le nœud (`kubectl delete node` ne suffit pas ; utilisez `az aks nodepool scale` à 2, puis drainez l'ancien nœud). Que devient WordPress, avec son volume RWO ?

**Questions.**

1. Un Pod reste en `ContainerCreating` avec l'événement `FailedMount ... secrets-store`. Quelles causes chercher ?
2. Pourquoi `imagePullPolicy: Always` dans l'overlay Azure ?

<details>
<summary>Solution</summary>

```bash
DIGEST=$(az acr repository show -n $ACR --image tp-api:sha-1a2b3c4 --query digest -o tsv)
kubectl kustomize k8s/azure \
  | sed "s|image: tp-api:dev|image: $ACR.azurecr.io/tp-api@$DIGEST|" \
  | kubectl apply -f -

kubectl -n wordpress get pods -w
kubectl -n wordpress describe pod -l app=api | grep -A3 -i "secrets-store\|Events"
kubectl -n wordpress get gateway web          # PROGRAMMED True, ADDRESS = IP publique
kubectl -n wordpress get certificate          # web-tls READY True (après une ou deux minutes)

curl -sI https://$DNS_LABEL.$LOC.cloudapp.azure.com/
curl -s  https://$DNS_LABEL.$LOC.cloudapp.azure.com/api/posts
```

Passage en production de Let's Encrypt :

```bash
# Dans k8s/azure/cluster/clusterissuer.yaml : server: https://acme-v02.api.letsencrypt.org/directory
kubectl apply -f k8s/azure/cluster/clusterissuer.yaml
kubectl -n wordpress delete secret web-tls        # cert-manager redemande un certificat
```

Étape 5 : le nouveau Pod WordPress ne démarre sur l'autre nœud qu'une fois le disque Azure détaché de l'ancien, ce qui prend une à plusieurs minutes. C'est la limite d'un volume RWO (TP 6, partie 5) : pas de haute disponibilité pour WordPress sans stockage partagé.

1. Dans l'ordre : le nom du coffre ou du tenant dans `params.env` ; le client ID de l'identité ; le federated credential (sujet exact `system:serviceaccount:wordpress:<sa>`, émetteur du bon cluster) ; le rôle *Key Vault Secrets User* de l'identité (et le délai de propagation) ; le nom des secrets dans le coffre (`objectName`). Les logs du driver : `kubectl -n kube-system logs -l app=secrets-store-provider-azure`.
2. L'image est référencée par digest : `Always` ne retélécharge que le manifeste, et le nœud garde les couches en cache. C'est surtout une sécurité : le nœud vérifie à chaque démarrage de Pod que l'image existe et qu'il a toujours le droit de la lire.

</details>

---

## Partie 7 : déployer depuis GitHub Actions

Documentation : [connexion OIDC depuis GitHub](https://learn.microsoft.com/en-us/azure/developer/github/connect-from-azure-openid-connect).

**Consignes.**

1. Créez une seconde identité managée `id-github-deploy`, avec un federated credential pour l'environnement GitHub `production` de votre dépôt.
2. Donnez-lui le minimum : importer dans le registry, récupérer les identifiants du cluster.
3. Dans GitHub, créez l'environnement `production` (avec vous comme relecteur obligatoire) et les variables `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `AKS_NAME`, `ACR_NAME`.
4. Lisez `.github/workflows/deploy-azure.yml`, lancez-le avec un tag produit par la CI, validez le déploiement dans l'interface.

**Questions.**

1. Le workflow n'a aucun secret Azure. Qu'est-ce qui empêche un autre dépôt, ou une autre branche, de déployer sur votre cluster ?
2. Le rôle *Azure Kubernetes Service Cluster User Role* donne quels droits dans le cluster ? Est-ce le moindre privilège ?

<details>
<summary>Solution</summary>

```bash
az identity create -g $RG -n id-github-deploy
GH_CLIENT_ID=$(az identity show -g $RG -n id-github-deploy --query clientId -o tsv)
GH_PRINCIPAL=$(az identity show -g $RG -n id-github-deploy --query principalId -o tsv)

# Confiance : uniquement les jobs de l'environnement "production" de ce dépôt
az identity federated-credential create -g $RG --identity-name id-github-deploy --name github-production \
  --issuer https://token.actions.githubusercontent.com \
  --subject repo:MateoNicoud/Ynov-tp-docker2:environment:production \
  --audience api://AzureADTokenExchange

# Droits : gérer le registry (dont l'import), récupérer le kubeconfig du cluster
az role assignment create --assignee-object-id $GH_PRINCIPAL --assignee-principal-type ServicePrincipal \
  --role Contributor --scope "$(az acr show -n $ACR --query id -o tsv)"
az role assignment create --assignee-object-id $GH_PRINCIPAL --assignee-principal-type ServicePrincipal \
  --role "Azure Kubernetes Service Cluster User Role" --scope "$(az aks show -g $RG -n $AKS --query id -o tsv)"

# Variables GitHub (pas des secrets : ce sont des identifiants publics)
gh api -X PUT repos/MateoNicoud/Ynov-tp-docker2/environments/production
for v in "AZURE_CLIENT_ID=$GH_CLIENT_ID" "AZURE_TENANT_ID=$(az account show --query tenantId -o tsv)" \
         "AZURE_SUBSCRIPTION_ID=$(az account show --query id -o tsv)" "AZURE_RESOURCE_GROUP=$RG" \
         "AKS_NAME=$AKS" "ACR_NAME=$ACR"; do
  gh variable set "${v%%=*}" --body "${v#*=}"
done

gh workflow run deploy-azure.yml -f tag=sha-1a2b3c4
```

Les étapes du workflow, commentées dans le fichier : connexion OIDC (`azure/login`), import GHCR vers ACR et lecture du digest, récupération du kubeconfig (`azure/aks-set-context`), rendu de `k8s/azure` avec l'image remplacée par son digest, `kubectl apply`, attente du rollout.

1. Le federated credential n'accepte qu'un sujet exact : `repo:MateoNicoud/Ynov-tp-docker2:environment:production`. Un autre dépôt, ou un job qui ne déclare pas cet environnement, reçoit de GitHub un token avec un autre sujet, refusé par Entra ID. Et l'environnement `production` exige une validation humaine avant que le job démarre.
2. Il permet de télécharger le kubeconfig « utilisateur » du cluster. Sur un cluster sans intégration Entra ID (le cas ici), ce kubeconfig contient un certificat qui a **tous les droits** dans le cluster : ce n'est pas le moindre privilège. Pour mieux faire : activer l'intégration Entra ID et Azure RBAC pour Kubernetes (`--enable-aad --enable-azure-rbac`), puis donner à l'identité de déploiement un rôle limité au namespace `wordpress`. À noter dans le dossier de soutenance comme écart connu.

</details>

---

## Partie 8 : arrêter, redémarrer, tout supprimer

Documentation : [arrêt et démarrage d'AKS](https://learn.microsoft.com/en-us/azure/aks/start-stop-cluster).

**Consignes.**

1. Arrêtez le cluster et le serveur MySQL. Qu'est-ce qui continue d'être facturé ?
2. Redémarrez-les. Le site revient-il seul ?
3. Supprimez tout.

<details>
<summary>Solution</summary>

```bash
# Arrêt : plus de facturation des VM ni du serveur MySQL ; disques, IP publiques,
# stockage MySQL et ACR continuent d'être facturés.
az aks stop -g $RG -n $AKS
az mysql flexible-server stop -g $RG -n $MYSQL
# Attention : un serveur MySQL arrêté redémarre automatiquement au bout de 30 jours.

az aks start -g $RG -n $AKS
az mysql flexible-server start -g $RG -n $MYSQL
kubectl get pods -A

# Fin de séance : tout supprimer (cluster, base, coffre, registry, identités)
az group delete -n $RG --yes --no-wait
# Key Vault est en suppression douce : il garde son nom réservé 90 jours.
az keyvault purge -n $KV        # une fois la suppression du groupe terminée
```

Au redémarrage d'AKS, les Deployments et StatefulSets reviennent, mais pas les Pods créés seuls. L'IP de l'API server peut changer : relancez `az aks get-credentials`. Attendez 15 à 30 minutes entre un arrêt et un démarrage, et sachez que le démarrage peut échouer si la région manque de capacité.

Pour recréer l'environnement à la séance suivante, toutes les commandes de ce TP tiennent dans un script : c'est l'intérêt d'avoir tout fait en ligne de commande plutôt que dans le portail. Écrivez `scripts/azure-up.sh` et `scripts/azure-down.sh`.

</details>

---

## Bilan et soutenance

Le parcours de démonstration du cours : *clone → compose up ; kubectl apply → Ingress joignable ; preuves : probes, HPA, scan, CI ; discours : lab vs prod, limites assumées*. Préparez :

1. **Compose** : `git clone`, `cp .env.example .env`, `docker compose up -d --build`, WordPress et `/api/posts` qui répondent.
2. **Kubernetes** : `kubectl apply -k k8s/minikube` (ou l'URL Azure), un rolling update sans coupure, la montée en charge du HPA, une NetworkPolicy qui bloque un intrus.
3. **Sécurité** : l'image distroless non-root, son scan Trivy, le `securityContext`, le compte `dev` qui ne lit pas les Secrets, les identifiants dynamiques d'OpenBao ou Key Vault.
4. **CI/CD** : un push qui passe la CI, une image sur GHCR, un déploiement Azure validé par un humain.
5. **Les écarts assumés**, chacun avec la raison et ce qu'il faudrait faire :
   - WordPress tourne en root ;
   - un seul réplica de WordPress (volume RWO) ;
   - OpenBao en mode dev au TP 8 ;
   - base MySQL joignable par IP publique ;
   - kubeconfig administrateur pour le déploiement ;
   - un seul nœud, donc pas de haute disponibilité réelle.
