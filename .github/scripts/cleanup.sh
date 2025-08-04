#!/bin/bash

set -euo pipefail

NAMESPACE="${1:-k6-operator-system}"

echo "🧹 Cleaning up test resources in namespace: $NAMESPACE"

echo "🗑️ Cleaning up old test runs (keeping 5 most recent)..."

kubectl get testruns -n "$NAMESPACE" --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[*].metadata.name}' | \
    tr ' ' '\n' | \
    head -n -5 | \
    xargs -r kubectl delete testrun -n "$NAMESPACE" || true

echo "📦 Cleaning up test script ConfigMap..."

kubectl delete configmap k6-test-script -n "$NAMESPACE" --ignore-not-found=true

echo "✅ Cleanup completed successfully"
