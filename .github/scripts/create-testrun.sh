#!/bin/bash
set -euo pipefail

export NAMESPACE="${1:-k6-operator-system}"
export KEYCLOAK_CLIENT_ID="${2:-}"
export KEYCLOAK_CLIENT_SECRET="${3:-}"
export KEYCLOAK_ISSUER="${4:-}"
export BASE_URL="${5:-}"
export TEST_CPF="${6:-}"
export TEST_SCENARIO="${7:-}"
export DURATION="${8:-}"
export VIRTUAL_USERS="${9:-}"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
COMMIT_SHA="${GITHUB_SHA:-$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')}"
export TEST_RUN_NAME="k6-load-test-${TIMESTAMP}-${COMMIT_SHA:0:7}"

echo "🚀 Creating k6 TestRun resource: $TEST_RUN_NAME in namespace: $NAMESPACE"

TEMPLATE_FILE=$(dirname "$0")/templates/testrun.yaml

yq eval '.metadata.name = env(TEST_RUN_NAME) |
         .metadata.namespace = env(NAMESPACE) |
         .spec.runner.env[0].value = env(KEYCLOAK_CLIENT_ID) |
         .spec.runner.env[1].value = env(KEYCLOAK_CLIENT_SECRET) |
         .spec.runner.env[2].value = env(KEYCLOAK_ISSUER) |
         .spec.runner.env[3].value = env(BASE_URL) |
         .spec.runner.env[4].value = (env(TEST_CPF) | tostring) |
         .spec.runner.env[5].value = env(TEST_SCENARIO) |
         .spec.runner.env[6].value = env(DURATION) |
         .spec.runner.env[7].value = (env(VIRTUAL_USERS) | tostring)' "$TEMPLATE_FILE" | \
    kubectl apply -f -

echo "TEST_RUN_NAME=$TEST_RUN_NAME" >> "$GITHUB_ENV"

echo "✅ k6 TestRun resource created successfully: $TEST_RUN_NAME"
