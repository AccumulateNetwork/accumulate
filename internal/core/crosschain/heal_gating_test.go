// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The conductor is the node's own healing path — internal/core/healing is only
// reachable from the debug CLI. Its breaker state transitions and heal gating
// had no tests: classifyRemoteError was covered, but the two functions it
// delegates to, and the interval gate that decides whether to heal at all,
// were not.

// shouldHeal rate-limits healing per destination. Without the gate, healing is
// scheduled from every block — dozens per second under DAG-BFT — and the
// re-drive load is what saturates a partition (#4056).
func TestShouldHeal_FirstCallAlwaysProceeds(t *testing.T) {
	c := &Conductor{HealInterval: time.Hour}
	assert.True(t, c.shouldHeal("BVN1"), "a destination never healed should proceed")
}

func TestShouldHeal_SecondCallInsideTheIntervalIsRefused(t *testing.T) {
	c := &Conductor{HealInterval: time.Hour}
	require.True(t, c.shouldHeal("BVN1"))
	for i := 0; i < 100; i++ {
		assert.False(t, c.shouldHeal("BVN1"),
			"inside the interval, repeat calls must be refused")
	}
}

// Destinations are gated independently: healing BVN1 must not suppress BVN2.
func TestShouldHeal_DestinationsAreIndependent(t *testing.T) {
	c := &Conductor{HealInterval: time.Hour}
	require.True(t, c.shouldHeal("BVN1"))
	assert.True(t, c.shouldHeal("BVN2"), "a different destination has its own clock")
	assert.True(t, c.shouldHeal("Directory"))
	assert.False(t, c.shouldHeal("BVN1"), "and BVN1 is still gated")
}

// Once the interval elapses, healing proceeds again.
func TestShouldHeal_ProceedsAgainAfterTheInterval(t *testing.T) {
	c := &Conductor{HealInterval: 20 * time.Millisecond}
	require.True(t, c.shouldHeal("BVN1"))
	require.False(t, c.shouldHeal("BVN1"))

	require.Eventually(t, func() bool { return c.shouldHeal("BVN1") },
		2*time.Second, 5*time.Millisecond,
		"after the interval the destination should be healable again")
}

// A zero or negative interval must fall back to the default rather than
// disabling the gate — an unset config should not turn healing into a flood.
func TestShouldHeal_ZeroIntervalUsesTheDefault(t *testing.T) {
	for _, iv := range []time.Duration{0, -time.Second} {
		c := &Conductor{HealInterval: iv}
		require.True(t, c.shouldHeal("BVN1"), "the first call proceeds")
		assert.False(t, c.shouldHeal("BVN1"),
			"interval %v must fall back to the default, not to 'always heal'", iv)
	}
}

func TestShouldHeal_IsSafeUnderConcurrency(t *testing.T) {
	c := &Conductor{HealInterval: time.Hour}
	var wg sync.WaitGroup
	var mu sync.Mutex
	proceeded := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.shouldHeal("BVN1") {
				mu.Lock()
				proceeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, proceeded,
		"exactly one caller may pass the gate; the rest must be refused")
}

// remoteFailed opens the circuit only after breakerThreshold CONSECUTIVE
// failures. One bad answer from an otherwise healthy remote must not stop
// healing to it.
func TestRemoteFailed_OpensOnlyAtTheThreshold(t *testing.T) {
	c := new(Conductor)
	for i := 1; i < breakerThreshold; i++ {
		c.remoteFailed("bvn1")
		assert.True(t, c.remoteAllowed("bvn1"),
			"%d failure(s) is below the threshold of %d", i, breakerThreshold)
	}
	c.remoteFailed("bvn1")
	assert.False(t, c.remoteAllowed("bvn1"),
		"the circuit should open at %d consecutive failures", breakerThreshold)
}

// remoteOK closes the circuit and clears the failure count, so a remote that
// recovers is usable immediately rather than serving out its backoff.
func TestRemoteOK_ClosesTheCircuitAndResetsTheCount(t *testing.T) {
	c := new(Conductor)
	for i := 0; i < breakerThreshold; i++ {
		c.remoteFailed("bvn1")
	}
	require.False(t, c.remoteAllowed("bvn1"), "precondition: the circuit is open")

	c.remoteOK("bvn1")
	assert.True(t, c.remoteAllowed("bvn1"), "a success must close the circuit at once")

	// And the count was reset, not merely the deadline: it should take a full
	// threshold of new failures to reopen.
	for i := 1; i < breakerThreshold; i++ {
		c.remoteFailed("bvn1")
		assert.True(t, c.remoteAllowed("bvn1"),
			"failure %d after a success must not reopen the circuit", i)
	}
}

// A success for an unknown remote must not create state or panic.
func TestRemoteOK_UnknownRemoteIsANoop(t *testing.T) {
	c := new(Conductor)
	assert.NotPanics(t, func() { c.remoteOK("never-seen") })
	assert.True(t, c.remoteAllowed("never-seen"))
}

// Remotes are tracked independently: one bad peer must not gate the others.
func TestBreaker_RemotesAreIndependent(t *testing.T) {
	c := new(Conductor)
	for i := 0; i < breakerThreshold; i++ {
		c.remoteFailed("bvn1")
	}
	assert.False(t, c.remoteAllowed("bvn1"))
	assert.True(t, c.remoteAllowed("bvn2"), "bvn2 must be unaffected by bvn1's failures")
	assert.True(t, c.remoteAllowed("directory"))
}

// Backoff grows with consecutive failures and is capped, so a remote that is
// down for a long time is retried on a bounded schedule rather than never.
func TestRemoteFailed_BackoffGrowsAndIsCapped(t *testing.T) {
	c := new(Conductor)
	for i := 0; i < breakerThreshold+40; i++ {
		c.remoteFailed("bvn1")
	}
	c.remoteMu.Lock()
	until := c.remotes["bvn1"].until
	c.remoteMu.Unlock()

	wait := time.Until(until)
	assert.Greater(t, wait, time.Duration(0), "the circuit is open")
	assert.LessOrEqual(t, wait, breakerMax,
		"backoff must be capped at breakerMax, not grow without bound")
}

// The breaker map is written from several healing goroutines at once.
func TestBreaker_IsSafeUnderConcurrency(t *testing.T) {
	c := new(Conductor)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				c.remoteFailed("bvn1")
			} else {
				c.remoteOK("bvn1")
			}
			_ = c.remoteAllowed("bvn1")
		}(i)
	}
	wg.Wait()
}
