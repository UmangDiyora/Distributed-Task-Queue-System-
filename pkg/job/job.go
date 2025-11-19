package job

import (
	"encoding/json"
	"time"
)

// Status represents the current state of a job
type Status string

const (
	StatusPending   Status = "PENDING"   // Job created but not yet picked
	StatusRunning   Status = "RUNNING"   // Currently being processed
	StatusSuccess   Status = "SUCCESS"   // Completed successfully
	StatusFailed    Status = "FAILED"    // Failed after all retries
	StatusRetrying  Status = "RETRYING"  // Failed but will retry
	StatusCancelled Status = "CANCELLED" // Manually cancelled
	StatusExpired   Status = "EXPIRED"   // Exceeded TTL
)

// Priority represents the priority level of a job
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityNormal Priority = 5
	PriorityHigh   Priority = 10
)

// Job represents a task to be processed by the queue system
type Job struct {
	// ID is the unique identifier for the job (UUID)
	ID string `json:"id"`

	// Type identifies the kind of job (e.g., "send_email", "process_image")
	Type string `json:"type"`

	// Payload contains the job-specific data as JSON
	Payload json.RawMessage `json:"payload"`

	// Priority determines the order of processing
	Priority Priority `json:"priority"`

	// Status represents the current state of the job
	Status Status `json:"status"`

	// MaxRetries is the maximum number of retry attempts
	MaxRetries int `json:"max_retries"`

	// CurrentRetry is the current retry attempt number
	CurrentRetry int `json:"current_retry"`

	// Timeout is the maximum duration for job execution
	Timeout time.Duration `json:"timeout"`

	// ScheduledAt is when the job should be executed (for delayed jobs)
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`

	// CreatedAt is when the job was created
	CreatedAt time.Time `json:"created_at"`

	// StartedAt is when the job started processing
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the job finished (success or failure)
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Error contains the error message if the job failed
	Error string `json:"error,omitempty"`

	// Result contains the job output as JSON
	Result json.RawMessage `json:"result,omitempty"`

	// Metadata contains additional custom key-value pairs
	Metadata map[string]string `json:"metadata,omitempty"`

	// Queue is the name of the queue this job belongs to
	Queue string `json:"queue"`

	// Progress represents the completion percentage (0-100)
	Progress int `json:"progress"`

	// WorkerID is the ID of the worker currently processing this job
	WorkerID string `json:"worker_id,omitempty"`
}

// IsTerminal returns true if the job is in a terminal state
func (j *Job) IsTerminal() bool {
	return j.Status == StatusSuccess ||
		j.Status == StatusFailed ||
		j.Status == StatusCancelled ||
		j.Status == StatusExpired
}

// CanRetry returns true if the job can be retried
func (j *Job) CanRetry() bool {
	return j.CurrentRetry < j.MaxRetries && !j.IsTerminal()
}

// IsReady returns true if a scheduled job is ready to be executed
func (j *Job) IsReady() bool {
	if j.ScheduledAt == nil {
		return true
	}
	return time.Now().After(*j.ScheduledAt)
}

// Duration returns how long the job took to complete
func (j *Job) Duration() time.Duration {
	if j.StartedAt == nil || j.CompletedAt == nil {
		return 0
	}
	return j.CompletedAt.Sub(*j.StartedAt)
}
