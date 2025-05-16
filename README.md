# RMI API

API for managing citizen data with self-declared information. This API provides endpoints for retrieving and updating citizen information, with support for caching and data validation.

## Features

- ✨ Versioned API endpoints (v1)
- 🔄 Self-declared data updates with validation
- 📝 Complete citizen data retrieval
- 🚀 High performance with Redis caching
- 📊 Prometheus metrics
- 🔍 OpenTelemetry tracing
- 📝 Structured logging with Zap
- 🔄 Circuit breaker for Redis
- 🌐 CORS support
- 📚 Swagger UI documentation
- 🐳 Docker support
- ☸️ Kubernetes/Knative ready
- 🏗️ Modern development workflow with Just

## Development Setup

### Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose
- Just command runner
- MongoDB
- Redis

### Quick Start

1. Clone the repository:
   ```bash
   git clone https://github.com/prefeitura-rio/app-rmi.git
   cd app-rmi
   ```

2. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

3. Start dependencies (MongoDB and Redis):
   ```bash
   just mongodb-start
   just redis-start
   ```

4. Run the application:
   ```bash
   just run
   ```

The API will be available at `http://localhost:8080` with the following endpoints:
- API Documentation (Swagger UI): `http://localhost:8080/swagger/index.html`
- Metrics (Prometheus): `http://localhost:8080/metrics`

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| PORT | Server port | 8080 |
| ENVIRONMENT | Environment (development/production) | development |
| MONGODB_URI | MongoDB connection URI | mongodb://localhost:27017 |
| MONGODB_DATABASE | MongoDB database name | rmi |
| REDIS_ADDR | Redis server address | localhost:6379 |
| REDIS_PASSWORD | Redis password | "" |
| REDIS_DB | Redis database number | 0 |
| CACHE_TTL | Cache duration (e.g., 15m, 1h, 24h) | 1h |

### Available Commands

- `just run` - Run the application
- `just dev` - Run the application with hot reload
- `just swagger` - Generate Swagger documentation
- `just test` - Run tests
- `just fmt` - Format code
- `just lint` - Run linters
- `just mongodb-start` - Start MongoDB container
- `just redis-start` - Start Redis container
- `just clean` - Clean build artifacts

## API Documentation

### Base URL

All API endpoints are prefixed with `/api/v1`

### Endpoints

#### GET /api/v1/citizen/{cpf}
Retrieves citizen data by CPF, combining base data with any self-declared updates.
- Self-declared data takes precedence over base data
- Results are cached using Redis with configurable TTL
- Requires valid 11-digit CPF

#### PUT /api/v1/citizen/{cpf}/self-declared
Updates or creates self-declared information for a citizen.
- Only specific fields (address and contact) can be updated
- All fields are optional
- Input validation for all fields
- Cache is automatically invalidated on updates

#### GET /api/v1/health
Health check endpoint that verifies the status of all dependencies:
- MongoDB connection
- Redis connection
- Application status

#### GET /metrics
Prometheus metrics endpoint providing:
- Request duration
- Cache hits/misses
- Database operations
- Active connections
- Self-declared updates
- System metrics

### Data Models

#### Address Update
```json
{
  "logradouro": "Rua das Flores",
  "numero": "123",
  "complemento": "Apto 101",
  "bairro": "Centro",
  "cidade": "Rio de Janeiro",
  "uf": "RJ",
  "cep": "20000000",
  "tipo_endereco": "RESIDENCIAL"
}
```

#### Contact Update
```json
{
  "telefones": [
    {
      "ddd": "21",
      "numero": "999887766",
      "tipo": "CELULAR",
      "observacoes": "Horário comercial"
    }
  ],
  "emails": [
    {
      "email": "user@example.com",
      "tipo": "PESSOAL",
      "observacoes": "Email principal"
    }
  ]
}
```

## Performance Considerations

- Redis caching for frequently accessed data
- Connection pooling for MongoDB
- Circuit breaker for Redis operations
- Configurable timeouts and retries
- Kubernetes-ready for horizontal scaling

## Monitoring and Observability

- Prometheus metrics for:
  - Request duration
  - Cache hits/misses
  - Database operations
  - Active connections
  - Self-declared updates
- OpenTelemetry tracing
- Structured logging with Zap
- Health checks for all dependencies

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Apache 2.0 License - see the LICENSE file for details. 