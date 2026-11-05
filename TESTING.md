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

Medida em **2026-11-05** no commit `1a76477` da `main` (run do `coverage-baseline.yaml` e artefato `coverage-report` do mesmo código), com MongoDB e Redis. **Total: 71,9%** (15.432 statements).

| Pacote | Statements | Cobertura |
|---|---:|---:|
| `internal/pii` | 57 | 98,2% |
| `internal/observability` | 96 | 94,8% |
| `internal/circuitbreaker` | 95 | 88,4% |
| `internal/models` | 328 | 86,0% |
| `internal/redisclient` | 282 | 84,8% |
| `internal/retry` | 68 | 83,8% |
| `internal/middleware` | 377 | 82,8% |
| `internal/clients` | 397 | 80,4% |
| `internal/services` | 5.658 | 80,1% |
| `internal/logging` | 37 | 75,7% |
| `internal/utils` | 1.130 | 74,6% |
| `internal/handlers` | 5.247 | 67,2% |
| `internal/config` | 1.080 | 66,9% |
| `cmd/api`, `cmd/sync` | 237 | sem teste (inicialização dos binários) |
| `internal/utils/httpclient` | 24 | sem teste |
| `scripts/seed_*`, `tests`, `tests/config`, `tests/fixtures`, `docs` | 319 | não se aplica (scripts, helpers de teste, swagger gerado) |

O badge do README mostra o baseline mais recente.

## Lacunas que ainda importam

Arquivos com mais statements sem teste, na mesma medição:

| Arquivo | Cobertura | Por que importa |
|---|---:|---|
| `internal/handlers/memory_handlers.go` | 0% | memória do bot de WhatsApp indexada por telefone (dado pessoal) |
| `internal/handlers/beta_group_handlers.go` | 28,3% | administração de grupos beta (rotas de admin) |
| `internal/utils/whatsapp.go` | 13,3% | envio de mensagens de verificação |
| `internal/utils/audit.go` | 37,4% | trilha de auditoria das escritas |
| `internal/services/pet_service.go` | 38,3% | cadastro de pets |
| `internal/handlers/vehicle.go` | 56,2% | Mobilidade (veículos e condutores) |
| `internal/config/database.go` | 61,1% | conexões, pools e índices do MongoDB |
| `internal/handlers/citizen.go` | 72,7% | maior volume sem teste (456 statements) nas rotas de dados do cidadão |

`cmd/api/main.go` (montagem das rotas e middlewares) não tem teste unitário; é exercitado só pelos E2E do deploy de staging.

## Testes de carga (k6)

### Arquivos

- `k6/load_test.js`: script usado pelo workflow `Load tests` (`.github/workflows/run-load-tests.yaml`).
- `k6/generate_token.js`: obtém um token do Keycloak (client credentials) para rodar o script localmente.
- `scripts/load_test/load-test.js`: script antigo (tokens de arquivo, CSV de CPFs, p95 < 2 s); não é usado pelo workflow.

### Cenários (`TEST_SCENARIO`)

| Cenário | Fluxo | Rotas |
|---|---|---|
| `first_login_onboarding` | primeiro acesso com atualização de dados | `GET/PUT /v1/citizen/{cpf}/firstlogin`, `GET /v1/citizen/{cpf}`, `PUT .../ethnicity`, `PUT .../phone`, `PUT .../email`, `GET .../wallet`, `GET .../maintenance-request` |
| `home_dashboard` | home em paralelo | `GET .../maintenance-request`, `GET .../wallet`, `GET /v1/citizen/{cpf}` |
| `personal_info_update` | edição de dados pessoais | `GET /v1/citizen/{cpf}`, `PUT .../phone`, `POST /v1/validate/phone`, `PUT .../email`, `PUT .../address`, `PUT .../optin` |
| `wallet_cards` | carteira e cartões | `GET .../wallet` (3x), `GET .../maintenance-request` |
| `category_browsing`, `search_experience`, `popular_services` | navegação deslogada no app-busca-search | `/api/v1/categorias-relevancia`, `/api/v1/categoria/...`, `/api/v1/busca-hibrida-multi`, `/api/v1/documento/...` |
| `legacy` | `GET` simples na URL base do RMI | — |
| `mixed` (padrão) | os 7 primeiros juntos | VUs divididos em 10/25/20/20/10/10/5%; cada iteração sorteia o fluxo (veja Limitações) |

