package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/storage"
	"github.com/Distributed-Task-Queue-System/pkg/errors"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// RecordJobCreated records a job creation event
func (r *RedisStore) RecordJobCreated(ctx context.Context, queueName string, jobType string) error {
	if queueName == "" {
		return errors.ErrInvalidValue
	}

	// Increment queue stats
	queueStatsKey := keyPrefixQueueStats + queueName
	pipe := r.client.Pipeline()
	pipe.HIncrBy(ctx, queueStatsKey, "pending_count", 1)
	pipe.HSet(ctx, queueStatsKey, "last_job_at", time.Now().Unix())

	// Increment job type stats
	if jobType != "" {
		jobTypeKey := keyPrefixJobType + jobType
		pipe.HIncrBy(ctx, jobTypeKey, "total_count", 1)
		pipe.HSet(ctx, jobTypeKey, "last_executed_at", time.Now().Unix())
	}

	_, err := pipe.Exec(ctx)
	return err
}

// RecordJobCompleted records a job completion event
func (r *RedisStore) RecordJobCompleted(ctx context.Context, queueName string, jobType string, duration time.Duration) error {
	if queueName == "" {
		return errors.ErrInvalidValue
	}

	queueStatsKey := keyPrefixQueueStats + queueName
	pipe := r.client.Pipeline()

	// Update queue stats
	pipe.HIncrBy(ctx, queueStatsKey, "success_count", 1)
	pipe.HIncrBy(ctx, queueStatsKey, "running_count", -1)
	pipe.HSet(ctx, queueStatsKey, "last_job_at", time.Now().Unix())

	// Update average processing time (simplified - store sum and count)
	pipe.HIncrBy(ctx, queueStatsKey, "total_duration_ms", duration.Milliseconds())
	pipe.HIncrBy(ctx, queueStatsKey, "completed_count", 1)

	// Update job type stats
	if jobType != "" {
		jobTypeKey := keyPrefixJobType + jobType
		pipe.HIncrBy(ctx, jobTypeKey, "success_count", 1)
		pipe.HIncrBy(ctx, jobTypeKey, "total_duration_ms", duration.Milliseconds())

		// Update min/max duration
		minDur, _ := r.client.HGet(ctx, jobTypeKey, "min_duration_ms").Int64()
		maxDur, _ := r.client.HGet(ctx, jobTypeKey, "max_duration_ms").Int64()

		durationMs := duration.Milliseconds()
		if minDur == 0 || durationMs < minDur {
			pipe.HSet(ctx, jobTypeKey, "min_duration_ms", durationMs)
		}
		if durationMs > maxDur {
			pipe.HSet(ctx, jobTypeKey, "max_duration_ms", durationMs)
		}
	}

	_, err := pipe.Exec(ctx)
	return err
}

