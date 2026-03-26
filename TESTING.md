# Testing Guide

This document explains how to run the comprehensive test suite for the RMI application.

## Test Coverage Progress

**Overall Coverage: 8.6%** (Target: 70%+)

### Current Coverage by Module:
- ✅ **Models**: 37.6% coverage
- ✅ **Utils**: 24.7% coverage
  - Validation: 100% coverage (all 13 functions fully tested)
  - CPF/CNPJ: 91%+ coverage
  - Phone parsing: 89-100% coverage
  - Name extraction/masking: 91-100% coverage
  - UUID/Verification/Bool: 80-100% coverage

## Test Types

### 1. Unit Tests (No Dependencies)
Pure function tests that don't require external services:
- Validation logic
- CPF/CNPJ validation
- Phone number parsing
- Name extraction and masking
- Model validation functions
- JWT claim parsing

### 2. Integration Tests (Require Services)
Tests that interact with real MongoDB and Redis:
- MongoDB utility functions (`internal/utils/mongodb_test.go`)
- Redis pipeline operations (`internal/utils/redis_pipeline_test.go`)

## Running Tests

### Quick Test (Unit Tests Only)
```bash
# Run all unit tests (no external dependencies needed)
just test

# Run with coverage
just test-coverage

# Run with race detection
just test-race
```

### Integration Tests (Requires MongoDB + Redis)

**Step 1: Start Dependencies**
```bash
# Using docker-compose
docker-compose up -d mongodb redis

# OR using Just command
just start-deps
```

**Step 2: Run Integration Tests**
```bash
# Set environment variables
export MONGODB_URI="mongodb://root:password@localhost:27017"
export REDIS_ADDR="localhost:6380"
export REDIS_PASSWORD=""

# Run all tests including integration tests
go test -v ./...

# Run only MongoDB integration tests
go test -v -run TestMongoDB ./internal/utils/

# Run only Redis integration tests
go test -v -run TestRedisPipeline ./internal/utils/
```

### CI-Style Testing (Like GitHub Actions)
```bash
# This sets all required environment variables and runs tests
just test-ci
```

## Test Organization

### Utils Layer (`internal/utils/*_test.go`)
- `cpf_test.go` - CPF and CNPJ validation (23 test cases)
- `phone_parsing_test.go` - Phone number parsing (12 test cases)
- `name_extraction_test.go` - Name extraction and masking (22 test cases)
- `bool_test.go` - Boolean helper functions
- `uuid_test.go` - UUID generation (3 test cases)
- `verification_test.go` - Verification code generation (4 test cases)
- `validation_test.go` - **100% coverage** - Input validation and sanitization (65+ test cases)
- `mongodb_test.go` - MongoDB utility integration tests (11 test scenarios)
- `redis_pipeline_test.go` - Redis pipeline integration tests (6 test scenarios)

### Models Layer (`internal/models/*_test.go`)
- `citizen_test.go` - Citizen model validation and conversion (14 test functions)
- `jwt_test.go` - JWT claims parsing (7 test cases)
- `errors_test.go` - Error constant validation
- `phone_mapping_test.go` - Phone mapping constants

## Writing New Tests

### Test Naming Convention
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name  string
        input YourInput
        want  YourExpected
        wantErr bool
    }{
        {
            name: "Description of test case",
            input: YourInput{...},
            want: YourExpected{...},
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := YourFunction(tt.input)
            // assertions...
        })
    }
}
```

### Integration Test Pattern
```go
func TestYourIntegration(t *testing.T) {
    // Check for required environment variables
    requiredVar := os.Getenv("REQUIRED_VAR")
    if requiredVar == "" {
        t.Skip("Skipping integration test: REQUIRED_VAR not set")
    }

    // Setup
    // ... initialize connections

    // Cleanup
    defer func() {
        // ... cleanup resources
    }()

    // Run tests
    t.Run("Test scenario", func(t *testing.T) {
        // test code
    })
}
```

## Test Principles

### TDD Approach
Following Test-Driven Development principles as outlined in `CLAUDE.md`:

1. **Tests define correct behavior** - Write tests first to specify how code should work
2. **Fix the code, not the test** - If tests fail, the implementation is wrong (not the test)
3. **No false positives** - Tests must have meaningful assertions
4. **No silent failures** - Never use `except: pass` or similar patterns
5. **Complete test setup** - Set up proper fixtures and dependencies

### Test Coverage Goals

According to `UNIT_TEST_PLAN.md`:
- **Utils Layer**: 90%+ coverage (currently 24.7%, needs more integration tests)
- **Models Layer**: 80%+ coverage (currently 37.6%, on track)
- **Services Layer**: 75%+ coverage (pending)
- **Middleware Layer**: 70%+ coverage (pending)
- **Handlers Layer**: 60%+ coverage (pending)
- **Overall**: 70%+ coverage (currently 8.6%)

## Troubleshooting

### MongoDB Connection Issues
```bash
# Check if MongoDB is running
docker ps | grep mongodb

# Check MongoDB logs
docker logs rmi-mongodb

# Test connection
mongosh "mongodb://root:password@localhost:27017"
```

### Redis Connection Issues
```bash
# Check if Redis is running
docker ps | grep redis

# Check Redis logs
docker logs rmi-redis

# Test connection
redis-cli -p 6380 ping
```

### Test Failures
```bash
# Run specific test with verbose output
go test -v -run TestSpecificFunction ./internal/utils/

# Run with race detection
go test -race -v -run TestSpecificFunction ./internal/utils/

# Clean test cache
go clean -testcache
```

## Next Steps

To reach 70%+ coverage, we need to:
1. ✅ Complete MongoDB integration tests
2. ✅ Complete Redis integration tests
3. ⏳ Test middleware functions (with Gin test context)
4. ⏳ Test service layer (with real MongoDB/Redis)
5. ⏳ Test handlers (integration tests with full setup)

See `UNIT_TEST_PLAN.md` for the complete testing roadmap.
