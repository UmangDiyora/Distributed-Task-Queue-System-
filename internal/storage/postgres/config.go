package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// ===== ConfigStore Implementation =====

func (p *PostgresStore) SaveQueueConfig(ctx context.Context, config *job.QueueConfig) error {
	metadataJSON, err := json.Marshal(config.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO queue_configs (name, max_workers, max_retries, retry_strategy,
								   retry_delay_seconds, max_retry_delay_seconds, timeout_seconds,
								   priority, weight, rate_limit, dead_letter_queue, metadata,
								   created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (name) DO UPDATE SET
			max_workers = $2,
			max_retries = $3,
			retry_strategy = $4,
			retry_delay_seconds = $5,
			max_retry_delay_seconds = $6,
			timeout_seconds = $7,
			priority = $8,
			weight = $9,
			rate_limit = $10,
			dead_letter_queue = $11,
			metadata = $12,
			updated_at = $14
	`

	now := time.Now()
	_, err = p.db.ExecContext(ctx, query,
		config.Name, config.MaxWorkers, config.MaxRetries, config.RetryStrategy,
		config.RetryDelay, config.MaxRetryDelay, config.Timeout,
		config.Priority, config.Weight, config.RateLimit,
		config.DeadLetterQueue, metadataJSON, now, now,
	)

	if err != nil {
		return fmt.Errorf("failed to save queue config: %w", err)
	}

	return nil
}

func (p *PostgresStore) GetQueueConfig(ctx context.Context, queue string) (*job.QueueConfig, error) {
	query := `
		SELECT name, max_workers, max_retries, retry_strategy,
			   retry_delay_seconds, max_retry_delay_seconds, timeout_seconds,
			   priority, weight, rate_limit, dead_letter_queue, metadata,
			   created_at, updated_at
		FROM queue_configs
		WHERE name = $1
	`

	config := &job.QueueConfig{}
	var metadataJSON []byte
	var deadLetterQueue sql.NullString
	var createdAt, updatedAt time.Time

	err := p.db.QueryRowContext(ctx, query, queue).Scan(
		&config.Name, &config.MaxWorkers, &config.MaxRetries, &config.RetryStrategy,
		&config.RetryDelay, &config.MaxRetryDelay, &config.Timeout,
		&config.Priority, &config.Weight, &config.RateLimit,
		&deadLetterQueue, &metadataJSON, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("queue config not found: %s", queue)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get queue config: %w", err)
	}

	// Unmarshal metadata
	if err := json.Unmarshal(metadataJSON, &config.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	if deadLetterQueue.Valid {
		config.DeadLetterQueue = deadLetterQueue.String
	}

	return config, nil
}

func (p *PostgresStore) DeleteQueueConfig(ctx context.Context, queue string) error {
	query := `DELETE FROM queue_configs WHERE name = $1`

	result, err := p.db.ExecContext(ctx, query, queue)
	if err != nil {
		return fmt.Errorf("failed to delete queue config: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("queue config not found: %s", queue)
	}

	return nil
}

func (p *PostgresStore) ListQueueConfigs(ctx context.Context) ([]*job.QueueConfig, error) {
	query := `
		SELECT name, max_workers, max_retries, retry_strategy,
			   retry_delay_seconds, max_retry_delay_seconds, timeout_seconds,
			   priority, weight, rate_limit, dead_letter_queue, metadata,
			   created_at, updated_at
		FROM queue_configs
		ORDER BY name
	`

	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list queue configs: %w", err)
	}
	defer rows.Close()

	var configs []*job.QueueConfig
	for rows.Next() {
		config := &job.QueueConfig{}
		var metadataJSON []byte
		var deadLetterQueue sql.NullString
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&config.Name, &config.MaxWorkers, &config.MaxRetries, &config.RetryStrategy,
			&config.RetryDelay, &config.MaxRetryDelay, &config.Timeout,
			&config.Priority, &config.Weight, &config.RateLimit,
			&deadLetterQueue, &metadataJSON, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue config: %w", err)
		}

		// Unmarshal metadata
		if err := json.Unmarshal(metadataJSON, &config.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		if deadLetterQueue.Valid {
			config.DeadLetterQueue = deadLetterQueue.String
		}

		configs = append(configs, config)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed during row iteration: %w", err)
	}

	return configs, nil
}
