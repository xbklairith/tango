package auth

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	ip := "192.168.1.1"

	for i := 0; i < 5; i++ {
		if !rl.Allow(ip) {
			t.Errorf("expected Allow to return true on attempt %d", i+1)
		}
		rl.Record(ip)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	ip := "10.0.0.1"

	for i := 0; i < 3; i++ {
		rl.Record(ip)
	}

	if rl.Allow(ip) {
		t.Error("expected Allow to return false after exceeding limit")
	}
}

func TestRateLimiter_SlidingWindowExpires(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)
	ip := "10.0.0.2"

	rl.Record(ip)
	rl.Record(ip)

	if rl.Allow(ip) {
		t.Error("expected blocked before window expires")
	}

	time.Sleep(100 * time.Millisecond)

	if !rl.Allow(ip) {
		t.Error("expected allowed after window expires")
	}
}

func TestRateLimiter_ResetClearsHistory(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	ip := "10.0.0.3"

	rl.Record(ip)
	rl.Record(ip)

	if rl.Allow(ip) {
		t.Error("expected blocked before reset")
	}

	rl.Reset(ip)

	if !rl.Allow(ip) {
		t.Error("expected allowed after reset")
	}
}

func TestRateLimiter_IndependentPerIP(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	ip1 := "10.0.0.10"
	ip2 := "10.0.0.20"

	rl.Record(ip1)
	rl.Record(ip1)

	if rl.Allow(ip1) {
		t.Error("expected ip1 to be blocked")
	}
	if !rl.Allow(ip2) {
		t.Error("expected ip2 to be allowed (independent)")
	}
}

func TestCheckAndRecord_AtomicBehavior(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	ip := "10.0.0.50"

	var wg sync.WaitGroup
	allowed := make(chan bool, 20)

	// Launch 20 goroutines concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- rl.CheckAndRecord(ip)
		}()
	}

	wg.Wait()
	close(allowed)

	trueCount := 0
	for v := range allowed {
		if v {
			trueCount++
		}
	}

	if trueCount != 5 {
		t.Errorf("expected exactly 5 allowed, got %d", trueCount)
	}
}

func TestStartCleanup_PrunesAndStops(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)
	ip := "10.0.0.99"

	rl.Record(ip)

	done := make(chan struct{})
	rl.StartCleanup(done, 50*time.Millisecond)

	// Wait for window to expire and cleanup to run
	time.Sleep(200 * time.Millisecond)

	rl.mu.Lock()
	remaining := len(rl.attempts)
	rl.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 entries after cleanup, got %d", remaining)
	}

	// Signal stop
	close(done)

	// Verify goroutine stops (no way to check directly, but at least no panic)
	time.Sleep(100 * time.Millisecond)
}
