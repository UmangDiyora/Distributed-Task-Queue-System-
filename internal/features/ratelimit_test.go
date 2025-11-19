package features

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_BasicLimit(t *testing.T) {
	rl := NewRateLimiter()
	rl.SetLimit("test-queue", 2.0, 2) // 2 req/sec, burst of 2

	// First two requests should succeed (burst)
	if !rl.Allow("test-queue") {
		t.Error("First request should be allowed")
	}
	if !rl.Allow("test-queue") {
		t.Error("Second request should be allowed")
	}

	// Third request should fail (no more tokens)
	if rl.Allow("test-queue") {
		t.Error("Third request should not be allowed")
	}

	// Wait for refill
	time.Sleep(600 * time.Millisecond)

	// Should have refilled ~1 token
	if !rl.Allow("test-queue") {
		t.Error("Request after refill should be allowed")
	}
}

func TestRateLimiter_NoLimit(t *testing.T) {
	rl := NewRateLimiter()

	// Should allow requests for unconfigured queue
	for i := 0; i < 10; i++ {
		if !rl.Allow("unlimited-queue") {
			t.Error("Requests to unconfigured queue should always be allowed")
		}
	}
}

func TestRateLimiter_GetTokens(t *testing.T) {
	rl := NewRateLimiter()
	rl.SetLimit("test-queue", 1.0, 5)

	tokens := rl.GetTokens("test-queue")
	if tokens != 5.0 {
		t.Errorf("Initial tokens = %v, want 5.0", tokens)
	}

	rl.Allow("test-queue")
	tokens = rl.GetTokens("test-queue")
	if tokens != 4.0 {
		t.Errorf("Tokens after one request = %v, want 4.0", tokens)
	}

	// Unconfigured queue
	tokens = rl.GetTokens("nonexistent")
	if tokens != -1 {
		t.Errorf("Tokens for unconfigured queue = %v, want -1", tokens)
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter()
	rl.SetLimit("test-queue", 10.0, 1) // 10 req/sec, burst of 1

	// Use up the token
	rl.Allow("test-queue")

	// Wait should succeed (should wait ~100ms for next token)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx, "test-queue")
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Wait failed: %v", err)
	}

	if elapsed < 50*time.Millisecond {
		t.Errorf("Wait returned too quickly: %v", elapsed)
	}
}

func TestRateLimiter_WaitTimeout(t *testing.T) {
	rl := NewRateLimiter()
	rl.SetLimit("test-queue", 0.1, 1) // 0.1 req/sec (very slow)

	// Use up the token
	rl.Allow("test-queue")

	// Wait should timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx, "test-queue")
	if err == nil {
		t.Error("Wait should timeout")
	}
}

func TestSlidingWindowRateLimiter_BasicLimit(t *testing.T) {
	sw := NewSlidingWindowRateLimiter()
	sw.SetLimit("test-queue", 3, 1*time.Second)

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		if !sw.Allow("test-queue") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Fourth request should fail
	if sw.Allow("test-queue") {
		t.Error("Fourth request should not be allowed")
	}

	// Wait for window to slide
	time.Sleep(1100 * time.Millisecond)

	// Should allow requests again
	if !sw.Allow("test-queue") {
		t.Error("Request after window slide should be allowed")
	}
}

func TestSlidingWindowRateLimiter_GetCount(t *testing.T) {
	sw := NewSlidingWindowRateLimiter()
	sw.SetLimit("test-queue", 5, 1*time.Second)

	// Make 3 requests
	for i := 0; i < 3; i++ {
		sw.Allow("test-queue")
	}

	count := sw.GetCount("test-queue")
	if count != 3 {
		t.Errorf("GetCount = %v, want 3", count)
	}

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	count = sw.GetCount("test-queue")
	if count != 0 {
		t.Errorf("GetCount after window expiry = %v, want 0", count)
	}
}

func TestSlidingWindowRateLimiter_NoLimit(t *testing.T) {
	sw := NewSlidingWindowRateLimiter()

	// Should allow requests for unconfigured queue
	for i := 0; i < 10; i++ {
		if !sw.Allow("unlimited-queue") {
			t.Error("Requests to unconfigured queue should always be allowed")
		}
	}
}

func TestRateLimitedQueue_EnqueueWithLimit(t *testing.T) {
	rq := NewRateLimitedQueue("test-queue", 5.0, 5)
	ctx := context.Background()

	// First 5 should succeed (burst)
	for i := 0; i < 5; i++ {
		if err := rq.EnqueueWithLimit(ctx); err != nil {
			t.Errorf("Request %d should succeed: %v", i+1, err)
		}
	}

	// Sixth should fail
	if err := rq.EnqueueWithLimit(ctx); err == nil {
		t.Error("Request beyond limit should fail")
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	rl := NewRateLimiter()
	rl.SetLimit("bench-queue", 1000.0, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("bench-queue")
	}
}

func BenchmarkSlidingWindow_Allow(b *testing.B) {
	sw := NewSlidingWindowRateLimiter()
	sw.SetLimit("bench-queue", 1000, 1*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Allow("bench-queue")
	}
}
