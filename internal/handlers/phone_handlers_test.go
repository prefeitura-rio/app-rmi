package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

// setupPhoneHandlersTest initializes MongoDB, Redis, and creates a test router
func setupPhoneHandlersTest(t *testing.T) (*PhoneHandlers, *gin.Engine, func()) {
	// Use the shared MongoDB from common_test.go TestMain
	gin.SetMode(gin.TestMode)

	// Configure test collections
	config.AppConfig.PhoneMappingCollection = "test_phone_mappings"
	config.AppConfig.CitizenCollection = "test_citizens"
	config.AppConfig.OptInHistoryCollection = "test_opt_in_history"
	config.AppConfig.AdminGroup = "go:admin"

	ctx := context.Background()
	database := config.MongoDB

	// Initialize services
	phoneMappingService := services.NewPhoneMappingService(logging.Logger)
	configService := services.NewConfigService()
	handlers := NewPhoneHandlers(logging.Logger, phoneMappingService, configService)

	router := gin.New()

	// Create admin middleware for testing
	adminMiddleware := func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "03561350712",
		}
		claims.RealmAccess.Roles = []string{"go:admin"}
		c.Set("claims", claims)
		c.Next()
	}

	// Create regular user middleware for testing
	userMiddleware := func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "03561350712",
		}
		c.Set("claims", claims)
		c.Next()
	}

	// Public routes
	router.GET("/phone/:phone_number/status", handlers.GetPhoneStatus)

	// Protected routes (require authentication)
	protected := router.Group("")
	protected.Use(userMiddleware)
	{
		protected.GET("/phone/:phone_number/citizen", handlers.GetCitizenByPhone)
		protected.POST("/phone/:phone_number/validate-registration", handlers.ValidateRegistration)
		protected.POST("/phone/:phone_number/opt-in", handlers.OptIn)
		protected.POST("/phone/:phone_number/opt-out", handlers.OptOut)
		protected.POST("/phone/:phone_number/reject-registration", handlers.RejectRegistration)
		protected.POST("/phone/:phone_number/bind", handlers.BindPhoneToCPF)
	}

	// Admin routes
	admin := router.Group("")
	admin.Use(adminMiddleware)
	{
		admin.POST("/phone/:phone_number/quarantine", handlers.QuarantinePhone)
		admin.DELETE("/phone/:phone_number/quarantine", handlers.ReleaseQuarantine)
		admin.GET("/admin/phone/quarantined", handlers.GetQuarantinedPhones)
		admin.GET("/admin/phone/quarantine/stats", handlers.GetQuarantineStats)
	}

	// Config routes
	router.GET("/config/channels", handlers.GetAvailableChannels)
	router.GET("/config/opt-out-reasons", handlers.GetOptOutReasons)

	// Cleanup function to drop test collections
	cleanup := func() {
		// Drop test collections used by this test file
		_ = database.Collection(config.AppConfig.PhoneMappingCollection).Drop(ctx)
		_ = database.Collection(config.AppConfig.CitizenCollection).Drop(ctx)
		_ = database.Collection(config.AppConfig.OptInHistoryCollection).Drop(ctx)
	}

	// Clean up at start to ensure fresh state
	cleanup()

	return handlers, router, cleanup
}

// insertTestPhoneMapping inserts a test phone mapping into MongoDB
func insertTestPhoneMapping(t *testing.T, phoneNumber, cpf string, optIn bool, status string) {
	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)

	// Parse phone number and convert to storage format (without + prefix)
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		t.Fatalf("Failed to parse phone number: %v", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	now := time.Now()
	mapping := models.PhoneCPFMapping{
		PhoneNumber: storagePhone,
		CPF:         cpf,
		Status:      status,
		OptIn:       optIn,
		Channel:     models.ChannelWhatsApp,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	_, err = collection.InsertOne(ctx, mapping)
	if err != nil {
		t.Fatalf("Failed to insert phone mapping: %v", err)
	}
}

// insertTestCitizen inserts a test citizen into MongoDB
func insertTestCitizen(t *testing.T, cpf, name, birthDate string) {
	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)

	// Parse birth date
	parsedDate, err := time.Parse("2006-01-02", birthDate)
	if err != nil {
		t.Fatalf("Failed to parse birth date: %v", err)
	}

	citizen := bson.M{
		"_id":  cpf,
		"cpf":  cpf,
		"nome": name,
		"nascimento": bson.M{
			"data": parsedDate,
		},
	}

	_, err = collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert citizen: %v", err)
	}
}

