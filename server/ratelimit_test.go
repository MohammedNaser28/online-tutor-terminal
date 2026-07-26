package main

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("10.0.0.1") {
			t.Fatalf("request %d should be allowed (under limit)", i+1)
		}
	}
	if rl.Allow("10.0.0.1") {
		t.Error("4th request should be blocked")
	}
}

func TestRateLimiter_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	if !rl.Allow("alice") {
		t.Error("alice's first request should be allowed")
	}
	if rl.Allow("alice") {
		t.Error("alice's second request should be blocked")
	}
	if !rl.Allow("bob") {
		t.Error("bob's first request should be allowed (different key)")
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)

	if !rl.Allow("10.0.0.1") {
		t.Fatal("first should be allowed")
	}
	if rl.Allow("10.0.0.1") {
		t.Fatal("second should be blocked")
	}

	time.Sleep(60 * time.Millisecond)

	if !rl.Allow("10.0.0.1") {
		t.Error("should be allowed after window expires")
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter(100, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow("10.0.0.1")
		}()
	}
	wg.Wait()

	// Should not panic — 50 concurrent calls is fine.
	// 51st should still be allowed (limit is 100).
	if !rl.Allow("10.0.0.1") {
		t.Error("should still be allowed within limit")
	}
}
