package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKeyValuePairs(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected map[string]interface{}
	}{
		{
			name: "valid key-value pairs",
			input: []interface{}{
				map[string]interface{}{"Key": "name", "Value": "Rex"},
				map[string]interface{}{"Key": "species", "Value": "dog"},
			},
			expected: map[string]interface{}{
				"name":    "Rex",
				"species": "dog",
			},
		},
		{
			name:     "empty slice",
			input:    []interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: map[string]interface{}{},
		},
		{
			name: "invalid item in slice",
			input: []interface{}{
				"not a map",
				map[string]interface{}{"Key": "name", "Value": "Rex"},
			},
			expected: map[string]interface{}{
				"name": "Rex",
			},
		},
		{
			name: "missing Key field",
			input: []interface{}{
				map[string]interface{}{"Value": "Rex"},
				map[string]interface{}{"Key": "species", "Value": "dog"},
			},
			expected: map[string]interface{}{
				"species": "dog",
			},
		},
		{
			name: "non-string key",
			input: []interface{}{
				map[string]interface{}{"Key": 123, "Value": "Rex"},
				map[string]interface{}{"Key": "species", "Value": "dog"},
			},
			expected: map[string]interface{}{
				"species": "dog",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseKeyValuePairs(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("ParseKeyValuePairs() returned map of length %v, want %v", len(result), len(tt.expected))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, ok := result[key]; !ok {
					t.Errorf("ParseKeyValuePairs() missing key %v", key)
				} else if actualValue != expectedValue {
					t.Errorf("ParseKeyValuePairs() key %v = %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestRawCitizenPets_ToCitizenPets(t *testing.T) {
	stats := &Statistics{DogCount: 1, CatCount: 1}
	var partition int64 = 1

	tests := []struct {
		name     string
		raw      *RawCitizenPets
		validate func(*testing.T, *CitizenPets)
	}{
		{
			name: "with pet data",
			raw: &RawCitizenPets{
				ID:           1,
				CPF:          "12345678901",
				Statistics:   stats,
				CPFPartition: &partition,
				PetData: &NestedPetData{
					Pets: []Pet{
						{Name: "Rex"},
						{Name: "Mimi"},
					},
				},
			},
			validate: func(t *testing.T, result *CitizenPets) {
				if result.ID != 1 {
					t.Errorf("ToCitizenPets() ID = %v, want 1", result.ID)
				}
				if result.CPF != "12345678901" {
					t.Errorf("ToCitizenPets() CPF = %v, want 12345678901", result.CPF)
				}
				if len(result.Pets) != 2 {
					t.Errorf("ToCitizenPets() Pets length = %v, want 2", len(result.Pets))
				}
				if result.Statistics == nil {
					t.Errorf("ToCitizenPets() Statistics is nil")
				}
			},
		},
		{
			name: "without pet data",
			raw: &RawCitizenPets{
				ID:           2,
				CPF:          "98765432109",
				Statistics:   stats,
				CPFPartition: &partition,
				PetData:      nil,
			},
			validate: func(t *testing.T, result *CitizenPets) {
				if result.ID != 2 {
					t.Errorf("ToCitizenPets() ID = %v, want 2", result.ID)
				}
				if len(result.Pets) != 0 {
					t.Errorf("ToCitizenPets() Pets length = %v, want 0", len(result.Pets))
				}
			},
		},
		{
			name: "with nested statistics",
			raw: &RawCitizenPets{
				ID:         3,
				CPF:        "11111111111",
				Statistics: nil,
				PetData: &NestedPetData{
					Pets:       []Pet{{Name: "Buddy"}},
					Statistics: stats,
				},
			},
			validate: func(t *testing.T, result *CitizenPets) {
				if result.Statistics == nil {
					t.Errorf("ToCitizenPets() Statistics should be inherited from nested")
				}
				if result.Statistics != nil && result.Statistics.DogCount != 1 {
					t.Errorf("ToCitizenPets() Statistics.DogCount = %v, want 1", result.Statistics.DogCount)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.raw.ToCitizenPets()
			if err != nil {
				t.Fatalf("ToCitizenPets() error = %v, want nil", err)
			}
			tt.validate(t, result)
		})
	}
}

func TestSelfRegisteredPet_ToPet(t *testing.T) {
	birthDate := time.Now().AddDate(-2, 0, 0)
	pedigreeIndicator := true

	selfRegistered := &SelfRegisteredPet{
		ID:                 123,
		CPF:                "12345678901",
		Name:               "Rex",
		MicrochipNumber:    "ABC123",
		SexAbbreviation:    "M",
		BirthDate:          &birthDate,
		NeuteredIndicator:  true,
		SpeciesName:        "dog",
		PedigreeIndicator:  &pedigreeIndicator,
		PedigreeOriginName: "Brazil",
		BreedName:          "Labrador",
		SizeName:           "Large",
		PhotoURL:           "https://example.com/rex.jpg",
		Source:             "self_registered",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	pet := selfRegistered.ToPet()

	if pet == nil {
		t.Fatal("ToPet() returned nil")
	}

	if pet.ID == nil || *pet.ID != 123 {
		t.Errorf("ToPet() ID = %v, want 123", pet.ID)
	}

	if pet.Name != "Rex" {
		t.Errorf("ToPet() Name = %v, want Rex", pet.Name)
	}

	if pet.MicrochipNumber != "ABC123" {
		t.Errorf("ToPet() MicrochipNumber = %v, want ABC123", pet.MicrochipNumber)
	}

	if pet.SexAbbreviation != "M" {
		t.Errorf("ToPet() SexAbbreviation = %v, want M", pet.SexAbbreviation)
	}

	if pet.BirthDate == nil || !pet.BirthDate.Equal(birthDate) {
		t.Errorf("ToPet() BirthDate = %v, want %v", pet.BirthDate, birthDate)
	}

	if pet.NeuteredIndicator == nil || *pet.NeuteredIndicator != true {
		t.Errorf("ToPet() NeuteredIndicator = %v, want true", pet.NeuteredIndicator)
	}

	if pet.SpeciesName != "dog" {
		t.Errorf("ToPet() SpeciesName = %v, want dog", pet.SpeciesName)
	}

	if pet.PedigreeIndicator == nil || *pet.PedigreeIndicator != true {
		t.Errorf("ToPet() PedigreeIndicator = %v, want true", pet.PedigreeIndicator)
	}

	if pet.PedigreeOriginName != "Brazil" {
		t.Errorf("ToPet() PedigreeOriginName = %v, want Brazil", pet.PedigreeOriginName)
	}

	if pet.BreedName != "Labrador" {
		t.Errorf("ToPet() BreedName = %v, want Labrador", pet.BreedName)
	}

	if pet.SizeName != "Large" {
		t.Errorf("ToPet() SizeName = %v, want Large", pet.SizeName)
	}

	if pet.PhotoURL != "https://example.com/rex.jpg" {
		t.Errorf("ToPet() PhotoURL = %v, want https://example.com/rex.jpg", pet.PhotoURL)
	}

	if pet.Source != "self_registered" {
		t.Errorf("ToPet() Source = %v, want self_registered", pet.Source)
	}
}

func TestSelfRegisteredPet_ToPet_MinimalData(t *testing.T) {
	birthDate := time.Now()

	selfRegistered := &SelfRegisteredPet{
		ID:                123,
		CPF:               "12345678901",
		Name:              "Buddy",
		SexAbbreviation:   "F",
		BirthDate:         &birthDate,
		NeuteredIndicator: false,
		SpeciesName:       "cat",
		BreedName:         "Mixed",
		SizeName:          "Small",
		Source:            "self_registered",
	}

	pet := selfRegistered.ToPet()

	if pet == nil {
		t.Fatal("ToPet() returned nil")
	}

	if pet.Name != "Buddy" {
		t.Errorf("ToPet() Name = %v, want Buddy", pet.Name)
	}

	if pet.MicrochipNumber != "" {
		t.Errorf("ToPet() MicrochipNumber = %v, want empty string", pet.MicrochipNumber)
	}

	if pet.NeuteredIndicator == nil || *pet.NeuteredIndicator != false {
		t.Errorf("ToPet() NeuteredIndicator = %v, want false", pet.NeuteredIndicator)
	}

	if pet.PedigreeIndicator != nil {
		t.Errorf("ToPet() PedigreeIndicator = %v, want nil", pet.PedigreeIndicator)
	}

	if pet.PhotoURL != "" {
		t.Errorf("ToPet() PhotoURL = %v, want empty string", pet.PhotoURL)
	}
}

func TestPetRegistrationRequest_Structure(t *testing.T) {
	birthDate := time.Now().AddDate(-1, 0, 0)
	pedigreeIndicator := false

	request := PetRegistrationRequest{
		Name:               "Max",
		MicrochipNumber:    "XYZ789",
		SexAbbreviation:    "M",
		BirthDate:          &birthDate,
		NeuteredIndicator:  true,
		SpeciesName:        "dog",
		PedigreeIndicator:  &pedigreeIndicator,
		PedigreeOriginName: "USA",
		BreedName:          "Beagle",
		SizeName:           "Medium",
		PhotoURL:           "https://example.com/max.jpg",
	}

	if request.Name != "Max" {
		t.Errorf("PetRegistrationRequest Name = %v, want Max", request.Name)
	}

	if request.SexAbbreviation != "M" {
		t.Errorf("PetRegistrationRequest SexAbbreviation = %v, want M", request.SexAbbreviation)
	}

	if !request.NeuteredIndicator {
		t.Errorf("PetRegistrationRequest NeuteredIndicator = %v, want true", request.NeuteredIndicator)
	}

	if request.PedigreeIndicator == nil || *request.PedigreeIndicator != false {
		t.Errorf("PetRegistrationRequest PedigreeIndicator = %v, want false", request.PedigreeIndicator)
	}
}

func TestRawCitizenPets_ToCitizenPets_WithTestify(t *testing.T) {
	stats := &Statistics{DogCount: 2, CatCount: 1}
	var partition int64 = 5

	t.Run("with both root and nested stats", func(t *testing.T) {
		nestedStats := &Statistics{DogCount: 3, CatCount: 2}
		raw := &RawCitizenPets{
			ID:           100,
			CPF:          "12345678901",
			Statistics:   stats,
			CPFPartition: &partition,
			PetData: &NestedPetData{
				Pets:         []Pet{{Name: "Rex"}, {Name: "Mimi"}},
				Statistics:   nestedStats,
				CPFPartition: &partition,
			},
		}

		result, err := raw.ToCitizenPets()
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 100, result.ID)
		assert.Equal(t, "12345678901", result.CPF)
		assert.Len(t, result.Pets, 2)
		// Root stats should be preserved
		require.NotNil(t, result.Statistics)
		assert.Equal(t, 2, result.Statistics.DogCount)
		assert.Equal(t, 1, result.Statistics.CatCount)
	})

	t.Run("with nested CPF partition only", func(t *testing.T) {
		var nestedPartition int64 = 10
		raw := &RawCitizenPets{
			ID:           200,
			CPF:          "98765432100",
			CPFPartition: nil,
			PetData: &NestedPetData{
				Pets:         []Pet{{Name: "Buddy"}},
				CPFPartition: &nestedPartition,
			},
		}

		result, err := raw.ToCitizenPets()
		require.NoError(t, err)
		require.NotNil(t, result.CPFPartition)
		assert.Equal(t, int64(10), *result.CPFPartition)
	})

	t.Run("empty pet data", func(t *testing.T) {
		raw := &RawCitizenPets{
			ID:      300,
			CPF:     "11111111111",
			PetData: &NestedPetData{Pets: []Pet{}},
		}

		result, err := raw.ToCitizenPets()
		require.NoError(t, err)
		assert.Len(t, result.Pets, 0)
	})

	t.Run("all fields populated", func(t *testing.T) {
		raw := &RawCitizenPets{
			ID:           400,
			CPF:          "22222222222",
			Statistics:   stats,
			CPFPartition: &partition,
			PetData: &NestedPetData{
				Pets: []Pet{
					{Name: "Dog1", SpeciesName: "Canine"},
					{Name: "Cat1", SpeciesName: "Feline"},
					{Name: "Dog2", SpeciesName: "Canine"},
				},
			},
		}

		result, err := raw.ToCitizenPets()
		require.NoError(t, err)
		assert.Equal(t, 400, result.ID)
		assert.Equal(t, "22222222222", result.CPF)
		assert.Len(t, result.Pets, 3)
		assert.Equal(t, "Dog1", result.Pets[0].Name)
		assert.Equal(t, "Cat1", result.Pets[1].Name)
		assert.Equal(t, "Dog2", result.Pets[2].Name)
	})
}
