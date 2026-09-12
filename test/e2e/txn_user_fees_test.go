// Copyright 2025 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestUserFees_HappyPath tests the AIP-50 user fees feature:
// 1. Submit a transaction with UserFees
// 2. Verify the fee is escrowed
// 3. Transaction succeeds
// 4. Verify the recipient receives the fee
func TestUserFees_HappyPath(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	feeRecipient := url.MustParse("feeRecipient")

	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	feeRecipientKey := acctesting.GenerateKey(feeRecipient)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Setup accounts
	// Alice: sender with tokens and credits
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100*AcmePrecision)) // 100 ACME

	// Bob: recipient of the token transfer
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// FeeRecipient: will receive the user fee
	MakeIdentity(t, sim.DatabaseFor(feeRecipient), feeRecipient, feeRecipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(feeRecipient), &TokenAccount{Url: feeRecipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Record initial balances
	aliceInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens")).Balance.Int64()
	feeRecipientInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(feeRecipient), feeRecipient.JoinPath("tokens")).Balance.Int64()

	// Fee amount: 1 ACME
	feeAmount := big.NewInt(1 * AcmePrecision)

	// Execute: Send tokens from Alice to Bob with a user fee to feeRecipient
	// Note: Must explicitly pass the payer (alice/tokens) because the default payer calculation
	// uses signer.RootIdentity().JoinPath("ACME") which would be alice/ACME
	st := sim.BuildAndSubmitTxn(
		build.Transaction().For(alice, "tokens").
			UserFee(feeRecipient.JoinPath("tokens"), feeAmount, AcmeUrl(), alice.JoinPath("tokens")).
			SendTokens(10, AcmePrecisionPower).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	sim.StepN(50)

	// Verify: Alice's balance decreased by transfer amount + fee
	aliceFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens")).Balance.Int64()
	expectedAliceDecrease := int64(10*AcmePrecision) + feeAmount.Int64()
	actualAliceDecrease := aliceInitial - aliceFinal
	require.Equal(t, expectedAliceDecrease, actualAliceDecrease, "Alice should have paid transfer amount + fee")

	// Verify: Bob received the tokens
	bobBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, int64(10*AcmePrecision), bobBalance, "Bob should have received 10 ACME")

	// Verify: Fee recipient received the fee
	feeRecipientFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(feeRecipient), feeRecipient.JoinPath("tokens")).Balance.Int64()
	feeReceived := feeRecipientFinal - feeRecipientInitial
	require.Equal(t, feeAmount.Int64(), feeReceived, "Fee recipient should have received 1 ACME fee")
}

