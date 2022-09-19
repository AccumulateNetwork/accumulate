package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestCreateKeyPage_LimitBookPages(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.BookPages = 1

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.Submit(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("book")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&CreateKeyPage{Keys: []*KeySpecParams{{KeyHash: hash([]byte{1})}}}).
			Initiate(SignatureTypeED25519, aliceKey).
			BuildDelivery())
	require.NotNil(t, st.Error)
	require.EqualError(t, st.Error, "book will have too many pages")
}

func TestCreateKeyPage_LimitPageEntries(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.PageEntries = 1

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.Submit(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("book")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&CreateKeyPage{Keys: []*KeySpecParams{{KeyHash: hash([]byte{1})}, {KeyHash: hash([]byte{2})}}}).
			Initiate(SignatureTypeED25519, aliceKey).
			BuildDelivery())
	require.NotNil(t, st.Error)
	require.EqualError(t, st.Error, "page will have too many entries")
}

func TestUpdateKeyPage_LimitPageEntries(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.PageEntries = 1

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.Submit(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("book", "1")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&UpdateKeyPage{Operation: []KeyPageOperation{
				&AddKeyOperation{Entry: KeySpecParams{KeyHash: hash([]byte{1})}},
			}}).
			Initiate(SignatureTypeED25519, aliceKey).
			BuildDelivery())
	require.NotNil(t, st.Error)
	require.EqualError(t, st.Error, "page will have too many entries")
}

func TestUpdateAccountAuth_LimitAccountAuthorities(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.AccountAuthorities = 1

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeKeyBook(t, sim.DatabaseFor(alice), alice.JoinPath("book2"), aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book2", "1"), 1e9)

	// Execute
	st := sim.Submit(
		acctesting.NewTransaction().
			WithPrincipal(alice).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&UpdateAccountAuth{Operations: []AccountAuthOperation{
				&AddAccountAuthorityOperation{Authority: alice.JoinPath("book2")},
			}}).
			Initiate(SignatureTypeED25519, aliceKey).
			WithSigner(alice.JoinPath("book2", "1"), 1).
			Sign(SignatureTypeED25519, aliceKey).
			BuildDelivery())
	require.NotNil(t, st.Error)
	require.EqualError(t, st.Error, "account will have too many authorities")
}

func TestWriteData_LimitDataEntryParts(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.DataEntryParts = 1

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	// Execute
	entry := new(AccumulateDataEntry)
	entry.Data = [][]byte{{1}, {2}}
	st := sim.Submit(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&WriteData{Entry: entry}).
			Initiate(SignatureTypeED25519, aliceKey).
			BuildDelivery())
	require.NotNil(t, st.Error)
	require.EqualError(t, st.Error, "data entry contains too many parts")
}
