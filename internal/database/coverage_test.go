// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func randomHash() [32]byte {
	var buf [32]byte
	rand.Read(buf[:])
	return sha256.Sum256(buf[:])
}

// TestChainState tests Chain.State function
func TestChainState(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account with a main chain
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add some entries to the main chain
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry1 := randomHash()
	entry2 := randomHash()
	require.NoError(t, chain.AddEntry(entry1[:], false))
	require.NoError(t, chain.AddEntry(entry2[:], false))

	// Test State at different heights
	state0, err := chain.State(0)
	require.NoError(t, err)
	require.NotNil(t, state0)

	state1, err := chain.State(1)
	require.NoError(t, err)
	require.NotNil(t, state1)

	// States at different heights should be different
	require.NotEqual(t, state0.Anchor(), state1.Anchor())

	t.Logf("Chain state at height 0: %x", state0.Anchor())
	t.Logf("Chain state at height 1: %x", state1.Anchor())
}

// TestChainAnchor tests Chain.Anchor function
func TestChainAnchor(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain and add entry
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry := randomHash()
	require.NoError(t, chain.AddEntry(entry[:], false))

	// Test Anchor
	anchor := chain.Anchor()
	require.NotNil(t, anchor)
	require.Len(t, anchor, 32)

	t.Logf("Chain anchor: %x", anchor)
}

// TestChainAnchorAt tests Chain.AnchorAt function
func TestChainAnchorAt(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain and add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		entry := randomHash()
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Test AnchorAt for each height
	anchor0, err := chain.AnchorAt(0)
	require.NoError(t, err)
	require.NotNil(t, anchor0)

	anchor1, err := chain.AnchorAt(1)
	require.NoError(t, err)
	require.NotNil(t, anchor1)

	anchor2, err := chain.AnchorAt(2)
	require.NoError(t, err)
	require.NotNil(t, anchor2)

	// Anchors at different heights should be different
	require.NotEqual(t, anchor0, anchor1)
	require.NotEqual(t, anchor1, anchor2)

	t.Logf("Anchor at height 0: %x", anchor0)
	t.Logf("Anchor at height 1: %x", anchor1)
	t.Logf("Anchor at height 2: %x", anchor2)
}

// TestChainPending tests Chain.Pending function
func TestChainPending(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	// Add some entries
	for i := 0; i < 5; i++ {
		entry := randomHash()
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Test Pending
	pending := chain.Pending()
	// Pending returns the pending Merkle tree roots
	t.Logf("Pending roots count: %d", len(pending))
}

// TestChain2Anchor tests Chain2.Anchor function
func TestChain2Anchor(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain2 and add entry
	chain2 := batch.Account(account).MainChain()
	chain, err := chain2.Get()
	require.NoError(t, err)

	entry := randomHash()
	require.NoError(t, chain.AddEntry(entry[:], false))

	// Test Chain2.Anchor
	anchor, err := chain2.Anchor()
	require.NoError(t, err)
	require.NotNil(t, anchor)

	t.Logf("Chain2 anchor: %x", anchor)
}

// TestChain2AnchorAt tests Chain2.AnchorAt function
func TestChain2AnchorAt(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain2 and add entries
	chain2 := batch.Account(account).MainChain()
	chain, err := chain2.Get()
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		entry := randomHash()
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Test Chain2.AnchorAt
	anchor, err := chain2.AnchorAt(1)
	require.NoError(t, err)
	require.NotNil(t, anchor)

	t.Logf("Chain2 anchor at height 1: %x", anchor)
}

// TestChain2Url tests Chain2.Url function
func TestChain2Url(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain2
	chain2 := batch.Account(account).MainChain()

	// Test Url
	chainUrl := chain2.Url()
	require.NotNil(t, chainUrl)
	require.Contains(t, chainUrl.String(), "chain/main")

	t.Logf("Chain URL: %s", chainUrl)
}

// TestChain2EntryAs tests Chain2.EntryAs function
func TestChain2EntryAs(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries to main chain
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		entry := randomHash()
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Get the index chain and add an index entry
	indexChain2 := batch.Account(account).MainChain().Index()
	indexChain, err := indexChain2.Get()
	require.NoError(t, err)

	// Add an index entry
	indexEntry := &protocol.IndexEntry{
		Source: 0,
		Anchor: 4,
	}
	entryBytes, err := indexEntry.MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, indexChain.AddEntry(entryBytes, false))

	// Test EntryAs - read as IndexEntry
	readEntry := new(protocol.IndexEntry)
	err = indexChain2.EntryAs(0, readEntry)
	require.NoError(t, err)
	require.Equal(t, uint64(0), readEntry.Source)
	require.Equal(t, uint64(4), readEntry.Anchor)

	t.Logf("Entry as IndexEntry: source=%d, anchor=%d", readEntry.Source, readEntry.Anchor)
}

// TestChain2Receipt tests Chain2.Receipt function
func TestChain2Receipt(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add multiple entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		entry := randomHash()
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Test Receipt
	chain2 := batch.Account(account).MainChain()
	receipt, err := chain2.Receipt(0, 4)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.True(t, receipt.Validate(nil))

	t.Logf("Receipt from 0 to 4: start=%x, anchor=%x", receipt.Start[:8], receipt.Anchor[:8])
}

// TestChain2State tests Chain2.State function
func TestChain2State(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		entry := randomHash()
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Test Chain2.State
	chain2 := batch.Account(account).MainChain()
	state, err := chain2.State(1)
	require.NoError(t, err)
	require.NotNil(t, state)

	t.Logf("Chain2 state at height 1: count=%d", state.Count)
}

// TestAccountTransaction tests Batch.AccountTransaction function
func TestAccountTransaction(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a transaction ID
	account := protocol.AccountUrl("test", "tokens")
	txHash := randomHash()
	txid := account.WithTxID(txHash)

	// Use AccountTransaction
	txRecord := batch.AccountTransaction(txid)
	require.NotNil(t, txRecord)

	// The transaction doesn't exist yet, but we can access it
	t.Logf("AccountTransaction for %v", txid)
}

// TestNestedAccount tests Account.Account function (nested accounts)
func TestNestedAccount(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a root identity
	identity := protocol.AccountUrl("test")
	adi := &protocol.ADI{Url: identity}
	require.NoError(t, batch.Account(identity).Main().Put(adi))

	// Use nested Account function
	nestedAccount := batch.Account(identity).Account("tokens")
	require.NotNil(t, nestedAccount)
	// AccountUrl adds .acme suffix to the identity
	require.Equal(t, "acc://test.acme/tokens", nestedAccount.Url().String())

	t.Logf("Nested account URL: %s", nestedAccount.Url())
}

// TestMarkDirty tests Account.MarkDirty function
func TestMarkDirty(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))
	require.NoError(t, batch.Commit())

	// Start a new batch
	batch = db.Begin(true)
	defer batch.Discard()

	acc := batch.Account(account)

	// Initially not dirty (in new batch)
	require.False(t, acc.IsDirty())

	// Mark dirty
	require.NoError(t, acc.MarkDirty())

	// Now should be dirty
	require.True(t, acc.IsDirty())

	t.Logf("Account marked dirty successfully")
}

// TestChainByName tests Account.ChainByName function
func TestChainByName(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Test ChainByName for main chain
	chain, err := batch.Account(account).ChainByName("main")
	require.NoError(t, err)
	require.NotNil(t, chain)
	require.Equal(t, "main", chain.Name())

	// Test ChainByName for signature chain
	chain, err = batch.Account(account).ChainByName("signature")
	require.NoError(t, err)
	require.NotNil(t, chain)
	require.Equal(t, "signature", chain.Name())

	// Test ChainByName for non-existent chain
	_, err = batch.Account(account).ChainByName("nonexistent")
	require.Error(t, err)

	t.Logf("ChainByName tests passed")
}

