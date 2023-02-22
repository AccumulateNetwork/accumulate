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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestRefundCycle(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Setup accounts
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e12)})

	// Send tokens
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			SendTokens(1, AcmePrecisionPower).To("bob.acme", "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Erase the sender
	Update(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("tokens")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("book", "1")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("book")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice))
	})

	// The deposit and refund should fail (because the account no longer exists)
	sim.StepUntil(
		Txn(st.TxID).Produced().Fails(),
		Txn(st.TxID).Refund().FailsWithCode(errors.NotFound))

	// Ensure the refund did not produce anything
	for _, id := range sim.QueryTransaction(st.TxID, nil).Produced.Records { //      Produced by SendTokens
		for _, id := range sim.QueryTransaction(id.Value, nil).Produced.Records { // Produced by deposit
			require.Zero(t, sim.QueryTransaction(id.Value, nil).Produced.Total) //   Produced by refund
		}
	}
}

func TestRefundFailedUserTransaction_Local(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	var timestamp uint64
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e12)})

	// Submit the transaction
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			SendTokens(1, AcmePrecisionPower).To("bob.acme", "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	// Zero the balance before the transaction is executed to make it fail
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), func(a *TokenAccount) { a.Balance.SetUint64(0) })

	// The transaction fails (insufficient balance) and issues a refund
	sim.StepUntil(
		Txn(st.TxID).Fails(),
		Txn(st.TxID).Produced().Succeeds())

	// The transaction produces a refund for the signer
	produced := sim.QueryTransaction(st.TxID, nil).Produced.Records
	require.Len(t, produced, 1, "Expected a single transaction to be produced")
	refund := sim.QueryTransaction(produced[0].Value, nil)
	require.IsType(t, (*SyntheticDepositCredits)(nil), refund.Transaction.Body)
	require.Equal(t, alice.JoinPath("book", "1").ShortString(), refund.Transaction.Header.Principal.ShortString())
}

func TestRefundFailedUserTransaction_Remote(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	var timestamp uint64
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl(), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: alice.JoinPath("book")}}}})

	// Submit a transaction with a remote signature
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(bob, "tokens").
			SendTokens(1, AcmePrecisionPower).To("charlie.acme", "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	// The transaction fails (insufficient balance) and issues a refund
	sim.StepUntil(
		Txn(st.TxID).Fails())

	sim.StepUntil(
		Txn(st.TxID).Produced().Succeeds())

	// The transaction produces a refund for the signer
	produced := sim.QueryTransaction(st.TxID, nil).Produced.Records
	require.Len(t, produced, 1, "Expected a single transaction to be produced")
	refund := sim.QueryTransaction(produced[0].Value, nil)
	require.IsType(t, (*SyntheticDepositCredits)(nil), refund.Transaction.Body)
	require.Equal(t, alice.JoinPath("book", "1").ShortString(), refund.Transaction.Header.Principal.ShortString())
}
