package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

func setupPhoneRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/citizen/:cpf/phone/validate", ValidatePhoneVerification)
	return r
}

func TestValidatePhoneVerificationEndpoint(t *testing.T) {
	r := setupPhoneRouter()
	cpf := "03561350712"

	// Invalid body
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/citizen/"+cpf+"/phone/validate", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Missing code/phone (simulate not found)
	w2 := httptest.NewRecorder()
	body := []byte(`{"code":"000000","ddi":"55","ddd":"21","valor":"999999999"}`)
	req2, _ := http.NewRequest("POST", "/v1/citizen/"+cpf+"/phone/validate", bytes.NewBuffer(body))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	assert.Contains(t, []int{http.StatusNotFound, http.StatusInternalServerError}, w2.Code)
}

func TestValidatePhoneVerification_InvalidCPF(t *testing.T) {
	r := setupPhoneRouter()

	tests := []struct {
		name           string
		cpf            string
		expectedStatus int
	}{
		{"empty CPF", "", http.StatusBadRequest}, // Empty CPF fails validation
		{"short CPF", "123", http.StatusBadRequest},
		{"letters in CPF", "abcdefghijk", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"code":"123456","ddi":"55","ddd":"21","valor":"999999999"}`)
			req, _ := http.NewRequest("POST", "/v1/citizen/"+tt.cpf+"/phone/validate", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestValidatePhoneVerification_MalformedJSON(t *testing.T) {
	r := setupPhoneRouter()

	req, _ := http.NewRequest("POST", "/v1/citizen/03561350712/phone/validate", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestValidatePhoneVerification_MissingFields(t *testing.T) {
	r := setupPhoneRouter()
	cpf := "03561350712"

	tests := []struct {
		name string
		body string
	}{
		{"missing code", `{"ddi":"55","ddd":"21","valor":"999999999"}`},
		{"missing ddi", `{"code":"123456","ddd":"21","valor":"999999999"}`},
		{"missing ddd", `{"code":"123456","ddi":"55","valor":"999999999"}`},
		{"empty ddd for Brazil", `{"code":"123456","ddi":"55","ddd":"","valor":"999999999"}`},
		{"missing valor", `{"code":"123456","ddi":"55","ddd":"21"}`},
		{"empty code", `{"code":"","ddi":"55","ddd":"21","valor":"999999999"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/v1/citizen/"+cpf+"/phone/validate", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// O pref.rio manda ddd vazio para número estrangeiro, no PUT /phone e na validação.
// O PUT aceita (DDD só é obrigatório com DDI 55); a validação tem que aceitar também.
func TestValidatePhoneVerification_ForeignNumberWithoutDDD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/v1/citizen/:cpf/phone", UpdateSelfDeclaredPhone)
	r.POST("/v1/citizen/:cpf/phone/validate", ValidatePhoneVerification)

	const cpf = "52998224725"
	ctx := context.Background()
	verifications := config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection)
	cleanup := func() {
		_, _ = verifications.DeleteMany(ctx, bson.M{"cpf": cpf})
		_, _ = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).DeleteMany(ctx, bson.M{"cpf": cpf})
		keys, _ := config.Redis.Keys(ctx, "*"+cpf+"*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	send := func(method, path, body string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	w := send(http.MethodPut, "/v1/citizen/"+cpf+"/phone", `{"ddi":"54","ddd":"","valor":"3511234567"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var verification models.PhoneVerification
	require.NoError(t, verifications.FindOne(ctx, bson.M{"cpf": cpf}).Decode(&verification))
	assert.Equal(t, "543511234567", verification.PhoneNumber)

	wrongCode := "000000"
	if verification.Code == wrongCode {
		wrongCode = "111111"
	}
	w = send(http.MethodPost, "/v1/citizen/"+cpf+"/phone/validate",
		`{"code":"`+wrongCode+`","ddi":"54","ddd":"","valor":"3511234567"}`)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	w = send(http.MethodPost, "/v1/citizen/"+cpf+"/phone/validate",
		`{"code":"`+verification.Code+`","ddi":"54","ddd":"","valor":"3511234567"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	remaining, err := verifications.CountDocuments(ctx, bson.M{"cpf": cpf})
	require.NoError(t, err)
	assert.Zero(t, remaining, "validated code must be consumed")
}

func TestValidatePhoneVerification_ResponseFormat(t *testing.T) {
	r := setupPhoneRouter()
	cpf := "03561350712"

	body := []byte(`{"code":"000000","ddi":"55","ddd":"21","valor":"999999999"}`)
	req, _ := http.NewRequest("POST", "/v1/citizen/"+cpf+"/phone/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return JSON response
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err, "Response should be valid JSON")
}
