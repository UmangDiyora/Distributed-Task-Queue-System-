package tests

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/queue"
	"github.com/Distributed-Task-Queue-System/internal/storage/memory"
	"github.com/Distributed-Task-Queue-System/internal/worker"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

func TestIntegration_JobLifecycle(t *testing.T) {
	// Setup
	store := memory.NewMemoryStore()
	qm, err := queue.NewManager(store)
	if err != nil {
		t.Fatalf("Failed to create queue manager: %v", err)
	}

	// Register default queue
	config := &job.QueueConfig{
		Name:           "default",
		MaxWorkers:     5,
		MaxRetries:     3,
		RetryStrategy:  job.RetryStrategyFixed,
		Priority:       job.PriorityNormal,
	}
	ctx := context.Background()
	if err := qm.RegisterQueue(ctx, config); err != nil {
		t.Fatalf("Failed to register queue: %v", err)
	}

	// Create worker pool
	workerConfig := &worker.Config{
		ID:          "test-worker",
		Queues:      []string{"default"},
		Concurrency: 2,
	}
	pool, err := worker.NewPool(qm, workerConfig)
	if err != nil {
		t.Fatalf("Failed to create worker pool: %v", err)
	}

	// Track job completion
	var completed int32
	pool.RegisterHandler("test-job", func(ctx context.Context, j *job.Job) error {
		atomic.AddInt32(&completed, 1)
		return nil
	})

	// Start worker pool
	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start worker pool: %v", err)
	}
	defer pool.Stop()

	// Create and enqueue job
	j := &job.Job{
		ID:          "test-job-1",
		Type:        "test-job",
		Queue:       "default",
		Status:      job.StatusPending,
		Priority:    job.PriorityNormal,
		Payload:     map[string]interface{}{"test": "data"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ScheduledAt: time.Now(),
	}

	if err := qm.Enqueue(ctx, j); err != nil {
		t.Fatalf("Failed to enqueue job: %v", err)
	}

	// Wait for job completion
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Job did not complete in time")
		case <-ticker.C:
			if atomic.LoadInt32(&completed) > 0 {
				return // Success!
			}
		}
	}
}

func TestIntegration_MultipleJobs(t *testing.T) {
	store := memory.NewMemoryStore()
	qm, _ := queue.NewManager(store)

	config := &job.QueueConfig{
		Name:       "default",
		MaxWorkers: 10,
		Priority:   job.PriorityNormal,
	}
	ctx := context.Background()
	qm.RegisterQueue(ctx, config)

	workerConfig := &worker.Config{
		ID:          "test-worker",
		Queues:      []string{"default"},
		Concurrency: 5,
	}
	pool, _ := worker.NewPool(qm, workerConfig)

	var completed int32
	pool.RegisterHandler("test-job", func(ctx context.Context, j *job.Job) error {
		time.Sleep(50 * time.Millisecond) // Simulate work
		atomic.AddInt32(&completed, 1)
		return nil
	})

	pool.Start()
	defer pool.Stop()

	// Create multiple jobs
	numJobs := 10
	for i := 0; i < numJobs; i++ {
		j := &job.Job{
			ID:          string(rune(i)),
			Type:        "test-job",
			Queue:       "default",
			Status:      job.StatusPending,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			ScheduledAt: time.Now(),
		}
		qm.Enqueue(ctx, j)
	}

	// Wait for all jobs to complete
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Only %d/%d jobs completed", atomic.LoadInt32(&completed), numJobs)
		case <-ticker.C:
			if atomic.LoadInt32(&completed) >= int32(numJobs) {
				return
			}
		}
	}
}

