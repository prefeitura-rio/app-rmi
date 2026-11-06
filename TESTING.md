# Testing Guide

This document explains how to run the comprehensive test suite for the RMI application.

## Test Coverage Progress

**Overall Coverage: 8.6%** (Target: 70%+)

### Current Coverage by Module:
- ✅ **Models**: 37.6% coverage
- ✅ **Utils**: 24.7% coverage
  - Validation: 100% coverage (all 13 functions fully tested)
  - CPF/CNPJ: 91%+ coverage
  - Phone parsing: 89-100% coverage
  - Name extraction/masking: 91-100% coverage
  - UUID/Verification/Bool: 80-100% coverage

## Test Types

### 1. Unit Tests (No Dependencies)
Pure function tests that don't require external services:
- Validation logic
- CPF/CNPJ validation
- Phone number parsing
- Name extraction and masking
- Model validation functions
- JWT claim parsing

### 2. Integration Tests (Require Services)
Tests that interact with real MongoDB and Redis:
- MongoDB utility functions (`internal/utils/mongodb_test.go`)
- Redis pipeline operations (`internal/utils/redis_pipeline_test.go`)

## Running Tests

### Quick Test (Unit Tests Only)
```bash
# Run all unit tests (no external dependencies needed)
just test

# Run with coverage
just test-coverage

# Run with race detection
just test-race
```

### Integration Tests (Requires MongoDB + Redis)

**Step 1: Start Dependencies**
```bash
# Using docker-compose
docker-compose up -d mongodb redis

# OR using Just command
just start-deps
```

**Step 2: Run Integration Tests**
```bash
# Set environment variables
export MONGODB_URI="mongodb://root:password@localhost:27017"
export REDIS_ADDR="localhost:6380"
export REDIS_PASSWORD=""

# Run all tests including integration tests
go test -v ./...

# Run only MongoDB integration tests
go test -v -run TestMongoDB ./internal/utils/

# Run only Redis integration tests
go test -v -run TestRedisPipeline ./internal/utils/
```

### CI-Style Testing (Like GitHub Actions)
```bash
# This sets all required environment variables and runs tests
just test-ci
```

## Test Organization

### Utils Layer (`internal/utils/*_test.go`)
- `cpf_test.go` - CPF and CNPJ validation (23 test cases)
- `phone_parsing_test.go` - Phone number parsing (12 test cases)
- `name_extraction_test.go` - Name extraction and masking (22 test cases)
- `bool_test.go` - Boolean helper functions
- `uuid_test.go` - UUID generation (3 test cases)
- `verification_test.go` - Verification code generation (4 test cases)
- `validation_test.go` - **100% coverage** - Input validation and sanitization (65+ test cases)
- `mongodb_test.go` - MongoDB utility integration tests (11 test scenarios)
- `redis_pipeline_test.go` - Redis pipeline integration tests (6 test scenarios)

### Models Layer (`internal/models/*_test.go`)
- `citizen_test.go` - Citizen model validation and conversion (14 test functions)
- `jwt_test.go` - JWT claims parsing (7 test cases)
- `errors_test.go` - Error constant validation
- `phone_mapping_test.go` - Phone mapping constants

## Writing New Tests

### Test Naming Convention
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name  string
        input YourInput
        want  YourExpected
        wantErr bool
    }{
        {
            name: "Description of test case",
            input: YourInput{...},
            want: YourExpected{...},
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := YourFunction(tt.input)
            // assertions...
        })
    }
}
```

### Integration Test Pattern
```go
func TestYourIntegration(t *testing.T) {
    // Check for required environment variables
    requiredVar := os.Getenv("REQUIRED_VAR")
    if requiredVar == "" {
        t.Skip("Skipping integration test: REQUIRED_VAR not set")
    }

    // Setup
    // ... initialize connections

    // Cleanup
    defer func() {
        // ... cleanup resources
    }()

    // Run tests
    t.Run("Test scenario", func(t *testing.T) {
        // test code
    })
}
```

## Test Principles

### TDD Approach
Following Test-Driven Development principles as outlined in `CLAUDE.md`:

1. **Tests define correct behavior** - Write tests first to specify how code should work
2. **Fix the code, not the test** - If tests fail, the implementation is wrong (not the test)
3. **No false positives** - Tests must have meaningful assertions
4. **No silent failures** - Never use `except: pass` or similar patterns
5. **Complete test setup** - Set up proper fixtures and dependencies

### Test Coverage Goals

According to `UNIT_TEST_PLAN.md`:
- **Utils Layer**: 90%+ coverage (currently 24.7%, needs more integration tests)
- **Models Layer**: 80%+ coverage (currently 37.6%, on track)
- **Services Layer**: 75%+ coverage (pending)
- **Middleware Layer**: 70%+ coverage (pending)
- **Handlers Layer**: 60%+ coverage (pending)
- **Overall**: 70%+ coverage (currently 8.6%)

## Troubleshooting

### MongoDB Connection Issues
```bash
# Check if MongoDB is running
docker ps | grep mongodb

