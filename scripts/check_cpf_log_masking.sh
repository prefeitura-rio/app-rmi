#!/usr/bin/env sh
# LGPD: CPF must not appear in clear text in zap logs or OTel span attributes.
# Allowed emitters: observability.CPFField / observability.CPFAttr (internal/observability/logging.go).
set -eu

root="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$root"

# Direct zap.String("cpf", ...) / attribute.String("cpf", ...) outside the helper file.
hits="$(
  grep -rn --include='*.go' --exclude='*_test.go' \
    -e 'zap\.String("cpf"' \
    -e 'attribute\.String("cpf"' \
    internal \
    | grep -v 'internal/observability/logging.go' \
    || true
)"

if [ -n "$hits" ]; then
  echo "LGPD violation: unmasked CPF in logs/spans."
  echo "Use observability.CPFField(cpf) or observability.CPFAttr(cpf) instead."
  echo
  echo "$hits"
  exit 1
fi

# Acceptance pattern: raw variable named cpf without MaskCPF/CPFField/CPFAttr.
hits="$(
  grep -rnE --include='*.go' --exclude='*_test.go' \
    'zap\.String\("cpf", *cpf\)|attribute\.String\("cpf", *cpf\)' \
    internal \
    || true
)"

if [ -n "$hits" ]; then
  echo "LGPD violation: zap/attribute String(\"cpf\", cpf) without masking."
  echo "$hits"
  exit 1
fi

echo "OK: CPF log/span masking check passed."
