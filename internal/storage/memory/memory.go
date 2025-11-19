package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Distributed-Task-Queue-System/pkg/errors"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// MemoryStore implements all storage interfaces using in-memory data structures
// Useful for testing and development
type MemoryStore struct {
	mu sync.RWMutex

	// Job storage
	jobs map[string]*job.Job

	// Queue storage - map of queue name to list of job IDs with priority
	queues map[string][]*queueEntry
	paused map[string]bool

	// Worker storage
	workers map[string]*job.Worker

	// Metrics storage
	metrics *metricsData

	// Queue configurations
	queueConfigs map[string]*job.QueueConfig
}

type queueEntry struct {
	JobID     string
	Priority  job.Priority
	Timestamp time.Time
}

type metricsData struct {
	mu               sync.RWMutex
	jobsCreated      int64
	jobsCompleted    int64
	jobsFailed       int64
	jobsRetried      int64
	queueStats       map[string]*job.QueueStats
	jobTypeStats     map[string]*jobTypeMetrics
	lastUpdate       time.Time
}

type jobTypeMetrics struct {
	Count          int64
	TotalDuration  time.Duration
	MinDuration    time.Duration
	MaxDuration    time.Duration
}

// NewMemoryStore creates a new in-memory storage backend
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		jobs:         make(map[string]*job.Job),
		queues:       make(map[string][]*queueEntry),
		paused:       make(map[string]bool),
		workers:      make(map[string]*job.Worker),
		queueConfigs: make(map[string]*job.QueueConfig),
		metrics: &metricsData{
			queueStats:   make(map[string]*job.QueueStats),
			jobTypeStats: make(map[string]*jobTypeMetrics),
			lastUpdate:   time.Now(),
		},
	}
}

// ===== JobStore Implementation =====

func (m *MemoryStore) CreateJob(ctx context.Context, j *job.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.jobs[j.ID]; exists {
		return errors.ErrJobAlreadyExists
	}

	// Create a copy to avoid external modifications
	jobCopy := *j
	m.jobs[j.ID] = &jobCopy

	return nil
}

func (m *MemoryStore) GetJob(ctx context.Context, id string) (*job.Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	j, exists := m.jobs[id]
	if !exists {
		return nil, errors.ErrJobNotFound
	}

	// Return a copy
	jobCopy := *j
	return &jobCopy, nil
}

func (m *MemoryStore) UpdateJob(ctx context.Context, j *job.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.jobs[j.ID]; !exists {
		return errors.ErrJobNotFound
	}

	jobCopy := *j
	m.jobs[j.ID] = &jobCopy

	return nil
}

func (m *MemoryStore) DeleteJob(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.jobs[id]; !exists {
		return errors.ErrJobNotFound
	}

	delete(m.jobs, id)
	return nil
}

func (m *MemoryStore) ListJobs(ctx context.Context, queue string, status job.Status, limit, offset int) ([]*job.Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var jobs []*job.Job
	for _, j := range m.jobs {
		if (queue == "" || j.Queue == queue) && (status == "" || j.Status == status) {
			jobCopy := *j
			jobs = append(jobs, &jobCopy)
		}
	}

	// Sort by created time
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})

	// Apply pagination
	if offset >= len(jobs) {
		return []*job.Job{}, nil
	}
	jobs = jobs[offset:]
	if limit > 0 && limit < len(jobs) {
		jobs = jobs[:limit]
	}

	return jobs, nil
}

func (m *MemoryStore) UpdateStatus(ctx context.Context, id string, status job.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, exists := m.jobs[id]
	if !exists {
		return errors.ErrJobNotFound
	}

	j.Status = status
	j.UpdatedAt = time.Now()

	if status == job.StatusRunning && j.StartedAt == nil {
		now := time.Now()
		j.StartedAt = &now
	}

	if j.IsTerminal() && j.CompletedAt == nil {
		now := time.Now()
		j.CompletedAt = &now
	}

	return nil
}

func (m *MemoryStore) UpdateProgress(ctx context.Context, id string, progress int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, exists := m.jobs[id]
	if !exists {
		return errors.ErrJobNotFound
	}

	j.Progress = progress
	j.UpdatedAt = time.Now()

	return nil
}

func (m *MemoryStore) SetResult(ctx context.Context, id string, result []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, exists := m.jobs[id]
	if !exists {
		return errors.ErrJobNotFound
	}

	j.Result = result
	j.UpdatedAt = time.Now()

	return nil
}

func (m *MemoryStore) SetError(ctx context.Context, id string, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, exists := m.jobs[id]
	if !exists {
		return errors.ErrJobNotFound
	}

	j.Error = errMsg
	j.UpdatedAt = time.Now()

	return nil
}

// ===== QueueStore Implementation =====

func (m *MemoryStore) Enqueue(ctx context.Context, queue string, jobID string, priority job.Priority, delay time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify job exists
	if _, exists := m.jobs[jobID]; !exists {
		return errors.ErrJobNotFound
	}

	entry := &queueEntry{
		JobID:     jobID,
		Priority:  priority,
		Timestamp: time.Now().Add(delay),
	}

	m.queues[queue] = append(m.queues[queue], entry)

	return nil
}

