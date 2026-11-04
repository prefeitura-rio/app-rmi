package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// Synthetic CPFs with valid check digits: the token owner and a third party.
const (
	accessOwnerCPF = "52998224725"
	accessOtherCPF = "11144477735"
	accessPhone    = "+5521988776655"
)

func claimsMiddleware(claims *models.JWTClaims) gin.HandlerFunc {
	return func(c *gin.Context) {
		if claims != nil {
			c.Set("claims", claims)
		}
		c.Next()
	}
}

func ownerClaims() *models.JWTClaims {
	return &models.JWTClaims{PreferredUsername: accessOwnerCPF}
}

func adminClaims() *models.JWTClaims {
	claims := &models.JWTClaims{PreferredUsername: "admin_user"}
	claims.RealmAccess.Roles = []string{"go:admin"}
	return claims
}

// setupPhoneAccessRouter registers the protected phone routes behind a middleware
// that authenticates the request with the given claims.
func setupPhoneAccessRouter(t *testing.T, claims *models.JWTClaims) (*gin.Engine, func()) {
	handlers, _, cleanup := setupPhoneHandlersTest(t)

	router := gin.New()
	group := router.Group("")
	group.Use(claimsMiddleware(claims))
	group.GET("/phone/:phone_number/citizen", handlers.GetCitizenByPhone)
	group.POST("/phone/:phone_number/validate-registration", handlers.ValidateRegistration)
	group.POST("/phone/:phone_number/opt-in", handlers.OptIn)
	group.POST("/phone/:phone_number/opt-out", handlers.OptOut)
	group.POST("/phone/:phone_number/reject-registration", handlers.RejectRegistration)
	group.POST("/phone/:phone_number/bind", handlers.BindPhoneToCPF)

	return router, cleanup
}

func doPhoneRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reader *bytes.Buffer
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewBuffer(raw)
	} else {
		reader = bytes.NewBuffer(nil)
	}
	req, err := http.NewRequest(method, path, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func storedMapping(t *testing.T, phoneNumber string) (*models.PhoneCPFMapping, bool) {
	components, err := utils.ParsePhoneNumber(phoneNumber)
	require.NoError(t, err)
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).
		FindOne(context.Background(), bson.M{"phone_number": storagePhone}).Decode(&mapping)
	if err != nil {
		return nil, false
	}
	return &mapping, true
}

func optInHistoryCount(t *testing.T) int64 {
	count, err := config.MongoDB.Collection(config.AppConfig.OptInHistoryCollection).
		CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err)
	return count
}

func TestValidateCPFAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &PhoneHandlers{}

	previousAdminGroup := config.AppConfig.AdminGroup
	previousTrusted := config.AppConfig.TrustedServiceClients
	config.AppConfig.AdminGroup = "go:admin"
	config.AppConfig.TrustedServiceClients = []string{"trusted-service"}
	t.Cleanup(func() {
		config.AppConfig.AdminGroup = previousAdminGroup
		config.AppConfig.TrustedServiceClients = previousTrusted
	})

	tests := []struct {
		name      string
		claims    interface{}
		targetCPF string
		allowed   bool
	}{
		{name: "no claims", claims: nil, targetCPF: accessOwnerCPF, allowed: false},
		{name: "invalid claims type", claims: "not-claims", targetCPF: accessOwnerCPF, allowed: false},
		{name: "own CPF", claims: ownerClaims(), targetCPF: accessOwnerCPF, allowed: true},
		{name: "own CPF formatted", claims: ownerClaims(), targetCPF: "529.982.247-25", allowed: true},
		{name: "third party CPF", claims: ownerClaims(), targetCPF: accessOtherCPF, allowed: false},
		{name: "empty target CPF", claims: ownerClaims(), targetCPF: "", allowed: false},
		{name: "token without CPF", claims: &models.JWTClaims{PreferredUsername: "service-account-x"}, targetCPF: "", allowed: false},
		{name: "admin role on any CPF", claims: adminClaims(), targetCPF: accessOtherCPF, allowed: true},
		{name: "trusted service client on any CPF", claims: &models.JWTClaims{AZP: "trusted-service"}, targetCPF: accessOtherCPF, allowed: true},
		{name: "untrusted client on third party CPF", claims: &models.JWTClaims{AZP: "superapp-like", PreferredUsername: accessOwnerCPF}, targetCPF: accessOtherCPF, allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			if tt.claims != nil {
				c.Set("claims", tt.claims)
			}
			err := h.validateCPFAccess(c, tt.targetCPF)
			if tt.allowed {
				assert.NoError(t, err)
			} else {
				assert.True(t, errors.Is(err, middleware.ErrAccessDenied), "expected access denied, got %v", err)
			}
		})
	}
}

