package models

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestGetAudiences_String(t *testing.T) {
	claims := &JWTClaims{
		AUD: "test-audience",
	}

	result := claims.GetAudiences()
	expected := []string{"test-audience"}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("GetAudiences() with string = %v, want %v", result, expected)
	}
}

func TestGetAudiences_StringSlice(t *testing.T) {
	claims := &JWTClaims{
		AUD: []string{"audience1", "audience2", "audience3"},
	}

	result := claims.GetAudiences()
	expected := []string{"audience1", "audience2", "audience3"}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("GetAudiences() with []string = %v, want %v", result, expected)
	}
}

func TestGetAudiences_InterfaceSlice(t *testing.T) {
	claims := &JWTClaims{
		AUD: []interface{}{"audience1", "audience2"},
	}

	result := claims.GetAudiences()
	expected := []string{"audience1", "audience2"}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("GetAudiences() with []interface{} = %v, want %v", result, expected)
	}
}

func TestGetAudiences_EmptyInterfaceSlice(t *testing.T) {
	claims := &JWTClaims{
		AUD: []interface{}{},
	}

	result := claims.GetAudiences()
	expected := []string{}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("GetAudiences() with empty []interface{} = %v, want %v", result, expected)
	}
}

func TestGetAudiences_Nil(t *testing.T) {
	claims := &JWTClaims{
		AUD: nil,
	}

	result := claims.GetAudiences()
	expected := []string{}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("GetAudiences() with nil = %v, want %v", result, expected)
	}
}

func TestGetAudiences_UnsupportedType(t *testing.T) {
	claims := &JWTClaims{
		AUD: 12345, // Integer, not supported
	}

	result := claims.GetAudiences()
	expected := []string{}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("GetAudiences() with unsupported type = %v, want %v", result, expected)
	}
}

func TestGetAudiences_MixedInterfaceSlice(t *testing.T) {
	// Test with mixed types in []interface{} (e.g., contains non-string)
	claims := &JWTClaims{
		AUD: []interface{}{"audience1", 123, "audience2"},
	}

	result := claims.GetAudiences()
	// Non-string values should be converted to empty strings
	expected := []string{"audience1", "", "audience2"}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("GetAudiences() with mixed types = %v, want %v", result, expected)
	}
}

// Regression: real Keycloak tokens key resource_access on the fully-qualified
// client id (e.g. "superapp.apps.rio.gov.br"), not the short "superapp". A fixed
// struct hardcoded to "superapp" silently dropped these roles, so admins were
// rejected. HasRole must find roles under any client id.
func TestHasRole_QualifiedResourceClientID(t *testing.T) {
	payload := `{
		"realm_access": {"roles": ["carioca-rio"]},
		"resource_access": {
			"superapp.apps.rio.gov.br": {"roles": ["heimdall-admin"]},
			"broker": {"roles": ["read-token"]}
		}
	}`

	var claims JWTClaims
	if err := json.Unmarshal([]byte(payload), &claims); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if !claims.HasResourceRole("heimdall-admin") {
		t.Error("HasResourceRole(\"heimdall-admin\") = false, want true")
	}
	if !claims.HasRole("heimdall-admin") {
		t.Error("HasRole(\"heimdall-admin\") = false, want true")
	}
	if !claims.HasRole("carioca-rio") {
		t.Error("HasRole(\"carioca-rio\") = false, want true (realm role)")
	}
	if claims.HasRole("nonexistent") {
		t.Error("HasRole(\"nonexistent\") = true, want false")
	}
}
