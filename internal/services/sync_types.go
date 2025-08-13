package services

import (
	"time"
)

// SyncJob represents a job to sync data from Redis to MongoDB
type SyncJob struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Key        string      `json:"key"`
	Collection string      `json:"collection"`
	Data       interface{} `json:"data"`
	Timestamp  time.Time   `json:"timestamp"`
	RetryCount int         `json:"retry_count"`
	MaxRetries int         `json:"max_retries"`
}

// DLQJob represents a job that has failed and been moved to the dead letter queue
type DLQJob struct {
	OriginalJob SyncJob   `json:"original_job"`
	Error       string    `json:"error"`
	FailedAt    time.Time `json:"failed_at"`
}

// SyncResult represents the result of a sync operation
type SyncResult struct {
	JobID    string        `json:"job_id"`
	Success  bool          `json:"success"`
	Error    string        `json:"error,omitempty"`
	SyncedAt time.Time     `json:"synced_at"`
	Duration time.Duration `json:"duration"`
}
