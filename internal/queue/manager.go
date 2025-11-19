package queue

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/storage"
	"github.com/Distributed-Task-Queue-System/pkg/errors"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// Manager manages job queues and routing
type Manager struct {
	storage storage.Storage
	configs map[string]*job.QueueConfig
	mu      sync.RWMutex
}

// Storage interface combines all storage interfaces
type Storage interface {
	storage.JobStore
	storage.QueueStore
	storage.WorkerStore
	storage.MetricsStore
	storage.ConfigStore
}

// NewManager creates a new queue manager
func NewManager(store Storage) (*Manager, error) {
	m := &Manager{
		storage: store,
		configs: make(map[string]*job.QueueConfig),
	}

	// Load existing queue configurations
	if err := m.loadConfigs(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load queue configs: %w", err)
	}

	return m, nil
}

// loadConfigs loads queue configurations from storage
func (m *Manager) loadConfigs(ctx context.Context) error {
	configs, err := m.storage.ListQueueConfigs(ctx)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, config := range configs {
		m.configs[config.Name] = config
	}

	return nil
}

// ===== Queue Configuration =====

// RegisterQueue registers a new queue configuration
func (m *Manager) RegisterQueue(ctx context.Context, config *job.QueueConfig) error {
	if config.Name == "" {
		return errors.NewValidationError("queue name is required", nil)
	}

	// Set defaults if not provided
	if config.MaxWorkers == 0 {
		config.MaxWorkers = 10
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryStrategy == "" {
		config.RetryStrategy = job.RetryStrategyExponential
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 60 // 60 seconds
	}
	if config.MaxRetryDelay == 0 {
		config.MaxRetryDelay = 3600 // 1 hour
	}
	if config.Timeout == 0 {
		config.Timeout = 300 // 5 minutes
	}
	if config.Weight == 0 {
		config.Weight = 1
	}

	// Save to storage
	if err := m.storage.SaveQueueConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to save queue config: %w", err)
	}

	m.mu.Lock()
	m.configs[config.Name] = config
	m.mu.Unlock()

	return nil
}

// GetQueueConfig retrieves a queue configuration
func (m *Manager) GetQueueConfig(ctx context.Context, name string) (*job.QueueConfig, error) {
	m.mu.RLock()
	config, exists := m.configs[name]
	m.mu.RUnlock()

	if !exists {
		// Try to load from storage
		config, err := m.storage.GetQueueConfig(ctx, name)
		if err != nil {
			return nil, errors.NewQueueError(fmt.Sprintf("queue config not found: %s", name), nil)
		}

		m.mu.Lock()
		m.configs[name] = config
		m.mu.Unlock()

		return config, nil
	}

	return config, nil
}

// UpdateQueueConfig updates a queue configuration
func (m *Manager) UpdateQueueConfig(ctx context.Context, config *job.QueueConfig) error {
	if config.Name == "" {
		return errors.NewValidationError("queue name is required", nil)
	}

	m.mu.RLock()
	_, exists := m.configs[config.Name]
	m.mu.RUnlock()

	if !exists {
		return errors.NewQueueError(fmt.Sprintf("queue not found: %s", config.Name), nil)
	}

	if err := m.storage.SaveQueueConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to update queue config: %w", err)
	}

	m.mu.Lock()
	m.configs[config.Name] = config
	m.mu.Unlock()

	return nil
}

// DeleteQueue deletes a queue configuration
func (m *Manager) DeleteQueue(ctx context.Context, name string) error {
	// Check if queue is empty
	length, err := m.storage.Length(ctx, name)
	if err != nil {
		return err
	}

	if length > 0 {
		return errors.NewQueueError(fmt.Sprintf("cannot delete non-empty queue: %s", name), nil)
	}

	if err := m.storage.DeleteQueueConfig(ctx, name); err != nil {
		return fmt.Errorf("failed to delete queue config: %w", err)
	}

	m.mu.Lock()
	delete(m.configs, name)
	m.mu.Unlock()

	return nil
}

// ListQueues returns all queue configurations
func (m *Manager) ListQueues(ctx context.Context) ([]*job.QueueConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	queues := make([]*job.QueueConfig, 0, len(m.configs))
	for _, config := range m.configs {
		queues = append(queues, config)
	}

	return queues, nil
}

// ===== Job Enqueue/Dequeue =====

