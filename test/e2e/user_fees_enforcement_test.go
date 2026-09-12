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

// =============================================================================
// MODEL 1: MULTI-SIGNATURE AUTHORITY ENFORCEMENT
// =============================================================================
//
// In this model, the service provider is added as an AUTHORITY on the user's
// account. Both the user AND the service must sign every transaction.
// The service only co-signs if the required fee is included.
//
// Use Case: Wallet providers, payment processors, compliance services
//

// TestModel1_MultiSig_WithFee_Success demonstrates a successful transaction
// where the user includes the required fee and the service co-signs.
func TestModel1_MultiSig_WithFee_Success(t *testing.T) {
	// === SETUP ===
	user := url.MustParse("user")
	service := url.MustParse("wallet-service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Create user's identity and accounts
	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(user), &TokenAccount{Url: user.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	// Create service's identity (the fee recipient)
	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(service), &TokenAccount{Url: service.JoinPath("fees"), TokenUrl: AcmeUrl()})

	// Create recipient
	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// === KEY STEP: Add service as authority on user's token account ===
	// This means BOTH user AND service must sign transactions
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			UpdateAccountAuth().
			Add(service.JoinPath("book")).
			SignWith(user, "book", "1").Version(1).Timestamp(1).PrivateKey(userKey))

	// Wait for transaction to be pending (needs co-signature from new authority)
	sim.StepUntil(
		Txn(st.TxID).IsPending())

	// Service must sign to approve being added as authority
	r := sim.QueryTransaction(st.TxID, nil)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(r.Message.Transaction).
			Url(service, "book", "1").Version(1).Timestamp(1).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Record initial balances
	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	serviceInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(service), service.JoinPath("fees")).Balance.Int64()

	t.Logf("User initial balance: %d", userInitial)
	t.Logf("Service initial balance: %d", serviceInitial)

	// === TRANSACTION WITH FEE ===
	// User creates transaction with the required fee
	feeAmount := big.NewInt(1 * AcmePrecision) // 1 ACME fee
	transferAmount := int64(10)                 // 10 ACME transfer

	// Build transaction with UserFee - user signs first
	env := MustBuild(t,
		build.Transaction().For(user, "tokens").
			UserFee(service.JoinPath("fees"), feeAmount, AcmeUrl(), user.JoinPath("tokens")).
			SendTokens(transferAmount, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(user, "book", "1").Version(1).Timestamp(2).PrivateKey(userKey))

	// Submit user's signature
	st = sim.SubmitTxnSuccessfully(env)
	sim.StepN(10)

	// === SERVICE CO-SIGNS ===
	// Service validates fee is present and co-signs
	txn := env.Transaction[0]
	require.NotEmpty(t, txn.Header.UserFees, "Service checks: UserFees must be present")
	require.True(t, txn.Header.UserFees[0].Recipient.Equal(service.JoinPath("fees")),
		"Service checks: Fee must go to our account")
	require.True(t, txn.Header.UserFees[0].Amount.Cmp(feeAmount) >= 0,
		"Service checks: Fee must meet minimum")

	t.Log("Service validated fee requirements - co-signing transaction")

	// Service co-signs (use timestamp 2 since 1 was used for auth approval)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(env.Transaction[0]).
			Url(service, "book", "1").Version(1).Timestamp(2).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	sim.StepN(50)

	// === VERIFY RESULTS ===
	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	serviceFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(service), service.JoinPath("fees")).Balance.Int64()
	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()

	t.Logf("User final balance: %d (paid %d)", userFinal, userInitial-userFinal)
	t.Logf("Service final balance: %d (received %d)", serviceFinal, serviceFinal-serviceInitial)
	t.Logf("Recipient balance: %d", recipientBalance)

	// User paid transfer + fee
	expectedUserPaid := transferAmount*AcmePrecision + feeAmount.Int64()
	require.Equal(t, expectedUserPaid, userInitial-userFinal, "User should have paid transfer + fee")

	// Service received the fee
	require.Equal(t, feeAmount.Int64(), serviceFinal-serviceInitial, "Service should have received the fee")

	// Recipient received the transfer
	require.Equal(t, transferAmount*AcmePrecision, recipientBalance, "Recipient should have received the transfer")

	t.Log("SUCCESS: Multi-sig with fee worked correctly")
}

