// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
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

func TestWriteData_AdiAccumulateEntryHash(t *testing.T) {
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

	// Write second entry
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

func TestWriteData_LiteAccumulateEntryHash(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })

	entry1 := &FactomDataEntry{ExtIds: [][]byte{[]byte("foo"), []byte("bar")}}
	entry1.AccountId = *(*[32]byte)(protocol.ComputeLiteDataAccountId(entry1.Wrap()))
	ldaAddr, err := protocol.LiteDataAddress(entry1.AccountId[:])
	require.NoError(t, err)
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(ldaAddr).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{Entry: entry1.Wrap()}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Write second entry
	entry2 := &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar")}}
	st, _ := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(ldaAddr).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{Entry: entry2}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Check the result
	require.Len(t, st, 1)
	require.IsType(t, (*WriteDataResult)(nil), st[0].Result)
	result := st[0].Result.(*WriteDataResult)
	require.Equal(t, entry2.Hash(), result.EntryHash[:])

	_ = sim.PartitionFor(alice).View(func(batch *database.Batch) error {
		data := batch.Account(ldaAddr).Data()
		entryHash, err := data.Entry().Get(1)
		require.NoError(t, err)
		require.Equal(t, entry2.Hash(), entryHash[:])

		txnHash, err := data.Transaction(entryHash).Get()
		require.NoError(t, err)
		require.Equal(t, st[0].TxID.Hash(), txnHash)

		return nil
	})
}

func TestWriteData_AdiFactomEntryHash(t *testing.T) {
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

	// Write second entry
	entry := &FactomDataEntry{ExtIds: [][]byte{[]byte("foo"), []byte("bar")}, AccountId: [32]byte{1}}
	st, _ := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{Entry: entry.Wrap()}).
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

func TestWriteData_LiteFactomEntryHash(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })

	entry1 := &FactomDataEntry{ExtIds: [][]byte{[]byte("foo"), []byte("bar")}}
	entry1.AccountId = *(*[32]byte)(protocol.ComputeLiteDataAccountId(entry1.Wrap()))
	ldaAddr, err := protocol.LiteDataAddress(entry1.AccountId[:])
	require.NoError(t, err)
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(ldaAddr).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{Entry: entry1.Wrap()}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Write second entry
	entry2 := &FactomDataEntry{ExtIds: [][]byte{[]byte("foo"), []byte("bar")}, AccountId: entry1.AccountId}
	st, _ := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(ldaAddr).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&WriteData{Entry: entry2.Wrap()}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Check the result
	require.Len(t, st, 1)
	require.IsType(t, (*WriteDataResult)(nil), st[0].Result)
	result := st[0].Result.(*WriteDataResult)
	require.Equal(t, entry2.Hash(), result.EntryHash[:])

	_ = sim.PartitionFor(alice).View(func(batch *database.Batch) error {
		data := batch.Account(ldaAddr).Data()
		entryHash, err := data.Entry().Get(1)
		require.NoError(t, err)
		require.Equal(t, entry2.Hash(), entryHash[:])

		txnHash, err := data.Transaction(entryHash).Get()
		require.NoError(t, err)
		require.Equal(t, st[0].TxID.Hash(), txnHash)

		return nil
	})
}