// Enqueue adds a job to a queue
func (m *Manager) Enqueue(ctx context.Context, j *job.Job) error {
	if j.Queue == "" {
		j.Queue = "default"
	}

	// Get queue config
	config, err := m.GetQueueConfig(ctx, j.Queue)
	if err != nil {
		// If queue doesn't exist, create a default one
		defaultConfig := &job.QueueConfig{
			Name:           j.Queue,
			MaxWorkers:     10,
			MaxRetries:     3,
			RetryStrategy:  job.RetryStrategyExponential,
			RetryDelay:     60,
			MaxRetryDelay:  3600,
			Timeout:        300,
			Priority:       job.PriorityNormal,
			Weight:         1,
		}
		if err := m.RegisterQueue(ctx, defaultConfig); err != nil {
			return err
		}
		config = defaultConfig
	}

	// Set job defaults from queue config if not specified
	if j.MaxRetries == 0 {
		j.MaxRetries = config.MaxRetries
	}
	if j.TimeoutSeconds == 0 {
		j.TimeoutSeconds = config.Timeout
	}

	// Create job in storage
	if err := m.storage.CreateJob(ctx, j); err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	// Calculate delay
	var delay time.Duration
	if j.ScheduledAt.After(time.Now()) {
		delay = time.Until(j.ScheduledAt)
	}

	// Enqueue to the queue
	if err := m.storage.Enqueue(ctx, j.Queue, j.ID, j.Priority, delay); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	// Record metrics
	if err := m.storage.RecordJobCreated(ctx, j.Queue, j.Type); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("warning: failed to record job created metric: %v\n", err)
	}

	return nil
}

