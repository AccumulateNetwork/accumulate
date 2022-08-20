package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/helpers"
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

	// Add credits
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

	// The balance should be added
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

	// Add credits
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

	// The balance should be added
	account := simulator.GetAccount[*DataAccount](sim, alice.JoinPath("data"))
	require.NotNil(t, account.Entry)
	require.True(t, EqualDataEntry(entry.Wrap(), account.Entry), "Account entry does not match")
}
