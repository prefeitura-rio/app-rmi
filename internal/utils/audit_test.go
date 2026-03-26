package utils

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
)

func TestAuditLog_Constants(t *testing.T) {
	// Test that action constants are defined
	actions := []string{
		AuditActionCreate,
		AuditActionRead,
		AuditActionUpdate,
		AuditActionDelete,
		AuditActionValidate,
		AuditActionLogin,
		AuditActionLogout,
	}

	for _, action := range actions {
		if action == "" {
			t.Error("Audit action constant is empty")
		}
	}

	// Test that resource constants are defined
	resources := []string{
		AuditResourceAddress,
		AuditResourcePhone,
		AuditResourceEmail,
		AuditResourceEthnicity,
		AuditResourceExhibitionName,
		AuditResourcePhoneVerification,
		AuditResourceUserConfig,
		AuditResourceBetaGroup,
		AuditResourceBetaWhitelist,
		AuditResourcePhoneMapping,
		AuditResourcePhoneQuarantine,
		AuditResourceAvatar,
		AuditResourceNotificationCategory,
		AuditResourceMemory,
		AuditResourcePet,
	}

	for _, resource := range resources {
		if resource == "" {
			t.Error("Audit resource constant is empty")
		}
	}
}

func TestGetAuditContextFromRequest(t *testing.T) {
	auditCtx := GetAuditContextFromRequest("12345678901", "user123", "req123", "192.168.1.1", "Mozilla/5.0")

	if auditCtx.CPF != "12345678901" {
		t.Errorf("GetAuditContextFromRequest() CPF = %v, want 12345678901", auditCtx.CPF)
	}

	if auditCtx.UserID != "user123" {
		t.Errorf("GetAuditContextFromRequest() UserID = %v, want user123", auditCtx.UserID)
	}

	if auditCtx.RequestID != "req123" {
		t.Errorf("GetAuditContextFromRequest() RequestID = %v, want req123", auditCtx.RequestID)
	}

	if auditCtx.IPAddress != "192.168.1.1" {
		t.Errorf("GetAuditContextFromRequest() IPAddress = %v, want 192.168.1.1", auditCtx.IPAddress)
	}

	if auditCtx.UserAgent != "Mozilla/5.0" {
		t.Errorf("GetAuditContextFromRequest() UserAgent = %v, want Mozilla/5.0", auditCtx.UserAgent)
	}
}

func TestGetAuditContextFromGin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	c.Request = &http.Request{}
	c.Set("user", map[string]interface{}{
		"id": "user123",
	})

	auditCtx := GetAuditContextFromGin(c, "12345678901")

	if auditCtx.CPF != "12345678901" {
		t.Errorf("GetAuditContextFromGin() CPF = %v, want 12345678901", auditCtx.CPF)
	}

	if auditCtx.UserID != "user123" {
		t.Errorf("GetAuditContextFromGin() UserID = %v, want user123", auditCtx.UserID)
	}
}

func TestGetAuditContextFromGin_NoUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{}

	auditCtx := GetAuditContextFromGin(c, "12345678901")

	if auditCtx.UserID != "" {
		t.Errorf("GetAuditContextFromGin() UserID = %v, want empty string", auditCtx.UserID)
	}
}

func TestSanitizeAuditData_Nil(t *testing.T) {
	result := SanitizeAuditData(nil)
	if result != nil {
		t.Errorf("SanitizeAuditData(nil) = %v, want nil", result)
	}
}

func TestSanitizeAuditData_RemovesSensitiveFields(t *testing.T) {
	data := map[string]interface{}{
		"username": "user123",
		"password": "secret123",
		"token":    "abc123",
		"secret":   "mysecret",
		"key":      "mykey",
		"code":     "123456",
		"email":    "test@example.com",
	}

	result := SanitizeAuditData(data)
	resultMap := result.(map[string]interface{})

	sensitiveFields := []string{"password", "token", "secret", "key", "code"}
	for _, field := range sensitiveFields {
		if val, exists := resultMap[field]; exists {
			if val != "[REDACTED]" {
				t.Errorf("SanitizeAuditData() %s = %v, want [REDACTED]", field, val)
			}
		}
	}

	// Non-sensitive fields should be preserved
	if resultMap["username"] != "user123" {
		t.Errorf("SanitizeAuditData() username = %v, want user123", resultMap["username"])
	}

	if resultMap["email"] != "test@example.com" {
		t.Errorf("SanitizeAuditData() email = %v, want test@example.com", resultMap["email"])
	}
}

