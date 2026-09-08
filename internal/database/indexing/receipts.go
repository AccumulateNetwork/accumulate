// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// LoadIndexEntryFromEnd loads the Nth-to-last entry from an index chain.
// LoadIndexEntryFromEnd will panic if the offset is zero. If the offset is
// greater than the chain height, LoadIndexEntryFromEnd returns nil, nil.
func LoadIndexEntryFromEnd(c *database.Chain2, offset uint64) (*protocol.IndexEntry, error) {
	if offset == 0 {
		panic("offset must be > 0")
	}

	// Load the chain
	chain, err := c.Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get account chain %s: %w", c.Name(), err)
	}

	if chain.Height() < int64(offset) {
		return nil, nil
	}
	index := uint64(chain.Height()) - offset

	// Load the entry
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(index), entry)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get account chain %s entry %d: %w", c.Name(), index, err)
	}

	return entry, nil
}

// LoadLastTwoIndexEntries loads the last and next to last entries of the index
// chain.
func LoadLastTwoIndexEntries(chain *database.Chain2) (last, nextLast *protocol.IndexEntry, err error) {
	last, err = LoadIndexEntryFromEnd(chain, 1)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	if last == nil {
		return
	}

	nextLast, err = LoadIndexEntryFromEnd(chain, 2)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	return
}

func getRootReceipt(partition config.NetworkUrl, batch *database.Batch, from, to int64) (*merkle.Receipt, error) {
	localChain, err := batch.Account(partition.Ledger()).RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get minor root chain: %w", err)
	}

	local, err := localChain.Receipt(from, to)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("unable to construct a receipt from %d to %d for the local root chain chain: %w", from, to, err)
	}

	// TODO Include the part of the receipt from the DN

	return local, nil
}

// loadIndexEntry loads an entry from an index chain.
func loadIndexEntry(c *database.Chain2, index uint64) (*protocol.IndexEntry, error) {
	// Load the chain
	chain, err := c.Index().Get()
	if err != nil {
		return nil, fmt.Errorf("unable to load the index chain of the %s chain: %w", c.Name(), err)
	}

	// Load the entry
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(index), entry)
	if err != nil {
		return nil, fmt.Errorf("unable to load index entry %d for the index chain of the %s chain: %w", index, c.Name(), err)
	}

	return entry, nil
}

// getIndexedChainReceipt locates a chain entry and gets a receipt from that
// entry to an indexed anchor.
func getIndexedChainReceipt(c *database.Chain2, chainEntry []byte, indexEntry *protocol.IndexEntry) (*merkle.Receipt, error) {
	// Load the chain
	chain, err := c.Get()
	if err != nil {
		return nil, fmt.Errorf("unable to load the %s chain: %w", c.Name(), err)
	}

	// Find the entry
	entryIndex, err := chain.HeightOf(chainEntry)
	if err != nil {
		return nil, fmt.Errorf("unable to find entry %X of the %s chain: %w", chainEntry, c.Name(), err)
	}

	// Get the receipt
	receipt, err := chain.Receipt(entryIndex, int64(indexEntry.Source))
	if err != nil {
		return nil, fmt.Errorf("unable to construct a receipt from %d to %d for chain %s: %w", entryIndex, indexEntry.Source, c.Name(), err)
	}

	return receipt, nil
}

func ReceiptForAccountState(partition config.NetworkUrl, batch *database.Batch, account *database.Account) (block *protocol.IndexEntry, receipt *merkle.Receipt, err error) {
	// Get a receipt from the BPT
	r, err := account.StateReceipt()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("get account state receipt: %w", err)
	}

	// Load the latest root index entry (just for the block index)
	ledger := batch.Account(partition.Ledger())
	rootEntry, err := LoadIndexEntryFromEnd(ledger.RootChain().Index(), 1)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	if rootEntry == nil {
		return nil, nil, errors.InternalError.With("root index chain is empty")
	}

	return rootEntry, r, nil
}

func ReceiptForChainEntry(net *config.Describe, batch *database.Batch, account *database.Account, hash []byte, entry *database.TransactionChainEntry) (*protocol.IndexEntry, *merkle.Receipt, error) {
	c, err := account.ChainByName(entry.Chain)
	if err != nil {
		return nil, nil, err
	}

	// Load the index entry
	accountIndex, err := loadIndexEntry(c, entry.ChainIndex)
	if err != nil {
		return nil, nil, err
	}

	// Load the root index entry
	ledger := batch.Account(net.Ledger())
	rootIndex, err := loadIndexEntry(ledger.RootChain(), entry.AnchorIndex)
	if err != nil {
		return nil, nil, err
	}

	// Get a receipt from the account's chain
	accountReceipt, err := getIndexedChainReceipt(c, hash, accountIndex)
	if err != nil {
		return nil, nil, err
	}

	// Get a receipt from the root chain
	rootReceipt, err := getRootReceipt(net.PartitionUrl(), batch, int64(accountIndex.Anchor), int64(rootIndex.Source))
	if err != nil {
		return nil, nil, err
	}

	// Finalize the receipt
	r, err := accountReceipt.Combine(rootReceipt)
	if err != nil {
		return nil, nil, err
	}

	return rootIndex, r, nil
}

