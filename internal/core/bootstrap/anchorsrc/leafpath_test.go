// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func entryHash(tag string, i int) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(i))
	h := sha256.Sum256(append([]byte(tag), b[:]...))
	return h[:]
}

// The shape this package reads a receipt by is the shape the chain builds:
// for every entry of every chain up to 70 entries, the chain's own receipt has
// the pattern leafPattern says, and leafOf reads the entry's index back.
func TestALeafsReceiptHasTheShapeItsIndexAndTheHeightDecide(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	chain, err := batch.Account(url.MustParse("acc://shape.acme")).MainChain().Get()
	require.NoError(t, err)

	const max = 70
	for h := 1; h <= max; h++ {
		require.NoError(t, chain.AddEntry(entryHash("shape", h), false))
		for j := 0; j < h; j++ {
			r, err := chain.Receipt(int64(j), int64(h-1))
			require.NoError(t, err)

			want, ok := leafPattern(uint64(j), uint64(h))
			require.True(t, ok)
			require.Len(t, r.Entries, len(want), "entry %d of %d", j, h)
			for i, e := range r.Entries {
				require.Equal(t, want[i], e.Right, "entry %d of %d, step %d", j, h, i)
			}

			got, ok := leafOf(r.Entries, uint64(h))
			require.True(t, ok, "entry %d of %d", j, h)
			require.Equal(t, uint64(j), got)
		}
	}
}

// Only one step of a receipt is where it enters the chain as an entry: no
// interior node's path, and no path with steps of another chain in front of
// it, has the shape of a leaf.
func TestOnlyOneStepOfAReceiptIsALeafOfTheChain(t *testing.T) {
	side := entryHash("side", 0)
	for h := uint64(1); h <= 70; h++ {
		for j := uint64(0); j < h; j++ {
			leaf, _ := leafPattern(j, h)
			for front := 0; front < 16; front++ { // Every 0..3 step prefix
				n := 0
				for f := front; f > 1; f >>= 1 {
					n++
				}
				var r merkle.Receipt
				r.Start = side
				for i := 0; i < n; i++ {
					r.Entries = append(r.Entries, &merkle.ReceiptEntry{Hash: side, Right: front>>i&1 == 1})
				}
				for _, right := range leaf {
					r.Entries = append(r.Entries, &merkle.ReceiptEntry{Hash: side, Right: right})
				}

				step, _, index, ok := splitAtLeaf(&r, h)
				require.True(t, ok, "entry %d of %d behind %d steps", j, h, n)
				require.Equal(t, n, step, "entry %d of %d behind %d steps", j, h, n)
				require.Equal(t, j, index)
			}
		}
	}
}
