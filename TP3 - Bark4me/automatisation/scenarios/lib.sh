#!/usr/bin/env bash
# Bibliothèque commune des scénarios de panne (à sourcer, pas à lancer).
#
# Variables d'environnement (valeur par défaut entre parenthèses) :
#   KUBECONFIG          kubeconfig du poste (automatisation/kubeconfig, écrit par 03-kubeadm.yml)
#   APP_NAMESPACE       namespace de l'application (bark4me)
#   APP_FQDN            FQDN du site (sortie tofu "fqdn", sinon hôte du kubeconfig)
#   APP_DEPLOYMENTS     Deployments à attendre après la panne, nommés comme les services, sans préfixe
#                       (web profiles swipes pedigree photos ; cf. app_services dans group_vars/all.yml)
#   PG_STATEFULSET      StatefulSet Postgres (postgres)
#   PROBE_PATHS         chemins sans Postgres, sondés en continu avec la tolérance stricte (/ /photos/chien-01.jpg)
#   PROBE_PATHS_PG      chemins qui dépendent de Postgres (/api/profiles /api/swipes/stats /api/pedigree),
#                       sondés en continu avec PROBE_PG_TOLERANCE
#                       (mêmes chemins que app_https_checks dans group_vars/all.yml)
#   PROBE_INTERVAL      secondes entre deux sondes (1)
#   PROBE_TOLERANCE     nombre maximal de sondes ratées consécutives toléré (15) ;
#                       le Load Balancer Azure met quelques secondes à retirer un nœud arrêté
#   PROBE_PG_TOLERANCE  idem pour les chemins Postgres (240) : une bascule de Postgres après un arrêt de
#                       nœud prend plusieurs minutes. drain.sh la ramène à PROBE_TOLERANCE (pas de coupure)
#   AZ_RESOURCE_GROUP   resource group Azure des VM (rg-tp3-bark4me, cf. tofu/ressources.tf)
#   RECOVERY_TIMEOUT    secondes accordées au retour à la normale (900)
#   SKIP_MAKE_CHECK     1 pour ne pas lancer "make check" en fin de scénario

set -uo pipefail

AUTO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export KUBECONFIG="${KUBECONFIG:-$AUTO_DIR/kubeconfig}"
APP_NAMESPACE="${APP_NAMESPACE:-bark4me}"
APP_DEPLOYMENTS="${APP_DEPLOYMENTS:-web profiles swipes pedigree photos}"
PG_STATEFULSET="${PG_STATEFULSET:-postgres}"
PHOTO_PATH="${PHOTO_PATH:-/photos/chien-01.jpg}"
PROBE_PATHS="${PROBE_PATHS:-/ $PHOTO_PATH}"
PROBE_PATHS_PG="${PROBE_PATHS_PG:-/api/profiles /api/swipes/stats /api/pedigree}"
PROBE_INTERVAL="${PROBE_INTERVAL:-1}"
PROBE_TOLERANCE="${PROBE_TOLERANCE:-15}"
PROBE_PG_TOLERANCE="${PROBE_PG_TOLERANCE:-240}"
AZ_RESOURCE_GROUP="${AZ_RESOURCE_GROUP:-rg-tp3-bark4me}"
RECOVERY_TIMEOUT="${RECOVERY_TIMEOUT:-900}"

PROBE_PID=""
PROBE_DIR=""
PROBE_FILES=()
PROBE_PATH_LIST=()
PROBE_TOLS=()
CLEANUP_CMDS=()

