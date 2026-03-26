# E2E/Smoke Test Suite

This directory contains the End-to-End (E2E) and smoke test suite for the RMI API.

## Overview

The test suite is built using Go's native testing framework with `testify` for assertions. It provides comprehensive coverage of all API endpoints including:

- Health checks and service status
- Authentication and authorization
- Citizen data operations (read, update)
- Self-declared data (address, phone, email, ethnicity)
- Avatar management (list, set, delete)
- Phone operations (opt-in, opt-out, verification, history)
- WhatsApp bot memory management
- Beta group administration
- Phone quarantine administration

## Architecture

### Test Structure

```
tests/
├── config/           # Test configuration
│   └── test_config.go
├── fixtures/         # Test helpers and utilities
│   ├── test_data.go
│   └── helpers.go
└── e2e/             # E2E test suites
    ├── suite_test.go          # Base test suite
    ├── health_test.go         # Health endpoint tests
    ├── validation_test.go     # Validation endpoint tests
    ├── citizen_test.go        # Citizen data tests
    ├── self_declared_test.go  # Self-declared data tests
    ├── avatar_test.go         # Avatar management tests
    ├── phone_test.go          # Phone operation tests
    ├── whatsapp_test.go       # WhatsApp bot tests
    ├── beta_groups_test.go    # Beta groups admin tests
    └── admin_test.go          # Admin/quarantine tests
```

### Test Categories

**Tier 1: Smoke Tests (~2 minutes)**
- Health endpoint verification
- Authentication validation
- Critical read operations (citizen data)
- Service dependency checks

**Tier 2: E2E Tests (~5-10 minutes)**
- Complete CRUD operations for all resources
- Data integrity verification
- Error handling validation
- Integration between services

## Configuration

Tests are configured via environment variables:

```bash
# Required Configuration
export TEST_BASE_URL="https://services.staging.app.dados.rio/rmi/v1"
export TEST_KEYCLOAK_URL="https://keycloak.example.com"
export TEST_KEYCLOAK_REALM="your-realm"
export TEST_KEYCLOAK_CLIENT_ID="your-client-id"
export TEST_USERNAME="test-user@example.com"
export TEST_PASSWORD="test-password"
export TEST_CPF="12345678900"  # Valid test CPF

# Optional Configuration
export TEST_HEALTH_CHECK_TIMEOUT=30  # Default: 30 seconds
export TEST_API_CALL_TIMEOUT=10      # Default: 10 seconds
```

### Local Development

For local testing, you can use the local API server:

```bash
export TEST_BASE_URL="http://localhost:8080/v1"
# ... other config vars ...
```

## Running Tests

### Using Just (Recommended)

```bash
# Ensure environment variables are set (via .env or export)
just test-e2e
```

### Using Go Test Directly

```bash
# Run all E2E tests
go test -v ./tests/e2e/...

# Run specific test file
go test -v ./tests/e2e/health_test.go

# Run specific test
go test -v ./tests/e2e/... -run TestHealth_Endpoint

# Run with race detection
go test -race -v ./tests/e2e/...
```

### CI/CD Integration

The test suite is designed to run in CI/CD pipelines:

```yaml
- name: Run E2E Tests
  env:
    TEST_BASE_URL: ${{ secrets.TEST_BASE_URL }}
    TEST_KEYCLOAK_URL: ${{ secrets.TEST_KEYCLOAK_URL }}
    TEST_KEYCLOAK_REALM: ${{ secrets.TEST_KEYCLOAK_REALM }}
    TEST_KEYCLOAK_CLIENT_ID: ${{ secrets.TEST_KEYCLOAK_CLIENT_ID }}
    TEST_USERNAME: ${{ secrets.TEST_USERNAME }}
    TEST_PASSWORD: ${{ secrets.TEST_PASSWORD }}
    TEST_CPF: ${{ secrets.TEST_CPF }}
  run: go test -v ./tests/e2e/...
```

## Test Fixtures and Helpers

### APIClient

The `fixtures.APIClient` provides authenticated HTTP operations:

```go
// Create authenticated client
client := fixtures.NewAPIClient(config, token)

// GET request
resp, err := client.Get("/citizen/12345678900")

// POST request
resp, err := client.Post("/phone/opt-in", data)

// PUT request
resp, err := client.Put("/citizen/12345678900/email", data)

// DELETE request
resp, err := client.Delete("/citizen/12345678900/avatar")
```

### Helper Functions

```go
// Assert HTTP status code
fixtures.AssertStatusCode(t, resp, http.StatusOK)

// Assert JSON response and parse
body := fixtures.AssertJSONResponse(t, resp)

// Assert field exists
fixtures.AssertFieldExists(t, body, "cpf")

// Assert field value
fixtures.AssertFieldValue(t, body, "cpf", "12345678900")

// Wait for service to be healthy
err := fixtures.WaitForHealthy(t, client, maxAttempts)
```

## Writing New Tests

### Example Test

```go
package e2e

import (
    "fmt"
    "net/http"
    "github.com/prefeitura-rio/app-rmi/tests/fixtures"
)

func (s *E2ETestSuite) TestMyNewEndpoint() {
    // Arrange
    testData := map[string]interface{}{
        "field": "value",
    }

    // Act
    resp, err := s.Client.Post("/my-endpoint", testData)
    s.Require().NoError(err)
    defer resp.Body.Close()

    // Assert
    fixtures.AssertStatusCode(s.T(), resp, http.StatusOK)
    body := fixtures.AssertJSONResponse(s.T(), resp)
    fixtures.AssertFieldValue(s.T(), body, "success", true)
}
```

### Test Naming Convention

- Use descriptive test names: `Test<Feature>_<Action>_<Scenario>`
- Examples:
  - `TestCitizen_GetData` - Happy path
  - `TestCitizen_GetData_Unauthorized` - Auth failure
  - `TestCitizen_GetData_InvalidCPF` - Validation error

### Best Practices

1. **Isolation**: Each test should be independent and not rely on other tests
2. **Cleanup**: Clean up test data after tests complete
3. **Error Handling**: Always check and handle errors appropriately
4. **Assertions**: Use specific assertions rather than generic ones
5. **Documentation**: Add comments explaining complex test scenarios

## Troubleshooting

### Tests Failing to Connect

Check that `TEST_BASE_URL` is correct and the service is running:

```bash
curl $TEST_BASE_URL/health
```

### Authentication Errors

Verify Keycloak configuration and credentials:

```bash
echo "Keycloak URL: $TEST_KEYCLOAK_URL"
echo "Realm: $TEST_KEYCLOAK_REALM"
echo "Client ID: $TEST_KEYCLOAK_CLIENT_ID"
```

### Service Not Healthy

The test suite waits for the service to be healthy before running. If it times out:

1. Check service logs
2. Verify MongoDB and Redis are accessible
3. Increase `TEST_HEALTH_CHECK_TIMEOUT`

## Integration with Blue-Green Deployment

The test suite is designed to run against the preview service in blue-green deployments:

1. New version deploys to `rmi-preview` service
2. Tests run against `https://services.staging.app.dados.rio/rmi-preview/v1`
3. If tests pass → promote to stable
4. If tests fail → rollback and revert PR

See `.github/workflows/deploy-staging.yaml` for the full deployment workflow.
