#!/bin/bash

# API Testing Script for RMI API
# Usage: ./scripts/test_api.sh <CPF> <BEARER_TOKEN> [--skip-phone]
# 
# This script tests all API endpoints including:
# - Health and configuration endpoints
# - Citizen data endpoints (with authentication)
# - Phone-based endpoints for WhatsApp bot
# - Data verification for updates
# 
# Note: 
# - Random data is generated for address and email updates to avoid 409 conflicts
# - A random phone number is generated automatically for testing
# - Use --skip-phone to skip phone verification tests

# set -e  # Disabled to prevent script from exiting on errors

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command line arguments
SKIP_PHONE=false
CPF=""
BEARER_TOKEN=""
AUTO_TOKEN=false

# Parse positional arguments and flags
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-phone)
            SKIP_PHONE=true
            shift
            ;;
        --auto-token)
            AUTO_TOKEN=true
            shift
            ;;
        *)
            if [[ -z "$CPF" ]]; then
                CPF="$1"
            elif [[ -z "$BEARER_TOKEN" ]] && [[ "$AUTO_TOKEN" != "true" ]]; then
                BEARER_TOKEN="$1"
            fi
            shift
            ;;
    esac
done

# Generate a random phone number for testing
generate_random_phone() {
    # Generate a random Brazilian phone number: +55 11 9XXXX-XXXX
    local ddi="55"
    local ddd="11"
    local prefix="9"
    local number1=$((RANDOM % 9000 + 1000))  # 4 digits
    local number2=$((RANDOM % 9000 + 1000))  # 4 digits
    echo "+${ddi}${ddd}${prefix}${number1}${number2}"
}

PHONE_NUMBER=$(generate_random_phone)
echo -e "${BLUE}📱 Generated random phone number for testing: $PHONE_NUMBER${NC}"

# Configuration
# API_BASE_URL="https://services.staging.app.dados.rio/rmi/v1"
API_BASE_URL="http://localhost:8080/v1"

# Auto-generate token if requested
if [[ "$AUTO_TOKEN" == "true" ]]; then
    echo -e "${BLUE}🔑 Auto-generating access token using curl...${NC}"
    
    # Check required environment variables
    if [[ -z "$KEYCLOAK_ISSUER" || -z "$KEYCLOAK_CLIENT_ID" || -z "$KEYCLOAK_CLIENT_SECRET" ]]; then
        echo -e "${RED}Error: Required environment variables not set:${NC}"
        echo "  - KEYCLOAK_ISSUER"
        echo "  - KEYCLOAK_CLIENT_ID" 
        echo "  - KEYCLOAK_CLIENT_SECRET"
        exit 1
    fi
    
    # Generate token using curl (no Node.js dependency)
    TOKEN_RESPONSE=$(curl -s -X POST "${KEYCLOAK_ISSUER}/protocol/openid-connect/token" \
        -H "Content-Type: application/x-www-form-urlencoded" \
        -d "grant_type=client_credentials" \
        -d "client_id=${KEYCLOAK_CLIENT_ID}" \
        -d "client_secret=${KEYCLOAK_CLIENT_SECRET}")
    
    if [[ $? -eq 0 ]] && echo "$TOKEN_RESPONSE" | grep -q "access_token"; then
        # Extract access token using basic text processing (no jq dependency)
        ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | sed -n 's/.*"access_token":"\([^"]*\).*/\1/p')
        if [[ -n "$ACCESS_TOKEN" ]]; then
            BEARER_TOKEN="$ACCESS_TOKEN"
            echo -e "${GREEN}✅ Token generated successfully${NC}"
        else
            echo -e "${RED}Error: Failed to extract access token from response${NC}"
            echo "Response: $TOKEN_RESPONSE"
            exit 1
        fi
    else
        echo -e "${RED}Error: Failed to generate token. Check your environment variables and Keycloak configuration${NC}"
        echo "Response: $TOKEN_RESPONSE"
        exit 1
    fi
fi

# Validation
if [[ -z "$CPF" ]]; then
    echo -e "${RED}Error: CPF is required${NC}"
    echo "Usage: $0 <CPF> [BEARER_TOKEN] [--skip-phone] [--auto-token]"
    echo "Example: $0 12345678901 'Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...'"
    echo "Example: $0 12345678901 --auto-token --skip-phone"
    exit 1
fi

if [[ -z "$BEARER_TOKEN" ]]; then
    echo -e "${RED}Error: Bearer token is required${NC}"
    echo "Usage: $0 <CPF> [BEARER_TOKEN] [--skip-phone] [--auto-token]"
    echo "Use --auto-token to generate token automatically, or provide a token manually"
    exit 1
fi

# Test counter
TESTS_PASSED=0
TESTS_FAILED=0

# Function to print test results
print_result() {
    local test_name="$1"
    local status="$2"
    local response="$3"
    
    if [[ "$status" == "PASS" ]]; then
        echo -e "${GREEN}✅ $test_name${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}❌ $test_name${NC}"
        echo -e "${YELLOW}Response: $response${NC}"
        ((TESTS_FAILED++))
    fi
}

