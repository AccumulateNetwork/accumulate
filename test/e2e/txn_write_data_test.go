package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestWriteData_ToState(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Write data
	entry := &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar")}}
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
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

	// Check the result
	account := simulator.GetAccount[*DataAccount](sim, alice.JoinPath("data"))
	require.NotNil(t, account.Entry)
	require.True(t, EqualDataEntry(entry, account.Entry), "Account entry does not match")
}

func TestWriteData_Factom(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Write data
	entry := &FactomDataEntry{AccountId: [32]byte{1}, Data: []byte("foo"), ExtIds: [][]byte{[]byte("bar")}}
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{
				Entry:        entry.Wrap(),
				WriteToState: true,
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Check the result
	account := simulator.GetAccount[*DataAccount](sim, alice.JoinPath("data"))
	require.NotNil(t, account.Entry)
	require.True(t, EqualDataEntry(entry.Wrap(), account.Entry), "Account entry does not match")
}

func TestWriteData_EntryHash(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Add credits
	entry := &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar")}}
	st, _ := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{Entry: entry}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Check the result
	require.Len(t, st, 1)
	require.IsType(t, (*WriteDataResult)(nil), st[0].Result)
	result := st[0].Result.(*WriteDataResult)
	require.Equal(t, entry.Hash(), result.EntryHash[:])

	_ = sim.PartitionFor(alice).View(func(batch *database.Batch) error {
		data := batch.Account(alice.JoinPath("data")).Data()
		entryHash, err := data.Entry().Get(0)
		require.NoError(t, err)
		require.Equal(t, entry.Hash(), entryHash[:])

		txnHash, err := data.Transaction(entryHash).Get()
		require.NoError(t, err)
		require.Equal(t, st[0].TxID.Hash(), txnHash)

		return nil
	})
}
