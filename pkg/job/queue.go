package job

import "time"

// RetryStrategy defines how jobs should be retried
type RetryStrategy string

const (
	RetryStrategyFixed       RetryStrategy = "FIXED"       // Fixed delay between retries
	RetryStrategyExponential RetryStrategy = "EXPONENTIAL" // Exponential backoff
	RetryStrategyLinear      RetryStrategy = "LINEAR"      // Linear backoff
)

// QueueConfig represents the configuration for a queue
type QueueConfig struct {
	// Name is the unique identifier for the queue
	Name string `json:"name"`

	// MaxWorkers is the maximum number of workers for this queue
	MaxWorkers int `json:"max_workers"`

	// MaxRetries is the default maximum number of retry attempts
	MaxRetries int `json:"max_retries"`

	// RetryStrategy defines how retries should be performed
	RetryStrategy RetryStrategy `json:"retry_strategy"`

	// DefaultTimeout is the default timeout for jobs in this queue
	DefaultTimeout time.Duration `json:"default_timeout"`

	// Priority is the queue priority (higher values = higher priority)
	Priority int `json:"priority"`

	// RateLimit is the maximum number of jobs per second (0 = no limit)
	RateLimit int `json:"rate_limit,omitempty"`

	// Weight determines worker allocation (higher weight = more workers)
	Weight int `json:"weight"`

	// IsPaused indicates if the queue is paused
	IsPaused bool `json:"is_paused"`

	// RetryDelay is the base delay for retry strategies
	RetryDelay time.Duration `json:"retry_delay"`

	// MaxRetryDelay is the maximum delay for exponential backoff
	MaxRetryDelay time.Duration `json:"max_retry_delay,omitempty"`

	// DeadLetterQueue is the queue name for failed jobs
	DeadLetterQueue string `json:"dead_letter_queue,omitempty"`
}

// QueueStats represents statistics for a queue
type QueueStats struct {
	// QueueName is the name of the queue
	QueueName string `json:"queue_name"`

	// PendingCount is the number of pending jobs
	PendingCount int64 `json:"pending_count"`

	// RunningCount is the number of jobs currently being processed
	RunningCount int64 `json:"running_count"`

	// SuccessCount is the total number of successful jobs
	SuccessCount int64 `json:"success_count"`

	// FailedCount is the total number of failed jobs
	FailedCount int64 `json:"failed_count"`

	// ActiveWorkers is the number of workers processing this queue
	ActiveWorkers int `json:"active_workers"`

	// AvgProcessingTime is the average time to process a job
	AvgProcessingTime time.Duration `json:"avg_processing_time"`

	// LastJobAt is the timestamp of the last job processed
	LastJobAt *time.Time `json:"last_job_at,omitempty"`
}

// DefaultQueueConfig returns a queue config with sensible defaults
func DefaultQueueConfig(name string) *QueueConfig {
	return &QueueConfig{
		Name:           name,
		MaxWorkers:     10,
		MaxRetries:     3,
		RetryStrategy:  RetryStrategyExponential,
		DefaultTimeout: 5 * time.Minute,
		Priority:       5,
		Weight:         1,
		IsPaused:       false,
		RetryDelay:     5 * time.Second,
		MaxRetryDelay:  1 * time.Hour,
	}
}
