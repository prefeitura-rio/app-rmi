# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Essential Commands

The project uses [Just](https://github.com/casey/just) for task automation. All commands are in the `justfile`:

### Development Workflow
```bash
just build          # Build API server (cmd/api/main.go)
just build-sync      # Build sync worker (cmd/sync/main.go) 
just build-all       # Build both services
just lint            # Run golangci-lint (installs if needed)
just fmt             # Format code with go fmt
just test            # Run tests
just test-race       # Run tests with race detection
just clean           # Clean test cache and coverage files
```

### Running Services
```bash
just run             # Run API server with hot reload (port 8080)
just run-sync        # Run sync worker
just dev             # Run API with air hot reload
```

**Important**: The system has TWO services that must run together:
- **API server** (`cmd/api/main.go`) - handles HTTP requests  
- **Sync worker** (`cmd/sync/main.go`) - processes background jobs

### Testing
```bash
just test-cpf <CPF>                    # Test with specific CPF
just test-coverage                     # Generate coverage report
./scripts/test_api.sh <CPF> --auto-token --skip-phone  # Full API test
```

### Docker Operations
```bash
just docker-build     # Build container
just start-deps       # Start MongoDB & Redis containers
just stop-deps        # Stop dependency containers
```

## Architecture Overview

### Dual-Service Architecture
- **API Service**: Gin HTTP server handling REST endpoints with authentication, validation, caching
- **Sync Worker**: Background service processing Redis queues for MongoDB operations
- **Communication**: Redis queues bridge API writes to MongoDB persistence

### Multi-Level Caching Strategy
- **Write Buffer**: Redis cache for immediate writes (`<type>:write:<key>`)
- **Read Cache**: Redis cache for read optimization (`<type>:cache:<key>`)
- **DataManager**: Orchestrates cache-first reads with MongoDB fallback
- **Batched Operations**: `getBatchedSelfDeclaredData()` reduces Redis calls from 5→2

### Core Services Architecture

#### Configuration (`internal/config/`)
- **Environment-based**: All settings via environment variables
- **Redis Cluster Support**: Auto-switching between single/cluster modes
- **Performance Optimized**: Pool sizes, timeouts, connection management

#### Data Layer (`internal/services/`)
- **DataManager**: Multi-level cache orchestration
- **CacheService**: Unified cache operations for all data types
- **SyncWorker**: Round-robin queue processing with 50ms intervals
- **BatchOperations**: Pipeline Redis operations for performance

#### Handlers (`internal/handlers/`)
- **Citizen Data**: CPF-based citizen data with self-declared overlays
- **Phone Operations**: Phone validation, opt-in/opt-out, quarantine
- **Beta Groups**: WhatsApp bot beta testing management

### Database Collections Structure
- **Citizens**: Base citizen data (governo data)
- **SelfDeclared**: User-submitted address/phone/email/ethnicity
- **PhoneMapping**: Phone→CPF mappings with status tracking
- **OptInHistory**: Complete opt-in/opt-out audit trail
- **BetaGroups**: WhatsApp beta testing groups
- **AuditLogs**: System audit trail with TTL cleanup

### Performance Optimizations

#### Redis Performance
- **Cluster Support**: Production Redis cluster with optimized settings
- **Connection Pools**: 200 connections (vs 50), 50 min idle (vs 20)
- **Batched Operations**: Pipeline multiple Redis calls
- **Enhanced Monitoring**: 10s intervals with progressive alerting

#### MongoDB Performance  
- **Connection Pools**: 1000 max, 50 min warm connections
- **Write Concerns**: W=0 for performance collections, W=1 for data integrity
- **Compression**: Snappy compression for CPU efficiency
- **Query Timeouts**: 10s default with projection support

#### Sync Worker Performance
- **Parallel Processing**: Round-robin across 11 queues
- **Non-blocking**: RPop instead of blocking BRPop
- **Job Limiting**: Max 3 jobs per 50ms cycle to prevent overwhelming

### Authentication & Security
- **Keycloak Integration**: JWT token validation via middleware
- **Route Protection**: Public phone endpoints, protected citizen data
- **Audit Trail**: Comprehensive logging with async workers
- **Input Validation**: CPF, phone number, data format validation

### Monitoring & Observability
- **OpenTelemetry**: Request tracing with span context
- **Prometheus Metrics**: Performance counters
- **Structured Logging**: Zap logger with performance monitoring
- **Health Checks**: `/v1/health` endpoint with dependency status

## Important Development Notes

### The "Commit Thing"
When asked to do the "commit thing":
1. Format code: `just fmt`
2. Check build: `just build` and `just lint` 
3. Review staged changes with `git diff --staged`
4. Use conventional commits format
5. Commit with meaningful message

### Redis Cluster Configuration
- **Local Development**: Single Redis instance (default)
- **Production**: Redis cluster via `REDIS_CLUSTER_ENABLED=true`
- **Environment Variables**: `REDIS_CLUSTER_ADDRS`, `REDIS_CLUSTER_PASSWORD`

### Performance-Critical Paths
- **GetCitizenData**: Uses batched Redis operations via `getBatchedSelfDeclaredData()`
- **UpdateSelfDeclared***: Field-specific queries vs full citizen data retrieval
- **Sync Workers**: Process background queues with optimized timing

### Testing Dependencies
- **MongoDB**: Requires running instance (use `just start-deps`)
- **Redis**: Required for caching and queue operations
- **Authentication**: Test script needs Keycloak credentials for protected endpoints

### Code Organization
- **Single Responsibility**: Separate API and sync binaries
- **Domain Separation**: Citizen, phone, beta groups have dedicated handlers
- **Performance First**: Batching, connection pooling, caching at every layer
- **Monitoring Built-in**: Connection pool monitoring, health checks, metrics