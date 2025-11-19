# GoTask - Distributed Task Queue System

A production-ready distributed task queue system built in Go, similar to Celery/RabbitMQ/Redis Queue, designed for handling asynchronous job processing with reliability, scalability, and comprehensive monitoring capabilities.

## Features

- **Distributed Architecture**: Support for multiple workers across different machines
- **Reliable Delivery**: At-least-once delivery guarantee for all jobs
- **Multiple Storage Backends**: Redis, PostgreSQL, and in-memory options
- **Smart Retry Logic**: Configurable retry strategies with exponential/linear backoff
- **Priority Queues**: Process important jobs first with multiple priority levels
- **Job Scheduling**: Support for delayed and recurring (cron-like) jobs
- **Progress Tracking**: Monitor job progress in real-time
- **Dead Letter Queue**: Automatic handling of failed jobs
- **REST & gRPC APIs**: Multiple API options for job submission
- **Web Dashboard**: Real-time monitoring and management interface
- **Prometheus Metrics**: Comprehensive metrics for monitoring

## Architecture

The system consists of several core components:

- **Queue Manager**: Central component managing job lifecycle
- **Worker Pool**: Concurrent workers processing jobs
- **Storage Backend**: Pluggable persistence layer (Redis, PostgreSQL, In-Memory)
- **API Server**: REST/gRPC endpoints for job submission
- **Scheduler**: Handles delayed and periodic jobs
- **Monitor**: Collects metrics and health status
- **Dashboard**: Web UI for monitoring and management

## Performance Targets

- Handle 10,000+ jobs per second
- Sub-second job pickup latency
- Support 1000+ concurrent workers
- 99.9% job completion reliability
- <100ms API response time (p95)

## Project Structure

```
gotask/
├── cmd/                  # Entry points
│   ├── server/          # API server
│   ├── worker/          # Worker process
│   └── dashboard/       # Dashboard server
├── internal/            # Private application code
│   ├── queue/          # Queue implementation
│   ├── worker/         # Worker pool logic
│   ├── storage/        # Storage backends
│   ├── api/            # API handlers
│   ├── scheduler/      # Job scheduling
│   ├── retry/          # Retry strategies
│   └── monitor/        # Metrics and monitoring
├── pkg/                # Public libraries
│   ├── job/           # Job definitions
│   ├── protocol/      # gRPC protobuf
│   └── errors/        # Custom errors
├── web/               # Dashboard frontend
├── configs/           # Configuration files
├── deployments/       # Docker and K8s configs
├── tests/            # Integration tests
├── docs/             # Documentation
└── examples/         # Usage examples
```

## Quick Start

### Using Docker Compose (Recommended)

1. **Clone the repository:**
   ```bash
   git clone https://github.com/UmangDiyora/Distributed-Task-Queue-System-.git
   cd Distributed-Task-Queue-System-
   ```

2. **Start all services:**
   ```bash
   docker-compose up -d
   ```

3. **Create a job:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/jobs \
     -H "Content-Type: application/json" \
     -d '{
       "type": "echo",
       "queue": "default",
       "payload": {"message": "Hello, GoTask!"},
       "priority": 1
     }'
   ```

4. **Check job status:**
   ```bash
   curl http://localhost:8080/api/v1/jobs/{job_id}
   ```

5. **View metrics:**
   - Prometheus: http://localhost:9091
   - Grafana: http://localhost:3000 (admin/admin)

### From Source

1. **Install Go 1.21+ and dependencies:**
   ```bash
   go mod download
   ```

2. **Start Redis (or PostgreSQL):**
   ```bash
   docker run -d -p 6379:6379 redis:7-alpine
   ```

3. **Run the server:**
   ```bash
   go run cmd/server/main.go -storage=redis -redis-addr=localhost:6379
   ```

4. **Run workers (in separate terminals):**
   ```bash
   go run cmd/worker/main.go -storage=redis -redis-addr=localhost:6379 -concurrency=4
   ```

## Usage Examples

### Creating Jobs

**Simple Echo Job:**
```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "echo",
    "queue": "default",
    "payload": {"message": "Hello!"}
  }'
