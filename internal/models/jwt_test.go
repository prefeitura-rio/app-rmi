package models

import (
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
