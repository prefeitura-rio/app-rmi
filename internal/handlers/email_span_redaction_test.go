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

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestValidateEmailAddress_DoesNotRecordAddressInSpans checks that the submitted
// e-mail address (personal data) never reaches the trace: not as an attribute
// nor inside a recorded error, where net/mail echoes the unparsed text.
func TestValidateEmailAddress_DoesNotRecordAddressInSpans(t *testing.T) {
	const sentinel = "pessoa-sintetica-dev377"

	tests := []struct {
		name      string
		email     string
		wantValid bool
	}{
		{"valid address", sentinel + "@exemplo.com", true},
		{"unparseable address", "a@exemplo.com " + sentinel, false},
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

			router, cleanup := setupEmailHandlersTest(t)
			defer cleanup()

			body, err := json.Marshal(EmailValidationRequest{Email: tt.email})
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
			}
			var resp EmailValidationResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.Valid != tt.wantValid {
				t.Fatalf("valid = %v, want %v (message %q)", resp.Valid, tt.wantValid, resp.Message)
			}

			ended := recorder.Ended()
			var root, parse sdktrace.ReadOnlySpan
			for _, span := range ended {
				switch span.Name() {
				case "ValidateEmailAddress":
					root = span
				case "endpoint.step.business_logic":
					for _, kv := range span.Attributes() {
						if string(kv.Key) == "logic.type" && kv.Value.AsString() == "parse_email_address" {
							parse = span
						}
					}
				}
				for _, kv := range span.Attributes() {
					if strings.Contains(kv.Value.Emit(), sentinel) {
						t.Errorf("span %q attribute %q records the e-mail address", span.Name(), kv.Key)
					}
				}
				for _, event := range span.Events() {
					for _, kv := range event.Attributes {
						if strings.Contains(kv.Value.Emit(), sentinel) {
							t.Errorf("span %q event %q attribute %q records the e-mail address", span.Name(), event.Name, kv.Key)
						}
					}
				}
			}
			if root == nil {
				t.Fatalf("root span not recorded; spans: %d", len(ended))
			}

			rootAttrs := make(map[string]string)
			for _, kv := range root.Attributes() {
				rootAttrs[string(kv.Key)] = kv.Value.Emit()
			}
			if v, ok := rootAttrs["input.email_prefix"]; ok {
				t.Errorf("root span records input.email_prefix = %q", v)
			}
			if got, want := rootAttrs["input.email_length"], strconv.Itoa(len(tt.email)); got != want {
				t.Errorf("input.email_length = %q, want %q", got, want)
			}

			if tt.wantValid {
				return
			}
			if parse == nil {
				t.Fatalf("parse span not recorded")
			}
			if len(parse.Events()) == 0 {
				t.Errorf("parse error not recorded in the span")
			}
		})
	}
}
