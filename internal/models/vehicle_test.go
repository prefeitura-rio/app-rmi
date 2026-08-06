package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(v bool) *bool { return &v }

func TestIsValidVehicleType(t *testing.T) {
	assert.True(t, IsValidVehicleType(VehicleTypeBicicletaEletrica))
	assert.True(t, IsValidVehicleType(VehicleTypeAutopropelido))
	assert.True(t, IsValidVehicleType(VehicleTypeCiclomotor))
	assert.False(t, IsValidVehicleType("carro"))
	assert.False(t, IsValidVehicleType(""))
}

func TestVehicleColors_ContainsExpectedValues(t *testing.T) {
	colors := VehicleColors()
	require.Len(t, colors, 23)
	assert.True(t, IsValidVehicleColor("Preto"))
	assert.True(t, IsValidVehicleColor("Verde Escuro"))
	assert.False(t, IsValidVehicleColor("Turquesa"))
}

func TestIsAllowedGCSURL(t *testing.T) {
	assert.True(t, IsAllowedGCSURL("https://storage.googleapis.com/bucket/nf.pdf"))
	assert.True(t, IsAllowedGCSURL("https://storage.cloud.google.com/bucket/photo.jpg"))
	assert.True(t, IsAllowedGCSURL("https://my-bucket.storage.googleapis.com/path/a.png"))
	assert.False(t, IsAllowedGCSURL("https://example.com/s.jpg"))
	assert.False(t, IsAllowedGCSURL("blob:https://pref.rio/abc"))
	assert.False(t, IsAllowedGCSURL("data:image/png;base64,xxx"))
	assert.False(t, IsAllowedGCSURL("http://storage.googleapis.com/insecure"))
	assert.False(t, IsAllowedGCSURL(""))
}

func TestVehicleCreateRequest_Validate_CatalogFlow(t *testing.T) {
	brandID := "brand_caloi"
	modelID := "model_e-vibe"
	invoiceURL := "https://storage.googleapis.com/bucket/nf.pdf"
	req := VehicleCreateRequest{
		DisplayName:          "Bike do trabalho",
		BrandID:              &brandID,
		ModelID:              &modelID,
		Color:                "Preto",
		SerialNumber:         "SN-1",
		SerialNumberPhotoURL: "https://storage.googleapis.com/serial.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/vehicle.jpg",
		HasInvoice:           boolPtr(true),
		InvoicePhotoURL:      &invoiceURL,
		SelfDeclaration:      true,
	}
	assert.NoError(t, req.Validate())
}

func TestVehicleCreateRequest_Validate_OtherFlow(t *testing.T) {
	brandOther := "Xiaomi"
	modelOther := "Mi Electric Scooter 4"
	vt := VehicleTypeAutopropelido
	req := VehicleCreateRequest{
		DisplayName:          "Meu patinete",
		BrandOther:           &brandOther,
		ModelOther:           &modelOther,
		VehicleType:          &vt,
		Color:                "Branco",
		SerialNumber:         "XM-1",
		SerialNumberPhotoURL: "https://storage.googleapis.com/serial.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/vehicle.jpg",
		HasInvoice:           boolPtr(false),
		SelfDeclaration:      true,
	}
	assert.NoError(t, req.Validate())
	assert.Nil(t, req.InvoicePhotoURL)
}

