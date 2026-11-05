package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePhoneNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/validate/phone", ValidatePhoneNumber)

	cases := []struct {
		name     string
		body     string
		wantCode int
		want     PhoneValidationResponse
	}{
		{
			name:     "brazilian mobile",
			body:     `{"phone":"+55 21 98765-4321"}`,
			wantCode: http.StatusOK,
			want:     PhoneValidationResponse{Valid: true, Message: "telefone válido", DDI: "55", DDD: "21", Numero: "987654321", E164: "+5521987654321", Region: "BR"},
		},
		{
			name:     "brazilian landline with 8 digits",
			body:     `{"phone":"+552133334444"}`,
			wantCode: http.StatusOK,
			want:     PhoneValidationResponse{Valid: true, Message: "telefone válido", DDI: "55", DDD: "21", Numero: "33334444", E164: "+552133334444", Region: "BR"},
		},
		{
			name:     "international number has no DDD",
			body:     `{"phone":"+1 650 253 0000"}`,
			wantCode: http.StatusOK,
			want:     PhoneValidationResponse{Valid: true, Message: "telefone válido", DDI: "1", DDD: "", Numero: "6502530000", E164: "+16502530000", Region: "US"},
		},
		{
			name:     "too short",
			body:     `{"phone":"+55 21 1234"}`,
			wantCode: http.StatusOK,
			want:     PhoneValidationResponse{Valid: false, Message: "telefone inválido"},
		},
		{
			name:     "not a phone number",
			body:     `{"phone":"telefone"}`,
			wantCode: http.StatusBadRequest,
			want:     PhoneValidationResponse{Valid: false, Message: "formato de telefone inválido"},
		},
		{
			name:     "national number without country code",
			body:     `{"phone":"21987654321"}`,
			wantCode: http.StatusBadRequest,
			want:     PhoneValidationResponse{Valid: false, Message: "formato de telefone inválido"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := serveJSON(t, r, http.MethodPost, "/v1/validate/phone", tc.body)
			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			var got PhoneValidationResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("missing phone field", func(t *testing.T) {
		w := serveJSON(t, r, http.MethodPost, "/v1/validate/phone", `{}`)
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, "campo phone é obrigatório", got.Error)
	})
}
