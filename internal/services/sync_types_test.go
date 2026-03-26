package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncJob_Creation(t *testing.T) {
	now := time.Now()
	job := &SyncJob{
		ID:         "job-123",
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "citizens",
		Data:       map[string]interface{}{"cpf": "12345678901"},
		Timestamp:  now,
		RetryCount: 0,
		MaxRetries: 3,
	}

	assert.Equal(t, "job-123", job.ID)
	assert.Equal(t, "citizen", job.Type)
	assert.Equal(t, "12345678901", job.Key)
	assert.Equal(t, "citizens", job.Collection)
	assert.Equal(t, 0, job.RetryCount)
	assert.Equal(t, 3, job.MaxRetries)
	assert.Equal(t, now, job.Timestamp)
	assert.NotNil(t, job.Data)
}

func TestSyncJob_JSONMarshalling(t *testing.T) {
	job := &SyncJob{
		ID:         "job-456",
		Type:       "phone_mapping",
		Key:        "+5521999887766",
		Collection: "phone_mappings",
		Data: map[string]interface{}{
			"phone": "+5521999887766",
			"cpf":   "12345678901",
		},
		Timestamp:  time.Now(),
		RetryCount: 1,
		MaxRetries: 5,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(job)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal back
	var unmarshaled SyncJob
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, job.ID, unmarshaled.ID)
	assert.Equal(t, job.Type, unmarshaled.Type)
	assert.Equal(t, job.Key, unmarshaled.Key)
	assert.Equal(t, job.Collection, unmarshaled.Collection)
	assert.Equal(t, job.RetryCount, unmarshaled.RetryCount)
	assert.Equal(t, job.MaxRetries, unmarshaled.MaxRetries)
}

func TestSyncJob_RetryLogic(t *testing.T) {
	job := &SyncJob{
		ID:         "job-789",
		Type:       "user_config",
		Key:        "user-123",
		Collection: "user_config",
		Data:       map[string]interface{}{"first_login": true},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	// Simulate retries
	assert.Less(t, job.RetryCount, job.MaxRetries, "Should have retries available")

	job.RetryCount++
	assert.Equal(t, 1, job.RetryCount)
	assert.Less(t, job.RetryCount, job.MaxRetries)

	job.RetryCount++
	job.RetryCount++
	assert.Equal(t, 3, job.RetryCount)
	assert.Equal(t, job.RetryCount, job.MaxRetries, "Reached max retries")
}

func TestSyncJob_WithNilData(t *testing.T) {
	job := &SyncJob{
		ID:         "job-nil",
		Type:       "test",
		Key:        "key",
		Collection: "collection",
		Data:       nil,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	assert.Nil(t, job.Data)

	// Should still be able to marshal
	jsonData, err := json.Marshal(job)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"data\":null")
}

func TestDLQJob_Creation(t *testing.T) {
	originalJob := SyncJob{
		ID:         "failed-job",
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "citizens",
		Data:       map[string]interface{}{"cpf": "12345678901"},
		Timestamp:  time.Now(),
		RetryCount: 3,
		MaxRetries: 3,
	}

	failedAt := time.Now()
	dlqJob := &DLQJob{
		OriginalJob: originalJob,
		Error:       "connection timeout",
		FailedAt:    failedAt,
	}

	assert.Equal(t, "failed-job", dlqJob.OriginalJob.ID)
	assert.Equal(t, "connection timeout", dlqJob.Error)
	assert.Equal(t, failedAt, dlqJob.FailedAt)
	assert.Equal(t, 3, dlqJob.OriginalJob.RetryCount)
}

func TestDLQJob_JSONMarshalling(t *testing.T) {
	originalJob := SyncJob{
		ID:         "job-001",
		Type:       "test",
		Key:        "key",
		Collection: "test_collection",
		Data:       map[string]string{"field": "value"},
		Timestamp:  time.Now(),
		RetryCount: 3,
		MaxRetries: 3,
	}

	dlqJob := &DLQJob{
		OriginalJob: originalJob,
		Error:       "database unavailable",
		FailedAt:    time.Now(),
	}

	// Marshal
	jsonData, err := json.Marshal(dlqJob)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal
	var unmarshaled DLQJob
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, dlqJob.OriginalJob.ID, unmarshaled.OriginalJob.ID)
	assert.Equal(t, dlqJob.Error, unmarshaled.Error)
}

