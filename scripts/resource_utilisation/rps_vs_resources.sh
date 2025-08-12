#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="rmi"
LABEL="app=rmi"
PROM_URL="http://localhost:9090"
OUT="rmi_usage_$(date +%Y%m%d_%H%M%S).csv"
INTERVAL=5  # seconds

# 1️⃣ Discover running pod
POD=$(kubectl get pods -n "$NAMESPACE" \
  -l "$LABEL" \
  --field-selector status.phase=Running \
  -o jsonpath='{.items[0].metadata.name}')

if [[ -z "$POD" ]]; then
  echo "❌ No Running pod found with label $LABEL in namespace $NAMESPACE"
  exit 1
fi

# 2️⃣ Initialize CSV
echo "Time,Istio_RPS,CPU_m,Memory" > "$OUT"

# 3️⃣ Loop and capture
while true; do
  TS=$(date +'%Y-%m-%d %H:%M:%S')

  # Istio RPS
  RPS=$(curl -s -G \
    --data-urlencode "query=sum(rate(istio_requests_total{destination_workload=\"rmi\"}[1m]))" \
    "$PROM_URL/api/v1/query" \
    | jq -r '.data.result[0].value[1] // "0"')

  # Pod CPU and Memory
  MET=$(kubectl top pod "$POD" -n "$NAMESPACE" --no-headers)
  CPU=$(awk '{print $2}' <<< "$MET")
  MEM=$(awk '{print $3}' <<< "$MET")

  # Append CSV line
  printf "%s,%s,%s,%s\n" "$TS" "$RPS" "$CPU" "$MEM" >> "$OUT"

  sleep "$INTERVAL"
done
