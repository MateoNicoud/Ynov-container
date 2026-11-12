#!/usr/bin/env bash
# Scénario 1 : drain d'un nœud (celui de Postgres par défaut, ou NODE=k8s-2), puis uncordon.
# Les PodDisruptionBudgets doivent empêcher toute coupure du site.
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$(dirname "$0")/lib.sh"
init

NODE="${NODE:-$(node_of_postgres)}"
[ -n "$NODE" ] || fail "nœud introuvable (Postgres non planifié ?)"
DRAIN_TIMEOUT="${DRAIN_TIMEOUT:-600s}"

wait_site 60
probe_start
on_cleanup "kubectl uncordon $NODE"

log "drain de $NODE"
kubectl drain "$NODE" --ignore-daemonsets --delete-emptydir-data --timeout="$DRAIN_TIMEOUT" || fail "drain de $NODE en échec"

wait_until "$RECOVERY_TIMEOUT" "${PG_STATEFULSET}-0 Ready hors de $NODE" \
  kubectl -n "$APP_NAMESPACE" wait --for=condition=Ready "pod/${PG_STATEFULSET}-0" --timeout=5s
sleep 15

log "uncordon de $NODE"
kubectl uncordon "$NODE" || fail "uncordon de $NODE en échec"
cleanup_clear

probe_stop
verify_recovery
log "scénario drain : OK"