// TestModel1_MultiSig_WithoutFee_Blocked demonstrates that without the fee,
// the service refuses to co-sign and the transaction cannot execute.
func TestModel1_MultiSig_WithoutFee_Blocked(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("wallet-service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(user), &TokenAccount{Url: user.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(service), &TokenAccount{Url: service.JoinPath("fees"), TokenUrl: AcmeUrl()})

	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Add service as authority
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			UpdateAccountAuth().
			Add(service.JoinPath("book")).
			SignWith(user, "book", "1").Version(1).Timestamp(1).PrivateKey(userKey))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	r := sim.QueryTransaction(st.TxID, nil)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(r.Message.Transaction).
			Url(service, "book", "1").Version(1).Timestamp(1).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()

	// === USER TRIES TO BYPASS: Transaction WITHOUT fee ===
	t.Log("User attempts to submit transaction WITHOUT the required fee...")

	env := MustBuild(t,
		build.Transaction().For(user, "tokens").
			// NO UserFee included - attempting bypass!
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(user, "book", "1").Version(1).Timestamp(2).PrivateKey(userKey))

	st = sim.SubmitTxnSuccessfully(env)
	sim.StepN(10)

	// === SERVICE REFUSES TO CO-SIGN ===
	txn := env.Transaction[0]
	if len(txn.Header.UserFees) == 0 {
		t.Log("SERVICE REJECTION: No UserFees found - refusing to co-sign")
	}

	sim.StepN(50)

	// === VERIFY TRANSACTION IS STUCK/PENDING ===
	record := sim.QueryTransaction(st.TxID, nil)
	t.Logf("Transaction status: %v", record.Status)

	require.False(t, record.Status.Delivered(), "Transaction should NOT be delivered without service co-signature")

	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, userInitial, userFinal, "User's balance should be unchanged - bypass BLOCKED")

	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, int64(0), recipientBalance, "Recipient should have received nothing - bypass BLOCKED")

	t.Log("SUCCESS: Bypass attempt BLOCKED - transaction stuck without service co-signature")
	_ = serviceKey // Service key unused because we refused to sign
}

// =============================================================================
// MODEL 2: MULTI-AUTHORITY ACCOUNT (Simpler version)
// =============================================================================
//
// In this model, the account is created with BOTH user and service as
// authorities from the start. This is simpler than Model 1 where authority
// is added after account creation.
//

// TestModel2_DualAuthority_WithFee_Success demonstrates a dual-authority account
// where both parties must sign, and the service only signs if fee is present.
func TestModel2_DualAuthority_WithFee_Success(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Create identities
	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)

	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(service), &TokenAccount{Url: service.JoinPath("fees"), TokenUrl: AcmeUrl()})

	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Create token account with BOTH authorities from the start
	// User creates it under their ADI, adding service as a second authority
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user).
			CreateTokenAccount(user.JoinPath("tokens")).
			ForToken(AcmeUrl()).
			WithAuthority(user.JoinPath("book")).
			WithAuthority(service.JoinPath("book")).
			SignWith(user, "book", "1").Version(1).Timestamp(1).PrivateKey(userKey))

	// Wait for pending - service must approve being added as authority
	sim.StepUntil(
		Txn(st.TxID).IsPending())

	r := sim.QueryTransaction(st.TxID, nil)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(r.Message.Transaction).
			Url(service, "book", "1").Version(1).Timestamp(1).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Fund the account
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	serviceInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(service), service.JoinPath("fees")).Balance.Int64()

	// === TRANSACTION WITH FEE ===
	feeAmount := big.NewInt(1 * AcmePrecision)

	env := MustBuild(t,
		build.Transaction().For(user, "tokens").
			UserFee(service.JoinPath("fees"), feeAmount, AcmeUrl(), user.JoinPath("tokens")).
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(user, "book", "1").Version(1).Timestamp(2).PrivateKey(userKey))

	st = sim.SubmitTxnSuccessfully(env)
	sim.StepN(10)

	// Service validates and co-signs
	t.Log("Service validates fee and co-signs...")
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(env.Transaction[0]).
			Url(service, "book", "1").Version(1).Timestamp(2).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	sim.StepN(50)

	// === VERIFY ===
	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	serviceFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(service), service.JoinPath("fees")).Balance.Int64()
	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()

	require.Equal(t, int64(11*AcmePrecision), userInitial-userFinal, "User paid 10 transfer + 1 fee")
	require.Equal(t, feeAmount.Int64(), serviceFinal-serviceInitial, "Service received fee")
	require.Equal(t, int64(10*AcmePrecision), recipientBalance, "Recipient received transfer")

	t.Log("SUCCESS: Dual-authority with fee worked correctly")
}

