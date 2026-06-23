package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterEntry pairs a token bucket with its last use, so stale buckets can be
// evicted individually instead of wiping every client's state at once.
type limiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// RateLimiter provides per-IP rate limiting.
type RateLimiter struct {
	limiters map[string]*limiterEntry
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a rate limiter with the given requests per second and burst size.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(rps),
		burst:    burst,
	}
}

// getLimiter returns the rate limiter for the given key, creating one if needed.
func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	now := time.Now()

	rl.mu.RLock()
	entry, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if exists {
		rl.mu.Lock()
		entry.lastAccess = now
		rl.mu.Unlock()
		return entry.limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, exists = rl.limiters[key]; exists {
		entry.lastAccess = now
		return entry.limiter
	}

	entry = &limiterEntry{limiter: rate.NewLimiter(rl.rate, rl.burst), lastAccess: now}
	rl.limiters[key] = entry
	return entry.limiter
}

// Allow checks if a request from the given key should be allowed.
func (rl *RateLimiter) Allow(key string) bool {
	return rl.getLimiter(key).Allow()
}

// Cleanup evicts limiters not used within maxAge. Buckets in active use are
// kept, so legitimate clients never lose their limiter state to a flush.
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, entry := range rl.limiters {
		if entry.lastAccess.Before(cutoff) {
			delete(rl.limiters, key)
		}
	}
}

// clientIP keys the limiter on the connection's remote address. Any
// proxy-supplied client IP must come via r.RemoteAddr (chi's RealIP is mounted
// only when TRUST_PROXY_HEADERS is set); reading X-Real-IP here directly would
// let any client mint fresh buckets per request or exhaust a victim's bucket.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// RateLimit returns middleware that limits requests per IP address.
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(rps, burst)

	// Background cleanup every minute; entries idle for 10 minutes are evicted.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			limiter.Cleanup(10 * time.Minute)
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// In-process calls (e.g. the embedded MCP endpoint dispatching to the
			// REST stack) are already bounded by the external request that
			// triggered them, which was limited at the edge. Don't double-limit —
			// and never let MCP fan-out share one bucket keyed by the loopback.
			if IsInternal(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			if !limiter.Allow(clientIP(r)) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