func TestPhoneAccess_BindThirdPartyCPF_Forbidden(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/bind",
		models.BindRequest{CPF: accessOtherCPF, Channel: models.ChannelWhatsApp})

	assert.Equal(t, http.StatusForbidden, w.Code)
	_, found := storedMapping(t, accessPhone)
	assert.False(t, found, "no mapping may be created for a third party CPF")
}

func TestPhoneAccess_BindOwnCPF_Allowed(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/bind",
		models.BindRequest{CPF: accessOwnerCPF, Channel: models.ChannelWhatsApp})

	assert.Equal(t, http.StatusOK, w.Code)
	mapping, found := storedMapping(t, accessPhone)
	require.True(t, found)
	assert.Equal(t, accessOwnerCPF, mapping.CPF)
}

func TestPhoneAccess_BindThirdPartyCPF_AdminAllowed(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, adminClaims())
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/bind",
		models.BindRequest{CPF: accessOtherCPF, Channel: models.ChannelWhatsApp})

	assert.Equal(t, http.StatusOK, w.Code)
	mapping, found := storedMapping(t, accessPhone)
	require.True(t, found)
	assert.Equal(t, accessOtherCPF, mapping.CPF)
}

func TestPhoneAccess_NoClaims_Forbidden(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, nil)
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/bind",
		models.BindRequest{CPF: accessOwnerCPF, Channel: models.ChannelWhatsApp})

	assert.Equal(t, http.StatusForbidden, w.Code)
	_, found := storedMapping(t, accessPhone)
	assert.False(t, found)
}

func TestPhoneAccess_OptInThirdPartyCPF_Forbidden(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/opt-in",
		models.OptInRequest{CPF: accessOtherCPF, Channel: models.ChannelWhatsApp})

	assert.Equal(t, http.StatusForbidden, w.Code)
	_, found := storedMapping(t, accessPhone)
	assert.False(t, found, "no mapping may be created for a third party CPF")
	assert.Equal(t, int64(0), optInHistoryCount(t), "no opt-in history may be recorded")
}

func TestPhoneAccess_OptInOwnCPF_Allowed(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/opt-in",
		models.OptInRequest{CPF: accessOwnerCPF, Channel: models.ChannelWhatsApp})

	assert.Equal(t, http.StatusOK, w.Code)
	mapping, found := storedMapping(t, accessPhone)
	require.True(t, found)
	assert.Equal(t, accessOwnerCPF, mapping.CPF)
}

func TestPhoneAccess_RejectThirdPartyCPF_Forbidden(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()
	insertTestPhoneMapping(t, accessPhone, accessOtherCPF, true, models.MappingStatusActive)

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/reject-registration",
		models.RejectRegistrationRequest{CPF: accessOtherCPF, Channel: models.ChannelWhatsApp, Reason: "x"})

	assert.Equal(t, http.StatusForbidden, w.Code)
	mapping, found := storedMapping(t, accessPhone)
	require.True(t, found)
	assert.Equal(t, models.MappingStatusActive, mapping.Status, "third party mapping must stay active")
}

func TestPhoneAccess_RejectOwnCPFOnThirdPartyPhone_Forbidden(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()
	insertTestPhoneMapping(t, accessPhone, accessOtherCPF, true, models.MappingStatusActive)

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/reject-registration",
		models.RejectRegistrationRequest{CPF: accessOwnerCPF, Channel: models.ChannelWhatsApp, Reason: "x"})

	assert.Equal(t, http.StatusForbidden, w.Code)
	mapping, found := storedMapping(t, accessPhone)
	require.True(t, found)
	assert.Equal(t, models.MappingStatusActive, mapping.Status, "third party mapping must stay active")
	assert.Equal(t, int64(0), optInHistoryCount(t), "no rejection may be recorded")
}

func TestPhoneAccess_RejectOwnPhone_Allowed(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()
	insertTestPhoneMapping(t, accessPhone, accessOwnerCPF, false, models.MappingStatusActive)

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/reject-registration",
		models.RejectRegistrationRequest{CPF: accessOwnerCPF, Channel: models.ChannelWhatsApp, Reason: "x"})

	assert.Equal(t, http.StatusOK, w.Code)
	mapping, found := storedMapping(t, accessPhone)
	require.True(t, found)
	assert.Equal(t, models.MappingStatusBlocked, mapping.Status)
}

