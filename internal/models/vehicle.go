package models

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// VehicleType is the fixed classification for Mobilidade vehicles.
type VehicleType string

const (
	VehicleTypeBicicletaEletrica VehicleType = "bicicleta_eletrica"
	VehicleTypeAutopropelido     VehicleType = "autopropelido"
	VehicleTypeCiclomotor        VehicleType = "ciclomotor"
)

// ValidVehicleTypes returns the allowed vehicle_type values.
func ValidVehicleTypes() []VehicleType {
	return []VehicleType{
		VehicleTypeBicicletaEletrica,
		VehicleTypeAutopropelido,
		VehicleTypeCiclomotor,
	}
}

// IsValidVehicleType reports whether t is one of the fixed types.
func IsValidVehicleType(t VehicleType) bool {
	for _, v := range ValidVehicleTypes() {
		if v == t {
			return true
		}
	}
	return false
}

// VehicleRole is the caller's relationship to a vehicle in list/detail responses.
type VehicleRole string

const (
	VehicleRoleOwner     VehicleRole = "owner"
	VehicleRoleConductor VehicleRole = "conductor"
)

// VehicleColors returns the fixed color catalog for Mobilidade forms.
func VehicleColors() []string {
	return []string{
		"Amarelo", "Azul", "Azul Claro", "Azul Escuro", "Bege", "Branco", "Bronze",
		"Cereja", "Cinza", "Creme", "Dourado", "Laranja", "Lilás", "Marrom", "Preto",
		"Prata", "Rosa", "Roxo", "Verde", "Verde Claro", "Verde Escuro", "Vermelho", "Vinho",
	}
}

// IsValidVehicleColor reports whether color is in the fixed catalog.
func IsValidVehicleColor(color string) bool {
	for _, c := range VehicleColors() {
		if c == color {
			return true
		}
	}
	return false
}

// allowedGCSHostSuffixes are accepted hosts for Mobilidade document/photo URLs.
var allowedGCSHostSuffixes = []string{
	"storage.googleapis.com",
	"storage.cloud.google.com",
}

