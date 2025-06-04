package utils

import (
	"fmt"
	"math/rand"
	"time"
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

// SendWhatsAppMessage sends a message via WhatsApp
// Now accepts DDI, DDD, phone, and message
func SendWhatsAppMessage(ddi, ddd, phone, message string) error {
	// For now, just log the message
	fmt.Printf("Sending WhatsApp message to +%s%s%s: %s\n", ddi, ddd, phone, message)
	return nil
} 