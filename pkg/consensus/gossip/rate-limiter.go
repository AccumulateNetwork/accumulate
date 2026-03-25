// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Default rate limiting configuration values.
const (
	// DefaultVotesPerSecond is the default maximum votes per second per peer.
	DefaultVotesPerSecond = 100

	// DefaultBurstSize is the default burst allowance for votes.
	DefaultBurstSize = 200

	// DefaultBanThreshold is the default number of rate limit violations before banning.
	DefaultBanThreshold = 5

	// DefaultBanDuration is the default duration for which a peer is banned.
	DefaultBanDuration = 5 * time.Minute
)

// RateLimitConfig configures the rate limiter.
type RateLimitConfig struct {
	// VotesPerSecond is the maximum number of votes per second per peer.
	// Defaults to DefaultVotesPerSecond.
	VotesPerSecond int

	// BurstSize is the maximum burst allowance.
	// Defaults to DefaultBurstSize.
	BurstSize int

	// BanThreshold is the number of violations before banning a peer.
	// Defaults to DefaultBanThreshold.
	BanThreshold int

	// BanDuration is how long a peer is banned after exceeding threshold.
	// Defaults to DefaultBanDuration.
	BanDuration time.Duration
}

// applyDefaults fills in default values for unset configuration fields.
func (c *RateLimitConfig) applyDefaults() {
	if c.VotesPerSecond <= 0 {
		c.VotesPerSecond = DefaultVotesPerSecond
	}
	if c.BurstSize <= 0 {
		c.BurstSize = DefaultBurstSize
	}
	if c.BanThreshold <= 0 {
		c.BanThreshold = DefaultBanThreshold
	}
	if c.BanDuration <= 0 {
		c.BanDuration = DefaultBanDuration
	}
}

// peerBucket tracks the token bucket state for a single peer.
type peerBucket struct {
	tokens        float64   // Current tokens available
	lastUpdate    time.Time // Last time tokens were updated
	violations    int       // Number of rate limit violations
	bannedUntil   time.Time // Time until which peer is banned
	totalRejected int64     // Total votes rejected for metrics
}

// PeerRateLimiter provides per-peer vote rate limiting using token bucket algorithm.
type PeerRateLimiter struct {
	config RateLimitConfig
	mu     sync.RWMutex
	peers  map[peer.ID]*peerBucket
}

// NewPeerRateLimiter creates a new rate limiter with the given configuration.
func NewPeerRateLimiter(config RateLimitConfig) *PeerRateLimiter {
	config.applyDefaults()
	return &PeerRateLimiter{
		config: config,
		peers:  make(map[peer.ID]*peerBucket),
	}
}

// Allow checks if a vote from the given peer should be allowed.
// Returns true if the vote is allowed, false if rate limited.
func (r *PeerRateLimiter) Allow(peerID peer.ID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	bucket, exists := r.peers[peerID]

	if !exists {
		// New peer - initialize with full bucket
		bucket = &peerBucket{
			tokens:     float64(r.config.BurstSize),
			lastUpdate: now,
		}
		r.peers[peerID] = bucket
	}

	// Check if peer is currently banned
	if now.Before(bucket.bannedUntil) {
		bucket.totalRejected++
		return false
	}

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	tokensToAdd := elapsed * float64(r.config.VotesPerSecond)
	bucket.tokens += tokensToAdd

	// Cap at burst size
	if bucket.tokens > float64(r.config.BurstSize) {
		bucket.tokens = float64(r.config.BurstSize)
	}

	bucket.lastUpdate = now

	// Check if we have enough tokens
	if bucket.tokens < 1.0 {
		// Rate limit exceeded
		bucket.violations++
		bucket.totalRejected++

		// Check if we should ban this peer
		if bucket.violations >= r.config.BanThreshold {
			bucket.bannedUntil = now.Add(r.config.BanDuration)
		}

		return false
	}

	// Consume one token
	bucket.tokens -= 1.0
	return true
}

// IsBanned returns true if the peer is currently banned.
func (r *PeerRateLimiter) IsBanned(peerID peer.ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	bucket, exists := r.peers[peerID]
	if !exists {
		return false
	}

	return time.Now().Before(bucket.bannedUntil)
}

// GetStats returns statistics for a peer.
func (r *PeerRateLimiter) GetStats(peerID peer.ID) (violations int, totalRejected int64, banned bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	bucket, exists := r.peers[peerID]
	if !exists {
		return 0, 0, false
	}

	return bucket.violations, bucket.totalRejected, time.Now().Before(bucket.bannedUntil)
}

// GetAllStats returns statistics for all peers.
func (r *PeerRateLimiter) GetAllStats() map[peer.ID]struct {
	Violations    int
	TotalRejected int64
	Banned        bool
} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	stats := make(map[peer.ID]struct {
		Violations    int
		TotalRejected int64
		Banned        bool
	})

	for peerID, bucket := range r.peers {
		stats[peerID] = struct {
			Violations    int
			TotalRejected int64
			Banned        bool
		}{
			Violations:    bucket.violations,
			TotalRejected: bucket.totalRejected,
			Banned:        now.Before(bucket.bannedUntil),
		}
	}

	return stats
}

// Reset clears all rate limiting state for a peer.
func (r *PeerRateLimiter) Reset(peerID peer.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, peerID)
}

// Cleanup removes state for peers that haven't been seen recently.
// Call this periodically to prevent unbounded memory growth.
func (r *PeerRateLimiter) Cleanup(maxAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for peerID, bucket := range r.peers {
		// Remove if not updated recently and not banned
		if now.Sub(bucket.lastUpdate) > maxAge && now.After(bucket.bannedUntil) {
			delete(r.peers, peerID)
		}
	}
}
