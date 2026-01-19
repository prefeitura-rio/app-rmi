# Unit Test Coverage Plan

## Current State
- **Total internal packages**: 86 Go files
- **Existing tests**: 3 files (citizen_test.go, phone_verification_test.go, data_manager_test.go)
- **Current coverage**: ~0.3% (only basic services tested)
- **E2E tests**: Comprehensive (13 test files covering all endpoints)

## Testing Strategy

### Phase 1: Utils Layer (Pure Functions - Highest Priority)
**Target Coverage**: 90%+

These are pure functions with no external dependencies - easiest to test and highest value.

#### `internal/utils/validation.go`
- [x] CPF validation (various formats, invalid checksums)
- [ ] Phone number validation (international, DDD, invalid)
- [ ] Email validation (RFC compliance, edge cases)

#### `internal/utils/cpf.go`
- [ ] CPF formatting (with/without punctuation)
- [ ] CPF cleaning/normalization
- [ ] Edge cases (all zeros, sequential numbers)

#### `internal/utils/phone_parsing.go`
- [ ] Brazilian phone parsing (with/without country code)
- [ ] DDD extraction
- [ ] Mobile vs landline detection

#### `internal/utils/name_extraction.go`
- [ ] First name extraction
- [ ] Last name extraction
- [ ] Full name normalization
- [ ] Edge cases (single names, titles, suffixes)

#### `internal/utils/verification.go`
- [ ] Verification token generation
- [ ] Token validation
- [ ] Expiration checks

#### `internal/utils/bool.go`
- [ ] String to bool conversion
- [ ] Various truthy/falsy values

#### `internal/utils/uuid.go`
- [ ] UUID generation
- [ ] UUID validation

### Phase 2: Models Layer (Data Structures)
**Target Coverage**: 80%+

Test serialization, validation, and business logic in models.

#### `internal/models/citizen.go`
- [ ] JSON marshaling/unmarshaling
- [ ] Field validation
- [ ] Empty value handling

#### `internal/models/self_declared.go`
- [ ] Self-declared data merging
- [ ] Validation rules
- [ ] Optional fields handling

#### `internal/models/phone_mapping.go`
- [ ] Status transitions (opt-in, opt-out, quarantined)
- [ ] Timestamp management

#### `internal/models/jwt.go`
- [ ] JWT claims parsing
- [ ] CPF extraction from claims
- [ ] Role extraction

#### `internal/models/errors.go`
- [ ] Custom error types
- [ ] Error wrapping
- [ ] HTTP status code mapping

### Phase 3: Services Layer (Business Logic)
**Target Coverage**: 75%+

Use mocks for external dependencies (MongoDB, Redis).

#### `internal/services/cache_service.go`
- [ ] Cache key generation
- [ ] Cache hit/miss scenarios
- [ ] TTL management
- [ ] Batch operations

#### `internal/services/citizen_cache_service.go`
- [ ] Citizen data caching
- [ ] Cache invalidation
- [ ] Read-through caching

#### `internal/services/phone_mapping_service.go`
- [ ] Phone opt-in flow
- [ ] Phone opt-out flow
- [ ] Quarantine management
- [ ] History tracking

#### `internal/services/avatar_service.go`
- [ ] Avatar CRUD operations
- [ ] Avatar selection
- [ ] Orphan cleanup

#### `internal/services/beta_group_service.go`
- [ ] Beta group creation
- [ ] Member management
- [ ] Beta status checking

#### `internal/services/registration_validator.go`
- [ ] Registration validation rules
- [ ] Required fields checking
- [ ] Format validation

#### `internal/services/rate_limiter.go`
- [ ] Rate limit enforcement
- [ ] Token bucket algorithm
- [ ] Per-user rate limiting

#### `internal/services/cf_lookup_service.go`
- [ ] CF lookup caching
- [ ] MCP client integration (mocked)
- [ ] Error handling

#### `internal/services/legal_entity_service.go`
- [ ] Legal entity CRUD
- [ ] CNPJ validation
- [ ] Associated data fetching

#### `internal/services/pet_service.go`
- [ ] Pet CRUD operations
- [ ] Self-registration handling

#### `internal/services/cnae_service.go`
- [ ] CNAE lookup
- [ ] Classification mapping

#### `internal/services/department_service.go`
- [ ] Department CRUD
- [ ] Department hierarchies

#### `internal/services/notification_category_service.go`
- [ ] Category management
- [ ] User preferences

### Phase 4: Middleware Layer
**Target Coverage**: 70%+

Test HTTP middleware behavior with mock requests.

#### `internal/middleware/auth.go`
- [ ] JWT token validation
- [ ] Unauthorized request handling
- [ ] Token expiration
- [ ] Missing token scenarios
- [ ] Invalid token scenarios