// IsAllowedGCSURL reports whether rawURL is an https URL on an allowlisted GCS host.
// Rejects blob:, data:, and arbitrary hosts (Apêndice A.2).
func IsAllowedGCSURL(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	lower := strings.ToLower(rawURL)
	if strings.HasPrefix(lower, "blob:") || strings.HasPrefix(lower, "data:") {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range allowedGCSHostSuffixes {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// Stable catalog sentinel IDs (seeded in every environment; not present in the CSV).
const (
	VehicleBrandOutroID = "brand_outro"
	VehicleModelOutroID = "model_outro"
)

// VehicleBrand is a seeded catalog brand (Marca).
type VehicleBrand struct {
	ID      string `bson:"_id" json:"id" example:"brand_caloi"`
	Name    string `bson:"name" json:"name" example:"Caloi"`
	IsOther bool   `bson:"is_other" json:"is_other" example:"false"`
}

// VehicleModel is a seeded catalog model belonging to a brand.
type VehicleModel struct {
	ID          string      `bson:"_id" json:"id" example:"model_e-vibe"`
	BrandID     string      `bson:"brand_id" json:"brand_id" example:"brand_caloi"`
	Name        string      `bson:"name" json:"name" example:"E-Vibe"`
	VehicleType VehicleType `bson:"vehicle_type" json:"vehicle_type"`
	IsOther     bool        `bson:"is_other" json:"is_other" example:"false"`
}

// VehicleBrandsResponse wraps the brands catalog for Orval-typed clients.
type VehicleBrandsResponse struct {
	Data []VehicleBrand `json:"data"`
}

// VehicleModelsResponse wraps the models catalog for Orval-typed clients.
type VehicleModelsResponse struct {
	Data []VehicleModel `json:"data"`
}

// VehicleColorsResponse wraps the fixed color list for Orval-typed clients.
type VehicleColorsResponse struct {
	Data []string `json:"data" example:"Amarelo,Azul,Preto"`
}

// Vehicle is the persisted Mobilidade vehicle document.
// OwnerName/OwnerPhone/OwnerEmail are response-only (enriched live from RMI via owner_cpf);
// they are not written on create for UI purposes.
type Vehicle struct {
	ID                   primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	OwnerCPF             string             `bson:"owner_cpf" json:"owner_cpf"`
	OwnerName            string             `bson:"-" json:"owner_name"`
	OwnerPhone           string             `bson:"-" json:"owner_phone"`
	OwnerEmail           string             `bson:"-" json:"owner_email"`
	DisplayName          string             `bson:"display_name" json:"display_name"`
	BrandID              *string            `bson:"brand_id,omitempty" json:"brand_id"`
	BrandOther           *string            `bson:"brand_other,omitempty" json:"brand_other"`
	ModelID              *string            `bson:"model_id,omitempty" json:"model_id"`
	ModelOther           *string            `bson:"model_other,omitempty" json:"model_other"`
	VehicleType          VehicleType        `bson:"vehicle_type" json:"vehicle_type"`
	Color                string             `bson:"color" json:"color"`
	SerialNumber         string             `bson:"serial_number" json:"serial_number"`
	// RegistrationNumber is a short wallet identifier generated on create (format RJ-E-XXXXXX). Not accepted in POST/PATCH.
	RegistrationNumber   string             `bson:"registration_number" json:"registration_number" example:"RJ-E-000001"`
	SerialNumberPhotoURL string             `bson:"serial_number_photo_url" json:"serial_number_photo_url"`
	VehiclePhotoURL      string             `bson:"vehicle_photo_url" json:"vehicle_photo_url"`
	InvoicePhotoURL      *string            `bson:"invoice_photo_url" json:"invoice_photo_url" extensions:"x-nullable"`
	HasInvoice           bool               `bson:"has_invoice" json:"has_invoice"`
	SelfDeclaration      bool               `bson:"self_declaration" json:"self_declaration"`

	// Optional file metadata for UI detail (Apêndice A.4).
	SerialNumberPhotoFileName *string `bson:"serial_number_photo_file_name,omitempty" json:"serial_number_photo_file_name,omitempty"`
	SerialNumberPhotoFileSize *int64  `bson:"serial_number_photo_file_size,omitempty" json:"serial_number_photo_file_size,omitempty"`
	VehiclePhotoFileName      *string `bson:"vehicle_photo_file_name,omitempty" json:"vehicle_photo_file_name,omitempty"`
	VehiclePhotoFileSize      *int64  `bson:"vehicle_photo_file_size,omitempty" json:"vehicle_photo_file_size,omitempty"`
	InvoicePhotoFileName      *string `bson:"invoice_photo_file_name,omitempty" json:"invoice_photo_file_name,omitempty"`
	InvoicePhotoFileSize      *int64  `bson:"invoice_photo_file_size,omitempty" json:"invoice_photo_file_size,omitempty"`

	CreatedAt time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time  `bson:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}

// VehicleListItem is a card on the wallet / home list.
type VehicleListItem struct {
	ID                 string      `json:"id"`
	DisplayName        string      `json:"display_name"`
	RegistrationNumber string      `json:"registration_number" example:"RJ-E-000001"`
	BrandID            *string     `json:"brand_id"`
	BrandOther         *string     `json:"brand_other"`
	ModelID            *string     `json:"model_id"`
	ModelOther         *string     `json:"model_other"`
	VehicleType        VehicleType `json:"vehicle_type"`
	Color              string      `json:"color"`
	VehiclePhotoURL    string      `json:"vehicle_photo_url"`
	Role               VehicleRole `json:"role"`
}

// VehicleDetail is the detail response including the caller's role.
type VehicleDetail struct {
	Vehicle
	Role VehicleRole `json:"role"`
}

// PaginatedVehicles is the paginated list response (pets pagination shape).
type PaginatedVehicles struct {
	Data       []VehicleListItem `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
}

// VehicleCreateRequest is the body for POST /citizen/{cpf}/vehicles.
// Owner contact fields (owner_name/owner_phone/owner_email) are not accepted — ignored if sent.
// registration_number is generated by the backend and must not be sent.
type VehicleCreateRequest struct {
	DisplayName          string       `json:"display_name" binding:"required"`
	BrandID              *string      `json:"brand_id"`
	BrandOther           *string      `json:"brand_other"`
	ModelID              *string      `json:"model_id"`
	ModelOther           *string      `json:"model_other"`
	VehicleType          *VehicleType `json:"vehicle_type"`
	Color                string       `json:"color" binding:"required"`
	SerialNumber         string       `json:"serial_number" binding:"required"`
	SerialNumberPhotoURL string       `json:"serial_number_photo_url" binding:"required"`
	VehiclePhotoURL      string       `json:"vehicle_photo_url" binding:"required"`
	InvoicePhotoURL      *string      `json:"invoice_photo_url" extensions:"x-nullable"`
	HasInvoice           *bool        `json:"has_invoice" binding:"required"`
	SelfDeclaration      bool         `json:"self_declaration" binding:"required"`

	SerialNumberPhotoFileName *string `json:"serial_number_photo_file_name"`
	SerialNumberPhotoFileSize *int64  `json:"serial_number_photo_file_size"`
	VehiclePhotoFileName      *string `json:"vehicle_photo_file_name"`
	VehiclePhotoFileSize      *int64  `json:"vehicle_photo_file_size"`
	InvoicePhotoFileName      *string `json:"invoice_photo_file_name"`
	InvoicePhotoFileSize      *int64  `json:"invoice_photo_file_size"`
}

// HasInvoiceValue returns the has_invoice flag (false when nil).
func (r *VehicleCreateRequest) HasInvoiceValue() bool {
	return r.HasInvoice != nil && *r.HasInvoice
}

// NormalizeInvoiceFields applies Apêndice A rules for has_invoice / invoice_photo_url.
func (r *VehicleCreateRequest) NormalizeInvoiceFields() {
	if !r.HasInvoiceValue() {
		r.InvoicePhotoURL = nil
		r.InvoicePhotoFileName = nil
		r.InvoicePhotoFileSize = nil
	}
}

// Validate checks create-request business rules (catalog vs "Outro", docs, GCS URLs).
func (r *VehicleCreateRequest) Validate() error {
	if r.HasInvoice == nil {
		return fmt.Errorf("has_invoice is required")
	}
	r.NormalizeInvoiceFields()

	if !r.SelfDeclaration {
		return fmt.Errorf("self_declaration must be true")
	}
	if !IsValidVehicleColor(r.Color) {
		return fmt.Errorf("invalid color")
	}
	if !IsAllowedGCSURL(r.SerialNumberPhotoURL) {
		return fmt.Errorf("serial_number_photo_url must be an https GCS URL")
	}
	if !IsAllowedGCSURL(r.VehiclePhotoURL) {
		return fmt.Errorf("vehicle_photo_url must be an https GCS URL")
	}
	if r.HasInvoiceValue() {
		if r.InvoicePhotoURL == nil || strings.TrimSpace(*r.InvoicePhotoURL) == "" {
			return fmt.Errorf("invoice_photo_url is required when has_invoice is true")
		}
		if !IsAllowedGCSURL(*r.InvoicePhotoURL) {
			return fmt.Errorf("invoice_photo_url must be an https GCS URL")
		}
	}

	catalogFlow := r.BrandID != nil && *r.BrandID != "" && r.ModelID != nil && *r.ModelID != ""
	otherFlow := (r.BrandOther != nil && *r.BrandOther != "") || (r.ModelOther != nil && *r.ModelOther != "")

	if catalogFlow {
		return nil
	}
	if otherFlow {
		if r.VehicleType == nil || !IsValidVehicleType(*r.VehicleType) {
			return fmt.Errorf("vehicle_type is required for Outro flow")
		}
		return nil
	}
	return fmt.Errorf("either catalog brand_id+model_id or brand_other/model_other must be provided")
}

// VehicleUpdateRequest is the body for PATCH /citizen/{cpf}/vehicles/{vehicle_id}.
type VehicleUpdateRequest struct {
	DisplayName          *string      `json:"display_name"`
	BrandID              *string      `json:"brand_id"`
	BrandOther           *string      `json:"brand_other"`
	ModelID              *string      `json:"model_id"`
	ModelOther           *string      `json:"model_other"`
	VehicleType          *VehicleType `json:"vehicle_type"`
	Color                *string      `json:"color"`
	SerialNumber         *string      `json:"serial_number"`
	SerialNumberPhotoURL *string      `json:"serial_number_photo_url"`
	VehiclePhotoURL      *string      `json:"vehicle_photo_url"`
	InvoicePhotoURL      *string      `json:"invoice_photo_url" extensions:"x-nullable"`
	HasInvoice           *bool        `json:"has_invoice"`

	SerialNumberPhotoFileName *string `json:"serial_number_photo_file_name"`
	SerialNumberPhotoFileSize *int64  `json:"serial_number_photo_file_size"`
	VehiclePhotoFileName      *string `json:"vehicle_photo_file_name"`
	VehiclePhotoFileSize      *int64  `json:"vehicle_photo_file_size"`
	InvoicePhotoFileName      *string `json:"invoice_photo_file_name"`
	InvoicePhotoFileSize      *int64  `json:"invoice_photo_file_size"`
}

// Validate checks PATCH document rules against the current vehicle state.
func (r *VehicleUpdateRequest) Validate(current *Vehicle) error {
	if r.SerialNumberPhotoURL != nil && !IsAllowedGCSURL(*r.SerialNumberPhotoURL) {
		return fmt.Errorf("serial_number_photo_url must be an https GCS URL")
	}
	if r.VehiclePhotoURL != nil && !IsAllowedGCSURL(*r.VehiclePhotoURL) {
		return fmt.Errorf("vehicle_photo_url must be an https GCS URL")
	}
	if r.Color != nil && !IsValidVehicleColor(*r.Color) {
		return fmt.Errorf("invalid color")
	}
	if r.VehicleType != nil && !IsValidVehicleType(*r.VehicleType) {
		return fmt.Errorf("invalid vehicle_type")
	}

	hasInvoice := current.HasInvoice
	if r.HasInvoice != nil {
		hasInvoice = *r.HasInvoice
	}

	if !hasInvoice {
		// Persist null when has_invoice=false (Apêndice A.2).
		r.InvoicePhotoURL = nil
		r.InvoicePhotoFileName = nil
		r.InvoicePhotoFileSize = nil
		return nil
	}

	invoiceURL := current.InvoicePhotoURL
	if r.InvoicePhotoURL != nil {
		invoiceURL = r.InvoicePhotoURL
	}
	if invoiceURL == nil || strings.TrimSpace(*invoiceURL) == "" {
		return fmt.Errorf("invoice_photo_url is required when has_invoice is true")
	}
	if !IsAllowedGCSURL(*invoiceURL) {
		return fmt.Errorf("invoice_photo_url must be an https GCS URL")
	}
	return nil
}
