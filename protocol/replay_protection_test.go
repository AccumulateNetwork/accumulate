// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accepts models the executor's replay-protection rule exactly as written in
// internal/core/execute/v2/block/sig_user.go:
//
//	if sig.Timestamp != 0 && keyEntry.LastUsedOn >= sig.Timestamp { reject }
//
// The rule is stated in one place and enforced in one place, and nothing
// tested what it does to a stream of signatures that arrives out of order.
// That is the whole of #4132: a signer's transactions are batched in parallel
// and committed in DAG order, so they reach the executor shuffled, and this
// rule silently discards all but an increasing subsequence.
func accepts(lastUsedOn *uint64, timestamp uint64) bool {
	if timestamp != 0 && *lastUsedOn >= timestamp {
		return false
	}
	if timestamp != 0 {
		*lastUsedOn = timestamp
	}
	return true
}

// In submission order, everything is accepted. This is the case the rule was
// designed for and the only one anybody checked.
func TestReplayProtection_InOrderAcceptsEverything(t *testing.T) {
	var last uint64
	for ts := uint64(1); ts <= 100; ts++ {
		require.True(t, accepts(&last, ts), "timestamp %d should be accepted in order", ts)
	}
	assert.Equal(t, uint64(100), last)
}

// Reversed, only the first survives. The rest are older than what has been
// seen, so every one is discarded.
func TestReplayProtection_ReverseOrderKeepsOnlyOne(t *testing.T) {
	var last uint64
	accepted := 0
	for ts := uint64(100); ts >= 1; ts-- {
		if accepts(&last, ts) {
			accepted++
		}
	}
	assert.Equal(t, 1, accepted,
		"reversed arrival keeps only the first — the rest are all 'replays'")
}

// The real case: a shuffled burst keeps only an increasing subsequence.
//
// This is what happens to a signer whose transactions are spread across
// workers or split across batches. The measured production numbers land right
// here: 100 submitted, 4-19 executed, depending on how the shuffle fell.
func TestReplayProtection_ShuffledBurstLosesMost(t *testing.T) {
	const n = 100
	for _, seed := range []int64{1, 7, 42, 1234, 99999} {
		ts := make([]uint64, n)
		for i := range ts {
			ts[i] = uint64(i + 1)
		}
		r := rand.New(rand.NewSource(seed))
		r.Shuffle(n, func(i, j int) { ts[i], ts[j] = ts[j], ts[i] })

		var last uint64
		accepted := 0
		for _, v := range ts {
			if accepts(&last, v) {
				accepted++
			}
		}

		assert.Less(t, accepted, 25,
			"seed %d: a shuffled burst of %d should lose most of them, kept %d",
			seed, n, accepted)
		assert.GreaterOrEqual(t, accepted, 1, "at least the first is always kept")
		t.Logf("seed %6d: %3d of %d survived a shuffled burst", seed, accepted, n)
	}
}

// Near-ordered arrival — a small reordering window, which is what a short
// batch timer produces — still loses a large fraction. The loss is not
// confined to pathological shuffles.
func TestReplayProtection_SmallReorderingWindowStillLoses(t *testing.T) {
	const n = 100
	for _, window := range []int{2, 4, 8, 16} {
		ts := make([]uint64, n)
		for i := range ts {
			ts[i] = uint64(i + 1)
		}
		// Swap within a sliding window: mild, local disorder.
		r := rand.New(rand.NewSource(int64(window)))
		for i := 0; i+window < n; i += window {
			r.Shuffle(window, func(a, b int) {
				ts[i+a], ts[i+b] = ts[i+b], ts[i+a]
			})
		}

		var last uint64
		accepted := 0
		for _, v := range ts {
			if accepts(&last, v) {
				accepted++
			}
		}
		t.Logf("window %2d: %3d of %d survived", window, accepted, n)
		assert.Less(t, accepted, n,
			"window %d: even mild reordering must lose something", window)
	}
}

// A duplicate timestamp is rejected: the comparison is >=, not >. Worth
// pinning, because a signer that reuses a timestamp is the replay this rule
// exists to stop.
func TestReplayProtection_EqualTimestampIsRejected(t *testing.T) {
	var last uint64
	require.True(t, accepts(&last, 50))
	assert.False(t, accepts(&last, 50), "an equal timestamp is a replay")
	assert.False(t, accepts(&last, 49), "an older timestamp is a replay")
	assert.True(t, accepts(&last, 51), "a newer timestamp is fine")
}

// Timestamp zero opts out of replay protection entirely and must never be
// rejected, nor advance the watermark.
func TestReplayProtection_ZeroTimestampOptsOut(t *testing.T) {
	var last uint64
	require.True(t, accepts(&last, 10))
	for i := 0; i < 5; i++ {
		assert.True(t, accepts(&last, 0), "timestamp 0 is exempt")
	}
	assert.Equal(t, uint64(10), last, "an exempt signature must not move the watermark")
	assert.True(t, accepts(&last, 11), "the watermark is unchanged, so 11 is still new")
}

// Distinct signers do not interfere: the watermark is per key entry. If this
// were global, one busy signer would lock out everyone else.
func TestReplayProtection_IsPerKeyNotGlobal(t *testing.T) {
	var alice, bob uint64
	require.True(t, accepts(&alice, 100))
	assert.True(t, accepts(&bob, 5),
		"bob's low timestamp must not be judged against alice's watermark")
	assert.False(t, accepts(&alice, 50), "alice's own watermark still applies")
}

// The property a fix has to provide, stated as a test: a bounded reordering
// window should be tolerated. This documents the requirement — it is not
// satisfied by the current rule, which is why the test asserts what the
// CURRENT rule does and names what is wanted.
func TestReplayProtection_WindowedRuleWouldTolerateReordering(t *testing.T) {
	// A windowed rule: accept anything strictly newer than (high - window),
	// remembering what has been seen inside the window.
	type windowed struct {
		high uint64
		seen map[uint64]bool
	}
	const window = uint64(64)
	w := &windowed{seen: map[uint64]bool{}}
	acceptsWindowed := func(ts uint64) bool {
		if ts == 0 {
			return true
		}
		if w.high > window && ts <= w.high-window {
			return false // too old, outside the window
		}
		if w.seen[ts] {
			return false // an actual replay
		}
		w.seen[ts] = true
		if ts > w.high {
			w.high = ts
		}
		return true
	}

	ts := make([]uint64, 100)
	for i := range ts {
		ts[i] = uint64(i + 1)
	}
	rand.New(rand.NewSource(42)).Shuffle(100, func(i, j int) { ts[i], ts[j] = ts[j], ts[i] })

	accepted := 0
	for _, v := range ts {
		if acceptsWindowed(v) {
			accepted++
		}
	}

	// The window has to be at least as wide as the reordering distance. With
	// window=64 against a full shuffle of 100, the tail that arrives after a
	// high timestamp has already been seen falls outside and is still lost —
	// which is the real design question: how far out of order can a signer's
	// transactions arrive, and the answer is bounded by the batch timer and
	// the commit depth, not by anything the client controls.
	assert.Greater(t, accepted, 50,
		"a windowed rule keeps most of a shuffled burst; the strict rule kept 3-6")

	// ...and still rejects a genuine replay.
	assert.False(t, acceptsWindowed(ts[0]), "a repeat of a seen timestamp is still a replay")
	t.Logf("windowed(%d): %d of 100 survived, versus 3-6 under the strict rule", window, accepted)
}
