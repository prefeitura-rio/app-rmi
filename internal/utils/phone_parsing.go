package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// PhoneComponents represents the parsed components of a phone number
type PhoneComponents struct {
	DDI   string `json:"ddi"`
	DDD   string `json:"ddd"`
	Valor string `json:"valor"`
	Full  string `json:"full"`
}

// ParsePhoneNumber parses a phone number string and returns its components
func ParsePhoneNumber(phoneString string) (*PhoneComponents, error) {
	// Clean the phone string
	cleanPhone := strings.TrimSpace(phoneString)

	// If it doesn't start with +, try to add it
	if !strings.HasPrefix(cleanPhone, "+") {
		// If it starts with 55 (Brazil), add +
		if strings.HasPrefix(cleanPhone, "55") {
			cleanPhone = "+" + cleanPhone
		} else {
			// Assume it's a Brazilian number
			cleanPhone = "+55" + cleanPhone
		}
	}

	// Parse with libphonenumber
	num, err := phonenumbers.Parse(cleanPhone, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse phone number: %w", err)
	}

	if !phonenumbers.IsValidNumber(num) {
		return nil, fmt.Errorf("invalid phone number: %s", phoneString)
	}

	// Extract components
	countryCode := num.GetCountryCode()
	nationalNumber := phonenumbers.GetNationalSignificantNumber(num)

	// Initialize components
	components := &PhoneComponents{
		DDI:  fmt.Sprintf("%d", countryCode),
		Full: phonenumbers.Format(num, phonenumbers.E164),
	}

	// Extract DDD and Valor based on country
	if countryCode == 55 { // Brazil
		if len(nationalNumber) >= 2 {
			components.DDD = nationalNumber[:2]
			components.Valor = nationalNumber[2:]
		} else {
			components.Valor = nationalNumber
		}
	} else {
		// For international numbers, DDD might be 1-4 digits
		// We'll try to extract it intelligently
		if len(nationalNumber) >= 4 {
			// Assume first 2-4 digits are area code
			areaCodeLength := 2
			if len(nationalNumber) >= 6 {
				areaCodeLength = 3
			}
			if len(nationalNumber) >= 8 {
				areaCodeLength = 4
			}
			components.DDD = nationalNumber[:areaCodeLength]
			components.Valor = nationalNumber[areaCodeLength:]
		} else {
			components.Valor = nationalNumber
		}
	}

	return components, nil
}

// ValidatePhoneFormat validates if a phone string is in a valid format
func ValidatePhoneFormat(phoneString string) error {
	// Basic format validation
	phoneRegex := regexp.MustCompile(`^\+?[0-9]{10,15}$`)
	if !phoneRegex.MatchString(strings.ReplaceAll(phoneString, " ", "")) {
		return fmt.Errorf("invalid phone number format: %s", phoneString)
	}
	return nil
}

// FormatPhoneForStorage formats phone components for consistent storage
func FormatPhoneForStorage(ddi, ddd, valor string) string {
	return fmt.Sprintf("%s%s%s", ddi, ddd, valor)
}

// ExtractPhoneFromComponents extracts the full phone number from components
func ExtractPhoneFromComponents(ddi, ddd, valor string) string {
	return fmt.Sprintf("+%s%s%s", ddi, ddd, valor)
}
