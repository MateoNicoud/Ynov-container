#!/usr/bin/env bash
URL="${1:-https://tp2-k8s-ynov.spaincentral.cloudapp.azure.com/}"
while true; do
  out=$(curl -s -o /dev/null --max-time 3 -w '%{http_code} %{time_total}s' "$URL" 2>/dev/null || echo "ERR -")
  printf '%s %s\n' "$(date +%T)" "$out"
  sleep 1
done
