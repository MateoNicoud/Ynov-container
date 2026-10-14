ui            = true
disable_mlock = true
api_addr      = "http://openbao:8200"
cluster_addr  = "http://openbao:8201"

storage "raft" {
  path    = "/openbao/file"
  node_id = "bao-1"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = true
}

# TP 2 partie 5 : OpenBao n'accepte plus `bao audit enable`, l'audit se déclare ici.
audit "file" "file" {
  options {
    file_path = "/openbao/logs/audit.log"
  }
}
