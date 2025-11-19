package monitoring

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// Job metrics
	JobsCreated   *prometheus.CounterVec
	JobsCompleted *prometheus.CounterVec
	JobsFailed    *prometheus.CounterVec
	JobsRetried   *prometheus.CounterVec
	JobDuration   *prometheus.HistogramVec

	// Queue metrics
	QueueLength   *prometheus.GaugeVec
	QueuePaused   *prometheus.GaugeVec

	// Worker metrics
	WorkersActive    prometheus.Gauge
	WorkerJobsTotal  *prometheus.CounterVec
	WorkerJobsCurrent *prometheus.GaugeVec

	// System metrics
	SystemUptime prometheus.Gauge
}

var (
	instance *Metrics
	once     sync.Once
)

// GetMetrics returns the singleton metrics instance
func GetMetrics() *Metrics {
	once.Do(func() {
		instance = &Metrics{
			// Job metrics
			JobsCreated: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "taskqueue_jobs_created_total",
					Help: "Total number of jobs created",
				},
				[]string{"queue", "type"},
			),
			JobsCompleted: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "taskqueue_jobs_completed_total",
					Help: "Total number of jobs completed successfully",
				},
				[]string{"queue", "type"},
			),
			JobsFailed: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "taskqueue_jobs_failed_total",
					Help: "Total number of jobs failed",
				},
				[]string{"queue", "type"},
			),
			JobsRetried: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "taskqueue_jobs_retried_total",
					Help: "Total number of job retries",
				},
				[]string{"queue", "type"},
			),
			JobDuration: promauto.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "taskqueue_job_duration_seconds",
					Help:    "Job execution duration in seconds",
					Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~102s
				},
				[]string{"queue", "type", "status"},
			),

			// Queue metrics
			QueueLength: promauto.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "taskqueue_queue_length",
					Help: "Current number of jobs in queue",
				},
				[]string{"queue"},
			),
			QueuePaused: promauto.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "taskqueue_queue_paused",
					Help: "Whether queue is paused (1) or not (0)",
				},
				[]string{"queue"},
			),

			// Worker metrics
			WorkersActive: promauto.NewGauge(
				prometheus.GaugeOpts{
					Name: "taskqueue_workers_active",
					Help: "Number of active workers",
				},
			),
			WorkerJobsTotal: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "taskqueue_worker_jobs_total",
					Help: "Total number of jobs processed by worker",
				},
				[]string{"worker_id", "status"},
			),
			WorkerJobsCurrent: promauto.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "taskqueue_worker_jobs_current",
					Help: "Current number of jobs being processed by worker",
				},
				[]string{"worker_id"},
			),

			// System metrics
			SystemUptime: promauto.NewGauge(
				prometheus.GaugeOpts{
					Name: "taskqueue_system_uptime_seconds",
					Help: "System uptime in seconds",
				},
			),
		}
	})

	return instance
}

// RecordJobCreated records a job creation event
func (m *Metrics) RecordJobCreated(queue, jobType string) {
	m.JobsCreated.WithLabelValues(queue, jobType).Inc()
}

// RecordJobCompleted records a job completion event
func (m *Metrics) RecordJobCompleted(queue, jobType string, duration time.Duration) {
	m.JobsCompleted.WithLabelValues(queue, jobType).Inc()
	m.JobDuration.WithLabelValues(queue, jobType, "success").Observe(duration.Seconds())
}

// RecordJobFailed records a job failure event
func (m *Metrics) RecordJobFailed(queue, jobType string, duration time.Duration) {
	m.JobsFailed.WithLabelValues(queue, jobType).Inc()
	m.JobDuration.WithLabelValues(queue, jobType, "failed").Observe(duration.Seconds())
}

// RecordJobRetried records a job retry event
func (m *Metrics) RecordJobRetried(queue, jobType string) {
	m.JobsRetried.WithLabelValues(queue, jobType).Inc()
}

// SetQueueLength sets the current queue length
func (m *Metrics) SetQueueLength(queue string, length int64) {
	m.QueueLength.WithLabelValues(queue).Set(float64(length))
}

// SetQueuePaused sets whether a queue is paused
func (m *Metrics) SetQueuePaused(queue string, paused bool) {
	value := 0.0
	if paused {
		value = 1.0
	}
	m.QueuePaused.WithLabelValues(queue).Set(value)
}

// SetWorkersActive sets the number of active workers
func (m *Metrics) SetWorkersActive(count int) {
	m.WorkersActive.Set(float64(count))
}

// RecordWorkerJob records a worker job processing event
func (m *Metrics) RecordWorkerJob(workerID, status string) {
	m.WorkerJobsTotal.WithLabelValues(workerID, status).Inc()
}

// SetWorkerJobsCurrent sets the current number of jobs for a worker
func (m *Metrics) SetWorkerJobsCurrent(workerID string, count int) {
	m.WorkerJobsCurrent.WithLabelValues(workerID).Set(float64(count))
}

// UpdateSystemUptime updates the system uptime
func (m *Metrics) UpdateSystemUptime(startTime time.Time) {
	m.SystemUptime.Set(time.Since(startTime).Seconds())
}

// MetricsServer provides a Prometheus metrics HTTP server
type MetricsServer struct {
	server *http.Server
}

// NewMetricsServer creates a new metrics server
func NewMetricsServer(addr string, port int) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	return &MetricsServer{
		server: &http.Server{
			Addr:    addr + ":" + string(rune(port)),
			Handler: mux,
		},
	}
}

// Start starts the metrics server
func (s *MetricsServer) Start() error {
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop stops the metrics server
func (s *MetricsServer) Stop() error {
	return s.server.Close()
}
