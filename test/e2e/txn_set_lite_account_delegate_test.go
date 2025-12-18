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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestSetLiteAccountDelegate_LiteToken_SetDelegate(t *testing.T) {
	// Setup: Create a lite token account and an ADI with a key book
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	recipient := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Recipient"))
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateAccount(&LiteIdentity{Url: lite.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: lite, TokenUrl: AcmeUrl(), Balance: *big.NewInt(100)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Verify owner can send tokens initially
	st := sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			SendTokens(10, 0).To(recipient).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).LiteCompletes())

	require.Equal(t, int64(10), simulator.GetAccount[*LiteTokenAccount](sim, recipient).Balance.Int64())

	// Set delegation to alice's key book
	st = sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			Body(&SetLiteAccountDelegate{Delegate: alice.JoinPath("book")}).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).LiteCompletes())

	// Verify delegate is set
	liteAccount := simulator.GetAccount[*LiteTokenAccount](sim, lite)
	require.NotNil(t, liteAccount.Delegate)
	require.True(t, liteAccount.Delegate.Equal(alice.JoinPath("book")))

	// Now the delegate (alice) can send tokens
	st = sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			SendTokens(10, 0).To(recipient).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes())

	require.Equal(t, int64(20), simulator.GetAccount[*LiteTokenAccount](sim, recipient).Balance.Int64())
}

func TestSetLiteAccountDelegate_LiteToken_OwnerLockedOut(t *testing.T) {
	// Setup: Create a lite token account and an ADI with a key book
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	recipient := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Recipient"))
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateAccount(&LiteIdentity{Url: lite.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: lite, TokenUrl: AcmeUrl(), Balance: *big.NewInt(100)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Set delegation to alice's key book
	st := sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			Body(&SetLiteAccountDelegate{Delegate: alice.JoinPath("book")}).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).LiteCompletes())

	// Owner should NOT be able to send tokens anymore
	// The user signature is accepted but the authority signature should fail
	st = sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			SendTokens(10, 0).To(recipient).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey))

	sim.H.StepUntil(
		Sig(st[1].TxID).AuthoritySignature().Fails().
			WithError(errors.Unauthorized))

	// Verify no tokens were transferred (source account still has all tokens)
	liteAccount := simulator.GetAccount[*LiteTokenAccount](sim, lite)
	require.Equal(t, int64(100), liteAccount.Balance.Int64(), "lite account should still have 100 tokens")
}

func TestSetLiteAccountDelegate_LiteToken_ClearDelegate(t *testing.T) {
	// Setup: Create a lite token account and an ADI with a key book
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	recipient := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Recipient"))
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateAccount(&LiteIdentity{Url: lite.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: lite, TokenUrl: AcmeUrl(), Balance: *big.NewInt(100)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Set delegation to alice's key book
	st := sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			Body(&SetLiteAccountDelegate{Delegate: alice.JoinPath("book")}).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).LiteCompletes())

	// Delegate clears delegation (sets to nil)
	st = sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			Body(&SetLiteAccountDelegate{Delegate: nil}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes())

	// Verify delegate is cleared
	liteAccount := simulator.GetAccount[*LiteTokenAccount](sim, lite)
	require.Nil(t, liteAccount.Delegate)

	// Owner can now send tokens again
	st = sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			SendTokens(10, 0).To(recipient).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).LiteCompletes())

	require.Equal(t, int64(10), simulator.GetAccount[*LiteTokenAccount](sim, recipient).Balance.Int64())
}

func TestSetLiteAccountDelegate_LiteToken_DelegateTransfer(t *testing.T) {
	// Setup: Create a lite token account and two ADIs with key books
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateAccount(&LiteIdentity{Url: lite.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: lite, TokenUrl: AcmeUrl(), Balance: *big.NewInt(100)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	sim.CreateIdentity(bob, bobKey[32:])
	updateAccount(sim, bob.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Set delegation to alice's key book
	st := sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			Body(&SetLiteAccountDelegate{Delegate: alice.JoinPath("book")}).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).LiteCompletes())

	// Alice transfers delegation to Bob
	st = sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			Body(&SetLiteAccountDelegate{Delegate: bob.JoinPath("book")}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes())

	// Verify delegate is now Bob
	liteAccount := simulator.GetAccount[*LiteTokenAccount](sim, lite)
	require.NotNil(t, liteAccount.Delegate)
	require.True(t, liteAccount.Delegate.Equal(bob.JoinPath("book")))
}

func TestSetLiteAccountDelegate_InvalidDelegate(t *testing.T) {
	// Setup: Create a lite token account. Use a 1-partition simulator to ensure
	// all accounts are local, so the delegate validation happens at execution time.
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize with 1 partition to ensure all accounts are local
	var timestamp uint64
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	sim.CreateAccount(&LiteIdentity{Url: lite.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: lite, TokenUrl: AcmeUrl(), Balance: *big.NewInt(100)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Try to set delegate to a token account (should fail because delegate must be a KeyBook)
	sim.CreateAccount(&TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(100), AccountAuth: AccountAuth{
		Authorities: []AuthorityEntry{{Url: alice.JoinPath("book")}},
	}})

	envs := sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(lite).
			Body(&SetLiteAccountDelegate{Delegate: alice.JoinPath("tokens")}).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey)),
	)
	sim.H.StepUntil(Txn(envs[0].Transaction[0].ID()).Fails())
}

func TestSetLiteAccountDelegate_InvalidPrincipal(t *testing.T) {
	// Setup: Try to set delegate on an ADI token account (should fail)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(100), AccountAuth: AccountAuth{
		Authorities: []AuthorityEntry{{Url: alice.JoinPath("book")}},
	}})

	// Try to set delegate on ADI token account (should fail - only lite accounts supported)
	envs := sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("tokens")).
			Body(&SetLiteAccountDelegate{Delegate: alice.JoinPath("book")}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)),
	)
	sim.H.StepUntil(Txn(envs[0].Transaction[0].ID()).Fails())
}