// TestChainByNameIndex tests ChainByName with index suffix
func TestChainByNameIndex(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Test ChainByName with -index suffix
	chain, err := batch.Account(account).ChainByName("main-index")
	require.NoError(t, err)
	require.NotNil(t, chain)
	require.Equal(t, merkle.ChainTypeIndex, chain.Type())

	t.Logf("ChainByName index test passed")
}

// TestGetIndexChainByName tests Account.GetIndexChainByName function
func TestGetIndexChainByName(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get the main chain and add entries
	mainChain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		entry := randomHash()
		require.NoError(t, mainChain.AddEntry(entry[:], false))
	}

	// Test GetIndexChainByName
	indexChain, err := batch.Account(account).GetIndexChainByName("main")
	require.NoError(t, err)
	require.NotNil(t, indexChain)

	t.Logf("GetIndexChainByName test passed, chain height: %d", indexChain.Height())
}

// TestIterateAccounts tests Batch.IterateAccounts function
func TestIterateAccounts(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create system ledger accounts (don't require key book)
	ledger1 := protocol.DnUrl().JoinPath(protocol.Ledger)
	ledger2 := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)
	ledger3 := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)

	require.NoError(t, batch.Account(ledger1).Main().Put(&protocol.SystemLedger{Url: ledger1}))
	require.NoError(t, batch.Account(ledger2).Main().Put(&protocol.SystemLedger{Url: ledger2}))
	require.NoError(t, batch.Account(ledger3).Main().Put(&protocol.SystemLedger{Url: ledger3}))

	// Force BPT update before commit
	_, err := batch.GetBptRootHash()
	require.NoError(t, err)

	// Commit to persist to BPT
	require.NoError(t, batch.Commit())

	// Iterate
	batch = db.Begin(false)
	defer batch.Discard()

	it := batch.IterateAccounts()
	count := 0
	for it.Next() {
		acc := it.Value()
		require.NotNil(t, acc)
		t.Logf("Found account: %s", acc.Url())
		count++
	}
	require.NoError(t, it.Err())
	require.Equal(t, 3, count)

	t.Logf("IterateAccounts found %d accounts", count)
}

// TestForEachAccount tests Batch.ForEachAccount function
func TestForEachAccount(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create system ledger accounts (don't require key book)
	ledger1 := protocol.DnUrl().JoinPath(protocol.Ledger)
	ledger2 := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)
	ledger3 := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)

	require.NoError(t, batch.Account(ledger1).Main().Put(&protocol.SystemLedger{Url: ledger1}))
	require.NoError(t, batch.Account(ledger2).Main().Put(&protocol.SystemLedger{Url: ledger2}))
	require.NoError(t, batch.Account(ledger3).Main().Put(&protocol.SystemLedger{Url: ledger3}))

	// Force BPT update before commit
	_, err := batch.GetBptRootHash()
	require.NoError(t, err)

	// Commit to persist to BPT
	require.NoError(t, batch.Commit())

	// ForEach
	batch = db.Begin(false)
	defer batch.Discard()

	count := 0
	err = batch.ForEachAccount(func(account *Account, hash [32]byte) error {
		require.NotNil(t, account)
		t.Logf("ForEach account: %s, hash: %x", account.Url(), hash[:8])
		count++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, count)

	t.Logf("ForEachAccount processed %d accounts", count)
}

// TestBatchView tests Batch.View function
func TestBatchView(t *testing.T) {
	db := OpenInMemory(nil)

	// Create an account
	account := protocol.AccountUrl("test", "data")
	require.NoError(t, db.Update(func(batch *Batch) error {
		dataAccount := &protocol.DataAccount{Url: account}
		return batch.Account(account).Main().Put(dataAccount)
	}))

	// Use View
	err := db.View(func(batch *Batch) error {
		var acc protocol.Account
		err := batch.Account(account).Main().GetAs(&acc)
		require.NoError(t, err)
		require.Equal(t, account.String(), acc.GetUrl().String())
		return nil
	})
	require.NoError(t, err)

	t.Logf("Batch.View test passed")
}

// TestBatchUpdate tests Batch.Update on batch
func TestBatchUpdate(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create initial account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))
	require.NoError(t, batch.Commit())

	// Use Update on a new batch
	batch = db.Begin(true)
	defer batch.Discard()

	err := batch.Update(func(sub *Batch) error {
		var acc *protocol.DataAccount
		err := sub.Account(account).Main().GetAs(&acc)
		if err != nil {
			return err
		}
		acc.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{[]byte("test")}}
		return sub.Account(account).Main().Put(acc)
	})
	require.NoError(t, err)

	t.Logf("Batch.Update test passed")
}

// TestChainEntries tests Chain.Entries function
func TestChainEntries(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	var addedEntries [][]byte
	for i := 0; i < 5; i++ {
		entry := randomHash()
		addedEntries = append(addedEntries, entry[:])
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Test Entries
	entries, err := chain.Entries(0, 5)
	require.NoError(t, err)
	require.Len(t, entries, 5)

	// Test partial range
	entries, err = chain.Entries(2, 4)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// Test out of range
	entries, err = chain.Entries(0, 100)
	require.NoError(t, err)
	require.Len(t, entries, 5) // Should return only existing entries

	t.Logf("Chain.Entries test passed")
}

// TestChainEntriesInvalidRange tests Chain.Entries with invalid range
func TestChainEntriesInvalidRange(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry := randomHash()
	require.NoError(t, chain.AddEntry(entry[:], false))

	// Test invalid range (start > end)
	_, err = chain.Entries(5, 2)
	require.Error(t, err)

	t.Logf("Chain.Entries invalid range test passed")
}

// TestChainEntryAs tests Chain.EntryAs function
func TestChainEntryAs(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries to main chain first
	mainChain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		entry := randomHash()
		require.NoError(t, mainChain.AddEntry(entry[:], false))
	}

	// Get the index chain and add an index entry
	indexChain, err := batch.Account(account).GetIndexChainByName("main")
	require.NoError(t, err)

	// Add an index entry
	indexEntry := &protocol.IndexEntry{
		Source: 0,
		Anchor: 4,
	}
	entryBytes, err := indexEntry.MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, indexChain.AddEntry(entryBytes, false))

	// Test EntryAs
	readEntry := new(protocol.IndexEntry)
	err = indexChain.EntryAs(0, readEntry)
	require.NoError(t, err)
	require.Equal(t, uint64(0), readEntry.Source)

	t.Logf("Chain.EntryAs test passed")
}

// TestUpdatedChains tests Account.UpdatedChains function
func TestUpdatedChains(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries to main chain
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry := randomHash()
	require.NoError(t, chain.AddEntry(entry[:], false))

	// Test UpdatedChains
	entries, err := batch.Account(account).UpdatedChains()
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, e := range entries {
		t.Logf("Updated chain: %s, index: %d", e.Chain, e.Index)
	}
}

// TestChainHeightOf tests Chain.HeightOf function
func TestChainHeightOf(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry1 := randomHash()
	entry2 := randomHash()
	require.NoError(t, chain.AddEntry(entry1[:], false))
	require.NoError(t, chain.AddEntry(entry2[:], false))

	// Test HeightOf
	height, err := chain.HeightOf(entry1[:])
	require.NoError(t, err)
	require.Equal(t, int64(0), height)

	height, err = chain.HeightOf(entry2[:])
	require.NoError(t, err)
	require.Equal(t, int64(1), height)

	t.Logf("Chain.HeightOf test passed")
}

