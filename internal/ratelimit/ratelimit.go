package ratelimit

import (
	"context"
	"sync"
	"time"
)

// TokenBucket implements a simple token bucket rate limiter.
// Thread-safe, suitable for controlling email send rate, API call rate, etc.
type TokenBucket struct {
	mu         sync.Mutex
	capacity   int
	tokens     int
	refillRate int           // tokens per interval
	interval   time.Duration
	lastRefill time.Time
}

// New creates a token bucket rate limiter.
// capacity: max burst size.
// refillRate: how many tokens to add per interval.
// interval: how often to refill.
func New(capacity, refillRate int, interval time.Duration) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		interval:   interval,
		lastRefill: time.Now(),
	}
}

// Allow checks if a token is available without blocking.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// Wait blocks until a token is available or context is cancelled.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	for {
		tb.mu.Lock()
		tb.refill()

		if tb.tokens > 0 {
			tb.tokens--
			tb.mu.Unlock()
			return nil
		}

		// Calculate time until next refill
		waitDuration := time.Until(tb.lastRefill.Add(tb.interval))
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Loop back to try again
		}
	}
}

// Available returns the number of tokens available.
func (tb *TokenBucket) Available() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// Capacity returns the maximum burst capacity.
func (tb *TokenBucket) Capacity() int {
	return tb.capacity
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	if elapsed < tb.interval {
		return
	}

	// Calculate how many intervals have passed
	intervals := int(elapsed / tb.interval)
	tokensToAdd := intervals * tb.refillRate

	tb.tokens += tokensToAdd
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	tb.lastRefill = now.Add(-(elapsed % tb.interval))
}

// MultiLimiter allows rate limiting across multiple scopes (e.g., per-domain).
type MultiLimiter struct {
	mu      sync.RWMutex
	limiters map[string]*TokenBucket
	capacity int
	refill   int
	interval time.Duration
}

// NewMulti creates a multi-scope rate limiter.
func NewMulti(capacity, refillRate int, interval time.Duration) *MultiLimiter {
	return &MultiLimiter{
		limiters: make(map[string]*TokenBucket),
		capacity: capacity,
		refill:   refillRate,
		interval: interval,
	}
}

// Get returns the rate limiter for a given scope key.
func (ml *MultiLimiter) Get(key string) *TokenBucket {
	ml.mu.RLock()
	limiter, ok := ml.limiters[key]
	ml.mu.RUnlock()

	if ok {
		return limiter
	}

	ml.mu.Lock()
	defer ml.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, ok := ml.limiters[key]; ok {
		return limiter
	}

	limiter = New(ml.capacity, ml.refill, ml.interval)
	ml.limiters[key] = limiter
	return limiter
}

// Allow checks if a token is available for the given scope.
func (ml *MultiLimiter) Allow(key string) bool {
	return ml.Get(key).Allow()
}
