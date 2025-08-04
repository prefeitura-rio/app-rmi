#!/bin/bash

set -euo pipefail

NAMESPACE="${1:-k6-operator-system}"

echo "📦 Creating ConfigMap with k6 test script in namespace: $NAMESPACE"

kubectl create configmap k6-test-script \
  --from-file=script.js=k6/load_test.js \
  --namespace="$NAMESPACE" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "✅ ConfigMap k6-test-script created successfully"
