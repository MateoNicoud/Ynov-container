#!/usr/bin/env bash
set -u
cd "$(dirname "$0")"
export KUBECONFIG="$PWD/kubeconfig"
RG=rg-tp2-k8s
FQDN=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' | sed -E 's#https://([^:]+):.*#\1#')
ETCDCTL="kubectl -n kube-system exec etcd-k8s-1 -- etcdctl --endpoints=https://127.0.0.1:2379 --cacert=/etc/kubernetes/pki/etcd/ca.crt --cert=/etc/kubernetes/pki/etcd/server.crt --key=/etc/kubernetes/pki/etcd/server.key"

title() { printf '\n\033[1;36m=== %s ===\033[0m\n' "$1"; }
pause() { read -rp "$(printf '\033[33m> %s [Entrée]\033[0m ' "$1")" _; }
run() { printf '\n\033[1m$ %s\033[0m\n' "$*"; eval "$*"; }
site() { sleep 5; curl -s "https://$FQDN/" | grep -o '<title>[^<]*' | sed 's/<title>/titre du site : /'; }
stop_vm() { run az vm stop -g $RG -n "$1" --skip-shutdown -o none; }
start_vm() { run az vm start -g $RG -n "$1" -o none; }

step1() {
  title "1. Cluster HA : nœuds, etcd, make check"
  run kubectl get nodes -o wide
  run "kubectl describe nodes | grep Taints"
  run "$ETCDCTL member list -w table"
  run "$ETCDCTL endpoint status --cluster -w table"
  run make check
}

step2() {
  title "2. Entrée : Traefik, cert-manager, certificat Let's Encrypt"
  run kubectl -n traefik get pods,svc -o wide
  run kubectl get clusterissuer
  run kubectl -n wordpress get certificate,ingress
  run "curl -sv -o /dev/null https://$FQDN/ 2>&1 | grep -E 'subject:|issuer:|expire date|verify ok'"
}

step3() {
  title "3. Stockage Longhorn"
  run "ssh -n -p 2201 azureuser@$FQDN 'df -h /var/lib/longhorn; grep longhorn /etc/fstab'"
  run kubectl get sc
  run kubectl get pvc -A
  run kubectl -n longhorn-system get volumes.longhorn.io
}

step4() {
  title "4. WordPress : persistance après suppression du pod"
  site
  run kubectl -n wordpress get pods -o wide
  run kubectl -n wordpress delete pod -l app=wordpress
  run kubectl -n wordpress rollout status deploy/wordpress --timeout=300s
  run kubectl -n wordpress wait --for=condition=Ready pod -l app=wordpress --timeout=300s
  site
}

step5() {
  title "5. Durcissement : probes, SecurityContext, RBAC, NetworkPolicy, HPA"
  run "kubectl -n wordpress describe pod -l app=wordpress | grep -E '^ +(Liveness|Readiness):'"
  run "kubectl -n wordpress describe pod mariadb-0 | grep -E '^ +(Liveness|Readiness):'"
  run kubectl -n wordpress exec mariadb-0 -- id
  run "kubectl -n wordpress get deploy wordpress -o jsonpath='{.spec.template.spec.containers[0].securityContext}'; echo"
  run "kubectl -n wordpress exec deploy/wordpress -- ls /var/run/secrets/kubernetes.io/serviceaccount"
  run kubectl auth can-i list configmaps -n wordpress --as=system:serviceaccount:wordpress:wordpress
  run kubectl auth can-i list secrets -n wordpress --as=system:serviceaccount:wordpress:wordpress
  run kubectl -n wordpress get networkpolicy
  run "kubectl -n wordpress run tiers --image=busybox:1.37 --restart=Never --rm -i -- nc -zv -w3 mariadb-0.mariadb 3306 </dev/null"
  run "kubectl -n wordpress exec deploy/wordpress -- php -r 'echo @fsockopen(\"mariadb-0.mariadb\",3306)?\"wordpress -> mariadb OK\n\":\"KO\n\";'"
  run kubectl top pods -n wordpress
  pause "Lancer la charge HPA"
  for i in 1 2 3; do
    kubectl run charge-$i --image=busybox:1.37 --restart=Never -- sh -c "while true; do wget -q -O /dev/null --header 'Host: $FQDN' http://traefik.traefik.svc.cluster.local/; done" >/dev/null
  done
  for i in $(seq 1 9); do sleep 20; kubectl -n wordpress get hpa wordpress --no-headers; done
  run kubectl -n wordpress get pods -o wide
  run kubectl delete pod charge-1 charge-2 charge-3 --now
}

