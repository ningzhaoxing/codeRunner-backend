package ratelimit

import (
	"sync"
	"time"
)

type Config struct {
	PerMinute int
	PerDay    int
}

type ipBucket struct {
	mu          sync.Mutex
	minuteCount int
	minuteReset time.Time
	dayCount    int
	dayReset    time.Time
	lastSeen    time.Time
}

type IPRateLimiter struct {
	cfg     Config
	buckets sync.Map // string -> *ipBucket
}

func NewIPRateLimiter(cfg Config) *IPRateLimiter {
	rl := &IPRateLimiter{cfg: cfg}
	go rl.janitor()
	return rl
}

func (rl *IPRateLimiter) Allow(ip string) bool {
	v, _ := rl.buckets.LoadOrStore(ip, &ipBucket{
		minuteReset: time.Now().Add(time.Minute),
		dayReset:    time.Now().Add(24 * time.Hour),
	})
	b := v.(*ipBucket)

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.lastSeen = now

	if now.After(b.minuteReset) {
		b.minuteCount = 0
		b.minuteReset = now.Add(time.Minute)
	}
	if now.After(b.dayReset) {
		b.dayCount = 0
		b.dayReset = now.Add(24 * time.Hour)
	}

	if b.minuteCount >= rl.cfg.PerMinute || b.dayCount >= rl.cfg.PerDay {
		return false
	}

	b.minuteCount++
	b.dayCount++
	return true
}

func (rl *IPRateLimiter) janitor() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-24 * time.Hour)
		rl.buckets.Range(func(k, v any) bool {
			b := v.(*ipBucket)
			b.mu.Lock()
			inactive := b.lastSeen.Before(cutoff)
			b.mu.Unlock()
			if inactive {
				rl.buckets.Delete(k)
			}
			return true
		})
	}
}