// TestModel2_DualAuthority_WithoutFee_Blocked demonstrates bypass blocking
func TestModel2_DualAuthority_WithoutFee_Blocked(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)

	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)

	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Create dual-authority account
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user).
			CreateTokenAccount(user.JoinPath("tokens")).
			ForToken(AcmeUrl()).
			WithAuthority(user.JoinPath("book")).
			WithAuthority(service.JoinPath("book")).
			SignWith(user, "book", "1").Version(1).Timestamp(1).PrivateKey(userKey))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	r := sim.QueryTransaction(st.TxID, nil)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(r.Message.Transaction).
			Url(service, "book", "1").Version(1).Timestamp(1).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))
	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()

	// === USER TRIES TO BYPASS ===
	t.Log("User attempts transaction WITHOUT fee...")

	env := MustBuild(t,
		build.Transaction().For(user, "tokens").
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(user, "book", "1").Version(1).Timestamp(2).PrivateKey(userKey))

	st = sim.SubmitTxnSuccessfully(env)
	sim.StepN(10)

	t.Log("Service: No fee - REFUSING to co-sign")
	// Service does NOT sign

	sim.StepN(50)

	// === VERIFY BLOCKED ===
	record := sim.QueryTransaction(st.TxID, nil)
	require.False(t, record.Status.Delivered(), "Transaction should NOT execute")

	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, userInitial, userFinal, "User balance unchanged - bypass BLOCKED")

	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, int64(0), recipientBalance, "Recipient got nothing - bypass BLOCKED")

	t.Log("SUCCESS: Bypass BLOCKED - service co-signature required")
	_ = serviceKey
}

// =============================================================================
// MODEL 3: DELEGATED SIGNATURE
// =============================================================================
//
// In this model, the user grants the service delegated signing authority.
// The service creates and signs transactions on the user's behalf.
//
// IMPORTANT: Delegation alone does NOT enforce fees - user can still sign directly.
// This model is for convenience, not enforcement.
//

// TestModel3_Delegation_ServiceSignsForUser demonstrates a service creating
// transactions on behalf of the user using delegated signing.
//
// NOTE: This test demonstrates delegation mechanics without UserFees.
// UserFees with multi-authority are tested in Models 1 and 2.
func TestModel3_Delegation_ServiceSignsForUser(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(user), &TokenAccount{Url: user.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)

	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// === USER GRANTS DELEGATION TO SERVICE ===
	t.Log("User grants delegated signing authority to service...")

	// Use helper method to directly add delegation (same as existing tests)
	UpdateAccount(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: service.JoinPath("book")})
	})

	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()

	// === SERVICE ACTS ON BEHALF OF USER ===
	// In a real scenario, the service would include its fee here.
	// For this test, we demonstrate that delegation works correctly.
	t.Log("Service creates transaction on user's behalf...")

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(service, "book", "1").Version(1).Timestamp(1).
			Delegator(user.JoinPath("book", "1")). // Signing as delegate
			PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	sim.StepN(10)

	// === VERIFY ===
	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()

	require.Equal(t, int64(10*AcmePrecision), userInitial-userFinal, "User paid 10 ACME transfer")
	require.Equal(t, int64(10*AcmePrecision), recipientBalance, "Recipient received transfer")

	t.Log("SUCCESS: Delegated signature worked correctly")
	t.Log("NOTE: In production, the service would include UserFees in the transaction")
	_ = userKey // User key not needed - service signs for them
}