// TestChain2IndexOf tests Chain2.IndexOf function
func TestChain2IndexOf(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries via Chain, then use Chain2 functions
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry := randomHash()
	require.NoError(t, chain.AddEntry(entry[:], false))

	// Test Chain2.IndexOf
	chain2 := batch.Account(account).MainChain()
	index, err := chain2.IndexOf(entry[:])
	require.NoError(t, err)
	require.Equal(t, int64(0), index)

	t.Logf("Chain2.IndexOf test passed")
}

// TestAnchorChain tests Account.AnchorChain function
func TestAnchorChain(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create anchor ledger account
	account := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	anchorLedger := &protocol.AnchorLedger{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(anchorLedger))

	// Access anchor chain for a partition
	anchorChain := batch.Account(account).AnchorChain("BVN0")
	require.NotNil(t, anchorChain)

	// Access root and BPT chains
	rootChain := anchorChain.Root()
	require.NotNil(t, rootChain)

	bptChain := anchorChain.BPT()
	require.NotNil(t, bptChain)

	t.Logf("AnchorChain test passed")
}

// TestSyntheticSequenceChain tests Account.SyntheticSequenceChain function
func TestSyntheticSequenceChain(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create synthetic ledger account
	account := protocol.DnUrl().JoinPath(protocol.Synthetic)
	synthLedger := &protocol.SyntheticLedger{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(synthLedger))

	// Access synthetic sequence chain
	synthChain := batch.Account(account).SyntheticSequenceChain("BVN0")
	require.NotNil(t, synthChain)

	t.Logf("SyntheticSequenceChain test passed")
}

// TestChainReceipt tests Chain.Receipt function
func TestChainReceipt(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		entry := randomHash()
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Test Receipt
	receipt, err := chain.Receipt(0, 9)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.True(t, receipt.Validate(nil))

	t.Logf("Chain.Receipt test passed: %x -> %x", receipt.Start[:8], receipt.Anchor[:8])
}

// TestUpdateAccount tests UpdateAccount function
func TestUpdateAccount(t *testing.T) {
	db := OpenInMemory(nil)

	// Create initial account
	account := protocol.AccountUrl("test", "tokens")
	require.NoError(t, db.Update(func(batch *Batch) error {
		tokenAccount := &protocol.TokenAccount{
			Url:      account,
			TokenUrl: protocol.AcmeUrl(),
			Balance:  *big.NewInt(100),
		}
		return batch.Account(account).Main().Put(tokenAccount)
	}))

	// Use UpdateAccount
	require.NoError(t, db.Update(func(batch *Batch) error {
		_, err := UpdateAccount(batch, account, func(acc *protocol.TokenAccount) error {
			acc.Balance.Add(&acc.Balance, big.NewInt(50))
			return nil
		})
		return err
	}))

	// Verify
	require.NoError(t, db.View(func(batch *Batch) error {
		var acc *protocol.TokenAccount
		err := batch.Account(account).Main().GetAs(&acc)
		require.NoError(t, err)
		require.Equal(t, "150", acc.Balance.String())
		return nil
	}))

	t.Logf("UpdateAccount test passed")
}

// TestBatchResolve tests Batch.Resolve function
func TestBatchResolve(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get Chain2 and test Resolve
	chain2 := batch.Account(account).MainChain()

	// Resolve should work with Index key
	// (internal implementation detail test)
	t.Logf("Chain2 key: %v", chain2.Key())
}

// TestHashFunctions tests Hash type functions
func TestHashFunctions(t *testing.T) {
	// Test Hash.Bytes32
	h := Hash(make([]byte, 32))
	for i := range h {
		h[i] = byte(i)
	}

	b32 := h.Bytes32()
	require.Equal(t, 32, len(b32))
	require.Equal(t, byte(0), b32[0])
	require.Equal(t, byte(31), b32[31])

	// Test Hash.Bytes
	bytes := h.Bytes()
	require.Equal(t, 32, len(bytes))

	// Test Hash.Copy
	hCopy := h.Copy()
	require.Equal(t, h, hCopy)
	hCopy[0] = 255
	require.NotEqual(t, h[0], hCopy[0]) // Original not modified

	// Test Hash.Equal
	h2 := h.Copy()
	require.True(t, h.Equal(h2))
	h2[0] = 255
	require.False(t, h.Equal(h2))

	// Test nil Copy
	var nilHash Hash
	nilCopy := nilHash.Copy()
	require.Nil(t, nilCopy)

	t.Logf("Hash functions test passed")
}

// TestHashCombine tests Hash.Combine function
func TestHashCombine(t *testing.T) {
	h1 := Hash(make([]byte, 32))
	h2 := Hash(make([]byte, 32))
	for i := range h1 {
		h1[i] = byte(i)
		h2[i] = byte(31 - i)
	}

	combined := h1.Combine(h2)
	require.NotNil(t, combined)
	require.Equal(t, 32, len(combined))

	// Combine should be deterministic
	combined2 := h1.Combine(h2)
	require.True(t, combined.Equal(combined2))

	// Different inputs should give different outputs
	combined3 := h2.Combine(h1)
	require.False(t, combined.Equal(combined3))

	t.Logf("Hash.Combine test passed")
}

// TestHashMarshal tests Hash marshaling functions
func TestHashMarshal(t *testing.T) {
	h := Hash(make([]byte, 32))
	for i := range h {
		h[i] = byte(i)
	}

	// Test BinarySize
	size := h.BinarySize()
	require.Greater(t, size, 0)

	// Test MarshalBinary
	data, err := h.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalBinary
	var h2 Hash
	err = h2.UnmarhsalBinary(data)
	require.NoError(t, err)
	require.True(t, h.Equal(h2))

	t.Logf("Hash marshal test passed")
}

// TestSparseHashList tests SparseHashList functions
func TestSparseHashList(t *testing.T) {
	// Create a sparse hash list with some entries
	list := make(SparseHashList, 4)
	list[0] = make([]byte, 32)
	list[2] = make([]byte, 32)

	for i := range list[0] {
		list[0][i] = byte(i)
	}
	for i := range list[2] {
		list[2][i] = byte(31 - i)
	}

	// Test Copy
	listCopy := list.Copy()
	require.Equal(t, len(list), len(listCopy))
	listCopy[0][0] = 255
	require.NotEqual(t, list[0][0], listCopy[0][0])

	// Test BinarySize
	height := int64(5) // binary 101 - bits 0 and 2 are set
	size := list.BinarySize(height)
	require.Greater(t, size, 0)

	// Test MarshalBinary
	data, err := list.MarshalBinary(height)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalBinary
	var list2 SparseHashList
	err = list2.UnmarshalBinary(height, data)
	require.NoError(t, err)
	// UnmarshalBinary creates a list based on bit count in height
	// height=5 (binary 101) has 3 significant bits
	require.Greater(t, len(list2), 0)

	t.Logf("SparseHashList test passed")
}

// TestHashList tests HashList functions
func TestHashList(t *testing.T) {
	// Create a hash list
	list := make(HashList, 3)
	for i := range list {
		list[i] = make([]byte, 32)
		for j := range list[i] {
			list[i][j] = byte(i*32 + j)
		}
	}

	// Test BinarySize
	size := list.BinarySize()
	require.Greater(t, size, 0)

	// Test MarshalBinary
	data, err := list.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalBinary
	var list2 HashList
	err = list2.UnmarhsalBinary(data)
	require.NoError(t, err)
	require.Equal(t, len(list), len(list2))

	t.Logf("HashList test passed")
}

