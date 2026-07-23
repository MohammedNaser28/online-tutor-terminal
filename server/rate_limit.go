package main

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu     sync.Mutex
	limits map[string][]time.Time
	window time.Duration
	max    int
}

func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limits: make(map[string][]time.Time),
		window: window,
		max:    max,
	}
}

func (r *RateLimiter) Allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	entries := r.limits[ip]
	valid := entries[:0]
	for _, t := range entries {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= r.max {
		r.limits[ip] = valid
		return false
	}

	r.limits[ip] = append(valid, now)
	return true
}
