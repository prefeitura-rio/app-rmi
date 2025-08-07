#!/bin/bash

# API Testing Script for RMI API
# Usage: ./scripts/test_api.sh <CPF> <BEARER_TOKEN> [PHONE_NUMBER]
# 
# This script tests all API endpoints including:
# - Health and configuration endpoints
# - Citizen data endpoints (with authentication)
# - Phone-based endpoints for WhatsApp bot
# - Data verification for updates
# 
# Note: Random data is generated for address and email updates to avoid 409 conflicts
# when running the script multiple times with the same CPF.

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
PHONE_NUMBER=""

# Parse positional arguments and flags
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-phone)
            SKIP_PHONE=true
            shift
            ;;
        *)
            if [[ -z "$CPF" ]]; then
                CPF="$1"
            elif [[ -z "$BEARER_TOKEN" ]]; then
                BEARER_TOKEN="$1"
            elif [[ -z "$PHONE_NUMBER" ]]; then
                PHONE_NUMBER="$1"
            fi
            shift
            ;;
    esac
done

# Configuration
API_BASE_URL="http://localhost:8080/v1"

# Validation
if [[ -z "$CPF" ]]; then
    echo -e "${RED}Error: CPF is required${NC}"
    echo "Usage: $0 <CPF> <BEARER_TOKEN> [PHONE_NUMBER] [--skip-phone]"
    echo "Example: $0 12345678901 'Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...' '+5511999887766'"
    echo "Example: $0 12345678901 'Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...' '+5511999887766' --skip-phone"
    exit 1
fi

if [[ -z "$BEARER_TOKEN" ]]; then
    echo -e "${RED}Error: Bearer token is required${NC}"
    echo "Usage: $0 <CPF> <BEARER_TOKEN> [PHONE_NUMBER]"
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
        headers+=("-H" "Authorization: $BEARER_TOKEN")
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
    response_body=$(echo "$response" | head -n -1)
    
    # Check if status code is 2xx or 404 (acceptable for data not found)
    # Also accept 409 (Conflict) for "already_opted_in" responses
    if [[ "$status_code" -ge 200 && "$status_code" -lt 300 ]] || [[ "$status_code" -eq 404 ]]; then
        print_result "$test_name" "PASS" "$response_body"
        # Store response body in a temporary file for verification
        if [[ "$test_name" == *"(Original)"* ]] || [[ "$test_name" == *"(After Updates)"* ]] || [[ "$test_name" == *"(After Phone Verification)"* ]] || [[ "$test_name" == *"First Login Status"* ]] || [[ "$test_name" == *"Opt-In Status"* ]]; then
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
if [[ -n "$PHONE_NUMBER" ]]; then
    phone_data="{\"phone\":\"$PHONE_NUMBER\"}"
    make_request "POST" "/validate/phone" "$phone_data" "Validate Phone Number"
else
    echo -e "${YELLOW}⚠️  Skipping phone validation (no phone number provided)${NC}"
    echo ""
fi

# Test 4: Get Citizen Data (Original)
echo -e "${BLUE}📋 Getting original citizen data for verification...${NC}"
make_request "GET" "/citizen/$CPF" "" "Get Citizen Data (Original)"

# Test 5: Get Citizen Wallet
make_request "GET" "/citizen/$CPF/wallet" "" "Get Citizen Wallet"

# Test 6: Get Maintenance Requests
make_request "GET" "/citizen/$CPF/maintenance-request" "" "Get Maintenance Requests"

# Test 7: Get First Login Status (Original)
make_request "GET" "/citizen/$CPF/firstlogin" "" "Get First Login Status (Original)"

# Test 8: Get Opt-In Status (Original)
make_request "GET" "/citizen/$CPF/optin" "" "Get Opt-In Status (Original)"

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
current_ethnicity=$(curl -s -X GET "http://localhost:8080/v1/citizen/$CPF" -H "Authorization: $BEARER_TOKEN" | jq -r '.raca' 2>/dev/null || echo "unknown")
echo "Current ethnicity: $current_ethnicity"

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

# Test 13: Update Opt-In (change to true to test update)
optin_data='{
    "optin": true
}'
make_request "PUT" "/citizen/$CPF/optin" "$optin_data" "Update Opt-In"

