package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestSelfDeclaredValidation_DoesNotRecordSubmittedValueInSpans checks that a
// self-declared value never reaches the trace: not as a span attribute
// (invalid_value, validated_value, new_<field>) nor inside the recorded error
// (exception.message). The submitted text is typed by the citizen (LGPD).
func TestSelfDeclaredValidation_DoesNotRecordSubmittedValueInSpans(t *testing.T) {
	const sentinel = "valor-sintetico-dev352"

	tests := []struct {
		name       string
		path       string
		handler    gin.HandlerFunc
		valor      string
		wantStatus int
	}{
		{"invalid ethnicity", "ethnicity", UpdateSelfDeclaredRaca, sentinel, http.StatusBadRequest},
		{"invalid family income", "family-income", UpdateSelfDeclaredRendaFamiliar, sentinel, http.StatusBadRequest},
		{"invalid education", "education", UpdateSelfDeclaredEscolaridade, sentinel, http.StatusBadRequest},
		{"invalid disability", "disability", UpdateSelfDeclaredDeficiencia, sentinel, http.StatusBadRequest},
		{"exhibition name too long", "exhibition-name", UpdateSelfDeclaredNomeExibicao, strings.Repeat(sentinel, 15), http.StatusBadRequest},
		{"free-text gender accepted", "gender", UpdateSelfDeclaredGenero, sentinel, http.StatusOK},
		{"invalid email format", "email", UpdateSelfDeclaredEmail, sentinel + "@dominio-sem-tld", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			defer func() {
				otel.SetTracerProvider(previous)
				_ = provider.Shutdown(context.Background())
			}()

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.PUT("/v1/citizen/:cpf/"+tt.path, tt.handler)

			jsonBody, err := json.Marshal(map[string]string{"valor": tt.valor})
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			req := httptest.NewRequest(http.MethodPut, "/v1/citizen/"+cpfTest+"/"+tt.path, bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}

			ended := recorder.Ended()
			var validation sdktrace.ReadOnlySpan
			for _, span := range ended {
				if span.Name() == "endpoint.step.validate_input" {
					validation = span
				}
				for _, kv := range span.Attributes() {
					if strings.Contains(kv.Value.Emit(), sentinel) {
						t.Errorf("span %q attribute %q records the submitted value", span.Name(), kv.Key)
					}
				}
				for _, event := range span.Events() {
					for _, kv := range event.Attributes {
						if strings.Contains(kv.Value.Emit(), sentinel) {
							t.Errorf("span %q event %q attribute %q records the submitted value", span.Name(), event.Name, kv.Key)
						}
					}
				}
			}
			if validation == nil {
				t.Fatalf("validation span not recorded; spans: %d", len(ended))
			}
			if tt.wantStatus != http.StatusBadRequest {
				return
			}

			attrs := make(map[string]string)
			for _, kv := range validation.Attributes() {
				attrs[string(kv.Key)] = kv.Value.Emit()
			}
			for _, key := range []string{"invalid_value", "email"} {
				if v, ok := attrs[key]; ok {
					t.Errorf("validation span records %s = %q", key, v)
				}
			}
			if got, want := attrs["invalid_value_length"], strconv.Itoa(len(tt.valor)); got != want {
				t.Errorf("invalid_value_length = %q, want %q", got, want)
			}
			if len(validation.Events()) == 0 {
				t.Errorf("validation error not recorded in the span")
			}
		})
	}
}
