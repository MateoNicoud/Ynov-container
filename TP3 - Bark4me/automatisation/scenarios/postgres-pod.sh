#!/usr/bin/env bash
# Scénario 4 : suppression du pod Postgres. Le StatefulSet le recrée sur le même volume Longhorn ;
# le site (web, profiles, photos) doit continuer à répondre, et les statistiques de swipes doivent revenir.
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$(dirname "$0")/lib.sh"
init

POD="${PG_STATEFULSET}-0"
STATS_PATH="${STATS_PATH:-/api/swipes/stats}"

wait_site 60
probe_start

log "suppression du pod $POD"
kubectl -n "$APP_NAMESPACE" delete pod "$POD" --wait=false || fail "suppression de $POD en échec"

wait_until "$RECOVERY_TIMEOUT" "$POD recréé et Ready" bash -c "
  kubectl -n $APP_NAMESPACE get pod $POD -o jsonpath='{.metadata.deletionTimestamp}' | grep -q . && exit 1
  kubectl -n $APP_NAMESPACE wait --for=condition=Ready pod/$POD --timeout=5s"
wait_until 120 "données de nouveau servies ($STATS_PATH)" curl -fsS -o /dev/null --max-time 3 "https://$APP_FQDN$STATS_PATH"

probe_stop
verify_recovery
log "scénario postgres-pod : OK"