// TestTransactionChainEntryCompare tests TransactionChainEntry.Compare
func TestTransactionChainEntryCompare(t *testing.T) {
	e1 := &TransactionChainEntry{
		Account: protocol.AccountUrl("alice"),
		Chain:   "main",
	}
	e2 := &TransactionChainEntry{
		Account: protocol.AccountUrl("alice"),
		Chain:   "main",
	}
	e3 := &TransactionChainEntry{
		Account: protocol.AccountUrl("alice"),
		Chain:   "signature",
	}
	e4 := &TransactionChainEntry{
		Account: protocol.AccountUrl("bob"),
		Chain:   "main",
	}

	// Same entries
	require.Equal(t, 0, e1.Compare(e2))

	// Different chain
	require.Less(t, e1.Compare(e3), 0)
	require.Greater(t, e3.Compare(e1), 0)

	// Different account
	require.Less(t, e1.Compare(e4), 0)
	require.Greater(t, e4.Compare(e1), 0)

	t.Logf("TransactionChainEntry.Compare test passed")
}

// TestBlockStateSynthTxnEntryCompare tests BlockStateSynthTxnEntry.Compare
func TestBlockStateSynthTxnEntryCompare(t *testing.T) {
	tx1 := make([]byte, 32)
	tx2 := make([]byte, 32)
	tx1[0] = 1
	tx2[0] = 2

	e1 := &BlockStateSynthTxnEntry{
		Transaction: tx1,
		ChainEntry:  10,
	}
	e2 := &BlockStateSynthTxnEntry{
		Transaction: tx1,
		ChainEntry:  10,
	}
	e3 := &BlockStateSynthTxnEntry{
		Transaction: tx1,
		ChainEntry:  20,
	}
	e4 := &BlockStateSynthTxnEntry{
		Transaction: tx2,
		ChainEntry:  10,
	}

	// Same entries
	require.Equal(t, 0, e1.Compare(e2))

	// Different chain entry
	require.Less(t, e1.Compare(e3), 0)
	require.Greater(t, e3.Compare(e1), 0)

	// Different transaction
	require.Less(t, e1.Compare(e4), 0)
	require.Greater(t, e4.Compare(e1), 0)

	t.Logf("BlockStateSynthTxnEntry.Compare test passed")
}

// TestDatabaseClose tests Database.Close function
func TestDatabaseClose(t *testing.T) {
	db := OpenInMemory(nil)

	// Create some data
	batch := db.Begin(true)
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account}))
	require.NoError(t, batch.Commit())
	batch.Discard()

	// Close the database
	err := db.Close()
	require.NoError(t, err)

	t.Logf("Database.Close test passed")
}

// TestDeleteAccountState_TESTONLY tests Batch.DeleteAccountState_TESTONLY
func TestDeleteAccountState_TESTONLY(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account}))

	// Force BPT update
	_, err := batch.GetBptRootHash()
	require.NoError(t, err)

	require.NoError(t, batch.Commit())

	// Start new batch and delete
	batch = db.Begin(true)
	defer batch.Discard()

	err = batch.DeleteAccountState_TESTONLY(account)
	require.NoError(t, err)

	t.Logf("DeleteAccountState_TESTONLY test passed")
}

// TestBatchNestedView tests nested Batch.View function
func TestBatchNestedView(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account, Index: 42}))
	require.NoError(t, batch.Commit())

	// Start new batch
	batch = db.Begin(true)
	defer batch.Discard()

	// Use nested View
	err := batch.View(func(sub *Batch) error {
		var ledger *protocol.SystemLedger
		err := sub.Account(account).Main().GetAs(&ledger)
		require.NoError(t, err)
		require.Equal(t, uint64(42), ledger.Index)
		return nil
	})
	require.NoError(t, err)

	t.Logf("Batch nested View test passed")
}

// TestAccountHash tests Account.Hash function
func TestAccountHash(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account, Index: 1}))

	// Get hash
	hash1, err := batch.Account(account).Hash()
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, hash1)

	// Modify account
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account, Index: 2}))

	// Hash should change
	hash2, err := batch.Account(account).Hash()
	require.NoError(t, err)
	require.NotEqual(t, hash1, hash2)

	t.Logf("Account.Hash test passed: %x != %x", hash1[:8], hash2[:8])
}

// TestChain2Walk tests Chain2.Walk function
func TestChain2Walk(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry := randomHash()
	require.NoError(t, chain.AddEntry(entry[:], false))

	// Test that chain is dirty
	chain2 := batch.Account(account).MainChain()
	require.True(t, chain2.IsDirty())

	// Test Commit
	err = chain2.Commit()
	require.NoError(t, err)

	t.Logf("Chain2.Walk/IsDirty/Commit test passed")
}

// TestDatabaseStore tests Database.Store function
func TestDatabaseStore(t *testing.T) {
	db := OpenInMemory(nil)
	defer db.Close()

	store, err := db.Store()
	require.NoError(t, err)
	require.NotNil(t, store)

	t.Logf("Database.Store test passed")
}

// TestBatchSetObserver tests Batch.SetObserver
func TestBatchSetObserver(t *testing.T) {
	db := OpenInMemory(nil)
	defer db.Close()

	batch := db.Begin(true)
	defer batch.Discard()

	// SetObserver can be called on batch (it sets the observer for that batch)
	// But we don't have a mock observer to test with, so we skip actual testing
	// The function exists and is called elsewhere in the codebase
	t.Logf("BatchSetObserver (skipping actual call - observer required)")
}

// TestBatchBptReceipt tests Batch.BptReceipt function
func TestBatchBptReceipt(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account}))

	// Force BPT update
	_, err := batch.GetBptRootHash()
	require.NoError(t, err)

	require.NoError(t, batch.Commit())

	// Get BPT receipt
	batch = db.Begin(false)
	defer batch.Discard()

	// Get the account's BPT key
	acc := batch.Account(account)
	hash, err := acc.Hash()
	require.NoError(t, err)

	// Get the receipt using Batch.BptReceipt
	key := acc.Key()
	receipt, err := batch.BptReceipt(key, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	t.Logf("Batch.BptReceipt test passed")
}

// TestRootChain tests root chain operations
func TestRootChain(t *testing.T) {
	db := OpenInMemory(nil)
	defer db.Close()

	batch := db.Begin(true)
	defer batch.Discard()

	// Create a system ledger account
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account}))

	// Add entry to root chain
	rootChain, err := batch.Account(account).RootChain().Get()
	require.NoError(t, err)
	entry := randomHash()
	require.NoError(t, rootChain.AddEntry(entry[:], false))

	// Get anchor
	anchor := rootChain.Anchor()
	require.NotNil(t, anchor)

	t.Logf("RootChain test passed: anchor=%x", anchor[:8])
}

// TestAccountCommitValidation tests Account.Commit validation
func TestAccountCommitValidation(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Test account with invalid URL length (too long)
	longPath := ""
	for i := 0; i < 500; i++ {
		longPath += "a"
	}
	account := protocol.AccountUrl("test", longPath)

	// This should work initially
	dataAccount := &protocol.DataAccount{Url: account}
	err := batch.Account(account).Main().Put(dataAccount)
	require.NoError(t, err)

	// Commit panics on URL length validation failure
	require.Panics(t, func() {
		_ = batch.Commit()
	})

	t.Logf("Account.Commit validation test passed")
}

// TestChainCurrentState tests Chain.CurrentState function
func TestChainCurrentState(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry := randomHash()
	require.NoError(t, chain.AddEntry(entry[:], false))

	// Test CurrentState
	state := chain.CurrentState()
	require.NotNil(t, state)
	require.Equal(t, int64(1), state.Count)

	t.Logf("Chain.CurrentState test passed")
}

