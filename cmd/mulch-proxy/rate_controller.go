package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// RateController will manage rate limits and request delays
// (using a internal hashmap of rate limiters per ip)
type RateController struct {
	tooManyRequestsCounter uint64
	config                 RateControllerConfig
	entries                map[string]*RateControllerEntry
	mu                     sync.Mutex
}

// RateControllerConfig holds controller config
type RateControllerConfig struct {
	Name string // name of this profile

	// max number of concurrent "running" requests (0 = unlimited)
	// WARN: the client may close a request (and therefore decrement the
	// counter) but the request can still exists in the backend
	ConcurrentMaxRequests     int32
	ConcurrentOverflowTimeout time.Duration // when we are over the limit, how long to wait before returning an error

	RateEnable            bool          // enable rate limiting (if false, only MaxConcurrentRequests is used)
	RateBurst             int           // number of requests that be accepted without delay… (must be > 0)
	RateRequestsPerSecond float64       // … and after that, requests will be delayed to match this rate … (must be > 0)
	RateMaxDelay          time.Duration // … with a maximum delay of this duration

	VipList map[string]bool // list of IPs that are not rate limited
}

// RateControllerEntry holds data for a single IP
type RateControllerEntry struct {
	rateController      *RateController
	currentRequestSlots chan bool
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

func (rc *RateController) IsActive() bool {
	return rc.config.ConcurrentMaxRequests > 0 || rc.config.RateEnable
}

// GetEntry will return the RateControllerEntry for a given IP
func (rc *RateController) GetEntry(ip string) *RateControllerEntry {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	entry, ok := rc.entries[ip]
	if !ok {
		entry = &RateControllerEntry{
			rateController: rc,
		}

		if rc.config.ConcurrentMaxRequests > 0 {
			entry.currentRequestSlots = make(chan bool, rc.config.ConcurrentMaxRequests)
		}

		if rc.config.RateEnable {
			entry.rateLimiter = rate.NewLimiter(rate.Limit(rc.config.RateRequestsPerSecond), rc.config.RateBurst)
		}

		rc.entries[ip] = entry
	}

	entry.lastUseTime = time.Now()

	return entry
}

// IsVIP will check if an IP is in the VIP list
func (rc *RateController) IsVIP(ip string) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	_, ok := rc.config.VipList[ip]
	return ok
}

// Clean will remove old entries
// WARNING: a scheduled goroutine calling this function will prevent
// the RateController from being garbage collected! (ex: a RateController per VM)
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
// returns (allowed, need_finish, reason)
func (rce *RateControllerEntry) IsAllowed(reqCtx context.Context) (bool, bool, string) {
	// check if we are over the limit
	if rce.rateController.config.ConcurrentMaxRequests > 0 {
		select {
		case rce.currentRequestSlots <- true:
		case <-time.After(rce.rateController.config.ConcurrentOverflowTimeout):
			atomic.AddUint64(&rce.rateController.tooManyRequestsCounter, 1)
			// tell caller to NOT call FinishRequest, because we failed to get a slot
			return false, false, "concurrent requests limit timeout reached"
		}
	}

	if rce.rateLimiter == nil {
		return true, true, ""
	}

	ctx, cancel := context.WithTimeout(reqCtx, rce.rateController.config.RateMaxDelay)
	defer cancel()

	// will return an error if the context deadline (MaxDelay) is too short
	// interesting fact: it fails immediately (it's not a timeout)
	// (Tokens() is then typically -49.3 for a rate of 50 req/s)
	err := rce.rateLimiter.Wait(ctx)
	if err != nil {
		atomic.AddUint64(&rce.rateController.tooManyRequestsCounter, 1)
		return false, true, "rate limit maximum delay reached"
	}

	return true, true, ""
}

// RemoveRequest will free a request slot
func (rce *RateControllerEntry) FinishRequest() {
	if rce.rateController.config.ConcurrentMaxRequests > 0 {
		<-rce.currentRequestSlots
	}
}

// Dump will return a string representation of the controller
func (rc *RateController) Dump(w io.Writer) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	fmt.Fprintf(w, "-- Too many requests (429) error counter: %d\n", atomic.LoadUint64(&rc.tooManyRequestsCounter))

	fmt.Fprintf(w, "-- RateController %s: %d entrie(s), exp %s\n", rc.config.Name, len(rc.entries), RateControllerCleanupInterval)

	cnt := 0
	for ip, entry := range rc.entries {
		if len(entry.currentRequestSlots) == 0 && (!entry.rateController.config.RateEnable || entry.rateLimiter.Tokens() == float64(entry.rateLimiter.Burst())) {
			continue
		}

		cnt++
		fmt.Fprintf(w, "  %s:\n", ip)
		fmt.Fprintf(w, "    lastUseTime: %s\n", entry.lastUseTime)
		if entry.rateController.config.ConcurrentMaxRequests > 0 {
			fmt.Fprintf(w, "    currentRequestSlots: %d / %d\n", len(entry.currentRequestSlots), entry.rateController.config.ConcurrentMaxRequests)
		}
		if entry.rateController.config.RateEnable {
			fmt.Fprintf(w, "    rateLimiter free tokens (negative = waiting): %f / %d\n", entry.rateLimiter.Tokens(), entry.rateLimiter.Burst())
		}
	}

	fmt.Fprintf(w, "-- displayed %d non-idle entrie(s)\n\n", cnt)
}
