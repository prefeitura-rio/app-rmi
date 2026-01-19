package config

import (
	"os"
	"testing"
	"time"
)

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		setEnv       bool
		want         string
	}{
		{
			name:         "environment variable set",
			key:          "TEST_KEY_1",
			defaultValue: "default",
			envValue:     "custom",
			setEnv:       true,
			want:         "custom",
		},
		{
			name:         "environment variable not set",
			key:          "TEST_KEY_2",
			defaultValue: "default",
			envValue:     "",
			setEnv:       false,
			want:         "default",
		},
		{
			name:         "empty environment variable",
			key:          "TEST_KEY_3",
			defaultValue: "default",
			envValue:     "",
			setEnv:       true,
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvAsIntOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		setEnv       bool
		want         int
	}{
		{
			name:         "valid integer",
			key:          "TEST_INT_1",
			defaultValue: 10,
			envValue:     "42",
			setEnv:       true,
			want:         42,
		},
		{
			name:         "not set",
			key:          "TEST_INT_2",
			defaultValue: 10,
			envValue:     "",
			setEnv:       false,
			want:         10,
		},
		{
			name:         "invalid integer",
			key:          "TEST_INT_3",
			defaultValue: 10,
			envValue:     "invalid",
			setEnv:       true,
			want:         10,
		},
		{
			name:         "negative integer",
			key:          "TEST_INT_4",
			defaultValue: 10,
			envValue:     "-5",
			setEnv:       true,
			want:         -5,
		},
		{
			name:         "zero",
			key:          "TEST_INT_5",
			defaultValue: 10,
			envValue:     "0",
			setEnv:       true,
			want:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvAsIntOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvAsIntOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvAsDurationOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue time.Duration
		envValue     string
		setEnv       bool
		want         time.Duration
	}{
		{
			name:         "valid duration",
			key:          "TEST_DUR_1",
			defaultValue: 10 * time.Second,
			envValue:     "5m",
			setEnv:       true,
			want:         5 * time.Minute,
		},
		{
			name:         "not set",
			key:          "TEST_DUR_2",
			defaultValue: 10 * time.Second,
			envValue:     "",
			setEnv:       false,
			want:         10 * time.Second,
		},
		{
			name:         "invalid duration",
			key:          "TEST_DUR_3",
			defaultValue: 10 * time.Second,
			envValue:     "invalid",
			setEnv:       true,
			want:         10 * time.Second,
		},
		{
			name:         "hours",
			key:          "TEST_DUR_4",
			defaultValue: 10 * time.Second,
			envValue:     "2h",
			setEnv:       true,
			want:         2 * time.Hour,
		},
		{
			name:         "milliseconds",
			key:          "TEST_DUR_5",
			defaultValue: 10 * time.Second,
			envValue:     "500ms",
			setEnv:       true,
			want:         500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvAsDurationOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvAsDurationOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCommaSeparatedList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{
			name:  "simple list",
			value: "a,b,c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "list with spaces",
			value: "a, b, c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "single item",
			value: "single",
			want:  []string{"single"},
		},
		{
			name:  "empty string",
			value: "",
			want:  []string{},
		},
		{
			name:  "only commas",
			value: ",,,",
			want:  []string{},
		},
		{
			name:  "trailing comma",
			value: "a,b,",
			want:  []string{"a", "b"},
		},
		{
			name:  "leading comma",
			value: ",a,b",
			want:  []string{"a", "b"},
		},
		{
			name:  "multiple spaces",
			value: "a  ,  b  ,  c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "redis cluster addresses",
			value: "redis1:6379,redis2:6379,redis3:6379",
			want:  []string{"redis1:6379", "redis2:6379", "redis3:6379"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaSeparatedList(tt.value)
			if len(got) != len(tt.want) {
				t.Errorf("parseCommaSeparatedList() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseCommaSeparatedList()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestConfig_Structure(t *testing.T) {
	cfg := &Config{
		Port:        8080,
		Environment: "test",
		MongoURI:    "mongodb://localhost:27017",
	}

	if cfg.Port != 8080 {
		t.Errorf("Config.Port = %v, want 8080", cfg.Port)
	}

	if cfg.Environment != "test" {
		t.Errorf("Config.Environment = %v, want test", cfg.Environment)
	}

	if cfg.MongoURI != "mongodb://localhost:27017" {
		t.Errorf("Config.MongoURI = %v, want mongodb://localhost:27017", cfg.MongoURI)
	}
}

func TestGetEnvOrDefault_Concurrent(t *testing.T) {
	key := "TEST_CONCURRENT"
	defaultValue := "default"

	os.Setenv(key, "value")
	defer os.Unsetenv(key)

	// Run concurrent reads
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			result := getEnvOrDefault(key, defaultValue)
			if result != "value" {
				t.Errorf("getEnvOrDefault() = %v, want value", result)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestParseCommaSeparatedList_EmptyItems(t *testing.T) {
	value := "a,,b,,c"
	got := parseCommaSeparatedList(value)

	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Errorf("parseCommaSeparatedList() length = %v, want %v", len(got), len(want))
	}
}

func TestGetEnvAsIntOrDefault_LargeNumber(t *testing.T) {
	key := "TEST_LARGE_INT"
	os.Setenv(key, "2147483647") // max int32
	defer os.Unsetenv(key)

	got := getEnvAsIntOrDefault(key, 0)
	if got != 2147483647 {
		t.Errorf("getEnvAsIntOrDefault() = %v, want 2147483647", got)
	}
}

func TestGetEnvAsDurationOrDefault_Complex(t *testing.T) {
	key := "TEST_COMPLEX_DUR"
	os.Setenv(key, "1h30m45s")
	defer os.Unsetenv(key)

	expected := 1*time.Hour + 30*time.Minute + 45*time.Second
	got := getEnvAsDurationOrDefault(key, 0)
	if got != expected {
		t.Errorf("getEnvAsDurationOrDefault() = %v, want %v", got, expected)
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	// Test that Config struct can be initialized with zero values
	cfg := &Config{}

	if cfg.Port != 0 {
		t.Errorf("Config.Port default = %v, want 0", cfg.Port)
	}

	if cfg.RedisDB != 0 {
		t.Errorf("Config.RedisDB default = %v, want 0", cfg.RedisDB)
	}

	if cfg.RedisTTL != 0 {
		t.Errorf("Config.RedisTTL default = %v, want 0", cfg.RedisTTL)
	}
}
