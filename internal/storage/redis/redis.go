package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/storage"
	"github.com/Distributed-Task-Queue-System/pkg/errors"
	"github.com/Distributed-Task-Queue-System/pkg/job"
	"github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes
	keyPrefixJob        = "job:"
	keyPrefixQueue      = "queue:"
	keyPrefixScheduled  = "scheduled:"
	keyPrefixWorker     = "worker:"
	keyPrefixQueueStats = "stats:queue:"
	keyPrefixJobType    = "stats:jobtype:"
	keyPrefixConfig     = "config:queue:"
	keyPrefixQueuePause = "pause:queue:"

	// Redis sorted set for priority queue
	keyQueueZSet = "zset:"
)

// RedisStore implements the storage.Store interface using Redis
type RedisStore struct {
	client *redis.Client
	config *storage.RedisConfig
}

// NewRedisStore creates a new Redis storage backend
func NewRedisStore(config *storage.RedisConfig) (*RedisStore, error) {
	if config == nil {
		return nil, errors.ErrInvalidConfiguration
	}

	client := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		MaxRetries:   config.MaxRetries,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisStore{
		client: client,
		config: config,
	}, nil
}

// Create creates a new job
func (r *RedisStore) Create(ctx context.Context, j *job.Job) error {
	if j == nil || j.ID == "" {
		return errors.ErrInvalidValue
	}

	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	key := keyPrefixJob + j.ID

	// Use SETNX to ensure job doesn't already exist
	set, err := r.client.SetNX(ctx, key, data, 0).Result()
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}
	if !set {
		return errors.ErrJobAlreadyExists
	}

	return nil
}

// Get retrieves a job by ID
func (r *RedisStore) Get(ctx context.Context, jobID string) (*job.Job, error) {
	if jobID == "" {
		return nil, errors.ErrInvalidValue
	}

	key := keyPrefixJob + jobID
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, errors.ErrJobNotFound
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	var j job.Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &j, nil
}

// Update updates an existing job
func (r *RedisStore) Update(ctx context.Context, j *job.Job) error {
	if j == nil || j.ID == "" {
		return errors.ErrInvalidValue
	}

	// Check if job exists
	key := keyPrefixJob + j.ID
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check job existence: %w", err)
	}
	if exists == 0 {
		return errors.ErrJobNotFound
	}

	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := r.client.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	return nil
}

// Delete deletes a job by ID
func (r *RedisStore) Delete(ctx context.Context, jobID string) error {
	if jobID == "" {
		return errors.ErrInvalidValue
	}

	key := keyPrefixJob + jobID
	result, err := r.client.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}
	if result == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

// List retrieves jobs with filtering and pagination
func (r *RedisStore) List(ctx context.Context, filter storage.JobFilter) ([]*job.Job, error) {
	// Use SCAN to iterate through all job keys
	pattern := keyPrefixJob + "*"
	var cursor uint64
	var jobs []*job.Job

	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan jobs: %w", err)
		}

		for _, key := range keys {
			data, err := r.client.Get(ctx, key).Bytes()
			if err != nil {
				continue
			}

			var j job.Job
			if err := json.Unmarshal(data, &j); err != nil {
				continue
			}

			// Apply filters
			if filter.Queue != "" && j.Queue != filter.Queue {
				continue
			}
			if filter.Type != "" && j.Type != filter.Type {
				continue
			}
			if filter.WorkerID != "" && j.WorkerID != filter.WorkerID {
				continue
			}
			if len(filter.Status) > 0 {
				statusMatch := false
				for _, s := range filter.Status {
					if j.Status == s {
						statusMatch = true
						break
					}
				}
				if !statusMatch {
					continue
				}
			}
			if filter.CreatedAfter != nil && j.CreatedAt.Before(*filter.CreatedAfter) {
				continue
			}
			if filter.CreatedBefore != nil && j.CreatedAt.After(*filter.CreatedBefore) {
				continue
			}

			jobs = append(jobs, &j)
		}

		if cursor == 0 {
			break
		}
	}

	// Apply limit and offset
	if filter.Offset > 0 {
		if filter.Offset >= len(jobs) {
			return []*job.Job{}, nil
		}
		jobs = jobs[filter.Offset:]
	}
	if filter.Limit > 0 && len(jobs) > filter.Limit {
		jobs = jobs[:filter.Limit]
	}

	return jobs, nil
}

// UpdateStatus updates only the job status
func (r *RedisStore) UpdateStatus(ctx context.Context, jobID string, status job.Status) error {
	j, err := r.Get(ctx, jobID)
	if err != nil {
		return err
	}

	j.Status = status
	if status == job.StatusRunning && j.StartedAt == nil {
		now := time.Now()
		j.StartedAt = &now
	}
	if j.IsTerminal() && j.CompletedAt == nil {
		now := time.Now()
		j.CompletedAt = &now
	}

	return r.Update(ctx, j)
}

// UpdateProgress updates job progress
func (r *RedisStore) UpdateProgress(ctx context.Context, jobID string, progress int) error {
	j, err := r.Get(ctx, jobID)
	if err != nil {
		return err
	}

	j.Progress = progress
	return r.Update(ctx, j)
}

// SetResult sets the job result
func (r *RedisStore) SetResult(ctx context.Context, jobID string, result []byte) error {
	j, err := r.Get(ctx, jobID)
	if err != nil {
		return err
	}

	j.Result = result
	return r.Update(ctx, j)
}

// SetError sets the job error
func (r *RedisStore) SetError(ctx context.Context, jobID string, errMsg string) error {
	j, err := r.Get(ctx, jobID)
	if err != nil {
		return err
	}

	j.Error = errMsg
	return r.Update(ctx, j)
}

// Health checks storage health
func (r *RedisStore) Health(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close closes the storage connection
func (r *RedisStore) Close() error {
	return r.client.Close()
}
