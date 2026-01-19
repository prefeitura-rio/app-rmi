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
	"go.mongodb.org/mongo-driver/bson"
)

func setupPetHandlersTest(t *testing.T) (*gin.Engine, func()) {
	// Use the shared MongoDB from common_test.go TestMain
	gin.SetMode(gin.TestMode)

	// Configure test collection
	config.AppConfig.PetCollection = "test_pets"

	ctx := context.Background()
	database := config.MongoDB

	// Initialize global pet service instance
	services.PetServiceInstance = services.NewPetService(database, logging.Logger)

	router := gin.New()
	router.GET("/citizen/:cpf/pets", GetPets)
	router.GET("/citizen/:cpf/pets/:pet_id", GetPet)
	router.GET("/citizen/:cpf/pets/stats", GetPetStats)
	router.POST("/citizen/:cpf/pets", RegisterPet)

	return router, func() {
		database.Drop(ctx)
		services.PetServiceInstance = nil
	}
}

func TestGetPets_InvalidCPF(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name string
		cpf  string
	}{
		{"empty CPF", ""},
		{"invalid format", "12345"},
		{"letters in CPF", "abcdefghijk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/citizen/"+tt.cpf+"/pets?page=1&per_page=10", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("GetPets() with %s status = %v, want %v", tt.name, w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestGetPets_ValidCPF_Empty(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/citizen/03561350712/pets?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetPets() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.PaginatedPets
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Pagination.Total != 0 {
		t.Errorf("GetPets() Total = %v, want 0", response.Pagination.Total)
	}
}

func TestGetPets_WithData(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test pets
	collection := config.MongoDB.Collection(config.AppConfig.PetCollection)
	pets := []interface{}{
		bson.M{
			"_id":          1,
			"cpf":          "03561350712",
			"animal_nome":  "Rex",
			"especie_nome": "dog",
		},
		bson.M{
			"_id":          2,
			"cpf":          "03561350712",
			"animal_nome":  "Mimi",
			"especie_nome": "cat",
		},
	}

	_, err := collection.InsertMany(ctx, pets)
	if err != nil {
		t.Fatalf("Failed to insert pets: %v", err)
	}

	req, _ := http.NewRequest("GET", "/citizen/03561350712/pets?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetPets() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.PaginatedPets
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Pagination.Total != 2 {
		t.Errorf("GetPets() Total = %v, want 2", response.Pagination.Total)
	}

	if len(response.Data) != 2 {
		t.Errorf("GetPets() len(Data) = %v, want 2", len(response.Data))
	}
}

func TestGetPets_InvalidPagination(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name    string
		page    string
		perPage string
	}{
		{"invalid page", "invalid", "10"},
		{"page zero", "0", "10"},
		{"per_page too large", "1", "101"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/citizen/03561350712/pets?page="+tt.page+"&per_page="+tt.perPage, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("GetPets() with %s status = %v, want %v", tt.name, w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestGetPet_InvalidCPF(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/citizen/12345/pets/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GetPet() with invalid CPF status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestGetPet_InvalidPetID(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name  string
		petID string
	}{
		{"non-numeric ID", "abc"},
		{"empty ID", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/citizen/03561350712/pets/"+tt.petID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
				t.Errorf("GetPet() with %s status = %v, want %v or %v", tt.name, w.Code, http.StatusBadRequest, http.StatusNotFound)
			}
		})
	}
}

func TestGetPet_NotFound(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/citizen/03561350712/pets/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GetPet() not found status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestGetPet_Success(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test pet
	collection := config.MongoDB.Collection(config.AppConfig.PetCollection)
	pet := bson.M{
		"_id":          1,
		"cpf":          "03561350712",
		"animal_nome":  "Rex",
		"especie_nome": "dog",
	}

	_, err := collection.InsertOne(ctx, pet)
	if err != nil {
		t.Fatalf("Failed to insert pet: %v", err)
	}

	req, _ := http.NewRequest("GET", "/citizen/03561350712/pets/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetPet() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.Pet
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Name != "Rex" {
		t.Errorf("GetPet() Name = %v, want Rex", response.Name)
	}
}

func TestGetPetStats_InvalidCPF(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/citizen/12345/pets/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GetPetStats() with invalid CPF status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestGetPetStats_Empty(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/citizen/03561350712/pets/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetPetStats() status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRegisterPet_InvalidCPF(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	birthDate := time.Now().AddDate(-2, 0, 0) // 2 years old
	pedigreeIndicator := false
	reqBody := models.PetRegistrationRequest{
		Name:               "Rex",
		SpeciesName:        "dog",
		SexAbbreviation:    "M",
		BirthDate:          &birthDate,
		NeuteredIndicator:  true,
		BreedName:          "Mixed",
		SizeName:           "Medium",
		PedigreeIndicator:  &pedigreeIndicator,
		MicrochipNumber:    "123456789",
		PedigreeOriginName: "Brazil",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/citizen/12345/pets", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("RegisterPet() with invalid CPF status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestRegisterPet_InvalidRequest(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/citizen/03561350712/pets", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("RegisterPet() with invalid JSON status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestRegisterPet_Success(t *testing.T) {
	router, cleanup := setupPetHandlersTest(t)
	defer cleanup()

	birthDate := time.Now().AddDate(-2, 0, 0) // 2 years old
	pedigreeIndicator := false
	reqBody := models.PetRegistrationRequest{
		Name:               "Rex",
		SpeciesName:        "dog",
		SexAbbreviation:    "M",
		BirthDate:          &birthDate,
		NeuteredIndicator:  true,
		BreedName:          "Mixed",
		SizeName:           "Medium",
		PedigreeIndicator:  &pedigreeIndicator,
		MicrochipNumber:    "123456789",
		PedigreeOriginName: "Brazil",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/citizen/03561350712/pets", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("RegisterPet() status = %v, want %v (body: %s)", w.Code, http.StatusCreated, w.Body.String())
	}

	var response models.Pet
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Name != "Rex" {
		t.Errorf("RegisterPet() Name = %v, want Rex", response.Name)
	}

	if response.ID == nil {
		t.Error("RegisterPet() ID is nil")
	}
}
