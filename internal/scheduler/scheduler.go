package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/queue"
	"github.com/Distributed-Task-Queue-System/pkg/job"
	"github.com/google/uuid"
)

// Schedule represents a recurring job schedule
type Schedule struct {
	ID          string
	Name        string
	JobType     string
	Queue       string
	Payload     map[string]interface{}
	CronExpr    string         // Cron expression for scheduling
	Interval    time.Duration  // Alternative: simple interval
	NextRun     time.Time
	LastRun     *time.Time
	Enabled     bool
	MaxRuns     int            // 0 = unlimited
	RunCount    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Metadata    map[string]interface{}
}

// Scheduler manages recurring jobs and schedules
type Scheduler struct {
	queueManager *queue.Manager
	schedules    map[string]*Schedule
	mu           sync.RWMutex
	ticker       *time.Ticker
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewScheduler creates a new scheduler
func NewScheduler(qm *queue.Manager) *Scheduler {
	return &Scheduler{
		queueManager: qm,
		schedules:    make(map[string]*Schedule),
		stopCh:       make(chan struct{}),
	}
}

// AddSchedule adds a new recurring schedule
func (s *Scheduler) AddSchedule(schedule *Schedule) error {
	if schedule.ID == "" {
		schedule.ID = uuid.New().String()
	}

	if schedule.JobType == "" {
		return fmt.Errorf("job type is required")
	}

	if schedule.Queue == "" {
		schedule.Queue = "default"
	}

	now := time.Now()
	schedule.CreatedAt = now
	schedule.UpdatedAt = now

	// Calculate next run time
	if schedule.Interval > 0 {
		schedule.NextRun = now.Add(schedule.Interval)
	} else if schedule.CronExpr != "" {
		nextRun, err := s.parseNextCronTime(schedule.CronExpr, now)
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		schedule.NextRun = nextRun
	} else {
		return fmt.Errorf("either interval or cron expression is required")
	}

	schedule.Enabled = true

	s.mu.Lock()
	s.schedules[schedule.ID] = schedule
	s.mu.Unlock()

	fmt.Printf("Added schedule %s (%s) - next run: %s\n",
		schedule.ID, schedule.Name, schedule.NextRun.Format(time.RFC3339))

	return nil
}

// RemoveSchedule removes a schedule
func (s *Scheduler) RemoveSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.schedules[id]; !exists {
		return fmt.Errorf("schedule not found: %s", id)
	}

	delete(s.schedules, id)
	return nil
}

// GetSchedule retrieves a schedule by ID
func (s *Scheduler) GetSchedule(id string) (*Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedule, exists := s.schedules[id]
	if !exists {
		return nil, fmt.Errorf("schedule not found: %s", id)
	}

	return schedule, nil
}

// ListSchedules returns all schedules
func (s *Scheduler) ListSchedules() []*Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedules := make([]*Schedule, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		schedules = append(schedules, schedule)
	}

	return schedules
}

// EnableSchedule enables a schedule
func (s *Scheduler) EnableSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, exists := s.schedules[id]
	if !exists {
		return fmt.Errorf("schedule not found: %s", id)
	}

	schedule.Enabled = true
	schedule.UpdatedAt = time.Now()

	return nil
}

// DisableSchedule disables a schedule
func (s *Scheduler) DisableSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, exists := s.schedules[id]
	if !exists {
		return fmt.Errorf("schedule not found: %s", id)
	}

	schedule.Enabled = false
	schedule.UpdatedAt = time.Now()

	return nil
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context, tickInterval time.Duration) error {
	if tickInterval == 0 {
		tickInterval = 1 * time.Second
	}

	s.ticker = time.NewTicker(tickInterval)

	s.wg.Add(1)
	go s.run(ctx)

	fmt.Printf("Scheduler started with tick interval: %v\n", tickInterval)
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	fmt.Println("Stopping scheduler...")
	close(s.stopCh)
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.wg.Wait()
	fmt.Println("Scheduler stopped")
	return nil
}

// run is the main scheduler loop
func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-s.ticker.C:
			s.processDueSchedules(ctx)
		}
	}
}

