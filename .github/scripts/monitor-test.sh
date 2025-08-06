#!/bin/bash
set -euo pipefail

NAMESPACE="${1:-k6-operator-system}"

echo "📊 Monitoring test execution..."

if [[ -n "${TEST_RUN_NAME:-}" ]]; then
  echo "🔍 Using TestRun from environment: $TEST_RUN_NAME"
else
  TEST_RUN_NAME=$(kubectl get testruns -n "$NAMESPACE" --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')
  echo "🔍 Found latest TestRun: $TEST_RUN_NAME"
fi

timeout 1800s bash -c "
  while true; do
    STATUS=\$(kubectl get testrun \$TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.stage}')
    echo \"⏳ Current status: \$STATUS\"
    if [[ \"\$STATUS\" == \"finished\" || \"\$STATUS\" == \"error\" ]]; then
      break
    fi
    sleep 10
  done
" || echo "⏰ Test execution timed out"

echo "📋 Final test status:"

kubectl get testrun "$TEST_RUN_NAME" -n "$NAMESPACE" -o yaml

echo "📄 Test execution logs:"

kubectl logs -l k6_cr="$TEST_RUN_NAME" -n "$NAMESPACE" --tail=100

echo "TEST_RUN_NAME=$TEST_RUN_NAME" >> "$GITHUB_ENV"
