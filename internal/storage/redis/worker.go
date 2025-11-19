package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Distributed-Task-Queue-System/pkg/errors"
	"github.com/Distributed-Task-Queue-System/pkg/job"
	"github.com/redis/go-redis/v9"
)

// Register registers a new worker
func (r *RedisStore) Register(ctx context.Context, w *job.Worker) error {
	if w == nil || w.ID == "" {
		return errors.ErrInvalidValue
	}

	data, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal worker: %w", err)
	}

	key := keyPrefixWorker + w.ID
	if err := r.client.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}

	// Also add to a set for easy listing
	if err := r.client.SAdd(ctx, keyPrefixWorker+"set", w.ID).Err(); err != nil {
		return fmt.Errorf("failed to add worker to set: %w", err)
	}

	return nil
}

// Unregister removes a worker
func (r *RedisStore) Unregister(ctx context.Context, workerID string) error {
	if workerID == "" {
		return errors.ErrInvalidValue
	}

	key := keyPrefixWorker + workerID

	// Remove from hash
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to unregister worker: %w", err)
	}

	// Remove from set
	if err := r.client.SRem(ctx, keyPrefixWorker+"set", workerID).Err(); err != nil {
		return fmt.Errorf("failed to remove worker from set: %w", err)
	}

	return nil
}

// Get retrieves a worker by ID
func (r *RedisStore) Get(ctx context.Context, workerID string) (*job.Worker, error) {
	if workerID == "" {
		return nil, errors.ErrInvalidValue
	}

	key := keyPrefixWorker + workerID
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, errors.ErrWorkerNotFound
		}
		return nil, fmt.Errorf("failed to get worker: %w", err)
	}

	var w job.Worker
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("failed to unmarshal worker: %w", err)
	}

	return &w, nil
}

// List retrieves all workers
func (r *RedisStore) List(ctx context.Context) ([]*job.Worker, error) {
	// Get all worker IDs from the set
	workerIDs, err := r.client.SMembers(ctx, keyPrefixWorker+"set").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list workers: %w", err)
	}

	var workers []*job.Worker
	for _, workerID := range workerIDs {
		w, err := r.Get(ctx, workerID)
		if err != nil {
			continue // Skip workers that can't be retrieved
		}
		workers = append(workers, w)
	}

	return workers, nil
}

// UpdateHeartbeat updates the worker's last heartbeat time
func (r *RedisStore) UpdateHeartbeat(ctx context.Context, workerID string) error {
	w, err := r.Get(ctx, workerID)
	if err != nil {
		return err
	}

	w.LastHeartbeat = time.Now()

	data, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal worker: %w", err)
	}

	key := keyPrefixWorker + workerID
	return r.client.Set(ctx, key, data, 0).Err()
}

// UpdateStatus updates the worker status
func (r *RedisStore) UpdateStatus(ctx context.Context, workerID string, status job.WorkerStatus) error {
	w, err := r.Get(ctx, workerID)
	if err != nil {
		return err
	}

	w.Status = status

	data, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal worker: %w", err)
	}

	key := keyPrefixWorker + workerID
	return r.client.Set(ctx, key, data, 0).Err()
}

// SetCurrentJob sets the current job being processed
func (r *RedisStore) SetCurrentJob(ctx context.Context, workerID string, jobID string) error {
	w, err := r.Get(ctx, workerID)
	if err != nil {
		return err
	}

	w.CurrentJobID = jobID
	if jobID != "" {
		w.Status = job.WorkerStatusProcessing
	} else {
		w.Status = job.WorkerStatusIdle
	}

	data, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal worker: %w", err)
	}

	key := keyPrefixWorker + workerID
	return r.client.Set(ctx, key, data, 0).Err()
}

// IncrementProcessed increments the processed job count
func (r *RedisStore) IncrementProcessed(ctx context.Context, workerID string) error {
	w, err := r.Get(ctx, workerID)
	if err != nil {
		return err
	}

	w.ProcessedCount++

	data, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal worker: %w", err)
	}

	key := keyPrefixWorker + workerID
	return r.client.Set(ctx, key, data, 0).Err()
}

// IncrementFailed increments the failed job count
func (r *RedisStore) IncrementFailed(ctx context.Context, workerID string) error {
	w, err := r.Get(ctx, workerID)
	if err != nil {
		return err
	}

	w.FailedCount++

	data, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal worker: %w", err)
	}

	key := keyPrefixWorker + workerID
	return r.client.Set(ctx, key, data, 0).Err()
}

// GetStaleWorkers returns workers that haven't sent heartbeat within timeout
func (r *RedisStore) GetStaleWorkers(ctx context.Context, timeout time.Duration) ([]*job.Worker, error) {
	workers, err := r.List(ctx)
	if err != nil {
		return nil, err
	}

	var staleWorkers []*job.Worker
	for _, w := range workers {
		if !w.IsAlive(timeout) {
			staleWorkers = append(staleWorkers, w)
		}
	}

	return staleWorkers, nil
}
