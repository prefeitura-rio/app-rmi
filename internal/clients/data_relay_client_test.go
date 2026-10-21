package clients

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataRelayClient_SendEmail_Success(t *testing.T) {
	var gotKey string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/data/mailman", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		gotKey = r.Header.Get("X-Api-Key")
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewDataRelayClient(srv.URL, "test-api-key", time.Second)
	err := client.SendEmail(context.Background(), &EmailRequest{
		ToAddresses: []string{"joao@example.com"},
		Subject:     "Convite Mobilidade",
		Body:        "Abra a carteira",
		IsHTMLBody:  false,
	})
	require.NoError(t, err)
	assert.Equal(t, "test-api-key", gotKey)
	assert.Equal(t, []interface{}{"joao@example.com"}, gotBody["to_addresses"])
	assert.Equal(t, "Convite Mobilidade", gotBody["subject"])
	assert.Equal(t, false, gotBody["is_html_body"])
}

func TestDataRelayClient_SendEmail_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream"}`))
	}))
	defer srv.Close()

	client := NewDataRelayClient(srv.URL, "key", time.Second)
	err := client.SendEmail(context.Background(), &EmailRequest{
		ToAddresses: []string{"a@example.com"},
		Subject:     "x",
		Body:        "y",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestDataRelayClient_SendEmail_RequiresConfig(t *testing.T) {
	err := NewDataRelayClient("", "key", 0).SendEmail(context.Background(), &EmailRequest{
		ToAddresses: []string{"a@example.com"}, Subject: "s", Body: "b",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base URL")

	err = NewDataRelayClient("https://relay.example.com", "", 0).SendEmail(context.Background(), &EmailRequest{
		ToAddresses: []string{"a@example.com"}, Subject: "s", Body: "b",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

func TestDataRelayClient_Configured(t *testing.T) {
	assert.False(t, (*DataRelayClient)(nil).Configured())
	assert.False(t, NewDataRelayClient("", "k", 0).Configured())
	assert.False(t, NewDataRelayClient("https://x", "", 0).Configured())
	assert.True(t, NewDataRelayClient("https://x/", "k", 0).Configured())
}

func TestDataRelayClient_TrimsTrailingSlash(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	client := NewDataRelayClient(srv.URL+"/", "k", time.Second)
	require.NoError(t, client.SendEmail(context.Background(), &EmailRequest{
		ToAddresses: []string{"a@example.com"}, Subject: "s", Body: "b",
	}))
	assert.Equal(t, "/data/mailman", path)
}
