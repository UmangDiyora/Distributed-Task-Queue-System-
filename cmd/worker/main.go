package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/queue"
	"github.com/Distributed-Task-Queue-System/internal/storage"
	"github.com/Distributed-Task-Queue-System/internal/storage/memory"
	"github.com/Distributed-Task-Queue-System/internal/storage/postgres"
	"github.com/Distributed-Task-Queue-System/internal/storage/redis"
	"github.com/Distributed-Task-Queue-System/internal/worker"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

var (
	storageBackend = flag.String("storage", "memory", "Storage backend: memory, redis, postgres")
	workerID       = flag.String("worker-id", "", "Worker ID (auto-generated if not provided)")
	queues         = flag.String("queues", "default", "Comma-separated list of queues to process")
	concurrency    = flag.Int("concurrency", 0, "Number of concurrent workers (defaults to CPU count)")
	redisAddr      = flag.String("redis-addr", "localhost:6379", "Redis address")
	postgresHost   = flag.String("postgres-host", "localhost", "PostgreSQL host")
	postgresPort   = flag.Int("postgres-port", 5432, "PostgreSQL port")
	postgresDB     = flag.String("postgres-db", "taskqueue", "PostgreSQL database")
	postgresUser   = flag.String("postgres-user", "postgres", "PostgreSQL user")
	postgresPass   = flag.String("postgres-pass", "", "PostgreSQL password")
)

func main() {
	flag.Parse()

	log.Println("Starting GoTask Worker...")

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

	// Parse queues
	queueList := []string{"default"}
	if *queues != "" {
		queueList = []string{*queues}
		// In a real implementation, split by comma
		// queueList = strings.Split(*queues, ",")
	}

	// Initialize worker pool
	workerConfig := &worker.Config{
		ID:                *workerID,
		Queues:            queueList,
		Concurrency:       *concurrency,
		PollInterval:      1 * time.Second,
		HeartbeatInterval: 30 * time.Second,
		ShutdownTimeout:   30 * time.Second,
	}

	pool, err := worker.NewPool(queueManager, workerConfig)
	if err != nil {
		log.Fatalf("Failed to create worker pool: %v", err)
	}

	// Register job handlers
	registerHandlers(pool)

	// Start worker pool
	if err := pool.Start(); err != nil {
		log.Fatalf("Failed to start worker pool: %v", err)
	}

	log.Printf("Worker started successfully!")
	log.Printf("Processing queues: %v", queueList)
	log.Printf("Concurrency: %d", pool.GetWorker().Concurrency)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down worker...")

	// Graceful shutdown
	if err := pool.Stop(); err != nil {
		log.Printf("Error stopping worker pool: %v", err)
	}

	log.Println("Worker stopped gracefully")
}

// registerHandlers registers job type handlers
func registerHandlers(pool *worker.Pool) {
	// Example: Echo job - just prints the payload
	pool.RegisterHandler("echo", func(ctx context.Context, j *job.Job) error {
		log.Printf("Processing echo job %s", j.ID)

		// Simulate work
		time.Sleep(2 * time.Second)

		// Update progress
		// (In real implementation, you'd get queueManager from pool)
		fmt.Printf("Echo job payload: %+v\n", j.Payload)

		return nil
	})

	// Example: Math job - performs simple calculations
	pool.RegisterHandler("math", func(ctx context.Context, j *job.Job) error {
		log.Printf("Processing math job %s", j.ID)

		operation, ok := j.Payload["operation"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid operation")
		}

		a, ok := j.Payload["a"].(float64)
		if !ok {
			return fmt.Errorf("missing or invalid operand a")
		}

		b, ok := j.Payload["b"].(float64)
		if !ok {
			return fmt.Errorf("missing or invalid operand b")
		}

		var result float64
		switch operation {
		case "add":
			result = a + b
		case "subtract":
			result = a - b
		case "multiply":
			result = a * b
		case "divide":
			if b == 0 {
				return fmt.Errorf("division by zero")
			}
			result = a / b
		default:
			return fmt.Errorf("unknown operation: %s", operation)
		}

		// Set result
		resultBytes, _ := json.Marshal(map[string]float64{"result": result})
		j.Result = resultBytes

		log.Printf("Math job result: %f %s %f = %f", a, operation, b, result)

		return nil
	})

	// Example: Email job - simulates sending an email
	pool.RegisterHandler("email", func(ctx context.Context, j *job.Job) error {
		log.Printf("Processing email job %s", j.ID)

		to, _ := j.Payload["to"].(string)
		subject, _ := j.Payload["subject"].(string)
		body, _ := j.Payload["body"].(string)

		// Simulate email sending
		time.Sleep(1 * time.Second)

		log.Printf("Email sent to: %s, subject: %s", to, subject)
		log.Printf("Body: %s", body)

		return nil
	})

	// Example: Report job - generates a report
	pool.RegisterHandler("report", func(ctx context.Context, j *job.Job) error {
		log.Printf("Processing report job %s", j.ID)

		reportType, _ := j.Payload["type"].(string)

		// Simulate report generation with progress updates
		for i := 0; i <= 100; i += 20 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				time.Sleep(500 * time.Millisecond)
				// Update progress (would need queueManager access)
				log.Printf("Report generation progress: %d%%", i)
			}
		}

		log.Printf("Report generated: %s", reportType)

		return nil
	})

	log.Println("Registered job handlers: echo, math, email, report")
}
