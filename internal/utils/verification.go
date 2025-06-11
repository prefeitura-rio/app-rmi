package utils

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
)

// GenerateVerificationCode generates a random 6-digit verification code
func GenerateVerificationCode() string {
	rand.Seed(time.Now().UnixNano())
	code := ""
	for i := 0; i < 6; i++ {
		code += fmt.Sprintf("%d", rand.Intn(10))
	}
	return code
}

// SendVerificationCode sends a verification code to a single phone number
func SendVerificationCode(ctx context.Context, phone string, code string) error {
	vars := map[string]interface{}{
		"codigo": code,
		"nome": "Fulaninho",
	}
	
	return SendWhatsAppMessage(
		ctx,
		[]string{phone},
		config.AppConfig.WhatsAppHSMID,
		[]map[string]interface{}{vars},
	)
} 