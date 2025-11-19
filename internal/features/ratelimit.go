package features

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Distributed-Task-Queue-System/pkg/errors"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	limiters map[string]*tokenBucket
	mu       sync.RWMutex
}

type tokenBucket struct {
	tokens        float64
	maxTokens     float64
	refillRate    float64 // tokens per second
	lastRefill    time.Time
	mu            sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*tokenBucket),
	}
}

// SetLimit configures rate limit for a queue
// rate: requests per second
// burst: maximum burst size
func (rl *RateLimiter) SetLimit(queue string, rate float64, burst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.limiters[queue] = &tokenBucket{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(queue string) bool {
	rl.mu.RLock()
	tb, exists := rl.limiters[queue]
	rl.mu.RUnlock()

	if !exists {
		return true // No limit configured
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate

	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}

	tb.lastRefill = now

	// Check if we have tokens
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// Wait waits until a request can be allowed
func (rl *RateLimiter) Wait(ctx context.Context, queue string) error {
	for {
		if rl.Allow(queue) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Continue waiting
		}
	}
}

// GetTokens returns the current number of available tokens
func (rl *RateLimiter) GetTokens(queue string) float64 {
	rl.mu.RLock()
	tb, exists := rl.limiters[queue]
	rl.mu.RUnlock()

	if !exists {
		return -1 // No limit configured
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	return tb.tokens
}

// SlidingWindowRateLimiter implements sliding window rate limiting
type SlidingWindowRateLimiter struct {
	windows    map[string]*slidingWindow
	mu         sync.RWMutex
}

type slidingWindow struct {
	limit      int
	window     time.Duration
	requests   []time.Time
	mu         sync.Mutex
}

// NewSlidingWindowRateLimiter creates a new sliding window rate limiter
func NewSlidingWindowRateLimiter() *SlidingWindowRateLimiter {
	return &SlidingWindowRateLimiter{
		windows: make(map[string]*slidingWindow),
	}
}

// SetLimit configures rate limit for a queue
// limit: maximum requests
// window: time window duration
func (sw *SlidingWindowRateLimiter) SetLimit(queue string, limit int, window time.Duration) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.windows[queue] = &slidingWindow{
		limit:    limit,
		window:   window,
		requests: make([]time.Time, 0, limit),
	}
}

// Allow checks if a request should be allowed
func (sw *SlidingWindowRateLimiter) Allow(queue string) bool {
	sw.mu.RLock()
	w, exists := sw.windows[queue]
	sw.mu.RUnlock()

	if !exists {
		return true // No limit configured
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-w.window)

	// Remove old requests
	validRequests := make([]time.Time, 0, len(w.requests))
	for _, req := range w.requests {
		if req.After(cutoff) {
			validRequests = append(validRequests, req)
		}
	}
	w.requests = validRequests

	// Check if we can add another request
	if len(w.requests) < w.limit {
		w.requests = append(w.requests, now)
		return true
	}

	return false
}

// GetCount returns the current request count in the window
func (sw *SlidingWindowRateLimiter) GetCount(queue string) int {
	sw.mu.RLock()
	w, exists := sw.windows[queue]
	sw.mu.RUnlock()

	if !exists {
		return 0
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-w.window)

	count := 0
	for _, req := range w.requests {
		if req.After(cutoff) {
			count++
		}
	}

	return count
}

// RateLimitedQueue wraps a queue with rate limiting
type RateLimitedQueue struct {
	queueName   string
	rateLimiter *RateLimiter
}

// NewRateLimitedQueue creates a new rate-limited queue
func NewRateLimitedQueue(queue string, rps float64, burst int) *RateLimitedQueue {
	limiter := NewRateLimiter()
	limiter.SetLimit(queue, rps, burst)

	return &RateLimitedQueue{
		queueName:   queue,
		rateLimiter: limiter,
	}
}

// EnqueueWithLimit enqueues a job with rate limiting
func (rq *RateLimitedQueue) EnqueueWithLimit(ctx context.Context) error {
	if !rq.rateLimiter.Allow(rq.queueName) {
		return errors.NewQueueError(
			fmt.Sprintf("rate limit exceeded for queue: %s", rq.queueName),
			nil,
		)
	}
	return nil
}

// WaitAndEnqueue waits for rate limit and then enqueues
func (rq *RateLimitedQueue) WaitAndEnqueue(ctx context.Context) error {
	return rq.rateLimiter.Wait(ctx, rq.queueName)
}
