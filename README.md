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

## Variáveis de Ambiente

| Variável | Descrição | Padrão | Obrigatório |
|----------|-----------|---------|------------|
| PORT | Porta do servidor | 8080 | Não |
| MONGODB_URI | String de conexão MongoDB | mongodb://localhost:27017 | Sim |
| MONGODB_DATABASE | Nome do banco de dados MongoDB | citizen_data | Não |
| MONGODB_CITIZEN_COLLECTION | Nome da coleção de dados do cidadão | citizens | Não |
| MONGODB_SELF_DECLARED_COLLECTION | Nome da coleção de dados autodeclarados | self_declared | Não |
| MONGODB_PHONE_VERIFICATION_COLLECTION | Nome da coleção de verificação de telefone | phone_verifications | Não |
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

## Endpoints da API

### GET /citizen/{cpf}
Recupera os dados do cidadão por CPF, combinando dados base com atualizações autodeclaradas.
- Dados autodeclarados têm precedência sobre dados base
- Resultados são armazenados em cache usando Redis com TTL configurável

### PUT /citizen/{cpf}/address
Atualiza ou cria o endereço autodeclarado de um cidadão.
- Apenas o campo de endereço é atualizado
- Endereço é validado automaticamente

### PUT /citizen/{cpf}/phone
Atualiza ou cria o telefone autodeclarado de um cidadão.
- Apenas o campo de telefone é atualizado
- Número de telefone requer verificação via WhatsApp
- Código de verificação é enviado para o número fornecido

### PUT /citizen/{cpf}/email
Atualiza ou cria o email autodeclarado de um cidadão.
- Apenas o campo de email é atualizado
- Email é validado automaticamente

### PUT /citizen/{cpf}/ethnicity
Atualiza ou cria a etnia autodeclarada de um cidadão.
- Apenas o campo de etnia é atualizado
- Valor deve ser uma das opções válidas retornadas pelo endpoint /citizen/ethnicity/options

### GET /citizen/ethnicity/options
Retorna a lista de opções válidas de etnia para autodeclaração.
- Usado para validar as atualizações de etnia autodeclarada
- Não requer autenticação

### POST /citizen/{cpf}/phone/validate
Valida um número de telefone usando um código de verificação.
- Código é enviado via WhatsApp quando o telefone é atualizado
- Código expira após o TTL configurado (padrão: 15 minutos)
- Telefone é marcado como verificado após validação bem-sucedida

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

## Caching

The API uses Redis for caching citizen data:
- Cache key: `citizen:{cpf}`
- TTL: Configurable via `REDIS_TTL` (default: 60 minutes)
- Cache is invalidated when self-declared data is updated

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