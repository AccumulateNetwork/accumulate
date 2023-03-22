// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestFastSyntheticMessages verifies that synthetic messages are executed
// immediately (i.e. in the same block) if the producer and destination belong
// to the same domain.
func TestFastSyntheticMessages(t *testing.T) {
	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens2"), TokenUrl: AcmeUrl()})

	// Cause a synthetic transaction
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(1, AcmeOraclePrecisionPower).To(alice, "tokens2").
			SignWith(alice, "book", "1").Timestamp(1).Version(1).PrivateKey(aliceKey))

	// Wait until the signature is received
	sim.StepUntil(
		Sig(st[1].TxID).Received())

	// Verify that the signature, transaction, and synthetic transaction are
	// fully complete without stepping the simulator further
	var sig, txn, synth *protocol.TransactionStatus
	sim.Verify(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).Completes(),

		Sig(st[1].TxID).Capture(&sig).Received(),
		Txn(st[0].TxID).Capture(&txn).Received(),
		Txn(st[0].TxID).Produced().Capture(&synth).Received(),
	)
	require.NotZero(t, sig.Received)
	require.Equal(t, sig.Received, txn.Received)
	require.Equal(t, sig.Received, synth.Received)

	require.NotZero(t, GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens")).Balance)
}
