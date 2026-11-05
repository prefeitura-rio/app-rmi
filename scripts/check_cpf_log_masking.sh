#!/usr/bin/env sh
# LGPD: personal data must not appear in clear text in zap logs or OTel span attributes.
#   - CPF: only via observability.CPFField / observability.CPFAttr
#   - phone: only via observability.PhoneField / PhonesField / PhoneAttr (or observability.MaskPhone)
#   - person name: only via observability.NameField (or observability.MaskName)
#   - birth date: never logged nor recorded in spans
# The helpers live in internal/observability/logging.go, the only file exempt from this check.
set -eu

root="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$root"

helper='internal/observability/logging.go'
status=0

# Non-test Go lines under internal/ and cmd/ matching the extended regex $1, outside the helper.
go_lines() {
  grep -rnE --include='*.go' --exclude='*_test.go' "$1" internal cmd 2>/dev/null \
    | grep -v "^$helper:" \
    || true
}

fail() {
  echo "LGPD violation: $1"
  echo
  echo "$2"
  echo
  status=1
}

# zap/attribute constructors that can carry the value itself (zap.Bool("missing_telefone", ...) cannot).
emitter='(zap\.(String|Strings|Stringer|ByteString|Any|Reflect|Time)|attribute\.(String|StringSlice))\("'
# utils.AddSpanAttribute(span, "key", ...)
span_emitter='AddSpanAttribute\([^,]+, *"'

# 1. CPF
hits="$(go_lines 'zap\.String\("cpf"|attribute\.String\("cpf"')"
[ -z "$hits" ] || fail "unmasked CPF in logs/spans. Use observability.CPFField(cpf) or observability.CPFAttr(cpf)." "$hits"

hits="$(go_lines 'zap\.String\("cpf", *cpf\)|attribute\.String\("cpf", *cpf\)')"
[ -z "$hits" ] || fail "zap/attribute String(\"cpf\", cpf) without masking." "$hits"

# 2. Phone: phone, phone_number, phones, telefone, celular, also prefixed
#    ("input.phone", "storage_phone", "full_phone_number").
phone_key='([a-z0-9]+[._])*(phone|phone_number|phones|telefone|celular)"'
hits="$(go_lines "${emitter}${phone_key}" | grep -v 'observability\.MaskPhone(' || true)"
[ -z "$hits" ] || fail "unmasked phone number in logs/spans. Use observability.PhoneField / PhonesField / PhoneAttr (or observability.MaskPhone)." "$hits"

# 3. Person name: name, nome and variants, also with dotted prefixes ("input.name").
#    Names of other things use a specific key (avatar_name, group_name, circuit_breaker...).
name_key='([a-z0-9]+\.)*(name|nome|first_name|full_name|matched_name|nome_exibicao)"'
hits="$(go_lines "${emitter}${name_key}" | grep -v 'observability\.MaskName(' || true)"
[ -z "$hits" ] || fail "unmasked person name in logs/spans. Use observability.NameField(name) (or observability.MaskName)." "$hits"

# 4. Birth date: any key containing birth/nascimento, masked or not.
birth_key='[a-zA-Z0-9_.]*([Bb]irth|[Nn]ascimento)[a-zA-Z0-9_.]*"'
hits="$(go_lines "(${emitter}|${span_emitter})${birth_key}")"
[ -z "$hits" ] || fail "birth date in logs/spans. Birth dates must not be logged nor recorded as span attributes." "$hits"

if [ "$status" -ne 0 ]; then
  exit "$status"
fi

echo "OK: CPF/phone/name/birth date log and span masking check passed."
