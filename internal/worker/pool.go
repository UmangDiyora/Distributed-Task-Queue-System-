package worker

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/queue"
	"github.com/Distributed-Task-Queue-System/pkg/errors"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// JobHandler is a function that processes a job
type JobHandler func(ctx context.Context, j *job.Job) error

// Pool manages a pool of workers that process jobs
type Pool struct {
	id               string
	queueManager     *queue.Manager
	handlers         map[string]JobHandler
	queues           []string
	concurrency      int
	pollInterval     time.Duration
	heartbeatInterval time.Duration
	shutdownTimeout  time.Duration

	worker           *job.Worker
	mu               sync.RWMutex
	wg               sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
	stopCh           chan struct{}
	jobCh            chan *job.Job
}

// Config holds worker pool configuration
type Config struct {
	ID                string
	Queues            []string
	Concurrency       int
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	ShutdownTimeout   time.Duration
	Tags              map[string]string
}

// NewPool creates a new worker pool
func NewPool(qm *queue.Manager, config *Config) (*Pool, error) {
	if config.ID == "" {
		hostname, _ := os.Hostname()
		config.ID = fmt.Sprintf("%s-%d", hostname, time.Now().Unix())
	}

	if config.Concurrency == 0 {
		config.Concurrency = runtime.NumCPU()
	}

	if config.PollInterval == 0 {
		config.PollInterval = 1 * time.Second
	}

	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}

	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 30 * time.Second
	}

	if len(config.Queues) == 0 {
		config.Queues = []string{"default"}
	}

	ctx, cancel := context.WithCancel(context.Background())

	hostname, _ := os.Hostname()
	now := time.Now()

	w := &job.Worker{
		ID:            config.ID,
		Hostname:      hostname,
		Version:       "1.0.0",
		Status:        job.WorkerStatusIdle,
		Queues:        config.Queues,
		Tags:          config.Tags,
		Concurrency:   config.Concurrency,
		StartedAt:     &now,
		LastHeartbeat: &now,
		Metadata:      make(map[string]interface{}),
	}

	p := &Pool{
		id:                config.ID,
		queueManager:      qm,
		handlers:          make(map[string]JobHandler),
		queues:            config.Queues,
		concurrency:       config.Concurrency,
		pollInterval:      config.PollInterval,
		heartbeatInterval: config.HeartbeatInterval,
		shutdownTimeout:   config.ShutdownTimeout,
		worker:            w,
		ctx:               ctx,
		cancel:            cancel,
		stopCh:            make(chan struct{}),
		jobCh:             make(chan *job.Job, config.Concurrency),
	}

	return p, nil
}

// RegisterHandler registers a job handler for a specific job type
func (p *Pool) RegisterHandler(jobType string, handler JobHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[jobType] = handler
}

// RegisterHandlers registers multiple job handlers at once
func (p *Pool) RegisterHandlers(handlers map[string]JobHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for jobType, handler := range handlers {
		p.handlers[jobType] = handler
	}
}

// Start starts the worker pool
func (p *Pool) Start() error {
	// Register worker
	if err := p.queueManager.storage.RegisterWorker(p.ctx, p.worker); err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}

	// Update status to processing
	p.worker.Status = job.WorkerStatusProcessing
	if err := p.queueManager.storage.UpdateWorkerStatus(p.ctx, p.worker.ID, job.WorkerStatusProcessing); err != nil {
		return fmt.Errorf("failed to update worker status: %w", err)
	}

	// Start heartbeat
	p.wg.Add(1)
	go p.heartbeatLoop()

	// Start job fetcher
	p.wg.Add(1)
	go p.fetchLoop()

	// Start worker goroutines
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.workerLoop(i)
	}

	fmt.Printf("Worker pool %s started with %d workers on queues: %v\n",
		p.id, p.concurrency, p.queues)

	return nil
}

// Stop gracefully stops the worker pool
func (p *Pool) Stop() error {
	fmt.Printf("Stopping worker pool %s...\n", p.id)

	// Signal stop
	close(p.stopCh)

	// Wait for graceful shutdown with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("Worker pool stopped gracefully")
	case <-time.After(p.shutdownTimeout):
		fmt.Println("Worker pool shutdown timeout, forcing stop")
		p.cancel()
		<-done
	}

	// Update worker status
	p.worker.Status = job.WorkerStatusStopped
	if err := p.queueManager.storage.UpdateWorkerStatus(context.Background(), p.worker.ID, job.WorkerStatusStopped); err != nil {
		fmt.Printf("warning: failed to update worker status: %v\n", err)
	}

	// Unregister worker
	if err := p.queueManager.storage.UnregisterWorker(context.Background(), p.worker.ID); err != nil {
		fmt.Printf("warning: failed to unregister worker: %v\n", err)
	}

	return nil
}

