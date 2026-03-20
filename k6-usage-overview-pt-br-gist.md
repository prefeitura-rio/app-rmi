# Como Usamos k6 para Testes de Carga

Visão geral da nossa infraestrutura de testes de carga com k6 em 5 repositórios.

---

## 📊 Visão Geral

| Repositório | Ambiente de Execução | Análise Automatizada |
|-------------|---------------------|---------------------|
| **app-rmi** | Kubernetes (k6 Operator) | ✅ Scripts Shell + Python |
| **danfe-ai** | VM GCP dedicada | ✅ Pipeline completo (3 scripts) |
| **ai-gateway** | Local (Nix) | ✅ Python charts |
| **superapp** | K8s + Local | ✅ 2 scripts Python |
| **app-eai-agent-gateway** | Local (Docker Compose) | ✅ Python charts |

---

## 🏗️ Padrões de Arquitetura

### 1. **app-rmi** - Kubernetes com k6 Operator

**Stack:**
- ✅ k6 Operator rodando no GKE
- ✅ Execução via GitHub Actions (workflow manual)
- ✅ Paralelização automática em múltiplos pods
- ✅ Recursos configuráveis por pod (CPU / RAM)

**Workflow:**
```
GitHub Actions
    ↓
ConfigMap (scripts k6)
    ↓
TestRun CRD
    ↓
k6 Operator
    ↓
10 pods k6 (paralelos)
    ↓
Logs agregados → GitHub Actions Summary
```

**Análise de métricas:**
- Script Bash coleta métricas do Prometheus (RPS via Istio)
- `kubectl top pod` para CPU/Memory
- Script Python gera gráficos correlacionando carga vs recursos
- **Output:** CSV + PNG com 3 painéis (RPS, CPU, Mem)

**Comandos:**
```bash
# GitHub Actions - manual dispatch
# Workflow: .github/workflows/run-load-tests.yaml

# Análise local de recursos durante teste
./scripts/resource_utilisation/rps_vs_resources.sh
python scripts/resource_utilisation/plot_usage.py rmi_usage_*.csv
```

**Vantagens:**
- 🚀 Escala horizontal automática
- 🔒 Isolamento de recursos
- 📊 Integração com Prometheus/Istio
- ♻️ Cleanup automático

**Desvantagens:**
- ⚙️ Setup complexo (k6 Operator)
- 💰 Custo de infra K8s

---

### 2. **danfe-ai** - VM GCP com Pipeline Completo

**Stack:**
- ✅ VM dedicada: `load-test-danfe` (us-central1-f)
- ✅ Nix + direnv para ambiente reprodutível
- ✅ Token MSAL gerado localmente, atualizado na VM
- ✅ Upload real de arquivos para GCS durante teste

**Workflow:**
```
1. [Local] Gerar token MSAL
        ↓
2. [Local] just refresh-tokens-gcp
        ↓
3. [VM] SSH para load-test-danfe
        ↓
4. [VM] just load-test (configurável)
        ↓
        ├─→ API (processamento)
        └─→ GCS (upload arquivos)
        ↓
5. [VM] just extract-results
        ↓
6. [VM] just plot-results
        ↓
7. [Local] gcloud compute scp (download)
        ↓
8. [Local] Upload Google Drive
```

**Pipeline de análise (3 etapas):**

1. **extract-results.py** - Parseia logs k6
   - Input: `k6-output.log`
   - Output: `test-results.json` (estruturado)

2. **analyze-results.py** - Análise estatística
   - Gera: 4 gráficos PNG + `analysis_report.md`
   - Métricas por endpoint

3. **generate-report.py** - Relatórios detalhados
   - Gera: 10+ gráficos PNG + `load_test_report.md`
   - Histogramas, timelines, distribuições

**Comandos:**
```bash
# Preparação
just refresh-tokens-gcp  # Atualiza tokens na VM

# Execução na VM
gcloud compute ssh load-test-danfe --zone us-central1-f
cd danfe-ai && nix develop
just load-test load-testing/config/test-files/

# Análise
just extract-results
just plot-results

# Download
gcloud compute scp --recurse load-test-danfe:~/danfe-ai/load-testing/results ~/Desktop/
```

