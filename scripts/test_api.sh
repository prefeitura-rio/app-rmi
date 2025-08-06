#!/bin/bash

# API Testing Script for RMI API
# Usage: ./scripts/test_api.sh <CPF> <BEARER_TOKEN> [PHONE_NUMBER]

# set -e  # Disabled to prevent script from exiting on errors

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
API_BASE_URL="http://localhost:8080/v1"
CPF="${1:-}"
BEARER_TOKEN="${2:-}"
PHONE_NUMBER="${3:-}"

# Validation
if [[ -z "$CPF" ]]; then
    echo -e "${RED}Error: CPF is required${NC}"
    echo "Usage: $0 <CPF> <BEARER_TOKEN> [PHONE_NUMBER]"
    echo "Example: $0 12345678901 'Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...' '+5511999887766'"
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
    if [[ "$status_code" -ge 200 && "$status_code" -lt 300 ]] || [[ "$status_code" -eq 404 ]]; then
        print_result "$test_name" "PASS" "$response_body"
        # Store response body in a global variable for verification
        if [[ "$test_name" == *"(Original)"* ]] || [[ "$test_name" == *"(After Updates)"* ]] || [[ "$test_name" == *"(After Phone Verification)"* ]]; then
            # For verification tests, store in a temporary file
            echo "$response_body" > "/tmp/api_response_${test_name//[^a-zA-Z0-9]/_}"
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
address_data='{
    "logradouro": "Rua Teste API",
    "numero": "999",
    "complemento": "Sala 42",
    "bairro": "Botafogo",
    "municipio": "Rio de Janeiro",
    "estado": "RJ",
    "cep": "22250-040"
}'
make_request "PUT" "/citizen/$CPF/address" "$address_data" "Update Address"

# Test 10: Update Email
email_data='{
    "valor": "teste.api@exemplo.com"
}'
make_request "PUT" "/citizen/$CPF/email" "$email_data" "Update Email"

# Test 11: Update Ethnicity
ethnicity_data='{
    "valor": "preta"
}'
make_request "PUT" "/citizen/$CPF/ethnicity" "$ethnicity_data" "Update Ethnicity"

# Test 12: Update First Login
firstlogin_data='{
    "first_login": false
}'
make_request "PUT" "/citizen/$CPF/firstlogin" "$firstlogin_data" "Update First Login"

# Test 13: Update Opt-In
optin_data='{
    "optin": true
}'
make_request "PUT" "/citizen/$CPF/optin" "$optin_data" "Update Opt-In"

# Test 14: Update Phone (requires verification)
if [[ -n "$PHONE_NUMBER" ]]; then
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

# Verification Section
echo -e "${BLUE}🔍 Verifying Data Updates...${NC}"
echo "=================================================="

# Read data from temporary files for verification
if [[ -f "/tmp/api_response_Get_Citizen_Data_Original" ]] && [[ -f "/tmp/api_response_Get_Citizen_Data_After_Updates" ]]; then
    original_citizen_data=$(cat "/tmp/api_response_Get_Citizen_Data_Original")
    updated_citizen_data=$(cat "/tmp/api_response_Get_Citizen_Data_After_Updates")
    
    # Verify Address Update
    verify_update "Address Update" "$original_citizen_data" "$updated_citizen_data" ".endereco.logradouro"
    
    # Verify Email Update
    verify_update "Email Update" "$original_citizen_data" "$updated_citizen_data" ".email"
    
    # Verify Ethnicity Update
    verify_update "Ethnicity Update" "$original_citizen_data" "$updated_citizen_data" ".raca"
else
    echo -e "${YELLOW}⚠️  Skipping citizen data verification (data not available)${NC}"
fi

# Verify First Login Update
if [[ -f "/tmp/api_response_Get_First_Login_Status_Original" ]] && [[ -f "/tmp/api_response_Get_First_Login_Status_Updated" ]]; then
    original_firstlogin_data=$(cat "/tmp/api_response_Get_First_Login_Status_Original")
    updated_firstlogin_data=$(cat "/tmp/api_response_Get_First_Login_Status_Updated")
    verify_update "First Login Update" "$original_firstlogin_data" "$updated_firstlogin_data" ".first_login"
else
    echo -e "${YELLOW}⚠️  Skipping first login verification (data not available)${NC}"
fi

# Verify Opt-In Update
if [[ -f "/tmp/api_response_Get_Opt_In_Status_Original" ]] && [[ -f "/tmp/api_response_Get_Opt_In_Status_Updated" ]]; then
    original_optin_data=$(cat "/tmp/api_response_Get_Opt_In_Status_Original")
    updated_optin_data=$(cat "/tmp/api_response_Get_Opt_In_Status_Updated")
    verify_update "Opt-In Update" "$original_optin_data" "$updated_optin_data" ".optin"
else
    echo -e "${YELLOW}⚠️  Skipping opt-in verification (data not available)${NC}"
fi

# Verify Phone Update (if phone verification was completed)
if [[ -f "/tmp/api_response_Get_Citizen_Data_After_Phone_Verification" ]]; then
    final_citizen_data=$(cat "/tmp/api_response_Get_Citizen_Data_After_Phone_Verification")
    verify_update "Phone Update" "$original_citizen_data" "$final_citizen_data" ".telefone.principal.valor"
fi

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