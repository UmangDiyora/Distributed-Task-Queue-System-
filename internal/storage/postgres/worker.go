package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Distributed-Task-Queue-System/pkg/errors"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// ===== WorkerStore Implementation =====

func (p *PostgresStore) RegisterWorker(ctx context.Context, w *job.Worker) error {
	queuesJSON, err := json.Marshal(w.Queues)
	if err != nil {
		return fmt.Errorf("failed to marshal queues: %w", err)
	}

	tagsJSON, err := json.Marshal(w.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	metadataJSON, err := json.Marshal(w.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO workers (id, hostname, version, status, queues, tags, concurrency,
							 processed_count, failed_count, started_at, last_heartbeat, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			hostname = $2,
			version = $3,
			status = $4,
			queues = $5,
			tags = $6,
			concurrency = $7,
			last_heartbeat = $11
	`

	_, err = p.db.ExecContext(ctx, query,
		w.ID, w.Hostname, w.Version, w.Status, queuesJSON, tagsJSON,
		w.Concurrency, w.ProcessedCount, w.FailedCount,
		w.StartedAt, w.LastHeartbeat, metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}

	return nil
}

func (p *PostgresStore) UnregisterWorker(ctx context.Context, id string) error {
	query := `DELETE FROM workers WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to unregister worker: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrWorkerNotFound
	}

	return nil
}

func (p *PostgresStore) GetWorker(ctx context.Context, id string) (*job.Worker, error) {
	query := `
		SELECT id, hostname, version, status, queues, tags, concurrency,
			   current_job_id, processed_count, failed_count, started_at,
			   last_heartbeat, metadata
		FROM workers
		WHERE id = $1
	`

	w := &job.Worker{}
	var queuesJSON, tagsJSON, metadataJSON []byte
	var currentJobID sql.NullString

	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&w.ID, &w.Hostname, &w.Version, &w.Status, &queuesJSON, &tagsJSON,
		&w.Concurrency, &currentJobID, &w.ProcessedCount, &w.FailedCount,
		&w.StartedAt, &w.LastHeartbeat, &metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, errors.ErrWorkerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get worker: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal(queuesJSON, &w.Queues); err != nil {
		return nil, fmt.Errorf("failed to unmarshal queues: %w", err)
	}

	if err := json.Unmarshal(tagsJSON, &w.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	if err := json.Unmarshal(metadataJSON, &w.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	if currentJobID.Valid {
		w.CurrentJobID = &currentJobID.String
	}

	return w, nil
}

func (p *PostgresStore) ListWorkers(ctx context.Context, queue string) ([]*job.Worker, error) {
	var query string
	var args []interface{}

	if queue == "" {
		query = `
			SELECT id, hostname, version, status, queues, tags, concurrency,
				   current_job_id, processed_count, failed_count, started_at,
				   last_heartbeat, metadata
			FROM workers
			ORDER BY started_at DESC
		`
	} else {
		query = `
			SELECT id, hostname, version, status, queues, tags, concurrency,
				   current_job_id, processed_count, failed_count, started_at,
				   last_heartbeat, metadata
			FROM workers
			WHERE queues @> $1::jsonb
			ORDER BY started_at DESC
		`
		queueJSON, _ := json.Marshal([]string{queue})
		args = append(args, queueJSON)
	}

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list workers: %w", err)
	}
	defer rows.Close()

	var workers []*job.Worker
	for rows.Next() {
		w := &job.Worker{}
		var queuesJSON, tagsJSON, metadataJSON []byte
		var currentJobID sql.NullString

		err := rows.Scan(
			&w.ID, &w.Hostname, &w.Version, &w.Status, &queuesJSON, &tagsJSON,
			&w.Concurrency, &currentJobID, &w.ProcessedCount, &w.FailedCount,
			&w.StartedAt, &w.LastHeartbeat, &metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan worker: %w", err)
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal(queuesJSON, &w.Queues); err != nil {
			return nil, fmt.Errorf("failed to unmarshal queues: %w", err)
		}

		if err := json.Unmarshal(tagsJSON, &w.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		if err := json.Unmarshal(metadataJSON, &w.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		if currentJobID.Valid {
			w.CurrentJobID = &currentJobID.String
		}

		workers = append(workers, w)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed during row iteration: %w", err)
	}

	return workers, nil
}

func (p *PostgresStore) UpdateHeartbeat(ctx context.Context, id string) error {
	query := `UPDATE workers SET last_heartbeat = $2 WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, id, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrWorkerNotFound
	}

	return nil
}

func (p *PostgresStore) UpdateWorkerStatus(ctx context.Context, id string, status job.WorkerStatus) error {
	query := `UPDATE workers SET status = $2 WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("failed to update worker status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrWorkerNotFound
	}

	return nil
}

func (p *PostgresStore) SetCurrentJob(ctx context.Context, workerID string, jobID string) error {
	var query string
	if jobID == "" {
		query = `UPDATE workers SET current_job_id = NULL WHERE id = $1`
	} else {
		query = `UPDATE workers SET current_job_id = $2 WHERE id = $1`
	}

	var result sql.Result
	var err error

	if jobID == "" {
		result, err = p.db.ExecContext(ctx, query, workerID)
	} else {
		result, err = p.db.ExecContext(ctx, query, workerID, jobID)
	}

	if err != nil {
		return fmt.Errorf("failed to set current job: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrWorkerNotFound
	}

	return nil
}

func (p *PostgresStore) IncrementProcessed(ctx context.Context, workerID string) error {
	query := `UPDATE workers SET processed_count = processed_count + 1 WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, workerID)
	if err != nil {
		return fmt.Errorf("failed to increment processed count: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrWorkerNotFound
	}

	return nil
}

func (p *PostgresStore) IncrementFailed(ctx context.Context, workerID string) error {
	query := `UPDATE workers SET failed_count = failed_count + 1 WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, workerID)
	if err != nil {
		return fmt.Errorf("failed to increment failed count: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errors.ErrWorkerNotFound
	}

	return nil
}

func (p *PostgresStore) GetStaleWorkers(ctx context.Context, threshold time.Duration) ([]*job.Worker, error) {
	cutoff := time.Now().Add(-threshold)

	query := `
		SELECT id, hostname, version, status, queues, tags, concurrency,
			   current_job_id, processed_count, failed_count, started_at,
			   last_heartbeat, metadata
		FROM workers
		WHERE last_heartbeat < $1
		ORDER BY last_heartbeat ASC
	`

	rows, err := p.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to get stale workers: %w", err)
	}
	defer rows.Close()

	var workers []*job.Worker
	for rows.Next() {
		w := &job.Worker{}
		var queuesJSON, tagsJSON, metadataJSON []byte
		var currentJobID sql.NullString

		err := rows.Scan(
			&w.ID, &w.Hostname, &w.Version, &w.Status, &queuesJSON, &tagsJSON,
			&w.Concurrency, &currentJobID, &w.ProcessedCount, &w.FailedCount,
			&w.StartedAt, &w.LastHeartbeat, &metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan worker: %w", err)
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal(queuesJSON, &w.Queues); err != nil {
			return nil, fmt.Errorf("failed to unmarshal queues: %w", err)
		}

		if err := json.Unmarshal(tagsJSON, &w.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		if err := json.Unmarshal(metadataJSON, &w.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		if currentJobID.Valid {
			w.CurrentJobID = &currentJobID.String
		}

		workers = append(workers, w)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed during row iteration: %w", err)
	}

	return workers, nil
}