func TestIntegration_PriorityOrdering(t *testing.T) {
	store := memory.NewMemoryStore()
	qm, _ := queue.NewManager(store)

	config := &job.QueueConfig{
		Name:       "default",
		MaxWorkers: 1, // Single worker to ensure ordering
		Priority:   job.PriorityNormal,
	}
	ctx := context.Background()
	qm.RegisterQueue(ctx, config)

	workerConfig := &worker.Config{
		ID:          "test-worker",
		Queues:      []string{"default"},
		Concurrency: 1,
	}
	pool, _ := worker.NewPool(qm, workerConfig)

	var executionOrder []string
	pool.RegisterHandler("test-job", func(ctx context.Context, j *job.Job) error {
		executionOrder = append(executionOrder, j.ID)
		return nil
	})

	pool.Start()
	defer pool.Stop()

	// Create jobs with different priorities
	jobs := []*job.Job{
		{ID: "low", Type: "test-job", Queue: "default", Priority: job.PriorityLow, Status: job.StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now(), ScheduledAt: time.Now()},
		{ID: "high", Type: "test-job", Queue: "default", Priority: job.PriorityHigh, Status: job.StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now(), ScheduledAt: time.Now()},
		{ID: "normal", Type: "test-job", Queue: "default", Priority: job.PriorityNormal, Status: job.StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now(), ScheduledAt: time.Now()},
	}

	for _, j := range jobs {
		qm.Enqueue(ctx, j)
	}

	// Wait for completion
	time.Sleep(1 * time.Second)

	// Check execution order (should be high, normal, low)
	if len(executionOrder) != 3 {
		t.Fatalf("Expected 3 jobs, got %d", len(executionOrder))
	}

	// First should be high priority
	if executionOrder[0] != "high" {
		t.Errorf("First job should be 'high', got '%s'", executionOrder[0])
	}
}

func TestIntegration_JobRetry(t *testing.T) {
	store := memory.NewMemoryStore()
	qm, _ := queue.NewManager(store)

	config := &job.QueueConfig{
		Name:          "default",
		MaxWorkers:    1,
		MaxRetries:    3,
		RetryStrategy: job.RetryStrategyFixed,
		RetryDelay:    1, // 1 second
		Priority:      job.PriorityNormal,
	}
	ctx := context.Background()
	qm.RegisterQueue(ctx, config)

	workerConfig := &worker.Config{
		ID:          "test-worker",
		Queues:      []string{"default"},
		Concurrency: 1,
	}
	pool, _ := worker.NewPool(qm, workerConfig)

	var attempts int32
	pool.RegisterHandler("failing-job", func(ctx context.Context, j *job.Job) error {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			return fmt.Errorf("simulated failure")
		}
		return nil // Succeed on third attempt
	})

	pool.Start()
	defer pool.Stop()

	j := &job.Job{
		ID:          "retry-job",
		Type:        "failing-job",
		Queue:       "default",
		Status:      job.StatusPending,
		MaxRetries:  3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ScheduledAt: time.Now(),
	}
	qm.Enqueue(ctx, j)

	// Wait for retries to complete
	time.Sleep(5 * time.Second)

	if atomic.LoadInt32(&attempts) < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func BenchmarkIntegration_JobThroughput(b *testing.B) {
	store := memory.NewMemoryStore()
	qm, _ := queue.NewManager(store)

	config := &job.QueueConfig{
		Name:       "default",
		MaxWorkers: 100,
		Priority:   job.PriorityNormal,
	}
	ctx := context.Background()
	qm.RegisterQueue(ctx, config)

	workerConfig := &worker.Config{
		ID:          "bench-worker",
		Queues:      []string{"default"},
		Concurrency: 10,
	}
	pool, _ := worker.NewPool(qm, workerConfig)

	pool.RegisterHandler("bench-job", func(ctx context.Context, j *job.Job) error {
		return nil
	})

	pool.Start()
	defer pool.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		j := &job.Job{
			ID:          string(rune(i)),
			Type:        "bench-job",
			Queue:       "default",
			Status:      job.StatusPending,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			ScheduledAt: time.Now(),
		}
		qm.Enqueue(ctx, j)
	}
}