func (m *MemoryStore) Dequeue(ctx context.Context, queue string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if queue is paused
	if m.paused[queue] {
		return "", errors.ErrQueuePaused
	}

	entries := m.queues[queue]
	if len(entries) == 0 {
		return "", errors.ErrQueueEmpty
	}

	now := time.Now()
	var bestIdx = -1
	var bestPriority job.Priority = -1

	// Find the highest priority job that's ready to run
	for i, entry := range entries {
		if entry.Timestamp.After(now) {
			continue // Not ready yet
		}
		if bestIdx == -1 || entry.Priority > bestPriority {
			bestIdx = i
			bestPriority = entry.Priority
		}
	}

	if bestIdx == -1 {
		return "", errors.ErrQueueEmpty
	}

	jobID := entries[bestIdx].JobID

	// Remove from queue
	m.queues[queue] = append(entries[:bestIdx], entries[bestIdx+1:]...)

	return jobID, nil
}

func (m *MemoryStore) Requeue(ctx context.Context, queue string, jobID string, priority job.Priority, delay time.Duration) error {
	// For in-memory, requeue is the same as enqueue
	return m.Enqueue(ctx, queue, jobID, priority, delay)
}

func (m *MemoryStore) Remove(ctx context.Context, queue string, jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries := m.queues[queue]
	for i, entry := range entries {
		if entry.JobID == jobID {
			m.queues[queue] = append(entries[:i], entries[i+1:]...)
			return nil
		}
	}

	return errors.ErrJobNotFound
}

func (m *MemoryStore) Length(ctx context.Context, queue string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return int64(len(m.queues[queue])), nil
}

func (m *MemoryStore) Peek(ctx context.Context, queue string, count int) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := m.queues[queue]
	if count > len(entries) {
		count = len(entries)
	}

	result := make([]string, 0, count)
	for i := 0; i < count; i++ {
		result = append(result, entries[i].JobID)
	}

	return result, nil
}

func (m *MemoryStore) GetScheduled(ctx context.Context, queue string, until time.Time) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string
	for _, entry := range m.queues[queue] {
		if !entry.Timestamp.After(until) {
			result = append(result, entry.JobID)
		}
	}

	return result, nil
}

func (m *MemoryStore) PauseQueue(ctx context.Context, queue string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.paused[queue] = true
	return nil
}

func (m *MemoryStore) ResumeQueue(ctx context.Context, queue string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.paused[queue] = false
	return nil
}

func (m *MemoryStore) IsQueuePaused(ctx context.Context, queue string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.paused[queue], nil
}

// ===== WorkerStore Implementation =====

func (m *MemoryStore) RegisterWorker(ctx context.Context, w *job.Worker) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	workerCopy := *w
	m.workers[w.ID] = &workerCopy

	return nil
}

func (m *MemoryStore) UnregisterWorker(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.workers[id]; !exists {
		return errors.ErrWorkerNotFound
	}

	delete(m.workers, id)
	return nil
}

func (m *MemoryStore) GetWorker(ctx context.Context, id string) (*job.Worker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	w, exists := m.workers[id]
	if !exists {
		return nil, errors.ErrWorkerNotFound
	}

	workerCopy := *w
	return &workerCopy, nil
}

func (m *MemoryStore) ListWorkers(ctx context.Context, queue string) ([]*job.Worker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var workers []*job.Worker
	for _, w := range m.workers {
		if queue == "" {
			workerCopy := *w
			workers = append(workers, &workerCopy)
		} else {
			for _, q := range w.Queues {
				if q == queue {
					workerCopy := *w
					workers = append(workers, &workerCopy)
					break
				}
			}
		}
	}

	return workers, nil
}

func (m *MemoryStore) UpdateHeartbeat(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.workers[id]
	if !exists {
		return errors.ErrWorkerNotFound
	}

	now := time.Now()
	w.LastHeartbeat = &now

	return nil
}

func (m *MemoryStore) UpdateWorkerStatus(ctx context.Context, id string, status job.WorkerStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.workers[id]
	if !exists {
		return errors.ErrWorkerNotFound
	}

	w.Status = status

	return nil
}

func (m *MemoryStore) SetCurrentJob(ctx context.Context, workerID string, jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.workers[workerID]
	if !exists {
		return errors.ErrWorkerNotFound
	}

	if jobID == "" {
		w.CurrentJobID = nil
	} else {
		w.CurrentJobID = &jobID
	}

	return nil
}

func (m *MemoryStore) IncrementProcessed(ctx context.Context, workerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.workers[workerID]
	if !exists {
		return errors.ErrWorkerNotFound
	}

	w.ProcessedCount++

	return nil
}

func (m *MemoryStore) IncrementFailed(ctx context.Context, workerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.workers[workerID]
	if !exists {
		return errors.ErrWorkerNotFound
	}

	w.FailedCount++

	return nil
}

