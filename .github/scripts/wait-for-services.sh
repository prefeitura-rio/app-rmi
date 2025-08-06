#!/bin/bash

set -euo pipefail

NAMESPACE="${1:-k6-operator-system}"
K8S_SVC="${2:-rmi}"

echo "⏳ Waiting for services to be ready..."

echo "🌐 Checking $K8S_SVC service availability via port forward..."

kubectl port-forward -n rmi service/"$K8S_SVC" 8080:80 &

PORT_FORWARD_PID=$!

sleep 5

uvx wait-for-it --service localhost:8080 --timeout 30 --quiet -- echo "✅ $K8S_SVC service is accessible"

kill "$PORT_FORWARD_PID" 2>/dev/null || true

echo "🔍 Checking app-busca-search service availability via port forward..."

kubectl port-forward -n busca service/app-busca-search 8081:8080 &

BUSCA_PORT_FORWARD_PID=$!

sleep 5

uvx wait-for-it --service localhost:8081 --timeout 30 --quiet -- echo "✅ app-busca-search service is accessible"

kill "$BUSCA_PORT_FORWARD_PID" 2>/dev/null || true

echo "🔍 Waiting for k6 operator to be ready..."
kubectl wait --for=condition=available deployment/k6-operator-controller-manager -n "$NAMESPACE" --timeout=60s

echo "✅ All services are ready"