func TestPhoneAccess_OptOutThirdPartyPhone_Forbidden(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()
	insertTestPhoneMapping(t, accessPhone, accessOtherCPF, true, models.MappingStatusActive)

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/opt-out",
		models.OptOutRequest{Channel: models.ChannelWhatsApp, Reason: "user_request"})

	assert.Equal(t, http.StatusForbidden, w.Code)
	mapping, found := storedMapping(t, accessPhone)
	require.True(t, found)
	assert.Equal(t, models.MappingStatusActive, mapping.Status, "third party mapping must stay active")
	assert.Equal(t, int64(0), optInHistoryCount(t), "no opt-out may be recorded")
}

func TestPhoneAccess_OptOutUnboundPhone_ForbiddenForCitizen(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/opt-out",
		models.OptOutRequest{Channel: models.ChannelWhatsApp, Reason: "user_request"})

	assert.Equal(t, http.StatusForbidden, w.Code)
	_, found := storedMapping(t, accessPhone)
	assert.False(t, found, "a citizen may not create a blocked record for a phone that is not theirs")
}

func TestPhoneAccess_OptOutUnboundPhone_AdminAllowed(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, adminClaims())
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/opt-out",
		models.OptOutRequest{Channel: models.ChannelWhatsApp, Reason: "user_request"})

	assert.Equal(t, http.StatusOK, w.Code)
	mapping, found := storedMapping(t, accessPhone)
	require.True(t, found)
	assert.Equal(t, models.MappingStatusBlocked, mapping.Status)
}

func TestPhoneAccess_OptOutOwnPhone_Allowed(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()
	insertTestPhoneMapping(t, accessPhone, accessOwnerCPF, true, models.MappingStatusActive)

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/opt-out",
		models.OptOutRequest{Channel: models.ChannelWhatsApp, Reason: "user_request"})

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPhoneAccess_OptOutInvalidPhone_BadRequest(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/invalid-phone/opt-out",
		models.OptOutRequest{Channel: models.ChannelWhatsApp, Reason: "user_request"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPhoneAccess_ValidateRegistrationThirdPartyCPF_Forbidden(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()
	insertTestCitizen(t, accessOtherCPF, "Pessoa Terceira", "1990-01-01")

	w := doPhoneRequest(t, router, http.MethodPost, "/phone/"+accessPhone+"/validate-registration",
		models.ValidateRegistrationRequest{Name: "Pessoa Terceira", CPF: accessOtherCPF, BirthDate: "1990-01-01", Channel: models.ChannelWhatsApp})

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NotContains(t, w.Body.String(), accessOtherCPF)
}

func TestPhoneAccess_GetCitizenByThirdPartyPhone_Forbidden(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()
	insertTestPhoneMapping(t, accessPhone, accessOtherCPF, true, models.MappingStatusActive)
	insertTestCitizen(t, accessOtherCPF, "Pessoa Terceira", "1990-01-01")

	w := doPhoneRequest(t, router, http.MethodGet, "/phone/"+accessPhone+"/citizen", nil)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NotContains(t, w.Body.String(), "Pessoa")
}

func TestPhoneAccess_GetCitizenByOwnPhone_Allowed(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, ownerClaims())
	defer cleanup()
	insertTestPhoneMapping(t, accessPhone, accessOwnerCPF, true, models.MappingStatusActive)
	insertTestCitizen(t, accessOwnerCPF, "Pessoa Dona", "1990-01-01")

	w := doPhoneRequest(t, router, http.MethodGet, "/phone/"+accessPhone+"/citizen", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	var response models.PhoneCitizenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Found)
}

func TestPhoneAccess_GetCitizenByThirdPartyPhone_AdminAllowed(t *testing.T) {
	router, cleanup := setupPhoneAccessRouter(t, adminClaims())
	defer cleanup()
	insertTestPhoneMapping(t, accessPhone, accessOtherCPF, true, models.MappingStatusActive)
	insertTestCitizen(t, accessOtherCPF, "Pessoa Terceira", "1990-01-01")

	w := doPhoneRequest(t, router, http.MethodGet, "/phone/"+accessPhone+"/citizen", nil)

	assert.Equal(t, http.StatusOK, w.Code)
}
