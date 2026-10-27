package utils

import (
	"regexp"
	"strconv"
	"strings"
)

var nonDigitRE = regexp.MustCompile(`\D`)

// NormalizeCPF strips non-digit characters from a CPF string.
func NormalizeCPF(cpf string) string {
	return nonDigitRE.ReplaceAllString(cpf, "")
}

// ValidateCPF validates a CPF number
// It checks if the CPF has 11 digits and validates the check digits
func ValidateCPF(cpf string) bool {
	cpf = NormalizeCPF(cpf)

	// Check if CPF has 11 digits
	if len(cpf) != 11 {
		return false
	}

	// Check if all digits are the same
	allSame := true
	for i := 1; i < len(cpf); i++ {
		if cpf[i] != cpf[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}

	// Validate first check digit
	sum := 0
	for i := 0; i < 9; i++ {
		digit, _ := strconv.Atoi(string(cpf[i]))
		sum += digit * (10 - i)
	}
	remainder := sum % 11
	if remainder < 2 {
		if cpf[9] != '0' {
			return false
		}
	} else {
		expected := strconv.Itoa(11 - remainder)
		if string(cpf[9]) != expected {
			return false
		}
	}

	// Validate second check digit
	sum = 0
	for i := 0; i < 10; i++ {
		digit, _ := strconv.Atoi(string(cpf[i]))
		sum += digit * (11 - i)
	}
	remainder = sum % 11
	if remainder < 2 {
		if cpf[10] != '0' {
			return false
		}
	} else {
		expected := strconv.Itoa(11 - remainder)
		if string(cpf[10]) != expected {
			return false
		}
	}

	return true
}

// MaskEmail redacts an email for logs (keeps domain).
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "***"
	}
	local, domain := email[:at], email[at+1:]
	if len(local) <= 2 {
		return "*@" + domain
	}
	return local[:1] + "***" + local[len(local)-1:] + "@" + domain
}

// ValidateCNPJ validates a CNPJ number
// It checks if the CNPJ has 14 digits and validates the check digits
func ValidateCNPJ(cnpj string) bool {
	// Remove any non-digit characters
	cnpj = nonDigitRE.ReplaceAllString(cnpj, "")

	// Check if CNPJ has 14 digits
	if len(cnpj) != 14 {
		return false
	}

	// Check if all digits are the same
	allSame := true
	for i := 1; i < len(cnpj); i++ {
		if cnpj[i] != cnpj[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}

	// Validate first check digit
	weights := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		digit, _ := strconv.Atoi(string(cnpj[i]))
		sum += digit * weights[i]
	}
	remainder := sum % 11
	var expectedDigit int
	if remainder < 2 {
		expectedDigit = 0
	} else {
		expectedDigit = 11 - remainder
	}
	actualDigit, _ := strconv.Atoi(string(cnpj[12]))
	if actualDigit != expectedDigit {
		return false
	}

	// Validate second check digit
	weights = []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 13; i++ {
		digit, _ := strconv.Atoi(string(cnpj[i]))
		sum += digit * weights[i]
	}
	remainder = sum % 11
	if remainder < 2 {
		expectedDigit = 0
	} else {
		expectedDigit = 11 - remainder
	}
	actualDigit, _ = strconv.Atoi(string(cnpj[13]))
	return actualDigit == expectedDigit
}