func TestSanitizeAuditData_NestedMap(t *testing.T) {
	data := map[string]interface{}{
		"user": map[string]interface{}{
			"name":     "John",
			"password": "secret",
		},
	}

	result := SanitizeAuditData(data)
	resultMap := result.(map[string]interface{})
	userMap := resultMap["user"].(map[string]interface{})

	if userMap["password"] != "[REDACTED]" {
		t.Errorf("SanitizeAuditData() nested password = %v, want [REDACTED]", userMap["password"])
	}

	if userMap["name"] != "John" {
		t.Errorf("SanitizeAuditData() nested name = %v, want John", userMap["name"])
	}
}

func TestSanitizeAuditData_Array(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{
			"name":     "User1",
			"password": "secret1",
		},
		map[string]interface{}{
			"name":     "User2",
			"password": "secret2",
		},
	}

	result := SanitizeAuditData(data)
	resultArray := result.([]interface{})

	for i, item := range resultArray {
		itemMap := item.(map[string]interface{})
		if itemMap["password"] != "[REDACTED]" {
			t.Errorf("SanitizeAuditData() array[%d] password = %v, want [REDACTED]", i, itemMap["password"])
		}
	}
}

func TestLogAuditEvent_DisabledConfig(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	originalEnabled := config.AppConfig.AuditLogsEnabled
	config.AppConfig.AuditLogsEnabled = false
	defer func() {
		config.AppConfig.AuditLogsEnabled = originalEnabled
	}()

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	err := LogAuditEvent(ctx, auditCtx, AuditActionCreate, AuditResourceAddress, "res123", nil, nil, nil)
	if err != nil {
		t.Errorf("LogAuditEvent() with disabled config error = %v, want nil", err)
	}
}

func TestAuditWorker_GetAuditWorkerStats_NotInitialized(t *testing.T) {
	var aw *AuditWorker = nil

	stats := aw.GetAuditWorkerStats()

	if stats["status"] != "not_initialized" {
		t.Errorf("GetAuditWorkerStats() status = %v, want not_initialized", stats["status"])
	}
}

func TestGetAuditWorker_BeforeInit(t *testing.T) {
	// Reset global instance
	once = sync.Once{}
	auditWorker = nil

	worker := GetAuditWorker()
	if worker != nil {
		// If already initialized, that's fine
		t.Logf("GetAuditWorker() returned initialized worker")
	}
}

func TestLogAddressUpdate(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsEnabled = false

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	oldAddress := map[string]string{"street": "Old Street"}
	newAddress := map[string]string{"street": "New Street"}

	err := LogAddressUpdate(ctx, auditCtx, oldAddress, newAddress)
	if err != nil {
		t.Errorf("LogAddressUpdate() error = %v, want nil", err)
	}
}

func TestLogPhoneUpdate(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsEnabled = false

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	err := LogPhoneUpdate(ctx, auditCtx, "11999999999", "11888888888")
	if err != nil {
		t.Errorf("LogPhoneUpdate() error = %v, want nil", err)
	}
}

func TestLogPhoneVerification(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsEnabled = false

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	err := LogPhoneVerification(ctx, auditCtx, "11999999999", "123456")
	if err != nil {
		t.Errorf("LogPhoneVerification() error = %v, want nil", err)
	}
}

func TestLogPhoneVerificationSuccess(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsEnabled = false

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	err := LogPhoneVerificationSuccess(ctx, auditCtx, "11999999999")
	if err != nil {
		t.Errorf("LogPhoneVerificationSuccess() error = %v, want nil", err)
	}
}

func TestLogEmailUpdate(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsEnabled = false

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	err := LogEmailUpdate(ctx, auditCtx, "old@example.com", "new@example.com")
	if err != nil {
		t.Errorf("LogEmailUpdate() error = %v, want nil", err)
	}
}

func TestLogEthnicityUpdate(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsEnabled = false

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	err := LogEthnicityUpdate(ctx, auditCtx, "parda", "branca")
	if err != nil {
		t.Errorf("LogEthnicityUpdate() error = %v, want nil", err)
	}
}

