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
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func setupTestRouterWithClaims(claims *models.JWTClaims) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if claims != nil {
			c.Set("claims", claims)
		}
		c.Next()
	})
	r.GET("/v1/citizen/:cpf", GetCitizenData)
	r.PUT("/v1/citizen/:cpf/birth-date", UpdateSelfDeclaredBirthDate)
	return r
}

func cleanupTestCPF(ctx context.Context, cpf string) {
	_, _ = config.MongoDB.Collection(config.AppConfig.CitizenCollection).DeleteOne(ctx, bson.M{"cpf": cpf})
	_, _ = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).DeleteOne(ctx, bson.M{"cpf": cpf})
	_ = config.Redis.Del(ctx, "citizen:"+cpf).Err()
	_ = config.Redis.Del(ctx, "citizen:cache:"+cpf).Err()
	_ = config.Redis.Del(ctx, "citizen:write:"+cpf).Err()
	_ = config.Redis.Del(ctx, "self_declared_nascimento:write:"+cpf).Err()
	_ = config.Redis.Del(ctx, "self_declared_nascimento:cache:"+cpf).Err()
}

func TestLazyMinimalCitizenCreation(t *testing.T) {
	cpf := "11144477735"
	testName := "Cidadão De Fora Teste"

	ctx := context.Background()
	cleanupTestCPF(ctx, cpf)
	t.Cleanup(func() { cleanupTestCPF(ctx, cpf) })

	claims := &models.JWTClaims{
		PreferredUsername: cpf,
		Name:              testName,
	}

	r := setupTestRouterWithClaims(claims)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/citizen/"+cpf, nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.CitizenResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, cpf, resp.CPF)
	require.NotNil(t, resp.Nome)
	assert.Equal(t, testName, *resp.Nome)

	var dbCitizen models.Citizen
	err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&dbCitizen)
	require.NoError(t, err)
	assert.Equal(t, cpf, dbCitizen.CPF)
	require.NotNil(t, dbCitizen.Nome)
	assert.Equal(t, testName, *dbCitizen.Nome)
}

func TestGetCitizenData_NotFoundWhenNoClaimsForMissingCitizen(t *testing.T) {
	cpf := "52998224725"

	ctx := context.Background()
	cleanupTestCPF(ctx, cpf)
	t.Cleanup(func() { cleanupTestCPF(ctx, cpf) })

	r := setupTestRouterWithClaims(nil)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/citizen/"+cpf, nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSelfDeclaredBirthDate_SuccessAndMerge(t *testing.T) {
	cpf := "03561350712"
	testName := "Cidadão Sem Nascimento Oficial"

	ctx := context.Background()
	cleanupTestCPF(ctx, cpf)
	t.Cleanup(func() { cleanupTestCPF(ctx, cpf) })

	claims := &models.JWTClaims{
		PreferredUsername: cpf,
		Name:              testName,
	}

	r := setupTestRouterWithClaims(claims)

	birthDateBody := map[string]interface{}{
		"data": "1995-08-25",
	}
	bodyBytes, _ := json.Marshal(birthDateBody)
	wPut := httptest.NewRecorder()
	reqPut, _ := http.NewRequest("PUT", "/v1/citizen/"+cpf+"/birth-date", bytes.NewBuffer(bodyBytes))
	reqPut.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wPut, reqPut)

	assert.Equal(t, http.StatusOK, wPut.Code)

	wGet := httptest.NewRecorder()
	reqGet, _ := http.NewRequest("GET", "/v1/citizen/"+cpf, nil)
	r.ServeHTTP(wGet, reqGet)

	assert.Equal(t, http.StatusOK, wGet.Code)

	var resp models.CitizenResponse
	err := json.Unmarshal(wGet.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, cpf, resp.CPF)
	require.NotNil(t, resp.Nascimento)
	require.NotNil(t, resp.Nascimento.Data)
	assert.Equal(t, 1995, resp.Nascimento.Data.Year())
	assert.Equal(t, time.August, resp.Nascimento.Data.Month())
	assert.Equal(t, 25, resp.Nascimento.Data.Day())

	t.Cleanup(func() { cleanupTestCPF(ctx, cpf) })
}

func TestSelfDeclaredBirthDate_BlockedWhenOfficialExists(t *testing.T) {
	cpf := "12345678909"
	officialDate := time.Date(1980, time.January, 15, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	cleanupTestCPF(ctx, cpf)
	t.Cleanup(func() { cleanupTestCPF(ctx, cpf) })

	citizen := models.Citizen{
		CPF:  cpf,
		Nome: strPtr("Cidadão Com Dado Oficial"),
		Nascimento: &models.Nascimento{
			Data: &officialDate,
		},
	}
	_, err := config.MongoDB.Collection(config.AppConfig.CitizenCollection).InsertOne(ctx, citizen)
	require.NoError(t, err)

	claims := &models.JWTClaims{
		PreferredUsername: cpf,
		Name:              "Cidadão Com Dado Oficial",
	}

	r := setupTestRouterWithClaims(claims)

	birthDateBody := map[string]interface{}{
		"data": "1995-08-25",
	}
	bodyBytes, _ := json.Marshal(birthDateBody)
	wPut := httptest.NewRecorder()
	reqPut, _ := http.NewRequest("PUT", "/v1/citizen/"+cpf+"/birth-date", bytes.NewBuffer(bodyBytes))
	reqPut.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wPut, reqPut)

	assert.Equal(t, http.StatusUnprocessableEntity, wPut.Code)

	var errResp ErrorResponse
	err = json.Unmarshal(wPut.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Equal(t, "Data de nascimento oficial não pode ser alterada por autodeclaração", errResp.Error)

	wGet := httptest.NewRecorder()
	reqGet, _ := http.NewRequest("GET", "/v1/citizen/"+cpf, nil)
	r.ServeHTTP(wGet, reqGet)

	assert.Equal(t, http.StatusOK, wGet.Code)

	var resp models.CitizenResponse
	err = json.Unmarshal(wGet.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Nascimento)
	require.NotNil(t, resp.Nascimento.Data)
	assert.Equal(t, 1980, resp.Nascimento.Data.Year())
}
