#!/bin/bash
set -euo pipefail

export NAMESPACE="$1"
export KEYCLOAK_CLIENT_ID="$2"
export KEYCLOAK_CLIENT_SECRET="$3"
export KEYCLOAK_ISSUER="$4"
export BASE_URL_RMI="$5"
export BASE_URL_BUSCA="$6"
export USER="$7"
export TEST_SCENARIO="$8"
export DURATION="$9"
export VIRTUAL_USERS="${10}"
export RAMP_UP_DURATION="${11:-}"
export STEADY_DURATION="${12:-}"
export RAMP_DOWN_DURATION="${13:-}"

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
         .spec.runner.env[3].value = strenv(BASE_URL_RMI) |
         .spec.runner.env[4].value = strenv(BASE_URL_BUSCA) |
         .spec.runner.env[5].value = (env(USER) | tostring) |
         .spec.runner.env[6].value = strenv(TEST_SCENARIO) |
         .spec.runner.env[7].value = strenv(DURATION) |
         .spec.runner.env[8].value = (env(VIRTUAL_USERS) | tostring) |
         .spec.runner.env[9].value = strenv(RAMP_UP_DURATION) |
         .spec.runner.env[10].value = strenv(STEADY_DURATION) |
         .spec.runner.env[11].value = strenv(RAMP_DOWN_DURATION)' "$TEMPLATE_FILE" | \
    kubectl apply -f -

echo "TEST_RUN_NAME=$TEST_RUN_NAME" >> "$GITHUB_ENV"

echo "✅ k6 TestRun resource created successfully: $TEST_RUN_NAME"
