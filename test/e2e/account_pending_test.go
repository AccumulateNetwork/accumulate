package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestAccountPending(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice, "main")
	otherKey := acctesting.GenerateKey(alice, "other")

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AcceptThreshold = 2
		page.AddKeySpec(&KeySpec{PublicKeyHash: hash(otherKey[32:])})
	})
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Write data
	entry := &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar")}}
	_, txns := sim.WaitForTransactions(pending, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{
				Entry:        entry,
				WriteToState: true,
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Verify the transaction is in the account's pending list after processing
	// and is remains there after 50 blocks
	require.Contains(t, getPendingAsString(sim, alice.JoinPath("data")), txns[0].ID().ShortString(), "Expected transaction to be in alice's pending list immediately")
	sim.ExecuteBlocks(50)
	require.Contains(t, getPendingAsString(sim, alice.JoinPath("data")), txns[0].ID().ShortString(), "Expected transaction to be in alice's pending list after 50 blocks")

	// Sign the transaction a second time
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithTransaction(txns[0]).
			WithSigner(alice.JoinPath("book", "1"), 1).
			Sign(SignatureTypeED25519, otherKey).
			Build(),
	)...)

	// Verify the transaction is no longer in the account's pending list after
	// delivery
	require.NotContains(t, getPendingAsString(sim, alice.JoinPath("data")), txns[0].ID().ShortString(), "Expected transaction not to be in alice's pending list after signing again")
}

func TestAccountPending_SingleSig(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice, "main")

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Write data
	entry := &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar")}}
	_, txns := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{
				Entry:        entry,
				WriteToState: true,
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Verify the transaction is not in the account's pending list
	require.NotContains(t, getPendingAsString(sim, alice.JoinPath("data")), txns[0].ID().ShortString(), "Expected transaction not to be in alice's pending list")
}