**Queries SQL para validação:**
```sql
-- Distribuição de status dos arquivos processados
SELECT das.status, COUNT(*)
FROM danfe_arquivo_solicitacao das
WHERE dsp.data_solicitacao BETWEEN "START" AND "END"
GROUP BY 1;
```

**Vantagens:**
- 💪 Alta capacidade para testes pesados
- 🔐 Isolamento de rede/recursos
- 🔄 Ambiente reprodutível (Nix)
- 💾 Arquivos de teste persistentes

**Desvantagens:**
- 👨‍💻 Processo manual
- 💰 Custo contínuo da VM
- 🔑 Gerenciamento de tokens

---

### 3. **ai-gateway** - Desenvolvimento Local com Nix

**Stack:**
- ✅ Nix flake com k6 + Python + dependências
- ✅ Execução local via `just` commands
- ✅ Testa workflow assíncrono com polling

**Workflow:**
```
just load-test TOKEN
    ↓
k6 run (configurável)
    ↓
results.json (NDJSON)
    ↓
just plot-results
    ↓
generate-charts.py
    ↓
    ├─→ message_completion_histograms.png (grid 2×2)
    ├─→ detailed_analysis.png (percentis + box plots)
    └─→ summary_report.txt (estatísticas)
```

**Outputs de análise:**
- `message_completion_histograms.png` - Grid 2×2 com distribuições
- `detailed_analysis.png` - Percentis + box plots
- `summary_report.txt` - Estatísticas resumidas

**Comandos:**
```bash
# Setup
nix develop

# Executar
just load-test $BEARER_TOKEN

# Analisar
just plot-results

# Visualizar
open load-tests/charts/message_completion_histograms.png
cat load-tests/charts/summary_report.txt
```

**Vantagens:**
- ⚡ Rápido para desenvolvimento
- 📦 Nix garante reprodutibilidade
- 🎯 Simples e direto

**Desvantagens:**
- 💻 Limitado por recursos locais
- 🌐 Não reflete latência de rede real

---

### 4. **superapp** - Híbrido (Local + Kubernetes)

**Stack:**
- ✅ Testes locais via `just` para desenvolvimento
- ✅ Testes K8s via GitHub Actions para staging/prod
- ✅ k6 Operator com auto-scaling de pods
- ✅ Resultados armazenados no GCS

**Workflow K8s:**
```
GitHub Actions (manual dispatch)
    ↓
Parâmetros: peak_vus, duration, env
    ↓
Calcula paralelismo automaticamente
    ↓
k6 Operator → pods k6 distribuídos
    ↓
Coleta logs de todos os pods
    ↓
Upload para GCS
    ↓
gs://rj-superapp-{env}-k6-results/
```

**Workflow Local:**
```
just load-test
    ↓
k6 run (configurável)
    ↓
results.json
    ↓
just analyze-results
    ↓
analyze.py
    ↓
    ├─→ failures.csv (Excel-friendly)
    ├─→ 5 gráficos PNG
    └─→ summary.txt
```

**Análise de resultados:**

**Script 1: analyze.py** (principal)
- Gera: `failures.csv` (Excel-friendly com detalhes de falhas)
- Gera: 5 gráficos PNG:
  - `journey_comparison.png` - Box plot comparativo
  - `response_time_trends.png` - Tendências por endpoint
  - `status_code_distribution.png` - Distribuição de status
  - `percentile_analysis.png` - P50/P75/P90/P95/P99
  - `timeline_by_endpoint.png` - Timeline de requisições
- Gera: `summary.txt` - Estatísticas por jornada

**Script 2: generate_charts.py** (análise de erros)
- `error_timeline.png` - Taxa de erro ao longo do tempo
- `error_distribution.png` - Distribuição de tipos de erro
- `journey_errors.png` - Erros por jornada