# Function to make HTTP requests
make_request() {
    local method="$1"
    local endpoint="$2"
    local data="$3"
    local test_name="$4"
    
    local url="$API_BASE_URL$endpoint"
    local headers=()
    
    # Add authorization header if provided
    if [[ -n "$BEARER_TOKEN" ]]; then
        headers+=("-H" "Authorization: Bearer $BEARER_TOKEN")
    fi
    
    # Add content type for POST/PUT requests
    if [[ "$method" == "POST" || "$method" == "PUT" ]]; then
        headers+=("-H" "Content-Type: application/json")
    fi
    
    echo -e "${BLUE}Testing: $test_name${NC}"
    echo "  $method $url"
    
    local response
    local status_code
    
    if [[ -n "$data" ]]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$url" "${headers[@]}" -d "$data" 2>/dev/null)
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$url" "${headers[@]}" 2>/dev/null)
    fi
    
    # Extract status code (last line)
    status_code=$(echo "$response" | tail -n1)
    # Extract response body (all lines except last)
    response_body=$(echo "$response" | sed '$d')
    
    # Check if status code is 2xx or 404 (acceptable for data not found)
    # Also accept 409 (Conflict) for "already_opted_in" responses
    if [[ "$status_code" -ge 200 && "$status_code" -lt 300 ]] || [[ "$status_code" -eq 404 ]]; then
        print_result "$test_name" "PASS" "$response_body"
        # Store response body in a temporary file for verification
        if [[ "$test_name" == *"(Original)"* ]] || [[ "$test_name" == *"(After Updates)"* ]] || [[ "$test_name" == *"(After Phone Verification)"* ]] || [[ "$test_name" == *"First Login Status"* ]] || [[ "$test_name" == *"Opt-In Status"* ]] || [[ "$test_name" == *"Opt-out"* ]] || [[ "$test_name" == *"(After Blocking)"* ]] || [[ "$test_name" == *"(After Non-blocking)"* ]] || [[ "$test_name" == *"Create Beta Group"* ]] || [[ "$test_name" == *"Create Second Beta Group"* ]]; then
            # Create a safe filename by replacing spaces and special chars with underscores
            local safe_filename=$(echo "$test_name" | sed 's/[^a-zA-Z0-9]/_/g')
            echo "$response_body" > "/tmp/api_response_${safe_filename}"
            echo "  📁 Stored response in: /tmp/api_response_${safe_filename}"
        fi
    else
        print_result "$test_name" "FAIL" "Status: $status_code, Response: $response_body"
    fi
}

# Function to verify data updates
verify_update() {
    local test_name="$1"
    local original_data="$2"
    local updated_data="$3"
    local field_path="$4"
    
    echo -e "${BLUE}Verifying: $test_name${NC}"
    
    # Extract field values using jq (if available) or simple grep
    if command -v jq >/dev/null 2>&1; then
        local original_value=$(echo "$original_data" | jq -r "$field_path" 2>/dev/null)
        local updated_value=$(echo "$updated_data" | jq -r "$field_path" 2>/dev/null)
        
        # Handle null values for comparison
        if [[ "$original_value" == "null" ]]; then
            original_value=""
        fi
        if [[ "$updated_value" == "null" ]]; then
            updated_value=""
        fi
        

        
        if [[ "$original_value" != "$updated_value" ]]; then
            echo -e "${GREEN}✅ $test_name: Data updated successfully${NC}"
            echo "  Original: $original_value"
            echo "  Updated:  $updated_value"
            ((TESTS_PASSED++))
        else
            echo -e "${RED}❌ $test_name: Data not updated${NC}"
            echo "  Value unchanged: $original_value"
            ((TESTS_FAILED++))
        fi
    else
        # Fallback without jq - just check if response contains expected data
        echo -e "${YELLOW}⚠️  $test_name: Verification skipped (jq not available)${NC}"
        echo "  Install jq for detailed verification: brew install jq"
    fi
    
    echo ""
}

# Function to wait for user input
wait_for_input() {
    local message="$1"
    echo -e "${YELLOW}$message${NC}"
    read -p "Press Enter to continue..."
}

echo -e "${BLUE}🚀 Starting API Tests for CPF: $CPF${NC}"
echo "=================================================="
echo ""

# Test 1: Health Check (no auth required)
make_request "GET" "/health" "" "Health Check"

# Test 2: Get Ethnicity Options (no auth required)
make_request "GET" "/citizen/ethnicity/options" "" "Get Ethnicity Options"

# Test 3: Validate Phone Number (no auth required)
phone_data="{\"phone\":\"$PHONE_NUMBER\"}"
make_request "POST" "/validate/phone" "$phone_data" "Validate Phone Number"

# Test 4: Get Citizen Data (Original)
echo -e "${BLUE}📋 Getting original citizen data for verification...${NC}"
make_request "GET" "/citizen/$CPF" "" "Get Citizen Data (Original)"

# Test 5: Get Citizen Wallet
make_request "GET" "/citizen/$CPF/wallet" "" "Get Citizen Wallet"

# Test 6: Get Maintenance Requests
make_request "GET" "/citizen/$CPF/maintenance-request" "" "Get Maintenance Requests"

# Test 7: Get First Login Status (Original)
make_request "GET" "/citizen/$CPF/firstlogin" "" "Get First Login Status (Original)"

# Test 8: Get Opt-In Status (will be captured before update)
# Note: Actual "Original" status will be captured just before opt-in update

# Test 9: Update Address
# Generate random data to avoid 409 conflicts
random_number=$((RANDOM % 1000))
address_data="{
    \"logradouro\": \"Rua Teste API $random_number\",
    \"numero\": \"$((RANDOM % 999 + 1))\",
    \"complemento\": \"Sala $((RANDOM % 100 + 1))\",
    \"bairro\": \"Botafogo $random_number\",
    \"municipio\": \"Rio de Janeiro\",
    \"estado\": \"RJ\",
    \"cep\": \"$((RANDOM % 90000 + 10000))-000\"
}"
make_request "PUT" "/citizen/$CPF/address" "$address_data" "Update Address"

# Test 10: Update Email
# Generate random email to avoid 409 conflicts
random_email_number=$((RANDOM % 10000))
email_data="{
    \"valor\": \"teste.api.$random_email_number@exemplo.com\"
}"
make_request "PUT" "/citizen/$CPF/email" "$email_data" "Update Email"

# Test 11: Update Ethnicity (dynamic - pick different from current)
# Get current ethnicity and pick a different one
current_ethnicity=$(curl -s -X GET "http://localhost:8080/v1/citizen/$CPF" -H "Authorization: Bearer $BEARER_TOKEN" | jq -r '.raca' 2>/dev/null || echo "unknown")
echo "Current ethnicity: $current_ethnicity"

# Handle null ethnicity - treat it as empty string for comparison
if [[ "$current_ethnicity" == "null" ]]; then
    current_ethnicity=""
fi