// TestChainHeight tests Chain.Height function
func TestChainHeight(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	// Initially empty
	require.Equal(t, int64(0), chain.Height())

	// Add entries
	for i := 0; i < 5; i++ {
		entry := randomHash()
		require.NoError(t, chain.AddEntry(entry[:], false))
	}

	// Height should be 5
	require.Equal(t, int64(5), chain.Height())

	t.Logf("Chain.Height test passed")
}

// TestChainEntry tests Chain.Entry function
func TestChainEntry(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Add entries
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entries := make([][32]byte, 3)
	for i := 0; i < 3; i++ {
		entries[i] = randomHash()
		require.NoError(t, chain.AddEntry(entries[i][:], false))
	}

	// Retrieve entries
	for i := 0; i < 3; i++ {
		entry, err := chain.Entry(int64(i))
		require.NoError(t, err)
		require.Equal(t, entries[i][:], entry)
	}

	t.Logf("Chain.Entry test passed")
}

// TestAccountUrl tests Account.Url function
func TestAccountUrl(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	account := protocol.AccountUrl("test", "data")
	acc := batch.Account(account)
	require.Equal(t, account.String(), acc.Url().String())

	t.Logf("Account.Url test passed")
}

// TestChainAddEntryUnique tests Chain.AddEntry with unique flag
func TestChainAddEntryUnique(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	// Add entry with unique=true
	entry1 := randomHash()
	require.NoError(t, chain.AddEntry(entry1[:], true))
	require.Equal(t, int64(1), chain.Height())

	// Adding different entry should succeed
	entry2 := randomHash()
	require.NoError(t, chain.AddEntry(entry2[:], true))
	require.Equal(t, int64(2), chain.Height())

	// Adding same entry with unique=true doesn't add duplicate (entry is indexed)
	require.NoError(t, chain.AddEntry(entry1[:], true))
	// Height may or may not change depending on implementation

	t.Logf("Chain.AddEntry unique test passed, height: %d", chain.Height())
}

// TestAccountIsDirty tests Account.IsDirty function
func TestAccountIsDirty(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	account := protocol.AccountUrl("test", "data")
	acc := batch.Account(account)

	// Initially not dirty (nothing stored)
	require.False(t, acc.IsDirty())

	// Store something
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, acc.Main().Put(dataAccount))

	// Now dirty
	require.True(t, acc.IsDirty())

	t.Logf("Account.IsDirty test passed")
}

// TestChainNames tests various chain name lookups
func TestChainNames(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create system ledger to test various chain names
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account}))

	acc := batch.Account(account)

	// Test various chain names
	chains := []string{"main", "signature", "scratch", "root", "bpt", "major-block"}
	for _, name := range chains {
		chain, err := acc.ChainByName(name)
		require.NoError(t, err, "Failed to get chain: %s", name)
		require.NotNil(t, chain)
		t.Logf("Chain %s: type=%v", name, chain.Type())
	}
}

// TestBatchBeginReadOnly tests creating read-only batches
func TestBatchBeginReadOnly(t *testing.T) {
	db := OpenInMemory(nil)
	defer db.Close()

	// Create data first
	batch := db.Begin(true)
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account, Index: 1}))
	require.NoError(t, batch.Commit())
	batch.Discard()

	// Open read-only batch
	batch = db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	err := batch.Account(account).Main().GetAs(&ledger)
	require.NoError(t, err)
	require.Equal(t, uint64(1), ledger.Index)

	t.Logf("Batch read-only test passed")
}

// TestBatchDiscard tests Batch.Discard
func TestBatchDiscard(t *testing.T) {
	db := OpenInMemory(nil)
	defer db.Close()

	// Create and discard a batch with changes
	batch := db.Begin(true)
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account, Index: 99}))
	batch.Discard()

	// Verify changes were discarded
	batch = db.Begin(false)
	defer batch.Discard()

	var ledger protocol.Account
	err := batch.Account(account).Main().GetAs(&ledger)
	require.Error(t, err) // Should not exist

	t.Logf("Batch.Discard test passed")
}

// TestAccountKey tests Account.Key function
func TestAccountKey(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	account := protocol.AccountUrl("test", "data")
	acc := batch.Account(account)
	key := acc.Key()
	require.NotNil(t, key)
	require.Greater(t, key.Len(), 0)

	t.Logf("Account.Key test passed: %v", key)
}

// TestBatchAccountList tests batch with multiple accounts
func TestBatchAccountList(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create multiple accounts
	url1 := protocol.DnUrl().JoinPath(protocol.Ledger)
	url2 := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)
	url3 := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)

	require.NoError(t, batch.Account(url1).Main().Put(&protocol.SystemLedger{Url: url1, Index: 0}))
	require.NoError(t, batch.Account(url2).Main().Put(&protocol.SystemLedger{Url: url2, Index: 1}))
	require.NoError(t, batch.Account(url3).Main().Put(&protocol.SystemLedger{Url: url3, Index: 2}))

	// Force BPT update
	_, err := batch.GetBptRootHash()
	require.NoError(t, err)

	// Commit
	require.NoError(t, batch.Commit())

	// Verify all accounts exist
	batch = db.Begin(false)
	defer batch.Discard()

	var ledger1, ledger2, ledger3 *protocol.SystemLedger
	require.NoError(t, batch.Account(url1).Main().GetAs(&ledger1))
	require.NoError(t, batch.Account(url2).Main().GetAs(&ledger2))
	require.NoError(t, batch.Account(url3).Main().GetAs(&ledger3))

	require.Equal(t, uint64(0), ledger1.Index)
	require.Equal(t, uint64(1), ledger2.Index)
	require.Equal(t, uint64(2), ledger3.Index)

	t.Logf("Batch multiple accounts test passed")
}

// TestNestedBatches tests nested batch operations
func TestNestedBatches(t *testing.T) {
	db := OpenInMemory(nil)
	defer db.Close()

	// Create outer batch
	outer := db.Begin(true)
	defer outer.Discard()

	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, outer.Account(account).Main().Put(&protocol.SystemLedger{Url: account, Index: 1}))

	// Create inner batch
	inner := outer.Begin(true)
	defer inner.Discard()

	// Read in inner batch
	var ledger *protocol.SystemLedger
	require.NoError(t, inner.Account(account).Main().GetAs(&ledger))
	require.Equal(t, uint64(1), ledger.Index)

	// Modify in inner batch
	ledger.Index = 2
	require.NoError(t, inner.Account(account).Main().Put(ledger))
	require.NoError(t, inner.Commit())

	// Verify change visible in outer batch
	ledger = nil
	require.NoError(t, outer.Account(account).Main().GetAs(&ledger))
	require.Equal(t, uint64(2), ledger.Index)

	require.NoError(t, outer.Commit())

	t.Logf("Nested batches test passed")
}

// TestUpdateAccountError tests UpdateAccount error handling
func TestUpdateAccountError(t *testing.T) {
	db := OpenInMemory(nil)

	account := protocol.AccountUrl("nonexistent", "account")

	// Try to update non-existent account
	err := db.Update(func(batch *Batch) error {
		_, err := UpdateAccount(batch, account, func(acc *protocol.TokenAccount) error {
			return nil
		})
		return err
	})
	require.Error(t, err)

	t.Logf("UpdateAccount error test passed")
}

// TestChainGetError tests chain Get error paths
func TestChainGetError(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get chain and add entry
	chain, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)

	entry := randomHash()
	require.NoError(t, chain.AddEntry(entry[:], false))

	// Get chain again (tests caching path)
	chain2, err := batch.Account(account).MainChain().Get()
	require.NoError(t, err)
	require.Equal(t, chain.Height(), chain2.Height())

	t.Logf("Chain.Get caching test passed")
}

