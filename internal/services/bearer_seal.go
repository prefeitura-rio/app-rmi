package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/prefeitura-rio/app-rmi/internal/config"
)

const (
	bearerSealPrefix = "enc:v1:"

	bearerReasonAbsent               = "absent"
	bearerReasonSealed               = "sealed"
	bearerReasonEncryptionKeyMissing = "encryption_key_missing"
	bearerReasonUnsealFailed         = "unseal_failed"
	bearerReasonEnqueueFailed        = "enqueue_failed"
	bearerReasonEnqueued             = "enqueued"
	bearerReasonAbsentOnJob          = "absent_on_job"
)

func bearerEncryptionKey() []byte {
	if config.AppConfig == nil {
		return nil
	}
	return config.AppConfig.SyncJobBearerEncryptionKey
}

func bearerAAD(jobType, jobKey string) []byte {
	return []byte(jobType + "\x00" + jobKey)
}

func prepareBearerForQueue(plain, jobType, jobKey string) (string, string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", bearerReasonAbsent, nil
	}
	key := bearerEncryptionKey()
	if len(key) == 0 {
		if jobType == SalesforcePushQueue {
			return "", bearerReasonEncryptionKeyMissing, fmt.Errorf("SYNC_JOB_BEARER_ENCRYPTION_KEY is required to enqueue salesforce push")
		}
		// Never persist a JWT in plaintext. Other queues omit the token until a key is configured.
		return "", bearerReasonEncryptionKeyMissing, nil
	}
	sealed, err := sealBearerToken(key, plain, bearerAAD(jobType, jobKey))
	if err != nil {
		return "", "", err
	}
	return sealed, bearerReasonSealed, nil
}

func bearerFromQueue(stored, jobType, jobKey string) (string, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, bearerSealPrefix) {
		return stored, nil
	}
	key := bearerEncryptionKey()
	if len(key) == 0 {
		return "", fmt.Errorf("encrypted bearer token requires SYNC_JOB_BEARER_ENCRYPTION_KEY")
	}
	return openBearerToken(key, stored, bearerAAD(jobType, jobKey))
}

func sealBearerToken(key []byte, plain string, aad []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("invalid bearer encryption key: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("bearer encryption init failed: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("bearer encryption nonce failed: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), aad)
	return bearerSealPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func openBearerToken(key []byte, stored string, aad []byte) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, bearerSealPrefix))
	if err != nil {
		return "", fmt.Errorf("invalid sealed bearer token: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("invalid bearer encryption key: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("bearer decryption init failed: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize+gcm.Overhead() {
		return "", fmt.Errorf("sealed bearer token is truncated")
	}
	plain, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], aad)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt bearer token: %w", err)
	}
	return string(plain), nil
}
