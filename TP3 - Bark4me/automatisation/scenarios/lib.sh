#!/usr/bin/env bash
# Bibliothèque commune des scénarios de panne (à sourcer, pas à lancer).
#
# Variables d'environnement (valeur par défaut entre parenthèses) :
#   KUBECONFIG          kubeconfig du poste (automatisation/kubeconfig, écrit par 03-kubeadm.yml)
#   APP_NAMESPACE       namespace de l'application (bark4me)
#   APP_FQDN            FQDN du site (sortie tofu "fqdn", sinon hôte du kubeconfig)
#   APP_DEPLOYMENTS     Deployments à attendre après la panne (web profiles swipes pedigree photos)
#   PG_STATEFULSET      StatefulSet Postgres (postgres)
#   PROBE_PATHS         chemins sondés pendant la panne (/ /api/profiles /photos/chien-01.jpg)
#   PROBE_INTERVAL      secondes entre deux sondes (1)
#   PROBE_TOLERANCE     nombre maximal de sondes ratées consécutives toléré (15) ;
#                       le Load Balancer Azure met quelques secondes à retirer un nœud arrêté
#   AZ_RESOURCE_GROUP   resource group Azure des VM (rg-tp3-bark4me, cf. tofu/ressources.tf)
#   RECOVERY_TIMEOUT    secondes accordées au retour à la normale (900)
#   SKIP_MAKE_CHECK     1 pour ne pas lancer "make check" en fin de scénario

set -uo pipefail

AUTO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export KUBECONFIG="${KUBECONFIG:-$AUTO_DIR/kubeconfig}"
APP_NAMESPACE="${APP_NAMESPACE:-bark4me}"
APP_DEPLOYMENTS="${APP_DEPLOYMENTS:-web profiles swipes pedigree photos}"
PG_STATEFULSET="${PG_STATEFULSET:-postgres}"
PROBE_PATHS="${PROBE_PATHS:-/ /api/profiles /photos/chien-01.jpg}"
PROBE_INTERVAL="${PROBE_INTERVAL:-1}"
PROBE_TOLERANCE="${PROBE_TOLERANCE:-15}"
AZ_RESOURCE_GROUP="${AZ_RESOURCE_GROUP:-rg-tp3-bark4me}"
RECOVERY_TIMEOUT="${RECOVERY_TIMEOUT:-900}"

PROBE_PID=""
PROBE_LOG=""
CLEANUP_CMDS=()

log() { printf '%s \033[1;36m%s\033[0m\n' "$(date +%T)" "$*"; }
fail() { printf '%s \033[1;31mECHEC : %s\033[0m\n' "$(date +%T)" "$*" >&2; exit 1; }

resolve_fqdn() {
  if [ -n "${APP_FQDN:-}" ]; then return; fi
  APP_FQDN="$(tofu -chdir="$AUTO_DIR/../tofu" output -raw fqdn 2>/dev/null)" || APP_FQDN=""
  if [ -z "$APP_FQDN" ]; then
    APP_FQDN="$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' | sed -E 's#https://([^:]+):.*#\1#')"
  fi
  [ -n "$APP_FQDN" ] || fail "FQDN introuvable : définir APP_FQDN"
}

