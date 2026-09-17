// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

// A segment held in memory builds byte-identical receipts and receipt lists
// to the stored chain, for every span inside it, whether or not the segment
// starts on a mark point. This is what lets the producer's cache prove its
// entries without reading a chain.
func TestSegment_MatchesTheChain(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, markPower := range []int64{2, 3} {
		for _, height := range []int64{1, 2, 5, 9, 16, 17, 33, 70} {
			t.Run(fmt.Sprintf("mark%d/height%d", markPower, height), func(t *testing.T) {
				store := begin()
				c := testChain(store, markPower)
				var rh common.RandHash
				for i := int64(0); i < height; i++ {
					require.NoError(t, c.AddEntry(rh.Next(), false))
				}

				for first := int64(0); first < height; first++ {
					seg, err := NewSegment(c, first)
					require.NoError(t, err)
					require.Equal(t, height-1, seg.Last())

					// Every span, or a sample of them for tall chains
					for trial := 0; trial < 40; trial++ {
						from := first + rng.Int63n(height-first)
						to := from + rng.Int63n(height-from)

						want, err := c.Receipt(from, to)
						require.NoError(t, err)
						got, err := seg.Receipt(from, to)
						require.NoError(t, err)
						require.True(t, want.Equal(got), "receipt %d..%d differs (first %d)", from, to, first)
						require.True(t, got.Validate(nil))

						wantList, err := GetReceiptList(c, from, to)
						require.NoError(t, err)
						gotList, err := seg.ReceiptList(from, to)
						require.NoError(t, err)
						require.True(t, wantList.MerkleState.Equal(gotList.MerkleState), "state before %d differs (first %d)", from, first)
						require.Equal(t, wantList.Elements, gotList.Elements)
						require.True(t, wantList.Receipt.Equal(gotList.Receipt), "list receipt %d..%d differs (first %d)", from, to, first)
						require.True(t, gotList.Validate(nil))
					}
				}
			})
		}
	}
}

// A segment grown by Append proves the same as one captured whole.
func TestSegment_Append(t *testing.T) {
	store := begin()
	c := testChain(store, 2)
	var rh common.RandHash
	for i := 0; i < 5; i++ {
		require.NoError(t, c.AddEntry(rh.Next(), false))
	}
	seg, err := NewSegment(c, 5)
	require.NoError(t, err)
	require.Empty(t, seg.Elements)
	for i := 0; i < 11; i++ {
		h := rh.Next()
		require.NoError(t, c.AddEntry(h, false))
		seg.Append(h)
	}
	whole, err := NewSegment(c, 5)
	require.NoError(t, err)
	require.Equal(t, whole.Elements, seg.Elements)
	want, err := c.Receipt(7, 15)
	require.NoError(t, err)
	got, err := seg.Receipt(7, 15)
	require.NoError(t, err)
	require.True(t, want.Equal(got))
}
