# Testes

Documento único de testes do RMI: como rodar, como funciona o gate de cobertura, a cobertura atual por pacote, as lacunas que ainda importam e os testes de carga (k6).

## Como rodar

Os testes de integração usam MongoDB e Redis reais (sem mocks). Suba as dependências antes:

```bash
just start-deps      # MongoDB (27017) e Redis (6379) em containers
just test            # go test -v ./... com MONGODB_DATABASE=rmi_test e integrações externas desligadas
just test-ci         # mesmas variáveis do job "Unit Tests" do CI (go test -v -race -coverprofile=coverage.out ./...)
just test-coverage   # gera coverage.out e abre o relatório HTML
just test-race       # detector de corrida
just lint            # golangci-lint + scripts/check_cpf_log_masking.sh (máscara de CPF/telefone em logs e spans)
just stop-deps
```

- Sem MongoDB, `internal/handlers`, `internal/services` e `internal/utils` falham no `TestMain` (conexão recusada); os testes de integração dos outros pacotes se pulam quando `MONGODB_URI` não está definida.
- Um pacote ou teste: `go test ./internal/services/ -run TestNome -v`.
- Para medir como o CI: `just test-ci` e `go tool cover -func=coverage.out` (a linha `total` é o número que o gate compara).
- E2E (`tests/e2e`): rodam contra um ambiente implantado com `just test-e2e` e as variáveis `TEST_BASE_URL`, `TEST_KEYCLOAK_URL`, `TEST_KEYCLOAK_REALM`, `TEST_KEYCLOAK_CLIENT_ID`, `TEST_USERNAME`, `TEST_PASSWORD` e `TEST_CPF` (veja `tests/e2e/README.md`). O `deploy-staging.yaml` os roda contra o preview blue-green antes de promover o Rollout; falha aborta a promoção.
- Teste manual da API: `./scripts/test_api.sh <CPF> --auto-token --skip-phone`.

## Gate de cobertura (baseline)

- **Baseline** (`.github/workflows/coverage-baseline.yaml`): a cada push na `main`, roda `go test -race -coverprofile=coverage.out ./...` com MongoDB e Redis como service containers, extrai o total com `scripts/get-coverage.sh` e grava na branch `gh-pages` os arquivos `coverage-baseline.txt` (o número), `coverage-badge.json` (badge do README) e `coverage-updated.txt` (data e commit).
- **Gate** (job `Unit Tests` do `.github/workflows/pr-quality-gate.yaml`, em todo PR para a `main`): roda a mesma suíte (até 3 tentativas), compara o total do PR com o `coverage-baseline.txt` do `gh-pages` e **falha se a cobertura cair mais de 0,1 ponto percentual**. O `coverage.out` do PR fica no artefato `coverage-report` e o resumo do run mostra baseline, PR e diferença.
- Consequência: todo PR precisa manter (ou subir) a cobertura total; código novo sem teste derruba o check.

## Cobertura atual por pacote

Medida em **2026-11-19** no commit `467d362` da `main` (artefato `coverage-report` do job `Unit Tests` do PR #124, mesma árvore do commit), com MongoDB e Redis. **Total: 72,3%** (15.687 statements); o `coverage-baseline.yaml` do mesmo commit registrou 72,0% no `gh-pages` (a suíte com `-race` varia alguns décimos entre execuções).

| Pacote | Statements | Cobertura |
|---|---:|---:|
| `internal/pii` | 57 | 98,2% |
| `internal/observability` | 104 | 95,2% |
| `internal/circuitbreaker` | 95 | 88,4% |
| `internal/models` | 335 | 86,0% |
| `internal/redisclient` | 282 | 84,8% |
| `internal/middleware` | 425 | 84,5% |
| `internal/retry` | 68 | 83,8% |
| `internal/clients` | 397 | 80,4% |
| `internal/services` | 5.805 | 80,2% |
| `internal/logging` | 37 | 75,7% |
| `internal/utils` | 1.140 | 74,8% |
| `internal/handlers` | 5.258 | 67,6% |
| `internal/config` | 1.083 | 66,9% |
| `cmd/sync` | 44 | 36,4% |
| `cmd/api` | 218 | 3,7% (só `registerPhoneLookupRoutes`; remedido em 2026-11-21) |
| `internal/utils/httpclient` | 24 | sem teste |
| `scripts/seed_*`, `tests`, `tests/config`, `tests/fixtures`, `docs` | 319 | não se aplica (scripts, helpers de teste, swagger gerado) |

A linha `cmd/api` foi remedida em **2026-11-21** no commit `cda9c2a` da `main` (`go test -coverprofile=coverage.out ./cmd/api/`, sem dependências), depois de o PR #125 acrescentar `cmd/api/main_test.go`; o total e as outras linhas são da medição de 2026-11-19.

O badge do README mostra o baseline mais recente.

## Lacunas que ainda importam

Arquivos com mais statements sem teste, na mesma medição:

| Arquivo | Cobertura | Por que importa |
|---|---:|---|
| `internal/handlers/memory_handlers.go` | 0% | memória do bot de WhatsApp indexada por telefone (dado pessoal) |
| `internal/handlers/beta_group_handlers.go` | 28,3% | administração de grupos beta (rotas de admin) |
| `internal/utils/whatsapp.go` | 13,3% | envio de mensagens de verificação |
| `internal/utils/audit.go` | 37,4% | trilha de auditoria das escritas |
| `internal/services/pet_service.go` | 37,3% | cadastro de pets |
| `internal/handlers/vehicle.go` | 56,2% | Mobilidade (veículos e condutores) |
| `internal/config/database.go` | 61,1% | conexões, pools e índices do MongoDB |
| `internal/handlers/citizen.go` | 73,9% | maior volume sem teste (436 statements) nas rotas de dados do cidadão |

`cmd/api/main.go` (montagem das rotas e middlewares) só tem teste unitário em `registerPhoneLookupRoutes` (`cmd/api/main_test.go`: JWT exigido nas rotas de consulta de telefone e validação); o resto da montagem é exercitado só pelos E2E do deploy de staging.

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

### Resultado de referência (staging)

Ainda não há: o workflow `Load tests` não tem nenhuma execução registrada no GitHub (conferido em 2026-11-19). A correção do script e do workflow (DEV-366, PR #106) já está na `main`; a primeira execução em staging deve registrar aqui a data, o commit, o cenário, os VUs, a duração, o p95 de `http_req_duration`, as requisições por segundo (`http_reqs`) e a taxa de erro (`http_req_failed`).
