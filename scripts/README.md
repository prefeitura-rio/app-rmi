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

The script now supports **non-interactive phone verification**:

1. **Run the script** with your phone number:
   ```bash
   ./scripts/test_api.sh <CPF> <TOKEN> <PHONE>
   ```

2. **Script automatically generates a random phone number** if none provided

3. **Script sends phone update request** and waits for verification code to be stored

4. **Script queries MongoDB directly** to retrieve the verification code

5. **Script automatically completes verification** without user input

**Alternative: Skip Phone Verification**
```bash
# Skip phone verification entirely
./scripts/test_api.sh <CPF> <TOKEN> --skip-phone
```

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

#### WhatsApp Bot Endpoints (No Auth Required)
- ✅ Get Available Channels (`GET /config/channels`)
- ✅ Get Opt-Out Reasons (`GET /config/opt-out-reasons`)
- ✅ Get Citizen by Phone (`GET /phone/{phone}/citizen`)
- ✅ Validate Registration (`POST /phone/{phone}/validate-registration`)

#### Phone-CPF Mapping Endpoints (Auth Required)
- ✅ Setup Phone-CPF Mapping (`POST /phone/{phone}/opt-in`)
- ✅ Opt-out (Non-blocking) (`POST /phone/{phone}/opt-out`)
- ✅ Opt-out (Blocking) (`POST /phone/{phone}/opt-out`)
- ✅ Opt-in Same CPF (`POST /phone/{phone}/opt-in`)
- ✅ Opt-in Different CPF (`POST /phone/{phone}/opt-in`)

#### Quarantine Endpoints (Admin Required)
- ✅ Get Phone Status (`GET /phone/{phone}/status`)
- ✅ Bind Phone to CPF (`POST /phone/{phone}/bind`)
- ✅ Quarantine Phone (`POST /phone/{phone}/quarantine`)
- ✅ Release Quarantine (`DELETE /phone/{phone}/quarantine`)
- ✅ Get Quarantined Phones (`GET /admin/phone/quarantined`)
- ✅ Get Quarantine Stats (`GET /admin/phone/quarantine/stats`)

#### Beta Whitelist Endpoints (Admin Required)
- ✅ Get Beta Status (`GET /phone/{phone}/beta-status`)
- ✅ Create Beta Group (`POST /admin/beta/groups`)
- ✅ List Beta Groups (`GET /admin/beta/groups`)
- ✅ Get Beta Group by ID (`GET /admin/beta/groups/{id}`)
- ✅ Add Phone to Whitelist (`POST /admin/beta/whitelist/{phone}`)
- ✅ List Whitelisted Phones (`GET /admin/beta/whitelist`)
- ✅ Bulk Add Phones (`POST /admin/beta/whitelist/bulk-add`)
- ✅ Bulk Move Phones (`POST /admin/beta/whitelist/bulk-move`)
- ✅ Update Beta Group (`PUT /admin/beta/groups/{id}`)
- ✅ Bulk Remove Phones (`POST /admin/beta/whitelist/bulk-remove`)
- ✅ Remove Phone from Whitelist (`DELETE /admin/beta/whitelist/{phone}`)
- ✅ Delete Beta Groups (`DELETE /admin/beta/groups/{id}`)

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
- **Address**: Random address with unique numbers (e.g., `Rua Teste API 123, Sala 42, Botafogo 123, RJ, 12345-678`)
- **Email**: Random email with unique numbers (e.g., `teste.api.1234@exemplo.com`)
- **Ethnicity**: Dynamically selected based on current value (ensures change)
- **First Login**: `false` (API always sets to false)
- **Opt-In**: Toggled based on current value (true ↔ false)
- **Phone**: Random phone number generated automatically (e.g., `+5511999887766`)

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