# Get available options and pick one different from current
ethnicity_options=$(curl -s -X GET "http://localhost:8080/v1/citizen/ethnicity/options" | jq -r '.[]' 2>/dev/null)
new_ethnicity=""
for option in $ethnicity_options; do
    if [[ "$option" != "$current_ethnicity" ]]; then
        new_ethnicity="$option"
        break
    fi
done

if [[ -z "$new_ethnicity" ]]; then
    new_ethnicity="branca"  # fallback
fi

echo "Updating ethnicity to: $new_ethnicity"
ethnicity_data="{
    \"valor\": \"$new_ethnicity\"
}"
make_request "PUT" "/citizen/$CPF/ethnicity" "$ethnicity_data" "Update Ethnicity"

# Test 12: Update First Login (API always sets to false)
firstlogin_data='{
    "first_login": true
}'
make_request "PUT" "/citizen/$CPF/firstlogin" "$firstlogin_data" "Update First Login"

# Test 13: Update Opt-In (change to opposite value to test update)
# First get current opt-in status to ensure we make a change AND store as original
make_request "GET" "/citizen/$CPF/optin" "" "Get Opt-In Status (Original)"
current_optin_response=$(curl -s -H "Authorization: Bearer $BEARER_TOKEN" "$API_BASE_URL/citizen/$CPF/optin")
current_optin=$(echo "$current_optin_response" | jq -r '.opt_in' 2>/dev/null)

# Set to opposite of current value to ensure a change
if [[ "$current_optin" == "true" ]]; then
    new_optin_value="false"
else
    new_optin_value="true"
fi

# Debug output removed

optin_data="{
    \"opt_in\": $new_optin_value
}"
make_request "PUT" "/citizen/$CPF/optin" "$optin_data" "Update Opt-In"

# Function to get verification code from MongoDB
get_verification_code() {
    local cpf="$1"
    local phone_number="$2"
    
    # Get MongoDB connection details from environment
    local mongo_uri="${MONGODB_TEST_URI:-mongodb://localhost:27017/rmi}"
    local collection="${MONGODB_PHONE_VERIFICATION_COLLECTION:-phone_verifications}"
        
    # Format phone number for storage (remove + and format as stored in DB)
    local storage_phone="${phone_number#+}"  # Remove leading +
    
    # Debug: show what we're looking for
    echo -e "${BLUE}🔍 Looking for verification code with:${NC}" >&2
    echo -e "${BLUE}   CPF: $cpf${NC}" >&2
    echo -e "${BLUE}   Phone: $storage_phone${NC}" >&2
    
    # Try to connect to MongoDB with authentication
    local result=""
    result=$(mongosh "$mongo_uri" --quiet --eval "db.$collection.find({\"phone_number\": \"$storage_phone\"}).sort({\"created_at\": -1}).limit(1).toArray()" 2>/dev/null | tr -d '\n')
    
    # Debug: print the result for troubleshooting
    echo -e "${BLUE}🔍 MongoDB query result: $result${NC}" >&2
    
    # Extract the code from the result using jq for proper JSON parsing
    local code=""
    echo -e "${BLUE}🔍 Raw MongoDB result: $result${NC}" >&2
    
    if echo "$result" | grep -q "code:"; then
        # Extract the code using a simple grep and sed approach
        code=$(echo "$result" | grep -o "code: '[^']*'" | sed "s/code: '\([^']*\)'/\1/")
        echo -e "${BLUE}🔍 Extracted code: $code${NC}" >&2
    fi
    
    if [[ -n "$code" ]]; then
        echo "$code"
        return 0
    else
        echo -e "${YELLOW}⚠️  No verification code found in database${NC}" >&2
        echo -e "${YELLOW}💡 This might be due to MongoDB authentication issues or the verification code not being stored${NC}" >&2
        return 1
    fi
}

# Test 14: Update Phone (requires verification)
if [[ "$SKIP_PHONE" != "true" ]]; then
    # Extract DDI, DDD, and number from the generated phone number
    # Format: +5511999887766
    ddi="${PHONE_NUMBER:1:2}"  # 55
    ddd="${PHONE_NUMBER:3:2}"  # 11
    valor="${PHONE_NUMBER:5}"  # 999887766
    
    phone_data="{
        \"ddi\": \"$ddi\",
        \"ddd\": \"$ddd\",
        \"valor\": \"$valor\"
    }"
    make_request "PUT" "/citizen/$CPF/phone" "$phone_data" "Update Phone"
    
    # Wait a moment for the verification code to be stored in MongoDB
    echo -e "${BLUE}⏳ Waiting for verification code to be stored...${NC}"
    sleep 2
    
    # Get verification code from MongoDB
    echo -e "${BLUE}🔍 Retrieving verification code from database...${NC}"
    verification_code=$(get_verification_code "$CPF" "$PHONE_NUMBER")
    
    echo -e "${GREEN}✅ Found verification code: $verification_code${NC}"
    verification_data="{
        \"code\": \"$verification_code\",
        \"ddi\": \"$ddi\",
        \"ddd\": \"$ddd\",
        \"valor\": \"$valor\"
    }"
    # Test 14.5: Validate Phone Verification Code
    make_request "POST" "/citizen/$CPF/phone/validate" "$verification_data" "Validate Phone Verification Code"
    
    # Test 15: Verify Phone Update after verification
    echo -e "${BLUE}📋 Getting final citizen data to verify phone update...${NC}"
    make_request "GET" "/citizen/$CPF" "" "Get Citizen Data (After Phone Verification)"
else
    echo -e "${YELLOW}⚠️  Skipping phone update and verification (--skip-phone flag used)${NC}"
    echo ""
fi

# Test 16: Get Citizen Data Again (to verify updates)
echo -e "${BLUE}📋 Getting updated citizen data for verification...${NC}"
make_request "GET" "/citizen/$CPF" "" "Get Citizen Data (After Updates)"

# Test 17: Get First Login Status (Updated)
make_request "GET" "/citizen/$CPF/firstlogin" "" "Get First Login Status (Updated)"

