# Logged User Experience Tests

This test suite implements comprehensive testing for logged user experiences in the RMI application, covering all the user flows described in the requirements and integrating with the existing k6 operator and GitHub Actions infrastructure.

## Test Scenarios

### Mixed Scenario (Default)

- **Scenario**: `mixed`
- **Description**: Runs all scenarios with proportional load distribution
- **Load Distribution**:
  - First Login: 10% of VUs
  - Home Access: 40% of VUs
  - Personal Info: 20% of VUs
  - Wallet: 30% of VUs

### Individual Scenarios

#### First Login User

- **Scenario**: `first_login`
- **Flow**: Checks first login status → Updates first login flag → Loads home data
- **Timing**: 10s delay after first login check, 3s delay before home data loading

#### Home Access

- **Scenario**: `home_access`
- **Flow**: Concurrent loading of wallet and maintenance request data
- **Timing**: Immediate parallel API calls

#### Personal Information Management

- **Scenario**: `personal_info`
- **Flow**: Complete personal data management including:
  - Personal info fetch
  - Ethnicity update (2s delay)
  - Phone number update and validation (10s + 15s delays)
  - Email update (10s delay)
  - Address update (10s delay)
  - Optin preferences (10s + 2s delays)

#### Wallet Interactions

- **Scenario**: `wallet`
- **Flow**: Wallet overview + sequential card interactions
- **Cards**: Clínica da Família, Educação, Cadúnico, Chamados
- **Timing**: 2s initial delay, 5s between card interactions

#### Legacy Load Test

- **Scenario**: `legacy`
- **Flow**: Simple load test against BASE_URL (backward compatibility)

## Environment Variables

### Required (GitHub k6 Environment)

Authentication variables are required, test configuration variables have fallback defaults for safety:

```bash
# Authentication (Required)
KEYCLOAK_ISSUER=https://your-keycloak-server
KEYCLOAK_CLIENT_ID=your-client-id
KEYCLOAK_CLIENT_SECRET=your-client-secret

# Test Configuration (Optional - has fallback defaults)
BASE_URL=http://rmi.rmi.svc.cluster.local  # Default if not set
TEST_CPF=12345678901  # Default if not set
TEST_SCENARIO=mixed  # Default if not set
DURATION=30s  # Default if not set
VIRTUAL_USERS=10  # Default if not set
```

## GitHub Actions Integration

The test suite integrates seamlessly with the existing GitHub Actions workflow. The following variables can be configured in your repository:

### Repository Variables (k6 Environment)

All variables are configured in the GitHub k6 environment:

- `K6_NAMESPACE` - Kubernetes namespace
- `KEYCLOAK_CLIENT_ID` - Keycloak client ID
- `KEYCLOAK_ISSUER` - Keycloak issuer URL
- `BASE_URL` - Base API URL (e.g., `http://rmi.rmi.svc.cluster.local`)
- `TEST_CPF` - Test CPF (e.g., `12345678901`)
- `TEST_SCENARIO` - Test scenario (`mixed|first_login|home_access|personal_info|wallet|legacy`)
- `DURATION` - Test duration (e.g., `30s`, `2m`)
- `VIRTUAL_USERS` - Number of VUs (e.g., `10`)

### Repository Secrets (k6 Environment)

- `KEYCLOAK_CLIENT_SECRET` - Keycloak client secret

## Running Tests

### Via GitHub Actions (Recommended)

1. Go to Actions → Load tests
2. Click "Run workflow"
3. Tests will run automatically in the k6 operator

### Local Development

```bash
# Set environment variables
export KEYCLOAK_ISSUER=https://auth-idriohom.apps.rio.gov.br/auth/realms/idrio_cidadao
export KEYCLOAK_CLIENT_ID=superapp.apps.rio.gov.br
export KEYCLOAK_CLIENT_SECRET=your-secret
export TARGET_URL=https://rmi.services.app.dados.rio

# Run specific scenario
TEST_SCENARIO=home_access k6 run k6/load_test.js

# Run with custom parameters
TEST_SCENARIO=mixed VIRTUAL_USERS=20 DURATION=60s k6 run k6/load_test.js
```

## API Endpoints Tested

### User Management

- `GET /citizen/{cpf}/firstlogin` - Check first login status
- `PUT /citizen/{cpf}/firstlogin` - Update first login flag
- `GET /citizen/{cpf}` - Get personal information
- `PUT /citizen/{cpf}/ethnicity` - Update ethnicity
- `POST /citizen/{cpf}/phone` - Update phone number
- `PUT /citizen/{cpf}/phone/validate` - Validate phone token
- `PUT /citizen/{cpf}/email` - Update email
- `PUT /citizen/{cpf}/address` - Update address
- `GET /citizen/{cpf}/optin` - Get optin preferences
- `PUT /citizen/{cpf}/optin` - Update optin preferences

### Application Data

- `GET /citizen/{cpf}/wallet` - Get wallet information
- `GET /citizen/{cpf}/maintenance-request` - Get maintenance requests

## Performance Thresholds

- 95th percentile response time < 5000ms
- Request failure rate < 5%
- Custom failure rate tracking < 5%

## Features

### Realistic User Behavior

- Variable timing between user actions (2s to 15s delays)
- Sequential and parallel API calls as per actual user flows
- Proper authentication token management and reuse

### Comprehensive Validation

- HTTP status code validation (200 expected)
- Response time validation (< 5000ms)
- Authentication failure detection
- Error rate tracking with custom metrics

### Flexible Configuration

- Multiple test scenarios (mixed, individual, legacy)
- Configurable load distribution
- Environment-specific configuration
- Backward compatibility with existing infrastructure

### Monitoring and Observability

- Custom metrics for failed requests
- Scenario-specific tagging for performance analysis
- Detailed error logging with status codes
- Setup and teardown hooks for proper resource management

## Architecture Integration

This test suite maintains full compatibility with the existing k6 operator infrastructure:

- **ConfigMap Creation**: Uses existing `create-configmap.sh` script
- **TestRun Management**: Extends existing TestRun template with new environment variables
- **Monitoring**: Compatible with existing `monitor-test.sh` and `generate-summary.sh` scripts
- **Cleanup**: Uses existing `cleanup.sh` script

The implementation provides both new logged user experience testing capabilities while preserving backward compatibility with the original load testing functionality.

## Configuration Notes

Authentication variables must be configured in the GitHub k6 environment. Test configuration variables have fallback defaults to prevent script errors.

### Setting Variables in GitHub:

1. Go to your repository → **Settings** → **Environments**
2. Select or create the **k6** environment
3. Add **Environment variables** for configuration variables (optional due to defaults)
4. Add **Environment secrets** for sensitive data like `KEYCLOAK_CLIENT_SECRET`

**Required variables**: `KEYCLOAK_ISSUER`, `KEYCLOAK_CLIENT_ID`, `KEYCLOAK_CLIENT_SECRET`
**Optional variables**: All test configuration variables have safe defaults