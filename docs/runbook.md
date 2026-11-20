# Runbook operacional — app-rmi

Guia para quem está de plantão quando o RMI dá problema: o que o serviço faz, de quem depende, onde olhar, o que cada alerta significa e como reverter ou reprocessar. Os comandos são de exemplo e partem de quem tem acesso de leitura ao cluster e ao repositório; nenhum valor de segredo aparece aqui (eles ficam no Infisical, veja [Variáveis críticas](#7-variáveis-críticas-e-chaves-de-funcionalidade)).

> **Dados pessoais.** O RMI guarda CPF, telefone, e-mail, endereço e nome de cidadãos. Jobs da DLQ, chaves do Redis e documentos do Mongo carregam esses dados: não cole o conteúdo em issue, chat, Discord ou PR. Cite o `job_id`, a fila e o CPF mascarado (`123.***.789-**`).

## 1. Visão rápida

| | |
|---|---|
| **O que é** | API Go (Gin) de dados do cidadão por CPF: dados de governo (`citizens`), autodeclarados (`self_declared`), telefone↔CPF, opt-in/opt-out, quarentena de telefones, beta do chatbot, memória do chatbot, carteira (Clínica da Família), Mobilidade, chamados do 1746, integração com o Salesforce. |
| **Processos** | Dois binários da mesma imagem `ghcr.io/prefeitura-rio/app-rmi`: **API** (`./api`, Rollout `rmi`, porta 8080) e **sync worker** (`./sync`, Deployment `rmi-sync`, sem porta HTTP). A API grava no Redis e enfileira; o worker leva para o MongoDB e fala com os sistemas externos assíncronos. |
| **Onde roda** | GKE `application`, namespace `rmi`: projeto `rj-superapp-staging` (staging) e `rj-superapp` (produção). Manifests em `k8s/staging` e `k8s/prod`, aplicados pelo Argo CD (Application `rmi`, namespace `argocd`, `targetRevision: main`, `selfHeal`). |
| **URLs** | Staging: `https://services.staging.app.dados.rio/rmi/v1/...` · Produção: `https://services.pref.rio/rmi/v1/...` (gateway Istio com autorização Cerbos/Heimdall; o prefixo `/rmi` é do gateway). |
| **Escala** | API: KEDA `rmi-scaled-object`, 3 a 10 réplicas, pela taxa de requisições do Istio. Worker: réplica fixa (o ScaledObject do worker está comentado em `k8s/*/resources.yaml`). PDB `minAvailable: 1` para os dois. |

**Quem consome**

- Superapp / Portal (cidadão logado, JWT do Keycloak): `/v1/citizen/{cpf}/...`, `/v1/auth/validate`, `/v1/salesforce/...`.
- Chatbot do WhatsApp (EAí): rotas `/v1/phone/{phone}/...`, `/v1/memory/...`, beta.
- Salesforce (client de serviço do Keycloak): webhook `POST /v1/webhooks/salesforce/cidadao`.
- Público sem login: `GET /v1/maintenance-request/{protocolo}`, listas de opções, `/v1/health`.
- Administradores (grupo `ADMIN_GROUP`) e clients de serviço confiáveis (`TRUSTED_SERVICE_CLIENTS`): rotas `/v1/admin/...`.
- **app-go-api** (service account do Keycloak, com o `azp` em `TRUSTED_SERVICE_CLIENTS`): chama `GET /v1/cpf-secretaria/{cpf}` (rota com `RequireOwnCPF`, que aceita o client de serviço confiável). O RMI **não chama** o go-api: ele é consumidor, não dependência. Se o `azp` dele sair de `TRUSTED_SERVICE_CLIENTS`, as chamadas dele passam a receber 403.

**De quem depende**

| Dependência | Para quê | Se cair |
|---|---|---|
| **MongoDB** (Bitnami, réplica de 3 nós no namespace `rmi`; banco `rmi`) | Persistência de tudo. Backup diário às 05:00 UTC (CronJob de dump) no bucket `rmi-backups` (`rmi-backups-staging` em staging). | `/v1/health` responde 500 → readiness e liveness falham → pods da API saem do Service e reiniciam. O worker entra em **modo degradado** e para de consumir filas. |
| **Redis / Valkey** (release `redis` no namespace `rmi`; Service alias `redis-valkey`) | Cache de leitura, buffer de escrita, filas do sync worker, DLQ, locks, rate limit. | `/v1/health` responde 500 → API fora. Escritas não chegam ao Mongo. |
| **Keycloak** (Identidade Carioca) | Emite os JWTs. O gateway (Cerbos/Heimdall) valida o token; a API lê as claims, e só a rota pública de chamados verifica a assinatura pelo JWKS de `JWT_ISSUER_URL`. | Login quebra (401 em tudo que é autenticado). Com o JWKS fora, a consulta por protocolo cai para a visão pública (sem erro). |
| **Salesforce CRM** (`SALESFORCE_BASE_URL`, via Mule) | Sincronização de cadastro no login, push de alterações, proxies de consentimento/exportação/anonimização/chamados, webhook de entrada. | `/v1/auth/validate` responde 502; jobs `salesforce_push` vão para retry e depois DLQ. O resto da API segue. |
| **WhatsApp (Wetalkie)** ou **Salesforce Marketing Cloud** (`VERIFICATION_PROVIDER`) | Envio do código de verificação de telefone. | Validação de telefone falha; o resto segue. |
| **MCP da Prefeitura** (`MCP_SERVER_URL`) | Busca da Clínica da Família mais próxima (carteira). | Lookup síncrono falha em 15 s e vira job `cf_lookup` com retry (até 5 tentativas). |
| **Data Relay** (`DATA_RELAY_BASE_URL`) | E-mail de convite de condutor (Mobilidade). | Jobs `mobilidade_invite_email` vão para retry/DLQ; sem a configuração, o worker só registra em log. |

**Não há PostgreSQL** no RMI: toda a persistência é MongoDB. Também não há Flagger: as entregas usam **Argo Rollouts** (blue-green em staging, canário em produção).

## 2. Onde olhar

### Saúde e versão

```bash
curl -s https://services.pref.rio/rmi/v1/health | jq
curl -s https://services.staging.app.dados.rio/rmi/v1/health | jq
```

Resposta esperada: `status: true`, `services.mongodb.status/message = true/"Connected"`, `services.redis.status/message = true/"Connected"` e `version` = o commit curto implantado (`git rev-parse --short`, normalmente 7 caracteres). Qualquer dependência fora → HTTP 500 com `status: false`.

### Cluster (leitura)

```bash
gcloud container clusters get-credentials application --dns-endpoint --region us-central1 --project rj-superapp   # rj-superapp-staging em staging
kubectl -n rmi get rollout rmi -o wide                       # fase do Rollout (Healthy, Paused, Degraded, Aborted)
kubectl argo rollouts get rollout rmi -n rmi                 # passos do canário/blue-green (plugin do Argo Rollouts)
kubectl -n rmi get deploy rmi-sync
kubectl -n rmi get pods -l app=rmi ; kubectl -n rmi get pods -l app=rmi-sync
kubectl -n rmi get events --field-selector type=Warning --sort-by=.lastTimestamp
kubectl -n rmi logs -l app=rmi-sync --since=15m --tail=200
kubectl -n argocd get application rmi -o jsonpath='{.status.sync.status} {.status.health.status} {.spec.source.kustomize.images}'
```

A imagem que vale é o **override do Application** (`spec.source.kustomize.images`), que os workflows de deploy atualizam; a tag escrita em `k8s/prod/resources.yaml` não é a fonte da verdade.

### SigNoz (logs e traces)

O SigNoz de cada ambiente roda no próprio cluster (namespace `signoz`) e é acessado pela Tailscale. Os logs dos pods são coletados pelo `k8s-infra`.

- **Logs**: filtre por `k8s.namespace.name = rmi` e `k8s.container.name = rmi` (API) ou `rmi-sync` (worker). Os logs são JSON do Zap com `level`, `msg`, `caller`, `timestamp` e os campos fixos `service = app-rmi` e `version = v1`.
- **Traces** (só quando `TRACING_ENABLED=true`): serviço `app-rmi`.
- Mensagens úteis para busca (`msg`):

| `msg` | Processo | Significado |
|---|---|---|
| `job moved to DLQ` (ERROR) | worker | Um job esgotou as tentativas ou teve erro permanente. Campos `job_id`, `type`, `error`. |
| `DLQ has failed jobs` (WARN, na subida e a cada 5 min) | worker | Há itens na DLQ de `queue`, com `dlq_size`. |
| `degraded mode activated` / `deactivated` | worker | Worker parou/voltou a consumir filas (`reason`: `mongodb_down` ou `redis_memory_high`). |
| `recovered stale inflight sync job` / `inflight recovery hit cap` | worker | Jobs presos em processamento há mais de 5 min voltaram à fila (filas confiáveis). |
| `CF lookup failed` (WARN) | worker | Tentativa de `cf_lookup` falhou (`attempt`, `max_attempts`, `permanent`). |
| `salesforce auth validate failed` | API | Sincronização de login com o Salesforce falhou (resposta 502). |
| `salesforce proxy upstream error` | API | Salesforce respondeu erro a um proxy (`status`). |
| `salesforce webhook client rejected` | API | Webhook chamado por um client cujo `azp` não está em `SALESFORCE_WEBHOOK_CLIENTS` (403). |
| `Critical Redis connection usage - immediate action required` / `High Redis connection usage detected` / `No idle Redis connections available - potential bottleneck` / `High number of stale Redis connections` | API e worker | Pool do Redis no limite (`usage_percent`) ou conexões caindo. |
| `MongoDB connection pool under pressure` / `MongoDB connection pool critical - immediate attention required` | API | Sessões em uso acima de 800/950. |
| `request completed` | API | Uma linha por requisição (`path` e `query` mascarados, `status`, `latency`). |

Os logs mascaram CPF, telefone, nome, chaves de cache e caminhos (`observability.*`); nunca procure por um CPF em claro.

### Métricas

- **API**: `GET /metrics` (formato Prometheus, porta 8080; `/v1/metrics` é só um texto fixo). Os pods não têm anotação de scrape: para consultar, use `kubectl -n rmi port-forward svc/rmi 8080:80` e `curl -s localhost:8080/metrics`. Principais séries: `app_rmi_request_duration_seconds{path,method,status}`, `app_rmi_database_operations_total`, `app_rmi_cache_hits_total`, `app_rmi_phone_verification_*`.
- **Worker**: o `./sync` expõe `GET /metrics` na porta `METRICS_PORT` (padrão 9090; `0` desliga; log `metrics server listening` na subida). Os pods também não têm anotação de scrape: `kubectl -n rmi port-forward deploy/rmi-sync 9090:9090` e `curl -s localhost:9090/metrics`. Séries: `rmi_sync_queue_depth{queue="<fila>"}` (backlog de `sync:queue:<fila>` de todas as filas do worker, na subida e a cada 15 s), `rmi_sync_queue_depth{queue="dlq_<fila>"}` (todas as DLQs do worker, na subida e a cada 5 min; para só o backlog, `queue!~"dlq_.*"`), `rmi_sync_operations_total{queue,status}`, `rmi_sync_failures_total{queue}`, `rmi_degraded_mode_active`. Junto dos logs acima e do tamanho das filas no Redis ([seção 4.2](#42-filas-e-dlq-do-sync-worker)).
- **Istio / Prometheus** (`prometheus-server.istio-system:9090`, o mesmo que o KEDA e a análise do canário usam). Taxa de 5xx do RMI:

```promql
sum(rate(istio_requests_total{destination_workload="rmi",response_code=~"5.."}[5m]))
  / sum(rate(istio_requests_total{destination_workload="rmi"}[5m]))
```

Volume (é o que escala a API): `sum(rate(istio_requests_total{destination_workload="rmi"}[30s]))`.

### Grafana

Não há dashboard do RMI versionado no repositório nem no `prefeitura-rio/infra`. Use o SigNoz (logs e traces) e as consultas PromQL acima.

## 3. Alertas e primeiro passo

| Alerta | Onde chega | O que significa | Primeiro passo |
|---|---|---|---|
| **Gatus "RMI API"** (definido em `superapp/modules/k8s/yamls/gatus/values.yaml` do `infra`) | Discord | `GET /rmi/v1/health` a cada 5 min falhou: status ≠ 200, resposta ≥ 1 s, ou Mongo/Redis ≠ `Connected`. | Abra o health e veja qual dependência está `false`. Mongo: `kubectl -n rmi get pods \| grep mongodb` e eventos. Redis: pods do Valkey e logs de pool. Ambos ok e resposta lenta: veja réplicas da API, KEDA e a taxa de requisições. |
| **Staging Deployment Failed** (workflow `deploy-staging.yaml`) | Discord e resumo do run | E2E contra o preview blue-green falhou; o workflow já abortou o Rollout e o ativo não mudou. | Leia o job "Run E2E Tests Against Preview" do run. Corrija em novo PR; nada a reverter no cluster. |
| **Production Release Aborted / Failed** (workflow `release.yaml`) | Discord (com menção ao grupo) | Canário abortado pela análise `rmi-success-rate` (sucesso < 95% ou 5xx > 5% em 5 medições) ou Rollout `Degraded`. Em `Aborted` o tráfego já voltou para a versão estável. | `kubectl argo rollouts get rollout rmi -n rmi` e `kubectl -n rmi get analysisrun --sort-by=.metadata.creationTimestamp`. Confirme no health a versão estável e siga a [seção 4.1](#41-rollback). O `rmi-sync` **não** passa pelo canário: confira se ele está na versão nova e, se sim, reverta (seção 4.1). |
| Log `job moved to DLQ` / `DLQ has failed jobs` | SigNoz | Jobs do worker que não serão mais tentados. | Identifique fila e erro ([seção 4.2](#42-filas-e-dlq-do-sync-worker)); corrija a causa antes de reprocessar. |
| Log `degraded mode activated` | SigNoz | Worker parado: Mongo inalcançável ou Redis com memória ≥ 85% do `maxmemory`. As filas crescem. | `reason=mongodb_down`: trate o Mongo. `reason=redis_memory_high`: `INFO memory` no Redis, procure filas/DLQs grandes (`LLEN`) e chaves sem TTL. O worker volta sozinho em até 10 s depois que a condição some. |
| Logs de pool do Redis/Mongo | SigNoz | Pool perto do limite; latência e timeouts em seguida. | Veja réplicas e carga (Istio), conexões no servidor (`INFO clients`, `db.serverStatus().connections`). Ajuste `REDIS_POOL_SIZE`/limites só pelo caminho de configuração (seção 7). |
| Argo CD `OutOfSync`/`Degraded` no Application `rmi` | Argo CD | Manifest de `k8s/` não aplicou ou pod não fica pronto. | `kubectl -n argocd get application rmi -o yaml` (condições e `operationState`), eventos do namespace. |

### Regras de alerta do Prometheus

As regras mínimas de produção ficam versionadas em `internal/observability/alerts/rmi-alerts.rules.yml` (formato de arquivo de regras do Prometheus, o mesmo `groups:` do `spec` de uma PrometheusRule), com testes em `rmi-alerts.test.yml` (`just test-alerts`, com o `promtool` 3.x no `PATH`). O arquivo **não** está no `kustomization.yaml` de `k8s/<env>`: carregá-lo no Prometheus do `istio-system`, o Alertmanager e o roteamento das notificações são da raia Infra (`superapp/modules/k8s/istio.tf` do `infra`). As regras de API e do worker dependem do scrape dos pods (anotações `prometheus.io/*`); as de pod, do kube-state-metrics.

| Alerta | Severidade | Dispara quando | Primeiro passo |
|---|---|---|---|
| `RMIHighErrorRate` | critical | Respostas 5xx acima de 2% das requisições da API em 5 min (`app_rmi_request_duration_seconds_count`, `status=~"5.."`). | Health, logs `request completed` com status 5xx, deploy recente (seção 4). |
| `RMIHighLatencyP95` | warning | p95 de `app_rmi_request_duration_seconds` acima de 1 s em 10 min, em todas as rotas menos `/metrics` e `/v1/health`. | Réplicas e KEDA, logs de pool do Redis/Mongo, dependências ([seção 4.4](#44-dependência-fora-do-ar)). |
| `RMIPodRestarting` | critical | Um container do namespace `rmi` reiniciou mais de 3 vezes em 15 min. | `kubectl -n rmi describe pod <pod>` e `kubectl -n rmi logs <pod> -c <container> --previous`. |
| `RMIPodCrashLooping` | critical | Um container do namespace `rmi` está em `CrashLoopBackOff` há 5 min. | Idem. |
| `RMISyncQueueBacklog` | warning | `rmi_sync_queue_depth` de uma fila (não DLQ) acima de 100 itens por 10 min. O limite é calibrável: troque o `100` da expressão (e do `summary`) e o caso do teste. | `degraded mode activated`, réplicas do `rmi-sync`, Mongo ([seção 4.2](#42-filas-e-dlq-do-sync-worker)). |
| `RMISyncDLQNewItems` | warning | Uma DLQ (`rmi_sync_queue_depth{queue="dlq_<fila>"}`) está maior que há 15 min. A DLQ é medida a cada 5 min, então o alerta chega em até 5 min e some 15 min depois do último item novo. | Fila e erro do item, corrigir a causa antes de reprocessar ([seção 4.2](#42-filas-e-dlq-do-sync-worker)). |

## 4. Rotina de incidente

1. **Delimite**: health dos dois ambientes, versão (`version` do health), fase do Rollout, pods da API e do worker, eventos `Warning`.
2. **Veio de um deploy?** Compare a versão em produção com a última GitHub Release (`gh release list -R prefeitura-rio/app-rmi`) e com o horário do último run de `release.yaml` (`gh run list -R prefeitura-rio/app-rmi --workflow release.yaml`). Se sim, reverta primeiro e investigue depois (4.1).
3. **Dependência?** Mongo, Redis, Keycloak, Salesforce, MCP: veja a tabela da seção 1 e os logs.
4. **Escritas pendentes?** Tamanho das filas e DLQs (4.2).
5. **Registre** o que fez (horário, comando, resultado) no incidente, sem dados pessoais.

### 4.1 Rollback

**Produção (canário do Argo Rollouts)**

- **Canário em andamento e com problema**: `kubectl argo rollouts abort rmi -n rmi` devolve 100% do tráfego à versão estável. Depois, publique a correção ou reverta (abaixo) para o Rollout não voltar a promover a mesma imagem.
- **Versão já promovida (`Healthy`)**: volte para a release anterior reexecutando o workflow dela. O `release.yaml` faz o checkout da tag, reconstrói a imagem e troca o override do Application, então o Argo CD aplica a versão anterior nos dois processos (API por canário, worker por RollingUpdate):

  ```bash
  gh release list -R prefeitura-rio/app-rmi --limit 5
  gh run list -R prefeitura-rio/app-rmi --workflow release.yaml --limit 10   # ache o run da tag anterior
  gh run rerun <run-id> -R prefeitura-rio/app-rmi
  ```

  O GitHub só reexecuta runs de até 30 dias. Para uma release mais antiga, ou como alternativa equivalente, publique uma nova release a partir de um commit que reverte a mudança (`git revert` na `main` por PR, depois a release).
- **Não use `kubectl argo rollouts undo`** nem edite o Rollout à mão: o Application `rmi` tem `selfHeal` e a imagem fixada no override, então o Argo CD desfaz a mudança.

**Staging (blue-green)**

- Falha de E2E já aborta sozinha (o ativo não muda).
- Para tirar de staging algo que passou: reverta o commit na `main` por PR; o push dispara `deploy-staging.yaml` com a imagem do revert. Comandos de ponta a ponta na [seção 8.1](#81-rollback-em-staging).

**Mudança só em `k8s/`** (o deploy de staging pula o build quando só `k8s/` mudou): reverta o commit por PR; o Argo CD aplica o estado anterior.

### 4.2 Filas e DLQ do sync worker

A API enfileira jobs com `LPUSH` e o worker consome com `RPOP` (ou `RPOPLPUSH` nas filas confiáveis), 3 jobs por ciclo de 50 ms em rodízio entre as filas. Falha transitória volta à fila com espera crescente (`available_at`); ao esgotar as tentativas, ou em erro permanente, o job vai para a DLQ da fila.

| Chave | Filas |
|---|---|
| `sync:queue:<fila>` / `sync:dlq:<fila>` | `citizen`, `phone_mapping`, `user_config`, `opt_in_history`, `beta_group`, `phone_verification`, `maintenance_request`, `self_declared_address`, `self_declared_email`, `self_declared_phone`, `self_declared_raca`, `self_declared_nome_exibicao`, `self_declared_genero`, `self_declared_renda_familiar`, `self_declared_escolaridade`, `self_declared_deficiencia`, `self_declared_nascimento`, `cf_lookup` |
| `sync:queue:{<fila>}` / `sync:dlq:{<fila>}` + `sync:processing:{<fila>}` e `sync:processing:claimed:{<fila>}` (filas confiáveis, com hash tag) | `mobilidade_invite_email`, `salesforce_sync`, `salesforce_push` |

O alerta `DLQ has failed jobs` e as séries `rmi_sync_queue_depth{queue="<fila>"}` (backlog) e `rmi_sync_queue_depth{queue="dlq_<fila>"}` (DLQ) cobrem todas essas filas: os scans usam a mesma lista que o worker consome.

O buffer de escrita (`<tipo>:write:<chave>`) e o cache de leitura (`<tipo>:cache:<chave>`) ficam ao lado; o worker apaga o buffer e atualiza o cache quando o job grava no Mongo.

**Inspecionar** (dentro do pod do Valkey, autenticando com a senha que está no Secret do chart; não a imprima nem a cole em lugar nenhum):

```bash
kubectl -n rmi get pods | grep -iE 'redis|valkey'
kubectl -n rmi exec -it <pod-do-valkey> -- sh
# dentro do pod (valkey-cli ou redis-cli):
LLEN sync:queue:citizen
LLEN sync:dlq:citizen
LLEN "sync:dlq:{salesforce_push}"
LRANGE sync:dlq:citizen -1 -1          # o mais antigo; JSON {original_job, error, failed_at}
INFO memory
```

Para varrer todas: `SCAN 0 MATCH sync:dlq:* COUNT 1000` (nunca `KEYS *` em produção). Leia `error` e `original_job.type`/`id`/`retry_count`; o `original_job.data` contém dados pessoais.

**Reprocessar** (só depois de corrigir a causa):

1. Confira se o dado do job ainda é o mais recente. O worker grava `original_job.data` com `$set` e upsert, sem comparar versão: se o cidadão alterou o mesmo campo depois da falha, reprocessar o job antigo **sobrescreve o dado novo**. Compare o `original_job.timestamp` (hora em que o job foi enfileirado; o `original_job.data` dos autodeclarados também traz `updated_at`) com o `updated_at` do documento no Mongo antes de devolver.
2. Devolva um job por vez: pegue o JSON de `original_job`, zere `retry_count`, remova `available_at` e faça `LPUSH` na fila de origem (`sync:queue:<fila>` ou `sync:queue:{<fila>}`), então remova esse item da DLQ (`LREM <dlq> 1 '<json exato do item>'`). Faça a cópia antes de remover.
3. Acompanhe nos logs `sync job succeeded - cache updated and write buffer cleaned` ou um novo `job moved to DLQ` para o mesmo `job_id`.

Os comandos de ponta a ponta (sem colar senha nem dado pessoal no terminal) estão na [seção 8.2](#82-reprocessar-um-job-da-dlq-em-staging).

Casos especiais:

- **`salesforce_push`**: o JWT do cidadão é retirado do job ao ir para a DLQ, e o push exige o token (`bearer token absent on job`). **Esses jobs não podem ser reprocessados**; a próxima alteração do cidadão (ou o próximo login, por `/v1/auth/validate`) volta a sincronizar. Descarte os itens depois de registrar a contagem e o erro.
- **`cf_lookup`**: pode ser descartado; a próxima consulta da carteira refaz o lookup (o dedupe por CPF, `cf_lookup:pending:<cpf>`, dura 1 h).
- **`mobilidade_invite_email`**: o status fica no vínculo (`email_status = failed`, `email_last_error`). Reprocessar reenvia o e-mail real: confirme com o time de Mobilidade antes.
- Jobs presos em `sync:processing:{<fila>}` há mais de 5 min voltam sozinhos à fila (recuperação a cada 1 min); não mexa à mão.

Limpeza de avatar (`DELETE` de avatar): o job vai para `sync:queue:user_config` com `data.type = avatar_cleanup` e remove o `avatar_id` dos `user_config` que o usavam; na DLQ (`sync:dlq:user_config`) pode ser reprocessado, porque a limpeza é idempotente. A lista `user_config:write` é legado (versões antigas enfileiravam ali e nenhum worker a lê): ao iniciar, o worker migra para `sync:queue:user_config` os payloads `avatar_cleanup` parados nela (até 1000 por início, log `legacy avatar cleanup list processed`); o que não for `avatar_cleanup` fica na lista e é só contado em log (`unrecognized`).

### 4.3 Salesforce respondendo 401

O RMI não tem credencial própria no Salesforce: todas as chamadas usam o **JWT do cidadão** (login, proxies e push) e o Mule valida esse token. Um 401 quase sempre é token, não indisponibilidade.

| Onde aparece | Causa provável | O que fazer |
|---|---|---|
| Proxies `/v1/salesforce/...` repassam 401 ao app | Token do cidadão expirado ou recusado pela política de JWT do Mule (a resposta vem do Mule, sem o formato `errors[]` do Salesforce). | Caso isolado: o app renova o login. Em massa: o Mule deixou de aceitar tokens do realm (issuer, audiência, JWKS). Confirme com um token recém-emitido; se persistir, acione o time do Salesforce/Mule com horário, rota e quantidade (sem tokens nem CPFs). |
| `/v1/auth/validate` responde 502 com `salesforce auth validate failed` | Mesma causa, no GET/PATCH/POST do login. | Igual acima. |
| `job moved to DLQ` em `salesforce_push` com `salesforce patch failed` (401) ou `bearer token expired` | O JWT capturado na escrita expirou antes do worker processar o push (fila parada ou atrasada) ou foi recusado. São erros permanentes: vão direto para a DLQ. | Veja se a fila `sync:queue:{salesforce_push}` estava represada (modo degradado, worker parado). Não reprocesse (4.2); a próxima alteração ou login sincroniza de novo. |
| Webhook SF→RMI recebe 401/403 | 401: chamada sem JWT ou token inválido. 403: `azp` fora de `SALESFORCE_WEBHOOK_CLIENTS` (log `salesforce webhook client rejected`). 503: lista não configurada. | 401: o Salesforce precisa obter token do client de serviço no Keycloak. 403/503: ajuste `SALESFORCE_WEBHOOK_CLIENTS` pelo caminho de configuração (seção 7). |

### 4.4 Dependência fora do ar

- **MongoDB**: health 500 e pods da API reiniciando; worker em modo degradado (as filas acumulam no Redis e drenam quando o Mongo volta). Não reinicie o worker em laço; acompanhe o tamanho das filas. Restauração de dados: dumps diários no bucket de backup (seção 1).
- **Redis**: API fora (health 500). Filas e buffers de escrita ainda não gravados no Mongo dependem da persistência do Valkey: se ele voltar vazio, essas escritas se perdem. Registre o horário da queda e o tamanho das filas antes, se houver.
- **MCP / Data Relay / WhatsApp / SFMC**: degradação parcial; veja a tabela da seção 1. Os jobs assíncronos seguem em retry e depois DLQ.

## 5. Deploy, em resumo

- **Staging**: todo push na `main` roda `deploy-staging.yaml`: build da imagem `:<sha>`, troca do override do Application, preview blue-green (`rmi-preview`), E2E contra o preview, `kubectl argo rollouts promote` se passou e `abort` se falhou.
- **Produção**: publicar uma GitHub Release roda `release.yaml`: build da tag, troca do override, canário 10% → 20% → 40% → 60% → 80% com pausas e a análise `rmi-success-rate` a partir do segundo passo; `Healthy` encerra.

## 6. Comandos de leitura úteis

```bash
gh run list -R prefeitura-rio/app-rmi --workflow deploy-staging.yaml --limit 5
gh run list -R prefeitura-rio/app-rmi --workflow release.yaml --limit 5
kubectl -n rmi get scaledobject rmi-scaled-object
kubectl -n rmi get analysisrun --sort-by=.metadata.creationTimestamp | tail -5
kubectl -n rmi get cronjob                      # dump diário do Mongo
```

## 7. Variáveis críticas e chaves de funcionalidade

A configuração vem toda de variáveis de ambiente (`internal/config/config.go`); a lista completa, com padrões, está no [README](../README.md#variáveis-de-ambiente). Em staging e produção os valores ficam no **Infisical** (projeto do RMI), que o operador sincroniza no Secret `rmi-secrets`; os pods têm `secrets.infisical.com/auto-reload` e reiniciam quando o Secret muda. Mude valores só por esse caminho, nunca com `kubectl edit`/`set env` no cluster.

**Sem estas o processo não sobe** (erro em `LoadConfig`): `MONGODB_CITIZEN_COLLECTION`, `MONGODB_MAINTENANCE_REQUEST_COLLECTION`, `MONGODB_LEGAL_ENTITY_COLLECTION`, `MONGODB_PET_COLLECTION`, `MONGODB_PETS_SELF_REGISTERED_COLLECTION`, `MONGODB_CHAT_MEMORY_COLLECTION`, `MONGODB_DEPARTMENT_COLLECTION`, `MONGODB_CNAE_COLLECTION`, `WHATSAPP_API_BASE_URL`, `WHATSAPP_API_USERNAME`, `WHATSAPP_API_PASSWORD`, `WHATSAPP_HSM_ID`, `WHATSAPP_COST_CENTER_ID`, `WHATSAPP_CAMPAIGN_NAME`; `MCP_SERVER_URL` e `MCP_AUTH_TOKEN` com `CF_LOOKUP_ENABLED=true`; `SFMC_*` com `VERIFICATION_PROVIDER=salesforce`; `REDIS_CLUSTER_ADDRS` com `REDIS_CLUSTER_ENABLED=true`; `SYNC_JOB_BEARER_ENCRYPTION_KEY`, se definida, precisa ser válida (64 hex). Valor inválido de `MONGODB_MAINTENANCE_REQUEST_SGRC_ENABLED` também impede o boot. Pod em `CrashLoopBackOff` logo após mudança de configuração: veja o fim do log (`failed to load config` na API, `Failed to load config:` no worker), que diz qual variável falta ou é inválida.

**Migração SGRC (histórico 1746):** `MONGODB_MAINTENANCE_REQUEST_SGRC_ENABLED` (default `false`) escolhe a collection de **leitura** na subida do processo — `false` → `MONGODB_MAINTENANCE_REQUEST_COLLECTION` (legado); `true` → `MONGODB_MAINTENANCE_REQUEST_SGRC_COLLECTION` (default `maintenance_requests_sgrc`; **não** é obrigatória no boot — o merge para main não exige a chave no Infisical). Não vem de query/header/payload. Antes de ligar a flag em staging/prod, confirme no Infisical o nome da collection (se diferente do default) e que a collection existe com dados/índices. Rollback operacional: Infisical → `false` → reload dos pods. As duas collections precisam existir com leitura durante a transição; a carga Airbyte alimenta só a SGRC.

**Conexão**: `MONGODB_URI`, `MONGODB_DATABASE` (padrão `rmi`), `REDIS_URI`, `REDIS_PASSWORD`, `REDIS_CLUSTER_ENABLED`, `REDIS_POOL_SIZE` (200), `REDIS_MIN_IDLE_CONNS` (50), timeouts `REDIS_*_TIMEOUT`.

**Chaves de funcionalidade** (não há serviço de feature flag; são variáveis):

| Variável | Padrão | Efeito |
|---|---|---|
| `WHATSAPP_ENABLED` | `true` | Envio do código de verificação pelo WhatsApp. |
| `VERIFICATION_PROVIDER` | `wetalkie` | `wetalkie` ou `salesforce` (Marketing Cloud) para o código de verificação. |
| `SFMC_ENABLED` | `true` | Envio via Marketing Cloud quando ele é o provedor. |
| `CF_LOOKUP_ENABLED` | `true` | Busca da Clínica da Família pelo MCP. |
| `SALESFORCE_BASE_URL` | vazio | Vazio desliga a integração com o Salesforce (login responde 503, push não roda). |
| `SALESFORCE_WEBHOOK_CLIENTS` | vazio | `azp` aceitos no webhook; vazio responde 503. |
| `SYNC_JOB_BEARER_ENCRYPTION_KEY` | vazio | Sem ela o JWT não é guardado nos jobs e `salesforce_push` não é enfileirado. |
| `DATA_RELAY_BASE_URL` + `DATA_RELAY_API_KEY` | vazio | Sem os dois, e-mails de convite só vão para o log. |
| `JWT_ISSUER_URL` | vazio | Sem ele, a consulta pública de chamados só mostra a visão pública. |
| `TRUSTED_SERVICE_CLIENTS` / `ADMIN_GROUP` | vazio / `heimdall-admin` | Quem tem privilégio de administrador ou de client de serviço. |
| `AUDIT_LOGS_ENABLED` | `true` | Auditoria automática das escritas (coleção `audit_logs`). |
| `TRACING_ENABLED` / `TRACING_ENDPOINT` | `false` / `localhost:4317` | Envio de traces OTLP ao SigNoz. |
| `LOG_LEVEL` | `info` | `debug` mostra o ciclo das filas; não deixe ligado em produção. |

**Ajustes de comportamento**: `DB_WORKER_COUNT` (workers do sync, 10), `CF_LOOKUP_SYNC_TIMEOUT` (15s), `CF_LOOKUP_JOB_TIMEOUT` (90s), `CF_LOOKUP_GLOBAL_RATE_LIMIT`, `PHONE_VERIFICATION_TTL` (5m), `PHONE_QUARANTINE_TTL` (4320h), `REDIS_TTL`, `INDEX_MAINTENANCE_INTERVAL` (1h).

## 8. Passo a passo em staging (para quem não é do time)

Roteiro para fazer, só com este documento, um rollback e o reprocessamento de uma fila em **staging**. Pré-requisitos: `gcloud` autenticado com acesso ao projeto `rj-superapp-staging` (com permissão de `exec` no namespace `rmi` e de leitura do Secret `rmi-secrets`), `kubectl`, `gh` autenticado com permissão de abrir PR no `prefeitura-rio/app-rmi`, `git` e `jq`. Em produção os passos de rollback são os da [seção 4.1](#41-rollback).

### 8.1 Rollback em staging

1. **Veja o que está implantado** e ache o commit que causou o problema (cada PR entra na `main` como um commit):

   ```bash
   curl -s https://services.staging.app.dados.rio/rmi/v1/health | jq '{status, version}'   # version = commit curto em staging
   gh run list -R prefeitura-rio/app-rmi --workflow deploy-staging.yaml --limit 5
   git clone https://github.com/prefeitura-rio/app-rmi.git && cd app-rmi
   git log --oneline -10 origin/main
   ```

2. **Abra o PR de revert** (nunca faça push direto na `main`):

   ```bash
   SHA=<commit a reverter>
   git switch -c "revert-${SHA:0:7}" origin/main
   git revert --no-edit "$SHA"            # commit de merge ("Merge pull request #..."): git revert --no-edit -m 1 "$SHA"
   git push origin "revert-${SHA:0:7}"
   gh pr create -R prefeitura-rio/app-rmi --base main --head "revert-${SHA:0:7}" \
     --title "revert: <título do PR revertido>" --body "Reverte $SHA em staging: <motivo, sem dados pessoais>."
   ```

   O PR segue as regras da `main` (CI e revisão). Depois do merge, o push dispara o `deploy-staging.yaml`.

3. **Acompanhe o deploy e confira**:

   ```bash
   gh run list -R prefeitura-rio/app-rmi --workflow deploy-staging.yaml --limit 1     # o run do merge do revert
   gh run watch <run-id> -R prefeitura-rio/app-rmi                                   # termina em "Promote to Stable"
   curl -s https://services.staging.app.dados.rio/rmi/v1/health | jq '{status, version}'
   gcloud container clusters get-credentials application --dns-endpoint --region us-central1 --project rj-superapp-staging
   kubectl -n rmi get rollout rmi -o wide                                            # Healthy
   kubectl -n rmi get deploy rmi-sync -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
   ```

   Pronto quando o run terminou com sucesso, `version` é o commit curto do merge do revert (`git rev-parse --short <sha do merge>`), o Rollout está `Healthy` e o `rmi-sync` usa a imagem `:<sha do merge>`.

   - Run com falha no E2E: o workflow já abortou o Rollout e o staging continua na versão anterior; leia o job "Run E2E Tests Against Preview".
   - **Atalho de emergência** (até o revert entrar): `gh run rerun <run-id de um deploy bom> -R prefeitura-rio/app-rmi` reconstrói e promove aquela versão em staging. O GitHub só reexecuta runs de até 30 dias, e o próximo push na `main` volta a implantar o que está nela: o PR de revert continua necessário.
   - Não use `kubectl argo rollouts undo` nem edite o Rollout ou o Application à mão (o Argo CD desfaz, seção 4.1).

### 8.2 Reprocessar um job da DLQ em staging

Antes: corrija a causa do erro (seção 4.2) e confira que o worker está consumindo (`kubectl -n rmi get pods -l app=rmi-sync`, sem `degraded mode activated` recente). Não reprocesse `salesforce_push` (não tem mais o token) nem `mobilidade_invite_email` (reenvia e-mail real); `cf_lookup` pode ser descartado (seção 4.2).

1. **Conecte no cluster de staging e prepare o acesso ao Valkey.** A senha é a `REDIS_PASSWORD` do Secret `rmi-secrets` (a mesma que a API usa). Ela vai para uma variável e entra no `valkey-cli` pela entrada padrão, nunca como argumento (os argumentos do `kubectl exec` ficam no log de auditoria do cluster). Não dê `echo` nela e não rode com `set -x`.

   ```bash
   gcloud container clusters get-credentials application --dns-endpoint --region us-central1 --project rj-superapp-staging
   kubectl config current-context                       # confira que é o rj-superapp-staging antes de seguir
   VK_POD="$(kubectl -n rmi get pods -o name | grep redis-master | head -n1)"
   VK_PASS="$(kubectl -n rmi get secret rmi-secrets -o jsonpath='{.data.REDIS_PASSWORD}' | base64 -d)"
   rcli()  { printf '%s\n' "$VK_PASS" | kubectl -n rmi exec -i "$VK_POD" -c redis-master -- sh -c 'IFS= read -r VALKEYCLI_AUTH; export VALKEYCLI_AUTH REDISCLI_AUTH="$VALKEYCLI_AUTH"; exec valkey-cli --raw "$@"' sh "$@"; }
   rclix() { { printf '%s\n' "$VK_PASS"; cat; } | kubectl -n rmi exec -i "$VK_POD" -c redis-master -- sh -c 'IFS= read -r VALKEYCLI_AUTH; export VALKEYCLI_AUTH REDISCLI_AUTH="$VALKEYCLI_AUTH"; exec valkey-cli --raw -x "$@"' sh "$@"; }
   rcli PING                                            # PONG
   ```

   O Valkey é o Deployment `redis-master` (chart `valkey`, definido em `superapp/modules/k8s/rmi.tf` do `infra`); o container do servidor também se chama `redis-master` (o outro, `metrics`, é o exporter). `rclix` manda a entrada padrão como último argumento do comando (`-x`), o que evita problemas de aspas com o JSON do job.

2. **Escolha a fila e olhe o item mais antigo da DLQ** (sem imprimir `original_job.data` nem `original_job.key`, que têm dados pessoais):

   ```bash
   FILA=citizen                       # nome da seção 4.2; filas confiáveis com chaves: FILA='{salesforce_sync}'
   rcli LLEN "sync:dlq:$FILA"
   ITEM="$(rcli LRANGE "sync:dlq:$FILA" -1 -1)"       # a DLQ recebe LPUSH: o fim da lista é o mais antigo
   printf '%s' "$ITEM" | jq '{job_id: .original_job.id, type: .original_job.type, retry_count: .original_job.retry_count, enfileirado: .original_job.timestamp, failed_at, error}'
   ```

   Se o erro ainda não foi corrigido, pare aqui. Confira também o passo 1 do reprocessamento da seção 4.2 (o job antigo sobrescreve um dado mais novo).

3. **Devolva o job à fila e tire da DLQ**, nessa ordem:

   ```bash
   JOB="$(printf '%s' "$ITEM" | jq -c '.original_job | .retry_count = 0 | del(.available_at)')"
   printf '%s' "$JOB"  | rclix LPUSH "sync:queue:$FILA"     # responde o novo tamanho da fila
   printf '%s' "$ITEM" | rclix LREM "sync:dlq:$FILA" 1       # responde 1 (0: o item já tinha saído da DLQ)
   ```

   O `ITEM` fica na variável até o fim: é a cópia para refazer o passo se algo der errado (`printf '%s' "$ITEM" | rclix LPUSH "sync:dlq:$FILA"` devolve o item à DLQ).

4. **Confira o resultado** pelo `job_id` do passo 2:

   ```bash
   rcli LLEN "sync:queue:$FILA"                              # volta a 0 em segundos com o worker consumindo
   rcli LLEN "sync:dlq:$FILA"
   kubectl -n rmi logs deploy/rmi-sync --since=10m | grep '<job_id>'
   ```

   Sucesso: `sync job succeeded - cache updated and write buffer cleaned` com esse `job_id` (no SigNoz, `k8s.container.name = rmi-sync` e `job_id = <job_id>`). Um novo `job moved to DLQ` com o mesmo `job_id` quer dizer que a causa continua: não insista.

   Job de `citizen` ou `self_declared_*` reprocessado grava no Mongo, mas **não** sincroniza com o Salesforce: a DLQ retira o JWT do job, e o worker registra `skipping salesforce push enqueue: ...` (sem token no job) em vez de enfileirar o `salesforce_push`. É esperado; a próxima alteração ou o próximo login do cidadão (`/v1/auth/validate`) sincroniza de novo.

5. **Mais de um item**: repita os passos 2 a 4 um por vez, olhando o resultado de cada um antes do próximo.

6. **Ao terminar**: `unset VK_PASS ITEM JOB` e feche o terminal. Registre no incidente a fila, os `job_id`, a quantidade e o resultado, sem dados pessoais.
