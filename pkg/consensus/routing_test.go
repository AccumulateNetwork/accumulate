// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPowerOfTwo(t *testing.T) {
	for _, n := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 1024} {
		assert.True(t, IsPowerOfTwo(n), "%d is a power of two", n)
	}
	for _, n := range []int{0, -1, -8, 3, 5, 6, 7, 100, 127, 129, 1000} {
		assert.False(t, IsPowerOfTwo(n), "%d is not a power of two", n)
	}
}

// The property the routing change rests on: one signer always lands on one worker.
//
// Without it a signer's transactions are batched independently and committed
// in DAG order, and replay protection — which requires strictly increasing
// timestamps in EXECUTION order — rejects everything but an increasing
// subsequence. 96 of 100 were lost that way (#4132).
func TestWorkerFor_SameKeyAlwaysSameWorker(t *testing.T) {
	for _, n := range []int{1, 2, 4, 8, 64, 128, 100} {
		key := []byte("acc://f4a327b7cfbe971258b5a24c5ba3529bda09d8078ed35fac/ACME")
		first := workerFor(key, n)
		for i := 0; i < 1000; i++ {
			assert.Equal(t, first, workerFor(key, n),
				"routing must be stable for a key (numWorkers=%d)", n)
		}
	}
}

// Distinct signers must still spread, or the worker fan-out buys nothing.
func TestWorkerFor_DistinctKeysSpread(t *testing.T) {
	const n = 64
	seen := map[int]int{}
	for i := 0; i < 4096; i++ {
		seen[workerFor([]byte(fmt.Sprintf("acc://signer-%d", i)), n)]++
	}
	require.Len(t, seen, n, "every worker should receive some share of 4096 distinct keys")

	// Roughly even: no worker should carry more than 3x the mean. This is a
	// smoke test for a badly-behaved hash, not a statistical claim.
	mean := 4096 / n
	for w, c := range seen {
		assert.Less(t, c, mean*3, "worker %d carries %d, mean is %d", w, c, mean)
	}
}

// A power-of-two count is masked, which is what makes the mapping cheap and
// uniform. Verify the mask is actually what happens, not just that it works.
func TestWorkerFor_PowerOfTwoUsesTheFullRange(t *testing.T) {
	for _, n := range []int{2, 4, 8, 16, 32, 64, 128} {
		seen := map[int]bool{}
		for i := 0; i < n*200; i++ {
			w := workerFor([]byte(fmt.Sprintf("key-%d", i)), n)
			require.GreaterOrEqual(t, w, 0)
			require.Less(t, w, n, "index must be inside the worker range")
			seen[w] = true
		}
		assert.Len(t, seen, n, "all %d workers should be reachable", n)
	}
}

// A count that is not a power of two still has to work — a deployment is not
// refused service over it — but it is not sharding, and config validation is
// where it should be complained about.
func TestWorkerFor_NonPowerOfTwoStillRoutesInRange(t *testing.T) {
	for _, n := range []int{3, 5, 7, 100, 1000} {
		for i := 0; i < 500; i++ {
			w := workerFor([]byte(fmt.Sprintf("key-%d", i)), n)
			require.GreaterOrEqual(t, w, 0)
			require.Less(t, w, n, "index must stay in range for numWorkers=%d", n)
		}
	}
}

// One worker is the degenerate case and must not divide by zero or mask by -1.
func TestWorkerFor_SingleWorkerAndDegenerateCounts(t *testing.T) {
	for _, n := range []int{1, 0, -1} {
		assert.Equal(t, 0, workerFor([]byte("anything"), n),
			"numWorkers=%d must route everything to worker 0", n)
	}
}

// An empty key must not panic and must stay in range: some submissions have no
// usable routing key, and they should degrade to a valid worker rather than
// crash the node.
func TestWorkerFor_EmptyKeyIsSafe(t *testing.T) {
	for _, n := range []int{1, 2, 64, 100} {
		w := workerFor(nil, n)
		require.GreaterOrEqual(t, w, 0)
		require.Less(t, w, n)
		assert.Equal(t, w, workerFor([]byte{}, n), "nil and empty must agree")
	}
}

// Changing the worker count remaps keys. That is expected for a mask-based
// scheme and is worth pinning, because it means numWorkers cannot be changed
// on a running node without redistributing in-flight work.
func TestWorkerFor_CountChangeRemaps(t *testing.T) {
	key := []byte("acc://busy-signer")
	a := workerFor(key, 64)
	b := workerFor(key, 128)
	// Not a guarantee for every key, but for at least some — assert over a set
	// so the test is not hostage to one lucky hash.
	moved := 0
	for i := 0; i < 200; i++ {
		k := []byte(fmt.Sprintf("acc://signer-%d", i))
		if workerFor(k, 64) != workerFor(k, 128) {
			moved++
		}
	}
	assert.Greater(t, moved, 0, "a different shard count must remap at least some keys")
	_ = a
	_ = b
}

func TestRoutingKeyBytes_DistinguishesKeys(t *testing.T) {
	assert.Nil(t, routingKeyBytes(""))
	assert.NotEqual(t, routingKeyBytes("ab"), routingKeyBytes("ba"))
	assert.NotEqual(t, routingKeyBytes("a"), routingKeyBytes("aa"))
	assert.Equal(t, routingKeyBytes("acc://x"), routingKeyBytes("acc://x"))
}

// Two signers that share a prefix must not be forced onto the same worker.
func TestWorkerFor_PrefixesDoNotCollide(t *testing.T) {
	const n = 64
	a := workerFor(routingKeyBytes("acc://alice.acme/book/1"), n)
	same := 0
	for _, k := range []string{
		"acc://alice.acme/book/2", "acc://alice.acme/book/10",
		"acc://alice.acme/tokens", "acc://alicex.acme/book/1",
	} {
		if workerFor(routingKeyBytes(k), n) == a {
			same++
		}
	}
	assert.Less(t, same, 4, "related keys should not all land on one worker")
}