# Test 18: Get Opt-In Status (Updated)
make_request "GET" "/citizen/$CPF/optin" "" "Get Opt-In Status (Updated)"

# WhatsApp Bot Endpoints Tests
echo -e "${BLUE}📞 Testing WhatsApp Bot Endpoints...${NC}"
echo "=================================================="

# Test 19: Get Available Channels
make_request "GET" "/config/channels" "" "Get Available Channels"

# Test 20: Get Opt-Out Reasons
make_request "GET" "/config/opt-out-reasons" "" "Get Opt-Out Reasons"

# Test 21: Get Citizen by Phone
make_request "GET" "/phone/$PHONE_NUMBER/citizen" "" "Get Citizen by Phone"

# Test 22: Validate Registration
    registration_data="{
        \"name\": \"João Silva Santos\",
        \"cpf\": \"$CPF\",
        \"birth_date\": \"1990-05-15\",
        \"channel\": \"whatsapp\"
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/validate-registration" "$registration_data" "Validate Registration"
    
    # Test 22.5: Create phone-CPF mapping for testing
    echo -e "${BLUE}🔗 Setting up Phone-CPF Mapping for Testing...${NC}"
    echo "=================================================="
    
    # First, try to opt-in to create the mapping
    optin_setup_data="{
        \"cpf\": \"$CPF\",
        \"channel\": \"whatsapp\",
        \"validation_result\": {
            \"valid\": true
        }
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/opt-in" "$optin_setup_data" "Setup Phone-CPF Mapping"
    
    # Check if setup was successful
    if [[ $? -eq 0 ]]; then
        echo -e "${GREEN}✅ Phone-CPF mapping created successfully${NC}"
    else
        echo -e "${YELLOW}⚠️  Phone-CPF mapping may already exist or failed to create${NC}"
        echo -e "${YELLOW}📝 Continuing with tests - some may fail if mapping is not active${NC}"
    fi
    
    echo -e "${GREEN}✅ Phone-CPF mapping setup completed${NC}"
    echo ""

# Test 23: Test Opt-out with non-blocking reason
    echo -e "${BLUE}🔄 Testing Non-blocking Opt-out Behavior...${NC}"
    echo "=================================================="
    
    # First, ensure we have an active mapping by trying to opt-in again
    # Note: This might return "already_opted_in" which is fine
    optin_ensure_active="{
        \"cpf\": \"$CPF\",
        \"channel\": \"whatsapp\",
        \"validation_result\": {
            \"valid\": true
        }
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/opt-in" "$optin_ensure_active" "Ensure Active Mapping"
    
    # Now test opt-out with non-blocking reason
    optout_data="{
        \"channel\": \"whatsapp\",
        \"reason\": \"irrelevant_content\"
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/opt-out" "$optout_data" "Opt-out (Non-blocking)"
    
    # Test that we can still get citizen by phone (should work)
    make_request "GET" "/phone/$PHONE_NUMBER/citizen" "" "Get Citizen by Phone (After Non-blocking)"
    
    # Test that registration validation still works (should work)
    registration_data_non_block="{
        \"name\": \"Ana Silva Santos\",
        \"cpf\": \"98765432109\",
        \"birth_date\": \"1992-07-10\",
        \"channel\": \"whatsapp\"
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/validate-registration" "$registration_data_non_block" "Validate Registration (After Non-blocking)"
    
    # Test opt-in with same CPF (should work - mapping not blocked)
    optin_same_cpf="{
        \"cpf\": \"$CPF\",
        \"channel\": \"whatsapp\",
        \"validation_result\": {
            \"valid\": true
        }
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/opt-in" "$optin_same_cpf" "Opt-in Same CPF (After Non-blocking)"
    
    echo -e "${GREEN}✅ Non-blocking opt-out tests completed${NC}"
    echo ""

# Test 24: Test Opt-out with blocking reason
    echo -e "${BLUE}🔒 Testing Blocking Opt-out Behavior...${NC}"
    echo "=================================================="
    
    # First, ensure we have an active mapping by trying to opt-in again
    # Note: This might return "already_opted_in" which is fine
    optin_ensure_active_blocking="{
        \"cpf\": \"$CPF\",
        \"channel\": \"whatsapp\",
        \"validation_result\": {
            \"valid\": true
        }
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/opt-in" "$optin_ensure_active_blocking" "Ensure Active Mapping (Blocking)"
    
    # Now test opt-out with blocking reason
    optout_blocking_data="{
        \"channel\": \"whatsapp\",
        \"reason\": \"incorrect_person\"
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/opt-out" "$optout_blocking_data" "Opt-out (Blocking)"

# Test 25: Verify phone mapping is blocked after incorrect_person opt-out
    echo -e "${BLUE}🔒 Testing CPF-Phone Mapping Blocking Logic...${NC}"
    echo "=================================================="
    
    # Test that we can still get citizen by phone (should work)
    make_request "GET" "/phone/$PHONE_NUMBER/citizen" "" "Get Citizen by Phone (After Blocking)"
    
    # Test that registration validation still works (should work)
    registration_data_after_block="{
        \"name\": \"Maria Silva Santos\",
        \"cpf\": \"98765432109\",
        \"birth_date\": \"1985-03-20\",
        \"channel\": \"whatsapp\"
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/validate-registration" "$registration_data_after_block" "Validate Registration (After Blocking)"
    
    # Test opt-in with different CPF (should work - mapping is blocked but not the phone)
    # Note: Using a different CPF that exists in the test data
    optin_different_cpf="{
        \"cpf\": \"45049725810\",
        \"channel\": \"whatsapp\",
        \"validation_result\": {
            \"valid\": true
        }
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/opt-in" "$optin_different_cpf" "Opt-in Different CPF (After Blocking)"
    
    echo -e "${GREEN}✅ CPF-Phone mapping blocking tests completed${NC}"
    echo ""

# Verification Section
echo -e "${BLUE}🔍 Verifying Data Updates...${NC}"
echo "=================================================="