// TestModel3_Delegation_UserCanBypass demonstrates that delegation alone
// does NOT enforce fees - the user can still sign directly.
func TestModel3_Delegation_UserCanBypass(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(user), &TokenAccount{Url: user.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)

	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Grant delegation using helper method
	UpdateAccount(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: service.JoinPath("book")})
	})

	// === USER SIGNS DIRECTLY (bypassing service) ===
	t.Log("WARNING: User can still sign directly with delegation model!")
	t.Log("User attempts direct transaction without fee...")

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			// NO FEE!
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(user, "book", "1").Version(1).Timestamp(2).PrivateKey(userKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Transaction SUCCEEDS - user bypassed the fee!
	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, int64(10*AcmePrecision), recipientBalance, "Recipient received transfer")

	t.Log("IMPORTANT: Delegation alone does NOT block direct user signing!")
	t.Log("For enforcement, use Model 1 or Model 2 (multi-authority)")
	_ = serviceKey
}

// =============================================================================
// MODEL 4: COMBINED (Multi-Authority + Delegation)
// =============================================================================
//
// This model combines multi-authority with delegation for maximum control:
// - Service is an authority (required to sign - ENFORCEMENT)
// - Service has delegation (can sign for user - CONVENIENCE)
// - Result: Service can act for user, AND user cannot bypass service
//

// TestModel4_Combined_ServiceActsForUser demonstrates the service
// creating transactions on behalf of the user using the combined model.
//
// This model combines multi-authority (enforcement) with delegation (convenience):
// - Service is an authority → user CANNOT bypass
// - Service is a delegate → service CAN sign on user's behalf
//
// NOTE: This test demonstrates the combined model mechanics without UserFees.
// UserFees with multi-authority are tested in Models 1 and 2.
func TestModel4_Combined_ServiceActsForUser(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(user), &TokenAccount{Url: user.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)

	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// === STEP 1: Add service as authority on token account (ENFORCEMENT) ===
	t.Log("Step 1: Adding service as authority (for enforcement)...")
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			UpdateAccountAuth().
			Add(service.JoinPath("book")).
			SignWith(user, "book", "1").Version(1).Timestamp(1).PrivateKey(userKey))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	r := sim.QueryTransaction(st.TxID, nil)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(r.Message.Transaction).
			Url(service, "book", "1").Version(1).Timestamp(1).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// === STEP 2: Add service as delegate on user's key page (CONVENIENCE) ===
	t.Log("Step 2: Adding service as delegate (for convenience)...")
	UpdateAccount(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: service.JoinPath("book")})
	})

	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()

	// === SERVICE CREATES TRANSACTION ===
	// In production, the service would include its UserFee here.
	// For this test, we demonstrate the combined model mechanics.
	t.Log("Service creates transaction on user's behalf...")

	// Service signs as delegate for user
	env := MustBuild(t,
		build.Transaction().For(user, "tokens").
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(service, "book", "1").Version(1).Timestamp(2).
			Delegator(user.JoinPath("book", "1")).
			PrivateKey(serviceKey))

	st = sim.SubmitTxnSuccessfully(env)
	sim.StepN(10)

	// Service also signs as authority (same key, different role)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(env.Transaction[0]).
			Url(service, "book", "1").Version(1).Timestamp(3).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	sim.StepN(10)

	// === VERIFY SUCCESS ===
	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()

	require.Equal(t, int64(10*AcmePrecision), userInitial-userFinal, "User paid 10 ACME transfer")
	require.Equal(t, int64(10*AcmePrecision), recipientBalance, "Recipient received transfer")

	t.Log("SUCCESS: Combined model - service signed as both delegate AND authority")
	t.Log("NOTE: In production, the service would include UserFees in the transaction")
	_ = userKey
}