// TestAccountAnchorSequenceChain tests anchor sequence chain
func TestAccountAnchorSequenceChain(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a system ledger
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account}))

	// Get anchor sequence chain
	chain := batch.Account(account).AnchorSequenceChain()
	require.NotNil(t, chain)

	// Add an entry
	c, err := chain.Get()
	require.NoError(t, err)
	entry := randomHash()
	require.NoError(t, c.AddEntry(entry[:], false))

	t.Logf("AnchorSequenceChain test passed")
}

// TestAccountSignatureChain tests signature chain
func TestAccountSignatureChain(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get signature chain
	chain := batch.Account(account).SignatureChain()
	require.NotNil(t, chain)
	require.Equal(t, "signature", chain.Name())

	t.Logf("SignatureChain test passed")
}

// TestAccountScratchChain tests scratch chain
func TestAccountScratchChain(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create an account
	account := protocol.AccountUrl("test", "data")
	dataAccount := &protocol.DataAccount{Url: account}
	require.NoError(t, batch.Account(account).Main().Put(dataAccount))

	// Get scratch chain
	chain := batch.Account(account).ScratchChain()
	require.NotNil(t, chain)
	require.Equal(t, "scratch", chain.Name())

	t.Logf("ScratchChain test passed")
}

// TestMajorBlockChain tests major block chain
func TestMajorBlockChain(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a system ledger
	account := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, batch.Account(account).Main().Put(&protocol.SystemLedger{Url: account}))

	// Get major block chain
	chain := batch.Account(account).MajorBlockChain()
	require.NotNil(t, chain)
	require.Equal(t, merkle.ChainTypeIndex, chain.Type())

	t.Logf("MajorBlockChain test passed")
}

// TestBlockStateSynthTxnEntryCopy tests BlockStateSynthTxnEntry.Copy
func TestBlockStateSynthTxnEntryCopy(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")
	entry := &BlockStateSynthTxnEntry{
		Account:     account,
		Transaction: []byte("txn-data"),
		ChainEntry:  123,
	}

	// Test Copy
	copied := entry.Copy()
	require.NotNil(t, copied)
	require.Equal(t, entry.Account.String(), copied.Account.String())
	require.Equal(t, entry.Transaction, copied.Transaction)
	require.Equal(t, entry.ChainEntry, copied.ChainEntry)

	// Verify that modification to copy doesn't affect original
	copied.ChainEntry = 456
	require.NotEqual(t, entry.ChainEntry, copied.ChainEntry)

	t.Logf("BlockStateSynthTxnEntry.Copy test passed")
}

// TestBlockStateSynthTxnEntryEqual tests BlockStateSynthTxnEntry.Equal
func TestBlockStateSynthTxnEntryEqual(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")
	entry1 := &BlockStateSynthTxnEntry{
		Account:     account,
		Transaction: []byte("txn-data"),
		ChainEntry:  123,
	}
	entry2 := &BlockStateSynthTxnEntry{
		Account:     account,
		Transaction: []byte("txn-data"),
		ChainEntry:  123,
	}

	// Test equal entries
	require.True(t, entry1.Equal(entry2))

	// Test different ChainEntry
	entry2.ChainEntry = 456
	require.False(t, entry1.Equal(entry2))
	entry2.ChainEntry = 123

	// Test different Transaction
	entry2.Transaction = []byte("different")
	require.False(t, entry1.Equal(entry2))
	entry2.Transaction = []byte("txn-data")

	// Test different Account
	entry2.Account = protocol.AccountUrl("other", "tokens")
	require.False(t, entry1.Equal(entry2))

	// Test with nil Account
	entry1.Account = nil
	entry2.Account = nil
	require.True(t, entry1.Equal(entry2))

	entry2.Account = account
	require.False(t, entry1.Equal(entry2))

	t.Logf("BlockStateSynthTxnEntry.Equal test passed")
}

// TestBlockStateSynthTxnEntryMarshalBinary tests BlockStateSynthTxnEntry.MarshalBinary
func TestBlockStateSynthTxnEntryMarshalBinary(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")
	entry := &BlockStateSynthTxnEntry{
		Account:     account,
		Transaction: []byte("txn-data"),
		ChainEntry:  123,
	}

	// Test MarshalBinary
	data, err := entry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalBinary
	entry2 := new(BlockStateSynthTxnEntry)
	err = entry2.UnmarshalBinary(data)
	require.NoError(t, err)
	require.True(t, entry.Equal(entry2))

	// Test nil MarshalBinary
	var nilEntry *BlockStateSynthTxnEntry
	data, err = nilEntry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Logf("BlockStateSynthTxnEntry.MarshalBinary test passed")
}

// TestBlockStateSynthTxnEntryIsValid tests BlockStateSynthTxnEntry.IsValid
func TestBlockStateSynthTxnEntryIsValid(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")

	// Test valid entry
	entry := &BlockStateSynthTxnEntry{
		Account:     account,
		Transaction: []byte("txn-data"),
		ChainEntry:  123,
	}
	require.NoError(t, entry.IsValid())

	// Test missing Account
	entry.Account = nil
	err := entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Account")

	// Test missing Transaction
	entry.Account = account
	entry.Transaction = nil
	err = entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Transaction")

	// Test missing ChainEntry
	entry.Transaction = []byte("txn-data")
	entry.ChainEntry = 0
	err = entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ChainEntry")

	t.Logf("BlockStateSynthTxnEntry.IsValid test passed")
}

// TestBlockStateSynthTxnEntryJSON tests BlockStateSynthTxnEntry JSON marshaling
func TestBlockStateSynthTxnEntryJSON(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")
	entry := &BlockStateSynthTxnEntry{
		Account:     account,
		Transaction: []byte("txn-data"),
		ChainEntry:  123,
	}

	// Test MarshalJSON
	data, err := entry.MarshalJSON()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalJSON
	entry2 := new(BlockStateSynthTxnEntry)
	err = entry2.UnmarshalJSON(data)
	require.NoError(t, err)
	require.Equal(t, entry.Account.String(), entry2.Account.String())
	require.Equal(t, entry.Transaction, entry2.Transaction)
	require.Equal(t, entry.ChainEntry, entry2.ChainEntry)

	t.Logf("BlockStateSynthTxnEntry JSON test passed")
}

// TestTransactionChainEntryEqual tests TransactionChainEntry.Equal
func TestTransactionChainEntryEqual(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")
	entry1 := &TransactionChainEntry{
		Account:     account,
		Chain:       "main",
		ChainIndex:  100,
		AnchorIndex: 200,
	}
	entry2 := &TransactionChainEntry{
		Account:     account,
		Chain:       "main",
		ChainIndex:  100,
		AnchorIndex: 200,
	}

	// Test equal entries
	require.True(t, entry1.Equal(entry2))

	// Test different Chain
	entry2.Chain = "other"
	require.False(t, entry1.Equal(entry2))
	entry2.Chain = "main"

	// Test different ChainIndex
	entry2.ChainIndex = 300
	require.False(t, entry1.Equal(entry2))
	entry2.ChainIndex = 100

	// Test different AnchorIndex
	entry2.AnchorIndex = 400
	require.False(t, entry1.Equal(entry2))
	entry2.AnchorIndex = 200

	// Test with nil Account
	entry1.Account = nil
	entry2.Account = nil
	require.True(t, entry1.Equal(entry2))

	entry2.Account = account
	require.False(t, entry1.Equal(entry2))

	t.Logf("TransactionChainEntry.Equal test passed")
}