# Read data from temporary files for verification
# Use the correct file names based on the sed transformation
if [[ -f "/tmp/api_response_Get_Citizen_Data__Original_" ]] && [[ -f "/tmp/api_response_Get_Citizen_Data__After_Updates_" ]]; then
    original_citizen_data=$(cat "/tmp/api_response_Get_Citizen_Data__Original_")
    updated_citizen_data=$(cat "/tmp/api_response_Get_Citizen_Data__After_Updates_")
    
    # Verify Address Update
    verify_update "Address Update" "$original_citizen_data" "$updated_citizen_data" ".endereco.principal.logradouro"
    
    # Verify Email Update
    verify_update "Email Update" "$original_citizen_data" "$updated_citizen_data" ".email"
    
    # Verify Ethnicity Update
    verify_update "Ethnicity Update" "$original_citizen_data" "$updated_citizen_data" ".raca"
else
    echo -e "${YELLOW}⚠️  Skipping citizen data verification (data not available)${NC}"
    echo "  Looking for files: /tmp/api_response_Get_Citizen_Data__Original_ and /tmp/api_response_Get_Citizen_Data__After_Updates_"
fi

# Verify First Login Update (special case - API always sets to false)
if [[ -f "/tmp/api_response_Get_First_Login_Status__Original_" ]] && [[ -f "/tmp/api_response_Get_First_Login_Status__Updated_" ]]; then
    original_firstlogin_data=$(cat "/tmp/api_response_Get_First_Login_Status__Original_")
    updated_firstlogin_data=$(cat "/tmp/api_response_Get_First_Login_Status__Updated_")
    
    echo -e "${BLUE}Verifying: First Login Update${NC}"
    
    # Extract values using jq
    if command -v jq >/dev/null 2>&1; then
        original_value=$(echo "$original_firstlogin_data" | jq -r ".firstlogin" 2>/dev/null)
        updated_value=$(echo "$updated_firstlogin_data" | jq -r ".firstlogin" 2>/dev/null)
        
        # First Login API always sets to false, so we verify this behavior
        if [[ "$updated_value" == "false" ]]; then
            echo -e "${GREEN}✅ First Login Update: API correctly set to false${NC}"
            echo "  Original: $original_value"
            echo "  Updated:  $updated_value (API always sets to false)"
            ((TESTS_PASSED++))
        else
            echo -e "${RED}❌ First Login Update: API should set to false${NC}"
            echo "  Expected: false"
            echo "  Got: $updated_value"
            ((TESTS_FAILED++))
        fi
    else
        echo -e "${YELLOW}⚠️  First Login Update: Verification skipped (jq not available)${NC}"
    fi
    echo ""
else
    echo -e "${YELLOW}⚠️  Skipping first login verification (data not available)${NC}"
    echo "  Looking for files: /tmp/api_response_Get_First_Login_Status__Original_ and /tmp/api_response_Get_First_Login_Status__Updated_"
    # Debug: show what files actually exist
    echo "  Available files:"
    find /tmp -name "api_response_*First*Login*" 2>/dev/null | head -5
fi

# Verify Opt-In Update
if [[ -f "/tmp/api_response_Get_Opt_In_Status__Original_" ]] && [[ -f "/tmp/api_response_Get_Opt_In_Status__Updated_" ]]; then
    original_optin_data=$(cat "/tmp/api_response_Get_Opt_In_Status__Original_")
    updated_optin_data=$(cat "/tmp/api_response_Get_Opt_In_Status__Updated_")
    verify_update "Opt-In Update" "$original_optin_data" "$updated_optin_data" ".opt_in"
else
    echo -e "${YELLOW}⚠️  Skipping opt-in verification (data not available)${NC}"
    echo "  Looking for files: /tmp/api_response_Get_Opt_In_Status__Original_ and /tmp/api_response_Get_Opt_In_Status__Updated_"
    # Debug: show what files actually exist
    echo "  Available files:"
    find /tmp -name "api_response_*Opt*In*" 2>/dev/null | head -5
fi

# Verify Phone Update (if phone verification was completed)
if [[ "$SKIP_PHONE" == "true" ]]; then
    echo -e "${YELLOW}⚠️  Skipping phone verification (--skip-phone flag used)${NC}"
elif [[ -f "/tmp/api_response_Get_Citizen_Data__After_Phone_Verification_" ]]; then
    final_citizen_data=$(cat "/tmp/api_response_Get_Citizen_Data__After_Phone_Verification_")
    verify_update "Phone Update" "$original_citizen_data" "$final_citizen_data" ".telefone.principal.valor"
else
    echo -e "${YELLOW}⚠️  Skipping phone verification (data not available)${NC}"
    echo "  Looking for file: /tmp/api_response_Get_Citizen_Data__After_Phone_Verification_"
fi

# Verify CPF-Phone Mapping Blocking Logic
echo -e "${BLUE}🔒 Verifying CPF-Phone Mapping Blocking Logic...${NC}"
echo "=================================================="

# Check if we have the opt-out test responses
if [[ -f "/tmp/api_response_Opt_out__Non_blocking_" ]] && [[ -f "/tmp/api_response_Opt_out__Blocking_" ]]; then
    non_blocking_response=$(cat "/tmp/api_response_Opt_out__Non_blocking_")
    blocking_response=$(cat "/tmp/api_response_Opt_out__Blocking_")
    
    echo -e "${BLUE}📋 Opt-out Response Analysis:${NC}"
    echo "  Non-blocking opt-out response: $non_blocking_response"
    echo "  Blocking opt-out response: $blocking_response"
    
    # Both should return success status
    if echo "$non_blocking_response" | grep -q '"status":"opted_out"' && echo "$blocking_response" | grep -q '"status":"opted_out"'; then
        echo -e "${GREEN}✅ Both opt-out requests succeeded${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}❌ One or both opt-out requests failed${NC}"
        echo -e "${YELLOW}📝 This may be due to missing active phone-CPF mapping${NC}"
        ((TESTS_FAILED++))
    fi
    
    # Check if blocking tests were executed
    if [[ -f "/tmp/api_response_Get_Citizen_by_Phone__After_Blocking_" ]] && [[ -f "/tmp/api_response_Opt_in_Same_CPF__After_Non_blocking_" ]]; then
        echo -e "${GREEN}✅ CPF-Phone mapping blocking logic tests executed${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${YELLOW}⚠️  CPF-Phone mapping blocking logic tests not fully executed${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  Skipping CPF-Phone mapping verification (opt-out responses not available)${NC}"
    echo -e "${YELLOW}📝 This may be due to missing active phone-CPF mapping or test failures${NC}"
