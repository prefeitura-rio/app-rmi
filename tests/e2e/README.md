# E2E Tests

End-to-end tests for the RMI API, designed to run against deployed environments (staging/production preview).

## Test Coverage

### Health Tests (`health_test.go`)
- **TestHealth**: Verifies the `/health` endpoint is responding correctly
  - Checks HTTP 200 status
  - Validates response JSON structure
  - Confirms "healthy" status

### Citizen Data Tests (`citizen_test.go`)
- **TestCitizenDataWorkflow**: Complete citizen data CRUD workflow
  - Get citizen data by CPF (authenticated)
  - Update self-declared address
  - Update self-declared email with timestamp to avoid conflicts
  - Validates response structures and status codes

### Phone Verification Tests (`phone_test.go`)
- **TestPhoneOptInOptOut**: Phone verification and opt-in/opt-out workflow
  - Phone opt-in with CPF
  - Retrieve CPF from phone number
  - Update self-declared phone number
  - Phone opt-out
  - Verify opt-out status (404 after opt-out)

## Environment Variables

### Required for All Tests
- `TEST_BASE_URL`: Base URL of the API (e.g., `https://services.staging.app.dados.rio/rmi-preview/v1`)

### Required for Authenticated Tests
Option 1: Direct token
- `TEST_BEARER_TOKEN`: Pre-generated JWT token

Option 2: Keycloak authentication (recommended)
- `TEST_KEYCLOAK_URL`: Keycloak server URL
- `TEST_KEYCLOAK_REALM`: Keycloak realm name
- `TEST_KEYCLOAK_CLIENT_ID`: Client ID for authentication
- `TEST_USERNAME`: Test user username
- `TEST_PASSWORD`: Test user password
- `TEST_CPF`: CPF associated with the test user

## Running Tests

### Local Development
```bash
# Set environment variables
export TEST_BASE_URL="http://localhost:8080/v1"
export TEST_KEYCLOAK_URL="https://keycloak.example.com"
export TEST_KEYCLOAK_REALM="rmi"
export TEST_KEYCLOAK_CLIENT_ID="rmi-api"
export TEST_USERNAME="testuser"
export TEST_PASSWORD="testpass"
export TEST_CPF="12345678900"

# Run all E2E tests
go test -v ./tests/e2e/... -timeout 15m
```

### CI/CD (GitHub Actions)
Tests run automatically in the deploy-staging workflow:
- Triggered on push to main branch
- Runs against preview service after deployment
- Uses GitHub Secrets for credentials
- 15-minute timeout for complete test suite

## Test Design Principles

1. **No Mocking**: All tests hit real API endpoints
2. **Idempotent**: Tests handle 409 Conflict responses gracefully
3. **Unique Data**: Uses timestamps to generate unique phone numbers and emails
4. **Complete Workflows**: Tests cover entire user journeys, not just individual endpoints
5. **Proper Cleanup**: Opt-out tests clean up phone data
6. **Timeout Protection**: 30-second timeouts for individual requests, 15-minute total

## Adding New Tests

When adding new E2E tests:
1. Create a new `*_test.go` file in `tests/e2e/`
2. Use the `e2e_test` package
3. Reuse helper functions: `getBaseURL()`, `getAuthToken()`, `getTestCPF()`
4. Handle both success and conflict (409) status codes
5. Use unique identifiers (timestamps) to avoid conflicts
6. Add documentation to this README