// TestModel4_Combined_UserCannotBypass demonstrates that the user
// CANNOT bypass the service in the combined model.
func TestModel4_Combined_UserCannotBypass(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(user), &TokenAccount{Url: user.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)

	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Add service as authority
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			UpdateAccountAuth().
			Add(service.JoinPath("book")).
			SignWith(user, "book", "1").Version(1).Timestamp(1).PrivateKey(userKey))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	r := sim.QueryTransaction(st.TxID, nil)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(r.Message.Transaction).
			Url(service, "book", "1").Version(1).Timestamp(1).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Add delegation
	UpdateAccount(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: service.JoinPath("book")})
	})

	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()

	// === USER TRIES TO BYPASS ===
	t.Log("User attempts to bypass service by signing directly without fee...")

	env := MustBuild(t,
		build.Transaction().For(user, "tokens").
			// NO FEE - attempting bypass!
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(user, "book", "1").Version(1).Timestamp(3).PrivateKey(userKey))

	st = sim.SubmitTxnSuccessfully(env)
	sim.StepN(10)

	// Service sees no fee - refuses to co-sign
	t.Log("Service: No fee detected - REFUSING to co-sign")

	sim.StepN(50)

	// === VERIFY BLOCKED ===
	record := sim.QueryTransaction(st.TxID, nil)
	require.False(t, record.Status.Delivered(), "Transaction should NOT execute")

	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, userInitial, userFinal, "User balance unchanged - bypass BLOCKED")

	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, int64(0), recipientBalance, "Recipient got nothing - bypass BLOCKED")

	t.Log("SUCCESS: Combined model prevents user bypass - service co-signature required")
	_ = serviceKey
}

// =============================================================================
// MODEL 3 + USER FEES: Delegation with UserFee enforcement
// =============================================================================
//
// Demonstrates that delegation alone does NOT enforce fees — a user with
// delegation can still sign directly without including a UserFee.
// This test confirms the expected behavior: delegation is for convenience,
// not enforcement.

// TestModel3_Delegation_WithUserFees demonstrates delegation combined with
// UserFees. The service signs as delegate and includes a fee. The test also
// confirms the user can bypass fees when only delegation is configured.
func TestModel3_Delegation_WithUserFees(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Setup user with tokens and credits
	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(user), &TokenAccount{Url: user.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	// Setup service
	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(service), &TokenAccount{Url: service.JoinPath("fees"), TokenUrl: AcmeUrl()})

	// Setup recipient
	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Grant delegation to service
	UpdateAccount(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: service.JoinPath("book")})
	})

	// === PART A: Service signs as delegate WITH UserFee ===
	t.Log("Part A: Service creates transaction with UserFee via delegation...")

	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	feeAmount := big.NewInt(1 * AcmePrecision)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			UserFee(service.JoinPath("fees"), feeAmount, AcmeUrl(), user.JoinPath("tokens")).
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(service, "book", "1").Version(1).Timestamp(1).
			Delegator(user.JoinPath("book", "1")).
			PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	sim.StepN(50)

	// Verify user paid transfer + fee
	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	expectedDecrease := int64(10*AcmePrecision) + feeAmount.Int64()
	require.Equal(t, expectedDecrease, userInitial-userFinal,
		"User should have paid transfer + service fee")

	// Verify service received fee
	serviceFees := GetAccount[*TokenAccount](t, sim.DatabaseFor(service), service.JoinPath("fees")).Balance.Int64()
	require.Equal(t, feeAmount.Int64(), serviceFees,
		"Service should have received the fee")

	t.Log("Part A SUCCESS: Delegation + UserFee works correctly")

	// === PART B: User signs directly WITHOUT fee (bypass) ===
	t.Log("Part B: User bypasses service by signing directly without fee...")

	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			// No UserFee — bypassing!
			SendTokens(5, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(user, "book", "1").Version(1).Timestamp(2).PrivateKey(userKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// User CAN bypass — delegation alone does not enforce fees
	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, int64(15*AcmePrecision), recipientBalance,
		"Recipient should have received both transfers (10+5)")

	t.Log("Part B CONFIRMED: Delegation alone does NOT enforce fees")
	t.Log("For enforcement, use Model 1 or Model 2 (multi-authority)")
}

// =============================================================================
// MODEL 4 + USER FEES: Combined (Multi-Authority + Delegation) with UserFees
// =============================================================================
//
// Demonstrates the strongest enforcement model: service is both an authority
// (enforcement) and a delegate (convenience), with UserFees.
// The user CANNOT bypass fees because the service must co-sign as authority.