```

**Math Calculation:**
```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "math",
    "queue": "default",
    "payload": {
      "operation": "add",
      "a": 10,
      "b": 5
    }
  }'
```

**Email Job:**
```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "email",
    "queue": "default",
    "payload": {
      "to": "user@example.com",
      "subject": "Test Email",
      "body": "This is a test email"
    }
  }'
```

**Scheduled Job (runs in 1 hour):**
```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "report",
    "queue": "default",
    "payload": {"type": "daily"},
    "scheduled_at": "'$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)'"
  }'
```

### Managing Queues

**Create a Queue:**
```bash
curl -X POST http://localhost:8080/api/v1/queues \
  -H "Content-Type: application/json" \
  -d '{
    "name": "high_priority",
    "max_workers": 20,
    "max_retries": 5,
    "retry_strategy": "exponential",
    "priority": 2,
    "weight": 10
  }'
```

**Pause a Queue:**
```bash
curl -X POST http://localhost:8080/api/v1/queues/default/pause
```

**Resume a Queue:**
```bash
curl -X POST http://localhost:8080/api/v1/queues/default/resume
```

**Get Queue Stats:**
```bash
curl http://localhost:8080/api/v1/queues/default/stats
```

### Recurring Jobs (Schedules)

**Create a Recurring Schedule:**
```bash
curl -X POST http://localhost:8080/api/v1/schedules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "daily-report",
    "job_type": "report",
    "queue": "default",
    "cron_expr": "@daily",
    "payload": {"type": "daily"}
  }'
```

**List Schedules:**
```bash
curl http://localhost:8080/api/v1/schedules
```

## Configuration

### Server Configuration

Command-line flags:
- `-storage`: Storage backend (memory, redis, postgres) - default: memory
- `-http-addr`: HTTP server address - default: 0.0.0.0
- `-http-port`: HTTP server port - default: 8080
- `-redis-addr`: Redis address - default: localhost:6379
- `-postgres-host`: PostgreSQL host - default: localhost
- `-postgres-port`: PostgreSQL port - default: 5432
- `-postgres-db`: PostgreSQL database - default: taskqueue
- `-postgres-user`: PostgreSQL user - default: postgres
- `-postgres-pass`: PostgreSQL password

### Worker Configuration

Command-line flags:
- `-storage`: Storage backend (same as server)
- `-worker-id`: Unique worker ID (auto-generated if not provided)
- `-queues`: Queues to process - default: default
- `-concurrency`: Number of concurrent workers - default: CPU count
- Redis/PostgreSQL flags (same as server)

## API Reference

### REST API Endpoints

#### Jobs
- `POST /api/v1/jobs` - Create a new job
- `GET /api/v1/jobs` - List jobs (with filtering)
- `GET /api/v1/jobs/{id}` - Get job details
- `POST /api/v1/jobs/{id}/cancel` - Cancel a job
- `DELETE /api/v1/jobs/{id}` - Delete a job

#### Queues
- `POST /api/v1/queues` - Create a queue
- `GET /api/v1/queues` - List queues
- `GET /api/v1/queues/{name}` - Get queue config
- `PUT /api/v1/queues/{name}` - Update queue config
- `DELETE /api/v1/queues/{name}` - Delete queue
- `POST /api/v1/queues/{name}/pause` - Pause queue
- `POST /api/v1/queues/{name}/resume` - Resume queue
- `GET /api/v1/queues/{name}/stats` - Get queue statistics

#### Schedules
- `POST /api/v1/schedules` - Create a schedule
- `GET /api/v1/schedules` - List schedules
- `GET /api/v1/schedules/{id}` - Get schedule details
- `DELETE /api/v1/schedules/{id}` - Delete schedule

#### Monitoring
- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /api/v1/metrics` - System metrics
- `GET /metrics` - Prometheus metrics (port 9090)

## Storage Backends

### Redis (Recommended for Production)

**Pros:**
- High performance
- Low latency
- Production-tested
- Atomic operations

**Cons:**
- In-memory (requires sufficient RAM)
- Persistence is optional

**Configuration:**
```bash
-storage=redis -redis-addr=localhost:6379
```

### PostgreSQL

