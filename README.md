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
- 📞 Suporte a WhatsApp bot com phone-based endpoints
- 🔐 Sistema de opt-in/opt-out com histórico detalhado
- 📋 Validação de registros contra dados base
- 🎯 Mapeamento phone-CPF com controle de status

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
- Requer autenticação JWT
- Bloqueia mapeamento phone-CPF
- Registra histórico de opt-out com motivo
- Atualiza dados autodeclarados

### POST /phone/{phone_number}/reject-registration
Rejeita um registro e bloqueia mapeamento phone-CPF.
- Requer autenticação JWT com acesso ao CPF
- Bloqueia mapeamento existente
- Registra rejeição no histórico
- Permite novo registro para o número

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