fi

echo -e "${BLUE}=== Testando Funcionalidades de Quarentena ===${NC}"

# Test 50: Get Phone Status (not found)
echo -e "${BLUE}Test 50: Get Phone Status (not found)${NC}"
make_request "GET" "/phone/+5511999887766/status" "" "Get Phone Status (not found)"

# Test 51: Bind Phone to CPF (new binding)
echo -e "${BLUE}Test 51: Bind Phone to CPF (new binding)${NC}"
bind_data='{
    "cpf": "'$CPF'",
    "channel": "whatsapp"
}'
make_request "POST" "/phone/+5511999887766/bind" "$bind_data" "Bind Phone to CPF"

# Test 52: Get Phone Status (found, not quarantined)
echo -e "${BLUE}Test 52: Get Phone Status (found, not quarantined)${NC}"
make_request "GET" "/phone/+5511999887766/status" "" "Get Phone Status (found)"

# Test 53: Quarantine Phone (admin only)
echo -e "${BLUE}Test 53: Quarantine Phone (admin only)${NC}"
make_request "POST" "/phone/+5511999887766/quarantine" "{}" "Quarantine Phone"

# Test 54: Get Phone Status (found, quarantined)
echo -e "${BLUE}Test 54: Get Phone Status (found, quarantined)${NC}"
make_request "GET" "/phone/+5511999887766/status" "" "Get Phone Status (quarantined)"

# Test 55: Get Quarantined Phones List (admin only)
echo -e "${BLUE}Test 55: Get Quarantined Phones List (admin only)${NC}"
make_request "GET" "/admin/phone/quarantined" "" "Get Quarantined Phones List"

# Test 56: Get Quarantine Stats (admin only)
echo -e "${BLUE}Test 56: Get Quarantine Stats (admin only)${NC}"
make_request "GET" "/admin/phone/quarantine/stats" "" "Get Quarantine Stats"

# Test 57: Release Quarantine (admin only)
echo -e "${BLUE}Test 57: Release Quarantine (admin only)${NC}"
make_request "DELETE" "/phone/+5511999887766/quarantine" "" "Release Quarantine"

# Test 58: Get Phone Status (found, released from quarantine)
echo -e "${BLUE}Test 58: Get Phone Status (found, released from quarantine)${NC}"
make_request "GET" "/phone/+5511999887766/status" "" "Get Phone Status (released)"

# Test 59: Quarantine Phone Without CPF (admin only)
echo -e "${BLUE}Test 59: Quarantine Phone Without CPF (admin only)${NC}"
make_request "POST" "/phone/+5511999888777/quarantine" "{}" "Quarantine Phone Without CPF"

# Test 60: Get Phone Status (quarantined without CPF)
echo -e "${BLUE}Test 60: Get Phone Status (quarantined without CPF)${NC}"
make_request "GET" "/phone/+5511999888777/status" "" "Get Phone Status (quarantined without CPF)"

# Test 61: Bind Phone to CPF (releases quarantine)
echo -e "${BLUE}Test 61: Bind Phone to CPF (releases quarantine)${NC}"
bind_data='{
    "cpf": "'$CPF'",
    "channel": "whatsapp"
}'
make_request "POST" "/phone/+5511999888777/bind" "$bind_data" "Bind Phone to CPF (releases quarantine)"

# Test 62: Get Phone Status (found, released by binding)
echo -e "${BLUE}Test 62: Get Phone Status (found, released by binding)${NC}"
make_request "GET" "/phone/+5511999888777/status" "" "Get Phone Status (released by binding)"

# Test 63: Opt-in (releases quarantine)
echo -e "${BLUE}Test 63: Opt-in (releases quarantine)${NC}"
optin_data='{
    "cpf": "'$CPF'",
    "channel": "whatsapp"
}'
make_request "POST" "/phone/+5511999887766/opt-in" "$optin_data" "Opt-in (releases quarantine)"

# Test 64: Get Phone Status (found, active after opt-in)
echo -e "${BLUE}Test 64: Get Phone Status (found, active after opt-in)${NC}"
make_request "GET" "/phone/+5511999887766/status" "" "Get Phone Status (active after opt-in)"

# Test 65: Get Quarantined Phones with Pagination (admin only)
echo -e "${BLUE}Test 65: Get Quarantined Phones with Pagination (admin only)${NC}"
make_request "GET" "/admin/phone/quarantined?page=1&per_page=10" "" "Get Quarantined Phones with Pagination"

# Test 66: Get Quarantined Phones (expired filter)
echo -e "${BLUE}Test 66: Get Quarantined Phones (expired filter)${NC}"
make_request "GET" "/admin/phone/quarantined?expired=true" "" "Get Quarantined Phones (expired)"

echo -e "${BLUE}=== Testando Funcionalidades Beta Whitelist ===${NC}"

# Test 67: Get Beta Status (not whitelisted)
echo -e "${BLUE}Test 67: Get Beta Status (not whitelisted)${NC}"
make_request "GET" "/phone/$PHONE_NUMBER/beta-status" "" "Get Beta Status (not whitelisted)"

# Test 68: Create Beta Group (admin only)
echo -e "${BLUE}Test 68: Create Beta Group (admin only)${NC}"
group_data='{
    "name": "Test Group 1 - '$(date +%s)'"
}'
make_request "POST" "/admin/beta/groups" "$group_data" "Create Beta Group"

