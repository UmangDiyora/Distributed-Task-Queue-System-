package storage

import (
	"context"
	"time"
)

const (
	// DefaultTimeout is the default timeout for storage operations
	DefaultTimeout = 30 * time.Second

	// DefaultPoolSize is the default connection pool size
	DefaultPoolSize = 10

	// DefaultMaxRetries is the default number of retries for failed operations
	DefaultMaxRetries = 3
)

// Config represents storage configuration
type Config struct {
	// Type specifies the storage backend type (redis, postgres, memory)
	Type string

	// Redis configuration
	Redis *RedisConfig

	// Postgres configuration
	Postgres *PostgresConfig

	// Common settings
	MaxRetries      int
	OperationTimeout time.Duration
}

// RedisConfig represents Redis-specific configuration
type RedisConfig struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
}

// PostgresConfig represents PostgreSQL-specific configuration
type PostgresConfig struct {
	DSN                 string
	MaxConnections      int
	MaxIdleConnections  int
	ConnectionLifetime  time.Duration
}

// DefaultConfig returns a default storage configuration
func DefaultConfig() *Config {
	return &Config{
		Type:             "memory",
		MaxRetries:       DefaultMaxRetries,
		OperationTimeout: DefaultTimeout,
	}
}

// WithContext returns a context with timeout
func WithContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return context.WithTimeout(ctx, timeout)
}
