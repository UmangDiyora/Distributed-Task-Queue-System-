# GoTask Deployment Guide

This directory contains deployment configurations for GoTask Distributed Task Queue System.

## Quick Start

### Docker Compose (Recommended for Development)

1. **Start all services:**
   ```bash
   docker-compose up -d
   ```

2. **Access the services:**
   - API Server: http://localhost:8080
   - Health Check: http://localhost:8080/health
   - Metrics: http://localhost:9090/metrics
   - Grafana: http://localhost:3000 (admin/admin)
   - Prometheus: http://localhost:9091

3. **Scale workers:**
   ```bash
   docker-compose up -d --scale worker=5
   ```

4. **View logs:**
   ```bash
   docker-compose logs -f server
   docker-compose logs -f worker
   ```

5. **Stop all services:**
   ```bash
   docker-compose down
   ```

### Docker

1. **Build the image:**
   ```bash
   docker build -t taskqueue:latest .
   ```

2. **Run server:**
   ```bash
   docker run -d \
     --name taskqueue-server \
     -p 8080:8080 \
     -p 9090:9090 \
     taskqueue:latest
   ```

3. **Run worker:**
   ```bash
   docker run -d \
     --name taskqueue-worker \
     taskqueue:latest \
     -storage=redis \
     -redis-addr=redis:6379
   ```

### Kubernetes

1. **Apply manifests:**
   ```bash
   kubectl apply -f deployments/kubernetes/deployment.yaml
   ```

2. **Check status:**
   ```bash
   kubectl get pods -n taskqueue
   kubectl get services -n taskqueue
   ```

3. **Scale workers:**
   ```bash
   kubectl scale deployment taskqueue-worker --replicas=5 -n taskqueue
   ```

4. **View logs:**
   ```bash
   kubectl logs -f deployment/taskqueue-server -n taskqueue
   kubectl logs -f deployment/taskqueue-worker -n taskqueue
   ```

5. **Access the API:**
   ```bash
   kubectl port-forward service/taskqueue-server 8080:80 -n taskqueue
   ```

## Configuration

### Environment Variables

#### Server
- `STORAGE_BACKEND`: Storage backend (memory, redis, postgres)
- `REDIS_ADDR`: Redis address (default: localhost:6379)
- `POSTGRES_HOST`: PostgreSQL host (default: localhost)
- `POSTGRES_PORT`: PostgreSQL port (default: 5432)
- `POSTGRES_DB`: PostgreSQL database name
- `POSTGRES_USER`: PostgreSQL username
- `POSTGRES_PASSWORD`: PostgreSQL password

#### Worker
- Same as server, plus:
- `WORKER_ID`: Unique worker ID
- `QUEUES`: Comma-separated list of queues to process
- `CONCURRENCY`: Number of concurrent jobs per worker

### Storage Backends

#### In-Memory (Development Only)
```bash
-storage=memory
```

#### Redis (Recommended for Production)
```bash
-storage=redis \
-redis-addr=localhost:6379
```

#### PostgreSQL
```bash
-storage=postgres \
-postgres-host=localhost \
-postgres-port=5432 \
-postgres-db=taskqueue \
-postgres-user=taskqueue \
-postgres-pass=password
```

## Monitoring

### Prometheus Metrics

Metrics are exposed at `/metrics` endpoint on port 9090.

Key metrics:
- `taskqueue_jobs_created_total`: Total jobs created
- `taskqueue_jobs_completed_total`: Total jobs completed
- `taskqueue_jobs_failed_total`: Total jobs failed
- `taskqueue_job_duration_seconds`: Job execution time
- `taskqueue_queue_length`: Current queue length
- `taskqueue_workers_active`: Active worker count

### Grafana Dashboards

Access Grafana at http://localhost:3000 (admin/admin)

Pre-configured dashboards:
- Job Statistics
- Queue Monitoring
- Worker Performance
- System Health

## Scaling

### Horizontal Scaling

**Workers:**
- Docker Compose: `docker-compose up -d --scale worker=N`
- Kubernetes: `kubectl scale deployment taskqueue-worker --replicas=N -n taskqueue`

**Servers:**
- Docker Compose: `docker-compose up -d --scale server=N`
- Kubernetes: `kubectl scale deployment taskqueue-server --replicas=N -n taskqueue`

### Auto-Scaling (Kubernetes)

The HPA (Horizontal Pod Autoscaler) is configured to scale workers based on:
- CPU utilization (target: 70%)
- Memory utilization (target: 80%)
- Min replicas: 2
- Max replicas: 10

## Production Checklist

- [ ] Configure persistent storage for Redis/PostgreSQL
- [ ] Set up SSL/TLS for API endpoints
- [ ] Configure authentication and authorization
- [ ] Set up monitoring alerts
- [ ] Configure log aggregation
- [ ] Set resource limits and requests
- [ ] Configure backup strategy
- [ ] Set up health checks and readiness probes
- [ ] Configure network policies
- [ ] Set up disaster recovery plan

## Troubleshooting

### Server won't start
- Check storage backend connectivity
- Verify configuration values
- Check logs: `docker-compose logs server`

### Workers not processing jobs
- Verify queue names match
- Check worker logs
- Ensure storage backend is accessible
- Verify jobs are being created

### High latency
- Scale up workers
- Check storage backend performance
- Review job execution times
- Check network connectivity

### Memory issues
- Adjust worker concurrency
- Scale horizontally instead of vertically
- Review job payload sizes
- Check for memory leaks in custom handlers

## Security

### Production Recommendations

1. **API Authentication:**
   - Implement JWT or API key authentication
   - Use TLS for all connections

2. **Storage Security:**
   - Enable Redis AUTH
   - Use PostgreSQL SSL connections
   - Rotate credentials regularly

3. **Network Security:**
   - Use Kubernetes Network Policies
   - Configure firewall rules
   - Use private networks

4. **Secrets Management:**
   - Use Kubernetes Secrets or external secret managers
   - Never commit credentials to version control
   - Use environment-specific configurations