# Check MongoDB logs
docker logs rmi-mongodb

# Test connection
mongosh "mongodb://root:password@localhost:27017"
```

### Redis Connection Issues
```bash
# Check if Redis is running
docker ps | grep redis

# Check Redis logs
docker logs rmi-redis

# Test connection
redis-cli -p 6380 ping
```

### Test Failures
```bash
# Run specific test with verbose output
go test -v -run TestSpecificFunction ./internal/utils/

# Run with race detection
go test -race -v -run TestSpecificFunction ./internal/utils/

# Clean test cache
go clean -testcache
```

## Testes de carga (k6)

### Arquivos

- `k6/load_test.js`: script usado pelo workflow `Load tests` (`.github/workflows/run-load-tests.yaml`).
- `k6/generate_token.js`: obtém um token do Keycloak (client credentials) para rodar o script localmente.
- `scripts/load_test/load-test.js`: script antigo (tokens de arquivo, CSV de CPFs, p95 < 2 s); não é usado pelo workflow.

### Cenários (`TEST_SCENARIO`)

| Cenário | Função (`exec`) | Fluxo | Rotas |
|---|---|---|---|
| `first_login_onboarding` | `firstLoginOnboarding` | primeiro acesso com atualização de dados | `GET/PUT /v1/citizen/{cpf}/firstlogin`, `GET /v1/citizen/{cpf}`, `PUT .../ethnicity`, `PUT .../phone` (só com `ENABLE_PHONE_WRITES=true`), `PUT .../email`, `GET .../wallet`, `GET .../maintenance-request` |
| `home_dashboard` | `homeDashboard` | home em paralelo | `GET .../maintenance-request`, `GET .../wallet`, `GET /v1/citizen/{cpf}` |
| `personal_info_update` | `personalInfoUpdate` | edição de dados pessoais | `GET /v1/citizen/{cpf}`, `PUT .../phone` (só com `ENABLE_PHONE_WRITES=true`), `POST /v1/validate/phone`, `PUT .../email`, `PUT .../address`, `PUT .../optin` |
| `wallet_cards` | `walletCards` | carteira e cartões | `GET .../wallet` (3x), `GET .../maintenance-request` |
| `category_browsing`, `search_experience`, `popular_services` | `categoryBrowsing`, `searchExperience`, `popularServices` | navegação deslogada no app-busca-search | `/api/v1/categorias-relevancia`, `/api/v1/categoria/...`, `/api/v1/busca-hibrida-multi`, `/api/v1/documento/...` |
| `legacy` | `legacy` | `GET` simples na URL base do RMI | — |
| `mixed` (padrão) | uma por cenário | os 7 primeiros juntos, cada um no seu cenário do k6 com a sua função | VUs divididos em 10/25/20/20/10/10/5% (no mínimo 1 por cenário) |

Um `TEST_SCENARIO` fora da lista faz o script falhar no início. Os cenários de escrita alteram os dados autodeclarados do CPF de teste (`USER`): use só a conta de teste de staging. `PUT /v1/citizen/{cpf}/phone` envia um código de verificação real por WhatsApp, então só roda com `ENABLE_PHONE_WRITES=true`; o workflow não passa essa variável, e em staging nenhum fluxo cadastra telefone.

### Como disparar

1. GitHub → Actions → **Load tests** → **Run workflow** (branch `main`), ou `gh workflow run run-load-tests.yaml --ref main`. Não há inputs: tudo vem do environment `k6` do repositório (Settings → Environments → `k6`).
2. Variáveis do environment: `GCP_WIF_PROVIDER`, `GCP_SERVICE_ACCOUNT`, `GCP_PROJECT_ID`, `K6_NAMESPACE`, `K8S_SVC`, `KEYCLOAK_ISSUER`, `KEYCLOAK_CLIENT_ID`, `BASE_URL_RMI`, `BASE_URL_BUSCA`, `USER`, `TEST_SCENARIO`, `DURATION`, `VIRTUAL_USERS` e, opcionais, `RAMP_UP_DURATION`, `STEADY_DURATION`, `RAMP_DOWN_DURATION`; segredo `KEYCLOAK_CLIENT_SECRET`.
3. Execução local: `KEYCLOAK_ISSUER=... KEYCLOAK_CLIENT_ID=... KEYCLOAK_CLIENT_SECRET=... BASE_URL_RMI=https://services.staging.app.dados.rio/rmi BASE_URL_BUSCA=... USER=<CPF de teste> TEST_SCENARIO=home_dashboard VIRTUAL_USERS=10 DURATION=2m k6 run k6/load_test.js`.

