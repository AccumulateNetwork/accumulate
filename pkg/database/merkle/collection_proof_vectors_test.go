// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test vectors for collection proofs (#4105).
//
// These pin the properties that cross-partition delivery is built on, as
// opposed to the properties the primitive happens to have today. Emission
// sends one proof per package and recovery pulls a range against a root the
// destination already holds; both are only sound if the four claims below
// hold, so each is a separate vector and each has a matching negative case.
//
//	1. A proof over a span validates on its own, with no reference to any
//	   other span. This is what makes packages independent, and therefore
//	   what makes delivery order-independent.
//	2. The span binds absolute indices, so a recovered entry can be placed
//	   without trusting the sender's word about where it goes.
//	3. Order within the span is committed. Reordering must fail.
//	4. Membership is exact. Neither adding nor removing an element can keep
//	   the proof valid.
//
// The chain is deterministic — entry i is sha256("cp-vector" || i) — so the
// golden anchors below are stable and a change to the hashing or to the
// merkle construction shows up here rather than in a soak three weeks later.

// vectorChain builds the deterministic chain these vectors are taken from.
func vectorChain(t *testing.T, n int64) *Chain {
	t.Helper()
	c := testChain(begin(), 4, "cp-vectors")
	for i := int64(0); i < n; i++ {
		require.NoError(t, c.AddEntry(vectorEntry(i), false))
	}
	return c
}

func vectorEntry(i int64) []byte {
	h := sha256.Sum256([]byte(fmt.Sprintf("cp-vector%d", i)))
	return h[:]
}

// Golden anchors for the deterministic chain. If these change, the merkle
// construction changed, and every proof already written to a database is
// affected — which is exactly the kind of change that must not pass silently.
const (
	goldenAnchor16 = "6fc032112054852357b838673ed0004bc0a17c888a5d045255cbbc39380929ec"
	goldenEntry0   = "1849b26bf04dd1294f27f9a3e59425dbb37ac05fb160a50866bea6cb31fdfb05"
)

// TestVectorSpanIsSelfContained is claim 1: a proof over a sub-span validates
// with nothing but itself. Packages are built this way so that losing one does
// not block the next, which is the property per-message receipts do not have
// and the reason #4103 wedged on a single missing message.
func TestVectorSpanIsSelfContained(t *testing.T) {
	c := vectorChain(t, 64)

	for _, span := range [][2]int64{{0, 0}, {0, 7}, {8, 15}, {20, 20}, {33, 47}} {
		t.Run(fmt.Sprintf("span_%d_%d", span[0], span[1]), func(t *testing.T) {
			rl, err := GetReceiptList(c, span[0], span[1])
			require.NoError(t, err)
			require.NotNil(t, rl)

			require.True(t, rl.Validate(nil),
				"a proof over [%d,%d] must validate on its own", span[0], span[1])
			require.Len(t, rl.Elements, int(span[1]-span[0]+1),
				"the proof must cover exactly its own span, no more and no less")

			// Every element of the span is provably in it, and nothing else is.
			for i := span[0]; i <= span[1]; i++ {
				require.True(t, rl.Included(vectorEntry(i)),
					"entry %d is inside the span and must be included", i)
			}
			require.False(t, rl.Included(vectorEntry(span[1]+1)),
				"an entry outside the span must not be included")
		})
	}
}

// TestVectorBindsAbsoluteIndex is claim 2, as far as it currently holds. This
// branch documents that a ReceiptList proves absolute indices because
// MerkleState is counted. GetReceiptList does report the right value, but it
// is not cryptographically bound (#4106), so recovery cannot yet rely on it
// to place a recovered entry without trusting the sender.
func TestVectorBindsAbsoluteIndex(t *testing.T) {
	c := vectorChain(t, 64)

	for _, start := range []int64{0, 1, 8, 33} {
		rl, err := GetReceiptList(c, start, start+3)
		require.NoError(t, err)
		require.True(t, rl.Validate(nil))

		// The value is correct as produced by GetReceiptList. It is not
		// cryptographically bound — see #4106 and the vector below.
		require.Equal(t, start, rl.MerkleState.Count,
			"GetReceiptList must report the span's absolute start index")

		for j, e := range rl.Elements {
			require.Equal(t, vectorEntry(start+int64(j)), e,
				"element %d of the span must be the entry at absolute index %d", j, start+int64(j))
		}
	}
}

