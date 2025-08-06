# API RMI

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
| LOG_LEVEL | Nível de log (debug, info, warn, error) | info | Não |
| METRICS_PORT | Porta para métricas Prometheus | 9090 | Não |
| TRACING_ENABLED | Habilitar rastreamento OpenTelemetry | false | Não |
| TRACING_ENDPOINT | Endpoint do coletor OpenTelemetry | http://localhost:4317 | Não |
| INDEX_MAINTENANCE_INTERVAL | Intervalo para verificação de índices (ex: "1h", "24h") | 1h | Não |
| WHATSAPP_COD_PARAMETER | Parâmetro do código no template HSM do WhatsApp | COD | Não |

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

## Modelos de Dados

### Citizen
Modelo principal contendo todas as informações do cidadão:
- Informações básicas (nome, CPF, etc.)
- Informações de contato (endereço, telefone, email)
- Informações de saúde
- Metadados (última atualização, etc.)

### SelfDeclaredData
Stores self-declared updates to citizen data:
- Only stores fields that have been updated
- Includes validation status
- Maintains update history

### PhoneVerification
Manages phone number verification process:
- Stores verification codes
- Tracks verification status
- Handles code expiration
- Automatic cleanup via TTL indexes
- Optimized queries with compound indexes

### AuditLog
Comprehensive audit trail system:
- Tracks all data changes with metadata
- Records user context (IP, user agent, user ID)
- Automatic cleanup after 1 year
- Structured for compliance and debugging

## Caching

The API uses Redis for caching citizen data:
- Cache key: `citizen:{cpf}`
- TTL: Configurable via `REDIS_TTL` (default: 60 minutes)
- Cache is invalidated when self-declared data is updated
- Comprehensive cache invalidation for related data
- Cache invalidation for citizen, wallet, and maintenance request data

## Monitoring

### Metrics
Prometheus metrics are available at `/metrics`:
- Request counts and durations
- Cache hits and misses
- Self-declared updates
- Phone verifications

### Tracing
OpenTelemetry tracing is available when enabled:
- Request tracing
- Database operations
- Cache operations
- External service calls

### Logging
Structured logging with Zap:
- Request logging
- Error tracking
- Performance monitoring
- Audit trail

### Index Management
The API automatically manages MongoDB indexes to ensure optimal query performance:
- **Automatic Index Creation**: Creates required indexes on startup if they don't exist
- **Periodic Verification**: Checks for indexes at configurable intervals and recreates them if missing
- **Multi-Instance Safe**: Uses MongoDB's `createIndex` with background building and duplicate key error handling
- **Collection Overwrite Protection**: Ensures indexes exist after BigQuery/Airbyte collection overwrites
- **Configurable Interval**: Set via `INDEX_MAINTENANCE_INTERVAL` environment variable (default: 1h)

**Managed Indexes:**
- `citizen` collection: Unique index on `cpf` field (`cpf_1`)
- `maintenance_request` collection: Index on `cpf` field (`cpf_1`)
- `self_declared` collection: Unique index on `cpf` field (`cpf_1`)
- `phone_verifications` collection: 
  - Unique compound index on `cpf` and `phone_number` (`cpf_1_phone_number_1`)
  - TTL index on `expires_at` for automatic cleanup (`expires_at_1`)
  - Compound index for verification queries (`verification_query_1`)
- `user_config` collection: Unique index on `cpf` field (`cpf_1`)
- `audit_logs` collection:
  - Index on `cpf` field (`cpf_1`)
  - Index on `timestamp` field (`timestamp_1`)
  - Compound index on `action` and `resource` (`action_1_resource_1`)
  - TTL index for automatic cleanup after 1 year (`timestamp_ttl`)

**Safety Features:**
- **Background Index Building**: Indexes are built in the background, allowing other operations to continue
- **Duplicate Key Handling**: Gracefully handles cases where another instance creates the same index
- **Error Recovery**: Failed index creation doesn't crash the application
- **Concurrent Safety**: Multiple API instances can run index maintenance simultaneously without conflicts

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

## Development

### Prerequisites
- Go 1.21 or later
- MongoDB
- Redis
- WhatsApp API service

### Building
```bash
go build -o api cmd/api/main.go
```

### Running
```bash
./api
```

### Testing
```bash
go test ./...
```

## License

MIT