package features

import (
	"context"
	"sync"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/queue"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// BatchProcessor handles batch job processing
type BatchProcessor struct {
	queueManager  *queue.Manager
	batchSize     int
	batchTimeout  time.Duration
	batches       map[string]*batch
	mu            sync.RWMutex
	stopCh        chan struct{}
}

type batch struct {
	jobs      []*job.Job
	timer     *time.Timer
	mu        sync.Mutex
	processor func(context.Context, []*job.Job) error
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(qm *queue.Manager, batchSize int, batchTimeout time.Duration) *BatchProcessor {
	if batchSize == 0 {
		batchSize = 10
	}
	if batchTimeout == 0 {
		batchTimeout = 5 * time.Second
	}

	return &BatchProcessor{
		queueManager: qm,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		batches:      make(map[string]*batch),
		stopCh:       make(chan struct{}),
	}
}

// RegisterBatchHandler registers a batch processing function for a job type
func (bp *BatchProcessor) RegisterBatchHandler(jobType string, processor func(context.Context, []*job.Job) error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.batches[jobType] = &batch{
		jobs:      make([]*job.Job, 0, bp.batchSize),
		processor: processor,
	}
}

// AddJob adds a job to the batch
func (bp *BatchProcessor) AddJob(ctx context.Context, j *job.Job) error {
	bp.mu.Lock()
	b, exists := bp.batches[j.Type]
	if !exists {
		bp.mu.Unlock()
		return nil // Job type not registered for batching
	}
	bp.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Add job to batch
	b.jobs = append(b.jobs, j)

	// If this is the first job, start the timer
	if len(b.jobs) == 1 {
		b.timer = time.AfterFunc(bp.batchTimeout, func() {
			bp.processBatch(ctx, j.Type)
		})
	}

	// If batch is full, process immediately
	if len(b.jobs) >= bp.batchSize {
		if b.timer != nil {
			b.timer.Stop()
		}
		go bp.processBatch(ctx, j.Type)
	}

	return nil
}

// processBatch processes a batch of jobs
func (bp *BatchProcessor) processBatch(ctx context.Context, jobType string) {
	bp.mu.RLock()
	b, exists := bp.batches[jobType]
	bp.mu.RUnlock()

	if !exists {
		return
	}

	b.mu.Lock()
	if len(b.jobs) == 0 {
		b.mu.Unlock()
		return
	}

	// Take current batch
	jobs := b.jobs
	b.jobs = make([]*job.Job, 0, bp.batchSize)
	b.mu.Unlock()

	// Process the batch
	err := b.processor(ctx, jobs)

	// Update job statuses
	for _, j := range jobs {
		if err != nil {
			j.Status = job.StatusFailed
			j.Error = err.Error()
		} else {
			j.Status = job.StatusSuccess
		}
		bp.queueManager.UpdateJob(ctx, j)
	}
}

// Stop stops the batch processor
func (bp *BatchProcessor) Stop() {
	close(bp.stopCh)

	// Process remaining batches
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	ctx := context.Background()
	for jobType := range bp.batches {
		bp.processBatch(ctx, jobType)
	}
}