**Comandos:**
```bash
# Local
just load-test
just analyze-results

# Fetch do GCS (após teste K8s)
just fetch-results staging
just fetch-results production

# Análise manual
python load-tests/scripts/analyze.py load-tests/data/results.json
```

**GitHub Actions - parâmetros:**
- `target_url` - URL a testar
- `peak_vus` - VUs máximos (default: 100)
- `ramp_up_minutes` - Tempo de ramp-up (default: 2)
- `sustained_minutes` - Tempo sustentado (default: 5)
- `environment` - staging ou production

**Vantagens:**
- 🎭 Flexibilidade (local ou K8s)
- 📊 Análise rica (2 scripts)
- 💾 Histórico no GCS
- 🔀 Simula 4 jornadas de usuário

**Desvantagens:**
- 🧩 Complexidade maior
- 📝 Requer mais setup inicial

---

### 5. **app-eai-agent-gateway** - Local com Docker Compose

**Stack:**
- ✅ Docker Compose para infraestrutura local
- ✅ Testa com RabbitMQ + Redis + Workers
- ✅ Simula produção localmente

**Workflow:**
```
just compose-up
    ↓
RabbitMQ + Redis + Gateway + Workers
    ↓
just load-test TOKEN
    ↓
k6 (configurável) → Gateway
    ↓
    ├─→ Enqueue (RabbitMQ)
    ├─→ Workers processam
    ├─→ Armazenam resultado (Redis)
    └─→ k6 poll resultado (Redis)
    ↓
results.json
    ↓
just plot-results → 3 arquivos
```

**Comandos:**
```bash
# Preparar ambiente
just compose-up

# Executar teste
just load-test $BEARER_TOKEN

# Analisar
just plot-results

# Escalar workers (opcional)
just compose-scale-workers 5

# Verificar logs
just compose-logs worker
```

**Vantagens:**
- 🏠 Testa stack completo localmente
- 🐳 Docker Compose = setup fácil
- 🔧 Permite testar mudanças antes do deploy
- ⚡ Feedback rápido

**Desvantagens:**
- 💻 Limitado por recursos locais
- 🌐 Latência diferente de produção

---

## 🔧 Métricas e Thresholds Comuns

### Métricas Built-in do k6 (todos os repos)

```javascript
http_req_duration      // Duração das requisições
http_req_failed        // Taxa de falha
http_reqs              // Total de requisições
vus                    // Virtual users ativos
data_sent/received     // Tráfego de rede
```

### Métricas Customizadas por Padrão

**Workflow assíncrono** (ai-gateway, app-eai-agent-gateway, danfe-ai):
```javascript
// Tempo de conclusão end-to-end (submit → poll → complete)
message_completion_time
successful_message_completion_time
failed_message_completion_time
message_success_rate
```

**Múltiplas jornadas** (app-rmi, superapp):
```javascript
// Duração por jornada de usuário
journey_duration_{scenario_name}
// Tags: journey, status, method, url
```

### Thresholds Típicos

```javascript
thresholds: {
  'http_req_duration': ['p(95)<3000'],        // 95% sob 3s
  'http_req_failed': ['rate<0.05'],           // < 5% erro
  'message_completion_time': ['p(99)<60000'], // 99% sob 60s
}
```

---

## 📈 Estratégias de Análise

### 1. Análise em Tempo Real (durante teste)

**app-rmi:**
```bash
# Correlaciona k6 com métricas Prometheus
./scripts/resource_utilisation/rps_vs_resources.sh &
# Gera CSV a cada 5s: timestamp, RPS, CPU, Memory
```

### 2. Análise Post-Mortem (após teste)

**Pipeline completo (danfe-ai):**
```bash
just extract-results  # Logs → JSON estruturado
just plot-results     # JSON → Gráficos + Reports
```

**Pipeline simples (ai-gateway, app-eai-agent-gateway):**
```bash
just plot-results     # JSON → 3 arquivos (PNG + TXT)
```

**Pipeline intermediário (superapp):**
```bash
just analyze-results  # JSON → CSV + 5 PNGs + TXT
```

### 3. Tipos de Gráficos Gerados

