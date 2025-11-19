package storage

import (
	"context"
	"time"

	"github.com/UmangDiyora/gotask/pkg/job"
)

// JobStore defines the interface for job persistence operations
type JobStore interface {
	// Create creates a new job
	Create(ctx context.Context, j *job.Job) error

	// Get retrieves a job by ID
	Get(ctx context.Context, jobID string) (*job.Job, error)

	// Update updates an existing job
	Update(ctx context.Context, j *job.Job) error

	// Delete deletes a job by ID
	Delete(ctx context.Context, jobID string) error

	// List retrieves jobs with filtering and pagination
	List(ctx context.Context, filter JobFilter) ([]*job.Job, error)

	// UpdateStatus updates only the job status
	UpdateStatus(ctx context.Context, jobID string, status job.Status) error

	// UpdateProgress updates job progress
	UpdateProgress(ctx context.Context, jobID string, progress int) error

	// SetResult sets the job result
	SetResult(ctx context.Context, jobID string, result []byte) error

	// SetError sets the job error
	SetError(ctx context.Context, jobID string, errMsg string) error
}

// QueueStore defines the interface for queue operations
type QueueStore interface {
	// Enqueue adds a job to a queue
	Enqueue(ctx context.Context, queueName string, jobID string, priority job.Priority) error

	// Dequeue retrieves the next job from a queue (atomic claim operation)
	Dequeue(ctx context.Context, queueName string, workerID string) (string, error)

	// Requeue puts a job back into the queue
	Requeue(ctx context.Context, queueName string, jobID string, priority job.Priority) error

	// Remove removes a job from the queue
	Remove(ctx context.Context, queueName string, jobID string) error

	// Length returns the number of jobs in a queue
	Length(ctx context.Context, queueName string) (int64, error)

	// Peek returns the next job without dequeuing it
	Peek(ctx context.Context, queueName string) (string, error)

	// GetScheduled retrieves jobs scheduled for execution before the given time
	GetScheduled(ctx context.Context, before time.Time, limit int) ([]*job.Job, error)

	// ListQueues returns all queue names
	ListQueues(ctx context.Context) ([]string, error)

	// PauseQueue pauses a queue
	PauseQueue(ctx context.Context, queueName string) error

	// ResumeQueue resumes a paused queue
	ResumeQueue(ctx context.Context, queueName string) error

	// IsQueuePaused checks if a queue is paused
	IsQueuePaused(ctx context.Context, queueName string) (bool, error)
}

// WorkerStore defines the interface for worker management
type WorkerStore interface {
	// Register registers a new worker
	Register(ctx context.Context, w *job.Worker) error

	// Unregister removes a worker
	Unregister(ctx context.Context, workerID string) error

	// Get retrieves a worker by ID
	Get(ctx context.Context, workerID string) (*job.Worker, error)

	// List retrieves all workers
	List(ctx context.Context) ([]*job.Worker, error)

	// UpdateHeartbeat updates the worker's last heartbeat time
	UpdateHeartbeat(ctx context.Context, workerID string) error

	// UpdateStatus updates the worker status
	UpdateStatus(ctx context.Context, workerID string, status job.WorkerStatus) error

	// SetCurrentJob sets the current job being processed
	SetCurrentJob(ctx context.Context, workerID string, jobID string) error

	// IncrementProcessed increments the processed job count
	IncrementProcessed(ctx context.Context, workerID string) error

	// IncrementFailed increments the failed job count
	IncrementFailed(ctx context.Context, workerID string) error

	// GetStaleWorkers returns workers that haven't sent heartbeat within timeout
	GetStaleWorkers(ctx context.Context, timeout time.Duration) ([]*job.Worker, error)
}

// MetricsStore defines the interface for metrics persistence
type MetricsStore interface {
	// RecordJobCreated records a job creation event
	RecordJobCreated(ctx context.Context, queueName string, jobType string) error

	// RecordJobCompleted records a job completion event
	RecordJobCompleted(ctx context.Context, queueName string, jobType string, duration time.Duration) error

	// RecordJobFailed records a job failure event
	RecordJobFailed(ctx context.Context, queueName string, jobType string, errType string) error

	// RecordJobRetried records a job retry event
	RecordJobRetried(ctx context.Context, queueName string, jobType string) error

	// GetQueueStats retrieves statistics for a queue
	GetQueueStats(ctx context.Context, queueName string) (*job.QueueStats, error)

	// GetQueueStatsAll retrieves statistics for all queues
	GetQueueStatsAll(ctx context.Context) (map[string]*job.QueueStats, error)

	// GetJobTypeStats retrieves statistics grouped by job type
	GetJobTypeStats(ctx context.Context, since time.Time) (map[string]*JobTypeStats, error)
}

// ConfigStore defines the interface for queue configuration management
type ConfigStore interface {
	// SaveQueueConfig saves a queue configuration
	SaveQueueConfig(ctx context.Context, config *job.QueueConfig) error

	// GetQueueConfig retrieves a queue configuration
	GetQueueConfig(ctx context.Context, queueName string) (*job.QueueConfig, error)

	// DeleteQueueConfig deletes a queue configuration
	DeleteQueueConfig(ctx context.Context, queueName string) error

	// ListQueueConfigs lists all queue configurations
	ListQueueConfigs(ctx context.Context) ([]*job.QueueConfig, error)
}

// Store is the main storage interface combining all sub-interfaces
type Store interface {
	JobStore
	QueueStore
	WorkerStore
	MetricsStore
	ConfigStore

	// Health checks storage health
	Health(ctx context.Context) error

	// Close closes the storage connection
	Close() error
}

// JobFilter represents filtering options for job queries
type JobFilter struct {
	// Queue filters by queue name
	Queue string

	// Status filters by job status
	Status []job.Status

	// Type filters by job type
	Type string

	// WorkerID filters by worker ID
	WorkerID string

	// CreatedAfter filters jobs created after this time
	CreatedAfter *time.Time

	// CreatedBefore filters jobs created before this time
	CreatedBefore *time.Time

	// Limit limits the number of results
	Limit int

	// Offset for pagination
	Offset int

	// SortBy defines the sort field (e.g., "created_at", "priority")
	SortBy string

	// SortDesc sorts in descending order if true
	SortDesc bool
}

// JobTypeStats represents statistics for a specific job type
type JobTypeStats struct {
	JobType        string
	TotalCount     int64
	SuccessCount   int64
	FailedCount    int64
	AvgDuration    time.Duration
	MinDuration    time.Duration
	MaxDuration    time.Duration
	LastExecutedAt time.Time
}