// heartbeatLoop sends periodic heartbeats
func (p *Pool) heartbeatLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			if err := p.queueManager.storage.UpdateHeartbeat(p.ctx, p.worker.ID); err != nil {
				fmt.Printf("warning: failed to update heartbeat: %v\n", err)
			}
		}
	}
}

// fetchLoop continuously fetches jobs from queues
func (p *Pool) fetchLoop() {
	defer p.wg.Done()
	defer close(p.jobCh)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.fetchJobs()
		}
	}
}

// fetchJobs fetches available jobs from queues
func (p *Pool) fetchJobs() {
	// Try to fill the job channel
	for len(p.jobCh) < p.concurrency {
		j, err := p.queueManager.Dequeue(p.ctx, p.queues)
		if err != nil {
			if err != errors.ErrQueueEmpty {
				fmt.Printf("error dequeuing job: %v\n", err)
			}
			return
		}

		// Send job to workers
		select {
		case p.jobCh <- j:
		case <-p.stopCh:
			// Requeue the job if we're stopping
			_ = p.queueManager.Requeue(p.ctx, j, 0)
			return
		}
	}
}

// workerLoop processes jobs from the job channel
func (p *Pool) workerLoop(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.stopCh:
			return
		case j, ok := <-p.jobCh:
			if !ok {
				return
			}
			p.processJob(j)
		}
	}
}

// processJob processes a single job
func (p *Pool) processJob(j *job.Job) {
	startTime := time.Now()

	// Update job status to running
	if err := p.queueManager.UpdateJobStatus(p.ctx, j.ID, job.StatusRunning); err != nil {
		fmt.Printf("error updating job status: %v\n", err)
		return
	}

	// Set current job for worker
	if err := p.queueManager.storage.SetCurrentJob(p.ctx, p.worker.ID, j.ID); err != nil {
		fmt.Printf("warning: failed to set current job: %v\n", err)
	}

	// Create job context with timeout
	jobCtx, cancel := context.WithTimeout(p.ctx, time.Duration(j.TimeoutSeconds)*time.Second)
	defer cancel()

	// Process job with panic recovery
	err := p.executeJob(jobCtx, j)

	// Clear current job
	if err := p.queueManager.storage.SetCurrentJob(p.ctx, p.worker.ID, ""); err != nil {
		fmt.Printf("warning: failed to clear current job: %v\n", err)
	}

	duration := time.Since(startTime)

	// Handle result
	if err != nil {
		p.handleJobFailure(j, err, duration)
	} else {
		p.handleJobSuccess(j, duration)
	}
}

// executeJob executes a job with panic recovery
func (p *Pool) executeJob(ctx context.Context, j *job.Job) (err error) {
	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("job panicked: %v", r)
			// Capture stack trace
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			fmt.Printf("Job %s panicked: %v\nStack trace:\n%s\n", j.ID, r, buf[:n])
		}
	}()

	// Get handler
	p.mu.RLock()
	handler, exists := p.handlers[j.Type]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no handler registered for job type: %s", j.Type)
	}

	// Execute handler
	return handler(ctx, j)
}

// handleJobSuccess handles successful job completion
func (p *Pool) handleJobSuccess(j *job.Job, duration time.Duration) {
	// Update job status
	if err := p.queueManager.UpdateJobStatus(p.ctx, j.ID, job.StatusSuccess); err != nil {
		fmt.Printf("error updating job status: %v\n", err)
	}

	// Update job progress to 100%
	if err := p.queueManager.UpdateJobProgress(p.ctx, j.ID, 100); err != nil {
		fmt.Printf("warning: failed to update job progress: %v\n", err)
	}

	// Record metrics
	if err := p.queueManager.storage.RecordJobCompleted(p.ctx, j.Queue, j.Type, duration); err != nil {
		fmt.Printf("warning: failed to record job completed metric: %v\n", err)
	}

	// Increment worker processed count
	if err := p.queueManager.storage.IncrementProcessed(p.ctx, p.worker.ID); err != nil {
		fmt.Printf("warning: failed to increment processed count: %v\n", err)
	}

	fmt.Printf("Job %s completed successfully in %v\n", j.ID, duration)
}