// TestUserFees_AcmeOnly tests that only ACME tokens are allowed for fees
func TestUserFees_AcmeOnly(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	feeRecipient := url.MustParse("feeRecipient")

	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	feeRecipientKey := acctesting.GenerateKey(feeRecipient)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Setup accounts
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	MakeIdentity(t, sim.DatabaseFor(feeRecipient), feeRecipient, feeRecipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(feeRecipient), &TokenAccount{Url: feeRecipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Record initial balances
	aliceInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens")).Balance.Int64()

	// Try to use a non-ACME token for the fee (should fail)
	fakeToken := url.MustParse("acc://fake-token")
	feeAmount := big.NewInt(1 * AcmePrecision)

	// This should fail because we're trying to use a non-ACME token
	st := sim.BuildAndSubmitTxn(
		build.Transaction().For(alice, "tokens").
			UserFee(feeRecipient.JoinPath("tokens"), feeAmount, fakeToken, alice.JoinPath("tokens")).
			SendTokens(10, AcmePrecisionPower).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Check the submission status
	t.Logf("Submission status: %v, TxID: %v", st.Code, st.TxID)
	t.Logf("Fee token: %v, ACME URL: %v, Equal: %v", fakeToken, AcmeUrl(), AcmeUrl().Equal(fakeToken))

	// Step to process the transaction
	sim.StepN(50)

	// Query the transaction to see its status
	record := sim.QueryTransaction(st.TxID, nil)
	t.Logf("Transaction status: %v, error: %v", record.Status, record.Error)

	// The signature should have failed with the ACME-only error
	// When signature processing fails, the transaction doesn't complete
	// Check the signature status for the error
	if record.Signatures != nil && record.Signatures.Total > 0 {
		for i, sigSet := range record.Signatures.Records {
			t.Logf("Signature set %d: Account=%v", i, sigSet.Account)
			if sigSet.Signatures != nil {
				for j, sig := range sigSet.Signatures.Records {
					// Query individual signature message status
					sigStatus := sim.QueryMessage(sig.ID, nil)
					t.Logf("  Signature %d: Status=%v, Error=%v", j, sigStatus.Status, sigStatus.Error)
					if sigStatus.Error != nil {
						// Found the error on the signature
						require.Contains(t, sigStatus.Error.Message, "only ACME tokens are allowed")
						require.Contains(t, sigStatus.Error.Message, "acc://fake-token")
						return // Test passed
					}
				}
			}
		}
	}

	// If we couldn't find the error on signatures, check the transaction error
	if record.Error != nil {
		require.Contains(t, record.Error.Message, "only ACME tokens are allowed")
		require.Contains(t, record.Error.Message, "acc://fake-token")
		return // Test passed
	}

	// Fallback: Verify behavior - Alice's tokens should NOT have been debited for fee
	// (since escrow failed before it could happen) and Bob should NOT have received tokens
	aliceFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens")).Balance.Int64()
	bobBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens")).Balance.Int64()

	t.Logf("Alice initial: %d, final: %d, Bob balance: %d", aliceInitial, aliceFinal, bobBalance)

	// Bob should NOT have received any tokens (transaction didn't execute)
	require.Equal(t, int64(0), bobBalance, "Bob should NOT have received tokens since the fee validation failed")

	// Alice should NOT have lost any tokens (neither fee nor transfer)
	require.Equal(t, aliceInitial, aliceFinal, "Alice's balance should be unchanged since the transaction was rejected")
}

// TestUserFees_FailureRefund tests that fees are refunded when transaction fails
func TestUserFees_FailureRefund(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	feeRecipient := url.MustParse("feeRecipient")

	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	feeRecipientKey := acctesting.GenerateKey(feeRecipient)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Setup accounts
	// Alice has exactly 10 ACME
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(10*AcmePrecision)) // Only 10 ACME

	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	MakeIdentity(t, sim.DatabaseFor(feeRecipient), feeRecipient, feeRecipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(feeRecipient), &TokenAccount{Url: feeRecipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Record initial balances
	aliceInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens")).Balance.Int64()
	t.Logf("Alice initial balance: %d (%.2f ACME)", aliceInitial, float64(aliceInitial)/float64(AcmePrecision))

	// Fee: 1 ACME
	// Transfer: 10 ACME (but Alice only has 10 total, so after 1 ACME fee escrow, only 9 remain)
	// This should FAIL due to insufficient balance for the transfer
	feeAmount := big.NewInt(1 * AcmePrecision)
	transferAmount := int64(10) // 10 ACME - more than remaining after fee

	st := sim.BuildAndSubmitTxn(
		build.Transaction().For(alice, "tokens").
			UserFee(feeRecipient.JoinPath("tokens"), feeAmount, AcmeUrl(), alice.JoinPath("tokens")).
			SendTokens(transferAmount, AcmePrecisionPower).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	t.Logf("Transaction submitted: %v, status: %v", st.TxID, st.Code)

	// Step to process
	sim.StepN(100)

	// Query transaction status
	record := sim.QueryTransaction(st.TxID, nil)
	t.Logf("Transaction status: %v, error: %v", record.Status, record.Error)

	// The transaction should fail due to insufficient balance
	// (After escrowing 1 ACME fee, Alice only has 9 ACME left, but transfer needs 10 ACME)
	// Delivered() returns true when processed (even with errors), so check for the error
	require.NotNil(t, record.Error, "Transaction should have failed with an error")
	require.Contains(t, record.Error.Message, "insufficient balance", "Transaction should fail due to insufficient balance")

	// Give time for any refunds to process
	sim.StepN(100)

	// Verify: Alice should have her original balance back (fee refunded)
	aliceFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens")).Balance.Int64()
	t.Logf("Alice final balance: %d (%.2f ACME)", aliceFinal, float64(aliceFinal)/float64(AcmePrecision))

	// Bob should NOT have received any tokens
	bobBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens")).Balance.Int64()
	t.Logf("Bob balance: %d", bobBalance)
	require.Equal(t, int64(0), bobBalance, "Bob should NOT have received tokens since transaction failed")

	// Fee recipient should NOT have received any fee (refunded to payer)
	feeRecipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(feeRecipient), feeRecipient.JoinPath("tokens")).Balance.Int64()
	t.Logf("Fee recipient balance: %d", feeRecipientBalance)
	require.Equal(t, int64(0), feeRecipientBalance, "Fee recipient should NOT have received fee since transaction failed")

	// Alice should have gotten her fee back
	// Note: Depending on when failure occurs, Alice might have full balance or partial
	// If escrow happened then refund happened: aliceFinal should equal aliceInitial
	// If escrow never happened: aliceFinal should equal aliceInitial
	require.Equal(t, aliceInitial, aliceFinal, "Alice should have her original balance (fee refunded on failure)")
}

// TestUserFees_MultipleFees tests multiple fee recipients
func TestUserFees_MultipleFees(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	feeRecipient1 := url.MustParse("feeRecipient1")
	feeRecipient2 := url.MustParse("feeRecipient2")

	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	feeRecipient1Key := acctesting.GenerateKey(feeRecipient1)
	feeRecipient2Key := acctesting.GenerateKey(feeRecipient2)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Setup accounts
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	MakeIdentity(t, sim.DatabaseFor(feeRecipient1), feeRecipient1, feeRecipient1Key[32:])
	MakeAccount(t, sim.DatabaseFor(feeRecipient1), &TokenAccount{Url: feeRecipient1.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	MakeIdentity(t, sim.DatabaseFor(feeRecipient2), feeRecipient2, feeRecipient2Key[32:])
	MakeAccount(t, sim.DatabaseFor(feeRecipient2), &TokenAccount{Url: feeRecipient2.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Fee amounts
	fee1 := big.NewInt(1 * AcmePrecision) // 1 ACME
	fee2 := big.NewInt(2 * AcmePrecision) // 2 ACME

	// Execute: Send tokens with multiple user fees
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			UserFee(feeRecipient1.JoinPath("tokens"), fee1, AcmeUrl(), alice.JoinPath("tokens")).
			UserFee(feeRecipient2.JoinPath("tokens"), fee2, AcmeUrl(), alice.JoinPath("tokens")).
			SendTokens(10, AcmePrecisionPower).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Give time for synthetic messages
	sim.StepN(50)

	// Verify both recipients received their fees
	balance1 := GetAccount[*TokenAccount](t, sim.DatabaseFor(feeRecipient1), feeRecipient1.JoinPath("tokens")).Balance.Int64()
	balance2 := GetAccount[*TokenAccount](t, sim.DatabaseFor(feeRecipient2), feeRecipient2.JoinPath("tokens")).Balance.Int64()

	require.Equal(t, fee1.Int64(), balance1, "Fee recipient 1 should have received 1 ACME")
	require.Equal(t, fee2.Int64(), balance2, "Fee recipient 2 should have received 2 ACME")
}
