// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// #4270: the 13 July 2025 restore discarded mark points, and every read of the
// surviving tail fails because StateAt replays from one. The state at the
// boundary is recoverable from the head's peaks — these tests hold that
// against ground truth, which a synthetic chain has and mainnet does not.

func buildTo(t testing.TB, n int, markPower int64) (*Chain, *State) {
	t.Helper()
	c := testChain(begin(), markPower, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, c.AddEntry(d[:], false))
	}
	head, err := c.Head().Get()
	require.NoError(t, err)
	return c, head
}

// The reconstruction must equal the mark point that was actually written, not
// merely produce a state that happens to verify.
func TestStateAtBoundary_EqualsTheRealMarkPoint(t *testing.T) {
	const markPower, markFreq = 8, 256
	for _, n := range []int{257, 300, 511, 512, 513, 700, 1000, 1023, 1024, 1025, 1500, 2000, 4097} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			c, head := buildTo(t, n, markPower)
			boundary := BoundaryFor(head.Count, markFreq)
			require.NotZero(t, boundary, "n=%d should have a closed mark set", n)

			got := StateAtBoundary(head, boundary)
			if !CanReconstruct(head.Count, boundary) {
				// The boundary's subtree was absorbed into a larger peak. Not
				// recoverable from the head, and the code must say so rather
				// than invent a state.
				require.Nil(t, got, "n=%d boundary=%d: must refuse, not guess", n, boundary)
				t.Skipf("n=%d: boundary %d absorbed into a larger peak (count %b)", n, boundary, n)
			}
			require.NotNil(t, got, "n=%d boundary=%d: reconstruction failed", n, boundary)

			// Ground truth: the mark point the chain actually stored
			want, err := c.States(boundary - 1).Get()
			require.NoError(t, err, "n=%d: no stored mark point at %d", n, boundary-1)

			require.Equal(t, want.Count, got.Count, "n=%d boundary=%d: count", n, boundary)
			require.Equal(t, want.Anchor(), got.Anchor(), "n=%d boundary=%d: anchor", n, boundary)
			for i := range want.Pending {
				if i < len(got.Pending) {
					require.Equal(t, want.Pending[i], got.Pending[i],
						"n=%d boundary=%d: peak %d", n, boundary, i)
				}
			}

			// And the check that is available on real data
			open, err := c.OpenSet(head)
			require.NoError(t, err)
			require.True(t, VerifyAgainstHead(got, head, open),
				"n=%d boundary=%d: replay does not reproduce the head", n, boundary)
		})
	}
}

// The verification must reject a wrong reconstruction — otherwise it proves
// nothing and a bad rebuild would be written to disk.
func TestVerifyAgainstHead_RejectsAWrongState(t *testing.T) {
	const markPower, markFreq = 8, 256
	c, head := buildTo(t, 441, markPower)
	boundary := BoundaryFor(head.Count, markFreq)
	open, err := c.OpenSet(head)
	require.NoError(t, err)

	good := StateAtBoundary(head, boundary)
	require.True(t, VerifyAgainstHead(good, head, open))

	// A peak corrupted by one bit
	bad := good.Copy()
	for i, p := range bad.Pending {
		if p != nil {
			q := copyHash(p)
			q[0] ^= 1
			bad.Pending[i] = q
			break
		}
		_ = i
	}
	require.False(t, VerifyAgainstHead(bad, head, open), "a corrupted peak must not verify")

	// The right peaks at the wrong count
	bad = good.Copy()
	bad.Count--
	require.False(t, VerifyAgainstHead(bad, head, open), "the wrong count must not verify")

	require.False(t, VerifyAgainstHead(nil, head, open))
}

// A chain that never closed a mark set has nothing to reconstruct, and must
// not pretend otherwise.
func TestStateAtBoundary_NothingToDoBelowTheFirstBoundary(t *testing.T) {
	_, head := buildTo(t, 9, 8)
	require.Zero(t, BoundaryFor(head.Count, 256))
	require.Nil(t, StateAtBoundary(head, 0))
	require.Nil(t, StateAtBoundary(head, 999), "a boundary past the head is not reconstructible")
}

// The same at other mark powers, since the boundary arithmetic depends on it.
func TestStateAtBoundary_OtherMarkPowers(t *testing.T) {
	for _, mp := range []int64{2, 4, 6, 10} {
		markFreq := int64(1) << uint(mp)
		n := int(markFreq*3 + markFreq/2) // three closed sets and a partial
		t.Run(fmt.Sprintf("markPower=%d", mp), func(t *testing.T) {
			c, head := buildTo(t, n, mp)
			boundary := BoundaryFor(head.Count, markFreq)
			got := StateAtBoundary(head, boundary)
			require.NotNil(t, got)
			want, err := c.States(boundary - 1).Get()
			require.NoError(t, err)
			require.Equal(t, want.Anchor(), got.Anchor())
			open, err := c.OpenSet(head)
			require.NoError(t, err)
			require.True(t, VerifyAgainstHead(got, head, open))
		})
	}
}

// How much of the damage this recovers. A chain is recoverable when its
// boundary is a submask of its count; it is not when the boundary's subtree
// has been absorbed into a larger peak.
func TestCanReconstruct_Coverage(t *testing.T) {
	const markFreq = 256
	var eligible, recoverable int
	var absorbed []int64
	for n := int64(1); n <= 200000; n++ {
		b := BoundaryFor(n, markFreq)
		if b == 0 {
			continue // never closed a mark set; nothing to recover
		}
		eligible++
		if CanReconstruct(n, b) {
			recoverable++
		} else if len(absorbed) < 8 {
			absorbed = append(absorbed, n)
		}
	}
	pct := float64(recoverable) * 100 / float64(eligible)
	t.Logf("counts 1..200000: %d have a closed mark set, %d recoverable (%.3f%%)",
		eligible, recoverable, pct)
	t.Logf("not recoverable, first few counts: %v", absorbed)

	require.Greater(t, pct, 99.0, "the method should recover the overwhelming majority")

	// The exceptions are exactly the counts whose boundary was absorbed, and
	// a count that is itself a power of two always loses its boundary.
	for _, n := range []int64{512, 1024, 2048, 4096} {
		require.False(t, CanReconstruct(n, BoundaryFor(n, markFreq)),
			"%d is a power of two; its boundary is absorbed", n)
	}
	require.True(t, CanReconstruct(441, BoundaryFor(441, markFreq)))
	require.True(t, CanReconstruct(1000, BoundaryFor(1000, markFreq)))
}
