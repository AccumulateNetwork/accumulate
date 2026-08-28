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

// ---------------------------------------------------------------------------
// The real rule, as now implemented by KeyEntry.CanUseTimestamp/UseTimestamp
// (keys.go). Everything above tests the OLD strict rule as a model, kept to
// document what #4132 was; everything below tests the shipped code.
// ---------------------------------------------------------------------------

// A full burst of ReplayWindowSize transactions survives arbitrary
// reordering. This is the property the strict rule lacked: the production
// loadgen's 100-transaction treasury burst kept 4.
func TestKeyEntry_ShuffledBurstFullySurvives(t *testing.T) {
	for _, seed := range []int64{1, 7, 42, 1234, 99999} {
		ts := make([]uint64, 100)
		for i := range ts {
			ts[i] = uint64(i + 1)
		}
		rand.New(rand.NewSource(seed)).Shuffle(len(ts), func(i, j int) { ts[i], ts[j] = ts[j], ts[i] })

		li := new(LiteIdentity)
		for _, v := range ts {
			require.NoError(t, li.CanUseTimestamp(v), "seed %d: timestamp %d must be accepted", seed, v)
			li.UseTimestamp(v)
		}
		assert.Equal(t, uint64(100), li.LastUsedOn, "LastUsedOn is the highest spent timestamp")
	}
}

// Every spent timestamp is a replay, whether it is the watermark or deep in
// the retained window — this is the property replay protection exists for,
// and the window must not weaken it.
func TestKeyEntry_EverySpentTimestampIsRejected(t *testing.T) {
	li := new(LiteIdentity)
	spent := []uint64{50, 10, 30, 20, 40} // deliberately out of order
	for _, v := range spent {
		require.NoError(t, li.CanUseTimestamp(v))
		li.UseTimestamp(v)
	}
	for _, v := range spent {
		assert.Error(t, li.CanUseTimestamp(v), "spent timestamp %d must be rejected", v)
	}
	assert.NoError(t, li.CanUseTimestamp(25), "an unspent timestamp inside the window is fine")
	assert.NoError(t, li.CanUseTimestamp(60), "a new high timestamp is fine")
}

// Once the window is full, anything below the oldest retained entry is
// rejected — the entry can no longer prove it is not a replay.
func TestKeyEntry_WindowFloorRejectsTheTooOld(t *testing.T) {
	li := new(LiteIdentity)
	// Spend 1000 timestamps in order; the window retains the last
	// ReplayWindowSize of them.
	for ts := uint64(1); ts <= 1000; ts++ {
		require.NoError(t, li.CanUseTimestamp(ts))
		li.UseTimestamp(ts)
	}
	assert.Equal(t, uint64(1000), li.LastUsedOn)
	assert.Len(t, li.PriorUsedOn, ReplayWindowSize-1, "retention is bounded")
	floor := li.PriorUsedOn[0]
	assert.Equal(t, uint64(1000-ReplayWindowSize+1), floor)

	assert.Error(t, li.CanUseTimestamp(floor-1), "below the floor is unverifiable, rejected")
	assert.Error(t, li.CanUseTimestamp(floor), "the floor itself was spent")
	assert.NoError(t, li.CanUseTimestamp(1001), "the future is always open")
}

// KeySpec (key page entries) enforces the same rule as LiteIdentity.
func TestKeyEntry_KeySpecUsesTheSameRule(t *testing.T) {
	ks := new(KeySpec)
	require.NoError(t, ks.CanUseTimestamp(10))
	ks.UseTimestamp(10)
	require.NoError(t, ks.CanUseTimestamp(5), "out of order but unspent is accepted")
	ks.UseTimestamp(5)
	assert.Error(t, ks.CanUseTimestamp(10), "replay rejected")
	assert.Error(t, ks.CanUseTimestamp(5), "replay rejected")
	assert.Equal(t, uint64(10), ks.LastUsedOn)
	assert.Equal(t, []uint64{5}, ks.PriorUsedOn)
}

// Timestamp zero still opts out entirely: accepted, and no state is touched.
func TestKeyEntry_ZeroTimestampOptsOut(t *testing.T) {
	li := new(LiteIdentity)
	require.NoError(t, li.CanUseTimestamp(10))
	li.UseTimestamp(10)
	for i := 0; i < 3; i++ {
		assert.NoError(t, li.CanUseTimestamp(0))
		li.UseTimestamp(0)
	}
	assert.Equal(t, uint64(10), li.LastUsedOn)
	assert.Empty(t, li.PriorUsedOn, "exempt signatures leave no trace")
}

// UseTimestamp keeps PriorUsedOn sorted and deduplicated regardless of the
// order timestamps are spent in — the binary searches in CanUseTimestamp
// depend on it.
func TestKeyEntry_PriorUsedOnStaysSorted(t *testing.T) {
	li := new(LiteIdentity)
	for _, v := range []uint64{7, 3, 9, 1, 8, 2} {
		require.NoError(t, li.CanUseTimestamp(v))
		li.UseTimestamp(v)
	}
	assert.Equal(t, uint64(9), li.LastUsedOn)
	assert.Equal(t, []uint64{1, 2, 3, 7, 8}, li.PriorUsedOn)
}

// The property a fix has to provide, stated as a test: a bounded reordering
// window should be tolerated. This documented the requirement before the fix
// existed; the windowed rule is now implemented for real by
// KeyEntry.CanUseTimestamp/UseTimestamp and tested above.
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
