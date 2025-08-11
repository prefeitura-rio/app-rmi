package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CPF        string             `bson:"cpf" json:"cpf"`
	Action     string             `bson:"action" json:"action"`
	Resource   string             `bson:"resource" json:"resource"`
	ResourceID string             `bson:"resource_id" json:"resource_id"`
	OldValue   interface{}        `bson:"old_value,omitempty" json:"old_value,omitempty"`
	NewValue   interface{}        `bson:"new_value,omitempty" json:"new_value,omitempty"`
	UserID     string             `bson:"user_id,omitempty" json:"user_id,omitempty"`
	IPAddress  string             `bson:"ip_address,omitempty" json:"ip_address,omitempty"`
	UserAgent  string             `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	RequestID  string             `bson:"request_id,omitempty" json:"request_id,omitempty"`
	Timestamp  time.Time          `bson:"timestamp" json:"timestamp"`
	Metadata   map[string]string  `bson:"metadata,omitempty" json:"metadata,omitempty"`
}

// Audit constants
const (
	AuditActionCreate   = "CREATE"
	AuditActionRead     = "READ"
	AuditActionUpdate   = "UPDATE"
	AuditActionDelete   = "DELETE"
	AuditActionValidate = "VALIDATE"
	AuditActionLogin    = "LOGIN"
	AuditActionLogout   = "LOGOUT"

	AuditResourceAddress           = "address"
	AuditResourcePhone             = "phone"
	AuditResourceEmail             = "email"
	AuditResourceEthnicity         = "ethnicity"
	AuditResourcePhoneVerification = "phone_verification"
	AuditResourceUserConfig        = "user_config"
	AuditResourceBetaGroup         = "beta_group"
	AuditResourceBetaWhitelist     = "beta_whitelist"
)

// AuditContext contains context information for audit logging
type AuditContext struct {
	CPF       string
	UserID    string
	IPAddress string
	UserAgent string
	RequestID string
}

// AuditWorker manages asynchronous audit logging
type AuditWorker struct {
	auditChan chan AuditLog
	workers   int
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

var (
	auditWorker *AuditWorker
	once        sync.Once
)

// InitAuditWorker initializes the audit worker
func InitAuditWorker(workers int, bufferSize int) {
	once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		auditWorker = &AuditWorker{
			auditChan: make(chan AuditLog, bufferSize),
			workers:   workers,
			ctx:       ctx,
			cancel:    cancel,
		}
		auditWorker.start()
	})
}

// start starts the audit worker pool
func (aw *AuditWorker) start() {
	for i := 0; i < aw.workers; i++ {
		aw.wg.Add(1)
		go aw.worker()
	}
}

// worker processes audit logs from the channel
func (aw *AuditWorker) worker() {
	defer aw.wg.Done()

	for {
		select {
		case auditLog, ok := <-aw.auditChan:
			if !ok {
				return
			}
			aw.processAuditLog(auditLog)
		case <-aw.ctx.Done():
			return
		}
	}
}

// processAuditLog processes a single audit log entry
func (aw *AuditWorker) processAuditLog(auditLog AuditLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use W(0) write concern for audit logs - fire and forget to avoid blocking
	// This prevents audit logging from slowing down main operations
	opts := GetInsertOptionsWithWriteConcern("audit")
	_, err := config.MongoDB.Collection(config.AppConfig.AuditLogsCollection).InsertOne(ctx, auditLog, opts)
	if err != nil {
		logging.Logger.Error("failed to insert audit log",
			zap.Error(err),
			zap.String("cpf", auditLog.CPF),
			zap.String("action", auditLog.Action),
			zap.String("resource", auditLog.Resource))
		return
	}

	logging.Logger.Info("audit event logged successfully",
		zap.String("cpf", auditLog.CPF),
		zap.String("action", auditLog.Action),
		zap.String("resource", auditLog.Resource),
		zap.String("resource_id", auditLog.ResourceID))
}

// Stop stops the audit worker
func (aw *AuditWorker) Stop() {
	if aw != nil {
		aw.cancel()
		close(aw.auditChan)
		aw.wg.Wait()
	}
}

// GetAuditWorker returns the global audit worker instance
func GetAuditWorker() *AuditWorker {
	return auditWorker
}

// LogAuditEvent logs an audit event to the audit collection asynchronously
func LogAuditEvent(ctx context.Context, auditCtx AuditContext, action, resource, resourceID string, oldValue, newValue interface{}, metadata map[string]string) error {
	// If audit logging is disabled, return immediately
	if !config.AppConfig.AuditLogsEnabled {
		return nil
	}

	// If audit worker is not initialized, log synchronously as fallback
	if auditWorker == nil {
		return logAuditEventSync(ctx, auditCtx, action, resource, resourceID, oldValue, newValue, metadata)
	}

	auditLog := AuditLog{
		CPF:        auditCtx.CPF,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		OldValue:   oldValue,
		NewValue:   newValue,
		UserID:     auditCtx.UserID,
		IPAddress:  auditCtx.IPAddress,
		UserAgent:  auditCtx.UserAgent,
		RequestID:  auditCtx.RequestID,
		Timestamp:  time.Now(),
		Metadata:   metadata,
	}

	// Try to send to audit channel, but don't block
	select {
	case auditWorker.auditChan <- auditLog:
		return nil
	default:
		// Channel is full, fall back to synchronous logging
		logging.Logger.Warn("audit channel full, falling back to synchronous logging",
			zap.String("cpf", auditCtx.CPF),
			zap.String("action", action))
		return logAuditEventSync(ctx, auditCtx, action, resource, resourceID, oldValue, newValue, metadata)
	}
}

// logAuditEventSync logs an audit event synchronously (fallback method)
func logAuditEventSync(ctx context.Context, auditCtx AuditContext, action, resource, resourceID string, oldValue, newValue interface{}, metadata map[string]string) error {
	logger := logging.Logger.With(
		zap.String("cpf", auditCtx.CPF),
		zap.String("action", action),
		zap.String("resource", resource),
	)

	auditLog := AuditLog{
		CPF:        auditCtx.CPF,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		OldValue:   oldValue,
		NewValue:   newValue,
		UserID:     auditCtx.UserID,
		IPAddress:  auditCtx.IPAddress,
		UserAgent:  auditCtx.UserAgent,
		RequestID:  auditCtx.RequestID,
		Timestamp:  time.Now(),
		Metadata:   metadata,
	}

	// Use a separate context with timeout for the database operation
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Insert into audit collection
	_, err := config.MongoDB.Collection(config.AppConfig.AuditLogsCollection).InsertOne(dbCtx, auditLog)
	if err != nil {
		logger.Error("failed to insert audit log", zap.Error(err))
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	logger.Info("audit event logged successfully",
		zap.String("resource_id", resourceID),
		zap.String("user_id", auditCtx.UserID))

	return nil
}

// LogAddressUpdate logs an address update audit event
func LogAddressUpdate(ctx context.Context, auditCtx AuditContext, oldAddress, newAddress interface{}) error {
	metadata := map[string]string{
		"operation": "self_declared_update",
		"field":     "address",
	}

	return LogAuditEvent(ctx, auditCtx, AuditActionUpdate, AuditResourceAddress, auditCtx.CPF, oldAddress, newAddress, metadata)
}

// LogPhoneUpdate logs a phone update audit event
func LogPhoneUpdate(ctx context.Context, auditCtx AuditContext, oldPhone, newPhone interface{}) error {
	metadata := map[string]string{
		"operation": "self_declared_update",
		"field":     "phone",
	}

	return LogAuditEvent(ctx, auditCtx, AuditActionUpdate, AuditResourcePhone, auditCtx.CPF, oldPhone, newPhone, metadata)
}

// LogPhoneVerification logs a phone verification audit event
func LogPhoneVerification(ctx context.Context, auditCtx AuditContext, phoneNumber, code string) error {
	metadata := map[string]string{
		"operation": "phone_verification",
		"phone":     phoneNumber,
		"code":      code,
	}

	return LogAuditEvent(ctx, auditCtx, AuditActionCreate, AuditResourcePhoneVerification, auditCtx.CPF, nil, map[string]string{"phone": phoneNumber}, metadata)
}

// LogPhoneVerificationSuccess logs a successful phone verification
func LogPhoneVerificationSuccess(ctx context.Context, auditCtx AuditContext, phoneNumber string) error {
	metadata := map[string]string{
		"operation": "phone_verification_success",
		"phone":     phoneNumber,
	}

	return LogAuditEvent(ctx, auditCtx, AuditActionValidate, AuditResourcePhoneVerification, auditCtx.CPF, nil, map[string]string{"phone": phoneNumber, "status": "verified"}, metadata)
}

// LogEmailUpdate logs an email update audit event
func LogEmailUpdate(ctx context.Context, auditCtx AuditContext, oldEmail, newEmail interface{}) error {
	metadata := map[string]string{
		"operation": "self_declared_update",
		"field":     "email",
	}

	return LogAuditEvent(ctx, auditCtx, AuditActionUpdate, AuditResourceEmail, auditCtx.CPF, oldEmail, newEmail, metadata)
}

// LogEthnicityUpdate logs an ethnicity update audit event
func LogEthnicityUpdate(ctx context.Context, auditCtx AuditContext, oldEthnicity, newEthnicity interface{}) error {
	metadata := map[string]string{
		"operation": "self_declared_update",
		"field":     "ethnicity",
	}

	return LogAuditEvent(ctx, auditCtx, AuditActionUpdate, AuditResourceEthnicity, auditCtx.CPF, oldEthnicity, newEthnicity, metadata)
}

// LogUserConfigUpdate logs a user config update audit event
func LogUserConfigUpdate(ctx context.Context, auditCtx AuditContext, field string, oldValue, newValue interface{}) error {
	metadata := map[string]string{
		"operation": "user_config_update",
		"field":     field,
	}

	return LogAuditEvent(ctx, auditCtx, AuditActionUpdate, AuditResourceUserConfig, auditCtx.CPF, oldValue, newValue, metadata)
}

// GetAuditContextFromRequest extracts audit context from HTTP request
func GetAuditContextFromRequest(cpf, userID, requestID string, ipAddress, userAgent string) AuditContext {
	return AuditContext{
		CPF:       cpf,
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		RequestID: requestID,
	}
}

// GetAuditContextFromGin extracts audit context from Gin context
func GetAuditContextFromGin(c *gin.Context, cpf string) AuditContext {
	userID := ""
	if user, exists := c.Get("user"); exists {
		if userMap, ok := user.(map[string]interface{}); ok {
			if id, exists := userMap["id"]; exists {
				userID = fmt.Sprintf("%v", id)
			}
		}
	}

	return AuditContext{
		CPF:       cpf,
		UserID:    userID,
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		RequestID: c.GetHeader("X-Request-ID"),
	}
}

// SanitizeAuditData removes sensitive information from audit data
func SanitizeAuditData(data interface{}) interface{} {
	if data == nil {
		return nil
	}

	// Convert to JSON and back to sanitize
	jsonData, err := json.Marshal(data)
	if err != nil {
		return data
	}

	var sanitized interface{}
	err = json.Unmarshal(jsonData, &sanitized)
	if err != nil {
		return data
	}

	// Remove sensitive fields recursively
	sanitizeMap(sanitized)

	return sanitized
}

// sanitizeMap recursively removes sensitive fields from a map
func sanitizeMap(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Remove sensitive fields
		sensitiveFields := []string{"password", "token", "secret", "key", "code"}
		for _, field := range sensitiveFields {
			if _, exists := v[field]; exists {
				v[field] = "[REDACTED]"
			}
		}

		// Recursively sanitize nested maps
		for _, value := range v {
			sanitizeMap(value)
		}

	case []interface{}:
		// Recursively sanitize array elements
		for _, item := range v {
			sanitizeMap(item)
		}
	}
}