### Variáveis do script

| Variável | Uso | Padrão |
|---|---|---|
| `BASE_URL_RMI` | URL base do RMI, sem `/v1` (o script acrescenta); `BASE_URL` vale quando ela não vem | `http://rmi.rmi.svc.cluster.local:80` |
| `BASE_URL_BUSCA` | URL base do app-busca-search | `http://app-busca-search.busca.svc.cluster.local:8080` |
| `VIRTUAL_USERS` | VUs do cenário (no `mixed`, o total dividido pelas proporções) | `10` |
| `DURATION` | duração do patamar estável quando `STEADY_DURATION` não vem | `10m` |
| `RAMP_UP_DURATION`, `STEADY_DURATION`, `RAMP_DOWN_DURATION` | estágios de subida, patamar e descida de cada cenário | `5m`, `DURATION`, `2m` |
| `ENABLE_PHONE_WRITES` | `true` liga o `PUT .../phone` (envio real de código) | desligado |

A duração de cada execução é subida + patamar + descida (17 min com os padrões). O `monitor-test.sh` espera até 30 min: acima disso o workflow para de acompanhar o `TestRun`.

### Ambiente alvo

O workflow autentica no GCP por Workload Identity, pega a credencial do cluster GKE `application` do projeto `GCP_PROJECT_ID` do environment `k6`, espera os pods de `K8S_SVC`, cria o ConfigMap `k6-test-script` com `k6/load_test.js` e um `TestRun` do k6-operator no namespace `K6_NAMESPACE` (`parallelism: 10`, imagem `grafana/k6:2.3.0`, 0,5–1 CPU e 1–2 GiB por runner). O `TestRun` passa aos runners `BASE_URL_RMI`, `BASE_URL_BUSCA`, `DURATION` e os estágios do environment; variável vazia usa o padrão do script. O token é obtido no `setup()` por client credentials no Keycloak; o `cleanup.sh` remove os recursos no fim.

O k6-operator (namespace `k6-operator-system`) é instalado pelo Terraform do `superapp` nos clusters `application` de staging (`rj-superapp-staging`) e de produção (`rj-superapp`): o ambiente testado é o do `GCP_PROJECT_ID`. Antes de disparar, confira que o environment aponta para `rj-superapp-staging`; carga em produção só com combinação prévia.

### Thresholds

| Métrica | Limite |
|---|---|
| `http_req_duration` | p95 < 5.000 ms |
| `http_req_failed` | < 5% |
| `failed_requests`, `http_failures` (falha de rede ou 5xx) | < 5% |
| `check_failures` (status inesperado ou resposta > 5 s) | < 15% |

O resultado sai no resumo do run (configuração e status do `TestRun`) e nos logs dos runners (o sumário de fim de teste de cada um dos 10 runners, sem agregação). As métricas levam a tag `scenario` com o nome do cenário.

## Next Steps

To reach 70%+ coverage, we need to:
1. ✅ Complete MongoDB integration tests
2. ✅ Complete Redis integration tests
3. ⏳ Test middleware functions (with Gin test context)
4. ⏳ Test service layer (with real MongoDB/Redis)
5. ⏳ Test handlers (integration tests with full setup)

See `UNIT_TEST_PLAN.md` for the complete testing roadmap.
