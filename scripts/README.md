# API Testing Scripts

This directory contains scripts for testing the RMI API.

## test_api.sh

A comprehensive API testing script that tests all endpoints with proper authentication and handles phone verification flow.

### Prerequisites

- API running at `http://localhost:8080`
- Valid CPF for testing
- Valid Bearer token for authentication
- `curl` command available
- Optional: Real phone number for WhatsApp verification testing

### Usage

```bash
# Basic usage (tests all endpoints except phone verification)
./scripts/test_api.sh <CPF> <BEARER_TOKEN>

# Complete testing including interactive phone verification
./scripts/test_api.sh <CPF> <BEARER_TOKEN> <PHONE_NUMBER>
```

### Examples

```bash
# Test with a valid CPF and token
./scripts/test_api.sh 12345678901 "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# Complete testing with interactive phone verification
./scripts/test_api.sh 12345678901 "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." "+5511999887766"
```

### Phone Verification Workflow

The script is **interactive** for phone verification:

1. **Run the script** with your phone number:
   ```bash
   ./scripts/test_api.sh <CPF> <TOKEN> <PHONE>
   ```

2. **Script sends phone update request** and waits for your input

3. **Check your WhatsApp** for the verification code

4. **Enter the code** when prompted by the script

5. **Script continues** with phone verification test automatically

### What the Script Tests

#### Public Endpoints (No Auth Required)
- ✅ Health Check (`GET /health`)
- ✅ Get Ethnicity Options (`GET /citizen/ethnicity/options`)
- ✅ Validate Phone Number (`POST /validate/phone`)

#### Protected Endpoints (Auth Required)
- ✅ Get Citizen Data (`GET /citizen/{cpf}`)
- ✅ Get Citizen Wallet (`GET /citizen/{cpf}/wallet`)
- ✅ Get Maintenance Requests (`GET /citizen/{cpf}/maintenance-request`)
- ✅ Get First Login Status (`GET /citizen/{cpf}/firstlogin`)
- ✅ Get Opt-In Status (`GET /citizen/{cpf}/optin`)

#### Update Endpoints (Auth Required) + Verification
- ✅ Update Address (`PUT /citizen/{cpf}/address`) → 🔍 Verify Update
- ✅ Update Email (`PUT /citizen/{cpf}/email`) → 🔍 Verify Update
- ✅ Update Ethnicity (`PUT /citizen/{cpf}/ethnicity`) → 🔍 Verify Update
- ✅ Update First Login (`PUT /citizen/{cpf}/firstlogin`) → 🔍 Verify Update
- ✅ Update Opt-In (`PUT /citizen/{cpf}/optin`) → 🔍 Verify Update
- ✅ Update Phone (`PUT /citizen/{cpf}/phone`) → 🔍 Verify Update
- ✅ Validate Phone Verification (`POST /citizen/{cpf}/phone/validate`)

### Output

The script provides:
- 🟢 **Green checkmarks** for successful tests
- 🔴 **Red X marks** for failed tests
- 📊 **Summary** of passed/failed tests
- 🔍 **Detailed responses** for debugging

### Exit Codes

- `0`: All tests passed
- `1`: Some tests failed

### Data Verification

The script now **verifies that updates are actually applied**:

1. **Gets original data** before making updates
2. **Makes update calls** with unique test data
3. **Gets updated data** after making changes
4. **Compares values** to ensure changes were applied
5. **Reports verification results** with before/after values

**Test Data Used:**
- **Address**: `Rua Teste API, 999, Sala 42, Botafogo, RJ, 22250-040`
- **Email**: `teste.api@exemplo.com`
- **Ethnicity**: `Preta`
- **First Login**: `false`
- **Opt-In**: `true`
- **Phone**: Your provided phone number

### Prerequisites

- **jq** (optional but recommended for detailed verification)
  ```bash
  brew install jq  # macOS
  # or
  apt-get install jq  # Ubuntu/Debian
  ```

### Notes

- The script accepts 404 responses as valid (data not found is acceptable)
- All other 2xx responses are considered successful
- Failed tests show the actual response for debugging
- The script uses unique test data to ensure verification works
- Without `jq`, verification is skipped but tests still run 