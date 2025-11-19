package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/Distributed-Task-Queue-System/internal/storage"
	"github.com/Distributed-Task-Queue-System/pkg/errors"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// PostgresStore implements all storage interfaces using PostgreSQL
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a new PostgreSQL storage backend
func NewPostgresStore(config *storage.PostgresConfig) (*PostgresStore, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.Database, config.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &PostgresStore{db: db}

	// Initialize schema
	if err := store.initSchema(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the necessary tables if they don't exist
func (p *PostgresStore) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS jobs (
		id VARCHAR(255) PRIMARY KEY,
		type VARCHAR(255) NOT NULL,
		queue VARCHAR(255) NOT NULL,
		payload JSONB NOT NULL,
		result JSONB,
		error TEXT,
		status VARCHAR(50) NOT NULL,
		priority INTEGER NOT NULL DEFAULT 0,
		retry_count INTEGER NOT NULL DEFAULT 0,
		max_retries INTEGER NOT NULL DEFAULT 3,
		timeout_seconds INTEGER NOT NULL DEFAULT 300,
		progress INTEGER NOT NULL DEFAULT 0,
		metadata JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		scheduled_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_queue_status ON jobs(queue, status);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_type ON jobs(type);
	CREATE INDEX IF NOT EXISTS idx_jobs_scheduled_at ON jobs(scheduled_at);

	CREATE TABLE IF NOT EXISTS queues (
		id SERIAL PRIMARY KEY,
		queue_name VARCHAR(255) NOT NULL,
		job_id VARCHAR(255) NOT NULL,
		priority INTEGER NOT NULL DEFAULT 0,
		enqueued_at TIMESTAMP NOT NULL DEFAULT NOW(),
		scheduled_for TIMESTAMP NOT NULL DEFAULT NOW(),
		UNIQUE(queue_name, job_id),
		FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_queues_name_scheduled ON queues(queue_name, scheduled_for, priority DESC);

	CREATE TABLE IF NOT EXISTS workers (
		id VARCHAR(255) PRIMARY KEY,
		hostname VARCHAR(255) NOT NULL,
		version VARCHAR(50) NOT NULL,
		status VARCHAR(50) NOT NULL,
		queues JSONB NOT NULL,
		tags JSONB,
		concurrency INTEGER NOT NULL DEFAULT 1,
		current_job_id VARCHAR(255),
		processed_count BIGINT NOT NULL DEFAULT 0,
		failed_count BIGINT NOT NULL DEFAULT 0,
		started_at TIMESTAMP NOT NULL DEFAULT NOW(),
		last_heartbeat TIMESTAMP NOT NULL DEFAULT NOW(),
		metadata JSONB
	);

	CREATE INDEX IF NOT EXISTS idx_workers_status ON workers(status);
	CREATE INDEX IF NOT EXISTS idx_workers_heartbeat ON workers(last_heartbeat);

	CREATE TABLE IF NOT EXISTS queue_configs (
		name VARCHAR(255) PRIMARY KEY,
		max_workers INTEGER NOT NULL DEFAULT 10,
		max_retries INTEGER NOT NULL DEFAULT 3,
		retry_strategy VARCHAR(50) NOT NULL DEFAULT 'exponential',
		retry_delay_seconds INTEGER NOT NULL DEFAULT 60,
		max_retry_delay_seconds INTEGER NOT NULL DEFAULT 3600,
		timeout_seconds INTEGER NOT NULL DEFAULT 300,
		priority INTEGER NOT NULL DEFAULT 0,
		weight INTEGER NOT NULL DEFAULT 1,
		rate_limit INTEGER NOT NULL DEFAULT 0,
		dead_letter_queue VARCHAR(255),
		metadata JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS metrics (
		id SERIAL PRIMARY KEY,
		queue_name VARCHAR(255) NOT NULL,
		job_type VARCHAR(255) NOT NULL,
		event_type VARCHAR(50) NOT NULL,
		duration_ms BIGINT,
		recorded_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_metrics_queue ON metrics(queue_name, recorded_at);
	CREATE INDEX IF NOT EXISTS idx_metrics_type ON metrics(job_type, recorded_at);
	CREATE INDEX IF NOT EXISTS idx_metrics_event ON metrics(event_type, recorded_at);

	CREATE TABLE IF NOT EXISTS paused_queues (
		queue_name VARCHAR(255) PRIMARY KEY,
		paused_at TIMESTAMP NOT NULL DEFAULT NOW()
	);
	`

	_, err := p.db.ExecContext(ctx, schema)
	return err
}

// ===== JobStore Implementation =====

func (p *PostgresStore) CreateJob(ctx context.Context, j *job.Job) error {
	payloadJSON, err := json.Marshal(j.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	metadataJSON, err := json.Marshal(j.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO jobs (id, type, queue, payload, status, priority, retry_count,
						  max_retries, timeout_seconds, progress, metadata, created_at,
						  updated_at, scheduled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err = p.db.ExecContext(ctx, query,
		j.ID, j.Type, j.Queue, payloadJSON, j.Status, j.Priority,
		j.RetryCount, j.MaxRetries, j.TimeoutSeconds, j.Progress,
		metadataJSON, j.CreatedAt, j.UpdatedAt, j.ScheduledAt,
	)

	if err != nil {
		if isDuplicateKeyError(err) {
			return errors.ErrJobAlreadyExists
		}
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

func (p *PostgresStore) GetJob(ctx context.Context, id string) (*job.Job, error) {
	query := `
		SELECT id, type, queue, payload, result, error, status, priority,
			   retry_count, max_retries, timeout_seconds, progress, metadata,
			   created_at, updated_at, started_at, completed_at, scheduled_at
		FROM jobs
		WHERE id = $1
	`

	j := &job.Job{}
	var payloadJSON, metadataJSON []byte
	var resultJSON sql.NullString
	var errorStr sql.NullString
	var startedAt, completedAt sql.NullTime

	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&j.ID, &j.Type, &j.Queue, &payloadJSON, &resultJSON, &errorStr,
		&j.Status, &j.Priority, &j.RetryCount, &j.MaxRetries,
		&j.TimeoutSeconds, &j.Progress, &metadataJSON,
		&j.CreatedAt, &j.UpdatedAt, &startedAt, &completedAt, &j.ScheduledAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal(payloadJSON, &j.Payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	if err := json.Unmarshal(metadataJSON, &j.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	if resultJSON.Valid {
		if err := json.Unmarshal([]byte(resultJSON.String), &j.Result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	if errorStr.Valid {
		j.Error = errorStr.String
	}

	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}

	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}

	return j, nil
}

func (p *PostgresStore) UpdateJob(ctx context.Context, j *job.Job) error {
	payloadJSON, err := json.Marshal(j.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	metadataJSON, err := json.Marshal(j.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var resultJSON []byte
	if j.Result != nil {
		resultJSON, err = json.Marshal(j.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
	}

	query := `
		UPDATE jobs
		SET type = $2, queue = $3, payload = $4, result = $5, error = $6,
			status = $7, priority = $8, retry_count = $9, max_retries = $10,
			timeout_seconds = $11, progress = $12, metadata = $13, updated_at = $14,
			started_at = $15, completed_at = $16, scheduled_at = $17
		WHERE id = $1
	`

	result, err := p.db.ExecContext(ctx, query,
		j.ID, j.Type, j.Queue, payloadJSON, resultJSON, j.Error,
		j.Status, j.Priority, j.RetryCount, j.MaxRetries,
		j.TimeoutSeconds, j.Progress, metadataJSON, j.UpdatedAt,
		j.StartedAt, j.CompletedAt, j.ScheduledAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

func (p *PostgresStore) DeleteJob(ctx context.Context, id string) error {
	query := `DELETE FROM jobs WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

func (p *PostgresStore) ListJobs(ctx context.Context, queue string, status job.Status, limit, offset int) ([]*job.Job, error) {
	query := `
		SELECT id, type, queue, payload, result, error, status, priority,
			   retry_count, max_retries, timeout_seconds, progress, metadata,
			   created_at, updated_at, started_at, completed_at, scheduled_at
		FROM jobs
		WHERE ($1 = '' OR queue = $1) AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := p.db.QueryContext(ctx, query, queue, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*job.Job
	for rows.Next() {
		j := &job.Job{}
		var payloadJSON, metadataJSON []byte
		var resultJSON sql.NullString
		var errorStr sql.NullString
		var startedAt, completedAt sql.NullTime

		err := rows.Scan(
			&j.ID, &j.Type, &j.Queue, &payloadJSON, &resultJSON, &errorStr,
			&j.Status, &j.Priority, &j.RetryCount, &j.MaxRetries,
			&j.TimeoutSeconds, &j.Progress, &metadataJSON,
			&j.CreatedAt, &j.UpdatedAt, &startedAt, &completedAt, &j.ScheduledAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal(payloadJSON, &j.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		if err := json.Unmarshal(metadataJSON, &j.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		if resultJSON.Valid {
			if err := json.Unmarshal([]byte(resultJSON.String), &j.Result); err != nil {
				return nil, fmt.Errorf("failed to unmarshal result: %w", err)
			}
		}

		if errorStr.Valid {
			j.Error = errorStr.String
		}

		if startedAt.Valid {
			j.StartedAt = &startedAt.Time
		}

		if completedAt.Valid {
			j.CompletedAt = &completedAt.Time
		}

		jobs = append(jobs, j)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed during row iteration: %w", err)
	}

	return jobs, nil
}

func (p *PostgresStore) UpdateStatus(ctx context.Context, id string, status job.Status) error {
	query := `
		UPDATE jobs
		SET status = $2,
			updated_at = $3,
			started_at = CASE WHEN $2 = 'running' AND started_at IS NULL THEN $3 ELSE started_at END,
			completed_at = CASE WHEN $2 IN ('success', 'failed', 'cancelled', 'expired') AND completed_at IS NULL THEN $3 ELSE completed_at END
		WHERE id = $1
	`

	now := time.Now()
	result, err := p.db.ExecContext(ctx, query, id, status, now)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

func (p *PostgresStore) UpdateProgress(ctx context.Context, id string, progress int) error {
	query := `UPDATE jobs SET progress = $2, updated_at = $3 WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, id, progress, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update progress: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

func (p *PostgresStore) SetResult(ctx context.Context, id string, result []byte) error {
	query := `UPDATE jobs SET result = $2, updated_at = $3 WHERE id = $1`

	res, err := p.db.ExecContext(ctx, query, id, result, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set result: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

func (p *PostgresStore) SetError(ctx context.Context, id string, errMsg string) error {
	query := `UPDATE jobs SET error = $2, updated_at = $3 WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, id, errMsg, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set error: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

// ===== QueueStore Implementation =====

func (p *PostgresStore) Enqueue(ctx context.Context, queue string, jobID string, priority job.Priority, delay time.Duration) error {
	scheduledFor := time.Now().Add(delay)

	query := `
		INSERT INTO queues (queue_name, job_id, priority, scheduled_for)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (queue_name, job_id) DO UPDATE SET priority = $3, scheduled_for = $4
	`

	_, err := p.db.ExecContext(ctx, query, queue, jobID, priority, scheduledFor)
	if err != nil {
		if isForeignKeyError(err) {
			return errors.ErrJobNotFound
		}
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	return nil
}

func (p *PostgresStore) Dequeue(ctx context.Context, queue string) (string, error) {
	// Check if queue is paused
	paused, err := p.IsQueuePaused(ctx, queue)
	if err != nil {
		return "", err
	}
	if paused {
		return "", errors.ErrQueuePaused
	}

	// Use a transaction to atomically get and delete
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get the highest priority job that's ready
	query := `
		SELECT job_id
		FROM queues
		WHERE queue_name = $1 AND scheduled_for <= $2
		ORDER BY priority DESC, scheduled_for ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`

	var jobID string
	err = tx.QueryRowContext(ctx, query, queue, time.Now()).Scan(&jobID)
	if err == sql.ErrNoRows {
		return "", errors.ErrQueueEmpty
	}
	if err != nil {
		return "", fmt.Errorf("failed to dequeue job: %w", err)
	}

	// Remove from queue
	deleteQuery := `DELETE FROM queues WHERE queue_name = $1 AND job_id = $2`
	_, err = tx.ExecContext(ctx, deleteQuery, queue, jobID)
	if err != nil {
		return "", fmt.Errorf("failed to remove from queue: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return jobID, nil
}

func (p *PostgresStore) Requeue(ctx context.Context, queue string, jobID string, priority job.Priority, delay time.Duration) error {
	return p.Enqueue(ctx, queue, jobID, priority, delay)
}

func (p *PostgresStore) Remove(ctx context.Context, queue string, jobID string) error {
	query := `DELETE FROM queues WHERE queue_name = $1 AND job_id = $2`

	result, err := p.db.ExecContext(ctx, query, queue, jobID)
	if err != nil {
		return fmt.Errorf("failed to remove from queue: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrJobNotFound
	}

	return nil
}

func (p *PostgresStore) Length(ctx context.Context, queue string) (int64, error) {
	query := `SELECT COUNT(*) FROM queues WHERE queue_name = $1`

	var count int64
	err := p.db.QueryRowContext(ctx, query, queue).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get queue length: %w", err)
	}

	return count, nil
}

func (p *PostgresStore) Peek(ctx context.Context, queue string, count int) ([]string, error) {
	query := `
		SELECT job_id
		FROM queues
		WHERE queue_name = $1
		ORDER BY priority DESC, scheduled_for ASC
		LIMIT $2
	`

	rows, err := p.db.QueryContext(ctx, query, queue, count)
	if err != nil {
		return nil, fmt.Errorf("failed to peek queue: %w", err)
	}
	defer rows.Close()

	var jobIDs []string
	for rows.Next() {
		var jobID string
		if err := rows.Scan(&jobID); err != nil {
			return nil, fmt.Errorf("failed to scan job ID: %w", err)
		}
		jobIDs = append(jobIDs, jobID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed during row iteration: %w", err)
	}

	return jobIDs, nil
}

func (p *PostgresStore) GetScheduled(ctx context.Context, queue string, until time.Time) ([]string, error) {
	query := `
		SELECT job_id
		FROM queues
		WHERE queue_name = $1 AND scheduled_for <= $2
		ORDER BY scheduled_for ASC
	`

	rows, err := p.db.QueryContext(ctx, query, queue, until)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled jobs: %w", err)
	}
	defer rows.Close()

	var jobIDs []string
	for rows.Next() {
		var jobID string
		if err := rows.Scan(&jobID); err != nil {
			return nil, fmt.Errorf("failed to scan job ID: %w", err)
		}
		jobIDs = append(jobIDs, jobID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed during row iteration: %w", err)
	}

	return jobIDs, nil
}

func (p *PostgresStore) PauseQueue(ctx context.Context, queue string) error {
	query := `INSERT INTO paused_queues (queue_name) VALUES ($1) ON CONFLICT DO NOTHING`

	_, err := p.db.ExecContext(ctx, query, queue)
	if err != nil {
		return fmt.Errorf("failed to pause queue: %w", err)
	}

	return nil
}

func (p *PostgresStore) ResumeQueue(ctx context.Context, queue string) error {
	query := `DELETE FROM paused_queues WHERE queue_name = $1`

	_, err := p.db.ExecContext(ctx, query, queue)
	if err != nil {
		return fmt.Errorf("failed to resume queue: %w", err)
	}

	return nil
}

func (p *PostgresStore) IsQueuePaused(ctx context.Context, queue string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM paused_queues WHERE queue_name = $1)`

	var paused bool
	err := p.db.QueryRowContext(ctx, query, queue).Scan(&paused)
	if err != nil {
		return false, fmt.Errorf("failed to check if queue is paused: %w", err)
	}

	return paused, nil
}

// Continued in next part due to length...