step6() {
  title "6. Snapshot etcd rapatrié sur le poste"
  run kubectl create namespace marqueur-avant
  run make snapshot
  run ls -lh backups/
  run kubectl delete namespace marqueur-avant
}

step7() {
  title "7. Snapshot et revert Longhorn"
  kubectl -n longhorn-system port-forward svc/longhorn-frontend 8080:80 >/dev/null 2>&1 &
  pf=$!
  echo "UI Longhorn : http://localhost:8080"
  pause "Dans l'UI : volume de wordpress-data, Take Snapshot"
  run "kubectl -n wordpress exec deploy/wordpress -- sh -c 'echo CASSE > /var/www/html/casse.txt; ls -l /var/www/html/casse.txt'"
  run kubectl -n wordpress scale deploy/wordpress --replicas=0
  pause "Dans l'UI : attendre Detached, Attach en maintenance mode, Revert vers le snapshot, Detach"
  run kubectl -n wordpress scale deploy/wordpress --replicas=1
  run kubectl -n wordpress wait --for=condition=Ready pod -l app=wordpress --timeout=300s
  run "kubectl -n wordpress exec deploy/wordpress -- ls -l /var/www/html/casse.txt"
  site
  kill $pf
}

step8() {
  title "8. P3 : arrêt du premier control-plane (k8s-1)"
  echo "Terminal 2 : ./ha-probe.sh   Terminal 3 : watch -n2 'kubectl get nodes; kubectl -n wordpress get pods -o wide'"
  pause "Prédiction annoncée ? Arrêter k8s-1"
  stop_vm k8s-1
  for i in $(seq 1 8); do
    s=$(date +%s.%N); r=$(kubectl get nodes --no-headers --request-timeout=8s 2>&1 | awk '{printf "%s=%s ", $1, $2}')
    printf '%s kubectl en %.1fs : %s\n' "$(date +%T)" "$(echo "$(date +%s.%N) - $s" | bc)" "$r"
    sleep 10
  done
  pause "Rallumer k8s-1"
  start_vm k8s-1
  run kubectl wait --for=condition=Ready node/k8s-1 --timeout=400s
}

step9() {
  title "9. P1 : arrêt du nœud qui porte MariaDB"
  node=$(kubectl -n wordpress get pod mariadb-0 -o jsonpath='{.spec.nodeName}')
  [ "$node" = k8s-1 ] && node=k8s-2
  echo "Nœud arrêté : $node. Suivre ha-probe et le watch, la recréation prend 6 à 9 min."
  pause "Prédiction annoncée ? Arrêter $node"
  stop_vm "$node"
  pause "Site de nouveau en 200 dans ha-probe ? Rallumer $node"
  start_vm "$node"
  run kubectl wait --for=condition=Ready node/"$node" --timeout=400s
  run kubectl -n wordpress get pods -o wide
}

step10() {
  title "10. P5 : perte du quorum (k8s-2 et k8s-3)"
  pause "Prédiction annoncée ? Arrêter k8s-2 et k8s-3"
  run az vm stop -g $RG -n k8s-2 --skip-shutdown --no-wait -o none
  stop_vm k8s-3
  run kubectl get nodes --request-timeout=8s
  run kubectl create namespace p5-test --request-timeout=8s
  run "curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 https://$FQDN/"
  pause "Rallumer k8s-2 (quorum 2/3)"
  start_vm k8s-2
  run kubectl wait --for=condition=Ready node/k8s-2 --timeout=400s
  start_vm k8s-3
  run kubectl wait --for=condition=Ready nodes --all --timeout=400s
  run make check
}

for n in $(seq "${1:-1}" "${2:-10}"); do step$n; pause "Fin de l'étape $n"; done