// TestModel4_Combined_WithUserFees demonstrates the combined model with
// service fees. The service creates a transaction with a UserFee on behalf
// of the user, signs as delegate AND as authority.
func TestModel4_Combined_WithUserFees(t *testing.T) {
	user := url.MustParse("user")
	service := url.MustParse("service")
	recipient := url.MustParse("recipient")

	userKey := acctesting.GenerateKey(user)
	serviceKey := acctesting.GenerateKey(service)
	recipientKey := acctesting.GenerateKey(recipient)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Setup user
	MakeIdentity(t, sim.DatabaseFor(user), user, userKey[32:])
	CreditCredits(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(user), &TokenAccount{Url: user.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(user), user.JoinPath("tokens"), big.NewInt(100*AcmePrecision))

	// Setup service
	MakeIdentity(t, sim.DatabaseFor(service), service, serviceKey[32:])
	CreditCredits(t, sim.DatabaseFor(service), service.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(service), &TokenAccount{Url: service.JoinPath("fees"), TokenUrl: AcmeUrl()})

	// Setup recipient
	MakeIdentity(t, sim.DatabaseFor(recipient), recipient, recipientKey[32:])
	MakeAccount(t, sim.DatabaseFor(recipient), &TokenAccount{Url: recipient.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Step 1: Add service as authority on token account (ENFORCEMENT)
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(user, "tokens").
			UpdateAccountAuth().
			Add(service.JoinPath("book")).
			SignWith(user, "book", "1").Version(1).Timestamp(1).PrivateKey(userKey))

	sim.StepUntil(Txn(st.TxID).IsPending())

	r := sim.QueryTransaction(st.TxID, nil)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(r.Message.Transaction).
			Url(service, "book", "1").Version(1).Timestamp(1).PrivateKey(serviceKey))

	sim.StepUntil(Txn(st.TxID).Succeeds())

	// Step 2: Add service as delegate on user's key page (CONVENIENCE)
	UpdateAccount(t, sim.DatabaseFor(user), user.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: service.JoinPath("book")})
	})

	userInitial := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()

	// === SERVICE CREATES TRANSACTION WITH FEE ===
	t.Log("Service creates transaction with UserFee on behalf of user...")

	feeAmount := big.NewInt(1 * AcmePrecision)

	// Service signs as delegate for user (initiator) with UserFee
	env := MustBuild(t,
		build.Transaction().For(user, "tokens").
			UserFee(service.JoinPath("fees"), feeAmount, AcmeUrl(), user.JoinPath("tokens")).
			SendTokens(10, AcmePrecisionPower).To(recipient, "tokens").
			SignWith(service, "book", "1").Version(1).Timestamp(2).
			Delegator(user.JoinPath("book", "1")).
			PrivateKey(serviceKey))

	st = sim.SubmitTxnSuccessfully(env)
	sim.StepN(10)

	// Service also signs as authority (co-signature for enforcement)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(env.Transaction[0]).
			Url(service, "book", "1").Version(1).Timestamp(3).PrivateKey(serviceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	sim.StepN(50)

	// === VERIFY ===
	userFinal := GetAccount[*TokenAccount](t, sim.DatabaseFor(user), user.JoinPath("tokens")).Balance.Int64()
	expectedDecrease := int64(10*AcmePrecision) + feeAmount.Int64()
	require.Equal(t, expectedDecrease, userInitial-userFinal,
		"User should have paid transfer + service fee")

	recipientBalance := GetAccount[*TokenAccount](t, sim.DatabaseFor(recipient), recipient.JoinPath("tokens")).Balance.Int64()
	require.Equal(t, int64(10*AcmePrecision), recipientBalance,
		"Recipient should have received 10 ACME")

	serviceFees := GetAccount[*TokenAccount](t, sim.DatabaseFor(service), service.JoinPath("fees")).Balance.Int64()
	require.Equal(t, feeAmount.Int64(), serviceFees,
		"Service should have received 1 ACME fee")

	t.Log("SUCCESS: Combined model with UserFees - service enforced fee AND signed on user's behalf")
	_ = userKey
}
