#!/usr/bin/env sh
# LGPD: personal data must not appear in clear text in zap logs or OTel span attributes.
#   - CPF: only via observability.CPFField / observability.CPFAttr
#   - phone: only via observability.PhoneField / PhonesField / PhoneAttr (or observability.MaskPhone)
#   - person name: only via observability.NameField (or observability.MaskName)
#   - birth date: never logged nor recorded in spans
#   - cache/data/Redis keys (may carry CPF or phone): only via observability.KeyField /
#     CacheKeyField (or observability.MaskCacheKey / pii.MaskCacheKey)
#   - MongoDB filters: never zap.Any/Reflect("filter", ...) (the value is the CPF or phone)
#   - one-time codes (OTP): never logged nor recorded in spans
#   - error messages: no CPF or phone formatted in clear with %s/%q/%v
#   - request path/query/URL (carry CPF or phone in route params): only via observability.PathField /
#     QueryField (or observability.MaskURLPath / MaskQuery / MaskURL), or the route (c.FullPath())
#   - panic recovery: only middleware.Recovery() (gin.Default() and gin's recovery middlewares dump the raw request to stderr)
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

# 5. Cache/data/Redis keys: key, keys, cache_key, write_key, redis.key, redis.pattern...
cache_key_key='([a-z0-9]+[._])*(key|keys|pattern)"'
hits="$(go_lines "${emitter}${cache_key_key}" | grep -vE '(observability|pii)\.MaskCacheKeys?\(' || true)"
[ -z "$hits" ] || fail "unmasked cache/Redis key in logs/spans (keys carry CPF or phone). Use observability.KeyField / CacheKeyField (or observability.MaskCacheKey)." "$hits"

# 6. MongoDB filters: the filter value is the CPF or the phone. Log only the field name.
hits="$(go_lines 'zap\.(Any|Reflect)\("([a-z0-9]+\.)*filter"')"
[ -z "$hits" ] || fail "MongoDB filter in logs. Log only the filter field name (e.g. zap.String(\"filter_field\", ...))." "$hits"

# 7. One-time codes (OTP): never logged nor recorded in spans, masked or not.
otp_key='([a-z0-9]+\.)*(code|otp|otp_code|verification_code|codigo)"'
hits="$(go_lines "(${emitter}|${span_emitter})${otp_key}")"
[ -z "$hits" ] || fail "one-time code (OTP) in logs/spans. Verification codes must never be logged nor recorded as span attributes." "$hits"

# 8. Error messages with a CPF or phone formatted in clear.
hits="$(go_lines '(Errorf|errors\.New)\("[^"]*([Pp]hone|[Tt]elefone|[Cc]elular|cpf|CPF)[^"]*%[sqv]' | grep -vE 'observability\.Mask(CPF|Phone)\(' || true)"
[ -z "$hits" ] || fail "CPF or phone formatted in clear in an error message (errors end up in logs). Leave the value out or use observability.MaskCPF / MaskPhone." "$hits"

# 9. Request path, query and URL: /v1/citizen/<cpf>/..., /v1/memory/<phone>... carry personal data.
url_masked='observability\.(Mask(URLPath|Query|URL)|PathField|QueryField)\(|FullPath\(\)'
hits="$(go_lines "${emitter}[^\"]*\", *[^)]*(Request\.(URL|RequestURI)|URL\.(Path|RawQuery|RawPath|String\(\)|EscapedPath\(\)))" | grep -vE "$url_masked" || true)"
[ -z "$hits" ] || fail "unmasked request path/query/URL in logs/spans. Use observability.PathField / QueryField (or observability.MaskURLPath / MaskQuery / MaskURL) or the route c.FullPath()." "$hits"

request_key='([a-z0-9]+[._])*(path|query|endpoint|request_uri|http\.url|http\.target)"'
hits="$(go_lines "${emitter}${request_key}" | grep -vE "$url_masked" || true)"
[ -z "$hits" ] || fail "request path/query/URL field without masking. Use observability.PathField / QueryField (or observability.MaskURLPath / MaskQuery / MaskURL) or the route c.FullPath()." "$hits"

# 10. Panic recovery: gin.Default() and gin's recovery middlewares dump the raw request line (path with CPF/phone)
#     and headers to stderr. The router uses middleware.Recovery().
gin_recovery='gin\.(Default|Recovery|RecoveryWithWriter|CustomRecovery|CustomRecoveryWithWriter)\('
hits="$(go_lines "$gin_recovery" | sed 's#//.*$##' | grep -E "$gin_recovery" || true)"
[ -z "$hits" ] || fail "gin.Default() or a gin recovery middleware dumps the raw request (path with CPF/phone) to stderr on panic. Use middleware.Recovery()." "$hits"

if [ "$status" -ne 0 ]; then
  exit "$status"
fi

echo "OK: CPF/phone/name/birth date/cache key/OTP/request path log and span masking check passed."
