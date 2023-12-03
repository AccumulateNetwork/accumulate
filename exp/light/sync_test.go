// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestPartialIndexReceivedAnchors(t *testing.T) {
	entries := make([]*protocol.Transaction, 256)
	for i := range entries {
		txn, err := build.Transaction().For("foo").
			Metadata([]byte{byte(i)}).
			Body(&protocol.BlockValidatorAnchor{}).
			Done()
		require.NoError(t, err)
		entries[i] = txn
	}

	type Case struct {
		Txns    [2]int
		Indexed [2]int
	}
	cases := []Case{}
	ranges := [][2]int{
		{0, 50},
		{50, 100},
		{100, 150},
		{200, 256},
		{0, 256},
	}
	for _, txns := range ranges {
		for _, indexed := range ranges {
			cases = append(cases, Case{txns, indexed})
		}
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%d-%d x %d-%d", c.Txns[0], c.Txns[1], c.Indexed[0], c.Indexed[1]), func(t *testing.T) {
			part := protocol.PartitionUrl("foo")
			db := memory.New(nil)

			// Populate the chain
			batch := OpenDB(db, nil, true)
			defer batch.Discard()
			chain := batch.Account(part.JoinPath(protocol.AnchorPool)).MainChain()
			for _, txn := range entries {
				require.NoError(t, chain.Inner().AddHash(txn.GetHash(), false))
			}

			// Populate transactions
			for i := c.Txns[0]; i < c.Txns[1]; i++ {
				txn := entries[i]
				require.NoError(t, batch.
					Message(txn.Hash()).Main().
					Put(&messaging.TransactionMessage{
						Transaction: txn,
					}))
			}

			// Populate the anchor index
			for i := c.Indexed[0]; i < c.Indexed[1]; i++ {
				txn := entries[i]
				require.NoError(t, batch.
					Index().Partition(part).
					Anchors().Received().
					Add(&AnchorMetadata{
						Index:       uint64(i),
						Hash:        txn.Hash(),
						Transaction: txn,
						Anchor:      txn.Body.(protocol.AnchorBody).GetPartitionAnchor(),
					}))
			}

			require.NoError(t, batch.Commit())

			// Index received anchors
			require.NoError(t, (&Client{store: db}).IndexReceivedAnchors(context.Background(), part))

			// Verify the index
			batch = OpenDB(db, nil, true)
			defer batch.Discard()
			values, err := batch.Index().Partition(part).Anchors().Received().Get()
			require.NoError(t, err)

			indices := map[uint64]bool{}
			for _, v := range values {
				indices[v.Index] = true
			}

			for i := c.Txns[0]; i < c.Txns[1]; i++ {
				require.Truef(t, indices[uint64(i)], "expected transaction %d", i)
			}
			for i := c.Indexed[0]; i < c.Indexed[1]; i++ {
				require.Truef(t, indices[uint64(i)], "expected indexed %d", i)
			}
		})
	}
}
