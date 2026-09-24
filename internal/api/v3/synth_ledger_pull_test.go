// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestASyntheticLedgerWithQueuedLocalDeliveriesCanBePulled is the live
// failure of run 20260924T052134Z: a restarted BVN1 validator re-syncing
// logged "acc://bvn-BVN1.acme/synthetic: the state served does not hash into
// the anchored root" on every pass for the rest of the run.
//
// The synthetic ledger's BPT leaf commits to its LocalDeliveryQueue
// (internal/database/observer_prod.go, hashSecondaryState, #4155). Under load
// every block queues same-partition deliveries for the next block
// (exec_local_queue.go, splitLocalDeliveries), so at any block a peer serves
// the queue is not empty. A current-state account query serves the body, the
// directory, the pending list and a receipt — and not the queue — and the pull
// (pull.Fetch) writes the body, directory, pending and chain heads — and not
// the queue. The account a joining node assembles therefore carries whatever
// queue its own store held, and hashes into no root any peer served.
//
// Idle, the queue is empty on both sides and the account verifies by accident,
// which is why every simulator restart test passes.
//
// It runs the real querier and the real pull: nothing here is a stub.
func TestASyntheticLedgerWithQueuedLocalDeliveriesCanBePulled(t *testing.T) {
	const partitionID = "SynthPull"
	part := protocol.PartitionUrl(partitionID)
	synth := part.JoinPath(protocol.Synthetic)
	sysLedger := part.JoinPath(protocol.Ledger)
	other := protocol.PartitionUrl("Other")

	// The queued delivery is a real message, stored as splitLocalDeliveries
	// stores it before queueing it: the queue names destination@hash, and the
	// drain at the next block executes the stored message (#4399).
	deposit := &messaging.TransactionMessage{Transaction: &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("alice", "tokens")},
		Body:   &protocol.SyntheticDepositTokens{Token: protocol.AcmeUrl(), Amount: *big.NewInt(1)},
	}}
	queued := protocol.AccountUrl("alice", "tokens").WithTxID(deposit.Hash())
	stale := protocol.AccountUrl("bob", "tokens").WithTxID([32]byte{9})

	for _, c := range []struct {
		name   string
		peer   []*url.TxID
		joiner []*url.TxID
	}{
		{name: "idle: both queues empty (control)"},
		{name: "loaded: the peer has a delivery queued", peer: []*url.TxID{queued}},
		{name: "restart: the joiner holds its own stale queue", joiner: []*url.TxID{stale}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()

			// The peer: a synthetic ledger that has produced into a stream,
			// with c.peer queued for local delivery at its next block.
			peerDB := database.OpenInMemory(nil)
			peerDB.SetObserver(database.NewDatabaseObserver())
			batch := peerDB.Begin(true)
			sl := new(protocol.SyntheticLedger)
			sl.Url = synth
			sl.Partition(other).Produced = 5
			require.NoError(t, batch.Account(synth).Main().Put(sl))
			for _, id := range c.peer {
				require.NoError(t, batch.Account(synth).LocalDeliveryQueue().Add(id))
				require.NoError(t, batch.Message(id.Hash()).Main().Put(deposit))
			}
			ledger := new(protocol.SystemLedger)
			ledger.Url = sysLedger
			ledger.Index = 204
			require.NoError(t, batch.Account(sysLedger).Main().Put(ledger))
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())

			// A receipt for an account's state is reported at the block the
			// root index chain ends at.
			batch = peerDB.Begin(true)
			data, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
			require.NoError(t, err)
			root, err := batch.Account(sysLedger).RootChain().Index().Get()
			require.NoError(t, err)
			require.NoError(t, root.AddEntry(data, false))
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())

			batch = peerDB.Begin(false)
			peerRoot, err := batch.GetBptRootHash()
			require.NoError(t, err)
			batch.Discard()

			// The joining node: its own synthetic ledger as its last block
			// left it.
			joinDB := database.OpenInMemory(nil)
			joinDB.SetObserver(database.NewDatabaseObserver())
			jb := joinDB.Begin(true)
			defer jb.Discard()
			for _, id := range c.joiner {
				require.NoError(t, jb.Account(synth).LocalDeliveryQueue().Add(id))
			}

			src := api.Querier2{Querier: NewQuerier(QuerierParams{Database: peerDB, Partition: partitionID})}
			p, err := pull.Fetch(ctx, src, jb, synth, pull.Options{Mode: pull.ModeStateOnly, Partition: part}, true)
			require.NoError(t, err)
			require.Equal(t, peerRoot, p.Root(), "the receipt must end at the peer's root")

			// The peer's root is a true root and the peer served honestly, so
			// the account must settle against it.
			require.NoError(t, p.Settle(peerRoot),
				"an honest peer's synthetic ledger does not verify against the peer's own root")

			// And the joined node can drain what it pulled at its next block:
			// every queued delivery's message is held, and it is the message
			// the queue names (block.drainDeliveryQueues loads it by hash).
			got, err := jb.Account(synth).LocalDeliveryQueue().Get()
			require.NoError(t, err)
			require.Len(t, got, len(c.peer), "the joiner's queue is not the peer's")
			for _, id := range got {
				msg, err := jb.Message(id.Hash()).Main().Get()
				require.NoError(t, err, "the queued delivery %v was pulled without its message", id)
				require.Equal(t, id.Hash(), msg.Hash())
			}
		})
	}
}
