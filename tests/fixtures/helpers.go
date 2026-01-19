package fixtures

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertStatusCode checks HTTP response status code
func AssertStatusCode(t *testing.T, resp *http.Response, expectedStatus int) {
	t.Helper()
	if resp.StatusCode != expectedStatus {
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, expectedStatus, resp.StatusCode,
			"Unexpected status code. Response body: %s", string(body))
	}
}

// AssertJSONResponse validates response is valid JSON and returns parsed body
func AssertJSONResponse(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err, "Response is not valid JSON: %s", string(body))

	return result
}

// AssertHealthy checks if health endpoint returns healthy status
func AssertHealthy(t *testing.T, client *APIClient) {
	t.Helper()

	resp, err := client.Get("/health")
	require.NoError(t, err, "Failed to call health endpoint")
	defer resp.Body.Close()

	AssertStatusCode(t, resp, http.StatusOK)

	body := AssertJSONResponse(t, resp)
	status, ok := body["status"].(string)
	require.True(t, ok, "Health response missing 'status' field")
	assert.Equal(t, "healthy", status, "Service is not healthy")
}

// AssertFieldExists checks if a field exists in JSON response
func AssertFieldExists(t *testing.T, data map[string]interface{}, field string) {
	t.Helper()
	_, exists := data[field]
	assert.True(t, exists, "Expected field '%s' not found in response", field)
}

// AssertFieldValue checks if a field has expected value
func AssertFieldValue(t *testing.T, data map[string]interface{}, field string, expected interface{}) {
	t.Helper()
	actual, exists := data[field]
	require.True(t, exists, "Field '%s' not found in response", field)
	assert.Equal(t, expected, actual, "Field '%s' has unexpected value", field)
}

// PrintResponseBody prints response body for debugging (only on test failure)
func PrintResponseBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if t.Failed() {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Response body: %s", string(body))
	}
}

// WaitForHealthy polls health endpoint until service is ready or timeout
func WaitForHealthy(t *testing.T, client *APIClient, maxAttempts int) error {
	t.Helper()

	for i := 0; i < maxAttempts; i++ {
		resp, err := client.Get("/health")
		if err != nil {
			t.Logf("Health check attempt %d/%d failed: %v", i+1, maxAttempts, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var healthResp map[string]interface{}
			resp, _ := client.Get("/health")
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			json.Unmarshal(body, &healthResp)

			if status, ok := healthResp["status"].(string); ok && status == "healthy" {
				t.Logf("Service healthy after %d attempts", i+1)
				return nil
			}
		}

		if i < maxAttempts-1 {
			t.Logf("Service not healthy yet, retrying... (%d/%d)", i+1, maxAttempts)
		}
	}

	return fmt.Errorf("service did not become healthy after %d attempts", maxAttempts)
}

// CompareJSONFields compares specific fields between two JSON objects
func CompareJSONFields(t *testing.T, original, updated map[string]interface{}, fields []string) {
	t.Helper()

	for _, field := range fields {
		originalVal, originalExists := original[field]
		updatedVal, updatedExists := updated[field]

		require.True(t, originalExists, "Field '%s' missing in original data", field)
		require.True(t, updatedExists, "Field '%s' missing in updated data", field)

		assert.Equal(t, originalVal, updatedVal,
			"Field '%s' changed unexpectedly (original: %v, updated: %v)",
			field, originalVal, updatedVal)
	}
}