| Tipo | Repos | Propósito |
|------|-------|-----------|
| **Histogramas** | Todos | Distribuição de tempos de resposta |
| **Box plots** | ai-gateway, superapp | Comparação entre cenários |
| **Timelines** | danfe-ai, superapp | Evolução ao longo do teste |
| **Pie charts** | Todos | Taxa de sucesso/erro, status codes |
| **Scatter plots** | danfe-ai | Duração por status HTTP |
| **Bar charts** | superapp | Percentis, status codes |
| **Correlation plots** | app-rmi | RPS vs CPU/Memory |

---

## 🚀 Padrões de Execução

### GitHub Actions (CI/CD)

**Repos:** app-rmi, superapp

**Estrutura típica:**
```yaml
# Trigger manual
on: workflow_dispatch

# Steps comuns:
1. Autenticar no GCP
2. Configurar kubectl
3. Criar ConfigMap com scripts k6
4. Criar TestRun CRD
5. Aguardar conclusão
6. Coletar logs
7. Upload para GCS/GitHub
8. Cleanup
```

**Vantagens:**
- 🔄 Processo repetível
- 📊 Logs centralizados
- 🔐 Secrets gerenciados pelo GitHub
- 📝 Histórico de execuções

### VM Dedicada

**Repo:** danfe-ai

**Vantagens:**
- 💪 Alta capacidade (1500 VUs estáveis)
- 🔌 Isolamento de rede
- 📦 Ambiente reprodutível (Nix)
- 💾 Arquivos de teste persistentes

**Desvantagens:**
- 👨‍💻 Processo manual
- 💰 Custo contínuo da VM

### Local com Containers

**Repos:** ai-gateway, app-eai-agent-gateway

**Vantagens:**
- ⚡ Rápido para desenvolvimento
- 🏠 Simula produção localmente
- 💸 Sem custo de infra

**Desvantagens:**
- 💻 Limitado por recursos locais
- 🌐 Não reflete latência de rede real

---

## 📊 Exportação de Métricas

### Formato de Saída

**Todos usam JSON (NDJSON):**
```javascript
// k6 run --out json=results.json script.js

// Estrutura:
{"type":"Metric","data":{...},"metric":"http_req_duration"}
{"type":"Point","data":{"time":"...","value":123,...}}
```

### Destinos

| Repo | Destino | Retenção |
|------|---------|----------|
| app-rmi | Logs k6 + GH Summary | Efêmero |
| danfe-ai | Local → Google Drive | Manual |
| ai-gateway | Local | Versionado no repo |
| superapp | GCS buckets | Permanente |
| app-eai-agent-gateway | Local | Efêmero |

### Não Usamos (mas k6 suporta)

❌ InfluxDB
❌ Prometheus Remote Write
❌ Datadog
❌ Grafana Cloud
❌ New Relic

**Motivo:** Análise post-mortem com scripts Python é suficiente para nossas necessidades.

---

## 🛠️ Ferramentas e Dependências

### Ambiente de Execução

| Repo | Gerenciador | Ferramentas |
|------|-------------|-------------|
| app-rmi | Manual | k6, kubectl, Python |
| danfe-ai | Nix flake | k6, Python, Node.js |
| ai-gateway | Nix flake | k6, Python |
| superapp | Nix flake | k6, Python, Just |
| app-eai-agent-gateway | Docker + Just | k6, Python, Go |

### Python - Bibliotecas de Análise

**Comuns a todos:**
```python
matplotlib  # Gráficos
pandas      # Manipulação de dados
numpy       # Cálculos estatísticos
seaborn     # Visualizações avançadas
```

**Específicas:**
```python
# danfe-ai
scipy       # Distribuições estatísticas

# superapp
rich        # Output colorido no terminal
tabulate    # Formatação de tabelas
```

---

## 📝 Configuração de Testes

### Variáveis de Ambiente Comuns