// RecordJobFailed records a job failure event
func (r *RedisStore) RecordJobFailed(ctx context.Context, queueName string, jobType string, errType string) error {
	if queueName == "" {
		return errors.ErrInvalidValue
	}

	queueStatsKey := keyPrefixQueueStats + queueName
	pipe := r.client.Pipeline()

	pipe.HIncrBy(ctx, queueStatsKey, "failed_count", 1)
	pipe.HIncrBy(ctx, queueStatsKey, "running_count", -1)
	pipe.HSet(ctx, queueStatsKey, "last_job_at", time.Now().Unix())

	// Update job type stats
	if jobType != "" {
		jobTypeKey := keyPrefixJobType + jobType
		pipe.HIncrBy(ctx, jobTypeKey, "failed_count", 1)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// RecordJobRetried records a job retry event
func (r *RedisStore) RecordJobRetried(ctx context.Context, queueName string, jobType string) error {
	if queueName == "" {
		return errors.ErrInvalidValue
	}

	queueStatsKey := keyPrefixQueueStats + queueName
	return r.client.HIncrBy(ctx, queueStatsKey, "retried_count", 1).Err()
}

// GetQueueStats retrieves statistics for a queue
func (r *RedisStore) GetQueueStats(ctx context.Context, queueName string) (*job.QueueStats, error) {
	if queueName == "" {
		return nil, errors.ErrInvalidValue
	}

	queueStatsKey := keyPrefixQueueStats + queueName
	data, err := r.client.HGetAll(ctx, queueStatsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	stats := &job.QueueStats{
		QueueName: queueName,
	}

	if val, ok := data["pending_count"]; ok {
		fmt.Sscanf(val, "%d", &stats.PendingCount)
	}
	if val, ok := data["running_count"]; ok {
		fmt.Sscanf(val, "%d", &stats.RunningCount)
	}
	if val, ok := data["success_count"]; ok {
		fmt.Sscanf(val, "%d", &stats.SuccessCount)
	}
	if val, ok := data["failed_count"]; ok {
		fmt.Sscanf(val, "%d", &stats.FailedCount)
	}

	// Calculate average processing time
	var totalDuration, completedCount int64
	if val, ok := data["total_duration_ms"]; ok {
		fmt.Sscanf(val, "%d", &totalDuration)
	}
	if val, ok := data["completed_count"]; ok {
		fmt.Sscanf(val, "%d", &completedCount)
	}
	if completedCount > 0 {
		avgMs := totalDuration / completedCount
		stats.AvgProcessingTime = time.Duration(avgMs) * time.Millisecond
	}

	// Get last job timestamp
	if val, ok := data["last_job_at"]; ok {
		var timestamp int64
		fmt.Sscanf(val, "%d", &timestamp)
		t := time.Unix(timestamp, 0)
		stats.LastJobAt = &t
	}

	// Get active workers count (would need to query workers separately)
	workers, _ := r.List(ctx)
	for _, w := range workers {
		for _, q := range w.Queues {
			if q == queueName && w.Status == job.WorkerStatusProcessing {
				stats.ActiveWorkers++
			}
		}
	}

	return stats, nil
}

// GetQueueStatsAll retrieves statistics for all queues
func (r *RedisStore) GetQueueStatsAll(ctx context.Context) (map[string]*job.QueueStats, error) {
	queues, err := r.ListQueues(ctx)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]*job.QueueStats)
	for _, queueName := range queues {
		queueStats, err := r.GetQueueStats(ctx, queueName)
		if err != nil {
			continue
		}
		stats[queueName] = queueStats
	}

	return stats, nil
}

// GetJobTypeStats retrieves statistics grouped by job type
func (r *RedisStore) GetJobTypeStats(ctx context.Context, since time.Time) (map[string]*storage.JobTypeStats, error) {
	pattern := keyPrefixJobType + "*"
	var cursor uint64
	stats := make(map[string]*storage.JobTypeStats)

	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan job type stats: %w", err)
		}

		for _, key := range keys {
			jobType := key[len(keyPrefixJobType):]
			data, err := r.client.HGetAll(ctx, key).Result()
			if err != nil {
				continue
			}

			stat := &storage.JobTypeStats{
				JobType: jobType,
			}

			if val, ok := data["total_count"]; ok {
				fmt.Sscanf(val, "%d", &stat.TotalCount)
			}
			if val, ok := data["success_count"]; ok {
				fmt.Sscanf(val, "%d", &stat.SuccessCount)
			}
			if val, ok := data["failed_count"]; ok {
				fmt.Sscanf(val, "%d", &stat.FailedCount)
			}
			if val, ok := data["min_duration_ms"]; ok {
				var ms int64
				fmt.Sscanf(val, "%d", &ms)
				stat.MinDuration = time.Duration(ms) * time.Millisecond
			}
			if val, ok := data["max_duration_ms"]; ok {
				var ms int64
				fmt.Sscanf(val, "%d", &ms)
				stat.MaxDuration = time.Duration(ms) * time.Millisecond
			}
			if val, ok := data["total_duration_ms"]; ok {
				var ms int64
				fmt.Sscanf(val, "%d", &ms)
				if stat.SuccessCount > 0 {
					stat.AvgDuration = time.Duration(ms/stat.SuccessCount) * time.Millisecond
				}
			}
			if val, ok := data["last_executed_at"]; ok {
				var timestamp int64
				fmt.Sscanf(val, "%d", &timestamp)
				stat.LastExecutedAt = time.Unix(timestamp, 0)
			}

			stats[jobType] = stat
		}

		if cursor == 0 {
			break
		}
	}

	return stats, nil
}

// SaveQueueConfig saves a queue configuration
func (r *RedisStore) SaveQueueConfig(ctx context.Context, config *job.QueueConfig) error {
	if config == nil || config.Name == "" {
		return errors.ErrInvalidValue
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal queue config: %w", err)
	}

	key := keyPrefixConfig + config.Name
	return r.client.Set(ctx, key, data, 0).Err()
}

// GetQueueConfig retrieves a queue configuration
func (r *RedisStore) GetQueueConfig(ctx context.Context, queueName string) (*job.QueueConfig, error) {
	if queueName == "" {
		return nil, errors.ErrInvalidValue
	}

	key := keyPrefixConfig + queueName
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err.Error() == "redis: nil" {
			return nil, errors.ErrQueueNotFound
		}
		return nil, fmt.Errorf("failed to get queue config: %w", err)
	}

	var config job.QueueConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal queue config: %w", err)
	}

	return &config, nil
}

// DeleteQueueConfig deletes a queue configuration
func (r *RedisStore) DeleteQueueConfig(ctx context.Context, queueName string) error {
	if queueName == "" {
		return errors.ErrInvalidValue
	}

	key := keyPrefixConfig + queueName
	result, err := r.client.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to delete queue config: %w", err)
	}
	if result == 0 {
		return errors.ErrQueueNotFound
	}

	return nil
}

// ListQueueConfigs lists all queue configurations
func (r *RedisStore) ListQueueConfigs(ctx context.Context) ([]*job.QueueConfig, error) {
	pattern := keyPrefixConfig + "*"
	var cursor uint64
	var configs []*job.QueueConfig

	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue configs: %w", err)
		}

		for _, key := range keys {
			data, err := r.client.Get(ctx, key).Bytes()
			if err != nil {
				continue
			}

			var config job.QueueConfig
			if err := json.Unmarshal(data, &config); err != nil {
				continue
			}

			configs = append(configs, &config)
		}

		if cursor == 0 {
			break
		}
	}

	return configs, nil
}
