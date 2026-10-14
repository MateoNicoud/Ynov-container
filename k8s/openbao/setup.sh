#!/bin/sh
# TP 8 : configure OpenBao (installé par Helm en mode dev) pour délivrer à l'API
# des identifiants MariaDB dynamiques. À lancer depuis la racine du dépôt.
set -eu

# Raccourci : la CLI bao du Pod openbao-0, avec le token root du mode dev.
bao() { kubectl -n openbao exec -i openbao-0 -- env BAO_TOKEN=root bao "$@"; }

# 1. Compte MariaDB pour OpenBao (droits : créer des utilisateurs, donner SELECT sur la base).
#    Le mot de passe initial est jetable : OpenBao le remplace à l'étape 3.
ROOT_PW=$(kubectl -n wordpress get secret db -o jsonpath='{.data.root-password}' | base64 -d)
BAO_PW=$(openssl rand -hex 16)
kubectl -n wordpress exec -i mariadb-0 -- mariadb -uroot -p"$ROOT_PW" <<SQL
CREATE USER IF NOT EXISTS 'bao'@'%' IDENTIFIED BY '$BAO_PW';
ALTER USER 'bao'@'%' IDENTIFIED BY '$BAO_PW';
GRANT CREATE USER ON *.* TO 'bao'@'%';
GRANT SELECT ON wordpress.* TO 'bao'@'%' WITH GRANT OPTION;
SQL

# 2. Authentification Kubernetes : un Pod prouve son identité avec le token de son ServiceAccount.
#    OpenBao le fait vérifier par l'API Kubernetes (TokenReview).
bao auth list | grep -q '^kubernetes/' || bao auth enable kubernetes
bao write auth/kubernetes/config kubernetes_host=https://kubernetes.default.svc:443

# 3. Moteur database branché sur MariaDB, puis rotation : plus personne ne connaît le mot de passe de bao.
bao secrets list | grep -q '^database/' || bao secrets enable database
bao write database/config/wordpress \
  plugin_name=mysql-database-plugin \
  connection_url='{{username}}:{{password}}@tcp(mariadb.wordpress.svc.cluster.local:3306)/' \
  allowed_roles=api \
  username=bao password="$BAO_PW"
bao write -f database/rotate-root/wordpress

# 4. Rôle "api" : un utilisateur en lecture seule, valable 1 h, 24 h au plus.
bao write database/roles/api \
  db_name=wordpress \
  creation_statements="CREATE USER '{{name}}'@'%' IDENTIFIED BY '{{password}}'; GRANT SELECT ON wordpress.* TO '{{name}}'@'%';" \
  revocation_statements="DROP USER IF EXISTS '{{name}}'@'%';" \
  default_ttl=1h max_ttl=24h

# 5. Policy + rôle Kubernetes : seul le ServiceAccount "api" du namespace "wordpress" obtient cette policy.
bao policy write api - <<'HCL'
path "database/creds/api" {
  capabilities = ["read"]
}
HCL
bao write auth/kubernetes/role/api \
  bound_service_account_names=api \
  bound_service_account_namespaces=wordpress \
  token_policies=api \
  token_ttl=1h

echo "OpenBao configuré"
