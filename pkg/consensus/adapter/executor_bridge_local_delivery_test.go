// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4146 through the real ProduceBlock seam: a same-partition deposit is
// delivered through the local queue at the start of the NEXT block — no
// sequence number, no synthetic main chain position, no dispatch — and the
// drain runs before the block's own certificate.

// sendBetween builds a transfer from one lite account to another.
func sendBetween(t *testing.T, fromKey, toKey []byte, amount, timestamp uint64) *messaging.Envelope {
	t.Helper()
	from := protocol.LiteAuthorityForKey(fromKey[32:], protocol.SignatureTypeED25519)
	to := protocol.LiteAuthorityForKey(toKey[32:], protocol.SignatureTypeED25519)
	env, err := build.Transaction().For(from.JoinPath("ACME")).
		SendTokens(amount, 0).To(to.JoinPath("ACME")).
		SignWith(from).Version(1).Timestamp(timestamp).PrivateKey(fromKey).
		Done()
	require.NoError(t, err)
	return env
}

func synthState(t *testing.T, db *database.Database) (mainChainHeight int64, produced map[string]uint64) {
	t.Helper()
	produced = map[string]uint64{}
	require.NoError(t, db.View(func(batch *database.Batch) error {
		synth := batch.Account(protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic))
		chain, err := synth.MainChain().Get()
		if err != nil {
			return err
		}
		mainChainHeight = chain.Height()
		var ledger *protocol.SyntheticLedger
		err = synth.Main().GetAs(&ledger)
		if err != nil {
			return err
		}
		for _, p := range ledger.Sequence {
			produced[p.Url.ShortString()] = p.Produced
		}
		return nil
	}))
	return mainChainHeight, produced
}

// A same-partition deposit executes in the next block via the queue, taking
// no sequence number and no synthetic main chain position — so it consumes
// none of the collection-proof span every cross-partition proof is built
// over.
func TestBridge_LocalDeliveryTakesNoSequenceNumberAndNoChainPosition(t *testing.T) {
	alice := acctesting.GenerateKey("bridge", "alice")
	bob := acctesting.GenerateKey("bridge", "bob")
	r := newRealBridge(t, 0, alice, bob)

	heightBefore, producedBefore := synthState(t, r.db)
	bobBefore := liteBalance(t, r.db, bob)

	// Block N: the transfer executes and the deposit is queued.
	_, err := r.produce(t, types.NewBatch([][]byte{marshalEnv(t, sendBetween(t, alice, bob, 5, 1))}))
	require.NoError(t, err)
	require.Equal(t, bobBefore, liteBalance(t, r.db, bob), "the deposit waits for the next block")

	// Block N+1: an EMPTY certificate — the drain alone delivers it.
	_, err = r.produce(t)
	require.NoError(t, err)
	assert.Equal(t, bobBefore+5, liteBalance(t, r.db, bob), "the queue delivers at the start of the next block")

	heightAfter, producedAfter := synthState(t, r.db)
	assert.Equal(t, heightBefore, heightAfter, "a local delivery takes no synthetic main chain position")
	assert.Equal(t, producedBefore, producedAfter, "and no sequence number on any stream")
}

// The queue drains BEFORE the block's own certificate: a spend that depends
// on a queued deposit succeeds in the very block that delivers the deposit.
func TestBridge_QueueDrainsBeforeCertificateBatches(t *testing.T) {
	alice := acctesting.GenerateKey("bridge", "alice")
	bob := acctesting.GenerateKey("bridge", "bob")
	r := newRealBridge(t, 0, alice, bob)

	// Drain bob to exactly zero spendable margin: bob's spend below only
	// works if alice's deposit lands first.
	bobStart := liteBalance(t, r.db, bob)
	_, err := r.produce(t, types.NewBatch([][]byte{marshalEnv(t, transfer(t, bob, bobStart, 1))}))
	require.NoError(t, err)
	require.Zero(t, liteBalance(t, r.db, bob))

	// Block N: alice sends bob 7.
	_, err = r.produce(t, types.NewBatch([][]byte{marshalEnv(t, sendBetween(t, alice, bob, 7, 1))}))
	require.NoError(t, err)
	require.Zero(t, liteBalance(t, r.db, bob))

	// Block N+1: the certificate carries bob spending those exact 7 tokens.
	// If the certificate executed before the drain, the burn would fail on
	// insufficient balance.
	_, err = r.produce(t, types.NewBatch([][]byte{marshalEnv(t, transfer(t, bob, 7, 2))}))
	require.NoError(t, err)
	assert.Zero(t, liteBalance(t, r.db, bob),
		"the deposit must land before the certificate executes, and the burn must consume it")
}

// A locally produced synthetic that itself produces a local synthetic takes
// one block per hop: no cascade within a block, no termination bound. Here:
// block N executes a transfer to a nonexistent lite token account whose
// deposit (block N+1) fails and produces a refund, which lands in block N+2.
func TestBridge_ChainOfLocalSyntheticsTakesABlockPerHop(t *testing.T) {
	alice := acctesting.GenerateKey("bridge", "alice")
	r := newRealBridge(t, 0, alice)

	aliceBefore := liteBalance(t, r.db, alice)

	// Block N: the transfer executes, deposit queued. The destination is a
	// nonexistent ADI account — a lite address would be auto-created and the
	// deposit would succeed.
	from := protocol.LiteAuthorityForKey(alice[32:], protocol.SignatureTypeED25519)
	env, err := build.Transaction().For(from.JoinPath("ACME")).
		SendTokens(9, 0).To(protocol.AccountUrl("ghost", "tokens")).
		SignWith(from).Version(1).Timestamp(1).PrivateKey(alice).
		Done()
	require.NoError(t, err)
	_, err = r.produce(t, types.NewBatch([][]byte{marshalEnv(t, env)}))
	require.NoError(t, err)
	require.Equal(t, aliceBefore-9, liteBalance(t, r.db, alice))

	// Block N+1: the deposit fails (no ghost identity) and produces a refund.
	_, err = r.produce(t)
	require.NoError(t, err)
	require.Equal(t, aliceBefore-9, liteBalance(t, r.db, alice),
		"the refund is itself queued — it must NOT land in the same block the deposit failed in")

	// Block N+2: the refund lands.
	_, err = r.produce(t)
	require.NoError(t, err)
	assert.Equal(t, aliceBefore, liteBalance(t, r.db, alice),
		"one block per hop: transfer, failed deposit, refund")
}

// The queue is persisted state: a node restarted between the producing block
// and the next block still delivers.
func TestBridge_QueueSurvivesRestart(t *testing.T) {
	alice := acctesting.GenerateKey("bridge", "alice")
	bob := acctesting.GenerateKey("bridge", "bob")
	db, valKey := newRealBridgeDB(t, alice, bob)

	r1 := newBridgeOver(t, db, valKey, 0)
	bobBefore := liteBalance(t, db, bob)
	_, err := r1.produce(t, types.NewBatch([][]byte{marshalEnv(t, sendBetween(t, alice, bob, 3, 1))}))
	require.NoError(t, err)
	require.Equal(t, bobBefore, liteBalance(t, db, bob))

	// "Restart": a fresh executor and bridge over the same database.
	r2 := newBridgeOver(t, db, valKey, 0)
	_, err = r2.produce(t)
	require.NoError(t, err)
	assert.Equal(t, bobBefore+3, liteBalance(t, db, bob),
		"the queue is persisted — a restart between produce and drain loses nothing")
}
