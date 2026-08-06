# API RMI

[![Coverage](https://img.shields.io/endpoint?url=https://prefeitura-rio.github.io/app-rmi/coverage-badge.json)](https://github.com/prefeitura-rio/app-rmi/actions/workflows/coverage-baseline.yaml)

API para gerenciamento de dados de cidadãos do Rio de Janeiro, incluindo autodeclaração de informações e verificação de contato.

## Funcionalidades

- 🔍 Consulta de dados do cidadão por CPF
- 🔄 Atualização de dados autodeclarados com validação
- 📱 Verificação de número de telefone via WhatsApp
- 💾 Cache Redis para melhor performance
- 📊 Métricas Prometheus para monitoramento
- 🔍 Rastreamento de requisições com OpenTelemetry
- 📝 Logs estruturados com Zap
- 🗂️ Gerenciamento automático de índices MongoDB
- 🔒 Sistema de auditoria completo
- ⚡ Controle de concorrência com optimistic locking
- ✅ Validação de dados abrangente
- 🔄 Transações de banco de dados
- 🧹 Limpeza automática de dados expirados
- 📞 Suporte a WhatsApp bot com phone-based endpoints
- 🔐 Sistema de opt-in/opt-out com histórico detalhado
- 📋 Validação de registros contra dados base
- 🎯 Mapeamento phone-CPF com controle de status
- 🚫 Sistema de quarentena de telefones com TTL configurável
- 🧪 Sistema de whitelist beta para chatbot com grupos
- 🏥 **CF Lookup Automático**: Busca automática de Clínica da Família via integração MCP
- 🔍 **Tracing e Monitoramento de Performance**: Sistema abrangente de observabilidade com OpenTelemetry e SignOz

## Variáveis de Ambiente

| Variável | Descrição | Padrão | Obrigatório |
|----------|-----------|---------|------------|
| PORT | Porta do servidor | 8080 | Não |
| MONGODB_URI | String de conexão MongoDB | mongodb://localhost:27017 | Sim |
| MONGODB_DATABASE | Nome do banco de dados MongoDB | citizen_data | Não |
| MONGODB_CITIZEN_COLLECTION | Nome da coleção de dados do cidadão | citizens | Não |
| MONGODB_SELF_DECLARED_COLLECTION | Nome da coleção de dados autodeclarados | self_declared | Não |
| MONGODB_PHONE_VERIFICATION_COLLECTION | Nome da coleção de verificação de telefone | phone_verifications | Não |
| MONGODB_MAINTENANCE_REQUEST_COLLECTION | Nome da coleção de chamados do 1746 | - | Sim |
| MONGODB_USER_CONFIG_COLLECTION | Nome da coleção de configurações do usuário | user_config | Não |
| MONGODB_PHONE_MAPPING_COLLECTION | Nome da coleção de mapeamentos phone-CPF | phone_cpf_mappings | Não |
| MONGODB_OPT_IN_HISTORY_COLLECTION | Nome da coleção de histórico opt-in/opt-out | opt_in_history | Não |
| MONGODB_BETA_GROUP_COLLECTION | Nome da coleção de grupos beta | beta_groups | Não |
| MONGODB_AUDIT_LOGS_COLLECTION | Nome da coleção de logs de auditoria | audit_logs | Não |
| PHONE_QUARANTINE_TTL | TTL da quarentena de telefones (ex: "4320h" = 6 meses) | 4320h | Não |
| BETA_STATUS_CACHE_TTL | TTL do cache de status beta (ex: "24h") | 24h | Não |
| REDIS_URI | String de conexão Redis | redis://localhost:6379 | Sim |
| REDIS_TTL | TTL do cache Redis em minutos | 60 | Não |
| PHONE_VERIFICATION_TTL | TTL dos códigos de verificação de telefone (ex: "15m", "1h") | 15m | Não |
| WHATSAPP_ENABLED | Habilita/desabilita o envio de mensagens WhatsApp | true | Não |
| WHATSAPP_API_BASE_URL | URL base da API do WhatsApp | - | Sim |
| WHATSAPP_API_USERNAME | Usuário da API do WhatsApp | - | Sim |
| WHATSAPP_API_PASSWORD | Senha da API do WhatsApp | - | Sim |
| WHATSAPP_HSM_ID | ID do template HSM do WhatsApp | - | Sim |
| WHATSAPP_COST_CENTER_ID | ID do centro de custo do WhatsApp | - | Sim |
| WHATSAPP_CAMPAIGN_NAME | Nome da campanha do WhatsApp | - | Sim |
| VERIFICATION_PROVIDER | Provedor de disparo do código OTP (`wetalkie` ou `salesforce`) | wetalkie | Não |
| SFMC_ENABLED | Habilita/desabilita o envio via Salesforce Marketing Cloud | true | Não |
| SFMC_SUBDOMAIN | Subdomínio do tenant do Marketing Cloud (ex: `mcpv1-xxxx`) | - | Sim (se salesforce) |
| SFMC_CLIENT_ID | Client ID da API do Marketing Cloud | - | Sim (se salesforce) |
| SFMC_CLIENT_SECRET | Client Secret da API do Marketing Cloud | - | Sim (se salesforce) |
| SFMC_ACCOUNT_ID | Account ID (MID) do Marketing Cloud | - | Sim (se salesforce) |
| SFMC_DEFINITION_KEY | Definition Key da OTT definition do Marketing Cloud | - | Sim (se salesforce) |
| SFMC_OTP_ATTRIBUTE | Nome do atributo do código OTP no payload OTT | codigo_otp | Não |
| MCP_SERVER_URL | URL do servidor MCP para lookup de CF | https://services.pref.rio/mcp/mcp/ | Não |
| MCP_AUTH_TOKEN | Token de autenticação do servidor MCP | - | Não* |
| DATA_RELAY_BASE_URL | URL base da API Data Relay (envio de e-mail via mailman) | - | Não* |
| DATA_RELAY_API_KEY | API key do Data Relay (`X-Api-Key`) | - | Não* |
| DATA_RELAY_TIMEOUT | Timeout HTTP do Data Relay | 30s | Não |
| MOBILIDADE_INVITE_DEEP_LINK_BASE | Base do deep link de convite Mobilidade | https://pref.rio | Não |
| CF_LOOKUP_COLLECTION | Nome da coleção de lookups de CF | cf_lookups | Não |
| CF_LOOKUP_CACHE_TTL | TTL do cache de CF lookups (ex: "24h") | 24h | Não |
| CF_LOOKUP_RATE_LIMIT | Rate limit por CPF para CF lookups (ex: "1h") | 1h | Não |
| CF_LOOKUP_GLOBAL_RATE_LIMIT | Rate limit global de CF lookups por minuto | 60 | Não |
| LOG_LEVEL | Nível de log (debug, info, warn, error) | info | Não |
| METRICS_PORT | Porta para métricas Prometheus | 9090 | Não |
| TRACING_ENABLED | Habilitar rastreamento OpenTelemetry | false | Não |
| TRACING_ENDPOINT | Endpoint do coletor OpenTelemetry | http://localhost:4317 | Não |
| AUDIT_LOGS_ENABLED | Habilitar logs de auditoria automáticos | true | Não |
| AUDIT_WORKER_COUNT | Número de workers para logging assíncrono | 20 | Não |
| AUDIT_BUFFER_SIZE | Tamanho do buffer para audit logs | 10000 | Não |
| VERIFICATION_WORKER_COUNT | Número de workers para verificação de telefone | 10 | Não |
| VERIFICATION_QUEUE_SIZE | Tamanho da fila de verificação | 5000 | Não |
| DB_WORKER_COUNT | Número de workers para operações de banco | 10 | Não |
| DB_BATCH_SIZE | Tamanho do lote para operações em lote | 100 | Não |
| INDEX_MAINTENANCE_INTERVAL | Intervalo para verificação de índices (ex: "1h", "24h") | 1h | Não |
| WHATSAPP_COD_PARAMETER | Parâmetro do código no template HSM do WhatsApp | COD | Não |

**Notas:**
- `*` MCP_AUTH_TOKEN é obrigatório apenas se a funcionalidade de CF lookup estiver habilitada
- `*` DATA_RELAY_BASE_URL e DATA_RELAY_API_KEY devem ser ambos definidos para envio real de e-mails (convites Mobilidade). Sem eles, o sync worker apenas registra o e-mail em log.

## 🏥 **CF (Clínica da Família) Lookup - Nova Funcionalidade**

### **Visão Geral**
Sistema automático de busca de Clínica da Família integrado ao MCP (Model Context Protocol) do Rio de Janeiro. Funciona de forma Netflix-style, buscando automaticamente a CF mais próxima para cidadãos que não possuem dados de CF nos registros base.

### **Funcionalidades**
- ⚡ **Lookup Automático**: Triggers automáticos quando `saude.clinica_familia.indicador = false`
- 🏠 **Baseado em Endereço**: Usa endereços self-declared ou base data para busca
- 🔄 **Background Processing**: Operações via sync worker (não bloqueia API)
- 💾 **Multi-Level Caching**: Redis + MongoDB com TTL configurável
- 🔍 **Address Fingerprinting**: SHA256 hashing para detecção de mudanças
- 🔄 **Retry Logic**: Exponential backoff com error categorization
- 🛡️ **Rate Limiting**: Token bucket global + per-CPF cooldown
- 📊 **Observabilidade**: Integração completa com logging e tracing

### **Fluxo de Operação**
1. **Trigger**: Usuário sem CF acessa `/citizen/{cpf}` 
2. **Verificação**: Sistema verifica endereço disponível
3. **Background Job**: Queue job para lookup via MCP
4. **MCP Integration**: Busca CF via protocolo JSON-RPC 2.0
5. **Storage**: Armazena resultado linkado ao endereço
6. **Cache**: Redis cache para futuras consultas
7. **Invalidation**: Mudança de endereço invalida CF anterior

### **Configuração**
```bash
# MCP Server (Rio de Janeiro)
MCP_SERVER_URL=https://services.pref.rio/mcp/mcp/
MCP_AUTH_TOKEN=your_token_here

# Performance Settings  
CF_LOOKUP_CACHE_TTL=24h
CF_LOOKUP_RATE_LIMIT=1h
CF_LOOKUP_GLOBAL_RATE_LIMIT=60

# Database
CF_LOOKUP_COLLECTION=cf_lookups
```

### **Monitoramento**
- **Logs Estruturados**: Performance tracking com duração de operações
- **Error Categorization**: Network, timeout, authorization, validation
- **Rate Limiting Stats**: Token bucket status e per-CPF cooldowns
- **Observability**: Integração com OpenTelemetry existente

## 🚀 **Otimização de Performance MongoDB - IMPLEMENTADA**

### **Configuração Code-Based (Recomendada)**

Para máxima performance e flexibilidade, **todas as configurações MongoDB são feitas via código**, permitindo ajuste fácil através de variáveis de ambiente sem conflitos de código.

### **✅ Otimizações Implementadas**

#### **1. Connection Pool Optimization**
- **minPoolSize**: 50 (conexões quentes)
- **maxPoolSize**: 1000 (alto throughput)
- **maxConnecting**: 100 (conexões concorrentes)
- **maxIdleTime**: 2 minutos (rotação mais rápida)

#### **2. Compression Optimization**
- **Compressor**: Snappy (em vez de Zlib level 6)
- **CPU Reduction**: 15-25% menos uso de CPU
- **Network**: Eficiência mantida com menos overhead

#### **3. Write Concern Optimization**
- **W=0**: Citizen, UserConfig, PhoneMapping, OptInHistory, BetaGroup, PhoneVerification, MaintenanceRequest, AuditLogs
- **W=1**: SelfDeclared (integridade de dados)
- **Performance**: 40-60% melhoria em cenários de alta escrita

#### **4. Timeout Optimization**
- **connectTimeout**: 2s (reduzido de 3s)
- **serverSelectionTimeout**: 1s (reduzido de 2s)
- **socketTimeout**: 15s (reduzido de 25s)
- **Failover**: Mais rápido e agressivo

#### **5. Batch Operations**
- **Audit Logs**: Processamento em lotes de 100
- **Phone Verifications**: Operações em lote para resultados
- **Phone Mappings**: Inserções e atualizações em lote
- **Performance**: 50-80% melhoria para operações em lote

#### **6. Index Optimization**
- **Removidos**: 8 índices desnecessários de coleções write-heavy
- **Mantidos**: Apenas índices essenciais para consultas
- **Impacto**: Melhor performance de escrita sem perda de funcionalidade

#### **URI Simplificada (Configuração via Código)**
```bash
mongodb://root:PASSWORD@mongodb-0.mongodb-headless.rmi.svc.cluster.local:27017,mongodb-1.mongodb-headless.rmi.svc.cluster.local:27017/?replicaSet=rs0&authSource=admin
```

**✅ VANTAGEM**: Todas as otimizações de performance são configuradas via código, tornando a URI mais limpa e manutenível.

**🔧 Configurações Aplicadas Automaticamente**:
- Connection pool: minPoolSize=50, maxPoolSize=1000
- Compression: Snappy
- Timeouts: connectTimeout=2s, serverSelectionTimeout=1s
- Write concerns: W=0 para performance, W=1 para integridade
- Read preference: nearest

### **Parâmetros de Performance Implementados**

| Parâmetro | Valor | Impacto | Status |
|-----------|-------|---------|---------|
| `minPoolSize` | 50 | **Conexões quentes** | ✅ **Implementado** |
| `maxPoolSize` | 1000 | **Alto throughput** | ✅ **Implementado** |
| `maxConnecting` | 100 | **Conexões concorrentes** | ✅ **Implementado** |
| `maxIdleTime` | 2min | **Rotação mais rápida** | ✅ **Implementado** |
| `compression` | snappy | **Menos CPU** | ✅ **Implementado** |
| `connectTimeout` | 2s | **Failover rápido** | ✅ **Implementado** |
| `serverSelectionTimeout` | 1s | **Seleção rápida** | ✅ **Implementado** |
| `socketTimeout` | 15s | **Timeout otimizado** | ✅ **Implementado** |
| `writeConcern` | W=0/W=1 | **Performance vs integridade** | ✅ **Implementado** |
| `readPreference` | nearest | **Distribuição de carga** | ✅ **Implementado** |

### **Vantagens da Abordagem Code-Based**

- **✅ Sem conflitos**: Configuração centralizada no código
- **✅ Flexibilidade**: Ajuste via variáveis de ambiente
- **✅ Performance**: Otimizações aplicadas automaticamente
- **✅ Manutenção**: Uma única fonte de verdade
- **✅ Escalabilidade**: Fácil ajuste para diferentes ambientes
- **✅ Versionamento**: Configurações versionadas no código
- **✅ Debugging**: Mais fácil de debugar e monitorar

## 🚀 **Arquitetura Multi-Level Cache - IMPLEMENTADA**

### **Visão Geral**

A API RMI agora implementa uma **arquitetura de cache em múltiplas camadas** que melhora dramaticamente a performance sob cargas pesadas de escrita. Em vez de escrever diretamente no MongoDB (que pode ser lento), o sistema agora:

1. **Escreve no Redis primeiro** (resposta rápida)
2. **Enfileira jobs de sincronização** para processamento em background
3. **Lê das camadas de cache** antes de recorrer ao MongoDB
4. **Sincroniza com MongoDB assincronamente** via workers dedicados

### **🏗️ Arquitetura**

```
┌─────────────────┐    ┌─────────────────┐
│   API Service   │    │  Sync Service   │
│  (Escritas Rápidas) │ (Background)    │
└─────────────────┘    └─────────────────┘
         │                       │
         ▼                       ▼
    ┌─────────────────────────────────┐
    │           Redis                 │
    │  ┌─────────────┐ ┌─────────────┐│
    │  │ Write Buffer│ │ Job Queues  ││
    │  │ (24h TTL)   │ │ (Sync Jobs) ││
    │  └─────────────┘ └─────────────┘│
    └─────────────────────────────────┘
                       │
                       ▼
                  ┌─────────────┐
                  │  MongoDB    │
                  │ (Durabilidade) │
                  └─────────────┘
```

### **✅ Componentes Implementados**

#### **1. DataManager (`internal/services/data_manager.go`)**
- **Write**: Escreve no Redis write buffer e enfileira job de sync
- **Read**: Lê do write buffer → read cache → MongoDB (fallback)
- **Delete**: Remove de todas as camadas de cache e MongoDB
- **Cache Management**: Gerencia TTL e limpeza

#### **2. SyncService (`internal/services/sync_service.go`)**
- **Worker Pool**: Número configurável de sync workers
- **Queue Processing**: Processa jobs das filas Redis
- **Error Handling**: Lógica de retry com exponential backoff
- **Dead Letter Queue**: Jobs falhados após max retries

#### **3. SyncWorker (`internal/services/sync_worker.go`)**
- **Job Processing**: Converte dados Redis para documentos MongoDB
- **Upsert Operations**: Gerencia inserções e atualizações
- **Cache Cleanup**: Remove do write buffer após sync bem-sucedido
- **Cache Update**: Atualiza read cache com dados sincronizados

#### **4. DegradedMode (`internal/services/degraded_mode.go`)**
- **MongoDB Health**: Monitora conectividade MongoDB
- **Redis Memory**: Verifica uso de memória Redis (>85% ativa modo degradado)
- **Service Protection**: Previne novas escritas quando sistema está estressado

#### **5. Metrics (`internal/services/metrics.go`)**
- **Queue Depths**: Número de jobs em cada fila
- **Sync Operations**: Contadores de sucesso/falha
- **Cache Performance**: Taxas de hit/miss
- **System Health**: Status do modo degradado

### **📊 Fluxo de Dados**

#### **Operação de Escrita**

1. **API Request**: Cliente envia requisição de escrita
2. **Redis Write**: Dados escritos no Redis write buffer (24h TTL)
3. **Job Queue**: Job de sync enfileirado no Redis
4. **Immediate Response**: API responde imediatamente (rápido)
5. **Background Sync**: Sync worker processa job assincronamente
6. **MongoDB Update**: Dados sincronizados com MongoDB
7. **Cache Cleanup**: Write buffer limpo
8. **Read Cache Update**: Read cache atualizado com dados finais

#### **Operação de Leitura**

1. **Cache Check**: Verifica Redis write buffer primeiro (dados mais recentes)
2. **Read Cache**: Verifica Redis read cache (1h TTL)
3. **MongoDB Fallback**: Se não estiver em cache, lê do MongoDB
4. **Cache Update**: Atualiza read cache para requisições futuras

### **🚀 Benefícios de Performance**

#### **Melhorias Esperadas**
- **Write Performance**: 40-60% melhoria com W=0 write concern
- **Response Time**: Resposta imediata para escritas (Redis)
- **Throughput**: 30-50% aumento em cenários de alta escrita
- **Scalability**: Escalabilidade independente dos sync workers

#### **Performance do Cache**
- **Write Buffer**: 24 horas TTL para escritas pendentes
- **Read Cache**: 1 hora TTL para dados frequentemente acessados
- **Hit Ratio**: Meta de 80-90% de taxa de cache hit
- **Memory Usage**: Limites de memória Redis configuráveis

### **🔧 Configuração**

#### **Variáveis de Ambiente**
```bash
# Configuração dos workers de banco de dados
DB_WORKER_COUNT=10      # Número de sync workers
DB_BATCH_SIZE=100       # Tamanho do lote para operações

# Configuração Redis
REDIS_URI=redis://localhost:6379
REDIS_POOL_SIZE=100
REDIS_MIN_IDLE_CONNS=10

# Configuração MongoDB
MONGODB_URI=mongodb://localhost:27017
MONGODB_DATABASE=rmi

# Configuração CF Lookup (Clínica da Família)
MCP_SERVER_URL=https://services.pref.rio/mcp/mcp/
MCP_AUTH_TOKEN=your_mcp_auth_token_here
CF_LOOKUP_CACHE_TTL=24h
CF_LOOKUP_RATE_LIMIT=1h
CF_LOOKUP_GLOBAL_RATE_LIMIT=60
```

#### **Configuração das Coleções**
Coleções são configuradas com diferentes write concerns:

- **Performance Collections** (W=0): `citizens`, `phone_mappings`, `user_configs`, etc.
- **Data Integrity Collections** (W=1): `self_declared`

### **💻 Exemplos de Uso**

#### **Operações Básicas de Dados**
```go
// Criar data manager
dataManager := services.NewDataManager(redis, mongo, logger)

// Operação de escrita
op := &services.CitizenDataOperation{
    CPF:  "12345678901",
    Data: citizenData,
}
err := dataManager.Write(ctx, op)

// Operação de leitura
var citizen models.Citizen
err = dataManager.Read(ctx, "12345678901", "citizens", "citizen", &citizen)
```

#### **Integração de Serviços**
```go
// Criar serviço de cache do cidadão
citizenService := services.NewCitizenCacheService()

// Atualizar cidadão (escrita rápida)
err := citizenService.UpdateCitizen(ctx, cpf, citizenData)

// Obter cidadão (leitura em cache)
citizen, err := citizenService.GetCitizen(ctx, cpf)
```

### **🚨 Modo Degradado**

O sistema entra automaticamente em **modo degradado** quando:

- **MongoDB está down**
- **Uso de memória Redis > 85%**

No modo degradado:
- ✅ **Leituras continuam** (do cache)
- ❌ **Novas escritas são prevenidas**
- 🔄 **Recuperação é automática**

### **📈 Métricas**

Todas as métricas são prefixadas com `rmi_`:

- `rmi_sync_queue_depth_{queue}`: Profundidade das filas
- `rmi_sync_operations_total_{queue}`: Operações de sync
- `rmi_sync_failures_total_{queue}`: Falhas de sync
- `rmi_cache_hit_ratio_{cache_type}`: Performance do cache
- `rmi_degraded_mode_active`: Saúde do sistema

### **🔍 Troubleshooting**

#### **Problemas Comuns**

1. **Alta Profundidade de Fila**: Aumentar `DB_WORKER_COUNT`
2. **Alta Taxa de Falha**: Verificar conectividade MongoDB
3. **Problemas de Memória**: Monitorar uso de memória Redis
4. **Modo Degradado**: Verificar saúde MongoDB e memória Redis

#### **Comandos de Debug**
```bash
# Verificar filas Redis
redis-cli LLEN sync:queue:citizen

# Verificar DLQ
redis-cli LLEN sync:dlq:citizen

# Monitorar sync workers
ps aux | grep sync

# Verificar conexão MongoDB
mongo --eval "db.runCommand('ping')"
```

### **🔄 Migração**

#### **De MongoDB Direto**

1. **Atualizar Serviços**: Substituir chamadas MongoDB diretas por DataManager
2. **Data Operations**: Implementar interface DataOperation para seus modelos
3. **Service Updates**: Atualizar métodos de serviço para usar cache service
4. **Testing**: Verificar comportamento do cache e operações de sync

#### **Rollout Gradual**

1. **Read-Only**: Começar com cache de leitura apenas
2. **Write Buffer**: Habilitar buffer de escrita para operações não-críticas
3. **Full Sync**: Habilitar sync completo para todas as operações
4. **Monitoring**: Monitorar performance e ajustar contagem de workers

### **📚 Documentação**

- **Quick Start**: `README_CACHE_SYSTEM.md`
- **Full Details**: `docs/MULTI_LEVEL_CACHE.md`
- **Code Examples**: `internal/services/citizen_cache_service.go`

### **🎉 O que é Novo**

✅ **Multi-level caching** com Redis write buffer e read cache  
✅ **Sincronização assíncrona MongoDB** via workers dedicados  
✅ **Modo degradado** para proteção do sistema  
✅ **Métricas abrangentes** com prefixo RMI  
✅ **Lógica de retry** com exponential backoff  
✅ **Dead letter queue** para jobs falhados  
✅ **Escalabilidade independente** dos sync workers  
✅ **Scripts de startup fáceis** para desenvolvimento  

### **🚀 Próximos Passos**

1. **Iniciar os serviços**: `just start-services`
2. **Executar o demo**: `just demo-cache`
3. **Monitorar performance**: Verificar chaves Redis e profundidade das filas
4. **Integrar com seu código**: Usar DataManager e cache services
5. **Escalar workers**: Ajustar `DB_WORKER_COUNT` baseado na carga

---

**Feliz caching! 🎯** O sistema é projetado para lidar com altas cargas de escrita mantendo integridade de dados e fornecendo excelente performance.

### **🔧 Otimizações de Connection Pool**

#### **Problema Resolvido: Connection Pool Exhaustion**
```
failed to insert audit log: canceled while checking out a connection from connection pool
context canceled; total connections: 333, maxPoolSize: 1000, idle connections: 0, wait duration: 15.807719752s
```

#### **Soluções Implementadas**

1. **Audit Logging Assíncrono**
   - **Worker pool** com 5 workers dedicados
   - **Buffer de 1000** logs para picos de tráfego
   - **Não bloqueia** operações principais
   - **Fallback síncrono** se buffer estiver cheio

2. **Connection Pool Monitoring**
   - **Monitoramento em tempo real** do pool de conexões
   - **Alertas** quando uso > 100 conexões
   - **Logs detalhados** de aquisição/retorno de conexões
   - **Verificação a cada 30s** do status do pool

3. **Configuração Otimizada para Alta Performance**
   ```bash
   # Workers para audit logging (aumentado para alta performance)
   AUDIT_WORKER_COUNT=20
   
   # Buffer size para audit logs (aumentado para picos de tráfego)
   AUDIT_BUFFER_SIZE=10000
   
   # Monitoramento de conexões
   # Automático a cada 30s
   ```

## 🔧 **MongoDB Cluster Configuration - Helm Parameters**

### **Configuração Recomendada para Helm**

Aqui estão os parâmetros específicos de configuração MongoDB que você pode definir via Helm values:

```yaml
# MongoDB Helm values.yaml
mongodb:
  # WiredTiger Engine Settings
  extraFlags:
    - "--wiredTigerCacheSizeGB=2"
    - "--wiredTigerJournalCompressor=snappy"
    - "--wiredTigerCollectionBlockCompressor=snappy"
    - "--wiredTigerIndexPrefixCompression=true"
  
  # Transaction and Lock Settings
  extraFlags:
    - "--setParameter=transactionLifetimeLimitSeconds=60"
    - "--setParameter=maxTransactionLockRequestTimeoutMillis=5000"
    - "--setParameter=logLevel=1"
  
  # Network and Compression Settings
  extraFlags:
    - "--networkMessageCompressors=snappy"
    - "--compressors=snappy"
  
  # Memory and Performance Settings
  extraFlags:
    - "--maxConns=2000"
    - "--maxInMemorySort=100"
    - "--wiredTigerConcurrentReadTransactions=128"
    - "--wiredTigerConcurrentWriteTransactions=128"
  
  # Journal Settings
  extraFlags:
    - "--journalCommitInterval=100"
    - "--wiredTigerCheckpointDelaySecs=60"
  
  # Query Optimization
  extraFlags:
    - "--setParameter=enableLocalhostAuthBypass=false"
    - "--setParameter=enableTestCommands=false"
    - "--setParameter=diagnosticDataCollectionEnabled=false"
```

**Ou como valores individuais do Helm:**
```yaml
mongodb:
  # Cache and Memory
  wiredTigerCacheSizeGB: 2
  wiredTigerJournalCompressor: "snappy"
  wiredTigerCollectionBlockCompressor: "snappy"
  
  # Transactions
  transactionLifetimeLimitSeconds: 60
  maxTransactionLockRequestTimeoutMillis: 5000
  
  # Compression
  networkMessageCompressors: ["snappy"]
  compressors: ["snappy"]
  
  # Connections
  maxConns: 2000
  
  # Performance
  journalCommitInterval: 100
  wiredTigerCheckpointDelaySecs: 60
```

## 🚀 **Redis Scaling & Performance**

### **Configuração de Connection Pool Otimizada**

#### **Configuração Atual (Produção)**
```bash
# Redis connection pool configuration
REDIS_POOL_SIZE=50           # Aumentado de 10 para 50
REDIS_MIN_IDLE_CONNS=20      # Aumentado de 5 para 20
REDIS_DIAL_TIMEOUT=2s        # Reduzido de 5s para 2s
REDIS_READ_TIMEOUT=1s        # Reduzido de 3s para 1s
REDIS_WRITE_TIMEOUT=1s       # Reduzido de 3s para 1s
REDIS_POOL_TIMEOUT=2s        # Timeout para obter conexão
REDIS_COMMAND_TIMEOUT=2s     # Timeout geral de comandos
```

#### **Estratégias de Scaling Redis**

1. **Horizontal Scaling (Recomendado)**
   ```yaml
   # k8s/staging/resources.yaml - Redis Cluster
   apiVersion: apps/v1
   kind: StatefulSet
   metadata:
     name: redis-cluster
   spec:
     replicas: 3  # Aumentar de 1 para 3
     template:
       spec:
         containers:
         - name: redis
           image: redis:7.2-alpine
           command: ["redis-server", "/etc/redis/redis.conf"]
           ports:
           - containerPort: 6379
           resources:
             requests:
               memory: "256Mi"
               cpu: "250m"
             limits:
               memory: "512Mi"
               cpu: "500m"
   ```

2. **Redis Sentinel para Alta Disponibilidade**
   ```yaml
   # Redis Sentinel Configuration
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: redis-sentinel-config
   data:
     sentinel.conf: |
       port 26379
       sentinel monitor mymaster redis-master 6379 2
       sentinel down-after-milliseconds mymaster 5000
       sentinel failover-timeout mymaster 10000
       sentinel parallel-syncs mymaster 1
   ```

3. **Redis Cluster para Sharding**
   ```bash
   # Redis Cluster com 6 nós (3 master + 3 replica)
   kubectl apply -f - <<EOF
   apiVersion: apps/v1
   kind: StatefulSet
   metadata:
     name: redis-cluster
   spec:
     serviceName: redis-cluster
     replicas: 6
     template:
       spec:
         containers:
         - name: redis
           image: redis:7.2-alpine
           command: ["redis-server", "/etc/redis/redis.conf", "--cluster-enabled", "yes"]
   EOF
   ```

### **Monitoramento Redis em SignOz**

#### **Métricas Disponíveis**
```yaml
# Redis Connection Pool Metrics
app_rmi_redis_connection_pool{status="total", uri="redis-master"}
app_rmi_redis_connection_pool{status="idle", uri="redis-master"}
app_rmi_redis_connection_pool{status="stale", uri="redis-master"}

# Redis Connection Pool Configuration
app_rmi_redis_connection_pool_size{type="max", uri="redis-master"}
app_rmi_redis_connection_pool_size{type="min_idle", uri="redis-master"}

# Redis Operation Metrics
app_rmi_redis_operations_total{operation="get", status="success"}
app_rmi_redis_operation_duration_seconds{operation="get"}
```

#### **Alertas Automáticos**
- **High Usage**: > 80% do pool size
- **Critical Usage**: > 90% do pool size
- **No Idle Connections**: 0 conexões ociosas
- **Stale Connections**: Conexões antigas detectadas

### **Otimizações de Performance**

1. **Connection Pool Tuning**
   ```bash
   # Para produção com alto tráfego
   export REDIS_POOL_SIZE=100
   export REDIS_MIN_IDLE_CONNS=50
   export REDIS_POOL_TIMEOUT=1s
   export REDIS_COMMAND_TIMEOUT=1s
   ```

2. **Redis Memory Optimization**
   ```bash
   # redis.conf
   maxmemory 512mb
   maxmemory-policy allkeys-lru
   save 900 1
   save 300 10
   save 60 10000
   ```

3. **Network Optimization**
   ```bash
   # Kubernetes Service
   apiVersion: v1
   kind: Service
   metadata:
     name: redis-master
     annotations:
       service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
   spec:
     type: LoadBalancer
     ports:
     - port: 6379
       targetPort: 6379
     selector:
       app: redis
   ```

### **Estratégias de Fallback**

1. **Circuit Breaker Pattern**
   ```go
   // Implementado automaticamente via Redis client
   // PoolTimeout: 2s - Falha rápido se pool estiver cheio
   // MaxRetries: 3 - Retry automático de comandos falhados
   ```

2. **Graceful Degradation**
   ```go
   // Cache miss não bloqueia operações principais
   // Fallback para MongoDB se Redis indisponível
   // Logs de auditoria assíncronos
   ```

3. **Health Checks**
   ```yaml
   # Kubernetes Liveness Probe
   livenessProbe:
     exec:
       command:
       - redis-cli
       - ping
     initialDelaySeconds: 30
     periodSeconds: 10
   ```

### **Troubleshooting Redis**

#### **Problemas Comuns e Soluções**

1. **Connection Pool Exhaustion**
   ```bash
   # Verificar métricas
   kubectl exec -it redis-master -- redis-cli info clients
   
   # Aumentar pool size
   export REDIS_POOL_SIZE=100
   ```

2. **High Latency**
   ```bash
   # Verificar rede
   kubectl exec -it redis-master -- redis-cli --latency
   
   # Verificar memória
   kubectl exec -it redis-master -- redis-cli info memory
   ```

3. **Memory Pressure**
   ```bash
   # Verificar uso de memória
   kubectl exec -it redis-master -- redis-cli info memory
   
   # Limpar cache se necessário
   kubectl exec -it redis-master -- redis-cli flushall
   ```

#### **Comandos de Debug**
```bash
# Verificar status do cluster
kubectl exec -it redis-master -- redis-cli cluster info

# Verificar nós do cluster
kubectl exec -it redis-master -- redis-cli cluster nodes

# Verificar slots de hash
kubectl exec -it redis-master -- redis-cli cluster slots

# Monitorar comandos em tempo real
kubectl exec -it redis-master -- redis-cli monitor
```

## 🔍 **MongoDB Connection Pool Optimization**

### **Configuração de Connection Pool MongoDB**

#### **URI Otimizada para Produção**
```bash
# MongoDB URI com connection pool otimizado
export MONGODB_URI="mongodb://root:PASSWORD@mongodb-0.mongodb-headless.rmi.svc.cluster.local:27017,mongodb-1.mongodb-headless.rmi.svc.cluster.local:27017,mongodb-arbiter.mongodb-headless.rmi.svc.cluster.local:27017/?replicaSet=rs0&authSource=admin&readPreference=nearest&maxPoolSize=500&minPoolSize=50&maxIdleTimeMS=60000&serverSelectionTimeoutMS=3000&socketTimeoutMS=30000&connectTimeoutMS=5000&retryWrites=true&retryReads=true&w=majority&readConcernLevel=majority&directConnection=false&maxStalenessSeconds=90&heartbeatFrequencyMS=10000&localThresholdMS=15&compressors=zlib&zlibCompressionLevel=6&maxConnecting=2&loadBalanced=false"
```

#### **Parâmetros de Connection Pool Explicados**
| Parâmetro | Valor | Impacto | Recomendação |
|-----------|-------|---------|--------------|
| `maxPoolSize=500` | 500 | Alto throughput | ✅ Manter |
| `minPoolSize=50` | 50 | Conexões quentes | ✅ Manter |
| `maxIdleTimeMS=60000` | 60s | Economia de recursos | ✅ Manter |
| `serverSelectionTimeoutMS=3000` | 3s | Failover rápido | ✅ Manter |
| `socketTimeoutMS=30000` | 30s | Timeout de operações | ✅ Manter |
| `connectTimeoutMS=5000` | 5s | Timeout de conexão | ✅ Manter |
| `maxConnecting=2` | 2 | Previne tempestades | ✅ Manter |

### **Monitoramento MongoDB em SignOz**

#### **Métricas Disponíveis**
```yaml
# MongoDB Connection Pool Metrics
app_rmi_mongodb_connection_pool{status="sessions_in_progress", database="rmi"}
app_rmi_mongodb_connection_pool{status="warning", database="rmi"}
app_rmi_mongodb_connection_pool{status="critical", database="rmi"}

# MongoDB Operation Metrics
app_rmi_mongodb_operation_duration_seconds{operation="insert", collection="audit_logs", database="rmi"}
app_rmi_mongodb_operation_duration_seconds{operation="find", collection="citizen", database="rmi"}
```

#### **Alertas Automáticos**
- **Warning**: > 300 conexões (60% do pool)
- **Critical**: > 400 conexões (80% do pool)
- **Connection Leak Detection**: Monitoramento contínuo

### **Estratégias de Otimização MongoDB**

1. **Connection Pool Tuning**
   ```bash
   # Para produção com alto tráfego
   # Ajustar via URI MongoDB
   maxPoolSize=1000        # Aumentar se necessário
   minPoolSize=100         # Manter conexões quentes
   maxIdleTimeMS=30000     # Reduzir para 30s
   ```

2. **Query Optimization**
   ```go
   // Usar índices compostos para consultas frequentes
   // Implementar paginação para listagens grandes
   // Usar projeções para reduzir dados transferidos
   ```

3. **Replica Set Optimization**
   ```bash
   # Configurar read preference para distribuir carga
   readPreference=nearest    # Lê do nó mais próximo
   maxStalenessSeconds=90   # Aceita dados com até 90s de atraso
   ```

### **Troubleshooting MongoDB Connection Pool**

#### **Problemas Comuns e Soluções**

1. **Connection Pool Exhaustion**
   ```bash
   # Verificar métricas em SignOz
   app_rmi_mongodb_connection_pool{status="critical"}
   
   # Verificar logs da aplicação
   kubectl logs -f deployment/rmi-api | grep "connection pool"
   
   # Aumentar maxPoolSize na URI
   maxPoolSize=1000
   ```

2. **Slow Queries Blocking Connections**
   ```bash
   # Verificar operações lentas
   kubectl exec -it mongodb-0 -- mongosh --eval "db.currentOp({'secs_running': {'$gt': 5}})"
   
   # Verificar índices
   kubectl exec -it mongodb-0 -- mongosh --eval "db.citizen.getIndexes()"
   ```

3. **Replica Set Issues**
   ```bash
   # Verificar status do replica set
   kubectl exec -it mongodb-0 -- mongosh --eval "rs.status()"
   
   # Verificar eleição primária
   kubectl exec -it mongodb-0 -- mongosh --eval "rs.isMaster()"
   ```

#### **Comandos de Debug MongoDB**
```bash
# Verificar status das conexões
kubectl exec -it mongodb-0 -- mongosh --eval "db.serverStatus().connections"

# Verificar operações ativas
kubectl exec -it mongodb-0 -- mongosh --eval "db.currentOp()"

# Verificar performance de queries
kubectl exec -it mongodb-0 -- mongosh --eval "db.citizen.find().explain('executionStats')"

# Verificar logs do MongoDB
kubectl logs -f mongodb-0 -c mongodb
```

## Endpoints da API

### GET /citizen/{cpf}
Recupera os dados do cidadão por CPF, combinando dados base com atualizações autodeclaradas.
- Dados autodeclarados têm precedência sobre dados base
- Resultados são armazenados em cache usando Redis com TTL configurável
- Campos internos (cpf_particao, datalake, row_number, documentos, saude) são excluídos da resposta

### GET /citizen/{cpf}/wallet
Recupera os dados da carteira do cidadão por CPF.
- Inclui informações de saúde (`saude`)
- Inclui documentos (`documentos`)
- Inclui assistência social (`assistencia_social`)
- Inclui educação (`educacao`)
- Resultados são armazenados em cache usando Redis com TTL configurável

### GET /citizen/{cpf}/maintenance-request
Recupera os chamados do 1746 de um cidadão por CPF com paginação.
- Suporta paginação com parâmetros `page` e `per_page`
- Ordenação por data de início (mais recentes primeiro)
- Resultados são armazenados em cache usando Redis com TTL configurável
- Parâmetros de paginação:
  - `page`: Número da página (padrão: 1, mínimo: 1)
  - `per_page`: Itens por página (padrão: 10, máximo: 100)

### PUT /citizen/{cpf}/address
Atualiza ou cria o endereço autodeclarado de um cidadão.
- Apenas o campo de endereço é atualizado
- Endereço é validado automaticamente
- Validação de formato de CEP brasileiro
- Verificação de campos obrigatórios
- Validação de limites de caracteres
- Detecção de endereços duplicados

### PUT /citizen/{cpf}/phone
Atualiza ou cria o telefone autodeclarado de um cidadão.
- Apenas o campo de telefone é atualizado
- Número de telefone requer verificação via WhatsApp
- Código de verificação é enviado para o número fornecido
- Validação de formato de telefone internacional
- Suporte a números brasileiros e internacionais
- Verificação de duplicatas antes da atualização

### PUT /citizen/{cpf}/email
Atualiza ou cria o email autodeclarado de um cidadão.
- Apenas o campo de email é atualizado
- Email é validado automaticamente
- Validação de formato RFC-compliant
- Verificação de duplicatas
- Normalização automática (lowercase)

### PUT /citizen/{cpf}/ethnicity
Atualiza ou cria a etnia autodeclarada de um cidadão.
- Apenas o campo de etnia é atualizado
- Valor deve ser uma das opções válidas retornadas pelo endpoint /citizen/ethnicity/options

### PUT /citizen/{cpf}/optin
Atualiza o status de opt-in de um cidadão.
- Atualiza o campo `opt_in` nos dados autodeclarados
- Requer autenticação JWT com acesso ao CPF
- Invalida cache relacionado automaticamente
- Registra auditoria da mudança

### GET /citizen/ethnicity/options
Retorna a lista de opções válidas de etnia para autodeclaração.
- Usado para validar as atualizações de etnia autodeclarada
- Não requer autenticação

### POST /validate/phone
Valida números de telefone internacionais usando a biblioteca libphonenumber do Google.
- Suporte a números de qualquer país
- Decomposição automática em DDI, DDD e número
- Validação de formato E.164
- Detecção automática de região
- Não requer autenticação

### POST /citizen/{cpf}/phone/validate
Valida um número de telefone usando um código de verificação.
- Código é enviado via WhatsApp quando o telefone é atualizado
- Código expira após o TTL configurado (padrão: 15 minutos)
- Telefone é marcado como verificado após validação bem-sucedida
- Operação atômica com transações de banco de dados
- Limpeza automática do código de verificação após uso
- Invalidação completa do cache relacionado
- Registro de auditoria da verificação

## WhatsApp Bot Endpoints

### GET /phone/{phone_number}/citizen
Busca um cidadão por número de telefone e retorna dados mascarados.
- Retorna CPF e nome mascarados se encontrado
- Retorna `{"found": false}` se não encontrado
- Não requer autenticação
- Suporte a números internacionais

### POST /phone/{phone_number}/validate-registration
Valida dados de registro (nome, CPF, data de nascimento) contra dados base.
- Validação contra coleção de dados base (read-only)
- Retorna resultado da validação e dados encontrados
- Registra tentativas de validação para auditoria
- Não requer autenticação

### POST /phone/{phone_number}/opt-in
Processa opt-in para um número de telefone.
- Requer autenticação JWT com acesso ao CPF
- Cria mapeamento phone-CPF ativo
- Registra histórico de opt-in
- Atualiza dados autodeclarados se validado
- Suporte a diferentes canais (WhatsApp, Web, Mobile)

### POST /phone/{phone_number}/opt-out
Processa opt-out para um número de telefone.
- **Nova funcionalidade**: Números que nunca fizeram opt-in podem agora fazer opt-out
- Cria mapeamento phone-CPF com status "blocked" para números desconhecidos
- Não requer autenticação JWT para números desconhecidos
- Registra histórico de opt-out com motivo
- Para números conhecidos: requer autenticação e atualiza dados autodeclarados
- **Status na resposta**: `"opted_out"` para todas as operações de opt-out bem-sucedidas
- **Campo opted_out**: Adicionado ao modelo `PhoneStatusResponse` para indicar status de opt-out

### POST /phone/{phone_number}/reject-registration
Rejeita um registro e bloqueia mapeamento phone-CPF.
- Requer autenticação JWT com acesso ao CPF
- Bloqueia mapeamento existente
- Registra rejeição no histórico
- Permite novo registro para o número

### GET /phone/{phone_number}/status
Verifica o status de um número de telefone.
- Retorna informações sobre mapeamento phone-CPF
- Inclui status de quarentena (se aplicável)
- Inclui informações de whitelist beta (se aplicável)
- Não requer autenticação
- Dados sensíveis (CPF, nome) são mascarados

### GET /phone/{phone_number}/beta-status
Verifica se um número de telefone está na whitelist beta.
- Retorna status beta e informações do grupo
- Cache Redis para performance
- Não requer autenticação

## Configuration Endpoints

### GET /config/channels
Retorna lista de canais disponíveis para opt-in/opt-out.
- Canais: WhatsApp, Web, Mobile
- Não requer autenticação

### GET /config/opt-out-reasons
Retorna lista de motivos disponíveis para opt-out.
- Motivos com título e subtítulo
- Não requer autenticação

## Modelos de Dados

### Citizen
Modelo principal contendo todas as informações do cidadão:
- Informações básicas (nome, CPF, etc.)
- Informações de contato (endereço, telefone, email)
- Informações de saúde
- Metadados (última atualização, etc.)

### SelfDeclaredData
Armazena atualizações autodeclaradas dos dados do cidadão:
- Armazena apenas campos que foram atualizados
- Inclui status de validação
- Mantém histórico de atualizações

### PhoneVerification
Gerencia o processo de verificação de números de telefone:
- Armazena códigos de verificação
- Rastreia status de verificação
- Gerencia expiração de códigos
- Limpeza automática via índices TTL
- Consultas otimizadas com índices compostos

### AuditLog
Sistema abrangente de auditoria:
- Rastreia todas as mudanças de dados com metadados
- Registra contexto do usuário (IP, user agent, ID do usuário)
- Limpeza automática após 1 ano
- Estruturado para compliance e debugging

### PhoneCPFMapping
Gerencia relacionamentos entre números de telefone e CPF:
- Rastreia mapeamentos ativos, bloqueados e pendentes
- Suporta registros autodeclarados e validados
- Registra tentativas de validação e canais
- Gerenciamento automático de status

### OptInHistory
Rastreia ações de opt-in e opt-out:
- Registra todos os eventos de opt-in/opt-out com timestamps
- Armazena informações de canal e motivos
- Vincula aos resultados de validação
- Trilha de auditoria completa para compliance

## Cache

A API usa Redis para cache de dados de cidadãos:
- Chave de cache: `citizen:{cpf}`
- TTL: Configurável via `REDIS_TTL` (padrão: 60 minutos)
- Cache é invalidado quando dados autodeclarados são atualizados
- Invalidação abrangente de cache para dados relacionados
- Invalidação de cache para dados de cidadão, carteira e chamados

## Monitoramento

### Métricas
Métricas Prometheus disponíveis em `/metrics`:
- Contagens e durações de requisições
- Hits e misses de cache
- Atualizações autodeclaradas
- Verificações de telefone

### Rastreamento
Rastreamento OpenTelemetry disponível quando habilitado:
- Rastreamento de requisições
- Operações de banco de dados
- Operações de cache
- Chamadas de serviços externos

### Logs
Logs estruturados com Zap:
- Logs de requisições
- Rastreamento de erros
- Monitoramento de performance
- Trilha de auditoria

### Gerenciamento de Índices
A API gerencia automaticamente os índices MongoDB para garantir performance otimizada de consultas:
- **Criação Automática de Índices**: Cria índices necessários na inicialização se não existirem
- **Verificação Periódica**: Verifica índices em intervalos configuráveis e os recria se estiverem ausentes
- **Seguro para Múltiplas Instâncias**: Usa `createIndex` do MongoDB com construção em background e tratamento de erros de chave duplicada
- **Proteção contra Sobrescrita de Coleções**: Garante que índices existam após sobrescritas de coleções do BigQuery/Airbyte
- **Intervalo Configurável**: Definido via variável de ambiente `INDEX_MAINTENANCE_INTERVAL` (padrão: 1h)

**Índices Gerenciados:**
- Coleção `citizen`: Índice único no campo `cpf` (`cpf_1`)
- Coleção `maintenance_request`: Índice no campo `cpf` (`cpf_1`)
- Coleção `self_declared`: Índice único no campo `cpf` (`cpf_1`)
- Coleção `phone_verifications`: 
  - Índice composto único em `cpf` e `phone_number` (`cpf_1_phone_number_1`)
  - Índice TTL em `expires_at` para limpeza automática (`expires_at_1`)
  - Índice composto para consultas de verificação (`verification_query_1`)
- Coleção `user_config`: Índice único no campo `cpf` (`cpf_1`)
- Coleção `audit_logs`:
  - Índice no campo `cpf` (`cpf_1`)
  - Índice no campo `timestamp` (`timestamp_1`)
  - Índice composto em `action` e `resource` (`action_1_resource_1`)
  - Índice TTL para limpeza automática após 1 ano (`timestamp_ttl`)
- Coleção `phone_cpf_mappings`:
  - Índice único no campo `phone_number` (`phone_number_1`)
  - Índice no campo `cpf` (`cpf_1`)
  - Índice no campo `status` (`status_1`)
  - Índice composto em `phone_number` e `status` (`phone_number_1_status_1`)
  - Índice no campo `created_at` (`created_at_1`)
- Coleção `opt_in_history`:
  - Índice no campo `phone_number` (`phone_number_1`)
  - Índice no campo `cpf` (`cpf_1`)
  - Índice no campo `action` (`action_1`)
  - Índice no campo `channel` (`channel_1`)
  - Índice no campo `timestamp` (`timestamp_1`)
  - Índice composto em `phone_number` e `timestamp` (`phone_number_1_timestamp_1`)
- Coleção `beta_groups`:
  - Índice único no campo `name` (`name_1`) - case-insensitive
  - Índice no campo `created_at` (`created_at_1`) - ordenação temporal
- Coleção `phone_cpf_mappings` (índices adicionais):
  - Índice no campo `beta_group_id` (`beta_group_id_1`) - consultas de whitelist beta

## WhatsApp Bot Scenarios

### Cenário 1: Opt-in de Usuário Existente
1. **Verificar Registro**: WhatsApp bot chama `GET /phone/{phone}/citizen`
2. **Retornar Dados Mascarados**: API retorna CPF e nome mascarados se encontrado
3. **Confirmação do Usuário**: Usuário confirma que o registro está correto
4. **Processamento de Opt-in**: Bot chama `POST /phone/{phone}/opt-in` com CPF e canal
5. **Criar Mapeamento**: API cria mapeamento phone-CPF ativo e atualiza status de opt-in

### Cenário 2: Novo Registro de Usuário
1. **Verificar Registro**: WhatsApp bot chama `GET /phone/{phone}/citizen` → Retorna `{"found": false}`
2. **Coletar Dados**: Bot coleta nome, CPF e data de nascimento do usuário
3. **Validar Registro**: Bot chama `POST /phone/{phone}/validate-registration`
4. **Resultado da Validação**: API valida contra dados base e retorna resultado
5. **Processamento de Opt-in**: Se válido, bot chama `POST /phone/{phone}/opt-in` com resultado da validação
6. **Criar Mapeamento Autodeclarado**: API cria mapeamento phone-CPF autodeclarado

### Cenário 3: Registro Incorreto
1. **Verificar Registro**: WhatsApp bot chama `GET /phone/{phone}/citizen` → Retorna registro existente
2. **Rejeição do Usuário**: Usuário diz que o registro pertence a outra pessoa
3. **Rejeitar Registro**: Bot chama `POST /phone/{phone}/reject-registration`
4. **Bloquear Mapeamento**: API bloqueia o mapeamento phone-CPF
5. **Novo Registro**: Bot prossegue com fluxo de novo registro (Cenário 2)

### Cenário 4: Processo de Opt-out
1. **Solicitação do Usuário**: Usuário solicita opt-out via WhatsApp
2. **Processamento de Opt-out**: Bot chama `POST /phone/{phone}/opt-out` com motivo e canal
3. **Bloqueio Condicional**: API só bloqueia o mapeamento phone-CPF se o motivo for "Mensagem era engano"
4. **Atualizar Status**: API atualiza status de opt-in dos dados autodeclarados
5. **Registrar Histórico**: API registra opt-out no histórico com motivo e timestamp

### Cenário 5: HSM Sucesso (Quarentena)
1. **Verificar Status**: Bot chama `GET /phone/{phone}/status`
2. **Se Encontrado e Não Quarantinado**: Bot chama `POST /phone/{phone}/bind` para vincular CPF sem opt-in
3. **Se Encontrado e Quarantinado**: Bot chama `POST /phone/{phone}/bind` que automaticamente libera da quarentena
4. **Se Não Encontrado**: Bot chama `POST /phone/{phone}/bind` para criar novo mapeamento

### Cenário 6: HSM Falha (Quarentena)
1. **Verificar Status**: Bot chama `GET /phone/{phone}/status`
2. **Quarentenar Número**: Admin chama `POST /phone/{phone}/quarantine` (sem CPF)
3. **Se Número Existe**: Quarentena é estendida
4. **Se Número Não Existe**: Nova quarentena é criada

**Motivos de Opt-out:**
- **Conteúdo irrelevante**: Mensagens não são úteis (não bloqueia mapeamento)
- **Não sou do Rio**: Não é do Rio de Janeiro (não bloqueia mapeamento)
- **Mensagem era engano**: Não é a pessoa na mensagem (**bloqueia mapeamento CPF-telefone**)
- **Quantidade de mensagens**: Muitas mensagens da Prefeitura (não bloqueia mapeamento)

## Funcionalidades de Quarentena

### Visão Geral
O sistema de quarentena permite gerenciar números de telefone que falharam na entrega de mensagens HSM (Highly Structured Messages) do WhatsApp. Números em quarentena são automaticamente excluídos de futuras campanhas por um período configurável.

### Características Principais
- **Quarentena Computada**: O status `quarantined` é calculado dinamicamente baseado em `quarantine_until > now()`
- **Histórico Completo**: Todas as ações de quarentena são registradas com timestamps
- **Liberação Automática**: Opt-in e binding automaticamente liberam números da quarentena
- **Extensão de Quarentena**: Quarentenas existentes são estendidas quando aplicadas novamente
- **Acesso Administrativo**: Apenas usuários com role `rmi-admin` podem gerenciar quarentenas

### Endpoints de Quarentena

#### Verificar Status do Telefone
```http
GET /v1/phone/{phone_number}/status
```
**Resposta:**
```json
{
  "phone_number": "+5511999887766",
  "found": true,
  "quarantined": true,
  "cpf": "***.***.***-**",
  "name": "*** ***",
  "quarantine_until": "2026-02-07T10:00:00Z"
}
```

#### Colocar em Quarentena (Admin)
```http
POST /v1/phone/{phone_number}/quarantine
```
**Corpo da Requisição:** `{}` (vazio)
**Resposta:**
```json
{
  "status": "quarantined",
  "phone_number": "+5511999887766",
  "quarantine_until": "2026-02-07T10:00:00Z",
  "message": "Phone number quarantined for 6 months"
}
```

#### Liberar da Quarentena (Admin)
```http
DELETE /v1/phone/{phone_number}/quarantine
```
**Resposta:**
```json
{
  "status": "released",
  "phone_number": "+5511999887766",
  "quarantine_until": "2025-08-07T10:00:00Z",
  "message": "Phone number released from quarantine"
}
```

#### Vincular Telefone a CPF
```http
POST /v1/phone/{phone_number}/bind
```
**Corpo da Requisição:**
```json
{
  "cpf": "12345678901",
  "channel": "whatsapp"
}
```
**Resposta:**
```json
{
  "status": "bound",
  "phone_number": "+5511999887766",
  "cpf": "12345678901",
  "opt_in": false,
  "message": "Phone number bound to CPF without opt-in"
}
```

#### Listar Telefones em Quarentena (Admin)
```http
GET /v1/admin/phone/quarantined?page=1&per_page=20&expired=false
```
**Parâmetros:**
- `page`: Número da página (padrão: 1)
- `per_page`: Itens por página (padrão: 20, máximo: 100)
- `expired`: Filtrar apenas quarentenas expiradas (padrão: false)

**Resposta:**
```json
{
  "data": [
    {
      "phone_number": "+5511999887766",
      "cpf": "***.***.***-**",
      "quarantine_until": "2026-02-07T10:00:00Z",
      "expired": false
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 150,
    "total_pages": 8
  }
}
```

#### Estatísticas de Quarentena (Admin)
```http
GET /v1/admin/phone/quarantine/stats
```
**Resposta:**
```json
{
  "total_quarantined": 150,
  "expired_quarantines": 25,
  "active_quarantines": 125,
  "quarantines_with_cpf": 80,
  "quarantines_without_cpf": 70,
  "quarantine_history_total": 300
}
```

### Configuração
```env
PHONE_QUARANTINE_TTL=4320h  # 6 meses (6 * 30 * 24 horas)
```

### Modelo de Dados
```json
{
  "phone_number": "+5511999887766",
  "cpf": "12345678901",  // null se não vinculado
  "status": "active|blocked|quarantined",
  "quarantine_until": "2026-02-07T10:00:00Z",  // null se não em quarentena
  "quarantine_history": [
    {
      "quarantined_at": "2025-08-07T10:00:00Z",
      "quarantine_until": "2026-02-07T10:00:00Z",
      "released_at": "2025-09-07T10:00:00Z"  // null se ainda em quarentena
    }
  ],
  "created_at": "2025-08-07T10:00:00Z",
  "updated_at": "2025-08-07T10:00:00Z"
}
```

### Índices de Banco de Dados
- `phone_number_1`: Índice no número de telefone
- `cpf_1`: Índice no CPF
- `status_1`: Índice no status
- `quarantine_until_1`: Índice para consultas de quarentena
- `quarantine_until_1_cpf_1`: Índice composto para quarentenas com CPF
- `phone_number_1_status_1`: Índice composto para consultas por telefone e status
- `created_at_1`: Índice para ordenação temporal

### Fluxo de Negócio

#### HSM Sucesso
1. Bot verifica status do telefone
2. Se encontrado e não quarantinado → Vincula CPF sem opt-in
3. Se encontrado e quarantinado → Libera da quarentena e vincula CPF
4. Se não encontrado → Cria novo mapeamento com CPF

#### HSM Falha
1. Bot verifica status do telefone
2. Admin coloca número em quarentena
3. Se número existe → Estende quarentena
4. Se número não existe → Cria nova quarentena

#### Liberação de Quarentena
- **Automática**: Opt-in ou binding liberam automaticamente
- **Manual**: Admin pode liberar via endpoint DELETE
- **Expiração**: Quarentenas expiram automaticamente após TTL configurado

### Segurança
- Apenas usuários com role `rmi-admin` podem gerenciar quarentenas
- Todos os endpoints de quarentena requerem autenticação
- Histórico completo de todas as ações para auditoria
- Dados sensíveis (CPF) são mascarados nas respostas

### Monitoramento
- Métricas de quarentena disponíveis via endpoint de estatísticas
- Logs estruturados para todas as operações de quarentena
- Rastreamento de histórico completo para compliance

## 🧠 **Funcionalidades de Memória**

Sistema de memória para chatbot que permite gerenciar memórias de longo prazo relacionadas ao cidadão, proporcionando contexto persistente para conversas e personalização de experiências.

### **Visão Geral**
- **🎯 Memórias Persistidas**: Armazenamento de informações contextuais de longo prazo
- **📝 Tipos de Memória**: Memórias base (fundamentais) e anexadas (contextuais)
- **⚡ Cache Inteligente**: Verificação rápida de memórias com cache Redis
- **🔍 Busca por Nome**: Acesso direto a memórias específicas via nome
- **📊 Relevância Hierárquica**: Classificação por importância (baixa, média, alta)
- **🔄 CRUD Completo**: Operações completas de criação, leitura, atualização e exclusão

### **Funcionalidades Principais**

#### **Gerenciamento de Memórias**
- **🆕 Criação de Memórias**: Criação de novas memórias com validação de unicidade
- **📖 Listagem de Memórias**: Recuperação de todas as memórias associadas a um telefone
- **🔍 Busca por Nome**: Acesso direto a memórias específicas via nome único
- **✏️ Atualização de Memórias**: Modificação de memórias existentes com verificação de duplicatas
- **🗑️ Exclusão de Memórias**: Remoção segura de memórias com limpeza de cache
- **🔄 Controle de Versões**: Timestamps automáticos para rastreamento de mudanças

#### **Características Técnicas**
- **🔐 Autenticação**: Endpoints protegidos com autenticação Bearer
- **💾 Cache Multi-Nível**: Cache Redis para listas e memórias individuais
- **📈 Performance**: Otimização com tracing OpenTelemetry e monitoramento
- **✅ Validação**: Validação robusta de formato de telefone e dados de entrada
- **🔄 Atomicidade**: Operações atômicas com tratamento de concorrência
- **📊 Observabilidade**: Métricas completas e logs estruturados

### **Modelo de Dados**

#### **MemoryModel**
```json
{
  "memory_id": "uuid-da-memoria",
  "memory_name": "nome-da-memoria",
  "description": "Descrição da memória",
  "relevance": "low|medium|high",
  "memory_type": "base|appended", 
  "value": "Conteúdo da memória",
  "created_at": "2025-08-07T15:30:00Z",
  "updated_at": "2025-08-07T15:30:00Z"
}
```

#### **Campos e Validações**
- **memory_name**: Nome único da memória (obrigatório, case-insensitive)
- **description**: Descrição da memória (obrigatório)
- **relevance**: Relevância (obrigatório: low, medium, high)
- **memory_type**: Tipo de memória (obrigatório: base, appended)
- **value**: Conteúdo da memória (obrigatório)
- **timestamps**: Criado/atualizado automaticamente

### **Endpoints da API**

#### **Endpoints de Memória**

##### **GET /memory/{phone_number}**
Recupera a lista de todas as memórias associadas ao telefone do cidadão.
- **Autenticação**: Requer role `rmi-admin`
- **Cache**: Resultados cacheados com TTL configurável
- **Resposta**: Array de objetos `MemoryModel` (vazio se não houver memórias)
- **Status**: 200 (sucesso), 400 (telefone inválido), 401/403 (autenticação), 500 (erro interno)

##### **GET /memory/{phone_number}/{memory_name}**
Recupera uma memória específica associada ao telefone pelo nome.
- **Autenticação**: Requer role `rmi-admin`
- **Cache**: Cache individual por memória
- **Resposta**: Objeto `MemoryModel`
- **Status**: 200 (sucesso), 400 (dados inválidos), 404 (não encontrado), 401/403 (autenticação), 500 (erro interno)

##### **POST /memory/{phone_number}**
Cria uma nova memória associada ao telefone do cidadão.
- **Autenticação**: Requer role `rmi-admin`
- **Body**: Objeto `MemoryModel` (sem memory_id e timestamps)
- **Validação**: Verificação de duplicatas (nome único por telefone)
- **Resposta**: Objeto `MemoryModel` criado
- **Status**: 201 (criado), 400 (dados inválidos), 409 (duplicado), 401/403 (autenticação), 500 (erro interno)

##### **PUT /memory/{phone_number}/{memory_name}**
Atualiza uma memória existente associada ao telefone.
- **Autenticação**: Requer role `rmi-admin`
- **Body**: Objeto `MemoryModel` com dados atualizados
- **Validação**: Verificação de duplicatas se nome for alterado
- **Resposta**: `{"message": "Memory updated successfully"}`
- **Status**: 200 (sucesso), 400 (dados inválidos), 404 (não encontrado), 409 (duplicado), 401/403 (autenticação), 500 (erro interno)

##### **DELETE /memory/{phone_number}/{memory_name}**
Remove uma memória associada ao telefone.
- **Autenticação**: Requer role `rmi-admin`
- **Resposta**: Status 204 (sem conteúdo)
- **Status**: 204 (sucesso), 400 (dados inválidos), 404 (não encontrado), 401/403 (autenticação), 500 (erro interno)

### **Configuração**

#### **Variáveis de Ambiente**
| Variável | Descrição | Padrão | Obrigatório |
|----------|-----------|---------|------------|
| MONGODB_CHAT_MEMORY_COLLECTION | Nome da coleção de memórias da conversa | chat_memory | Não |

### **Características Técnicas**

#### **Cache Redis**
- **Cache de Lista**: `memory_list:{phone_number}` - TTL configurável
- **Cache Individual**: `memory:{phone_number}:{memory_name}` - TTL configurável
- **Invalidação Inteligente**: Cache limpo automaticamente em operações de escrita
- **Performance**: Consultas rápidas sem necessidade de acesso ao banco

#### **Banco de Dados**
- **Índices Otimizados**: Índices para consultas por telefone e nome da memória
- **Integridade**: Constraints para nomes únicos de memória por telefone
- **Timestamps**: Controle automático de criação e atualização

#### **Segurança**
- **Controle de Acesso**: Endpoints requerem role `rmi-admin`
- **Validação**: Verificação de duplicatas e dados válidos
- **Auditoria**: Logs de todas as operações com tracing

#### **Performance**
- **Cache Inteligente**: Cache Redis para verificações frequentes
- **Tracing**: Monitoramento completo com OpenTelemetry
- **Validação Eficiente**: Validação rápida de formato de telefone
- **Operações em Lote**: Processamento batch para operações de cache

### **Fluxo de Operação**

#### **Criação de Memória**
1. **Validação**: Verifica formato do telefone e dados de entrada
2. **Verificação de Duplicata**: Confirma que nome da memória é único para o telefone
3. **Geração de ID**: UUID único gerado automaticamente
4. **Inserção no MongoDB**: Armazena memória com timestamps
5. **Invalidação de Cache**: Limpa cache de lista e individual
6. **Resposta**: Retorna memória criada com status 201

#### **Leitura de Memória**
1. **Validação**: Verifica formato do telefone
2. **Cache Check**: Tenta obter do cache Redis primeiro
3. **MongoDB Fallback**: Se cache miss, consulta MongoDB
4. **Cache Update**: Atualiza cache para futuras consultas
5. **Resposta**: Retorna memória(s) encontrada(s)

#### **Atualização de Memória**
1. **Validação**: Verifica dados de entrada
2. **Verificação de Existência**: Confirma que memória existe
3. **Verificação de Duplicata**: Se nome mudar, verifica se novo nome é único
4. **Atualização no MongoDB**: Aplica mudanças com novo timestamp
5. **Atualização de Cache**: Atualiza cache individual e limpa cache de lista
6. **Resposta**: Confirmação de sucesso

#### **Exclusão de Memória**
1. **Validação**: Verifica dados de entrada
2. **Verificação de Existência**: Confirma que memória existe
3. **Exclusão no MongoDB**: Remove documento
4. **Limpeza de Cache**: Remove cache individual e de lista
5. **Resposta**: Status 204 (sem conteúdo)

### **Monitoramento e Observabilidade**

#### **Métricas Disponíveis**
```yaml
# Operações de banco
app_rmi_database_operations_total{operation="find", collection="chat_memory", status="success"}
app_rmi_database_operations_total{operation="insert", collection="chat_memory", status="success"}
app_rmi_database_operations_total{operation="update", collection="chat_memory", status="success"}
app_rmi_database_operations_total{operation="delete", collection="chat_memory", status="success"}

# Performance
app_rmi_operation_duration_seconds{operation="GetMemoryList"}
app_rmi_operation_duration_seconds{operation="GetMemoryByName"}
app_rmi_operation_duration_seconds{operation="CreateMemory"}
app_rmi_operation_duration_seconds{operation="UpdateMemory"}
app_rmi_operation_duration_seconds{operation="DeleteMemory"}
```

#### **Traces OpenTelemetry**
- **Span Completo**: Rastreamento de toda a operação
- **Atributos**: Telefone, nome da memória, operação, duração
- **Erros**: Categorização e detalhamento de falhas
- **Performance**: Timing de cada etapa da operação

### **Casos de Uso**

#### **Chatbot com Contexto Persistente**
1. **Inicialização**: Bot recupera memórias do usuário
2. **Contextualização**: Usa memórias para personalizar conversa
3. **Atualização**: Adiciona novas informações como memórias
4. **Manutenção**: Remove memórias irrelevantes ou desatualizadas

#### **Sistema de Preferências**
1. **Preferências do Usuário**: Armazena preferências como memórias base
2. **Contexto de Conversa**: Memórias anexadas para contexto específico
3. **Evolução**: Atualiza preferências baseadas em interações

#### **Personalização de Experiência**
1. **Perfil do Usuário**: Memórias que definem características do usuário
2. **Histórico de Interações**: Contexto de conversas anteriores
3. **Preferências Comportamentais**: Padrões de interação do usuário

### **Exemplos de Uso**

#### **Criar Memória de Preferência**
```bash
curl -X POST "http://localhost:8080/memory/+5511999887766" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "memory_name": "preferencia_idioma",
    "description": "Preferência de idioma do usuário",
    "relevance": "high", 
    "memory_type": "base",
    "value": "portugues"
  }'
```

#### **Recuperar Todas as Memórias**
```bash
curl -X GET "http://localhost:8080/memory/+5511999887766" \
  -H "Authorization: Bearer <token>"
```

#### **Atualizar Memória Existente**
```bash
curl -X PUT "http://localhost:8080/memory/+5511999887766/preferencia_idioma" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "memory_name": "preferencia_idioma",
    "description": "Preferência de idioma atualizada",
    "relevance": "high",
    "memory_type": "base", 
    "value": "portugues_brasil"
  }'
```

### **Integração com Outros Sistemas**

#### **Chatbot Integration**
- **Contexto Persistente**: Memórias fornecem contexto entre sessões
- **Personalização**: Experiência customizada baseada em histórico
- **Aprendizado Contínuo**: Sistema evolui com interações do usuário

#### **Sistema de Analytics**
- **Padrões de Uso**: Análise de quais memórias são mais relevantes
- **Evolução de Preferências**: Tracking de mudanças ao longo do tempo
- **Otimização**: Identificação de memórias mais úteis

---

**🎯 Objetivo**: Prover um sistema robusto de memória que permita chatbots manterem contexto persistente e oferecerem experiências personalizadas baseadas no histórico de interações com o usuário.

### Configuração

#### Variáveis de Ambiente
| Variável | Descrição | Padrão | Obrigatório |
|----------|-----------|---------|------------|
| MONGODB_CHAT_MEMORY_COLLECTION | Nome da coleção de memórias da conversa | chat_memory | Não |

### Características Técnicas

#### Cache Redis
- **TTL Configurável**: Cache de status beta com TTL personalizável
- **Invalidação Inteligente**: Cache limpo quando associações mudam
- **Performance**: Verificações rápidas sem consulta ao banco

#### Segurança
- **Controle de Acesso**: Endpoints administrativos requerem role `rmi-admin`
- **Validação**: Verificação de duplicatas e dados válidos
- **Auditoria**: Logs de todas as operações administrativas

#### Performance
- **Cache Inteligente**: Cache Redis para verificações frequentes
- **Paginação**: Listagens paginadas para grandes volumes
- **Índices**: Índices otimizados para consultas rápidas

## Funcionalidades de Beta Whitelist

Sistema de whitelist para chatbot beta que permite gerenciar grupos de teste e controlar acesso de números de telefone.

### Visão Geral
- **Grupos Beta**: Criação e gerenciamento de grupos para testes do chatbot
- **Whitelist de Telefones**: Controle de quais números podem acessar o chatbot beta
- **Cache Inteligente**: Verificação rápida de status beta com cache Redis
- **Operações em Lote**: Suporte a operações bulk para gerenciamento eficiente
- **Analytics**: Rastreamento de grupos para fins analíticos

### Funcionalidades Principais

#### Gerenciamento de Grupos Beta
- **Criação de Grupos**: Criação de grupos com nomes únicos (case-insensitive)
- **Listagem Paginada**: Listagem de grupos com paginação
- **Atualização**: Modificação de nomes de grupos existentes
- **Exclusão**: Remoção de grupos com limpeza automática de associações
- **UUIDs**: Identificadores únicos automáticos para grupos

#### Whitelist de Telefones
- **Adição Individual**: Adicionar telefones a grupos específicos
- **Remoção Individual**: Remover telefones da whitelist
- **Operações em Lote**: Adicionar, remover e mover múltiplos telefones
- **Validação**: Verificação de duplicatas e grupos existentes
- **Cache**: Cache Redis para verificações rápidas de status

#### Verificação de Status
- **Endpoint Público**: Verificação rápida se um telefone está na whitelist
- **Cache TTL**: Cache configurável (padrão: 24 horas)
- **Invalidação Automática**: Cache limpo quando associações mudam
- **Informações Completas**: Inclui ID e nome do grupo

### Endpoints da API

#### Endpoints Públicos

##### GET /phone/{phone_number}/beta-status
Verifica se um número de telefone está na whitelist beta.
- **Resposta**: Status beta, ID do grupo, nome do grupo
- **Cache**: Resultados cacheados por 24 horas
- **Autenticação**: Não requerida

#### Endpoints Administrativos

##### GET /admin/beta/groups
Lista todos os grupos beta com paginação.
- **Parâmetros**: `page` (padrão: 1), `per_page` (padrão: 10, máximo: 100)
- **Autenticação**: Requer role `rmi-admin`

##### POST /admin/beta/groups
Cria um novo grupo beta.
- **Body**: `{"name": "Nome do Grupo"}`
- **Validação**: Nome único, case-insensitive
- **Autenticação**: Requer role `rmi-admin`

##### GET /admin/beta/groups/{group_id}
Obtém detalhes de um grupo beta específico.
- **Autenticação**: Requer role `rmi-admin`

##### PUT /admin/beta/groups/{group_id}
Atualiza o nome de um grupo beta.
- **Body**: `{"name": "Novo Nome"}`
- **Validação**: Nome único, case-insensitive
- **Autenticação**: Requer role `rmi-admin`

##### DELETE /admin/beta/groups/{group_id}
Remove um grupo beta e todas as associações de telefones.
- **Limpeza**: Remove automaticamente todos os telefones do grupo
- **Autenticação**: Requer role `rmi-admin`

##### GET /admin/beta/whitelist
Lista telefones na whitelist com paginação.
- **Parâmetros**: `page`, `per_page`, `group_id` (filtro opcional)
- **Autenticação**: Requer role `rmi-admin`

##### POST /admin/beta/whitelist/{phone_number}
Adiciona um telefone a um grupo beta.
- **Body**: `{"group_id": "uuid-do-grupo"}`
- **Validação**: Telefone não pode estar em outro grupo
- **Autenticação**: Requer role `rmi-admin`

##### DELETE /admin/beta/whitelist/{phone_number}
Remove um telefone da whitelist beta.
- **Autenticação**: Requer role `rmi-admin`

##### POST /admin/beta/whitelist/bulk-add
Adiciona múltiplos telefones a um grupo.
- **Body**: `{"phone_numbers": ["+5511999887766"], "group_id": "uuid"}`
- **Autenticação**: Requer role `rmi-admin`

##### POST /admin/beta/whitelist/bulk-remove
Remove múltiplos telefones da whitelist.
- **Body**: `{"phone_numbers": ["+5511999887766"]}`
- **Autenticação**: Requer role `rmi-admin`

##### POST /admin/beta/whitelist/bulk-move
Move telefones entre grupos.
- **Body**: `{"phone_numbers": ["+5511999887766"], "from_group_id": "uuid", "to_group_id": "uuid"}`
- **Autenticação**: Requer role `rmi-admin`

### Modelos de Dados

#### BetaGroup
```json
{
  "id": "uuid-do-grupo",
  "name": "Nome do Grupo",
  "created_at": "2025-08-07T15:30:00Z",
  "updated_at": "2025-08-07T15:30:00Z"
}
```

#### BetaStatusResponse
```json
{
  "phone_number": "+5511999887766",
  "beta_whitelisted": true,
  "group_id": "uuid-do-grupo",
  "group_name": "Nome do Grupo"
}
```

#### PhoneStatusResponse (Atualizado)
```json
{
  "phone_number": "+5511999887766",
  "found": true,
  "quarantined": false,
  "cpf": "12345678901",
  "name": "Nome do Cidadão",
  "quarantine_until": null,
  "beta_whitelisted": true,
  "beta_group_id": "uuid-do-grupo",
  "beta_group_name": "Nome do Grupo"
}
```

### Configuração

#### Variáveis de Ambiente
| Variável | Descrição | Padrão | Obrigatório |
|----------|-----------|---------|------------|
| BETA_STATUS_CACHE_TTL | TTL do cache de status beta (ex: "24h", "1h") | 24h | Não |
| MONGODB_BETA_GROUP_COLLECTION | Nome da coleção de grupos beta | beta_groups | Não |

### Características Técnicas

#### Cache Redis
- **TTL Configurável**: Cache de status beta com TTL personalizável
- **Invalidação Inteligente**: Cache limpo quando associações mudam
- **Performance**: Verificações rápidas sem consulta ao banco

#### Banco de Dados
- **Índices Otimizados**: Índices para consultas eficientes
- **Integridade**: Constraints para nomes únicos de grupos
- **Cascade**: Limpeza automática de associações ao deletar grupos

#### Segurança
- **Controle de Acesso**: Endpoints administrativos requerem role `rmi-admin`
- **Validação**: Verificação de duplicatas e dados válidos
- **Auditoria**: Logs de todas as operações administrativas

#### Performance
- **Cache Inteligente**: Cache Redis para verificações frequentes
- **Paginação**: Listagens paginadas para grandes volumes
- **Índices**: Índices otimizados para consultas rápidas

## 🔍 Tracing e Monitoramento de Performance

### Visão Geral
O sistema RMI agora possui **tracing abrangente** usando OpenTelemetry (OTel) e SignOz, permitindo identificação precisa de gargalos de performance e observabilidade completa de todas as operações.

### Funcionalidades Principais

#### **1. Tracing de Operações HTTP**
- **Middleware automático** adiciona spans detalhados para cada requisição
- **Atributos HTTP**: método, URL, rota, user-agent, client-IP
- **Timing automático** de toda a requisição
- **Métricas de latência** por endpoint

#### **2. Tracing de Operações de Banco**
- **Spans específicos** para operações MongoDB
- **Atributos de banco**: operação, coleção, sistema
- **Timing individual** de cada operação de banco
- **Rastreamento de queries** lentas

#### **3. Tracing de Cache Redis**
- **Instrumentação completa** de todas as operações Redis
- **Atributos enriquecidos**: operação, chave, cliente, duração
- **Métricas de performance** Redis em tempo real
- **Identificação de gargalos** de cache

#### **4. Sistema de Auditoria Automática**
- **Registro automático** de todas as mudanças de dados
- **Tracing completo** de eventos de auditoria
- **Logs estruturados** para análise de compliance
- **Configurável** via `AUDIT_LOGS_ENABLED`

### Métricas Disponíveis

#### **Performance**
```yaml
# Duração de operações
app_rmi_operation_duration_seconds{operation="update_ethnicity"}

# Uso de memória
app_rmi_operation_memory_bytes{operation="update_ethnicity"}

# Checkpoints de performance
app_rmi_performance_checkpoints_total{operation="update_ethnicity"}
```

#### **Redis**
```yaml
# Contadores de operações
app_rmi_redis_operations_total{operation="del", status="success"}

# Duração das operações
app_rmi_redis_operation_duration_seconds{operation="del"}

# Status de operações
app_rmi_redis_operations_total{operation="get", status="error"}
```

### Configuração

#### **Variáveis de Ambiente**
```bash
# Tracing
TRACING_ENABLED=true
TRACING_ENDPOINT=localhost:4317

# Auditoria
AUDIT_LOGS_ENABLED=true
```

#### **Middleware Automático**
```go
// Adicionado automaticamente:
router.Use(
    middleware.RequestTiming(),    // Timing abrangente
    middleware.RequestID(),
    middleware.RequestLogger(),
    middleware.RequestTracker(),
)
```

### Casos de Uso

#### **1. Debug de Operações Lentas**
```go
// Exemplo: UpdateSelfDeclaredRaca levando 24s
// Agora você verá:
- parse_input: 5ms
- validate_ethnicity: 2ms  
- find_existing_data: 5ms
- upsert_document: 475ms
- invalidate_cache: 23.5s  ← GARGALO IDENTIFICADO!
- log_audit_event: 10ms
- serialize_response: 1ms
```

#### **2. Monitoramento de Redis**
- **Operações lentas** são identificadas automaticamente
- **Timeouts** e falhas são rastreados
- **Performance** de cache é monitorada em tempo real

#### **3. Auditoria Automática**
- **Mudanças de etnia** são auditadas automaticamente
- **Atualizações de endereço** são rastreadas
- **Modificações de telefone** são registradas
- **Alterações de email** são documentadas

### Utilitários Disponíveis

#### **Performance Monitor**
```go
monitor := utils.NewPerformanceMonitor(ctx, "update_ethnicity")
defer monitor.End()

monitor.Checkpoint("parse_input")
monitor.Checkpoint("database_update")
monitor.Checkpoint("cache_invalidation")

// Avisos automáticos
monitor.PerformanceWarning(1*time.Second, "Operação muito lenta")
monitor.MemoryWarning(1024*1024, "Uso de memória alto")
```

#### **Tracing Utils**
```go
// Tracing de operações
ctx, span, cleanup := utils.TraceOperation(ctx, "custom_op", attrs)

// Tracing de banco
ctx, span, cleanup := utils.TraceDatabaseOperation(ctx, "find", "citizens", filter)

// Tracing de cache
ctx, span, cleanup := utils.TraceCacheOperation(ctx, "get", "user:123")
```

### Dashboard SignOz

#### **Métricas Principais**
- **Latência de requisições** por endpoint
- **Duração de operações** por tipo
- **Uso de memória** por operação
- **Operações Redis** por status

#### **Traces**
- **Span tree** completo de cada requisição
- **Timing** de cada operação
- **Erros** e exceções
- **Dependências** entre serviços

#### **Alertas Recomendados**
- Operações > 1 segundo
- Uso de memória > 100MB
- Taxa de erro > 5%
- Latência Redis > 100ms

---

## Melhorias Implementadas

### 🔒 Sistema de Auditoria
- **Audit Trail Completo**: Registra todas as mudanças de dados com metadados
- **Contexto do Usuário**: Captura IP, user agent, ID do usuário
- **Limpeza Automática**: Remove logs de auditoria após 1 ano
- **Estrutura Compliance**: Formato estruturado para requisitos regulatórios

### ⚡ Controle de Concorrência (Optimistic Locking)
- **Versionamento**: Cada documento tem um campo `version` que é incrementado a cada atualização
- **Prevenção de Conflitos**: Impede que atualizações simultâneas sobrescrevam dados
- **Retry Automático**: Lógica de retry com backoff exponencial para conflitos
- **Performance**: Não bloqueia operações de leitura

### ✅ Validação de Dados Abrangente
- **Endereços**: Validação de CEP, campos obrigatórios, limites de caracteres
- **Emails**: Validação RFC-compliant, normalização automática
- **Telefones**: Validação internacional usando libphonenumber (Google) - endpoint `/validate/phone`
- **Etnias**: Validação contra opções predefinidas
- **Sanitização**: Limpeza automática de dados de entrada
- **Nota**: A validação de telefone usa a implementação profissional já existente com libphonenumber

### 🔄 Transações de Banco de Dados
- **Operações Atômicas**: Garante consistência em operações multi-coleção
- **Rollback Automático**: Reverte mudanças em caso de falha
- **Sessões MongoDB**: Gerenciamento adequado de sessões

### 🧹 Limpeza Automática
- **TTL Indexes**: Remoção automática de códigos de verificação expirados
- **Audit Logs**: Limpeza automática após 1 ano
- **Performance**: Mantém o banco de dados otimizado

### 📱 Melhorias no WhatsApp
- **Parâmetro Único**: Uso do parâmetro "COD" no template HSM
- **Tratamento de Erros**: Melhor manipulação de falhas no envio
- **Logs Estruturados**: Rastreamento completo do processo de envio

## Melhorias Futuras

### 🔐 Criptografia de Dados
- **Criptografia de Campo**: Proteção de dados sensíveis (CPF, endereços, telefones)
- **Chaves Gerenciadas**: Sistema de gerenciamento de chaves de criptografia
- **Busca Criptografada**: Hash para busca sem revelar dados originais
- **Rotação de Chaves**: Atualização periódica de chaves de criptografia

### 📊 Event Sourcing
- **Histórico Completo**: Armazenamento de todos os eventos de mudança
- **Replay de Eventos**: Capacidade de recriar estado em qualquer momento
- **Consultas Temporais**: "Qual era o endereço em 1º de janeiro?"
- **Auditoria Avançada**: Rastreamento completo de mudanças para compliance

### 🔄 Padrão CQRS (Command Query Responsibility Segregation)
- **Separação de Responsabilidades**: Modelos otimizados para leitura e escrita
- **Performance**: Otimização independente de operações de leitura e escrita
- **Escalabilidade**: Escala separada para leituras e escritas
- **Flexibilidade**: Diferentes modelos para diferentes casos de uso

### ⚡ Cache Multi-Nível
- **Cache em Memória**: Cache de aplicação para dados mais acessados
- **Cache Redis**: Cache distribuído para múltiplas instâncias
- **Cache de Banco**: Fallback para dados persistentes
- **Estratégias de TTL**: Diferentes tempos de vida para diferentes tipos de dados

### 🚀 Outras Melhorias
- **API GraphQL**: Interface mais flexível para consultas complexas
- **Webhooks**: Notificações em tempo real de mudanças
- **Rate Limiting**: Proteção contra abuso da API
- **API Versioning**: Controle de versões da API
- **Documentação Interativa**: Swagger UI melhorado
- **Testes de Carga**: Validação de performance sob carga

## Desenvolvimento

### Pré-requisitos
- Go 1.21 ou superior
- MongoDB
- Redis
- Serviço de API do WhatsApp

### Compilação
```bash
go build -o api cmd/api/main.go
```

### Execução
```bash
./api
```

### Testes
```bash
go test ./...
```

## Licença

MIT

### 🚀 **Executando os Serviços**

### **Comandos Justfile Disponíveis**

A API RMI agora inclui comandos justfile para facilitar o desenvolvimento e execução dos serviços:

#### **🔨 Build Commands**
```bash
# Build do serviço API
just build

# Build do serviço Sync
just build-sync

# Build de ambos os serviços
just build-all
```

#### **🏃‍♂️ Run Commands**
```bash
# Executar apenas a API
just run

# Executar apenas o serviço Sync
just run-sync

# Executar ambos os serviços (em terminais separados)
just run-all

# Iniciar ambos os serviços usando script de startup
just start-services
```

#### **🧪 Test Commands**
```bash
# Executar testes
just test

# Executar testes com cobertura
just test-coverage

# Executar testes com race detection
just test-race

# Executar demo do sistema de cache
just demo-cache
```

#### **🐳 Docker Commands**
```bash
# Build da imagem Docker
just docker-build

# Executar container Docker
just docker-run
```

#### **📊 Dependencies Commands**
```bash
# Iniciar todas as dependências (MongoDB + Redis)
just start-deps

# Parar todas as dependências
just stop-deps

# Iniciar MongoDB
just mongodb-start

# Iniciar Redis
just redis-start
```

### **🔄 Executando o Sistema Completo**

#### **Opção 1: Script de Startup (Recomendado)**
```bash
# 1. Iniciar dependências
just start-deps

# 2. Iniciar ambos os serviços
just start-services
```

#### **Opção 2: Terminais Separados**
```bash
# Terminal 1: API Service
just run

# Terminal 2: Sync Service  
just run-sync
```

#### **Opção 3: Usando Tmux**
```bash
# Criar sessão tmux com ambos os serviços
just run-all
```

### **📊 Monitorando o Sistema**

#### **Verificar Status dos Serviços**
```bash
# Verificar se a API está rodando
curl http://localhost:8080/v1/health

# Verificar processos
ps aux | grep api
ps aux | grep sync

# Verificar portas
netstat -an | grep 8080
```

#### **Monitorar Redis**
```bash
# Verificar filas de sync
redis-cli LLEN sync:queue:citizen
redis-cli LLEN sync:queue:phone_mapping

# Verificar chaves de cache
redis-cli keys "citizen:write:*"
redis-cli keys "citizen:cache:*"

# Verificar DLQ
redis-cli LLEN sync:dlq:citizen
```

#### **Executar Demo do Cache**
```bash
# Executar script de demonstração
just demo-cache
```

### **🔧 Desenvolvimento**

#### **Hot Reload para API**
```bash
# Executar com hot reload usando Air
just dev
```

#### **Logs em Tempo Real**
```bash
# Ver logs da API (se disponível)
tail -f logs/api.log

# Ver logs do sync (se disponível)
tail -f logs/sync.log
```

#### **Debug e Troubleshooting**
```bash
# Verificar linting
just lint

# Verificar dependências
just deps-check

# Atualizar dependências
just deps-update
```

### **📋 Checklist de Deploy**

#### **Antes de Executar**
- [ ] MongoDB rodando na porta 27017
- [ ] Redis rodando na porta 6379
- [ ] Variáveis de ambiente configuradas
- [ ] Dependências Go instaladas

#### **Verificação de Funcionamento**
- [ ] API responde em `/v1/health`
- [ ] Sync workers estão processando jobs
- [ ] Redis filas estão sendo consumidas
- [ ] Métricas estão sendo coletadas

#### **Monitoramento Contínuo**
- [ ] Profundidade das filas Redis
- [ ] Taxa de sucesso dos sync workers
- [ ] Uso de memória Redis
- [ ] Status do modo degradado

---

**💡 Dica**: Use `just start-services` para uma experiência de desenvolvimento mais fluida. O script gerencia ambos os serviços automaticamente e fornece logs consolidados.