# Les actions de nettoyage (uncordon, redémarrage de VM) s'exécutent quoi qu'il arrive.
on_exit() {
  local rc=$?
  trap - EXIT
  [ -n "$PROBE_PID" ] && kill "$PROBE_PID" 2>/dev/null
  local i
  for ((i = ${#CLEANUP_CMDS[@]} - 1; i >= 0; i--)); do
    log "nettoyage : ${CLEANUP_CMDS[$i]}"
    eval "${CLEANUP_CMDS[$i]}" || true
  done
  [ -n "$PROBE_LOG" ] && rm -f "$PROBE_LOG"
  exit "$rc"
}
trap on_exit EXIT
trap 'exit 130' INT TERM

on_cleanup() { CLEANUP_CMDS+=("$1"); }
cleanup_clear() { CLEANUP_CMDS=(); }

init() {
  command -v kubectl >/dev/null || fail "kubectl absent"
  command -v curl >/dev/null || fail "curl absent"
  [ -f "$KUBECONFIG" ] || fail "kubeconfig introuvable : $KUBECONFIG"
  resolve_fqdn
  log "site https://$APP_FQDN, namespace $APP_NAMESPACE"
}

# Une sonde = tous les chemins répondent 2xx en HTTPS, certificat vérifié.
site_ok() {
  local p
  for p in $PROBE_PATHS; do
    curl -fsS -o /dev/null --max-time 3 "https://$APP_FQDN$p" 2>/dev/null || return 1
  done
}

wait_site() {
  local deadline=$((SECONDS + ${1:-120}))
  until site_ok; do
    [ "$SECONDS" -lt "$deadline" ] || fail "le site ne répond pas"
    sleep 2
  done
}

probe_start() {
  PROBE_LOG="$(mktemp)"
  (
    while true; do
      if site_ok; then echo "$(date +%T) OK"; else echo "$(date +%T) KO"; fi
      sleep "$PROBE_INTERVAL"
    done
  ) >"$PROBE_LOG" &
  PROBE_PID=$!
  log "sonde lancée sur : $PROBE_PATHS"
}

# Arrête la sonde et échoue si la plus longue série de sondes ratées dépasse la tolérance.
probe_stop() {
  [ -n "$PROBE_PID" ] && kill "$PROBE_PID" 2>/dev/null
  wait "$PROBE_PID" 2>/dev/null
  PROBE_PID=""
  local total ko worst
  total=$(wc -l <"$PROBE_LOG")
  ko=$(grep -c ' KO$' "$PROBE_LOG")
  worst=$(awk '/ KO$/ { n++; if (n > m) m = n; next } { n = 0 } END { print m + 0 }' "$PROBE_LOG")
  log "sonde : $total mesures, $ko ratées, plus longue série ratée : $worst (tolérance $PROBE_TOLERANCE)"
  if [ "$worst" -gt "$PROBE_TOLERANCE" ]; then
    grep ' KO$' "$PROBE_LOG" | head -3
    fail "le site a cessé de répondre ($worst sondes consécutives ratées)"
  fi
}

# Attente générique : wait_until <secondes> <description> <commande...>
wait_until() {
  local timeout=$1 what=$2
  shift 2
  local deadline=$((SECONDS + timeout))
  log "attente : $what"
  until "$@" >/dev/null 2>&1; do
    [ "$SECONDS" -lt "$deadline" ] || fail "délai dépassé : $what"
    sleep 5
  done
}

node_of_postgres() {
  kubectl -n "$APP_NAMESPACE" get pod "${PG_STATEFULSET}-0" -o jsonpath='{.spec.nodeName}'
}

# Retour à la normale : nœuds, workloads, site, puis make check.
verify_recovery() {
  log "vérification du retour à la normale"
  kubectl wait --for=condition=Ready nodes --all --timeout="${RECOVERY_TIMEOUT}s" || fail "nœuds non Ready"
  local d
  for d in $APP_DEPLOYMENTS; do
    kubectl -n "$APP_NAMESPACE" rollout status "deployment/$d" --timeout="${RECOVERY_TIMEOUT}s" || fail "deployment/$d non prêt"
  done
  kubectl -n "$APP_NAMESPACE" rollout status "statefulset/$PG_STATEFULSET" --timeout="${RECOVERY_TIMEOUT}s" || fail "statefulset/$PG_STATEFULSET non prêt"
  wait_site 120
  if [ "${SKIP_MAKE_CHECK:-0}" != 1 ]; then
    APP_NAMESPACE="$APP_NAMESPACE" APP_FQDN="$APP_FQDN" make -C "$AUTO_DIR" check || fail "make check en échec"
  fi
  log "retour à la normale confirmé"
}
