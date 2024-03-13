// Copyright 2024 The Accumulate Authors
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
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestLockAccount_LiteToken(t *testing.T) {
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	recipient := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Recipient"))

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateAccount(&LiteIdentity{Url: lite.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: lite, TokenUrl: AcmeUrl(), Balance: *big.NewInt(10)})

	// No lock, can send
	st, _ := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(lite).
			Body(&SendTokens{To: []*TokenRecipient{{Url: recipient, Amount: *big.NewInt(1)}}}).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey)),
	)...)
	require.False(t, st[0].Failed(), "Expected the transaction to succeed")
	require.Equal(t, int(1), int(simulator.GetAccount[*LiteTokenAccount](sim, recipient).Balance.Int64()))

	// Lock
	st, _ = sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(lite).
			Body(&LockAccount{Height: 10}).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey)),
	)...)
	require.False(t, st[0].Failed(), "Expected the transaction to succeed")
	require.Equal(t, int(10), int(simulator.GetAccount[*LiteTokenAccount](sim, lite).LockHeight))

	// Locked, cannot send
	st, err := sim.SubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(lite).
			Body(&SendTokens{To: []*TokenRecipient{{Url: recipient, Amount: *big.NewInt(1)}}}).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey)),
	)
	require.NoError(t, err)
	sim.H.StepUntil(harness.Txn(st[0].TxID).Capture(&st[0]).Fails())
	require.EqualError(t, st[0].AsError(), "account is locked until major block 10 (currently at 0)")

	// Fake a major block
	x := sim.PartitionFor(lite)
	_ = x.Database.Update(func(batch *database.Batch) error {
		entry, err := (&IndexEntry{BlockIndex: 10}).MarshalBinary()
		require.NoError(t, err)
		chain, err := batch.Account(x.Executor.Describe.AnchorPool()).MajorBlockChain().Get()
		require.NoError(t, err)
		require.NoError(t, chain.AddEntry(entry, false))
		return nil
	})

	// Lock expired, can send
	st = sim.H.BuildAndSubmitSuccessfully(
		build.Transaction().For(lite).
			SendTokens(1, 0).To(recipient).
			SignWith(lite).Version(1).Timestamp(&timestamp).PrivateKey(liteKey))

	sim.H.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).SingleCompletes())

	require.Equal(t, int(2), int(simulator.GetAccount[*LiteTokenAccount](sim, recipient).Balance.Int64()))
}

func TestLockAccount_LiteToken_WrongSigner(t *testing.T) {
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateAccount(&LiteIdentity{Url: lite.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: lite, TokenUrl: AcmeUrl(), Balance: *big.NewInt(10)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Attempt to lock
	envs := sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(lite).
			Body(&LockAccount{Height: 10}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)),
	)
	sim.WaitForTransaction(delivered, envs[0].Transaction[0].GetHash(), 50) // TODO How do we wait for the signature?

	// Verify nothing changed
	require.Zero(t, int(simulator.GetAccount[*LiteTokenAccount](sim, lite).LockHeight))
}
