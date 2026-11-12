#!/usr/bin/env bash
# Scénario 3 : rebuild d'un volume Longhorn. On supprime un réplica du volume de Postgres ;
# Longhorn doit en reconstruire un autre sans interrompre le site.
# PG_PVC permet de viser un autre PVC que celui monté par postgres-0.
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$(dirname "$0")/lib.sh"
init

LH_NS="${LH_NAMESPACE:-longhorn-system}"
PG_PVC="${PG_PVC:-$(kubectl -n "$APP_NAMESPACE" get pod "${PG_STATEFULSET}-0" -o jsonpath='{.spec.volumes[*].persistentVolumeClaim.claimName}' | awk '{print $1}')}"
[ -n "$PG_PVC" ] || fail "PVC de ${PG_STATEFULSET}-0 introuvable"
VOL="$(kubectl -n "$APP_NAMESPACE" get pvc "$PG_PVC" -o jsonpath='{.spec.volumeName}')"
[ -n "$VOL" ] || fail "volume Longhorn introuvable pour le PVC $PG_PVC"
WANTED="$(kubectl -n "$LH_NS" get volumes.longhorn.io "$VOL" -o jsonpath='{.spec.numberOfReplicas}')"

replicas() { kubectl -n "$LH_NS" get replicas.longhorn.io -l "longhornvolume=$VOL" -o jsonpath='{.items[*].metadata.name}'; }
rebuilt() {
  local n
  n=$(replicas | wc -w)
  [ "$n" -eq "$WANTED" ] || return 1
  [ "$(kubectl -n "$LH_NS" get volumes.longhorn.io "$VOL" -o jsonpath='{.status.robustness}')" = healthy ] || return 1
  # le réplica supprimé ne doit plus exister
  ! replicas | tr ' ' '\n' | grep -qx "$VICTIM"
}

log "volume $VOL (PVC $PG_PVC), $WANTED réplicas attendus"
[ "$(replicas | wc -w)" -eq "$WANTED" ] || fail "volume déjà dégradé : $(replicas)"
VICTIM="$(replicas | awk '{print $1}')"

wait_site 60
probe_start

log "suppression du réplica $VICTIM"
kubectl -n "$LH_NS" delete replicas.longhorn.io "$VICTIM" --wait=false || fail "suppression du réplica en échec"

wait_until "$RECOVERY_TIMEOUT" "volume $VOL de nouveau healthy avec $WANTED réplicas" rebuilt
log "réplicas : $(replicas)"

probe_stop
verify_recovery
log "scénario longhorn-rebuild : OK"
