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

(Coming soon)

## Documentation

See the [docs](./docs) directory for detailed documentation.

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