#### `internal/middleware/audit.go`
- [ ] Audit log creation
- [ ] Request body capture
- [ ] CPF extraction
- [ ] Resource type mapping
- [ ] Async processing

#### `internal/middleware/timing.go`
- [ ] Request duration tracking
- [ ] Performance metrics
- [ ] Slow request logging

#### `internal/middleware/observability.go`
- [ ] Span creation
- [ ] Trace context propagation
- [ ] Error recording

### Phase 5: Handlers Layer (HTTP Endpoints)
**Target Coverage**: 60%+

Use httptest for HTTP testing, mock services layer.

#### `internal/handlers/citizen.go`
- [ ] GET /citizen/:cpf (success, not found, invalid CPF)
- [ ] PUT /citizen/:cpf/address (validation, update)
- [ ] PUT /citizen/:cpf/phone (validation, update)
- [ ] PUT /citizen/:cpf/email (validation, update)
- [ ] PUT /citizen/:cpf/ethnicity (validation, update)

#### `internal/handlers/phone.go`
- [ ] Phone validation endpoint
- [ ] Phone opt-in (success, already exists)
- [ ] Phone opt-out (success, not found)

#### `internal/handlers/avatar_handlers.go`
- [ ] GET /avatars (list all)
- [ ] GET /citizen/:cpf/avatar (has avatar, no avatar)
- [ ] PUT /citizen/:cpf/avatar (set avatar, invalid avatar)
- [ ] DELETE /citizen/:cpf/avatar (remove avatar)

#### `internal/handlers/beta_group_handlers.go`
- [ ] List beta groups
- [ ] Create beta group (validation)
- [ ] Get beta group (exists, not found)
- [ ] Add member (success, duplicate, full group)
- [ ] Remove member (success, not in group)
- [ ] Delete beta group

#### `internal/handlers/admin_handlers.go`
- [ ] Quarantine phone (success, validation)
- [ ] Unquarantine phone
- [ ] List quarantined phones
- [ ] Check quarantine status

#### `internal/handlers/legal_entity.go`
- [ ] GET /legal-entity/:cnpj (authorization checks)
- [ ] CRUD operations

#### `internal/handlers/pet.go`
- [ ] Pet CRUD operations
- [ ] Self-registration

### Phase 6: Config and Infrastructure
**Target Coverage**: 50%+

Test configuration loading and validation.

#### `internal/config/config.go`
- [ ] Environment variable loading
- [ ] Required config validation
- [ ] Default values
- [ ] Invalid config handling

#### `internal/config/database.go`
- [ ] MongoDB connection string building
- [ ] Connection pool configuration
- [ ] Redis cluster vs single node

## Test Infrastructure

### Mocking Strategy
1. **MongoDB**: Use `miniredis` or custom mocks
2. **Redis**: Use `miniredis` for lightweight testing
3. **HTTP Clients**: Use `httptest.Server` for external APIs
4. **Time**: Use injectable clock for time-dependent tests

### Test Helpers to Create

```go
// internal/testutil/helpers.go
package testutil

// Mock factories
func NewMockMongoClient() *MockMongoClient
func NewMockRedisClient() *MockRedisClient
func NewMockHTTPClient() *MockHTTPClient

// Test data builders
func BuildCitizen(opts ...func(*models.Citizen)) *models.Citizen
func BuildPhoneMapping(opts ...func(*models.PhoneMapping)) *models.PhoneMapping
func BuildSelfDeclared(opts ...func(*models.SelfDeclared)) *models.SelfDeclared

// HTTP test helpers
func NewAuthenticatedRequest(method, url, body string) *http.Request
func AssertJSONResponse(t *testing.T, resp *httptest.ResponseRecorder, expected interface{})
```

### Coverage Goals by Layer
- **Utils**: 90%+ (pure functions, no excuse)
- **Models**: 80%+ (validation and serialization)
- **Services**: 75%+ (core business logic)
- **Middleware**: 70%+ (request processing)
- **Handlers**: 60%+ (HTTP layer, more E2E covered)
- **Config**: 50%+ (environment dependent)

### Overall Target: 70%+ coverage

## Implementation Order

1. **Week 1**: Utils layer (high value, easy wins)
2. **Week 2**: Models layer (foundation for other tests)
3. **Week 3**: Services layer (core business logic)
4. **Week 4**: Middleware + Handlers (HTTP layer)
5. **Week 5**: Config + Integration improvements

## Continuous Integration
- Run tests on every PR (already done via quality gate)
- Track coverage trends over time
- Require coverage increase for PRs touching critical paths
- Generate coverage reports and upload to artifacts

## Benefits
1. **Bug Prevention**: Catch regressions before they reach production
2. **Refactoring Confidence**: Safe to refactor with comprehensive tests
3. **Documentation**: Tests serve as living documentation
4. **Faster Debugging**: Isolated tests pinpoint issues quickly
5. **Code Quality**: Writing testable code improves design
