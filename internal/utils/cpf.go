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

// cnpjFormattingRE matches the punctuation and whitespace accepted around a CNPJ
// (e.g. "12.ABC.345/01DE-35"); any other character is kept and rejected by ValidateCNPJ.
var cnpjFormattingRE = regexp.MustCompile(`[\s./-]`)

// cnpjFormatRE is the Receita Federal CNPJ format: 12 alphanumeric positions
// (A-Z or 0-9) followed by 2 numeric check digits. Purely numeric CNPJs match it too.
var cnpjFormatRE = regexp.MustCompile(`^[0-9A-Z]{12}[0-9]{2}$`)

var cnpjFirstDVWeights = []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
var cnpjSecondDVWeights = []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}

// NormalizeCNPJ uppercases the ASCII letters of a CNPJ and strips its formatting
// (dots, slash, hyphen and whitespace), producing the form stored in the database.
// Only a-z is uppercased: strings.ToUpper would turn non-ASCII runes such as 'ı'
// or 'ſ' into 'I' or 'S', making them pass as a valid CNPJ.
func NormalizeCNPJ(cnpj string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r - ('a' - 'A')
		}
		return r
	}, cnpjFormattingRE.ReplaceAllString(cnpj, ""))
}

// ValidateCNPJ validates a CNPJ, numeric or alphanumeric (Receita Federal format).
// After NormalizeCNPJ it must have 12 alphanumeric positions (A-Z, 0-9) and 2 numeric
// check digits. Each character is worth its ASCII code minus 48 ('0' = 0, 'A' = 17)
// in the modulo 11 check digit calculation, with the same weights as the numeric CNPJ.
func ValidateCNPJ(cnpj string) bool {
	cnpj = NormalizeCNPJ(cnpj)

	if !cnpjFormatRE.MatchString(cnpj) {
		return false
	}

	// Reject CNPJs made of a single repeated character
	if strings.Count(cnpj, cnpj[:1]) == len(cnpj) {
		return false
	}

	return int(cnpj[12]-'0') == cnpjCheckDigit(cnpj[:12], cnpjFirstDVWeights) &&
		int(cnpj[13]-'0') == cnpjCheckDigit(cnpj[:13], cnpjSecondDVWeights)
}

// cnpjCheckDigit computes a CNPJ check digit (modulo 11) over base, whose characters
// are already known to be in [0-9A-Z], weighting each by its ASCII code minus 48.
func cnpjCheckDigit(base string, weights []int) int {
	sum := 0
	for i := 0; i < len(base); i++ {
		sum += int(base[i]-'0') * weights[i]
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}
