package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
)

var cpfTest = "03561350712"

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/citizen/:cpf", GetCitizenData)
	r.PUT("/v1/citizen/:cpf/address", UpdateSelfDeclaredAddress)
	r.PUT("/v1/citizen/:cpf/phone", UpdateSelfDeclaredPhone)
	r.PUT("/v1/citizen/:cpf/email", UpdateSelfDeclaredEmail)
	r.GET("/v1/health", HealthCheck)
	r.GET("/v1/citizen/:cpf/firstlogin", GetFirstLogin)
	r.PUT("/v1/citizen/:cpf/firstlogin", UpdateFirstLogin)
	r.GET("/v1/citizen/:cpf/optin", GetOptIn)
	r.PUT("/v1/citizen/:cpf/optin", UpdateOptIn)
	return r
}

func TestMain(m *testing.M) {
	if err := config.LoadConfig(); err != nil {
		panic(err)
	}
	config.InitMongoDB()
	config.InitRedis()
	m.Run()
}

func TestGetCitizenData(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/citizen/"+cpfTest, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d", w.Code)
	}
}

func TestUpdateSelfDeclaredAddress(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	body := map[string]interface{}{
		"Bairro": "Centro",
		"CEP": "20000000",
		"Estado": "RJ",
		"Logradouro": "Rua Teste",
		"Municipio": "Rio de Janeiro",
		"Numero": "123",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/v1/citizen/"+cpfTest+"/address", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusConflict && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200, 409, or 400, got %d", w.Code)
	}
}

func TestUpdateSelfDeclaredPhone(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	body := map[string]interface{}{
		"DDI": "+55",
		"DDD": "21",
		"Valor": "999999999",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/v1/citizen/"+cpfTest+"/phone", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusConflict && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200, 409, or 400, got %d", w.Code)
	}
}

func TestUpdateSelfDeclaredEmail(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	body := map[string]interface{}{
		"Valor": "test@example.com",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/v1/citizen/"+cpfTest+"/email", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusConflict && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200, 409, or 400, got %d", w.Code)
	}
}

func TestHealthCheck(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 200 or 503, got %d", w.Code)
	}
}

func TestGetFirstLogin(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/citizen/"+cpfTest+"/firstlogin", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUpdateFirstLogin(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/v1/citizen/"+cpfTest+"/firstlogin", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetOptIn(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/citizen/"+cpfTest+"/optin", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUpdateOptIn(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	body := map[string]interface{}{
		"OptIn": true,
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/v1/citizen/"+cpfTest+"/optin", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400, got %d", w.Code)
	}
}
