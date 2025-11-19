package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Distributed-Task-Queue-System/internal/queue"
	"github.com/Distributed-Task-Queue-System/internal/scheduler"
	"github.com/Distributed-Task-Queue-System/pkg/job"
)

// Server is the HTTP API server
type Server struct {
	queueManager *queue.Manager
	scheduler    *scheduler.Scheduler
	server       *http.Server
	mux          *http.ServeMux
}

// Config holds HTTP server configuration
type Config struct {
	Address string
	Port    int
}

// NewServer creates a new HTTP API server
func NewServer(qm *queue.Manager, sched *scheduler.Scheduler, config *Config) *Server {
	if config.Address == "" {
		config.Address = "0.0.0.0"
	}
	if config.Port == 0 {
		config.Port = 8080
	}

	s := &Server{
		queueManager: qm,
		scheduler:    sched,
		mux:          http.NewServeMux(),
	}

	s.setupRoutes()

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Address, config.Port),
		Handler:      s.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// setupRoutes sets up HTTP routes
func (s *Server) setupRoutes() {
	// Health check
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)

	// Job endpoints
	s.mux.HandleFunc("/api/v1/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/v1/jobs/", s.handleJob)
	s.mux.HandleFunc("/api/v1/jobs/{id}/cancel", s.handleCancelJob)

	// Queue endpoints
	s.mux.HandleFunc("/api/v1/queues", s.handleQueues)
	s.mux.HandleFunc("/api/v1/queues/", s.handleQueue)
	s.mux.HandleFunc("/api/v1/queues/{name}/pause", s.handlePauseQueue)
	s.mux.HandleFunc("/api/v1/queues/{name}/resume", s.handleResumeQueue)
	s.mux.HandleFunc("/api/v1/queues/{name}/stats", s.handleQueueStats)

	// Schedule endpoints
	s.mux.HandleFunc("/api/v1/schedules", s.handleSchedules)
	s.mux.HandleFunc("/api/v1/schedules/", s.handleSchedule)

	// Metrics endpoint
	s.mux.HandleFunc("/api/v1/metrics", s.handleMetrics)
}

// Start starts the HTTP server
func (s *Server) Start() error {
	fmt.Printf("Starting HTTP API server on %s\n", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	fmt.Println("Stopping HTTP API server...")
	return s.server.Shutdown(ctx)
}

// ===== Health Handlers =====

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// ===== Job Handlers =====

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// List jobs
		queue := r.URL.Query().Get("queue")
		status := job.Status(r.URL.Query().Get("status"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		if limit == 0 {
			limit = 50
		}

		jobs, err := s.queueManager.ListJobs(ctx, queue, status, limit, offset)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"jobs":   jobs,
			"count":  len(jobs),
			"limit":  limit,
			"offset": offset,
		})

	case http.MethodPost:
		// Create job
		var j job.Job
		if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Set defaults
		if j.ID == "" {
			j.ID = fmt.Sprintf("job_%d", time.Now().UnixNano())
		}
		j.Status = job.StatusPending
		j.CreatedAt = time.Now()
		j.UpdatedAt = time.Now()
		if j.ScheduledAt.IsZero() {
			j.ScheduledAt = time.Now()
		}

		if err := s.queueManager.Enqueue(ctx, &j); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusCreated, j)

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.URL.Path[len("/api/v1/jobs/"):]

	switch r.Method {
	case http.MethodGet:
		// Get job
		j, err := s.queueManager.GetJob(ctx, id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "job not found")
			return
		}

		s.writeJSON(w, http.StatusOK, j)

	case http.MethodDelete:
		// Delete job
		if err := s.queueManager.DeleteJob(ctx, id); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]string{
			"message": "job deleted",
		})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	id := r.URL.Path[len("/api/v1/jobs/"):]
	id = id[:len(id)-len("/cancel")]

	if err := s.queueManager.CancelJob(ctx, id); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "job cancelled",
	})
}

// ===== Queue Handlers =====

func (s *Server) handleQueues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// List queues
		queues, err := s.queueManager.ListQueues(ctx)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"queues": queues,
			"count":  len(queues),
		})

	case http.MethodPost:
		// Create queue
		var config job.QueueConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := s.queueManager.RegisterQueue(ctx, &config); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusCreated, config)

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.URL.Path[len("/api/v1/queues/"):]

	switch r.Method {
	case http.MethodGet:
		// Get queue config
		config, err := s.queueManager.GetQueueConfig(ctx, name)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "queue not found")
			return
		}

		s.writeJSON(w, http.StatusOK, config)

	case http.MethodPut:
		// Update queue config
		var config job.QueueConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		config.Name = name
		if err := s.queueManager.UpdateQueueConfig(ctx, &config); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusOK, config)

	case http.MethodDelete:
		// Delete queue
		if err := s.queueManager.DeleteQueue(ctx, name); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]string{
			"message": "queue deleted",
		})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handlePauseQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	name := r.URL.Path[len("/api/v1/queues/"):]
	name = name[:len(name)-len("/pause")]

	if err := s.queueManager.PauseQueue(ctx, name); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "queue paused",
	})
}

func (s *Server) handleResumeQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	name := r.URL.Path[len("/api/v1/queues/"):]
	name = name[:len(name)-len("/resume")]

	if err := s.queueManager.ResumeQueue(ctx, name); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "queue resumed",
	})
}

func (s *Server) handleQueueStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	name := r.URL.Path[len("/api/v1/queues/"):]
	name = name[:len(name)-len("/stats")]

	stats, err := s.queueManager.GetQueueStats(ctx, name)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, stats)
}

// ===== Schedule Handlers =====

func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List schedules
		schedules := s.scheduler.ListSchedules()

		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"schedules": schedules,
			"count":     len(schedules),
		})

	case http.MethodPost:
		// Create schedule
		var schedule scheduler.Schedule
		if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := s.scheduler.AddSchedule(&schedule); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusCreated, schedule)

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/v1/schedules/"):]

	switch r.Method {
	case http.MethodGet:
		// Get schedule
		schedule, err := s.scheduler.GetSchedule(id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "schedule not found")
			return
		}

		s.writeJSON(w, http.StatusOK, schedule)

	case http.MethodDelete:
		// Delete schedule
		if err := s.scheduler.RemoveSchedule(id); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]string{
			"message": "schedule deleted",
		})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ===== Metrics Handler =====

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()

	// Get all queue stats
	allStats, err := s.queueManager.GetAllQueueStats(ctx)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Get schedule stats
	scheduleStats := s.scheduler.GetScheduleStats()

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"queue_stats":    allStats,
		"schedule_stats": scheduleStats,
		"timestamp":      time.Now().Format(time.RFC3339),
	})
}

// ===== Helper Methods =====

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{
		"error": message,
	})
}