func TestLogExhibitionNameUpdate(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsEnabled = false

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	err := LogExhibitionNameUpdate(ctx, auditCtx, "Old Name", "New Name")
	if err != nil {
		t.Errorf("LogExhibitionNameUpdate() error = %v, want nil", err)
	}
}

func TestLogUserConfigUpdate(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsEnabled = false

	ctx := context.Background()
	auditCtx := AuditContext{
		CPF:    "12345678901",
		UserID: "user123",
	}

	err := LogUserConfigUpdate(ctx, auditCtx, "theme", "light", "dark")
	if err != nil {
		t.Errorf("LogUserConfigUpdate() error = %v, want nil", err)
	}
}

func TestAuditContext_AllFields(t *testing.T) {
	auditCtx := AuditContext{
		CPF:       "12345678901",
		UserID:    "user123",
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
		RequestID: "req123",
	}

	if auditCtx.CPF == "" {
		t.Error("AuditContext CPF is empty")
	}

	if auditCtx.UserID == "" {
		t.Error("AuditContext UserID is empty")
	}

	if auditCtx.IPAddress == "" {
		t.Error("AuditContext IPAddress is empty")
	}

	if auditCtx.UserAgent == "" {
		t.Error("AuditContext UserAgent is empty")
	}

	if auditCtx.RequestID == "" {
		t.Error("AuditContext RequestID is empty")
	}
}

func TestAuditLog_Fields(t *testing.T) {
	now := time.Now()
	auditLog := AuditLog{
		CPF:        "12345678901",
		Action:     AuditActionCreate,
		Resource:   AuditResourceAddress,
		ResourceID: "res123",
		OldValue:   "old",
		NewValue:   "new",
		UserID:     "user123",
		IPAddress:  "192.168.1.1",
		UserAgent:  "Mozilla/5.0",
		RequestID:  "req123",
		Timestamp:  now,
		Metadata: map[string]string{
			"key": "value",
		},
	}

	if auditLog.CPF != "12345678901" {
		t.Errorf("AuditLog CPF = %v, want 12345678901", auditLog.CPF)
	}

	if auditLog.Action != AuditActionCreate {
		t.Errorf("AuditLog Action = %v, want %v", auditLog.Action, AuditActionCreate)
	}

	if auditLog.Resource != AuditResourceAddress {
		t.Errorf("AuditLog Resource = %v, want %v", auditLog.Resource, AuditResourceAddress)
	}

	if auditLog.Metadata["key"] != "value" {
		t.Errorf("AuditLog Metadata[key] = %v, want value", auditLog.Metadata["key"])
	}
}

func TestSanitizeAuditData_ComplexNesting(t *testing.T) {
	data := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": map[string]interface{}{
					"password": "secret",
					"name":     "test",
				},
			},
		},
	}

	result := SanitizeAuditData(data)
	resultMap := result.(map[string]interface{})
	level1 := resultMap["level1"].(map[string]interface{})
	level2 := level1["level2"].(map[string]interface{})
	level3 := level2["level3"].(map[string]interface{})

	if level3["password"] != "[REDACTED]" {
		t.Errorf("SanitizeAuditData() deeply nested password = %v, want [REDACTED]", level3["password"])
	}

	if level3["name"] != "test" {
		t.Errorf("SanitizeAuditData() deeply nested name = %v, want test", level3["name"])
	}
}

func TestSanitizeAuditData_MixedArrayAndMap(t *testing.T) {
	data := map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{
				"credentials": map[string]interface{}{
					"token": "abc123",
					"email": "user@example.com",
				},
			},
		},
	}

	result := SanitizeAuditData(data)
	resultMap := result.(map[string]interface{})
	users := resultMap["users"].([]interface{})
	user := users[0].(map[string]interface{})
	creds := user["credentials"].(map[string]interface{})

	if creds["token"] != "[REDACTED]" {
		t.Errorf("SanitizeAuditData() nested token = %v, want [REDACTED]", creds["token"])
	}

	if creds["email"] != "user@example.com" {
		t.Errorf("SanitizeAuditData() nested email = %v, want user@example.com", creds["email"])
	}
}