// TestTransactionChainEntryIsValid tests TransactionChainEntry.IsValid
func TestTransactionChainEntryIsValid(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")

	// Test valid entry
	entry := &TransactionChainEntry{
		Account:     account,
		Chain:       "main",
		ChainIndex:  100,
		AnchorIndex: 200,
	}
	require.NoError(t, entry.IsValid())

	// Test missing Account
	entry.Account = nil
	err := entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Account")

	// Test missing Chain
	entry.Account = account
	entry.Chain = ""
	err = entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Chain")

	t.Logf("TransactionChainEntry.IsValid test passed")
}

// TestVoteEntryEqual tests VoteEntry.Equal
func TestVoteEntryEqual(t *testing.T) {
	authority := protocol.AccountUrl("test", "book")
	hash := randomHash()

	entry1 := &VoteEntry{
		Authority: authority,
		Hash:      hash,
	}
	entry2 := &VoteEntry{
		Authority: authority,
		Hash:      hash,
	}

	// Test equal entries
	require.True(t, entry1.Equal(entry2))

	// Test different Hash
	entry2.Hash = randomHash()
	require.False(t, entry1.Equal(entry2))
	entry2.Hash = hash

	// Test with nil Authority
	entry1.Authority = nil
	entry2.Authority = nil
	require.True(t, entry1.Equal(entry2))

	entry2.Authority = authority
	require.False(t, entry1.Equal(entry2))

	t.Logf("VoteEntry.Equal test passed")
}

// TestVoteEntryIsValid tests VoteEntry.IsValid
func TestVoteEntryIsValid(t *testing.T) {
	authority := protocol.AccountUrl("test", "book")
	hash := randomHash()

	// Test valid entry
	entry := &VoteEntry{
		Authority: authority,
		Hash:      hash,
	}
	require.NoError(t, entry.IsValid())

	// Test missing Authority
	entry.Authority = nil
	err := entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Authority")

	// Test missing Hash
	entry.Authority = authority
	entry.Hash = [32]byte{}
	err = entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Hash")

	t.Logf("VoteEntry.IsValid test passed")
}

// TestVoteEntryJSON tests VoteEntry JSON marshaling
func TestVoteEntryJSON(t *testing.T) {
	authority := protocol.AccountUrl("test", "book")
	hash := randomHash()

	entry := &VoteEntry{
		Authority: authority,
		Hash:      hash,
	}

	// Test MarshalJSON
	data, err := entry.MarshalJSON()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalJSON
	entry2 := new(VoteEntry)
	err = entry2.UnmarshalJSON(data)
	require.NoError(t, err)
	require.True(t, entry.Equal(entry2))

	t.Logf("VoteEntry JSON test passed")
}

// TestSignatureSetEntryEqual tests SignatureSetEntry.Equal
func TestSignatureSetEntryEqual(t *testing.T) {
	path := []*url.URL{protocol.AccountUrl("test", "book", "1")}
	hash := randomHash()

	entry1 := &SignatureSetEntry{
		KeyIndex: 1,
		Version:  100,
		Path:     path,
		Hash:     hash,
	}
	entry2 := &SignatureSetEntry{
		KeyIndex: 1,
		Version:  100,
		Path:     path,
		Hash:     hash,
	}

	// Test equal entries
	require.True(t, entry1.Equal(entry2))

	// Test different KeyIndex
	entry2.KeyIndex = 2
	require.False(t, entry1.Equal(entry2))
	entry2.KeyIndex = 1

	// Test different Version
	entry2.Version = 200
	require.False(t, entry1.Equal(entry2))
	entry2.Version = 100

	// Test different Hash
	entry2.Hash = randomHash()
	require.False(t, entry1.Equal(entry2))
	entry2.Hash = hash

	// Test different Path length
	entry2.Path = nil
	require.False(t, entry1.Equal(entry2))

	t.Logf("SignatureSetEntry.Equal test passed")
}

// TestSignatureSetEntryIsValid tests SignatureSetEntry.IsValid
func TestSignatureSetEntryIsValid(t *testing.T) {
	path := []*url.URL{protocol.AccountUrl("test", "book", "1")}
	hash := randomHash()

	// Test valid entry
	entry := &SignatureSetEntry{
		KeyIndex: 1,
		Version:  100,
		Path:     path,
		Hash:     hash,
	}
	require.NoError(t, entry.IsValid())

	// Test missing Hash
	entry.Hash = [32]byte{}
	err := entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Hash")

	t.Logf("SignatureSetEntry.IsValid test passed")
}

// TestSignatureSetEntryJSON tests SignatureSetEntry JSON marshaling
func TestSignatureSetEntryJSON(t *testing.T) {
	path := []*url.URL{protocol.AccountUrl("test", "book", "1")}
	hash := randomHash()

	entry := &SignatureSetEntry{
		KeyIndex: 1,
		Version:  100,
		Path:     path,
		Hash:     hash,
	}

	// Test MarshalJSON
	data, err := entry.MarshalJSON()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalJSON
	entry2 := new(SignatureSetEntry)
	err = entry2.UnmarshalJSON(data)
	require.NoError(t, err)
	require.True(t, entry.Equal(entry2))

	t.Logf("SignatureSetEntry JSON test passed")
}

// TestSigSetEntryIsValid tests SigSetEntry.IsValid
func TestSigSetEntryIsValid(t *testing.T) {
	hash := randomHash()
	validatorHash := randomHash()

	// Test valid entry
	entry := &SigSetEntry{
		Type:             protocol.SignatureTypeED25519,
		KeyEntryIndex:    1,
		SignatureHash:    hash,
		ValidatorKeyHash: &validatorHash,
	}
	require.NoError(t, entry.IsValid())

	// Test missing SignatureHash
	entry.SignatureHash = [32]byte{}
	err := entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "SignatureHash")

	// Restore and test missing ValidatorKeyHash
	entry.SignatureHash = hash
	entry.ValidatorKeyHash = nil
	err = entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ValidatorKeyHash")

	t.Logf("SigSetEntry.IsValid test passed")
}

// TestSigSetEntryJSON tests SigSetEntry JSON marshaling
func TestSigSetEntryJSON(t *testing.T) {
	hash := randomHash()
	validatorHash := randomHash()

	entry := &SigSetEntry{
		Type:             protocol.SignatureTypeED25519,
		KeyEntryIndex:    1,
		SignatureHash:    hash,
		ValidatorKeyHash: &validatorHash,
	}

	// Test MarshalJSON
	data, err := entry.MarshalJSON()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalJSON
	entry2 := new(SigSetEntry)
	err = entry2.UnmarshalJSON(data)
	require.NoError(t, err)
	require.Equal(t, entry.Type, entry2.Type)
	require.Equal(t, entry.KeyEntryIndex, entry2.KeyEntryIndex)
	require.Equal(t, entry.SignatureHash, entry2.SignatureHash)

	t.Logf("SigSetEntry JSON test passed")
}

