#!/usr/bin/env bash
URL="${1:-https://tp2-k8s-ynov.spaincentral.cloudapp.azure.com/}"
while true; do
  read -r code time < <(curl -s -o /dev/null --max-time 3 -w '%{http_code} %{time_total}' "$URL")
  [ "$code" = 000 ] && code="ERR"
  printf '%s %s %ss\n' "$(date +%T)" "$code" "$time"
  sleep 1
done
