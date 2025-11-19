package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// ===== MetricsStore Implementation =====

func (p *PostgresStore) RecordJobCreated(ctx context.Context, queue string, jobType string) error {
	query := `
		INSERT INTO metrics (queue_name, job_type, event_type, recorded_at)
		VALUES ($1, $2, 'created', NOW())
	`

	_, err := p.db.ExecContext(ctx, query, queue, jobType)
	if err != nil {
		return fmt.Errorf("failed to record job created: %w", err)
	}

	return nil
}

func (p *PostgresStore) RecordJobCompleted(ctx context.Context, queue string, jobType string, duration time.Duration) error {
	query := `
		INSERT INTO metrics (queue_name, job_type, event_type, duration_ms, recorded_at)
		VALUES ($1, $2, 'completed', $3, NOW())
	`

	durationMs := duration.Milliseconds()
	_, err := p.db.ExecContext(ctx, query, queue, jobType, durationMs)
	if err != nil {
		return fmt.Errorf("failed to record job completed: %w", err)
	}

	return nil
}

func (p *PostgresStore) RecordJobFailed(ctx context.Context, queue string, jobType string) error {
	query := `
		INSERT INTO metrics (queue_name, job_type, event_type, recorded_at)
		VALUES ($1, $2, 'failed', NOW())
	`

	_, err := p.db.ExecContext(ctx, query, queue, jobType)
	if err != nil {
		return fmt.Errorf("failed to record job failed: %w", err)
	}

	return nil
}

func (p *PostgresStore) RecordJobRetried(ctx context.Context, queue string, jobType string) error {
	query := `
		INSERT INTO metrics (queue_name, job_type, event_type, recorded_at)
		VALUES ($1, $2, 'retried', NOW())
	`

	_, err := p.db.ExecContext(ctx, query, queue, jobType)
	if err != nil {
		return fmt.Errorf("failed to record job retried: %w", err)
	}

	return nil
}

func (p *PostgresStore) GetQueueStats(ctx context.Context, queue string) (*job.QueueStats, error) {
	stats := &job.QueueStats{
		QueueName: queue,
	}

	// Count jobs by status in the jobs table
	query := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'pending') as pending_count,
			COUNT(*) FILTER (WHERE status = 'running') as running_count,
			COUNT(*) FILTER (WHERE status = 'success') as success_count,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_count
		FROM jobs
		WHERE queue = $1
	`

	err := p.db.QueryRowContext(ctx, query, queue).Scan(
		&stats.PendingCount,
		&stats.RunningCount,
		&stats.SuccessCount,
		&stats.FailedCount,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	return stats, nil
}

func (p *PostgresStore) GetQueueStatsAll(ctx context.Context) (map[string]*job.QueueStats, error) {
	query := `
		SELECT
			queue,
			COUNT(*) FILTER (WHERE status = 'pending') as pending_count,
			COUNT(*) FILTER (WHERE status = 'running') as running_count,
			COUNT(*) FILTER (WHERE status = 'success') as success_count,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_count
		FROM jobs
		GROUP BY queue
	`

	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all queue stats: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*job.QueueStats)
	for rows.Next() {
		stats := &job.QueueStats{}
		err := rows.Scan(
			&stats.QueueName,
			&stats.PendingCount,
			&stats.RunningCount,
			&stats.SuccessCount,
			&stats.FailedCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue stats: %w", err)
		}
		result[stats.QueueName] = stats
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed during row iteration: %w", err)
	}

	return result, nil
}

func (p *PostgresStore) GetJobTypeStats(ctx context.Context, jobType string) (count int64, avgDuration time.Duration, err error) {
	query := `
		SELECT
			COUNT(*),
			COALESCE(AVG(duration_ms), 0)
		FROM metrics
		WHERE job_type = $1 AND event_type = 'completed' AND duration_ms IS NOT NULL
	`

	var avgMs float64
	err = p.db.QueryRowContext(ctx, query, jobType).Scan(&count, &avgMs)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get job type stats: %w", err)
	}

	avgDuration = time.Duration(avgMs) * time.Millisecond

	return count, avgDuration, nil
}