// TestVectorRejectsReordering is claim 3, negative. Order is committed, so a
// proof whose elements have been permuted must not validate — otherwise a
// sender could reorder a package's contents undetected.
func TestVectorRejectsReordering(t *testing.T) {
	c := vectorChain(t, 64)
	rl, err := GetReceiptList(c, 8, 15)
	require.NoError(t, err)
	require.True(t, rl.Validate(nil), "control: the untampered proof validates")

	swapped := rl.Copy()
	swapped.Elements[2], swapped.Elements[5] = swapped.Elements[5], swapped.Elements[2]
	require.False(t, swapped.Validate(nil),
		"swapping two elements must invalidate the proof")

	reversed := rl.Copy()
	for i, j := 0, len(reversed.Elements)-1; i < j; i, j = i+1, j-1 {
		reversed.Elements[i], reversed.Elements[j] = reversed.Elements[j], reversed.Elements[i]
	}
	require.False(t, reversed.Validate(nil),
		"reversing the span must invalidate the proof")
}

// TestVectorRejectsMembershipChange is claim 4, negative. Neither substituting,
// adding nor removing an element may keep the proof valid.
func TestVectorRejectsMembershipChange(t *testing.T) {
	c := vectorChain(t, 64)

	base := func(t *testing.T) *ReceiptList {
		rl, err := GetReceiptList(c, 8, 15)
		require.NoError(t, err)
		require.True(t, rl.Validate(nil))
		return rl
	}

	t.Run("substituted", func(t *testing.T) {
		rl := base(t)
		rl.Elements[3] = vectorEntry(999)
		require.False(t, rl.Validate(nil), "substituting an element must fail")
	})

	t.Run("appended", func(t *testing.T) {
		rl := base(t)
		rl.Elements = append(rl.Elements, vectorEntry(16))
		require.False(t, rl.Validate(nil),
			"appending even a genuine next entry must fail: the span is what was proven")
	})

	t.Run("truncated", func(t *testing.T) {
		rl := base(t)
		rl.Elements = rl.Elements[:len(rl.Elements)-1]
		require.False(t, rl.Validate(nil), "dropping the last element must fail")
	})

	t.Run("emptied", func(t *testing.T) {
		rl := base(t)
		rl.Elements = nil
		require.False(t, rl.Validate(nil), "an empty element list must fail")
	})

	// KNOWN GAP (#4106). This branch documents that a ReceiptList "binds
	// element j to absolute index State.Count + j", and #4048 builds its
	// recovery model on that. It does not hold: State.Anchor() derives the
	// anchor from Pending alone and uses Count only as a zero check, so
	// Validate accepts any Count at all.
	//
	// The vector asserts the CURRENT behaviour rather than the documented
	// one, so it records the gap instead of hiding it. When #4106 is fixed,
	// this flips to require.False and the claim becomes real.
	t.Run("wrongStartCount_notYetBound", func(t *testing.T) {
		rl := base(t)
		rl.MerkleState.Count++
		require.True(t, rl.Validate(nil),
			"documents #4106: Count is not bound by the proof, so a false "+
				"absolute offset is currently accepted")
	})
}

// TestVectorGoldenAnchor pins the actual bytes. The other vectors would still
// pass if the merkle construction changed in a self-consistent way; this one
// would not. Proofs already written to a database are validated against these
// bytes, so a change here is a wire break, not a refactor.
func TestVectorGoldenAnchor(t *testing.T) {
	c := vectorChain(t, 16)

	head, err := c.Head().Get()
	require.NoError(t, err)
	anchor := hex.EncodeToString(head.Anchor())

	require.Equal(t, goldenEntry0, hex.EncodeToString(vectorEntry(0)),
		"the vector entries themselves must not drift")
	require.Equal(t, goldenAnchor16, anchor,
		"the anchor over 16 deterministic entries must not drift")
}
