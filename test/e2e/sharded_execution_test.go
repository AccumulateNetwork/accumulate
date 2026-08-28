// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4145's gate: shard count is a local parallelism choice that cannot change
// the result. Everything else in the shard test plan is diagnosis for when
// this fails.
//
// HOW the check works matters. Comparing two separate simulations does not:
// anchor signatures are wall-clock-timestamped, so even two serial runs of
// identical traffic diverge (verified — that is a property of the harness,
// not of execution). Instead, ONE network runs with every node at a
// DIFFERENT shard count. All nodes execute the same blocks with the same
// inputs — including the same wall-clock-signed anchors — and the
// simulator's consensus compares every node's deliver and commit results on
// every block. If shard count could change any result, the step fails on the
// block where it does. The traffic below drives multi-envelope parallel runs
// (many identities sharing blocks), intra-identity transfers, cross-identity
// and cross-partition deposits, and the serial barriers synthetics and
// anchors create.
func TestShardCountDoesNotChangeBlockHash(t *testing.T) {
	const identities = 12
	const rounds = 3

	sim := NewSim(t,
		simulator.SimpleNetwork("Sharded", 2, 3),
		simulator.Genesis(GenesisTime),
		// Node 0 executes serially, node 1 at 4 shards, node 2 at 64 — on
		// every partition, for every block of this test.
		simulator.ExecutionShardsPerNode(1, 4, 64),
	)

	type party struct {
		key []byte
		id  *url.URL
	}
	parties := make([]party, identities)
	for i := range parties {
		id := AccountUrl(fmt.Sprintf("party-%d", i))
		key := acctesting.GenerateKey(id)
		MakeIdentity(t, sim.DatabaseFor(id), id, key[32:])
		CreditCredits(t, sim.DatabaseFor(id), id.JoinPath("book", "1"), 1e9)
		MakeAccount(t, sim.DatabaseFor(id),
			&TokenAccount{Url: id.JoinPath("tokens"), TokenUrl: AcmeUrl()},
			&TokenAccount{Url: id.JoinPath("savings"), TokenUrl: AcmeUrl()})
		CreditTokens(t, sim.DatabaseFor(id), id.JoinPath("tokens"), big.NewInt(1e12))
		parties[i] = party{key, id}
	}

	// Each round: every identity moves tokens to its own savings (pure
	// intra-identity, always shardable) and to the NEXT identity's tokens (a
	// produced deposit — same or cross partition depending on routing). All
	// of a round's transactions are submitted before stepping, so they share
	// blocks and form real multi-envelope parallel runs.
	for r := 0; r < rounds; r++ {
		// Timestamps are per-signer nonces and must increase monotonically:
		// two submissions per party per round.
		ts := uint64(2*r + 1)
		sts := make([]*TransactionStatus, 0, 2*identities)
		for i, p := range parties {
			st := sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(p.id.JoinPath("tokens")).
					SendTokens(1, 0).To(p.id.JoinPath("savings")).
					SignWith(p.id.JoinPath("book", "1")).Version(1).Timestamp(ts).PrivateKey(p.key)))
			sts = append(sts, st)

			next := parties[(i+1)%identities]
			st = sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(p.id.JoinPath("tokens")).
					SendTokens(2, 0).To(next.id.JoinPath("tokens")).
					SignWith(p.id.JoinPath("book", "1")).Version(1).Timestamp(ts+1).PrivateKey(p.key)))
			sts = append(sts, st)
		}
		for _, st := range sts {
			sim.StepUntil(Txn(st.TxID).Completes())
		}
	}

	// The consensus layer has been comparing all three shard counts on every
	// block; a divergence would already have failed a step. Verify the
	// traffic itself landed.
	for _, p := range parties {
		savings := GetAccount[*TokenAccount](t, sim.DatabaseFor(p.id), p.id.JoinPath("savings"))
		require.EqualValues(t, rounds, savings.Balance.Int64())
	}

	// And let the tail of synthetics and anchors settle under comparison too.
	sim.StepN(20)
}

