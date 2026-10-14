# Sourcé par l'entrypoint MariaDB, uniquement à la première initialisation.
# Compte utilisé par OpenBao pour créer les utilisateurs dynamiques.
# Son mot de passe initial est celui de root ; OpenBao le change aussitôt (rotate-root).
docker_process_sql --database=mysql <<-EOSQL
	CREATE USER 'bao'@'%' IDENTIFIED BY '$(docker_sql_escape_string_literal "${MARIADB_ROOT_PASSWORD}")';
	GRANT CREATE USER ON *.* TO 'bao'@'%';
	GRANT ALL PRIVILEGES ON \`${MARIADB_DATABASE}\`.* TO 'bao'@'%' WITH GRANT OPTION;
EOSQL
