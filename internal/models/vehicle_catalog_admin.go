package models

import (
	"fmt"
	"strings"
)

// VehicleBrandCreateRequest is the body for POST /admin/mobilidade/vehicle-brands.
type VehicleBrandCreateRequest struct {
	Name string `json:"name" binding:"required" example:"Caloi"`
}

// Validate checks create-brand rules.
func (r *VehicleBrandCreateRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// VehicleBrandUpdateRequest is the body for PATCH /admin/mobilidade/vehicle-brands/{brand_id}.
type VehicleBrandUpdateRequest struct {
	Name string `json:"name" binding:"required" example:"Caloi"`
}

// Validate checks update-brand rules.
func (r *VehicleBrandUpdateRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// VehicleModelCreateRequest is the body for POST /admin/mobilidade/vehicle-models.
type VehicleModelCreateRequest struct {
	BrandID     string      `json:"brand_id" binding:"required" example:"brand_caloi"`
	Name        string      `json:"name" binding:"required" example:"E-Vibe"`
	VehicleType VehicleType `json:"vehicle_type" binding:"required" example:"bicicleta_eletrica"`
}

// Validate checks create-model rules.
func (r *VehicleModelCreateRequest) Validate() error {
	if strings.TrimSpace(r.BrandID) == "" {
		return fmt.Errorf("brand_id is required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if !IsValidVehicleType(r.VehicleType) {
		return fmt.Errorf("invalid vehicle_type")
	}
	return nil
}

// VehicleModelUpdateRequest is the body for PATCH /admin/mobilidade/vehicle-models/{model_id}.
type VehicleModelUpdateRequest struct {
	BrandID     *string      `json:"brand_id"`
	Name        *string      `json:"name"`
	VehicleType *VehicleType `json:"vehicle_type"`
}

// Validate checks update-model rules.
func (r *VehicleModelUpdateRequest) Validate() error {
	if r.Name != nil && strings.TrimSpace(*r.Name) == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if r.BrandID != nil && strings.TrimSpace(*r.BrandID) == "" {
		return fmt.Errorf("brand_id cannot be empty")
	}
	if r.VehicleType != nil && !IsValidVehicleType(*r.VehicleType) {
		return fmt.Errorf("invalid vehicle_type")
	}
	if r.Name == nil && r.BrandID == nil && r.VehicleType == nil {
		return fmt.Errorf("at least one field must be provided")
	}
	return nil
}