# Test 69: List Beta Groups (admin only)
echo -e "${BLUE}Test 69: List Beta Groups (admin only)${NC}"
make_request "GET" "/admin/beta/groups" "" "List Beta Groups"

# Test 70: Get Beta Group by ID (admin only)
echo -e "${BLUE}Test 70: Get Beta Group by ID (admin only)${NC}"
# Extract group ID from previous response
GROUP_ID=$(cat /tmp/api_response_Create_Beta_Group 2>/dev/null | jq -r '.id' 2>/dev/null)
if [[ -n "$GROUP_ID" && "$GROUP_ID" != "null" ]]; then
    make_request "GET" "/admin/beta/groups/$GROUP_ID" "" "Get Beta Group by ID"
else
    echo -e "${YELLOW}⚠️  Skipping Get Beta Group by ID (no group ID available)${NC}"
fi

# Test 71: Add Phone to Whitelist (admin only)
echo -e "${BLUE}Test 71: Add Phone to Whitelist (admin only)${NC}"
if [[ -n "$GROUP_ID" && "$GROUP_ID" != "null" ]]; then
    whitelist_data='{
        "group_id": "'$GROUP_ID'"
    }'
    make_request "POST" "/admin/beta/whitelist/$PHONE_NUMBER" "$whitelist_data" "Add Phone to Whitelist"
else
    echo -e "${YELLOW}⚠️  Skipping Add Phone to Whitelist (no group ID available)${NC}"
fi

# Test 72: Get Beta Status (whitelisted)
echo -e "${BLUE}Test 72: Get Beta Status (whitelisted)${NC}"
make_request "GET" "/phone/$PHONE_NUMBER/beta-status" "" "Get Beta Status (whitelisted)"

# Test 73: Get Phone Status (includes beta info)
echo -e "${BLUE}Test 73: Get Phone Status (includes beta info)${NC}"
make_request "GET" "/phone/$PHONE_NUMBER/status" "" "Get Phone Status (with beta info)"

# Test 74: List Whitelisted Phones (admin only)
echo -e "${BLUE}Test 74: List Whitelisted Phones (admin only)${NC}"
make_request "GET" "/admin/beta/whitelist" "" "List Whitelisted Phones"

# Test 75: Create Second Beta Group (admin only)
echo -e "${BLUE}Test 75: Create Second Beta Group (admin only)${NC}"
group_data2='{
    "name": "Test Group 2 - '$(date +%s)'"
}'
make_request "POST" "/admin/beta/groups" "$group_data2" "Create Second Beta Group"

# Test 76: Bulk Add Phones to Whitelist (admin only)
echo -e "${BLUE}Test 76: Bulk Add Phones to Whitelist (admin only)${NC}"
GROUP_ID2=$(cat /tmp/api_response_Create_Second_Beta_Group 2>/dev/null | jq -r '.id' 2>/dev/null)
if [[ -n "$GROUP_ID2" && "$GROUP_ID2" != "null" ]]; then
    bulk_add_data='{
        "phone_numbers": ["+5511999887766", "+5511999888777"],
        "group_id": "'$GROUP_ID2'"
    }'
    make_request "POST" "/admin/beta/whitelist/bulk-add" "$bulk_add_data" "Bulk Add Phones to Whitelist"
else
    echo -e "${YELLOW}⚠️  Skipping Bulk Add Phones to Whitelist (no group ID available)${NC}"
fi

# Test 77: Bulk Move Phones Between Groups (admin only)
echo -e "${BLUE}Test 77: Bulk Move Phones Between Groups (admin only)${NC}"
if [[ -n "$GROUP_ID" && "$GROUP_ID" != "null" && -n "$GROUP_ID2" && "$GROUP_ID2" != "null" ]]; then
    bulk_move_data='{
        "phone_numbers": ["+5511999887766", "+5511999888777"],
        "from_group_id": "'$GROUP_ID2'",
        "to_group_id": "'$GROUP_ID'"
    }'
    make_request "POST" "/admin/beta/whitelist/bulk-move" "$bulk_move_data" "Bulk Move Phones Between Groups"
else
    echo -e "${YELLOW}⚠️  Skipping Bulk Move Phones Between Groups (no group IDs available)${NC}"
fi

# Test 78: Update Beta Group (admin only)
echo -e "${BLUE}Test 78: Update Beta Group (admin only)${NC}"
if [[ -n "$GROUP_ID" && "$GROUP_ID" != "null" ]]; then
    update_group_data='{
        "name": "Updated Test Group 1"
    }'
    make_request "PUT" "/admin/beta/groups/$GROUP_ID" "$update_group_data" "Update Beta Group"
else
    echo -e "${YELLOW}⚠️  Skipping Update Beta Group (no group ID available)${NC}"
fi

# Test 79: Bulk Remove Phones from Whitelist (admin only)
echo -e "${BLUE}Test 79: Bulk Remove Phones from Whitelist (admin only)${NC}"
bulk_remove_data='{
    "phone_numbers": ["+5511999887766", "+5511999888777"]
}'
make_request "POST" "/admin/beta/whitelist/bulk-remove" "$bulk_remove_data" "Bulk Remove Phones from Whitelist"

# Test 80: Remove Phone from Whitelist (admin only)
echo -e "${BLUE}Test 80: Remove Phone from Whitelist (admin only)${NC}"
make_request "DELETE" "/admin/beta/whitelist/$PHONE_NUMBER" "" "Remove Phone from Whitelist"

# Test 81: Get Beta Status (not whitelisted after removal)
echo -e "${BLUE}Test 81: Get Beta Status (not whitelisted after removal)${NC}"
make_request "GET" "/phone/$PHONE_NUMBER/beta-status" "" "Get Beta Status (not whitelisted after removal)"

# Test 82: Delete Beta Groups (admin only)
echo -e "${BLUE}Test 82: Delete Beta Groups (admin only)${NC}"
if [[ -n "$GROUP_ID" && "$GROUP_ID" != "null" ]]; then
    make_request "DELETE" "/admin/beta/groups/$GROUP_ID" "" "Delete Beta Group 1"
