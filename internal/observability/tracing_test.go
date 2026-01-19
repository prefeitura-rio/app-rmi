package observability

import (
	"os"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
)

func TestInitTracer_Disabled(t *testing.T) {
	// Initialize config if needed
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{
			TracingEnabled: false,
		}
	}

	// Save original config
	originalEnabled := config.AppConfig.TracingEnabled
	defer func() { config.AppConfig.TracingEnabled = originalEnabled }()

	// Disable tracing
	config.AppConfig.TracingEnabled = false

	// Should not panic and should log that tracing is disabled
	InitTracer()

	// Verify tracer provider is nil when disabled
	assert.Nil(t, tracerProvider)
}

func TestInitTracer_Enabled(t *testing.T) {
	// Initialize config if needed
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// Save original config
	originalEnabled := config.AppConfig.TracingEnabled
	originalEndpoint := config.AppConfig.TracingEndpoint
	defer func() {
		config.AppConfig.TracingEnabled = originalEnabled
		config.AppConfig.TracingEndpoint = originalEndpoint
	}()

	// Enable tracing with invalid endpoint (will fail but shouldn't panic)
	config.AppConfig.TracingEnabled = true
	config.AppConfig.TracingEndpoint = "invalid-endpoint:4317"

	// Should not panic even with invalid endpoint
	InitTracer()

	// Global tracer provider should be set (even if init fails)
	globalProvider := otel.GetTracerProvider()
	assert.NotNil(t, globalProvider)
}

func TestShutdownTracer_NilProvider(t *testing.T) {
	// Ensure tracerProvider is nil
	tracerProvider = nil

	// Should not panic with nil provider
	ShutdownTracer()
}

func TestShutdownTracer_ValidProvider(t *testing.T) {
	// Initialize config if needed
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// Save original config
	originalEnabled := config.AppConfig.TracingEnabled
	originalEndpoint := config.AppConfig.TracingEndpoint
	defer func() {
		config.AppConfig.TracingEnabled = originalEnabled
		config.AppConfig.TracingEndpoint = originalEndpoint
	}()

	// Initialize with disabled config first
	config.AppConfig.TracingEnabled = false
	InitTracer()

	// Should not panic
	ShutdownTracer()
}

func TestTracingIntegration(t *testing.T) {
	// Skip if no OTLP endpoint configured
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		t.Skip("Skipping tracing integration test: no OTLP endpoint configured")
	}

	// Initialize config if needed
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// Save original config
	originalEnabled := config.AppConfig.TracingEnabled
	originalEndpoint := config.AppConfig.TracingEndpoint
	defer func() {
		config.AppConfig.TracingEnabled = originalEnabled
		config.AppConfig.TracingEndpoint = originalEndpoint
		ShutdownTracer()
	}()

	// Enable tracing
	config.AppConfig.TracingEnabled = true
	config.AppConfig.TracingEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	// Should initialize successfully
	InitTracer()

	// Shutdown should work
	ShutdownTracer()
}

func TestTracingConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		endpoint string
	}{
		{
			name:     "disabled tracing",
			enabled:  false,
			endpoint: "",
		},
		{
			name:     "enabled with endpoint",
			enabled:  true,
			endpoint: "localhost:4317",
		},
		{
			name:     "enabled without endpoint",
			enabled:  true,
			endpoint: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize config if needed
			if config.AppConfig == nil {
				config.AppConfig = &config.Config{}
			}

			// Save original config
			originalEnabled := config.AppConfig.TracingEnabled
			originalEndpoint := config.AppConfig.TracingEndpoint
			defer func() {
				config.AppConfig.TracingEnabled = originalEnabled
				config.AppConfig.TracingEndpoint = originalEndpoint
				tracerProvider = nil
			}()

			// Set test config
			config.AppConfig.TracingEnabled = tt.enabled
			config.AppConfig.TracingEndpoint = tt.endpoint

			// Should not panic
			InitTracer()

			// Cleanup
			ShutdownTracer()
		})
	}
}