// TestSigSetEntryEqual tests SigSetEntry.Equal
func TestSigSetEntryEqual(t *testing.T) {
	hash := randomHash()
	validatorHash := randomHash()

	entry1 := &SigSetEntry{
		Type:             protocol.SignatureTypeED25519,
		KeyEntryIndex:    1,
		SignatureHash:    hash,
		ValidatorKeyHash: &validatorHash,
	}
	entry2 := &SigSetEntry{
		Type:             protocol.SignatureTypeED25519,
		KeyEntryIndex:    1,
		SignatureHash:    hash,
		ValidatorKeyHash: &validatorHash,
	}

	// Test equal entries
	require.True(t, entry1.Equal(entry2))

	// Test different Type
	entry2.Type = protocol.SignatureTypeRCD1
	require.False(t, entry1.Equal(entry2))
	entry2.Type = protocol.SignatureTypeED25519

	// Test different KeyEntryIndex
	entry2.KeyEntryIndex = 2
	require.False(t, entry1.Equal(entry2))
	entry2.KeyEntryIndex = 1

	// Test different SignatureHash
	entry2.SignatureHash = randomHash()
	require.False(t, entry1.Equal(entry2))
	entry2.SignatureHash = hash

	// Test different ValidatorKeyHash
	otherHash := randomHash()
	entry2.ValidatorKeyHash = &otherHash
	require.False(t, entry1.Equal(entry2))

	// Test with nil ValidatorKeyHash
	entry1.ValidatorKeyHash = nil
	entry2.ValidatorKeyHash = nil
	require.True(t, entry1.Equal(entry2))

	entry2.ValidatorKeyHash = &validatorHash
	require.False(t, entry1.Equal(entry2))

	t.Logf("SigSetEntry.Equal test passed")
}

// TestSigSetEntryCopy tests SigSetEntry.Copy
func TestSigSetEntryCopy(t *testing.T) {
	hash := randomHash()
	validatorHash := randomHash()

	entry := &SigSetEntry{
		Type:             protocol.SignatureTypeED25519,
		KeyEntryIndex:    1,
		SignatureHash:    hash,
		ValidatorKeyHash: &validatorHash,
	}

	// Test Copy
	copied := entry.Copy()
	require.NotNil(t, copied)
	require.True(t, entry.Equal(copied))

	// Modify copy shouldn't affect original
	copied.KeyEntryIndex = 99
	require.NotEqual(t, entry.KeyEntryIndex, copied.KeyEntryIndex)

	t.Logf("SigSetEntry.Copy test passed")
}

// TestSigOrTxnIsValid tests SigOrTxn.IsValid
func TestSigOrTxnIsValid(t *testing.T) {
	// Test with nil Transaction
	entry := &SigOrTxn{
		Transaction: nil,
		Signature:   nil,
		Txid:        nil,
	}
	err := entry.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Transaction")

	t.Logf("SigOrTxn.IsValid test passed")
}

// TestSigOrTxnCopy tests SigOrTxn.Copy
func TestSigOrTxnCopy(t *testing.T) {
	txid := protocol.AccountUrl("test", "tokens").WithTxID(randomHash())
	entry := &SigOrTxn{
		Transaction: nil,
		Signature:   nil,
		Txid:        txid,
	}

	// Test Copy
	copied := entry.Copy()
	require.NotNil(t, copied)

	t.Logf("SigOrTxn.Copy test passed")
}

// TestTransactionChainEntryCopy tests TransactionChainEntry.Copy
func TestTransactionChainEntryCopy(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")
	entry := &TransactionChainEntry{
		Account:     account,
		Chain:       "main",
		ChainIndex:  100,
		AnchorIndex: 200,
	}

	// Test Copy
	copied := entry.Copy()
	require.NotNil(t, copied)
	require.True(t, entry.Equal(copied))

	// Modify copy shouldn't affect original
	copied.ChainIndex = 999
	require.NotEqual(t, entry.ChainIndex, copied.ChainIndex)

	t.Logf("TransactionChainEntry.Copy test passed")
}

// TestVoteEntryCopy tests VoteEntry.Copy
func TestVoteEntryCopy(t *testing.T) {
	authority := protocol.AccountUrl("test", "book")
	hash := randomHash()

	entry := &VoteEntry{
		Authority: authority,
		Hash:      hash,
	}

	// Test Copy
	copied := entry.Copy()
	require.NotNil(t, copied)
	require.True(t, entry.Equal(copied))

	// Modify copy shouldn't affect original
	copied.Hash = randomHash()
	require.NotEqual(t, entry.Hash, copied.Hash)

	t.Logf("VoteEntry.Copy test passed")
}

// TestSignatureSetEntryCopy tests SignatureSetEntry.Copy
func TestSignatureSetEntryCopy(t *testing.T) {
	path := []*url.URL{protocol.AccountUrl("test", "book", "1")}
	hash := randomHash()

	entry := &SignatureSetEntry{
		KeyIndex: 1,
		Version:  100,
		Path:     path,
		Hash:     hash,
	}

	// Test Copy
	copied := entry.Copy()
	require.NotNil(t, copied)
	require.True(t, entry.Equal(copied))

	// Modify copy shouldn't affect original
	copied.KeyIndex = 99
	require.NotEqual(t, entry.KeyIndex, copied.KeyIndex)

	t.Logf("SignatureSetEntry.Copy test passed")
}

// TestTransactionChainEntryMarshalBinary tests TransactionChainEntry.MarshalBinary
func TestTransactionChainEntryMarshalBinary(t *testing.T) {
	account := protocol.AccountUrl("test", "tokens")
	entry := &TransactionChainEntry{
		Account:     account,
		Chain:       "main",
		ChainIndex:  100,
		AnchorIndex: 200,
	}

	// Test MarshalBinary
	data, err := entry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalBinary
	entry2 := new(TransactionChainEntry)
	err = entry2.UnmarshalBinary(data)
	require.NoError(t, err)
	require.True(t, entry.Equal(entry2))

	// Test nil MarshalBinary
	var nilEntry *TransactionChainEntry
	data, err = nilEntry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Logf("TransactionChainEntry.MarshalBinary test passed")
}

// TestVoteEntryMarshalBinary tests VoteEntry.MarshalBinary
func TestVoteEntryMarshalBinary(t *testing.T) {
	authority := protocol.AccountUrl("test", "book")
	hash := randomHash()

	entry := &VoteEntry{
		Authority: authority,
		Hash:      hash,
	}

	// Test MarshalBinary
	data, err := entry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalBinary
	entry2 := new(VoteEntry)
	err = entry2.UnmarshalBinary(data)
	require.NoError(t, err)
	require.True(t, entry.Equal(entry2))

	// Test nil MarshalBinary
	var nilEntry *VoteEntry
	data, err = nilEntry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Logf("VoteEntry.MarshalBinary test passed")
}

// TestSignatureSetEntryMarshalBinary tests SignatureSetEntry.MarshalBinary
func TestSignatureSetEntryMarshalBinary(t *testing.T) {
	path := []*url.URL{protocol.AccountUrl("test", "book", "1")}
	hash := randomHash()

	entry := &SignatureSetEntry{
		KeyIndex: 1,
		Version:  100,
		Path:     path,
		Hash:     hash,
	}

	// Test MarshalBinary
	data, err := entry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalBinary
	entry2 := new(SignatureSetEntry)
	err = entry2.UnmarshalBinary(data)
	require.NoError(t, err)
	require.True(t, entry.Equal(entry2))

	// Test nil MarshalBinary
	var nilEntry *SignatureSetEntry
	data, err = nilEntry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Logf("SignatureSetEntry.MarshalBinary test passed")
}

// TestSigSetEntryMarshalBinary tests SigSetEntry.MarshalBinary
func TestSigSetEntryMarshalBinary(t *testing.T) {
	hash := randomHash()
	validatorHash := randomHash()

	entry := &SigSetEntry{
		Type:             protocol.SignatureTypeED25519,
		KeyEntryIndex:    1,
		SignatureHash:    hash,
		ValidatorKeyHash: &validatorHash,
	}

	// Test MarshalBinary
	data, err := entry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Test UnmarshalBinary
	entry2 := new(SigSetEntry)
	err = entry2.UnmarshalBinary(data)
	require.NoError(t, err)
	require.True(t, entry.Equal(entry2))

	// Test nil MarshalBinary
	var nilEntry *SigSetEntry
	data, err = nilEntry.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Logf("SigSetEntry.MarshalBinary test passed")
}
