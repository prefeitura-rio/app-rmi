#!/bin/bash

set -euo pipefail

echo "🔍 Verifying cluster connection..."

KUBE_API_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' | sed -E 's|https?://([^/]+).*|\1|')

if [[ -n "$KUBE_API_SERVER" ]]; then
  echo "🌐 Waiting for Kubernetes API server: $KUBE_API_SERVER"
  uvx wait-for-it --service "https://$KUBE_API_SERVER" --timeout 30 --quiet -- echo "✅ Kubernetes API server is reachable"
fi

echo "ℹ️ Retrieving cluster information..."

kubectl cluster-info

echo "✅ Cluster verification completed successfully"
