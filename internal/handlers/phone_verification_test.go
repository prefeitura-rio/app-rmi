package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupPhoneRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/citizen/:cpf/phone/validate", ValidatePhoneVerification)
	return r
}

func TestValidatePhoneVerificationEndpoint(t *testing.T) {
	r := setupPhoneRouter()
	cpf := "12345678901"

	// Invalid body
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/citizen/"+cpf+"/phone/validate", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", w.Code)
	}

	// Missing code/phone (simulate not found)
	w2 := httptest.NewRecorder()
	body := []byte(`{"code":"000000","ddi":"55","ddd":"21","valor":"999999999"}`)
	req2, _ := http.NewRequest("POST", "/v1/citizen/"+cpf+"/phone/validate", bytes.NewBuffer(body))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound && w2.Code != http.StatusInternalServerError {
		t.Errorf("expected 404 or 500 for not found/DB error, got %d", w2.Code)
	}

	// Note: Happy path would require a real verification code in the DB, which is not practical for a static test.
}
