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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

	AuditResourceAddress              = "address"
	AuditResourcePhone                = "phone"
	AuditResourceEmail                = "email"
	AuditResourceEthnicity            = "ethnicity"
	AuditResourceExhibitionName       = "exhibition_name"
	AuditResourcePhoneVerification    = "phone_verification"
	AuditResourceUserConfig           = "user_config"
	AuditResourceBetaGroup            = "beta_group"
	AuditResourceBetaWhitelist        = "beta_whitelist"
	AuditResourcePhoneMapping         = "phone_mapping"
	AuditResourcePhoneQuarantine      = "phone_quarantine"
	AuditResourceAvatar               = "avatar"
	AuditResourceNotificationCategory = "notification_category"
	AuditResourceMemory               = "memory"
	AuditResourcePet                  = "pet"
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
	aw.wg.Add(aw.workers)

	// Start workers using the new batched processing
	for i := 0; i < aw.workers; i++ {
		go func(workerID int) {
			defer aw.wg.Done()

			// Use batched processing for better performance
			aw.processAuditLogs()
		}(i)
	}

	logging.Logger.Info("audit worker started with batched processing",
		zap.Int("workers", aw.workers),
		zap.Int("buffer_size", cap(aw.auditChan)))
}

// processAuditLogs processes audit logs in batches for better performance
func (aw *AuditWorker) processAuditLogs() {
	batchTicker := time.NewTicker(100 * time.Millisecond) // Process batches every 100ms
	monitorTicker := time.NewTicker(30 * time.Second)     // Monitor buffer every 30 seconds
	defer batchTicker.Stop()
	defer monitorTicker.Stop()

	var batch []AuditLog
	batchSize := 100 // Process in batches of 100

	for {
		select {
		case auditLog, ok := <-aw.auditChan:
			if !ok {
				// Channel closed, process remaining batch and exit
				if len(batch) > 0 {
					aw.flushBatch(batch)
				}
				return
			}
			batch = append(batch, auditLog)

			// Process batch when it reaches batchSize items
			if len(batch) >= batchSize {
				aw.flushBatch(batch)
				batch = batch[:0] // Reset slice but keep capacity
			}
		case <-batchTicker.C:
			// Process any remaining items in batch
			if len(batch) > 0 {
				aw.flushBatch(batch)
				batch = batch[:0] // Reset slice but keep capacity
			}
		case <-monitorTicker.C:
			// Monitor buffer usage less frequently
			aw.adjustAuditWorkerBuffer()
		}
	}
}

// flushBatch processes a batch of audit logs using bulk insert for better performance
func (aw *AuditWorker) flushBatch(batch []AuditLog) {
	if len(batch) == 0 {
		return
	}

	logger := logging.Logger.With(
		zap.Int("batch_size", len(batch)),
		zap.String("operation", "audit_batch_insert"),
	)

	// Use bulk insert for better performance
	var operations []mongo.WriteModel
	for _, log := range batch {
		operations = append(operations, mongo.NewInsertOneModel().SetDocument(log))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use W=0 write concern for audit logs to prevent blocking
	opts := options.BulkWrite().SetOrdered(false) // Allow parallel execution

	result, err := config.MongoDB.Collection(config.AppConfig.AuditLogsCollection).BulkWrite(ctx, operations, opts)
	if err != nil {
		logger.Error("failed to insert audit log batch",
			zap.Error(err),
			zap.Int("batch_size", len(batch)))
		return
	}

	logger.Info("audit log batch inserted successfully",
		zap.Int64("inserted", result.InsertedCount),
		zap.Int("batch_size", len(batch)))
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

// LogExhibitionNameUpdate logs an exhibition name update audit event
func LogExhibitionNameUpdate(ctx context.Context, auditCtx AuditContext, oldExhibitionName, newExhibitionName interface{}) error {
	metadata := map[string]string{
		"operation": "self_declared_update",
		"field":     "exhibition_name",
	}
	return LogAuditEvent(ctx, auditCtx, AuditActionUpdate, AuditResourceExhibitionName, auditCtx.CPF, oldExhibitionName, newExhibitionName, metadata)
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

// adjustAuditWorkerBuffer dynamically adjusts the audit worker buffer size based on current load
func (aw *AuditWorker) adjustAuditWorkerBuffer() {
	// Get current buffer usage
	currentBufferUsage := len(aw.auditChan)
	bufferCapacity := cap(aw.auditChan)

	// Calculate buffer usage percentage
	bufferUsagePercentage := float64(currentBufferUsage) / float64(bufferCapacity) * 100

	// Only log when there are actual issues or significant changes
	if bufferUsagePercentage > 80 {
		// High buffer usage - process batches more frequently
		logging.Logger.Warn("high audit buffer usage detected",
			zap.Int("current_usage", currentBufferUsage),
			zap.Int("buffer_capacity", bufferCapacity),
			zap.Float64("usage_percentage", bufferUsagePercentage))
	} else if bufferUsagePercentage > 50 {
		// Medium buffer usage - log at debug level
		logging.Logger.Debug("audit worker buffer status",
			zap.Int("current_usage", currentBufferUsage),
			zap.Int("buffer_capacity", bufferCapacity),
			zap.Float64("usage_percentage", bufferUsagePercentage))
	}
	// For low usage (< 50%), don't log anything to reduce noise
}

// GetAuditWorkerStats returns current audit worker statistics
func (aw *AuditWorker) GetAuditWorkerStats() map[string]interface{} {
	if aw == nil {
		return map[string]interface{}{
			"status": "not_initialized",
		}
	}

	return map[string]interface{}{
		"status":           "running",
		"workers":          aw.workers,
		"buffer_capacity":  cap(aw.auditChan),
		"buffer_usage":     len(aw.auditChan),
		"buffer_available": cap(aw.auditChan) - len(aw.auditChan),
	}
}
