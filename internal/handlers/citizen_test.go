package handlers

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"go.uber.org/zap"
)

var cpfTest string

func TestMain(m *testing.M) {
	// Parse flags before running tests
	flag.StringVar(&cpfTest, "cpf", "03561350712", "CPF to use for testing")
	flag.Parse()

	// Initialize configuration and connections
	if err := config.LoadConfig(); err != nil {
		panic(err)
	}
	config.InitMongoDB()
	config.InitRedis()

	// Log the CPF being used
	zap.L().Info("Running tests with CPF", zap.String("cpf", cpfTest))

	// Run tests
	os.Exit(m.Run())
}

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/citizen/:cpf", GetCitizenData)
	r.GET("/v1/citizen/:cpf/wallet", GetCitizenWallet)
	r.GET("/v1/citizen/:cpf/maintenance-request", GetMaintenanceRequests)
	r.PUT("/v1/citizen/:cpf/address", UpdateSelfDeclaredAddress)
	r.PUT("/v1/citizen/:cpf/phone", UpdateSelfDeclaredPhone)
	r.PUT("/v1/citizen/:cpf/email", UpdateSelfDeclaredEmail)
	r.PUT("/v1/citizen/:cpf/ethnicity", UpdateSelfDeclaredRaca)
	r.GET("/v1/health", HealthCheck)
	r.GET("/v1/citizen/:cpf/firstlogin", GetFirstLogin)
	r.PUT("/v1/citizen/:cpf/firstlogin", UpdateFirstLogin)
	r.GET("/v1/citizen/:cpf/optin", GetOptIn)
	r.PUT("/v1/citizen/:cpf/optin", UpdateOptIn)
	return r
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

func TestGetCitizenWallet(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/citizen/"+cpfTest+"/wallet", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d", w.Code)
	}
}

func TestGetMaintenanceRequests(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/citizen/"+cpfTest+"/maintenance-request", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d", w.Code)
	}
}

func TestUpdateSelfDeclaredAddress(t *testing.T) {
	r := setupRouter()
	w := httptest.NewRecorder()
	body := map[string]interface{}{
		"Bairro":     "Centro",
		"CEP":        "20000000",
		"Estado":     "RJ",
		"Logradouro": "Rua Teste",
		"Municipio":  "Rio de Janeiro",
		"Numero":     "123",
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
		"DDI":   "+55",
		"DDD":   "21",
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

func TestUpdateSelfDeclaredRaca(t *testing.T) {
	t.Logf("Testing with CPF: %s", cpfTest)
	r := setupRouter()
	w := httptest.NewRecorder()
	body := map[string]interface{}{
		"Valor": "Branca",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/v1/citizen/"+cpfTest+"/ethnicity", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Log the response for debugging
	t.Logf("Response status: %d", w.Code)
	if w.Body.Len() > 0 {
		t.Logf("Response body: %s", w.Body.String())
	}

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
