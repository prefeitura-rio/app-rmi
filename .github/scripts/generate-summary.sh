#!/bin/bash
set -euo pipefail

NAMESPACE="${1:-k6-operator-system}"
TARGET_URL="${2:-}"
TEST_RUN_NAME="${3:-}"

echo "📝 Generating test summary..."

if [[ -z "$TEST_RUN_NAME" ]]; then
  TEST_RUN_NAME=$(kubectl get testruns -n "$NAMESPACE" --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')
fi

STATUS=$(kubectl get testrun "$TEST_RUN_NAME" -n "$NAMESPACE" -o jsonpath='{.status.stage}')

{
  echo "## K6 Load Test Results (Kubernetes Operator)"
  echo ""
  echo "### Test Configuration"
  echo "- **Test Run**: $TEST_RUN_NAME"
  echo "- **Target**: $TARGET_URL"
  echo "- **Namespace**: $NAMESPACE"
  echo "- **Status**: $STATUS"
  echo ""

  if [[ "$STATUS" == "finished" ]]; then
    echo "✅ **Test completed successfully**"
  else
    echo "❌ **Test failed or timed out**"
  fi

  echo ""
  echo "### Logs"
  echo '```'
  kubectl logs -l k6_cr="$TEST_RUN_NAME" -n "$NAMESPACE" --tail=50 2>/dev/null || echo "❌ Could not retrieve logs"
  echo '```'
} >> "$GITHUB_STEP_SUMMARY"

echo "✅ Test summary generated successfully"