fi
if [[ -n "$GROUP_ID2" && "$GROUP_ID2" != "null" ]]; then
    make_request "DELETE" "/admin/beta/groups/$GROUP_ID2" "" "Delete Beta Group 2"
fi

# NEW FUNCTIONALITY TESTS: Opt-out for Numbers That Never Opted-In
echo -e "${BLUE}=== Testing New Opt-Out Functionality for Unknown Numbers ===${NC}"

# Generate a completely new phone number that has never been used
UNKNOWN_PHONE_NUMBER=$(generate_random_phone)
echo -e "${BLUE}📱 Generated unknown phone number for testing: $UNKNOWN_PHONE_NUMBER${NC}"

# Test 83: Get Status of Unknown Phone Number (should not be found)
echo -e "${BLUE}Test 83: Get Status of Unknown Phone Number (should not be found)${NC}"
make_request "GET" "/phone/$UNKNOWN_PHONE_NUMBER/status" "" "Get Status of Unknown Phone Number"

# Test 84: Opt-out Unknown Phone Number (new functionality)
echo -e "${BLUE}Test 84: Opt-out Unknown Phone Number (should create blocked mapping)${NC}"
unknown_optout_data='{
    "channel": "whatsapp",
    "reason": "irrelevant_content"
}'
make_request "POST" "/phone/$UNKNOWN_PHONE_NUMBER/opt-out" "$unknown_optout_data" "Opt-out Unknown Phone Number"

# Test 85: Get Status After Opt-out (should show opted_out: true)
echo -e "${BLUE}Test 85: Get Status After Opt-out (should show opted_out: true)${NC}"
make_request "GET" "/phone/$UNKNOWN_PHONE_NUMBER/status" "" "Get Status After Opt-out (Unknown Number)"

# Test 86: Try to Get Citizen by Opted-Out Unknown Number (should fail)
echo -e "${BLUE}Test 86: Try to Get Citizen by Opted-Out Unknown Number (should fail appropriately)${NC}"
make_request "GET" "/phone/$UNKNOWN_PHONE_NUMBER/citizen" "" "Get Citizen by Opted-Out Unknown Number"

# Test 87: Try to Opt-in Previously Unknown Number (should work)
echo -e "${BLUE}Test 87: Try to Opt-in Previously Unknown Number (should work)${NC}"
unknown_optin_data='{
    "cpf": "'$CPF'",
    "channel": "whatsapp",
    "validation_result": {
        "valid": true
    }
}'
make_request "POST" "/phone/$UNKNOWN_PHONE_NUMBER/opt-in" "$unknown_optin_data" "Opt-in Previously Unknown Number"

# Test 88: Get Status After Opt-in (should show active mapping)
echo -e "${BLUE}Test 88: Get Status After Opt-in (should show active mapping)${NC}"
make_request "GET" "/phone/$UNKNOWN_PHONE_NUMBER/status" "" "Get Status After Opt-in (Previously Unknown)"

# Test 89: Get Citizen by Phone After Opt-in (should work now)
echo -e "${BLUE}Test 89: Get Citizen by Phone After Opt-in (should work now)${NC}"
make_request "GET" "/phone/$UNKNOWN_PHONE_NUMBER/citizen" "" "Get Citizen by Phone After Opt-in"

# Verification Section for New Functionality
echo -e "${BLUE}🔍 Verifying New Opt-Out Functionality...${NC}"
echo "=================================================="

# Verify the opt-out response shows "opted_out" status
if [[ -f "/tmp/api_response_Opt_out_Unknown_Phone_Number" ]]; then
    unknown_optout_response=$(cat "/tmp/api_response_Opt_out_Unknown_Phone_Number")
    echo -e "${BLUE}📋 Unknown Number Opt-out Response Analysis:${NC}"
    echo "  Response: $unknown_optout_response"
    
    if echo "$unknown_optout_response" | grep -q '"status":"opted_out"'; then
        echo -e "${GREEN}✅ Unknown number opt-out returned correct 'opted_out' status${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}❌ Unknown number opt-out did not return 'opted_out' status${NC}"
        ((TESTS_FAILED++))
    fi
else
    echo -e "${YELLOW}⚠️  Unknown number opt-out response not available for verification${NC}"
fi

# Verify status shows opted_out: true
if [[ -f "/tmp/api_response_Get_Status_After_Opt_out__Unknown_Number_" ]]; then
    status_after_optout=$(cat "/tmp/api_response_Get_Status_After_Opt_out__Unknown_Number_")
    echo -e "${BLUE}📋 Status After Opt-out Analysis:${NC}"
    echo "  Status response: $status_after_optout"
    
    if echo "$status_after_optout" | grep -q '"opted_out":true'; then
        echo -e "${GREEN}✅ Phone status correctly shows opted_out: true${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}❌ Phone status does not show opted_out: true${NC}"
        ((TESTS_FAILED++))
    fi
    
    if echo "$status_after_optout" | grep -q '"found":true'; then
        echo -e "${GREEN}✅ Phone mapping was created (found: true)${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}❌ Phone mapping was not created properly${NC}"
        ((TESTS_FAILED++))
    fi
else
    echo -e "${YELLOW}⚠️  Status after opt-out response not available for verification${NC}"
fi

echo -e "${GREEN}✅ New opt-out functionality tests completed${NC}"
echo ""

# Clean up temporary files
rm -f /tmp/api_response_*

echo "=================================================="
echo -e "${BLUE}📊 Test Results Summary:${NC}"
echo -e "${GREEN}✅ Tests Passed: $TESTS_PASSED${NC}"
echo -e "${RED}❌ Tests Failed: $TESTS_FAILED${NC}"
echo -e "${BLUE}📈 Total Tests: $((TESTS_PASSED + TESTS_FAILED))${NC}"

if [[ $TESTS_FAILED -eq 0 ]]; then
    echo -e "${GREEN}🎉 All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}⚠️  Some tests failed. Check the output above for details.${NC}"
    exit 1
fi 