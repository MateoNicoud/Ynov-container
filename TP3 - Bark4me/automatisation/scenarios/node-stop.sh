#!/usr/bin/env bash
# Scénario 2 : arrêt brutal d'un nœud (celui de Postgres par défaut, ou NODE=k8s-2) via az CLI, puis redémarrage.
# Le nom de la VM Azure est celui du nœud (k8s-1..3, cf. tofu/ressources.tf) ; resource group dans AZ_RESOURCE_GROUP.
# Avec 3 control-planes, l'arrêt d'un seul garde le quorum etcd. La recréation de Postgres prend plusieurs minutes.
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$(dirname "$0")/lib.sh"
init
command -v az >/dev/null || fail "az CLI absent"

NODE="${NODE:-$(node_of_postgres)}"
[ -n "$NODE" ] || fail "nœud introuvable (Postgres non planifié ?)"
VM="${VM:-$NODE}"

wait_site 60
probe_start
on_cleanup "az vm start -g $AZ_RESOURCE_GROUP -n $VM -o none"

log "arrêt de la VM $VM"
az vm stop -g "$AZ_RESOURCE_GROUP" -n "$VM" --skip-shutdown -o none || fail "arrêt de $VM en échec"

wait_until 300 "$NODE NotReady" bash -c "[ \"\$(kubectl get node $NODE -o jsonpath='{.status.conditions[?(@.type==\"Ready\")].status}')\" != True ]"
wait_until "$RECOVERY_TIMEOUT" "${PG_STATEFULSET}-0 Ready hors de $NODE" bash -c "
  n=\$(kubectl -n $APP_NAMESPACE get pod ${PG_STATEFULSET}-0 -o jsonpath='{.spec.nodeName}')
  [ \"\$n\" != $NODE ] && kubectl -n $APP_NAMESPACE wait --for=condition=Ready pod/${PG_STATEFULSET}-0 --timeout=5s"

log "redémarrage de la VM $VM"
az vm start -g "$AZ_RESOURCE_GROUP" -n "$VM" -o none || fail "redémarrage de $VM en échec"
cleanup_clear
wait_until "$RECOVERY_TIMEOUT" "$NODE Ready" kubectl wait --for=condition=Ready "node/$NODE" --timeout=5s

probe_stop
verify_recovery
log "scénario node-stop : OK"
