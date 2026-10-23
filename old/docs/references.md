# Références

Liens vérifiés le 14 octobre 2026. Les TP ont été testés avec `openbao/openbao:2.7.1`, `hashicorp/vault:2.1.2`, Docker 29.8 et Docker Compose 5.5.

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
