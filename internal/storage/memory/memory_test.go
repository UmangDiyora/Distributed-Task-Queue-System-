package memory

import (
	"context"
	"testing"
	"time"

	"github.com/Distributed-Task-Queue-System/pkg/job"
)

func TestMemoryStore_JobCRUD(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create job
	j := &job.Job{
		ID:          "test-job-1",
		Type:        "test",
		Queue:       "default",
		Status:      job.StatusPending,
		Priority:    job.PriorityNormal,
		Payload:     map[string]interface{}{"key": "value"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ScheduledAt: time.Now(),
	}

	err := store.CreateJob(ctx, j)
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Get job
	retrieved, err := store.GetJob(ctx, "test-job-1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}

	if retrieved.ID != j.ID {
		t.Errorf("GetJob returned wrong job: got %v, want %v", retrieved.ID, j.ID)
	}

	// Update job
	j.Status = job.StatusRunning
	err = store.UpdateJob(ctx, j)
	if err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}

	retrieved, _ = store.GetJob(ctx, "test-job-1")
	if retrieved.Status != job.StatusRunning {
		t.Errorf("UpdateJob didn't update status: got %v, want %v", retrieved.Status, job.StatusRunning)
	}

	// Delete job
	err = store.DeleteJob(ctx, "test-job-1")
	if err != nil {
		t.Fatalf("DeleteJob failed: %v", err)
	}

	_, err = store.GetJob(ctx, "test-job-1")
	if err == nil {
		t.Error("GetJob should fail after deletion")
	}
}

func TestMemoryStore_QueueOperations(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create job
	j := &job.Job{
		ID:          "test-job-1",
		Type:        "test",
		Queue:       "default",
		Status:      job.StatusPending,
		Priority:    job.PriorityHigh,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ScheduledAt: time.Now(),
	}
	store.CreateJob(ctx, j)

	// Enqueue
	err := store.Enqueue(ctx, "default", "test-job-1", job.PriorityHigh, 0)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Check length
	length, err := store.Length(ctx, "default")
	if err != nil {
		t.Fatalf("Length failed: %v", err)
	}
	if length != 1 {
		t.Errorf("Length = %v, want 1", length)
	}

	// Dequeue
	jobID, err := store.Dequeue(ctx, "default")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if jobID != "test-job-1" {
		t.Errorf("Dequeue returned wrong job: got %v, want test-job-1", jobID)
	}

	// Check empty queue
	length, _ = store.Length(ctx, "default")
	if length != 0 {
		t.Errorf("Length after dequeue = %v, want 0", length)
	}
}

func TestMemoryStore_PriorityOrdering(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create jobs with different priorities
	jobs := []*job.Job{
		{ID: "low", Priority: job.PriorityLow, Queue: "default", Status: job.StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now(), ScheduledAt: time.Now()},
		{ID: "high", Priority: job.PriorityHigh, Queue: "default", Status: job.StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now(), ScheduledAt: time.Now()},
		{ID: "normal", Priority: job.PriorityNormal, Queue: "default", Status: job.StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now(), ScheduledAt: time.Now()},
	}

	for _, j := range jobs {
		store.CreateJob(ctx, j)
		store.Enqueue(ctx, "default", j.ID, j.Priority, 0)
	}

	// Dequeue should return high priority first
	jobID, _ := store.Dequeue(ctx, "default")
	if jobID != "high" {
		t.Errorf("First dequeue = %v, want high", jobID)
	}

	jobID, _ = store.Dequeue(ctx, "default")
	if jobID != "normal" {
		t.Errorf("Second dequeue = %v, want normal", jobID)
	}

	jobID, _ = store.Dequeue(ctx, "default")
	if jobID != "low" {
		t.Errorf("Third dequeue = %v, want low", jobID)
	}
}

func TestMemoryStore_WorkerOperations(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	now := time.Now()
	w := &job.Worker{
		ID:            "worker-1",
		Hostname:      "localhost",
		Status:        job.WorkerStatusIdle,
		Queues:        []string{"default"},
		Concurrency:   4,
		StartedAt:     &now,
		LastHeartbeat: &now,
	}

	// Register worker
	err := store.RegisterWorker(ctx, w)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Get worker
	retrieved, err := store.GetWorker(ctx, "worker-1")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if retrieved.ID != w.ID {
		t.Errorf("GetWorker returned wrong worker: got %v, want %v", retrieved.ID, w.ID)
	}

	// Update heartbeat
	time.Sleep(10 * time.Millisecond)
	err = store.UpdateHeartbeat(ctx, "worker-1")
	if err != nil {
		t.Fatalf("UpdateHeartbeat failed: %v", err)
	}

	// Get stale workers
	stale, err := store.GetStaleWorkers(ctx, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("GetStaleWorkers failed: %v", err)
	}
	if len(stale) == 0 {
		t.Error("GetStaleWorkers should return worker with old heartbeat")
	}

	// Unregister worker
	err = store.UnregisterWorker(ctx, "worker-1")
	if err != nil {
		t.Fatalf("UnregisterWorker failed: %v", err)
	}

	_, err = store.GetWorker(ctx, "worker-1")
	if err == nil {
		t.Error("GetWorker should fail after unregistration")
	}
}

func TestMemoryStore_QueuePauseResume(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Pause queue
	err := store.PauseQueue(ctx, "default")
	if err != nil {
		t.Fatalf("PauseQueue failed: %v", err)
	}

	// Check if paused
	paused, err := store.IsQueuePaused(ctx, "default")
	if err != nil {
		t.Fatalf("IsQueuePaused failed: %v", err)
	}
	if !paused {
		t.Error("Queue should be paused")
	}

	// Try to dequeue from paused queue
	j := &job.Job{
		ID:          "test-job",
		Queue:       "default",
		Status:      job.StatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ScheduledAt: time.Now(),
	}
	store.CreateJob(ctx, j)
	store.Enqueue(ctx, "default", "test-job", job.PriorityNormal, 0)

	_, err = store.Dequeue(ctx, "default")
	if err == nil {
		t.Error("Dequeue should fail on paused queue")
	}

	// Resume queue
	err = store.ResumeQueue(ctx, "default")
	if err != nil {
		t.Fatalf("ResumeQueue failed: %v", err)
	}

	paused, _ = store.IsQueuePaused(ctx, "default")
	if paused {
		t.Error("Queue should not be paused after resume")
	}

	// Should be able to dequeue now
	_, err = store.Dequeue(ctx, "default")
	if err != nil {
		t.Errorf("Dequeue should succeed after resume: %v", err)
	}
}

func TestMemoryStore_Metrics(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Record metrics
	store.RecordJobCreated(ctx, "default", "test")
	store.RecordJobCompleted(ctx, "default", "test", 100*time.Millisecond)
	store.RecordJobFailed(ctx, "default", "test")

	// Get job type stats
	count, avgDuration, err := store.GetJobTypeStats(ctx, "test")
	if err != nil {
		t.Fatalf("GetJobTypeStats failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Job count = %v, want 1", count)
	}
	if avgDuration == 0 {
		t.Error("Average duration should not be 0")
	}
}
