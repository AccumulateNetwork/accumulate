package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/helpers"
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
		acctesting.NewTransaction().
			WithPrincipal(lite).
			WithSigner(lite, 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{To: []*TokenRecipient{{Url: recipient, Amount: *big.NewInt(1)}}}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)
	require.False(t, st[0].Failed(), "Expected the transaction to succeed")
	require.Equal(t, int(1), int(simulator.GetAccount[*LiteTokenAccount](sim, recipient).Balance.Int64()))

	// Lock
	st, _ = sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(lite).
			WithSigner(lite, 1).
			WithTimestampVar(&timestamp).
			WithBody(&LockAccount{Height: 10}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)
	require.False(t, st[0].Failed(), "Expected the transaction to succeed")
	require.Equal(t, int(10), int(simulator.GetAccount[*LiteTokenAccount](sim, lite).LockHeight))

	// Locked, cannot send
	_, err := sim.SubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(lite).
			WithSigner(lite, 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{To: []*TokenRecipient{{Url: recipient, Amount: *big.NewInt(1)}}}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)
	require.Error(t, err)

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
	st, _ = sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(lite).
			WithSigner(lite, 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{To: []*TokenRecipient{{Url: recipient, Amount: *big.NewInt(1)}}}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)
	require.False(t, st[0].Failed(), "Expected the transaction to succeed")
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
		acctesting.NewTransaction().
			WithPrincipal(lite).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&LockAccount{Height: 10}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)
	sim.WaitForTransaction(delivered, envs[0].Transaction[0].GetHash(), 50) // TODO How do we wait for the signature?

	// Verify nothing changed
	require.Zero(t, int(simulator.GetAccount[*LiteTokenAccount](sim, lite).LockHeight))
}