// Dequeue retrieves the next job from queues, considering priority and weights
func (m *Manager) Dequeue(ctx context.Context, queues []string) (*job.Job, error) {
	if len(queues) == 0 {
		return nil, errors.NewQueueError("no queues specified", nil)
	}

	// Select a queue based on weights and priorities
	selectedQueue, err := m.selectQueue(ctx, queues)
	if err != nil {
		return nil, err
	}

	if selectedQueue == "" {
		return nil, errors.ErrQueueEmpty
	}

	// Dequeue from selected queue
	jobID, err := m.storage.Dequeue(ctx, selectedQueue)
	if err != nil {
		if err == errors.ErrQueueEmpty {
			// Try other queues
			for _, qName := range queues {
				if qName == selectedQueue {
					continue
				}
				jobID, err = m.storage.Dequeue(ctx, qName)
				if err == nil {
					break
				}
			}
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// Get job details
	j, err := m.storage.GetJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	return j, nil
}

// selectQueue selects a queue based on priority and weight
func (m *Manager) selectQueue(ctx context.Context, queues []string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type queueCandidate struct {
		name     string
		priority job.Priority
		weight   int
		length   int64
	}

	var candidates []queueCandidate

	// Gather candidate queues that have jobs
	for _, qName := range queues {
		config, exists := m.configs[qName]
		if !exists {
			continue
		}

		// Check if queue is paused
		paused, err := m.storage.IsQueuePaused(ctx, qName)
		if err != nil || paused {
			continue
		}

		// Check queue length
		length, err := m.storage.Length(ctx, qName)
		if err != nil || length == 0 {
			continue
		}

		candidates = append(candidates, queueCandidate{
			name:     qName,
			priority: config.Priority,
			weight:   config.Weight,
			length:   length,
		})
	}

	if len(candidates) == 0 {
		return "", nil
	}

	// Sort by priority (highest first)
	maxPriority := job.PriorityLow
	for _, c := range candidates {
		if c.priority > maxPriority {
			maxPriority = c.priority
		}
	}

	// Filter to highest priority queues
	highPriorityQueues := []queueCandidate{}
	for _, c := range candidates {
		if c.priority == maxPriority {
			highPriorityQueues = append(highPriorityQueues, c)
		}
	}

	// If only one queue at highest priority, use it
	if len(highPriorityQueues) == 1 {
		return highPriorityQueues[0].name, nil
	}

	// Use weighted random selection among highest priority queues
	totalWeight := 0
	for _, c := range highPriorityQueues {
		totalWeight += c.weight
	}

	r := rand.Intn(totalWeight)
	cumWeight := 0
	for _, c := range highPriorityQueues {
		cumWeight += c.weight
		if r < cumWeight {
			return c.name, nil
		}
	}

	// Fallback to first queue (shouldn't reach here)
	return highPriorityQueues[0].name, nil
}

// Requeue puts a job back into the queue
func (m *Manager) Requeue(ctx context.Context, j *job.Job, delay time.Duration) error {
	if err := m.storage.UpdateJob(ctx, j); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	if err := m.storage.Requeue(ctx, j.Queue, j.ID, j.Priority, delay); err != nil {
		return fmt.Errorf("failed to requeue job: %w", err)
	}

	return nil
}

// RemoveJob removes a job from the queue
func (m *Manager) RemoveJob(ctx context.Context, queue string, jobID string) error {
	if err := m.storage.Remove(ctx, queue, jobID); err != nil {
		return fmt.Errorf("failed to remove job: %w", err)
	}

	return nil
}

// ===== Queue Operations =====

// PauseQueue pauses a queue
func (m *Manager) PauseQueue(ctx context.Context, name string) error {
	if err := m.storage.PauseQueue(ctx, name); err != nil {
		return fmt.Errorf("failed to pause queue: %w", err)
	}

	return nil
}

// ResumeQueue resumes a paused queue
func (m *Manager) ResumeQueue(ctx context.Context, name string) error {
	if err := m.storage.ResumeQueue(ctx, name); err != nil {
		return fmt.Errorf("failed to resume queue: %w", err)
	}

	return nil
}

// IsQueuePaused checks if a queue is paused
func (m *Manager) IsQueuePaused(ctx context.Context, name string) (bool, error) {
	return m.storage.IsQueuePaused(ctx, name)
}

// GetQueueLength returns the number of jobs in a queue
func (m *Manager) GetQueueLength(ctx context.Context, name string) (int64, error) {
	return m.storage.Length(ctx, name)
}

// PeekQueue returns the next N jobs without removing them
func (m *Manager) PeekQueue(ctx context.Context, name string, count int) ([]*job.Job, error) {
	jobIDs, err := m.storage.Peek(ctx, name, count)
	if err != nil {
		return nil, err
	}

	jobs := make([]*job.Job, 0, len(jobIDs))
	for _, id := range jobIDs {
		j, err := m.storage.GetJob(ctx, id)
		if err != nil {
			continue
		}
		jobs = append(jobs, j)
	}

	return jobs, nil
}

// GetQueueStats returns statistics for a queue
func (m *Manager) GetQueueStats(ctx context.Context, name string) (*job.QueueStats, error) {
	return m.storage.GetQueueStats(ctx, name)
}

// GetAllQueueStats returns statistics for all queues
func (m *Manager) GetAllQueueStats(ctx context.Context) (map[string]*job.QueueStats, error) {
	return m.storage.GetQueueStatsAll(ctx)
}

// ===== Job Operations =====

// GetJob retrieves a job by ID
func (m *Manager) GetJob(ctx context.Context, id string) (*job.Job, error) {
	return m.storage.GetJob(ctx, id)
}

// UpdateJob updates a job
func (m *Manager) UpdateJob(ctx context.Context, j *job.Job) error {
	return m.storage.UpdateJob(ctx, j)
}

// DeleteJob deletes a job
func (m *Manager) DeleteJob(ctx context.Context, id string) error {
	return m.storage.DeleteJob(ctx, id)
}

// ListJobs lists jobs with filtering
func (m *Manager) ListJobs(ctx context.Context, queue string, status job.Status, limit, offset int) ([]*job.Job, error) {
	return m.storage.ListJobs(ctx, queue, status, limit, offset)
}

// UpdateJobStatus updates a job's status
func (m *Manager) UpdateJobStatus(ctx context.Context, id string, status job.Status) error {
	return m.storage.UpdateStatus(ctx, id, status)
}

// UpdateJobProgress updates a job's progress
func (m *Manager) UpdateJobProgress(ctx context.Context, id string, progress int) error {
	return m.storage.UpdateProgress(ctx, id, progress)
}

// SetJobResult sets a job's result
func (m *Manager) SetJobResult(ctx context.Context, id string, result []byte) error {
	return m.storage.SetResult(ctx, id, result)
}

// SetJobError sets a job's error
func (m *Manager) SetJobError(ctx context.Context, id string, errMsg string) error {
	return m.storage.SetError(ctx, id, errMsg)
}

// CancelJob cancels a running job
func (m *Manager) CancelJob(ctx context.Context, id string) error {
	j, err := m.storage.GetJob(ctx, id)
	if err != nil {
		return err
	}

	if j.IsTerminal() {
		return errors.NewJobError("job is already in terminal state", nil)
	}

	// Update job status
	if err := m.storage.UpdateStatus(ctx, id, job.StatusCancelled); err != nil {
		return err
	}

	// Remove from queue if pending
	if j.Status == job.StatusPending {
		if err := m.storage.Remove(ctx, j.Queue, id); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("warning: failed to remove cancelled job from queue: %v\n", err)
		}
	}

	return nil
}

// Close closes the manager (currently no cleanup needed)
func (m *Manager) Close() error {
	return nil
}
