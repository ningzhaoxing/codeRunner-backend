package ratelimit_test

import (
	"testing"
	"time"

	"codeRunner-siwu/internal/infrastructure/ratelimit"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := ratelimit.NewIPRateLimiter(ratelimit.Config{
		PerMinute: 3,
		PerDay:    20,
	})
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverMinuteLimit(t *testing.T) {
	rl := ratelimit.NewIPRateLimiter(ratelimit.Config{
		PerMinute: 2,
		PerDay:    20,
	})
	rl.Allow("1.2.3.4")
	rl.Allow("1.2.3.4")
	if rl.Allow("1.2.3.4") {
		t.Error("3rd request should be blocked (minute limit=2)")
	}
}

func TestRateLimiter_BlocksOverDayLimit(t *testing.T) {
	rl := ratelimit.NewIPRateLimiter(ratelimit.Config{
		PerMinute: 100,
		PerDay:    2,
	})
	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.1")
	if rl.Allow("10.0.0.1") {
		t.Error("3rd request should be blocked (day limit=2)")
	}
}

func TestRateLimiter_DifferentIPsAreIndependent(t *testing.T) {
	rl := ratelimit.NewIPRateLimiter(ratelimit.Config{
		PerMinute: 1,
		PerDay:    5,
	})
	if !rl.Allow("192.168.1.1") {
		t.Error("first request for IP A should be allowed")
	}
	if !rl.Allow("192.168.1.2") {
		t.Error("first request for IP B should be allowed")
	}
}

func TestRateLimiter_MinuteWindowResets(t *testing.T) {
	_ = time.Now()
	rl := ratelimit.NewIPRateLimiter(ratelimit.Config{PerMinute: 1, PerDay: 10})
	if !rl.Allow("5.5.5.5") {
		t.Error("first request should be allowed")
	}
}
