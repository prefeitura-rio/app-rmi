package utils

import "testing"

func TestMobilidadeSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Caloi", "caloi"},
		{"E-Bike X", "e_bike_x"},
		{"  Mi Electric  ", "mi_electric"},
		{"São Paulo", "sao_paulo"},
		{"A/B", "ab"}, // '/' dropped (not mapped to '_')
		{"", ""},
		{"Ação", "acao"},
		{"Ñandú", "nandu"},
	}
	for _, tc := range cases {
		if got := MobilidadeSlugify(tc.in); got != tc.want {
			t.Errorf("MobilidadeSlugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMobilidadeBrandAndModelIDs(t *testing.T) {
	if got := MobilidadeBrandIDFromName("Monark"); got != "brand_monark" {
		t.Errorf("MobilidadeBrandIDFromName = %q", got)
	}
	if got := MobilidadeModelIDFromBrandAndName("Monark", "E-Bike X"); got != "model_monark_e_bike_x" {
		t.Errorf("MobilidadeModelIDFromBrandAndName = %q", got)
	}
}