func ReceiptForChainIndex(partition config.NetworkUrl, batch *database.Batch, c *database.Chain2, index int64, targetHeight *uint64) (*protocol.IndexEntry, uint64, *merkle.Receipt, error) {
	if c.Type() == merkle.ChainTypeIndex {
		return nil, 0, nil, errors.BadRequest.WithFormat("cannot get a receipt for %s: index chains are not anchored", c.Name())
	}
	var entry, rootEntry *protocol.IndexEntry
	var rootIndexIndex uint64
	indexChain, err := c.Index().Get()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("unable to load %s index chain: %w", c.Name(), err)
	}
	if indexChain.Height() == 0 {
		return nil, 0, nil, fmt.Errorf("cannot create receipt for entry %d of %s chain: index chain is empty", index, c.Name())
	}

	// The two positions this receipt needs -- which index-chain entry covers
	// this entry, and which root index entry covers that -- were both recorded
	// when the entry was written, in the transaction's chains index
	// (TransactionChainEntry.ChainIndex and .AnchorIndex). Read them rather
	// than search for them (#4263). Falls through to the search when the entry
	// is not a transaction, or its record is absent, or the caller asked for a
	// different height than the one the entry was anchored at.
	entry, rootIndexIndex, rootEntry, ok := storedAnchorPositions(partition, batch, c, index, targetHeight)
	if ok {
		return finishChainReceipt(partition, batch, c, index, entry, rootIndexIndex, rootEntry)
	}

	_, entry, err = SearchIndexChain(indexChain, uint64(indexChain.Height())-1, MatchAfter, SearchIndexChainBySource(uint64(index)))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("unable to locate index entry for entry %d of %s chain: %w", index, c.Name(), err)
	}

	chain, err := c.Get()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("unable to load %s chain: %w", c.Name(), err)
	}

	rootIndexChain, err := batch.Account(partition.Ledger()).RootChain().Index().Get()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("unable to load minor root index chain: %w", err)
	}
	if rootIndexChain.Height() == 0 {
		return nil, 0, nil, errors.InternalError.With("root index chain is empty")
	}

	if targetHeight == nil {
		targetHeight = &entry.Anchor
	} else if *targetHeight < entry.Anchor {
		return nil, 0, nil, fmt.Errorf("cannot satisfy target height %v: entry is anchored at height %v", *targetHeight, entry.Anchor)

	}

	rootIndexIndex, rootEntry, err = SearchIndexChain(rootIndexChain, uint64(rootIndexChain.Height())-1, MatchAfter, SearchIndexChainBySource(*targetHeight))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("unable to locate index entry for entry %d of the minor root chain: %w", entry.Anchor, err)
	}

	_ = chain
	return finishChainReceipt(partition, batch, c, index, entry, rootIndexIndex, rootEntry)
}

// storedAnchorPositions returns the index entries a receipt needs, read from
// the transaction's chains index instead of searched for. The second return is
// the root index entry's position, which the caller reports as
// rootIndexIndex. ok is false when the positions are not available and the
// caller must search.
func storedAnchorPositions(partition config.NetworkUrl, batch *database.Batch, c *database.Chain2, index int64, targetHeight *uint64) (*protocol.IndexEntry, uint64, *protocol.IndexEntry, bool) {
	// A receipt for a specific height is not the entry's own anchor, so the
	// stored position does not answer it
	if targetHeight != nil {
		return nil, 0, nil, false
	}

	// The chains index is keyed by the entry's hash, so the entry must be one
	chain, err := c.Get()
	if err != nil {
		return nil, 0, nil, false
	}
	hash, err := chain.Entry(index)
	if err != nil {
		return nil, 0, nil, false
	}

	entries, err := batch.Transaction(hash).Chains().Get()
	if err != nil {
		return nil, 0, nil, false
	}
	var rec *database.TransactionChainEntry
	for _, e := range entries {
		if e.Chain == c.Name() && e.Account.Equal(c.Account()) {
			rec = e
			break
		}
	}
	if rec == nil {
		return nil, 0, nil, false
	}

	entry, err := loadIndexEntry(c, rec.ChainIndex)
	if err != nil {
		return nil, 0, nil, false
	}
	// The recorded position must actually cover this entry; if it does not,
	// the record is stale or wrong and the search is the safe answer
	if entry.Source < uint64(index) {
		return nil, 0, nil, false
	}

	rootEntry, err := loadIndexEntry(batch.Account(partition.Ledger()).RootChain(), rec.AnchorIndex)
	if err != nil {
		return nil, 0, nil, false
	}
	return entry, rec.AnchorIndex, rootEntry, true
}

// finishChainReceipt builds the receipt once both index entries are known,
// however they were obtained.
func finishChainReceipt(partition config.NetworkUrl, batch *database.Batch, c *database.Chain2, index int64, entry *protocol.IndexEntry, rootIndexIndex uint64, rootEntry *protocol.IndexEntry) (*protocol.IndexEntry, uint64, *merkle.Receipt, error) {
	chain, err := c.Get()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("unable to load %s chain: %w", c.Name(), err)
	}

	// Get a receipt from the account's chain
	accountReceipt, err := chain.Receipt(index, int64(entry.Source))
	if err != nil {
		return nil, 0, nil, err
	}

	// Get a receipt from the root chain
	rootReceipt, err := getRootReceipt(partition, batch, int64(entry.Anchor), int64(rootEntry.Source))
	if err != nil {
		return nil, 0, nil, err
	}

	// Finalize the receipt
	r, err := accountReceipt.Combine(rootReceipt)
	if err != nil {
		return nil, 0, nil, err
	}

	return rootEntry, rootIndexIndex, r, nil
}
