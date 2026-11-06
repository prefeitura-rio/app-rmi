package pii

import "testing"

func TestMaskCacheKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "empty", key: "", want: ""},
		{name: "bare CPF", key: "12345678901", want: "***"},
		{name: "formatted CPF", key: "123.456.789-01", want: "***"},
		{name: "citizen write buffer", key: "citizen:write:12345678901", want: "citizen:write:***"},
		{name: "memory list by phone", key: "memory_list:5521987654321", want: "memory_list:***"},
		{name: "memory by phone and name", key: "memory:5521987654321:preferencias", want: "memory:***:preferencias"},
		{name: "phone with DDI sign", key: "phone_mapping:+5521987654321", want: "phone_mapping:***"},
		{name: "e-mail segment", key: "invite:fulano@example.com", want: "invite:***"},
		{name: "glob pattern with phone", key: "phone:55219876*", want: "phone:***"},
		{name: "type-only pattern kept", key: "citizen:*", want: "citizen:*"},
		{name: "short numeric id kept", key: "bairro:1234", want: "bairro:1234"},
		{name: "object id kept", key: "avatar:65a1b2c3d4e5f60718293a4b", want: "avatar:65a1b2c3d4e5f60718293a4b"},
		{name: "uuid kept", key: "job:3f2b7c1e-9a4d-4e2b-8c1a-0d9e8f7a6b5c", want: "job:3f2b7c1e-9a4d-4e2b-8c1a-0d9e8f7a6b5c"},
		{name: "no separator, no data", key: "notification_categories", want: "notification_categories"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskCacheKey(tt.key); got != tt.want {
				t.Errorf("MaskCacheKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestMaskCacheKeys(t *testing.T) {
	if got := MaskCacheKeys(nil); got != nil {
		t.Errorf("MaskCacheKeys(nil) = %v, want nil", got)
	}
	got := MaskCacheKeys([]string{"citizen:12345678901", "bairro:1"})
	want := []string{"citizen:***", "bairro:1"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("MaskCacheKeys() = %v, want %v", got, want)
	}
}