log() { printf '%s \033[1;36m%s\033[0m\n' "$(date +%T)" "$*"; }
fail() { printf '%s \033[1;31mECHEC : %s\033[0m\n' "$(date +%T)" "$*" >&2; exit 1; }
# Affiche une commande telle qu'elle est lancée (présentation : on voit ce qui tourne).
cmd() { printf '%s \033[1;33m$ %s\033[0m\n' "$(date +%T)" "$*"; }
# run <commande...> : affiche la commande, puis l'exécute avec sa sortie visible.
run() { cmd "$*"; "$@"; }
# Indente la sortie d'une commande lancée en arrière-plan ou en boucle.
indent() { sed 's/^/    /'; }

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
  # shellcheck disable=SC2086
  [ -n "$PROBE_PID" ] && kill $PROBE_PID 2>/dev/null
  local i
  for ((i = ${#CLEANUP_CMDS[@]} - 1; i >= 0; i--)); do
    cmd "nettoyage : ${CLEANUP_CMDS[$i]}"
    eval "${CLEANUP_CMDS[$i]}" || true
  done
  [ -n "$PROBE_DIR" ] && rm -rf "$PROBE_DIR"
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
  run kubectl get nodes -o wide
}

# Une sonde = tous les chemins donnés répondent 2xx en HTTPS, certificat vérifié.
paths_ok() {
  local p
  for p in "$@"; do
    curl -fsS -o /dev/null --max-time 3 "https://$APP_FQDN$p" 2>/dev/null || return 1
  done
}

# Tous les chemins, avec ou sans Postgres (état normal, avant et après la panne).
site_ok() {
  # shellcheck disable=SC2086
  paths_ok $PROBE_PATHS $PROBE_PATHS_PG
}

# Commande de sonde, affichée telle quelle.
probe_curl() { curl -fsS -o /dev/null --max-time 3 "https://$APP_FQDN$1"; }

# État du site (code HTTP par chemin), affiché pour la présentation.
site_status() {
  local p
  # shellcheck disable=SC2086
  for p in $PROBE_PATHS $PROBE_PATHS_PG; do
    cmd "curl -sS -o /dev/null -w 'HTTP %{http_code}' --max-time 3 https://$APP_FQDN$p"
    curl -sS -o /dev/null -w '    HTTP %{http_code} en %{time_total}s\n' --max-time 3 "https://$APP_FQDN$p" || true
  done
}

wait_site() {
  local deadline=$((SECONDS + ${1:-120}))
  log "attente : le site répond sur tous les chemins"
  until site_ok; do
    [ "$SECONDS" -lt "$deadline" ] || { site_status; fail "le site ne répond pas"; }
    sleep 2
  done
  site_status
}

# Une sonde par chemin, en continu : chaque chemin a son journal, sa tolérance et ses messages.
# Les changements d'état (OK/KO + erreur curl) sont affichés en direct.
probe_loop() { # <chemin> <journal>
  local state="" now err
  while true; do
    if err=$(probe_curl "$1" 2>&1); then now=OK; else now=KO; fi
    echo "$(date +%T) $now" >>"$2"
    if [ "$now" != "$state" ]; then
      if [ "$now" = OK ]; then
        printf '%s \033[1;32msonde %s : OK\033[0m\n' "$(date +%T)" "$1"
      else
        printf '%s \033[1;31msonde %s : KO (%s)\033[0m\n' "$(date +%T)" "$1" "${err:-sans message}"
      fi
      state=$now
    fi
    sleep "$PROBE_INTERVAL"
  done
}

probe_start() {
  PROBE_DIR="$(mktemp -d)"
  local p i=0 pids=()
  # shellcheck disable=SC2086
  for p in $PROBE_PATHS $PROBE_PATHS_PG; do
    PROBE_PATH_LIST+=("$p")
    PROBE_FILES+=("$PROBE_DIR/$i.log")
    case " $PROBE_PATHS " in *" $p "*) PROBE_TOLS+=("$PROBE_TOLERANCE") ;; *) PROBE_TOLS+=("$PROBE_PG_TOLERANCE") ;; esac
    i=$((i + 1))
  done
  cmd "curl -fsS -o /dev/null --max-time 3 https://$APP_FQDN<chemin>   (1 sonde par chemin, toutes les ${PROBE_INTERVAL}s)"
  for i in "${!PROBE_PATH_LIST[@]}"; do
    probe_loop "${PROBE_PATH_LIST[$i]}" "${PROBE_FILES[$i]}" &
    pids+=($!)
    log "sonde ${PROBE_PATH_LIST[$i]} (tolérance ${PROBE_TOLS[$i]} échecs consécutifs)"
  done
  PROBE_PID="${pids[*]}"
}

