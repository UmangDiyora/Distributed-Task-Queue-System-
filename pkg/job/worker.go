package job

import "time"

// WorkerStatus represents the current state of a worker
type WorkerStatus string

const (
	WorkerStatusIdle       WorkerStatus = "IDLE"       // Worker is waiting for jobs
	WorkerStatusProcessing WorkerStatus = "PROCESSING" // Worker is processing a job
	WorkerStatusStopped    WorkerStatus = "STOPPED"    // Worker has stopped
	WorkerStatusFailed     WorkerStatus = "FAILED"     // Worker has failed
)

// Worker represents a worker instance that processes jobs
type Worker struct {
	// ID is the unique identifier for the worker
	ID string `json:"id"`

	// Hostname is the machine hostname where the worker is running
	Hostname string `json:"hostname"`

	// StartedAt is when the worker was started
	StartedAt time.Time `json:"started_at"`

	// LastHeartbeat is the last time the worker sent a heartbeat
	LastHeartbeat time.Time `json:"last_heartbeat"`

	// CurrentJobID is the ID of the job currently being processed
	CurrentJobID string `json:"current_job_id,omitempty"`

	// ProcessedCount is the total number of jobs processed by this worker
	ProcessedCount int64 `json:"processed_count"`

	// FailedCount is the total number of jobs that failed on this worker
	FailedCount int64 `json:"failed_count"`

	// Status represents the current state of the worker
	Status WorkerStatus `json:"status"`

	// Queues is the list of queue names this worker is processing
	Queues []string `json:"queues"`

	// Concurrency is the number of jobs this worker can process concurrently
	Concurrency int `json:"concurrency"`

	// Tags are custom labels for the worker
	Tags map[string]string `json:"tags,omitempty"`

	// Version is the worker software version
	Version string `json:"version,omitempty"`
}

// IsAlive returns true if the worker has sent a heartbeat recently
func (w *Worker) IsAlive(timeout time.Duration) bool {
	return time.Since(w.LastHeartbeat) < timeout
}

// Uptime returns how long the worker has been running
func (w *Worker) Uptime() time.Duration {
	return time.Since(w.StartedAt)
}

// SuccessRate returns the success rate as a percentage
func (w *Worker) SuccessRate() float64 {
	total := w.ProcessedCount
	if total == 0 {
		return 0
	}
	successful := total - w.FailedCount
	return float64(successful) / float64(total) * 100
}
