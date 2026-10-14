pid_file = "/tmp/agent.pid"

vault {
  address = "http://openbao:8200"
}

auto_auth {
  method "approle" {
    config = {
      role_id_file_path                   = "/bao-agent/role_id"
      secret_id_file_path                 = "/bao-agent/secret_id"
      remove_secret_id_file_after_reading = false
    }
  }
}

# Les deux fichiers lisent le même secret : l'agent ne fait qu'une requête,
# donc l'utilisateur et le mot de passe vont toujours ensemble.
template {
  contents    = "{{ with secret \"database/creds/wordpress\" }}{{ .Data.username }}{{ end }}"
  destination = "/run/bao/db_user"
  perms       = "0444"
}

template {
  contents    = "{{ with secret \"database/creds/wordpress\" }}{{ .Data.password }}{{ end }}"
  destination = "/run/bao/db_password"
  perms       = "0444"
}

# API (TP 3) : compte en lecture seule, dans un volume que WordPress ne voit pas.
template {
  contents    = "{{ with secret \"database/creds/wp-readonly\" }}{{ .Data.username }}{{ end }}"
  destination = "/run/bao-api/db_user"
  perms       = "0444"
}

template {
  contents    = "{{ with secret \"database/creds/wp-readonly\" }}{{ .Data.password }}{{ end }}"
  destination = "/run/bao-api/db_password"
  perms       = "0444"
}
