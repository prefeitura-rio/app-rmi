package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
		{"empty CPF", "", http.StatusNotFound}, // Route won't match
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
