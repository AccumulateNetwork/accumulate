// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Rate limiting for the info server (#4039). The endpoints are unauthenticated
// and cheap to call but not cheap to serve — each one walks live peer state —
// so an unmetered listener is a free amplifier.
//
// Deliberately a token bucket per client IP, implemented here rather than
// pulled from a library: accumulated-http's --connection-limit caps concurrent
// requests in flight, which bounds memory but not request RATE, and one caller
// can still hammer a single connection. The two limits answer different
// questions and this one needs the rate.
const (
	// defaultRateBurst is how many requests one client may make back to back.
	defaultRateBurst = 20
	// defaultRatePerSecond is the sustained per-client refill.
	defaultRatePerSecond = 5
	// idleEvictAfter bounds the bucket map: a client that stops calling is
	// forgotten, so the limiter cannot be turned into a memory leak by
	// rotating source addresses.
	idleEvictAfter = 10 * time.Minute
)

type bucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter allows burst requests per client, refilling at perSecond.
type rateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	burst     float64
	perSecond float64
	lastSweep time.Time
}

func newRateLimiter(burst, perSecond float64) *rateLimiter {
	return &rateLimiter{
		buckets:   map[string]*bucket{},
		burst:     burst,
		perSecond: perSecond,
		lastSweep: time.Now(),
	}
}

// allow reports whether this client may make a request now.
func (l *rateLimiter) allow(client string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastSweep) > idleEvictAfter {
		for k, b := range l.buckets {
			if now.Sub(b.last) > idleEvictAfter {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}

	b, ok := l.buckets[client]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[client] = b
	}

	// Refill for elapsed time, capped at the burst size.
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens += elapsed * l.perSecond
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.last = now
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// clientIP is the rate-limit key. The RemoteAddr is used rather than any
// forwarded header: headers are attacker-controlled, so keying on them would
// let one caller mint unlimited buckets and bypass the limit entirely.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limit wraps h, rejecting clients over their budget with 429.
func (l *rateLimiter) limit(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r), time.Now()) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		h.ServeHTTP(w, r)
	})
}