// processDueSchedules processes all schedules that are due to run
func (s *Scheduler) processDueSchedules(ctx context.Context) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, schedule := range s.schedules {
		if !schedule.Enabled {
			continue
		}

		// Check if schedule is due
		if now.Before(schedule.NextRun) {
			continue
		}

		// Check max runs limit
		if schedule.MaxRuns > 0 && schedule.RunCount >= schedule.MaxRuns {
			schedule.Enabled = false
			fmt.Printf("Schedule %s reached max runs (%d), disabling\n",
				schedule.ID, schedule.MaxRuns)
			continue
		}

		// Create and enqueue job
		if err := s.executeSchedule(ctx, schedule); err != nil {
			fmt.Printf("Error executing schedule %s: %v\n", schedule.ID, err)
			continue
		}

		// Update schedule
		schedule.LastRun = &now
		schedule.RunCount++
		schedule.UpdatedAt = now

		// Calculate next run time
		if schedule.Interval > 0 {
			schedule.NextRun = now.Add(schedule.Interval)
		} else if schedule.CronExpr != "" {
			nextRun, err := s.parseNextCronTime(schedule.CronExpr, now)
			if err != nil {
				fmt.Printf("Error parsing cron expression for schedule %s: %v\n",
					schedule.ID, err)
				schedule.Enabled = false
				continue
			}
			schedule.NextRun = nextRun
		}

		fmt.Printf("Executed schedule %s (%s) - next run: %s\n",
			schedule.ID, schedule.Name, schedule.NextRun.Format(time.RFC3339))
	}
}

// executeSchedule creates and enqueues a job for a schedule
func (s *Scheduler) executeSchedule(ctx context.Context, schedule *Schedule) error {
	j := &job.Job{
		ID:       uuid.New().String(),
		Type:     schedule.JobType,
		Queue:    schedule.Queue,
		Payload:  schedule.Payload,
		Status:   job.StatusPending,
		Priority: job.PriorityNormal,
		Metadata: map[string]interface{}{
			"schedule_id":   schedule.ID,
			"schedule_name": schedule.Name,
		},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ScheduledAt: time.Now(),
	}

	// Merge schedule metadata
	if schedule.Metadata != nil {
		for k, v := range schedule.Metadata {
			j.Metadata[k] = v
		}
	}

	return s.queueManager.Enqueue(ctx, j)
}

// parseNextCronTime parses a simple cron expression and returns the next run time
// This is a simplified cron parser. For production, use github.com/robfig/cron
func (s *Scheduler) parseNextCronTime(cronExpr string, from time.Time) (time.Time, error) {
	// Simplified cron parsing - supports common patterns
	// For a full implementation, integrate with github.com/robfig/cron

	// Common patterns:
	// "@hourly" - every hour
	// "@daily" - every day at midnight
	// "@weekly" - every week
	// "@monthly" - every month

	switch cronExpr {
	case "@minutely":
		return from.Add(1 * time.Minute).Truncate(time.Minute), nil
	case "@hourly":
		return from.Add(1 * time.Hour).Truncate(time.Hour), nil
	case "@daily":
		next := from.AddDate(0, 0, 1)
		return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, from.Location()), nil
	case "@weekly":
		next := from.AddDate(0, 0, 7)
		return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, from.Location()), nil
	case "@monthly":
		next := from.AddDate(0, 1, 0)
		return time.Date(next.Year(), next.Month(), 1, 0, 0, 0, 0, from.Location()), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported cron expression: %s", cronExpr)
	}
}

// GetScheduleStats returns statistics for schedules
func (s *Scheduler) GetScheduleStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalSchedules := len(s.schedules)
	enabledSchedules := 0
	disabledSchedules := 0
	totalRuns := 0

	for _, schedule := range s.schedules {
		if schedule.Enabled {
			enabledSchedules++
		} else {
			disabledSchedules++
		}
		totalRuns += schedule.RunCount
	}

	return map[string]interface{}{
		"total_schedules":    totalSchedules,
		"enabled_schedules":  enabledSchedules,
		"disabled_schedules": disabledSchedules,
		"total_runs":         totalRuns,
	}
}
