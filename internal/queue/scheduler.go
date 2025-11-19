package queue

import (
	"context"
	"fmt"
	"time"
)

// ScheduledJobProcessor handles delayed/scheduled job processing
type ScheduledJobProcessor struct {
	manager  *Manager
	interval time.Duration
	stopCh   chan struct{}
}

// NewScheduledJobProcessor creates a new scheduled job processor
func NewScheduledJobProcessor(manager *Manager, interval time.Duration) *ScheduledJobProcessor {
	if interval == 0 {
		interval = 1 * time.Second // Default to 1 second
	}

	return &ScheduledJobProcessor{
		manager:  manager,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start starts the scheduled job processor
func (s *ScheduledJobProcessor) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stopCh:
			return nil
		case <-ticker.C:
			if err := s.processScheduledJobs(ctx); err != nil {
				fmt.Printf("error processing scheduled jobs: %v\n", err)
			}
		}
	}
}

// Stop stops the scheduled job processor
func (s *ScheduledJobProcessor) Stop() {
	close(s.stopCh)
}

// processScheduledJobs processes jobs that are scheduled to run
func (s *ScheduledJobProcessor) processScheduledJobs(ctx context.Context) error {
	s.manager.mu.RLock()
	queues := make([]string, 0, len(s.manager.configs))
	for qName := range s.manager.configs {
		queues = append(queues, qName)
	}
	s.manager.mu.RUnlock()

	now := time.Now()

	// Check each queue for scheduled jobs
	for _, qName := range queues {
		// Get jobs scheduled up to now
		jobIDs, err := s.manager.storage.GetScheduled(ctx, qName, now)
		if err != nil {
			continue
		}

		// Process each scheduled job
		for _, jobID := range jobIDs {
			// The job should already be in the queue with the correct scheduled time
			// The storage backend handles this - we just log here
			if len(jobIDs) > 0 {
				fmt.Printf("processed %d scheduled jobs for queue %s\n", len(jobIDs), qName)
			}
		}
	}

	return nil
}