Os cenários de escrita alteram os dados autodeclarados do CPF de teste (`USER`) e cadastram telefones aleatórios: use só a conta de teste de staging.

### Como disparar

1. GitHub → Actions → **Load tests** → **Run workflow** (branch `main`), ou `gh workflow run run-load-tests.yaml --ref main`. Não há inputs: tudo vem do environment `k6` do repositório (Settings → Environments → `k6`).
2. Variáveis do environment: `GCP_WIF_PROVIDER`, `GCP_SERVICE_ACCOUNT`, `GCP_PROJECT_ID`, `K6_NAMESPACE`, `K8S_SVC`, `KEYCLOAK_ISSUER`, `KEYCLOAK_CLIENT_ID`, `BASE_URL_RMI`, `BASE_URL_BUSCA`, `USER`, `TEST_SCENARIO`, `DURATION`, `VIRTUAL_USERS`; segredo `KEYCLOAK_CLIENT_SECRET`.
3. Execução local: `KEYCLOAK_ISSUER=... KEYCLOAK_CLIENT_ID=... KEYCLOAK_CLIENT_SECRET=... BASE_URL=https://services.staging.app.dados.rio/rmi BASE_URL_BUSCA=... USER=<CPF de teste> TEST_SCENARIO=home_dashboard VIRTUAL_USERS=10 k6 run k6/load_test.js` (o script acrescenta `/v1` à `BASE_URL`).

### Ambiente alvo

O workflow autentica no GCP por Workload Identity, pega a credencial do cluster GKE `application` do projeto `GCP_PROJECT_ID` do environment `k6`, espera os pods de `K8S_SVC`, cria o ConfigMap `k6-test-script` com `k6/load_test.js` e um `TestRun` do k6-operator no namespace `K6_NAMESPACE` (`parallelism: 10`, imagem `grafana/k6:latest`, 0,5–1 CPU e 1–2 GiB por runner). Os runners chamam os serviços pelo DNS interno do cluster: RMI em `http://rmi.rmi.svc.cluster.local:80/v1` e busca em `http://app-busca-search.busca.svc.cluster.local:8080`. O token é obtido no `setup()` por client credentials no Keycloak. Cada execução dura ~17 min (5 min de subida, 10 min estáveis, 2 min de descida); o monitor espera até 30 min e o `cleanup.sh` remove os recursos no fim.

O k6-operator (namespace `k6-operator-system`) é instalado pelo Terraform do `superapp` nos clusters `application` de staging (`rj-superapp-staging`) e de produção (`rj-superapp`): o ambiente testado é o do `GCP_PROJECT_ID`. Antes de disparar, confira que o environment aponta para `rj-superapp-staging`; carga em produção só com combinação prévia.

### Thresholds

| Métrica | Limite |
|---|---|
| `http_req_duration` | p95 < 5.000 ms |
| `http_req_failed` | < 5% |
| `failed_requests`, `http_failures` (falha de rede ou 5xx) | < 5% |
| `check_failures` (status inesperado ou resposta > 5 s) | < 15% |

O resultado sai no resumo do run (configuração e status do `TestRun`) e nos logs dos runners (o sumário de fim de teste de cada um dos 10 runners, sem agregação).

### Resultado de referência (staging)

Ainda não há: o workflow `Load tests` não tem nenhuma execução registrada no GitHub. A primeira execução em staging deve registrar aqui a data, o commit, o cenário, os VUs, o p95 de `http_req_duration`, as requisições por segundo (`http_reqs`) e a taxa de erro (`http_req_failed`).

### Limitações conhecidas

- O `TestRun` passa a URL do RMI em `BASE_URL_RMI`, mas o script lê `BASE_URL`: a variável do environment é ignorada e vale o DNS interno padrão.
- `DURATION` só aparece no log; a duração vem de `RAMP_UP_DURATION`, `STEADY_DURATION` e `RAMP_DOWN_DURATION` (padrões 5m/10m/2m), que o workflow não passa.
- No `mixed`, a proporção de VUs por cenário não controla o fluxo: cada iteração sorteia um dos 7 fluxos, então a mistura efetiva é uniforme.
- A imagem do runner é `grafana/k6:latest` (não fixada).
