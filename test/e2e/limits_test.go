// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func urlSliceStrings(v []*url.URL) []string {
	s := make([]string, len(v))
	for i, v := range v {
		s[i] = v.String()
	}
	return s
}

func TestCreateKeyPage_LimitBookPages(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.BookPages = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("book")).
			Body(&CreateKeyPage{Keys: []*KeySpecParams{{KeyHash: hash([]byte{1})}}}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "book will have too many pages")
}

func TestCreateKeyPage_LimitPageEntries(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.PageEntries = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("book")).
			Body(&CreateKeyPage{Keys: []*KeySpecParams{{KeyHash: hash([]byte{1})}, {KeyHash: hash([]byte{2})}}}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "page will have too many entries")
}

func TestUpdateKeyPage_LimitPageEntries(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.PageEntries = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("book", "1")).
			Body(&UpdateKeyPage{Operation: []KeyPageOperation{
				&AddKeyOperation{Entry: KeySpecParams{KeyHash: hash([]byte{1})}},
			}}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "page will have too many entries")
}

func TestUpdateAccountAuth_LimitAccountAuthorities(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.AccountAuthorities = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeKeyBook(t, sim.DatabaseFor(alice), alice.JoinPath("book2"), aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book2", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&UpdateAccountAuth{Operations: []AccountAuthOperation{
				&AddAccountAuthorityOperation{Authority: alice.JoinPath("book2")},
			}}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey).
			SignWith(alice.JoinPath("book2", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "account will have too many authorities")
}

func TestWriteData_LimitDataEntryParts(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.DataEntryParts = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	// Execute
	entry := new(DoubleHashDataEntry)
	entry.Data = [][]byte{{1}, {2}}
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("data")).
			Body(&WriteData{Entry: entry}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "data entry contains too many parts")
}

func TestCreateIdentity_LimitIdentityAccounts(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.IdentityAccounts = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateIdentity{Url: alice.JoinPath("account"), KeyHash: make([]byte, 32), KeyBookUrl: alice.JoinPath("account", "book")}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "identity would have too many accounts")
}

// TestCreateIdentity_Directory verifies that CreateIdentity correctly populates
// the directory index because that impacts the enforcement of the account
// limit.
func TestCreateIdentity_Directory(t *testing.T) {
	t.Run("Root", func(t *testing.T) {
		alice := AccountUrl("alice")
		liteKey := acctesting.GenerateKey("lite")
		lite := acctesting.AcmeLiteAddressStdPriv(liteKey)

		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 3),
			simulator.Genesis(GenesisTime),
		)

		MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
		CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)

		// Execute
		st := sim.SubmitTxnSuccessfully(
			MustBuild(t, build.Transaction().
				For(alice).
				Body(&CreateIdentity{Url: alice, KeyHash: make([]byte, 32), KeyBookUrl: alice.JoinPath("book")}).
				SignWith(lite).Version(1).Timestamp(1).PrivateKey(liteKey)),
		)

		sim.StepUntil(
			Txn(st.TxID).Succeeds())

		// Make sure CreateIdentity doesn't add the ADI to its own directory
		View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
			dir, err := batch.Account(alice).Directory().Get()
			require.NoError(t, err)
			require.EqualValues(t, []string{alice.JoinPath("book").String()}, urlSliceStrings(dir))
		})
	})

	t.Run("Sub", func(t *testing.T) {
		alice := AccountUrl("alice")
		aliceKey := acctesting.GenerateKey(alice)

		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 3),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

		// Execute
		st := sim.SubmitTxnSuccessfully(
			MustBuild(t, build.Transaction().
				For(alice).
				Body(&CreateIdentity{Url: alice.JoinPath("account"), KeyHash: make([]byte, 32), KeyBookUrl: alice.JoinPath("account", "book")}).
				SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
		)

		sim.StepUntil(
			Txn(st.TxID).Succeeds())

		// Make sure CreateIdentity doesn't add the ADI to its own directory
		View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
			dir, err := batch.Account(alice.JoinPath("account")).Directory().Get()
			require.NoError(t, err)
			require.EqualValues(t, []string{alice.JoinPath("account", "book").String()}, urlSliceStrings(dir))
		})
	})
}

func TestCreateTokenAccount_LimitIdentityAccounts(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.IdentityAccounts = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateTokenAccount{Url: alice.JoinPath("account"), TokenUrl: AcmeUrl()}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "identity would have too many accounts")
}

func TestCreateDataAccount_LimitIdentityAccounts(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.IdentityAccounts = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateDataAccount{Url: alice.JoinPath("account")}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "identity would have too many accounts")
}

func TestCreateToken_LimitIdentityAccounts(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.IdentityAccounts = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateToken{Url: alice.JoinPath("account")}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "identity would have too many accounts")
}

func TestCreateKeyBook_LimitIdentityAccounts(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.Limits = new(NetworkLimits)
	globals.Globals.Limits.IdentityAccounts = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateKeyBook{Url: alice.JoinPath("account"), PublicKeyHash: make([]byte, 32)}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)
	sim.StepUntil(Txn(st.TxID).Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "identity would have too many accounts")
}
