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

## Development Guidelines

### NO MOCKING POLICY
**CRITICAL**: Never mock any behavior of the API. All implementations must be real and functional. If functionality is not yet implemented, use `NotImplementedError` exceptions rather than fake/mock implementations. This ensures the API behaves authentically and prevents false positive testing.

### TESTING INTEGRITY RULES
**CRITICAL TESTING PRINCIPLES - NEVER VIOLATE THESE**:

1. **NO SILENT FAILURES**: Never use `except: pass` or similar patterns that hide test failures
2. **NO 500 ACCEPTANCE**: Never accept 500 status codes as "passing" - they indicate server errors that must be fixed
3. **FIX THE CODE, NOT THE TEST**: If a test fails, fix the underlying code/configuration, don't make the test more permissive
4. **NEVER REMOVE TESTS**: Removing failing tests is worse than having them fail - fix them instead
5. **PROPER TEST ISOLATION**: Set up proper test fixtures and mocks that don't break the system under test
6. **MEANINGFUL ASSERTIONS**: Every test must have specific, meaningful assertions that verify correct behavior
7. **NO FALSE POSITIVES**: Tests that pass for the wrong reasons are worse than failing tests
8. **COMPLETE TEST SETUP**: If testing authenticated endpoints, set up proper authentication - don't skip the test

### Git Workflow
This project follows a structured git workflow:

#### Branch Structure
- **main**: Production-ready code only
- **staging**: Integration testing and pre-production  
- **feature branches**: Active development work with meaningful names

#### Branch Naming Convention
- **feat/**: New features (e.g., `feat/avatar-system`, `feat/phone-validation`)
- **fix/**: Bug fixes (e.g., `fix/auth-middleware`, `fix/memory-leak`)
- **docs/**: Documentation updates (e.g., `docs/api-specification`)
- **refactor/**: Code refactoring (e.g., `refactor/cache-layer`)
- **test/**: Test improvements (e.g., `test/integration-coverage`)

#### Development Rules
1. Always create a new feature branch from **staging** for new work
2. Use descriptive branch names with appropriate prefixes
3. Test every batch of implemented tasks before committing
4. Never push commits directly - create PRs for code review
5. Write descriptive commit messages with proper attribution:
   ```
   feat: implement Netflix-style avatar system
   
   - Add avatar CRUD operations with caching
   - Implement user avatar selection endpoints
   - Add background cleanup for orphaned references
   
   🤖 Generated with [Claude Code](https://claude.ai/code)
   
   Co-Authored-By: Claude <noreply@anthropic.com>
   ```

#### Testing Before Commits
- Run `just lint` to ensure code quality
- Run `just build` to verify compilation
- Test with `just run` and `just run-sync` to verify services start
- **CRITICAL**: Run `./scripts/test_api.sh <CPF> --auto-token --skip-phone` and ensure ALL tests pass
- Verify health endpoints still respond
- **CRITICAL**: Never commit with failing tests - fix them instead

#### Test Development Rules (test_api.sh Focus)
- **NO SKIPPING TESTS**: When `test_api.sh` tests fail, they must be fixed, not skipped or commented out
- **ALL TESTS MUST PASS**: The test script must show "🎉 All tests passed!" before committing
- **ADD TESTS FOR NEW FEATURES**: When implementing new endpoints, add corresponding test cases to `test_api.sh`
- **FIX UNDERLYING ISSUES**: If tests fail, debug and fix the API code, don't modify test expectations
- **PROPER ERROR HANDLING**: Ensure test script correctly validates both success and error scenarios
- **COMPREHENSIVE COVERAGE**: New features must have test cases covering success, validation errors, and edge cases
- **AUTHENTICATION TESTING**: Protected endpoints must be tested with proper JWT tokens via `--auto-token`
- **RESPONSE VALIDATION**: Tests must verify response structure, status codes, and data integrity

#### Documentation Maintenance Rules
- **CRITICAL**: Always keep documentation up to date when making code changes
- Update OpenAPI specification (`docs/`) when adding/modifying endpoints
- Update README.md when changing setup, configuration, or usage instructions
- Update CLAUDE.md when adding new development patterns or architectural changes
- Document complex business logic and performance considerations
- Include examples in API documentation for new endpoints

#### Code Quality Standards
- Follow Go best practices and idiomatic patterns
- Use proper error handling with meaningful error messages
- Implement comprehensive logging with appropriate levels
- Add metrics and observability for new features
- Ensure thread safety for concurrent operations
- Write clear, self-documenting code with minimal comments