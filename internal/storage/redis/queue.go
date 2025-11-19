package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/UmangDiyora/gotask/pkg/errors"
	"github.com/UmangDiyora/gotask/pkg/job"
	"github.com/redis/go-redis/v9"
)

// Enqueue adds a job to a queue using a sorted set for priority
func (r *RedisStore) Enqueue(ctx context.Context, queueName string, jobID string, priority job.Priority) error {
	if queueName == "" || jobID == "" {
		return errors.ErrInvalidValue
	}

	// Check if queue is paused
	paused, err := r.IsQueuePaused(ctx, queueName)
	if err != nil {
		return err
	}
	if paused {
		return errors.ErrQueuePaused
	}

	// Use sorted set with score based on priority and timestamp
	// Higher priority = lower score (processed first)
	// Format: -priority.timestamp to maintain FIFO within same priority
	score := float64(-int(priority))*1e10 + float64(time.Now().UnixNano())

	key := keyQueueZSet + queueName
	if err := r.client.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: jobID,
	}).Err(); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	return nil
}

// Dequeue retrieves the next job from a queue (atomic claim operation)
func (r *RedisStore) Dequeue(ctx context.Context, queueName string, workerID string) (string, error) {
	if queueName == "" || workerID == "" {
		return "", errors.ErrInvalidValue
	}

	// Check if queue is paused
	paused, err := r.IsQueuePaused(ctx, queueName)
	if err != nil {
		return "", err
	}
	if paused {
		return "", errors.ErrQueuePaused
	}

	key := keyQueueZSet + queueName

	// Use Lua script for atomic pop operation
	script := redis.NewScript(`
		local jobs = redis.call('ZRANGE', KEYS[1], 0, 0)
		if #jobs > 0 then
			redis.call('ZREM', KEYS[1], jobs[1])
			return jobs[1]
		end
		return nil
	`)

	result, err := script.Run(ctx, r.client, []string{key}).Result()
	if err != nil {
		if err == redis.Nil {
			return "", errors.ErrEmptyQueue
		}
		return "", fmt.Errorf("failed to dequeue job: %w", err)
	}

	if result == nil {
		return "", errors.ErrEmptyQueue
	}

	jobID, ok := result.(string)
	if !ok {
		return "", errors.ErrInvalidOperation
	}

	return jobID, nil
}

// Requeue puts a job back into the queue
func (r *RedisStore) Requeue(ctx context.Context, queueName string, jobID string, priority job.Priority) error {
	// Simply enqueue again
	return r.Enqueue(ctx, queueName, jobID, priority)
}

// Remove removes a job from the queue
func (r *RedisStore) Remove(ctx context.Context, queueName string, jobID string) error {
	if queueName == "" || jobID == "" {
		return errors.ErrInvalidValue
	}

	key := keyQueueZSet + queueName
	result, err := r.client.ZRem(ctx, key, jobID).Result()
	if err != nil {
		return fmt.Errorf("failed to remove job from queue: %w", err)
	}
	if result == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

// Length returns the number of jobs in a queue
func (r *RedisStore) Length(ctx context.Context, queueName string) (int64, error) {
	if queueName == "" {
		return 0, errors.ErrInvalidValue
	}

	key := keyQueueZSet + queueName
	count, err := r.client.ZCard(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue length: %w", err)
	}

	return count, nil
}

// Peek returns the next job without dequeuing it
func (r *RedisStore) Peek(ctx context.Context, queueName string) (string, error) {
	if queueName == "" {
		return "", errors.ErrInvalidValue
	}

	key := keyQueueZSet + queueName
	jobs, err := r.client.ZRange(ctx, key, 0, 0).Result()
	if err != nil {
		return "", fmt.Errorf("failed to peek queue: %w", err)
	}

	if len(jobs) == 0 {
		return "", errors.ErrEmptyQueue
	}

	return jobs[0], nil
}

// GetScheduled retrieves jobs scheduled for execution before the given time
func (r *RedisStore) GetScheduled(ctx context.Context, before time.Time, limit int) ([]*job.Job, error) {
	// Use a sorted set for scheduled jobs with timestamp as score
	key := keyPrefixScheduled + "jobs"

	// Get jobs with score (timestamp) <= before
	jobIDs, err := r.client.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", before.Unix()),
		Count: int64(limit),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled jobs: %w", err)
	}

	var jobs []*job.Job
	for _, jobID := range jobIDs {
		j, err := r.Get(ctx, jobID)
		if err != nil {
			continue
		}
		jobs = append(jobs, j)
	}

	return jobs, nil
}

// ListQueues returns all queue names
func (r *RedisStore) ListQueues(ctx context.Context) ([]string, error) {
	pattern := keyQueueZSet + "*"
	var cursor uint64
	var queues []string

	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan queues: %w", err)
		}

		for _, key := range keys {
			// Remove prefix to get queue name
			queueName := key[len(keyQueueZSet):]
			queues = append(queues, queueName)
		}

		if cursor == 0 {
			break
		}
	}

	return queues, nil
}

// PauseQueue pauses a queue
func (r *RedisStore) PauseQueue(ctx context.Context, queueName string) error {
	if queueName == "" {
		return errors.ErrInvalidValue
	}

	key := keyPrefixQueuePause + queueName
	return r.client.Set(ctx, key, "1", 0).Err()
}

// ResumeQueue resumes a paused queue
func (r *RedisStore) ResumeQueue(ctx context.Context, queueName string) error {
	if queueName == "" {
		return errors.ErrInvalidValue
	}

	key := keyPrefixQueuePause + queueName
	return r.client.Del(ctx, key).Err()
}

// IsQueuePaused checks if a queue is paused
func (r *RedisStore) IsQueuePaused(ctx context.Context, queueName string) (bool, error) {
	if queueName == "" {
		return false, errors.ErrInvalidValue
	}

	key := keyPrefixQueuePause + queueName
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check queue pause status: %w", err)
	}

	return exists > 0, nil
}
