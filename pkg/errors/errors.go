package errors

import (
	"errors"
	"fmt"
)

// Common errors
var (
	// Job errors
	ErrJobNotFound      = errors.New("job not found")
	ErrJobAlreadyExists = errors.New("job already exists")
	ErrJobCancelled     = errors.New("job was cancelled")
	ErrJobExpired       = errors.New("job has expired")
	ErrJobTimeout       = errors.New("job execution timeout")
	ErrInvalidJobType   = errors.New("invalid job type")
	ErrInvalidPayload   = errors.New("invalid job payload")

	// Queue errors
	ErrQueueNotFound  = errors.New("queue not found")
	ErrQueuePaused    = errors.New("queue is paused")
	ErrQueueFull      = errors.New("queue is full")
	ErrEmptyQueue     = errors.New("queue is empty")
	ErrInvalidQueue   = errors.New("invalid queue configuration")

	// Worker errors
	ErrWorkerNotFound      = errors.New("worker not found")
	ErrWorkerTimeout       = errors.New("worker heartbeat timeout")
	ErrWorkerBusy          = errors.New("worker is busy")
	ErrMaxWorkersReached   = errors.New("maximum workers reached")
	ErrNoAvailableWorkers  = errors.New("no available workers")

	// Storage errors
	ErrStorageUnavailable  = errors.New("storage backend unavailable")
	ErrStorageTimeout      = errors.New("storage operation timeout")
	ErrDuplicateKey        = errors.New("duplicate key")
	ErrInvalidOperation    = errors.New("invalid operation")
	ErrConnectionFailed    = errors.New("connection failed")

	// Retry errors
	ErrMaxRetriesExceeded  = errors.New("maximum retries exceeded")
	ErrRetryNotAllowed     = errors.New("retry not allowed")

	// Validation errors
	ErrInvalidConfiguration = errors.New("invalid configuration")
	ErrMissingRequired      = errors.New("missing required field")
	ErrInvalidValue         = errors.New("invalid value")
)

// JobError represents a job-specific error with context
type JobError struct {
	JobID   string
	Message string
	Err     error
}

func (e *JobError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("job %s: %s: %v", e.JobID, e.Message, e.Err)
	}
	return fmt.Sprintf("job %s: %s", e.JobID, e.Message)
}

func (e *JobError) Unwrap() error {
	return e.Err
}

// NewJobError creates a new job error
func NewJobError(jobID, message string, err error) *JobError {
	return &JobError{
		JobID:   jobID,
		Message: message,
		Err:     err,
	}
}

// QueueError represents a queue-specific error with context
type QueueError struct {
	QueueName string
	Message   string
	Err       error
}

func (e *QueueError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("queue %s: %s: %v", e.QueueName, e.Message, e.Err)
	}
	return fmt.Sprintf("queue %s: %s", e.QueueName, e.Message)
}

func (e *QueueError) Unwrap() error {
	return e.Err
}

// NewQueueError creates a new queue error
func NewQueueError(queueName, message string, err error) *QueueError {
	return &QueueError{
		QueueName: queueName,
		Message:   message,
		Err:       err,
	}
}

// WorkerError represents a worker-specific error with context
type WorkerError struct {
	WorkerID string
	Message  string
	Err      error
}

func (e *WorkerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("worker %s: %s: %v", e.WorkerID, e.Message, e.Err)
	}
	return fmt.Sprintf("worker %s: %s", e.WorkerID, e.Message)
}

func (e *WorkerError) Unwrap() error {
	return e.Err
}

// NewWorkerError creates a new worker error
func NewWorkerError(workerID, message string, err error) *WorkerError {
	return &WorkerError{
		WorkerID: workerID,
		Message:  message,
		Err:      err,
	}
}

// ValidationError represents a validation error with field information
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}