# probe_report <journal> <tolérance> <chemin> : affiche le bilan, renvoie 1 si la tolérance est dépassée.
probe_report() {
  local total ko worst
  total=$(wc -l <"$1")
  ko=$(grep -c ' KO$' "$1")
  worst=$(awk '/ KO$/ { n++; if (n > m) m = n; next } { n = 0 } END { print m + 0 }' "$1")
  log "bilan $3 : $total mesures, $ko ratées, plus longue série ratée : $worst (tolérance $2)"
  if [ "$worst" -gt "$2" ]; then
    grep ' KO$' "$1" | head -3 | indent
    return 1
  fi
}

# Arrête les sondes et vérifie la tolérance de chaque chemin.
probe_stop() {
  local pid i bad=()
  for pid in $PROBE_PID; do
    kill "$pid" 2>/dev/null
    wait "$pid" 2>/dev/null
  done
  PROBE_PID=""
  for i in "${!PROBE_PATH_LIST[@]}"; do
    probe_report "${PROBE_FILES[$i]}" "${PROBE_TOLS[$i]}" "${PROBE_PATH_LIST[$i]}" || bad+=("${PROBE_PATH_LIST[$i]}")
  done
  [ "${#bad[@]}" -eq 0 ] || fail "tolérance dépassée sur : ${bad[*]}"
}

# Attente générique : wait_until <secondes> <description> <commande...>
# La commande est affichée une fois ; sa sortie l'est à chaque changement.
wait_until() {
  local timeout=$1 what=$2
  shift 2
  local deadline=$((SECONDS + timeout)) out last="" shown
  log "attente : $what"
  if [ "$1" = bash ] && [ "$2" = -c ]; then shown="$(printf '%s' "$3" | sed -e 's/^[[:space:]]*//' -e '/^$/d' | paste -sd';' - | sed 's/;/; /g')"; else shown="$*"; fi
  cmd "$shown"
  until out="$("$@" 2>&1)"; do
    if [ "$out" != "$last" ]; then
      [ -z "$out" ] || printf '%s\n' "$out" | indent
      last="$out"
    fi
    [ "$SECONDS" -lt "$deadline" ] || fail "délai dépassé : $what"
    sleep 5
  done
  [ -z "$out" ] || [ "$out" = "$last" ] || printf '%s\n' "$out" | indent
}

node_of_postgres() {
  kubectl -n "$APP_NAMESPACE" get pod "${PG_STATEFULSET}-0" -o jsonpath='{.spec.nodeName}'
}

# Tous les pods du cluster Running et prêts.
wait_cluster_ready() {
  wait_until "$RECOVERY_TIMEOUT" "tous les pods du cluster prêts" bash -c "
    bad=\$(kubectl get pods -A --no-headers | awk '\$4 != \"Completed\" { split(\$3, r, \"/\"); if (\$4 != \"Running\" || r[1] != r[2]) print }')
    printf '%s' \"\$bad\"
    [ -z \"\$bad\" ]"
}

# Retour à la normale : nœuds, workloads, site, puis make check.
verify_recovery() {
  log "vérification du retour à la normale"
  run kubectl wait --for=condition=Ready nodes --all --timeout="${RECOVERY_TIMEOUT}s" || fail "nœuds non Ready"
  local d
  for d in $APP_DEPLOYMENTS; do
    run kubectl -n "$APP_NAMESPACE" rollout status "deployment/$d" --timeout="${RECOVERY_TIMEOUT}s" || fail "deployment/$d non prêt"
  done
  run kubectl -n "$APP_NAMESPACE" rollout status "statefulset/$PG_STATEFULSET" --timeout="${RECOVERY_TIMEOUT}s" || fail "statefulset/$PG_STATEFULSET non prêt"
  wait_cluster_ready
  wait_site 120
  run kubectl -n "$APP_NAMESPACE" get pods -o wide
  if [ "${SKIP_MAKE_CHECK:-0}" != 1 ]; then
    cmd "make -C automatisation check"
    APP_NAMESPACE="$APP_NAMESPACE" APP_FQDN="$APP_FQDN" make -C "$AUTO_DIR" check || fail "make check en échec"
  fi
  log "retour à la normale confirmé"
}
