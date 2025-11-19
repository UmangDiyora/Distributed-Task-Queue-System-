package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "github.com/Distributed-Task-Queue-System/internal/api/http"
	"github.com/Distributed-Task-Queue-System/internal/queue"
	"github.com/Distributed-Task-Queue-System/internal/scheduler"
	"github.com/Distributed-Task-Queue-System/internal/storage"
	"github.com/Distributed-Task-Queue-System/internal/storage/memory"
	"github.com/Distributed-Task-Queue-System/internal/storage/postgres"
	"github.com/Distributed-Task-Queue-System/internal/storage/redis"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

var (
	storageBackend = flag.String("storage", "memory", "Storage backend: memory, redis, postgres")
	httpAddr       = flag.String("http-addr", "0.0.0.0", "HTTP API address")
	httpPort       = flag.Int("http-port", 8080, "HTTP API port")
	redisAddr      = flag.String("redis-addr", "localhost:6379", "Redis address")
	postgresHost   = flag.String("postgres-host", "localhost", "PostgreSQL host")
	postgresPort   = flag.Int("postgres-port", 5432, "PostgreSQL port")
	postgresDB     = flag.String("postgres-db", "taskqueue", "PostgreSQL database")
	postgresUser   = flag.String("postgres-user", "postgres", "PostgreSQL user")
	postgresPass   = flag.String("postgres-pass", "", "PostgreSQL password")
)

func main() {
	flag.Parse()

	log.Println("Starting GoTask Distributed Task Queue Server...")

	// Initialize storage backend
	var store queue.Storage
	var err error

	switch *storageBackend {
	case "memory":
		log.Println("Using in-memory storage backend")
		store = memory.NewMemoryStore()

	case "redis":
		log.Printf("Using Redis storage backend at %s\n", *redisAddr)
		config := &storage.RedisConfig{
			Addr:     *redisAddr,
			Password: "",
			DB:       0,
		}
		redisStore, err := redis.NewRedisStore(config)
		if err != nil {
			log.Fatalf("Failed to initialize Redis storage: %v", err)
		}
		store = redisStore

	case "postgres":
		log.Printf("Using PostgreSQL storage backend at %s:%d\n", *postgresHost, *postgresPort)
		config := &storage.PostgresConfig{
			Host:            *postgresHost,
			Port:            *postgresPort,
			User:            *postgresUser,
			Password:        *postgresPass,
			Database:        *postgresDB,
			SSLMode:         "disable",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		}
		pgStore, err := postgres.NewPostgresStore(config)
		if err != nil {
			log.Fatalf("Failed to initialize PostgreSQL storage: %v", err)
		}
		store = pgStore

	default:
		log.Fatalf("Unknown storage backend: %s", *storageBackend)
	}

	// Initialize queue manager
	queueManager, err := queue.NewManager(store)
	if err != nil {
		log.Fatalf("Failed to initialize queue manager: %v", err)
	}

	// Register default queue
	defaultQueue := &job.QueueConfig{
		Name:           "default",
		MaxWorkers:     10,
		MaxRetries:     3,
		RetryStrategy:  job.RetryStrategyExponential,
		RetryDelay:     60,
		MaxRetryDelay:  3600,
		Timeout:        300,
		Priority:       job.PriorityNormal,
		Weight:         1,
	}
	if err := queueManager.RegisterQueue(context.Background(), defaultQueue); err != nil {
		log.Printf("Warning: Failed to register default queue: %v", err)
	}

	// Initialize scheduler
	sched := scheduler.NewScheduler(queueManager)

	// Start scheduler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx, 1*time.Second); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}

	// Initialize HTTP API server
	httpConfig := &httpapi.Config{
		Address: *httpAddr,
		Port:    *httpPort,
	}
	httpServer := httpapi.NewServer(queueManager, sched, httpConfig)

	// Start HTTP server in background
	go func() {
		if err := httpServer.Start(); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	log.Printf("GoTask server started successfully!")
	log.Printf("HTTP API listening on http://%s:%d", *httpAddr, *httpPort)
	log.Printf("Health check: http://%s:%d/health", *httpAddr, *httpPort)
	log.Printf("API documentation: http://%s:%d/api/v1", *httpAddr, *httpPort)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Printf("Error stopping HTTP server: %v", err)
	}

	// Stop scheduler
	if err := sched.Stop(); err != nil {
		log.Printf("Error stopping scheduler: %v", err)
	}

	log.Println("Server stopped gracefully")
}