```bash
# Target
BASE_URL=https://staging.example.com

# Autenticação
BEARER_TOKEN=xxx
JWT_TOKEN=xxx

# Configuração de carga
VIRTUAL_USERS=100
DURATION=5m
RAMP_UP_DURATION=2m

# Debug
LOG_LEVEL=INFO  # DEBUG, INFO, WARN, ERROR
```

### Arquivos de Configuração

**Padrão típico:**
```
load-tests/
├── main.js              # Script k6 principal
├── config.js            # Configurações (opcional)
├── journeys/            # Cenários (superapp)
├── scripts/             # Scripts de análise
│   ├── analyze.py
│   └── generate-charts.py
├── data/                # Resultados (.gitignore)
└── charts/              # Gráficos (.gitignore)
```

---

## 🎯 Comandos Rápidos por Repo

### app-rmi
```bash
# Executar via GitHub Actions (manual workflow)

# Análise de recursos durante teste
./scripts/resource_utilisation/rps_vs_resources.sh
python scripts/resource_utilisation/plot_usage.py rmi_usage_*.csv
```

### danfe-ai
```bash
just refresh-tokens-gcp
gcloud compute ssh load-test-danfe --zone us-central1-f
just load-test load-testing/config/test-files/
just extract-results && just plot-results
gcloud compute scp --recurse load-test-danfe:~/danfe-ai/load-testing/results ~/Desktop/
```

### ai-gateway
```bash
nix develop
just load-test $BEARER_TOKEN
just plot-results
```

### superapp
```bash
# Local
just load-test
just analyze-results

# Ou via GitHub Actions (workflow dispatch)

# Fetch resultados K8s
just fetch-results staging
```

### app-eai-agent-gateway
```bash
just compose-up
just load-test $BEARER_TOKEN
just plot-results
```

---

## 🔑 Principais Aprendizados

### ✅ O que funciona bem

**1. Nix para reprodutibilidade** (danfe-ai, ai-gateway, superapp)
- ✅ Mesmas versões de k6, Python, libs
- ✅ Setup automático com `nix develop`
- ✅ Funciona em Linux e macOS

**2. Just para automação** (todos exceto app-rmi)
- ✅ Comandos consistentes entre repos
- ✅ Pipelines complexos simplificados
- ✅ Documentação executável

**3. Scripts Python para análise**
- ✅ Gráficos profissionais com matplotlib/seaborn
- ✅ Fácil extensão e customização
- ✅ CSV para análise em Excel/Sheets
- ✅ Estatísticas detalhadas (percentis, correlações)

**4. k6 Operator para escala** (app-rmi, superapp)
- ✅ Paralelização automática
- ✅ Integração com K8s nativo
- ✅ Isolamento de recursos
- ✅ Cleanup automático

**5. Métricas customizadas**
- ✅ End-to-end completion time (não só HTTP)
- ✅ Tags para análise granular
- ✅ Thresholds específicos por cenário

### 🔄 Oportunidades de Melhoria

**1. Padronizar estrutura de análise**
- 📦 Criar biblioteca Python compartilhada
- 📊 Mesmos tipos de gráficos em todos os repos
- 🎨 Template único de relatórios

**2. CI/CD mais automatizado**
- 🤖 Testes automáticos em PRs importantes
- 📈 Comparação com baseline anterior
- 🚨 Alertas em regressões de performance
- 📊 Dashboard consolidado no Grafana

**3. Centralizar resultados**
- 🗄️ GCS bucket único para todos os repos
- 📊 Dashboard Grafana consolidado
- 📜 Histórico de testes acessível
- 🔍 Busca por timestamp/commit/env

**4. Documentação de playbooks**
- 📅 Quando rodar testes (antes de releases, mudanças críticas)
- 🧐 Como interpretar resultados
- 🎯 Thresholds por tipo de mudança
- 🔧 Troubleshooting comum

**5. Exportar para observability stack**
- 📊 InfluxDB ou Prometheus para métricas históricas
- 📈 Grafana dashboards permanentes
- 🔔 Alerting em degradação de performance

---

## 📊 Comparação de Abordagens

### Quando usar cada padrão?

