package utils

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/prefeitura-rio/app-rmi/internal/config"
)

// GenerateVerificationCode generates a random 6-digit verification code
func GenerateVerificationCode() string {
	code := ""
	for range 6 {
		code += fmt.Sprintf("%d", rand.Intn(10))
	}
	return code
}

// SendVerificationCode sends a verification code to a single phone number
func SendVerificationCode(ctx context.Context, phone string, code string) error {
	vars := map[string]interface{}{
		"COD": code,
	}
	
	return SendWhatsAppMessage(
		ctx,
		[]string{phone},
		config.AppConfig.WhatsAppHSMID,
		[]map[string]interface{}{vars},
	)
} 