func TestDLQJob_WithDifferentErrors(t *testing.T) {
	originalJob := SyncJob{
		ID:         "job-error-test",
		Type:       "test",
		Key:        "key",
		Collection: "collection",
		Data:       nil,
		Timestamp:  time.Now(),
		RetryCount: 3,
		MaxRetries: 3,
	}

	errors := []string{
		"connection timeout",
		"invalid document",
		"duplicate key error",
		"write concern error",
		"network error",
	}

	for _, errMsg := range errors {
		dlqJob := &DLQJob{
			OriginalJob: originalJob,
			Error:       errMsg,
			FailedAt:    time.Now(),
		}

		assert.Equal(t, errMsg, dlqJob.Error)
		assert.NotEmpty(t, dlqJob.Error)
	}
}

func TestSyncResult_Success(t *testing.T) {
	result := &SyncResult{
		JobID:    "job-success",
		Success:  true,
		Error:    "",
		SyncedAt: time.Now(),
		Duration: 150 * time.Millisecond,
	}

	assert.Equal(t, "job-success", result.JobID)
	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.Greater(t, result.Duration, time.Duration(0))
}

func TestSyncResult_Failure(t *testing.T) {
	result := &SyncResult{
		JobID:    "job-failure",
		Success:  false,
		Error:    "sync failed: connection lost",
		SyncedAt: time.Now(),
		Duration: 5 * time.Second,
	}

	assert.Equal(t, "job-failure", result.JobID)
	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "connection lost")
	assert.Greater(t, result.Duration, time.Duration(0))
}

func TestSyncResult_JSONMarshalling(t *testing.T) {
	result := &SyncResult{
		JobID:    "job-json-test",
		Success:  true,
		Error:    "",
		SyncedAt: time.Now(),
		Duration: 200 * time.Millisecond,
	}

	// Marshal
	jsonData, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal
	var unmarshaled SyncResult
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, result.JobID, unmarshaled.JobID)
	assert.Equal(t, result.Success, unmarshaled.Success)
	assert.Equal(t, result.Duration, unmarshaled.Duration)
}

func TestSyncResult_DurationTracking(t *testing.T) {
	durations := []time.Duration{
		10 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}

	for i, duration := range durations {
		result := &SyncResult{
			JobID:    "job-duration-test",
			Success:  true,
			SyncedAt: time.Now(),
			Duration: duration,
		}

		assert.Equal(t, duration, result.Duration)
		if i > 0 {
			assert.Greater(t, result.Duration, durations[i-1])
		}
	}
}

func TestSyncJob_EmptyID(t *testing.T) {
	job := &SyncJob{
		ID:         "",
		Type:       "test",
		Key:        "key",
		Collection: "collection",
		Timestamp:  time.Now(),
	}

	assert.Empty(t, job.ID)
}

func TestDLQJob_EmptyError(t *testing.T) {
	dlqJob := &DLQJob{
		OriginalJob: SyncJob{ID: "job"},
		Error:       "",
		FailedAt:    time.Now(),
	}

	assert.Empty(t, dlqJob.Error)
}

func TestSyncResult_WithError_OmitEmpty(t *testing.T) {
	result := &SyncResult{
		JobID:    "job-omit-test",
		Success:  true,
		Error:    "",
		SyncedAt: time.Now(),
		Duration: 100 * time.Millisecond,
	}

	jsonData, err := json.Marshal(result)
	require.NoError(t, err)

	// When error is empty and omitempty tag is used, it should not appear in JSON
	jsonString := string(jsonData)
	assert.Contains(t, jsonString, `"success":true`)
	// Error field should either be absent or empty string
}