**k6 Operator (Kubernetes)** → app-rmi, superapp
- ✅ Testes de alta escala (>500 VUs)
- ✅ Testes regulares/automatizados
- ✅ Ambiente já está em K8s
- ❌ Requer setup complexo inicial

**VM Dedicada** → danfe-ai
- ✅ Testes muito pesados
- ✅ Upload de arquivos grandes
- ✅ Testes frequentes no mesmo ambiente
- ❌ Custo contínuo + processo manual

**Local com Nix** → ai-gateway
- ✅ Desenvolvimento iterativo
- ✅ Testes rápidos (<10min)
- ✅ Ambiente reprodutível
- ❌ Limitado por recursos locais

**Local com Docker** → app-eai-agent-gateway
- ✅ Teste de stack completo
- ✅ Desenvolvimento local
- ✅ Validação pré-deploy
- ❌ Pode não refletir produção fielmente

**Híbrido** → superapp
- ✅ Flexibilidade máxima
- ✅ Dev local + CI/CD K8s
- ✅ Histórico permanente (GCS)
- ❌ Mais complexo de manter

---

## 🎓 Padrões de Teste Identificados

### Testes Síncronos vs Assíncronos

**Síncronos** (app-rmi, superapp):
- Request → Response imediata
- Métricas focadas em `http_req_duration`
- Simula navegação de usuário

**Assíncronos** (danfe-ai, ai-gateway, app-eai-agent-gateway):
- Submit → Poll → Complete
- Métricas customizadas de completion time
- Simula processamento background

### Jornadas de Usuário vs Carga Bruta

**Jornadas** (app-rmi, superapp):
- Múltiplos cenários com comportamentos diferentes
- Think times entre ações
- Distribuição realista de uso
- Exemplo: 35% Service Seeker, 25% Browser, etc.

**Carga Bruta** (danfe-ai, ai-gateway, app-eai-agent-gateway):
- Foco em volume e throughput
- Menos preocupação com realismo
- Objetivo: estressar sistema

---

## 📚 Recursos

### Documentação k6
- [Documentação oficial](https://k6.io/docs/)
- [k6 Operator](https://github.com/grafana/k6-operator)
- [Métricas customizadas](https://k6.io/docs/using-k6/metrics/)
- [Thresholds](https://k6.io/docs/using-k6/thresholds/)

### Repos com README detalhado
- `app-rmi/k6/README.md`
- `danfe-ai/load-testing/README.md`
- `superapp/load-tests/README.md`
- `superapp/load-tests/K8S_SETUP.md`

### Scripts de referência
- Análise completa: `danfe-ai/load-testing/scripts/`
- Correlação de recursos: `app-rmi/scripts/resource_utilisation/`
- Jornadas de usuário: `superapp/load-tests/journeys/`
- Charts profissionais: `ai-gateway/load-tests/generate-charts.py`

### Exemplos de configuração
- k6 Operator TestRun: `app-rmi/.github/scripts/templates/testrun.yaml`
- GitHub Actions workflow: `superapp/.github/workflows/k6-load-test.yaml`
- Nix flake: `ai-gateway/flake.nix`
- Docker Compose: `app-eai-agent-gateway/docker-compose.yml`

---

## 🤝 Contribuindo

### Melhorando os testes

**Antes de adicionar novos testes:**
1. Revise os padrões existentes acima
2. Escolha a abordagem que melhor se adequa
3. Reutilize scripts de análise existentes
4. Documente no README do repo

**Padrões a seguir:**
- ✅ Use `just` para comandos
- ✅ Export para JSON (NDJSON)
- ✅ Scripts Python para análise
- ✅ Thresholds explícitos
- ✅ .gitignore para resultados

**Evite:**
- ❌ Hardcoded URLs em scripts
- ❌ Secrets commitados
- ❌ Resultados versionados (exceto exemplos pequenos)
- ❌ Dependências não documentadas

---

**Última atualização:** 2025-02-03
**Mantenedor:** Time de Plataforma
**Dúvidas:** Abrir issue no repo relevante ou perguntar no Slack #plataforma
