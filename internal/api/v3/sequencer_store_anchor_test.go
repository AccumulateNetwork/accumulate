// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A source whose cache is empty -- a validator that restarted -- answers an
// anchor it produced from its store: the anchor from its anchor-sequence
// chain, its own signature, and the quorum its own executed copy holds
// (healing spec, "An anchor the cache does not hold is answered from the
// store"; #4517). Before this, a restart turned every anchor it had produced
// into "not in the cache", or "not produced yet" when the cache held none.
func TestSequencer_AnchorNotInTheCacheIsAnsweredFromTheStore(t *testing.T) {
	dn := protocol.DnUrl()
	bvn := protocol.PartitionUrl("BVN1")
	db := database.OpenInMemory(nil)

	// Twelve Directory validators; this node is the first.
	var keys []ed25519.PrivateKey
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{Version: 1}
	for i := 0; i < 12; i++ {
		_, k, _ := ed25519.GenerateKey(nil)
		keys = append(keys, k)
		globals.Network.AddValidator(k.Public().(ed25519.PublicKey), protocol.Directory, true)
	}

	// The Directory produced anchors 1 and 2 and executed its own copies with
	// a quorum of eight signatures each, as the executor records them.
	pool := dn.JoinPath(protocol.AnchorPool)
	var produced []*protocol.Transaction
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		for n := uint64(1); n <= 2; n++ {
			txn := new(protocol.Transaction)
			txn.Body = &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: dn, MinorBlockIndex: 10 + n}}
			produced = append(produced, txn)
			require.NoError(t, batch.Message(txn.ID().Hash()).Main().Put(&messaging.TransactionMessage{Transaction: txn}))
			require.NoError(t, batch.Account(pool).AnchorSequenceChain().Inner().AddEntry(txn.GetHash(), false))

			own := new(protocol.Transaction)
			own.Header.Principal = pool
			own.Body = txn.Body
			for _, k := range keys[:8] {
				sig := &protocol.ED25519Signature{PublicKey: k.Public().(ed25519.PublicKey), Signer: dn.JoinPath(protocol.Network), TransactionHash: own.ID().Hash()}
				require.NoError(t, batch.Account(pool).Transaction(own.ID().Hash()).ValidatorSignatures().Add(sig))
			}
		}
		return nil
	}))

	svc := NewSequencer(SequencerParams{
		Database:     db,
		EventBus:     events.NewBus(nil),
		Globals:      globals,
		Partition:    protocol.Directory,
		ValidatorKey: keys[0],
		Cache:        synthcache.New(0), // restarted: holds nothing
	})
	g := svc.globals.Load().(*core.GlobalValues)

	rs, err := svc.getAnchorRangeFromCache(g, bvn, 1, 2)
	require.NoError(t, err, "an anchor the source produced is answered from its store")
	require.Len(t, rs, 2)
	for i, r := range rs {
		require.Equal(t, uint64(i+1), r.Sequence.Number)
		require.True(t, bvn.Equal(r.Sequence.Destination))
		require.True(t, bvn.JoinPath(protocol.AnchorPool).Equal(r.Message.(*messaging.TransactionMessage).Transaction.Header.Principal))
		require.Equal(t, produced[i].Body.(*protocol.DirectoryAnchor).MinorBlockIndex,
			r.Message.(*messaging.TransactionMessage).Transaction.Body.(*protocol.DirectoryAnchor).MinorBlockIndex)
		require.GreaterOrEqual(t, int(r.Signatures.Total), 8, "anchor %d carries the held quorum", i+1)
	}

	// One past what it produced is on its way, not missing.
	_, err = svc.getAnchorRangeFromCache(g, bvn, 3, 3)
	require.ErrorIs(t, err, errors.NotReady)
}
