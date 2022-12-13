// Copyright 2022 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestRefundCycle(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e12)})

	// Send tokens
	txn := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{
				To: []*TokenRecipient{{
					Url:    AccountUrl("bob", "tokens"),
					Amount: *big.NewInt(1),
				}},
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)[0].Transaction[0]
	_, _, synth := sim.WaitForTransaction(delivered, txn.GetHash(), 50)

	// Erase the sender ADI
	_ = sim.PartitionFor(alice).Database.Update(func(batch *database.Batch) error {
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("tokens")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("book", "1")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("book")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice))
		return nil
	})

	// Wait for the deposit
	var allSynth []*url.TxID
	for _, id := range synth {
		h := id.Hash()
		_, _, synth := sim.WaitForTransaction(delivered, h[:], 50)
		allSynth = append(allSynth, synth...)
	}

	// Wait for the refunds
	synth, allSynth = allSynth, nil
	for _, id := range synth {
		h := id.Hash()
		_, status, synth := sim.WaitForTransaction(delivered, h[:], 50)
		allSynth = append(allSynth, synth...)

		// Verify the refunds failed (because the account no longer exists)
		require.NotNil(t, status.Error)
		require.Equal(t, errors.NotFound, status.Error.Code)
	}

	// Verify the failed refund did not generate a refund
	require.Empty(t, allSynth)
}

func TestRefundFailedUserTransaction_Local(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// The transaction is submitted but fails when delivered
	sim.S.SetExecutor(&overrideExecutor{
		typ:      TransactionTypeSendTokens,
		validate: func(st *chain.StateManager, tx *chain.Delivery) error { return nil },
		execute:  func(st *chain.StateManager, tx *chain.Delivery) error { return fmt.Errorf("") },
	})

	// Submit a transaction
	st := sim.H.SubmitSuccessfully(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{To: []*TokenRecipient{{}}}).
			Initiate(SignatureTypeED25519, aliceKey).
			BuildDelivery())
	sim.H.StepUntil(
		Txn(st.TxID).Fails())

	hash := st.TxID.Hash()
	sim.WaitForTransactionFlow(delivered, hash[:])

	// The transaction produces a refund for the signer
	produced := simulator.GetTxnState[[]*url.TxID](sim, st.TxID, (*database.Transaction).Produced)
	require.Len(t, produced, 1, "Expected a single transaction to be produced")
	refund := simulator.GetTxnState[*database.SigOrTxn](sim, produced[0], (*database.Transaction).Main)
	require.NotNil(t, refund.Transaction)
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
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateIdentity(bob, bobKey[32:])
	sim.CreateAccount(&TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl(), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: alice.JoinPath("book")}}}})

	// The transaction would fail if submitted directly
	sim.S.SetExecutor(&overrideExecutor{
		typ:      TransactionTypeSendTokens,
		validate: func(st *chain.StateManager, tx *chain.Delivery) error { return fmt.Errorf("") },
		execute:  func(st *chain.StateManager, tx *chain.Delivery) error { return fmt.Errorf("") },
	})

	// Submit a transaction with a remote signature
	txn := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bob.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{To: []*TokenRecipient{{}}}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)[0].Transaction[0]
	status, _ := sim.WaitForTransactionFlow(delivered, txn.GetHash())
	require.True(t, status[0].Failed())

	// The transaction produces a refund for the signer
	produced := simulator.GetTxnState[[]*url.TxID](sim, status[0].TxID, (*database.Transaction).Produced)
	require.Len(t, produced, 1, "Expected a single transaction to be produced")
	refund := simulator.GetTxnState[*database.SigOrTxn](sim, produced[0], (*database.Transaction).Main)
	require.NotNil(t, refund.Transaction)
	require.IsType(t, (*SyntheticDepositCredits)(nil), refund.Transaction.Body)
	require.Equal(t, alice.JoinPath("book", "1").ShortString(), refund.Transaction.Header.Principal.ShortString())
}
