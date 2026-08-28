// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"math/big"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4146 Stage 0: pin what the LocalTo inlining does TODAY, before the
// predicate widens from same-identity to same-partition. The measurable
// difference between the two paths is the synthetic ledger: an inlined
// (PseudoSynthetic) delivery consumes no sequence number, a sequenced one
// does — even when source and destination are the same partition.

// producedTotals sums Produced across every stream of a partition's
// synthetic ledger.
func producedTotals(t *testing.T, db database.Viewer, partition string) map[string]uint64 {
	t.Helper()
	totals := map[string]uint64{}
	View(t, db, func(batch *database.Batch) {
		var ledger *SyntheticLedger
		require.NoError(t, batch.Account(PartitionUrl(partition).JoinPath(Synthetic)).Main().GetAs(&ledger))
		for _, part := range ledger.Sequence {
			totals[part.Url.ShortString()] = part.Produced
		}
	})
	return totals
}

// A produced message whose destination shares the producer's ROOT IDENTITY is
// inlined as a PseudoSynthetic: it executes in the same bundle and never
// touches the synthetic ledger. No sequence number, no proof, no dispatch.
func TestCharacterize_SameIdentityDeliveryConsumesNoSequenceNumber(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("savings"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	before := producedTotals(t, sim.Database("BVN0"), "BVN0")

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice.JoinPath("tokens")).
			SendTokens(1, 0).To(alice.JoinPath("savings")).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Completes())

	require.EqualValues(t, 1,
		GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("savings")).Balance.Int64(),
		"the deposit landed")

	after := producedTotals(t, sim.Database("BVN0"), "BVN0")
	assert.Equal(t, before, after,
		"a same-identity deposit is inlined as a PseudoSynthetic — it must not consume a sequence number on any stream")
}

// A produced message to a DIFFERENT identity on the SAME partition used to
// be sequenced and dispatched — the partition opened a synthetic stream to
// ITSELF and the deposit paid for a sequence number, a proof, and a dispatch
// round-trip. #4146 removed that: it is delivered through the local queue at
// the start of the next block, and no stream ever opens. This test carried
// the old expectation as characterization until the removal landed; the flip
// below IS the removal, visible as a diff.
func TestCharacterize_SamePartitionDifferentIdentityConsumesNoSequenceNumber(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	alice, bob := AccountUrl("alice"), AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	before := producedTotals(t, sim.Database("BVN0"), "BVN0")

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice.JoinPath("tokens")).
			SendTokens(1, 0).To(bob.JoinPath("tokens")).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Completes())

	require.EqualValues(t, 1,
		GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens")).Balance.Int64())

	after := producedTotals(t, sim.Database("BVN0"), "BVN0")
	assert.Equal(t, before, after,
		"a same-partition deposit is delivered through the local queue (#4146) — no stream, no sequence number, on any stream")
}
