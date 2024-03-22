// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestSignatureMemo(t *testing.T) {
	// Tests AIP-006
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

	// Submit
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").
			Version(1).Timestamp(1).
			Memo("foo").
			Metadata("bar").
			PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())

	// Verify the signature
	sig := sim.QueryMessage(st[1].TxID, nil).Message.(*messaging.SignatureMessage).Signature.(*protocol.ED25519Signature)
	require.Equal(t, "foo", sig.Memo)
	require.Equal(t, "bar", string(sig.Data))
}

func TestVoteTypes(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)

	t.Run("Pre-Vandenburg", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 1),
			simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Baikonur),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Submit
		st := sim.BuildAndSubmit(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(alice, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))
		require.Error(t, st[1].AsError())
		require.EqualError(t, st[1].Error, "initial signature cannot be a suggest vote")
	})

	t.Run("Suggestathon pre-Vandenburg", func(t *testing.T) {
		// Pre-Vandenburg, what happens if everyone suggests?

		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 1),
			simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Baikonur),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Initiate with bob, acts like a suggestion
		st := sim.BuildAndSubmit(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(bob, "book", "1").
				Version(1).Timestamp(1).
				PrivateKey(bobKey))

		sim.StepUntil(
			Sig(st[1].TxID).CreditPayment().Completes(),
			Txn(st[0].TxID).IsPending())

		// Suggest with Alice
		st = sim.BuildAndSubmitSuccessfully(
			build.SignatureForTxID(st[0].TxID).
				Url(alice, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st[0].TxID).Fails().WithError(errors.Rejected))
	})

	t.Run("Bob suggests", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Suggest
		st := sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(bob, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(bobKey))

		sim.StepUntil(
			Sig(st[1].TxID).CreditPayment().Completes(),
			Txn(st[0].TxID).IsPending())

		// Sign
		sim.BuildAndSubmitSuccessfully(
			build.SignatureForTxID(st[0].TxID).
				Url(alice, "book", "1").
				Accept().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st[0].TxID).Succeeds())
	})

	t.Run("Suggesting does not execute", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		// Suggest
		st := sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(alice, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))

		sim.StepUntil(
			Sig(st[1].TxID).CreditPayment().Completes(),
			Txn(st[0].TxID).IsPending())

		// Verify the signature set is empty
		r := sim.QueryTransaction(st[0].TxID, nil)
		for _, sig := range r.Signatures.Records {
			for _, sig := range sig.Signatures.Records {
				_, ok := sig.Message.(*messaging.SignatureMessage)
				require.False(t, ok && !sig.Historical, "Should not have any active signatures")
			}
		}

	})

	t.Run("No secondary suggestions", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Suggest with Bob
		st := sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(bob, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(bobKey))

		sim.StepUntil(
			Sig(st[1].TxID).CreditPayment().Completes(),
			Txn(st[0].TxID).IsPending())

		// Suggest with Alice
		st = sim.BuildAndSubmit(
			build.SignatureForTxID(st[0].TxID).
				Url(alice, "book", "1").
				Suggest().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))
		require.Error(t, st[1].AsError())
		require.EqualError(t, st[1].Error, "suggestions cannot be secondary signatures")
	})

	t.Run("No merkle suggestions", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

		// Suggest
		bld := build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(bob, "book", "1").
			Suggest().
			Version(1).Timestamp(1).
			PrivateKey(bobKey)
		bld.InitMerkle = true
		st := sim.BuildAndSubmit(bld)
		require.Error(t, st[1].AsError())
		require.EqualError(t, st[1].Error, "suggestions cannot use Merkle initiator hashes")
	})

	t.Run("Cannot initiate with rejection", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

		// Initiate with rejection
		st := sim.BuildAndSubmit(
			build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(alice, "book", "1").
				Reject().
				Version(1).Timestamp(1).
				PrivateKey(aliceKey))
		require.Error(t, st[1].AsError())
		require.EqualError(t, st[1].Error, "initial signature cannot be a reject vote")
	})
}
