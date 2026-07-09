// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_Basic(t *testing.T) {
	config := RateLimitConfig{
		VotesPerSecond: 10,
		BurstSize:      20,
		BanThreshold:   3,
		BanDuration:    time.Minute,
	}
	limiter := NewPeerRateLimiter(config)

	peerID, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	require.NoError(t, err)

	// Should allow up to burst size
	for i := 0; i < 20; i++ {
		require.True(t, limiter.Allow(peerID), "Should allow request %d", i+1)
	}

	// Next one should be rate limited
	require.False(t, limiter.Allow(peerID), "Should rate limit after burst")
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	config := RateLimitConfig{
		VotesPerSecond: 10,
		BurstSize:      10,
		BanThreshold:   5,
		BanDuration:    time.Minute,
	}
	limiter := NewPeerRateLimiter(config)

	peerID, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	require.NoError(t, err)

	// Exhaust tokens
	for i := 0; i < 10; i++ {
		require.True(t, limiter.Allow(peerID))
	}
	require.False(t, limiter.Allow(peerID), "Should be rate limited")

	// Wait for token refill (100ms for 1 token at 10/sec)
	time.Sleep(150 * time.Millisecond)

	// Should allow one more
	require.True(t, limiter.Allow(peerID), "Should allow after token refill")
}

func TestRateLimiter_BanThreshold(t *testing.T) {
	config := RateLimitConfig{
		VotesPerSecond: 10,
		BurstSize:      5,
		BanThreshold:   3,
		BanDuration:    200 * time.Millisecond,
	}
	limiter := NewPeerRateLimiter(config)

	peerID, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	require.NoError(t, err)

	// Exhaust tokens
	for i := 0; i < 5; i++ {
		require.True(t, limiter.Allow(peerID))
	}

	// Generate violations
	for i := 0; i < 3; i++ {
		require.False(t, limiter.Allow(peerID))
	}

	// Should be banned now
	require.True(t, limiter.IsBanned(peerID), "Peer should be banned after threshold")

	// Even with time passing for token refill, should still be banned
	time.Sleep(100 * time.Millisecond)
	require.False(t, limiter.Allow(peerID), "Should stay banned during ban period")

	// Wait for ban to expire
	time.Sleep(150 * time.Millisecond)
	require.False(t, limiter.IsBanned(peerID), "Ban should expire")
}

func TestRateLimiter_Stats(t *testing.T) {
	config := RateLimitConfig{
		VotesPerSecond: 10,
		BurstSize:      5,
		BanThreshold:   3,
		BanDuration:    time.Minute,
	}
	limiter := NewPeerRateLimiter(config)

	peerID, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	require.NoError(t, err)

	// Exhaust tokens and generate violations
	for i := 0; i < 5; i++ {
		limiter.Allow(peerID)
	}
	for i := 0; i < 3; i++ {
		limiter.Allow(peerID)
	}

	violations, rejected, banned := limiter.GetStats(peerID)
	require.Equal(t, 3, violations, "Should have 3 violations")
	require.Equal(t, int64(3), rejected, "Should have rejected 3 votes")
	require.True(t, banned, "Peer should be banned")
}

func TestRateLimiter_MultiPeer(t *testing.T) {
	config := RateLimitConfig{
		VotesPerSecond: 10,
		BurstSize:      10,
		BanThreshold:   3,
		BanDuration:    time.Minute,
	}
	limiter := NewPeerRateLimiter(config)

	peer1, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	require.NoError(t, err)
	peer2, err := peer.Decode("12D3KooWGiKjiSqiDvmKPtVJQfDuNPkqPncUGRKwHRtpgKYjwqws")
	require.NoError(t, err)

	// Both peers should have independent limits
	for i := 0; i < 10; i++ {
		require.True(t, limiter.Allow(peer1))
		require.True(t, limiter.Allow(peer2))
	}

	// Both should be rate limited independently
	require.False(t, limiter.Allow(peer1))
	require.False(t, limiter.Allow(peer2))

	// Peer1 gets banned
	for i := 0; i < 3; i++ {
		limiter.Allow(peer1)
	}
	require.True(t, limiter.IsBanned(peer1))
	require.False(t, limiter.IsBanned(peer2))
}

func TestRateLimiter_Cleanup(t *testing.T) {
	config := RateLimitConfig{
		VotesPerSecond: 10,
		BurstSize:      10,
		BanThreshold:   3,
		BanDuration:    time.Millisecond,
	}
	limiter := NewPeerRateLimiter(config)

	peerID, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	require.NoError(t, err)

	// Use the peer
	limiter.Allow(peerID)

	// Should have one peer tracked
	stats := limiter.GetAllStats()
	require.Len(t, stats, 1)

	// Wait for peer state to age
	time.Sleep(10 * time.Millisecond)

	// Cleanup old peers
	limiter.Cleanup(5 * time.Millisecond)

	// Should be removed
	stats = limiter.GetAllStats()
	require.Len(t, stats, 0)
}

func TestRateLimiter_Reset(t *testing.T) {
	config := RateLimitConfig{
		VotesPerSecond: 10,
		BurstSize:      5,
		BanThreshold:   3,
		BanDuration:    time.Minute,
	}
	limiter := NewPeerRateLimiter(config)

	peerID, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	require.NoError(t, err)

	// Ban the peer
	for i := 0; i < 5; i++ {
		limiter.Allow(peerID)
	}
	for i := 0; i < 3; i++ {
		limiter.Allow(peerID)
	}
	require.True(t, limiter.IsBanned(peerID))

	// Reset
	limiter.Reset(peerID)
	require.False(t, limiter.IsBanned(peerID))

	// Should allow again
	require.True(t, limiter.Allow(peerID))
}

func TestRateLimiter_DoSMitigation(t *testing.T) {
	// Simulate a DoS attack scenario
	config := RateLimitConfig{
		VotesPerSecond: 100,
		BurstSize:      200,
		BanThreshold:   5,
		BanDuration:    time.Second,
	}
	limiter := NewPeerRateLimiter(config)

	attackerID, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	require.NoError(t, err)

	// Attacker floods with votes
	allowed := 0
	rejected := 0
	for i := 0; i < 1000; i++ {
		if limiter.Allow(attackerID) {
			allowed++
		} else {
			rejected++
		}
	}

	// Should have rate limited most requests
	require.Greater(t, rejected, 700, "Should reject most flooding requests")
	require.LessOrEqual(t, allowed, 300, "Should allow limited requests")

	// Attacker should be banned
	require.True(t, limiter.IsBanned(attackerID), "Attacker should be banned")

	// Even more requests should all be rejected
	for i := 0; i < 100; i++ {
		require.False(t, limiter.Allow(attackerID), "Banned peer should be rejected")
	}

	_, totalRejected, _ := limiter.GetStats(attackerID)
	require.Greater(t, totalRejected, int64(800), "Should have high rejection count")
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	config := RateLimitConfig{
		VotesPerSecond: 100,
		BurstSize:      200,
		BanThreshold:   5,
		BanDuration:    time.Minute,
	}
	limiter := NewPeerRateLimiter(config)

	peerID, _ := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(peerID)
	}
}

func BenchmarkRateLimiter_Parallel(b *testing.B) {
	config := RateLimitConfig{
		VotesPerSecond: 100,
		BurstSize:      200,
		BanThreshold:   5,
		BanDuration:    time.Minute,
	}
	limiter := NewPeerRateLimiter(config)

	peerIDs := make([]peer.ID, 10)
	for i := range peerIDs {
		peerIDs[i], _ = peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			limiter.Allow(peerIDs[i%len(peerIDs)])
			i++
		}
	})
}