**Pros:**
- ACID compliance
- Rich querying capabilities
- Persistent by default
- Familiar to most developers

**Cons:**
- Higher latency than Redis
- More resource-intensive

**Configuration:**
```bash
-storage=postgres \
-postgres-host=localhost \
-postgres-port=5432 \
-postgres-db=taskqueue \
-postgres-user=postgres \
-postgres-pass=password
```

### In-Memory

**Pros:**
- Zero dependencies
- Fastest performance
- Great for testing

**Cons:**
- No persistence
- Single-process only
- Not for production

**Configuration:**
```bash
-storage=memory
```

## Job Handlers

Register custom job handlers in `cmd/worker/main.go`:

```go
pool.RegisterHandler("my-job-type", func(ctx context.Context, j *job.Job) error {
    // Extract payload
    input := j.Payload["input"].(string)

    // Do work
    result := processData(input)

    // Set result
    resultBytes, _ := json.Marshal(map[string]interface{}{
        "output": result,
    })
    j.Result = resultBytes

    return nil
})
```

## Monitoring & Metrics

### Prometheus Metrics

Key metrics exported:
- `taskqueue_jobs_created_total` - Total jobs created by queue and type
- `taskqueue_jobs_completed_total` - Total jobs completed
- `taskqueue_jobs_failed_total` - Total jobs failed
- `taskqueue_jobs_retried_total` - Total job retries
- `taskqueue_job_duration_seconds` - Job execution time histogram
- `taskqueue_queue_length` - Current queue size
- `taskqueue_queue_paused` - Queue pause status
- `taskqueue_workers_active` - Active worker count
- `taskqueue_worker_jobs_total` - Per-worker job counters
- `taskqueue_system_uptime_seconds` - System uptime

### Grafana Dashboards

Access Grafana at http://localhost:3000 (admin/admin) for:
- Real-time job statistics
- Queue performance metrics
- Worker health and throughput
- System resource utilization

## Deployment

See [deployments/README.md](./deployments/README.md) for detailed deployment instructions including:
- Docker setup
- Docker Compose for local development
- Kubernetes for production
- Scaling strategies
- Production checklist

## Retry Strategies

GoTask supports three retry strategies:

1. **Fixed Delay**: Constant retry interval
   ```json
   {
     "retry_strategy": "fixed",
     "retry_delay": 60
   }
   ```

2. **Linear Backoff**: Retry delay increases linearly
   ```json
   {
     "retry_strategy": "linear",
     "retry_delay": 60
   }
   ```

3. **Exponential Backoff**: Retry delay doubles each time (recommended)
   ```json
   {
     "retry_strategy": "exponential",
     "retry_delay": 60,
     "max_retry_delay": 3600
   }
   ```

## Dead Letter Queue (DLQ)

Jobs that exceed max retries can be sent to a DLQ:

```json
{
  "name": "critical",
  "max_retries": 5,
  "dead_letter_queue": "failed-jobs"
}
```

## Priority Levels

Jobs and queues support three priority levels:
- **High (2)**: Processed first
- **Normal (1)**: Default priority
- **Low (0)**: Processed when no higher priority jobs exist

## Performance Tuning

### Worker Concurrency
- Start with `concurrency = CPU cores`
- Increase for I/O-bound jobs
- Decrease for CPU-intensive jobs

### Queue Configuration
- Use multiple queues for different job types
- Set appropriate priorities
- Configure weights for load balancing

### Storage Backend
- Use Redis for best performance
- Use connection pooling
- Monitor storage metrics

### Scaling
- Scale workers horizontally
- Use auto-scaling in Kubernetes
- Monitor queue lengths

## Troubleshooting

### Jobs not processing
1. Check worker logs
2. Verify queue names match
3. Ensure workers are registered
4. Check storage backend connectivity

### High latency
1. Scale workers horizontally
2. Check storage backend performance
3. Review job execution times
4. Optimize job handlers

### Memory issues
1. Reduce worker concurrency
2. Check for memory leaks in handlers
3. Monitor job payload sizes
4. Scale horizontally instead of vertically

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License

## Acknowledgments

Built with:
- Go 1.21+
- Redis
- PostgreSQL
- Prometheus
- Grafana
