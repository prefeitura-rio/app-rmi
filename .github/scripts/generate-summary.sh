#!/bin/bash
set -euo pipefail

NAMESPACE="${1:-k6-operator-system}"
RMI_URL="${2:-}"
API_BUSCA_SEARCH_URL="${3:-}"
TEST_RUN_NAME="${4:-}"

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
  echo "- **RMI API Target**: $RMI_URL"
  echo "- **Search API Target**: $API_BUSCA_SEARCH_URL"
  echo "- **Namespace**: $NAMESPACE"
  echo "- **Status**: $STATUS"
  echo ""
  echo "### Services Tested"
  echo "- **RMI Service**: Authentication, citizen management, wallet operations"
  echo "- **Search Service**: Category browsing, search functionality, popular services"
  echo ""

  if [[ "$STATUS" == "finished" ]]; then
    echo "✅ **Test completed successfully**"
    echo ""
    echo "### User Journey Scenarios Tested"
    echo "- **First-Time Onboarding** (10% of load): Complete user setup with personal info validation"
    echo "- **Home Dashboard Access** (25% of load): Fast access to wallet and maintenance requests"
    echo "- **Personal Info Updates** (20% of load): Phone, email, address, and preference management"
    echo "- **Wallet Card Interactions** (20% of load): Health, Education, and Social Assistance cards"
    echo "- **Category Browsing** (10% of load): Service discovery through category navigation"
    echo "- **Search Experience** (10% of load): Content discovery through search functionality"
    echo "- **Popular Services** (5% of load): Access to trending municipal services"
  else
    echo "❌ **Test failed or timed out**"
  fi

  echo ""
  echo "### Performance Thresholds"
  echo "- **95th percentile response time**: < 5000ms"
  echo "- **HTTP failure rate**: < 5%"
  echo "- **Custom failure rate**: < 5%"
  echo ""
  echo "### Logs"
  echo '```'
  kubectl logs -l k6_cr="$TEST_RUN_NAME" -n "$NAMESPACE" --tail=50 2>/dev/null || echo "❌ Could not retrieve logs"
  echo '```'
} >> "$GITHUB_STEP_SUMMARY"

echo "✅ Test summary generated successfully"
