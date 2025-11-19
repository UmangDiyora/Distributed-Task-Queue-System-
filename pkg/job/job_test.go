package job

import (
	"testing"
	"time"
)

func TestJobStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		terminal bool
		canRetry bool
	}{
		{"Pending", StatusPending, false, false},
		{"Running", StatusRunning, false, false},
		{"Success", StatusSuccess, true, false},
		{"Failed", StatusFailed, true, true},
		{"Retrying", StatusRetrying, false, true},
		{"Cancelled", StatusCancelled, true, false},
		{"Expired", StatusExpired, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &Job{Status: tt.status}

			if got := j.IsTerminal(); got != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.terminal)
			}
		})
	}
}

func TestCanRetry(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int
		maxRetries int
		status     Status
		want       bool
	}{
		{"No retries used", 0, 3, StatusFailed, true},
		{"Max retries reached", 3, 3, StatusFailed, false},
		{"Exceeded max retries", 4, 3, StatusFailed, false},
		{"Success - no retry", 0, 3, StatusSuccess, false},
		{"Cancelled - no retry", 0, 3, StatusCancelled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &Job{
				RetryCount: tt.retryCount,
				MaxRetries: tt.maxRetries,
				Status:     tt.status,
			}

			if got := j.CanRetry(); got != tt.want {
				t.Errorf("CanRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsReady(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		scheduledAt time.Time
		want        bool
	}{
		{"Past time", now.Add(-1 * time.Hour), true},
		{"Current time", now, true},
		{"Future time", now.Add(1 * time.Hour), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &Job{ScheduledAt: tt.scheduledAt}

			if got := j.IsReady(); got != tt.want {
				t.Errorf("IsReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

	tests := []struct {
		name        string
		startedAt   *time.Time
		completedAt *time.Time
		want        time.Duration
	}{
		{"Not started", nil, nil, 0},
		{"Started but not completed", &oneHourAgo, nil, 0},
		{"Completed", &oneHourAgo, &now, time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &Job{
				StartedAt:   tt.startedAt,
				CompletedAt: tt.completedAt,
			}

			got := j.Duration()
			if tt.want == 0 {
				if got != 0 {
					t.Errorf("Duration() = %v, want %v", got, tt.want)
				}
			} else {
				// Allow small differences due to timing
				diff := got - tt.want
				if diff < 0 {
					diff = -diff
				}
				if diff > time.Second {
					t.Errorf("Duration() = %v, want %v (diff: %v)", got, tt.want, diff)
				}
			}
		})
	}
}

func TestWorkerIsAlive(t *testing.T) {
	now := time.Now()
	recentHeartbeat := now.Add(-10 * time.Second)
	oldHeartbeat := now.Add(-5 * time.Minute)

	tests := []struct {
		name          string
		lastHeartbeat *time.Time
		want          bool
	}{
		{"No heartbeat", nil, false},
		{"Recent heartbeat", &recentHeartbeat, true},
		{"Old heartbeat", &oldHeartbeat, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{LastHeartbeat: tt.lastHeartbeat}

			if got := w.IsAlive(time.Minute); got != tt.want {
				t.Errorf("IsAlive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkerSuccessRate(t *testing.T) {
	tests := []struct {
		name      string
		processed int64
		failed    int64
		want      float64
	}{
		{"No jobs", 0, 0, 0.0},
		{"All success", 100, 0, 100.0},
		{"All failed", 0, 100, 0.0},
		{"50% success", 50, 50, 50.0},
		{"75% success", 75, 25, 75.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{
				ProcessedCount: tt.processed,
				FailedCount:    tt.failed,
			}

			if got := w.SuccessRate(); got != tt.want {
				t.Errorf("SuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPriorityLevels(t *testing.T) {
	if PriorityHigh != 2 {
		t.Errorf("PriorityHigh = %v, want 2", PriorityHigh)
	}
	if PriorityNormal != 1 {
		t.Errorf("PriorityNormal = %v, want 1", PriorityNormal)
	}
	if PriorityLow != 0 {
		t.Errorf("PriorityLow = %v, want 0", PriorityLow)
	}
}

func TestRetryStrategies(t *testing.T) {
	strategies := []RetryStrategy{
		RetryStrategyFixed,
		RetryStrategyLinear,
		RetryStrategyExponential,
	}

	for _, strategy := range strategies {
		if strategy == "" {
			t.Errorf("Retry strategy should not be empty")
		}
	}
}