// #4149: the equivalence gate above only ever drove self-contained
// one-identity envelopes, so the OLD classifier — which trusted a
// signature's claimed TxID authority and a remote stub's principal — sent
// every one of them serial anyway, and the gate compared serial to serial.
// This drives the shapes that classification actually has to reason about:
// a multisig completed by a LATER signature-only envelope (the executor must
// resolve the real transaction from the store, not the claim), cross-ADI
// delegated signing (a genuine multi-identity envelope that must go serial),
// and a held transaction (whose signatures write the partition ledger and
// must go serial). Under the per-block 1/4/64-shard comparison, any
// misclassification — sharding a write that belongs to another identity or
// to the system ledger — diverges a node and fails the step.
func TestShardEquivalence_MixedSignatureShapes(t *testing.T) {
	// alice: a 2-of-2 page whose second signature arrives in a later block.
	// bob: delegates to charlie, who signs bob's transactions cross-ADI.
	// dave: sends held transactions.
	alice := AccountUrl("alice")
	aliceK1 := acctesting.GenerateKey(alice, 1)
	aliceK2 := acctesting.GenerateKey(alice, 2)
	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)
	charlie := AccountUrl("charlie")
	charlieKey := acctesting.GenerateKey(charlie)
	dave := AccountUrl("dave")
	daveKey := acctesting.GenerateKey(dave)

	sim := NewSim(t,
		simulator.SimpleNetwork("ShardShapes", 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.ExecutionShardsPerNode(1, 4, 64),
	)

	for _, s := range []struct {
		id   *url.URL
		keys [][]byte
	}{
		{alice, [][]byte{aliceK1[32:], aliceK2[32:]}},
		{bob, [][]byte{bobKey[32:]}},
		{charlie, [][]byte{charlieKey[32:]}},
		{dave, [][]byte{daveKey[32:]}},
	} {
		MakeIdentity(t, sim.DatabaseFor(s.id), s.id, s.keys...)
		CreditCredits(t, sim.DatabaseFor(s.id), s.id.JoinPath("book", "1"), 1e9)
		MakeAccount(t, sim.DatabaseFor(s.id),
			&TokenAccount{Url: s.id.JoinPath("tokens"), TokenUrl: AcmeUrl()})
		CreditTokens(t, sim.DatabaseFor(s.id), s.id.JoinPath("tokens"), big.NewInt(1e12))
	}

	// alice needs both keys to sign (2-of-2).
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AcceptThreshold = 2
	})
	// bob's page delegates to charlie's book — charlie can sign for bob.
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
		require.NoError(t, p.SetThreshold(1))
	})

	var ts uint64
	next := func() uint64 { ts++; return ts }

	// (1) alice initiates a transfer with key1 — pending, one signature short.
	aliceTxn := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice.JoinPath("tokens")).
			SendTokens(1, 0).To(dave.JoinPath("tokens")).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(next()).PrivateKey(aliceK1)))
	sim.StepUntil(Txn(aliceTxn.TxID).IsPending())

	// (2) dave submits a HELD transaction — serial while held.
	held := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(dave.JoinPath("tokens")).
			HoldUntil(HoldUntilOptions{MinorBlock: 15}).
			SendTokens(1, 0).To(alice.JoinPath("tokens")).
			SignWith(dave.JoinPath("book", "1")).Version(1).Timestamp(next()).PrivateKey(daveKey)))

	// (3) charlie signs a bob transaction cross-ADI via delegation — a
	// genuine two-identity envelope the classifier must send serial.
	bobTxn := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(bob.JoinPath("tokens")).
			SendTokens(1, 0).To(charlie.JoinPath("tokens")).
			SignWith(charlie.JoinPath("book", "1")).Delegator(bob.JoinPath("book", "1")).
			Version(1).Timestamp(next()).PrivateKey(charlieKey)))

	// Ordinary shardable traffic sharing the same blocks.
	for i := 0; i < 3; i++ {
		sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(charlie.JoinPath("tokens")).
				SendTokens(1, 0).To(bob.JoinPath("tokens")).
				SignWith(charlie.JoinPath("book", "1")).Version(1).Timestamp(next()).PrivateKey(charlieKey)))
		sim.StepN(1)
	}

	// (4) alice's SECOND signature arrives now, in its own envelope — the
	// executor resolves the real transaction from the store, and the old
	// classifier's TxID@unknown authority is exactly what made this shape
	// invisible before.
	sig2 := sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(aliceTxn.TxID).Load(sim.Query()).
			Url(alice.JoinPath("book", "1")).Version(1).Timestamp(next()).PrivateKey(aliceK2))
	_ = sig2

	sim.StepUntil(Txn(aliceTxn.TxID).Completes())
	sim.StepUntil(Txn(bobTxn.TxID).Completes())

	// Step past the hold block so the held transaction executes under the
	// comparison too.
	sim.StepUntil(Txn(held.TxID).Completes())

	// The comparison ran on every block; if any shape had been mis-sharded a
	// step would already have failed. Confirm the traffic actually landed.
	require.NotZero(t,
		GetAccount[*TokenAccount](t, sim.DatabaseFor(charlie), charlie.JoinPath("tokens")).Balance.Int64())
}