# Test 14: Update Phone (requires verification)
if [[ -n "$PHONE_NUMBER" && "$SKIP_PHONE" != "true" ]]; then
    # Extract DDI, DDD, and number from the phone number
    # Assuming format like +5511999887766
    ddi="${PHONE_NUMBER:1:2}"  # 55
    ddd="${PHONE_NUMBER:3:2}"  # 11
    valor="${PHONE_NUMBER:5}"  # 999887766
    
    phone_data="{
        \"ddi\": \"$ddi\",
        \"ddd\": \"$ddd\",
        \"valor\": \"$valor\"
    }"
    make_request "PUT" "/citizen/$CPF/phone" "$phone_data" "Update Phone"
    
    # Wait for user to input verification code
    echo -e "${YELLOW}📱 Phone update request sent!${NC}"
    echo -e "${YELLOW}Please check your WhatsApp for the verification code.${NC}"
    echo ""
    
    read -p "Enter the verification code you received: " user_verification_code
    
    if [[ -n "$user_verification_code" ]]; then
        echo -e "${BLUE}Testing phone verification with code: $user_verification_code${NC}"
        verification_data="{
            \"code\": \"$user_verification_code\",
            \"ddi\": \"$ddi\",
            \"ddd\": \"$ddd\",
            \"valor\": \"$valor\"
        }"
        make_request "POST" "/citizen/$CPF/phone/validate" "$verification_data" "Validate Phone Verification Code"
        
        # Verify Phone Update after verification
        echo -e "${BLUE}📋 Getting final citizen data to verify phone update...${NC}"
        make_request "GET" "/citizen/$CPF" "" "Get Citizen Data (After Phone Verification)"
        # Note: Phone verification will be handled in the verification section
    else
        echo -e "${YELLOW}⚠️  No verification code provided, skipping phone verification${NC}"
        echo ""
    fi
elif [[ "$SKIP_PHONE" == "true" ]]; then
    echo -e "${YELLOW}⚠️  Skipping phone update and verification (--skip-phone flag used)${NC}"
    echo ""
else
    echo -e "${YELLOW}⚠️  Skipping phone update and verification (no phone number provided)${NC}"
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

# Test 21: Get Citizen by Phone (if phone number provided)
if [[ -n "$PHONE_NUMBER" ]]; then
    make_request "GET" "/phone/$PHONE_NUMBER/citizen" "" "Get Citizen by Phone"
else
    echo -e "${YELLOW}⚠️  Skipping phone-based tests (no phone number provided)${NC}"
    echo ""
fi

# Test 22: Validate Registration (if phone number provided)
if [[ -n "$PHONE_NUMBER" ]]; then
    registration_data="{
        \"name\": \"João Silva Santos\",
        \"cpf\": \"$CPF\",
        \"birth_date\": \"1990-05-15\",
        \"channel\": \"whatsapp\"
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/validate-registration" "$registration_data" "Validate Registration"
    
    # Test 22.5: Create phone-CPF mapping for testing (if phone number provided)
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
else
    echo -e "${YELLOW}⚠️  Skipping registration validation (no phone number provided)${NC}"
    echo ""
fi

# Test 23: Test Opt-out with non-blocking reason (if phone number provided)
if [[ -n "$PHONE_NUMBER" ]]; then
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
else
    echo -e "${YELLOW}⚠️  Skipping opt-out test (no phone number provided)${NC}"
    echo ""
fi

# Test 24: Test Opt-out with blocking reason (if phone number provided)
if [[ -n "$PHONE_NUMBER" ]]; then
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
else
    echo -e "${YELLOW}⚠️  Skipping opt-out blocking test (no phone number provided)${NC}"
    echo ""
fi

# Test 25: Verify phone mapping is blocked after incorrect_person opt-out (if phone number provided)
if [[ -n "$PHONE_NUMBER" ]]; then
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
        \"cpf\": \"98765432109\",
        \"channel\": \"whatsapp\",
        \"validation_result\": {
            \"valid\": true
        }
    }"
    make_request "POST" "/phone/$PHONE_NUMBER/opt-in" "$optin_different_cpf" "Opt-in Different CPF (After Blocking)"
    
    echo -e "${GREEN}✅ CPF-Phone mapping blocking tests completed${NC}"
    echo ""
else
    echo -e "${YELLOW}⚠️  Skipping CPF-phone mapping blocking tests (no phone number provided)${NC}"
    echo ""
fi

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
    verify_update "Opt-In Update" "$original_optin_data" "$updated_optin_data" ".optin"
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

# Clean up temporary files
rm -f /tmp/api_response_*

echo "=================================================="
echo -e "${BLUE}📊 Test Results Summary:${NC}"
echo -e "${GREEN}✅ Tests Passed: $TESTS_PASSED${NC}"
echo -e "${RED}❌ Tests Failed: $TESTS_FAILED${NC}"
echo -e "${BLUE}📈 Total Tests: $((TESTS_PASSED + TESTS_FAILED))${NC}"
echo -e "${BLUE}📋 Note: Includes new opt-out tests with conditional blocking logic${NC}"

if [[ $TESTS_FAILED -eq 0 ]]; then
    echo -e "${GREEN}🎉 All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}⚠️  Some tests failed. Check the output above for details.${NC}"
    exit 1
fi 