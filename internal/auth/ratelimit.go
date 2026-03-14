package auth

import (
	"sync"
	"time"
)

// RateLimiter tracks failed login attempts per IP address.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a rate limiter with the given limit and window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks whether the given IP is within the rate limit.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.pruneExpired(ip)
	return len(rl.attempts[ip]) < rl.limit
}

// CheckAndRecord atomically checks the rate limit and records a failed attempt.
// Returns true if the request should proceed, false if rate-limited.
func (rl *RateLimiter) CheckAndRecord(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.pruneExpired(ip)
	if len(rl.attempts[ip]) >= rl.limit {
		return false
	}
	rl.attempts[ip] = append(rl.attempts[ip], time.Now())
	return true
}

// Record adds a failed attempt for the given IP.
func (rl *RateLimiter) Record(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.attempts[ip] = append(rl.attempts[ip], time.Now())
}

// Reset clears the failed attempt history for the given IP.
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// pruneExpired removes attempts outside the sliding window. Must be called with mu held.
func (rl *RateLimiter) pruneExpired(ip string) {
	cutoff := time.Now().Add(-rl.window)
	attempts := rl.attempts[ip]
	i := 0
	for i < len(attempts) && attempts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		rl.attempts[ip] = attempts[i:]
	}
	if len(rl.attempts[ip]) == 0 {
		delete(rl.attempts, ip)
	}
}

// StartCleanup launches a background goroutine that prunes stale entries.
func (rl *RateLimiter) StartCleanup(done <-chan struct{}, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				rl.mu.Lock()
				for ip := range rl.attempts {
					rl.pruneExpired(ip)
				}
				rl.mu.Unlock()
			}
		}
	}()
}