func TestVehicleCreateRequest_Validate_HasInvoiceRequiresURL(t *testing.T) {
	brandID := "brand_caloi"
	modelID := "model_e-vibe"
	req := VehicleCreateRequest{
		DisplayName:          "Bike",
		BrandID:              &brandID,
		ModelID:              &modelID,
		Color:                "Preto",
		SerialNumber:         "SN-1",
		SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/v.jpg",
		HasInvoice:           boolPtr(true),
		SelfDeclaration:      true,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invoice_photo_url")
}

func TestVehicleCreateRequest_Validate_ClearsInvoiceWhenNoNF(t *testing.T) {
	brandID := "brand_caloi"
	modelID := "model_e-vibe"
	junk := "https://storage.googleapis.com/bucket/should-be-cleared.pdf"
	name := "nf.pdf"
	size := int64(1024)
	req := VehicleCreateRequest{
		DisplayName:          "Bike",
		BrandID:              &brandID,
		ModelID:              &modelID,
		Color:                "Preto",
		SerialNumber:         "SN-1",
		SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/v.jpg",
		HasInvoice:           boolPtr(false),
		InvoicePhotoURL:      &junk,
		InvoicePhotoFileName: &name,
		InvoicePhotoFileSize: &size,
		SelfDeclaration:      true,
	}
	require.NoError(t, req.Validate())
	assert.Nil(t, req.InvoicePhotoURL)
	assert.Nil(t, req.InvoicePhotoFileName)
	assert.Nil(t, req.InvoicePhotoFileSize)
}

func TestVehicleCreateRequest_Validate_RejectsNonGCSPhotoURL(t *testing.T) {
	brandID := "brand_caloi"
	modelID := "model_e-vibe"
	req := VehicleCreateRequest{
		DisplayName:          "Bike",
		BrandID:              &brandID,
		ModelID:              &modelID,
		Color:                "Preto",
		SerialNumber:         "SN-1",
		SerialNumberPhotoURL: "https://example.com/s.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/v.jpg",
		HasInvoice:           boolPtr(false),
		SelfDeclaration:      true,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serial_number_photo_url")
}

func TestVehicleCreateRequest_Validate_SelfDeclarationRequired(t *testing.T) {
	brandID := "brand_caloi"
	modelID := "model_e-vibe"
	req := VehicleCreateRequest{
		DisplayName:          "Bike",
		BrandID:              &brandID,
		ModelID:              &modelID,
		Color:                "Preto",
		SerialNumber:         "SN-1",
		SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/v.jpg",
		HasInvoice:           boolPtr(false),
		SelfDeclaration:      false,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "self_declaration")
}

func TestVehicleCreateRequest_Validate_OtherFlowRequiresType(t *testing.T) {
	brandOther := "Xiaomi"
	modelOther := "Mi 4"
	req := VehicleCreateRequest{
		DisplayName:          "Patinete",
		BrandOther:           &brandOther,
		ModelOther:           &modelOther,
		Color:                "Branco",
		SerialNumber:         "XM-1",
		SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/v.jpg",
		HasInvoice:           boolPtr(false),
		SelfDeclaration:      true,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vehicle_type")
}

func TestVehicleCreateRequest_Validate_InvalidColor(t *testing.T) {
	brandID := "brand_caloi"
	modelID := "model_e-vibe"
	req := VehicleCreateRequest{
		DisplayName:          "Bike",
		BrandID:              &brandID,
		ModelID:              &modelID,
		Color:                "Turquesa",
		SerialNumber:         "SN-1",
		SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/v.jpg",
		HasInvoice:           boolPtr(false),
		SelfDeclaration:      true,
	}
	require.Error(t, req.Validate())
}

func TestVehicleUpdateRequest_Validate_InvoiceRules(t *testing.T) {
	invoiceURL := "https://storage.googleapis.com/bucket/nf.pdf"
	current := &Vehicle{
		HasInvoice:      true,
		InvoicePhotoURL: &invoiceURL,
	}

	t.Run("clear when has_invoice false", func(t *testing.T) {
		falseVal := false
		junk := "https://storage.googleapis.com/bucket/ignore.pdf"
		req := &VehicleUpdateRequest{HasInvoice: &falseVal, InvoicePhotoURL: &junk}
		require.NoError(t, req.Validate(current))
		assert.Nil(t, req.InvoicePhotoURL)
	})

	t.Run("require url when enabling invoice", func(t *testing.T) {
		trueVal := true
		noInvoice := &Vehicle{HasInvoice: false}
		req := &VehicleUpdateRequest{HasInvoice: &trueVal}
		err := req.Validate(noInvoice)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invoice_photo_url")
	})

	t.Run("accept new gcs invoice url", func(t *testing.T) {
		newURL := "https://storage.googleapis.com/bucket/new-nf.pdf"
		req := &VehicleUpdateRequest{InvoicePhotoURL: &newURL}
		require.NoError(t, req.Validate(current))
	})

	t.Run("reject non-gcs photo url", func(t *testing.T) {
		bad := "blob:https://pref.rio/x"
		req := &VehicleUpdateRequest{VehiclePhotoURL: &bad}
		err := req.Validate(current)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vehicle_photo_url")
	})
}

func TestRespondInvitationRequest_Validate(t *testing.T) {
	assert.NoError(t, (&RespondInvitationRequest{Status: InvitationResponseAccepted}).Validate())
	assert.NoError(t, (&RespondInvitationRequest{Status: InvitationResponseRejected}).Validate())
	assert.Error(t, (&RespondInvitationRequest{Status: InvitationResponseStatus(ConductorStatusPending)}).Validate())
	assert.Error(t, (&RespondInvitationRequest{Status: InvitationResponseStatus(ConductorStatusRevoked)}).Validate())
}

func TestInviteConductorRequest_Validate(t *testing.T) {
	assert.NoError(t, (&InviteConductorRequest{CPF: "x", Email: "a@b.co"}).Validate())
	assert.Error(t, (&InviteConductorRequest{CPF: "x", Email: ""}).Validate())
	assert.Error(t, (&InviteConductorRequest{CPF: "x", Email: "not-an-email"}).Validate())
}

func TestVehicleListItem_JSONSnakeCase(t *testing.T) {
	brandID := "brand_caloi"
	item := VehicleListItem{
		ID:          "abc",
		DisplayName: "Bike",
		BrandID:     &brandID,
		VehicleType: VehicleTypeBicicletaEletrica,
		Color:       "Preto",
		Role:        VehicleRoleOwner,
	}
	b, err := json.Marshal(item)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"display_name"`)
	assert.Contains(t, string(b), `"vehicle_type"`)
	assert.Contains(t, string(b), `"brand_id"`)
	assert.NotContains(t, string(b), `"DisplayName"`)
}

func TestVehicle_JSONIncludesNullInvoiceURL(t *testing.T) {
	v := Vehicle{
		DisplayName: "Bike",
		HasInvoice:  false,
	}
	b, err := json.Marshal(v)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"invoice_photo_url":null`)
}
