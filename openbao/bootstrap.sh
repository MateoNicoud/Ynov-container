#!/bin/sh
# Amorçage idempotent d'OpenBao, rejoué à chaque `docker compose up`.
#   bootstrap.sh init : init + descellement + config de base (avant MariaDB)
#   bootstrap.sh db   : moteur database (une fois MariaDB démarrée)
#
# /bao-keys contient les clés de descellement et le token root.
# C'est le compromis de ce TP : sans auto-unseal (KMS, HSM), il faut bien
# ranger ces clés quelque part pour démarrer sans intervention humaine.
set -eu

KEYS=/bao-keys

random() { head -c 64 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32; }

wait_for_bao() {
  # bao status : 0 = descellé, 2 = scellé, 1 = injoignable
  i=0
  while :; do
    rc=0; bao status >/dev/null 2>&1 || rc=$?
    [ "$rc" -ne 1 ] && return 0
    i=$((i + 1)); [ "$i" -ge 60 ] && { echo "OpenBao injoignable" >&2; exit 1; }
    sleep 1
  done
}

init() {
  wait_for_bao

  if bao status -format=json | grep -q '"initialized": false'; then
    echo "première initialisation"
    umask 077
    bao operator init -key-shares=3 -key-threshold=2 > "$KEYS/init.txt"
    awk '/^Unseal Key/ {print $4}' "$KEYS/init.txt" > "$KEYS/unseal_keys"
    awk '/^Initial Root Token/ {print $4}' "$KEYS/init.txt" > "$KEYS/root_token"
    rm "$KEYS/init.txt"
  fi

  if bao status -format=json | grep -q '"sealed": true'; then
    head -n 2 "$KEYS/unseal_keys" | while read -r key; do
      bao operator unseal "$key" >/dev/null
    done
    echo "descellé"
  fi

  export BAO_TOKEN="$(cat "$KEYS/root_token")"

  # TP 1 partie 2 / TP 2 parties 2 et 6 (l'audit est dans config.hcl)
  bao secrets list -format=json | grep -q '"secret/"' || bao secrets enable -path=secret -version=2 kv
  bao secrets list -format=json | grep -q '"database/"' || bao secrets enable database
  bao secrets list -format=json | grep -q '"transit/"' || bao secrets enable transit
  bao read transit/keys/wordpress >/dev/null 2>&1 || bao write -f transit/keys/wordpress >/dev/null

  # Mot de passe root MariaDB : généré une seule fois, utilisé au premier démarrage de MariaDB
  bao kv get secret/mariadb/root >/dev/null 2>&1 \
    || bao kv put secret/mariadb/root password="$(random)" >/dev/null
  bao kv get -field=password secret/mariadb/root > /run/mariadb/root_password
  chmod 0444 /run/mariadb/root_password

  # Policies
  # L'agent sert WordPress (droits complets sur la base) et l'API (lecture seule).
  bao policy write wordpress-db - <<'EOF'
path "database/creds/wordpress" {
  capabilities = ["read"]
}
path "database/creds/wp-readonly" {
  capabilities = ["read"]
}
EOF
  bao policy write dev-readonly - <<'EOF'
path "database/creds/wp-readonly" {
  capabilities = ["read"]
}
EOF

  # AppRole de l'agent. token_period : le token se renouvelle tant que l'agent tourne.
  bao auth list -format=json | grep -q '"approle/"' || bao auth enable approle
  bao write auth/approle/role/wordpress \
    token_policies=wordpress-db token_period=1h secret_id_ttl=0 >/dev/null

  bao read -field=role_id auth/approle/role/wordpress/role-id > /bao-agent/role_id
  if [ -s /bao-agent/secret_id ]; then
    bao write auth/approle/role/wordpress/secret-id/destroy \
      secret_id="$(cat /bao-agent/secret_id)" >/dev/null 2>&1 || true
  fi
  bao write -f -field=secret_id auth/approle/role/wordpress/secret-id > /bao-agent/secret_id
  chmod 0400 /bao-agent/role_id /bao-agent/secret_id

  echo "OpenBao prêt"
}

db() {
  wait_for_bao
  export BAO_TOKEN="$(cat "$KEYS/root_token")"

  # Le compte `bao` est créé par MariaDB à sa première init (mariadb-init/01-bao.sh)
  # avec le mot de passe root. OpenBao le remplace aussitôt par un mot de passe
  # que personne ne connaît. Ne surtout pas réécrire la config ensuite.
  if ! bao read database/config/wordpress-db >/dev/null 2>&1; then
    bao write database/config/wordpress-db \
      plugin_name=mysql-database-plugin \
      connection_url='{{username}}:{{password}}@tcp(mariadb:3306)/' \
      allowed_roles='wordpress,wp-readonly' \
      username=bao \
      password="$(bao kv get -field=password secret/mariadb/root)"
    bao write -f database/rotate-root/wordpress-db
    echo "compte bao configuré, mot de passe tourné"
  fi

  # La révocation par défaut lit mysql.user, que le compte bao n'a pas le droit de lire.
  REVOKE="DROP USER IF EXISTS '{{name}}'@'%';"

  bao write database/roles/wordpress \
    db_name=wordpress-db \
    creation_statements="CREATE USER '{{name}}'@'%' IDENTIFIED BY '{{password}}'; GRANT ALL PRIVILEGES ON ${DB_NAME}.* TO '{{name}}'@'%';" \
    revocation_statements="$REVOKE" \
    default_ttl=1h max_ttl=24h >/dev/null

  bao write database/roles/wp-readonly \
    db_name=wordpress-db \
    creation_statements="CREATE USER '{{name}}'@'%' IDENTIFIED BY '{{password}}'; GRANT SELECT ON ${DB_NAME}.* TO '{{name}}'@'%';" \
    revocation_statements="$REVOKE" \
    default_ttl=5m max_ttl=1h >/dev/null

  echo "moteur database prêt"
}

"$@"