// TestNewPhoneHandlers tests the constructor
func TestNewPhoneHandlers(t *testing.T) {
	handlers := NewPhoneHandlers(logging.Logger, nil, nil)
	assert.NotNil(t, handlers)
	assert.NotNil(t, handlers.logger)
}

// TestGetPhoneStatus_NotFound tests getting status for non-existent phone
func TestGetPhoneStatus_NotFound(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/phone/+5521999887766/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PhoneStatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response.Found)
	assert.False(t, response.Quarantined)
	assert.False(t, response.OptIn)
}

// TestGetPhoneStatus_Found tests getting status for existing phone
func TestGetPhoneStatus_Found(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test data
	insertTestPhoneMapping(t, "+5521999887766", "03561350712", true, models.MappingStatusActive)
	insertTestCitizen(t, "03561350712", "João da Silva", "1990-01-01")

	req, _ := http.NewRequest("GET", "/phone/+5521999887766/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PhoneStatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Found)
	assert.Equal(t, "035***50712", response.CPF)    // Masked CPF
	assert.Equal(t, "João d* Silva", response.Name) // Masked name (masks middle names)
	assert.True(t, response.OptIn)
	assert.False(t, response.Quarantined)
}

// TestGetPhoneStatus_InvalidPhone tests getting status with invalid phone number
func TestGetPhoneStatus_InvalidPhone(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/phone/invalid/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetCitizenByPhone_Success tests getting citizen by phone number
func TestGetCitizenByPhone_Success(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test data
	insertTestPhoneMapping(t, "+5521999887766", "03561350712", true, models.MappingStatusActive)
	insertTestCitizen(t, "03561350712", "João da Silva", "1990-01-01")

	req, _ := http.NewRequest("GET", "/phone/+5521999887766/citizen", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PhoneCitizenResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Found)
	assert.Equal(t, "035***50712", response.CPF)    // Masked CPF
	assert.Equal(t, "João d* Silva", response.Name) // Masked name
	assert.Equal(t, "João", response.FirstName)     // First name is not masked
}

// TestGetCitizenByPhone_NotFound tests getting citizen for non-existent phone
func TestGetCitizenByPhone_NotFound(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/phone/+5521999887766/citizen", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PhoneCitizenResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response.Found)
}

// TestValidateRegistration_Success tests successful registration validation
func TestValidateRegistration_Success(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test citizen
	insertTestCitizen(t, "03561350712", "João da Silva", "1990-01-01")

	reqBody := models.ValidateRegistrationRequest{
		Name:      "João da Silva",
		CPF:       "03561350712",
		BirthDate: "1990-01-01",
		Channel:   models.ChannelWhatsApp,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/phone/+5521999887766/validate-registration", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ValidateRegistrationResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Valid)
	assert.Equal(t, "03561350712", response.MatchedCPF)
}

// TestValidateRegistration_InvalidData tests validation with invalid data
func TestValidateRegistration_InvalidData(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test citizen
	insertTestCitizen(t, "03561350712", "João da Silva", "1990-01-01")

	reqBody := models.ValidateRegistrationRequest{
		Name:      "Wrong Name",
		CPF:       "03561350712",
		BirthDate: "1999-12-31",
		Channel:   models.ChannelWhatsApp,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/phone/+5521999887766/validate-registration", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ValidateRegistrationResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response.Valid)
}

// TestValidateRegistration_InvalidJSON tests validation with invalid JSON
func TestValidateRegistration_InvalidJSON(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/phone/+5521999887766/validate-registration", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestOptIn_Success tests successful opt-in
func TestOptIn_Success(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test citizen
	insertTestCitizen(t, "03561350712", "João da Silva", "1990-01-01")

	reqBody := models.OptInRequest{
		CPF:     "03561350712",
		Channel: models.ChannelWhatsApp,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/phone/+5521999887766/opt-in", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.OptInResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.Status)
	assert.NotEmpty(t, response.PhoneMappingID)
}

// TestOptIn_InvalidJSON tests opt-in with invalid JSON
func TestOptIn_InvalidJSON(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/phone/+5521999887766/opt-in", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestOptOut_Success tests successful opt-out
func TestOptOut_Success(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test data with opt-in
	insertTestPhoneMapping(t, "+5521999887766", "03561350712", true, models.MappingStatusActive)

	reqBody := models.OptOutRequest{
		Channel: models.ChannelWhatsApp,
		Reason:  "user_request",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/phone/+5521999887766/opt-out", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.OptOutResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.Status)
}

// TestOptOut_InvalidJSON tests opt-out with invalid JSON
func TestOptOut_InvalidJSON(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/phone/+5521999887766/opt-out", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestRejectRegistration_Success tests successful registration rejection
func TestRejectRegistration_Success(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test data
	insertTestPhoneMapping(t, "+5521999887766", "03561350712", false, models.MappingStatusActive)

	reqBody := models.RejectRegistrationRequest{
		CPF:     "03561350712",
		Channel: models.ChannelWhatsApp,
		Reason:  "fraud_suspected",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/phone/+5521999887766/reject-registration", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.RejectRegistrationResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.Status)
}

// TestRejectRegistration_InvalidJSON tests rejection with invalid JSON
func TestRejectRegistration_InvalidJSON(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/phone/+5521999887766/reject-registration", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestBindPhoneToCPF_Success tests successful phone binding
func TestBindPhoneToCPF_Success(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test citizen
	insertTestCitizen(t, "03561350712", "João da Silva", "1990-01-01")

	reqBody := models.BindRequest{
		CPF:     "03561350712",
		Channel: models.ChannelWhatsApp,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/phone/+5521999887766/bind", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.BindResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.Status)
	assert.Equal(t, "03561350712", response.CPF)
	assert.False(t, response.OptIn) // Bind doesn't set opt-in
}

// TestBindPhoneToCPF_InvalidJSON tests binding with invalid JSON
func TestBindPhoneToCPF_InvalidJSON(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/phone/+5521999887766/bind", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuarantinePhone_Success tests successful phone quarantine (admin)
func TestQuarantinePhone_Success(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test data
	insertTestPhoneMapping(t, "+5521999887766", "03561350712", true, models.MappingStatusActive)

	req, _ := http.NewRequest("POST", "/phone/+5521999887766/quarantine", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.QuarantineResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.Status)
	assert.False(t, response.QuarantineUntil.IsZero())
}

// TestQuarantinePhone_EmptyPhoneNumber tests quarantine with empty phone
func TestQuarantinePhone_EmptyPhoneNumber(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/phone//quarantine", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Handler validates phone number and returns 400 for empty/invalid phone
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestReleaseQuarantine_Success tests successful quarantine release (admin)
func TestReleaseQuarantine_Success(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert test data with quarantine using storage format
	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	quarantineUntil := time.Now().Add(24 * time.Hour)
	now := time.Now()

	// Convert phone number to storage format
	components, err := utils.ParsePhoneNumber("+5521999887766")
	assert.NoError(t, err)
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	mapping := models.PhoneCPFMapping{
		PhoneNumber:     storagePhone,
		CPF:             "03561350712",
		Status:          models.MappingStatusQuarantined,
		OptIn:           false,
		QuarantineUntil: &quarantineUntil,
		Channel:         models.ChannelWhatsApp,
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}

	_, err = collection.InsertOne(ctx, mapping)
	assert.NoError(t, err)

	req, _ := http.NewRequest("DELETE", "/phone/+5521999887766/quarantine", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.QuarantineResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.Status)
}

// TestGetQuarantinedPhones_Empty tests getting quarantined phones when none exist
func TestGetQuarantinedPhones_Empty(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/admin/phone/quarantined?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.QuarantinedListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 0, response.Pagination.Total)
	assert.Empty(t, response.Data)
}

// TestGetQuarantinedPhones_WithData tests getting quarantined phones with data
func TestGetQuarantinedPhones_WithData(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert quarantined phones
	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	quarantineUntil := time.Now().Add(24 * time.Hour)
	now := time.Now()

	phones := []interface{}{
		models.PhoneCPFMapping{
			PhoneNumber:     "+5521999887766",
			CPF:             "03561350712",
			Status:          models.MappingStatusQuarantined,
			QuarantineUntil: &quarantineUntil,
			Channel:         models.ChannelWhatsApp,
			CreatedAt:       &now,
			UpdatedAt:       &now,
		},
		models.PhoneCPFMapping{
			PhoneNumber:     "+5521988776655",
			CPF:             "03561350712",
			Status:          models.MappingStatusQuarantined,
			QuarantineUntil: &quarantineUntil,
			Channel:         models.ChannelWhatsApp,
			CreatedAt:       &now,
			UpdatedAt:       &now,
		},
	}

	_, err := collection.InsertMany(ctx, phones)
	assert.NoError(t, err)

	req, _ := http.NewRequest("GET", "/admin/phone/quarantined?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.QuarantinedListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.Pagination.Total)
	assert.Len(t, response.Data, 2)
}

// TestGetQuarantinedPhones_Pagination tests pagination parameters
func TestGetQuarantinedPhones_Pagination(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/admin/phone/quarantined?page=2&per_page=5", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.QuarantinedListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.Pagination.Page)
	assert.Equal(t, 5, response.Pagination.PerPage)
}

// TestGetQuarantineStats_Empty tests getting stats when no quarantines exist
func TestGetQuarantineStats_Empty(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/admin/phone/quarantine/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.QuarantineStats
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 0, response.TotalQuarantined)
	assert.Equal(t, 0, response.ActiveQuarantines)
	assert.Equal(t, 0, response.ExpiredQuarantines)
}

// TestGetQuarantineStats_WithData tests getting stats with quarantined phones
func TestGetQuarantineStats_WithData(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	// Insert quarantined phones
	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	now := time.Now()
	futureQuarantine := now.Add(24 * time.Hour)
	expiredQuarantine := now.Add(-24 * time.Hour)

	phones := []interface{}{
		models.PhoneCPFMapping{
			PhoneNumber:     "+5521999887766",
			CPF:             "03561350712",
			Status:          models.MappingStatusQuarantined,
			QuarantineUntil: &futureQuarantine,
			Channel:         models.ChannelWhatsApp,
			CreatedAt:       &now,
			UpdatedAt:       &now,
		},
		models.PhoneCPFMapping{
			PhoneNumber:     "+5521988776655",
			CPF:             "03561350712",
			Status:          models.MappingStatusQuarantined,
			QuarantineUntil: &expiredQuarantine,
			Channel:         models.ChannelWhatsApp,
			CreatedAt:       &now,
			UpdatedAt:       &now,
		},
	}

	_, err := collection.InsertMany(ctx, phones)
	assert.NoError(t, err)

	req, _ := http.NewRequest("GET", "/admin/phone/quarantine/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.QuarantineStats
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.TotalQuarantined)
	assert.Equal(t, 1, response.ActiveQuarantines)
	assert.Equal(t, 1, response.ExpiredQuarantines)
}

// TestGetAvailableChannels tests getting available communication channels
func TestGetAvailableChannels(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/config/channels", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ChannelsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.Channels)
}

// TestGetOptOutReasons tests getting available opt-out reasons
func TestGetOptOutReasons(t *testing.T) {
	_, router, cleanup := setupPhoneHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/config/opt-out-reasons", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.OptOutReasonsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response.Reasons)
}

// TestIsPhoneParsingError tests the error detection function
func TestIsPhoneParsingError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "invalid phone number",
			err:      assert.AnError,
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPhoneParsingError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