// handleJobFailure handles job failure
func (p *Pool) handleJobFailure(j *job.Job, jobErr error, duration time.Duration) {
	// Set job error
	if err := p.queueManager.SetJobError(p.ctx, j.ID, jobErr.Error()); err != nil {
		fmt.Printf("error setting job error: %v\n", err)
	}

	// Check if job can be retried
	if j.CanRetry() {
		// Increment retry count
		j.RetryCount++

		// Calculate retry delay (handled by retry mechanism)
		config, err := p.queueManager.GetQueueConfig(p.ctx, j.Queue)
		if err != nil {
			fmt.Printf("error getting queue config: %v\n", err)
			return
		}

		delay := calculateRetryDelay(j.RetryCount, config)

		// Update job status to retrying
		if err := p.queueManager.UpdateJobStatus(p.ctx, j.ID, job.StatusRetrying); err != nil {
			fmt.Printf("error updating job status: %v\n", err)
		}

		// Requeue the job with delay
		if err := p.queueManager.Requeue(p.ctx, j, delay); err != nil {
			fmt.Printf("error requeuing job: %v\n", err)
		}

		// Record retry metric
		if err := p.queueManager.storage.RecordJobRetried(p.ctx, j.Queue, j.Type); err != nil {
			fmt.Printf("warning: failed to record job retried metric: %v\n", err)
		}

		fmt.Printf("Job %s failed (retry %d/%d), requeued with %v delay: %v\n",
			j.ID, j.RetryCount, j.MaxRetries, delay, jobErr)
	} else {
		// Max retries reached or job doesn't support retries
		if err := p.queueManager.UpdateJobStatus(p.ctx, j.ID, job.StatusFailed); err != nil {
			fmt.Printf("error updating job status: %v\n", err)
		}

		// Record failure metric
		if err := p.queueManager.storage.RecordJobFailed(p.ctx, j.Queue, j.Type); err != nil {
			fmt.Printf("warning: failed to record job failed metric: %v\n", err)
		}

		// Send to dead letter queue if configured
		config, err := p.queueManager.GetQueueConfig(p.ctx, j.Queue)
		if err == nil && config.DeadLetterQueue != "" {
			j.Queue = config.DeadLetterQueue
			j.Status = job.StatusPending
			if err := p.queueManager.Enqueue(p.ctx, j); err != nil {
				fmt.Printf("error sending job to DLQ: %v\n", err)
			} else {
				fmt.Printf("Job %s sent to dead letter queue: %s\n", j.ID, config.DeadLetterQueue)
			}
		}

		fmt.Printf("Job %s failed permanently: %v\n", j.ID, jobErr)
	}

	// Increment worker failed count
	if err := p.queueManager.storage.IncrementFailed(p.ctx, p.worker.ID); err != nil {
		fmt.Printf("warning: failed to increment failed count: %v\n", err)
	}
}

// calculateRetryDelay calculates the retry delay based on retry strategy
func calculateRetryDelay(retryCount int, config *job.QueueConfig) time.Duration {
	baseDelay := time.Duration(config.RetryDelay) * time.Second
	maxDelay := time.Duration(config.MaxRetryDelay) * time.Second

	var delay time.Duration

	switch config.RetryStrategy {
	case job.RetryStrategyFixed:
		delay = baseDelay

	case job.RetryStrategyLinear:
		delay = baseDelay * time.Duration(retryCount)

	case job.RetryStrategyExponential:
		// 2^retryCount * baseDelay
		multiplier := 1 << uint(retryCount-1) // 2^(retryCount-1)
		delay = baseDelay * time.Duration(multiplier)

	default:
		delay = baseDelay
	}

	// Cap at max delay
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// GetWorker returns the worker instance
func (p *Pool) GetWorker() *job.Worker {
	return p.worker
}

// GetStats returns worker statistics
func (p *Pool) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"id":              p.worker.ID,
		"status":          p.worker.Status,
		"queues":          p.worker.Queues,
		"concurrency":     p.concurrency,
		"processed_count": p.worker.ProcessedCount,
		"failed_count":    p.worker.FailedCount,
		"success_rate":    p.worker.SuccessRate(),
		"uptime":          p.worker.Uptime(),
	}
}
