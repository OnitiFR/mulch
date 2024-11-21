package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// RateController will manage rate limits and request delays
// (using a internal hashmap of rate limiters per ip)
type RateController struct {
	config  RateControllerConfig
	entries map[string]*RateControllerEntry
	mu      sync.Mutex
}

// RateControllerConfig holds controller config
type RateControllerConfig struct {
	// max number of concurrent "running" requests (0 = unlimited)
	// WARN: the client may close a request (and therefore decrement the
	// counter) but the request can still exists in the backend
	MaxConcurrentRequests int32

	EnableRateLimit   bool          // enable rate limiting (if false, only MaxConcurrentRequests is used)
	BurstRequests     int           // number of requests that be accepted without delay… (must be > 0)
	RequestsPerSecond float64       // … and after that, requests will be delayed to match this rate … (must be > 0)
	MaxDelay          time.Duration // … with a maximum delay of this duration
}

// RateControllerEntry holds data for a single IP
type RateControllerEntry struct {
	config              *RateControllerConfig
	currentRequestCount atomic.Int32
	rateLimiter         *rate.Limiter
	lastUseTime         time.Time
}

// NewRateController will create and return a new controller
func NewRateController(config RateControllerConfig) *RateController {
	return &RateController{
		config:  config,
		entries: make(map[string]*RateControllerEntry),
	}
}

// GetEntry will return the RateControllerEntry for a given IP
func (rc *RateController) GetEntry(ip string) *RateControllerEntry {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	entry, ok := rc.entries[ip]
	if !ok {
		entry = &RateControllerEntry{
			config: &rc.config,
		}

		if rc.config.EnableRateLimit {
			entry.rateLimiter = rate.NewLimiter(rate.Limit(rc.config.RequestsPerSecond), rc.config.BurstRequests)
		}

		rc.entries[ip] = entry
	}

	entry.lastUseTime = time.Now()

	return entry
}

// Clean will remove old entries
// WARNING: a scheduled goroutine calling this function will prevent
// the RateController from being garbage collected!
func (rc *RateController) Clean(UnusedTime time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	for ip, entry := range rc.entries {
		if time.Since(entry.lastUseTime) > UnusedTime {
			delete(rc.entries, ip)
		}
	}
}

// IsAllowed will check if a request is allowed for this entry
// returns true if the request is allowed, false otherwise, with a reason
func (rce *RateControllerEntry) IsAllowed(reqCtx context.Context) (bool, string) {
	// check if we are over the limit
	if rce.config.MaxConcurrentRequests > 0 && rce.currentRequestCount.Load() > rce.config.MaxConcurrentRequests {
		return false, "max concurrent requests reached"
	}

	if rce.rateLimiter == nil {
		return true, ""
	}

	ctx, cancel := context.WithTimeout(reqCtx, rce.config.MaxDelay)
	defer cancel()

	// will return an error if the context deadline (MaxDelay) is too short
	// interesting fact: it fails immediately (it's not a timeout)
	// (Tokens() is then typically -49.3 for a rate of 50 req/s)
	err := rce.rateLimiter.Wait(ctx)
	if err != nil {
		return false, "rate limit maximum delay reached"
	}

	return true, ""
}

// AddRequest will increment the request count
func (rce *RateControllerEntry) AddRequest() {
	rce.currentRequestCount.Add(1)
}

// RemoveRequest will decrement the request count
func (rce *RateControllerEntry) RemoveRequest() {
	rce.currentRequestCount.Add(-1)
}
