# Citizen Data API

API for managing citizen data with self-declared information. This API provides endpoints for retrieving and updating citizen information, with support for caching and data validation.

## Features

- 🔍 Citizen data retrieval by CPF
- 🔄 Self-declared data updates with validation
- 📱 Phone number verification via WhatsApp
- 💾 Redis caching for improved performance
- 📊 Prometheus metrics for monitoring
- 🔍 OpenTelemetry tracing for request tracking
- 📝 Structured logging with Zap

## Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| PORT | Port to run the server on | 8080 | No |
| MONGODB_URI | MongoDB connection string | mongodb://localhost:27017 | Yes |
| MONGODB_DATABASE | MongoDB database name | citizen_data | No |
| MONGODB_CITIZEN_COLLECTION | Collection name for citizen data | citizens | No |
| MONGODB_SELF_DECLARED_COLLECTION | Collection name for self-declared data | self_declared | No |
| MONGODB_PHONE_VERIFICATION_COLLECTION | Collection name for phone verification data | phone_verifications | No |
| REDIS_URI | Redis connection string | redis://localhost:6379 | Yes |
| REDIS_TTL | TTL for Redis cache in minutes | 60 | No |
| PHONE_VERIFICATION_TTL | TTL for phone verification codes (e.g., "15m", "1h") | 15m | No |
| WHATSAPP_API_URL | WhatsApp API URL for sending verification codes | http://localhost:3000 | Yes |
| WHATSAPP_API_KEY | API key for WhatsApp service | | Yes |
| WHATSAPP_TEMPLATE_NAME | Template name for verification messages | verification_code | No |
| WHATSAPP_NAMESPACE | Namespace for WhatsApp templates | citizen_verification | No |
| WHATSAPP_LANGUAGE | Language for WhatsApp messages | pt_BR | No |
| LOG_LEVEL | Logging level (debug, info, warn, error) | info | No |
| METRICS_PORT | Port for Prometheus metrics | 9090 | No |
| TRACING_ENABLED | Enable OpenTelemetry tracing | false | No |
| TRACING_ENDPOINT | OpenTelemetry collector endpoint | http://localhost:4317 | No |

## API Endpoints

### GET /citizen/{cpf}
Retrieves citizen data by CPF, combining base data with any self-declared updates.
- Self-declared data takes precedence over base data
- Results are cached using Redis with configurable TTL

### PUT /citizen/{cpf}/address
Updates or creates the self-declared address for a citizen.
- Only the address field is updated
- Address is automatically validated

### PUT /citizen/{cpf}/phone
Updates or creates the self-declared phone for a citizen.
- Only the phone field is updated
- Phone number requires verification via WhatsApp
- Verification code is sent to the provided number

### PUT /citizen/{cpf}/email
Updates or creates the self-declared email for a citizen.
- Only the email field is updated
- Email is automatically validated

### POST /citizen/{cpf}/phone/verify
Validates a phone number using a verification code.
- Code is sent via WhatsApp when phone is updated
- Code expires after configured TTL (default: 15 minutes)
- Phone is marked as verified after successful validation

## Data Models

### Citizen
Main data model containing all citizen information:
- Basic information (name, CPF, etc.)
- Contact information (address, phone, email)
- Health information
- Metadata (last update, etc.)

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