func (m *MemoryStore) GetStaleWorkers(ctx context.Context, threshold time.Duration) ([]*job.Worker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cutoff := time.Now().Add(-threshold)
	var staleWorkers []*job.Worker

	for _, w := range m.workers {
		if w.LastHeartbeat != nil && w.LastHeartbeat.Before(cutoff) {
			workerCopy := *w
			staleWorkers = append(staleWorkers, &workerCopy)
		}
	}

	return staleWorkers, nil
}

// ===== MetricsStore Implementation =====

func (m *MemoryStore) RecordJobCreated(ctx context.Context, queue string, jobType string) error {
	m.metrics.mu.Lock()
	defer m.metrics.mu.Unlock()

	m.metrics.jobsCreated++

	// Update queue stats
	if m.metrics.queueStats[queue] == nil {
		m.metrics.queueStats[queue] = &job.QueueStats{
			QueueName: queue,
		}
	}
	// Increment pending (will be updated on dequeue)

	return nil
}

func (m *MemoryStore) RecordJobCompleted(ctx context.Context, queue string, jobType string, duration time.Duration) error {
	m.metrics.mu.Lock()
	defer m.metrics.mu.Unlock()

	m.metrics.jobsCompleted++

	// Update queue stats
	if m.metrics.queueStats[queue] != nil {
		m.metrics.queueStats[queue].SuccessCount++
	}

	// Update job type stats
	if m.metrics.jobTypeStats[jobType] == nil {
		m.metrics.jobTypeStats[jobType] = &jobTypeMetrics{
			MinDuration: duration,
			MaxDuration: duration,
		}
	}
	stats := m.metrics.jobTypeStats[jobType]
	stats.Count++
	stats.TotalDuration += duration
	if duration < stats.MinDuration {
		stats.MinDuration = duration
	}
	if duration > stats.MaxDuration {
		stats.MaxDuration = duration
	}

	return nil
}

func (m *MemoryStore) RecordJobFailed(ctx context.Context, queue string, jobType string) error {
	m.metrics.mu.Lock()
	defer m.metrics.mu.Unlock()

	m.metrics.jobsFailed++

	if m.metrics.queueStats[queue] != nil {
		m.metrics.queueStats[queue].FailedCount++
	}

	return nil
}

func (m *MemoryStore) RecordJobRetried(ctx context.Context, queue string, jobType string) error {
	m.metrics.mu.Lock()
	defer m.metrics.mu.Unlock()

	m.metrics.jobsRetried++

	return nil
}

func (m *MemoryStore) GetQueueStats(ctx context.Context, queue string) (*job.QueueStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	stats := &job.QueueStats{
		QueueName: queue,
	}

	// Count jobs by status
	for _, j := range m.jobs {
		if j.Queue != queue {
			continue
		}
		switch j.Status {
		case job.StatusPending:
			stats.PendingCount++
		case job.StatusRunning:
			stats.RunningCount++
		case job.StatusSuccess:
			stats.SuccessCount++
		case job.StatusFailed:
			stats.FailedCount++
		}
	}

	// Add stored stats
	if m.metrics.queueStats[queue] != nil {
		if stats.SuccessCount == 0 {
			stats.SuccessCount = m.metrics.queueStats[queue].SuccessCount
		}
		if stats.FailedCount == 0 {
			stats.FailedCount = m.metrics.queueStats[queue].FailedCount
		}
	}

	return stats, nil
}

func (m *MemoryStore) GetQueueStatsAll(ctx context.Context) (map[string]*job.QueueStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get unique queue names
	queues := make(map[string]bool)
	for _, j := range m.jobs {
		queues[j.Queue] = true
	}

	result := make(map[string]*job.QueueStats)
	for queue := range queues {
		stats, _ := m.GetQueueStats(ctx, queue)
		result[queue] = stats
	}

	return result, nil
}

func (m *MemoryStore) GetJobTypeStats(ctx context.Context, jobType string) (count int64, avgDuration time.Duration, err error) {
	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	stats, exists := m.metrics.jobTypeStats[jobType]
	if !exists {
		return 0, 0, nil
	}

	count = stats.Count
	if count > 0 {
		avgDuration = stats.TotalDuration / time.Duration(count)
	}

	return count, avgDuration, nil
}

// ===== ConfigStore Implementation =====

func (m *MemoryStore) SaveQueueConfig(ctx context.Context, config *job.QueueConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	configCopy := *config
	m.queueConfigs[config.Name] = &configCopy

	return nil
}

func (m *MemoryStore) GetQueueConfig(ctx context.Context, queue string) (*job.QueueConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config, exists := m.queueConfigs[queue]
	if !exists {
		return nil, fmt.Errorf("queue config not found: %s", queue)
	}

	configCopy := *config
	return &configCopy, nil
}

func (m *MemoryStore) DeleteQueueConfig(ctx context.Context, queue string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.queueConfigs, queue)
	return nil
}

func (m *MemoryStore) ListQueueConfigs(ctx context.Context) ([]*job.QueueConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	configs := make([]*job.QueueConfig, 0, len(m.queueConfigs))
	for _, config := range m.queueConfigs {
		configCopy := *config
		configs = append(configs, &configCopy)
	}

	return configs, nil
}

// Close implements the storage interface (no-op for memory)
func (m *MemoryStore) Close() error {
	return nil
}
