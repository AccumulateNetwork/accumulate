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

// #4263: the cascade computes every intermediate a proof needs, so the proof
// must read them rather than rebuild the Merkle state that held them.
//
// The stored pair must be exactly what the rebuild would have produced —
// otherwise a proof taken from storage would differ from one taken by
// computation, which is a consensus fault, not a performance question. This
// asserts equality across a chain, and that the receipts agree.
func TestIntermediate_StoredPairMatchesTheRebuild(t *testing.T) {
	const n = 1024
	c := testChain(begin(), 8, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, c.AddEntry(d[:], false))
	}

	var checked, stored int
	for element := int64(1); element < n; element++ {
		for height := int64(1); height < 16; height++ {
			// what the rebuild produces
			hash, err := c.Entry(element)
			if err != nil {
				continue
			}
			s, err := c.StateAt(element - 1)
			if err != nil {
				continue
			}
			wantL, wantR, wantErr := getMerkleStateIntermediate(s, hash, height)

			// what is stored
			pair, err := c.Intermediate(uint64(element), uint64(height)).Get()
			if err != nil || len(pair) != 64 {
				// the cascade stopped before this height; the rebuild must
				// agree that there is nothing here
				require.Error(t, wantErr, "element %d height %d: nothing stored but the rebuild found a pair", element, height)
				continue
			}
			stored++
			require.NoError(t, wantErr, "element %d height %d: stored a pair the rebuild does not have", element, height)
			require.Equal(t, wantL, pair[:32], "element %d height %d: left", element, height)
			require.Equal(t, wantR, pair[32:], "element %d height %d: right", element, height)
			checked++
		}
	}
	require.NotZero(t, stored, "no intermediates were stored")
	t.Logf("%d stored intermediates over %d elements, every one equal to the rebuild", checked, n)
}

// And the receipt itself must be identical whether it was read or rebuilt.
func TestIntermediate_ReceiptIsTheSameEitherWay(t *testing.T) {
	const n = 512
	c := testChain(begin(), 8, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, c.AddEntry(d[:], false))
	}

	for _, start := range []int64{0, 1, 7, 100, 255, 300, n - 1} {
		fromStore, err := c.Receipt(start, n-1)
		require.NoError(t, err, "start %d", start)

		// force the rebuild by going through getMerkleStateIntermediate
		rebuilt := new(Receipt)
		rebuilt.StartIndex, rebuilt.EndIndex = start, n-1
		rebuilt.Start, err = c.Entry(start)
		require.NoError(t, err)
		rebuilt.End, err = c.Entry(n - 1)
		require.NoError(t, err)
		anchorState, err := c.StateAt(n - 1)
		require.NoError(t, err)
		anchorState.trim()
		err = rebuilt.build(func(element, height int64) ([]byte, []byte, error) {
			hash, err := c.Entry(element)
			if err != nil {
				return nil, nil, err
			}
			s, err := c.StateAt(element - 1)
			if err != nil {
				return nil, nil, err
			}
			return getMerkleStateIntermediate(s, hash, height)
		}, anchorState)
		require.NoError(t, err, "start %d", start)

		require.Equal(t, rebuilt.Anchor, fromStore.Anchor, "start %d: anchor", start)
		require.Equal(t, len(rebuilt.Entries), len(fromStore.Entries), "start %d: length", start)
		for i := range rebuilt.Entries {
			require.Equal(t, rebuilt.Entries[i].Hash, fromStore.Entries[i].Hash, "start %d entry %d", start, i)
			require.Equal(t, rebuilt.Entries[i].Right, fromStore.Entries[i].Right, "start %d entry %d side", start, i)
		}
		require.True(t, fromStore.Validate(nil), "start %d: the receipt must verify", start)
	}
}
